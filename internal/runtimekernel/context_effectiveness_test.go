package runtimekernel

import (
	"context"
	"strings"
	"testing"
)

func TestContextRuntimeEffectivenessAggregateBudgetKeepsPayloadBounded(t *testing.T) {
	results := make([]ToolResult, 0, 20)
	for i := 0; i < 20; i++ {
		results = append(results, ToolResult{
			ToolCallID: "call-" + string(rune('a'+i)),
			Content:    strings.Repeat("bounded generic payload ", 240),
			Summary:    "bounded generic summary",
		})
	}
	applied := ApplyAggregateToolResultBudget(AggregateToolResultBudgetInput{
		SessionID:          "session-generic",
		TurnID:             "turn-generic",
		Iteration:          1,
		Results:            results,
		MaxAggregateTokens: 2500,
		Thresholds:         DefaultContextBudgetPolicy(32000, 8000).Thresholds(),
	})
	if !applied.Applied {
		t.Fatal("expected aggregate budget to apply")
	}
	if applied.AfterTokens > applied.MaxAggregateTokens {
		t.Fatalf("after tokens = %d, want <= %d", applied.AfterTokens, applied.MaxAggregateTokens)
	}
	if len(applied.ReferenceIDs) == 0 {
		t.Fatal("expected externalized references")
	}
}

func TestContextRuntimeEffectivenessArtifactRangeReadAvoidsFullPayload(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	content := strings.Repeat("prefix ", 100) + "target-window" + strings.Repeat(" suffix", 100)
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		Kind:        "tool_result",
		ContentType: "text/plain",
		Content:     []byte(content),
		Summary:     "generic large artifact",
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}
	reader := NewContextArtifactReader(ContextArtifactReaderOptions{Repository: store, MaxReadBytes: 64})
	result, err := reader.Read(ContextArtifactReadRequest{ID: artifact.ID, Query: "target-window", Limit: 32})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(result.Matches) != 1 || !strings.Contains(result.Matches[0].Content, "target-window") {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if strings.Contains(result.Matches[0].Content, strings.Repeat("prefix ", 20)) {
		t.Fatalf("range read returned too much surrounding payload: %q", result.Matches[0].Content)
	}
}

func TestContextRuntimeEffectivenessResourceDedupeSuppressesRepeatedContent(t *testing.T) {
	state := NewObservationState()
	record := ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:     "store://artifacts/generic-resource",
			Version: "snapshot-1",
			Digest:  "sha256:generic",
			Range:   ResourceRange{Offset: 0, Limit: 100},
		},
		SourceRef: "artifact-generic",
		Summary:   "generic resource summary",
		Content:   strings.Repeat("generic repeated content ", 20),
	}
	first := state.CheckResource(record)
	second := state.CheckResource(record)
	if !first.Miss {
		t.Fatalf("first read = %#v, want miss", first)
	}
	if !second.Unchanged {
		t.Fatalf("second read = %#v, want unchanged", second)
	}
	if strings.Contains(second.ModelVisibleContent, "generic repeated content generic repeated content") {
		t.Fatalf("unchanged stub repeated full content: %q", second.ModelVisibleContent)
	}
}

func TestContextRuntimeEffectivenessCompactSummaryPreventsDrift(t *testing.T) {
	summary := CompactSummaryV1{
		SchemaVersion:      CompactSummarySchemaVersionV1,
		UserGoal:           "Continue the latest requested generic task.",
		LatestUserMessages: []CompactSummaryMessageRefV1{{TurnID: "turn-latest", Quote: "Use the newest constraint."}},
		CurrentTask:        CompactSummaryCurrentTaskV1{Description: "Keep working from the newest constraint."},
		NextStep: CompactSummaryNextStepV1{
			Action:          "Continue from the newest constraint.",
			SourceTurnID:    "turn-latest",
			RecentUserQuote: "Use the newest constraint.",
		},
	}
	if err := summary.Validate(); err != nil {
		t.Fatalf("valid summary rejected: %v", err)
	}
	summary.NextStep.RecentUserQuote = ""
	if err := summary.Validate(); err == nil {
		t.Fatal("summary without recentUserQuote should be rejected")
	}
}

func TestContextRuntimeEffectivenessSimpleInputDoesNotForceCompaction(t *testing.T) {
	cw := &ContextWindow{MaxTokens: 32000}
	messages := []Message{{ID: "user-simple", Role: "user", Content: "short generic question"}}
	result, err := ApplyContextPipeline(nilContext(), cw, messages, ContextPipelineOptions{
		SessionID:    "session-simple",
		TurnID:       "turn-simple",
		Iteration:    1,
		BudgetPolicy: DefaultContextBudgetPolicy(32000, 8000),
	})
	if err != nil {
		t.Fatalf("ApplyContextPipeline failed: %v", err)
	}
	if len(result.CompactedSegments) != 0 {
		t.Fatalf("simple input compacted unexpectedly: %#v", result.CompactedSegments)
	}
	if len(result.Messages) != 1 || result.Messages[0].Content != "short generic question" {
		t.Fatalf("messages = %#v", result.Messages)
	}
}

func nilContext() context.Context {
	return context.Background()
}
