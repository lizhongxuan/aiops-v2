package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/settings"
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
	if !meta.HasMCPSource() {
		t.Fatalf("connected tool should report MCP source, got %#v", meta)
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

func TestMCPRegistryListsAndReadsResources(t *testing.T) {
	r := NewRegistry()
	if err := r.OnServerResources("coroot", []Resource{{
		URI:      "coroot://projects/default/schema",
		Name:     "Coroot schema",
		MimeType: "application/json",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetResourceContent("coroot", "coroot://projects/default/schema", ResourceContent{
		URI:      "coroot://projects/default/schema",
		MimeType: "application/json",
		Text:     `{"project":"default"}`,
	}); err != nil {
		t.Fatal(err)
	}

	resources := r.ListResources("coroot")
	if len(resources) != 1 || resources[0].URI == "" {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0].ServerID != "coroot" {
		t.Fatalf("resource server id = %q, want coroot", resources[0].ServerID)
	}

	content, ok, err := r.ReadResource(context.Background(), "coroot", "coroot://projects/default/schema")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || content.Text == "" || content.Digest == "" {
		t.Fatalf("ReadResource() = %#v, %v, want content with digest", content, ok)
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

func TestRegistryRejectsCustomMCPServersWhenStrictPluginOnlyEnabled(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		RestrictToPluginOnly: []settings.CustomizationSurface{settings.SurfaceMCP},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	err := r.RegisterServer(ServerConfig{
		ID:        "custom-mcp",
		Transport: "stdio",
		Command:   []string{"custom-mcp"},
		Source:    string(settings.SourceUserSettings),
	})
	if err == nil {
		t.Fatal("expected strict plugin-only policy to reject userSettings MCP server")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
}

func TestRegistryAppliesAllowedMCPServersToCustomSources(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		AllowedMCPServers: []string{"allowed-mcp"},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	if err := r.RegisterServer(ServerConfig{
		ID:        "allowed-mcp",
		Transport: "stdio",
		Command:   []string{"allowed-mcp"},
		Source:    string(settings.SourceUserSettings),
	}); err != nil {
		t.Fatalf("expected listed custom MCP server to be allowed, got %v", err)
	}

	err := r.RegisterServer(ServerConfig{
		ID:        "blocked-mcp",
		Transport: "stdio",
		Command:   []string{"blocked-mcp"},
		Source:    string(settings.SourceUserSettings),
	})
	if err == nil {
		t.Fatal("expected unlisted custom MCP server to be rejected")
	}
	if !strings.Contains(err.Error(), "allowedMcpServers") {
		t.Fatalf("expected allowlist error, got %v", err)
	}

	if err := r.RegisterServer(ServerConfig{
		ID:        "plugin-mcp",
		Transport: "stdio",
		Command:   []string{"plugin-mcp"},
		Source:    "plugin",
	}); err != nil {
		t.Fatalf("expected plugin MCP server to bypass allowlist, got %v", err)
	}
}

func TestRegistryDynamicToolRefreshTokenStableAcrossOrdering(t *testing.T) {
	r := NewRegistry()

	tools := []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "z_tool", Description: "z"}},
		mockTool{meta: tooling.ToolMetadata{Name: "a_tool", Description: "a"}},
	}

	if err := r.OnServerConnected("coroot", tools); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	first := r.DynamicToolRefreshToken()
	if first == "" {
		t.Fatal("expected non-empty refresh token")
	}

	if err := r.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "a_tool", Description: "a"}},
		mockTool{meta: tooling.ToolMetadata{Name: "z_tool", Description: "z"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() reorder error = %v", err)
	}
	second := r.DynamicToolRefreshToken()
	if second != first {
		t.Fatalf("refresh token changed after reorder: %q vs %q", first, second)
	}

	if err := r.OnServerConnected("coroot", append(tools, mockTool{meta: tooling.ToolMetadata{Name: "b_tool", Description: "b"}})); err != nil {
		t.Fatalf("OnServerConnected() add error = %v", err)
	}
	third := r.DynamicToolRefreshToken()
	if third == first {
		t.Fatal("expected refresh token to change after tool set change")
	}
}

func TestAssemblerRefreshTokenTracksDynamicProviders(t *testing.T) {
	base := tooling.NewRegistry()
	baseTool := mockTool{meta: tooling.ToolMetadata{Name: "base_tool", Description: "base"}}
	if err := base.Register(baseTool); err != nil {
		t.Fatalf("base Register() error = %v", err)
	}

	dynamic := NewRegistry()
	if err := dynamic.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "dyn_tool", Description: "dyn"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	assembler := tooling.NewAssembler(base, dynamic)
	first := assembler.RefreshToken("host", "chat", tooling.AssembleOptions{})
	if first == "" {
		t.Fatal("expected non-empty assembler refresh token")
	}

	if err := dynamic.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "dyn_tool", Description: "dyn"}},
		mockTool{meta: tooling.ToolMetadata{Name: "extra_tool", Description: "extra"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() update error = %v", err)
	}
	second := assembler.RefreshToken("host", "chat", tooling.AssembleOptions{})
	if second == first {
		t.Fatal("expected assembler refresh token to change when dynamic tools change")
	}

	assembled := assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{})
	if len(assembled) != 3 {
		t.Fatalf("assembled tool count = %d, want 3", len(assembled))
	}
}
