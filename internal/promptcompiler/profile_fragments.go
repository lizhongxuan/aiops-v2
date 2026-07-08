package promptcompiler

import "strings"

const (
	PromptProfileAdvisor       = "advisor"
	PromptProfileEvidenceRCA   = "evidence_rca"
	PromptProfileHostWorker    = "host_worker"
	PromptProfileHostManager   = "host_manager"
	PromptProfileWorkflowAgent = "workflow_agent"
)

type runtimeProfileFragment struct {
	Profile string
	Lines   []string
}

func buildProfileFragment(profile string, hostContext string) string {
	fragment, ok := profileFragment(normalizePromptProfile(profile), strings.TrimSpace(hostContext))
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Runtime Profile Fragment")
	b.WriteString("\n\nprofile: ")
	b.WriteString(fragment.Profile)
	for _, line := range fragment.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("\n- ")
		b.WriteString(line)
	}
	return b.String()
}

func profileFragment(profile string, hostContext string) (runtimeProfileFragment, bool) {
	switch profile {
	case PromptProfileAdvisor:
		return runtimeProfileFragment{
			Profile: PromptProfileAdvisor,
			Lines: []string{
				"Answer advisory questions from provided context and safe public sources when current external facts are required.",
				"State when no host, OpsGraph, Coroot, or OpsManual context was explicitly mentioned or inspected.",
				"Do not run host commands.",
			},
		}, true
	case PromptProfileEvidenceRCA:
		return runtimeProfileFragment{
			Profile: PromptProfileEvidenceRCA,
			Lines: []string{
				"Build a concise incident timeline from user evidence and read-only tool evidence.",
				"Separate observed facts, hypotheses, missing evidence, and assumptions.",
				"Do not run host commands unless this turn has explicit host scope and visible host tools.",
			},
		}, true
	case PromptProfileHostWorker:
		return runtimeProfileFragment{
			Profile: PromptProfileHostWorker,
			Lines: []string{
				"Operate only within the bound host scope: " + firstNonEmptyRuntimeContractLine(hostContext, "runtime-selected host") + ".",
				"Use read-only inspection before any risky action.",
				"Mutations require runtime approval and verification after the action.",
			},
		}, true
	case PromptProfileHostManager:
		return runtimeProfileFragment{
			Profile: PromptProfileHostManager,
			Lines: []string{
				"Create a compact plan for complex host work before delegation.",
				"Do not run host commands directly.",
				"Delegate clear sub-tasks to host-bound child agents and wait for their results before synthesis.",
			},
		}, true
	case PromptProfileWorkflowAgent:
		return runtimeProfileFragment{
			Profile: PromptProfileWorkflowAgent,
			Lines: []string{
				"Inspect the current Runner Workflow snapshot before proposing edits.",
				"Separate current graph facts, assumptions, and requested changes.",
				"Produce a workflow edit plan before any patch.",
				"Propose one minimal workflow patch at a time and wait for confirmation before applying it.",
				"After applying a patch, use workflow.describe and effect status before moving to the next patch.",
				"Do not publish or execute workflows.",
				"Do not run host commands.",
			},
		}, true
	default:
		return runtimeProfileFragment{}, false
	}
}

func normalizePromptProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "chat_advisory":
		return PromptProfileAdvisor
	case "evidence-rca":
		return PromptProfileEvidenceRCA
	case "host-worker", "host_agent_full_runtime":
		return PromptProfileHostWorker
	case "host-manager", "manager_agent_full_runtime":
		return PromptProfileHostManager
	case "workflow-agent", "workflow", "workflow_planner", "workflow_agent_runtime":
		return PromptProfileWorkflowAgent
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}
