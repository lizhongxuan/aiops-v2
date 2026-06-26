package promptinput

import (
	"encoding/json"
	"testing"
)

func TestModelInputItemValidateRequiresProviderRole(t *testing.T) {
	item := ModelInputItem{ID: "item-1", Content: "hello"}
	if err := item.Validate(); err == nil {
		t.Fatal("Validate() succeeded, want provider role error")
	}
}

func TestModelInputItemValidateRequiresToolCallIDForToolResult(t *testing.T) {
	item := ModelInputItem{
		ID:           "tool-result",
		ProviderRole: ProviderRoleTool,
		ToolResult:   &ModelInputToolResult{Content: "ok"},
	}
	if err := item.Validate(); err == nil {
		t.Fatal("Validate() succeeded, want missing tool call id error")
	}
}

func TestModelInputItemHashIsStableForMetadataOrder(t *testing.T) {
	a := ModelInputItem{
		ID:           "item-1",
		ProviderRole: ProviderRoleUser,
		Content:      "hello",
		Metadata:     map[string]string{"b": "2", "a": "1"},
	}
	b := ModelInputItem{
		ID:           "item-1",
		ProviderRole: ProviderRoleUser,
		Content:      "hello",
		Metadata:     map[string]string{"a": "1", "b": "2"},
	}
	if a.StableHash() != b.StableHash() {
		t.Fatalf("hash differs: %s != %s", a.StableHash(), b.StableHash())
	}
}

func TestModelInputToolCallArgumentsMustBeJSON(t *testing.T) {
	item := ModelInputItem{
		ID:           "assistant-tool-call",
		ProviderRole: ProviderRoleAssistant,
		ToolCalls: []ModelInputToolCall{{
			ID:        "call-1",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"cmd":"date"}`),
		}},
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
