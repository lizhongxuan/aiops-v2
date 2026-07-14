package agentassembly

import (
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestToolSurfaceSnapshotRequiresVisibleSubsetOfDispatchable(t *testing.T) {
	snapshot := BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "host.exec"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "host.exec"}},
	})
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	bad := BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "host.exec"}, {Name: "host.hidden"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "host.exec"}},
	})
	if err := bad.Validate(); err == nil {
		t.Fatalf("Validate() nil, want visible tool outside dispatchable rejected")
	}
}

func TestToolSurfaceSnapshotRecordsHiddenReasonAndApprovalPolicy(t *testing.T) {
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	snapshot := BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{binding},
		PolicyHash:       "sha256:policy",
		ModelVisibleTools: []tooling.ToolMetadata{{
			Name:             "host.write",
			Mutating:         true,
			RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				ResourceTypes: []string{resourcebinding.ResourceTypeHost},
			},
		}},
		DispatchableTools: []tooling.ToolMetadata{{
			Name:             "host.write",
			Mutating:         true,
			RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				ResourceTypes: []string{resourcebinding.ResourceTypeHost},
			},
		}},
		HiddenTools: []HiddenToolInput{{Name: "host.raw", Reason: "profile_denied"}},
	})

	if len(snapshot.HiddenTools) != 1 || snapshot.HiddenTools[0].HiddenReason != "profile_denied" {
		t.Fatalf("hidden tools = %#v, want hidden reason", snapshot.HiddenTools)
	}
	item := snapshot.ModelVisibleTools[0]
	if !item.RequiresApproval || item.PolicyHash != "sha256:policy" {
		t.Fatalf("tool item = %#v, want approval policy", item)
	}
	if item.ResourceBindingHash == "" {
		t.Fatalf("tool item = %#v, want resource binding hash", item)
	}
}

func TestToolSurfaceSnapshotRejectsMissingOrUnknownHiddenReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
	}{
		{name: "missing", reason: ""},
		{name: "unknown", reason: "made_up_reason"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := BuildToolSurfaceSnapshot(ToolSurfaceInput{
				HiddenTools: []HiddenToolInput{{Name: "exec_command", Reason: tc.reason}},
			})
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want invalid hidden reason rejected")
			}
		})
	}
}

func TestMutationTurnAssemblyRejectsToolWithoutDeclarativeRollback(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:            &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}},
		PermissionProfile: "host-mutation",
		SessionTarget:     resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	mutationTool := tooling.ToolMetadata{
		Name:             "restart_service",
		Mutating:         true,
		RequiresApproval: true,
	}
	input := validTurnAssemblyInput(facts)
	input.PermissionProfile = "host-mutation"
	input.RollbackPolicy = "action-rollback-contract-required"
	input.CapabilityPolicy = BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: []tooling.ToolMetadata{mutationTool},
		DispatchableTools: []tooling.ToolMetadata{mutationTool},
	})

	if _, err := BuildTurnAssembly(input); err == nil {
		t.Fatal("BuildTurnAssembly() error = nil, want mutation tool without declarative rollback rejected")
	}
}

func TestMutationTurnAssemblyAcceptsToolWithDeclarativeRollback(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:            &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}},
		PermissionProfile: "host-mutation",
		SessionTarget:     resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	mutationTool := tooling.ToolMetadata{
		Name:             "restart_service",
		Mutating:         true,
		RequiresApproval: true,
		Rollback: &tooling.ToolRollbackMetadata{
			Strategy:  tooling.ToolRollbackStrategyAutomatic,
			Reference: "runbook://service/restart/rollback",
		},
	}
	input := validTurnAssemblyInput(facts)
	input.PermissionProfile = "host-mutation"
	input.RollbackPolicy = "action-rollback-contract-required"
	input.CapabilityPolicy = BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: []tooling.ToolMetadata{mutationTool},
		DispatchableTools: []tooling.ToolMetadata{mutationTool},
	})

	assembly, err := BuildTurnAssembly(input)
	if err != nil {
		t.Fatalf("BuildTurnAssembly() error = %v", err)
	}
	item := assembly.CapabilityPolicy.ModelVisibleTools[0]
	if !item.Mutating || !item.RollbackReady || item.RollbackDeclarationHash == "" {
		t.Fatalf("mutation tool surface item = %#v, want typed rollback readiness proof", item)
	}
}

func TestReadOnlyTurnAssemblyDoesNotRequireToolRollbackDeclaration(t *testing.T) {
	facts := mustAdmissionFactsForAssembly(t, runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	readTool := tooling.ToolMetadata{Name: "read_service_status"}
	input := validTurnAssemblyInput(facts)
	input.CapabilityPolicy = BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: []tooling.ToolMetadata{readTool},
		DispatchableTools: []tooling.ToolMetadata{readTool},
	})

	if _, err := BuildTurnAssembly(input); err != nil {
		t.Fatalf("BuildTurnAssembly() read-only error = %v", err)
	}
}
