package store

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentui"
)

func TestAgentEventStorePersistsEventsAndProjection(t *testing.T) {
	dataDir := t.TempDir()
	s, err := NewJSONFileStore(dataDir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}

	event1 := testStoreAgentEvent("evt-1", 1, "turn-1", agentui.TurnPayload{Prompt: "hello"})
	event2 := testStoreAgentEvent("evt-2", 2, "turn-1", agentui.AssistantPayload{Channel: "final", Delta: "world"})
	if err := s.AppendAgentEvent("sess-agent", event1); err != nil {
		t.Fatalf("AppendAgentEvent(event1) error = %v", err)
	}
	if err := s.AppendAgentEvent("sess-agent", event2); err != nil {
		t.Fatalf("AppendAgentEvent(event2) error = %v", err)
	}
	projection := agentui.AgentEventProjection{
		SessionID: "sess-agent",
		Status:    "working",
		LastSeq:   2,
		RuntimeLiveness: agentui.RuntimeLiveness{
			ActiveTurns: map[string]bool{"turn-1": true},
		},
	}
	if err := s.SaveAgentEventProjection("sess-agent", projection); err != nil {
		t.Fatalf("SaveAgentEventProjection() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewJSONFileStore(dataDir, time.Hour)
	if err != nil {
		t.Fatalf("reopen NewJSONFileStore() error = %v", err)
	}
	defer reopened.Close()

	events, err := reopened.ListAgentEvents("sess-agent", 0)
	if err != nil {
		t.Fatalf("ListAgentEvents(afterSeq=0) error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2: %+v", len(events), events)
	}
	if events[0].EventID != "evt-1" || events[0].Seq != 1 || events[1].EventID != "evt-2" || events[1].Seq != 2 {
		t.Fatalf("events = %+v, want seq ordered evt-1/evt-2", events)
	}

	afterOne, err := reopened.ListAgentEvents("sess-agent", 1)
	if err != nil {
		t.Fatalf("ListAgentEvents(afterSeq=1) error = %v", err)
	}
	if len(afterOne) != 1 || afterOne[0].EventID != "evt-2" {
		t.Fatalf("afterSeq events = %+v, want only evt-2", afterOne)
	}

	loaded, ok, err := reopened.LoadAgentEventProjection("sess-agent")
	if err != nil {
		t.Fatalf("LoadAgentEventProjection() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadAgentEventProjection() ok = false, want true")
	}
	if loaded.SessionID != "sess-agent" || loaded.Status != "working" || loaded.LastSeq != 2 {
		t.Fatalf("projection = %+v, want persisted working projection", loaded)
	}
}

func testStoreAgentEvent(eventID string, seq int64, turnID string, payload any) agentui.AgentEvent {
	raw, _ := json.Marshal(payload)
	kind := agentui.AgentEventTurn
	phase := agentui.AgentEventPhaseRequested
	status := agentui.AgentEventStatusQueued
	if _, ok := payload.(agentui.AssistantPayload); ok {
		kind = agentui.AgentEventAssistant
		phase = agentui.AgentEventPhaseDelta
		status = agentui.AgentEventStatusRunning
	}
	return agentui.AgentEvent{
		EventID:    eventID,
		Seq:        seq,
		SessionID:  "sess-agent",
		TurnID:     turnID,
		Kind:       kind,
		Phase:      phase,
		Status:     status,
		Visibility: agentui.AgentEventVisibilityPrimary,
		Source:     agentui.AgentEventSourceRuntime,
		CreatedAt:  "2026-04-24T00:00:00Z",
		Payload:    raw,
	}
}
