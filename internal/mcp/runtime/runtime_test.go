package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/mcp"
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
