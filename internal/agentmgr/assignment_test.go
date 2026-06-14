package agentmgr

import "testing"

func TestAssignmentLintRequiresSelfContainedContext(t *testing.T) {
	result := ValidateAgentAssignment(AgentAssignment{
		KnownFacts: []string{"fact from parent context"},
	})
	if result.Status != AssignmentLintFail {
		t.Fatalf("status = %q, want %q", result.Status, AssignmentLintFail)
	}
	for _, field := range []string{"objective", "background", "scope", "expectedOutput", "stopCondition", "evidenceRequirement"} {
		if !containsString(result.MissingFields, field) {
			t.Fatalf("missing fields = %#v, want %q", result.MissingFields, field)
		}
	}

	pass := ValidateAgentAssignment(AgentAssignment{
		Objective:      "collect independent evidence",
		Background:     "manager observed a discrepancy",
		KnownFacts:     []string{"symptom started in the requested window"},
		Scope:          AgentScope{ResourceRefs: []string{"synthetic.resource/service-a"}, TimeRange: "last_30m"},
		ExpectedOutput: "bounded summary with evidence refs",
		EvidenceRequirement: EvidenceRequirement{
			MinEvidenceRefs: 1,
			RequiredKinds:   []string{"metric"},
		},
		StopCondition: "stop after required evidence is collected or blocker is found",
	})
	if pass.Status != AssignmentLintPass {
		t.Fatalf("status = %q, want %q: %#v", pass.Status, AssignmentLintPass, pass)
	}
}

func TestAgentAssignmentSummaryIsBounded(t *testing.T) {
	assignment := AgentAssignment{
		Objective:      "collect evidence",
		Background:     "long background that should not become the complete child transcript",
		KnownFacts:     []string{"fact one", "fact two"},
		Scope:          AgentScope{ResourceRefs: []string{"synthetic.resource/service-a"}},
		ExpectedOutput: "summary",
		EvidenceRequirement: EvidenceRequirement{
			MinEvidenceRefs: 2,
			RequiredKinds:   []string{"metric", "log"},
		},
		StopCondition: "done",
	}
	summary := assignment.Summary(90)
	if summary == "" {
		t.Fatal("expected summary")
	}
	if len(summary) > 90 {
		t.Fatalf("summary length = %d, want <= 90", len(summary))
	}
}
