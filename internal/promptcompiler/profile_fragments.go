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
				"Treat the immutable Workflow turn artifact as the only tenant, workflow, revision, selection, neighborhood, and environment scope for this turn.",
				"Read only the needed context with workflow.get_context; use selection or neighborhood first and request paged full context only when necessary.",
				"Ground every action in workflow.get_action_catalog and record the returned catalog version and action IDs; never invent an action or fallback script.",
				"If a critical target, threshold, production parameter, secret reference, or environment is ambiguous, ask one concise clarification instead of guessing.",
				"When the catalog has no permitted match, report capability_gap and stop without creating placeholder operations.",
				"Create typed WorkflowPlanIR, call workflow.compile_plan, then pass only its normalized operations to workflow.preview_patch.",
				"Use typed compile, preview, and validation diagnostics to repair only affected PlanIR paths while preserving user constraints, pinned layout, verified references, and untouched semantics; allow at most two repair rounds in this Runtime turn, then stop with concrete blockers and retain the Candidate.",
				"Cancellation ends repair immediately; do not issue another mutation or report completed after cancellation.",
				"Apply only the latest reviewed preview with workflow.apply_patch after Runtime approval, then use its persisted receipt and workflow.validate as truth.",
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
