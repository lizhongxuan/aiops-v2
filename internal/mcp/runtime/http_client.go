package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/mcp"
)

type DefaultClientFactory struct {
	HTTPClient *http.Client
}

func (f DefaultClientFactory) NewClient(_ context.Context, cfg mcp.ServerConfig) (Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Transport)) {
	case "http":
		if len(cfg.Command) == 0 || strings.TrimSpace(cfg.Command[0]) == "" {
			return nil, fmt.Errorf("mcp runtime: http server %q requires url", cfg.ID)
		}
		client := f.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 10 * time.Second}
		}
		return &httpClient{baseURL: strings.TrimRight(strings.TrimSpace(cfg.Command[0]), "/"), client: client}, nil
	case "stdio":
		return nil, fmt.Errorf("mcp runtime: stdio transport for %q is not available in this runtime connector", cfg.ID)
	default:
		return nil, fmt.Errorf("mcp runtime: unsupported transport %q for %q", cfg.Transport, cfg.ID)
	}
}

type httpClient struct {
	baseURL string
	client  *http.Client
}

func (c *httpClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	type toolsPayload struct {
		Tools []ToolDefinition `json:"tools"`
	}
	attempts := []struct {
		method string
		url    string
		body   io.Reader
	}{
		{method: http.MethodGet, url: c.baseURL + "/tools"},
		{method: http.MethodGet, url: c.baseURL},
		{method: http.MethodPost, url: c.baseURL, body: bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":"list-tools","method":"tools/list","params":{}}`))},
	}
	var lastErr error
	for _, attempt := range attempts {
		req, err := http.NewRequestWithContext(ctx, attempt.method, attempt.url, attempt.body)
		if err != nil {
			lastErr = err
			continue
		}
		if attempt.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("list tools failed: %s", resp.Status)
			continue
		}
		var wrapped toolsPayload
		if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Tools) > 0 {
			return wrapped.Tools, nil
		}
		var rpc struct {
			Result toolsPayload `json:"result"`
		}
		if err := json.Unmarshal(data, &rpc); err == nil && len(rpc.Result.Tools) > 0 {
			return rpc.Result.Tools, nil
		}
		var direct []ToolDefinition
		if err := json.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
			return direct, nil
		}
		lastErr = fmt.Errorf("no tools discovered")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no tools discovered")
	}
	return nil, lastErr
}

func (c *httpClient) CallTool(ctx context.Context, name string, input json.RawMessage) (ToolCallResult, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-tool",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": json.RawMessage(input),
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ToolCallResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return ToolCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return ToolCallResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolCallResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolCallResult{}, fmt.Errorf("call tool failed: %s", resp.Status)
	}
	var wrapped struct {
		Result ToolCallResult `json:"result"`
		Error  any            `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && (wrapped.Result.Content != "" || wrapped.Result.Error != "" || len(wrapped.Result.Raw) > 0) {
		return wrapped.Result, nil
	}
	var direct ToolCallResult
	if err := json.Unmarshal(body, &direct); err == nil {
		if direct.Content != "" || direct.Error != "" || len(direct.Raw) > 0 {
			return direct, nil
		}
	}
	return ToolCallResult{Content: string(body)}, nil
}

func (c *httpClient) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/resources", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list resources failed: %s", resp.Status)
	}
	var wrapped struct {
		Resources []mcp.Resource `json:"resources"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil {
		return wrapped.Resources, nil
	}
	var direct []mcp.Resource
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct, nil
	}
	return nil, nil
}

func (c *httpClient) ReadResource(ctx context.Context, uri string) (mcp.ResourceContent, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      "read-resource",
		"method":  "resources/read",
		"params": map[string]any{
			"uri": uri,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcp.ResourceContent{}, fmt.Errorf("read resource failed: %s", resp.Status)
	}
	var wrapped struct {
		Result mcp.ResourceContent `json:"result"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && (wrapped.Result.Text != "" || len(wrapped.Result.Blob) > 0) {
		return wrapped.Result, nil
	}
	var direct mcp.ResourceContent
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}
	return mcp.ResourceContent{Text: string(body)}, nil
}

func (c *httpClient) Close(context.Context) error { return nil }
