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
			{ID: "one", Text: "汇总 evidence refs 并给出目标服务异常的根因判断", Status: StepStatusCompleted},
		},
	}

	if err := args.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestUpdatePlanToolValidatesAndReturnsCompactSummary(t *testing.T) {
	tool := NewUpdatePlanTool()
	input := json.RawMessage(`{"steps":[{"id":"one","text":"查询观测工具中的目标服务指标并判断关键指标是否异常","status":"in_progress"},{"id":"two","text":"汇总 evidence refs 并说明影响范围","status":"pending"}]}`)

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

func TestUpdatePlanToolAcceptsArtifactInput(t *testing.T) {
	tool := NewUpdatePlanTool()
	input := json.RawMessage(`{
		"artifact": {
			"id": "plan-synthetic-1",
			"version": 1,
			"status": "draft",
			"context": {"summary": "需要把复杂请求拆成可验证计划"},
			"recommendedApproach": [{"id": "approach-1", "summary": "先读取状态，再更新计划"}],
			"scope": {"in": ["计划 artifact"], "out": ["未批准写操作"]},
			"reuse": {"existingPatterns": ["复用 update_plan"]},
			"verification": {"checks": ["运行 focused tests"]},
			"openQuestions": [{"id": "question-1", "text": "是否允许执行写操作？"}],
			"steps": [
				{
					"id": "step-1",
					"text": "读取现有计划状态并整理结构化字段",
					"status": "completed",
					"owner": "agent:planner",
					"agentId": "agent-synthetic-1",
					"evidenceRefs": ["trace-synthetic-1"],
					"verificationStatus": "passed"
				},
				{
					"id": "step-2",
					"text": "根据结构化字段生成计划批准范围",
					"status": "pending",
					"dependsOn": ["step-1"],
					"requiredApprovals": ["approval-synthetic-1"]
				}
			]
		}
	}`)

	if err := tool.ValidateInput(context.Background(), input); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"plan updated: active", "artifact=plan-synthetic-1", "open_questions=1"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("result content missing %q: %s", want, result.Content)
		}
	}
}

func TestUpdatePlanToolVisibleInChatPlanAndExecute(t *testing.T) {
	tool := NewUpdatePlanTool()
	for _, mode := range []string{"chat", "plan", "execute"} {
		if !tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: mode}) {
			t.Fatalf("update_plan should be enabled for host/%s", mode)
		}
		if !tool.IsEnabled(tooling.ToolContext{SessionType: "workspace", Mode: mode}) {
			t.Fatalf("update_plan should be enabled for workspace/%s", mode)
		}
	}
	if tool.IsEnabled(tooling.ToolContext{SessionType: "host", Mode: "inspect"}) {
		t.Fatal("update_plan should stay hidden in inspect mode")
	}
}

func TestUpdatePlanToolPromptDescribesComplexTaskProtocol(t *testing.T) {
	tool := NewUpdatePlanTool()
	prompt := tool.Prompt(tooling.PromptContext{SessionType: "host", Mode: "chat"})
	for _, want := range []string{
		"Use this tool proactively for complex tasks",
		"RCA",
		"Keep exactly one step in_progress",
		"Never mark completed when evidence",
		"Skip for trivial",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
