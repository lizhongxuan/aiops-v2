package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/verification"
)

const verificationCompletionGateRetryMetadataKey = "verificationCompletionGate.retry"

const (
	VerificationCompletionActionAllow               = "allow"
	VerificationCompletionActionBlockSuccessFinal   = "block_success_final"
	VerificationCompletionActionRequireBlockerFinal = "require_blocker_final"
)

type VerificationCompletionDecision struct {
	Action      string                               `json:"action"`
	Requirement verification.VerificationRequirement `json:"requirement,omitempty"`
	Status      verification.ReportStatus            `json:"status,omitempty"`
	ReportRef   string                               `json:"reportRef,omitempty"`
	Report      *verification.VerificationReport     `json:"report,omitempty"`
	Validation  verification.ValidationDecision      `json:"validation,omitempty"`
	Reasons     []string                             `json:"reasons,omitempty"`
}

func EvaluateVerificationCompletionGate(profile taskdepth.Profile, snapshot *TurnSnapshot) VerificationCompletionDecision {
	requirement := verificationRequirementFromTaskDepth(profile)
	report, reportRef, ok := latestVerificationReportFromSnapshot(snapshot)
	decision := VerificationCompletionDecision{
		Action:      VerificationCompletionActionAllow,
		Requirement: requirement,
		ReportRef:   strings.TrimSpace(reportRef),
	}
	if !ok {
		if requirement == verification.VerificationExecutionRequired {
			decision.Action = VerificationCompletionActionBlockSuccessFinal
			decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "execution_required")
			decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "missing_verification_report")
			if verificationCompletionRuntimeApprovalGateMissing(snapshot) {
				decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "missing_runtime_approval_gate")
			}
		}
		return decision
	}

	normalized := report.Normalize()
	decision.Report = &normalized
	decision.Status = normalized.Status
	validation := verification.ValidateReport(normalized)
	decision.Validation = validation
	if requirement == verification.VerificationExecutionRequired && normalized.Requirement != verification.VerificationExecutionRequired {
		validation.Valid = false
		validation.Reasons = appendVerificationCompletionReason(validation.Reasons, "verification_requirement_mismatch")
		decision.Validation = validation
	}
	if !validation.Valid {
		decision.Action = VerificationCompletionActionBlockSuccessFinal
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "invalid_verification_report")
		for _, reason := range validation.Reasons {
			decision.Reasons = appendVerificationCompletionReason(decision.Reasons, reason)
		}
		return decision
	}

	switch normalized.Status {
	case verification.StatusPass:
		decision.Action = VerificationCompletionActionAllow
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, string(normalized.Requirement))
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "verification_pass")
	case verification.StatusPartial:
		decision.Action = VerificationCompletionActionRequireBlockerFinal
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "partial_requires_blocker")
		for _, blocker := range normalized.Blockers {
			if blocker.NextAction != "" {
				decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "next_action")
				break
			}
		}
	case verification.StatusFail:
		decision.Action = VerificationCompletionActionBlockSuccessFinal
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "verification_failed")
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "expected")
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "actual")
	default:
		decision.Action = VerificationCompletionActionBlockSuccessFinal
		decision.Reasons = appendVerificationCompletionReason(decision.Reasons, "unknown_status")
	}
	return decision
}

func verificationRequirementFromTaskDepth(profile taskdepth.Profile) verification.VerificationRequirement {
	if profile.AnalysisOnly {
		return verification.VerificationAnalysisAllowed
	}
	if profile.RequiresValidation ||
		profile.RequiresEvidence ||
		taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiStep) ||
		taskdepth.AtLeast(profile.Level, taskdepth.LevelInvestigation) ||
		taskdepth.AtLeast(profile.Level, taskdepth.LevelOperations) ||
		taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiAgent) {
		return verification.VerificationExecutionRequired
	}
	if taskdepth.AtLeast(profile.Level, taskdepth.LevelSimpleRead) || profile.Level == taskdepth.LevelTrivial || profile.Level == "" {
		return verification.VerificationAnalysisAllowed
	}
	return verification.VerificationAnalysisAllowed
}

func verificationCompletionGateAllowsFinal(answer string, decision VerificationCompletionDecision, snapshot *TurnSnapshot) bool {
	switch decision.Action {
	case "", VerificationCompletionActionAllow:
		return true
	case VerificationCompletionActionRequireBlockerFinal, VerificationCompletionActionBlockSuccessFinal:
		if safeTerminal := EvaluateSafeTerminalFinal(answer); len(safeTerminal.TerminalStates) > 0 {
			return safeTerminal.Valid
		}
		if verificationCompletionMissingReport(decision) && verificationCompletionMissingRuntimeApprovalGate(decision) {
			return false
		}
		if verificationCompletionMissingReport(decision) && !finalAnswerClaimsVerificationCompletion(answer) {
			return true
		}
		if verificationCompletionMissingReport(decision) && finalAnswerHasEvidenceBackedVerification(answer, snapshot) {
			return true
		}
		return finalLooksLikeVerificationBlocker(answer)
	default:
		if safeTerminal := EvaluateSafeTerminalFinal(answer); len(safeTerminal.TerminalStates) > 0 {
			return safeTerminal.Valid
		}
		return finalLooksLikeVerificationBlocker(answer)
	}
}

func verificationCompletionMissingReport(decision VerificationCompletionDecision) bool {
	for _, reason := range decision.Reasons {
		if reason == "missing_verification_report" {
			return true
		}
	}
	return false
}

func verificationCompletionMissingRuntimeApprovalGate(decision VerificationCompletionDecision) bool {
	for _, reason := range decision.Reasons {
		if reason == "missing_runtime_approval_gate" {
			return true
		}
	}
	return false
}

func verificationCompletionRuntimeApprovalGateMissing(snapshot *TurnSnapshot) bool {
	return verificationCompletionRuntimeApprovalGateRequired(snapshot) && !verificationCompletionRuntimeApprovalGateObserved(snapshot)
}

func verificationCompletionRuntimeApprovalGateRequired(snapshot *TurnSnapshot) bool {
	if snapshot == nil || snapshot.TaskDepth.AnalysisOnly || snapshot.TaskDepth.ExecutionProhibited {
		return false
	}
	metadata := snapshot.Metadata
	if !strings.EqualFold(strings.TrimSpace(metadata["aiops.intent.kind"]), "change") {
		return false
	}
	if metadataBool(metadata["aiops.route.userProhibitedHostExec"]) || metadataBool(metadata["aiops.execution.prohibited"]) {
		return false
	}
	if !metadataBool(metadata["aiops.tool.execCommandAllowed"]) && !metadataBool(metadata["aiops.route.allowsExecCommand"]) {
		return false
	}
	if !metadataBool(metadata["aiops.tool.hostMutationAllowed"]) {
		return false
	}
	return verificationCompletionHasTargetBinding(snapshot)
}

func verificationCompletionHasTargetBinding(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	metadata := snapshot.Metadata
	switch strings.ToLower(strings.TrimSpace(metadata["aiops.target.binding"])) {
	case "host", "multi_host", "resource":
		return true
	}
	return strings.TrimSpace(metadata["aiops.target.hostId"]) != "" ||
		strings.TrimSpace(metadata["aiops.target.refs"]) != ""
}

func verificationCompletionRuntimeApprovalGateObserved(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	if len(snapshotPendingApprovals(snapshot)) > 0 {
		return true
	}
	for _, iter := range snapshot.Iterations {
		for _, approval := range iter.PendingApprovals {
			if pendingStatus(approval.Status) {
				return true
			}
		}
		for _, invocation := range iter.ToolInvocations {
			if invocation.Mutating && invocation.Status != "" {
				return true
			}
		}
		if len(iter.ResourceLocks) > 0 {
			return true
		}
	}
	for _, item := range snapshot.AgentItems {
		if item.Type == agentstate.TurnItemTypeToolCall && item.Status == agentstate.ItemStatusBlocked {
			return true
		}
	}
	return false
}

func finalAnswerClaimsVerificationCompletion(answer string) bool {
	text := strings.ToLower(strings.TrimSpace(answer))
	for _, marker := range []string{
		"已完成", "完成了", "修复完成", "已修复", "已验证", "验证通过", "全部通过", "问题已解决",
		"verified", "fixed", "resolved", "all checks passed",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func finalLooksLikeVerificationBlocker(answer string) bool {
	if finalLooksLikeBlocker(answer) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(answer))
	for _, marker := range []string{"不能", "未完成", "未通过", "失败", "阻塞", "缺失", "partial", "failed", "incomplete", "cannot complete", "missing verification"} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func finalAnswerHasEvidenceBackedVerification(answer string, snapshot *TurnSnapshot) bool {
	if snapshot == nil || !finalAnswerClaimsVerificationCompletion(answer) {
		return false
	}
	terms := successfulVerificationEvidenceTerms(snapshot)
	if len(terms) == 0 {
		return false
	}
	answerText := normalizeVerificationEvidenceText(answer)
	matches := 0
	for _, term := range terms {
		term = normalizeVerificationEvidenceText(term)
		if term == "" {
			continue
		}
		if strings.Contains(answerText, term) {
			matches++
		}
	}
	return matches >= 2
}

func successfulVerificationEvidenceTerms(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	seen := map[string]bool{}
	var terms []string
	addTerm := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeVerificationEvidenceText(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		terms = append(terms, value)
	}
	for _, iter := range snapshot.Iterations {
		calls := map[string]ToolCall{}
		for _, call := range iter.ToolCalls {
			calls[call.ID] = call
		}
		for _, result := range iter.ToolResults {
			if strings.TrimSpace(result.Error) != "" {
				continue
			}
			if env := terminalEnvelopeFromToolResultContent(result.Content); env != nil {
				for _, term := range verificationEvidenceTermsFromCommand(env.Command) {
					addTerm(term)
				}
			}
			if call, ok := calls[result.ToolCallID]; ok {
				for _, term := range verificationEvidenceTermsFromToolCall(call) {
					addTerm(term)
				}
			}
		}
	}
	return terms
}

func verificationEvidenceTermsFromToolCall(call ToolCall) []string {
	var terms []string
	command := approvalCommandForToolCall(call)
	terms = append(terms, verificationEvidenceTermsFromCommand(command)...)
	name := strings.TrimSpace(call.Name)
	switch name {
	case "", "exec_command", "tool_search", "list_mcp_resources", "read_mcp_resource", "web_search", "browse_url":
	default:
		terms = append(terms, name)
	}
	return terms
}

func verificationEvidenceTermsFromCommand(command string) []string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return nil
	}
	base := fields[0]
	var terms []string
	if len(fields) >= 2 {
		terms = append(terms, base+" "+fields[1])
	}
	for _, field := range fields {
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			terms = append(terms, base+" "+field, field)
			break
		}
	}
	if base != "exec_command" && base != "tool_search" {
		terms = append(terms, base)
	}
	return terms
}

func normalizeVerificationEvidenceText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func verificationCompletionGateTrace(decision VerificationCompletionDecision) *promptinput.CompletionGateTrace {
	if decision.Action == "" || (decision.Action == VerificationCompletionActionAllow && len(decision.Reasons) == 0) {
		return nil
	}
	return &promptinput.CompletionGateTrace{
		Decision: decision.Action,
		Reasons:  append([]string(nil), decision.Reasons...),
	}
}

func verificationCompletionGateRetryPrompt(decision VerificationCompletionDecision) string {
	prompt := fmt.Sprintf("## Verification completion gate\nThe current answer cannot claim successful completion yet. Gate decision: %s. Verification requirement: %s. Status: %s. Reasons: %s. Continue by producing or fixing a structured VerificationReport, gather required evidence, or state the blocker/failure explicitly instead of claiming success.",
		firstNonBlankRuntimeString(decision.Action, VerificationCompletionActionBlockSuccessFinal),
		decision.Requirement,
		decision.Status,
		strings.Join(decision.Reasons, ", "),
	)
	if verificationCompletionMissingRuntimeApprovalGate(decision) {
		prompt += " This is a scoped mutating operation. Do not ask for approval in prose; invoke the scoped mutating tool so the runtime approval gate can pause before execution, then resume with post-change validation after approval."
	}
	return prompt
}

func appendVerificationCompletionGateItem(snapshot *TurnSnapshot, turnID string, iteration int, decision VerificationCompletionDecision) {
	if snapshot == nil || decision.Action == "" {
		return
	}
	if decision.Action == VerificationCompletionActionAllow && decision.Status == "" && len(decision.Reasons) == 0 {
		return
	}
	itemID := fmt.Sprintf("%s-verification-completion-gate-%d", turnID, iteration)
	status := agentstate.ItemStatusCompleted
	switch decision.Action {
	case VerificationCompletionActionRequireBlockerFinal, VerificationCompletionActionBlockSuccessFinal:
		status = agentstate.ItemStatusBlocked
	}
	if decision.Status == verification.StatusFail {
		status = agentstate.ItemStatusFailed
	}
	if hasAgentItemID(snapshot.AgentItems, itemID) {
		updateAgentItem(snapshot, itemID, status, verificationCompletionGateSummary(decision))
		return
	}
	appendAgentItem(snapshot, newAgentItem(
		itemID,
		agentstate.TurnItemTypeEvidence,
		status,
		verificationCompletionGateSummary(decision),
		decision,
	))
}

func verificationCompletionGateSummary(decision VerificationCompletionDecision) string {
	parts := []string{"verification completion gate", firstNonBlankRuntimeString(decision.Action, VerificationCompletionActionAllow)}
	if decision.Status != "" {
		parts = append(parts, string(decision.Status))
	}
	if decision.ReportRef != "" {
		parts = append(parts, decision.ReportRef)
	}
	if len(decision.Reasons) > 0 {
		parts = append(parts, strings.Join(decision.Reasons, ","))
	}
	return strings.Join(parts, ": ")
}

func latestVerificationReportFromSnapshot(snapshot *TurnSnapshot) (verification.VerificationReport, string, bool) {
	if snapshot == nil {
		return verification.VerificationReport{}, "", false
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		iter := snapshot.Iterations[i]
		for j := len(iter.ToolResults) - 1; j >= 0; j-- {
			result := iter.ToolResults[j]
			if report, ok := verificationReportFromToolResult(result); ok {
				ref := firstNonBlankRuntimeString(report.ID, result.ToolCallID, fmt.Sprintf("verification-report-%d-%d", i, j))
				return report.Normalize(), ref, true
			}
		}
	}
	return verification.VerificationReport{}, "", false
}

func verificationReportFromToolResult(result ToolResult) (verification.VerificationReport, bool) {
	if report, ok := parseVerificationReportJSON([]byte(result.Content)); ok {
		return report, true
	}
	if result.Display != nil {
		if strings.EqualFold(strings.TrimSpace(result.Display.Type), "verification_report") {
			if report, ok := parseVerificationReportJSON(result.Display.Data); ok {
				return report, true
			}
		}
		if report, ok := parseVerificationReportJSON(result.Display.Data); ok {
			return report, true
		}
	}
	return verification.VerificationReport{}, false
}

func parseVerificationReportJSON(data []byte) (verification.VerificationReport, bool) {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 || !json.Valid(data) {
		return verification.VerificationReport{}, false
	}
	var report verification.VerificationReport
	if err := json.Unmarshal(data, &report); err == nil && looksLikeTopLevelVerificationReport(report) {
		return report, true
	}
	var wrapped struct {
		VerificationReport verification.VerificationReport `json:"verificationReport"`
		Report             verification.VerificationReport `json:"report"`
		Data               struct {
			VerificationReport verification.VerificationReport `json:"verificationReport"`
			Report             verification.VerificationReport `json:"report"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return verification.VerificationReport{}, false
	}
	for _, candidate := range []verification.VerificationReport{
		wrapped.VerificationReport,
		wrapped.Report,
		wrapped.Data.VerificationReport,
		wrapped.Data.Report,
	} {
		if looksLikeVerificationReport(candidate) {
			return candidate, true
		}
	}
	return verification.VerificationReport{}, false
}

func looksLikeVerificationReport(report verification.VerificationReport) bool {
	report = report.Normalize()
	return report.ID != "" ||
		report.Status != "" ||
		report.Requirement != "" ||
		report.Subject != "" ||
		len(report.Evidence) > 0 ||
		len(report.Blockers) > 0
}

func looksLikeTopLevelVerificationReport(report verification.VerificationReport) bool {
	report = report.Normalize()
	return report.ID != "" ||
		report.Requirement != "" ||
		report.Subject != "" ||
		len(report.Evidence) > 0 ||
		len(report.Probes) > 0 ||
		len(report.ContractChecks) > 0 ||
		len(report.Blockers) > 0 ||
		(report.Expected != "" && report.Actual != "")
}

func appendVerificationCompletionReason(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
