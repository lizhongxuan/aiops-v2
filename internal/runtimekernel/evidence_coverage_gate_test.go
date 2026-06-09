package runtimekernel

import (
	"testing"

	"aiops-v2/internal/taskdepth"
)

func TestEvidenceCoverageGateBlocksSynthesisWhenRequiredDimensionsMissing(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-coverage")
	snapshot.TaskDepth = taskdepth.Profile{
		Level:              taskdepth.LevelOperations,
		RequiresPlan:       true,
		RequiresEvidence:   true,
		RequiresValidation: true,
	}
	snapshot.Metadata["coverage.coveredDimensions"] = "plan_context,risk_review,open_questions_resolved"

	decision := EvaluateEvidenceCoverageGate(snapshot)

	if decision.Action != "continue_gathering" {
		t.Fatalf("Action = %q, want continue_gathering", decision.Action)
	}
	for _, want := range []string{"tool_evidence", "verification"} {
		if !runtimeKernelTestContains(decision.MissingDimensions, want) {
			t.Fatalf("MissingDimensions = %#v, want %s", decision.MissingDimensions, want)
		}
	}
	if decision.Coverage >= 1 {
		t.Fatalf("Coverage = %v, want partial coverage", decision.Coverage)
	}
}

func TestEvidenceCoverageGateAllowsBlockerSynthesisWhenToolUnavailable(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-coverage-blocker")
	snapshot.TaskDepth = taskdepth.Profile{
		Level:              taskdepth.LevelOperations,
		RequiresPlan:       true,
		RequiresEvidence:   true,
		RequiresValidation: true,
	}
	snapshot.Metadata["coverage.coveredDimensions"] = "plan_context,risk_review,open_questions_resolved"
	snapshot.Metadata["coverage.blocker"] = "synthetic_tool_unavailable"
	snapshot.Metadata["coverage.nextAction"] = "synthetic_request_input"

	decision := EvaluateEvidenceCoverageGate(snapshot)

	if decision.Action != "blocker_final_allowed" {
		t.Fatalf("Action = %q, want blocker_final_allowed", decision.Action)
	}
	if !runtimeKernelTestContains(decision.Reasons, "blocker_with_next_action") {
		t.Fatalf("Reasons = %#v, want blocker_with_next_action", decision.Reasons)
	}
}
