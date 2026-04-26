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

type cancelledChatRuntime struct {
	started chan runtimekernel.TurnRequest
}

func newCancelledChatRuntime() *cancelledChatRuntime {
	return &cancelledChatRuntime{started: make(chan runtimekernel.TurnRequest, 1)}
}

func (r *cancelledChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "cancelled",
	}, nil
}

func (r *cancelledChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *cancelledChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type blockingChatRuntime struct {
	started chan runtimekernel.TurnRequest
	release chan struct{}
}

func newBlockingChatRuntime() *blockingChatRuntime {
	return &blockingChatRuntime{
		started: make(chan runtimekernel.TurnRequest, 1),
		release: make(chan struct{}),
	}
}

func (r *blockingChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	<-r.release
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
		Output:          "final output should not be in accepted response",
	}, nil
}

func (r *blockingChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *blockingChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestChatService_SendMessageAcceptedOnlyStartsRuntimeAsync(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	start := time.Now()
	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async",
		Content:         "需要异步执行",
		ClientMessageID: "client-msg-async",
		ClientTurnID:    "client-turn-async",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("SendMessage() took %s, want accepted-only quick return", elapsed)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	if result.Output != "" {
		t.Fatalf("Output = %q, want empty accepted response", result.Output)
	}
	if result.ClientMessageID != "client-msg-async" || result.ClientTurnID != "client-turn-async" {
		t.Fatalf("client ids = %q/%q", result.ClientMessageID, result.ClientTurnID)
	}

	select {
	case req := <-runtime.started:
		if req.ClientMessageID != "client-msg-async" || req.ClientTurnID != "client-turn-async" {
			t.Fatalf("runtime client ids = %q/%q", req.ClientMessageID, req.ClientTurnID)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}
	close(runtime.release)
	replayed := waitForAgentEvents(t, events, "sess-async", 3)
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
	if replayed[2].Kind != AgentEventAgent || replayed[2].Phase != AgentEventPhaseCompleted || replayed[2].Status != AgentEventStatusCompleted {
		t.Fatalf("third event = %s/%s/%s, want agent/completed/completed", replayed[2].Kind, replayed[2].Phase, replayed[2].Status)
	}
}

func TestChatService_SendMessageCancelledRuntimeDoesNotEmitTerminalFailureOrCompletion(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newCancelledChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async-cancelled",
		Content:         "需要异步取消",
		ClientMessageID: "client-msg-cancelled",
		ClientTurnID:    "client-turn-cancelled",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}

	replayed := waitForAgentEvents(t, events, "sess-async-cancelled", 2)
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want only requested + agent started", replayed)
	}
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
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

func waitForRunTurn(t *testing.T, runtime *chatRuntimeCapture) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.runCalled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RunTurn was not called")
}

func waitForAgentEvents(t *testing.T, events AgentEventService, sessionID string, wantAtLeast int) []AgentEvent {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		replayed, err := events.Replay(context.Background(), sessionID, 0)
		if err != nil {
			t.Fatalf("Replay() error = %v", err)
		}
		if len(replayed) >= wantAtLeast {
			return replayed
		}
		time.Sleep(10 * time.Millisecond)
	}
	replayed, err := events.Replay(context.Background(), sessionID, 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	t.Fatalf("agent events = %+v, want at least %d events", replayed, wantAtLeast)
	return nil
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
	waitForRunTurn(t, runtime)
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
	waitForRunTurn(t, runtime)
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

func TestChatService_CancelTurnAppendsCanceledAgentEvent(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.CancelTurn(context.Background(), CancelCommand{
		SessionID: "sess-cancel",
		TurnID:    "turn-cancel",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	replayed, err := events.Replay(context.Background(), "sess-cancel", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
	if replayed[0].Kind != AgentEventAgent || replayed[0].Phase != AgentEventPhaseCanceled || replayed[0].Status != AgentEventStatusCanceled {
		t.Fatalf("agent cancel event = %s/%s/%s, want agent/canceled/canceled", replayed[0].Kind, replayed[0].Phase, replayed[0].Status)
	}
	if replayed[1].Kind != AgentEventTurn || replayed[1].Phase != AgentEventPhaseCanceled || replayed[1].Status != AgentEventStatusCanceled {
		t.Fatalf("turn cancel event = %s/%s/%s, want turn/canceled/canceled", replayed[1].Kind, replayed[1].Phase, replayed[1].Status)
	}
}

func TestChatService_StopTurnAppendsCanceledAgentEvent(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-stop", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:        "turn-stop",
		SessionID: session.ID,
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.StopTurn(context.Background(), StopCommand{
		SessionID: "sess-stop",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("StopTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("StopTurn status = %q, want cancelled", result.Status)
	}

	replayed, err := events.Replay(context.Background(), "sess-stop", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
	if replayed[0].Kind != AgentEventAgent || replayed[0].TurnID != "turn-stop" || replayed[0].Phase != AgentEventPhaseCanceled || replayed[0].Status != AgentEventStatusCanceled {
		t.Fatalf("agent stop event = %+v, want turn-stop agent canceled event", replayed[0])
	}
	if replayed[1].Kind != AgentEventTurn || replayed[1].TurnID != "turn-stop" || replayed[1].Phase != AgentEventPhaseCanceled || replayed[1].Status != AgentEventStatusCanceled {
		t.Fatalf("turn stop event = %+v, want turn-stop canceled event", replayed[1])
	}
}

func TestChatService_StopTurnCancelsAcceptedTurnByExplicitIDsWithoutCurrentTurn(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	sessions.GetOrCreate("sess-explicit-stop", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.StopTurn(context.Background(), StopCommand{
		SessionID: "sess-explicit-stop",
		TurnID:    "turn-explicit-stop",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("StopTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("StopTurn status = %q, want cancelled", result.Status)
	}
	if runtime.cancelReq.SessionID != "sess-explicit-stop" || runtime.cancelReq.TurnID != "turn-explicit-stop" {
		t.Fatalf("CancelTurn request = %+v, want explicit session/turn ids", runtime.cancelReq)
	}

	replayed, err := events.Replay(context.Background(), "sess-explicit-stop", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
}
