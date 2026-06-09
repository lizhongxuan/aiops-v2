package planning

import "testing"

func TestApplyPlanUpdateRejectsEmptyPlan(t *testing.T) {
	_, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{})
	if err == nil {
		t.Fatal("expected empty plan update to fail")
	}
}

func TestApplyPlanUpdateRejectsMultipleInProgressSteps(t *testing.T) {
	_, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
		Steps: []PlanStep{
			{ID: "one", Text: "first", Status: StepStatusInProgress},
			{ID: "two", Text: "second", Status: StepStatusInProgress},
		},
	})
	if err == nil {
		t.Fatal("expected multiple in_progress steps to fail")
	}
}

func TestApplyPlanUpdateRejectsInProgressInFinalPlan(t *testing.T) {
	_, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
		Status: PlanStatusCompleted,
		Steps: []PlanStep{
			{ID: "one", Text: "first", Status: StepStatusInProgress},
		},
	})
	if err == nil {
		t.Fatal("expected final plan with in_progress step to fail")
	}
}

func TestApplyPlanUpdateTrimsAndDefaultsStatus(t *testing.T) {
	next, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
		Steps: []PlanStep{
			{ID: " one ", Text: " first step "},
			{ID: " two ", Text: " second step ", Status: StepStatusCompleted, Summary: " done "},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPlanUpdate failed: %v", err)
	}
	if next.Status != PlanStatusActive {
		t.Fatalf("plan status = %q, want active", next.Status)
	}
	if next.Steps[0].ID != "one" || next.Steps[0].Text != "first step" {
		t.Fatalf("first step was not trimmed: %#v", next.Steps[0])
	}
	if next.Steps[0].Status != StepStatusPending {
		t.Fatalf("default step status = %q, want pending", next.Steps[0].Status)
	}
	if next.Steps[1].Summary != "done" {
		t.Fatalf("summary was not trimmed: %#v", next.Steps[1])
	}
}

func TestApplyPlanUpdateRejectsDuplicateStepIDs(t *testing.T) {
	_, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
		Steps: []PlanStep{
			{ID: "same", Text: "first", Status: StepStatusPending},
			{ID: "same", Text: "second", Status: StepStatusPending},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate step ids to fail")
	}
}

func TestApplyPlanUpdateRejectsVagueSteps(t *testing.T) {
	for _, text := range []string{"分析问题", "检查服务", "处理故障", "查看情况"} {
		_, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
			Steps: []PlanStep{{ID: "vague", Text: text, Status: StepStatusInProgress}},
		})
		if err == nil {
			t.Fatalf("expected vague step %q to fail", text)
		}
	}
}

func TestApplyPlanUpdateAcceptsVerifiableOpsStep(t *testing.T) {
	next, err := ApplyPlanUpdate(PlanState{}, UpdatePlanArgs{
		Steps: []PlanStep{{
			ID:     "target-metrics",
			Text:   "查询观测工具中的目标服务关键指标并判断资源增长是否脱离请求量增长",
			Status: StepStatusInProgress,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyPlanUpdate() error = %v", err)
	}
	if next.Steps[0].ID != "target-metrics" {
		t.Fatalf("step was not kept: %#v", next.Steps[0])
	}
}
