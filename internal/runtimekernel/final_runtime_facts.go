package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/verification"
)

const (
	FinalCompletionStatusSucceeded = "succeeded"
	FinalCompletionStatusPartial   = "partial"
	FinalCompletionStatusFailed    = "failed"
	FinalCompletionStatusBlocked   = "blocked"
	FinalCompletionStatusUnknown   = "unknown"

	FinalPostcheckStatusNotRequired = "not_required"
	FinalPostcheckStatusPassed      = "passed"
	FinalPostcheckStatusPending     = "pending"
	FinalPostcheckStatusFailed      = "failed"

	FinalRollbackStatusNotRequired  = "not_required"
	FinalRollbackStatusAvailable    = "available"
	FinalRollbackStatusManual       = "manual_takeover"
	FinalRollbackStatusMissing      = "missing"
	FinalRollbackStatusNotPerformed = "not_performed"
)

// FinalRuntimeFacts is the sole business-state input accepted by FinalContract.
// Display text is intentionally absent: it may be cleaned for transport safety,
// but it cannot alter completion, evidence, postcheck, approval, or rollback facts.
type FinalRuntimeFacts struct {
	CompletionStatus string   `json:"completionStatus"`
	ToolOutcomes     []string `json:"toolOutcomes"`
	ApprovalOutcomes []string `json:"approvalOutcomes"`
	EvidenceRefs     []string `json:"evidenceRefs"`
	PostcheckStatus  string   `json:"postcheckStatus"`
	RollbackStatus   string   `json:"rollbackStatus"`
	FailureCodes     []string `json:"failureCodes"`

	EvidenceState    FinalEvidenceState        `json:"-"`
	EvidenceDecision FinalEvidenceVerification `json:"-"`
}

func BuildFinalRuntimeFacts(snapshot *TurnSnapshot, session *SessionState) FinalRuntimeFacts {
	state := BuildFinalEvidenceState(snapshot, session)
	completion := EvaluateVerificationCompletionGate(taskDepthFromSnapshot(snapshot), snapshot)
	planCompletion, planPresent := evaluateRuntimePlanCompletionGate(session, snapshot)
	coverage := EvaluateEvidenceCoverageGate(snapshot)
	policyCompletion := evaluateFinalPolicyCompletion(snapshot, session)
	state.PostChecks = completedPostchecksFromVerification(state.RequiredPostChecks, completion.Report)
	state.Confidence = inferFinalEvidenceConfidence(state)
	evidenceDecision := VerifyFinalEvidenceFacts(state)

	facts := FinalRuntimeFacts{
		CompletionStatus: FinalCompletionStatusUnknown,
		ToolOutcomes:     finalToolOutcomes(snapshot),
		ApprovalOutcomes: finalApprovalOutcomes(snapshot, session),
		EvidenceRefs:     finalTypedEvidenceRefs(snapshot, completion, state),
		PostcheckStatus:  finalPostcheckStatus(state.RequiredPostChecks, state.PostChecks, completion.Report),
		RollbackStatus:   finalRollbackStatus(snapshot),
		EvidenceState:    state,
		EvidenceDecision: evidenceDecision,
	}
	facts.FailureCodes = finalRuntimeFailureCodes(snapshot, session, completion, planCompletion.Action, planCompletion.Reasons, planPresent, coverage, policyCompletion, facts)
	facts.CompletionStatus = finalRuntimeCompletionStatus(completion, facts)
	return facts
}

func taskDepthFromSnapshot(snapshot *TurnSnapshot) taskdepth.Profile {
	if snapshot != nil {
		return snapshot.TaskDepth
	}
	return taskdepth.Profile{}
}

func finalToolOutcomes(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var outcomes []string
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			ref := finalActionRef(invocation.ToolName, invocation.ToolCallID)
			if ref == "" {
				ref = strings.TrimSpace(invocation.ID)
			}
			status := strings.TrimSpace(string(invocation.Status))
			if ref != "" && status != "" {
				outcomes = append(outcomes, ref+":"+status)
			}
		}
	}
	return uniqueSortedHarnessStrings(outcomes)
}

func finalApprovalOutcomes(snapshot *TurnSnapshot, session *SessionState) []string {
	var outcomes []string
	appendApproval := func(id, decision, status string) {
		id = strings.TrimSpace(id)
		outcome := firstNonEmptyString(strings.TrimSpace(decision), strings.TrimSpace(status))
		if id != "" && outcome != "" {
			outcomes = append(outcomes, id+":"+strings.ToLower(outcome))
		}
	}
	if snapshot != nil {
		for _, item := range snapshot.AgentItems {
			if item.Type != agentstate.TurnItemTypeApprovalDecided {
				continue
			}
			var data approvalAgentItemData
			if json.Unmarshal(item.Payload.Data, &data) == nil {
				appendApproval(data.ApprovalID, data.Decision, data.Status)
			}
		}
		for _, approval := range allSnapshotApprovals(snapshot) {
			appendApproval(approval.ID, approval.Decision, approval.Status)
		}
	}
	if session != nil {
		for _, approval := range session.PendingApprovals {
			appendApproval(approval.ID, approval.Decision, approval.Status)
		}
		for _, rejected := range session.RejectedApprovals {
			appendApproval(rejected.ID, rejected.Decision, "rejected")
		}
	}
	return uniqueSortedHarnessStrings(outcomes)
}

func allSnapshotApprovals(snapshot *TurnSnapshot) []PendingApproval {
	if snapshot == nil {
		return nil
	}
	out := append([]PendingApproval(nil), snapshot.PendingApprovals...)
	for _, iteration := range snapshot.Iterations {
		out = append(out, iteration.PendingApprovals...)
	}
	return out
}

func finalTypedEvidenceRefs(snapshot *TurnSnapshot, completion VerificationCompletionDecision, state FinalEvidenceState) []string {
	var refs []string
	if snapshot != nil {
		for _, iteration := range snapshot.Iterations {
			for _, result := range iteration.ToolResults {
				if strings.TrimSpace(result.Error) != "" {
					continue
				}
				if strings.TrimSpace(result.ToolCallID) != "" && checkedEvidenceSummaryForToolResult(toolNameForResult(iteration, result), result) != "" {
					refs = append(refs, "tool_result:"+strings.TrimSpace(result.ToolCallID))
				}
				refs = append(refs, evidenceRefsFromToolResultContent(result.Content)...)
				for _, external := range result.ExternalReferences {
					refs = append(refs, external.ID, external.URI)
				}
			}
		}
	}
	if completion.Report != nil {
		refs = append(refs, completion.Report.RawRefs...)
		for _, evidence := range completion.Report.Evidence {
			refs = append(refs, evidence.RawRef)
		}
		for _, probe := range completion.Report.Probes {
			refs = append(refs, probe.RawRef)
		}
	}
	return uniqueSortedHarnessStrings(refs)
}

func toolNameForResult(iteration IterationState, result ToolResult) string {
	for _, call := range iteration.ToolCalls {
		if call.ID == result.ToolCallID {
			return call.Name
		}
	}
	return ""
}

func completedPostchecksFromVerification(required []string, report *verification.VerificationReport) []string {
	if report == nil || len(required) == 0 {
		return nil
	}
	passed := verificationOutcomeByRef(report, verification.EvidenceResultPass)
	var completed []string
	for _, requirement := range required {
		if passed[normalizeFinalFactRef(requirement)] {
			completed = append(completed, requirement)
		}
	}
	return uniqueSortedHarnessStrings(completed)
}

func finalPostcheckStatus(required, completed []string, report *verification.VerificationReport) string {
	if len(required) == 0 {
		return FinalPostcheckStatusNotRequired
	}
	failed := verificationOutcomeByRef(report, verification.EvidenceResultFail)
	for _, requirement := range required {
		if failed[normalizeFinalFactRef(requirement)] {
			return FinalPostcheckStatusFailed
		}
	}
	if len(outstandingRequiredPostChecks(FinalEvidenceState{PostChecks: completed, RequiredPostChecks: required})) == 0 {
		return FinalPostcheckStatusPassed
	}
	return FinalPostcheckStatusPending
}

func verificationOutcomeByRef(report *verification.VerificationReport, outcome string) map[string]bool {
	values := map[string]bool{}
	if report == nil {
		return values
	}
	for _, evidence := range report.Evidence {
		if evidence.Result == outcome {
			for _, ref := range []string{evidence.Command, evidence.ToolCallID, evidence.RawRef} {
				if normalized := normalizeFinalFactRef(ref); normalized != "" {
					values[normalized] = true
				}
			}
		}
	}
	for _, check := range report.ContractChecks {
		if check.Checked && check.Result == outcome {
			if normalized := normalizeFinalFactRef(check.Name); normalized != "" {
				values[normalized] = true
			}
		}
	}
	return values
}

func normalizeFinalFactRef(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func finalRollbackStatus(snapshot *TurnSnapshot) string {
	if snapshot == nil {
		return FinalRollbackStatusNotRequired
	}
	mutating := map[string]ToolInvocationStatus{}
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Mutating {
				mutating[invocation.ToolCallID] = invocation.Status
			}
		}
	}
	if len(mutating) == 0 {
		return FinalRollbackStatusNotRequired
	}
	hasCompletedMutation := false
	hasRollback := false
	hasManual := false
	for _, status := range mutating {
		if status == ToolInvocationCompleted || status == ToolInvocationPartial {
			hasCompletedMutation = true
		}
	}
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Mutating && invocation.Status == ToolInvocationFailed && invocation.FailureKind == string(toolfailure.KindSideEffectUnknown) {
				hasCompletedMutation = true
			}
		}
	}
	for _, approval := range allSnapshotApprovals(snapshot) {
		if _, ok := mutating[approval.ToolCallID]; !ok || !approval.Mutating {
			continue
		}
		contract := BuildActionRollbackContractFromApproval(approval)
		if contract.ValidateMutating() != nil {
			continue
		}
		if contract.Rollback != "" {
			hasRollback = true
		}
		if contract.Rollback == "" && contract.ManualTakeover != "" {
			hasManual = true
		}
	}
	if hasRollback {
		return FinalRollbackStatusAvailable
	}
	if hasManual {
		return FinalRollbackStatusManual
	}
	if !hasCompletedMutation {
		return FinalRollbackStatusNotPerformed
	}
	return FinalRollbackStatusMissing
}

func finalRuntimeFailureCodes(
	snapshot *TurnSnapshot,
	session *SessionState,
	completion VerificationCompletionDecision,
	planAction string,
	planReasons []string,
	planPresent bool,
	coverage EvidenceCoverageDecision,
	policyCompletion policyengine.PolicyDecision,
	facts FinalRuntimeFacts,
) []string {
	var codes []string
	if completion.Action != VerificationCompletionActionAllow || completion.Status == verification.StatusPartial || completion.Status == verification.StatusFail {
		codes = append(codes, completion.Reasons...)
	}
	codes = append(codes, facts.EvidenceDecision.Reasons...)
	for _, iteration := range snapshotIterations(snapshot) {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Status == ToolInvocationFailed || invocation.Status == ToolInvocationPartial || invocation.Status == ToolInvocationBlocked {
				codes = append(codes, firstNonEmptyString(invocation.FailureKind, string(invocation.Status)))
			}
		}
	}
	for _, failed := range facts.EvidenceState.FailedTools {
		codes = append(codes, strings.TrimSpace(failed.FailureClass))
	}
	for _, missing := range facts.EvidenceState.NotChecked {
		codes = append(codes, strings.TrimSpace(missing.Reason), strings.TrimSpace(missing.RequiredAction))
	}
	pendingApproval, deniedApproval := finalApprovalState(snapshot, session)
	if deniedApproval {
		codes = append(codes, "approval_denied")
	}
	if pendingApproval {
		codes = append(codes, "approval_pending")
	}
	if planPresent && planAction != "allow" {
		codes = append(codes, "plan_completion_blocked")
		codes = append(codes, planReasons...)
	}
	if coverageGateMetadataPresent(snapshot) {
		switch coverage.Action {
		case "continue_gathering":
			codes = append(codes, "coverage_incomplete")
			codes = append(codes, coverage.Reasons...)
		case "blocker_final_allowed":
			codes = append(codes, "coverage_blocked")
		}
	}
	if policyCompletion.Action != policyengine.PolicyActionAllow {
		codes = append(codes, "completion_policy_"+string(policyCompletion.Action), strings.TrimSpace(policyCompletion.Reason))
	}
	if session != nil && len(session.PendingEvidence) > 0 {
		codes = append(codes, "pending_evidence")
	}
	if facts.PostcheckStatus == FinalPostcheckStatusFailed {
		codes = append(codes, "postcheck_failed")
	} else if facts.PostcheckStatus == FinalPostcheckStatusPending {
		codes = append(codes, "postcheck_pending")
	}
	if facts.RollbackStatus == FinalRollbackStatusMissing {
		codes = append(codes, "rollback_contract_missing")
	}
	if len(facts.EvidenceRefs) == 0 && snapshot != nil && (snapshot.TaskDepth.RequiresEvidence || snapshot.TaskDepth.RequiresValidation) {
		codes = append(codes, "missing_typed_evidence")
	}
	return uniqueSortedHarnessStrings(codes)
}

func finalApprovalState(snapshot *TurnSnapshot, session *SessionState) (pending bool, denied bool) {
	check := func(decision, status string) {
		for _, value := range []string{decision, status} {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "pending", "waiting":
				pending = true
			case "denied", "deny", "rejected", "reject":
				denied = true
			}
		}
	}
	if snapshot != nil {
		for _, approval := range allSnapshotApprovals(snapshot) {
			check(approval.Decision, approval.Status)
		}
		for _, item := range snapshot.AgentItems {
			if item.Type != agentstate.TurnItemTypeApprovalDecided {
				continue
			}
			var data approvalAgentItemData
			if json.Unmarshal(item.Payload.Data, &data) == nil {
				check(data.Decision, data.Status)
			}
		}
	}
	if session != nil {
		for _, approval := range session.PendingApprovals {
			check(approval.Decision, approval.Status)
		}
		if len(session.RejectedApprovals) > 0 {
			denied = true
		}
	}
	return pending, denied
}

func evaluateFinalPolicyCompletion(snapshot *TurnSnapshot, session *SessionState) policyengine.PolicyDecision {
	state := policyengine.TurnState{Completed: true}
	if snapshot != nil {
		state.SessionID = snapshot.SessionID
		state.TurnID = snapshot.ID
		for _, iteration := range snapshot.Iterations {
			state.ToolCallCount += len(iteration.ToolCalls)
		}
	}
	if session != nil {
		if state.SessionID == "" {
			state.SessionID = session.ID
		}
		for _, approval := range session.PendingApprovals {
			if pendingStatus(approval.Status) {
				state.PendingApprovals = append(state.PendingApprovals, approval.ID)
			}
		}
		for _, evidence := range session.PendingEvidence {
			if pendingStatus(evidence.Status) {
				state.PendingEvidence = append(state.PendingEvidence, evidence.ID)
			}
		}
	}
	return (&policyengine.DefaultCompletionEvaluator{}).CheckCompletion(context.Background(), state)
}

func snapshotIterations(snapshot *TurnSnapshot) []IterationState {
	if snapshot == nil {
		return nil
	}
	return snapshot.Iterations
}

func finalRuntimeCompletionStatus(completion VerificationCompletionDecision, facts FinalRuntimeFacts) string {
	if containsFinalRuntimeCode(facts.FailureCodes, "approval_denied") || containsFinalRuntimeCode(facts.FailureCodes, "approval_pending") {
		return FinalCompletionStatusBlocked
	}
	if completion.Status == verification.StatusFail || facts.PostcheckStatus == FinalPostcheckStatusFailed {
		return FinalCompletionStatusFailed
	}
	if completion.Action == VerificationCompletionActionBlockSuccessFinal || completion.Action == VerificationCompletionActionRequireBlockerFinal ||
		facts.EvidenceDecision.Action == FinalEvidenceActionBlock || containsFinalRuntimeCode(facts.FailureCodes, "plan_completion_blocked") ||
		containsFinalRuntimeCode(facts.FailureCodes, "coverage_incomplete") || containsFinalRuntimeCode(facts.FailureCodes, "completion_policy_deny") ||
		containsFinalRuntimeCode(facts.FailureCodes, "completion_policy_need_evidence") {
		return FinalCompletionStatusBlocked
	}
	if completion.Status == verification.StatusPartial || facts.EvidenceDecision.Action == FinalEvidenceActionDowngrade ||
		facts.PostcheckStatus == FinalPostcheckStatusPending || facts.RollbackStatus == FinalRollbackStatusMissing {
		return FinalCompletionStatusPartial
	}
	if completion.Status == verification.StatusPass && len(facts.EvidenceRefs) > 0 {
		return FinalCompletionStatusSucceeded
	}
	if completion.Action == VerificationCompletionActionAllow && len(facts.EvidenceState.FailedTools) == 0 &&
		len(facts.EvidenceState.NotChecked) == 0 && len(facts.EvidenceRefs) > 0 {
		return FinalCompletionStatusSucceeded
	}
	return FinalCompletionStatusUnknown
}

func containsFinalRuntimeCode(codes []string, target string) bool {
	for _, code := range codes {
		if strings.EqualFold(strings.TrimSpace(code), target) {
			return true
		}
	}
	return false
}

func finalEvidenceBoundaryFromFacts(facts FinalRuntimeFacts) string {
	if facts.CompletionStatus == FinalCompletionStatusSucceeded {
		return "sufficient"
	}
	return "limited"
}
