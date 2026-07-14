package agentassembly

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
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

	profileFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile:       "host-worker",
		SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	profileInput := baseInput
	profileInput.AdmissionFacts = profileFacts
	profile := mustTurnAssembly(t, profileInput)

	resourceFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile:       "advisor",
		SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"},
	})
	resourceInput := baseInput
	resourceInput.AdmissionFacts = resourceFacts
	resource := mustTurnAssembly(t, resourceInput)

	if base.Hash == permission.Hash || base.Hash == profile.Hash || base.Hash == resource.Hash {
		t.Fatalf("control fact change did not change hash: base=%q permission=%q profile=%q resource=%q", base.Hash, permission.Hash, profile.Hash, resource.Hash)
	}
}

func TestTurnAssemblyHashCoversTypedTargetRefs(t *testing.T) {
	baseFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:     &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile:    "advisor",
		TargetRefs: []resourcebinding.ResourceRef{{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}},
	})
	changedFacts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:     &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile:    "advisor",
		TargetRefs: []resourcebinding.ResourceRef{{Type: resourcebinding.ResourceTypeHost, ID: "host-b"}},
	})
	base := mustTurnAssembly(t, validTurnAssemblyInput(baseFacts))
	changed := mustTurnAssembly(t, validTurnAssemblyInput(changedFacts))

	if base.Hash == changed.Hash {
		t.Fatalf("typed target refs did not change TurnAssembly hash: %q", base.Hash)
	}

	tampered := base
	tampered.AdmissionFacts = changedFacts
	if err := tampered.Validate(); err == nil {
		t.Fatal("TurnAssembly.Validate() accepted replacement typed target refs with the old assembly hash")
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

func TestTurnAssemblyRejectsStaleAdmissionFactsHash(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:  &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose},
		Profile: "advisor",
	})
	facts.Profile = "tampered-profile"
	if _, err := BuildTurnAssembly(validTurnAssemblyInput(facts)); err == nil {
		t.Fatal("BuildTurnAssembly() error = nil, want stale admission hash rejection")
	}
}

func TestTurnAssemblyCompatibilitySpecHashCoversPreservedFields(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{Intent: &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose}})
	assembly := mustTurnAssembly(t, validTurnAssemblyInput(facts))
	baseInput := BuildInput{
		RuntimeRole:       "workspace.advisor",
		ProfilePromptHash: "sha256:profile-a",
		PromptSections:    []promptcompiler.PromptSectionTrace{{ID: "base", Hash: "sha256:base-a"}},
		TraceTags:         map[string]string{"route": "advisor"},
	}
	base, err := BuildSnapshotFromTurnAssembly(assembly, baseInput)
	if err != nil {
		t.Fatalf("BuildSnapshotFromTurnAssembly(base) error = %v", err)
	}
	variants := []BuildInput{baseInput, baseInput, baseInput, baseInput}
	variants[0].RuntimeRole = "workspace.worker"
	variants[1].ProfilePromptHash = "sha256:profile-b"
	variants[2].PromptSections = []promptcompiler.PromptSectionTrace{{ID: "base", Hash: "sha256:base-b"}}
	variants[3].TraceTags = map[string]string{"route": "worker"}
	for index, input := range variants {
		snapshot, err := BuildSnapshotFromTurnAssembly(assembly, input)
		if err != nil {
			t.Fatalf("variant %d BuildSnapshotFromTurnAssembly() error = %v", index, err)
		}
		if snapshot.SpecHash == base.SpecHash {
			t.Fatalf("variant %d preserved field missing from SpecHash %q", index, snapshot.SpecHash)
		}
	}
}

func TestLegacyBuildPreservesCallerProvidedNestedHashSemantics(t *testing.T) {
	empty := Build(BuildInput{ContextSelector: ContextSelectorSnapshot{Policy: "bounded"}, FinalContract: FinalContractSnapshot{Shape: "typed"}})
	prefilled := Build(BuildInput{
		ContextSelector: ContextSelectorSnapshot{Policy: "bounded", Hash: "sha256:caller-context"},
		FinalContract:   FinalContractSnapshot{Shape: "typed", Hash: "sha256:caller-final"},
	})
	if empty.ContextSelector.Hash == prefilled.ContextSelector.Hash || empty.FinalContract.Hash == prefilled.FinalContract.Hash {
		t.Fatalf("legacy nested hash semantics drifted: empty=%#v prefilled=%#v", empty, prefilled)
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
