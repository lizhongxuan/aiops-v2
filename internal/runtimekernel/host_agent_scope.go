package runtimekernel

import (
	"sort"
	"strings"
)

type RiskLevel string

const (
	RiskLevelReadOnly RiskLevel = "read_only"
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type SkillActivationMode string

const (
	SkillActivationModeExplicit         SkillActivationMode = "explicit"
	SkillActivationModeDelegated        SkillActivationMode = "delegated"
	SkillActivationModeManagerInherited SkillActivationMode = "manager_inherited"
)

type HostAgentSkillScope struct {
	SkillID            string              `json:"skillId"`
	ActivationMode     SkillActivationMode `json:"activationMode"`
	AllowedHostScope   []string            `json:"allowedHostScope,omitempty"`
	AllowedActionTypes []string            `json:"allowedActionTypes,omitempty"`
	MaxRisk            RiskLevel           `json:"maxRisk,omitempty"`
	Reason             string              `json:"reason,omitempty"`
}

type HostAgentSkillScopeRequest struct {
	HostID     string    `json:"hostId"`
	ActionType string    `json:"actionType,omitempty"`
	TargetHost string    `json:"targetHost,omitempty"`
	Risk       RiskLevel `json:"risk,omitempty"`
}

type HostAgentSkillScopeEvaluation struct {
	VisibleSkills   []HostAgentSkillScope  `json:"visibleSkills,omitempty"`
	ToolUseDecision HostAgentScopeDecision `json:"toolUseDecision"`
	Trace           []HostAgentScopeTrace  `json:"trace,omitempty"`
}

type MCPPermission string

const (
	MCPPermissionNone      MCPPermission = "none"
	MCPPermissionRead      MCPPermission = "read"
	MCPPermissionWrite     MCPPermission = "write"
	MCPPermissionReadWrite MCPPermission = "read_write"
)

type MCPResourceOperation string

const (
	MCPResourceOperationRead  MCPResourceOperation = "read"
	MCPResourceOperationWrite MCPResourceOperation = "write"
)

type MCPInstructionMode string

const (
	MCPInstructionModeHidden MCPInstructionMode = "hidden"
	MCPInstructionModeSparse MCPInstructionMode = "sparse"
	MCPInstructionModeFull   MCPInstructionMode = "full"
)

type HostAgentMCPScope struct {
	ServerID         string             `json:"serverId"`
	Permission       MCPPermission      `json:"permission"`
	AllowedResources []string           `json:"allowedResources,omitempty"`
	AllowedHostScope []string           `json:"allowedHostScope,omitempty"`
	RequiresApproval bool               `json:"requiresApproval,omitempty"`
	InstructionMode  MCPInstructionMode `json:"instructionMode,omitempty"`
	Reason           string             `json:"reason,omitempty"`
}

type HostAgentMCPScopeRequest struct {
	HostID    string               `json:"hostId"`
	ServerID  string               `json:"serverId,omitempty"`
	Resource  string               `json:"resource,omitempty"`
	Operation MCPResourceOperation `json:"operation,omitempty"`
}

type HostAgentMCPScopeEvaluation struct {
	VisibleServers   []HostAgentMCPScope    `json:"visibleServers,omitempty"`
	ResourceDecision HostAgentScopeDecision `json:"resourceDecision"`
	Trace            []HostAgentScopeTrace  `json:"trace,omitempty"`
}

type HostAgentScopeDecision struct {
	Allowed          bool   `json:"allowed"`
	RequiresApproval bool   `json:"requiresApproval,omitempty"`
	HiddenReason     string `json:"hiddenReason,omitempty"`
}

type HostAgentScopeTrace struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Visible      bool   `json:"visible"`
	HiddenReason string `json:"hiddenReason,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func EvaluateHostAgentSkillScopes(req HostAgentSkillScopeRequest, scopes []HostAgentSkillScope) HostAgentSkillScopeEvaluation {
	req.HostID = strings.TrimSpace(req.HostID)
	req.ActionType = strings.TrimSpace(req.ActionType)
	req.TargetHost = strings.TrimSpace(req.TargetHost)
	var eval HostAgentSkillScopeEvaluation
	eval.ToolUseDecision = HostAgentScopeDecision{Allowed: false, HiddenReason: "no_visible_skill_scope"}

	for _, scope := range scopes {
		normalized := normalizeHostAgentSkillScope(scope)
		visible, reason := hostAgentSkillHiddenReason(req, normalized)
		if visible {
			eval.VisibleSkills = append(eval.VisibleSkills, normalized)
			eval.ToolUseDecision = HostAgentScopeDecision{Allowed: true}
		} else if eval.ToolUseDecision.HiddenReason == "no_visible_skill_scope" && reason != "" {
			eval.ToolUseDecision.HiddenReason = reason
		}
		eval.Trace = append(eval.Trace, HostAgentScopeTrace{
			Kind:         "skill",
			ID:           normalized.SkillID,
			Visible:      visible,
			HiddenReason: reason,
			Reason:       normalized.Reason,
		})
	}
	sortHostAgentSkillScopes(eval.VisibleSkills)
	return eval
}

func EvaluateHostAgentMCPScope(req HostAgentMCPScopeRequest, scopes []HostAgentMCPScope) HostAgentMCPScopeEvaluation {
	req.HostID = strings.TrimSpace(req.HostID)
	req.ServerID = strings.TrimSpace(req.ServerID)
	req.Resource = strings.TrimSpace(req.Resource)
	var eval HostAgentMCPScopeEvaluation
	eval.ResourceDecision = HostAgentScopeDecision{Allowed: false, HiddenReason: "no_visible_mcp_scope"}

	for _, scope := range scopes {
		normalized := normalizeHostAgentMCPScope(scope)
		visible, reason := hostAgentMCPHiddenReason(req, normalized)
		if visible {
			eval.VisibleServers = append(eval.VisibleServers, normalized)
			if req.ServerID == "" || req.ServerID == normalized.ServerID {
				eval.ResourceDecision = hostAgentMCPResourceDecision(req, normalized)
			}
		}
		eval.Trace = append(eval.Trace, HostAgentScopeTrace{
			Kind:         "mcp",
			ID:           normalized.ServerID,
			Visible:      visible,
			HiddenReason: reason,
			Reason:       normalized.Reason,
		})
	}
	sortHostAgentMCPScope(eval.VisibleServers)
	return eval
}

func hostAgentSkillHiddenReason(req HostAgentSkillScopeRequest, scope HostAgentSkillScope) (bool, string) {
	if scope.SkillID == "" {
		return false, "skill_id_missing"
	}
	if scope.ActivationMode == SkillActivationModeManagerInherited {
		return false, "manager_skill_body_not_inherited"
	}
	if !hostScopeAllows(scope.AllowedHostScope, req.HostID) {
		return false, "host_out_of_scope"
	}
	if req.TargetHost != "" && !hostScopeAllows(scope.AllowedHostScope, req.TargetHost) {
		return false, "target_host_out_of_scope"
	}
	if req.ActionType != "" && !stringScopeAllows(scope.AllowedActionTypes, req.ActionType) {
		return false, "action_type_out_of_scope"
	}
	if riskExceeds(req.Risk, scope.MaxRisk) {
		return false, "risk_exceeds_skill_scope"
	}
	return true, ""
}

func hostAgentMCPHiddenReason(req HostAgentMCPScopeRequest, scope HostAgentMCPScope) (bool, string) {
	if scope.ServerID == "" {
		return false, "mcp_server_id_missing"
	}
	if scope.InstructionMode == MCPInstructionModeHidden {
		return false, "mcp_instructions_not_selected"
	}
	if req.ServerID != "" && req.ServerID != scope.ServerID {
		return false, "mcp_server_not_requested"
	}
	if !hostScopeAllows(scope.AllowedHostScope, req.HostID) {
		return false, "host_out_of_scope"
	}
	decision := hostAgentMCPResourceDecision(req, scope)
	if !decision.Allowed {
		return false, decision.HiddenReason
	}
	return true, ""
}

func hostAgentMCPResourceDecision(req HostAgentMCPScopeRequest, scope HostAgentMCPScope) HostAgentScopeDecision {
	if req.Resource != "" && !stringScopeAllows(scope.AllowedResources, req.Resource) {
		return HostAgentScopeDecision{Allowed: false, HiddenReason: "resource_out_of_scope"}
	}
	if req.Operation != "" && !mcpPermissionAllows(scope.Permission, req.Operation) {
		return HostAgentScopeDecision{Allowed: false, HiddenReason: "permission_out_of_scope"}
	}
	return HostAgentScopeDecision{Allowed: true, RequiresApproval: scope.RequiresApproval}
}

func normalizeHostAgentSkillScope(scope HostAgentSkillScope) HostAgentSkillScope {
	scope.SkillID = strings.TrimSpace(scope.SkillID)
	scope.ActivationMode = SkillActivationMode(strings.TrimSpace(string(scope.ActivationMode)))
	scope.AllowedHostScope = cloneNormalizedSortedStrings(scope.AllowedHostScope)
	scope.AllowedActionTypes = cloneNormalizedSortedStrings(scope.AllowedActionTypes)
	scope.MaxRisk = normalizeRiskLevel(scope.MaxRisk)
	scope.Reason = strings.TrimSpace(scope.Reason)
	return scope
}

func normalizeHostAgentMCPScope(scope HostAgentMCPScope) HostAgentMCPScope {
	scope.ServerID = strings.TrimSpace(scope.ServerID)
	scope.Permission = MCPPermission(strings.TrimSpace(string(scope.Permission)))
	scope.AllowedResources = cloneNormalizedSortedStrings(scope.AllowedResources)
	scope.AllowedHostScope = cloneNormalizedSortedStrings(scope.AllowedHostScope)
	scope.InstructionMode = MCPInstructionMode(strings.TrimSpace(string(scope.InstructionMode)))
	if scope.InstructionMode == "" {
		scope.InstructionMode = MCPInstructionModeSparse
	}
	scope.Reason = strings.TrimSpace(scope.Reason)
	return scope
}

func hostScopeAllows(scope []string, hostID string) bool {
	return stringScopeAllows(scope, strings.TrimSpace(hostID))
}

func stringScopeAllows(scope []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, candidate := range scope {
		if candidate == "*" || candidate == value {
			return true
		}
	}
	return false
}

func mcpPermissionAllows(permission MCPPermission, operation MCPResourceOperation) bool {
	switch permission {
	case MCPPermissionReadWrite:
		return operation == MCPResourceOperationRead || operation == MCPResourceOperationWrite
	case MCPPermissionRead:
		return operation == "" || operation == MCPResourceOperationRead
	case MCPPermissionWrite:
		return operation == MCPResourceOperationWrite
	default:
		return false
	}
}

func normalizeRiskLevel(risk RiskLevel) RiskLevel {
	risk = RiskLevel(strings.TrimSpace(string(risk)))
	if risk == "" {
		return RiskLevelReadOnly
	}
	return risk
}

func riskExceeds(actual, max RiskLevel) bool {
	actual = normalizeRiskLevel(actual)
	max = normalizeRiskLevel(max)
	return hostAgentRiskRank(actual) > hostAgentRiskRank(max)
}

func hostAgentRiskRank(risk RiskLevel) int {
	switch risk {
	case RiskLevelReadOnly:
		return 0
	case RiskLevelLow:
		return 1
	case RiskLevelMedium:
		return 2
	case RiskLevelHigh:
		return 3
	case RiskLevelCritical:
		return 4
	default:
		return 4
	}
}

func cloneNormalizedSortedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortHostAgentSkillScopes(scopes []HostAgentSkillScope) {
	sort.Slice(scopes, func(i, j int) bool {
		return scopes[i].SkillID < scopes[j].SkillID
	})
}

func sortHostAgentMCPScope(scopes []HostAgentMCPScope) {
	sort.Slice(scopes, func(i, j int) bool {
		return scopes[i].ServerID < scopes[j].ServerID
	})
}
