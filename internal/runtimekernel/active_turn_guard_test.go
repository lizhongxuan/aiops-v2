package runtimekernel

import (
	"context"
	"testing"
	"time"
)

func TestNoConcurrentRegularTurnsQueuesPendingInput(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-active-guard", SessionTypeHost, ModeChat)
	session.ActiveTurn = ActiveTurnState{TurnID: "turn-active", Kind: "regular", Status: string(TurnLifecycleRunning)}
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-active",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	kernel.sessions.Update(session)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:       session.ID,
		SessionType:     SessionTypeHost,
		Mode:            ModeChat,
		TurnID:          "turn-new",
		ClientTurnID:    "client-turn-new",
		ClientMessageID: "client-message-new",
		Input:           "追加一个排查条件",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "pending_input" {
		t.Fatalf("RunTurn status = %q, want pending_input", result.Status)
	}
	if result.TurnID != "turn-active" {
		t.Fatalf("RunTurn turn id = %q, want active turn id", result.TurnID)
	}

	updated := kernel.sessions.Get(session.ID)
	if updated.CurrentTurn == nil || updated.CurrentTurn.ID != "turn-active" {
		t.Fatalf("CurrentTurn = %#v, want original active turn", updated.CurrentTurn)
	}
	if len(updated.TurnHistory) != 1 || updated.TurnHistory[0].ID != "turn-active" {
		t.Fatalf("TurnHistory = %#v, want only active turn", updated.TurnHistory)
	}
	if len(updated.CurrentTurn.PendingInputs) != 1 {
		t.Fatalf("PendingInputs = %#v, want one queued input", updated.CurrentTurn.PendingInputs)
	}
	pending := updated.CurrentTurn.PendingInputs[0]
	if pending.Content != "追加一个排查条件" || pending.ClientMessageID != "client-message-new" || pending.ClientTurnID != "client-turn-new" {
		t.Fatalf("pending input = %#v, want queued client input", pending)
	}
	if updated.ActiveTurn.TurnID != "turn-active" || updated.ActiveTurn.Kind != "regular" || updated.ActiveTurn.Status != string(TurnLifecycleRunning) {
		t.Fatalf("ActiveTurn = %#v, want running regular turn-active", updated.ActiveTurn)
	}
}

func TestActiveTurnGuardIgnoresCompletedCurrentTurn(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-active-completed", SessionTypeHost, ModeChat)
	session.ActiveTurn = ActiveTurnState{TurnID: "turn-old", Kind: "regular", Status: string(TurnLifecycleCompleted)}
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-old",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleCompleted,
		ResumeState: TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
	}
	kernel.sessions.Update(session)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   session.ID,
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-new-after-completed",
		Input:       "开始新的问题",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status == "pending_input" {
		t.Fatalf("RunTurn status = pending_input, want new turn allowed after completed active turn")
	}
}
