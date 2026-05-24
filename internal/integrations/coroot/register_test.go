package coroot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/plugins"
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

type staticClientProvider struct {
	client *Client
}

func (p staticClientProvider) CorootClient(context.Context) (*Client, error) {
	return p.client, nil
}

func registerCorootPluginForTest(t *testing.T, registry *mcp.Registry) {
	t.Helper()
	client, err := NewClient(ClientConfigFromEnv("http://localhost:8080"))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	spec, err := BuiltinPluginSpec(staticClientProvider{client: client}, client.BaseURL())
	if err != nil {
		t.Fatalf("BuiltinPluginSpec() error = %v", err)
	}
	if err := (&plugins.Registrar{MCP: registry}).Register(spec); err != nil {
		t.Fatalf("Register(plugin spec) error = %v", err)
	}
}

func TestBuiltinPluginSpecRequiresClientProvider(t *testing.T) {
	if _, err := BuiltinPluginSpec(nil, "http://localhost:8080"); err == nil {
		t.Fatal("expected nil provider to fail")
	}
}

func TestBuiltinPluginSpecWithoutDisplayEndpointRegistersBuiltinServer(t *testing.T) {
	client, err := NewClient(ClientConfigFromEnv("http://localhost:8080"))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	spec, err := BuiltinPluginSpec(staticClientProvider{client: client}, "")
	if err != nil {
		t.Fatalf("BuiltinPluginSpec() error = %v", err)
	}
	if len(spec.MCPServers) != 1 {
		t.Fatalf("MCPServers len = %d, want 1", len(spec.MCPServers))
	}
	cfg := spec.MCPServers[0].Config
	if cfg.Transport != "builtin" {
		t.Fatalf("Transport = %q, want builtin", cfg.Transport)
	}
	if len(cfg.Command) != 0 {
		t.Fatalf("Command = %#v, want empty command for in-process builtin server", cfg.Command)
	}
}

func TestBuiltinPluginSpecRegistersCorootServerAndTools(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()

	registerCorootPluginForTest(t, mcpRegistry)

	cfg, ok := mcpRegistry.GetServer("coroot")
	if !ok {
		t.Fatal("expected coroot server config to be registered")
	}
	if cfg.Name != "coroot" {
		t.Fatalf("GetServer().Name = %q, want coroot", cfg.Name)
	}
	if cfg.Transport != "builtin" {
		t.Fatalf("GetServer().Transport = %q, want builtin", cfg.Transport)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "http://localhost:8080" {
		t.Fatalf("GetServer().Command = %#v, want endpoint command", cfg.Command)
	}

	tools := mcpRegistry.ListServerTools("coroot")
	if len(tools) != 29 {
		t.Fatalf("ListServerTools(coroot) len = %d, want 29", len(tools))
	}
	if dynamic := mcpRegistry.DynamicTools(); len(dynamic) != 29 {
		t.Fatalf("DynamicTools() len = %d, want 29", len(dynamic))
	}

	for _, name := range []string{
		"coroot.list_services",
		"coroot.health_check",
		"coroot.list_projects",
		"coroot.get_project_status",
		"coroot.collect_rca_context",
		"coroot.service_metrics",
		"coroot.rca_report",
		"coroot.service_topology",
		"coroot.nodes_overview",
		"coroot.traces_overview",
		"coroot.deployments_overview",
		"coroot.risks_overview",
		"coroot.application_logs",
		"coroot.application_traces",
		"coroot.application_profiling",
		"coroot.get_node",
		"coroot.alert_rules",
		"coroot.incidents",
		"coroot.incident_timeline",
		"coroot.slo_status",
		"coroot.list_dashboards",
		"coroot.get_dashboard",
		"coroot.get_panel_data",
		"coroot.list_integrations",
		"coroot.get_integration",
		"coroot.list_inspections",
		"coroot.get_inspection_config",
		"coroot.get_application_categories",
		"coroot.get_custom_applications",
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

func TestCorootInputSchemasAreProviderCompatible(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()
	registerCorootPluginForTest(t, mcpRegistry)
	for _, tool := range mcpRegistry.ListServerTools("coroot") {
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
			t.Fatalf("%s input schema is invalid json: %v", tool.Metadata().Name, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("%s input schema type = %#v, want object", tool.Metadata().Name, schema["type"])
		}
		for _, forbidden := range []string{"oneOf", "anyOf", "allOf", "enum", "not"} {
			if _, ok := schema[forbidden]; ok {
				t.Fatalf("%s input schema has top-level %s, which is rejected by provider function schema validation: %s", tool.Metadata().Name, forbidden, string(tool.InputSchema()))
			}
		}
	}
}

func TestBuiltinPluginSpecLayersCorootToolsIntoDeferredPacks(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()
	registerCorootPluginForTest(t, mcpRegistry)
	tools := mcpRegistry.ListServerTools("coroot")

	list := corootToolByName(t, tools, "coroot.list_services").Metadata()
	if list.Layer != tooling.ToolLayerCore || list.Pack != "" || list.DeferByDefault {
		t.Fatalf("coroot.list_services metadata = layer:%q pack:%q defer:%v, want core", list.Layer, list.Pack, list.DeferByDefault)
	}

	wantPacks := map[string][]string{
		"coroot_rca":           {"coroot.collect_rca_context"},
		"coroot_metrics":       {"coroot.service_metrics", "coroot.slo_status"},
		"coroot_rca_reference": {"coroot.rca_report"},
		"coroot_topology":      {"coroot.service_topology"},
		"coroot_nodes":         {"coroot.nodes_overview", "coroot.get_node"},
		"coroot_traces":        {"coroot.traces_overview", "coroot.application_traces"},
		"coroot_deployments":   {"coroot.deployments_overview"},
		"coroot_risks":         {"coroot.risks_overview"},
		"coroot_logs":          {"coroot.application_logs"},
		"coroot_profiling":     {"coroot.application_profiling"},
		"coroot_incident":      {"coroot.alert_rules", "coroot.incidents", "coroot.incident_timeline"},
		"coroot_admin_read":    {"coroot.health_check", "coroot.list_projects", "coroot.get_project_status"},
		"coroot_dashboard":     {"coroot.list_dashboards", "coroot.get_dashboard", "coroot.get_panel_data"},
		"coroot_config_read":   {"coroot.list_integrations", "coroot.get_integration", "coroot.list_inspections", "coroot.get_inspection_config", "coroot.get_application_categories", "coroot.get_custom_applications"},
	}
	for pack, names := range wantPacks {
		for _, name := range names {
			meta := corootToolByName(t, tools, name).Metadata()
			if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != pack || !meta.DeferByDefault {
				t.Fatalf("%s metadata = layer:%q pack:%q defer:%v, want %s deferred", name, meta.Layer, meta.Pack, meta.DeferByDefault, pack)
			}
			if len(meta.Triggers) == 0 || strings.TrimSpace(meta.SearchHint) == "" {
				t.Fatalf("%s should expose triggers and search hint for tool_search discovery, got triggers=%v searchHint=%q", name, meta.Triggers, meta.SearchHint)
			}
		}
	}

	assembler := tooling.NewAssembler(tooling.NewRegistry(), mcpRegistry)
	defaultNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{}))
	if len(defaultNames) != 1 || defaultNames[0] != "coroot.list_services" {
		t.Fatalf("default Coroot tools = %v, want only coroot.list_services", defaultNames)
	}
	rcaNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"coroot_rca"}}))
	for _, want := range []string{"coroot.list_services", "coroot.collect_rca_context"} {
		if !containsCorootToolName(rcaNames, want) {
			t.Fatalf("coroot_rca tools = %v, want %s", rcaNames, want)
		}
	}
	for _, forbidden := range []string{"coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.slo_status"} {
		if containsCorootToolName(rcaNames, forbidden) {
			t.Fatalf("coroot_rca tools = %v, should not include drilldown %s", rcaNames, forbidden)
		}
	}
	for _, forbidden := range []string{"coroot.alert_rules", "coroot.incidents", "coroot.incident_timeline"} {
		if containsCorootToolName(rcaNames, forbidden) {
			t.Fatalf("coroot_rca tools = %v, should not include %s", rcaNames, forbidden)
		}
	}
	for pack, names := range wantPacks {
		packNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{pack}}))
		for _, want := range append([]string{"coroot.list_services"}, names...) {
			if !containsCorootToolName(packNames, want) {
				t.Fatalf("%s tools = %v, want %s", pack, packNames, want)
			}
		}
	}
	metricsNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"coroot_metrics"}}))
	for _, forbidden := range []string{"coroot.application_logs", "coroot.application_traces", "coroot.rca_report", "coroot.service_topology"} {
		if containsCorootToolName(metricsNames, forbidden) {
			t.Fatalf("coroot_metrics tools = %v, should not include %s", metricsNames, forbidden)
		}
	}
	incidentNames := corootToolNames(assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"coroot_incident"}}))
	for _, want := range []string{"coroot.list_services", "coroot.alert_rules", "coroot.incidents", "coroot.incident_timeline"} {
		if !containsCorootToolName(incidentNames, want) {
			t.Fatalf("coroot_incident tools = %v, want %s", incidentNames, want)
		}
	}
	listPrompt := corootToolByName(t, tools, "coroot.list_services").Prompt(tooling.PromptContext{})
	if !strings.Contains(listPrompt, "tool_search") {
		t.Fatalf("coroot.list_services prompt should direct hidden Coroot discovery through tool_search: %s", listPrompt)
	}
	for _, hidden := range []string{"coroot.collect_rca_context", "coroot.service_metrics", "coroot.application_logs", "coroot.application_traces"} {
		if strings.Contains(listPrompt, hidden) {
			t.Fatalf("coroot.list_services prompt should not name hidden tool %s: %s", hidden, listPrompt)
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
