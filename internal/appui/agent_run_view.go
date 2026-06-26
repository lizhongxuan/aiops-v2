package appui

import "time"

type AgentRunStatus string

const (
	AgentRunStatusPending   AgentRunStatus = "pending"
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusFailed    AgentRunStatus = "failed"
	AgentRunStatusCancelled AgentRunStatus = "cancelled"
)

type AgentStepKind string

const (
	AgentStepKindReasoning     AgentStepKind = "reasoning"
	AgentStepKindToolSearch    AgentStepKind = "tool_search"
	AgentStepKindToolCall      AgentStepKind = "tool_call"
	AgentStepKindApproval      AgentStepKind = "approval"
	AgentStepKindMCPHealth     AgentStepKind = "mcp_health"
	AgentStepKindEvidence      AgentStepKind = "evidence"
	AgentStepKindCheckpoint    AgentStepKind = "checkpoint"
	AgentStepKindFinalResponse AgentStepKind = "final_response"
	AgentStepKindError         AgentStepKind = "error"
)

type AgentStepStatus string

const (
	AgentStepStatusPending         AgentStepStatus = "pending"
	AgentStepStatusRunning         AgentStepStatus = "running"
	AgentStepStatusWaitingApproval AgentStepStatus = "waiting_approval"
	AgentStepStatusSkipped         AgentStepStatus = "skipped"
	AgentStepStatusCompleted       AgentStepStatus = "completed"
	AgentStepStatusFailed          AgentStepStatus = "failed"
	AgentStepStatusCancelled       AgentStepStatus = "cancelled"
)

// AgentRunView is a read-only projection of existing session, turn, item, trace,
// tool-call, approval, and checkpoint state. It must not own execution control.
type AgentRunView struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId,omitempty"`
	RootTurnID     string          `json:"rootTurnId,omitempty"`
	ActiveTurnID   string          `json:"activeTurnId,omitempty"`
	UserGoal       string          `json:"userGoal,omitempty"`
	NormalizedGoal string          `json:"normalizedGoal,omitempty"`
	RouteMode      string          `json:"routeMode,omitempty"`
	Profile        string          `json:"profile,omitempty"`
	Status         AgentRunStatus  `json:"status,omitempty"`
	TargetSummary  string          `json:"targetSummary,omitempty"`
	CurrentStep    string          `json:"currentStep,omitempty"`
	CurrentStepID  string          `json:"currentStepId,omitempty"`
	CheckpointID   string          `json:"checkpointId,omitempty"`
	EvidenceCount  int             `json:"evidenceCount,omitempty"`
	StartedAt      time.Time       `json:"startedAt,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt,omitempty"`
	Steps          []AgentStepView `json:"steps,omitempty"`
}

type AgentStepView struct {
	ID            string          `json:"id"`
	RunID         string          `json:"runId,omitempty"`
	TurnID        string          `json:"turnId,omitempty"`
	Iteration     int             `json:"iteration,omitempty"`
	Kind          AgentStepKind   `json:"kind,omitempty"`
	Status        AgentStepStatus `json:"status,omitempty"`
	Title         string          `json:"title,omitempty"`
	InputSummary  string          `json:"inputSummary,omitempty"`
	OutputSummary string          `json:"outputSummary,omitempty"`
	ToolName      string          `json:"toolName,omitempty"`
	ToolCallID    string          `json:"toolCallId,omitempty"`
	ApprovalID    string          `json:"approvalId,omitempty"`
	CheckpointID  string          `json:"checkpointId,omitempty"`
	TargetRefs    []string        `json:"targetRefs,omitempty"`
	EvidenceRefs  []string        `json:"evidenceRefs,omitempty"`
	Error         string          `json:"error,omitempty"`
	StartedAt     time.Time       `json:"startedAt,omitempty"`
	CompletedAt   time.Time       `json:"completedAt,omitempty"`
}
