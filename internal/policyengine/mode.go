package policyengine

import (
	"encoding/json"
	"strings"

	"aiops-v2/internal/terminalpolicy"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Tool classification helpers — pattern-based tool name classification.
// ---------------------------------------------------------------------------

// readOnlyPatterns identifies tools that only read state.
var readOnlyPatterns = []string{
	"read", "list", "search", "get", "show", "status", "info",
	"browse", "fetch",
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

func isTerminalCommandTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "exec_command", "terminal_command", "shell_command":
		return true
	default:
		return false
	}
}

func isReadOnlyTerminalCommand(args []byte) bool {
	req, ok := terminalCommandRequestFromArgs(args)
	if !ok {
		return false
	}
	return terminalpolicy.IsReadOnlyCommand(req.command, req.args)
}

type terminalCommandRequest struct {
	command string
	args    []string
}

func terminalCommandFromArgs(args []byte) (string, bool) {
	req, ok := terminalCommandRequestFromArgs(args)
	return req.command, ok
}

func terminalCommandRequestFromArgs(args []byte) (terminalCommandRequest, bool) {
	var payload struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cmd     string   `json:"cmd"`
	}
	if len(args) == 0 || json.Unmarshal(args, &payload) != nil {
		return terminalCommandRequest{}, false
	}
	command := strings.TrimSpace(payload.Command)
	commandArgs := append([]string(nil), payload.Args...)
	if command != "" && len(commandArgs) == 0 {
		parsedCommand, parsedArgs, ok := terminalpolicy.SplitCommandLine(command)
		if !ok {
			return terminalCommandRequest{}, false
		}
		command = parsedCommand
		commandArgs = parsedArgs
	}
	if command == "" {
		parsedCommand, parsedArgs, ok := terminalpolicy.SplitCommandLine(payload.Cmd)
		if !ok {
			return terminalCommandRequest{}, false
		}
		command = parsedCommand
		commandArgs = parsedArgs
	}
	if command == "" || hasTerminalShellOperators(command) {
		return terminalCommandRequest{}, false
	}
	for _, arg := range commandArgs {
		if strings.ContainsAny(arg, "\x00\n\r") {
			return terminalCommandRequest{}, false
		}
	}
	return terminalCommandRequest{command: command, args: commandArgs}, true
}

func hasTerminalShellOperators(value string) bool {
	return strings.ContainsAny(value, ";&|<>`$")
}

// ---------------------------------------------------------------------------
// ChatModePolicy — chat mode: allows read-only + web search, denies mutation.
// ---------------------------------------------------------------------------

// ChatModePolicy implements ModePolicy for chat mode.
type ChatModePolicy struct{}

// CheckTool determines whether the given tool is permitted in chat mode.
func (p *ChatModePolicy) CheckTool(input PolicyInput) PolicyDecision {
	toolName := normalizeToolName(input)
	if isTerminalCommandTool(toolName) {
		req, ok := terminalCommandRequestFromArgs(input.Arguments)
		if !ok {
			return PolicyDecision{
				Action: PolicyActionDeny,
				Reason: "terminal command uses unsupported shell syntax or is missing a command",
			}
		}
		if terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionNeedApproval,
			Reason: "chat mode requires approval for local terminal commands that are not classified read-only",
			Approval: &ApprovalRequest{
				ToolName: toolName,
				Reason:   "local terminal command requires approval",
			},
		}
	}
	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode does not allow mutation operations",
		}
	}

	if isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isReadOnly(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isMCPTool(input) {
		if isReadOnly(toolName) || isWebSearch(toolName) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "chat mode only allows read-only MCP tools",
		}
	}

	return PolicyDecision{
		Action: PolicyActionDeny,
		Reason: "chat mode only allows read-only tools and web search",
	}
}

// ---------------------------------------------------------------------------
// InspectModePolicy — inspect mode: allows read/list/search/readonly shell, denies mutation.
// ---------------------------------------------------------------------------

// InspectModePolicy implements ModePolicy for inspect mode.
type InspectModePolicy struct{}

// CheckTool determines whether the given tool is permitted in inspect mode.
func (p *InspectModePolicy) CheckTool(input PolicyInput) PolicyDecision {
	toolName := normalizeToolName(input)
	if isTerminalCommandTool(toolName) {
		req, ok := terminalCommandRequestFromArgs(input.Arguments)
		if !ok {
			return PolicyDecision{
				Action: PolicyActionDeny,
				Reason: "terminal command uses unsupported shell syntax or is missing a command",
			}
		}
		if terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode only allows read-only terminal commands",
		}
	}
	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode does not allow mutation operations",
		}
	}

	if isReadOnly(toolName) || isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isMCPTool(input) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "inspect mode only allows read-only and search tools",
		}
	}
	return PolicyDecision{
		Action: PolicyActionDeny,
		Reason: "inspect mode only allows read-only and search tools",
	}
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

// CheckTool determines whether the given tool is permitted in plan mode.
func (p *PlanModePolicy) CheckTool(input PolicyInput) PolicyDecision {
	toolName := normalizeToolName(input)
	if isTerminalCommandTool(toolName) {
		req, ok := terminalCommandRequestFromArgs(input.Arguments)
		if !ok {
			return PolicyDecision{
				Action: PolicyActionDeny,
				Reason: "terminal command uses unsupported shell syntax or is missing a command",
			}
		}
		if terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode only allows read-only terminal commands",
		}
	}
	if isPlanTool(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isMutation(toolName) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode does not allow direct mutation execution",
		}
	}

	if isReadOnly(toolName) || isWebSearch(toolName) {
		return PolicyDecision{Action: PolicyActionAllow}
	}

	if isMCPTool(input) {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "plan mode only allows read-only, search, and plan tools",
		}
	}
	return PolicyDecision{
		Action: PolicyActionDeny,
		Reason: "plan mode only allows read-only, search, and plan tools",
	}
}

// ---------------------------------------------------------------------------
// ExecuteModePolicy — execute mode: allows all, mutation gets NeedApproval.
// ---------------------------------------------------------------------------

// ExecuteModePolicy implements ModePolicy for execute mode.
type ExecuteModePolicy struct{}

// CheckTool determines whether the given tool is permitted in execute mode.
func (p *ExecuteModePolicy) CheckTool(input PolicyInput) PolicyDecision {
	toolName := normalizeToolName(input)
	if isTerminalCommandTool(toolName) {
		req, ok := terminalCommandRequestFromArgs(input.Arguments)
		if !ok {
			return PolicyDecision{
				Action: PolicyActionDeny,
				Reason: "terminal command uses unsupported shell syntax or is missing a command",
			}
		}
		if terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionNeedApproval,
			Reason: "execute mode requires approval for local terminal commands that are not classified read-only",
			Approval: &ApprovalRequest{
				ToolName: toolName,
				Reason:   "local terminal command requires approval",
			},
		}
	}
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

func normalizeToolName(input PolicyInput) string {
	name := strings.TrimSpace(input.ToolName)
	if name != "" {
		return name
	}
	return strings.TrimSpace(input.Tool.Name)
}

func isMCPTool(input PolicyInput) bool {
	return input.Tool.HasSource(tooling.ToolSourceMCP) || input.Tool.IsMCP
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
