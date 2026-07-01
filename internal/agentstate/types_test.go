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

func TestTurnItemTimelineTypesAreStandardized(t *testing.T) {
	required := []TurnItemType{
		TurnItemTypeRouteSelected,
		TurnItemTypeToolSurfaceSnapshot,
		TurnItemTypeAssistantMessage,
		TurnItemTypeToolCall,
		TurnItemTypeToolResult,
		TurnItemTypeApprovalRequested,
		TurnItemTypeApprovalDecided,
		TurnItemTypeChildAgentStarted,
		TurnItemTypeChildAgentResult,
		TurnItemTypeContextCompacted,
		TurnItemTypePendingInputAccepted,
		TurnItemTypeTurnCancelled,
		TurnItemTypePermissionSnapshot,
		TurnItemTypeResourceLock,
	}

	seen := map[TurnItemType]bool{}
	for _, typ := range required {
		if typ == "" {
			t.Fatalf("empty turn item type in required list")
		}
		if seen[typ] {
			t.Fatalf("duplicate turn item type %q", typ)
		}
		seen[typ] = true
		if !typ.IsValid() {
			t.Fatalf("required turn item type %q is not valid", typ)
		}
	}
}

func TestTurnItemTypeAssistantMessageIsOnlyAssistantTextType(t *testing.T) {
	if !TurnItemType("assistant_message").IsValid() {
		t.Fatal("assistant_message must be a valid TurnItemType")
	}
	for _, legacy := range []TurnItemType{
		TurnItemType("assistant_progress"),
		TurnItemType("assistant_answer"),
		TurnItemType("final_answer"),
	} {
		if legacy.IsValid() {
			t.Fatalf("%s must not remain a valid production TurnItemType", legacy)
		}
	}
}
