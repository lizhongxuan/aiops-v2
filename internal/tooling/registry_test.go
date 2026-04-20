package tooling

import (
	"context"
	"encoding/json"
	"testing"
)

type mockTool struct {
	meta        ToolMetadata
	enabled     bool
	readOnly    bool
	destructive bool
	concurrency bool
	description string
}

func (m *mockTool) Metadata() ToolMetadata        { return m.meta }
func (m *mockTool) InputSchema() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) OutputSchema() json.RawMessage { return nil }
func (m *mockTool) Description(_ json.RawMessage, _ DescribeContext) string {
	if m.description != "" {
		return m.description
	}
	return m.meta.Description
}
func (m *mockTool) Prompt(_ PromptContext) string                        { return m.description }
func (m *mockTool) IsEnabled(_ ToolContext) bool                         { return m.enabled }
func (m *mockTool) IsReadOnly(_ json.RawMessage) bool                    { return m.readOnly }
func (m *mockTool) IsDestructive(_ json.RawMessage) bool                 { return m.destructive }
func (m *mockTool) IsConcurrencySafe(_ json.RawMessage) bool             { return m.concurrency }
func (m *mockTool) ValidateInput(context.Context, json.RawMessage) error { return nil }
func (m *mockTool) CheckPermissions(context.Context, json.RawMessage) PermissionDecision {
	return PermissionDecision{Action: PermissionActionAllow}
}
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

func TestRegistryBuiltInPriority(t *testing.T) {
	r := NewRegistry()

	builtin := &mockTool{
		meta:        ToolMetadata{Name: "read_file", Origin: ToolOriginBuiltin, Description: "builtin"},
		enabled:     true,
		description: "builtin",
	}
	mcp := &mockTool{
		meta:        ToolMetadata{Name: "read_file", Origin: ToolOriginMCP, Description: "mcp"},
		enabled:     true,
		description: "mcp",
	}

	if err := r.Register(mcp); err != nil {
		t.Fatalf("Register(mcp) error = %v", err)
	}
	if err := r.Register(builtin); err != nil {
		t.Fatalf("Register(builtin) error = %v", err)
	}

	got, ok := r.Get("read_file")
	if !ok {
		t.Fatal("Get(read_file) returned false")
	}
	if got.Metadata().Description != "builtin" {
		t.Fatalf("Get(read_file) description = %q, want builtin", got.Metadata().Description)
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	if list[0].Metadata().Description != "builtin" {
		t.Fatalf("List()[0] description = %q, want builtin", list[0].Metadata().Description)
	}

	assembled := r.AssembleTools("host", "chat")
	if len(assembled) != 1 {
		t.Fatalf("AssembleTools() len = %d, want 1", len(assembled))
	}
	if assembled[0].Metadata().Description != "builtin" {
		t.Fatalf("AssembleTools()[0] description = %q, want builtin", assembled[0].Metadata().Description)
	}

	pool := r.AssembleToolPool("host", "chat")
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPool() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Desc != "builtin" {
		t.Fatalf("Info().Desc = %q, want builtin", info.Desc)
	}
}

func TestRegistryVisibilityAndUnregister(t *testing.T) {
	r := NewRegistry()

	hostOnly := &mockTool{
		meta:        ToolMetadata{Name: "host_only", Origin: ToolOriginMeta},
		enabled:     true,
		description: "host",
	}
	workspaceOnly := &mockTool{
		meta:        ToolMetadata{Name: "workspace_only", Origin: ToolOriginMeta},
		enabled:     true,
		description: "workspace",
	}

	if err := r.Register(hostOnly); err != nil {
		t.Fatalf("Register(hostOnly) error = %v", err)
	}
	if err := r.Register(workspaceOnly); err != nil {
		t.Fatalf("Register(workspaceOnly) error = %v", err)
	}

	if len(r.AssembleTools("host", "chat")) != 2 {
		t.Fatalf("AssembleTools should return all enabled tools")
	}

	r.Unregister("host_only")
	if _, ok := r.Get("host_only"); ok {
		t.Fatal("host_only should be removed after Unregister")
	}
}
