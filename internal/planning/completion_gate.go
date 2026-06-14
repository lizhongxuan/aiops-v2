package planning

const (
	CompletionGateAllow               = "allow"
	CompletionGateDenySuccessFinal    = "deny_success_final"
	CompletionGateRequireBlockerFinal = "require_blocker_final"
)

type CompletionGateContext struct {
	ApprovedRefs        []string `json:"approvedRefs,omitempty"`
	PendingEvidenceRefs []string `json:"pendingEvidenceRefs,omitempty"`
	FailedToolRefs      []string `json:"failedToolRefs,omitempty"`
}

type CompletionGateDecision struct {
	Action  string   `json:"action"`
	Reasons []string `json:"reasons,omitempty"`
}

func EvaluateCompletionGate(plan PlanState, context CompletionGateContext) CompletionGateDecision {
	decision := CompletionGateDecision{Action: CompletionGateAllow}
	if len(context.PendingEvidenceRefs) > 0 {
		decision.add(CompletionGateDenySuccessFinal, "pending_evidence")
	}
	if len(context.FailedToolRefs) > 0 {
		decision.add(CompletionGateDenySuccessFinal, "failed_tool")
	}
	approved := stringSet(context.ApprovedRefs)
	for _, step := range plan.Steps {
		switch step.Status {
		case StepStatusPending, StepStatusInProgress:
			decision.add(CompletionGateDenySuccessFinal, "pending_step")
		case StepStatusBlocked:
			decision.add(CompletionGateRequireBlockerFinal, "blocked_step")
		case StepStatusFailed:
			decision.add(CompletionGateRequireBlockerFinal, "failed_step")
		}
		for _, approval := range step.RequiredApprovals {
			if !approved[approval] {
				decision.add(CompletionGateDenySuccessFinal, "missing_required_approval")
			}
		}
		switch step.VerificationStatus {
		case "skipped":
			decision.add(CompletionGateRequireBlockerFinal, "verification_skipped")
		case "failed":
			decision.add(CompletionGateDenySuccessFinal, "verification_failed")
		}
	}
	return decision
}

func (d *CompletionGateDecision) add(action, reason string) {
	if actionRank(action) > actionRank(d.Action) {
		d.Action = action
	}
	if !containsPlanningString(d.Reasons, reason) {
		d.Reasons = append(d.Reasons, reason)
	}
}

func actionRank(action string) int {
	switch action {
	case CompletionGateDenySuccessFinal:
		return 2
	case CompletionGateRequireBlockerFinal:
		return 1
	default:
		return 0
	}
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range trimStringSlice(values) {
		out[value] = true
	}
	return out
}

func containsPlanningString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
