package tooling

import "testing"

func TestToolMetadataEffectiveGovernanceDefaultsAreConservativeAndCompatible(t *testing.T) {
	meta := ToolMetadata{}

	governance := meta.EffectiveGovernance(4096)
	if governance.RiskLevel != ToolRiskMedium {
		t.Fatalf("default risk = %q, want %q", governance.RiskLevel, ToolRiskMedium)
	}
	if governance.Mutating {
		t.Fatal("default mutating = true, want false for compatibility")
	}
	if governance.RequiresApproval {
		t.Fatal("default requiresApproval = true, want false for compatibility")
	}
	if governance.FailurePolicy != ToolFailurePolicyFeedBackToModel {
		t.Fatalf("default failure policy = %q, want %q", governance.FailurePolicy, ToolFailurePolicyFeedBackToModel)
	}
	if governance.ResultBudget.MaxInlineResultBytes != 4096 {
		t.Fatalf("default result budget = %d, want 4096", governance.ResultBudget.MaxInlineResultBytes)
	}
}

func TestStaticToolCarriesExplicitGovernanceMetadata(t *testing.T) {
	tool := &StaticTool{Meta: ToolMetadata{
		Name:             "restart_service",
		RiskLevel:        ToolRiskHigh,
		Mutating:         true,
		RequiresApproval: true,
		ResultBudget:     ResultBudget{MaxInlineResultBytes: 2048, SpillPolicy: ResultSpillPolicyExternalize},
		FailurePolicy:    ToolFailurePolicyFailTurn,
	}}

	governance := tool.Metadata().EffectiveGovernance(4096)
	if governance.RiskLevel != ToolRiskHigh || !governance.Mutating || !governance.RequiresApproval {
		t.Fatalf("governance = %#v, want high mutating approval-required", governance)
	}
	if governance.ResultBudget.MaxInlineResultBytes != 2048 || governance.ResultBudget.SpillPolicy != ResultSpillPolicyExternalize {
		t.Fatalf("result budget = %#v, want explicit externalize budget", governance.ResultBudget)
	}
	if governance.FailurePolicy != ToolFailurePolicyFailTurn {
		t.Fatalf("failure policy = %q, want %q", governance.FailurePolicy, ToolFailurePolicyFailTurn)
	}
}
