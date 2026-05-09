package runtimekernel

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func TestRuntimeModelInputCompatibilityAdapters(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "old"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "old-call", Name: "read_file"}}},
		{Role: "tool", Content: "old result", ToolResult: &ToolResult{ToolCallID: "old-call", Content: "old result"}},
		{Role: "assistant", Content: "stable answer"},
		{Role: "user", Content: "current"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "current-call", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}}},
		{Role: "tool", Content: "current result", ToolResult: &ToolResult{ToolCallID: "current-call", Content: "current result"}},
	}

	filtered := messagesForCurrentTurnModelInput(history)
	if len(filtered) != 5 {
		t.Fatalf("filtered len = %d, want stable old answer plus current turn", len(filtered))
	}
	joined := strings.Builder{}
	for _, msg := range filtered {
		joined.WriteString(msg.Content)
	}
	if strings.Contains(joined.String(), "old result") || !strings.Contains(joined.String(), "stable answer") {
		t.Fatalf("filtered messages = %#v, want prior tool noise dropped and stable answer kept", filtered)
	}

	roundTrip := runtimeMessagesFromPromptInput([]promptinput.Message{{
		Role:      "assistant",
		ToolCalls: []promptinput.ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}},
	}, {
		Role:       "tool",
		Content:    "ok",
		ToolResult: &promptinput.ToolResult{ToolCallID: "call-1", Content: "ok"},
	}})
	if len(roundTrip) != 2 || roundTrip[0].ToolCalls[0].Name != "read_file" || roundTrip[1].ToolResult.ToolCallID != "call-1" {
		t.Fatalf("runtimeMessagesFromPromptInput() = %#v, want tool call/result preserved", roundTrip)
	}
}

func TestLegacyRuntimeSchemaAdaptersPreserveRolesAndToolCalls(t *testing.T) {
	messages, err := runtimeMessagesToSchema([]Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "user"},
		{Role: "assistant", Content: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}}},
		{Role: "tool", Content: "result", ToolResult: &ToolResult{ToolCallID: "call-1"}},
	})
	if err != nil {
		t.Fatalf("runtimeMessagesToSchema() error = %v", err)
	}
	if len(messages) != 4 || messages[0].Role != schema.System || messages[2].ToolCalls[0].Function.Name != "read_file" || messages[3].ToolCallID != "call-1" {
		t.Fatalf("schema messages = %#v, want roles and tool call metadata preserved", messages)
	}
	if _, err := runtimeMessagesToSchema([]Message{{Role: "unknown"}}); err == nil {
		t.Fatal("expected unsupported role to fail")
	}
}

func TestToolLifecyclePayloadBudgetBoundaries(t *testing.T) {
	summary, resultForEvent, preview, rawRef, bytes, truncated := summarizeToolLifecycleResultForEvent("turn-1", "call-1", " first line \nsecond line")
	if summary != "first line" || resultForEvent != "first line \nsecond line" || len(preview) == 0 || rawRef != "" || bytes == 0 || truncated {
		t.Fatalf("small result summary=%q result=%q preview=%s rawRef=%q bytes=%d truncated=%v", summary, resultForEvent, preview, rawRef, bytes, truncated)
	}

	medium := strings.Repeat("x", inlineToolLifecycleResultBytes+10)
	_, resultForEvent, preview, rawRef, _, truncated = summarizeToolLifecycleResultForEvent("turn-1", "call-1", medium)
	if resultForEvent == medium || len(preview) == 0 || rawRef == "" || !truncated {
		t.Fatalf("medium result not summarized correctly: resultLen=%d preview=%d rawRef=%q truncated=%v", len(resultForEvent), len(preview), rawRef, truncated)
	}

	huge := strings.Repeat("x", maxToolLifecyclePayloadBytes+10)
	_, _, preview, rawRef, _, truncated = summarizeToolLifecycleResultForEvent("turn-1", "call-1", huge)
	if len(preview) == 0 || rawRef == "" || !truncated {
		t.Fatalf("huge result preview=%d rawRef=%q truncated=%v, want preview with raw ref", len(preview), rawRef, truncated)
	}
	var previewText string
	if err := json.Unmarshal(preview, &previewText); err != nil {
		t.Fatalf("huge preview decode error = %v", err)
	}
	if len([]byte(previewText)) > inlineToolLifecycleResultBytes+len("...") {
		t.Fatalf("huge preview len = %d, want bounded preview", len([]byte(previewText)))
	}
	if got := rawToolLifecycleRef("", "call-1"); got != "" {
		t.Fatalf("rawToolLifecycleRef with missing turn = %q, want empty", got)
	}
	if got := truncateToolLifecycleBytes("你好世界", len("你好")+1); !strings.HasSuffix(got, "...") {
		t.Fatalf("truncateToolLifecycleBytes unicode result = %q, want ellipsis", got)
	}
}

func TestRunnerCallbackEmitsLifecycleEvents(t *testing.T) {
	emitter := &testMockEventEmitter{}
	cb := NewRunnerCallback("sess-1", "turn-1", emitter)
	cb.OnToolStart("read_file", json.RawMessage(`{"path":"x"}`))
	cb.OnToolComplete("read_file", strings.Repeat("x", inlineToolLifecycleResultBytes+1))
	cb.OnToolFailed("read_file", errors.New("boom"))

	if len(emitter.events) != 3 {
		t.Fatalf("events len = %d, want 3", len(emitter.events))
	}
	if emitter.events[0].Type != EventToolStarted || emitter.events[1].Type != EventToolCompleted || emitter.events[2].Type != EventToolFailed {
		t.Fatalf("events = %#v, want started/completed/failed", emitter.events)
	}
	if !strings.Contains(string(emitter.events[1].Payload), "rawRef") {
		t.Fatalf("completed payload missing rawRef: %s", emitter.events[1].Payload)
	}
}

func TestRecoveryHelpersReturnStructuredErrors(t *testing.T) {
	if msg := RecoverToolExec("panic_tool", func() error { panic("boom") }); !strings.Contains(msg, "panic_tool") || !strings.Contains(msg, "boom") {
		t.Fatalf("RecoverToolExec panic msg = %q", msg)
	}
	if msg := RecoverToolExec("error_tool", func() error { return errors.New("bad input") }); msg != "bad input" {
		t.Fatalf("RecoverToolExec error msg = %q, want bad input", msg)
	}
	if err := SafeExecute(func() error { panic("safe boom") }); err == nil || !strings.Contains(err.Error(), "safe boom") {
		t.Fatalf("SafeExecute panic err = %v, want recovered panic", err)
	}
	if err := SafeExecute(func() error { return errors.New("plain error") }); err == nil || err.Error() != "plain error" {
		t.Fatalf("SafeExecute error = %v, want plain error", err)
	}
}

func TestMiscRuntimeHelpers(t *testing.T) {
	if got := spillContentBytes(&tooling.ResultSpill{Bytes: 12}, "fallback"); got != 12 {
		t.Fatalf("spillContentBytes bytes = %d, want 12", got)
	}
	if got := spillContentBytes(&tooling.ResultSpill{Content: []byte("abc")}, "fallback"); got != 3 {
		t.Fatalf("spillContentBytes content = %d, want 3", got)
	}
	if got := spillContentBytes(nil, "fallback"); got != len("fallback") {
		t.Fatalf("spillContentBytes nil = %d, want fallback len", got)
	}
	if got := externalReferenceLabel(ExternalReference{CardRef: "card-1"}); got != "card-1" {
		t.Fatalf("externalReferenceLabel card = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{FilePath: "/tmp/file"}); got != "/tmp/file" {
		t.Fatalf("externalReferenceLabel file = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{ID: "ref-1"}); got != "ref-1" {
		t.Fatalf("externalReferenceLabel id = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{}); got != "external-reference" {
		t.Fatalf("externalReferenceLabel fallback = %q", got)
	}
	if got := firstNonEmpty("", "  ", "value"); got != "value" {
		t.Fatalf("firstNonEmpty = %q, want value", got)
	}
}

func TestReasoningSummaryKeyAndItemIDFallbacks(t *testing.T) {
	event := modelrouter.ReasoningStreamEvent{ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", SummaryIndex: 2}
	if got := reasoningSummaryKey(event); got != "item-1:2" {
		t.Fatalf("reasoningSummaryKey = %q, want item-1:2", got)
	}
	if got := reasoningItemID(event); got != "item-1" {
		t.Fatalf("reasoningItemID = %q, want item-1", got)
	}
	if got := reasoningItemID(modelrouter.ReasoningStreamEvent{ThreadID: "thread-1", TurnID: "turn-1", SummaryIndex: 3}); got != "turn-1:reasoning:3" {
		t.Fatalf("reasoningItemID fallback = %q", got)
	}
}
