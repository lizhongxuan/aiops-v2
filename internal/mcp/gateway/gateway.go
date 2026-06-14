package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/mcp"
	mcpruntime "aiops-v2/internal/mcp/runtime"
)

type GatewayOptions struct {
	HTTPClient *http.Client
}

type Gateway struct {
	mu        sync.RWMutex
	client    *http.Client
	endpoints map[string]string
	stdio     map[string]*stdioSession
}

type GatewayConnection struct {
	ServerID  string
	Tools     []mcpruntime.ToolDefinition
	Resources []mcp.Resource
}

type MCPToolCallRequest struct {
	ServerID  string          `json:"serverId"`
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type MCPResourceReadRequest struct {
	ServerID string `json:"serverId"`
	URI      string `json:"uri"`
}

func NewGateway(opts GatewayOptions) *Gateway {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Gateway{
		client:    client,
		endpoints: map[string]string{},
		stdio:     map[string]*stdioSession{},
	}
}

func (g *Gateway) Connect(ctx context.Context, cfg ServerConfigV2) (GatewayConnection, error) {
	if g == nil {
		return GatewayConnection{}, fmt.Errorf("mcp gateway is nil")
	}
	serverID := strings.TrimSpace(cfg.ID)
	if serverID == "" {
		return GatewayConnection{}, fmt.Errorf("mcp gateway: server id is required")
	}
	if endpoint := endpointURL(cfg); endpoint != "" {
		return g.connectHTTP(ctx, serverID, endpoint)
	}
	if cfg.Stdio != nil {
		return g.connectStdio(ctx, serverID, *cfg.Stdio)
	}
	return GatewayConnection{}, fmt.Errorf("mcp gateway: streamable http endpoint or stdio command is required for %q", serverID)
}

func (g *Gateway) connectHTTP(ctx context.Context, serverID, endpoint string) (GatewayConnection, error) {
	if _, err := g.rpc(ctx, endpoint, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"clientInfo": map[string]string{
			"name":    "aiops-v2",
			"version": "dev",
		},
	}); err != nil {
		return GatewayConnection{}, err
	}

	var toolsPayload struct {
		Tools []mcpruntime.ToolDefinition `json:"tools"`
	}
	if err := g.rpcInto(ctx, endpoint, "tools/list", map[string]any{}, &toolsPayload); err != nil {
		return GatewayConnection{}, err
	}

	var resourcesPayload struct {
		Resources []mcp.Resource `json:"resources"`
	}
	if err := g.rpcInto(ctx, endpoint, "resources/list", map[string]any{}, &resourcesPayload); err != nil {
		return GatewayConnection{}, err
	}

	g.mu.Lock()
	g.endpoints[serverID] = endpoint
	delete(g.stdio, serverID)
	g.mu.Unlock()

	return GatewayConnection{
		ServerID:  serverID,
		Tools:     toolsPayload.Tools,
		Resources: resourcesPayload.Resources,
	}, nil
}

func (g *Gateway) connectStdio(ctx context.Context, serverID string, cfg StdioConfig) (GatewayConnection, error) {
	session, err := startStdioSession(ctx, cfg)
	if err != nil {
		return GatewayConnection{}, err
	}
	if _, err := session.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"clientInfo": map[string]string{
			"name":    "aiops-v2",
			"version": "dev",
		},
	}); err != nil {
		_ = session.close(ctx)
		return GatewayConnection{}, err
	}

	var toolsPayload struct {
		Tools []mcpruntime.ToolDefinition `json:"tools"`
	}
	if err := session.rpcInto(ctx, "tools/list", map[string]any{}, &toolsPayload); err != nil {
		_ = session.close(ctx)
		return GatewayConnection{}, err
	}
	var resourcesPayload struct {
		Resources []mcp.Resource `json:"resources"`
	}
	if err := session.rpcInto(ctx, "resources/list", map[string]any{}, &resourcesPayload); err != nil {
		_ = session.close(ctx)
		return GatewayConnection{}, err
	}

	g.mu.Lock()
	delete(g.endpoints, serverID)
	if previous := g.stdio[serverID]; previous != nil {
		_ = previous.close(ctx)
	}
	g.stdio[serverID] = session
	g.mu.Unlock()

	return GatewayConnection{
		ServerID:  serverID,
		Tools:     toolsPayload.Tools,
		Resources: resourcesPayload.Resources,
	}, nil
}

func (g *Gateway) CallTool(ctx context.Context, req MCPToolCallRequest) (mcpruntime.ToolCallResult, error) {
	toolName := strings.TrimSpace(req.ToolName)
	if toolName == "" {
		return mcpruntime.ToolCallResult{}, fmt.Errorf("mcp gateway: tool name is required")
	}
	var result mcpruntime.ToolCallResult
	if err := g.rpcIntoServer(ctx, req.ServerID, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": json.RawMessage(req.Arguments),
	}, &result); err != nil {
		return mcpruntime.ToolCallResult{}, err
	}
	return result, nil
}

func (g *Gateway) ReadResource(ctx context.Context, req MCPResourceReadRequest) (mcp.ResourceContent, error) {
	uri := strings.TrimSpace(req.URI)
	if uri == "" {
		return mcp.ResourceContent{}, fmt.Errorf("mcp gateway: resource uri is required")
	}
	var result mcp.ResourceContent
	if err := g.rpcIntoServer(ctx, req.ServerID, "resources/read", map[string]any{"uri": uri}, &result); err != nil {
		return mcp.ResourceContent{}, err
	}
	if result.ServerID == "" {
		result.ServerID = strings.TrimSpace(req.ServerID)
	}
	if result.URI == "" {
		result.URI = uri
	}
	return result, nil
}

func (g *Gateway) Close(ctx context.Context) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	stdio := make([]*stdioSession, 0, len(g.stdio))
	for _, session := range g.stdio {
		stdio = append(stdio, session)
	}
	g.endpoints = map[string]string{}
	g.stdio = map[string]*stdioSession{}
	g.mu.Unlock()
	var firstErr error
	for _, session := range stdio {
		if err := session.close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (g *Gateway) rpcIntoServer(ctx context.Context, serverID, method string, params any, out any) error {
	endpoint, stdio, err := g.transportForServer(serverID)
	if err != nil {
		return err
	}
	if stdio != nil {
		return stdio.rpcInto(ctx, method, params, out)
	}
	return g.rpcInto(ctx, endpoint, method, params, out)
}

func (g *Gateway) transportForServer(serverID string) (string, *stdioSession, error) {
	if g == nil {
		return "", nil, fmt.Errorf("mcp gateway is nil")
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return "", nil, fmt.Errorf("mcp gateway: server id is required")
	}
	g.mu.RLock()
	endpoint := g.endpoints[serverID]
	stdio := g.stdio[serverID]
	g.mu.RUnlock()
	if endpoint == "" && stdio == nil {
		return "", nil, fmt.Errorf("mcp gateway: server %q is not connected", serverID)
	}
	return endpoint, stdio, nil
}

func endpointURL(cfg ServerConfigV2) string {
	if cfg.Endpoint == nil || cfg.Endpoint.Type != EndpointTypeStreamableHTTP {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(cfg.Endpoint.URL), "/")
}

func (g *Gateway) rpcInto(ctx context.Context, endpoint, method string, params any, out any) error {
	result, err := g.rpc(ctx, endpoint, method, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(result, out); err != nil {
		return fmt.Errorf("%s result decode: %w", method, err)
	}
	return nil
}

func (g *Gateway) rpc(ctx context.Context, endpoint, method string, params any) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      method,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s failed: %s", method, resp.Status)
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("%s response decode: %w", method, err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("%s failed: %s", method, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

type stdioSession struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

func startStdioSession(ctx context.Context, cfg StdioConfig) (*stdioSession, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("mcp gateway: stdio command is required")
	}
	cmd := exec.CommandContext(ctx, command, cfg.Args...)
	cmd.Env = append(os.Environ(), cfg.Env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &stdioSession{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
	}, nil
}

func (s *stdioSession) rpcInto(ctx context.Context, method string, params any, out any) error {
	result, err := s.rpc(ctx, method, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(result, out); err != nil {
		return fmt.Errorf("%s result decode: %w", method, err)
	}
	return nil
}

func (s *stdioSession) rpc(_ context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      method,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.stdin.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(line, &rpcResp); err != nil {
		return nil, fmt.Errorf("%s response decode: %w", method, err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("%s failed: %s", method, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (s *stdioSession) close(context.Context) error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.stdin.Close()
	if err := s.cmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
		return err
	}
	_ = s.cmd.Wait()
	return nil
}
