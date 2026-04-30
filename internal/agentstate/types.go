package agentstate

import (
	"encoding/json"
	"time"
)

// AgentPhase describes the coarse Plan/Act/Observe lifecycle for one turn.
type AgentPhase string

const (
	AgentPhaseUnderstanding AgentPhase = "understanding"
	AgentPhasePlanning      AgentPhase = "planning"
	AgentPhaseActing        AgentPhase = "acting"
	AgentPhaseObserving     AgentPhase = "observing"
	AgentPhaseReflecting    AgentPhase = "reflecting"
	AgentPhaseFinished      AgentPhase = "finished"
	AgentPhaseFailed        AgentPhase = "failed"
)

// TurnItemType is the protocol-level event type stored in shadow state.
type TurnItemType string

const (
	TurnItemTypeUserMessage TurnItemType = "user_message"
	TurnItemTypeModelCall   TurnItemType = "model_call"
	TurnItemTypeToolCall    TurnItemType = "tool_call"
	TurnItemTypeToolResult  TurnItemType = "tool_result"
	TurnItemTypePlan        TurnItemType = "plan"
	TurnItemTypeApproval    TurnItemType = "approval"
	TurnItemTypeEvidence    TurnItemType = "evidence"
	TurnItemTypeFinalAnswer TurnItemType = "final_answer"
	TurnItemTypeError       TurnItemType = "error"
)

// ItemStatus is the canonical lifecycle status for a TurnItem.
type ItemStatus string

const (
	ItemStatusPending   ItemStatus = "pending"
	ItemStatusRunning   ItemStatus = "running"
	ItemStatusCompleted ItemStatus = "completed"
	ItemStatusBlocked   ItemStatus = "blocked"
	ItemStatusFailed    ItemStatus = "failed"
	ItemStatusCancelled ItemStatus = "cancelled"
)

// AgentState is the shadow protocol state for a single agent turn.
type AgentState struct {
	SessionID string        `json:"sessionId"`
	TurnID    string        `json:"turnId"`
	Phase     AgentPhase    `json:"phase"`
	Items     []TurnItem    `json:"items,omitempty"`
	Plan      PlanState     `json:"plan,omitempty"`
	Evidence  EvidenceState `json:"evidence,omitempty"`
	Approvals ApprovalState `json:"approvals,omitempty"`
	Budget    BudgetState   `json:"budget,omitempty"`
}

// TurnItem records one protocol-level state transition.
type TurnItem struct {
	ID        string          `json:"id"`
	Type      TurnItemType    `json:"type"`
	Status    ItemStatus      `json:"status"`
	Payload   PayloadEnvelope `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"createdAt,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt,omitempty"`
}

// PayloadEnvelope keeps reducer/projection code independent of concrete tool,
// plan, approval, and model payload types.
type PayloadEnvelope struct {
	Kind    string          `json:"kind,omitempty"`
	Summary string          `json:"summary,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type PlanState struct {
	Steps []PlanStep `json:"steps,omitempty"`
}

type PlanStep struct {
	ID     string     `json:"id,omitempty"`
	Text   string     `json:"text"`
	Status ItemStatus `json:"status"`
}

type EvidenceState struct {
	Required []string `json:"required,omitempty"`
	Provided []string `json:"provided,omitempty"`
}

type ApprovalState struct {
	Pending []string `json:"pending,omitempty"`
	Granted []string `json:"granted,omitempty"`
	Denied  []string `json:"denied,omitempty"`
}

type BudgetState struct {
	MaxIterations int `json:"maxIterations,omitempty"`
	Iterations    int `json:"iterations,omitempty"`
	MaxToolCalls  int `json:"maxToolCalls,omitempty"`
	ToolCalls     int `json:"toolCalls,omitempty"`
}

func (p AgentPhase) IsValid() bool {
	switch p {
	case AgentPhaseUnderstanding, AgentPhasePlanning, AgentPhaseActing, AgentPhaseObserving, AgentPhaseReflecting, AgentPhaseFinished, AgentPhaseFailed:
		return true
	default:
		return false
	}
}

func (t TurnItemType) IsValid() bool {
	switch t {
	case TurnItemTypeUserMessage, TurnItemTypeModelCall, TurnItemTypeToolCall, TurnItemTypeToolResult, TurnItemTypePlan, TurnItemTypeApproval, TurnItemTypeEvidence, TurnItemTypeFinalAnswer, TurnItemTypeError:
		return true
	default:
		return false
	}
}

func (s ItemStatus) IsValid() bool {
	switch s {
	case ItemStatusPending, ItemStatusRunning, ItemStatusCompleted, ItemStatusBlocked, ItemStatusFailed, ItemStatusCancelled:
		return true
	default:
		return false
	}
}
