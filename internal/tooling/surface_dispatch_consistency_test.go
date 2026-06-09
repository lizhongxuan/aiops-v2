package tooling

import "testing"

func TestSurfaceDecisionRecordsDispatchActionForEveryTool(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", Mutating: true, RequiresApproval: true}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.hidden", Discovery: ToolDiscoveryMetadata{HiddenFromPrompt: true}}},
	}

	_, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "execute"})

	assertSurfaceDecisionForTest(t, snapshot, "synthetic.read", true, SurfaceDispatchAllow, "policy_allowed")
	assertSurfaceDecisionForTest(t, snapshot, "synthetic.write", true, SurfaceDispatchNeedApproval, "approval_required_schema_hidden")
	assertSurfaceDecisionForTest(t, snapshot, "synthetic.hidden", false, SurfaceDispatchDeny, "hidden_from_prompt")
	if err := ValidateSurfaceDispatchConsistency(snapshot); err != nil {
		t.Fatalf("ValidateSurfaceDispatchConsistency() error = %v", err)
	}
}

func TestSurfaceDispatchConsistencyRejectsVisibleDeniedTool(t *testing.T) {
	snapshot := ToolSurfacePolicySnapshot{
		SurfaceDecisions: []SurfaceDecision{{
			Name:           "synthetic.bad",
			Visible:        true,
			DispatchAction: SurfaceDispatchDeny,
			Reason:         "runtime_denied",
		}},
	}

	if err := ValidateSurfaceDispatchConsistency(snapshot); err == nil {
		t.Fatal("ValidateSurfaceDispatchConsistency() error = nil, want visible denied tool rejected")
	}
}

func TestRuntimeDispatchDecisionUpdatesToolSurface(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", Mutating: true, RequiresApproval: true}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.conflict_write", Mutating: true, RequiresApproval: true}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{
		Mode: "execute",
		RuntimeDecisions: []SurfaceRuntimeDecision{
			{Name: "synthetic.read", DispatchAction: SurfaceDispatchDeny, Reason: "safety_signal"},
			{Name: "synthetic.write", DispatchAction: SurfaceDispatchNeedApproval, Reason: "safety_signal"},
			{Name: "synthetic.conflict_write", DispatchAction: SurfaceDispatchDeny, Reason: "unexpected_state"},
		},
	})

	if got := toolNamesForTest(filtered); len(got) != 0 {
		t.Fatalf("filtered tools = %#v, want runtime-gated tools removed from executable prompt surface", got)
	}
	assertSurfaceDecisionForTest(t, snapshot, "synthetic.read", false, SurfaceDispatchDeny, "safety_signal")
	assertSurfaceDecisionForTest(t, snapshot, "synthetic.write", true, SurfaceDispatchNeedApproval, "safety_signal")
	assertSurfaceDecisionForTest(t, snapshot, "synthetic.conflict_write", false, SurfaceDispatchDeny, "unexpected_state")
	if err := ValidateSurfaceDispatchConsistency(snapshot); err != nil {
		t.Fatalf("ValidateSurfaceDispatchConsistency() error = %v", err)
	}
}

func TestVisibleApprovalToolCarriesApprovalRequiredReason(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.high", RiskLevel: ToolRiskHigh}},
	}

	_, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "execute"})

	decision, ok := SurfaceDecisionForTool(snapshot, ToolMetadata{Name: "synthetic.high"}, "synthetic.high")
	if !ok {
		t.Fatalf("SurfaceDecisionForTool() missing decision: %#v", snapshot.SurfaceDecisions)
	}
	if !decision.Visible || !decision.SummaryOnly || decision.DispatchAction != SurfaceDispatchNeedApproval {
		t.Fatalf("decision = %#v, want visible summary-only need approval", decision)
	}
	if decision.Reason == "" {
		t.Fatalf("decision reason empty: %#v", decision)
	}
}

func assertSurfaceDecisionForTest(t *testing.T, snapshot ToolSurfacePolicySnapshot, name string, visible bool, action SurfaceDispatchAction, reason string) {
	t.Helper()
	decision, ok := SurfaceDecisionForTool(snapshot, ToolMetadata{Name: name}, name)
	if !ok {
		t.Fatalf("SurfaceDecisionForTool(%q) missing; decisions=%#v", name, snapshot.SurfaceDecisions)
	}
	if decision.Visible != visible || decision.DispatchAction != action || decision.Reason != reason {
		t.Fatalf("decision for %s = %#v, want visible=%v action=%s reason=%q", name, decision, visible, action, reason)
	}
}
