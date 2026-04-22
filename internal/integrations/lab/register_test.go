package lab

import (
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func labToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %q", name)
	return nil
}

func assertLabVisibility(t *testing.T, tool tooling.Tool, session, mode string, want bool) {
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

func TestRegisterBuiltinsRegistersLabServerAndTools(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()

	if err := RegisterBuiltins(mcpRegistry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	cfg, ok := mcpRegistry.GetServer("lab")
	if !ok {
		t.Fatal("expected lab server config to be registered")
	}
	if cfg.Name != "lab" {
		t.Fatalf("GetServer().Name = %q, want lab", cfg.Name)
	}
	if cfg.Transport != "local" {
		t.Fatalf("GetServer().Transport = %q, want local", cfg.Transport)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "lab" {
		t.Fatalf("GetServer().Command = %#v, want lab command", cfg.Command)
	}

	tools := mcpRegistry.ListServerTools("lab")
	if len(tools) != 4 {
		t.Fatalf("ListServerTools(lab) len = %d, want 4", len(tools))
	}

	expectations := map[string]struct {
		destructive bool
		executeOnly bool
	}{
		"lab.create_environment": {destructive: false, executeOnly: false},
		"lab.start_environment":  {destructive: false, executeOnly: false},
		"lab.inject_fault":       {destructive: true, executeOnly: true},
		"lab.reset_environment":  {destructive: true, executeOnly: false},
	}

	for name, want := range expectations {
		tool := labToolByName(t, tools, name)
		meta := tool.Metadata()
		if !meta.HasMCPSource() {
			t.Fatalf("%s should expose MCP source metadata, got %#v", name, meta)
		}
		if tool.IsReadOnly(nil) {
			t.Fatalf("%s should not be read-only", name)
		}
		if tool.IsDestructive(nil) != want.destructive {
			t.Fatalf("%s destructive = %v, want %v", name, tool.IsDestructive(nil), want.destructive)
		}
		if tool.IsConcurrencySafe(nil) {
			t.Fatalf("%s should not be concurrency-safe", name)
		}
		assertLabVisibility(t, tool, "host", "execute", true)
		assertLabVisibility(t, tool, "workspace", "inspect", false)
		assertLabVisibility(t, tool, "workspace", "plan", !want.executeOnly)
	}
}
