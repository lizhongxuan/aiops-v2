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

func TestAgentRuntimeProfileNamedProfiles(t *testing.T) {
	cases := []struct {
		name          string
		profile       AgentRuntimeProfile
		wantProfile   string
		wantName      string
		wantAgentKind string
		wantSession   SessionType
		wantMode      Mode
		wantAllowed   []string
		wantForbidden []string
	}{
		{
			name:          "advisor",
			profile:       AdvisorRuntimeProfile(),
			wantProfile:   "advisor",
			wantName:      "advisor",
			wantAgentKind: "planner",
			wantSession:   SessionTypeWorkspace,
			wantMode:      ModeChat,
			wantAllowed:   []string{"answer_advisory", "use_public_research", "request_user_evidence"},
			wantForbidden: []string{"direct_host_command", "host_mutation"},
		},
		{
			name:          "evidence_rca",
			profile:       EvidenceRCARuntimeProfile(),
			wantProfile:   "evidence_rca",
			wantName:      "evidence_rca",
			wantAgentKind: "planner",
			wantSession:   SessionTypeWorkspace,
			wantMode:      ModeInspect,
			wantAllowed:   []string{"parse_user_evidence", "query_observability", "summarize_missing_evidence"},
			wantForbidden: []string{"direct_host_command", "host_mutation"},
		},
		{
			name:          "host_worker",
			profile:       HostWorkerRuntimeProfile(" host-a "),
			wantProfile:   "host_worker",
			wantName:      "host_worker",
			wantAgentKind: "worker",
			wantSession:   SessionTypeHost,
			wantMode:      ModeExecute,
			wantAllowed:   []string{"inspect_bound_host", "call_host_command_tool", "request_command_approval"},
			wantForbidden: []string{"operate_other_host", "bypass_host_command_tool"},
		},
		{
			name:          "host_manager",
			profile:       HostManagerRuntimeProfile(),
			wantProfile:   "host_manager",
			wantName:      "host_manager",
			wantAgentKind: "planner",
			wantSession:   SessionTypeWorkspace,
			wantMode:      ModeExecute,
			wantAllowed:   []string{"create_plan", "spawn_host_agent", "wait_host_report"},
			wantForbidden: []string{"direct_host_command", "direct_host_mutation"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.profile.Profile != tc.wantProfile || tc.profile.Name != tc.wantName || tc.profile.AgentKind != tc.wantAgentKind || tc.profile.SessionType != tc.wantSession || tc.profile.Mode != tc.wantMode {
				t.Fatalf("profile = %#v, want profile=%s name=%s kind=%s session=%s mode=%s", tc.profile, tc.wantProfile, tc.wantName, tc.wantAgentKind, tc.wantSession, tc.wantMode)
			}
			for _, allowed := range tc.wantAllowed {
				if !containsRuntimeProfileValue(tc.profile.AllowedActions, allowed) {
					t.Fatalf("%s allowed actions = %#v, missing %q", tc.name, tc.profile.AllowedActions, allowed)
				}
			}
			for _, forbidden := range tc.wantForbidden {
				if !containsRuntimeProfileValue(tc.profile.ForbiddenActions, forbidden) {
					t.Fatalf("%s forbidden actions = %#v, missing %q", tc.name, tc.profile.ForbiddenActions, forbidden)
				}
			}
			if !tc.profile.HasCapability(RuntimeCapabilityToolSurfacePolicy) || !tc.profile.HasCapability(RuntimeCapabilityApprovalGate) {
				t.Fatalf("%s missing base runtime capabilities: %#v", tc.name, tc.profile.Capabilities)
			}
		})
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
