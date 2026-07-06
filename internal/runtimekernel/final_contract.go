package runtimekernel

import "strings"

const FinalContractSchemaVersion = "aiops.harness.final.v1"

type FinalContractStatus string

const (
	FinalContractStatusVerified        FinalContractStatus = "verified"
	FinalContractStatusPartial         FinalContractStatus = "partial"
	FinalContractStatusBlocked         FinalContractStatus = "blocked"
	FinalContractStatusNeedsEvidence   FinalContractStatus = "needs_evidence"
	FinalContractStatusApprovalDenied  FinalContractStatus = "approval_denied"
	FinalContractStatusToolUnavailable FinalContractStatus = "tool_unavailable"
	FinalContractStatusCancelled       FinalContractStatus = "cancelled"
	FinalContractStatusFailed          FinalContractStatus = "failed"
	FinalContractStatusUnknown         FinalContractStatus = "unknown"
)

type FinalContract struct {
	SchemaVersion         string              `json:"schemaVersion"`
	Status                FinalContractStatus `json:"status"`
	Confidence            string              `json:"confidence,omitempty"`
	AnswerText            string              `json:"answerText,omitempty"`
	CheckedEvidenceRefs   []string            `json:"checkedEvidenceRefs,omitempty"`
	UncheckedRequirements []string            `json:"uncheckedRequirements,omitempty"`
	FailedToolImpacts     []FailedToolImpact  `json:"failedToolImpacts,omitempty"`
	ApprovedActions       []string            `json:"approvedActions,omitempty"`
	PerformedActions      []string            `json:"performedActions,omitempty"`
	PostChecks            []string            `json:"postChecks,omitempty"`
	Limitations           []string            `json:"limitations,omitempty"`
}

func BuildFinalContract(answer string, verification FinalEvidenceVerification) FinalContract {
	state := verification.State
	confidence := firstNonEmptyString(verification.Confidence, state.Confidence)
	if strings.TrimSpace(confidence) != "" {
		confidence = normalizeFinalEvidenceConfidence(confidence)
	}
	return FinalContract{
		SchemaVersion:         FinalContractSchemaVersion,
		Status:                classifyFinalContractStatus(answer, verification),
		Confidence:            confidence,
		AnswerText:            strings.TrimSpace(answer),
		CheckedEvidenceRefs:   checkedEvidenceRefs(state.Checked),
		UncheckedRequirements: uncheckedRequirementRefs(state.NotChecked),
		FailedToolImpacts:     append([]FailedToolImpact(nil), state.FailedTools...),
		Limitations:           finalContractLimitations(verification),
	}
}

func BuildTerminalFinalContract(answer string, status FinalContractStatus, limitations []string) FinalContract {
	status = normalizeFinalContractStatus(status)
	return FinalContract{
		SchemaVersion: FinalContractSchemaVersion,
		Status:        status,
		Confidence:    FinalEvidenceConfidenceLow,
		AnswerText:    strings.TrimSpace(answer),
		Limitations:   uniqueSortedHarnessStrings(limitations),
	}
}

func normalizeFinalContractStatus(status FinalContractStatus) FinalContractStatus {
	switch status {
	case FinalContractStatusVerified,
		FinalContractStatusPartial,
		FinalContractStatusBlocked,
		FinalContractStatusNeedsEvidence,
		FinalContractStatusApprovalDenied,
		FinalContractStatusToolUnavailable,
		FinalContractStatusCancelled,
		FinalContractStatusFailed:
		return status
	default:
		return FinalContractStatusUnknown
	}
}

func classifyFinalContractStatus(answer string, verification FinalEvidenceVerification) FinalContractStatus {
	if finalContractHasApprovalDenied(answer, verification) {
		return FinalContractStatusApprovalDenied
	}
	state := verification.State
	if finalContractHasToolUnavailable(state) {
		return FinalContractStatusToolUnavailable
	}
	switch verification.Action {
	case FinalEvidenceActionBlock:
		if state.MutationIntentWithoutTarget || finalEvidenceHasReason(verification, "mutation_intent_requires_explicit_target_binding") {
			return FinalContractStatusBlocked
		}
		if len(state.NotChecked) > 0 ||
			finalEvidenceHasReason(verification, "checked_claim_without_checked_evidence") ||
			finalEvidenceHasReason(verification, "not_checked_item_requires_lower_confidence") {
			return FinalContractStatusNeedsEvidence
		}
		return FinalContractStatusBlocked
	case FinalEvidenceActionDowngrade:
		if len(state.NotChecked) > 0 ||
			finalEvidenceHasReason(verification, "checked_claim_without_checked_evidence") ||
			(len(state.Checked) == 0 && finalAnswerClaimsChecked(answer)) {
			return FinalContractStatusNeedsEvidence
		}
		if len(state.FailedTools) > 0 {
			return FinalContractStatusPartial
		}
		return FinalContractStatusPartial
	case FinalEvidenceActionAllow:
		if finalEvidenceHasReason(verification, "partial_mutation") {
			return FinalContractStatusPartial
		}
		if len(state.FailedTools) > 0 {
			return FinalContractStatusPartial
		}
		if len(state.Checked) > 0 && len(state.NotChecked) == 0 && len(state.FailedTools) == 0 {
			return FinalContractStatusVerified
		}
	}
	return FinalContractStatusUnknown
}

func finalContractHasApprovalDenied(answer string, verification FinalEvidenceVerification) bool {
	if finalEvidenceHasReason(verification, "approval_denied") {
		return true
	}
	compact := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(answer)), " ", "")
	return strings.Contains(compact, `"status":"approval_denied"`)
}

func finalContractHasToolUnavailable(state FinalEvidenceState) bool {
	for _, failed := range state.FailedTools {
		if finalContractUnavailableMarker(failed.FailureClass) ||
			finalContractUnavailableMarker(failed.Impact) ||
			finalContractUnavailableMarker(failed.ToolName) {
			return true
		}
	}
	for _, missing := range state.NotChecked {
		if finalContractUnavailableMarker(missing.Reason) ||
			finalContractUnavailableMarker(missing.RequiredAction) {
			return true
		}
	}
	return false
}

func finalContractUnavailableMarker(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, marker := range []string{
		"needs_host_agent",
		"tool_unavailable",
		"tool_not_found",
		"tool_not_dispatchable",
		"not_dispatchable",
		"host_agent_unavailable",
		"agent 7072",
		"7072 refused",
		"mcp_unavailable",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func finalContractLimitations(verification FinalEvidenceVerification) []string {
	values := append([]string(nil), verification.Reasons...)
	for _, missing := range verification.State.NotChecked {
		if missing.Reason != "" {
			values = append(values, missing.ToolName+":"+missing.Reason)
		}
	}
	for _, failed := range verification.State.FailedTools {
		if failed.FailureClass != "" {
			values = append(values, failed.ToolName+":"+failed.FailureClass)
		}
	}
	return uniqueSortedHarnessStrings(values)
}
