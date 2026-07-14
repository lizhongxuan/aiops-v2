package runtimekernel

import (
	"testing"

	"aiops-v2/internal/resourcebinding"
)

func TestHostAgentSkillScopeDoesNotAutoInheritManagerSkillBody(t *testing.T) {
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostRef:    hostAgentTestHostRef("host-a"),
		HostID:     "host-a",
		ActionType: "inspect",
		TargetRef:  hostAgentTestHostRef("host-a"),
		TargetHost: "host-a",
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.inspect",
		ActivationMode:     SkillActivationModeManagerInherited,
		AllowedHostRefs:    []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
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

func TestHostAgentSkillScopeAllowsSameTypedHost(t *testing.T) {
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostRef:    hostAgentTestHostRef("host-a"),
		ActionType: "inspect",
		TargetRef:  hostAgentTestHostRef("host-a"),
		Risk:       RiskLevelReadOnly,
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.inspect",
		ActivationMode:     SkillActivationModeDelegated,
		AllowedHostRefs:    []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
		AllowedActionTypes: []string{"inspect"},
		MaxRisk:            RiskLevelLow,
	}})

	if !eval.ToolUseDecision.Allowed {
		t.Fatalf("tool decision = %#v, want same typed host allowed", eval.ToolUseDecision)
	}
	if len(eval.VisibleSkills) != 1 || eval.VisibleSkills[0].SkillID != "skill.generic.inspect" {
		t.Fatalf("visible skills = %#v, want typed same-host scope visible", eval.VisibleSkills)
	}
}

func TestHostAgentSkillScopeBlocksCrossHostToolUse(t *testing.T) {
	boundHost := hostAgentTestHostRef("shared-host")
	boundHost.Namespace = "cluster-a"
	crossHost := hostAgentTestHostRef("shared-host")
	crossHost.Namespace = "cluster-b"
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostRef:    boundHost,
		ActionType: "write_file",
		TargetRef:  crossHost,
		Risk:       RiskLevelMedium,
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.change",
		ActivationMode:     SkillActivationModeDelegated,
		AllowedHostRefs:    []resourcebinding.ResourceRef{boundHost},
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

func TestHostAgentSkillScopeRejectsTypedLegacyHostConflict(t *testing.T) {
	eval := EvaluateHostAgentSkillScopes(HostAgentSkillScopeRequest{
		HostRef:    hostAgentTestHostRef("host-a"),
		HostID:     "host-b",
		ActionType: "inspect",
		TargetRef:  hostAgentTestHostRef("host-a"),
	}, []HostAgentSkillScope{{
		SkillID:            "skill.generic.inspect",
		ActivationMode:     SkillActivationModeDelegated,
		AllowedHostRefs:    []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
		AllowedActionTypes: []string{"inspect"},
		MaxRisk:            RiskLevelLow,
	}})

	if eval.ToolUseDecision.Allowed {
		t.Fatalf("tool decision = %#v, want typed-vs-legacy conflict denied", eval.ToolUseDecision)
	}
	if eval.ToolUseDecision.HiddenReason != "typed_legacy_host_conflict" {
		t.Fatalf("hidden reason = %q, want typed_legacy_host_conflict", eval.ToolUseDecision.HiddenReason)
	}
	trace := findHostScopeTrace(t, eval.Trace, "skill", "skill.generic.inspect")
	if trace.Visible || trace.HiddenReason != "typed_legacy_host_conflict" {
		t.Fatalf("trace = %#v, want fail-closed typed-vs-legacy conflict", trace)
	}
}

func TestHostAgentMCPScopeHidesUnselectedServerInstructions(t *testing.T) {
	eval := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostRef:   hostAgentTestHostRef("host-a"),
		HostID:    "host-a",
		ServerID:  "mcp.selected",
		Resource:  "resource://selected/config",
		Operation: MCPResourceOperationRead,
	}, []HostAgentMCPScope{
		{
			ServerID:         "mcp.selected",
			Permission:       MCPPermissionRead,
			AllowedResources: []string{"resource://selected/config"},
			AllowedHostRefs:  []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
			AllowedHostScope: []string{"host-a"},
			InstructionMode:  MCPInstructionModeSparse,
			Reason:           "selected for this host task",
		},
		{
			ServerID:         "mcp.unselected",
			Permission:       MCPPermissionRead,
			AllowedResources: []string{"resource://unselected/config"},
			AllowedHostRefs:  []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
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
		HostRef:   hostAgentTestHostRef("host-a"),
		ServerID:  "mcp.generic",
		Resource:  "resource://host-a/config",
		Operation: MCPResourceOperationWrite,
	}, []HostAgentMCPScope{{
		ServerID:         "mcp.generic",
		Permission:       MCPPermissionReadWrite,
		AllowedResources: []string{"resource://host-a/config"},
		AllowedHostRefs:  []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
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

func TestHostAgentMCPScopeEnforcesTypedHostAndResourceScope(t *testing.T) {
	scopes := []HostAgentMCPScope{{
		ServerID:         "mcp.generic",
		Permission:       MCPPermissionRead,
		AllowedResources: []string{"resource://host-a/config"},
		AllowedHostRefs:  []resourcebinding.ResourceRef{hostAgentTestHostRef("host-a")},
		InstructionMode:  MCPInstructionModeSparse,
	}}

	allowed := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostRef:   hostAgentTestHostRef("host-a"),
		ServerID:  "mcp.generic",
		Resource:  "resource://host-a/config",
		Operation: MCPResourceOperationRead,
	}, scopes)
	if !allowed.ResourceDecision.Allowed {
		t.Fatalf("same-host resource decision = %#v, want allowed", allowed.ResourceDecision)
	}

	crossHost := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostRef:   hostAgentTestHostRef("host-b"),
		ServerID:  "mcp.generic",
		Resource:  "resource://host-a/config",
		Operation: MCPResourceOperationRead,
	}, scopes)
	if crossHost.ResourceDecision.Allowed {
		t.Fatalf("cross-host resource decision = %#v, want denied", crossHost.ResourceDecision)
	}
	if crossHost.ResourceDecision.HiddenReason != "host_out_of_scope" {
		t.Fatalf("cross-host hidden reason = %q, want host_out_of_scope", crossHost.ResourceDecision.HiddenReason)
	}

	wrongResource := EvaluateHostAgentMCPScope(HostAgentMCPScopeRequest{
		HostRef:   hostAgentTestHostRef("host-a"),
		ServerID:  "mcp.generic",
		Resource:  "resource://host-a/secret",
		Operation: MCPResourceOperationRead,
	}, scopes)
	if wrongResource.ResourceDecision.Allowed {
		t.Fatalf("out-of-scope resource decision = %#v, want denied", wrongResource.ResourceDecision)
	}
	if wrongResource.ResourceDecision.HiddenReason != "resource_out_of_scope" {
		t.Fatalf("out-of-scope resource hidden reason = %q, want resource_out_of_scope", wrongResource.ResourceDecision.HiddenReason)
	}
}

func hostAgentTestHostRef(id string) resourcebinding.ResourceRef {
	return resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: id}
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
