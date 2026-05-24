package runtimekernel

import (
	"strings"
	"testing"
)

func TestMicrocompactRetainsRecentToolResults(t *testing.T) {
	messages := []Message{
		testToolResultMessage("tr-1", "logs.search", "old logs", "ref-1"),
		testToolResultMessage("tr-2", "metrics.query", "old metrics", "ref-2"),
		testToolResultMessage("tr-3", "trace.query", "recent trace", "ref-3"),
	}
	result := MicrocompactMessages(messages, MicrocompactOptions{KeepRecentGroups: 1})
	if len(result.Messages) != 3 {
		t.Fatalf("messages = %d", len(result.Messages))
	}
	if !strings.Contains(result.Messages[0].ToolResult.Content, "Old tool result compacted") {
		t.Fatalf("old result not compacted: %q", result.Messages[0].ToolResult.Content)
	}
	if result.Messages[2].ToolResult.Content != "recent trace" {
		t.Fatalf("recent result changed: %q", result.Messages[2].ToolResult.Content)
	}
	if messages[0].ToolResult.Content != "old logs" {
		t.Fatal("microcompact mutated original message")
	}
}

func TestMicrocompactProtectsErrorsAndInlineResults(t *testing.T) {
	messages := []Message{
		{ID: "inline", Role: "tool", ToolResult: &ToolResult{ToolCallID: "inline", Content: "plain inline"}},
		{ID: "err", Role: "tool", ToolResult: &ToolResult{ToolCallID: "err", Content: "failure", Error: "boom", Spilled: true}},
		testToolResultMessage("tr-1", "logs.search", "old logs", "ref-1"),
		testToolResultMessage("tr-2", "logs.search", "new logs", "ref-2"),
	}
	result := MicrocompactMessages(messages, MicrocompactOptions{KeepRecentGroups: 1})
	if result.Messages[0].ToolResult.Content != "plain inline" {
		t.Fatalf("inline result changed: %q", result.Messages[0].ToolResult.Content)
	}
	if result.Messages[1].ToolResult.Content != "failure" {
		t.Fatalf("error result changed: %q", result.Messages[1].ToolResult.Content)
	}
	if !strings.Contains(result.Messages[2].ToolResult.Content, "Old tool result compacted") {
		t.Fatalf("compactable result was not compacted: %q", result.Messages[2].ToolResult.Content)
	}
}

func TestMicrocompactSmallContextKeepsTwoGroupsByDefault(t *testing.T) {
	messages := []Message{
		testToolResultMessage("tr-1", "logs.search", "one", "ref-1"),
		testToolResultMessage("tr-2", "logs.search", "two", "ref-2"),
		testToolResultMessage("tr-3", "logs.search", "three", "ref-3"),
	}
	result := MicrocompactMessages(messages, MicrocompactOptions{SmallContextMode: true})
	if !strings.Contains(result.Messages[0].ToolResult.Content, "Old tool result compacted") {
		t.Fatalf("oldest should be compacted: %q", result.Messages[0].ToolResult.Content)
	}
	if result.Messages[1].ToolResult.Content != "two" || result.Messages[2].ToolResult.Content != "three" {
		t.Fatalf("small-context recent groups changed: %#v", result.Messages)
	}
}

func TestMicrocompactProducesTraceEvent(t *testing.T) {
	messages := []Message{
		testToolResultMessage("tr-1", "logs.search", "one", "ref-1"),
		testToolResultMessage("tr-2", "logs.search", "two", "ref-2"),
	}
	result := MicrocompactMessages(messages, MicrocompactOptions{
		SessionID:        "s1",
		TurnID:           "t1",
		Iteration:        2,
		KeepRecentGroups: 1,
	})
	if len(result.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(result.Events))
	}
	event := result.Events[0]
	if event.Layer != ContextGovernanceLayerL3 || event.Kind != "tool_result.microcompacted" {
		t.Fatalf("event = %#v", event)
	}
	if len(event.ReferenceIDs) != 1 || event.ReferenceIDs[0] != "ref-1" {
		t.Fatalf("reference ids = %#v", event.ReferenceIDs)
	}
}

func testToolResultMessage(id, toolName, content, refID string) Message {
	return Message{
		ID:   id,
		Role: "tool",
		ToolResult: &ToolResult{
			ToolCallID: id,
			Content:    content,
			Summary:    toolName + " summary",
			Spilled:    true,
			ExternalReferences: []ExternalReference{{
				ID:        refID,
				SessionID: "s1",
				TurnID:    "t1",
			}},
		},
	}
}
