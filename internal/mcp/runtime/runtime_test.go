package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

type fakeClient struct {
	tools     []ToolDefinition
	resources []mcp.Resource
	calls     []string
}

func (c *fakeClient) ListTools(context.Context) ([]ToolDefinition, error) {
	return append([]ToolDefinition(nil), c.tools...), nil
}

func (c *fakeClient) CallTool(_ context.Context, name string, input json.RawMessage) (ToolCallResult, error) {
	c.calls = append(c.calls, name+":"+string(input))
	return ToolCallResult{Content: "called " + name}, nil
}

func (c *fakeClient) ListResources(context.Context) ([]mcp.Resource, error) {
	return append([]mcp.Resource(nil), c.resources...), nil
}

func (c *fakeClient) ReadResource(context.Context, string) (mcp.ResourceContent, error) {
	return mcp.ResourceContent{Text: "resource"}, nil
}

func (c *fakeClient) Close(context.Context) error { return nil }

func TestRuntimeConnectPublishesToolsAndDispatchesCalls(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "docs", Name: "Docs", Transport: "stdio", Command: []string{"docs-mcp"}}); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{tools: []ToolDefinition{{
		Name:        "search",
		Description: "Search docs",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		ReadOnly:    true,
	}}}
	rt := New(RuntimeOptions{
		Registry: registry,
		ClientFactory: ClientFactoryFunc(func(context.Context, mcp.ServerConfig) (Client, error) {
			return client, nil
		}),
	})

	if err := rt.Connect(context.Background(), "docs"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	tools := registry.ListServerTools("docs")
	if len(tools) != 1 {
		t.Fatalf("ListServerTools len = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if !meta.IsMCP || meta.MCPInfo.ServerID != "docs" || meta.MCPInfo.ToolName != "search" {
		t.Fatalf("tool metadata = %#v", meta)
	}
	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"q":"latency"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "called search" {
		t.Fatalf("Execute() content = %q", result.Content)
	}
	if len(client.calls) != 1 || client.calls[0] != `search:{"q":"latency"}` {
		t.Fatalf("client calls = %#v", client.calls)
	}
	status, ok := registry.GetServerStatus("docs")
	if !ok || status.State != mcp.ServerStateConnected || status.LastError != "" {
		t.Fatalf("status = %#v, ok=%v", status, ok)
	}
}

func TestMCPDynamicToolDefaultsDeferred(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "docs", Name: "Docs", Transport: "stdio", Command: []string{"docs-mcp"}}); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{tools: []ToolDefinition{{
		Name:        "search",
		Description: "Search docs",
		ReadOnly:    true,
	}}}
	rt := New(RuntimeOptions{
		Registry: registry,
		ClientFactory: ClientFactoryFunc(func(context.Context, mcp.ServerConfig) (Client, error) {
			return client, nil
		}),
	})
	if err := rt.Connect(context.Background(), "docs"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	tools := registry.ListServerTools("docs")
	if len(tools) != 1 {
		t.Fatalf("ListServerTools len = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if !meta.IsMCP || meta.Layer != tooling.ToolLayerDeferred || !meta.DeferByDefault {
		t.Fatalf("metadata = %#v, want MCP deferred by default", meta)
	}
	if meta.Pack == "" || meta.Pack == "docs" {
		t.Fatalf("pack = %q, want stable generic MCP pack prefix", meta.Pack)
	}

	assembler := tooling.NewAssembler(tooling.NewRegistry(), registry)
	defaultNames := toolNamesForMCPRuntimeTest(assembler.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{}))
	if containsMCPRuntimeTool(defaultNames, "search") {
		t.Fatalf("default assembled tools = %v, should not include dynamic MCP tool", defaultNames)
	}
	enabledNames := toolNamesForMCPRuntimeTest(assembler.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{EnabledPacks: []string{meta.Pack}}))
	if !containsMCPRuntimeTool(enabledNames, "search") {
		t.Fatalf("enabled pack tools = %v, want search after selecting %s", enabledNames, meta.Pack)
	}
}

func TestRuntimeConnectAppliesServerGovernanceProvider(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "ops", Name: "Ops", Transport: "stdio", Command: []string{"ops-mcp"}}); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{tools: []ToolDefinition{{
		Name:        "restart_service",
		Description: "Restart a service",
		ReadOnly:    false,
		Destructive: true,
	}}}
	rt := New(RuntimeOptions{
		Registry: registry,
		ClientFactory: ClientFactoryFunc(func(context.Context, mcp.ServerConfig) (Client, error) {
			return client, nil
		}),
		GovernanceProvider: ServerGovernanceProviderFunc(func(serverID string) mcp.ServerGovernance {
			if serverID == "ops" {
				return mcp.ServerGovernance{
					ID:                           "ops",
					Permission:                   "readwrite",
					Risk:                         "high",
					RequiresExplicitUserApproval: true,
				}
			}
			return mcp.ServerGovernance{}
		}),
	})

	if err := rt.Connect(context.Background(), "ops"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	tools := registry.ListServerTools("ops")
	if len(tools) != 1 {
		t.Fatalf("ListServerTools len = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if meta.RiskLevel != tooling.ToolRiskHigh || !meta.RequiresApproval {
		t.Fatalf("metadata governance = risk %q approval %v, want high approval", meta.RiskLevel, meta.RequiresApproval)
	}
	if !meta.IsMCP || meta.MCPInfo.ServerID != "ops" || meta.MCPInfo.ServerName != "Ops" {
		t.Fatalf("mcp metadata = %#v", meta.MCPInfo)
	}
}

func TestRuntimeDisconnectRemovesToolsAndResources(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{ID: "docs", Transport: "stdio", Command: []string{"docs-mcp"}}); err != nil {
		t.Fatal(err)
	}
	rt := New(RuntimeOptions{
		Registry: registry,
		ClientFactory: ClientFactoryFunc(func(context.Context, mcp.ServerConfig) (Client, error) {
			return &fakeClient{
				tools: []ToolDefinition{{Name: "search", Description: "Search docs", ReadOnly: true}},
				resources: []mcp.Resource{{
					URI:  "docs://guide",
					Name: "Guide",
				}},
			}, nil
		}),
	})

	if err := rt.Connect(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	if len(registry.DynamicTools()) != 1 {
		t.Fatalf("DynamicTools before disconnect = %d, want 1", len(registry.DynamicTools()))
	}
	if len(registry.ListResources("docs")) != 1 {
		t.Fatalf("ListResources before disconnect = %d, want 1", len(registry.ListResources("docs")))
	}
	if err := rt.Disconnect(context.Background(), "docs"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if len(registry.DynamicTools()) != 0 {
		t.Fatalf("DynamicTools after disconnect = %d, want 0", len(registry.DynamicTools()))
	}
	if len(registry.ListResources("docs")) != 0 {
		t.Fatalf("ListResources after disconnect = %d, want 0", len(registry.ListResources("docs")))
	}
	status, ok := registry.GetServerStatus("docs")
	if !ok || status.State != mcp.ServerStateDisconnected {
		t.Fatalf("status = %#v, ok=%v", status, ok)
	}
}

func toolNamesForMCPRuntimeTest(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func containsMCPRuntimeTool(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
