package agentstate

import "testing"

func TestProjectPromptItemsReturnsStableStateItems(t *testing.T) {
	state := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseObserving,
		Items: []TurnItem{
			{ID: "tool-1", Type: TurnItemTypeToolCall, Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "read config"}},
			{ID: "answer-1", Type: TurnItemTypeFinalAnswer, Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "done"}},
		},
	}

	items := ProjectPromptItems(state)
	if len(items) != 2 {
		t.Fatalf("expected two projected prompt items, got %#v", items)
	}
	if items[0].Kind != string(TurnItemTypeToolCall) || items[0].ID != "tool-1" || items[0].Status != string(ItemStatusCompleted) {
		t.Fatalf("unexpected first projection: %#v", items[0])
	}
	if items[1].Text != "done" {
		t.Fatalf("expected payload summary as projection text, got %#v", items[1])
	}
}
