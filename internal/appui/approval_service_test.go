package appui

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type approvalRuntimeStub struct {
	resumeReq runtimekernel.ResumeRequest
}

func (s *approvalRuntimeStub) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (s *approvalRuntimeStub) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	s.resumeReq = req
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (s *approvalRuntimeStub) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestApprovalService_DecideResumesMatchingApproval(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:          "chk-approval",
			SessionID:   session.ID,
			TurnID:      "turn-approval",
			Iteration:   1,
			Sequence:    1,
			Kind:        "approval_needed",
			Lifecycle:   runtimekernel.TurnLifecycleSuspended,
			ResumeState: runtimekernel.TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-1",
			SessionID: session.ID,
			TurnID:    "turn-approval",
			Iteration: 1,
			ToolName:  "exec_command",
			HostID:    "host-a",
			Reason:    "needs approval",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	result, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-1",
		Decision: "accept",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if runtime.resumeReq.SessionID != session.ID {
		t.Fatalf("ResumeTurn sessionId = %q, want %q", runtime.resumeReq.SessionID, session.ID)
	}
	if runtime.resumeReq.TurnID != "turn-approval" {
		t.Fatalf("ResumeTurn turnId = %q, want turn-approval", runtime.resumeReq.TurnID)
	}
	if runtime.resumeReq.ApprovalID != "approval-1" {
		t.Fatalf("ResumeTurn approvalId = %q, want approval-1", runtime.resumeReq.ApprovalID)
	}
	if runtime.resumeReq.Decision != "approved" {
		t.Fatalf("ResumeTurn decision = %q, want approved", runtime.resumeReq.Decision)
	}
	if result.Status != "completed" {
		t.Fatalf("ActionResult status = %q, want completed", result.Status)
	}
}
