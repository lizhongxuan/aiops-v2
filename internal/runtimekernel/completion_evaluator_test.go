package runtimekernel

import (
	"testing"

	"aiops-v2/internal/taskdepth"
)

func TestCompletionEvaluatorBlocksSuccessWhenCoverageMissing(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-completion")
	snapshot.TaskDepth = taskdepth.Profile{
		Level:              taskdepth.LevelOperations,
		RequiresPlan:       true,
		RequiresEvidence:   true,
		RequiresValidation: true,
	}
	snapshot.Metadata["coverage.coveredDimensions"] = "plan_context,tool_evidence,risk_review,open_questions_resolved"

	decision := EvaluateCompletionReadiness(snapshot, "Synthetic work completed successfully with checked evidence.")

	if decision.Action != "block_success_final" {
		t.Fatalf("Action = %q, want block_success_final", decision.Action)
	}
	if !runtimeKernelTestContains(decision.Reasons, "missing_coverage_dimension") {
		t.Fatalf("Reasons = %#v, want missing_coverage_dimension", decision.Reasons)
	}
	if !runtimeKernelTestContains(decision.MissingDimensions, "verification") {
		t.Fatalf("MissingDimensions = %#v, want verification", decision.MissingDimensions)
	}
}

func TestCompletionEvaluatorAllowsExplicitBlockerFinal(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-completion-blocker")
	snapshot.TaskDepth = taskdepth.Profile{
		Level:              taskdepth.LevelOperations,
		RequiresPlan:       true,
		RequiresEvidence:   true,
		RequiresValidation: true,
	}
	snapshot.Metadata["coverage.coveredDimensions"] = "plan_context,risk_review,open_questions_resolved"
	snapshot.Metadata["coverage.blocker"] = "synthetic_tool_unavailable"
	snapshot.Metadata["coverage.nextAction"] = "synthetic_request_input"

	decision := EvaluateCompletionReadiness(snapshot, "Blocked: synthetic evidence cannot be collected. Next action: request synthetic input.")

	if decision.Action != "allow_blocker_final" {
		t.Fatalf("Action = %q, want allow_blocker_final", decision.Action)
	}
	if !runtimeKernelTestContains(decision.Reasons, "explicit_blocker_final") {
		t.Fatalf("Reasons = %#v, want explicit_blocker_final", decision.Reasons)
	}
}
