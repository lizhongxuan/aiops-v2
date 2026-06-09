package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/promptinput"
)

const planCompletionGateRetryMetadataKey = "planCompletionGate.retry"

func latestPlanStateFromSnapshot(snapshot *TurnSnapshot) (planning.PlanState, bool) {
	if snapshot == nil {
		return planning.PlanState{}, false
	}
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		if item.Type != agentstate.TurnItemTypePlan {
			continue
		}
		var plan planning.PlanState
		if err := json.Unmarshal(item.Payload.Data, &plan); err != nil || len(plan.Steps) == 0 {
			continue
		}
		return plan, true
	}
	return planning.PlanState{}, false
}

func evaluateRuntimePlanCompletionGate(session *SessionState, snapshot *TurnSnapshot) (planning.CompletionGateDecision, bool) {
	plan, ok := latestPlanStateFromSnapshot(snapshot)
	if !ok {
		return planning.CompletionGateDecision{Action: planning.CompletionGateAllow}, false
	}
	return planning.EvaluateCompletionGate(plan, completionGateContextFromRuntime(session, snapshot)), true
}

func completionGateContextFromRuntime(session *SessionState, snapshot *TurnSnapshot) planning.CompletionGateContext {
	var ctx planning.CompletionGateContext
	if session != nil {
		for _, approval := range session.PendingApprovals {
			if approvalApproved(approval) {
				ctx.ApprovedRefs = append(ctx.ApprovedRefs, approval.ID)
			}
		}
		for _, grant := range session.ApprovalGrants {
			ctx.ApprovedRefs = append(ctx.ApprovedRefs, grant.ID, grant.InputHash)
		}
		for _, evidence := range session.PendingEvidence {
			if strings.TrimSpace(evidence.Status) == "" || strings.EqualFold(evidence.Status, "pending") {
				ctx.PendingEvidenceRefs = append(ctx.PendingEvidenceRefs, evidence.ID)
			}
		}
	}
	finalEvidence := BuildFinalEvidenceState(snapshot, session)
	for _, failed := range finalEvidence.FailedTools {
		ctx.FailedToolRefs = append(ctx.FailedToolRefs, firstNonBlankRuntimeString(failed.ToolCallID, failed.ToolName, failed.FailureClass))
	}
	return ctx
}

func approvalApproved(approval PendingApproval) bool {
	for _, value := range []string{approval.Status, approval.Decision} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "approved", "allow", "allowed":
			return true
		}
	}
	return false
}

func planCompletionGateTrace(decision planning.CompletionGateDecision, present bool) *promptinput.PlanCompletionGateTrace {
	if !present && decision.Action == planning.CompletionGateAllow {
		return nil
	}
	return &promptinput.PlanCompletionGateTrace{
		Decision: decision.Action,
		Reasons:  append([]string(nil), decision.Reasons...),
	}
}

func planCompletionGateAllowsFinal(answer string, decision planning.CompletionGateDecision) bool {
	if decision.Action == planning.CompletionGateAllow {
		return true
	}
	return finalLooksLikeBlocker(answer)
}

func planCompletionGateRetryPrompt(decision planning.CompletionGateDecision) string {
	return fmt.Sprintf("## Plan completion gate\nThe current plan is not ready for a success final answer. Gate decision: %s. Reasons: %s. Continue the task, update the plan, gather missing evidence/approval/verification, or state the blocker explicitly instead of claiming completion.",
		decision.Action,
		strings.Join(decision.Reasons, ", "),
	)
}
