package appui

import (
	"context"
	"testing"
	"time"
)

func serviceTestEvent(sessionID, eventID string, seq int64) AgentEvent {
	event := testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, seq, TurnPayload{Title: eventID})
	event.SessionID = sessionID
	event.EventID = eventID
	return event
}

func TestAgentEventServiceAppendAssignsMonotonicSeqPerSession(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)

	first, err := service.Append(ctx, serviceTestEvent("session-a", "evt-a-1", 0))
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := service.Append(ctx, serviceTestEvent("session-a", "evt-a-2", 0))
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	other, err := service.Append(ctx, serviceTestEvent("session-b", "evt-b-1", 0))
	if err != nil {
		t.Fatalf("Append(other) error = %v", err)
	}

	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("session-a seqs = %d/%d, want 1/2", first.Seq, second.Seq)
	}
	if other.Seq != 1 {
		t.Fatalf("session-b seq = %d, want independent seq 1", other.Seq)
	}
}

func TestAgentEventServiceAppendDedupesEventID(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)

	first, err := service.Append(ctx, serviceTestEvent("session-a", "evt-dup", 0))
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	duplicate := serviceTestEvent("session-a", "evt-dup", 0)
	duplicate.Payload = nil
	second, err := service.Append(ctx, duplicate)
	if err != nil {
		t.Fatalf("Append(duplicate) error = %v", err)
	}

	if first.Seq != second.Seq {
		t.Fatalf("duplicate seq = %d, want existing seq %d", second.Seq, first.Seq)
	}
	events, err := service.Replay(ctx, "session-a", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Replay() returned %d events, want 1", len(events))
	}
	proj, err := service.Projection(ctx, "session-a")
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	if proj.LastSeq != 1 {
		t.Fatalf("Projection.LastSeq = %d, want 1", proj.LastSeq)
	}
}

func TestAgentEventServiceSubscribeReceivesSessionEventsOnly(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)
	ch, unsubscribe := service.Subscribe(ctx, "session-a", 0)
	defer unsubscribe()

	if _, err := service.Append(ctx, serviceTestEvent("session-b", "evt-b-1", 0)); err != nil {
		t.Fatalf("Append(session-b) error = %v", err)
	}
	select {
	case event := <-ch:
		t.Fatalf("received event from wrong session: %+v", event)
	case <-time.After(25 * time.Millisecond):
	}

	appended, err := service.Append(ctx, serviceTestEvent("session-a", "evt-a-1", 0))
	if err != nil {
		t.Fatalf("Append(session-a) error = %v", err)
	}
	select {
	case got := <-ch:
		if got.EventID != appended.EventID || got.Seq != appended.Seq {
			t.Fatalf("subscription event = %+v, want %+v", got, appended)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session-a event")
	}
}

func TestAgentEventServiceReplayReturnsEventsAfterSeq(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)
	for _, id := range []string{"evt-1", "evt-2", "evt-3"} {
		if _, err := service.Append(ctx, serviceTestEvent("session-a", id, 0)); err != nil {
			t.Fatalf("Append(%s) error = %v", id, err)
		}
	}

	events, err := service.Replay(ctx, "session-a", 1)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(events) != 2 || events[0].EventID != "evt-2" || events[1].EventID != "evt-3" {
		t.Fatalf("Replay(after 1) = %+v, want evt-2 and evt-3", events)
	}
}

func TestAgentEventServiceProjectionMatchesReplay(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)
	events := []AgentEvent{
		serviceTestEvent("session-a", "evt-1", 0),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 0, AssistantPayload{Channel: "final", Delta: "hello"}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 0, TurnPayload{Summary: "done"}),
	}
	for i := range events {
		events[i].SessionID = "session-a"
		events[i].EventID = []string{"evt-1", "evt-2", "evt-3"}[i]
		if _, err := service.Append(ctx, events[i]); err != nil {
			t.Fatalf("Append(%d) error = %v", i, err)
		}
	}

	serviceProjection, err := service.Projection(ctx, "session-a")
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	replayedEvents, err := service.Replay(ctx, "session-a", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	replayProjection, err := NewAgentEventProjector().Replay("session-a", replayedEvents)
	if err != nil {
		t.Fatalf("projector Replay() error = %v", err)
	}
	if serviceProjection.LastSeq != replayProjection.LastSeq || serviceProjection.Status != replayProjection.Status {
		t.Fatalf("service projection = %+v, replay projection = %+v", serviceProjection, replayProjection)
	}
	if serviceProjection.FinalMessages["turn-1"].Text != "hello" {
		t.Fatalf("FinalMessages = %+v, want hello", serviceProjection.FinalMessages)
	}
}

func TestAgentEventServiceProjectionSnapshotDoesNotShareMutableMaps(t *testing.T) {
	ctx := context.Background()
	service := NewAgentEventService(nil)

	if _, err := service.Append(ctx, serviceTestEvent("session-a", "evt-1", 0)); err != nil {
		t.Fatalf("Append(started) error = %v", err)
	}
	snapshot, err := service.Projection(ctx, "session-a")
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	if !snapshot.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("snapshot ActiveTurns = %+v, want turn-1 active", snapshot.RuntimeLiveness.ActiveTurns)
	}

	completed := testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 0, TurnPayload{Summary: "done"})
	completed.SessionID = "session-a"
	completed.EventID = "evt-2"
	if _, err := service.Append(ctx, completed); err != nil {
		t.Fatalf("Append(completed) error = %v", err)
	}

	if !snapshot.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("previous projection snapshot was mutated after append: %+v", snapshot.RuntimeLiveness.ActiveTurns)
	}
}
