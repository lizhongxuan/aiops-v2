package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/taskdepth"
)

type UXProgressTrace struct {
	TurnID           string   `json:"turnId"`
	TaskDepth        string   `json:"taskDepth,omitempty"`
	Phase            string   `json:"phase"`
	CurrentStepID    string   `json:"currentStepId,omitempty"`
	PendingApprovals []string `json:"pendingApprovals,omitempty"`
	ChildAgents      []string `json:"childAgents,omitempty"`
	Blockers         []string `json:"blockers,omitempty"`
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`
}

type InstructionReconcileDecision struct {
	Action           string   `json:"action"`
	Reasons          []string `json:"reasons,omitempty"`
	CancelledStepIDs []string `json:"cancelledStepIds,omitempty"`
	BlockedStepIDs   []string `json:"blockedStepIds,omitempty"`
	NewStepIDs        []string `json:"newStepIds,omitempty"`
}

type ResumeContinuationPolicy struct {
	Action       string   `json:"action"`
	NextStepID   string   `json:"nextStepId,omitempty"`
	RecapAllowed bool     `json:"recapAllowed"`
	Reasons      []string `json:"reasons,omitempty"`
}

type EvidenceCoverageDecision struct {
	Action             string   `json:"action"`
	Coverage           float64  `json:"coverage"`
	RequiredDimensions []string `json:"requiredDimensions,omitempty"`
	CoveredDimensions  []string `json:"coveredDimensions,omitempty"`
	MissingDimensions  []string `json:"missingDimensions,omitempty"`
	OpenQuestions      []string `json:"openQuestions,omitempty"`
	VerificationStatus string   `json:"verificationStatus,omitempty"`
	Reasons            []string `json:"reasons,omitempty"`
}

type ManagerSynthesisGate struct {
	Action           string   `json:"action"`
	ManagerAnswerRef string   `json:"managerAnswerRef,omitempty"`
	WorkerOutputRefs []string `json:"workerOutputRefs,omitempty"`
	Reasons          []string `json:"reasons,omitempty"`
}

type CompletionReadinessDecision struct {
	Action             string   `json:"action"`
	MissingDimensions  []string `json:"missingDimensions,omitempty"`
	OpenQuestions      []string `json:"openQuestions,omitempty"`
	VerificationStatus string   `json:"verificationStatus,omitempty"`
	Reasons            []string `json:"reasons,omitempty"`
}

type FailureSignatureDecision struct {
	Signature        string   `json:"signature"`
	SeenCount        int      `json:"seenCount"`
	Action           string   `json:"action"`
	SwitchPathReason string   `json:"switchPathReason,omitempty"`
	Reasons          []string `json:"reasons,omitempty"`
}

func BuildUXProgressTrace(snapshot *TurnSnapshot) UXProgressTrace {
	if snapshot == nil {
		return UXProgressTrace{Phase: "completed"}
	}
	trace := UXProgressTrace{
		TurnID:    strings.TrimSpace(snapshot.ID),
		TaskDepth: strings.TrimSpace(string(snapshot.TaskDepth.Level)),
		Phase:     "executing",
	}
	if trace.TaskDepth == "" {
		trace.TaskDepth = strings.TrimSpace(snapshot.Metadata["taskDepth"])
	}
	plan, hasPlan := latestPlanFromAgentItems(snapshot.AgentItems)
	if hasPlan {
		for _, step := range plan.Steps {
			switch step.Status {
			case planning.StepStatusInProgress:
				if trace.CurrentStepID == "" {
					trace.CurrentStepID = strings.TrimSpace(step.ID)
				}
			case planning.StepStatusBlocked, planning.StepStatusFailed:
				trace.Blockers = appendUniqueSorted(trace.Blockers, firstNonEmpty(step.ID, step.Text))
			}
			trace.ChildAgents = appendUniqueSorted(trace.ChildAgents, step.AgentID)
			trace.EvidenceRefs = appendUniqueSorted(trace.EvidenceRefs, step.EvidenceRefs...)
		}
	}
	for _, approval := range snapshot.PendingApprovals {
		if pendingStatus(approval.Status) {
			trace.PendingApprovals = appendUniqueSorted(trace.PendingApprovals, approval.ID)
		}
	}
	for _, evidence := range snapshot.PendingEvidence {
		if blockedEvidenceStatus(evidence.Status) {
			trace.Blockers = appendUniqueSorted(trace.Blockers, evidence.ID)
		}
		trace.EvidenceRefs = appendUniqueSorted(trace.EvidenceRefs, firstNonEmpty(evidence.ID, evidence.ToolCallID))
	}
	trace.Phase = uxPhase(snapshot, hasPlan, trace)
	return trace
}

func EvaluateInstructionReconcile(previous *TurnSnapshot, _ string, metadata map[string]string) InstructionReconcileDecision {
	revision := strings.ToLower(strings.TrimSpace(firstNonEmpty(metadata["instruction.revision"], metadata["revision"])))
	decision := InstructionReconcileDecision{Action: "continue_current"}
	plan, hasPlan := latestPlanFromAgentItems(nil)
	if previous != nil {
		plan, hasPlan = latestPlanFromAgentItems(previous.AgentItems)
	}
	switch revision {
	case "replace_goal", "supersede", "new_goal":
		decision.Action = "supersede_plan"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "goal_replaced")
		if hasPlan {
			for _, step := range plan.Steps {
				switch step.Status {
				case planning.StepStatusPending:
					decision.CancelledStepIDs = appendUniqueSorted(decision.CancelledStepIDs, step.ID)
				case planning.StepStatusInProgress:
					decision.BlockedStepIDs = appendUniqueSorted(decision.BlockedStepIDs, step.ID)
				}
			}
		}
		decision.NewStepIDs = splitCSV(metadata["instruction.newStepIds"])
	case "add_constraint", "constraint", "revise":
		decision.Action = "revise_plan"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "constraint_added")
	case "clarify":
		decision.Action = "ask_clarification"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "instruction_delta_unclear")
	default:
		if !hasPlan {
			decision.Reasons = appendUniqueSorted(decision.Reasons, "no_active_plan")
		}
	}
	return decision
}

func EvaluateResumeContinuationPolicy(snapshot *TurnSnapshot, input string) ResumeContinuationPolicy {
	policy := ResumeContinuationPolicy{Action: "continue_next_step", Reasons: []string{"resume_continue_by_default"}}
	if snapshot == nil {
		return ResumeContinuationPolicy{Action: "ask_clarification", Reasons: []string{"missing_resume_snapshot"}}
	}
	if resumeRecapRequested(input) {
		return ResumeContinuationPolicy{Action: "recap_requested", RecapAllowed: true, NextStepID: strings.TrimSpace(snapshot.Metadata["resume.nextStepId"]), Reasons: []string{"user_requested_recap"}}
	}
	nextStepID := strings.TrimSpace(snapshot.Metadata["resume.nextStepId"])
	if nextStepID == "" {
		nextStepID = nextPendingStepID(snapshot)
	}
	if nextStepID == "" {
		return ResumeContinuationPolicy{Action: "ask_clarification", Reasons: []string{"missing_next_step"}}
	}
	policy.NextStepID = nextStepID
	return policy
}

func EvaluateEvidenceCoverageGate(snapshot *TurnSnapshot) EvidenceCoverageDecision {
	required := requiredCoverageDimensions(snapshot)
	covered := coveredCoverageDimensions(snapshot)
	missing := missingCoverageDimensions(required, covered)
	verificationStatus := snapshotMetadata(snapshot, "coverage.verificationStatus", "verificationStatus")
	openQuestions := splitCSV(snapshotMetadata(snapshot, "coverage.openQuestions", "openQuestions"))
	decision := EvidenceCoverageDecision{
		Action:             "synthesis_allowed",
		Coverage:           coverageRatio(len(required), len(missing)),
		RequiredDimensions: required,
		CoveredDimensions:  covered,
		MissingDimensions:  missing,
		OpenQuestions:      openQuestions,
		VerificationStatus: verificationStatus,
	}
	if len(missing) > 0 || len(openQuestions) > 0 {
		decision.Action = "continue_gathering"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "missing_coverage_dimension")
	}
	if snapshotMetadata(snapshot, "coverage.blocker", "blocker") != "" && snapshotMetadata(snapshot, "coverage.nextAction", "nextAction") != "" {
		decision.Action = "blocker_final_allowed"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "blocker_with_next_action")
	}
	if decision.Action == "synthesis_allowed" {
		decision.Reasons = appendUniqueSorted(decision.Reasons, "coverage_complete")
	}
	return decision
}

func EvaluateManagerSynthesisGate(snapshot *TurnSnapshot, answer string) ManagerSynthesisGate {
	workerRefs := splitCSV(snapshotMetadata(snapshot, "managerSynthesis.workerOutputRefs", "workerOutputRefs"))
	managerRef := snapshotMetadata(snapshot, "managerSynthesis.managerAnswerRef", "managerAnswerRef")
	gate := ManagerSynthesisGate{
		Action:           "allow_final",
		ManagerAnswerRef: managerRef,
		WorkerOutputRefs: workerRefs,
	}
	if len(workerRefs) == 0 {
		return gate
	}
	answerLower := strings.ToLower(answer)
	for _, ref := range workerRefs {
		if ref != "" && strings.Contains(answerLower, strings.ToLower(ref)) {
			gate.Action = "block_worker_dump"
			gate.Reasons = appendUniqueSorted(gate.Reasons, "worker_output_dump_detected")
			return gate
		}
	}
	if managerRef != "" && !strings.Contains(answerLower, strings.ToLower(managerRef)) {
		gate.Action = "require_manager_synthesis"
		gate.Reasons = appendUniqueSorted(gate.Reasons, "final_answer_must_use_manager_synthesis")
	}
	return gate
}

func EvaluateCompletionReadiness(snapshot *TurnSnapshot, answer string) CompletionReadinessDecision {
	coverage := EvaluateEvidenceCoverageGate(snapshot)
	decision := CompletionReadinessDecision{
		Action:             "allow_success_final",
		MissingDimensions:  coverage.MissingDimensions,
		OpenQuestions:      coverage.OpenQuestions,
		VerificationStatus: coverage.VerificationStatus,
	}
	if coverage.Action == "blocker_final_allowed" && finalLooksLikeBlocker(answer) {
		decision.Action = "allow_blocker_final"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "explicit_blocker_final")
		return decision
	}
	if coverage.Action == "continue_gathering" || coverage.Action == "block_success_final" {
		decision.Action = "block_success_final"
		decision.Reasons = appendUniqueSorted(decision.Reasons, coverage.Reasons...)
		if len(coverage.MissingDimensions) > 0 {
			decision.Reasons = appendUniqueSorted(decision.Reasons, "missing_coverage_dimension")
		}
		return decision
	}
	if len(snapshotPendingApprovals(snapshot)) > 0 && !finalLooksLikeBlocker(answer) {
		decision.Action = "block_success_final"
		decision.Reasons = appendUniqueSorted(decision.Reasons, "pending_approval")
	}
	return decision
}

func BuildFailureSignature(toolName string, args json.RawMessage, result ToolResult) string {
	normalized := map[string]any{
		"tool":  strings.TrimSpace(toolName),
		"error": normalizeFailureText(firstNonEmpty(result.Error, result.Summary, result.Content)),
	}
	if scope := normalizedFailureScope(args); len(scope) > 0 {
		normalized["scope"] = scope
	}
	data, _ := json.Marshal(normalized)
	sum := sha256.Sum256(data)
	return "failure:" + hex.EncodeToString(sum[:12])
}

func EvaluateFailureSignatureDecision(signature string, seenCount int) FailureSignatureDecision {
	decision := FailureSignatureDecision{
		Signature:  strings.TrimSpace(signature),
		SeenCount:  seenCount,
		Action:     "retry_same_path",
		Reasons:    []string{"below_repeat_threshold"},
	}
	if seenCount >= 3 {
		decision.Action = "switch_path"
		decision.SwitchPathReason = "same normalized failure repeated; use an independent method or ask for the smallest missing input"
		decision.Reasons = []string{"repeat_threshold_reached"}
	}
	if decision.Signature == "" {
		decision.Action = "ask_user"
		decision.Reasons = []string{"missing_failure_signature"}
	}
	return decision
}

func resumeContinuationPrompt(policy ResumeContinuationPolicy) string {
	parts := []string{"## Resume continuation policy", "Resume action: " + strings.TrimSpace(policy.Action)}
	if policy.NextStepID != "" {
		parts = append(parts, "Next step id: "+policy.NextStepID)
	}
	parts = append(parts, fmt.Sprintf("Recap allowed: %t", policy.RecapAllowed))
	if len(policy.Reasons) > 0 {
		parts = append(parts, "Reasons: "+strings.Join(policy.Reasons, ", "))
	}
	parts = append(parts, "Continue the next executable step by default. Do not recap prior work unless the user explicitly requested a summary.")
	return strings.Join(parts, "\n")
}

func managerSynthesisRetryPrompt(gate ManagerSynthesisGate) string {
	parts := []string{"## Manager synthesis gate", "Gate action: " + strings.TrimSpace(gate.Action)}
	if gate.ManagerAnswerRef != "" {
		parts = append(parts, "Required manager answer ref: "+gate.ManagerAnswerRef)
	}
	if len(gate.WorkerOutputRefs) > 0 {
		parts = append(parts, "Worker output refs: "+strings.Join(gate.WorkerOutputRefs, ", "))
	}
	if len(gate.Reasons) > 0 {
		parts = append(parts, "Reasons: "+strings.Join(gate.Reasons, ", "))
	}
	parts = append(parts, "Produce a consolidated manager synthesis final answer. Do not dump raw worker outputs; cite refs compactly when needed.")
	return strings.Join(parts, "\n")
}

func completionReadinessRetryPrompt(decision CompletionReadinessDecision) string {
	parts := []string{"## Completion readiness gate", "Gate action: " + strings.TrimSpace(decision.Action)}
	if len(decision.MissingDimensions) > 0 {
		parts = append(parts, "Missing dimensions: "+strings.Join(decision.MissingDimensions, ", "))
	}
	if len(decision.OpenQuestions) > 0 {
		parts = append(parts, "Open questions: "+strings.Join(decision.OpenQuestions, ", "))
	}
	if decision.VerificationStatus != "" {
		parts = append(parts, "Verification status: "+decision.VerificationStatus)
	}
	if len(decision.Reasons) > 0 {
		parts = append(parts, "Reasons: "+strings.Join(decision.Reasons, ", "))
	}
	parts = append(parts, "Continue gathering missing evidence, request approval/input, or produce an explicit blocker final. Do not claim success while required coverage is missing.")
	return strings.Join(parts, "\n")
}

func coverageGateMetadataPresent(snapshot *TurnSnapshot) bool {
	if snapshot == nil || snapshot.Metadata == nil {
		return false
	}
	for _, key := range []string{
		"coverage.coveredDimensions",
		"coverage.requiredDimensions",
		"coverage.blocker",
		"coverage.nextAction",
		"coverage.openQuestions",
	} {
		if strings.TrimSpace(snapshot.Metadata[key]) != "" {
			return true
		}
	}
	return false
}

func latestPlanFromAgentItems(items []agentstate.TurnItem) (planning.PlanState, bool) {
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if item.Type != agentstate.TurnItemTypePlan || len(item.Payload.Data) == 0 {
			continue
		}
		var plan planning.PlanState
		if err := json.Unmarshal(item.Payload.Data, &plan); err == nil {
			return plan, true
		}
	}
	return planning.PlanState{}, false
}

func uxPhase(snapshot *TurnSnapshot, hasPlan bool, trace UXProgressTrace) string {
	if len(trace.Blockers) > 0 {
		return "blocked"
	}
	if len(trace.PendingApprovals) > 0 {
		return "waiting_approval"
	}
	if snapshot != nil && snapshot.ResumeState == TurnResumeStatePendingEvidence {
		return "waiting_input"
	}
	if snapshotMetadata(snapshot, "synthesis.status", "managerSynthesis.status") == "pending" {
		return "synthesizing"
	}
	if trace.CurrentStepID != "" {
		return "executing"
	}
	if hasPlan {
		return "planning"
	}
	if snapshot != nil && snapshot.Lifecycle.IsTerminal() {
		return "completed"
	}
	return "executing"
}

func pendingStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "" || status == "pending" || status == "waiting"
}

func blockedEvidenceStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "blocked", "failed", "unavailable":
		return true
	default:
		return false
	}
}

func resumeRecapRequested(input string) bool {
	lower := strings.ToLower(input)
	for _, token := range []string{"summary", "summarize", "recap", "总结", "概括", "回顾"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func nextPendingStepID(snapshot *TurnSnapshot) string {
	plan, ok := latestPlanFromAgentItems(snapshot.AgentItems)
	if !ok {
		return ""
	}
	for _, step := range plan.Steps {
		if step.Status == planning.StepStatusPending || step.Status == planning.StepStatusInProgress {
			return strings.TrimSpace(step.ID)
		}
	}
	return ""
}

func requiredCoverageDimensions(snapshot *TurnSnapshot) []string {
	required := []string{"plan_context"}
	if snapshot == nil {
		return required
	}
	profile := snapshot.TaskDepth
	if profile.RequiresEvidence || taskdepth.AtLeast(profile.Level, taskdepth.LevelInvestigation) {
		required = append(required, "tool_evidence", "risk_review", "open_questions_resolved")
	}
	if profile.RequiresValidation || taskdepth.AtLeast(profile.Level, taskdepth.LevelOperations) {
		required = append(required, "verification")
	}
	if override := snapshotMetadata(snapshot, "coverage.requiredDimensions", "requiredDimensions"); override != "" {
		required = splitCSV(override)
	}
	return uniqueSorted(required)
}

func coveredCoverageDimensions(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	return uniqueSorted(splitCSV(snapshotMetadata(snapshot, "coverage.coveredDimensions", "coveredDimensions")))
}

func missingCoverageDimensions(required, covered []string) []string {
	coveredSet := map[string]bool{}
	for _, value := range covered {
		coveredSet[value] = true
	}
	var missing []string
	for _, value := range required {
		if !coveredSet[value] {
			missing = append(missing, value)
		}
	}
	return missing
}

func coverageRatio(required, missing int) float64 {
	if required == 0 {
		return 1
	}
	return float64(required-missing) / float64(required)
}

func snapshotMetadata(snapshot *TurnSnapshot, keys ...string) string {
	if snapshot == nil || snapshot.Metadata == nil {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(snapshot.Metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func snapshotPendingApprovals(snapshot *TurnSnapshot) []PendingApproval {
	if snapshot == nil {
		return nil
	}
	var out []PendingApproval
	for _, approval := range snapshot.PendingApprovals {
		if pendingStatus(approval.Status) {
			out = append(out, approval)
		}
	}
	return out
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return uniqueSorted(out)
}

func uniqueSorted(values []string) []string {
	var out []string
	for _, value := range values {
		out = appendUniqueSorted(out, value)
	}
	return out
}

func appendUniqueSorted(values []string, next ...string) []string {
	for _, value := range next {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

var failureVolatilePattern = regexp.MustCompile(`(?i)(request[-_ ]?[a-z0-9-]+|[0-9]+(?:\.[0-9]+)?\s*(ms|s|sec|seconds|minute|minutes)|\b\d{4,}\b)`)

func normalizeFailureText(value string) string {
	value = failureVolatilePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "<volatile>")
	fields := strings.Fields(value)
	if len(fields) > 12 {
		fields = fields[:12]
	}
	return strings.Join(fields, " ")
}

func normalizedFailureScope(args json.RawMessage) map[string]string {
	if len(args) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil
	}
	scope := map[string]string{}
	for _, key := range []string{"resourceType", "resource_type", "scopeType", "scope_type"} {
		if value := runtimeAnyString(payload[key]); value != "" {
			scope["type"] = value
			break
		}
	}
	for _, key := range []string{"resourceId", "resource_id", "scopeId", "scope_id"} {
		if value := runtimeAnyString(payload[key]); value != "" {
			scope["id"] = value
			break
		}
	}
	if len(scope) == 0 {
		return nil
	}
	return scope
}

func runtimeAnyString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
