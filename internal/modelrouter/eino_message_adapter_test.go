package modelrouter

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
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

func TestModelInputItemsToEinoMessagesPreservesCanonicalOrderExactly(t *testing.T) {
	items := canonicalAdapterModelInputItems()
	messages, _, err := ModelInputItemsToEinoMessages(items)
	if err != nil {
		t.Fatalf("ModelInputItemsToEinoMessages() error = %v", err)
	}
	if len(messages) != len(items) {
		t.Fatalf("message count = %d, want %d", len(messages), len(items))
	}
	for index, message := range messages {
		if message.Extra["model_input_item_id"] != items[index].ID || message.Extra["source_layer"] != items[index].Source.Layer {
			t.Fatalf("message[%d] was reordered or lost identity: %#v", index, message)
		}
	}
	if len(messages[5].ToolCalls) != 1 || messages[5].ToolCalls[0].ID != "call-1" || messages[6].ToolCallID != "call-1" {
		t.Fatalf("provider causal group changed: %#v", messages)
	}
	last := messages[len(messages)-1]
	if last.Extra["semantic_role"] != "continuation_instruction" || last.Extra["source_layer"] != string(promptcompiler.LayerCurrentUserInput) {
		t.Fatalf("provider tail = %#v, want typed L6 continuation", last)
	}
}

func TestModelInputItemsToEinoMessagesRejectsInvalidCanonicalOrder(t *testing.T) {
	t.Run("orphan tool result", func(t *testing.T) {
		items := canonicalAdapterModelInputItems()
		items = append(items[:5], items[6:]...)
		_, _, err := ModelInputItemsToEinoMessages(items)
		if err == nil || !strings.Contains(err.Error(), "causal") {
			t.Fatalf("error = %v, want causal rejection", err)
		}
	})
	t.Run("logical layer regression", func(t *testing.T) {
		items := canonicalAdapterModelInputItems()
		items[4], items[7] = items[7], items[4]
		_, _, err := ModelInputItemsToEinoMessages(items)
		if err == nil || !strings.Contains(err.Error(), "logical") {
			t.Fatalf("error = %v, want logical rejection", err)
		}
	})
	t.Run("mixed typed and untyped", func(t *testing.T) {
		items := canonicalAdapterModelInputItems()
		untyped := promptinput.ModelInputItem{ID: "untyped", ProviderRole: promptinput.ProviderRoleSystem, Content: "must not bypass typed order"}
		items = append(items[:len(items)-1], untyped, items[len(items)-1])
		_, _, err := ModelInputItemsToEinoMessages(items)
		if err == nil || !strings.Contains(err.Error(), "logical") {
			t.Fatalf("error = %v, want mixed-input rejection", err)
		}
	})
}

func canonicalAdapterModelInputItems() []promptinput.ModelInputItem {
	layerItem := func(id string, role promptinput.ProviderRole, layer promptcompiler.PromptLogicalLayer, content string) promptinput.ModelInputItem {
		return promptinput.ModelInputItem{
			ID: id, ProviderRole: role, SemanticRole: string(layer), Content: content,
			Source: promptinput.ModelInputSource{Layer: string(layer)},
		}
	}
	items := []promptinput.ModelInputItem{
		layerItem("l0", promptinput.ProviderRoleSystem, promptcompiler.LayerAbsoluteSystemCore, "system"),
		layerItem("l1", promptinput.ProviderRoleSystem, promptcompiler.LayerRoleProfileCore, "role"),
		layerItem("l2", promptinput.ProviderRoleSystem, promptcompiler.LayerStableRuntimeContract, "contract"),
		layerItem("l3", promptinput.ProviderRoleSystem, promptcompiler.LayerTurnStableFacts, "turn facts"),
		layerItem("history-user", promptinput.ProviderRoleUser, promptcompiler.LayerConversationHistory, "inspect"),
		layerItem("history-call", promptinput.ProviderRoleAssistant, promptcompiler.LayerConversationHistory, "checking"),
		layerItem("history-result", promptinput.ProviderRoleTool, promptcompiler.LayerConversationHistory, "result"),
		layerItem("l5", promptinput.ProviderRoleSystem, promptcompiler.LayerStepDynamicContext, "dynamic"),
		layerItem("l6", promptinput.ProviderRoleDeveloper, promptcompiler.LayerCurrentUserInput, "continue"),
	}
	items[5].ToolCalls = []promptinput.ModelInputToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)}}
	items[6].ToolCallID = "call-1"
	items[6].ToolResult = &promptinput.ModelInputToolResult{ToolCallID: "call-1", Content: "result"}
	items[8].SemanticRole = "continuation_instruction"
	return items
}
