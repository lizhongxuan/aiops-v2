package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

type mockTool struct {
	meta tooling.ToolMetadata
}

func (m mockTool) Metadata() tooling.ToolMetadata { return m.meta }

func (m mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (m mockTool) OutputSchema() json.RawMessage { return nil }

func (m mockTool) Description(_ json.RawMessage, _ tooling.DescribeContext) string {
	return m.meta.Description
}

func (m mockTool) Prompt(_ tooling.PromptContext) string { return m.meta.Description }

func (m mockTool) IsEnabled(tooling.ToolContext) bool { return true }

func (m mockTool) IsReadOnly(json.RawMessage) bool { return true }

func (m mockTool) IsDestructive(json.RawMessage) bool { return false }

func (m mockTool) IsConcurrencySafe(json.RawMessage) bool { return true }

func (m mockTool) ValidateInput(context.Context, json.RawMessage) error { return nil }

func (m mockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}

func (m mockTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func toolNamesForTest(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func TestRegistryRegisterServerAndGet(t *testing.T) {
	r := NewRegistry()

	cfg := ServerConfig{
		ID:        "coroot",
		Name:      "Coroot",
		Transport: "stdio",
		Command:   []string{"coroot-mcp"},
	}
	if err := r.RegisterServer(cfg); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}

	got, ok := r.GetServer("coroot")
	if !ok {
		t.Fatal("expected server config to be registered")
	}
	if got.Name != "Coroot" {
		t.Fatalf("GetServer().Name = %q, want %q", got.Name, "Coroot")
	}
	got.Command[0] = "changed"
	again, ok := r.GetServer("coroot")
	if !ok {
		t.Fatal("expected server config to remain registered")
	}
	if again.Command[0] != "coroot-mcp" {
		t.Fatalf("GetServer() should return a cloned command slice, got %#v", again.Command)
	}
}

func TestRegistryServerManifestAppliesToDynamicToolDiscovery(t *testing.T) {
	r := NewRegistry()

	cfg := ServerConfig{
		ID:                   "synthetic-web",
		Name:                 "Synthetic Web",
		Transport:            "stdio",
		Command:              []string{"synthetic-web-mcp"},
		Source:               "plugin",
		CapabilityDomain:     "public_web",
		ResourceTypes:        []string{"public_web", "url"},
		OperationKinds:       []string{"search", "read"},
		DefaultLoadingPolicy: "deferred",
		RiskLevel:            "low",
		HealthCheckType:      "tool_ping",
		OwnerSource:          "builtin",
		ToolPack:             "public_web",
		PermissionScope:      "read",
		PromptBudgetClass:    "compact",
		SchemaBudgetClass:    "on_demand",
		DiscoveryTags:        []string{"current", "internet"},
	}
	if err := r.RegisterServer(cfg); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	got, ok := r.GetServer("synthetic-web")
	if !ok {
		t.Fatal("expected server config to be registered")
	}
	if got.CapabilityDomain != "public_web" || got.HealthCheckType != "tool_ping" || got.OwnerSource != "builtin" {
		t.Fatalf("manifest fields were not preserved: %+v", got)
	}
	got.ResourceTypes[0] = "changed"
	again, _ := r.GetServer("synthetic-web")
	if again.ResourceTypes[0] != "public_web" {
		t.Fatalf("GetServer() should clone manifest slices, got %+v", again.ResourceTypes)
	}

	if err := r.OnServerConnected("synthetic-web", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "web.search", Description: "Search public web"}}}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}
	tools := r.ListServerTools("synthetic-web")
	if len(tools) != 1 {
		t.Fatalf("ListServerTools len = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if meta.Pack != "public_web" || meta.Layer != tooling.ToolLayerDeferred || !meta.DeferByDefault {
		t.Fatalf("metadata loading policy = layer:%q pack:%q deferred:%v", meta.Layer, meta.Pack, meta.DeferByDefault)
	}
	if meta.RiskLevel != tooling.ToolRiskLow {
		t.Fatalf("risk = %q, want low", meta.RiskLevel)
	}
	discovery := meta.EffectiveDiscovery()
	if discovery.DiscoveryGroup != "public_web" {
		t.Fatalf("discovery group = %q, want public_web", discovery.DiscoveryGroup)
	}
	for _, want := range []string{"public_web", "url"} {
		if !containsString(discovery.ResourceTypes, want) {
			t.Fatalf("resource types = %#v, missing %q", discovery.ResourceTypes, want)
		}
	}
	for _, want := range []string{"read", "search"} {
		if !containsString(discovery.OperationKinds, want) {
			t.Fatalf("operation kinds = %#v, missing %q", discovery.OperationKinds, want)
		}
	}
	if discovery.LoadingPolicy != tooling.ToolLoadingPolicyDeferred || !discovery.RequiresSelect {
		t.Fatalf("discovery loading/select = %q/%v", discovery.LoadingPolicy, discovery.RequiresSelect)
	}
	if discovery.PermissionScope != "read" || discovery.PromptBudgetClass != "compact" || discovery.SchemaBudgetClass != "on_demand" {
		t.Fatalf("discovery budget/scope = %+v", discovery)
	}
}

func TestRegistryRegistersRunnerCapabilityAsMCPWorkflowEditor(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterRunnerCapability([]tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "workflow.get_snapshot", Pack: "workflow_editor"}}}); err != nil {
		t.Fatalf("RegisterRunnerCapability() error = %v", err)
	}
	cfg, ok := r.GetServer("runner")
	if !ok {
		t.Fatal("runner server not registered")
	}
	if cfg.CapabilityDomain != "runner" || cfg.ToolPack != "workflow_editor" || cfg.Source != "builtin" {
		t.Fatalf("runner config = %+v, want runner workflow_editor builtin capability", cfg)
	}
	tools := r.ListServerTools("runner")
	if len(tools) != 1 {
		t.Fatalf("runner tools = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if meta.MCPInfo.ServerID != "runner" || meta.Pack != "workflow_editor" {
		t.Fatalf("runner tool metadata = %+v", meta)
	}
}

func TestRegistryTracksServerInstructions(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterServer(ServerConfig{ID: "synthetic-docs", Name: "Synthetic Docs"}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	r.SetServerInstructions("synthetic-docs", "Use bounded resource reads only.")

	instructions := r.ListServerInstructions()
	if len(instructions) != 1 {
		t.Fatalf("instructions = %+v", instructions)
	}
	if instructions[0].ServerID != "synthetic-docs" || instructions[0].Hash == "" {
		t.Fatalf("instruction = %+v", instructions[0])
	}
	if strings.Contains(instructions[0].Text, "password=") {
		t.Fatalf("instruction leaked sensitive text: %+v", instructions[0])
	}
}

func TestRegistryReportsInstructionDelta(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterServer(ServerConfig{ID: "synthetic-docs"}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	r.SetServerInstructions("synthetic-docs", "Initial instruction.")
	state := MCPInstructionSessionState{}

	delta := r.ServerInstructionDelta(&state)
	if len(delta) != 1 || delta[0].Action != "added" {
		t.Fatalf("first delta = %+v", delta)
	}
	state.Apply(delta)
	r.SetServerInstructions("synthetic-docs", "Changed instruction.")
	delta = r.ServerInstructionDelta(&state)
	if len(delta) != 1 || delta[0].Action != "changed" {
		t.Fatalf("changed delta = %+v", delta)
	}
	state.Apply(delta)
	r.SetServerDisabled("synthetic-docs", true)
	delta = r.ServerInstructionDelta(&state)
	if len(delta) != 1 || delta[0].Action != "removed" {
		t.Fatalf("removed delta = %+v", delta)
	}
}

func TestMCPServerGovernanceMergesWithToolRisk(t *testing.T) {
	merged := MergeMCPGovernance(
		ServerConfig{ID: "synthetic", Name: "Synthetic"},
		ServerGovernance{ID: "synthetic", Permission: "readwrite", Risk: "high", RequiresExplicitUserApproval: true},
		tooling.ToolMetadata{Name: "synthetic.read", RiskLevel: tooling.ToolRiskLow},
	)
	if merged.RiskLevel != tooling.ToolRiskHigh {
		t.Fatalf("RiskLevel = %q, want high", merged.RiskLevel)
	}
	if !merged.RequiresApproval {
		t.Fatalf("RequiresApproval = false, want true")
	}
	if merged.MCPInfo.ServerID != "synthetic" || merged.MCPInfo.ServerName != "Synthetic" {
		t.Fatalf("MCPInfo = %+v", merged.MCPInfo)
	}
}

func TestMCPServerPermissionCannotDowngradeToolRisk(t *testing.T) {
	merged := MergeMCPGovernance(
		ServerConfig{ID: "synthetic", Disabled: true},
		ServerGovernance{ID: "synthetic", Permission: "readonly", Risk: "low"},
		tooling.ToolMetadata{Name: "synthetic.write", RiskLevel: tooling.ToolRiskCritical, Mutating: true},
	)
	if merged.RiskLevel != tooling.ToolRiskCritical {
		t.Fatalf("RiskLevel = %q, want critical", merged.RiskLevel)
	}
	if !merged.Mutating {
		t.Fatal("Mutating = false, want true")
	}
	if merged.Discovery.HiddenFromPrompt != true {
		t.Fatalf("Discovery = %+v, want hidden disabled server", merged.Discovery)
	}
}

func TestRegistryDynamicToolsLifecycle(t *testing.T) {
	r := NewRegistry()

	tools := []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "list_services", Description: "List services"}},
		mockTool{meta: tooling.ToolMetadata{Name: "service_metrics", Description: "Get metrics"}},
	}

	if err := r.OnServerConnected("coroot", tools); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	listed := r.ListServerTools("coroot")
	if len(listed) != 2 {
		t.Fatalf("ListServerTools() len = %d, want 2", len(listed))
	}
	meta := listed[0].Metadata()
	if !meta.HasMCPSource() {
		t.Fatalf("connected tool should report MCP source, got %#v", meta)
	}
	if meta.MCPInfo.ServerID != "coroot" {
		t.Fatalf("connected tool server id = %q, want coroot", meta.MCPInfo.ServerID)
	}

	all := r.DynamicTools()
	if len(all) != 2 {
		t.Fatalf("DynamicTools() len = %d, want 2", len(all))
	}

	r.OnServerDisconnected("coroot")
	if got := r.ListServerTools("coroot"); got != nil {
		t.Fatalf("ListServerTools() after disconnect = %#v, want nil", got)
	}
	if got := r.DynamicTools(); len(got) != 0 {
		t.Fatalf("DynamicTools() after disconnect len = %d, want 0", len(got))
	}
}

func TestRegistryDynamicToolsWithTenantScope(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterServer(ServerConfig{
		ID:        "tenant-a",
		Transport: "stdio",
		Command:   []string{"tenant-a"},
		TenantScope: TenantScope{
			TenantIDs: []string{"tenant-a"},
		},
	}); err != nil {
		t.Fatalf("RegisterServer tenant-a error = %v", err)
	}
	if err := r.RegisterServer(ServerConfig{
		ID:        "tenant-b",
		Transport: "stdio",
		Command:   []string{"tenant-b"},
		TenantScope: TenantScope{
			TenantIDs: []string{"tenant-b"},
		},
	}); err != nil {
		t.Fatalf("RegisterServer tenant-b error = %v", err)
	}
	if err := r.RegisterServer(ServerConfig{
		ID:        "unscoped",
		Transport: "stdio",
		Command:   []string{"unscoped"},
	}); err != nil {
		t.Fatalf("RegisterServer unscoped error = %v", err)
	}

	if err := r.OnServerConnected("tenant-a", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "tenant_a_tool", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}}}); err != nil {
		t.Fatalf("OnServerConnected tenant-a error = %v", err)
	}
	if err := r.OnServerConnected("tenant-b", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "tenant_b_tool", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}}}); err != nil {
		t.Fatalf("OnServerConnected tenant-b error = %v", err)
	}
	if err := r.OnServerConnected("unscoped", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "unscoped_tool"}}}); err != nil {
		t.Fatalf("OnServerConnected unscoped error = %v", err)
	}

	tenantATools := r.DynamicToolsWithOptions(DynamicToolOptions{TenantID: "tenant-a"})
	if len(tenantATools) != 1 || tenantATools[0].Metadata().Name != "tenant_a_tool" {
		t.Fatalf("tenant-a tools = %v, want only tenant_a_tool", toolNamesForTest(tenantATools))
	}
	tenantBTools := r.DynamicToolsWithOptions(DynamicToolOptions{TenantID: "tenant-b"})
	if len(tenantBTools) != 1 || tenantBTools[0].Metadata().Name != "tenant_b_tool" {
		t.Fatalf("tenant-b tools = %v, want only tenant_b_tool", toolNamesForTest(tenantBTools))
	}
	if all := r.DynamicTools(); len(all) != 3 {
		t.Fatalf("legacy DynamicTools len = %d, want unchanged all connected tools", len(all))
	}
}

func TestAssemblerCompileContextWithMetadataScopesMCPDynamicTools(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterServer(ServerConfig{
		ID:        "tenant-a",
		Transport: "stdio",
		Command:   []string{"tenant-a"},
		TenantScope: TenantScope{
			TenantIDs: []string{"tenant-a"},
		},
	}); err != nil {
		t.Fatalf("RegisterServer tenant-a error = %v", err)
	}
	if err := r.RegisterServer(ServerConfig{
		ID:        "tenant-b",
		Transport: "stdio",
		Command:   []string{"tenant-b"},
		TenantScope: TenantScope{
			TenantIDs: []string{"tenant-b"},
		},
	}); err != nil {
		t.Fatalf("RegisterServer tenant-b error = %v", err)
	}
	if err := r.OnServerConnected("tenant-a", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "tenant_a_tool", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}}}); err != nil {
		t.Fatalf("OnServerConnected tenant-a error = %v", err)
	}
	if err := r.OnServerConnected("tenant-b", []tooling.Tool{mockTool{meta: tooling.ToolMetadata{Name: "tenant_b_tool", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}}}); err != nil {
		t.Fatalf("OnServerConnected tenant-b error = %v", err)
	}
	assembler := tooling.NewAssembler(tooling.NewRegistry(), r)

	tenantATools := toolNamesForTest(assembler.CompileContextWithMetadata("host", "inspect", map[string]string{"tenantId": "tenant-a"}))
	if strings.Join(tenantATools, ",") != "tenant_a_tool" {
		t.Fatalf("tenant-a assembled tools = %#v, want tenant_a_tool", tenantATools)
	}
	tenantBTools := toolNamesForTest(assembler.CompileContextWithMetadata("host", "inspect", map[string]string{"tenantId": "tenant-b"}))
	if strings.Join(tenantBTools, ",") != "tenant_b_tool" {
		t.Fatalf("tenant-b assembled tools = %#v, want tenant_b_tool", tenantBTools)
	}
}

func TestMCPAlwaysLoadCanEnterCore(t *testing.T) {
	r := NewRegistry()
	if err := r.OnServerConnected("docs", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{
			Name:       "safe_search",
			AlwaysLoad: true,
			RiskLevel:  tooling.ToolRiskLow,
		}},
		mockTool{meta: tooling.ToolMetadata{
			Name:       "dangerous_write",
			AlwaysLoad: true,
			RiskLevel:  tooling.ToolRiskHigh,
			Mutating:   true,
		}},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	tools := r.ListServerTools("docs")
	if len(tools) != 2 {
		t.Fatalf("ListServerTools len = %d, want 2", len(tools))
	}
	byName := map[string]tooling.ToolMetadata{}
	for _, tool := range tools {
		byName[tool.Metadata().Name] = tool.Metadata()
	}
	if !byName["safe_search"].AlwaysLoad {
		t.Fatalf("safe_search metadata = %#v, want AlwaysLoad preserved for low-risk read-only MCP tool", byName["safe_search"])
	}
	if byName["dangerous_write"].AlwaysLoad {
		t.Fatalf("dangerous_write metadata = %#v, want AlwaysLoad stripped for high-risk mutating MCP tool", byName["dangerous_write"])
	}
	if byName["dangerous_write"].Layer != tooling.ToolLayerDeferred || !byName["dangerous_write"].DeferByDefault {
		t.Fatalf("dangerous_write metadata = %#v, want deferred after AlwaysLoad stripped", byName["dangerous_write"])
	}

	assembler := tooling.NewAssembler(tooling.NewRegistry(), r)
	defaultNames := toolNamesForTest(assembler.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{}))
	if !containsMCPRegistryTool(defaultNames, "safe_search") {
		t.Fatalf("default assembled tools = %v, want safe AlwaysLoad MCP tool", defaultNames)
	}
	if containsMCPRegistryTool(defaultNames, "dangerous_write") {
		t.Fatalf("default assembled tools = %v, should not include high-risk AlwaysLoad MCP tool", defaultNames)
	}
}

func TestMCPRegistryListsAndReadsResources(t *testing.T) {
	r := NewRegistry()
	if err := r.OnServerResources("coroot", []Resource{{
		URI:      "coroot://projects/default/schema",
		Name:     "Coroot schema",
		MimeType: "application/json",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetResourceContent("coroot", "coroot://projects/default/schema", ResourceContent{
		URI:      "coroot://projects/default/schema",
		MimeType: "application/json",
		Text:     `{"project":"default"}`,
	}); err != nil {
		t.Fatal(err)
	}

	resources := r.ListResources("coroot")
	if len(resources) != 1 || resources[0].URI == "" {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0].ServerID != "coroot" {
		t.Fatalf("resource server id = %q, want coroot", resources[0].ServerID)
	}

	content, ok, err := r.ReadResource(context.Background(), "coroot", "coroot://projects/default/schema")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || content.Text == "" || content.Digest == "" {
		t.Fatalf("ReadResource() = %#v, %v, want content with digest", content, ok)
	}
}

func containsMCPRegistryTool(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRegistryRejectsEmptyServerID(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterServer(ServerConfig{}); err == nil {
		t.Fatal("RegisterServer() should fail for empty ID")
	}
	if err := r.OnServerConnected("", nil); err == nil {
		t.Fatal("OnServerConnected() should fail for empty server ID")
	}
}

func TestRegistryRejectsCustomMCPServersWhenStrictPluginOnlyEnabled(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		RestrictToPluginOnly: []settings.CustomizationSurface{settings.SurfaceMCP},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	err := r.RegisterServer(ServerConfig{
		ID:        "custom-mcp",
		Transport: "stdio",
		Command:   []string{"custom-mcp"},
		Source:    string(settings.SourceUserSettings),
	})
	if err == nil {
		t.Fatal("expected strict plugin-only policy to reject userSettings MCP server")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
}

func TestRegistryAppliesAllowedMCPServersToCustomSources(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		AllowedMCPServers: []string{"allowed-mcp"},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	if err := r.RegisterServer(ServerConfig{
		ID:        "allowed-mcp",
		Transport: "stdio",
		Command:   []string{"allowed-mcp"},
		Source:    string(settings.SourceUserSettings),
	}); err != nil {
		t.Fatalf("expected listed custom MCP server to be allowed, got %v", err)
	}

	err := r.RegisterServer(ServerConfig{
		ID:        "blocked-mcp",
		Transport: "stdio",
		Command:   []string{"blocked-mcp"},
		Source:    string(settings.SourceUserSettings),
	})
	if err == nil {
		t.Fatal("expected unlisted custom MCP server to be rejected")
	}
	if !strings.Contains(err.Error(), "allowedMcpServers") {
		t.Fatalf("expected allowlist error, got %v", err)
	}

	if err := r.RegisterServer(ServerConfig{
		ID:        "plugin-mcp",
		Transport: "stdio",
		Command:   []string{"plugin-mcp"},
		Source:    "plugin",
	}); err != nil {
		t.Fatalf("expected plugin MCP server to bypass allowlist, got %v", err)
	}
}

func TestRegistryDynamicToolRefreshTokenStableAcrossOrdering(t *testing.T) {
	r := NewRegistry()

	tools := []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "z_tool", Description: "z"}},
		mockTool{meta: tooling.ToolMetadata{Name: "a_tool", Description: "a"}},
	}

	if err := r.OnServerConnected("coroot", tools); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	first := r.DynamicToolRefreshToken()
	if first == "" {
		t.Fatal("expected non-empty refresh token")
	}

	if err := r.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "a_tool", Description: "a"}},
		mockTool{meta: tooling.ToolMetadata{Name: "z_tool", Description: "z"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() reorder error = %v", err)
	}
	second := r.DynamicToolRefreshToken()
	if second != first {
		t.Fatalf("refresh token changed after reorder: %q vs %q", first, second)
	}

	if err := r.OnServerConnected("coroot", append(tools, mockTool{meta: tooling.ToolMetadata{Name: "b_tool", Description: "b"}})); err != nil {
		t.Fatalf("OnServerConnected() add error = %v", err)
	}
	third := r.DynamicToolRefreshToken()
	if third == first {
		t.Fatal("expected refresh token to change after tool set change")
	}
}

func TestAssemblerRefreshTokenTracksDynamicProviders(t *testing.T) {
	base := tooling.NewRegistry()
	baseTool := mockTool{meta: tooling.ToolMetadata{Name: "base_tool", Description: "base"}}
	if err := base.Register(baseTool); err != nil {
		t.Fatalf("base Register() error = %v", err)
	}

	dynamic := NewRegistry()
	if err := dynamic.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "dyn_tool", Description: "dyn"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}

	assembler := tooling.NewAssembler(base, dynamic)
	first := assembler.RefreshToken("host", "chat", tooling.AssembleOptions{})
	if first == "" {
		t.Fatal("expected non-empty assembler refresh token")
	}

	if err := dynamic.OnServerConnected("coroot", []tooling.Tool{
		mockTool{meta: tooling.ToolMetadata{Name: "dyn_tool", Description: "dyn"}},
		mockTool{meta: tooling.ToolMetadata{Name: "extra_tool", Description: "extra"}},
	}); err != nil {
		t.Fatalf("OnServerConnected() update error = %v", err)
	}
	second := assembler.RefreshToken("host", "chat", tooling.AssembleOptions{})
	if second == first {
		t.Fatal("expected assembler refresh token to change when dynamic tools change")
	}

	assembled := assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{})
	if len(assembled) != 1 {
		t.Fatalf("assembled tool count = %d, want only base tool before deferred select", len(assembled))
	}
	deferredCatalog := assembler.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{IncludeDeferredCatalog: true})
	if len(deferredCatalog) != 3 {
		t.Fatalf("deferred catalog tool count = %d, want base + 2 dynamic tools", len(deferredCatalog))
	}
}
