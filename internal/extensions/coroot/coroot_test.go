package coroot

import (
	"testing"

	"aiops-v2/internal/capability"
)

func TestCorootExtension_Name(t *testing.T) {
	ext := NewCorootExtension("http://localhost:8080")
	if ext.Name() != "coroot" {
		t.Errorf("Name() = %q, want %q", ext.Name(), "coroot")
	}
}

func TestCorootExtension_Register(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewCorootExtension("http://localhost:8080")

	if err := ext.Register(reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify all 7 tools are registered
	expectedTools := []string{
		"coroot/list_services",
		"coroot/service_metrics",
		"coroot/rca_report",
		"coroot/service_topology",
		"coroot/alert_rules",
		"coroot/incident_timeline",
		"coroot/slo_status",
	}

	for _, id := range expectedTools {
		entry, ok := reg.Get(id)
		if !ok {
			t.Errorf("expected %q to be registered", id)
			continue
		}
		if entry.Kind != capability.KindMCPTool {
			t.Errorf("%q kind = %q, want %q", id, entry.Kind, capability.KindMCPTool)
		}
		if entry.Tool == nil {
			t.Errorf("%q should have non-nil Tool", id)
		}
	}
}

func TestCorootExtension_RegistersExactly7Tools(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewCorootExtension("http://localhost:8080")

	_ = ext.Register(reg)

	visible := reg.VisibleCapabilities("host", "inspect")
	count := 0
	for _, e := range visible {
		if e.Kind == capability.KindMCPTool {
			count++
		}
	}
	if count != 7 {
		t.Errorf("expected 7 MCP tools, got %d", count)
	}
}

func TestCorootExtension_Unregister(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewCorootExtension("http://localhost:8080")

	_ = ext.Register(reg)

	if err := ext.Unregister(reg); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Verify all tools are removed
	for _, id := range toolIDs {
		if _, ok := reg.Get(id); ok {
			t.Errorf("%q should be removed after Unregister", id)
		}
	}
}

func TestCorootExtension_ToolsAreReadOnly(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewCorootExtension("http://localhost:8080")
	_ = ext.Register(reg)

	for _, id := range toolIDs {
		entry, ok := reg.Get(id)
		if !ok {
			t.Fatalf("missing %q", id)
		}
		if !entry.Tool.IsReadOnly() {
			t.Errorf("%q should be read-only", id)
		}
		if entry.Tool.IsDestructive() {
			t.Errorf("%q should not be destructive", id)
		}
	}
}

func TestCorootExtension_Visibility(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewCorootExtension("http://localhost:8080")
	_ = ext.Register(reg)

	// Should be visible in host+inspect
	visible := reg.VisibleCapabilities("host", "inspect")
	found := 0
	for _, e := range visible {
		if e.Kind == capability.KindMCPTool {
			found++
		}
	}
	if found != 7 {
		t.Errorf("host+inspect: expected 7 tools visible, got %d", found)
	}

	// Should NOT be visible in host+chat (mode restriction)
	visible = reg.VisibleCapabilities("host", "chat")
	for _, e := range visible {
		if e.Kind == capability.KindMCPTool {
			t.Errorf("coroot tools should not be visible in chat mode, found %q", e.ID)
		}
	}
}

func TestCorootExtension_ViaExtensionManager(t *testing.T) {
	reg := capability.NewRegistry()
	mgr := capability.NewExtensionManager(reg)
	ext := NewCorootExtension("http://localhost:8080")

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("ExtensionManager.Register() error = %v", err)
	}

	// Verify tools are accessible
	if _, ok := reg.Get("coroot/list_services"); !ok {
		t.Error("coroot tools should be accessible via ExtensionManager")
	}

	// Unregister via manager
	if err := mgr.Unregister("coroot"); err != nil {
		t.Fatalf("ExtensionManager.Unregister() error = %v", err)
	}

	if _, ok := reg.Get("coroot/list_services"); ok {
		t.Error("coroot tools should be removed after ExtensionManager.Unregister")
	}
}
