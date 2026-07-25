package runtimekernel

import (
	"encoding/json"
	"testing"
)

func TestPromptInputMakesTypedToolResultVisibleToModel(t *testing.T) {
	typed := json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","structuredContent":{"kind":"preview","changeSet":{"id":"change-1","previewHash":"sha256:preview"}}}`)
	messages := promptInputMessagesFromRuntime([]Message{{
		Role:    "tool",
		Content: "已生成候选预览。",
		ToolResult: &ToolResult{
			ToolCallID:        "call-preview",
			Content:           "已生成候选预览。",
			StructuredContent: typed,
		},
	}})
	if len(messages) != 1 || messages[0].ToolResult == nil {
		t.Fatalf("prompt messages = %#v, want one typed tool result", messages)
	}
	var visible struct {
		SchemaVersion     string          `json:"schemaVersion"`
		Summary           string          `json:"summary"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal([]byte(messages[0].ToolResult.Content), &visible); err != nil {
		t.Fatalf("model-visible tool result is not JSON: %v\n%s", err, messages[0].ToolResult.Content)
	}
	if visible.SchemaVersion != "aiops.model_tool_result.v1" || visible.Summary != "已生成候选预览。" || visible.IsError {
		t.Fatalf("model-visible envelope = %#v", visible)
	}
	var gotStructured, wantStructured any
	if err := json.Unmarshal(visible.StructuredContent, &gotStructured); err != nil {
		t.Fatalf("decode model-visible structuredContent: %v", err)
	}
	if err := json.Unmarshal(typed, &wantStructured); err != nil {
		t.Fatalf("decode expected structuredContent: %v", err)
	}
	gotJSON, _ := json.Marshal(gotStructured)
	wantJSON, _ := json.Marshal(wantStructured)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("structuredContent = %s, want %s", gotJSON, wantJSON)
	}
	if messages[0].Content != "已生成候选预览。" {
		t.Fatalf("page-safe summary changed to %q", messages[0].Content)
	}
}

func TestPromptInputFallsBackToSummaryForInvalidStructuredToolResult(t *testing.T) {
	messages := promptInputMessagesFromRuntime([]Message{{
		Role:    "tool",
		Content: "可读摘要",
		ToolResult: &ToolResult{
			ToolCallID:        "call-invalid",
			Content:           "可读摘要",
			StructuredContent: json.RawMessage(`{`),
		},
	}})
	if got := messages[0].ToolResult.Content; got != "可读摘要" {
		t.Fatalf("invalid structured result content = %q, want readable summary", got)
	}
}
