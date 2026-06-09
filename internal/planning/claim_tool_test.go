package planning

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestClaimNextTaskToolReturnsOnlyEligibleTaskAndCompactResult(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	store, err := NewTaskStore(taskStorePlanForTest())
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}
	tool := NewClaimNextTaskTool(store, func() time.Time { return now })
	input := json.RawMessage(`{"owner":"agent:planner","agentId":"agent-synthetic-1"}`)

	if tool.Metadata().Name != "claim_next_task" {
		t.Fatalf("tool name = %q, want claim_next_task", tool.Metadata().Name)
	}
	if err := tool.ValidateInput(context.Background(), input); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var payload ClaimNextTaskResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("unmarshal result content: %v\n%s", err, result.Content)
	}
	if !payload.Claimed || payload.TaskID != "step-2" || payload.LeaseID == "" {
		t.Fatalf("payload = %#v, want compact claim for step-2", payload)
	}
	if len(payload.DependsOnSatisfied) != 1 || payload.DependsOnSatisfied[0] != "step-1" {
		t.Fatalf("dependsOnSatisfied = %#v", payload.DependsOnSatisfied)
	}
	if payload.BlockedCount != 1 {
		t.Fatalf("blockedCount = %d, want one blocked step", payload.BlockedCount)
	}
}

func TestClaimNextTaskToolReportsNoEligibleTask(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	store, err := NewTaskStore(PlanState{
		Status: PlanStatusActive,
		Steps: []PlanStep{{
			ID:     "step-1",
			Text:   "已经完成所有计划步骤并记录验证结果",
			Status: StepStatusCompleted,
		}},
	})
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}
	tool := NewClaimNextTaskTool(store, func() time.Time { return now })
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"owner":"agent:planner"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var payload ClaimNextTaskResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.Claimed || payload.Reason != "no_eligible_task" {
		t.Fatalf("payload = %#v, want no eligible task", payload)
	}
}
