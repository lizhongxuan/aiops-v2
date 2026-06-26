package runtimekernel

import (
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modelrouter"
)

func TestAppendProviderNativeWebSearchTurnItemsCreatesSearchLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	snapshot := &TurnSnapshot{
		ID:        "turn-1",
		SessionID: "session-1",
		StartedAt: now,
		UpdatedAt: now,
	}
	iteration := &IterationState{Iteration: 1}
	events := []modelrouter.ProviderNativeWebSearchEvent{{
		ID:       "ws_123",
		Provider: "zhipu",
		Query:    "OpenAI web_search docs",
		Sources: []modelrouter.ProviderNativeWebSearchSource{{
			Title:   "Web search guide",
			URL:     "https://platform.openai.com/docs/guides/tools-web-search",
			Snippet: "Use web_search as a hosted tool.",
		}},
	}}

	appendProviderNativeWebSearchTurnItems(snapshot, iteration, "turn-1", events)

	if len(iteration.ToolCalls) != 1 || iteration.ToolCalls[0].Name != "web_search" {
		t.Fatalf("iteration tool calls = %#v, want synthetic web_search", iteration.ToolCalls)
	}
	if len(iteration.ToolResults) != 1 || iteration.ToolResults[0].ToolCallID != iteration.ToolCalls[0].ID {
		t.Fatalf("iteration tool results = %#v, want matching synthetic result", iteration.ToolResults)
	}
	if len(snapshot.AgentItems) != 2 {
		t.Fatalf("agent items = %#v, want tool_call and tool_result", snapshot.AgentItems)
	}
	if snapshot.AgentItems[0].Type != agentstate.TurnItemTypeToolCall || snapshot.AgentItems[0].Status != agentstate.ItemStatusCompleted {
		t.Fatalf("tool call item = %#v, want completed tool_call", snapshot.AgentItems[0])
	}
	if snapshot.AgentItems[1].Type != agentstate.TurnItemTypeToolResult || snapshot.AgentItems[1].Status != agentstate.ItemStatusCompleted {
		t.Fatalf("tool result item = %#v, want completed tool_result", snapshot.AgentItems[1])
	}
	if snapshot.AgentItems[0].Payload.Kind != "browser.search" || snapshot.AgentItems[1].Payload.Kind != "browser.search" {
		t.Fatalf("payload kinds = %q/%q, want browser.search", snapshot.AgentItems[0].Payload.Kind, snapshot.AgentItems[1].Payload.Kind)
	}
}
