package agentmgr

import "testing"

func TestAgentContinuationDecisionContinuesFailedWorker(t *testing.T) {
	decision := EvaluateAgentContinuation(ContinuationInput{
		Target: ExistingAgentContext{
			AgentID:         "worker-1",
			Status:          AgentStatusFailed,
			CapabilityKinds: []string{"logs"},
			ResourceTypes:   []string{"service"},
		},
		RequiredCapability:   "logs",
		RequiredResourceType: "service",
		SameProblem:          true,
		HasRecoverableState:  true,
	})
	if decision.Action != AgentContinuationContinue {
		t.Fatalf("action = %q, want %q: %#v", decision.Action, AgentContinuationContinue, decision)
	}
}

func TestContinuationStopRecordRequiresReasonAndEvidence(t *testing.T) {
	record := AgentStopRecord{AgentID: "worker-1", Reason: AgentStopCompleted, EvidenceRefs: []string{"store://artifact/1"}}
	if err := record.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := (AgentStopRecord{AgentID: "worker-1"}).Validate(); err == nil {
		t.Fatal("expected validation error for missing reason")
	}
}

func TestKillAgentWithReasonRecordsStopReason(t *testing.T) {
	manager := NewAgentManager(nil, nil, nil)
	if _, err := manager.Spawn(nil, SpawnRequest{
		ID:        "synthetic-worker-1",
		Kind:      AgentKindWorker,
		MissionID: "synthetic-mission",
		SessionID: "synthetic-session",
		Task:      "synthetic task",
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := manager.KillAgentWithReason(nil, "synthetic-worker-1", "user_cancelled"); err != nil {
		t.Fatalf("kill with reason: %v", err)
	}
	results := manager.CollectResults("synthetic-mission")
	if len(results) != 1 || results[0].Status != AgentStatusKilled || results[0].Error != "user_cancelled" {
		t.Fatalf("results = %#v, want killed with user_cancelled", results)
	}
}
