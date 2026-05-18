package diagnostics

import (
	"encoding/json"
	"testing"
)

func TestDiagnosticTraceJSONRoundTripPreservesCoreFields(t *testing.T) {
	original := DiagnosticTrace{
		TurnID:           "turn-1",
		ScopeHash:        "scope-abc",
		ScopeSummary:     "redis primary latency",
		Hypotheses:       []string{"redis memory pressure"},
		ObservedEvidence: []string{"used_memory_peak is elevated"},
		RefutingEvidence: []string{"network latency is normal"},
		MissingEvidence:  []string{"slowlog unavailable"},
		ToolFailures: []ToolFailure{{
			ToolName: "redis-cli",
			Semantic: ToolFailureTimeout,
			Detail:   "timed out reading slowlog",
			Critical: true,
		}},
		ManualBindingID:  "manual-redis-latency",
		Confidence:       ConfidenceMedium,
		ConfidenceReason: "critical probe timed out",
		RequiresApproval: true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	var decoded DiagnosticTrace
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}

	if decoded.ScopeHash != original.ScopeHash {
		t.Fatalf("ScopeHash = %q, want %q", decoded.ScopeHash, original.ScopeHash)
	}
	if len(decoded.Hypotheses) != 1 || decoded.Hypotheses[0] != original.Hypotheses[0] {
		t.Fatalf("Hypotheses = %#v, want %#v", decoded.Hypotheses, original.Hypotheses)
	}
	if len(decoded.MissingEvidence) != 1 || decoded.MissingEvidence[0] != original.MissingEvidence[0] {
		t.Fatalf("MissingEvidence = %#v, want %#v", decoded.MissingEvidence, original.MissingEvidence)
	}
	if len(decoded.ToolFailures) != 1 || decoded.ToolFailures[0].Semantic != ToolFailureTimeout {
		t.Fatalf("ToolFailures = %#v, want timeout failure", decoded.ToolFailures)
	}
	if decoded.Confidence != original.Confidence {
		t.Fatalf("Confidence = %q, want %q", decoded.Confidence, original.Confidence)
	}
}

func TestConfidenceLevelValues(t *testing.T) {
	got := []ConfidenceLevel{ConfidenceHigh, ConfidenceMedium, ConfidenceLow}
	want := []ConfidenceLevel{"high", "medium", "low"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("confidence level %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToolFailureSemanticValues(t *testing.T) {
	got := []ToolFailureSemantic{
		ToolFailurePolicyBlocked,
		ToolFailureCommandNotAllowed,
		ToolFailurePermissionDenied,
		ToolFailureTimeout,
		ToolFailureNonZeroExit,
		ToolFailureEmptyOutput,
	}
	want := []ToolFailureSemantic{
		"policy_blocked",
		"command_not_allowed",
		"permission_denied",
		"timeout",
		"non_zero_exit",
		"empty_output",
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tool failure semantic %d = %q, want %q", i, got[i], want[i])
		}
	}
}
