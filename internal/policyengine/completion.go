package policyengine

import "context"

// DefaultCompletionEvaluator implements CompletionEvaluator by checking that
// all pending approvals are resolved and all required evidence is collected
// before allowing a turn to finalize.
type DefaultCompletionEvaluator struct{}

// CheckCompletion evaluates whether the turn can be finalized given the
// current turn state.
//
// Decision logic:
//  1. If there are pending approvals → Deny with reason "pending approvals"
//  2. If there is pending evidence → NeedEvidence with reason "pending evidence"
//  3. Otherwise → Allow
func (e *DefaultCompletionEvaluator) CheckCompletion(_ context.Context, turnState TurnState) PolicyDecision {
	if len(turnState.PendingApprovals) > 0 {
		return PolicyDecision{
			Action: PolicyActionDeny,
			Reason: "pending approvals",
		}
	}

	if len(turnState.PendingEvidence) > 0 {
		return PolicyDecision{
			Action: PolicyActionNeedEvidence,
			Reason: "pending evidence",
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}
