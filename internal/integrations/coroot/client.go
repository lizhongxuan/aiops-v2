package coroot

import (
	"bytes"
	"context"
	"crypto/hmac"
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
	BaseURL          string
	ProductBasePath  string
	Token            string
	AuthMode         string
	EmbedTrustSecret string
	Project          string
	Timeout          time.Duration
	Client           *http.Client
}

type Client struct {
	baseURL          *url.URL
	token            string
	sessionCookie    string
	authMode         string
	embedTrustSecret string
	project          string
	httpClient       *http.Client
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
		httpClient = newCorootHTTPClient(timeout)
	}
	project := strings.TrimSpace(cfg.Project)
	if project == "" {
		project = "default"
	}
	return &Client{
		baseURL:          corootClientBaseURL(parsed, cfg.ProductBasePath),
		token:            strings.TrimSpace(cfg.Token),
		sessionCookie:    corootSessionCookieValue(cfg.Token),
		authMode:         strings.TrimSpace(cfg.AuthMode),
		embedTrustSecret: strings.TrimSpace(cfg.EmbedTrustSecret),
		project:          project,
		httpClient:       httpClient,
	}, nil
}

func corootClientBaseURL(parsed *url.URL, productBasePath string) *url.URL {
	if parsed == nil {
		return nil
	}
	cp := *parsed
	product := corootClientProductBasePath(productBasePath)
	if product == "" {
		return &cp
	}
	base := strings.TrimRight(strings.TrimSpace(cp.Path), "/")
	if base == "." || base == "/" {
		base = ""
	}
	switch {
	case base == "":
		cp.Path = product
	case base == product || strings.HasSuffix(base, product):
		cp.Path = base
	default:
		cp.Path = base + product
	}
	cp.RawPath = ""
	return &cp
}

func corootClientProductBasePath(raw string) string {
	product := strings.Trim(strings.TrimSpace(raw), "/")
	if product == "" || product == "." {
		return ""
	}
	return "/" + product
}

func newCorootHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = corootProxyFromEnvironment
	return &http.Client{Timeout: timeout, Transport: transport}
}

func corootProxyFromEnvironment(req *http.Request) (*url.URL, error) {
	if req != nil && req.URL != nil && corootShouldBypassProxy(req.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func corootShouldBypassProxy(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
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
		BaseURL:         baseURL,
		ProductBasePath: firstNonEmpty("AIOPS_COROOT_PRODUCT_BASE_PATH", "COROOT_PRODUCT_BASE_PATH"),
		Token:           firstNonEmpty("AIOPS_COROOT_TOKEN", "COROOT_TOKEN"),
		Project:         firstNonEmpty("AIOPS_COROOT_PROJECT", "COROOT_PROJECT"),
		Timeout:         timeout,
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
		if strings.EqualFold(project, "default") {
			if configured := c.DefaultProject(); !strings.EqualFold(configured, "default") {
				return configured
			}
		}
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
	c.applyAuthHeaders(req.Header)

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
	if corootLooksLikeHTMLResponse(resp.Header.Get("Content-Type"), body) {
		return nil, &CorootError{
			Kind:       "authentication_required",
			StatusCode: resp.StatusCode,
			URI:        target.String(),
			Message:    "coroot returned an HTML page instead of JSON; the backend evidence client is not authenticated or the API product path is wrong",
		}
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

func (c *Client) CheckHealth(ctx context.Context) (*CorootRawRef, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return nil, &CorootError{Kind: "not_configured", Message: "coroot client is not configured"}
	}
	target := *c.baseURL
	target.Path = healthPath()
	target.RawPath = ""
	target.RawQuery = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, &CorootError{Kind: "bad_request", URI: target.String(), Message: err.Error()}
	}
	req.Header.Set("Accept", "*/*")
	c.applyAuthHeaders(req.Header)

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

	sum := sha256.Sum256(body)
	return &CorootRawRef{
		URI:    target.String(),
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
		Bytes:  int64(len(body)),
	}, nil
}

func (c *Client) Login(ctx context.Context, email string, password string) error {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return &CorootError{Kind: "not_configured", Message: "coroot client is not configured"}
	}
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return &CorootError{Kind: "bad_config", Message: "Coroot username and password are required for Web session authentication"}
	}
	target := *c.baseURL
	target.Path = joinURLPath(c.baseURL.Path, "/api/login")
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return &CorootError{Kind: "bad_request", URI: target.String(), Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		kind := "transport_error"
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			kind = "timeout"
		}
		return &CorootError{Kind: kind, URI: target.String(), Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &CorootError{Kind: "authentication_required", StatusCode: resp.StatusCode, URI: target.String(), Message: trimBodyForError(body)}
	}
	for _, cookie := range resp.Cookies() {
		if cookie != nil && cookie.Name == "coroot_session" && strings.TrimSpace(cookie.Value) != "" {
			c.sessionCookie = strings.TrimSpace(cookie.Value)
			return nil
		}
	}
	return &CorootError{Kind: "authentication_required", StatusCode: resp.StatusCode, URI: target.String(), Message: "coroot login did not return coroot_session cookie"}
}

func (c *Client) applyAuthHeaders(header http.Header) {
	if header == nil || c == nil {
		return
	}
	if strings.TrimSpace(c.authMode) == "embed_trust" && strings.TrimSpace(c.embedTrustSecret) != "" {
		for key, values := range signedCorootClientEmbedHeaders(c.embedTrustSecret, time.Now()) {
			for _, value := range values {
				header.Add(key, value)
			}
		}
		return
	}
	if cookie := strings.TrimSpace(c.sessionCookie); cookie != "" {
		header.Set("Cookie", (&http.Cookie{Name: "coroot_session", Value: cookie}).String())
	}
	token := strings.TrimSpace(c.token)
	if token == "" || corootTokenLooksLikeSessionCookie(token) {
		return
	}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Api-Key", token)
}

func corootSessionCookieValue(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(token), "cookie:") {
		token = strings.TrimSpace(token[len("cookie:"):])
	}
	for _, part := range strings.Split(token, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "coroot_session=") {
			return strings.TrimSpace(part[len("coroot_session="):])
		}
	}
	if strings.Contains(token, ".") && !strings.ContainsAny(token, " \t\r\n;") {
		return token
	}
	return ""
}

func corootTokenLooksLikeSessionCookie(token string) bool {
	return corootSessionCookieValue(token) != ""
}

func signedCorootClientEmbedHeaders(secret string, now time.Time) http.Header {
	header := http.Header{}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return header
	}
	user := "aiops-v2"
	roles := "coroot-readonly"
	tenant := "default"
	timestamp := now.UTC().Format(time.RFC3339)
	payload := user + "\n" + roles + "\n" + tenant + "\n" + timestamp
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	header.Set("X-Aiops-Embed-User", user)
	header.Set("X-Aiops-Embed-Roles", roles)
	header.Set("X-Aiops-Embed-Tenant", tenant)
	header.Set("X-Aiops-Embed-Timestamp", timestamp)
	header.Set("X-Aiops-Embed-Signature", hex.EncodeToString(mac.Sum(nil)))
	return header
}

func corootLooksLikeHTMLResponse(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	preview := string(trimmed)
	if len(preview) > 80 {
		preview = preview[:80]
	}
	preview = strings.ToLower(preview)
	return strings.HasPrefix(preview, "<!doctype html") ||
		strings.HasPrefix(preview, "<html") ||
		strings.HasPrefix(preview, "<")
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
