package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSplitContextForCompactionPreservesToolCausalGroup(t *testing.T) {
	messages := causalCompactionMessages()
	cw := &ContextWindow{MaxTokens: 10}
	plan := SplitContextForCompaction(cw, messages)
	assertCausalGroupAtomic(t, plan.Compactable, plan.Retained, []string{"assistant-tools", "tool-result-a", "tool-result-b"})
	if plan.TrimmedCount != len(plan.Compactable) || cw.TruncatedAt != plan.TrimmedCount {
		t.Fatalf("compaction accounting = plan:%d compactable:%d window:%d", plan.TrimmedCount, len(plan.Compactable), cw.TruncatedAt)
	}
}

func TestContextCompactionPreservesToolCausalGroupsAcrossBudgets(t *testing.T) {
	messages := causalCompactionMessages()
	group := []string{"assistant-tools", "tool-result-a", "tool-result-b"}
	for maxTokens := 4; maxTokens <= 48; maxTokens++ {
		t.Run(fmt.Sprintf("budget_%d", maxTokens), func(t *testing.T) {
			cw := &ContextWindow{MaxTokens: maxTokens}
			result, err := ApplyContextPipeline(context.Background(), cw, messages, ContextPipelineOptions{
				SessionID: "sess-causal", TurnID: "turn-causal", Iteration: 2,
				BudgetPolicy: DefaultContextBudgetPolicy(20000, 8000),
			})
			if err != nil {
				t.Fatalf("ApplyContextPipeline() error = %v", err)
			}
			assertResultCausalGroupAtomic(t, result, group, 1, 3)
			assertCompactionCoverage(t, messages, result)
		})
	}
}

func TestContextCompactionBudgetFallbackDoesNotLoseRetainedMessages(t *testing.T) {
	messages := make([]Message, 0, 8)
	for i := 0; i < 8; i++ {
		messages = append(messages, Message{ID: fmt.Sprintf("msg-%d", i), Role: "user", Content: strings.Repeat(string(rune('a'+i)), 16)})
	}
	result, err := ApplyContextPipeline(context.Background(), &ContextWindow{MaxTokens: 20}, messages, ContextPipelineOptions{
		SessionID: "sess-coverage", TurnID: "turn-coverage", Iteration: 1,
		BudgetPolicy: DefaultContextBudgetPolicy(20000, 8000),
	})
	if err != nil {
		t.Fatalf("ApplyContextPipeline() error = %v", err)
	}
	assertCompactionCoverage(t, messages, result)
}

func causalCompactionMessages() []Message {
	return []Message{
		{ID: "old-user", Role: "user", Content: strings.Repeat("old ", 10)},
		{ID: "assistant-tools", Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call-a", Name: "read_a", Arguments: json.RawMessage(`{}`)},
			{ID: "call-b", Name: "read_b", Arguments: json.RawMessage(`{}`)},
		}},
		{ID: "tool-result-a", Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-a", Content: strings.Repeat("a", 20)}},
		{ID: "tool-result-b", Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-b", Content: strings.Repeat("b", 20)}},
		{ID: "tail-user", Role: "user", Content: strings.Repeat("tail ", 4)},
	}
}

func assertCausalGroupAtomic(t *testing.T, compactable, retained []Message, group []string) {
	t.Helper()
	compactIDs := messageIDSet(compactable)
	retainedIDs := messageIDSet(retained)
	compactCount := 0
	retainedCount := 0
	for _, id := range group {
		if compactIDs[id] {
			compactCount++
		}
		if retainedIDs[id] {
			retainedCount++
		}
	}
	if compactCount != 0 && compactCount != len(group) {
		t.Fatalf("causal group partially compacted: compact=%v retained=%v", compactIDs, retainedIDs)
	}
	if retainedCount != 0 && retainedCount != len(group) {
		t.Fatalf("causal group partially retained: compact=%v retained=%v", compactIDs, retainedIDs)
	}
	if compactCount+retainedCount != len(group) {
		t.Fatalf("causal group lost or duplicated: compact=%v retained=%v", compactIDs, retainedIDs)
	}
}

func assertResultCausalGroupAtomic(t *testing.T, result ContextPipelineResult, group []string, groupStart, groupEnd int) {
	t.Helper()
	compactedThrough := -1
	if len(result.CompactedSegments) > 0 {
		compactedThrough = result.CompactedSegments[0].EndIndex
	}
	if compactedThrough >= groupStart && compactedThrough < groupEnd {
		t.Fatalf("compaction boundary %d split causal group [%d,%d]", compactedThrough, groupStart, groupEnd)
	}
	retained := messageIDSet(result.Messages)
	if compactedThrough < groupStart {
		for _, id := range group {
			if !retained[id] {
				t.Fatalf("uncompacted causal message %q missing from retained set %v", id, retained)
			}
		}
	}
}

func assertCompactionCoverage(t *testing.T, original []Message, result ContextPipelineResult) {
	t.Helper()
	retained := messageIDSet(result.Messages)
	compactedThrough := -1
	if len(result.CompactedSegments) > 0 {
		compactedThrough = result.CompactedSegments[0].EndIndex
	}
	for index, message := range original {
		if index <= compactedThrough {
			continue
		}
		if !retained[message.ID] {
			t.Fatalf("message %q at index %d was neither compacted nor retained; segment end=%d retained=%v", message.ID, index, compactedThrough, retained)
		}
	}
}

func messageIDSet(messages []Message) map[string]bool {
	out := make(map[string]bool, len(messages))
	for _, message := range messages {
		if message.ID != "" {
			out[message.ID] = true
		}
	}
	return out
}
