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
