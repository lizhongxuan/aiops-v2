package policyengine

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Tool classification helpers — pattern-based tool name classification.
// ---------------------------------------------------------------------------

// readOnlyPatterns identifies tools that only read state.
var readOnlyPatterns = []string{
	"read", "list", "search", "get", "show", "status", "info",
	"ps", "df", "top", "cat", "head", "tail", "ls",
}

// mutationPatterns identifies tools that modify state.
var mutationPatterns = []string{
	"write", "delete", "remove", "create", "update",
	"exec", "run", "restart", "stop", "kill",
}

// webSearchPatterns identifies web search tools.
var webSearchPatterns = []string{
	"web", "search",
}

// containsAny reports whether name contains any of the given patterns (case-insensitive).
func containsAny(name string, patterns []string) bool {
	lower := strings.ToLower(name)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isReadOnly reports whether the tool name matches read-only patterns.
func isReadOnly(name string) bool {
	return containsAny(name, readOnlyPatterns)
}

// isMutation reports whether the tool name matches mutation patterns.
func isMutation(name string) bool {
	return containsAny(name, mutationPatterns)
}

// isWebSearch reports whether the tool name matches web search patterns.
func isWebSearch(name string) bool {
	return containsAny(name, webSearchPatterns)
}

// ---------------------------------------------------------------------------
// ChatModePolicy — chat mode: allows read-only + web search, denies mutation.
// ---------------------------------------------------------------------------

// ChatModePolicy implements ModePolicy for chat mode.
type ChatModePolicy struct{}

// CheckCapability determines whether the given tool is permitted in chat mode.
func (p *ChatModePolicy) CheckCapability(toolName string, toolKind CapabilityKind) PolicyDecision {
	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode does not allow mutation operations",
		}
	}

	if toolKind == KindSkill {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindTool && isReadOnly(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindTool {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode only allows read-only tools and web search",
		}
	}

	if toolKind == KindUISurface || toolKind == KindModeRule {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindMCPTool {
		if isReadOnly(toolName) || isWebSearch(toolName) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode only allows read-only MCP tools",
		}
	}

	if toolKind == KindWorkspace {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode does not allow workspace operations",
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}

// ---------------------------------------------------------------------------
// InspectModePolicy — inspect mode: allows read/list/search/readonly shell, denies mutation.
// ---------------------------------------------------------------------------

// InspectModePolicy implements ModePolicy for inspect mode.
type InspectModePolicy struct{}

// CheckCapability determines whether the given tool is permitted in inspect mode.
func (p *InspectModePolicy) CheckCapability(toolName string, toolKind CapabilityKind) PolicyDecision {
	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode does not allow mutation operations",
		}
	}

	if toolKind == KindSkill {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isReadOnly(toolName) || isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindUISurface || toolKind == KindModeRule {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindTool || toolKind == KindMCPTool {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode only allows read-only and search tools",
		}
	}

	if toolKind == KindWorkspace {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode does not allow workspace operations",
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}

// ---------------------------------------------------------------------------
// PlanModePolicy — plan mode: allows inspect + plan capabilities, denies direct mutation.
// ---------------------------------------------------------------------------

// PlanModePolicy implements ModePolicy for plan mode.
type PlanModePolicy struct{}

// planPatterns identifies plan-related tools.
var planPatterns = []string{
	"plan", "draft", "propose", "schedule", "preview",
}

// isPlanTool reports whether the tool name matches plan-related patterns.
func isPlanTool(name string) bool {
	return containsAny(name, planPatterns)
}

// CheckCapability determines whether the given tool is permitted in plan mode.
func (p *PlanModePolicy) CheckCapability(toolName string, toolKind CapabilityKind) PolicyDecision {
	if isPlanTool(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode does not allow direct mutation execution",
		}
	}

	if toolKind == KindSkill {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isReadOnly(toolName) || isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindUISurface || toolKind == KindModeRule {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if toolKind == KindTool || toolKind == KindMCPTool {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode only allows read-only, search, and plan tools",
		}
	}

	if toolKind == KindWorkspace {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode does not allow workspace dispatch",
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}

// ---------------------------------------------------------------------------
// ExecuteModePolicy — execute mode: allows all, mutation gets NeedApproval.
// ---------------------------------------------------------------------------

// ExecuteModePolicy implements ModePolicy for execute mode.
type ExecuteModePolicy struct{}

// CheckCapability determines whether the given tool is permitted in execute mode.
func (p *ExecuteModePolicy) CheckCapability(toolName string, toolKind CapabilityKind) PolicyDecision {
	_ = toolKind // execute mode doesn't differentiate by kind for mutation
	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionNeedApproval,
			Reason: "execute mode requires approval for mutation operations",
			Approval: &ApprovalRequest{
				ToolName: toolName,
				Reason:   "mutation operation requires approval",
			},
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}

// ---------------------------------------------------------------------------
// NewDefaultModePolicies returns a map of all four mode policies.
// ---------------------------------------------------------------------------

// Mode constants (mirrors runtimekernel.Mode values).
const (
	ModeChat    Mode = "chat"
	ModeInspect Mode = "inspect"
	ModePlan    Mode = "plan"
	ModeExecute Mode = "execute"
)

// NewDefaultModePolicies returns a map of all four canonical mode policies.
func NewDefaultModePolicies() map[Mode]ModePolicy {
	return map[Mode]ModePolicy{
		ModeChat:    &ChatModePolicy{},
		ModeInspect: &InspectModePolicy{},
		ModePlan:    &PlanModePolicy{},
		ModeExecute: &ExecuteModePolicy{},
	}
}
