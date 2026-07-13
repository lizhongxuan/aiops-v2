package runtimecontract

import (
	"strings"
	"testing"

	"aiops-v2/internal/resourcebinding"
)

func TestAdmissionFactsRejectsMutationWithoutTarget(t *testing.T) {
	_, err := BuildAdmissionFacts(AdmissionInput{
		Intent: &IntentFrame{Kind: IntentKindChange, RiskBudget: []ActionRisk{ActionRiskWrite}},
	})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want missing mutation target failure")
	}
}

func TestAdmissionFactsTypedInputIgnoresShadowedControlMetadata(t *testing.T) {
	typed := IntentFrame{Kind: IntentKindDiagnose, RiskBudget: []ActionRisk{ActionRiskReadOnly}}
	first, err := BuildAdmissionFacts(AdmissionInput{
		Intent:  &typed,
		Profile: "typed-profile",
		Metadata: map[string]string{
			MetadataIntentFrame: `{"kind":`,
			MetadataIntentKind:  string(IntentKindChange),
			MetadataProfile:     "shadow-profile",
		},
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(shadowed metadata) error = %v", err)
	}
	second, err := BuildAdmissionFacts(AdmissionInput{Intent: &typed, Profile: "typed-profile"})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(typed only) error = %v", err)
	}
	if first.Hash != second.Hash || first.Profile != "typed-profile" || first.Intent.Kind != IntentKindDiagnose {
		t.Fatalf("shadowed metadata changed typed facts: first=%#v second=%#v", first, second)
	}
	for _, ref := range first.SourceRefs {
		if strings.Contains(ref, "intent") || strings.Contains(ref, "profile") {
			t.Fatalf("shadowed metadata entered source refs: %#v", first.SourceRefs)
		}
	}
}

func TestAdmissionFactsStructuredMetadataFrameShadowsLegacyIntentKeys(t *testing.T) {
	frameOnly, err := BuildAdmissionFacts(AdmissionInput{Metadata: map[string]string{
		MetadataIntentFrame: `{"kind":"diagnose","risk_budget":["read_only"]}`,
	}})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(frame only) error = %v", err)
	}
	withLegacy, err := BuildAdmissionFacts(AdmissionInput{Metadata: map[string]string{
		MetadataIntentFrame:      `{"kind":"diagnose","risk_budget":["read_only"]}`,
		MetadataIntentKind:       string(IntentKindChange),
		MetadataIntentRiskBudget: string(ActionRiskUnknown),
	}})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(frame plus shadowed legacy) error = %v", err)
	}
	if frameOnly.Hash != withLegacy.Hash {
		t.Fatalf("shadowed legacy metadata changed frame hash: %q != %q", frameOnly.Hash, withLegacy.Hash)
	}
}

func TestAdmissionFactsRejectsConflictingRoleResources(t *testing.T) {
	hostA := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}
	hostB := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"}
	_, err := BuildAdmissionFacts(AdmissionInput{
		Intent: &IntentFrame{Kind: IntentKindDiagnose, RiskBudget: []ActionRisk{ActionRiskReadOnly}},
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{
			verifiedAdmissionBinding(hostA),
			verifiedAdmissionBinding(hostB),
		},
		RoleBindings: []resourcebinding.ResourceRoleBinding{
			resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{ResourceRef: hostA, Role: "primary", Confidence: 1}),
			resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{ResourceRef: hostB, Role: "primary", Confidence: 1}),
		},
	})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want conflicting role/resource failure")
	}
}

func TestAdmissionFactsNormalizationProducesStableHash(t *testing.T) {
	hostA := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}
	hostB := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"}
	first, err := BuildAdmissionFacts(AdmissionInput{
		Metadata: map[string]string{
			MetadataIntentKind:       string(IntentKindDiagnose),
			MetadataIntentRiskBudget: "read_only,network",
			MetadataProfile:          "advisor",
			"custom.compat":          "first",
		},
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{
			verifiedAdmissionBinding(hostB),
			verifiedAdmissionBinding(hostA),
		},
		SourceRefs: []string{"turn:1", "admission:test"},
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(first) error = %v", err)
	}
	second, err := BuildAdmissionFacts(AdmissionInput{
		Metadata: map[string]string{
			"custom.compat":          "changed-but-non-control",
			MetadataProfile:          "advisor",
			MetadataIntentRiskBudget: "network,read_only",
			MetadataIntentKind:       string(IntentKindDiagnose),
		},
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{
			verifiedAdmissionBinding(hostA),
			verifiedAdmissionBinding(hostB),
		},
		SourceRefs: []string{"admission:test", "turn:1"},
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts(second) error = %v", err)
	}
	if first.Hash == "" || first.Hash != second.Hash {
		t.Fatalf("hashes = %q and %q, want same non-empty stable hash", first.Hash, second.Hash)
	}
	if first.Profile != "advisor" || second.Profile != "advisor" {
		t.Fatalf("profiles = %q and %q, want registered control value", first.Profile, second.Profile)
	}
	if len(first.CompatibilityOnlyKeys) != 1 || first.CompatibilityOnlyKeys[0] != "custom.compat" {
		t.Fatalf("compatibility keys = %#v, want only custom.compat", first.CompatibilityOnlyKeys)
	}
	if len(second.CompatibilityOnlyKeys) != 1 || second.CompatibilityOnlyKeys[0] != "custom.compat" {
		t.Fatalf("compatibility keys = %#v, want only custom.compat", second.CompatibilityOnlyKeys)
	}
}

func TestAdmissionFactsRejectsInvalidRegisteredIntentFrame(t *testing.T) {
	_, err := BuildAdmissionFacts(AdmissionInput{Metadata: map[string]string{
		MetadataIntentFrame: `{"kind":`,
	}})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want invalid registered control metadata failure")
	}
}

func TestAdmissionFactsDeepCopiesIntentAndRejectsInvalidScope(t *testing.T) {
	frame := IntentFrame{
		Kind:       IntentKindDiagnose,
		DataScopes: []DataScope{DataScopeWorkspace, DataScopeLocalRuntime},
		Capabilities: []CapabilityCandidate{{
			Name:       "inspect",
			DataScopes: []DataScope{DataScopeWorkspace},
			Risks:      []ActionRisk{ActionRiskReadOnly},
			Reasons:    []string{"typed capability"},
		}},
	}
	facts, err := BuildAdmissionFacts(AdmissionInput{Intent: &frame})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts() error = %v", err)
	}
	frame.DataScopes[0] = DataScope("mutated")
	frame.Capabilities[0].Reasons[0] = "mutated"
	if facts.Intent.DataScopes[0] != DataScopeLocalRuntime || facts.Intent.Capabilities[0].Reasons[0] != "typed capability" {
		t.Fatalf("facts changed after input mutation: %#v", facts.Intent)
	}

	_, err = BuildAdmissionFacts(AdmissionInput{Metadata: map[string]string{
		MetadataIntentKind:       string(IntentKindDiagnose),
		MetadataIntentDataScopes: "unregistered_scope",
	}})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want invalid registered data scope failure")
	}
}

func TestAdmissionFactsRejectsRegisteredUnknownEnumsBeforeNormalization(t *testing.T) {
	cases := []AdmissionInput{
		{Metadata: map[string]string{MetadataIntentKind: string(IntentKindDiagnose), MetadataIntentRiskBudget: string(ActionRiskUnknown)}},
		{Metadata: map[string]string{MetadataIntentKind: string(IntentKindDiagnose), MetadataIntentDataScopes: string(DataScopeUnknown)}},
		{Intent: &IntentFrame{Kind: IntentKindDiagnose, Evidence: EvidenceEnvelope{EvidenceKinds: []string{"unknown_evidence"}}}},
		{Intent: &IntentFrame{Kind: IntentKindDiagnose, Evidence: EvidenceEnvelope{WeakSignals: []WeakSignal{{Name: "unknown_signal"}}}}},
	}
	for index, input := range cases {
		if _, err := BuildAdmissionFacts(input); err == nil {
			t.Fatalf("case %d BuildAdmissionFacts() error = nil, want registered enum failure", index)
		}
	}
}

func TestAdmissionFactsDoesNotUpgradeFailClosedResourceBinding(t *testing.T) {
	binding := verifiedAdmissionBinding(resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"})
	binding.FailClosed = true
	facts, err := BuildAdmissionFacts(AdmissionInput{
		Intent:           &IntentFrame{Kind: IntentKindChange, RiskBudget: []ActionRisk{ActionRiskWrite}},
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{binding},
	})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want fail-closed binding to remain unverified")
	}
	if len(facts.ResourceBindings) != 1 || !facts.ResourceBindings[0].FailClosed || facts.ResourceBindings[0].TraceHash != binding.TraceHash {
		t.Fatalf("resource binding trust was rewritten: got=%#v want=%#v", facts.ResourceBindings, binding)
	}
}

func TestAdmissionFactsIntegrityRejectsStaleOrTamperedHash(t *testing.T) {
	facts, err := BuildAdmissionFacts(AdmissionInput{
		Intent:  &IntentFrame{Kind: IntentKindDiagnose},
		Profile: "advisor",
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts() error = %v", err)
	}
	tamperedField := facts
	tamperedField.Profile = "host-worker"
	if err := ValidateAdmissionFactsIntegrity(tamperedField); err == nil {
		t.Fatal("ValidateAdmissionFactsIntegrity(field tamper) error = nil")
	}
	tamperedHash := facts
	tamperedHash.Hash = "sha256:tampered"
	if err := ValidateAdmissionFactsIntegrity(tamperedHash); err == nil {
		t.Fatal("ValidateAdmissionFactsIntegrity(hash tamper) error = nil")
	}
}

func verifiedAdmissionBinding(ref resourcebinding.ResourceRef) resourcebinding.ResourceBindingSnapshot {
	return resourcebinding.NewBindingSnapshot(ref, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: "admission-test",
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
}
