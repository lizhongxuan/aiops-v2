package generator

import (
	"testing"

	"aiops-v2/internal/capability"
)

func TestGeneratorExtension_Name(t *testing.T) {
	ext := NewGeneratorExtension()
	if ext.Name() != "generator" {
		t.Errorf("Name() = %q, want %q", ext.Name(), "generator")
	}
}

func TestGeneratorExtension_Register(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()

	if err := ext.Register(reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify all 4 tools are registered
	for _, id := range generatorToolIDs {
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

func TestGeneratorExtension_FourStepFlow(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	// Verify the 4-step flow order: generate → lint → preview → publish_draft
	steps := []struct {
		id   string
		name string
	}{
		{"generator/generate", "generator.generate"},
		{"generator/lint", "generator.lint"},
		{"generator/preview", "generator.preview"},
		{"generator/publish_draft", "generator.publish_draft"},
	}

	for _, s := range steps {
		entry, ok := reg.Get(s.id)
		if !ok {
			t.Errorf("step %q not registered", s.id)
			continue
		}
		if entry.Name != s.name {
			t.Errorf("step %q name = %q, want %q", s.id, entry.Name, s.name)
		}
	}
}

func TestGeneratorExtension_Unregister(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()

	_ = ext.Register(reg)

	if err := ext.Unregister(reg); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	for _, id := range generatorToolIDs {
		if _, ok := reg.Get(id); ok {
			t.Errorf("%q should be removed after Unregister", id)
		}
	}
}

func TestGeneratorExtension_LintAndPreviewAreReadOnly(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	for _, id := range []string{"generator/lint", "generator/preview"} {
		entry, ok := reg.Get(id)
		if !ok {
			t.Fatalf("%q should be registered", id)
		}
		if !entry.Tool.IsReadOnly() {
			t.Errorf("%q should be read-only", id)
		}
	}
}

func TestGeneratorExtension_GenerateAndPublishAreNotReadOnly(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	for _, id := range []string{"generator/generate", "generator/publish_draft"} {
		entry, ok := reg.Get(id)
		if !ok {
			t.Fatalf("%q should be registered", id)
		}
		if entry.Tool.IsReadOnly() {
			t.Errorf("%q should not be read-only", id)
		}
	}
}

func TestGeneratorExtension_PublishDraftOnlyInExecuteMode(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	// publish_draft should only be visible in execute mode
	visible := reg.VisibleCapabilities("host", "plan")
	for _, e := range visible {
		if e.ID == "generator/publish_draft" {
			t.Error("publish_draft should not be visible in plan mode")
		}
	}

	visible = reg.VisibleCapabilities("host", "execute")
	found := false
	for _, e := range visible {
		if e.ID == "generator/publish_draft" {
			found = true
		}
	}
	if !found {
		t.Error("publish_draft should be visible in execute mode")
	}
}

func TestGeneratorExtension_NotVisibleInChatMode(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	visible := reg.VisibleCapabilities("host", "chat")
	for _, e := range visible {
		if e.Kind == capability.KindMCPTool {
			t.Errorf("generator tools should not be visible in chat mode, found %q", e.ID)
		}
	}
}

func TestGeneratorExtension_ViaExtensionManager(t *testing.T) {
	reg := capability.NewRegistry()
	mgr := capability.NewExtensionManager(reg)
	ext := NewGeneratorExtension()

	if err := mgr.Register(ext); err != nil {
		t.Fatalf("ExtensionManager.Register() error = %v", err)
	}

	if _, ok := reg.Get("generator/generate"); !ok {
		t.Error("generator tools should be accessible via ExtensionManager")
	}

	if err := mgr.Unregister("generator"); err != nil {
		t.Fatalf("ExtensionManager.Unregister() error = %v", err)
	}

	if _, ok := reg.Get("generator/generate"); ok {
		t.Error("generator tools should be removed after ExtensionManager.Unregister")
	}
}

func TestGeneratorExtension_NoneAreDestructive(t *testing.T) {
	reg := capability.NewRegistry()
	ext := NewGeneratorExtension()
	_ = ext.Register(reg)

	for _, id := range generatorToolIDs {
		entry, ok := reg.Get(id)
		if !ok {
			t.Fatalf("%q should be registered", id)
		}
		if entry.Tool.IsDestructive() {
			t.Errorf("%q should not be destructive", id)
		}
	}
}
