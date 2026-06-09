package runtimekernel

import (
	"fmt"
	"strings"
	"time"
)

type PlanArtifactStatus string

const (
	PlanArtifactDraft           PlanArtifactStatus = "draft"
	PlanArtifactPendingApproval PlanArtifactStatus = "pending_approval"
	PlanArtifactApproved        PlanArtifactStatus = "approved"
	PlanArtifactRejected        PlanArtifactStatus = "rejected"
)

type RuntimePlanArtifact struct {
	ID             string               `json:"id"`
	Type           PlanModeExpectedType `json:"type,omitempty"`
	Status         PlanArtifactStatus   `json:"status,omitempty"`
	Objective      string               `json:"objective,omitempty"`
	Steps          []RuntimePlanStep    `json:"steps,omitempty"`
	OpenQuestions  []string             `json:"openQuestions,omitempty"`
	ApprovalScope  *PlanApprovalScope   `json:"approvalScope,omitempty"`
	Rejections     []PlanRejection      `json:"rejections,omitempty"`
	ApprovedAt     *time.Time           `json:"approvedAt,omitempty"`
	LastModifiedAt time.Time            `json:"lastModifiedAt,omitempty"`
	Metadata       map[string]string    `json:"metadata,omitempty"`
}

type RuntimePlanStep struct {
	ID      string `json:"id,omitempty"`
	Text    string `json:"text"`
	Status  string `json:"status,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type PlanRejection struct {
	ApprovalID string    `json:"approvalId,omitempty"`
	Reason     string    `json:"reason"`
	RejectedAt time.Time `json:"rejectedAt"`
}

type ExitPlanModeResult struct {
	Allowed         bool             `json:"allowed"`
	Status          string           `json:"status"`
	MissingSections []string         `json:"missingSections,omitempty"`
	Reason          string           `json:"reason,omitempty"`
	ApprovalID      string           `json:"approvalId,omitempty"`
	PlanMode        PlanModeState    `json:"planMode"`
	PendingApproval *PendingApproval `json:"pendingApproval,omitempty"`
}

func RequestExitPlanMode(session *SessionState, turnID string, artifact RuntimePlanArtifact, now time.Time) (ExitPlanModeResult, RuntimePlanArtifact, error) {
	if session == nil {
		return ExitPlanModeResult{}, artifact, fmt.Errorf("session is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	state := session.PlanMode.Normalize()
	if state.State != PlanModeStateActive {
		return ExitPlanModeResult{Allowed: false, Status: "denied", Reason: "exit_plan_mode requires active plan mode", PlanMode: state}, artifact, nil
	}
	missing := missingPlanArtifactSections(artifact)
	if len(missing) > 0 {
		return ExitPlanModeResult{Allowed: false, Status: "missing_sections", MissingSections: missing, PlanMode: state}, artifact, nil
	}
	openQuestions := normalizedPlanQuestions(artifact.OpenQuestions)
	if len(openQuestions) > 0 {
		state.PendingQuestions = openQuestions
		session.PlanMode = state
		return ExitPlanModeResult{Allowed: false, Status: "open_questions_remaining", Reason: "plan has open questions", PlanMode: state}, artifact, nil
	}
	approvalID := fmt.Sprintf("plan-exit-%d", now.UnixNano())
	if turnID == "" && session.CurrentTurn != nil {
		turnID = session.CurrentTurn.ID
	}
	if turnID == "" {
		turnID = fmt.Sprintf("turn-plan-exit-%d", now.UnixNano())
	}
	artifact.Status = PlanArtifactPendingApproval
	artifact.LastModifiedAt = now
	approval := PendingApproval{
		ID:             approvalID,
		SessionID:      session.ID,
		TurnID:         turnID,
		ToolName:       "exit_plan_mode",
		Reason:         firstNonEmpty(strings.TrimSpace(artifact.Objective), "approve plan before execution"),
		Risk:           firstNonEmpty(planScopeRisk(artifact.ApprovalScope), "medium"),
		Source:         PlanExitApprovalSource,
		Status:         "pending",
		ExpectedEffect: "Approve this plan and unlock execution only within the approved scope.",
		Rollback:       "Reject to stay in plan mode and revise the plan.",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	state.State = PlanModeStatePendingExitApproval
	state.PlanID = artifact.ID
	state.ApprovalID = approvalID
	state.PendingQuestions = nil
	state.ReminderLevel = PlanModeReminderSparse
	session.PlanMode = state
	session.PendingApprovals = upsertPendingApproval(session.PendingApprovals, approval)
	if session.CurrentTurn != nil && session.CurrentTurn.ID == turnID {
		session.CurrentTurn.PendingApprovals = upsertPendingApproval(session.CurrentTurn.PendingApprovals, approval)
	}
	return ExitPlanModeResult{
		Allowed:         true,
		Status:          string(PlanModeStatePendingExitApproval),
		ApprovalID:      approvalID,
		PlanMode:        state,
		PendingApproval: &approval,
	}, artifact, nil
}

func ApplyPlanApprovalDecision(session *SessionState, artifact RuntimePlanArtifact, approvalID, decision, reason string, now time.Time) (RuntimePlanArtifact, PlanModeState, error) {
	if session == nil {
		return artifact, PlanModeState{}, fmt.Errorf("session is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	state := session.PlanMode.Normalize()
	if state.State != PlanModeStatePendingExitApproval {
		return artifact, state, fmt.Errorf("plan exit approval is not pending")
	}
	if approvalID != "" && state.ApprovalID != "" && approvalID != state.ApprovalID {
		return artifact, state, fmt.Errorf("approval id %q does not match plan exit approval %q", approvalID, state.ApprovalID)
	}
	session.PendingApprovals = removePendingApproval(session.PendingApprovals, state.ApprovalID)
	if session.CurrentTurn != nil {
		session.CurrentTurn.PendingApprovals = removePendingApproval(session.CurrentTurn.PendingApprovals, state.ApprovalID)
	}
	artifact.LastModifiedAt = now
	if isApprovedResumeDecision(decision) {
		artifact.Status = PlanArtifactApproved
		artifact.ApprovedAt = &now
		state.State = PlanModeStateApproved
		state.ApprovedPlanID = firstNonEmpty(artifact.ID, state.PlanID)
		state.PlanID = state.ApprovedPlanID
		state.ApprovalID = ""
		state.LastRejectionReason = ""
		state.ReminderLevel = ""
		session.Mode = ModeExecute
		if artifact.ApprovalScope != nil {
			scope := *artifact.ApprovalScope
			scope.PlanID = firstNonEmpty(scope.PlanID, state.ApprovedPlanID)
			session.PlanApprovalScopes = upsertPlanApprovalScope(session.PlanApprovalScopes, scope)
		}
		session.PlanMode = state
		return artifact, state, nil
	}
	rejection := strings.TrimSpace(firstNonEmpty(reason, "plan rejected"))
	artifact.Status = PlanArtifactRejected
	artifact.Rejections = append(artifact.Rejections, PlanRejection{ApprovalID: state.ApprovalID, Reason: rejection, RejectedAt: now})
	state.State = PlanModeStateActive
	state.ApprovalID = ""
	state.LastRejectionReason = rejection
	state.ReminderLevel = PlanModeReminderResume
	state.AllowDraftPlan = true
	session.Mode = ModePlan
	session.PlanMode = state
	return artifact, state, nil
}

func missingPlanArtifactSections(artifact RuntimePlanArtifact) []string {
	var missing []string
	if strings.TrimSpace(artifact.ID) == "" {
		missing = append(missing, "id")
	}
	if strings.TrimSpace(artifact.Objective) == "" {
		missing = append(missing, "objective")
	}
	if len(artifact.Steps) == 0 {
		missing = append(missing, "steps")
	} else {
		for i, step := range artifact.Steps {
			if strings.TrimSpace(step.Text) == "" {
				missing = append(missing, fmt.Sprintf("steps[%d].text", i))
			}
		}
	}
	return missing
}

func normalizedPlanQuestions(questions []string) []string {
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		if trimmed := strings.TrimSpace(question); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func planScopeRisk(scope *PlanApprovalScope) string {
	if scope == nil {
		return ""
	}
	return strings.TrimSpace(scope.RiskCeiling)
}

func upsertPlanApprovalScope(items []PlanApprovalScope, next PlanApprovalScope) []PlanApprovalScope {
	out := make([]PlanApprovalScope, 0, len(items)+1)
	replaced := false
	for _, item := range items {
		if item.PlanID == next.PlanID && item.ApprovalID == next.ApprovalID {
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
