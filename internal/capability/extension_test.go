package capability

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock extension for testing
// ---------------------------------------------------------------------------

type mockExtension struct {
	name        string
	entries     []Entry
	registerErr error
	unregErr    error
}

func (m *mockExtension) Name() string { return m.name }

func (m *mockExtension) Register(registry *Registry) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	return registry.RegisterBatch(m.entries)
}

func (m *mockExtension) Unregister(registry *Registry) error {
	if m.unregErr != nil {
		return m.unregErr
	}
	for _, e := range m.entries {
		registry.Unregister(e.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestExtensionManager_Register(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{
		name: "test-ext",
		entries: []Entry{
			{ID: "test-ext/tool1", Name: "test.tool1", Kind: KindMCPTool, Description: "Tool 1"},
			{ID: "test-ext/tool2", Name: "test.tool2", Kind: KindMCPTool, Description: "Tool 2"},
		},
	}

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify tools are in the registry
	if _, ok := reg.Get("test-ext/tool1"); !ok {
		t.Error("expected test-ext/tool1 to be registered")
	}
	if _, ok := reg.Get("test-ext/tool2"); !ok {
		t.Error("expected test-ext/tool2 to be registered")
	}
}

func TestExtensionManager_RegisterDuplicate(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{
		name:    "dup-ext",
		entries: []Entry{{ID: "dup/t1", Name: "dup.t1", Kind: KindMCPTool, Description: "T1"}},
	}

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	err := mgr.Register(ext)
	if err == nil {
		t.Error("second Register() should return error for duplicate")
	}
}

func TestExtensionManager_RegisterNil(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	err := mgr.Register(nil)
	if err == nil {
		t.Error("Register(nil) should return error")
	}
}

func TestExtensionManager_RegisterEmptyName(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{name: "", entries: nil}
	err := mgr.Register(ext)
	if err == nil {
		t.Error("Register with empty name should return error")
	}
}

func TestExtensionManager_RegisterFailure(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{
		name:        "fail-ext",
		registerErr: fmt.Errorf("simulated failure"),
	}

	err := mgr.Register(ext)
	if err == nil {
		t.Error("Register() should propagate extension registration error")
	}

	// Extension should not be tracked
	if mgr.Get("fail-ext") != nil {
		t.Error("failed extension should not be tracked")
	}
}

func TestExtensionManager_Unregister(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{
		name: "removable",
		entries: []Entry{
			{ID: "removable/t1", Name: "removable.t1", Kind: KindMCPTool, Description: "T1"},
		},
	}

	_ = mgr.Register(ext)

	// Verify tool exists
	if _, ok := reg.Get("removable/t1"); !ok {
		t.Fatal("tool should exist before unregister")
	}

	if err := mgr.Unregister("removable"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Verify tool is removed
	if _, ok := reg.Get("removable/t1"); ok {
		t.Error("tool should be removed after unregister")
	}

	// Extension should no longer be tracked
	if mgr.Get("removable") != nil {
		t.Error("extension should not be tracked after unregister")
	}
}

func TestExtensionManager_UnregisterNotFound(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	err := mgr.Unregister("nonexistent")
	if err == nil {
		t.Error("Unregister(nonexistent) should return error")
	}
}

func TestExtensionManager_Get(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{name: "findme", entries: nil}
	_ = mgr.Register(ext)

	found := mgr.Get("findme")
	if found == nil {
		t.Error("Get() should return registered extension")
	}
	if found.Name() != "findme" {
		t.Errorf("Get() name = %q, want %q", found.Name(), "findme")
	}

	if mgr.Get("missing") != nil {
		t.Error("Get(missing) should return nil")
	}
}

func TestExtensionManager_List(t *testing.T) {
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	_ = mgr.Register(&mockExtension{name: "ext-a", entries: nil})
	_ = mgr.Register(&mockExtension{name: "ext-b", entries: nil})

	names := mgr.List()
	if len(names) != 2 {
		t.Fatalf("List() len = %d, want 2", len(names))
	}

	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["ext-a"] || !nameSet["ext-b"] {
		t.Errorf("List() = %v, want [ext-a, ext-b]", names)
	}
}

func TestExtensionManager_IsolationConstraint(t *testing.T) {
	// Verify that extensions can only access the system through Registry.
	// The Extension interface only receives *Registry — no RuntimeKernel,
	// PromptCompiler, or PolicyEngine references are passed.
	reg := NewRegistry()
	mgr := NewExtensionManager(reg)

	ext := &mockExtension{
		name: "isolated",
		entries: []Entry{
			{ID: "isolated/tool", Name: "isolated.tool", Kind: KindMCPTool, Description: "Isolated tool"},
		},
	}

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// The extension's tools should be visible through the registry
	visible := reg.VisibleCapabilities("host", "chat")
	found := false
	for _, e := range visible {
		if e.ID == "isolated/tool" {
			found = true
		}
	}
	if !found {
		t.Error("extension tool should be visible through registry")
	}
}
