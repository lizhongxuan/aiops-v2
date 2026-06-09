package runtimekernel

import (
	"strings"
	"testing"
)

func TestCompactSummaryValidationRejectsMissingSourceTurnID(t *testing.T) {
	summary := validCompactSummaryV1()
	summary.NextStep.SourceTurnID = ""

	err := summary.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want missing sourceTurnId error")
	}
	if !strings.Contains(err.Error(), "nextStep.sourceTurnId") {
		t.Fatalf("Validate() error = %v, want nextStep.sourceTurnId", err)
	}
}

func TestCompactSummaryValidationRequiresSourceRefForConfirmedFacts(t *testing.T) {
	summary := validCompactSummaryV1()
	summary.ConfirmedFacts = []CompactSummaryFactV1{{
		Statement: "A previously observed condition was confirmed.",
	}}

	err := summary.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want confirmedFacts sourceRef error")
	}
	if !strings.Contains(err.Error(), "confirmedFacts[0].sourceRef") {
		t.Fatalf("Validate() error = %v, want confirmedFacts sourceRef", err)
	}
}

func TestCompactSummaryValidationAcceptsCompleteNoDriftSummary(t *testing.T) {
	summary := validCompactSummaryV1()

	if err := summary.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func validCompactSummaryV1() CompactSummaryV1 {
	return CompactSummaryV1{
		SchemaVersion:      CompactSummarySchemaVersionV1,
		UserGoal:           "Continue the current user task without drifting to older context.",
		LatestUserMessages: []CompactSummaryMessageRefV1{{TurnID: "turn-latest", Quote: "Keep the latest requested action anchored."}},
		ActiveConstraints:  []string{"Do not invent unavailable facts."},
		CurrentTask: CompactSummaryCurrentTaskV1{
			Description:  "Preserve compacted context for the active task.",
			SourceTurnID: "turn-latest",
		},
		ConfirmedFacts: []CompactSummaryFactV1{{
			Statement: "One relevant fact was confirmed.",
			SourceRef: "ref-1",
		}},
		OpenQuestions:    []string{"Which follow-up evidence is still needed?"},
		Decisions:        []CompactSummaryDecisionV1{{Decision: "Keep recent user quote.", SourceRef: "turn-latest"}},
		Artifacts:        []CompactSummaryArtifactV1{{ID: "artifact-1", SourceRef: "ref-1", Summary: "Bounded artifact summary."}},
		PendingApprovals: []CompactSummaryPendingItemV1{{ID: "approval-1", SourceRef: "turn-latest"}},
		PendingEvidence:  []CompactSummaryPendingItemV1{{ID: "evidence-1", SourceRef: "ref-1"}},
		PlanState:        CompactSummaryPlanStateV1{Status: "in_progress", CurrentStep: "Validate compact summary."},
		NextStep: CompactSummaryNextStepV1{
			Action:          "Continue from the latest user request.",
			SourceTurnID:    "turn-latest",
			RecentUserQuote: "Keep the latest requested action anchored.",
		},
	}
}
