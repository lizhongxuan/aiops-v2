package appui

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type chatRuntimeCapture struct {
	runCalled    bool
	runReq       runtimekernel.TurnRequest
	resumeCalled bool
	resumeReq    runtimekernel.ResumeRequest
	cancelReq    runtimekernel.CancelRequest
}

func (r *chatRuntimeCapture) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.runCalled = true
	r.runReq = req
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
	}, nil
}

func (r *chatRuntimeCapture) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	r.resumeCalled = true
	r.resumeReq = req
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "completed"}, nil
}

func (r *chatRuntimeCapture) CancelTurn(_ context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	r.cancelReq = req
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "cancelled"}, nil
}

func TestChatService_SendMessageResumesPendingEvidenceTurn(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-evidence", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-evidence",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		Iteration:   2,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-1",
			SessionID:  session.ID,
			TurnID:     "turn-evidence",
			Iteration:  2,
			ToolName:   "readonly_host_inspect",
			ToolCallID: "call-1",
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
	}
	session.PendingEvidence = append([]runtimekernel.PendingEvidence(nil), session.CurrentTurn.PendingEvidence...)
	sessions.Update(session)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-evidence",
		Content:   "这是补充证据和操作上下文",
		Metadata:  map[string]string{"client": "protocol-workspace"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if runtime.runCalled {
		t.Fatal("SendMessage() called RunTurn, want ResumeTurn for pending evidence")
	}
	if !runtime.resumeCalled {
		t.Fatal("SendMessage() did not call ResumeTurn")
	}
	if runtime.resumeReq.SessionID != "sess-evidence" || runtime.resumeReq.TurnID != "turn-evidence" {
		t.Fatalf("ResumeTurn target = %+v, want sess-evidence/turn-evidence", runtime.resumeReq)
	}
	if runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		t.Fatalf("ResumeState = %q, want pending_evidence", runtime.resumeReq.ResumeState)
	}
	if runtime.resumeReq.CheckpointID != "evidence-1" {
		t.Fatalf("CheckpointID = %q, want evidence-1", runtime.resumeReq.CheckpointID)
	}
	if got := runtime.resumeReq.Metadata["resume.input"]; got != "这是补充证据和操作上下文" {
		t.Fatalf("metadata[resume.input] = %q, want follow-up content", got)
	}
	if got := runtime.resumeReq.Metadata["evidence.id"]; got != "evidence-1" {
		t.Fatalf("metadata[evidence.id] = %q, want evidence-1", got)
	}
}

func TestChatService_SendMessageDefaultsToLatestSessionWhenSessionIDMissing(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	older := sessions.GetOrCreate("sess-older", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	older.UpdatedAt = time.Now().Add(-time.Minute)
	sessions.Update(older)

	latest := sessions.GetOrCreate("sess-latest", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessions.Update(latest)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		Content: "今天几号",
		HostID:  "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !runtime.runCalled {
		t.Fatal("SendMessage() did not call RunTurn")
	}
	if runtime.runReq.SessionID != latest.ID {
		t.Fatalf("RunTurn sessionId = %q, want latest session %q", runtime.runReq.SessionID, latest.ID)
	}
	if runtime.runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runtime.runReq.HostID)
	}
}

func TestChatService_SendMessageCarriesClientIDs(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-client",
		Content:         "需要即时反馈",
		ClientMessageID: "client-msg-1",
		ClientTurnID:    "client-turn-1",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if runtime.runReq.ClientMessageID != "client-msg-1" {
		t.Fatalf("RunTurn ClientMessageID = %q, want client-msg-1", runtime.runReq.ClientMessageID)
	}
	if runtime.runReq.ClientTurnID != "client-turn-1" {
		t.Fatalf("RunTurn ClientTurnID = %q, want client-turn-1", runtime.runReq.ClientTurnID)
	}
	if result.ClientMessageID != "client-msg-1" {
		t.Fatalf("TurnResponse ClientMessageID = %q, want client-msg-1", result.ClientMessageID)
	}
	if result.ClientTurnID != "client-turn-1" {
		t.Fatalf("TurnResponse ClientTurnID = %q, want client-turn-1", result.ClientTurnID)
	}
}
