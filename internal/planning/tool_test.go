package planning

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestValidateUpdatePlanArgsAcceptsCompletedPlanWithoutInProgress(t *testing.T) {
	args := UpdatePlanArgs{
		Status: PlanStatusCompleted,
		Steps: []PlanStep{
			{ID: "one", Text: "汇总 evidence refs 并给出 Redis 内存增长的根因判断", Status: StepStatusCompleted},
		},
	}

	if err := args.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestUpdatePlanToolValidatesAndReturnsCompactSummary(t *testing.T) {
	tool := NewUpdatePlanTool()
	input := json.RawMessage(`{"steps":[{"id":"one","text":"查询 Coroot 中 Redis 内存指标并判断 RSS 增长是否异常","status":"in_progress"},{"id":"two","text":"汇总 evidence refs 并说明影响范围","status":"pending"}]}`)

	if tool.Metadata().Name != "update_plan" {
		t.Fatalf("tool name = %q, want update_plan", tool.Metadata().Name)
	}
	if !tool.IsReadOnly(input) {
		t.Fatal("update_plan should be classified read-only/runtime-state")
	}
	if tool.IsDestructive(input) {
		t.Fatal("update_plan must not be destructive")
	}
	if !tool.IsConcurrencySafe(input) {
		t.Fatal("update_plan should be concurrency-safe")
	}
	if got := tool.CheckPermissions(context.Background(), input); got.Action != tooling.PermissionActionAllow {
		t.Fatalf("CheckPermissions() = %#v, want allow", got)
	}
	if err := tool.ValidateInput(context.Background(), input); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, "plan updated: active") || !strings.Contains(result.Content, "1/2 in_progress") {
		t.Fatalf("result content = %q, want compact plan summary", result.Content)
	}
}

func TestUpdatePlanToolRejectsInvalidPlan(t *testing.T) {
	tool := NewUpdatePlanTool()
	input := json.RawMessage(`{"steps":[{"text":"one","status":"in_progress"},{"text":"two","status":"in_progress"}]}`)

	if err := tool.ValidateInput(context.Background(), input); err == nil {
		t.Fatal("ValidateInput() error = nil, want invalid plan error")
	}
	if _, err := tool.Execute(context.Background(), input); err == nil {
		t.Fatal("Execute() error = nil, want invalid plan error")
	}
}
