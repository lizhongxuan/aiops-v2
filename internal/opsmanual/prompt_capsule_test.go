package opsmanual

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOpsManualPromptCapsuleBudgetsContext(t *testing.T) {
	now := time.Now().UTC()
	facts := make([]SessionOpsFact, 0, 100)
	for i := 0; i < 100; i++ {
		facts = append(facts, SessionOpsFact{
			Key:             fmt.Sprintf("target_fact_%02d", i),
			Value:           fmt.Sprintf("value-%02d", i),
			Source:          "session_fact",
			Confidence:      0.9,
			ConfirmedByUser: true,
			ExpiresAt:       now.Add(time.Hour),
			UpdatedAt:       now.Add(time.Duration(i) * time.Second),
		})
	}
	hints := make([]OpsManualPromptHint, 0, 20)
	for i := 0; i < 20; i++ {
		hints = append(hints, OpsManualPromptHint{ID: fmt.Sprintf("hint-%02d", i), Text: fmt.Sprintf("hint text %02d", i), Score: float64(i)})
	}

	capsule := BuildOpsManualPromptCapsule(OpsManualPromptCapsuleInput{
		FlowID:         "flow-redis",
		CurrentTarget:  "redis redis-01",
		SelectedManual: "manual-redis-rca",
		Decision:       "need_info",
		Missing:        []string{"target_instance"},
		BlockedBy:      []string{"missing target"},
		SessionFacts:   facts,
		LettaHints:     hints,
		DiscoveryRefs: []OpsManualPromptRef{
			{ID: "artifact-1", Kind: "discovery", Summary: "redis container found", Raw: strings.Repeat("raw discovery output ", 20)},
		},
		PreviousCapsule: strings.Repeat("previous capsule full text ", 20),
		MaxChars:        1000,
	})
	rendered := capsule.Markdown()

	if len(capsule.ConfirmedFacts) > 8 {
		t.Fatalf("confirmed facts = %d, want <= 8", len(capsule.ConfirmedFacts))
	}
	if len(capsule.LettaHints) > 3 {
		t.Fatalf("letta hints = %d, want <= 3", len(capsule.LettaHints))
	}
	if !strings.Contains(rendered, "artifact-1") || !strings.Contains(rendered, "redis container found") {
		t.Fatalf("capsule missing discovery summary/ref:\n%s", rendered)
	}
	if strings.Contains(rendered, "raw discovery output") {
		t.Fatalf("capsule leaked raw discovery output:\n%s", rendered)
	}
	if strings.Contains(rendered, "previous capsule full text") {
		t.Fatalf("capsule included previous capsule full text:\n%s", rendered)
	}
	if !hasAny(capsule.DroppedContextReasons, "session_fact_limit") ||
		!hasAny(capsule.DroppedContextReasons, "letta_hint_limit") ||
		!hasAny(capsule.DroppedContextReasons, "artifact_ref_only") ||
		!hasAny(capsule.DroppedContextReasons, "previous_capsule_omitted") {
		t.Fatalf("dropped reasons = %#v", capsule.DroppedContextReasons)
	}
}

func TestOpsManualPromptCapsuleCompactsWhenOverBudget(t *testing.T) {
	capsule := BuildOpsManualPromptCapsule(OpsManualPromptCapsuleInput{
		FlowID:         "flow-budget",
		CurrentTarget:  "redis redis-01",
		SelectedManual: "manual-redis-rca",
		Decision:       "need_info",
		Missing:        []string{"target_instance"},
		BlockedBy:      []string{"missing target"},
		SessionFacts: []SessionOpsFact{
			{Key: "target_host", Value: strings.Repeat("server-local ", 50), ConfirmedByUser: true, Confidence: 1},
		},
		LettaHints: []OpsManualPromptHint{{ID: "hint-1", Text: strings.Repeat("hint ", 80), Score: 1}},
		MaxChars:   160,
	})
	rendered := capsule.Markdown()

	for _, want := range []string{"flow-budget", "redis redis-01", "manual-redis-rca", "need_info", "target_instance", "missing target"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compacted capsule missing %q:\n%s", want, rendered)
		}
	}
	if len(capsule.ConfirmedFacts) != 0 || len(capsule.LettaHints) != 0 || !hasAny(capsule.DroppedContextReasons, "budget_compacted") {
		t.Fatalf("capsule = %#v, want only base fields after compaction", capsule)
	}
}
