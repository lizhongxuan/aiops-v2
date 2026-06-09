package tooling

import (
	"reflect"
	"testing"
)

func TestToolSurfacePolicySnapshotFiltersPrompt(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.visible", Layer: ToolLayerCore, RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.internal", Layer: ToolLayerInternal}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.hidden", Discovery: ToolDiscoveryMetadata{HiddenFromPrompt: true}}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.profile", Profiles: []string{"debug"}}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", Mutating: true, RiskLevel: ToolRiskHigh}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "chat", Profile: "default"})

	if got := toolNamesForTest(filtered); !reflect.DeepEqual(got, []string{"synthetic.visible"}) {
		t.Fatalf("filtered tools = %#v, want only synthetic.visible", got)
	}
	wantReasons := map[string]string{
		"synthetic.hidden":   "hidden_from_prompt",
		"synthetic.internal": "internal_tool",
		"synthetic.profile":  "profile_denied",
		"synthetic.write":    "mode_denied",
	}
	for _, hidden := range snapshot.HiddenTools {
		if wantReasons[hidden.Name] != hidden.Reason {
			t.Fatalf("hidden reason for %s = %q, want %q; snapshot=%#v", hidden.Name, hidden.Reason, wantReasons[hidden.Name], snapshot.HiddenTools)
		}
		delete(wantReasons, hidden.Name)
	}
	if len(wantReasons) != 0 {
		t.Fatalf("missing hidden reasons: %#v", wantReasons)
	}
	if snapshot.Hash == "" {
		t.Fatal("snapshot hash is empty")
	}
}

func TestToolSurfacePolicyHonorsAgentRoleReadOnlyBoundary(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", Layer: ToolLayerCore, RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", Layer: ToolLayerMutation, Mutating: true, RequiresApproval: true}},
	}
	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "execute", AgentRole: "verify"})
	if len(filtered) != 1 || filtered[0].Metadata().Name != "synthetic.read" {
		t.Fatalf("filtered tools = %#v, want only synthetic.read", toolNamesForTest(filtered))
	}
	if !hiddenReasonForTest(snapshot, "synthetic.write", "agent_role_read_only") {
		t.Fatalf("hidden tools = %#v, want agent_role_read_only", snapshot.HiddenTools)
	}
}

func TestToolSurfacePolicyRequiresApprovalForExecutorMutation(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write.no_approval", Layer: ToolLayerMutation, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write.approved", Layer: ToolLayerMutation, Mutating: true, RequiresApproval: true}},
	}
	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "execute", AgentRole: "execute"})
	if len(filtered) != 0 {
		t.Fatalf("filtered tools = %#v, want approved mutation schema hidden behind approval summary", toolNamesForTest(filtered))
	}
	if !hiddenReasonForTest(snapshot, "synthetic.write.no_approval", "agent_role_mutation_requires_approval") {
		t.Fatalf("hidden tools = %#v, want agent_role_mutation_requires_approval", snapshot.HiddenTools)
	}
	if !visibleSummaryOnlyForTest(snapshot, "synthetic.write.approved", "approval_required_schema_hidden") {
		t.Fatalf("visible tools = %#v, want approved mutation summary-only", snapshot.VisibleTools)
	}
}

func TestPlanModeOnlyAllowsReadOnlyAndPlanArtifactToolsOnSurface(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", Layer: ToolLayerCore, RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "update_plan", Layer: ToolLayerMutation, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "request_plan_approval", Layer: ToolLayerMutation, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "draft_config_write", Layer: ToolLayerMutation, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "propose_restart", Layer: ToolLayerMutation, Mutating: true}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "plan"})

	if got := toolNamesForTest(filtered); !reflect.DeepEqual(got, []string{"synthetic.read", "update_plan", "request_plan_approval"}) {
		t.Fatalf("filtered tools = %#v, want read plus exact plan artifact tools", got)
	}
	for _, name := range []string{"draft_config_write", "propose_restart"} {
		if !hiddenReasonForTest(snapshot, name, "mode_denied") {
			t.Fatalf("hidden tools = %#v, want %s mode_denied", snapshot.HiddenTools, name)
		}
	}
}

func TestPlanActiveAllowsExplorePlanAgentsAndBlocksExecutorMutation(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", Layer: ToolLayerCore, RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.search", Layer: ToolLayerDeferred, RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "update_plan", Layer: ToolLayerMutation, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", Layer: ToolLayerMutation, Mutating: true, RequiresApproval: true}},
	}

	cases := []struct {
		role string
		want []string
	}{
		{role: "explore", want: []string{"synthetic.read", "synthetic.search"}},
		{role: "plan", want: []string{"synthetic.read", "synthetic.search", "update_plan"}},
		{role: "verify", want: []string{"synthetic.read", "synthetic.search"}},
		{role: "execute", want: []string{"synthetic.read", "synthetic.search"}},
		{role: "host_child", want: []string{"synthetic.read", "synthetic.search"}},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "plan", AgentRole: tc.role})
			if got := toolNamesForTest(filtered); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("filtered tools = %#v, want %#v", got, tc.want)
			}
			if !hiddenReasonForTest(snapshot, "synthetic.write", "mode_denied") && !hiddenReasonForTest(snapshot, "synthetic.write", "agent_role_read_only") {
				t.Fatalf("hidden tools = %#v, want synthetic.write denied under plan_active", snapshot.HiddenTools)
			}
		})
	}
}

func TestApprovalToolShownAsSummaryOnly(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.approval_read", RiskLevel: ToolRiskHigh}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{Mode: "execute"})

	if len(filtered) != 0 {
		t.Fatalf("filtered tools = %#v, want approval schema hidden from prompt", toolNamesForTest(filtered))
	}
	if len(snapshot.HiddenTools) != 0 {
		t.Fatalf("hidden tools = %#v, want approval tool to remain dispatch-visible", snapshot.HiddenTools)
	}
	if len(snapshot.VisibleTools) != 1 || !snapshot.VisibleTools[0].SummaryOnly || snapshot.VisibleTools[0].Reason != "approval_required_schema_hidden" {
		t.Fatalf("visible tools = %#v, want summary-only approval tool", snapshot.VisibleTools)
	}
}

func TestSkillGovernanceFiltersToolSurface(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.read", RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.write", RiskLevel: ToolRiskMedium, Mutating: true}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.other", RiskLevel: ToolRiskLow}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{
		Mode: "execute",
		ActiveSkillPolicies: []SkillToolPolicy{{
			SkillName:    "synthetic.triage",
			AllowedTools: []string{"synthetic.read"},
			DeniedTools:  []string{"synthetic.write"},
			RiskCeiling:  "low",
		}},
	})

	if got := toolNamesForTest(filtered); !reflect.DeepEqual(got, []string{"synthetic.read"}) {
		t.Fatalf("filtered tools = %#v, want only synthetic.read", got)
	}
	wantReasons := map[string]string{
		"synthetic.write": "skill_denied_tool",
		"synthetic.other": "skill_allowed_tools",
	}
	for _, hidden := range snapshot.HiddenTools {
		delete(wantReasons, hidden.Name)
	}
	if len(wantReasons) != 0 {
		t.Fatalf("missing skill hidden reasons %#v; snapshot=%+v", wantReasons, snapshot)
	}
}

func TestSkillGovernanceRiskCeilingHidesHigherRiskTools(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.low", RiskLevel: ToolRiskLow}},
		&StaticTool{Meta: ToolMetadata{Name: "synthetic.medium", RiskLevel: ToolRiskMedium}},
	}

	filtered, snapshot := ApplyToolSurfacePolicy(tools, ToolSurfacePolicyOptions{
		Mode: "execute",
		ActiveSkillPolicies: []SkillToolPolicy{{
			SkillName:   "synthetic.readonly",
			RiskCeiling: "low",
		}},
	})

	if got := toolNamesForTest(filtered); !reflect.DeepEqual(got, []string{"synthetic.low"}) {
		t.Fatalf("filtered tools = %#v, want only low-risk tool", got)
	}
	if len(snapshot.HiddenTools) != 1 || snapshot.HiddenTools[0].Reason != "skill_risk_ceiling" {
		t.Fatalf("hidden tools = %+v, want skill_risk_ceiling", snapshot.HiddenTools)
	}
}

func hiddenReasonForTest(snapshot ToolSurfacePolicySnapshot, name, reason string) bool {
	for _, hidden := range snapshot.HiddenTools {
		if hidden.Name == name && hidden.Reason == reason {
			return true
		}
	}
	return false
}

func visibleSummaryOnlyForTest(snapshot ToolSurfacePolicySnapshot, name, reason string) bool {
	for _, visible := range snapshot.VisibleTools {
		if visible.Name == name && visible.Reason == reason && visible.SummaryOnly {
			return true
		}
	}
	return false
}
