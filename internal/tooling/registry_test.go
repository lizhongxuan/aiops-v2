package tooling

import (
	"context"
	"encoding/json"
	"testing"
)

type mockTool struct {
	meta        ToolMetadata
	enabled     bool
	readOnly    bool
	destructive bool
	concurrency bool
	description string
}

func (m *mockTool) Metadata() ToolMetadata        { return m.meta }
func (m *mockTool) InputSchema() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) OutputSchema() json.RawMessage { return nil }
func (m *mockTool) Description(_ json.RawMessage, _ DescribeContext) string {
	if m.description != "" {
		return m.description
	}
	return m.meta.Description
}
func (m *mockTool) Prompt(_ PromptContext) string                        { return m.description }
func (m *mockTool) IsEnabled(_ ToolContext) bool                         { return m.enabled }
func (m *mockTool) IsReadOnly(_ json.RawMessage) bool                    { return m.readOnly }
func (m *mockTool) IsDestructive(_ json.RawMessage) bool                 { return m.destructive }
func (m *mockTool) IsConcurrencySafe(_ json.RawMessage) bool             { return m.concurrency }
func (m *mockTool) ValidateInput(context.Context, json.RawMessage) error { return nil }
func (m *mockTool) CheckPermissions(context.Context, json.RawMessage) PermissionDecision {
	return PermissionDecision{Action: PermissionActionAllow}
}
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

func TestRegistryBuiltInPriority(t *testing.T) {
	r := NewRegistry()

	builtin := &mockTool{
		meta:        ToolMetadata{Name: "read_file", Origin: ToolOriginBuiltin, Description: "builtin"},
		enabled:     true,
		description: "builtin",
	}
	mcp := &mockTool{
		meta:        ToolMetadata{Name: "read_file", Origin: ToolOriginMCP, Description: "mcp"},
		enabled:     true,
		description: "mcp",
	}

	if err := r.Register(mcp); err != nil {
		t.Fatalf("Register(mcp) error = %v", err)
	}
	if err := r.Register(builtin); err != nil {
		t.Fatalf("Register(builtin) error = %v", err)
	}

	got, ok := r.Get("read_file")
	if !ok {
		t.Fatal("Get(read_file) returned false")
	}
	if got.Metadata().Description != "builtin" {
		t.Fatalf("Get(read_file) description = %q, want builtin", got.Metadata().Description)
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	if list[0].Metadata().Description != "builtin" {
		t.Fatalf("List()[0] description = %q, want builtin", list[0].Metadata().Description)
	}

	assembled := r.AssembleTools("host", "chat")
	if len(assembled) != 1 {
		t.Fatalf("AssembleTools() len = %d, want 1", len(assembled))
	}
	if assembled[0].Metadata().Description != "builtin" {
		t.Fatalf("AssembleTools()[0] description = %q, want builtin", assembled[0].Metadata().Description)
	}

	pool := r.AssembleToolPool("host", "chat")
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPool() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Desc != "builtin" {
		t.Fatalf("Info().Desc = %q, want builtin", info.Desc)
	}
}

func TestRegistryPrefersNonMCPWithoutOriginHints(t *testing.T) {
	r := NewRegistry()

	mcp := &mockTool{
		meta: ToolMetadata{
			Name:        "read_file",
			Description: "mcp",
			IsMCP:       true,
			MCPInfo:     MCPInfo{ServerID: "coroot", ToolName: "read_file"},
		},
		enabled:     true,
		description: "mcp",
	}
	nonMCP := &mockTool{
		meta: ToolMetadata{
			Name:        "read_file",
			Description: "builtin-like",
		},
		enabled:     true,
		description: "builtin-like",
	}

	if err := r.Register(nonMCP); err != nil {
		t.Fatalf("Register(nonMCP) error = %v", err)
	}
	if err := r.Register(mcp); err != nil {
		t.Fatalf("Register(mcp) error = %v", err)
	}

	got, ok := r.Get("read_file")
	if !ok {
		t.Fatal("Get(read_file) returned false")
	}
	if got.Metadata().Description != "builtin-like" {
		t.Fatalf("Get(read_file) description = %q, want builtin-like", got.Metadata().Description)
	}

	assembled := r.AssembleTools("host", "chat")
	if len(assembled) != 1 {
		t.Fatalf("AssembleTools() len = %d, want 1", len(assembled))
	}
	if assembled[0].Metadata().Description != "builtin-like" {
		t.Fatalf("AssembleTools()[0] description = %q, want builtin-like", assembled[0].Metadata().Description)
	}
}

func TestRegistryVisibilityAndUnregister(t *testing.T) {
	r := NewRegistry()

	hostOnly := &mockTool{
		meta:        ToolMetadata{Name: "host_only", Origin: ToolOriginMeta},
		enabled:     true,
		description: "host",
	}
	workspaceOnly := &mockTool{
		meta:        ToolMetadata{Name: "workspace_only", Origin: ToolOriginMeta},
		enabled:     true,
		description: "workspace",
	}

	if err := r.Register(hostOnly); err != nil {
		t.Fatalf("Register(hostOnly) error = %v", err)
	}
	if err := r.Register(workspaceOnly); err != nil {
		t.Fatalf("Register(workspaceOnly) error = %v", err)
	}

	if len(r.AssembleTools("host", "chat")) != 2 {
		t.Fatalf("AssembleTools should return all enabled tools")
	}

	r.Unregister("host_only")
	if _, ok := r.Get("host_only"); ok {
		t.Fatal("host_only should be removed after Unregister")
	}
}

func TestRegistryAssembleToolsHonorsStaticToolVisibility(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	static := &StaticTool{
		Meta: ToolMetadata{Name: "workspace_only", Description: "workspace static"},
		Visibility: Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"chat"},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	}

	if err := r.Register(static); err != nil {
		t.Fatalf("Register(static) error = %v", err)
	}

	if got := r.AssembleTools("host", "chat"); len(got) != 0 {
		t.Fatalf("AssembleTools(host, chat) len = %d, want 0", len(got))
	}

	got := r.AssembleTools("workspace", "chat")
	if len(got) != 1 {
		t.Fatalf("AssembleTools(workspace, chat) len = %d, want 1", len(got))
	}
	if got[0].Metadata().Name != "workspace_only" {
		t.Fatalf("AssembleTools(workspace, chat)[0].Name = %q, want workspace_only", got[0].Metadata().Name)
	}
}

func TestRegistryAssembleToolsWithOptionsAppliesFilterTransformAndExtraTools(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(&mockTool{
		meta:        ToolMetadata{Name: "read_file", Description: "builtin"},
		enabled:     true,
		readOnly:    true,
		concurrency: true,
		description: "builtin",
	}); err != nil {
		t.Fatalf("Register(read_file) error = %v", err)
	}
	if err := r.Register(&mockTool{
		meta:        ToolMetadata{Name: "write_file", Description: "writer"},
		enabled:     true,
		description: "writer",
	}); err != nil {
		t.Fatalf("Register(write_file) error = %v", err)
	}

	extra := &mockTool{
		meta: ToolMetadata{
			Name:        "service_metrics",
			Description: "dynamic mcp",
			IsMCP:       true,
			MCPInfo:     MCPInfo{ServerID: "coroot", ToolName: "service_metrics"},
		},
		enabled:     true,
		readOnly:    true,
		concurrency: true,
		description: "dynamic mcp",
	}

	assembled := r.AssembleToolsWithOptions("host", "chat", AssembleOptions{
		ExtraTools: []Tool{extra},
		MetadataTransform: func(meta ToolMetadata) ToolMetadata {
			if meta.Name == "read_file" {
				meta.ShouldDefer = true
			}
			return meta
		},
		Filter: func(_ Tool, _ ToolContext, meta ToolMetadata) bool {
			return meta.Name != "write_file"
		},
	})

	if len(assembled) != 2 {
		t.Fatalf("AssembleToolsWithOptions() len = %d, want 2", len(assembled))
	}
	if assembled[0].Metadata().Name != "read_file" {
		t.Fatalf("AssembleToolsWithOptions()[0].Name = %q, want read_file", assembled[0].Metadata().Name)
	}
	if !assembled[0].Metadata().ShouldDefer {
		t.Fatalf("expected transformed read_file metadata to set ShouldDefer, got %#v", assembled[0].Metadata())
	}
	if assembled[1].Metadata().Name != "service_metrics" {
		t.Fatalf("AssembleToolsWithOptions()[1].Name = %q, want service_metrics", assembled[1].Metadata().Name)
	}
	if !assembled[1].Metadata().HasMCPSource() {
		t.Fatalf("expected extra dynamic tool to retain MCP traits, got %#v", assembled[1].Metadata())
	}
}

func TestRegistryAssembleToolsWithOptionsAppliesLayerPackAndProfileFilters(t *testing.T) {
	r := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "legacy_default", Description: "old metadata remains visible"},
		{Name: "core_read", Description: "core", Layer: ToolLayerCore},
		{Name: "deferred_manual", Description: "deferred", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
		{Name: "internal_record", Description: "internal", Layer: ToolLayerInternal},
		{Name: "debug_trace", Description: "debug", Layer: ToolLayerDebug},
		{Name: "mutation_restart", Description: "mutation", Layer: ToolLayerMutation, Pack: "mutation", Mutating: true},
		{Name: "profile_only", Description: "profile", Layer: ToolLayerCore, Profiles: []string{"demo"}},
	} {
		if err := r.Register(&mockTool{
			meta:        meta,
			enabled:     true,
			readOnly:    !meta.Mutating,
			concurrency: true,
			description: meta.Description,
		}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	defaultNames := toolNamesForTest(r.AssembleToolsWithOptions("host", "chat", AssembleOptions{}))
	for _, want := range []string{"core_read", "legacy_default"} {
		if !containsToolNameForRegistryTest(defaultNames, want) {
			t.Fatalf("default names = %v, want %q", defaultNames, want)
		}
	}
	for _, forbidden := range []string{"deferred_manual", "internal_record", "debug_trace", "mutation_restart", "profile_only"} {
		if containsToolNameForRegistryTest(defaultNames, forbidden) {
			t.Fatalf("default names = %v, should not include %q", defaultNames, forbidden)
		}
	}

	manualNames := toolNamesForTest(r.AssembleToolsWithOptions("host", "chat", AssembleOptions{EnabledPacks: []string{"ops_manual_flow"}}))
	if !containsToolNameForRegistryTest(manualNames, "deferred_manual") {
		t.Fatalf("manual pack names = %v, want deferred_manual", manualNames)
	}

	mutationNames := toolNamesForTest(r.AssembleToolsWithOptions("host", "chat", AssembleOptions{EnabledPacks: []string{"mutation"}}))
	if !containsToolNameForRegistryTest(mutationNames, "mutation_restart") {
		t.Fatalf("mutation pack names = %v, want mutation_restart", mutationNames)
	}

	debugNames := toolNamesForTest(r.AssembleToolsWithOptions("host", "chat", AssembleOptions{Profile: "debug"}))
	if !containsToolNameForRegistryTest(debugNames, "debug_trace") {
		t.Fatalf("debug profile names = %v, want debug_trace", debugNames)
	}

	demoNames := toolNamesForTest(r.AssembleToolsWithOptions("host", "chat", AssembleOptions{Profile: "demo"}))
	if !containsToolNameForRegistryTest(demoNames, "profile_only") {
		t.Fatalf("demo profile names = %v, want profile_only", demoNames)
	}
}

func containsToolNameForRegistryTest(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func TestRegistryCompileContextWithMetadataFiltersOpsManualToolsAfterOptOut(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight", "host_read"} {
		if err := r.Register(&mockTool{
			meta:        ToolMetadata{Name: name, Description: name},
			enabled:     true,
			readOnly:    true,
			concurrency: true,
			description: name,
		}); err != nil {
			t.Fatalf("Register(%s) error = %v", name, err)
		}
	}

	assembled := r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualAction":  "skip_ops_manual",
		"opsManualSkipped": "true",
	})
	if len(assembled) != 1 {
		t.Fatalf("CompileContextWithMetadata() len = %d, want 1", len(assembled))
	}
	if assembled[0].Metadata().Name != "host_read" {
		t.Fatalf("remaining tool = %q, want host_read", assembled[0].Metadata().Name)
	}
}

func TestRegistryCompileContextWithMetadataEnablesOpsManualPackProgressively(t *testing.T) {
	r := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "host_read", Description: "host", Layer: ToolLayerCore},
		{Name: "search_ops_manuals", Description: "search", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
		{Name: "resolve_ops_manual_params", Description: "resolve", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
		{Name: "run_ops_manual_preflight", Description: "preflight", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
	} {
		if err := r.Register(&mockTool{
			meta:        meta,
			enabled:     true,
			readOnly:    true,
			concurrency: true,
			description: meta.Description,
		}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	initial := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", nil))
	for _, want := range []string{"host_read"} {
		if !containsToolNameForRegistryTest(initial, want) {
			t.Fatalf("initial tools = %v, want %q", initial, want)
		}
	}
	for _, forbidden := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"} {
		if containsToolNameForRegistryTest(initial, forbidden) {
			t.Fatalf("initial tools = %v, should not include %q", initial, forbidden)
		}
	}

	manualIntent := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"enableToolPack": "ops_manual_flow",
	}))
	if !containsToolNameForRegistryTest(manualIntent, "search_ops_manuals") {
		t.Fatalf("manual intent tools = %v, want search_ops_manuals", manualIntent)
	}
	for _, forbidden := range []string{"resolve_ops_manual_params", "run_ops_manual_preflight"} {
		if containsToolNameForRegistryTest(manualIntent, forbidden) {
			t.Fatalf("manual intent tools = %v, should not include %q before match/direct_execute", manualIntent, forbidden)
		}
	}

	afterMatch := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualMatched": "true",
		"enableToolPack":   "ops_manual_flow",
	}))
	if !containsToolNameForRegistryTest(afterMatch, "resolve_ops_manual_params") {
		t.Fatalf("after match tools = %v, want resolve_ops_manual_params", afterMatch)
	}
	if containsToolNameForRegistryTest(afterMatch, "run_ops_manual_preflight") {
		t.Fatalf("after match tools = %v, should not include run_ops_manual_preflight before params resolve", afterMatch)
	}

	afterDirect := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualMatched":       "true",
		"opsManualDirectExecute": "true",
		"enableToolPack":         "ops_manual_flow",
	}))
	if !containsToolNameForRegistryTest(afterDirect, "run_ops_manual_preflight") {
		t.Fatalf("after direct_execute tools = %v, want run_ops_manual_preflight", afterDirect)
	}

	afterParams := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualMatched":        "true",
		"opsManualParamsResolved": "true",
		"enableToolPack":          "ops_manual_flow",
		"enableTool":              "run_ops_manual_preflight",
	}))
	if !containsToolNameForRegistryTest(afterParams, "run_ops_manual_preflight") {
		t.Fatalf("after params tools = %v, want run_ops_manual_preflight", afterParams)
	}

	referenceOnly := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualAction":         "reference_ops_manual",
		"opsManualMatched":        "true",
		"opsManualParamsResolved": "true",
		"enableToolPack":          "ops_manual_flow",
		"enableTool":              "run_ops_manual_preflight",
	}))
	if containsToolNameForRegistryTest(referenceOnly, "run_ops_manual_preflight") {
		t.Fatalf("reference tools = %v, should not include run_ops_manual_preflight", referenceOnly)
	}

	skipped := toolNamesForTest(r.CompileContextWithMetadata("host", "chat", map[string]string{
		"opsManualAction":         "skip_ops_manual",
		"opsManualMatched":        "true",
		"opsManualParamsResolved": "true",
		"enableToolPack":          "ops_manual_flow",
		"enableTool":              "run_ops_manual_preflight",
	}))
	if !containsToolNameForRegistryTest(skipped, "host_read") || len(skipped) != 1 {
		t.Fatalf("skipped tools = %v, want only host_read", skipped)
	}
}

func TestRegistryAssembleToolPoolWithOptionsPrefersBuiltinOverExtraMCPConflict(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(&mockTool{
		meta:        ToolMetadata{Name: "read_file", Description: "builtin"},
		enabled:     true,
		description: "builtin",
	}); err != nil {
		t.Fatalf("Register(read_file) error = %v", err)
	}

	pool := r.AssembleToolPoolWithOptions("host", "chat", AssembleOptions{
		ExtraTools: []Tool{
			&mockTool{
				meta: ToolMetadata{
					Name:        "read_file",
					Description: "mcp",
					IsMCP:       true,
					MCPInfo:     MCPInfo{ServerID: "filesystem", ToolName: "read_file"},
				},
				enabled:     true,
				description: "mcp",
			},
		},
	})

	if len(pool) != 1 {
		t.Fatalf("AssembleToolPoolWithOptions() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Desc != "builtin" {
		t.Fatalf("Info().Desc = %q, want builtin", info.Desc)
	}
}

func TestToolMetadataAndResultSpillHelpers(t *testing.T) {
	meta := ToolMetadata{ResultBudget: ResultBudget{MaxInlineResultBytes: 8192}}
	if got := meta.EffectiveResultBudget(4096); got.MaxInlineResultBytes != 8192 {
		t.Fatalf("EffectiveResultBudget().MaxInlineResultBytes = %d, want 8192", got.MaxInlineResultBytes)
	}

	zeroBudget := ToolMetadata{}
	if got := zeroBudget.EffectiveResultBudget(4096); got.MaxInlineResultBytes != 4096 {
		t.Fatalf("EffectiveResultBudget().MaxInlineResultBytes = %d, want 4096", got.MaxInlineResultBytes)
	}

	result := ToolResult{ResultBudget: ResultBudget{MaxInlineResultBytes: 2048, SpillPolicy: ResultSpillPolicyExternalize}}
	if got := result.EffectiveResultBudget(4096); got.MaxInlineResultBytes != 2048 {
		t.Fatalf("ToolResult.EffectiveResultBudget().MaxInlineResultBytes = %d, want 2048", got.MaxInlineResultBytes)
	}
	if got := result.EffectiveResultBudget(4096); got.SpillPolicy != ResultSpillPolicyExternalize {
		t.Fatalf("ToolResult.EffectiveResultBudget().SpillPolicy = %q, want %q", got.SpillPolicy, ResultSpillPolicyExternalize)
	}
	if result.HasSpill() {
		t.Fatal("HasSpill() = true, want false")
	}

	result.Spill = &ResultSpill{
		ID:          "spill-1",
		ContentType: "text/plain",
		Bytes:       12,
	}
	if !result.HasSpill() {
		t.Fatal("HasSpill() = false, want true")
	}
	if result.Spill.ID != "spill-1" {
		t.Fatalf("Spill.ID = %q, want spill-1", result.Spill.ID)
	}

	result.References = []ResultReference{
		{Kind: ResultReferenceKindCard, CardRef: "card-1"},
	}
	if !result.HasReferences() {
		t.Fatal("HasReferences() = false, want true")
	}
}
