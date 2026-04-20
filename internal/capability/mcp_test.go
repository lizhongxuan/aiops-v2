package capability

import (
	"testing"

	
)

func TestMCPServerManager_OnServerConnected(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	tools := []Entry{
		{ID: "list_services", Name: "coroot.list_services", Kind: KindMCPTool, Description: "List services"},
		{ID: "service_metrics", Name: "coroot.service_metrics", Kind: KindMCPTool, Description: "Get metrics"},
	}

	if err := mgr.OnServerConnected("coroot", tools); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	// Verify tools are visible in the registry
	e1, ok := reg.Get("coroot/list_services")
	if !ok {
		t.Fatal("expected coroot/list_services to be registered")
	}
	if e1.Kind != KindMCPTool {
		t.Errorf("expected kind %q, got %q", KindMCPTool, e1.Kind)
	}
	if e1.Name != "coroot.list_services" {
		t.Errorf("expected name %q, got %q", "coroot.list_services", e1.Name)
	}

	e2, ok := reg.Get("coroot/service_metrics")
	if !ok {
		t.Fatal("expected coroot/service_metrics to be registered")
	}
	if e2.Description != "Get metrics" {
		t.Errorf("expected description %q, got %q", "Get metrics", e2.Description)
	}
}

func TestMCPServerManager_OnServerDisconnected(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	tools := []Entry{
		{ID: "tool1", Name: "server.tool1", Kind: KindMCPTool, Description: "Tool 1"},
		{ID: "tool2", Name: "server.tool2", Kind: KindMCPTool, Description: "Tool 2"},
	}

	_ = mgr.OnServerConnected("server-a", tools)

	// Verify tools exist before disconnect
	if _, ok := reg.Get("server-a/tool1"); !ok {
		t.Fatal("tool1 should exist before disconnect")
	}

	mgr.OnServerDisconnected("server-a")

	// Verify tools are removed after disconnect
	if _, ok := reg.Get("server-a/tool1"); ok {
		t.Error("tool1 should not exist after disconnect")
	}
	if _, ok := reg.Get("server-a/tool2"); ok {
		t.Error("tool2 should not exist after disconnect")
	}
}

func TestMCPServerManager_NoDanglingReferences(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	tools := []Entry{
		{ID: "rca", Name: "coroot.rca_report", Kind: KindMCPTool, Description: "RCA"},
	}

	_ = mgr.OnServerConnected("coroot", tools)
	mgr.OnServerDisconnected("coroot")

	// ListServerTools should return nil after disconnect
	result := mgr.ListServerTools("coroot")
	if result != nil {
		t.Errorf("ListServerTools() after disconnect should be nil, got %v", result)
	}

	// VisibleCapabilities should not include disconnected tools
	visible := reg.VisibleCapabilities("host", "chat")
	for _, e := range visible {
		if e.ID == "coroot/rca" {
			t.Error("disconnected tool should not appear in VisibleCapabilities")
		}
	}
}

func TestMCPServerManager_MultipleServersCoexist(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	corootTools := []Entry{
		{ID: "list_services", Name: "coroot.list_services", Kind: KindMCPTool, Description: "List services"},
	}
	labTools := []Entry{
		{ID: "create_env", Name: "lab.create_env", Kind: KindMCPTool, Description: "Create env"},
		{ID: "inject_fault", Name: "lab.inject_fault", Kind: KindMCPTool, Description: "Inject fault"},
	}

	if err := mgr.OnServerConnected("coroot", corootTools); err != nil {
		t.Fatalf("OnServerConnected(coroot) error = %v", err)
	}
	if err := mgr.OnServerConnected("lab", labTools); err != nil {
		t.Fatalf("OnServerConnected(lab) error = %v", err)
	}

	// Both servers' tools should be visible
	if _, ok := reg.Get("coroot/list_services"); !ok {
		t.Error("coroot tool should be visible")
	}
	if _, ok := reg.Get("lab/create_env"); !ok {
		t.Error("lab create_env should be visible")
	}
	if _, ok := reg.Get("lab/inject_fault"); !ok {
		t.Error("lab inject_fault should be visible")
	}

	// Disconnect one server, other should remain
	mgr.OnServerDisconnected("coroot")

	if _, ok := reg.Get("coroot/list_services"); ok {
		t.Error("coroot tool should be gone after disconnect")
	}
	if _, ok := reg.Get("lab/create_env"); !ok {
		t.Error("lab tool should still be visible after coroot disconnect")
	}
	if _, ok := reg.Get("lab/inject_fault"); !ok {
		t.Error("lab inject_fault should still be visible after coroot disconnect")
	}
}

func TestMCPServerManager_ListServerTools(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	tools := []Entry{
		{ID: "t1", Name: "server.t1", Kind: KindMCPTool, Description: "T1"},
		{ID: "t2", Name: "server.t2", Kind: KindMCPTool, Description: "T2"},
	}

	_ = mgr.OnServerConnected("my-server", tools)

	listed := mgr.ListServerTools("my-server")
	if len(listed) != 2 {
		t.Fatalf("ListServerTools() len = %d, want 2", len(listed))
	}

	// Verify the entries have correct prefixed IDs
	ids := map[string]bool{}
	for _, e := range listed {
		ids[e.ID] = true
	}
	if !ids["my-server/t1"] {
		t.Error("expected my-server/t1 in listed tools")
	}
	if !ids["my-server/t2"] {
		t.Error("expected my-server/t2 in listed tools")
	}
}

func TestMCPServerManager_ListServerTools_UnknownServer(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	result := mgr.ListServerTools("nonexistent")
	if result != nil {
		t.Errorf("ListServerTools(nonexistent) should be nil, got %v", result)
	}
}

func TestMCPServerManager_ReconnectServer(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	// First connection
	tools1 := []Entry{
		{ID: "old_tool", Name: "server.old_tool", Kind: KindMCPTool, Description: "Old"},
	}
	_ = mgr.OnServerConnected("server-x", tools1)

	// Reconnect with different tools (simulates server restart with new capabilities)
	tools2 := []Entry{
		{ID: "new_tool", Name: "server.new_tool", Kind: KindMCPTool, Description: "New"},
	}
	_ = mgr.OnServerConnected("server-x", tools2)

	// Old tool should be gone
	if _, ok := reg.Get("server-x/old_tool"); ok {
		t.Error("old_tool should be removed on reconnect")
	}

	// New tool should be present
	if _, ok := reg.Get("server-x/new_tool"); !ok {
		t.Error("new_tool should be registered after reconnect")
	}

	// ListServerTools should only show new tools
	listed := mgr.ListServerTools("server-x")
	if len(listed) != 1 {
		t.Fatalf("ListServerTools() len = %d, want 1", len(listed))
	}
	if listed[0].Name != "server.new_tool" {
		t.Errorf("expected new_tool, got %q", listed[0].Name)
	}
}

func TestMCPServerManager_EmptyServerID(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	err := mgr.OnServerConnected("", []Entry{
		{ID: "t1", Name: "t1", Kind: KindMCPTool, Description: "T1"},
	})
	if err == nil {
		t.Error("OnServerConnected with empty serverID should return error")
	}
}

func TestMCPServerManager_DisconnectUnknownServer(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	// Should not panic
	mgr.OnServerDisconnected("nonexistent")
}

func TestMCPServerManager_ToolsVisibleInCapabilities(t *testing.T) {
	reg := NewRegistry()
	mgr := NewMCPServerManager(reg)

	tools := []Entry{
		{
			ID:          "query",
			Name:        "coroot.query",
			Kind:        KindMCPTool,
			Description: "Query Coroot",
			Visibility: Visibility{
				SessionTypes: []string{"host"},
				Modes:        []string{"inspect", "execute"},
			},
		},
	}

	_ = mgr.OnServerConnected("coroot", tools)

	// Should be visible in host+inspect
	visible := reg.VisibleCapabilities("host", "inspect")
	found := false
	for _, e := range visible {
		if e.ID == "coroot/query" {
			found = true
		}
	}
	if !found {
		t.Error("coroot/query should be visible in host+inspect")
	}

	// Should NOT be visible in host+chat (mode restriction)
	visible = reg.VisibleCapabilities("host", "chat")
	for _, e := range visible {
		if e.ID == "coroot/query" {
			t.Error("coroot/query should NOT be visible in host+chat")
		}
	}

	// Should NOT be visible in workspace+inspect (session restriction)
	visible = reg.VisibleCapabilities("workspace", "inspect")
	for _, e := range visible {
		if e.ID == "coroot/query" {
			t.Error("coroot/query should NOT be visible in workspace+inspect")
		}
	}
}
