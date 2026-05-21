package appui

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
)

type mcpRepoStub struct {
	items []store.MCPServerRecord
}

func (r *mcpRepoStub) GetMCPServers() ([]store.MCPServerRecord, error) {
	out := make([]store.MCPServerRecord, 0, len(r.items))
	for _, item := range r.items {
		cp := item
		cp.Args = append([]string(nil), cp.Args...)
		cp.Env = cloneStringMap(cp.Env)
		out = append(out, cp)
	}
	return out, nil
}

func (r *mcpRepoStub) SaveMCPServers(items []store.MCPServerRecord) error {
	r.items = make([]store.MCPServerRecord, 0, len(items))
	for _, item := range items {
		cp := item
		cp.Args = append([]string(nil), cp.Args...)
		cp.Env = cloneStringMap(cp.Env)
		r.items = append(r.items, cp)
	}
	return nil
}

type mcpRuntimeStub struct {
	registry     *mcp.Registry
	connected    []string
	refreshed    []string
	disconnected []string
}

func (r *mcpRuntimeStub) Connect(_ context.Context, serverID string) error {
	r.connected = append(r.connected, serverID)
	return r.registry.OnServerConnected(serverID, []tooling.Tool{&tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: serverID + ".search", Description: "search"},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}})
}

func (r *mcpRuntimeStub) Disconnect(_ context.Context, serverID string) error {
	r.disconnected = append(r.disconnected, serverID)
	r.registry.OnServerDisconnected(serverID)
	return nil
}

func (r *mcpRuntimeStub) RefreshTools(ctx context.Context, serverID string) error {
	r.refreshed = append(r.refreshed, serverID)
	return r.Connect(ctx, serverID)
}

func TestMCPServiceListAndRuntimeActions(t *testing.T) {
	repo := &mcpRepoStub{
		items: []store.MCPServerRecord{
			{
				Name:      "docs",
				Transport: "stdio",
				Command:   "docs-mcp",
				Args:      []string{"--flag"},
				Disabled:  true,
				Status:    "disconnected",
				Env:       map[string]string{"TOKEN": "secret"},
			},
		},
	}
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{
		ID:        "generator",
		Name:      "generator",
		Transport: "local",
		Command:   []string{"generator"},
		Source:    "builtin",
	}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	if err := registry.OnServerConnected("generator", []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: "generator.generate", Description: "generate"},
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				return tooling.ToolResult{}, nil
			},
		},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	svc := NewMCPService(repo, registry)

	listed, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if listed.ConfigPath != "mcp-servers.json" {
		t.Fatalf("ConfigPath = %q, want mcp-servers.json", listed.ConfigPath)
	}
	if len(listed.Items) != 2 {
		t.Fatalf("List().Items len = %d, want 2", len(listed.Items))
	}
	if got := findMCPItem(listed.Items, "generator"); got == nil || got.ToolCount != 1 || got.Status != "connected" {
		t.Fatalf("generator item = %+v, want connected with tool count 1", got)
	}
	if got := findMCPItem(listed.Items, "docs"); got == nil || !got.Disabled || got.Status != "disconnected" {
		t.Fatalf("docs item = %+v, want disconnected", got)
	}

	created, err := svc.Create(context.Background(), MCPServerUpsert{
		Name:      "search",
		Transport: "http",
		URL:       "http://127.0.0.1:8088/mcp",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := findMCPItem(created.Items, "search"); got == nil || got.Status != "connected" {
		t.Fatalf("search item after create = %+v, want connected", got)
	}
	if _, ok := registry.GetServer("search"); !ok {
		t.Fatal("search should be registered after create")
	}

	closed, err := svc.Act(context.Background(), "search", "close")
	if err != nil {
		t.Fatalf("Act(close) error = %v", err)
	}
	if got := findMCPItem(closed.Items, "search"); got == nil || !got.Disabled || got.Status != "disconnected" {
		t.Fatalf("search item after close = %+v, want disconnected", got)
	}
	if !registry.IsServerDisabled("search") {
		t.Fatal("search should be disabled after close")
	}

	opened, err := svc.Act(context.Background(), "search", "open")
	if err != nil {
		t.Fatalf("Act(open) error = %v", err)
	}
	if got := findMCPItem(opened.Items, "search"); got == nil || got.Disabled || got.Status != "connected" {
		t.Fatalf("search item after open = %+v, want connected", got)
	}
	if registry.IsServerDisabled("search") {
		t.Fatal("search should be enabled after open")
	}

	refreshed, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got := findMCPItem(refreshed.Items, "search"); got == nil || got.Status != "connected" {
		t.Fatalf("search item after refresh = %+v, want connected", got)
	}

	deleted, err := svc.Delete(context.Background(), "search")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if findMCPItem(deleted.Items, "search") != nil {
		t.Fatal("search should be removed after delete")
	}
}

func TestMCPServiceUsesRuntimeForConnectRefreshAndDisconnect(t *testing.T) {
	repo := &mcpRepoStub{}
	registry := mcp.NewRegistry()
	runtime := &mcpRuntimeStub{registry: registry}
	svc := NewMCPServiceWithRuntime(repo, registry, runtime)

	created, err := svc.Create(context.Background(), MCPServerUpsert{
		Name:      "docs",
		Transport: "stdio",
		Command:   "docs-mcp",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(runtime.connected) != 1 || runtime.connected[0] != "docs" {
		t.Fatalf("runtime connected = %#v, want docs", runtime.connected)
	}
	if got := findMCPItem(created.Items, "docs"); got == nil || got.ToolCount != 1 || got.Status != "connected" {
		t.Fatalf("docs after create = %+v, want connected with tool count 1", got)
	}

	refreshed, err := svc.Act(context.Background(), "docs", "refresh")
	if err != nil {
		t.Fatalf("Act(refresh) error = %v", err)
	}
	if len(runtime.refreshed) != 1 || runtime.refreshed[0] != "docs" {
		t.Fatalf("runtime refreshed = %#v, want docs", runtime.refreshed)
	}
	if got := findMCPItem(refreshed.Items, "docs"); got == nil || got.ToolCount != 1 {
		t.Fatalf("docs after refresh = %+v, want tool count 1", got)
	}

	closed, err := svc.Act(context.Background(), "docs", "close")
	if err != nil {
		t.Fatalf("Act(close) error = %v", err)
	}
	if len(runtime.disconnected) != 1 || runtime.disconnected[0] != "docs" {
		t.Fatalf("runtime disconnected = %#v, want docs", runtime.disconnected)
	}
	if got := findMCPItem(closed.Items, "docs"); got == nil || got.ToolCount != 0 || got.Status != "disconnected" {
		t.Fatalf("docs after close = %+v, want disconnected with no tools", got)
	}
}

func findMCPItem(items []MCPServerView, name string) *MCPServerView {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}
