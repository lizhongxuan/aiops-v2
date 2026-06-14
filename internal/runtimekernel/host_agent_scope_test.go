package runtimekernel

import "testing"

func TestHostAgentSkillScopeDoesNotAutoInheritManagerSkillBody(t *testing.T) {
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostID:     "host-a",
		ActionType: "inspect",
		TargetHost: "host-a",
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.inspect",
		ActivationMode:     SkillActivationModeManagerInherited,
		AllowedHostScope:   []string{"host-a"},
		AllowedActionTypes: []string{"inspect"},
		MaxRisk:            RiskLevelLow,
		Reason:             "manager loaded this skill for its own planning context",
	}})

	if len(eval.VisibleSkills) != 0 {
		t.Fatalf("visible skills = %#v, want no manager-inherited skill body visible to host agent", eval.VisibleSkills)
	}
	trace := findHostScopeTrace(t, eval.Trace, "skill", "skill.generic.inspect")
	if trace.Visible {
		t.Fatalf("trace = %#v, want manager-inherited skill hidden", trace)
	}
	if trace.HiddenReason != "manager_skill_body_not_inherited" {
		t.Fatalf("hidden reason = %q, want manager_skill_body_not_inherited", trace.HiddenReason)
	}
}

func TestHostAgentSkillScopeBlocksCrossHostToolUse(t *testing.T) {
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostID:     "host-a",
		ActionType: "write_file",
		TargetHost: "host-b",
		Risk:       RiskLevelMedium,
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.change",
		ActivationMode:     SkillActivationModeDelegated,
		AllowedHostScope:   []string{"host-a"},
		AllowedActionTypes: []string{"inspect", "write_file"},
		MaxRisk:            RiskLevelHigh,
		Reason:             "host-bound change task",
	}})

	if eval.ToolUseDecision.Allowed {
		t.Fatalf("tool decision = %#v, want cross-host tool use blocked", eval.ToolUseDecision)
	}
	if eval.ToolUseDecision.HiddenReason != "target_host_out_of_scope" {
		t.Fatalf("tool hidden reason = %q, want target_host_out_of_scope", eval.ToolUseDecision.HiddenReason)
	}
	trace := findHostScopeTrace(t, eval.Trace, "skill", "skill.generic.change")
	if trace.Visible {
		t.Fatalf("trace = %#v, want skill hidden for cross-host request", trace)
	}
	if trace.HiddenReason != "target_host_out_of_scope" {
		t.Fatalf("hidden reason = %q, want target_host_out_of_scope", trace.HiddenReason)
	}
}

func TestHostAgentMCPScopeHidesUnselectedServerInstructions(t *testing.T) {
	eval := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostID:    "host-a",
		ServerID:  "mcp.selected",
		Resource:  "resource://selected/config",
		Operation: MCPResourceOperationRead,
	}, []HostAgentMCPScope{
		{
			ServerID:         "mcp.selected",
			Permission:       MCPPermissionRead,
			AllowedResources: []string{"resource://selected/config"},
			AllowedHostScope: []string{"host-a"},
			InstructionMode:  MCPInstructionModeSparse,
			Reason:           "selected for this host task",
		},
		{
			ServerID:         "mcp.unselected",
			Permission:       MCPPermissionRead,
			AllowedResources: []string{"resource://unselected/config"},
			AllowedHostScope: []string{"host-a"},
			InstructionMode:  MCPInstructionModeHidden,
			Reason:           "not selected for this host task",
		},
	})

	if len(eval.VisibleServers) != 1 || eval.VisibleServers[0].ServerID != "mcp.selected" {
		t.Fatalf("visible MCP servers = %#v, want only selected server instructions", eval.VisibleServers)
	}
	trace := findHostScopeTrace(t, eval.Trace, "mcp", "mcp.unselected")
	if trace.Visible {
		t.Fatalf("trace = %#v, want unselected MCP instructions hidden", trace)
	}
	if trace.HiddenReason != "mcp_instructions_not_selected" {
		t.Fatalf("hidden reason = %q, want mcp_instructions_not_selected", trace.HiddenReason)
	}
}

func TestHostAgentMCPScopeRequiresApprovalForReadWriteResource(t *testing.T) {
	eval := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostID:    "host-a",
		ServerID:  "mcp.generic",
		Resource:  "resource://host-a/config",
		Operation: MCPResourceOperationWrite,
	}, []HostAgentMCPScope{{
		ServerID:         "mcp.generic",
		Permission:       MCPPermissionReadWrite,
		AllowedResources: []string{"resource://host-a/config"},
		AllowedHostScope: []string{"host-a"},
		RequiresApproval: true,
		InstructionMode:  MCPInstructionModeSparse,
		Reason:           "resource can change host state",
	}})

	if !eval.ResourceDecision.Allowed {
		t.Fatalf("resource decision = %#v, want scoped read-write resource allowed after approval gate", eval.ResourceDecision)
	}
	if !eval.ResourceDecision.RequiresApproval {
		t.Fatalf("resource decision = %#v, want approval required", eval.ResourceDecision)
	}
	if eval.ResourceDecision.HiddenReason != "" {
		t.Fatalf("resource hidden reason = %q, want empty", eval.ResourceDecision.HiddenReason)
	}
	trace := findHostScopeTrace(t, eval.Trace, "mcp", "mcp.generic")
	if !trace.Visible {
		t.Fatalf("trace = %#v, want MCP server visible", trace)
	}
}

func findHostScopeTrace(t *testing.T, traces []HostAgentScopeTrace, kind, id string) HostAgentScopeTrace {
	t.Helper()
	for _, trace := range traces {
		if trace.Kind == kind && trace.ID == id {
			return trace
		}
	}
	t.Fatalf("missing %s trace for %s in %#v", kind, id, traces)
	return HostAgentScopeTrace{}
}
