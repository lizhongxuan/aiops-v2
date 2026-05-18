package diagnostics

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTraceEmptyInputReturnsEmptyTrace(t *testing.T) {
	if got := BuildTrace(TraceBuildInput{}); !reflect.DeepEqual(got, DiagnosticTrace{}) {
		t.Fatalf("BuildTrace(empty) = %#v, want empty trace", got)
	}
}

func TestBuildTraceFromEnvironmentContext(t *testing.T) {
	input := TraceBuildInput{
		TurnID:       "turn-7",
		CurrentScope: DiagnosticScope{Hash: "scope-new", Summary: "checkout api redis latency", Confirmed: true},
		Facts: []DiagnosticFact{
			{ScopeHash: "scope-new", Summary: "redis p99 latency > 2s", Status: EvidenceStatusActive, DirectSupport: true},
			{ScopeHash: "scope-new", Summary: "slowlog blocked by policy", Status: EvidenceStatusBlocked, Critical: true},
			{ScopeHash: "scope-new", Summary: "cpu sample is stale", Status: EvidenceStatusStale, Critical: true},
			{ScopeHash: "scope-new", Summary: "replica lag missing", Status: EvidenceStatusMissing, Critical: true},
			{ScopeHash: "scope-old", Summary: "old root cause should not leak", Status: EvidenceStatusActive, DirectSupport: true},
		},
		Hypotheses:       []ScopedText{{ScopeHash: "scope-new", Text: "redis command queue saturation"}, {ScopeHash: "scope-old", Text: "old root cause"}},
		RefutingEvidence: []ScopedText{{ScopeHash: "scope-new", Text: "database connection pool normal"}},
		ToolFailures: []ToolFailure{{
			ToolName:  "redis-cli",
			Semantic:  ToolFailurePolicyBlocked,
			Detail:    "policy blocked redis://:secret@127.0.0.1:6379/0 slowlog",
			Critical:  true,
			ScopeHash: "scope-new",
		}},
		ManualBindingID: "manual-checkout-redis",
	}

	trace := BuildTrace(input)
	if trace.TurnID != "turn-7" {
		t.Fatalf("TurnID = %q, want turn-7", trace.TurnID)
	}
	if trace.ScopeHash != "scope-new" || trace.ScopeSummary != "checkout api redis latency" {
		t.Fatalf("scope = (%q, %q), want current scope", trace.ScopeHash, trace.ScopeSummary)
	}
	if trace.ManualBindingID != "manual-checkout-redis" {
		t.Fatalf("ManualBindingID = %q", trace.ManualBindingID)
	}
	if !contains(trace.ObservedEvidence, "redis p99 latency > 2s") {
		t.Fatalf("ObservedEvidence = %#v, want active fact", trace.ObservedEvidence)
	}
	for _, unexpected := range []string{"slowlog blocked by policy", "cpu sample is stale", "replica lag missing"} {
		if contains(trace.ObservedEvidence, unexpected) {
			t.Fatalf("ObservedEvidence = %#v, should not include stale/blocked/missing %q", trace.ObservedEvidence, unexpected)
		}
		if !contains(trace.MissingEvidence, unexpected) {
			t.Fatalf("MissingEvidence = %#v, want %q", trace.MissingEvidence, unexpected)
		}
	}
	if contains(trace.Hypotheses, "old root cause") || contains(trace.ObservedEvidence, "old root cause should not leak") {
		t.Fatalf("trace includes old scope content: %#v / %#v", trace.Hypotheses, trace.ObservedEvidence)
	}
	if trace.Confidence != ConfidenceLow {
		t.Fatalf("Confidence = %q, want low because stale evidence blocks confidence", trace.Confidence)
	}
	if len(trace.ToolFailures) != 1 {
		t.Fatalf("ToolFailures = %#v, want one scoped failure", trace.ToolFailures)
	}
	if strings.Contains(trace.ToolFailures[0].Detail, "secret") {
		t.Fatalf("tool failure detail was not redacted: %q", trace.ToolFailures[0].Detail)
	}
}

func TestBuildTraceStaleManualBindingLowersConfidence(t *testing.T) {
	trace := BuildTrace(TraceBuildInput{
		CurrentScope: DiagnosticScope{Hash: "scope-1", Summary: "payments api", Confirmed: true},
		Facts: []DiagnosticFact{
			{ScopeHash: "scope-1", Summary: "5xx rate elevated", Status: EvidenceStatusActive, DirectSupport: true},
		},
		RefutingEvidence:     []ScopedText{{ScopeHash: "scope-1", Text: "dependency health checked"}},
		ManualBindingID:      "manual-payments",
		ManualBindingIsStale: true,
	})

	if trace.Confidence != ConfidenceLow {
		t.Fatalf("Confidence = %q, want low for stale manual binding", trace.Confidence)
	}
	if !contains(trace.MissingEvidence, "manual binding manual-payments is stale") {
		t.Fatalf("MissingEvidence = %#v, want stale manual binding", trace.MissingEvidence)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
