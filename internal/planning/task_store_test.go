package planning

import (
	"testing"
	"time"
)

func taskStorePlanForTest() PlanState {
	return PlanState{
		Status: PlanStatusActive,
		Steps: []PlanStep{
			{
				ID:     "step-1",
				Text:   "读取现有状态并记录基础证据",
				Status: StepStatusCompleted,
			},
			{
				ID:        "step-2",
				Text:      "根据基础证据生成计划批准范围",
				Status:    StepStatusPending,
				DependsOn: []string{"step-1"},
			},
			{
				ID:        "step-3",
				Text:      "在批准后执行验证并记录输出",
				Status:    StepStatusPending,
				DependsOn: []string{"step-2"},
			},
			{
				ID:        "step-4",
				Text:      "等待用户提供缺失的执行范围",
				Status:    StepStatusBlocked,
				BlockedBy: []string{"question-synthetic-1"},
			},
		},
	}
}

func TestTaskStoreClaimNextSkipsBlockedAndDependencyIncompleteTasks(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	store, err := NewTaskStore(taskStorePlanForTest())
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}

	claim, ok := store.ClaimNext("agent:planner", "agent-synthetic-1", now)
	if !ok {
		t.Fatal("ClaimNext() ok = false, want claim")
	}
	if claim.TaskID != "step-2" {
		t.Fatalf("claimed task = %q, want step-2", claim.TaskID)
	}
	if claim.Owner != "agent:planner" || claim.AgentID != "agent-synthetic-1" {
		t.Fatalf("claim owner fields = %#v", claim)
	}

	if _, ok := store.ClaimNext("agent:planner", "agent-synthetic-1", now.Add(time.Minute)); ok {
		t.Fatal("same owner should not claim a second active task")
	}
	if _, ok := store.ClaimNext("agent:other", "agent-synthetic-2", now.Add(time.Minute)); ok {
		t.Fatal("step-3 dependency should not be claimable while step-2 in_progress")
	}

	if err := store.Complete(claim, []string{"trace-synthetic-1"}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	next, ok := store.ClaimNext("agent:other", "agent-synthetic-2", now.Add(3*time.Minute))
	if !ok {
		t.Fatal("ClaimNext() after completion ok = false, want step-3")
	}
	if next.TaskID != "step-3" {
		t.Fatalf("next claimed task = %q, want step-3", next.TaskID)
	}
}

func TestTaskStoreReclaimsExpiredLease(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	store, err := NewTaskStore(taskStorePlanForTest())
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}

	first, ok := store.ClaimNext("agent:first", "agent-synthetic-1", now)
	if !ok {
		t.Fatal("first ClaimNext() ok = false")
	}
	second, ok := store.ClaimNext("agent:second", "agent-synthetic-2", now.Add(DefaultTaskLeaseTTL+time.Second))
	if !ok {
		t.Fatal("second ClaimNext() after lease expiry ok = false")
	}
	if second.TaskID != first.TaskID {
		t.Fatalf("second task = %q, want reclaimed %q", second.TaskID, first.TaskID)
	}
	if second.LeaseID == first.LeaseID {
		t.Fatalf("lease id was not renewed: %q", second.LeaseID)
	}
}

func TestTaskStoreBlockAndReleaseClaim(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	store, err := NewTaskStore(taskStorePlanForTest())
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}
	claim, ok := store.ClaimNext("agent:planner", "agent-synthetic-1", now)
	if !ok {
		t.Fatal("ClaimNext() ok = false")
	}
	if err := store.Block(claim, []string{"approval-synthetic-1"}, "等待计划批准", now.Add(time.Minute)); err != nil {
		t.Fatalf("Block() error = %v", err)
	}
	state := store.State()
	if state.Steps[1].Status != StepStatusBlocked || len(state.Steps[1].BlockedBy) != 1 {
		t.Fatalf("blocked step = %#v", state.Steps[1])
	}

	releasedPlan := taskStorePlanForTest()
	releaseStore, err := NewTaskStore(releasedPlan)
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}
	releaseClaim, ok := releaseStore.ClaimNext("agent:planner", "agent-synthetic-1", now)
	if !ok {
		t.Fatal("release claim ok = false")
	}
	if err := releaseStore.Release(releaseClaim, now.Add(time.Minute)); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if got := releaseStore.State().Steps[1].Status; got != StepStatusPending {
		t.Fatalf("released status = %q, want pending", got)
	}
}
