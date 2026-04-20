package lab

import (
	"testing"

	"aiops-v2/internal/capability"
)

func TestLabExtension_Name(t *testing.T) {
	ext := NewLabExtension()
	if ext.Name() != "lab" {
		t.Errorf("Name() = %q, want %q", ext.Name(), "lab")
	}
}

func TestLabExtension_Register(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()

	if err := ext.Register(reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify all 4 tools are registered
	for _, id := range labToolIDs {
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

func TestLabExtension_Unregister(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()

	_ = ext.Register(reg)

	if err := ext.Unregister(reg); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	for _, id := range labToolIDs {
		if _, ok := reg.Get(id); ok {
			t.Errorf("%q should be removed after Unregister", id)
		}
	}
}

func TestLabExtension_FaultInjectionIsDestructive(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()
	_ = ext.Register(reg)

	entry, ok := reg.Get("lab/inject_fault")
	if !ok {
		t.Fatal("inject_fault should be registered")
	}
	if !entry.Tool.IsDestructive() {
		t.Error("inject_fault should be destructive")
	}
	if entry.Tool.IsReadOnly() {
		t.Error("inject_fault should not be read-only")
	}
}

func TestLabExtension_ResetIsDestructive(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()
	_ = ext.Register(reg)

	entry, ok := reg.Get("lab/reset_environment")
	if !ok {
		t.Fatal("reset_environment should be registered")
	}
	if !entry.Tool.IsDestructive() {
		t.Error("reset_environment should be destructive")
	}
}

func TestLabExtension_CreateAndStartAreNotDestructive(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()
	_ = ext.Register(reg)

	for _, id := range []string{"lab/create_environment", "lab/start_environment"} {
		entry, ok := reg.Get(id)
		if !ok {
			t.Fatalf("%q should be registered", id)
		}
		if entry.Tool.IsDestructive() {
			t.Errorf("%q should not be destructive", id)
		}
	}
}

func TestLabExtension_Visibility(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewLabExtension()
	_ = ext.Register(reg)

	// inject_fault is only visible in execute mode
	visible := reg.VisibleCapabilities("host", "plan")
	for _, e := range visible {
		if e.ID == "lab/inject_fault" {
			t.Error("inject_fault should not be visible in plan mode")
		}
	}

	visible = reg.VisibleCapabilities("host", "execute")
	found := false
	for _, e := range visible {
		if e.ID == "lab/inject_fault" {
			found = true
		}
	}
	if !found {
		t.Error("inject_fault should be visible in execute mode")
	}

	// No lab tools visible in chat mode
	visible = reg.VisibleCapabilities("host", "chat")
	for _, e := range visible {
		if e.Kind == capability.KindMCPTool {
			t.Errorf("lab tools should not be visible in chat mode, found %q", e.ID)
		}
	}
}

func TestLabExtension_ViaExtensionManager(t *testing.T) {
	reg := capability.NewRegistry()
	mgr := capability.NewExtensionManager(reg)
	ext := NewLabExtension()

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("ExtensionManager.Register() error = %v", err)
	}

	if _, ok := reg.Get("lab/create_environment"); !ok {
		t.Error("lab tools should be accessible via ExtensionManager")
	}

	if err := mgr.Unregister("lab"); err != nil {
		t.Fatalf("ExtensionManager.Unregister() error = %v", err)
	}

	if _, ok := reg.Get("lab/create_environment"); ok {
		t.Error("lab tools should be removed after ExtensionManager.Unregister")
	}
}
