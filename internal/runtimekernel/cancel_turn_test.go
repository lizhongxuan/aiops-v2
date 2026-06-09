package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
)

type cancelAwareBlockingModel struct {
	started chan struct{}
}

func newCancelAwareBlockingModel() *cancelAwareBlockingModel {
	return &cancelAwareBlockingModel{started: make(chan struct{}, 1)}
}

func (m *cancelAwareBlockingModel) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	select {
	case m.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *cancelAwareBlockingModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *cancelAwareBlockingModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func TestCancelTurn_PersistsCanceledLifecycle(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	kernel.sessions.Update(session)

	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn result status = %q, want cancelled", result.Status)
	}

	updated := kernel.sessions.Get(session.ID)
	if updated == nil || updated.CurrentTurn == nil {
		t.Fatal("updated session/current turn is nil")
	}
	if updated.CurrentTurn.Lifecycle != TurnLifecycleCanceled {
		t.Fatalf("CurrentTurn lifecycle = %q, want %q", updated.CurrentTurn.Lifecycle, TurnLifecycleCanceled)
	}
	if updated.CurrentTurn.CompletedAt == nil {
		t.Fatal("CurrentTurn.CompletedAt is nil after cancel")
	}
}

func TestCancelTurn_EmitsTurnAbortedProjection(t *testing.T) {
	kernel := newTestKernel(nil)
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel-event", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel-event",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	kernel.sessions.Update(session)

	if _, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    session.CurrentTurn.ID,
		Reason:    "user stop",
	}); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}

	if len(emitter.events) == 0 {
		t.Fatal("expected projection event after cancel")
	}
	last := emitter.events[len(emitter.events)-1]
	if last.Type != EventTurnAborted {
		t.Fatalf("last event type = %q, want %q", last.Type, EventTurnAborted)
	}
	if last.SessionID != session.ID || last.TurnID != session.CurrentTurn.ID {
		t.Fatalf("last event = %+v, want session %q turn %q", last, session.ID, session.CurrentTurn.ID)
	}
}

func TestCancelTurn_CancelsInFlightRunTurn(t *testing.T) {
	kernel := newTestKernel(nil)
	blockingModel := newCancelAwareBlockingModel()
	kernel.modelRouter = modelrouter.NewRouter("blocking", map[string]modelrouter.ChatModel{"blocking": blockingModel}, nil)

	session := kernel.sessions.GetOrCreate("sess-cancel-live", SessionTypeHost, ModeChat)
	done := make(chan struct {
		result TurnResult
		err    error
	}, 1)

	go func() {
		result, err := kernel.RunTurn(context.Background(), TurnRequest{
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			SessionID:   session.ID,
			TurnID:      "turn-cancel-live",
			Input:       "请持续生成",
		})
		done <- struct {
			result TurnResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case <-blockingModel.started:
	case <-time.After(time.Second):
		t.Fatal("model did not start before cancel")
	}

	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel-live",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("RunTurn() error = %v, want nil canceled result", outcome.err)
		}
		if outcome.result.Status != "cancelled" {
			t.Fatalf("RunTurn status = %q, want cancelled", outcome.result.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunTurn did not exit after cancel")
	}
}

func TestRunTurn_DoesNotStartModelWhenCanceledBeforeExecution(t *testing.T) {
	kernel := newTestKernel(nil)
	blockingModel := newCancelAwareBlockingModel()
	kernel.modelRouter = modelrouter.NewRouter("blocking", map[string]modelrouter.ChatModel{"blocking": blockingModel}, nil)

	session := kernel.sessions.GetOrCreate("sess-cancel-pending", SessionTypeHost, ModeChat)
	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel-pending",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	runResult, runErr := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   session.ID,
		TurnID:      "turn-cancel-pending",
		Input:       "不要启动模型",
	})
	if runErr != nil {
		t.Fatalf("RunTurn() error = %v", runErr)
	}
	if runResult.Status != "cancelled" {
		t.Fatalf("RunTurn status = %q, want cancelled", runResult.Status)
	}
	select {
	case <-blockingModel.started:
		t.Fatal("model started despite pending cancel")
	default:
	}
}

func TestResumeTurn_DeniedDecisionEmitsApprovalDecidedProjection(t *testing.T) {
	kernel := newTestKernel(nil)
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-denied", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-denied",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleSuspended,
		ResumeState: TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &CheckpointMetadata{
			ID:          "chk-denied",
			SessionID:   session.ID,
			TurnID:      "turn-denied",
			Iteration:   1,
			Sequence:    1,
			Kind:        "approval_needed",
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []PendingApproval{{
			ID:        "approval-1",
			SessionID: session.ID,
			TurnID:    "turn-denied",
			Iteration: 1,
			ToolName:  "exec_command",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	kernel.sessions.Update(session)

	result, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:  session.ID,
		TurnID:     session.CurrentTurn.ID,
		ApprovalID: "approval-1",
		Decision:   "denied",
		Metadata:   map[string]string{"rejection.reason": "synthetic rejection reason"},
	})
	if err != nil {
		t.Fatalf("ResumeTurn() error = %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("ResumeTurn status = %q, want blocked", result.Status)
	}

	if len(emitter.events) == 0 {
		t.Fatal("expected projection event after denied approval")
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("denied approval must not emit %s", EventToolStarted)
		}
	}
	last := emitter.events[len(emitter.events)-1]
	if last.Type != EventApprovalDecided {
		t.Fatalf("last event type = %q, want %q", last.Type, EventApprovalDecided)
	}
	var payload map[string]string
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("approval.decided payload decode error = %v", err)
	}
	if payload["status"] != "denied" || payload["decision"] != "denied" {
		t.Fatalf("approval.decided payload = %#v, want denied decision/status", payload)
	}
	session = kernel.sessions.Get(session.ID)
	if got := len(session.PendingApprovals); got != 0 {
		t.Fatalf("pending approvals after denied decision = %d, want 0", got)
	}
	if len(session.RejectedApprovals) != 1 || session.RejectedApprovals[0].Reason != "synthetic rejection reason" {
		t.Fatalf("rejected approvals = %#v, want rejection reason recorded", session.RejectedApprovals)
	}
	protocolState := buildProtocolPromptState(session.CurrentTurn, promptcompiler.ToolPromptDelta{}, nil, nil, session.RejectedApprovals)
	if len(protocolState.Items) != 1 || protocolState.Items[0].Status != "denied" || !strings.Contains(protocolState.Items[0].Text, "synthetic rejection reason") {
		t.Fatalf("protocol state = %#v, want denied approval reason", protocolState)
	}
}
