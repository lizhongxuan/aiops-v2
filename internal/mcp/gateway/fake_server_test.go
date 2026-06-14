package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"aiops-v2/internal/mcp"
)

func TestGatewayStreamableHTTPConformance(t *testing.T) {
	fake := newFakeStreamableHTTPMCPServer(t, false)
	defer fake.Close()

	gateway := NewGateway(GatewayOptions{HTTPClient: fake.Client()})
	connected, err := gateway.Connect(context.Background(), ServerConfigV2{
		ID:   "fake",
		Name: "Fake MCP",
		Endpoint: &EndpointConfig{
			Type: EndpointTypeStreamableHTTP,
			URL:  fake.URL,
		},
	})
	if err != nil {
		t.Fatalf("Connect error = %v", err)
	}
	if len(connected.Tools) != 1 || connected.Tools[0].Name != "metrics" {
		t.Fatalf("connected tools = %#v, want metrics", connected.Tools)
	}
	if len(connected.Resources) != 1 || connected.Resources[0].URI != "resource://metrics/latest" {
		t.Fatalf("connected resources = %#v, want metrics resource", connected.Resources)
	}

	toolResult, err := gateway.CallTool(context.Background(), MCPToolCallRequest{
		ServerID:  "fake",
		ToolName:  "metrics",
		Arguments: json.RawMessage(`{"service":"api"}`),
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	if toolResult.Content != "api metrics ok" {
		t.Fatalf("tool result content = %q, want api metrics ok", toolResult.Content)
	}

	resource, err := gateway.ReadResource(context.Background(), MCPResourceReadRequest{
		ServerID: "fake",
		URI:      "resource://metrics/latest",
	})
	if err != nil {
		t.Fatalf("ReadResource error = %v", err)
	}
	if resource.Text != "latest metrics" {
		t.Fatalf("resource text = %q, want latest metrics", resource.Text)
	}

	if got, want := strings.Join(fake.Methods(), " -> "), "initialize -> tools/list -> resources/list -> tools/call -> resources/read"; got != want {
		t.Fatalf("methods = %s, want %s", got, want)
	}
}

func TestGatewayDoesNotReplayToolCallAfterSessionExpired(t *testing.T) {
	fake := newFakeStreamableHTTPMCPServer(t, true)
	defer fake.Close()

	gateway := NewGateway(GatewayOptions{HTTPClient: fake.Client()})
	if _, err := gateway.Connect(context.Background(), ServerConfigV2{
		ID: "fake",
		Endpoint: &EndpointConfig{
			Type: EndpointTypeStreamableHTTP,
			URL:  fake.URL,
		},
	}); err != nil {
		t.Fatalf("Connect error = %v", err)
	}

	_, err := gateway.CallTool(context.Background(), MCPToolCallRequest{
		ServerID:  "fake",
		ToolName:  "metrics",
		Arguments: json.RawMessage(`{"service":"api"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("CallTool error = %v, want session expired", err)
	}
	if got := fake.CallCount("tools/call"); got != 1 {
		t.Fatalf("tools/call count = %d, want no replay after session expired", got)
	}
}

type fakeStreamableHTTPMCPServer struct {
	*httptest.Server
	t              *testing.T
	expireToolCall bool
	mu             sync.Mutex
	methods        []string
	callsByMethod  map[string]int
}

func newFakeStreamableHTTPMCPServer(t *testing.T, expireToolCall bool) *fakeStreamableHTTPMCPServer {
	t.Helper()
	fake := &fakeStreamableHTTPMCPServer{
		t:              t,
		expireToolCall: expireToolCall,
		callsByMethod:  map[string]int{},
	}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.handle))
	return fake
}

func (s *fakeStreamableHTTPMCPServer) Methods() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.methods...)
}

func (s *fakeStreamableHTTPMCPServer) CallCount(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callsByMethod[method]
}

func (s *fakeStreamableHTTPMCPServer) record(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, method)
	s.callsByMethod[method]++
}

func (s *fakeStreamableHTTPMCPServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     any             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.record(req.Method)
	w.Header().Set("Content-Type", "application/json")
	switch req.Method {
	case "initialize":
		writeRPCResult(w, req.ID, map[string]any{"protocolVersion": "2025-03-26"})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": []map[string]any{{
			"name":        "metrics",
			"description": "Read metrics",
			"inputSchema": map[string]any{"type": "object"},
		}}})
	case "resources/list":
		writeRPCResult(w, req.ID, map[string]any{"resources": []mcp.Resource{{
			URI:      "resource://metrics/latest",
			Name:     "Latest metrics",
			MimeType: "text/plain",
		}}})
	case "tools/call":
		if s.expireToolCall {
			writeRPCError(w, req.ID, -32001, "session expired")
			return
		}
		writeRPCResult(w, req.ID, map[string]any{"content": "api metrics ok"})
	case "resources/read":
		writeRPCResult(w, req.ID, mcp.ResourceContent{URI: "resource://metrics/latest", Text: "latest metrics", MimeType: "text/plain"})
	default:
		writeRPCError(w, req.ID, -32601, "method not found")
	}
}

func writeRPCResult(w http.ResponseWriter, id any, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeRPCError(w http.ResponseWriter, id any, code int, message string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
