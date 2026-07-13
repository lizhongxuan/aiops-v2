package agentmgr

import "testing"

func TestAgentFinalGateBlocksPendingWorkerClaim(t *testing.T) {
	decision := EvaluateAgentFinalGate(FinalGateInput{
		FinalText: "worker-1 found the cause from evidence ref",
		Agents: []AgentNotification{
			{AgentID: "worker-1", Status: string(AgentStatusRunning), Summary: "found the cause"},
		},
	})
	if decision.Action != AgentFinalGateRequireWait {
		t.Fatalf("action = %q, want %q", decision.Action, AgentFinalGateRequireWait)
	}
	if !containsString(decision.PendingAgents, "worker-1") {
		t.Fatalf("pending agents = %#v, want worker-1", decision.PendingAgents)
	}
}

func TestAgentFinalGateAllowsCompletedEvidenceOnly(t *testing.T) {
	decision := EvaluateAgentFinalGate(FinalGateInput{
		FinalText: "worker-2 completed evidence store://artifact/1",
		Agents: []AgentNotification{
			{AgentID: "worker-2", Status: string(AgentStatusCompleted), Summary: "bounded summary", ResultRefs: []string{"store://artifact/1"}},
		},
	})
	if decision.Action != AgentFinalGateAllow {
		t.Fatalf("action = %q, want %q: %#v", decision.Action, AgentFinalGateAllow, decision)
	}
}

func TestAgentFinalGatePendingDisclosureCannotBypassStatusFacts(t *testing.T) {
	decision := EvaluateAgentFinalGate(FinalGateInput{
		FinalText: "worker-3 is still running and not confirmed",
		Agents:    []AgentNotification{{AgentID: "worker-3", Status: string(AgentStatusRunning)}},
	})
	if decision.Action != AgentFinalGateRequireWait {
		t.Fatalf("action = %q, want %q", decision.Action, AgentFinalGateRequireWait)
	}
}
