package appui

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

func TestApprovalServicePlanApprovalListAndDecision(t *testing.T) {
	now := time.Date(2026, 6, 7, 14, 0, 0, 0, time.UTC)
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-plan-approval", runtimekernel.SessionTypeHost, runtimekernel.ModePlan)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-plan",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "plan-exit-1",
			SessionID: session.ID,
			TurnID:    "turn-plan",
			Iteration: 1,
			ToolName:  "exit_plan_mode",
			Reason:    "approve plan before execution",
			Risk:      "medium",
			Source:    runtimekernel.PlanExitApprovalSource,
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	approvals, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(approvals) != 1 || approvals[0].ToolName != "exit_plan_mode" || approvals[0].Source != runtimekernel.PlanExitApprovalSource {
		t.Fatalf("approvals = %#v, want plan approval view", approvals)
	}

	_, err = service.Decide(context.Background(), ApprovalDecision{ID: "plan-exit-1", Decision: "approve"})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if runtime.resumeReq.ApprovalID != "plan-exit-1" || runtime.resumeReq.Decision != "approved" || runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingApproval {
		t.Fatalf("resume request = %#v, want plan approval resume", runtime.resumeReq)
	}
}
