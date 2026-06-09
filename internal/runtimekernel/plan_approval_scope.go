package runtimekernel

import (
	"fmt"
	"path"
	"strings"
	"time"
)

type PlanApprovalScope struct {
	PlanID         string                      `json:"planId"`
	ApprovalID     string                      `json:"approvalId,omitempty"`
	AllowedActions []string                    `json:"allowedActions,omitempty"`
	ResourceScopes []PlanApprovalResourceScope `json:"resourceScopes,omitempty"`
	RiskCeiling    string                      `json:"riskCeiling,omitempty"`
	ExpiresAt      *time.Time                  `json:"expiresAt,omitempty"`
	ApprovedAt     *time.Time                  `json:"approvedAt,omitempty"`
	ApprovedBy     string                      `json:"approvedBy,omitempty"`
	InputHash      string                      `json:"inputHash,omitempty"`
}

type PlanApprovalResourceScope struct {
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

type PlanScopedToolCall struct {
	PlanID       string `json:"planId,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	Action       string `json:"action,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
	ResourcePath string `json:"resourcePath,omitempty"`
	Risk         string `json:"risk,omitempty"`
	InputHash    string `json:"inputHash,omitempty"`
}

type PlanApprovalScopeMatch struct {
	Allowed       bool   `json:"allowed"`
	NeedsApproval bool   `json:"needsApproval"`
	Reason        string `json:"reason"`
}

func (s PlanApprovalScope) Validate() error {
	if strings.TrimSpace(s.PlanID) == "" {
		return fmt.Errorf("planId is required")
	}
	if len(normalizePlanScopeStrings(s.AllowedActions)) == 0 {
		return fmt.Errorf("allowedActions is required")
	}
	if strings.TrimSpace(s.RiskCeiling) != "" && riskRank(s.RiskCeiling) < 0 {
		return fmt.Errorf("invalid risk ceiling %q", s.RiskCeiling)
	}
	return nil
}

func (s PlanApprovalScope) Match(call PlanScopedToolCall, now time.Time) PlanApprovalScopeMatch {
	if now.IsZero() {
		now = time.Now()
	}
	if strings.TrimSpace(s.PlanID) == "" || strings.TrimSpace(call.PlanID) == "" || strings.TrimSpace(s.PlanID) != strings.TrimSpace(call.PlanID) {
		return planScopeNeedsApproval("plan id is not covered by the approved plan scope")
	}
	if s.ExpiresAt != nil && !now.Before(*s.ExpiresAt) {
		return planScopeNeedsApproval("approved plan scope has expired")
	}
	if strings.TrimSpace(s.InputHash) != "" && strings.TrimSpace(call.InputHash) != strings.TrimSpace(s.InputHash) {
		return planScopeNeedsApproval("input hash is outside the approved plan scope")
	}
	action := strings.ToLower(strings.TrimSpace(firstNonEmpty(call.Action, call.ToolName)))
	if !scopeStringContains(s.AllowedActions, action) {
		return planScopeNeedsApproval("action is outside the approved plan scope")
	}
	ceiling := strings.TrimSpace(s.RiskCeiling)
	if ceiling == "" {
		ceiling = "medium"
	}
	callRisk := riskRank(call.Risk)
	if callRisk < 0 {
		callRisk = riskRank("medium")
	}
	if callRisk > riskRank(ceiling) {
		return planScopeNeedsApproval("risk exceeds the approved plan scope")
	}
	if len(s.ResourceScopes) > 0 && !scopeMatchesResource(s.ResourceScopes, call) {
		return planScopeNeedsApproval("resource is outside the approved plan scope")
	}
	return PlanApprovalScopeMatch{Allowed: true, Reason: "tool call matches approved plan scope"}
}

func planScopeNeedsApproval(reason string) PlanApprovalScopeMatch {
	return PlanApprovalScopeMatch{Allowed: false, NeedsApproval: true, Reason: reason}
}

func scopeStringContains(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "*" || value == target {
			return true
		}
	}
	return false
}

func normalizePlanScopeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func scopeMatchesResource(scopes []PlanApprovalResourceScope, call PlanScopedToolCall) bool {
	callType := strings.TrimSpace(call.ResourceType)
	callID := strings.TrimSpace(call.ResourceID)
	callPath := strings.TrimSpace(call.ResourcePath)
	for _, scope := range scopes {
		if scope.Type != "" && callType != "" && !strings.EqualFold(strings.TrimSpace(scope.Type), callType) {
			continue
		}
		if scope.ID == "*" || scope.Path == "*" || scope.Pattern == "*" {
			return true
		}
		if scope.ID != "" && callID != "" && resourceValueMatches(scope.ID, callID) {
			return true
		}
		if scope.Path != "" && callPath != "" && resourceValueMatches(scope.Path, callPath) {
			return true
		}
		if scope.Pattern != "" && (resourceValueMatches(scope.Pattern, callID) || resourceValueMatches(scope.Pattern, callPath)) {
			return true
		}
	}
	return false
}

func resourceValueMatches(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" || value == "" {
		return false
	}
	if pattern == value {
		return true
	}
	if strings.HasSuffix(pattern, "*") && strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
		return true
	}
	if ok, _ := path.Match(pattern, value); ok {
		return true
	}
	return false
}

func riskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "read":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "destructive", "critical":
		return 4
	default:
		return -1
	}
}
