package runtimekernel

import (
	"fmt"
	"strings"
)

const FinalContractSchemaVersion = "aiops.harness.final.v1"

const FinalContractLimitationInvalidVerifiedFacts = "invalid_verified_contract_facts"

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
	RequiredPostChecks    []string            `json:"requiredPostChecks,omitempty"`
	Limitations           []string            `json:"limitations,omitempty"`
}

func (contract FinalContract) Validate() error {
	failures := finalContractVerifiedInvariantFailures(contract)
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", FinalContractLimitationInvalidVerifiedFacts, strings.Join(failures, ","))
}

// NormalizeForProjection fails closed without making a malformed persisted
// contract prevent the rest of the turn from being hydrated.
func (contract FinalContract) NormalizeForProjection() FinalContract {
	if len(finalContractVerifiedInvariantFailures(contract)) == 0 {
		return contract
	}
	contract.Status = FinalContractStatusNeedsEvidence
	contract.Limitations = uniqueSortedHarnessStrings(append(
		append([]string(nil), contract.Limitations...),
		FinalContractLimitationInvalidVerifiedFacts,
	))
	return contract
}

func finalContractVerifiedInvariantFailures(contract FinalContract) []string {
	if contract.Status != FinalContractStatusVerified {
		return nil
	}
	var failures []string
	if len(uniqueSortedHarnessStrings(contract.CheckedEvidenceRefs)) == 0 {
		failures = append(failures, "missing_checked_evidence")
	}
	if len(uniqueSortedHarnessStrings(contract.UncheckedRequirements)) > 0 {
		failures = append(failures, "unchecked_requirements_present")
	}
	state := FinalEvidenceState{
		PostChecks:         contract.PostChecks,
		RequiredPostChecks: contract.RequiredPostChecks,
	}
	if len(outstandingRequiredPostChecks(state)) > 0 {
		failures = append(failures, "required_postchecks_incomplete")
	}
	return failures
}

func BuildFinalContract(answer string, facts FinalRuntimeFacts) FinalContract {
	state := facts.EvidenceState
	verification := facts.EvidenceDecision
	confidence := firstNonEmptyString(verification.Confidence, state.Confidence)
	if strings.TrimSpace(confidence) != "" {
		confidence = normalizeFinalEvidenceConfidence(confidence)
	}
	if len(outstandingRequiredPostChecks(state)) > 0 {
		cap := FinalEvidenceConfidenceMedium
		if len(state.Checked) == 0 {
			cap = FinalEvidenceConfidenceLow
		}
		if strings.TrimSpace(confidence) == "" {
			confidence = cap
		} else {
			confidence = minFinalEvidenceConfidence(confidence, cap)
		}
	}
	return FinalContract{
		SchemaVersion:         FinalContractSchemaVersion,
		Status:                classifyFinalContractStatus(facts),
		Confidence:            confidence,
		AnswerText:            strings.TrimSpace(answer),
		CheckedEvidenceRefs:   append([]string(nil), facts.EvidenceRefs...),
		UncheckedRequirements: uncheckedRequirementRefs(state.NotChecked),
		FailedToolImpacts:     append([]FailedToolImpact(nil), state.FailedTools...),
		ApprovedActions:       append([]string(nil), state.ApprovedActions...),
		PerformedActions:      append([]string(nil), state.PerformedActions...),
		PostChecks:            append([]string(nil), state.PostChecks...),
		RequiredPostChecks:    append([]string(nil), state.RequiredPostChecks...),
		Limitations:           finalContractLimitations(facts),
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

func classifyFinalContractStatus(facts FinalRuntimeFacts) FinalContractStatus {
	if containsFinalRuntimeCode(facts.FailureCodes, "approval_denied") {
		return FinalContractStatusApprovalDenied
	}
	state := facts.EvidenceState
	if finalContractHasToolUnavailable(facts.FailureCodes) {
		return FinalContractStatusToolUnavailable
	}
	switch facts.CompletionStatus {
	case FinalCompletionStatusSucceeded:
		if facts.PostcheckStatus == FinalPostcheckStatusFailed {
			return FinalContractStatusFailed
		}
		if facts.PostcheckStatus != FinalPostcheckStatusPassed && facts.PostcheckStatus != FinalPostcheckStatusNotRequired {
			return FinalContractStatusNeedsEvidence
		}
		if len(facts.EvidenceRefs) == 0 || len(state.NotChecked) > 0 || len(outstandingRequiredPostChecks(state)) > 0 {
			return FinalContractStatusNeedsEvidence
		}
		return FinalContractStatusVerified
	case FinalCompletionStatusPartial:
		if len(state.NotChecked) > 0 || facts.PostcheckStatus == FinalPostcheckStatusPending ||
			containsFinalRuntimeCode(facts.FailureCodes, "missing_typed_evidence") {
			return FinalContractStatusNeedsEvidence
		}
		return FinalContractStatusPartial
	case FinalCompletionStatusBlocked:
		if state.MutationIntentWithoutTarget || containsFinalRuntimeCode(facts.FailureCodes, "approval_pending") {
			return FinalContractStatusBlocked
		}
		if len(state.NotChecked) > 0 || facts.PostcheckStatus == FinalPostcheckStatusPending ||
			containsFinalRuntimeCode(facts.FailureCodes, "missing_typed_evidence") ||
			containsFinalRuntimeCode(facts.FailureCodes, "missing_verification_report") {
			return FinalContractStatusNeedsEvidence
		}
		return FinalContractStatusBlocked
	case FinalCompletionStatusFailed:
		return FinalContractStatusFailed
	}
	return FinalContractStatusUnknown
}

func outstandingRequiredPostChecks(state FinalEvidenceState) []string {
	completed := make(map[string]bool, len(state.PostChecks))
	for _, check := range state.PostChecks {
		if normalized := strings.TrimSpace(check); normalized != "" {
			completed[normalized] = true
		}
	}
	out := make([]string, 0, len(state.RequiredPostChecks))
	for _, check := range uniqueSortedHarnessStrings(state.RequiredPostChecks) {
		if !completed[check] {
			out = append(out, check)
		}
	}
	return out
}

func finalContractHasToolUnavailable(failureCodes []string) bool {
	for _, code := range failureCodes {
		switch strings.ToLower(strings.TrimSpace(code)) {
		case "needs_host_agent", "tool_unavailable", "tool_not_found", "tool_not_dispatchable",
			"not_dispatchable", "host_agent_unavailable", "mcp_unavailable":
			return true
		}
	}
	return false
}

func finalContractLimitations(facts FinalRuntimeFacts) []string {
	values := append([]string(nil), facts.FailureCodes...)
	values = append(values, facts.EvidenceDecision.Reasons...)
	for _, missing := range facts.EvidenceState.NotChecked {
		if missing.Reason != "" {
			values = append(values, missing.ToolName+":"+missing.Reason)
		}
	}
	for _, failed := range facts.EvidenceState.FailedTools {
		if failed.FailureClass != "" {
			values = append(values, failed.ToolName+":"+failed.FailureClass)
		}
	}
	return uniqueSortedHarnessStrings(values)
}
