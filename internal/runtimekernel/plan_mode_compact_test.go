package runtimekernel

import "testing"

func TestCompactPlanModeRecoverySetsResumeReminder(t *testing.T) {
	session := &SessionState{ID: "sess-compact", Type: SessionTypeHost, Mode: ModePlan}
	input := PlanModeCompactRecoveryInput{
		State:               string(PlanModeStateActive),
		PlanID:              "plan-1",
		ApprovalID:          "approval-1",
		PendingQuestions:    []string{"confirm scope"},
		LastRejectionReason: "too broad",
	}

	state := RecoverPlanModeFromCompactSummary(session, input, "compact_recovery_v1")
	if state.State != PlanModeStateActive || state.PlanID != "plan-1" || state.ReminderLevel != PlanModeReminderResume {
		t.Fatalf("recovered state = %#v, want active plan with resume reminder", state)
	}
	if state.CompactRecovery != "compact_recovery_v1" || len(state.PendingQuestions) != 1 || state.LastRejectionReason != "too broad" {
		t.Fatalf("recovered details = %#v", state)
	}
}
