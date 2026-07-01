package runtimecontract

import (
	"encoding/json"
	"testing"
)

func TestIntentFrameJSONRoundTripPreservesStructuredFields(t *testing.T) {
	frame := IntentFrame{
		Kind:       IntentKindResearch,
		DataScopes: []DataScope{DataScopePublicWeb, DataScopeOpsKnowledge},
		RiskBudget: []ActionRisk{ActionRiskNetwork},
		Constraints: []IntentConstraint{{
			Name:       "no_host_exec",
			Value:      "true",
			Confidence: ConfidenceHigh,
			Source:     "user",
		}},
		Capabilities: []CapabilityCandidate{{
			Name:       "search_ops_manuals",
			DataScopes: []DataScope{DataScopeOpsKnowledge},
			Risks:      []ActionRisk{ActionRiskReadOnly},
			Reasons:    []string{"ops knowledge requested"},
		}},
		Evidence: EvidenceEnvelope{
			HasUserProvidedEvidence: true,
			EvidenceKinds:           []string{EvidenceKindLog},
			DataScopes:              []DataScope{DataScopeWorkspace},
			WeakSignals: []WeakSignal{{
				Name:       WeakSignalLogLikeText,
				Source:     "input",
				Confidence: ConfidenceMedium,
				Summary:    "log-like text",
			}},
		},
		Confidence: ConfidenceMedium,
		Classifier: "unit-test",
	}

	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded IntentFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Kind != IntentKindResearch {
		t.Fatalf("Kind = %q, want %q", decoded.Kind, IntentKindResearch)
	}
	if !ContainsDataScope(decoded.DataScopes, DataScopePublicWeb) || !ContainsDataScope(decoded.DataScopes, DataScopeOpsKnowledge) {
		t.Fatalf("DataScopes = %#v, want public web and ops knowledge", decoded.DataScopes)
	}
	if !ContainsActionRisk(decoded.RiskBudget, ActionRiskNetwork) {
		t.Fatalf("RiskBudget = %#v, want network", decoded.RiskBudget)
	}
	if len(decoded.Evidence.WeakSignals) != 1 || decoded.Evidence.WeakSignals[0].Name != WeakSignalLogLikeText {
		t.Fatalf("WeakSignals = %#v, want log-like signal", decoded.Evidence.WeakSignals)
	}
}

func TestNormalizeIntentFrameDefaultsAreConservative(t *testing.T) {
	frame := NormalizeIntentFrame(IntentFrame{})

	if frame.Kind != IntentKindUnknown {
		t.Fatalf("Kind = %q, want unknown", frame.Kind)
	}
	if frame.Confidence != ConfidenceLow {
		t.Fatalf("Confidence = %q, want low", frame.Confidence)
	}
	if ContainsActionRisk(frame.RiskBudget, ActionRiskHostExec) {
		t.Fatalf("unknown frame RiskBudget = %#v, must not include host_exec", frame.RiskBudget)
	}
	if ContainsDataScope(frame.DataScopes, DataScopePublicWeb) {
		t.Fatalf("unknown frame DataScopes = %#v, must not include public_web", frame.DataScopes)
	}
	if frame.Evidence.HasUserProvidedEvidence {
		t.Fatal("unknown frame must not claim user-provided evidence")
	}
}

func TestAppendHelpersDeduplicateAndIgnoreUnknownValues(t *testing.T) {
	scopes := AppendDataScope([]DataScope{DataScopeWorkspace}, DataScopeWorkspace, DataScopeUnknown, DataScopePublicWeb)
	if len(scopes) != 2 || !ContainsDataScope(scopes, DataScopeWorkspace) || !ContainsDataScope(scopes, DataScopePublicWeb) {
		t.Fatalf("AppendDataScope() = %#v, want workspace and public web only", scopes)
	}

	risks := AppendActionRisk([]ActionRisk{ActionRiskReadOnly}, ActionRiskReadOnly, ActionRiskUnknown, ActionRiskHostExec)
	if len(risks) != 2 || !ContainsActionRisk(risks, ActionRiskReadOnly) || !ContainsActionRisk(risks, ActionRiskHostExec) {
		t.Fatalf("AppendActionRisk() = %#v, want read_only and host_exec only", risks)
	}
}
