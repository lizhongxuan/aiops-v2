package runtimekernel

import (
	"fmt"
	"strings"
	"time"
)

const (
	PlanModeEntryApprovalSource = "plan_mode_entry"
	PlanExitApprovalSource      = "plan_exit_approval"
)

type EnterPlanModeRequest struct {
	Reason           string               `json:"reason"`
	ExpectedPlanType PlanModeExpectedType `json:"expectedPlanType,omitempty"`
}

type PlanModeToolResult struct {
	Allowed         bool          `json:"allowed"`
	Status          string        `json:"status"`
	Reason          string        `json:"reason,omitempty"`
	ApprovalID      string        `json:"approvalId,omitempty"`
	PlanMode        PlanModeState `json:"planMode"`
	PendingApproval *PendingApproval
}

func RequestEnterPlanMode(session *SessionState, turnID string, req EnterPlanModeRequest, now time.Time) (PlanModeToolResult, error) {
	if session == nil {
		return PlanModeToolResult{}, fmt.Errorf("session is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "plan mode requested"
	}
	expected := req.ExpectedPlanType
	if expected == "" {
		expected = PlanModeExpectedGeneric
	}
	if session.Mode == ModeExecute {
		session.PlanMode = PlanModeState{
			State:               PlanModeStateInactive,
			ExpectedPlanType:    expected,
			RequestedReason:     reason,
			LastRejectionReason: "enter_plan_mode is not available in execute mode",
		}
		return PlanModeToolResult{Allowed: false, Status: "denied", Reason: session.PlanMode.LastRejectionReason, PlanMode: session.PlanMode}, nil
	}
	current := session.PlanMode.Normalize()
	if current.State == PlanModeStateActive || current.State == PlanModeStatePendingExitApproval || current.State == PlanModeStateApproved {
		return PlanModeToolResult{Allowed: true, Status: string(current.State), ApprovalID: current.ApprovalID, PlanMode: current}, nil
	}
	approvalID := fmt.Sprintf("plan-entry-%d", now.UnixNano())
	if turnID == "" && session.CurrentTurn != nil {
		turnID = session.CurrentTurn.ID
	}
	if turnID == "" {
		turnID = fmt.Sprintf("turn-plan-entry-%d", now.UnixNano())
	}
	approval := PendingApproval{
		ID:        approvalID,
		SessionID: session.ID,
		TurnID:    turnID,
		ToolName:  "enter_plan_mode",
		Reason:    reason,
		Risk:      "low",
		Source:    PlanModeEntryApprovalSource,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.PlanMode = PlanModeState{
		State:            PlanModeStateRequested,
		ExpectedPlanType: expected,
		RequestedReason:  reason,
		ApprovalID:       approvalID,
		PreviousMode:     session.Mode,
		ReminderLevel:    PlanModeReminderFull,
		AllowDraftPlan:   true,
	}
	session.PendingApprovals = upsertPendingApproval(session.PendingApprovals, approval)
	if session.CurrentTurn != nil && session.CurrentTurn.ID == turnID {
		session.CurrentTurn.PendingApprovals = upsertPendingApproval(session.CurrentTurn.PendingApprovals, approval)
	}
	return PlanModeToolResult{
		Allowed:         true,
		Status:          string(PlanModeStateRequested),
		ApprovalID:      approvalID,
		PlanMode:        session.PlanMode,
		PendingApproval: &approval,
	}, nil
}

func ApplyPlanModeEntryDecision(session *SessionState, approvalID, decision, reason string, now time.Time) (PlanModeState, error) {
	if session == nil {
		return PlanModeState{}, fmt.Errorf("session is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	state := session.PlanMode.Normalize()
	if state.State != PlanModeStateRequested {
		return state, fmt.Errorf("plan mode entry is not requested")
	}
	if approvalID != "" && state.ApprovalID != "" && approvalID != state.ApprovalID {
		return state, fmt.Errorf("approval id %q does not match plan mode approval %q", approvalID, state.ApprovalID)
	}
	session.PendingApprovals = removePendingApproval(session.PendingApprovals, state.ApprovalID)
	if session.CurrentTurn != nil {
		session.CurrentTurn.PendingApprovals = removePendingApproval(session.CurrentTurn.PendingApprovals, state.ApprovalID)
	}
	if isApprovedResumeDecision(decision) {
		state.State = PlanModeStateActive
		state.ApprovalID = ""
		state.LastRejectionReason = ""
		state.ReminderLevel = PlanModeReminderFull
		state.AllowDraftPlan = true
		if state.PreviousMode == "" {
			state.PreviousMode = session.Mode
		}
		session.Mode = ModePlan
		session.PlanMode = state
		return state, nil
	}
	state.State = PlanModeStateInactive
	state.ApprovalID = ""
	state.LastRejectionReason = strings.TrimSpace(firstNonEmpty(reason, "plan mode entry rejected"))
	state.ReminderLevel = ""
	if state.PreviousMode.IsValid() {
		session.Mode = state.PreviousMode
	}
	session.PlanMode = state
	return state, nil
}

func upsertPendingApproval(items []PendingApproval, next PendingApproval) []PendingApproval {
	out := make([]PendingApproval, 0, len(items)+1)
	replaced := false
	for _, item := range items {
		if item.ID == next.ID {
			out = append(out, next)
			replaced = true
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, next)
	}
	return out
}

func removePendingApproval(items []PendingApproval, approvalID string) []PendingApproval {
	target := strings.TrimSpace(approvalID)
	if target == "" {
		return items
	}
	out := make([]PendingApproval, 0, len(items))
	for _, item := range items {
		if item.ID != target {
			out = append(out, item)
		}
	}
	return out
}
