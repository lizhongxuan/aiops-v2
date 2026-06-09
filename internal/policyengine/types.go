package policyengine

import (
	"context"
	"encoding/json"
	"time"

	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Type aliases to avoid circular imports with runtimekernel.
// These mirror the types defined in runtimekernel.
// ---------------------------------------------------------------------------

// SessionType mirrors runtimekernel.SessionType.
type SessionType = string

// Mode mirrors runtimekernel.Mode.
type Mode = string

// PolicyAction represents the decision outcome of a policy evaluation.
type PolicyAction string

const (
	PolicyActionAllow        PolicyAction = "allow"
	PolicyActionDeny         PolicyAction = "deny"
	PolicyActionNeedApproval PolicyAction = "need_approval"
	PolicyActionNeedEvidence PolicyAction = "need_evidence"
)

// IsValid reports whether the value is one of the canonical policy actions.
func (a PolicyAction) IsValid() bool {
	switch a {
	case PolicyActionAllow, PolicyActionDeny, PolicyActionNeedApproval, PolicyActionNeedEvidence:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// PolicyInput carries all context needed for a policy decision.
// ---------------------------------------------------------------------------

// PolicyInput contains the full context for a policy evaluation request.
type PolicyInput struct {
	ToolName    string               `json:"toolName"`
	Tool        tooling.ToolMetadata `json:"tool,omitempty"`
	SessionType SessionType          `json:"sessionType"`
	Mode        Mode                 `json:"mode"`
	HostID      string               `json:"hostId,omitempty"`
	Arguments   json.RawMessage      `json:"arguments,omitempty"`
	UserContext map[string]string    `json:"userContext,omitempty"`
}

// ---------------------------------------------------------------------------
// PolicyDecision is the result of a policy evaluation.
// ---------------------------------------------------------------------------

// PolicyDecision represents the outcome of a policy check.
type PolicyDecision struct {
	Action        PolicyAction     `json:"action"`
	Reason        string           `json:"reason,omitempty"`
	Approval      *ApprovalRequest `json:"approval,omitempty"` // non-nil when Action == NeedApproval
	SafetySignals []SafetySignal   `json:"safetySignals,omitempty"`
}

// ---------------------------------------------------------------------------
// ApprovalRequest describes a pending approval when policy requires it.
// ---------------------------------------------------------------------------

// ApprovalRequest holds the details of an approval that must be granted
// before a tool call can proceed.
type ApprovalRequest struct {
	ID            string         `json:"id"`
	ToolName      string         `json:"toolName"`
	Command       string         `json:"command,omitempty"`
	HostID        string         `json:"hostId,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	TTL           time.Duration  `json:"ttl,omitempty"`
	SafetySignals []SafetySignal `json:"safetySignals,omitempty"`
}

// ---------------------------------------------------------------------------
// TurnState carries the state of a turn for completion evaluation.
// ---------------------------------------------------------------------------

// TurnState represents the current state of a turn, used by CompletionEvaluator
// to determine whether the turn can be finalized.
type TurnState struct {
	SessionID        string   `json:"sessionId"`
	TurnID           string   `json:"turnId"`
	PendingApprovals []string `json:"pendingApprovals,omitempty"`
	PendingEvidence  []string `json:"pendingEvidence,omitempty"`
	ToolCallCount    int      `json:"toolCallCount"`
	Completed        bool     `json:"completed"`
}

// ---------------------------------------------------------------------------
// Evaluator interfaces — the four policy dimensions.
// ---------------------------------------------------------------------------

// ModePolicy evaluates whether a tool is allowed under the current mode's
// capability boundary. Each mode (chat/inspect/plan/execute) has its own
// ModePolicy implementation.
type ModePolicy interface {
	// CheckTool determines whether the given tool is permitted under this mode's
	// capability boundary.
	CheckTool(input PolicyInput) PolicyDecision
}

// PermissionEvaluator checks user-level permissions for a tool call.
type PermissionEvaluator interface {
	// CheckPermission evaluates whether the user has permission to invoke
	// the tool described in the input.
	CheckPermission(ctx context.Context, input PolicyInput) PolicyDecision
}

// EvidenceEvaluator determines whether evidence must be collected before
// a tool call can proceed.
type EvidenceEvaluator interface {
	// CheckEvidence evaluates whether evidence collection is required
	// for the tool call described in the input.
	CheckEvidence(ctx context.Context, input PolicyInput) PolicyDecision
}

// CompletionEvaluator performs the final gate check at turn end, ensuring
// all necessary approvals and evidence have been collected.
type CompletionEvaluator interface {
	// CheckCompletion evaluates whether the turn can be finalized given
	// the current turn state.
	CheckCompletion(ctx context.Context, turnState TurnState) PolicyDecision
}

// ---------------------------------------------------------------------------
// Engine is the unified policy engine entry point.
// ---------------------------------------------------------------------------

// Engine is the V2 policy engine that orchestrates the four policy dimensions:
// mode policy, permission evaluation, evidence evaluation, and completion evaluation.
type Engine struct {
	ModePolicy       map[Mode]ModePolicy
	PermissionPolicy PermissionEvaluator
	EvidencePolicy   EvidenceEvaluator
	CompletionPolicy CompletionEvaluator
}

// CheckToolCall executes the full policy check pipeline for a tool call:
// ModePolicy → PermissionPolicy → EvidencePolicy.
// The final decision is the most restrictive result across all layers.
func (e *Engine) CheckToolCall(ctx context.Context, input PolicyInput) PolicyDecision {
	// Layer 1: Mode capability boundary check
	if mp, ok := e.ModePolicy[input.Mode]; ok {
		decision := mp.CheckTool(input)
		if decision.Action != PolicyActionAllow {
			return decision
		}
	}

	// Layer 2: Permission check
	if e.PermissionPolicy != nil {
		decision := e.PermissionPolicy.CheckPermission(ctx, input)
		if decision.Action != PolicyActionAllow {
			return decision
		}
	}

	// Layer 3: Evidence check
	if e.EvidencePolicy != nil {
		decision := e.EvidencePolicy.CheckEvidence(ctx, input)
		if decision.Action != PolicyActionAllow {
			return decision
		}
	}

	return PolicyDecision{Action: PolicyActionAllow}
}
