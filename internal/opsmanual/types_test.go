package opsmanual

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpsManualJSONUsesStableSnakeCaseFields(t *testing.T) {
	manual := OpsManual{
		ID:             "manual-redis-memory",
		ManualFamilyID: "redis-rca",
		Title:          "Redis memory triage",
		Status:         ManualStatusVerified,
		WorkflowRef: WorkflowRef{
			WorkflowID:     "workflow-redis-memory",
			WorkflowDigest: "sha256:abc",
		},
		Operation: OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss"},
		},
		Preconditions:    []string{"can connect to Redis"},
		Validation:       []string{"used_memory_rss no longer rises"},
		CannotUseWhen:    []string{"target instance unknown"},
		DocumentMarkdown: "Redis memory pressure manual",
	}
	raw, err := json.Marshal(manual)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"manual_family_id", "workflow_ref", "target_type", "required_context", "cannot_use_when"} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want field %s", text, want)
		}
	}
}

func TestDecisionStateCanonicalValues(t *testing.T) {
	cases := map[DecisionState]string{
		DecisionDirectExecute: "direct_execute",
		DecisionNeedInfo:      "need_info",
		DecisionAdapt:         "adapt",
		DecisionReference:     "reference_only",
		DecisionNoMatch:       "no_match",
	}
	for state, want := range cases {
		if string(state) != want {
			t.Fatalf("state %v = %q, want %q", state, string(state), want)
		}
	}
	if DecisionDirect != DecisionDirectExecute {
		t.Fatalf("DecisionDirect = %q, want %q", DecisionDirect, DecisionDirectExecute)
	}
	if DecisionNeedMoreInfo != DecisionNeedInfo {
		t.Fatalf("DecisionNeedMoreInfo = %q, want %q", DecisionNeedMoreInfo, DecisionNeedInfo)
	}
}
