package agentstate

import "testing"

func TestProjectPromptItemsReturnsStableStateItems(t *testing.T) {
	state := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseObserving,
		Items: []TurnItem{
			{ID: "tool-1", Type: TurnItemTypeToolCall, Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "read config"}},
			{ID: "assistant-running", Type: TurnItemTypeAssistantMessage, Status: ItemStatusRunning, Payload: PayloadEnvelope{Summary: "draft"}},
			{ID: "assistant-final", Type: TurnItemTypeAssistantMessage, Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "done"}},
		},
	}

	items := ProjectPromptItems(state)
	if len(items) != 3 {
		t.Fatalf("expected three projected prompt items, got %#v", items)
	}
	if items[0].Kind != string(TurnItemTypeToolCall) || items[0].ID != "tool-1" || items[0].Status != string(ItemStatusCompleted) {
		t.Fatalf("unexpected first projection: %#v", items[0])
	}
	if items[1].Kind != string(TurnItemTypeAssistantMessage) || items[1].Text != "draft" {
		t.Fatalf("unexpected assistant message projection: %#v", items[1])
	}
	if items[2].Text != "done" {
		t.Fatalf("expected payload summary as projection text, got %#v", items[2])
	}
}
