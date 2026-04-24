package runtimekernel

import (
	"context"
	"testing"
	"time"
)

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

func TestCancelTurn_EmitsTurnCompleteProjection(t *testing.T) {
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
	if last.Type != EventTurnComplete {
		t.Fatalf("last event type = %q, want %q", last.Type, EventTurnComplete)
	}
	if last.SessionID != session.ID || last.TurnID != session.CurrentTurn.ID {
		t.Fatalf("last event = %+v, want session %q turn %q", last, session.ID, session.CurrentTurn.ID)
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
	last := emitter.events[len(emitter.events)-1]
	if last.Type != EventApprovalDecided {
		t.Fatalf("last event type = %q, want %q", last.Type, EventApprovalDecided)
	}
}
