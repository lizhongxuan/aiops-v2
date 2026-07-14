package runtimekernel

import "strings"

const (
	RuntimePromptProfileAdvisor       = "advisor"
	RuntimePromptProfileEvidenceRCA   = "evidence_rca"
	RuntimePromptProfileHostWorker    = "host_worker"
	RuntimePromptProfileHostManager   = "host_manager"
	RuntimePromptProfileWorkflowAgent = "workflow_agent"
)

type RuntimeCapability string

const (
	RuntimeCapabilityPromptCompiler    RuntimeCapability = "prompt_compiler"
	RuntimeCapabilityContextGovernance RuntimeCapability = "context_governance"
	RuntimeCapabilityContextBudget     RuntimeCapability = "context_budget"
	RuntimeCapabilityCompact           RuntimeCapability = "compact"
	RuntimeCapabilitySpill             RuntimeCapability = "spill"
	RuntimeCapabilityArtifactRead      RuntimeCapability = "artifact_read"
	RuntimeCapabilitySkillsDiscovery   RuntimeCapability = "skills_discovery"
	RuntimeCapabilityMCPDiscovery      RuntimeCapability = "mcp_discovery"
	RuntimeCapabilityToolSurfacePolicy RuntimeCapability = "tool_surface_policy"
	RuntimeCapabilityApprovalGate      RuntimeCapability = "approval_gate"
	RuntimeCapabilityEvidenceGate      RuntimeCapability = "evidence_gate"
	RuntimeCapabilityCompletionGate    RuntimeCapability = "completion_gate"
	RuntimeCapabilityTrace             RuntimeCapability = "trace"
	RuntimeCapabilityAudit             RuntimeCapability = "audit"
	RuntimeCapabilityObservability     RuntimeCapability = "observability"
	RuntimeCapabilityFailureRecovery   RuntimeCapability = "failure_recovery"
)

type AgentRuntimeProfile struct {
	Name             string
	Profile          string
	AgentKind        string
	SessionType      SessionType
	Mode             Mode
	BoundHostID      string
	Capabilities     map[RuntimeCapability]bool
	AllowedActions   []string
	ForbiddenActions []string
}

func BaseAgentRuntimeProfile() AgentRuntimeProfile {
	capabilities := map[RuntimeCapability]bool{}
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
		capabilities[capability] = true
	}
	return AgentRuntimeProfile{Name: "base_agent_runtime", Capabilities: capabilities}
}

func AdvisorRuntimeProfile() AgentRuntimeProfile {
	profile := BaseAgentRuntimeProfile()
	profile.Name = RuntimePromptProfileAdvisor
	profile.Profile = RuntimePromptProfileAdvisor
	profile.AgentKind = "planner"
	profile.SessionType = SessionTypeWorkspace
	profile.Mode = ModeChat
	profile.AllowedActions = []string{
		"answer_advisory",
		"use_public_research",
		"request_user_evidence",
		"summarize_limitations",
	}
	profile.ForbiddenActions = []string{"direct_host_command", "host_mutation"}
	return profile
}

func EvidenceRCARuntimeProfile() AgentRuntimeProfile {
	profile := BaseAgentRuntimeProfile()
	profile.Name = RuntimePromptProfileEvidenceRCA
	profile.Profile = RuntimePromptProfileEvidenceRCA
	profile.AgentKind = "planner"
	profile.SessionType = SessionTypeWorkspace
	profile.Mode = ModeInspect
	profile.AllowedActions = []string{
		"parse_user_evidence",
		"query_observability",
		"request_missing_evidence",
		"summarize_missing_evidence",
	}
	profile.ForbiddenActions = []string{"direct_host_command", "host_mutation"}
	return profile
}

func ManagerAgentRuntimeProfile() AgentRuntimeProfile {
	profile := BaseAgentRuntimeProfile()
	profile.Name = "manager_agent_full_runtime"
	profile.Profile = RuntimePromptProfileHostManager
	profile.AgentKind = "planner"
	profile.SessionType = SessionTypeWorkspace
	profile.Mode = ModeExecute
	profile.AllowedActions = []string{
		"extract_ops_semantics",
		"create_plan",
		"revise_plan",
		"spawn_host_agent",
		"send_host_subtask",
		"wait_host_report",
		"summarize_result",
	}
	profile.ForbiddenActions = []string{"direct_host_command", "direct_host_mutation"}
	return profile
}

func HostManagerRuntimeProfile() AgentRuntimeProfile {
	profile := ManagerAgentRuntimeProfile()
	profile.Name = RuntimePromptProfileHostManager
	return profile
}

func HostAgentRuntimeProfile(hostID string) AgentRuntimeProfile {
	profile := BaseAgentRuntimeProfile()
	profile.Name = "host_agent_full_runtime"
	profile.Profile = RuntimePromptProfileHostWorker
	profile.AgentKind = "worker"
	profile.SessionType = SessionTypeHost
	profile.Mode = ModeExecute
	profile.BoundHostID = strings.TrimSpace(hostID)
	profile.AllowedActions = []string{
		"inspect_bound_host",
		"plan_bound_host_subtask",
		"use_host_scoped_tools",
		"call_host_command_tool",
		"request_command_approval",
		"collect_evidence",
		"return_host_task_report",
	}
	profile.ForbiddenActions = []string{
		"operate_other_host",
		"read_other_host_agent_private_context",
		"bypass_host_command_tool",
		"directly_change_manager_plan",
	}
	return profile
}

func HostWorkerRuntimeProfile(hostID string) AgentRuntimeProfile {
	profile := HostAgentRuntimeProfile(hostID)
	profile.Name = RuntimePromptProfileHostWorker
	return profile
}

func WorkflowAgentRuntimeProfile() AgentRuntimeProfile {
	profile := BaseAgentRuntimeProfile()
	profile.Name = RuntimePromptProfileWorkflowAgent
	profile.Profile = RuntimePromptProfileWorkflowAgent
	profile.AgentKind = "workflow_planner"
	profile.SessionType = SessionTypeWorkspace
	profile.Mode = ModePlan
	profile.AllowedActions = []string{
		"create_workflow_from_requirement",
		"inspect_workflow",
		"inspect_workflow_step",
		"inspect_action_catalog",
		"create_workflow_edit_plan",
		"propose_workflow_patch",
		"validate_workflow_patch",
		"preview_workflow_patch",
		"describe_workflow_after_patch",
		"detect_noop_or_duplicate_patch",
		"apply_confirmed_workflow_patch",
		"undo_last_ai_patch",
		"save_workflow_draft",
		"propose_ops_manual_candidate",
		"propose_ops_manual_update",
	}
	profile.ForbiddenActions = []string{
		"publish_workflow",
		"execute_workflow",
		"direct_host_command",
		"host_mutation",
		"bypass_workflow_editor_api",
		"modify_ops_manual_without_preview",
		"auto_apply_unapproved_patch",
		"replace_full_graph_without_confirmation",
		"bypass_workflow_review",
		"mark_ops_manual_verified",
		"ordinary_chat_add_workflow",
	}
	return profile
}

func (p AgentRuntimeProfile) HasCapability(capability RuntimeCapability) bool {
	return p.Capabilities[capability]
}
