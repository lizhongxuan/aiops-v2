package runtimekernel

import "testing"

func TestHostAgentRuntimeProfileInheritsBaseCapabilities(t *testing.T) {
	profile := HostAgentRuntimeProfile("host-a")
	for _, capability := range []RuntimeCapability{
		RuntimeCapabilityPromptCompiler,
		RuntimeCapabilityContextGovernance,
		RuntimeCapabilityContextBudget,
		RuntimeCapabilityCompact,
		RuntimeCapabilitySpill,
		RuntimeCapabilityArtifactRead,
		RuntimeCapabilitySkillsDiscovery,
		RuntimeCapabilityMCPDiscovery,
		RuntimeCapabilityToolSurfacePolicy,
		RuntimeCapabilityApprovalGate,
		RuntimeCapabilityEvidenceGate,
		RuntimeCapabilityCompletionGate,
		RuntimeCapabilityTrace,
		RuntimeCapabilityAudit,
		RuntimeCapabilityObservability,
		RuntimeCapabilityFailureRecovery,
	} {
		if !profile.HasCapability(capability) {
			t.Fatalf("host profile missing base capability %q", capability)
		}
	}
	if profile.Name != "host_agent_full_runtime" || profile.BoundHostID != "host-a" || profile.SessionType != SessionTypeHost || profile.Mode != ModeExecute {
		t.Fatalf("host profile = %#v, want full runtime host execute profile", profile)
	}
}

func TestManagerAgentRuntimeProfileForbidsDirectHostMutation(t *testing.T) {
	profile := ManagerAgentRuntimeProfile()
	if !containsRuntimeProfileValue(profile.ForbiddenActions, "direct_host_command") || !containsRuntimeProfileValue(profile.ForbiddenActions, "direct_host_mutation") {
		t.Fatalf("manager forbidden actions = %#v, want direct host command and mutation forbidden", profile.ForbiddenActions)
	}
	if profile.SessionType != SessionTypeWorkspace || profile.AgentKind != "planner" {
		t.Fatalf("manager profile = %#v, want workspace planner", profile)
	}
}

func TestHostAgentRuntimeProfileBindsExactlyOneHostScope(t *testing.T) {
	profile := HostAgentRuntimeProfile(" host-a ")
	if profile.BoundHostID != "host-a" {
		t.Fatalf("BoundHostID = %q, want trimmed host-a", profile.BoundHostID)
	}
	for _, forbidden := range []string{"operate_other_host", "read_other_host_agent_private_context", "bypass_host_command_tool", "directly_change_manager_plan"} {
		if !containsRuntimeProfileValue(profile.ForbiddenActions, forbidden) {
			t.Fatalf("host forbidden actions = %#v, missing %q", profile.ForbiddenActions, forbidden)
		}
	}
}

func containsRuntimeProfileValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
