package agentmgr

import "testing"

func TestDelegationDecisionSpawnContinueInline(t *testing.T) {
	spawn := EvaluateDelegationDecision(DelegationEvaluationInput{
		Objective:            "compare independent evidence",
		Scope:                "resource group",
		EvidenceRequirement:  "metrics and logs",
		ReadOnly:             true,
		EvidenceSurfaces:     []EvidenceSurface{{CapabilityKind: "metrics", ResourceType: "service"}, {CapabilityKind: "logs", ResourceType: "service"}},
		AvailableAgents:      []CandidateAgent{{Name: "synthetic.metrics_reader", CapabilityKinds: []string{"metrics"}, ResourceTypes: []string{"service"}}},
		RequiredCapability:   "metrics",
		RequiredResourceType: "service",
	})
	if spawn.Action != DelegationSpawnNew {
		t.Fatalf("spawn action = %q, want %q (%s)", spawn.Action, DelegationSpawnNew, spawn.Reason)
	}
	if spawn.CandidateAgent != "synthetic.metrics_reader" {
		t.Fatalf("candidate = %q, want synthetic.metrics_reader", spawn.CandidateAgent)
	}

	cont := EvaluateDelegationDecision(DelegationEvaluationInput{
		Objective:            "follow up on the same evidence gap",
		Scope:                "same resource group",
		EvidenceRequirement:  "additional logs",
		ReadOnly:             true,
		RequiredCapability:   "logs",
		RequiredResourceType: "service",
		ExistingAgents: []ExistingAgentContext{{
			AgentID:         "worker-1",
			Status:          AgentStatusFailed,
			CapabilityKinds: []string{"logs"},
			ResourceTypes:   []string{"service"},
		}},
	})
	if cont.Action != DelegationContinueExisting {
		t.Fatalf("continue action = %q, want %q (%s)", cont.Action, DelegationContinueExisting, cont.Reason)
	}
	if cont.ExistingAgentID != "worker-1" {
		t.Fatalf("existing id = %q, want worker-1", cont.ExistingAgentID)
	}

	inline := EvaluateDelegationDecision(DelegationEvaluationInput{
		Objective: "summarize one known fact",
		Scope:     "current turn",
		Simple:    true,
		ReadOnly:  true,
	})
	if inline.Action != DelegationHandleInline {
		t.Fatalf("inline action = %q, want %q (%s)", inline.Action, DelegationHandleInline, inline.Reason)
	}
}

func TestDelegationDecisionMissingFieldsAsksClarification(t *testing.T) {
	decision := EvaluateDelegationDecision(DelegationEvaluationInput{
		Objective:        "investigate",
		EvidenceSurfaces: []EvidenceSurface{{CapabilityKind: "metrics", ResourceType: "service"}},
		ReadOnly:         true,
	})
	if decision.Action != DelegationAskClarification {
		t.Fatalf("action = %q, want %q", decision.Action, DelegationAskClarification)
	}
	if len(decision.RequiredFields) == 0 {
		t.Fatal("expected missing required fields")
	}
}
