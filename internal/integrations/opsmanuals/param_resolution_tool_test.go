package opsmanuals

import (
	"context"
	"encoding/json"
	"testing"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsInstallsResolveOpsManualParams(t *testing.T) {
	registry := tooling.NewRegistry()
	service := core.NewService(core.NewMemoryStore())
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("resolve_ops_manual_params")
	if !ok {
		t.Fatal("resolve_ops_manual_params tool not registered")
	}
	meta := tool.Metadata()
	if meta.Name != "resolve_ops_manual_params" || meta.RiskLevel != tooling.ToolRiskLow {
		t.Fatalf("metadata = %#v, want low-risk resolve tool", meta)
	}
	if !hasAlias(meta.Aliases, "ops_manual.resolve_params") {
		t.Fatalf("aliases = %#v, want ops_manual.resolve_params", meta.Aliases)
	}
	if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "ops_manual_flow" || !meta.DeferByDefault {
		t.Fatalf("layer metadata = layer:%q pack:%q defer:%v, want deferred ops_manual_flow", meta.Layer, meta.Pack, meta.DeferByDefault)
	}
	if len(meta.Description) > 500 {
		t.Fatalf("description length = %d, want <= 500: %q", len(meta.Description), meta.Description)
	}
	for _, want := range []string{"Resolve parameters", "matched ops manual", "chat/session context", "safe read-only resolvers", "resolved parameters", "dynamic form fields"} {
		if !containsString([]string{meta.Description}, want) {
			t.Fatalf("description = %q, want %q", meta.Description, want)
		}
	}
	if !tool.IsReadOnly(json.RawMessage(`{"request_text":"排查 Redis"}`)) {
		t.Fatal("resolve_ops_manual_params must be read-only")
	}
	if tool.IsDestructive(json.RawMessage(`{"request_text":"排查 Redis"}`)) {
		t.Fatal("resolve_ops_manual_params must not be destructive")
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"request_text", "manual_id", "workflow_id", "operation_frame", "known_params", "metadata"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("schema missing %q: %s", name, string(tool.InputSchema()))
		}
	}
	if !containsString([]string{string(schema.Properties["operation_frame"])}, "target.name") ||
		!containsString([]string{string(schema.Properties["known_params"])}, "target_instance") {
		t.Fatalf("resolve tool lacks explicit target guidance: description=%q schema=%s", meta.Description, string(tool.InputSchema()))
	}
}

func TestResolveOpsManualParamsToolExecutesAndCachesForPreflight(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	manual.PreflightProbe = core.PreflightProbe{ID: "redis-readonly", ReadOnly: true, RequiredOutputs: []string{"redis_ping"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	searchTool, _ := registry.Get("search_ops_manuals")
	resolveTool, _ := registry.Get("resolve_ops_manual_params")
	preflightTool, _ := registry.Get("run_ops_manual_preflight")
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{SessionID: "sess-redis", TurnID: "turn-redis"})

	if _, err := searchTool.Execute(ctx, json.RawMessage(`{"text":"排查 Redis","metadata":{"selected_host":"server-local","resource_candidates":[{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92}]}}`)); err != nil {
		t.Fatal(err)
	}
	result, err := resolveTool.Execute(ctx, json.RawMessage(`{"manual_id":"manual-redis-rca","metadata":{"selected_host":"server-local","resource_candidates":[{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Display == nil || result.Display.Type != "ops_manual_param_resolution" {
		t.Fatalf("display = %#v, want param resolution", result.Display)
	}
	var payload core.ParamResolutionResult
	if err := json.Unmarshal([]byte(result.Content), &struct{}{}); err != nil {
		t.Fatalf("model content is not JSON: %v", err)
	}
	if err := json.Unmarshal(result.Display.Data, &payload); err != nil {
		t.Fatalf("display data is not ParamResolutionResult: %v", err)
	}
	if payload.Status != core.ParamResolutionResolved {
		t.Fatalf("payload = %#v, want resolved", payload)
	}

	preflight, err := preflightTool.Execute(ctx, json.RawMessage(`{"manual_id":"manual-redis-rca","operation_frame":{},"parameters":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	var preflightPayload core.PreflightResult
	if err := json.Unmarshal([]byte(preflight.Content), &preflightPayload); err != nil {
		t.Fatal(err)
	}
	if preflightPayload.Status != core.PreflightStatusPassed {
		t.Fatalf("preflight payload = %#v, want passed using cached resolved params", preflightPayload)
	}
}

func TestResolveOpsManualParamsToolUsesExecutionHostWhenModelOmitsMetadata(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	manual.RequiredContext.RequiredInputs = []string{"target_instance", "execution_surface"}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo, core.WithResourceDiscovery(singleRedisDiscovery{}))
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	searchTool, _ := registry.Get("search_ops_manuals")
	resolveTool, _ := registry.Get("resolve_ops_manual_params")
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-host-fallback",
		TurnID:    "turn-host-fallback",
		HostID:    "server-local",
	})

	if _, err := searchTool.Execute(ctx, json.RawMessage(`{"text":"排查 Redis"}`)); err != nil {
		t.Fatal(err)
	}
	result, err := resolveTool.Execute(ctx, json.RawMessage(`{"manual_id":"manual-redis-rca"}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload core.ParamResolutionResult
	if err := json.Unmarshal(result.Display.Data, &payload); err != nil {
		t.Fatalf("display data is not ParamResolutionResult: %v", err)
	}
	if payload.Status != core.ParamResolutionResolved {
		t.Fatalf("payload = %#v, want resolved using HostID and discovery", payload)
	}
	if !resolvedParam(payload.ResolvedParams, "target_host", "server-local") {
		t.Fatalf("resolved params = %#v, want target_host from execution HostID", payload.ResolvedParams)
	}
	if !resolvedParam(payload.ResolvedParams, "target_instance", "docker:aiops-redis") {
		t.Fatalf("resolved params = %#v, want redis from discovery", payload.ResolvedParams)
	}
	if !resolvedParam(payload.ResolvedParams, "execution_surface", "docker exec aiops-redis") {
		t.Fatalf("resolved params = %#v, want execution surface from discovery", payload.ResolvedParams)
	}
	for _, field := range payload.Fields {
		if field.ID == "target_host" {
			t.Fatalf("fields = %#v, should not ask target_host already bound to session HostID", payload.Fields)
		}
	}
}

func TestResolveOpsManualParamsToolPreservesExplicitTargetOverCurrentHost(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	searchTool, _ := registry.Get("search_ops_manuals")
	resolveTool, _ := registry.Get("resolve_ops_manual_params")
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-explicit-target",
		TurnID:    "turn-explicit-target",
		HostID:    "server-local",
	})

	if _, err := searchTool.Execute(ctx, json.RawMessage(`{
		"text":"Troubleshoot Redis instance aiops-redis-b on current host server-local using ops manuals",
		"operation_frame":{
			"object_type":"redis",
			"target":{"type":"redis","name":"aiops-redis-b"},
			"target_scope":{"hosts":["server-local"]},
			"operation":{"target_type":"redis","action":"rca_or_repair"}
		},
		"metadata":{"resource_candidates":[
			{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92},
			{"id":"docker:aiops-redis-b","name":"aiops-redis-b","type":"redis","source":"docker","confidence":0.92}
		]}
	}`)); err != nil {
		t.Fatal(err)
	}
	result, err := resolveTool.Execute(ctx, json.RawMessage(`{
		"manual_id":"manual-redis-rca",
		"metadata":{"resource_candidates":[
			{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92},
			{"id":"docker:aiops-redis-b","name":"aiops-redis-b","type":"redis","source":"docker","confidence":0.92}
		]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload core.ParamResolutionResult
	if err := json.Unmarshal(result.Display.Data, &payload); err != nil {
		t.Fatalf("display data is not ParamResolutionResult: %v", err)
	}
	if payload.Status != core.ParamResolutionResolved {
		t.Fatalf("payload = %#v, want resolved explicit target", payload)
	}
	if !resolvedParam(payload.ResolvedParams, "target_host", "server-local") {
		t.Fatalf("resolved params = %#v, want current host preserved as target_host", payload.ResolvedParams)
	}
	if !resolvedParam(payload.ResolvedParams, "target_instance", "docker:aiops-redis-b") {
		t.Fatalf("resolved params = %#v, want explicit target, not current host", payload.ResolvedParams)
	}
	if len(payload.Fields) != 0 {
		t.Fatalf("fields = %#v, want no confirmation form", payload.Fields)
	}
}

func TestResolveOpsManualParamsToolDoesNotPromoteCurrentHostToMiddlewareInstance(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	searchTool, _ := registry.Get("search_ops_manuals")
	resolveTool, _ := registry.Get("resolve_ops_manual_params")
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-host-not-instance",
		TurnID:    "turn-host-not-instance",
		HostID:    "server-local",
	})

	if _, err := searchTool.Execute(ctx, json.RawMessage(`{
		"text":"Troubleshoot Redis on current host server-local using ops manuals",
		"operation_frame":{
			"object_type":"redis",
			"target":{"type":"redis"},
			"target_scope":{"hosts":["server-local"]},
			"operation":{"target_type":"redis","action":"rca_or_repair"}
		},
		"metadata":{"resource_candidates":[
			{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92},
			{"id":"docker:aiops-redis-b","name":"aiops-redis-b","type":"redis","source":"docker","confidence":0.92}
		]}
	}`)); err != nil {
		t.Fatal(err)
	}
	result, err := resolveTool.Execute(ctx, json.RawMessage(`{
		"manual_id":"manual-redis-rca",
		"metadata":{"resource_candidates":[
			{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92},
			{"id":"docker:aiops-redis-b","name":"aiops-redis-b","type":"redis","source":"docker","confidence":0.92}
		]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload core.ParamResolutionResult
	if err := json.Unmarshal(result.Display.Data, &payload); err != nil {
		t.Fatalf("display data is not ParamResolutionResult: %v", err)
	}
	if payload.Status != core.ParamResolutionAmbiguous || len(payload.Fields) != 1 || payload.Fields[0].ID != "target_instance" {
		t.Fatalf("payload = %#v, want target_instance ambiguity", payload)
	}
	if resolvedParam(payload.ResolvedParams, "target_instance", "server-local") {
		t.Fatalf("resolved params = %#v, current host must not become middleware instance", payload.ResolvedParams)
	}
}

func TestResolveOpsManualParamsToolWithoutManualIDGuidesSearchRefinement(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	if err := repo.SaveManual(testRedisManual()); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	resolveTool, _ := registry.Get("resolve_ops_manual_params")

	result, err := resolveTool.Execute(context.Background(), json.RawMessage(`{"request_text":"排查 Redis"}`))
	if err != nil {
		t.Fatalf("Execute returned hard error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool result should guide the model without hard tool error, got %q", result.Error)
	}
	if result.Display != nil {
		t.Fatalf("display = %#v, want no param card without manual_id", result.Display)
	}
	var payload struct {
		Status       string   `json:"status"`
		NextAction   string   `json:"next_action"`
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not model-facing JSON: %v", err)
	}
	if payload.Status != string(core.ParamResolutionUnresolved) || payload.NextAction != "refine_search" {
		t.Fatalf("payload = %#v, want unresolved refine_search", payload)
	}
	for _, want := range []string{
		"manual_id is missing",
		"search_ops_manuals returns a matched manual",
		"explicit operation_frame",
		"smallest missing question",
	} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestResolveOpsManualParamsAmbiguousInstructsStopForForm(t *testing.T) {
	payload := resolveOpsManualParamsModelResult(core.ParamResolutionResult{
		Status:     core.ParamResolutionAmbiguous,
		ManualID:   "manual-redis-rca",
		WorkflowID: "workflow-redis-rca",
		Fields: []core.ParamResolutionFormField{{
			ID:        "target_instance",
			Label:     "实例/服务",
			Type:      "resource_ref",
			UIControl: "select",
			Candidates: []core.ParamCandidate{
				{Value: "docker:aiops-redis", Label: "aiops-redis"},
				{Value: "docker:aiops-redis-b", Label: "aiops-redis-b"},
			},
		}},
		NextAction: "ask_user",
	})
	for _, want := range []string{
		"Stop tool use now",
		"wait for the user to submit the structured Agent-to-UI form",
		"Do not run host commands",
		"preflight",
	} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestResolveOpsManualParamsNeedInputIncludesDiscoveryBlocker(t *testing.T) {
	payload := resolveOpsManualParamsModelResult(core.ParamResolutionResult{
		Status:     core.ParamResolutionNeedUserInput,
		ManualID:   "manual-redis-rca",
		WorkflowID: "workflow-redis-rca",
		MissingParams: []core.MissingParam{{
			ParamRequirement: core.ParamRequirement{ID: "target_instance", Label: "实例/服务", Type: "resource_ref", Required: true},
			Reason:           "No Redis resource was discovered on server-local by read-only resource discovery.",
		}},
		Fields: []core.ParamResolutionFormField{{
			ID:          "target_instance",
			Label:       "实例/服务",
			Type:        "resource_ref",
			Required:    true,
			UIControl:   "select",
			Placeholder: "No Redis resource was discovered on server-local by read-only resource discovery.",
		}},
		NextAction: "ask_user",
	})
	if len(payload.Blockers) != 1 || payload.Blockers[0] == "" {
		t.Fatalf("blockers = %#v, want discovery blocker", payload.Blockers)
	}
	if !containsString(payload.Instructions, "When blockers are present") {
		t.Fatalf("instructions = %#v, want blocker guidance", payload.Instructions)
	}
}

type singleRedisDiscovery struct{}

func (singleRedisDiscovery) DiscoverHostResources(_ context.Context, host string) ([]core.ResourceCandidate, error) {
	if host != "server-local" {
		return nil, nil
	}
	return []core.ResourceCandidate{
		{ID: "docker:aiops-redis", Name: "aiops-redis", Type: "redis", Surface: "docker exec aiops-redis", Source: "docker", Confidence: 0.92, Evidence: "docker ps"},
	}, nil
}

func (singleRedisDiscovery) DiscoverExecutionSurfaces(context.Context, string) ([]core.ParamCandidate, error) {
	return nil, nil
}

func resolvedParam(params []core.ResolvedParam, id string, want any) bool {
	for _, param := range params {
		if param.ID == id && param.Value == want {
			return true
		}
	}
	return false
}
