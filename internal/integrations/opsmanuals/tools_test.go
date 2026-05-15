package opsmanuals

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
	for _, want := range []string{"high-risk", "database backup", "decision", "direct_execute", "need_info", "adapt", "reference_only", "no_match"} {
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
	var payload core.SearchOpsManualsResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not a SearchOpsManualsResult: %v", err)
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

func hasAlias(aliases []string, want string) bool {
	for _, alias := range aliases {
		if alias == want {
			return true
		}
	}
	return false
}
