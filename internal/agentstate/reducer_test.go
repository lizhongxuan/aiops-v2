package agentstate

import "testing"

func TestAppendItemDoesNotMutateInputSlice(t *testing.T) {
	initial := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseActing,
		Items: []TurnItem{
			{ID: "item-1", Type: TurnItemTypeUserMessage, Status: ItemStatusCompleted},
		},
	}

	next, err := AppendItem(initial, TurnItem{
		ID:     "item-2",
		Type:   TurnItemTypeModelCall,
		Status: ItemStatusRunning,
	})
	if err != nil {
		t.Fatalf("append item: %v", err)
	}

	if len(initial.Items) != 1 {
		t.Fatalf("input state was mutated: %#v", initial.Items)
	}
	if len(next.Items) != 2 {
		t.Fatalf("expected appended item, got %#v", next.Items)
	}
}

func TestUpdateItemReturnsErrorForMissingItem(t *testing.T) {
	state := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseActing,
		Items: []TurnItem{
			{ID: "item-1", Type: TurnItemTypeUserMessage, Status: ItemStatusCompleted},
		},
	}

	_, err := UpdateItem(state, "missing", func(item TurnItem) (TurnItem, error) {
		item.Status = ItemStatusCompleted
		return item, nil
	})
	if err == nil {
		t.Fatalf("expected missing item update to fail")
	}
}

func TestUpdateItemDoesNotMutateInputSlice(t *testing.T) {
	initial := AgentState{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Phase:     AgentPhaseActing,
		Items: []TurnItem{
			{ID: "item-1", Type: TurnItemTypeModelCall, Status: ItemStatusRunning},
		},
	}

	next, err := UpdateItem(initial, "item-1", func(item TurnItem) (TurnItem, error) {
		item.Status = ItemStatusCompleted
		return item, nil
	})
	if err != nil {
		t.Fatalf("update item: %v", err)
	}

	if initial.Items[0].Status != ItemStatusRunning {
		t.Fatalf("input state was mutated: %#v", initial.Items[0])
	}
	if next.Items[0].Status != ItemStatusCompleted {
		t.Fatalf("expected updated status, got %#v", next.Items[0])
	}
}
