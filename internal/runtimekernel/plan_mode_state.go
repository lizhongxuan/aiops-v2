package runtimekernel

import (
	"fmt"
	"strings"
)

type PlanModeLifecycleState string

const (
	PlanModeStateInactive            PlanModeLifecycleState = "inactive"
	PlanModeStateRequested           PlanModeLifecycleState = "requested"
	PlanModeStateActive              PlanModeLifecycleState = "active"
	PlanModeStatePendingExitApproval PlanModeLifecycleState = "pending_exit_approval"
	PlanModeStateApproved            PlanModeLifecycleState = "approved"
	PlanModeStateRejected            PlanModeLifecycleState = "rejected"
)

type PlanModeReminderLevel string

const (
	PlanModeReminderFull   PlanModeReminderLevel = "full"
	PlanModeReminderSparse PlanModeReminderLevel = "sparse"
	PlanModeReminderResume PlanModeReminderLevel = "resume"
)

type PlanModeExpectedType string

const (
	PlanModeExpectedImplementation PlanModeExpectedType = "implementation"
	PlanModeExpectedInvestigation  PlanModeExpectedType = "investigation"
	PlanModeExpectedOperations     PlanModeExpectedType = "operations"
	PlanModeExpectedResearch       PlanModeExpectedType = "research"
	PlanModeExpectedGeneric        PlanModeExpectedType = "generic"
)

type PlanModeState struct {
	State               PlanModeLifecycleState `json:"state,omitempty"`
	PlanID              string                 `json:"planId,omitempty"`
	ApprovedPlanID      string                 `json:"approvedPlanId,omitempty"`
	ExpectedPlanType    PlanModeExpectedType   `json:"expectedPlanType,omitempty"`
	RequestedReason     string                 `json:"requestedReason,omitempty"`
	ApprovalID          string                 `json:"approvalId,omitempty"`
	PendingQuestions    []string               `json:"pendingQuestions,omitempty"`
	LastRejectionReason string                 `json:"lastRejectionReason,omitempty"`
	ReminderLevel       PlanModeReminderLevel  `json:"reminderLevel,omitempty"`
	PreviousMode        Mode                   `json:"previousMode,omitempty"`
	AllowDraftPlan      bool                   `json:"allowDraftPlan,omitempty"`
	CompactRecovery     string                 `json:"compactRecoveryVersion,omitempty"`
}

func (s PlanModeState) Normalize() PlanModeState {
	if s.State == "" {
		s.State = PlanModeStateInactive
	}
	if s.ExpectedPlanType == "" {
		s.ExpectedPlanType = PlanModeExpectedGeneric
	}
	s.PlanID = strings.TrimSpace(s.PlanID)
	s.ApprovedPlanID = strings.TrimSpace(s.ApprovedPlanID)
	s.RequestedReason = strings.TrimSpace(s.RequestedReason)
	s.ApprovalID = strings.TrimSpace(s.ApprovalID)
	s.LastRejectionReason = strings.TrimSpace(s.LastRejectionReason)
	s.CompactRecovery = strings.TrimSpace(s.CompactRecovery)
	if s.PreviousMode != "" && !s.PreviousMode.IsValid() {
		s.PreviousMode = ""
	}
	questions := make([]string, 0, len(s.PendingQuestions))
	for _, question := range s.PendingQuestions {
		if trimmed := strings.TrimSpace(question); trimmed != "" {
			questions = append(questions, trimmed)
		}
	}
	s.PendingQuestions = questions
	return s
}

func (s PlanModeState) Validate() error {
	s = s.Normalize()
	switch s.State {
	case PlanModeStateInactive, PlanModeStateRequested, PlanModeStateActive, PlanModeStatePendingExitApproval, PlanModeStateApproved, PlanModeStateRejected:
	default:
		return fmt.Errorf("invalid state %q", s.State)
	}
	switch s.ExpectedPlanType {
	case "", PlanModeExpectedImplementation, PlanModeExpectedInvestigation, PlanModeExpectedOperations, PlanModeExpectedResearch, PlanModeExpectedGeneric:
	default:
		return fmt.Errorf("invalid expected plan type %q", s.ExpectedPlanType)
	}
	if s.State == PlanModeStateActive && s.PlanID == "" && !s.AllowDraftPlan {
		return fmt.Errorf("active state requires planId or allowDraftPlan")
	}
	if s.State == PlanModeStatePendingExitApproval && s.ApprovalID == "" {
		return fmt.Errorf("pending_exit_approval requires approvalId")
	}
	if s.State == PlanModeStateApproved && s.ApprovedPlanID == "" {
		return fmt.Errorf("approved state requires approvedPlanId")
	}
	switch s.ReminderLevel {
	case "", PlanModeReminderFull, PlanModeReminderSparse, PlanModeReminderResume:
	default:
		return fmt.Errorf("invalid reminder level %q", s.ReminderLevel)
	}
	for i, question := range s.PendingQuestions {
		if len([]rune(question)) > 500 {
			return fmt.Errorf("pending question[%d] exceeds 500 characters", i)
		}
	}
	return nil
}

func (s PlanModeLifecycleState) IsActiveLike() bool {
	switch s {
	case PlanModeStateActive, PlanModeStatePendingExitApproval:
		return true
	default:
		return false
	}
}
