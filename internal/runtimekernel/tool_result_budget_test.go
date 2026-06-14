package runtimekernel

import (
	"strings"
	"testing"
)

func TestAggregateToolResultBudgetExternalizesLargestResults(t *testing.T) {
	results := []ToolResult{
		{
			ToolCallID: "small",
			Content:    strings.Repeat("small ", 40),
			Summary:    "small result",
		},
		{
			ToolCallID: "large-a",
			Content:    strings.Repeat("large-a ", 1800),
			Summary:    "large a result",
		},
		{
			ToolCallID: "large-b",
			Content:    strings.Repeat("large-b ", 1500),
			Summary:    "large b result",
		},
		{
			ToolCallID: "error",
			Content:    "tool failed with actionable error",
			Error:      "actionable error",
			Summary:    "error summary",
		},
	}

	applied := ApplyAggregateToolResultBudget(AggregateToolResultBudgetInput{
		SessionID:            "sess",
		TurnID:               "turn",
		Iteration:            2,
		Results:              results,
		MaxAggregateTokens:   1200,
		CurrentNonToolTokens: 300,
		Thresholds: ContextBudgetThresholds{
			MaxContextTokens:       8000,
			ReservedOutputTokens:   1000,
			EffectiveContextWindow: 7000,
		},
	})

	if !applied.Applied {
		t.Fatal("expected aggregate budget to be applied")
	}
	if applied.BeforeTokens <= applied.AfterTokens {
		t.Fatalf("expected after tokens to be lower, before=%d after=%d", applied.BeforeTokens, applied.AfterTokens)
	}
	if applied.AfterTokens > applied.MaxAggregateTokens {
		t.Fatalf("after tokens = %d, want <= %d", applied.AfterTokens, applied.MaxAggregateTokens)
	}
	if len(applied.Results) != len(results) {
		t.Fatalf("result count changed: got %d want %d", len(applied.Results), len(results))
	}

	largeA := findToolResultByID(applied.Results, "large-a")
	if largeA == nil {
		t.Fatal("large-a result missing")
	}
	if !largeA.Spilled || len(largeA.ExternalReferences) == 0 {
		t.Fatalf("largest result was not externalized: %+v", largeA)
	}
	if strings.Contains(largeA.Content, strings.Repeat("large-a ", 100)) {
		t.Fatalf("largest result still contains a long raw payload")
	}
	if !strings.Contains(largeA.Content, "External ref:") {
		t.Fatalf("largest result content should expose external ref, got %q", largeA.Content)
	}

	errResult := findToolResultByID(applied.Results, "error")
	if errResult == nil {
		t.Fatal("error result missing")
	}
	if errResult.Spilled {
		t.Fatalf("error result should be preserved, got %+v", errResult)
	}
	if !strings.Contains(errResult.Content, "actionable error") {
		t.Fatalf("error content should remain model-visible, got %q", errResult.Content)
	}

	if len(applied.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(applied.Events))
	}
	event := applied.Events[0]
	if event.Kind != "tool_result.aggregate_budget_applied" {
		t.Fatalf("event kind = %q", event.Kind)
	}
	if event.Layer != ContextGovernanceLayerL1 {
		t.Fatalf("event layer = %q", event.Layer)
	}
	if len(event.ReferenceIDs) == 0 {
		t.Fatal("event should include externalized reference ids")
	}
}

func findToolResultByID(results []ToolResult, id string) *ToolResult {
	for i := range results {
		if results[i].ToolCallID == id {
			return &results[i]
		}
	}
	return nil
}
