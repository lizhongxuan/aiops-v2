package hostops

import (
	"strings"

	"aiops-v2/internal/opssemantic"
)

type CommandPolicyConfig struct {
	GlobalWhitelist []CommandPolicyRule
	Overrides       []CommandPolicyOverride
	TaskGrants      []CommandPolicyGrant
}

type CommandPolicyRule struct {
	ID      string
	Pattern string
	MaxRisk opssemantic.OpsRiskLevel
}

type CommandPolicyOverride struct {
	HostID      string
	Environment string
	Rules       []CommandPolicyRule
}

type CommandPolicyGrant struct {
	MissionID    string
	ChildAgentID string
	PlanStepID   string
	HostID       string
	Command      string
	RiskLevel    opssemantic.OpsRiskLevel
}

type CommandPolicyContext struct {
	MissionID    string
	ChildAgentID string
	PlanStepID   string
	HostID       string
	Environment  string
	Command      string
	RiskLevel    opssemantic.OpsRiskLevel
}

type CommandPolicyDecision struct {
	Allowed          bool
	RequiresApproval bool
	Reason           string
	MatchedRuleID    string
}

type CommandPolicy struct {
	globalWhitelist []CommandPolicyRule
	overrides       []CommandPolicyOverride
	taskGrants      []CommandPolicyGrant
}

func NewCommandPolicy(config CommandPolicyConfig) *CommandPolicy {
	return &CommandPolicy{
		globalWhitelist: append([]CommandPolicyRule(nil), config.GlobalWhitelist...),
		overrides:       append([]CommandPolicyOverride(nil), config.Overrides...),
		taskGrants:      append([]CommandPolicyGrant(nil), config.TaskGrants...),
	}
}

func (p *CommandPolicy) Evaluate(ctx CommandPolicyContext) CommandPolicyDecision {
	ctx = normalizeCommandPolicyContext(ctx)
	if p == nil {
		return commandRequiresApproval("no_policy_match")
	}
	if grantID := p.matchTaskGrant(ctx); grantID != "" {
		return CommandPolicyDecision{Allowed: true, RequiresApproval: false, Reason: "task_grant", MatchedRuleID: grantID}
	}
	if ruleID := matchRules(p.globalWhitelist, ctx); ruleID != "" {
		return CommandPolicyDecision{Allowed: true, RequiresApproval: false, Reason: "global_whitelist", MatchedRuleID: ruleID}
	}
	for _, override := range p.overrides {
		if !overrideMatches(override, ctx) {
			continue
		}
		if ruleID := matchRules(override.Rules, ctx); ruleID != "" {
			return CommandPolicyDecision{Allowed: true, RequiresApproval: false, Reason: "override_whitelist", MatchedRuleID: ruleID}
		}
	}
	return commandRequiresApproval("no_policy_match")
}

func (p *CommandPolicy) matchTaskGrant(ctx CommandPolicyContext) string {
	for _, grant := range p.taskGrants {
		if !sameTrimmed(grant.MissionID, ctx.MissionID) ||
			!sameTrimmed(grant.ChildAgentID, ctx.ChildAgentID) ||
			!sameTrimmed(grant.PlanStepID, ctx.PlanStepID) ||
			!sameTrimmed(grant.HostID, ctx.HostID) ||
			normalizeCommand(grant.Command) != ctx.Command ||
			grant.RiskLevel != ctx.RiskLevel {
			continue
		}
		return firstNonEmptyString(grant.PlanStepID, "task_grant")
	}
	return ""
}

func matchRules(rules []CommandPolicyRule, ctx CommandPolicyContext) string {
	for _, rule := range rules {
		if !riskWithin(ctx.RiskLevel, rule.MaxRisk) {
			continue
		}
		if commandPatternMatches(rule.Pattern, ctx.Command) {
			return firstNonEmptyString(rule.ID, rule.Pattern)
		}
	}
	return ""
}

func overrideMatches(override CommandPolicyOverride, ctx CommandPolicyContext) bool {
	if strings.TrimSpace(override.HostID) != "" && !sameTrimmed(override.HostID, ctx.HostID) {
		return false
	}
	if strings.TrimSpace(override.Environment) != "" && !sameTrimmed(override.Environment, ctx.Environment) {
		return false
	}
	return true
}

func riskWithin(actual, max opssemantic.OpsRiskLevel) bool {
	if max == "" {
		max = opssemantic.RiskReadOnly
	}
	return riskRank(actual) <= riskRank(max)
}

func commandPatternMatches(pattern, command string) bool {
	pattern = normalizeCommand(pattern)
	command = normalizeCommand(command)
	if pattern == "" || command == "" {
		return false
	}
	if pattern == command {
		return true
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return false
	}
	position := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(command[position:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		position += idx + len(part)
	}
	lastPart := parts[len(parts)-1]
	return lastPart == "" || strings.HasSuffix(command, lastPart)
}

func normalizeCommandPolicyContext(ctx CommandPolicyContext) CommandPolicyContext {
	ctx.MissionID = strings.TrimSpace(ctx.MissionID)
	ctx.ChildAgentID = strings.TrimSpace(ctx.ChildAgentID)
	ctx.PlanStepID = strings.TrimSpace(ctx.PlanStepID)
	ctx.HostID = strings.TrimSpace(ctx.HostID)
	ctx.Environment = strings.TrimSpace(ctx.Environment)
	ctx.Command = normalizeCommand(ctx.Command)
	if ctx.RiskLevel == "" {
		ctx.RiskLevel = opssemantic.ClassifyRisk(ctx.Command)
	}
	return ctx
}

func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
}

func sameTrimmed(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func commandRequiresApproval(reason string) CommandPolicyDecision {
	return CommandPolicyDecision{Allowed: false, RequiresApproval: true, Reason: reason}
}
