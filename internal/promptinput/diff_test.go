package promptinput

import (
	"strings"
	"testing"
)

func TestDiffTraceReportsAddedAndRemovedSemanticItems(t *testing.T) {
	prev := PromptInputTrace{Items: []TraceItem{
		{Source: "conversation", SemanticRole: "user", Content: "old"},
		{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "pending"},
	}}
	next := PromptInputTrace{Items: []TraceItem{
		{Source: "conversation", SemanticRole: "user", Content: "old"},
		{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "completed"},
		{Source: "conversation", SemanticRole: "tool_result", ID: "call-1", Content: "ok"},
	}}

	diff := DiffTrace(prev, next)
	if len(diff.Added) != 2 {
		t.Fatalf("added = %#v, want completed plan and tool result", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Status != "pending" {
		t.Fatalf("removed = %#v, want pending plan", diff.Removed)
	}
}

func TestRenderDiffMarkdownRedactsSecretsAndShowsSemanticDeltas(t *testing.T) {
	prev := PromptInputTrace{Items: []TraceItem{
		{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "pending", Content: "inspect"},
	}}
	next := PromptInputTrace{Items: []TraceItem{
		{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "completed", Content: "inspect"},
		{Source: "conversation", SemanticRole: "tool_result", ID: "call-1", Content: "token=super-secret-value"},
	}}

	markdown := RenderDiffMarkdown(DiffTrace(prev, next))
	for _, want := range []string{"# Prompt Input Diff", "tool_result", "plan", "completed", "[REDACTED]"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("diff markdown missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "super-secret-value") {
		t.Fatalf("diff markdown leaked raw secret:\n%s", markdown)
	}
}
