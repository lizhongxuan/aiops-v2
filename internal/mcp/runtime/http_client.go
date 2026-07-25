package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
	mu      sync.RWMutex
	version string
	session string
	caps    json.RawMessage
	legacy  bool
	seq     atomic.Uint64
}

type ProtocolError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("MCP protocol error %d: %s", e.Code, e.Message)
}

type httpStatusError struct {
	Code   int
	Status string
}

func (e *httpStatusError) Error() string { return "MCP HTTP transport error: " + e.Status }

func (c *httpClient) Initialize(ctx context.Context, request InitializeRequest) (ClientSession, error) {
	versions := normalizedProtocolVersions(request.SupportedProtocolVersions)
	if len(versions) == 0 {
		return ClientSession{}, fmt.Errorf("MCP initialize requires a supported protocol version")
	}
	capabilities := request.Capabilities
	if len(capabilities) == 0 {
		capabilities = json.RawMessage(`{}`)
	}
	clientInfo := request.ClientInfo
	if strings.TrimSpace(clientInfo.Name) == "" {
		clientInfo = ImplementationInfo{Name: "aiops-v2", Version: "dev"}
	}
	raw, header, err := c.rpc(ctx, "initialize", "initialize", map[string]any{
		"protocolVersion": versions[0],
		"capabilities":    capabilities,
		"clientInfo":      clientInfo,
	})
	if isLegacyInitializeFailure(err) {
		c.mu.Lock()
		c.legacy = true
		c.mu.Unlock()
		return ClientSession{ProtocolVersion: "legacy-http", Legacy: true}, nil
	}
	if err != nil {
		return ClientSession{}, err
	}
	var result struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ServerInfo      json.RawMessage `json:"serverInfo"`
		Instructions    string          `json:"instructions"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return ClientSession{}, fmt.Errorf("decode MCP initialize result: %w", err)
	}
	if !containsProtocolVersion(versions, result.ProtocolVersion) {
		return ClientSession{}, fmt.Errorf("unsupported MCP protocol version %q", result.ProtocolVersion)
	}
	var capabilityObject map[string]json.RawMessage
	if len(result.Capabilities) == 0 || json.Unmarshal(result.Capabilities, &capabilityObject) != nil {
		return ClientSession{}, fmt.Errorf("decode MCP initialize result: capabilities must be an object")
	}
	sessionID := header.Get("Mcp-Session-Id")
	if !validMCPSessionID(sessionID) {
		return ClientSession{}, fmt.Errorf("invalid MCP session id")
	}
	c.mu.Lock()
	c.version, c.session = result.ProtocolVersion, sessionID
	c.caps = append(json.RawMessage(nil), result.Capabilities...)
	c.legacy = false
	c.mu.Unlock()
	return ClientSession{ProtocolVersion: result.ProtocolVersion, Capabilities: append(json.RawMessage(nil), result.Capabilities...), ServerInfo: append(json.RawMessage(nil), result.ServerInfo...), Instructions: result.Instructions, SessionID: sessionID}, nil
}

func (c *httpClient) NotifyInitialized(ctx context.Context) error {
	if c.isLegacy() {
		return nil
	}
	if !c.isNegotiated() {
		return fmt.Errorf("MCP client is not initialized")
	}
	_, _, err := c.rpc(ctx, "", "notifications/initialized", map[string]any{})
	return err
}

func (c *httpClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	type toolsPayload struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if c.isNegotiated() {
		if !c.hasCapability("tools") {
			return nil, nil
		}
		raw, _, err := c.rpc(ctx, "tools-list", "tools/list", map[string]any{})
		if err != nil {
			return nil, err
		}
		var payload toolsPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode MCP tools/list result: %w", err)
		}
		return payload.Tools, nil
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
	if err := ctx.Err(); err != nil {
		return ToolCallResult{}, err
	}
	if c.isNegotiated() && !c.hasCapability("tools") {
		return ToolCallResult{}, fmt.Errorf("MCP server did not negotiate tools capability")
	}
	token, ok := MCPRequestTokenFromContext(ctx)
	if !ok {
		token = fmt.Sprintf("tools-call-%d", c.seq.Add(1))
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	raw, _, err := c.rpc(ctx, token, "tools/call", map[string]any{
		"name":      strings.TrimSpace(name),
		"arguments": json.RawMessage(input),
		"_meta":     map[string]any{"progressToken": token},
	})
	if ctx.Err() != nil {
		cancelErr := c.notifyCancelled(token, context.Cause(ctx))
		if cancelErr != nil {
			return ToolCallResult{}, fmt.Errorf("%w; send MCP cancellation: %v", ctx.Err(), cancelErr)
		}
		return ToolCallResult{}, ctx.Err()
	}
	if err != nil {
		return ToolCallResult{}, err
	}
	if !json.Valid(raw) && !c.isNegotiated() {
		return ToolCallResult{Content: string(raw)}, nil
	}
	var result ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ToolCallResult{}, fmt.Errorf("decode MCP tools/call result: %w", err)
	}
	return result, nil
}

func (c *httpClient) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	if c.isNegotiated() {
		if !c.hasCapability("resources") {
			return nil, nil
		}
		raw, _, err := c.rpc(ctx, "resources-list", "resources/list", map[string]any{})
		if err != nil {
			return nil, err
		}
		var payload struct {
			Resources []mcp.Resource `json:"resources"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode MCP resources/list result: %w", err)
		}
		return payload.Resources, nil
	}
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
	if c.isNegotiated() && !c.hasCapability("resources") {
		return mcp.ResourceContent{}, fmt.Errorf("MCP server did not negotiate resources capability")
	}
	raw, _, err := c.rpc(ctx, "resource-read", "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	if !json.Valid(raw) && !c.isNegotiated() {
		return mcp.ResourceContent{Text: string(raw)}, nil
	}
	var result mcp.ResourceContent
	if err := json.Unmarshal(raw, &result); err != nil {
		return mcp.ResourceContent{}, fmt.Errorf("decode MCP resources/read result: %w", err)
	}
	return result, nil
}

func (c *httpClient) rpc(ctx context.Context, id, method string, params any) (json.RawMessage, http.Header, error) {
	payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id != "" {
		payload["id"] = id
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if method != "initialize" {
		version, session := c.negotiatedHeaders()
		if version != "" {
			req.Header.Set("MCP-Protocol-Version", version)
		}
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if protocolErr := decodeMCPProtocolError(body); protocolErr != nil {
			return nil, resp.Header, protocolErr
		}
		return nil, resp.Header, &httpStatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	if id == "" {
		return nil, resp.Header, nil
	}
	strict := method == "initialize" || c.isNegotiated()
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		if !strict {
			return append(json.RawMessage(nil), body...), resp.Header, nil
		}
		return nil, resp.Header, fmt.Errorf("decode MCP JSON-RPC response: %w", err)
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		var protocolErr ProtocolError
		if err := json.Unmarshal(envelope.Error, &protocolErr); err != nil {
			return nil, resp.Header, fmt.Errorf("decode MCP JSON-RPC error: %w", err)
		}
		return nil, resp.Header, &protocolErr
	}
	if len(envelope.Result) == 0 {
		if !strict {
			return append(json.RawMessage(nil), body...), resp.Header, nil
		}
		return nil, resp.Header, fmt.Errorf("MCP protocol response is missing result")
	}
	if strict {
		var responseID string
		if envelope.JSONRPC != "2.0" || json.Unmarshal(envelope.ID, &responseID) != nil || responseID != id {
			return nil, resp.Header, fmt.Errorf("MCP protocol response id does not match request")
		}
	}
	return append(json.RawMessage(nil), envelope.Result...), resp.Header, nil
}

func (c *httpClient) notifyCancelled(requestID string, cause error) error {
	if !c.isNegotiated() {
		return nil
	}
	reason := "request cancelled"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		reason = strings.TrimSpace(cause.Error())
	}
	if runes := []rune(reason); len(runes) > 256 {
		reason = string(runes[:256])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.rpc(ctx, "", "notifications/cancelled", map[string]any{"requestId": requestID, "reason": reason})
	return err
}

func (c *httpClient) negotiatedHeaders() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version, c.session
}

func (c *httpClient) isNegotiated() bool {
	version, _ := c.negotiatedHeaders()
	return version != ""
}

func (c *httpClient) isLegacy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.legacy
}

func (c *httpClient) hasCapability(name string) bool {
	c.mu.RLock()
	raw := append(json.RawMessage(nil), c.caps...)
	c.mu.RUnlock()
	var capabilities map[string]json.RawMessage
	if json.Unmarshal(raw, &capabilities) != nil {
		return false
	}
	_, found := capabilities[name]
	return found
}

func normalizedProtocolVersions(values []string) []string {
	versions := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if isSupportedMCPProtocolVersion(value) && !containsProtocolVersion(versions, value) {
			versions = append(versions, value)
		}
	}
	return versions
}

func containsProtocolVersion(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validMCPSessionID(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func decodeMCPProtocolError(body []byte) *ProtocolError {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Error) == 0 {
		return nil
	}
	var protocolErr ProtocolError
	if json.Unmarshal(envelope.Error, &protocolErr) != nil || protocolErr.Message == "" {
		return nil
	}
	return &protocolErr
}

func isLegacyInitializeFailure(err error) bool {
	switch typed := err.(type) {
	case *httpStatusError:
		return typed.Code == http.StatusNotFound || typed.Code == http.StatusMethodNotAllowed
	case *ProtocolError:
		return typed.Code == -32601
	default:
		return false
	}
}

func (c *httpClient) Close(context.Context) error { return nil }
