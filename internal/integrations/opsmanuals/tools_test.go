package opsmanuals

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsInstallsSearchOpsManuals(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	service := core.NewService(repo)

	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("search_ops_manuals")
	if !ok {
		t.Fatal("search_ops_manuals tool not registered")
	}
	meta := tool.Metadata()
	if meta.Name != "search_ops_manuals" {
		t.Fatalf("name = %q, want search_ops_manuals", meta.Name)
	}
	if !hasAlias(meta.Aliases, "ops_manual.search") {
		t.Fatalf("aliases = %#v, want ops_manual.search", meta.Aliases)
	}
	if meta.Origin != tooling.ToolOriginBuiltin {
		t.Fatalf("origin = %q, want builtin", meta.Origin)
	}
	if meta.RiskLevel != tooling.ToolRiskLow {
		t.Fatalf("risk level = %q, want low", meta.RiskLevel)
	}
	if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "ops_manual_flow" || !meta.DeferByDefault {
		t.Fatalf("layer metadata = layer:%q pack:%q defer:%v, want deferred ops_manual_flow", meta.Layer, meta.Pack, meta.DeferByDefault)
	}
	for _, want := range []string{"repair", "recover", "restore", "修复", "恢复"} {
		if !containsString(meta.Triggers, want) {
			t.Fatalf("triggers = %#v, missing %q", meta.Triggers, want)
		}
	}
	discovery := meta.EffectiveDiscovery()
	if discovery.DiscoveryGroup != "runbook" || discovery.LoadingPolicy != tooling.ToolLoadingPolicyDeferred || !discovery.RequiresSelect {
		t.Fatalf("search_ops_manuals discovery = %+v, want deferred runbook select-only discovery", discovery)
	}
	for _, want := range []string{"manual", "runbook", "procedure"} {
		if !containsString(discovery.ResourceTypes, want) {
			t.Fatalf("search_ops_manuals resource types = %#v, missing %q", discovery.ResourceTypes, want)
		}
	}
	if len(meta.Description) > 600 {
		t.Fatalf("description length = %d, want <= 600: %q", len(meta.Description), meta.Description)
	}
	for _, want := range []string{"verified ops manuals", "decision", "direct_execute", "need_info", "adapt", "reference_only", "no_match", "operation_frame"} {
		if !strings.Contains(meta.Description, want) {
			t.Fatalf("description = %q, want %q", meta.Description, want)
		}
	}
	if !tool.IsReadOnly(json.RawMessage(`{"text":"排查 Redis"}`)) {
		t.Fatal("search_ops_manuals must be read-only")
	}
	if tool.IsDestructive(json.RawMessage(`{"text":"排查 Redis"}`)) {
		t.Fatal("search_ops_manuals must not be destructive")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "workspace", Mode: "inspect"}) {
		t.Fatal("search_ops_manuals should be visible in workspace inspect mode")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "plan"}) {
		t.Fatal("search_ops_manuals should be visible in host plan mode")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "execute"}) {
		t.Fatal("search_ops_manuals should be visible in host execute mode")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "chat"}) {
		t.Fatal("search_ops_manuals should be visible in host chat mode")
	}
	decision := tool.CheckPermissions(context.Background(), json.RawMessage(`{"text":"排查 Redis"}`))
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("permission = %#v, want allow", decision)
	}

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("input schema is invalid JSON: %v", err)
	}
	for _, name := range []string{"text", "metadata", "operation_frame", "limit"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("input schema missing %q: %s", name, string(tool.InputSchema()))
		}
	}
	if !strings.Contains(string(schema.Properties["operation_frame"]), "object_type") || !strings.Contains(string(schema.Properties["operation_frame"]), "operation.action") || !strings.Contains(string(schema.Properties["operation_frame"]), "target.name") {
		t.Fatalf("operation_frame schema lacks semantic guidance: %s", string(schema.Properties["operation_frame"]))
	}
}

func TestDomainRulesOpsManualToolMetadataIncludesWorkflowGuidance(t *testing.T) {
	repo := core.NewMemoryStore()
	service := core.NewService(repo)
	tool := newSearchOpsManualsTool(service, nil)
	prompt := tool.Prompt(tooling.PromptContext{})
	for _, want := range []string{
		"OpsManual workflow guidance",
		"Do not execute workflows directly.",
		"resolve_ops_manual_params",
		"run_ops_manual_preflight",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("ops manual prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRegisterBuiltinsInstallsRunOpsManualPreflight(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	service := core.NewService(repo)

	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("run_ops_manual_preflight")
	if !ok {
		t.Fatal("run_ops_manual_preflight tool not registered")
	}
	meta := tool.Metadata()
	if meta.Name != "run_ops_manual_preflight" {
		t.Fatalf("name = %q, want run_ops_manual_preflight", meta.Name)
	}
	if !hasAlias(meta.Aliases, "ops_manual.preflight") {
		t.Fatalf("aliases = %#v, want ops_manual.preflight", meta.Aliases)
	}
	if len(meta.Description) > 500 {
		t.Fatalf("description length = %d, want <= 500: %q", len(meta.Description), meta.Description)
	}
	for _, want := range []string{"read-only", "Node 0 preflight", "parameter readiness", "environment compatibility", "permission gaps", "probe evidence", "without executing"} {
		if !strings.Contains(meta.Description, want) {
			t.Fatalf("description = %q, want %q", meta.Description, want)
		}
	}
	if meta.RiskLevel != tooling.ToolRiskLow {
		t.Fatalf("risk level = %q, want low", meta.RiskLevel)
	}
	if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "ops_manual_flow" || !meta.DeferByDefault {
		t.Fatalf("layer metadata = layer:%q pack:%q defer:%v, want deferred ops_manual_flow", meta.Layer, meta.Pack, meta.DeferByDefault)
	}
	discovery := meta.EffectiveDiscovery()
	if discovery.DiscoveryGroup != "runbook" || discovery.LoadingPolicy != tooling.ToolLoadingPolicyDeferred || !discovery.RequiresSelect {
		t.Fatalf("run_ops_manual_preflight discovery = %+v, want deferred runbook select-only discovery", discovery)
	}
	for _, want := range []string{"manual", "runbook", "preflight"} {
		if !containsString(discovery.ResourceTypes, want) {
			t.Fatalf("run_ops_manual_preflight resource types = %#v, missing %q", discovery.ResourceTypes, want)
		}
	}
	for _, want := range []string{"preflight", "read"} {
		if !containsString(discovery.OperationKinds, want) {
			t.Fatalf("run_ops_manual_preflight operation kinds = %#v, missing %q", discovery.OperationKinds, want)
		}
	}
	if !tool.IsReadOnly(json.RawMessage(`{"manual_id":"manual-redis-rca","parameters":{"target_instance":"redis-01"}}`)) {
		t.Fatal("run_ops_manual_preflight must be read-only")
	}
	if tool.IsDestructive(json.RawMessage(`{"manual_id":"manual-redis-rca","parameters":{"target_instance":"redis-01"}}`)) {
		t.Fatal("run_ops_manual_preflight must not be destructive")
	}
	if !tool.IsConcurrencySafe(json.RawMessage(`{"manual_id":"manual-redis-rca","parameters":{"target_instance":"redis-01"}}`)) {
		t.Fatal("run_ops_manual_preflight should be concurrency safe")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "workspace", Mode: "plan"}) {
		t.Fatal("run_ops_manual_preflight should be visible in workspace plan mode")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "execute"}) {
		t.Fatal("run_ops_manual_preflight should be visible in host execute mode")
	}
	if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "chat"}) {
		t.Fatal("run_ops_manual_preflight should be visible in host chat mode")
	}
	if tool.IsEnabled(tooling.ToolContext{SessionType: "workspace", Mode: "inspect"}) {
		t.Fatal("run_ops_manual_preflight should not be visible in inspect mode")
	}
	decision := tool.CheckPermissions(context.Background(), json.RawMessage(`{"manual_id":"manual-redis-rca","parameters":{"target_instance":"redis-01"}}`))
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("permission = %#v, want allow", decision)
	}

	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
		Extra      map[string]json.RawMessage `json:"-"`
		Raw        map[string]json.RawMessage `json:"-"`
		Unknown    map[string]json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("input schema is invalid JSON: %v", err)
	}
	for _, name := range []string{"manual_id", "operation_frame", "parameters"} {
		if !containsString(schema.Required, name) {
			t.Fatalf("required = %#v, want %q", schema.Required, name)
		}
	}
	for _, name := range []string{"manual_id", "workflow_id", "operation_frame", "parameters", "triggered_by"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("input schema missing %q: %s", name, string(tool.InputSchema()))
		}
	}
}

func TestSearchOpsManualsToolExecutes(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	if err := repo.SaveManual(testRedisManual()); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("search_ops_manuals")
	if !ok {
		t.Fatal("search_ops_manuals tool not registered")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"text":"排查 Redis","limit":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Display == nil || result.Display.Type != "ops_manual_search_result" {
		t.Fatalf("display = %#v, want ops_manual_search_result", result.Display)
	}
	if len(result.Content) == 0 {
		t.Fatal("content should contain JSON result")
	}
	var payload struct {
		Decision        string   `json:"decision"`
		OpsManualFlowID string   `json:"ops_manual_flow_id"`
		NextQuestions   []string `json:"next_questions"`
		Instructions    []string `json:"instructions"`
		Manuals         []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Manual any    `json:"manual"`
		} `json:"manuals"`
		MissingFields []string `json:"missing_fields"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not model-facing JSON: %v", err)
	}
	if payload.Decision != string(core.DecisionNeedInfo) {
		t.Fatalf("decision = %q, want need_info", payload.Decision)
	}
	if payload.OpsManualFlowID == "" {
		t.Fatal("model content should include ops_manual_flow_id")
	}
	if len(payload.NextQuestions) > 2 {
		t.Fatalf("next questions = %#v, want at most two", payload.NextQuestions)
	}
	if containsString(payload.NextQuestions, "Coroot") || containsString(payload.NextQuestions, "监控指标") {
		t.Fatalf("next questions = %#v, should not ask the user whether Coroot or monitoring evidence exists", payload.NextQuestions)
	}
	if len(payload.MissingFields) != 0 {
		t.Fatalf("model content leaked missing fields: %#v", payload.MissingFields)
	}
	if len(payload.Manuals) != 1 || payload.Manuals[0].ID == "" || payload.Manuals[0].Title == "" {
		t.Fatalf("manuals = %#v, want compact top manual", payload.Manuals)
	}
	if payload.Manuals[0].Manual != nil {
		t.Fatalf("model content should not include full manual payload: %#v", payload.Manuals[0].Manual)
	}
	if !containsString(payload.Instructions, "Do not execute the workflow.") {
		t.Fatalf("instructions = %#v, want execution boundary", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Call resolve_ops_manual_params before asking the user any manual parameters.") {
		t.Fatalf("instructions = %#v, want param resolution first", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Your immediate next action must be a resolve_ops_manual_params tool call with the matched manual_id; do not run host commands, ask prose questions, or fall back to normal investigation before it returns.") {
		t.Fatalf("instructions = %#v, want mandatory param resolution next action", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Do not ask fixed target/location/execution/symptom fields yourself.") {
		t.Fatalf("instructions = %#v, want no fixed field asking", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Do not repeat card details such as manual id, workflow id, decision, score, or all missing fields.") {
		t.Fatalf("instructions = %#v, want no duplicated card details", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Do not ask the user whether Coroot evidence exists") {
		t.Fatalf("instructions = %#v, want Coroot auto-probe boundary", payload.Instructions)
	}
	var displayPayload core.SearchOpsManualsResult
	if err := json.Unmarshal(result.Display.Data, &displayPayload); err != nil {
		t.Fatalf("display data is not a SearchOpsManualsResult: %v", err)
	}
	if len(displayPayload.Manuals) == 0 || len(displayPayload.Manuals[0].MissingFields) == 0 {
		t.Fatalf("display data = %#v, want full UI/debug payload", displayPayload)
	}
	if displayPayload.OpsManualFlowID == "" || displayPayload.OpsManualFlowID != payload.OpsManualFlowID {
		t.Fatalf("flow id model=%q display=%q, want both non-empty and equal", payload.OpsManualFlowID, displayPayload.OpsManualFlowID)
	}
}

func TestSearchOpsManualsToolReturnsSuppressionMetadataCompactly(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	store := core.NewMemorySessionOpsContextStore()
	if err := repo.SaveManual(testPostgresBackupManual()); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo, core.WithSessionOpsContextStore(store))
	if err := store.UpsertFact(context.Background(), "sess-pg-suppressed", core.NewOpsManualSuppressionFact(core.OpsManualSuppression{
		ManualID:    "manual-pg-backup-ubuntu",
		ObjectType:  "postgresql",
		Action:      "backup",
		TargetScope: "host:pg-ubuntu-01",
		Reason:      "user_opt_out",
	}, time.Now().UTC())); err != nil {
		t.Fatalf("UpsertFact() error = %v", err)
	}
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("search_ops_manuals")
	if !ok {
		t.Fatal("search_ops_manuals tool not registered")
	}
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-pg-suppressed",
		TurnID:    "turn-pg-suppressed",
	})

	result, err := tool.Execute(ctx, json.RawMessage(`{"text":"在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常","metadata":{"target_name":"pg-ubuntu-01"},"limit":3}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Decision          string   `json:"decision"`
		SuppressedManuals []string `json:"suppressed_manuals"`
		SuppressionReason string   `json:"suppression_reason"`
		Manuals           []any    `json:"manuals"`
		Instructions      []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not model-facing JSON: %v", err)
	}
	if payload.Decision != string(core.DecisionNoMatch) {
		t.Fatalf("decision = %q, want no_match after suppression", payload.Decision)
	}
	if len(payload.Manuals) != 0 {
		t.Fatalf("model manuals = %#v, want suppressed manual omitted", payload.Manuals)
	}
	if !containsString(payload.SuppressedManuals, "manual-pg-backup-ubuntu") || payload.SuppressionReason != "user_opt_out" {
		t.Fatalf("suppression payload = %#v / %q, want compact user opt-out metadata", payload.SuppressedManuals, payload.SuppressionReason)
	}
	if !containsString(payload.Instructions, "Continue normal safe read-only evidence-driven investigation with available tools.") {
		t.Fatalf("instructions = %#v, want normal read-only continuation", payload.Instructions)
	}
	var displayPayload core.SearchOpsManualsResult
	if err := json.Unmarshal(result.Display.Data, &displayPayload); err != nil {
		t.Fatalf("display data is not a SearchOpsManualsResult: %v", err)
	}
	if len(displayPayload.Manuals) != 0 || !containsString(displayPayload.SuppressedManuals, "manual-pg-backup-ubuntu") {
		t.Fatalf("display payload = %#v, want suppressed metadata without card manual", displayPayload)
	}
}

func TestSearchOpsManualsReferenceOnlyInstructsReadOnlyAutomationOrBlocker(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionReference,
		Summary:               "找到可参考手册，但不能直接执行绑定工作流。",
		RecommendedNextAction: "没有可直接运行的 Workflow；继续只读自动化排查，若缺目标、时间范围、权限或观测数据会说明阻塞原因。",
		Manuals: []core.SearchManualHit{
			{
				Manual:         core.OpsManual{ID: "manual-k8s-pod-crashloop-rca", Title: "Kubernetes Pod CrashLoop/OOM 排障运维手册"},
				UsableMode:     core.DecisionReference,
				BlockedReasons: []string{"object_type differs"},
			},
		},
	})
	if payload.Decision != string(core.DecisionReference) {
		t.Fatalf("decision = %q, want reference_only", payload.Decision)
	}
	if payload.RecommendedNextAction == "" || !strings.Contains(payload.RecommendedNextAction, "继续只读自动化排查") {
		t.Fatalf("recommended_next_action = %q, want read-only continuation", payload.RecommendedNextAction)
	}
	for _, want := range []string{
		"Do not mention operations manual search or runnable Workflow status unless the user explicitly asked about manuals.",
		"Continue safe read-only investigation",
		"state the concrete blocker",
		"Kafka tooling",
		"metrics/log source",
	} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestOpsManualHighConfidenceRecommendationIncludesBoundaryAndStats(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionDirectExecute,
		Summary:               "找到高置信运维手册。",
		RecommendedNextAction: "confirm_use_ops_manual",
		Manuals: []core.SearchManualHit{{
			Manual: core.OpsManual{
				ID:      "manual-pg-backup-ubuntu",
				Title:   "PostgreSQL 备份 Ubuntu 运维手册",
				Version: "v1",
				WorkflowRef: core.WorkflowRef{
					WorkflowID: "workflow-pg-backup-ubuntu",
				},
				Operation: core.OperationProfile{
					TargetType: "postgresql",
					Action:     "backup",
					RiskLevel:  "medium",
				},
				Applicability: core.ApplicabilityProfile{
					Middleware:         "postgresql",
					MiddlewareVersions: []string{"15"},
					OS:                 []string{"ubuntu"},
					Platform:           []string{"vm"},
					ExecutionSurface:   []string{"ssh"},
				},
				Diagnosis: core.DiagnosisProfile{
					ApplicableSymptoms: []string{"pg_isready normal"},
					NotApplicableWhen:  []string{"database is still in archive recovery"},
				},
				CannotUseWhen: []string{"目标实例未知"},
				RiskPolicy: core.RiskPolicy{
					BlastRadius:          "single_host",
					DataMutation:         false,
					ApprovalRequiredWhen: []string{"production_backup_window"},
				},
			},
			BoundWorkflowID:   "workflow-pg-backup-ubuntu",
			MatchLevel:        "same_object_same_operation",
			UsableMode:        core.DecisionDirectExecute,
			MatchedFields:     []string{"object_type", "operation_type", "required_context", "environment"},
			RecommendedAction: "run_preflight_probe",
			RunRecordSummary:  core.RunRecordSummary{SuccessCount: 3, FailureCount: 1, RecentResult: "passed", LatestStatus: "passed"},
			PreflightStatus:   core.PreflightStatusNotRun,
			ScoreBreakdown:    core.ScoreBreakdown{FinalScore: 0.91, RunHistoryScore: 0.2},
			EnvironmentDiffs:  nil,
			BlockedReasons:    nil,
			HintSources:       []string{"memory_hint"},
		}},
	})

	if len(payload.Manuals) != 1 {
		t.Fatalf("manuals = %#v, want one high confidence manual", payload.Manuals)
	}
	manual := payload.Manuals[0]
	if manual.Confidence != "high" || manual.TraceOnly {
		t.Fatalf("manual confidence/trace_only = %q/%v, want high/false", manual.Confidence, manual.TraceOnly)
	}
	for _, want := range []string{"same_object_same_operation", "successful_run_history", "workflow_bound", "risk_explainable"} {
		if !containsString(manual.MatchReasons, want) {
			t.Fatalf("match reasons = %#v, want %q", manual.MatchReasons, want)
		}
	}
	for _, want := range []string{"target=postgresql", "operation=backup", "version=15", "os=ubuntu", "execution_surface=ssh", "risk=medium"} {
		if !containsString(manual.ApplicabilityBoundary, want) {
			t.Fatalf("applicability boundary = %#v, want %q", manual.ApplicabilityBoundary, want)
		}
	}
	if !containsString(manual.NotApplicableWhen, "database is still in archive recovery") || !containsString(manual.NotApplicableWhen, "目标实例未知") {
		t.Fatalf("not applicable = %#v, want diagnosis and cannot-use boundaries", manual.NotApplicableWhen)
	}
	if manual.HistoryStats.UsedCount != 4 || manual.HistoryStats.SuccessCount != 3 || manual.HistoryStats.SuccessRate != 75 {
		t.Fatalf("history stats = %#v, want used=4 success=3 success_rate=75", manual.HistoryStats)
	}
	if manual.Workflow.ID != "workflow-pg-backup-ubuntu" || manual.Workflow.RiskLevel != "medium" || manual.Workflow.RequiredTarget != "postgresql" {
		t.Fatalf("workflow summary = %#v, want id/risk/required target", manual.Workflow)
	}
	if !containsString(payload.Instructions, "user may skip") {
		t.Fatalf("instructions = %#v, want explicit skip option", payload.Instructions)
	}
}

func TestWorkflowSkipFallsBackToGeneralChat(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionDirectExecute,
		Summary:               "找到高置信手册和绑定 Workflow。",
		RecommendedNextAction: "confirm_use_ops_manual",
		Manuals: []core.SearchManualHit{{
			Manual: core.OpsManual{
				ID:    "manual-redis-repair",
				Title: "Redis 修复手册",
				WorkflowRef: core.WorkflowRef{
					WorkflowID: "workflow-redis-repair",
				},
				Operation: core.OperationProfile{
					TargetType: "redis",
					Action:     "repair",
					RiskLevel:  "medium",
				},
			},
			BoundWorkflowID:  "workflow-redis-repair",
			MatchLevel:       "same_object_same_operation",
			UsableMode:       core.DecisionDirectExecute,
			RunRecordSummary: core.RunRecordSummary{SuccessCount: 1},
		}},
	})

	if payload.RecommendedNextAction != "confirm_use_ops_manual" {
		t.Fatalf("recommended_next_action = %q, want confirm_use_ops_manual before user decision", payload.RecommendedNextAction)
	}
	if !containsString(payload.Instructions, "The user may skip; if the user declines, continue ordinary AI Chat handling without this manual/workflow.") {
		t.Fatalf("instructions = %#v, want skip to ordinary AI Chat boundary", payload.Instructions)
	}
	if !containsString(payload.Instructions, "Do not execute the workflow directly.") {
		t.Fatalf("instructions = %#v, want no direct workflow execution", payload.Instructions)
	}
}

func TestOpsManualLowConfidenceIsTraceOnly(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionReference,
		Summary:               "找到可参考手册，但不是高置信匹配。",
		RecommendedNextAction: "continue_general_chat",
		Manuals: []core.SearchManualHit{{
			Manual: core.OpsManual{
				ID:    "manual-generic-stateful-cluster-repair",
				Title: "通用有状态集群恢复运维手册",
				Operation: core.OperationProfile{
					TargetType: "stateful_cluster",
					Action:     "rca_or_repair",
					RiskLevel:  "high",
				},
			},
			MatchLevel:        "generic_stateful_cluster_repair",
			UsableMode:        core.DecisionReference,
			RecommendedAction: "reference_manual",
			BlockedReasons:    []string{"generic fallback is not a high-confidence manual match"},
		}},
	})

	if payload.RecommendedNextAction == "confirm_use_ops_manual" {
		t.Fatalf("recommended_next_action = %q, low confidence must not ask for manual confirmation", payload.RecommendedNextAction)
	}
	if len(payload.Manuals) != 1 {
		t.Fatalf("manuals = %#v, want trace-only manual", payload.Manuals)
	}
	manual := payload.Manuals[0]
	if !manual.TraceOnly || manual.Confidence != "low" {
		t.Fatalf("manual confidence/trace_only = %q/%v, want low/true", manual.Confidence, manual.TraceOnly)
	}
	if manual.RecommendedAction == "confirm_use_ops_manual" {
		t.Fatalf("manual = %#v, low confidence must not recommend confirm_use_ops_manual", manual)
	}
	if !containsString(payload.Instructions, "Continue safe read-only investigation") {
		t.Fatalf("instructions = %#v, want general chat continuation", payload.Instructions)
	}
}

func TestSearchOpsManualsNeedInfoWithoutManualDoesNotInstructParamResolution(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionNeedInfo,
		Summary:               "信息不足，不能直接使用工作流。",
		NextQuestions:         []string{"要处理的运维对象是什么？", "要执行的操作类型是什么？"},
		RecommendedNextAction: "补充缺失信息后重新检索。",
	})
	if payload.Decision != string(core.DecisionNeedInfo) {
		t.Fatalf("decision = %q, want need_info", payload.Decision)
	}
	if len(payload.Manuals) != 0 {
		t.Fatalf("manuals = %#v, want none", payload.Manuals)
	}
	for _, forbidden := range []string{
		"Call resolve_ops_manual_params before asking the user any manual parameters.",
		"Your immediate next action must be a resolve_ops_manual_params tool call",
	} {
		if containsString(payload.Instructions, forbidden) {
			t.Fatalf("instructions = %#v, should not contain %q without a matched manual", payload.Instructions, forbidden)
		}
	}
	for _, want := range []string{
		"No matched manual_id is available yet; do not call resolve_ops_manual_params.",
		"ask only the smallest missing question",
		"Do not mention operations manual search or no-match status unless the user explicitly asked about manuals.",
	} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestSearchOpsManualsNeedInfoWithManualStillInstructsParamResolution(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision: core.DecisionNeedInfo,
		Summary:  "信息不足，不能直接使用工作流。",
		Manuals: []core.SearchManualHit{{
			Manual:          core.OpsManual{ID: "manual-redis-rca", Title: "Redis 故障排查"},
			UsableMode:      core.DecisionNeedInfo,
			BoundWorkflowID: "workflow-redis-rca",
		}},
	})
	if len(payload.Manuals) != 1 || payload.Manuals[0].ID != "manual-redis-rca" {
		t.Fatalf("manuals = %#v, want matched manual", payload.Manuals)
	}
	if !containsString(payload.Instructions, "Your immediate next action must be a resolve_ops_manual_params tool call with the matched manual_id") {
		t.Fatalf("instructions = %#v, want param resolution when manual is matched", payload.Instructions)
	}
}

func TestSearchOpsManualsToolParsesTextWhenExecutionHostIsInjected(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	if err := repo.SaveManual(testRedisManual()); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("search_ops_manuals")
	if !ok {
		t.Fatal("search_ops_manuals tool not registered")
	}
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-search-host",
		TurnID:    "turn-search-host",
		HostID:    "server-local",
	})

	result, err := tool.Execute(ctx, json.RawMessage(`{"text":"troubleshoot Redis on current host server-local with ops manuals and read-only discovery"}`))
	if err != nil {
		t.Fatal(err)
	}
	var displayPayload core.SearchOpsManualsResult
	if err := json.Unmarshal(result.Display.Data, &displayPayload); err != nil {
		t.Fatalf("display data is not a SearchOpsManualsResult: %v", err)
	}
	if displayPayload.OperationFrame.Target.Type != "redis" || displayPayload.OperationFrame.Operation.Action != "rca_or_repair" {
		t.Fatalf("operation frame = %#v, want text-derived redis rca despite injected HostID", displayPayload.OperationFrame)
	}
	if displayPayload.OperationFrame.Metadata["selected_host"] != "server-local" {
		t.Fatalf("metadata = %#v, want selected_host from execution context", displayPayload.OperationFrame.Metadata)
	}
	if len(displayPayload.Manuals) == 0 || displayPayload.Manuals[0].Manual.ID != "manual-redis-rca" {
		t.Fatalf("manuals = %#v, want redis manual hit", displayPayload.Manuals)
	}
}

func TestSearchOpsManualsNoMatchInstructsReadOnlyAutomationOrBlocker(t *testing.T) {
	payload := searchOpsManualsModelResult(core.SearchOpsManualsResult{
		Decision:              core.DecisionNoMatch,
		Summary:               "没有找到合适的运维手册。",
		RecommendedNextAction: "不使用 Workflow；继续只读收集证据，若缺目标、时间范围、权限或观测数据会说明阻塞原因。",
	})
	if payload.Decision != string(core.DecisionNoMatch) {
		t.Fatalf("decision = %q, want no_match", payload.Decision)
	}
	if payload.RecommendedNextAction == "" || !strings.Contains(payload.RecommendedNextAction, "继续只读收集证据") {
		t.Fatalf("recommended_next_action = %q, want read-only continuation", payload.RecommendedNextAction)
	}
	for _, want := range []string{
		"Do not mention operations manual search or no-match status unless the user explicitly asked about manuals.",
		"Do not mention or expose cross-object manuals",
		"Continue normal safe read-only evidence-driven investigation",
		"state the concrete blocker",
		"Kafka tooling",
		"host/session availability",
		"compact Agent-to-UI form",
		"do not duplicate the form as a multiline prose template",
	} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestRunOpsManualPreflightToolExecutes(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	manual.PreflightProbe = core.PreflightProbe{ID: "redis-readonly-probe", ReadOnly: true, RequiredOutputs: []string{"ssh_access", "metrics_available"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("run_ops_manual_preflight")
	if !ok {
		t.Fatal("run_ops_manual_preflight tool not registered")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"manual_id":"manual-redis-rca","operation_frame":{"target":{"type":"redis","name":"redis-01"},"operation":{"target_type":"redis","action":"rca_or_repair"},"environment":{"execution_surface":"ssh"}},"parameters":{"target_instance":"redis-01"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Display == nil || result.Display.Type != "ops_manual_preflight_result" {
		t.Fatalf("display = %#v, want ops_manual_preflight_result", result.Display)
	}
	var payload core.PreflightResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not a PreflightResult: %v", err)
	}
	if payload.Status != core.PreflightStatusPassed || !payload.Ready {
		t.Fatalf("payload = %#v, want passed ready", payload)
	}
	blockedResult, err := tool.Execute(context.Background(), json.RawMessage(`{"manual_id":"manual-redis-rca","parameters":{"target_instance":"redis-01","simulate_permission_missing":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	var blockedPayload core.PreflightResult
	if err := json.Unmarshal([]byte(blockedResult.Content), &blockedPayload); err != nil {
		t.Fatalf("blocked content is not a PreflightResult: %v", err)
	}
	if blockedPayload.Status != core.PreflightStatusBlocked || len(blockedPayload.MissingPermissions) == 0 {
		t.Fatalf("blocked payload = %#v, want blocked with missing permissions", blockedPayload)
	}
}

func TestRunOpsManualPreflightToolGuidesStatusCheckToCompactFinal(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	manual.PreflightProbe = core.PreflightProbe{ID: "redis-readonly-probe", ReadOnly: true, RequiredOutputs: []string{"redis_ping", "metrics_available"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("run_ops_manual_preflight")
	if !ok {
		t.Fatal("run_ops_manual_preflight tool not registered")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"manual_id":"manual-redis-rca","operation_frame":{"intent":"status_check","target":{"type":"redis","name":"redis-01"},"operation":{"target_type":"redis","action":"status_check"},"environment":{"execution_surface":"ssh"}},"parameters":{"target_instance":"redis-01"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		core.PreflightResult
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not preflight model payload: %v", err)
	}
	if payload.Status != core.PreflightStatusPassed || !payload.Ready {
		t.Fatalf("payload = %#v, want passed ready", payload)
	}
	for _, want := range []string{"Stop tool use now", "1-3 bullets total", "no introductory sentence", "no change was executed"} {
		if !containsString(payload.Instructions, want) {
			t.Fatalf("instructions = %#v, want %q", payload.Instructions, want)
		}
	}
}

func TestRunOpsManualPreflightToolAcceptsRequiredParamsListInOperationFrame(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	manual := testRedisManual()
	manual.PreflightProbe = core.PreflightProbe{ID: "redis-readonly-probe", ReadOnly: true, RequiredOutputs: []string{"redis_ping"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("run_ops_manual_preflight")
	if !ok {
		t.Fatal("run_ops_manual_preflight tool not registered")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"manual_id":"manual-redis-rca",
		"operation_frame":{
			"target":{"type":"redis","name":"redis"},
			"operation":{"target_type":"redis","action":"rca_or_repair"},
			"required_params":["target_instance","symptom"]
		},
		"parameters":{"target_instance":"redis"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload core.PreflightResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != core.PreflightStatusPassed || !payload.Ready {
		t.Fatalf("payload = %#v, want passed ready", payload)
	}
}

func TestRunOpsManualPreflightReusesSameTurnSearchContext(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := core.NewMemoryStore()
	if err := repo.SaveManual(testPostgresBackupManual()); err != nil {
		t.Fatal(err)
	}
	service := core.NewService(repo)
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatal(err)
	}
	searchTool, ok := registry.Get("search_ops_manuals")
	if !ok {
		t.Fatal("search_ops_manuals tool not registered")
	}
	preflightTool, ok := registry.Get("run_ops_manual_preflight")
	if !ok {
		t.Fatal("run_ops_manual_preflight tool not registered")
	}

	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID: "sess-pg-backup",
		TurnID:    "turn-pg-backup",
	})
	search, err := searchTool.Execute(ctx, json.RawMessage(`{"text":"在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常","metadata":{"target_name":"pg-ubuntu-01"},"limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	var searchPayload core.SearchOpsManualsResult
	if err := json.Unmarshal(search.Display.Data, &searchPayload); err != nil {
		t.Fatalf("search display data is not a SearchOpsManualsResult: %v", err)
	}
	if searchPayload.OpsManualFlowID == "" {
		t.Fatal("search payload missing ops_manual_flow_id")
	}
	result, err := preflightTool.Execute(ctx, json.RawMessage(`{"manual_id":"manual-pg-backup-ubuntu","workflow_id":"workflow-pg-backup-ubuntu","requested_by":"user","triggered_by":"chat"}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload core.PreflightResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not a PreflightResult: %v", err)
	}
	if payload.Status != core.PreflightStatusPassed || !payload.Ready {
		t.Fatalf("payload = %#v, want passed ready with parameters reused from same-turn search", payload)
	}
	if payload.OpsManualFlowID != searchPayload.OpsManualFlowID {
		t.Fatalf("preflight flow id = %q, want search flow id %q", payload.OpsManualFlowID, searchPayload.OpsManualFlowID)
	}
}

func testRedisManual() core.OpsManual {
	return core.OpsManual{
		ID:      "manual-redis-rca",
		Title:   "Redis 故障排查",
		Status:  core.ManualStatusVerified,
		Version: "v1",
		WorkflowRef: core.WorkflowRef{
			WorkflowID: "workflow-redis-rca",
		},
		Operation: core.OperationProfile{
			TargetType: "redis",
			Action:     "rca_or_repair",
			RiskLevel:  "medium",
			Stateful:   true,
		},
		Applicability: core.ApplicabilityProfile{
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: core.RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"metrics"},
		},
		Validation:       []string{"确认 Redis 指标恢复"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Redis 故障排查手册",
	}
}

func testPostgresBackupManual() core.OpsManual {
	return core.OpsManual{
		ID:      "manual-pg-backup-ubuntu",
		Title:   "PostgreSQL 备份 Ubuntu 运维手册",
		Status:  core.ManualStatusVerified,
		Version: "v1",
		WorkflowRef: core.WorkflowRef{
			WorkflowID: "workflow-pg-backup-ubuntu",
		},
		Operation: core.OperationProfile{
			TargetType: "postgresql",
			Action:     "backup",
			RiskLevel:  "medium",
			Stateful:   true,
		},
		Applicability: core.ApplicabilityProfile{
			Middleware:       "postgresql",
			OS:               []string{"ubuntu"},
			Platform:         []string{"vm"},
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: core.RequiredContext{
			RequiredInputs:   []string{"target_instance", "backup_path"},
			RequiredEvidence: []string{"ssh_access", "pg_isready"},
		},
		RunnableConditions: core.RunnableConditions{
			RequiredParams: []string{"target_instance", "backup_path"},
		},
		PreflightProbe: core.PreflightProbe{
			ID:              "check_pg_backup_ssh_and_path",
			ReadOnly:        true,
			RequiredOutputs: []string{"ssh_access", "pg_isready", "backup_path_writable"},
		},
		SearchDoc:        "PostgreSQL backup Ubuntu ssh pg_isready backup_path",
		DocumentMarkdown: "用于在 Ubuntu 主机上通过 SSH 执行 PostgreSQL 备份。",
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func hasAlias(aliases []string, want string) bool {
	for _, alias := range aliases {
		if alias == want {
			return true
		}
	}
	return false
}
