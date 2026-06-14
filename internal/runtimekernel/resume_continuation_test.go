package runtimekernel

import "testing"

func TestResumeContinuationDefaultsToNextStep(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-resume")
	snapshot.Lifecycle = TurnLifecycleResumable
	snapshot.ResumeState = TurnResumeStateCheckpointReady
	snapshot.Metadata["resume.nextStepId"] = "synthetic-step-2"

	policy := EvaluateResumeContinuationPolicy(snapshot, "continue synthetic work")

	if policy.Action != "continue_next_step" {
		t.Fatalf("Action = %q, want continue_next_step", policy.Action)
	}
	if policy.NextStepID != "synthetic-step-2" {
		t.Fatalf("NextStepID = %q, want synthetic-step-2", policy.NextStepID)
	}
	if policy.RecapAllowed {
		t.Fatalf("RecapAllowed = true, want false for default resume")
	}
}

func TestResumeAllowsRecapWhenUserExplicitlyAsksSummary(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-resume-summary")
	snapshot.Lifecycle = TurnLifecycleResumable
	snapshot.ResumeState = TurnResumeStateCheckpointReady
	snapshot.Metadata["resume.nextStepId"] = "synthetic-step-2"

	policy := EvaluateResumeContinuationPolicy(snapshot, "please summarize the synthetic work before continuing")

	if policy.Action != "recap_requested" {
		t.Fatalf("Action = %q, want recap_requested", policy.Action)
	}
	if !policy.RecapAllowed {
		t.Fatalf("RecapAllowed = false, want true for explicit summary request")
	}
}
