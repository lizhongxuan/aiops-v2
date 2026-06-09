package runtimekernel

import (
	"fmt"
	"sort"
	"strings"
)

const (
	FinalEvidenceConfidenceHigh   = "high"
	FinalEvidenceConfidenceMedium = "medium"
	FinalEvidenceConfidenceLow    = "low"

	FinalEvidenceActionAllow     = "allow"
	FinalEvidenceActionDowngrade = "downgrade"
	FinalEvidenceActionBlock     = "block"
)

type FinalEvidenceState struct {
	Checked     []CheckedEvidence  `json:"checked,omitempty"`
	NotChecked  []NotCheckedItem   `json:"notChecked,omitempty"`
	FailedTools []FailedToolImpact `json:"failedTools,omitempty"`
	Confidence  string             `json:"confidence"`
}

type CheckedEvidence struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type NotCheckedItem struct {
	ToolName             string `json:"toolName,omitempty"`
	ToolCallID           string `json:"toolCallId,omitempty"`
	Reason               string `json:"reason,omitempty"`
	RequiredAction       string `json:"requiredAction,omitempty"`
	SuggestedSearchQuery string `json:"suggestedSearchQuery,omitempty"`
}

type FailedToolImpact struct {
	ToolName     string `json:"toolName,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	FailureClass string `json:"failureClass,omitempty"`
	Impact       string `json:"impact,omitempty"`
}

type FinalEvidenceVerification struct {
	Action     string             `json:"action"`
	Confidence string             `json:"confidence"`
	Reasons    []string           `json:"reasons,omitempty"`
	State      FinalEvidenceState `json:"state"`
}

func BuildFinalEvidenceState(snapshot *TurnSnapshot, session *SessionState) FinalEvidenceState {
	state := FinalEvidenceState{
		Checked:     checkedEvidenceFromSnapshot(snapshot),
		FailedTools: failedToolImpactsFromSnapshot(snapshot),
	}
	if session != nil {
		state.NotChecked = notCheckedItemsFromDiscovery(session.ToolDiscovery)
	}
	state.Confidence = inferFinalEvidenceConfidence(state)
	return state
}

func VerifyFinalEvidence(answer string, state FinalEvidenceState) FinalEvidenceVerification {
	state.Confidence = normalizeFinalEvidenceConfidence(state.Confidence)
	decision := FinalEvidenceVerification{
		Action:     FinalEvidenceActionAllow,
		Confidence: state.Confidence,
		State:      state,
	}
	answer = strings.TrimSpace(answer)
	claimsChecked := finalAnswerClaimsChecked(answer)
	highConfidenceClaim := finalAnswerClaimsHighConfidence(answer) || state.Confidence == FinalEvidenceConfidenceHigh

	if len(state.FailedTools) > 0 && highConfidenceClaim {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceMedium)
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "failed_tool_requires_lower_confidence")
	}
	if len(state.NotChecked) > 0 && (claimsChecked || highConfidenceClaim) {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceLow)
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "not_checked_item_requires_lower_confidence")
	}
	if claimsChecked && len(state.Checked) == 0 {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = FinalEvidenceConfidenceLow
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "checked_claim_without_checked_evidence")
	}
	if len(decision.Reasons) == 0 {
		decision.Action = FinalEvidenceActionAllow
	}
	return decision
}

func checkedEvidenceFromSnapshot(snapshot *TurnSnapshot) []CheckedEvidence {
	if snapshot == nil {
		return nil
	}
	var out []CheckedEvidence
	seen := map[string]bool{}
	for _, iter := range snapshot.Iterations {
		toolNames := map[string]string{}
		for _, call := range iter.ToolCalls {
			toolNames[call.ID] = call.Name
		}
		for _, result := range iter.ToolResults {
			if strings.TrimSpace(result.Error) != "" {
				continue
			}
			summary := strings.TrimSpace(result.Summary)
			if summary == "" {
				summary = firstNonEmptyLine(result.Content)
			}
			if summary == "" {
				continue
			}
			item := CheckedEvidence{
				ToolCallID: strings.TrimSpace(result.ToolCallID),
				ToolName:   strings.TrimSpace(toolNames[result.ToolCallID]),
				Summary:    truncateRunes(summary, 180),
			}
			key := item.ToolCallID + "\x00" + item.ToolName + "\x00" + item.Summary
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

func failedToolImpactsFromSnapshot(snapshot *TurnSnapshot) []FailedToolImpact {
	summaries := failedToolSummariesFromSnapshot(snapshot)
	if len(summaries) == 0 {
		return nil
	}
	out := make([]FailedToolImpact, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, FailedToolImpact{
			ToolName:     strings.TrimSpace(summary.Tool),
			FailureClass: strings.TrimSpace(summary.FailureClass),
			Impact:       "required evidence may be missing; do not use this failed tool as checked evidence",
		})
	}
	return out
}

func notCheckedItemsFromDiscovery(discovery ToolDiscoverySessionState) []NotCheckedItem {
	if len(discovery.RejectedCalls) == 0 {
		return nil
	}
	out := make([]NotCheckedItem, 0, len(discovery.RejectedCalls))
	for _, call := range discovery.RejectedCalls {
		switch strings.TrimSpace(call.ErrorType) {
		case "tool_unloaded", "tool_hidden_by_policy", "tool_not_found", "dedicated_tool_preferred":
		default:
			continue
		}
		out = append(out, NotCheckedItem{
			ToolName:             strings.TrimSpace(call.ToolName),
			ToolCallID:           strings.TrimSpace(call.ToolCallID),
			Reason:               strings.TrimSpace(call.ErrorType),
			RequiredAction:       strings.TrimSpace(call.RequiredAction),
			SuggestedSearchQuery: strings.TrimSpace(call.SuggestedSearchQuery),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ToolName == out[j].ToolName {
			return out[i].Reason < out[j].Reason
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out
}

func inferFinalEvidenceConfidence(state FinalEvidenceState) string {
	if len(state.Checked) == 0 {
		return FinalEvidenceConfidenceLow
	}
	if len(state.FailedTools) > 0 || len(state.NotChecked) > 0 {
		return FinalEvidenceConfidenceMedium
	}
	return FinalEvidenceConfidenceHigh
}

func normalizeFinalEvidenceConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case FinalEvidenceConfidenceHigh:
		return FinalEvidenceConfidenceHigh
	case FinalEvidenceConfidenceMedium:
		return FinalEvidenceConfidenceMedium
	case FinalEvidenceConfidenceLow:
		return FinalEvidenceConfidenceLow
	default:
		return FinalEvidenceConfidenceLow
	}
}

func minFinalEvidenceConfidence(current, cap string) string {
	current = normalizeFinalEvidenceConfidence(current)
	cap = normalizeFinalEvidenceConfidence(cap)
	if finalEvidenceConfidenceRank(current) <= finalEvidenceConfidenceRank(cap) {
		return current
	}
	return cap
}

func finalEvidenceConfidenceRank(value string) int {
	switch normalizeFinalEvidenceConfidence(value) {
	case FinalEvidenceConfidenceHigh:
		return 3
	case FinalEvidenceConfidenceMedium:
		return 2
	default:
		return 1
	}
}

func finalAnswerClaimsChecked(answer string) bool {
	text := strings.ToLower(answer)
	for _, marker := range []string{
		"已检查", "已确认", "确认全部", "全部检查", "checked", "verified", "confirmed", "inspected",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func finalAnswerClaimsHighConfidence(answer string) bool {
	text := strings.ToLower(answer)
	for _, marker := range []string{
		"高置信", "确定", "明确", "confirmed", "definitely", "no issue", "normal", "正常",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func appendFinalEvidenceReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func finalEvidenceRetryPrompt(decision FinalEvidenceVerification) string {
	if decision.Action == FinalEvidenceActionAllow {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Final evidence verifier\n")
	fmt.Fprintf(&b, "Your previous final answer used confidence that was not supported by checked evidence. Revise the final answer without inventing facts.\n")
	fmt.Fprintf(&b, "- Required confidence ceiling: `%s`\n", decision.Confidence)
	if len(decision.Reasons) > 0 {
		fmt.Fprintf(&b, "- Reasons: `%s`\n", strings.Join(decision.Reasons, "`, `"))
	}
	if len(decision.State.Checked) > 0 {
		fmt.Fprintf(&b, "- Checked evidence: `%d` item(s)\n", len(decision.State.Checked))
	}
	if len(decision.State.NotChecked) > 0 {
		fmt.Fprintf(&b, "- Not checked: `%d` item(s); name them as not checked if relevant.\n", len(decision.State.NotChecked))
	}
	if len(decision.State.FailedTools) > 0 {
		fmt.Fprintf(&b, "- Failed tools: `%d` item(s); do not use them as successful evidence.\n", len(decision.State.FailedTools))
	}
	return b.String()
}
