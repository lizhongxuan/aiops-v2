package agentassembly

import (
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

func TestTurnAssemblyHashStableAcrossMetadataMapOrder(t *testing.T) {
	firstFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{Metadata: map[string]string{
		runtimecontract.MetadataIntentKind:       string(runtimecontract.IntentKindDiagnose),
		runtimecontract.MetadataIntentRiskBudget: string(runtimecontract.ActionRiskReadOnly),
		runtimecontract.MetadataProfile:          "advisor",
	}})
	secondFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{Metadata: map[string]string{
		runtimecontract.MetadataProfile:          "advisor",
		runtimecontract.MetadataIntentRiskBudget: string(runtimecontract.ActionRiskReadOnly),
		runtimecontract.MetadataIntentKind:       string(runtimecontract.IntentKindDiagnose),
	}})
	first := mustTurnAssembly(t, validTurnAssemblyInput(firstFacts))
	second := mustTurnAssembly(t, validTurnAssemblyInput(secondFacts))
	if first.Hash == "" || first.Hash != second.Hash {
		t.Fatalf("TurnAssembly hashes = %q and %q, want same non-empty hash", first.Hash, second.Hash)
	}
}

func TestTurnAssemblyHashChangesWithPermissionProfileAndResource(t *testing.T) {
	baseFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile:       "advisor",
		SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	baseInput := validTurnAssemblyInput(baseFacts)
	base := mustTurnAssembly(t, baseInput)

	permissionInput := baseInput
	permissionInput.PermissionProfile = "restricted"
	permission := mustTurnAssembly(t, permissionInput)

	profileFacts := baseFacts
	profileFacts.Profile = "host-worker"
	profileInput := baseInput
	profileInput.AdmissionFacts = profileFacts
	profile := mustTurnAssembly(t, profileInput)

	resourceFacts := baseFacts
	resourceFacts.SessionTarget.ID = "host-b"
	resourceInput := baseInput
	resourceInput.AdmissionFacts = resourceFacts
	resource := mustTurnAssembly(t, resourceInput)

	if base.Hash == permission.Hash || base.Hash == profile.Hash || base.Hash == resource.Hash {
		t.Fatalf("control fact change did not change hash: base=%q permission=%q profile=%q resource=%q", base.Hash, permission.Hash, profile.Hash, resource.Hash)
	}
}

func TestTurnAssemblyRejectsMutationWithoutTargetPermissionOrRollback(t *testing.T) {
	missingTarget, err := runtimecontract.BuildAdmissionFacts(runtimecontract.AdmissionInput{
		Intent:            &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}},
		PermissionProfile: "host-mutation",
	})
	if err == nil {
		t.Fatal("BuildAdmissionFacts() error = nil, want missing target setup error")
	}
	validMutation := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:            &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}},
		PermissionProfile: "host-mutation",
		SessionTarget:     resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	mutationWithoutPermission := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}},
		SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})

	cases := []TurnAssemblyInput{
		{AdmissionFacts: missingTarget, PermissionProfile: "host-mutation", RollbackPolicy: "restore previous state"},
		{AdmissionFacts: mutationWithoutPermission, RollbackPolicy: "restore previous state"},
		{AdmissionFacts: validMutation, PermissionProfile: "host-mutation"},
	}
	for index, input := range cases {
		if _, err := BuildTurnAssembly(input); err == nil {
			t.Fatalf("case %d BuildTurnAssembly() error = nil, want fail-closed mutation validation", index)
		}
	}
}

func TestTurnAssemblyRejectsShadowAdmissionError(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent: &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose},
	})
	input := validTurnAssemblyInput(facts)
	input.AdmissionError = "admission_facts_invalid"
	if _, err := BuildTurnAssembly(input); err == nil {
		t.Fatal("BuildTurnAssembly() error = nil, want shadow admission error rejection")
	}
}

func TestTurnAssemblyFreezesInputAndProjectsCompatibilitySnapshot(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:          &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		UserConstraints: []string{"read-only"},
		AgentKind:       "advisor",
		Profile:         "advisor",
		SessionTarget:   resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	input := validTurnAssemblyInput(facts)
	input.SourceRefs = []string{"policy:v1"}
	input.CapabilityPolicy.ModelVisibleTools = []ToolSurfaceItem{{Name: "read_status", Capability: resourcebinding.CapabilityRead}}
	assembly := mustTurnAssembly(t, input)

	input.SourceRefs[0] = "mutated"
	input.AdmissionFacts.UserConstraints[0] = "mutated"
	input.CapabilityPolicy.ModelVisibleTools[0].Name = "mutated"
	if assembly.SourceRefs[0] != "policy:v1" || assembly.AdmissionFacts.UserConstraints[0] != "read-only" || assembly.CapabilityPolicy.ModelVisibleTools[0].Name != "read_status" {
		t.Fatalf("TurnAssembly changed after input mutation: %#v", assembly)
	}
	snapshot, err := BuildSnapshotFromTurnAssembly(assembly, BuildInput{RuntimeRole: "workspace.advisor"})
	if err != nil {
		t.Fatalf("BuildSnapshotFromTurnAssembly() error = %v", err)
	}
	if snapshot.AgentKind != "advisor" || snapshot.Profile != "advisor" || snapshot.RuntimeRole != "workspace.advisor" {
		t.Fatalf("compatibility snapshot identity = %#v", snapshot)
	}
	if snapshot.ToolSurface.Hash != assembly.CapabilityPolicy.Hash || snapshot.ContextSelector.Hash != assembly.ContextPolicy.Hash || snapshot.LoopPolicy.Hash != assembly.LoopPolicy.Hash || snapshot.FinalContract.Hash != assembly.FinalContractPolicy.Hash {
		t.Fatalf("compatibility snapshot policies drifted: snapshot=%#v assembly=%#v", snapshot, assembly)
	}
}

func TestTurnAssemblyCompatibilityProjectionRejectsHashMismatch(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:  &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile: "advisor",
	})
	assembly := mustTurnAssembly(t, validTurnAssemblyInput(facts))
	assembly.AdmissionFacts.Profile = "tampered-profile"
	if _, err := BuildSnapshotFromTurnAssembly(assembly, BuildInput{}); err == nil {
		t.Fatal("BuildSnapshotFromTurnAssembly() error = nil, want hash mismatch rejection")
	}
}

func validTurnAssemblyInput(facts runtimecontract.AdmissionFacts) TurnAssemblyInput {
	return TurnAssemblyInput{
		AdmissionFacts:      facts,
		PermissionProfile:   "workspace-default",
		CapabilityPolicy:    CapabilityPolicySnapshot{PolicyHash: "sha256:capability-policy"},
		ContextPolicy:       ContextSelectorSnapshot{Policy: "bounded", Budget: "default"},
		LoopPolicy:          LoopPolicySnapshot{MaxIterations: 6, ToolCallPolicy: "governed"},
		FinalContractPolicy: FinalContractSnapshot{Shape: "typed-final"},
		RollbackPolicy:      "not-required-for-read-only",
		SourceRefs:          []string{"policy:v1"},
	}
}

func mustAdmissionFactsForAssembly(t *testing.T, input runtimecontract.AdmissionInput) runtimecontract.AdmissionFacts {
	t.Helper()
	facts, err := runtimecontract.BuildAdmissionFacts(input)
	if err != nil {
		t.Fatalf("BuildAdmissionFacts() error = %v", err)
	}
	return facts
}

func mustTurnAssembly(t *testing.T, input TurnAssemblyInput) TurnAssembly {
	t.Helper()
	assembly, err := BuildTurnAssembly(input)
	if err != nil {
		t.Fatalf("BuildTurnAssembly() error = %v", err)
	}
	return assembly
}
