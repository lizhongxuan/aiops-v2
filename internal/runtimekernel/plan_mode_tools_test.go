package runtimekernel

import (
	"testing"
	"time"
)

func TestEnterPlanModeCreatesRequestedStateAndPendingChoice(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	session := &SessionState{ID: "sess-plan", Type: SessionTypeHost, Mode: ModeChat}

	result, err := RequestEnterPlanMode(session, "turn-1", EnterPlanModeRequest{
		Reason:           "implementation needs an explicit plan",
		ExpectedPlanType: PlanModeExpectedImplementation,
	}, now)
	if err != nil {
		t.Fatalf("RequestEnterPlanMode() error = %v", err)
	}
	if !result.Allowed || result.Status != string(PlanModeStateRequested) {
		t.Fatalf("enter result = %#v, want requested", result)
	}
	if session.Mode != ModeChat {
		t.Fatalf("mode changed before approval = %q", session.Mode)
	}
	if session.PlanMode.State != PlanModeStateRequested || session.PlanMode.ApprovalID == "" {
		t.Fatalf("plan mode = %#v, want requested with approval", session.PlanMode)
	}
	if len(session.PendingApprovals) != 1 || session.PendingApprovals[0].Source != PlanModeEntryApprovalSource || session.PendingApprovals[0].ToolName != "enter_plan_mode" {
		t.Fatalf("pending approvals = %#v, want plan entry approval", session.PendingApprovals)
	}

	state, err := ApplyPlanModeEntryDecision(session, session.PlanMode.ApprovalID, "approved", "", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ApplyPlanModeEntryDecision(approved) error = %v", err)
	}
	if state.State != PlanModeStateActive || session.Mode != ModePlan || len(session.PendingApprovals) != 0 {
		t.Fatalf("approved state=%#v mode=%q approvals=%d, want active plan/no approvals", state, session.Mode, len(session.PendingApprovals))
	}
}

func TestEnterPlanModeDeniedKeepsOriginalModeAndReason(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	session := &SessionState{ID: "sess-deny", Type: SessionTypeHost, Mode: ModeInspect}
	_, err := RequestEnterPlanMode(session, "turn-1", EnterPlanModeRequest{Reason: "need a plan"}, now)
	if err != nil {
		t.Fatalf("RequestEnterPlanMode() error = %v", err)
	}
	state, err := ApplyPlanModeEntryDecision(session, session.PlanMode.ApprovalID, "denied", "too early", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ApplyPlanModeEntryDecision(denied) error = %v", err)
	}
	if state.State != PlanModeStateInactive || session.Mode != ModeInspect || state.LastRejectionReason != "too early" {
		t.Fatalf("denied state=%#v mode=%q, want inactive original mode with reason", state, session.Mode)
	}
}

func TestEnterPlanModeInActiveStateDoesNotDuplicateRequest(t *testing.T) {
	session := &SessionState{
		ID:   "sess-active",
		Type: SessionTypeHost,
		Mode: ModePlan,
		PlanMode: PlanModeState{
			State:          PlanModeStateActive,
			AllowDraftPlan: true,
		},
	}
	result, err := RequestEnterPlanMode(session, "turn-1", EnterPlanModeRequest{Reason: "again"}, time.Now())
	if err != nil {
		t.Fatalf("RequestEnterPlanMode() error = %v", err)
	}
	if result.Status != string(PlanModeStateActive) || len(session.PendingApprovals) != 0 {
		t.Fatalf("result=%#v approvals=%d, want active without duplicate approval", result, len(session.PendingApprovals))
	}
}
