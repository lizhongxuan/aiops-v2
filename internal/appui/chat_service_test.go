package appui

import (
	"context"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type chatRuntimeCapture struct {
	mu           sync.Mutex
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

type blockedChatRuntime struct {
	started chan runtimekernel.TurnRequest
}

func newBlockedChatRuntime() *blockedChatRuntime {
	return &blockedChatRuntime{started: make(chan runtimekernel.TurnRequest, 1)}
}

func (r *blockedChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "blocked",
		Error:           "approval required",
	}, nil
}

func (r *blockedChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *blockedChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
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

type lifecycleContextRuntime struct {
	ctxErr chan error
}

func newLifecycleContextRuntime() *lifecycleContextRuntime {
	return &lifecycleContextRuntime{ctxErr: make(chan error, 1)}
}

func (r *lifecycleContextRuntime) RunTurn(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.ctxErr <- ctx.Err()
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "cancelled",
	}, nil
}

func (r *lifecycleContextRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *lifecycleContextRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
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

func TestChatService_SendMessageRecordsAcceptedEventsWhenRequestContextCanceled(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	defer close(runtime.release)
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.SendMessage(ctx, ChatCommand{
		SessionID:       "sess-canceled-request",
		Content:         "请求上下文已取消但 accepted 事件仍应记录",
		ClientMessageID: "client-msg-canceled-request",
		ClientTurnID:    "client-turn-canceled-request",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}
	replayed := waitForAgentEvents(t, events, "sess-canceled-request", 2)
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
}

func TestDefaultAsyncTurnRunnerUsesLifecycleContext(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := newLifecycleContextRuntime()
	runner := defaultAsyncTurnRunner{runtime: runtime, baseContext: baseCtx}

	runner.run(runtimekernel.TurnRequest{SessionID: "sess-lifecycle", TurnID: "turn-lifecycle"})

	select {
	case err := <-runtime.ctxErr:
		if err != context.Canceled {
			t.Fatalf("RunTurn context error = %v, want context.Canceled", err)
		}
	default:
		t.Fatal("RunTurn was not called")
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

func TestChatService_SendMessageBlockedRuntimeDoesNotEmitTerminalFailure(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockedChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async-blocked",
		Content:         "需要审批",
		ClientMessageID: "client-msg-blocked",
		ClientTurnID:    "client-turn-blocked",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}

	replayed := waitForAgentEvents(t, events, "sess-async-blocked", 3)
	if len(replayed) != 3 {
		t.Fatalf("agent events = %+v, want requested + agent started + agent blocked", replayed)
	}
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
	if replayed[2].Kind != AgentEventAgent || replayed[2].Phase != AgentEventPhaseBlocked || replayed[2].Status != AgentEventStatusBlocked {
		t.Fatalf("third event = %s/%s/%s, want agent/blocked/blocked", replayed[2].Kind, replayed[2].Phase, replayed[2].Status)
	}
	for _, event := range replayed {
		if event.Kind == AgentEventTurn && event.Phase == AgentEventPhaseFailed {
			t.Fatalf("blocked runtime emitted terminal failure event: %+v", event)
		}
	}
}

func (r *chatRuntimeCapture) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.runCalled = true
	r.runReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
	}, nil
}

func (r *chatRuntimeCapture) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.resumeCalled = true
	r.resumeReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "completed"}, nil
}

func (r *chatRuntimeCapture) CancelTurn(_ context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.cancelReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "cancelled"}, nil
}

func (r *chatRuntimeCapture) runSnapshot() (runtimekernel.TurnRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runReq, r.runCalled
}

func (r *chatRuntimeCapture) resumeSnapshot() (runtimekernel.ResumeRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resumeReq, r.resumeCalled
}

func (r *chatRuntimeCapture) cancelSnapshot() runtimekernel.CancelRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelReq
}

func waitForRunTurn(t *testing.T, runtime *chatRuntimeCapture) runtimekernel.TurnRequest {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if req, ok := runtime.runSnapshot(); ok {
			return req
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RunTurn was not called")
	return runtimekernel.TurnRequest{}
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
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("SendMessage() called RunTurn, want ResumeTurn for pending evidence")
	}
	resumeReq, resumeCalled := runtime.resumeSnapshot()
	if !resumeCalled {
		t.Fatal("SendMessage() did not call ResumeTurn")
	}
	if resumeReq.SessionID != "sess-evidence" || resumeReq.TurnID != "turn-evidence" {
		t.Fatalf("ResumeTurn target = %+v, want sess-evidence/turn-evidence", resumeReq)
	}
	if resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		t.Fatalf("ResumeState = %q, want pending_evidence", resumeReq.ResumeState)
	}
	if resumeReq.CheckpointID != "evidence-1" {
		t.Fatalf("CheckpointID = %q, want evidence-1", resumeReq.CheckpointID)
	}
	if got := resumeReq.Metadata["resume.input"]; got != "这是补充证据和操作上下文" {
		t.Fatalf("metadata[resume.input] = %q, want follow-up content", got)
	}
	if got := resumeReq.Metadata["evidence.id"]; got != "evidence-1" {
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
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionID != latest.ID {
		t.Fatalf("RunTurn sessionId = %q, want latest session %q", runReq.SessionID, latest.ID)
	}
	if runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runReq.HostID)
	}
}

func TestChatService_SendMessageUsesSessionModeWhenSessionIDProvided(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-host-exec", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.HostID = "server-local"
	sessions.Update(session)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-host-exec",
		Content:         "帮我启动 docker",
		HostID:          "server-local",
		ClientMessageID: "client-msg-1",
		ClientTurnID:    "client-turn-1",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("result status = %q, want accepted", result.Status)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeHost {
		t.Fatalf("RunTurn sessionType = %q, want host", runReq.SessionType)
	}
	if runReq.Mode != runtimekernel.ModeExecute {
		t.Fatalf("RunTurn mode = %q, want execute", runReq.Mode)
	}
	if runReq.SessionID != "sess-host-exec" {
		t.Fatalf("RunTurn sessionID = %q, want sess-host-exec", runReq.SessionID)
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
	runReq := waitForRunTurn(t, runtime)
	if runReq.ClientMessageID != "client-msg-1" {
		t.Fatalf("RunTurn ClientMessageID = %q, want client-msg-1", runReq.ClientMessageID)
	}
	if runReq.ClientTurnID != "client-turn-1" {
		t.Fatalf("RunTurn ClientTurnID = %q, want client-turn-1", runReq.ClientTurnID)
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
	cancelReq := runtime.cancelSnapshot()
	if cancelReq.SessionID != "sess-explicit-stop" || cancelReq.TurnID != "turn-explicit-stop" {
		t.Fatalf("CancelTurn request = %+v, want explicit session/turn ids", cancelReq)
	}

	replayed, err := events.Replay(context.Background(), "sess-explicit-stop", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
}
