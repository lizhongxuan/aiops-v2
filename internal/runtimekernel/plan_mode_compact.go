package runtimekernel

import "strings"

type PlanModeCompactRecoveryInput struct {
	State               string   `json:"state,omitempty"`
	PlanID              string   `json:"planId,omitempty"`
	ApprovalID          string   `json:"approvalId,omitempty"`
	PendingQuestions    []string `json:"pendingQuestions,omitempty"`
	LastRejectionReason string   `json:"lastRejectionReason,omitempty"`
}

func RecoverPlanModeFromCompactSummary(session *SessionState, compact PlanModeCompactRecoveryInput, recoveryVersion string) PlanModeState {
	if session == nil {
		return PlanModeState{}
	}
	current := session.PlanMode.Normalize()
	if strings.TrimSpace(compact.State) != "" {
		current.State = PlanModeLifecycleState(strings.TrimSpace(compact.State))
	}
	if strings.TrimSpace(compact.PlanID) != "" {
		current.PlanID = strings.TrimSpace(compact.PlanID)
	}
	if strings.TrimSpace(compact.ApprovalID) != "" {
		current.ApprovalID = strings.TrimSpace(compact.ApprovalID)
	}
	current.PendingQuestions = append([]string(nil), compact.PendingQuestions...)
	if strings.TrimSpace(compact.LastRejectionReason) != "" {
		current.LastRejectionReason = strings.TrimSpace(compact.LastRejectionReason)
	}
	current.CompactRecovery = strings.TrimSpace(recoveryVersion)
	if current.State == PlanModeStateActive {
		current.ReminderLevel = PlanModeReminderResume
		current.AllowDraftPlan = current.PlanID == ""
	}
	session.PlanMode = current.Normalize()
	return session.PlanMode
}
