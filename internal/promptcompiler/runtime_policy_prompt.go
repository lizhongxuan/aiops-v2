package promptcompiler

import (
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Layer 4: Runtime Policy Prompt — mode-specific policy constraints
// ---------------------------------------------------------------------------

// buildRuntimePolicyPrompt compiles Layer 4: the runtime policy prompt
// containing mode-specific constraints and any custom policy overrides.
func (c *PromptCompilerImpl) buildRuntimePolicyPrompt(ctx CompileContext) (RuntimePolicyPrompt, error) {
	return RuntimePolicyPrompt{
		Content: buildRuntimeStateFragment(ctx),
		Mode:    ctx.Mode,
	}, nil
}

func buildRuntimeStateFragment(ctx CompileContext) string {
	lines := []string{
		"# Runtime State",
		"- profile: " + firstNonEmptyRuntimeContractLine(normalizePromptProfile(ctx.Profile), "default"),
		"- mode: " + firstNonEmptyRuntimeContractLine(strings.TrimSpace(ctx.Mode), "unknown"),
		"- mutation: " + runtimeMutationState(ctx.Mode),
		"- host_scope: " + runtimeHostScopeState(ctx.HostContext),
		"- web: " + runtimeStateValue(ctx.WebState, "not_requested"),
		"- ops_graph: " + runtimeStateValue(ctx.OpsGraphState, "not_requested"),
		"- coroot: " + runtimeStateValue(ctx.CorootState, "not_requested"),
		"- ops_manus: " + runtimeStateValue(ctx.OpsManusState, "not_requested"),
		"- pending_approvals: " + runtimeCountState(ctx.PendingApprovals),
		"- pending_evidence: " + runtimeCountState(ctx.PendingEvidence),
		"- visible_tool_fingerprint: " + runtimeStateValue(ctx.VisibleToolFingerprint, "unknown"),
		"- user_constraints: " + runtimeListState(ctx.UserConstraints),
		"- timeout_recovery_state: " + runtimeStateValue(ctx.TimeoutRecoveryState, "none"),
	}
	if policy := strings.TrimSpace(ctx.RuntimePolicy); policy != "" {
		lines = append(lines, "- policy_override: "+policy)
	}
	if answerStyle := strings.TrimSpace(ctx.AnswerStyle); answerStyle != "" {
		lines = append(lines, "- answer_style: "+answerStyle)
	}
	if strings.TrimSpace(string(ctx.TaskDepth.Level)) != "" {
		lines = append(lines, "- task_depth: "+strings.TrimSpace(string(ctx.TaskDepth.Level)))
	}
	if ctx.TaskDepth.RequiresPlan {
		lines = append(lines, "- requires_plan: true")
	}
	if ctx.TaskDepth.RequiresEvidence {
		lines = append(lines, "- requires_evidence: true")
	}
	if ctx.TaskDepth.AnalysisOnly {
		lines = append(lines, "- analysis_only: true")
	}
	if effort := strings.TrimSpace(ctx.ReasoningEffort); effort != "" {
		lines = append(lines, "- reasoning_effort: "+effort)
	}
	if len(ctx.ToolDelta.NewlyAvailable) > 0 || len(ctx.ToolDelta.NewlyAvailablePacks) > 0 || len(ctx.ToolDelta.TemporarilyUnavailable) > 0 || len(ctx.ToolDelta.ApprovalRequired) > 0 {
		lines = append(lines, "- tool_delta: present")
	}
	if len(ctx.EvidenceReminders) > 0 {
		lines = append(lines, "- pending_evidence: present")
	}
	if len(ctx.ProtocolState.Items) > 0 || ctx.ProtocolState.PlanMode != nil || ctx.ProtocolState.TaskTodo != nil || ctx.ProtocolState.FailureSwitchPath != nil {
		lines = append(lines, "- protocol_state: present")
	}
	return strings.Join(lines, "\n")
}

func runtimeStateValue(value string, fallback string) string {
	return firstNonEmptyRuntimeContractLine(strings.TrimSpace(value), fallback)
}

func runtimeCountState(count int) string {
	if count <= 0 {
		return "0"
	}
	return strconv.Itoa(count)
}

func runtimeListState(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return "none"
	}
	return strings.Join(cleaned, "; ")
}

func runtimeMutationState(mode Mode) string {
	switch mode {
	case "execute":
		return "approval_required"
	case "chat", "inspect", "plan":
		return "read-only"
	default:
		return "read_only_default"
	}
}

func runtimeHostScopeState(hostContext string) string {
	if strings.TrimSpace(hostContext) == "" {
		return "none"
	}
	return "bound"
}
