package agentstate

import "testing"

func TestTurnItemValidateRequiresIDTypeAndStatus(t *testing.T) {
	tests := []struct {
		name string
		item TurnItem
	}{
		{
			name: "missing id",
			item: TurnItem{Type: TurnItemTypeUserMessage, Status: ItemStatusCompleted},
		},
		{
			name: "invalid type",
			item: TurnItem{ID: "item-1", Type: TurnItemType("unknown"), Status: ItemStatusCompleted},
		},
		{
			name: "invalid status",
			item: TurnItem{ID: "item-1", Type: TurnItemTypeUserMessage, Status: ItemStatus("unknown")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.item.Validate(); err == nil {
				t.Fatalf("expected validation error for %#v", tt.item)
			}
		})
	}
}

func TestAgentStateValidateChecksPhaseAndItems(t *testing.T) {
	state := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseActing,
		Items: []TurnItem{
			{ID: "item-1", Type: TurnItemTypeUserMessage, Status: ItemStatusCompleted},
		},
	}

	if err := state.Validate(); err != nil {
		t.Fatalf("expected valid state: %v", err)
	}

	state.Phase = AgentPhase("invalid")
	if err := state.Validate(); err == nil {
		t.Fatalf("expected invalid phase to fail")
	}
}
