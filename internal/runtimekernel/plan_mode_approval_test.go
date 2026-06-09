package runtimekernel

import (
	"testing"
	"time"
)

func TestExitPlanModeRequiresValidPlanArtifactAndCreatesApprovalRequest(t *testing.T) {
	now := time.Date(2026, 6, 7, 11, 0, 0, 0, time.UTC)
	session := &SessionState{ID: "sess-exit", Type: SessionTypeHost, Mode: ModePlan, PlanMode: PlanModeState{State: PlanModeStateActive, AllowDraftPlan: true}}

	result, _, err := RequestExitPlanMode(session, "turn-1", RuntimePlanArtifact{}, now)
	if err != nil {
		t.Fatalf("RequestExitPlanMode() error = %v", err)
	}
	if result.Status != "missing_sections" || len(result.MissingSections) == 0 || len(session.PendingApprovals) != 0 {
		t.Fatalf("missing result=%#v approvals=%d, want missing sections/no approval", result, len(session.PendingApprovals))
	}

	result, _, err = RequestExitPlanMode(session, "turn-1", RuntimePlanArtifact{
		ID:            "plan-1",
		Objective:     "deploy safely",
		Steps:         []RuntimePlanStep{{ID: "s1", Text: "verify preconditions"}},
		OpenQuestions: []string{"Which maintenance window?"},
	}, now)
	if err != nil {
		t.Fatalf("RequestExitPlanMode(open questions) error = %v", err)
	}
	if result.Status != "open_questions_remaining" || len(session.PendingApprovals) != 0 || len(session.PlanMode.PendingQuestions) != 1 {
		t.Fatalf("open question result=%#v state=%#v approvals=%d", result, session.PlanMode, len(session.PendingApprovals))
	}

	artifact := RuntimePlanArtifact{
		ID:        "plan-1",
		Objective: "deploy safely",
		Steps:     []RuntimePlanStep{{ID: "s1", Text: "verify preconditions"}},
		ApprovalScope: &PlanApprovalScope{
			PlanID:         "plan-1",
			AllowedActions: []string{"restart_service"},
			ResourceScopes: []PlanApprovalResourceScope{{Type: "service", ID: "svc-a"}},
			RiskCeiling:    "high",
		},
	}
	result, artifact, err = RequestExitPlanMode(session, "turn-1", artifact, now)
	if err != nil {
		t.Fatalf("RequestExitPlanMode(valid) error = %v", err)
	}
	if !result.Allowed || result.Status != string(PlanModeStatePendingExitApproval) || artifact.Status != PlanArtifactPendingApproval {
		t.Fatalf("valid result=%#v artifact=%#v, want pending approval", result, artifact)
	}
	if session.PlanMode.State != PlanModeStatePendingExitApproval || session.PlanMode.ApprovalID == "" || len(session.PendingApprovals) != 1 {
		t.Fatalf("session state=%#v approvals=%#v", session.PlanMode, session.PendingApprovals)
	}
}

func TestPlanApprovalApproveBindsApprovedPlanAndScope(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	session := &SessionState{ID: "sess-approve", Type: SessionTypeHost, Mode: ModePlan, PlanMode: PlanModeState{
		State:      PlanModeStatePendingExitApproval,
		PlanID:     "plan-1",
		ApprovalID: "approval-1",
	}}
	session.PendingApprovals = []PendingApproval{{ID: "approval-1", SessionID: session.ID, TurnID: "turn-1", ToolName: "exit_plan_mode", Source: PlanExitApprovalSource}}
	artifact := RuntimePlanArtifact{
		ID: "plan-1",
		ApprovalScope: &PlanApprovalScope{
			PlanID:         "plan-1",
			ApprovalID:     "approval-1",
			AllowedActions: []string{"restart_service"},
			RiskCeiling:    "high",
		},
	}

	artifact, state, err := ApplyPlanApprovalDecision(session, artifact, "approval-1", "approved", "", now)
	if err != nil {
		t.Fatalf("ApplyPlanApprovalDecision(approved) error = %v", err)
	}
	if artifact.Status != PlanArtifactApproved || state.State != PlanModeStateApproved || state.ApprovedPlanID != "plan-1" || session.Mode != ModeExecute {
		t.Fatalf("artifact=%#v state=%#v mode=%q, want approved execute", artifact, state, session.Mode)
	}
	if len(session.PendingApprovals) != 0 || len(session.PlanApprovalScopes) != 1 {
		t.Fatalf("approvals=%d scopes=%d, want no pending and one scope", len(session.PendingApprovals), len(session.PlanApprovalScopes))
	}
}

func TestPlanRejectionKeepsActiveAndRecordsReason(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	session := &SessionState{ID: "sess-reject", Type: SessionTypeHost, Mode: ModePlan, PlanMode: PlanModeState{
		State:      PlanModeStatePendingExitApproval,
		PlanID:     "plan-1",
		ApprovalID: "approval-1",
	}}
	artifact, state, err := ApplyPlanApprovalDecision(session, RuntimePlanArtifact{ID: "plan-1"}, "approval-1", "denied", "scope is too broad", now)
	if err != nil {
		t.Fatalf("ApplyPlanApprovalDecision(denied) error = %v", err)
	}
	if artifact.Status != PlanArtifactRejected || len(artifact.Rejections) != 1 || artifact.Rejections[0].Reason != "scope is too broad" {
		t.Fatalf("artifact rejection = %#v, want reason", artifact)
	}
	if state.State != PlanModeStateActive || session.Mode != ModePlan || state.LastRejectionReason != "scope is too broad" || state.ReminderLevel != PlanModeReminderResume {
		t.Fatalf("state=%#v mode=%q, want active plan with rejection summary", state, session.Mode)
	}
}
