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

func TestRegisterBuiltinsLayersCorootToolsIntoDeferredPacks(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()
	if err := RegisterBuiltins(mcpRegistry, "http://localhost:8080"); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := mcpRegistry.ListServerTools("coroot")

	list := corootToolByName(t, tools, "coroot.list_services").Metadata()
	if list.Layer != tooling.ToolLayerCore || list.Pack != "" || list.DeferByDefault {
		t.Fatalf("coroot.list_services metadata = layer:%q pack:%q defer:%v, want core", list.Layer, list.Pack, list.DeferByDefault)
	}
	for _, name := range []string{"coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.slo_status"} {
		meta := corootToolByName(t, tools, name).Metadata()
		if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "coroot_rca" || !meta.DeferByDefault {
			t.Fatalf("%s metadata = layer:%q pack:%q defer:%v, want coroot_rca deferred", name, meta.Layer, meta.Pack, meta.DeferByDefault)
		}
	}
	for _, name := range []string{"coroot.alert_rules", "coroot.incident_timeline"} {
		meta := corootToolByName(t, tools, name).Metadata()
		if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "coroot_incident" || !meta.DeferByDefault {
			t.Fatalf("%s metadata = layer:%q pack:%q defer:%v, want coroot_incident deferred", name, meta.Layer, meta.Pack, meta.DeferByDefault)
		}
	}

	assembler := tooling.NewAssembler(tooling.NewRegistry(), mcpRegistry)
	defaultNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{}))
	if len(defaultNames) != 1 || defaultNames[0] != "coroot.list_services" {
		t.Fatalf("default Coroot tools = %v, want only coroot.list_services", defaultNames)
	}
	rcaNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"coroot_rca"}}))
	for _, want := range []string{"coroot.list_services", "coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.slo_status"} {
		if !containsCorootToolName(rcaNames, want) {
			t.Fatalf("coroot_rca tools = %v, want %s", rcaNames, want)
		}
	}
	for _, forbidden := range []string{"coroot.alert_rules", "coroot.incident_timeline"} {
		if containsCorootToolName(rcaNames, forbidden) {
			t.Fatalf("coroot_rca tools = %v, should not include %s", rcaNames, forbidden)
		}
	}
	incidentNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"coroot_incident"}}))
	for _, want := range []string{"coroot.list_services", "coroot.alert_rules", "coroot.incident_timeline"} {
		if !containsCorootToolName(incidentNames, want) {
			t.Fatalf("coroot_incident tools = %v, want %s", incidentNames, want)
		}
	}
}

func corootToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func containsCorootToolName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
