package runtimekernel

import (
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
)

func TestInstructionReconcileCancelsPendingStepsOnGoalChange(t *testing.T) {
	previous := syntheticRuntimeKernelSnapshot("synthetic-turn-reconcile")
	previous.AgentItems = []agentstate.TurnItem{
		syntheticPlanItem(t, previous.ID, planning.PlanState{
			Status: planning.PlanStatusActive,
			Steps: []planning.PlanStep{
				{ID: "synthetic-step-running", Text: "synthetic running step", Status: planning.StepStatusInProgress},
				{ID: "synthetic-step-pending", Text: "synthetic pending step", Status: planning.StepStatusPending},
				{ID: "synthetic-step-done", Text: "synthetic completed step", Status: planning.StepStatusCompleted},
			},
		}),
	}

	decision := EvaluateInstructionReconcile(previous, "synthetic replacement goal", map[string]string{
		"instruction.revision":   "replace_goal",
		"instruction.newStepIds": "synthetic-step-new",
	})

	if decision.Action != "supersede_plan" {
		t.Fatalf("Action = %q, want supersede_plan", decision.Action)
	}
	if !runtimeKernelTestContains(decision.CancelledStepIDs, "synthetic-step-pending") {
		t.Fatalf("CancelledStepIDs = %#v, want synthetic-step-pending", decision.CancelledStepIDs)
	}
	if !runtimeKernelTestContains(decision.BlockedStepIDs, "synthetic-step-running") {
		t.Fatalf("BlockedStepIDs = %#v, want synthetic-step-running", decision.BlockedStepIDs)
	}
	if runtimeKernelTestContains(decision.CancelledStepIDs, "synthetic-step-done") || runtimeKernelTestContains(decision.BlockedStepIDs, "synthetic-step-done") {
		t.Fatalf("completed step should not be cancelled or blocked: %#v", decision)
	}
	if !runtimeKernelTestContains(decision.NewStepIDs, "synthetic-step-new") {
		t.Fatalf("NewStepIDs = %#v, want synthetic-step-new", decision.NewStepIDs)
	}
}

func TestInstructionReconcileDoesNotCancelWhenUserAddsConstraint(t *testing.T) {
	previous := syntheticRuntimeKernelSnapshot("synthetic-turn-reconcile-constraint")
	previous.AgentItems = []agentstate.TurnItem{
		syntheticPlanItem(t, previous.ID, planning.PlanState{
			Status: planning.PlanStatusActive,
			Steps: []planning.PlanStep{
				{ID: "synthetic-step-running", Text: "synthetic running step", Status: planning.StepStatusInProgress},
				{ID: "synthetic-step-pending", Text: "synthetic pending step", Status: planning.StepStatusPending},
			},
		}),
	}

	decision := EvaluateInstructionReconcile(previous, "synthetic additional constraint", map[string]string{
		"instruction.revision": "add_constraint",
	})

	if decision.Action != "revise_plan" {
		t.Fatalf("Action = %q, want revise_plan", decision.Action)
	}
	if len(decision.CancelledStepIDs) != 0 || len(decision.BlockedStepIDs) != 0 {
		t.Fatalf("constraint update should not cancel or block existing steps: %#v", decision)
	}
	if !runtimeKernelTestContains(decision.Reasons, "constraint_added") {
		t.Fatalf("Reasons = %#v, want constraint_added", decision.Reasons)
	}
}
