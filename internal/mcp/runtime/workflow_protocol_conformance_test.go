package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	workflowintegration "aiops-v2/internal/integrations/workfloweditor"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

type workflowProtocolRequest struct {
	Method    string
	Version   string
	SessionID string
	ID        string
	Params    map[string]any
}

func TestWorkflowOutboundMCPProtocolConformance(t *testing.T) {
	contracts := workflowintegration.WorkflowToolContractsV2()
	names := make([]string, 0, len(contracts))
	for name := range contracts {
		names = append(names, name)
	}
	sort.Strings(names)
	var mu sync.Mutex
	requests := make([]workflowProtocolRequest, 0, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		method, _ := envelope["method"].(string)
		id, _ := envelope["id"].(string)
		params, _ := envelope["params"].(map[string]any)
		mu.Lock()
		requests = append(requests, workflowProtocolRequest{Method: method, Version: r.Header.Get("MCP-Protocol-Version"), SessionID: r.Header.Get("Mcp-Session-Id"), ID: id, Params: params})
		mu.Unlock()
		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "workflow-session-v1")
			writeWorkflowProtocolResult(t, w, envelope["id"], map[string]any{
				"protocolVersion": MCPProtocolVersion20250618,
				"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
				"serverInfo":      map[string]any{"name": "workflow-conformance", "version": "1"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			definitions := make([]map[string]any, 0, len(names))
			for _, name := range names {
				contract := contracts[name]
				definitions = append(definitions, map[string]any{
					"name": name, "description": "workflow conformance " + name,
					"inputSchema": json.RawMessage(contract.InputSchema), "outputSchema": json.RawMessage(contract.OutputSchema),
					"readOnly":    name == "workflow.get_context" || name == "workflow.get_action_catalog" || name == "workflow.validate",
					"destructive": name == "workflow.apply_patch",
				})
			}
			writeWorkflowProtocolResult(t, w, envelope["id"], map[string]any{"tools": definitions})
		case "resources/list":
			writeWorkflowProtocolResult(t, w, envelope["id"], map[string]any{"resources": []map[string]any{{
				"uri": "workflow://tenant-a/workflow-a/context/revision-a", "name": "Workflow context", "mimeType": "application/json",
			}}})
		case "tools/call":
			writeWorkflowProtocolResult(t, w, envelope["id"], map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "workflow revision conflict"},
					{"type": "resource_link", "uri": "workflow://tenant-a/workflow-a/context/revision-a", "name": "Workflow context", "mimeType": "application/json", "size": 128},
				},
				"structuredContent": map[string]any{"schemaVersion": "aiops.workflow.edit.v2", "kind": "error", "errorCode": "STALE_REVISION"},
				"isError":           true,
			})
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "workflow-conformance", Name: "Workflow Conformance", Transport: "http", Command: []string{server.URL}}); err != nil {
		t.Fatal(err)
	}
	runtime := New(RuntimeOptions{Registry: registry, ClientFactory: DefaultClientFactory{HTTPClient: server.Client()}})
	if err := runtime.Connect(context.Background(), "workflow-conformance"); err != nil {
		t.Fatal(err)
	}
	tools := registry.ListServerTools("workflow-conformance")
	if len(tools) != len(names) {
		t.Fatalf("discovered tools = %d, want %d", len(tools), len(names))
	}
	var contextTool tooling.Tool
	for _, tool := range tools {
		if tool.Metadata().Name == "workflow.get_context" {
			contextTool = tool
			break
		}
	}
	if contextTool == nil {
		t.Fatal("workflow.get_context was not discovered")
	}
	result, err := contextTool.Execute(WithMCPRequestToken(context.Background(), "workflow-progress-1"), json.RawMessage(`{"workflowKey":{"tenantId":"tenant-a","workflowId":"workflow-a"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content != "workflow revision conflict" || len(result.StructuredContent) == 0 || len(result.References) != 1 || result.References[0].Kind != tooling.ResultReferenceKindMCPResource {
		t.Fatalf("mapped workflow result = %#v", result)
	}
	if strings.Contains(string(result.StructuredContent), `"status":"ok"`) {
		t.Fatalf("remote typed error degraded to fake success: %s", result.StructuredContent)
	}

	mu.Lock()
	got := append([]workflowProtocolRequest(nil), requests...)
	mu.Unlock()
	methods := make([]string, len(got))
	for index := range got {
		methods[index] = got[index].Method
	}
	if want := []string{"initialize", "notifications/initialized", "tools/list", "resources/list", "tools/call"}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("protocol order = %v, want %v", methods, want)
	}
	if got[0].Version != "" || got[0].SessionID != "" {
		t.Fatalf("initialize carried negotiated headers: %#v", got[0])
	}
	for _, request := range got[1:] {
		if request.Version != MCPProtocolVersion20250618 || request.SessionID != "workflow-session-v1" {
			t.Fatalf("negotiated headers = %#v", request)
		}
	}
	meta, _ := got[len(got)-1].Params["_meta"].(map[string]any)
	if got[len(got)-1].ID != "workflow-progress-1" || meta["progressToken"] != "workflow-progress-1" {
		t.Fatalf("progress correlation = %#v", got[len(got)-1])
	}
}

func TestWorkflowOutboundMCPToolCancellationConformance(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope map[string]any
		_ = json.NewDecoder(r.Body).Decode(&envelope)
		switch envelope["method"] {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "workflow-cancel-session")
			writeWorkflowProtocolResult(t, w, envelope["id"], map[string]any{"protocolVersion": MCPProtocolVersion20250326, "capabilities": map[string]any{"tools": map[string]any{}}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			close(started)
			<-r.Context().Done()
		case "notifications/cancelled":
			params, _ := envelope["params"].(map[string]any)
			cancelled <- params
			w.WriteHeader(http.StatusAccepted)
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
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
	ctx, cancel := context.WithCancelCause(WithMCPRequestToken(context.Background(), "workflow-cancel-1"))
	done := make(chan error, 1)
	go func() {
		result, err := client.CallTool(ctx, "workflow.get_context", json.RawMessage(`{}`))
		if result.Content != "" || result.IsError || len(result.StructuredContent) != 0 {
			done <- errors.New("cancelled call returned a completion payload")
			return
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("workflow MCP call did not start")
	}
	cancel(errors.New("user cancelled workflow edit"))
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled call error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("workflow MCP call did not stop")
	}
	select {
	case params := <-cancelled:
		if params["requestId"] != "workflow-cancel-1" || params["reason"] != "user cancelled workflow edit" {
			t.Fatalf("cancel notification = %#v", params)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("workflow cancellation notification was not sent")
	}
}

func writeWorkflowProtocolResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
