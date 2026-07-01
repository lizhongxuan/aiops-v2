package modelrouter

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestModelInputItemsToEinoMessagesPreservesToolCalls(t *testing.T) {
	items := []promptinput.ModelInputItem{{
		ID:           "assistant-1",
		ProviderRole: promptinput.ProviderRoleAssistant,
		Content:      "I will inspect disk",
		ToolCalls: []promptinput.ModelInputToolCall{{
			ID:        "call-1",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"cmd":"df -h"}`),
		}},
	}}

	messages, audit, err := ModelInputItemsToEinoMessages(items)
	if err != nil {
		t.Fatalf("ModelInputItemsToEinoMessages() error = %v", err)
	}
	if len(messages) != 1 || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("tool calls not preserved: %#v", messages)
	}
	if audit.ProviderMessagesHash == "" || audit.Items[0].ItemID != "assistant-1" {
		t.Fatalf("audit missing hashes or item id: %#v", audit)
	}
}

func TestModelInputItemsToEinoMessagesRejectsInvalidItem(t *testing.T) {
	_, _, err := ModelInputItemsToEinoMessages([]promptinput.ModelInputItem{{ID: "bad"}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
