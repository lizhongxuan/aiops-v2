package coroot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxCorootResponseBytes = 10 << 20

type ClientConfig struct {
	BaseURL string
	Token   string
	Project string
	Timeout time.Duration
	Client  *http.Client
}

type Client struct {
	baseURL    *url.URL
	token      string
	project    string
	httpClient *http.Client
}

type CorootError struct {
	Kind       string
	StatusCode int
	URI        string
	Message    string
}

func (e *CorootError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{"coroot", e.Kind}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("coroot: base url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("coroot: invalid base url %q", baseURL)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	httpClient := cfg.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	project := strings.TrimSpace(cfg.Project)
	if project == "" {
		project = "default"
	}
	return &Client{
		baseURL:    parsed,
		token:      strings.TrimSpace(cfg.Token),
		project:    project,
		httpClient: httpClient,
	}, nil
}

func ClientConfigFromEnv(endpoint string) ClientConfig {
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("AIOPS_COROOT_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	baseURL := strings.TrimSpace(endpoint)
	if baseURL == "" {
		baseURL = firstNonEmpty("AIOPS_COROOT_BASE_URL", "COROOT_BASE_URL")
	}
	return ClientConfig{
		BaseURL: baseURL,
		Token:   firstNonEmpty("AIOPS_COROOT_TOKEN", "COROOT_TOKEN"),
		Project: firstNonEmpty("AIOPS_COROOT_PROJECT", "COROOT_PROJECT"),
		Timeout: timeout,
	}
}

func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

func (c *Client) DefaultProject() string {
	if c == nil || strings.TrimSpace(c.project) == "" {
		return "default"
	}
	return c.project
}

func (c *Client) ResolveProject(project string) string {
	if project = strings.TrimSpace(project); project != "" {
		return project
	}
	return c.DefaultProject()
}

func (c *Client) GetJSON(ctx context.Context, apiPath string, query url.Values, out any) (*CorootRawRef, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return nil, &CorootError{Kind: "not_configured", Message: "coroot client is not configured"}
	}
	target := *c.baseURL
	target.Path = joinURLPath(c.baseURL.Path, apiPath)
	target.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, &CorootError{Kind: "bad_request", URI: target.String(), Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		kind := "transport_error"
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			kind = "timeout"
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			kind = "timeout"
		}
		return nil, &CorootError{Kind: kind, URI: target.String(), Message: err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCorootResponseBytes+1))
	if err != nil {
		return nil, &CorootError{Kind: "read_error", StatusCode: resp.StatusCode, URI: target.String(), Message: err.Error()}
	}
	if len(body) > maxCorootResponseBytes {
		return nil, &CorootError{Kind: "response_too_large", StatusCode: resp.StatusCode, URI: target.String(), Message: "coroot response exceeded 10MiB"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		kind := "upstream_error"
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			kind = "upstream_client_error"
		}
		if resp.StatusCode >= 500 {
			kind = "upstream_server_error"
		}
		return nil, &CorootError{Kind: kind, StatusCode: resp.StatusCode, URI: target.String(), Message: trimBodyForError(body)}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, &CorootError{Kind: "empty_response", StatusCode: resp.StatusCode, URI: target.String(), Message: "coroot returned an empty response"}
	}

	payload := unwrapCorootData(body)
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return nil, &CorootError{Kind: "decode_error", StatusCode: resp.StatusCode, URI: target.String(), Message: err.Error()}
		}
	}

	sum := sha256.Sum256(body)
	return &CorootRawRef{
		URI:    target.String(),
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
		Bytes:  int64(len(body)),
	}, nil
}

func unwrapCorootData(body []byte) json.RawMessage {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return append(json.RawMessage(nil), body...)
	}
	if data, ok := root["data"]; ok && len(bytes.TrimSpace(data)) > 0 && string(bytes.TrimSpace(data)) != "null" {
		return append(json.RawMessage(nil), data...)
	}
	return append(json.RawMessage(nil), body...)
}

func joinURLPath(basePath, apiPath string) string {
	base := strings.TrimRight(strings.TrimSpace(basePath), "/")
	part := "/" + strings.TrimLeft(strings.TrimSpace(apiPath), "/")
	if base == "" {
		return part
	}
	return base + part
}

func trimBodyForError(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500] + "..."
	}
	return text
}

func firstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
