package runtimekernel

import "testing"

func TestStepRevisionBuildsTypedMultiCauseTransition(t *testing.T) {
	previousStep := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	previousFacts := mustFreezeStepRevisionFactsForTest(t, StepRevisionFacts{
		TurnAssemblyHash:      previousStep.TurnAssemblyHash,
		IntentHash:            "intent-1",
		SessionTargetHash:     "target-1",
		ResourceBindingsHash:  "resources-1",
		RoleBindingsHash:      "roles-1",
		PermissionProfileHash: "permission-profile-1",
		LoadedSkillsHash:      "skills-1",
		LoadedSkillRefs:       []string{"skill:base"},
		MCPHealthHash:         "mcp-health-1",
		MCPServerRefs:         []string{"mcp:synthetic"},
	})
	previousRef, err := BuildStepReference(nil, previousStep, previousFacts)
	if err != nil {
		t.Fatalf("BuildStepReference(previous) error = %v", err)
	}

	currentInput := validRuntimeStepContextForHashTest()
	currentInput.Iteration++
	currentInput.CheckpointRef = "checkpoint-2"
	currentStep := mustFreezeRuntimeStepContextForTest(t, currentInput)
	currentFacts := previousFacts
	currentFacts.Hash = ""
	currentFacts.LoadedSkillsHash = "skills-2"
	currentFacts.LoadedSkillRefs = []string{"skill:base", "skill:diagnose"}
	currentFacts.MCPHealthHash = "mcp-health-2"
	currentFacts = mustFreezeStepRevisionFactsForTest(t, currentFacts)

	currentRef, err := BuildStepReference(&previousRef, currentStep, currentFacts)
	if err != nil {
		t.Fatalf("BuildStepReference(current) error = %v", err)
	}
	if currentRef.StepHash != currentStep.Hash || currentRef.Transition.PreviousHash != previousStep.Hash || currentRef.Transition.NextHash != currentStep.Hash {
		t.Fatalf("step reference chain = %#v", currentRef)
	}
	for _, kind := range []string{StepRevisionKindSkillLoaded, StepRevisionKindMCPHealthChanged} {
		if !stepTransitionHasKind(currentRef.Transition, kind) {
			t.Fatalf("revisions = %#v, missing %q", currentRef.Transition.Revisions, kind)
		}
	}
	if currentRef.Transition.Hash == "" || currentRef.Hash == "" {
		t.Fatalf("reference hashes missing: %#v", currentRef)
	}
}

func TestStepRevisionRejectsImmutableTurnDrift(t *testing.T) {
	previousStep := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	facts := mustFreezeStepRevisionFactsForTest(t, StepRevisionFacts{
		TurnAssemblyHash: previousStep.TurnAssemblyHash, IntentHash: "intent-1", SessionTargetHash: "target-1",
		ResourceBindingsHash: "resources-1", RoleBindingsHash: "roles-1", PermissionProfileHash: "permission-profile-1",
	})
	previousRef, err := BuildStepReference(nil, previousStep, facts)
	if err != nil {
		t.Fatalf("BuildStepReference(previous) error = %v", err)
	}
	currentInput := validRuntimeStepContextForHashTest()
	currentInput.Iteration++
	currentStep := mustFreezeRuntimeStepContextForTest(t, currentInput)
	drifted := facts
	drifted.Hash = ""
	drifted.IntentHash = "intent-drifted"
	drifted = mustFreezeStepRevisionFactsForTest(t, drifted)
	if _, err := BuildStepReference(&previousRef, currentStep, drifted); err == nil {
		t.Fatal("BuildStepReference accepted immutable intent drift")
	}
}

func TestStepRevisionLegacySnapshotKeepsResumeCause(t *testing.T) {
	input := validRuntimeStepContextForHashTest()
	input.Iteration = 3
	step := mustFreezeRuntimeStepContextForTest(t, input)
	facts := mustFreezeStepRevisionFactsForTest(t, StepRevisionFacts{
		TurnAssemblyHash: step.TurnAssemblyHash,
		Cause: StepRevisionCause{
			Kind: StepRevisionKindModelRetryResumed, CheckpointID: "checkpoint-timeout",
		},
	})
	reference, err := BuildStepReference(nil, step, facts)
	if err != nil {
		t.Fatalf("BuildStepReference() error = %v", err)
	}
	for _, kind := range []string{StepRevisionKindLegacyPreviousUnknown, StepRevisionKindModelRetryResumed} {
		if !stepTransitionHasKind(reference.Transition, kind) {
			t.Fatalf("legacy revisions = %#v, missing %q", reference.Transition.Revisions, kind)
		}
	}
}

func mustFreezeStepRevisionFactsForTest(t *testing.T, facts StepRevisionFacts) StepRevisionFacts {
	t.Helper()
	frozen, err := FreezeStepRevisionFacts(facts)
	if err != nil {
		t.Fatalf("FreezeStepRevisionFacts() error = %v", err)
	}
	return frozen
}
