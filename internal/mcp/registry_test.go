package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

type mockTool struct {
	meta tooling.ToolMetadata
}

func (m mockTool) Metadata() tooling.ToolMetadata { return m.meta }

func (m mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (m mockTool) OutputSchema() json.RawMessage { return nil }

func (m mockTool) Description(_ json.RawMessage, _ tooling.DescribeContext) string {
	return m.meta.Description
}

func (m mockTool) Prompt(_ tooling.PromptContext) string { return m.meta.Description }

func (m mockTool) IsEnabled(tooling.ToolContext) bool { return true }

func (m mockTool) IsReadOnly(json.RawMessage) bool { return true }

func (m mockTool) IsDestructive(json.RawMessage) bool { return false }

func (m mockTool) IsConcurrencySafe(json.RawMessage) bool { return true }

func (m mockTool) ValidateInput(context.Context, json.RawMessage) error { return nil }

func (m mockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}

func (m mockTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func TestRegistryRegisterServerAndGet(t *testing.T) {
	r := NewRegistry()

	cfg := ServerConfig{
		ID:        "coroot",
		Name:      "Coroot",
		Transport: "stdio",
		Command:   []string{"coroot-mcp"},
	}
	if err := r.RegisterServer(cfg); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}

	got, ok := r.GetServer("coroot")
	if !ok {
		t.Fatal("expected server config to be registered")
	}
	if got.Name != "Coroot" {
		t.Fatalf("GetServer().Name = %q, want %q", got.Name, "Coroot")
	}
	got.Command[0] = "changed"
	again, ok := r.GetServer("coroot")
	if !ok {
		t.Fatal("expected server config to remain registered")
	}
	if again.Command[0] != "coroot-mcp" {
		t.Fatalf("GetServer() should return a cloned command slice, got %#v", again.Command)
	}
}

func TestRegistryDynamicToolsLifecycle(t *testing.T) {
	r := NewRegistry()

	tools := []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "list_services", Description: "List services"}},
		mockTool{meta: tooling.ToolMetadata{Name: "service_metrics", Description: "Get metrics"}},
	}

	if err := r.OnServerConnected("coroot", tools); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	listed := r.ListServerTools("coroot")
	if len(listed) != 2 {
		t.Fatalf("ListServerTools() len = %d, want 2", len(listed))
	}
	meta := listed[0].Metadata()
	if meta.Origin != tooling.ToolOriginMCP {
		t.Fatalf("connected tool origin = %q, want %q", meta.Origin, tooling.ToolOriginMCP)
	}
	if meta.MCPInfo.ServerID != "coroot" {
		t.Fatalf("connected tool server id = %q, want coroot", meta.MCPInfo.ServerID)
	}

	all := r.DynamicTools()
	if len(all) != 2 {
		t.Fatalf("DynamicTools() len = %d, want 2", len(all))
	}

	r.OnServerDisconnected("coroot")
	if got := r.ListServerTools("coroot"); got != nil {
		t.Fatalf("ListServerTools() after disconnect = %#v, want nil", got)
	}
	if got := r.DynamicTools(); len(got) != 0 {
		t.Fatalf("DynamicTools() after disconnect len = %d, want 0", len(got))
	}
}

func TestRegistryRejectsEmptyServerID(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterServer(ServerConfig{}); err == nil {
		t.Fatal("RegisterServer() should fail for empty ID")
	}
	if err := r.OnServerConnected("", nil); err == nil {
		t.Fatal("OnServerConnected() should fail for empty server ID")
	}
}
