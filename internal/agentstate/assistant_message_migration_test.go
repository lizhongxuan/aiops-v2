package agentstate

import (
	"encoding/json"
	"testing"
)

func TestMigrateLegacyAssistantItemsToAssistantMessage(t *testing.T) {
	items := []TurnItem{
		{ID: "progress-1", Type: TurnItemType("assistant_progress"), Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "我先查公开来源。"}},
		{ID: "answer-1", Type: TurnItemType("assistant_answer"), Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "旧候选答案", Data: json.RawMessage(`{"answerState":"superseded"}`)}},
		{ID: "final-1", Type: TurnItemType("final_answer"), Status: ItemStatusCompleted, Payload: PayloadEnvelope{Summary: "最终回答。"}},
	}
	got := MigrateLegacyAssistantItemsToAssistantMessage(items, "最终回答。")
	if containsLegacyAssistantItem(got) {
		t.Fatalf("legacy item remained: %#v", got)
	}
	if countAssistantMessagePhase(got, "commentary") != 1 || countAssistantMessagePhase(got, "final_answer") != 1 {
		t.Fatalf("migrated items = %#v, want one commentary and one final", got)
	}
	final := assistantMessageByPhase(got, "final_answer")
	if final.Payload.Summary != "最终回答。" {
		t.Fatalf("final summary = %q, want finalOutput calibration", final.Payload.Summary)
	}
	if !payloadStringSliceContains(final.Payload.Data, "legacySourceIds", "answer-1") {
		t.Fatalf("final payload = %s, want superseded source id", string(final.Payload.Data))
	}
}

func containsLegacyAssistantItem(items []TurnItem) bool {
	for _, item := range items {
		switch item.Type {
		case TurnItemType("assistant_progress"), TurnItemType("assistant_answer"), TurnItemType("final_answer"):
			return true
		}
	}
	return false
}

func countAssistantMessagePhase(items []TurnItem, phase string) int {
	count := 0
	for _, item := range items {
		if item.Type == TurnItemTypeAssistantMessage && payloadString(item.Payload.Data, "phase") == phase {
			count++
		}
	}
	return count
}

func assistantMessageByPhase(items []TurnItem, phase string) TurnItem {
	for _, item := range items {
		if item.Type == TurnItemTypeAssistantMessage && payloadString(item.Payload.Data, "phase") == phase {
			return item
		}
	}
	return TurnItem{}
}

func payloadString(raw json.RawMessage, key string) string {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func payloadStringSliceContains(raw json.RawMessage, key, want string) bool {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return false
	}
	values, _ := payload[key].([]any)
	for _, value := range values {
		if text, _ := value.(string); text == want {
			return true
		}
	}
	return false
}
