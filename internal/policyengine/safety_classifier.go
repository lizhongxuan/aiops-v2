package policyengine

import (
	"encoding/json"
	"sort"
	"strings"

	"aiops-v2/internal/tooling"
)

type SafetySignalCategory string

const (
	SafetySignalForce               SafetySignalCategory = "force"
	SafetySignalSkipValidation      SafetySignalCategory = "skip_validation"
	SafetySignalDeleteLock          SafetySignalCategory = "delete_lock"
	SafetySignalDisableGuard        SafetySignalCategory = "disable_guard"
	SafetySignalResetState          SafetySignalCategory = "reset_state"
	SafetySignalOverwriteUnexpected SafetySignalCategory = "overwrite_unexpected"
	SafetySignalHighRisk            SafetySignalCategory = "high_risk"
)

type SafetySeverity string

const (
	SafetySeverityLow      SafetySeverity = "low"
	SafetySeverityMedium   SafetySeverity = "medium"
	SafetySeverityHigh     SafetySeverity = "high"
	SafetySeverityCritical SafetySeverity = "critical"
)

type SafetySignal struct {
	Category SafetySignalCategory `json:"category"`
	Severity SafetySeverity       `json:"severity"`
	Reasons  []string             `json:"reasons,omitempty"`
}

type safetyPattern struct {
	category SafetySignalCategory
	severity SafetySeverity
	reason   string
	terms    []string
	allTerms []string
}

var destructiveWorkaroundPatterns = []safetyPattern{
	{
		category: SafetySignalForce,
		severity: SafetySeverityHigh,
		reason:   "force operation requested",
		terms: []string{
			"--force", "force=true", `"force":true`, "force overwrite", "forced overwrite",
			"force apply", "force update", "force write",
		},
	},
	{
		category: SafetySignalSkipValidation,
		severity: SafetySeverityHigh,
		reason:   "validation or preflight checks would be skipped",
		terms: []string{
			"skip validation", "skip_valid", "skip preflight", "skip checks",
			"--no-verify", "no verify", "ignore validation", "ignore checks",
			"without validation",
		},
	},
	{
		category: SafetySignalDeleteLock,
		severity: SafetySeverityCritical,
		reason:   "lock deletion requested",
		terms: []string{
			"delete lock", "remove lock", "delete_lock", "remove_lock",
			"lock file", ".lock",
		},
	},
	{
		category: SafetySignalDisableGuard,
		severity: SafetySeverityCritical,
		reason:   "guard or protection would be disabled",
		terms: []string{
			"disable guard", "disable safety", "disable protection", "disable_guard",
			"bypass guard", "bypass safety", "bypass protection", "turn off guard",
		},
	},
	{
		category: SafetySignalResetState,
		severity: SafetySeverityHigh,
		reason:   "state reset requested",
		terms: []string{
			"reset state", "state reset", "reset_state", "hard reset", "reset --hard",
			"git reset --hard",
		},
	},
	{
		category: SafetySignalOverwriteUnexpected,
		severity: SafetySeverityCritical,
		reason:   "unexpected state would be overwritten",
		terms: []string{
			"overwrite unexpected", "overwrite conflict", "ignore conflict", "ignore drift",
			"overwrite drift", "precondition failed",
		},
		allTerms: []string{"overwrite", "conflict"},
	},
}

func DetectSafetySignals(input PolicyInput) []SafetySignal {
	raw := rawSafetyText(input)
	text := normalizeSafetyText(raw)
	signals := make(map[SafetySignalCategory]SafetySignal)
	for _, pattern := range destructiveWorkaroundPatterns {
		if safetyPatternMatches(text, pattern) {
			addSafetySignal(signals, pattern.category, pattern.severity, pattern.reason)
		}
	}
	if IsHighRiskCommand(raw) {
		addSafetySignal(signals, SafetySignalHighRisk, SafetySeverityCritical, "high-risk command pattern matched")
	}
	if !isTerminalCommandTool(normalizeToolName(input)) {
		switch input.Tool.EffectiveGovernance(defaultPolicyInlineBudgetBytes).RiskLevel {
		case tooling.ToolRiskHigh:
			addSafetySignal(signals, SafetySignalHighRisk, SafetySeverityHigh, "tool metadata classifies this action as high risk")
		case tooling.ToolRiskCritical:
			addSafetySignal(signals, SafetySignalHighRisk, SafetySeverityCritical, "tool metadata classifies this action as critical risk")
		}
	}
	return sortedSafetySignals(signals)
}

func addSafetySignal(signals map[SafetySignalCategory]SafetySignal, category SafetySignalCategory, severity SafetySeverity, reason string) {
	signal := signals[category]
	signal.Category = category
	if safetySeverityRank(severity) > safetySeverityRank(signal.Severity) {
		signal.Severity = severity
	}
	if reason != "" && !containsString(signal.Reasons, reason) {
		signal.Reasons = append(signal.Reasons, reason)
	}
	signals[category] = signal
}

func sortedSafetySignals(signals map[SafetySignalCategory]SafetySignal) []SafetySignal {
	if len(signals) == 0 {
		return nil
	}
	out := make([]SafetySignal, 0, len(signals))
	for _, signal := range signals {
		sort.Strings(signal.Reasons)
		out = append(out, signal)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return safetySeverityRank(out[i].Severity) > safetySeverityRank(out[j].Severity)
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func safetyPatternMatches(text string, pattern safetyPattern) bool {
	for _, term := range pattern.terms {
		if strings.Contains(text, normalizeSafetyText(term)) {
			return true
		}
	}
	if len(pattern.allTerms) > 0 {
		for _, term := range pattern.allTerms {
			if !strings.Contains(text, normalizeSafetyText(term)) {
				return false
			}
		}
		return true
	}
	return false
}

func rawSafetyText(input PolicyInput) string {
	parts := []string{
		input.ToolName,
		input.Tool.Name,
		input.Tool.Description,
		strings.Join(input.Tool.Discovery.OperationKinds, " "),
		strings.Join(input.Tool.Discovery.ResourceTypes, " "),
		string(input.Arguments),
	}
	if len(input.Arguments) > 0 {
		var decoded any
		if json.Unmarshal(input.Arguments, &decoded) == nil {
			parts = append(parts, flattenSafetyJSON(decoded)...)
		}
	}
	return strings.Join(parts, " ")
}

func flattenSafetyJSON(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		out := make([]string, 0, len(typed)*2)
		for key, child := range typed {
			out = append(out, key)
			out = append(out, flattenSafetyJSON(child)...)
		}
		return out
	case []any:
		var out []string
		for _, child := range typed {
			out = append(out, flattenSafetyJSON(child)...)
		}
		return out
	case string:
		return []string{typed}
	case bool:
		if typed {
			return []string{"true"}
		}
		return []string{"false"}
	case float64:
		return []string{}
	default:
		return nil
	}
}

func normalizeSafetyText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "_", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func safetyDecision(input PolicyInput, allowApproval bool, modeName string) (PolicyDecision, bool) {
	signals := DetectSafetySignals(input)
	if !hasHighSafetySignal(signals) {
		return PolicyDecision{}, false
	}
	reason := "safety signal requires approval"
	if !allowApproval {
		return PolicyDecision{
			Action:        PolicyActionDeny,
			Reason:        modeName + " mode blocks high-risk safety signal",
			SafetySignals: signals,
		}, true
	}
	return PolicyDecision{
		Action:        PolicyActionNeedApproval,
		Reason:        reason,
		SafetySignals: signals,
		Approval: &ApprovalRequest{
			ToolName:      normalizeToolName(input),
			Reason:        reason + ": " + safetySignalSummary(signals),
			SafetySignals: signals,
		},
	}, true
}

func hasHighSafetySignal(signals []SafetySignal) bool {
	for _, signal := range signals {
		if safetySeverityRank(signal.Severity) >= safetySeverityRank(SafetySeverityHigh) {
			return true
		}
	}
	return false
}

func safetySeverityRank(severity SafetySeverity) int {
	switch severity {
	case SafetySeverityLow:
		return 1
	case SafetySeverityMedium:
		return 2
	case SafetySeverityHigh:
		return 3
	case SafetySeverityCritical:
		return 4
	default:
		return 0
	}
}

func safetySignalSummary(signals []SafetySignal) string {
	if len(signals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(signals))
	for _, signal := range signals {
		parts = append(parts, string(signal.Category))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
