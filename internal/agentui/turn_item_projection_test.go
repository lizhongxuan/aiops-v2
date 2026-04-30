package agentui

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestProjectTurnItemsToAgentEventsIsStable(t *testing.T) {
	createdAt := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	items := []agentstate.TurnItem{
		{
			ID:        "model-1",
			Type:      agentstate.TurnItemTypeModelCall,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Summary: "model response received"},
			CreatedAt: createdAt,
		},
		{
			ID:        "final-1",
			Type:      agentstate.TurnItemTypeFinalAnswer,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Summary: "done"},
			CreatedAt: createdAt,
		},
	}

	first := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 10)
	second := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 10)

	if len(first) != len(second) {
		t.Fatalf("projection length changed: %d vs %d", len(first), len(second))
	}
	for i := range first {
		firstJSON, _ := json.Marshal(first[i])
		secondJSON, _ := json.Marshal(second[i])
		if string(firstJSON) != string(secondJSON) {
			t.Fatalf("projection[%d] changed:\nfirst=%#v\nsecond=%#v", i, first[i], second[i])
		}
		if err := first[i].Validate(); err != nil {
			t.Fatalf("projected event invalid: %v", err)
		}
	}
}

func TestProjectTurnItemsToAgentEventsDeduplicatesFinalAnswer(t *testing.T) {
	items := []agentstate.TurnItem{
		{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "first"}},
		{ID: "final-2", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "second"}},
	}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	if len(events) != 1 {
		t.Fatalf("events = %d, want one final answer event", len(events))
	}
	if events[0].Kind != AgentEventAssistant {
		t.Fatalf("final event kind = %q, want assistant", events[0].Kind)
	}
}

func TestProjectTurnItemsToAgentEventsKeepsToolStartBeforeCompletion(t *testing.T) {
	items := []agentstate.TurnItem{
		{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "read_file"}},
		{ID: "tool-result-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "ok"}},
	}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	if len(events) != 2 {
		t.Fatalf("events = %d, want two tool events", len(events))
	}
	if events[0].Kind != AgentEventTool || events[0].Phase != AgentEventPhaseStarted {
		t.Fatalf("first event = %#v, want tool started", events[0])
	}
	if events[1].Kind != AgentEventTool || events[1].Phase != AgentEventPhaseCompleted {
		t.Fatalf("second event = %#v, want tool completed", events[1])
	}
}
