package coroot

import (
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func corootToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %q", name)
	return nil
}

func assertCorootVisibility(t *testing.T, tool tooling.Tool, session, mode string, want bool) {
	t.Helper()
	got := tool.IsEnabled(tooling.ToolContext{SessionType: session, Mode: mode})
	if got != want {
		t.Fatalf("%s visibility for %s/%s = %v, want %v", tool.Metadata().Name, session, mode, got, want)
	}
}

func TestRegisterBuiltinsRequiresRegistry(t *testing.T) {
	if err := RegisterBuiltins(nil, "http://localhost:8080"); err == nil {
		t.Fatal("expected nil registry to fail")
	}
}

func TestRegisterBuiltinsRegistersCorootServerAndTools(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()

	if err := RegisterBuiltins(mcpRegistry, "http://localhost:8080"); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	cfg, ok := mcpRegistry.GetServer("coroot")
	if !ok {
		t.Fatal("expected coroot server config to be registered")
	}
	if cfg.Name != "coroot" {
		t.Fatalf("GetServer().Name = %q, want coroot", cfg.Name)
	}
	if cfg.Transport != "http" {
		t.Fatalf("GetServer().Transport = %q, want http", cfg.Transport)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "http://localhost:8080" {
		t.Fatalf("GetServer().Command = %#v, want endpoint command", cfg.Command)
	}

	tools := mcpRegistry.ListServerTools("coroot")
	if len(tools) != 7 {
		t.Fatalf("ListServerTools(coroot) len = %d, want 7", len(tools))
	}
	if dynamic := mcpRegistry.DynamicTools(); len(dynamic) != 7 {
		t.Fatalf("DynamicTools() len = %d, want 7", len(dynamic))
	}

	for _, name := range []string{
		"coroot.list_services",
		"coroot.service_metrics",
		"coroot.rca_report",
		"coroot.service_topology",
		"coroot.alert_rules",
		"coroot.incident_timeline",
		"coroot.slo_status",
	} {
		tool := corootToolByName(t, tools, name)
		meta := tool.Metadata()
		if !meta.HasMCPSource() {
			t.Fatalf("%s should expose MCP source metadata, got %#v", name, meta)
		}
		if meta.MCPInfo.ServerID != "coroot" {
			t.Fatalf("%s server id = %q, want coroot", name, meta.MCPInfo.ServerID)
		}
		if !tool.IsReadOnly(nil) {
			t.Fatalf("%s should be read-only", name)
		}
		if tool.IsDestructive(nil) {
			t.Fatalf("%s should not be destructive", name)
		}
		if !tool.IsConcurrencySafe(nil) {
			t.Fatalf("%s should be concurrency-safe", name)
		}
		assertCorootVisibility(t, tool, "host", "inspect", true)
		assertCorootVisibility(t, tool, "workspace", "execute", true)
		assertCorootVisibility(t, tool, "host", "chat", true)
		assertCorootVisibility(t, tool, "workspace", "chat", true)
	}
}
