package agentmgr

import (
	"fmt"
	"strings"
	"time"
)

type AgentContinuationAction string

const (
	AgentContinuationContinue AgentContinuationAction = "continue_existing"
	AgentContinuationFresh    AgentContinuationAction = "spawn_fresh"
	AgentContinuationStop     AgentContinuationAction = "stop"
)

type AgentContinuationDecision struct {
	Action          AgentContinuationAction `json:"action"`
	Reason          string                  `json:"reason"`
	AgentID         string                  `json:"agentId,omitempty"`
	ExistingAgentID string                  `json:"existingAgentId,omitempty"`
	FailureSummary  string                  `json:"failureSummary,omitempty"`
}

type ContinuationInput struct {
	Target               ExistingAgentContext
	RequiredCapability   string
	RequiredResourceType string
	SameProblem          bool
	HasRecoverableState  bool
}

func EvaluateAgentContinuation(in ContinuationInput) AgentContinuationDecision {
	if in.Target.AgentID == "" {
		return AgentContinuationDecision{Action: AgentContinuationFresh, Reason: "no_existing_agent"}
	}
	if !in.SameProblem || !matchesCapabilityResource(in.Target.CapabilityKinds, in.Target.ResourceTypes, in.RequiredCapability, in.RequiredResourceType) {
		return AgentContinuationDecision{Action: AgentContinuationFresh, Reason: "different_problem_or_scope"}
	}
	if in.Target.Status == AgentStatusCompleted || (in.Target.Status == AgentStatusFailed && in.HasRecoverableState) || in.Target.Status == AgentStatusWaiting {
		return AgentContinuationDecision{Action: AgentContinuationContinue, Reason: "same_problem_recoverable_context", AgentID: in.Target.AgentID, ExistingAgentID: in.Target.AgentID}
	}
	return AgentContinuationDecision{Action: AgentContinuationFresh, Reason: "existing_agent_not_continuable"}
}

type AgentStopReason string

const (
	AgentStopCompleted AgentStopReason = "completed"
	AgentStopBlocked   AgentStopReason = "blocked"
	AgentStopFailed    AgentStopReason = "failed"
	AgentStopKilled    AgentStopReason = "killed"
	AgentStopBudget    AgentStopReason = "budget_exhausted"
)

type AgentStopRecord struct {
	AgentID      string          `json:"agentId"`
	Reason       AgentStopReason `json:"reason"`
	Message      string          `json:"message,omitempty"`
	EvidenceRefs []string        `json:"evidenceRefs,omitempty"`
	StoppedAt    time.Time       `json:"stoppedAt,omitempty"`
}

func (r AgentStopRecord) Validate() error {
	if strings.TrimSpace(r.AgentID) == "" {
		return fmt.Errorf("agent id is required")
	}
	switch r.Reason {
	case AgentStopCompleted, AgentStopBlocked, AgentStopFailed, AgentStopKilled, AgentStopBudget:
	default:
		return fmt.Errorf("valid stop reason is required")
	}
	if r.Reason == AgentStopCompleted && len(r.EvidenceRefs) == 0 {
		return fmt.Errorf("completed stop record requires evidence refs")
	}
	return nil
}
