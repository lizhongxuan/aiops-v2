package generator

import (
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func generatorToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %q", name)
	return nil
}

func assertGeneratorVisibility(t *testing.T, tool tooling.Tool, session, mode string, want bool) {
	t.Helper()
	got := tool.IsEnabled(tooling.ToolContext{SessionType: session, Mode: mode})
	if got != want {
		t.Fatalf("%s visibility for %s/%s = %v, want %v", tool.Metadata().Name, session, mode, got, want)
	}
}

func TestRegisterBuiltinsRequiresRegistry(t *testing.T) {
	if err := RegisterBuiltins(nil); err == nil {
		t.Fatal("expected nil registry to fail")
	}
}

func TestRegisterBuiltinsRegistersGeneratorServerAndTools(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()

	if err := RegisterBuiltins(mcpRegistry, Options{Mode: "dev"}); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	cfg, ok := mcpRegistry.GetServer("generator")
	if !ok {
		t.Fatal("expected generator server config to be registered")
	}
	if cfg.Name != "generator" {
		t.Fatalf("GetServer().Name = %q, want generator", cfg.Name)
	}
	if cfg.Transport != "local" {
		t.Fatalf("GetServer().Transport = %q, want local", cfg.Transport)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "generator" {
		t.Fatalf("GetServer().Command = %#v, want generator command", cfg.Command)
	}

	tools := mcpRegistry.ListServerTools("generator")
	if len(tools) != 4 {
		t.Fatalf("ListServerTools(generator) len = %d, want 4", len(tools))
	}

	expectations := map[string]struct {
		readOnly    bool
		destructive bool
	}{
		"generator.generate":      {readOnly: false, destructive: false},
		"generator.lint":          {readOnly: true, destructive: false},
		"generator.preview":       {readOnly: true, destructive: false},
		"generator.publish_draft": {readOnly: false, destructive: false},
	}

	for name, want := range expectations {
		tool := generatorToolByName(t, tools, name)
		meta := tool.Metadata()
		if !meta.HasMCPSource() {
			t.Fatalf("%s should expose MCP source metadata, got %#v", name, meta)
		}
		if tool.IsReadOnly(nil) != want.readOnly {
			t.Fatalf("%s readOnly = %v, want %v", name, tool.IsReadOnly(nil), want.readOnly)
		}
		if tool.IsDestructive(nil) != want.destructive {
			t.Fatalf("%s destructive = %v, want %v", name, tool.IsDestructive(nil), want.destructive)
		}
		if tool.IsConcurrencySafe(nil) {
			t.Fatalf("%s should not be concurrency-safe", name)
		}
		assertGeneratorVisibility(t, tool, "host", "plan", name != "generator.publish_draft")
		assertGeneratorVisibility(t, tool, "workspace", "execute", true)
		assertGeneratorVisibility(t, tool, "workspace", "inspect", false)
	}
}

func TestRegisterBuiltinsHidesGeneratorToolsOutsideDevMode(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()

	if err := RegisterBuiltins(mcpRegistry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	if tools := mcpRegistry.DynamicTools(); len(tools) != 0 {
		t.Fatalf("DynamicTools() len = %d, want 0", len(tools))
	}
	if tools := mcpRegistry.ListServerTools("generator"); len(tools) != 0 {
		t.Fatalf("ListServerTools(generator) len = %d, want 0", len(tools))
	}

	prodRegistry := mcp.NewRegistry()
	if err := RegisterBuiltins(prodRegistry, Options{Mode: "prod"}); err != nil {
		t.Fatalf("RegisterBuiltins(prod) error = %v", err)
	}
	if tools := prodRegistry.DynamicTools(); len(tools) != 0 {
		t.Fatalf("prod DynamicTools() len = %d, want 0", len(tools))
	}
}
