package runtimekernel

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestContextGovernanceEventRoundTrip(t *testing.T) {
	event := ContextGovernanceEvent{
		ID:        "ctxgov-1",
		Layer:     ContextGovernanceLayerL4,
		Kind:      "context.compaction.started",
		SessionID: "s1",
		TurnID:    "t1",
		Iteration: 2,
		Budget: ContextBudgetThresholds{
			MaxContextTokens:       200000,
			ReservedOutputTokens:   20000,
			EffectiveContextWindow: 180000,
			WarningThreshold:       147000,
			AutoCompactThreshold:   167000,
			BlockingLimit:          177000,
		},
		Message: "正在压缩上下文，当前任务会继续",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var restored ContextGovernanceEvent
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Kind != event.Kind || restored.Budget.AutoCompactThreshold != 167000 {
		t.Fatalf("restored event = %#v", restored)
	}
}

func TestContextGovernanceTracePayloadIsRedactionSafe(t *testing.T) {
	event := ContextGovernanceEvent{
		ID:                  "ctxgov-2",
		Layer:               ContextGovernanceLayerL1,
		Kind:                "tool_result.materialized",
		ToolCallID:          "call-logs-1",
		ToolName:            "logs.search",
		MaterializationTier: "large",
		OriginalBytes:       49152,
		InlineBytes:         512,
		ReferenceIDs:        []string{"ref-1"},
		Message:             "materialized large result",
	}
	data, err := json.Marshal(event.TracePayload())
	if err != nil {
		t.Fatal(err)
	}
	payload := string(data)
	for _, secret := range []string{"password=", "stack trace line", "raw tool result"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("trace payload leaked %q: %s", secret, payload)
		}
	}
	for _, want := range []string{"ref-1", "call-logs-1", "logs.search", `"materializationTier":"large"`, `"originalBytes":49152`, `"inlineBytes":512`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("trace payload missing %q: %s", want, payload)
		}
	}
}

func TestSortContextGovernanceEvents(t *testing.T) {
	base := time.Unix(100, 0).UTC()
	events := []ContextGovernanceEvent{
		{ID: "b", CreatedAt: base.Add(time.Second), Kind: "second", Layer: ContextGovernanceLayerL4},
		{ID: "a", CreatedAt: base, Kind: "first", Layer: ContextGovernanceLayerL3},
		{ID: "c", CreatedAt: base.Add(time.Second), Kind: "third", Layer: ContextGovernanceLayerL5},
	}
	sorted := SortContextGovernanceEvents(events)
	got := []string{sorted[0].ID, sorted[1].ID, sorted[2].ID}
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("sorted ids = %#v", got)
	}
	if events[0].ID != "b" {
		t.Fatal("sort mutated input")
	}
}
