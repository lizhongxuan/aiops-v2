package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/mcp"
)

type lifecycleClient struct {
	events []string
	init   InitializeRequest
}

func (c *lifecycleClient) Initialize(_ context.Context, request InitializeRequest) (ClientSession, error) {
	c.events = append(c.events, "initialize")
	c.init = request
	return ClientSession{ProtocolVersion: MCPProtocolVersion20250326, Capabilities: json.RawMessage(`{"tools":{},"resources":{}}`)}, nil
}
func (c *lifecycleClient) NotifyInitialized(context.Context) error {
	c.events = append(c.events, "initialized")
	return nil
}
func (c *lifecycleClient) ListTools(context.Context) ([]ToolDefinition, error) {
	c.events = append(c.events, "tools/list")
	return []ToolDefinition{{Name: "inspect", ReadOnly: true}}, nil
}
func (c *lifecycleClient) ListResources(context.Context) ([]mcp.Resource, error) {
	c.events = append(c.events, "resources/list")
	return nil, nil
}
func (*lifecycleClient) CallTool(context.Context, string, json.RawMessage) (ToolCallResult, error) {
	return ToolCallResult{}, nil
}
func (*lifecycleClient) ReadResource(context.Context, string) (mcp.ResourceContent, error) {
	return mcp.ResourceContent{}, nil
}
func (*lifecycleClient) Close(context.Context) error { return nil }

func TestRuntimeConnectRunsOptionalProtocolLifecycleBeforeDiscovery(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "lifecycle", Transport: "http", Command: []string{"http://mcp.test"}}); err != nil {
		t.Fatal(err)
	}
	client := &lifecycleClient{}
	runtime := New(RuntimeOptions{Registry: registry, ClientFactory: ClientFactoryFunc(func(context.Context, mcp.ServerConfig) (Client, error) { return client, nil })})
	if err := runtime.Connect(context.Background(), "lifecycle"); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(client.events, " -> "), "initialize -> initialized -> tools/list -> resources/list"; got != want {
		t.Fatalf("lifecycle order = %q, want %q", got, want)
	}
	if !containsString(client.init.SupportedProtocolVersions, MCPProtocolVersion20250326) || len(client.init.Capabilities) == 0 || client.init.ClientInfo.Name != "aiops-v2" {
		t.Fatalf("initialize request = %#v", client.init)
	}
}

type recordedMCPRequest struct {
	Method          string
	ProtocolVersion string
	SessionID       string
	Accept          string
	Body            map[string]any
}

func TestHTTPClientNegotiatesSessionHeadersProgressAndErrorSemantics(t *testing.T) {
	var mu sync.Mutex
	requests := make([]recordedMCPRequest, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		method, _ := body["method"].(string)
		mu.Lock()
		requests = append(requests, recordedMCPRequest{Method: method, ProtocolVersion: r.Header.Get("MCP-Protocol-Version"), SessionID: r.Header.Get("Mcp-Session-Id"), Accept: r.Header.Get("Accept"), Body: body})
		mu.Unlock()
		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"protocolVersion": MCPProtocolVersion20250326, "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}}, "serverInfo": map[string]any{"name": "test-server", "version": "1"}})
		case "notifications/initialized", "notifications/cancelled":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"tools": []map[string]any{{"name": "inspect", "inputSchema": map[string]any{"type": "object"}}}})
		case "resources/list":
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"resources": []any{}})
		case "tools/call":
			params := body["params"].(map[string]any)
			if params["name"] == "protocol_error" {
				writeLifecycleRPCError(t, w, body["id"], -32602, "invalid arguments")
				return
			}
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"content": []map[string]any{{"type": "text", "text": "remote execution failed"}}, "structuredContent": map[string]any{"code": "REMOTE_FAILURE"}, "isError": true})
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "http", Transport: "http", Command: []string{server.URL}}); err != nil {
		t.Fatal(err)
	}
	runtime := New(RuntimeOptions{Registry: registry, ClientFactory: DefaultClientFactory{HTTPClient: server.Client()}})
	if err := runtime.Connect(context.Background(), "http"); err != nil {
		t.Fatal(err)
	}
	ctx := WithMCPRequestToken(context.Background(), "progress-123")
	result, err := runtime.CallTool(ctx, "http", "execution_error", json.RawMessage(`{"scope":"db"}`))
	if err != nil || !result.IsError {
		t.Fatalf("tool execution result=%#v err=%v", result, err)
	}
	if _, err := runtime.CallTool(WithMCPRequestToken(context.Background(), "progress-456"), "http", "protocol_error", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "-32602") {
		t.Fatalf("protocol error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 6 {
		t.Fatalf("requests = %#v", requests)
	}
	if got := requestMethods(requests); got != "initialize -> notifications/initialized -> tools/list -> resources/list -> tools/call -> tools/call" {
		t.Fatalf("request order = %q", got)
	}
	if requests[0].ProtocolVersion != "" || requests[0].SessionID != "" {
		t.Fatalf("initialize carried negotiated headers: %#v", requests[0])
	}
	for _, request := range requests[1:] {
		if request.ProtocolVersion != MCPProtocolVersion20250326 || request.SessionID != "session-1" || !strings.Contains(request.Accept, "application/json") || !strings.Contains(request.Accept, "text/event-stream") {
			t.Fatalf("subsequent request headers = %#v", request)
		}
	}
	params := requests[4].Body["params"].(map[string]any)
	meta := params["_meta"].(map[string]any)
	if meta["progressToken"] != "progress-123" || requests[4].Body["id"] != "progress-123" {
		t.Fatalf("stable progress/request token missing: %#v", requests[4].Body)
	}
}

func TestHTTPClientRejectsUnknownNegotiatedVersionAndCanDowngradeLegacyHTTP(t *testing.T) {
	t.Run("unknown version", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"protocolVersion": "2099-01-01", "capabilities": map[string]any{"tools": map[string]any{}}})
		}))
		defer server.Close()
		client := &httpClient{baseURL: server.URL, client: server.Client()}
		if _, err := client.Initialize(context.Background(), DefaultInitializeRequest()); err == nil || !strings.Contains(err.Error(), "unsupported MCP protocol version") {
			t.Fatalf("initialize error = %v", err)
		}
	})

	t.Run("legacy REST downgrade", func(t *testing.T) {
		var mu sync.Mutex
		methods := make([]string, 0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			methods = append(methods, r.Method+" "+r.URL.Path)
			mu.Unlock()
			if r.Method == http.MethodPost {
				http.Error(w, "initialize unsupported", http.StatusNotFound)
				return
			}
			switch r.URL.Path {
			case "/tools":
				_ = json.NewEncoder(w).Encode(map[string]any{"tools": []map[string]any{{"name": "legacy_read"}}})
			case "/resources":
				_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		registry := mcp.NewRegistry()
		if err := registry.RegisterServer(mcp.ServerConfig{ID: "legacy", Transport: "http", Command: []string{server.URL}}); err != nil {
			t.Fatal(err)
		}
		runtime := New(RuntimeOptions{Registry: registry, ClientFactory: DefaultClientFactory{HTTPClient: server.Client()}})
		if err := runtime.Connect(context.Background(), "legacy"); err != nil {
			t.Fatalf("legacy connect: %v", err)
		}
		mu.Lock()
		got := strings.Join(methods, " -> ")
		mu.Unlock()
		if got != "POST / -> GET /tools -> GET /resources" {
			t.Fatalf("legacy requests = %q", got)
		}
	})
}

func TestHTTPClientCancellationSendsCancelledNotificationAndReturnsNoCompletion(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["method"] {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "cancel-session")
			writeLifecycleRPCResult(t, w, body["id"], map[string]any{"protocolVersion": MCPProtocolVersion20250326, "capabilities": map[string]any{"tools": map[string]any{}}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			close(started)
			<-r.Context().Done()
		case "notifications/cancelled":
			cancelled <- body["params"].(map[string]any)
			w.WriteHeader(http.StatusAccepted)
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := &httpClient{baseURL: server.URL, client: server.Client()}
	if _, err := client.Initialize(context.Background(), DefaultInitializeRequest()); err != nil {
		t.Fatal(err)
	}
	if err := client.NotifyInitialized(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(WithMCPRequestToken(context.Background(), "cancel-token"))
	done := make(chan struct {
		result ToolCallResult
		err    error
	}, 1)
	go func() {
		result, err := client.CallTool(ctx, "long_running", json.RawMessage(`{}`))
		done <- struct {
			result ToolCallResult
			err    error
		}{result, err}
	}()
	<-started
	cancel(errors.New("user stopped the tool"))
	select {
	case outcome := <-done:
		if !errors.Is(outcome.err, context.Canceled) || outcome.result.Content != "" || outcome.result.IsError {
			t.Fatalf("cancel outcome = %#v, err=%v", outcome.result, outcome.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tool call did not stop")
	}
	select {
	case params := <-cancelled:
		if params["requestId"] != "cancel-token" || params["reason"] != "user stopped the tool" {
			t.Fatalf("cancel notification = %#v", params)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel notification was not sent")
	}
}

func writeLifecycleRPCResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		t.Error(err)
	}
}

func writeLifecycleRPCError(t *testing.T, w http.ResponseWriter, id any, code int, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}); err != nil {
		t.Error(err)
	}
}

func requestMethods(requests []recordedMCPRequest) string {
	methods := make([]string, len(requests))
	for index := range requests {
		methods[index] = requests[index].Method
	}
	return strings.Join(methods, " -> ")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
