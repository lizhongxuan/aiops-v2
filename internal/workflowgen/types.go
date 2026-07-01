package workflowgen

import (
	"context"
	"time"
)

type SessionStatus string

const (
	SessionStatusPlanStarted       SessionStatus = "plan_started"
	SessionStatusPlanReady         SessionStatus = "plan_ready"
	SessionStatusSlotRequired      SessionStatus = "slot_required"
	SessionStatusGenerationStarted SessionStatus = "generation_started"
	SessionStatusGraphReady        SessionStatus = "graph_preview_ready"
	SessionStatusValidationStarted SessionStatus = "validation_started"
	SessionStatusValidationPassed  SessionStatus = "validation_passed"
	SessionStatusValidationFailed  SessionStatus = "validation_failed"
	SessionStatusRepairStarted     SessionStatus = "repair_started"
	SessionStatusDraftSaved        SessionStatus = "draft_saved"
	SessionStatusFailed            SessionStatus = "failed"
)

type ValidationProvider string

const (
	ValidationProviderNone   ValidationProvider = "none"
	ValidationProviderDocker ValidationProvider = "docker"
)

type EventType string

const (
	EventPlanStarted       EventType = "plan.started"
	EventPlanReady         EventType = "plan.ready"
	EventSlotRequired      EventType = "slot.required"
	EventGenerationStarted EventType = "generation.started"
	EventNodeGenerating    EventType = "node.generating"
	EventNodeGenerated     EventType = "node.generated"
	EventGraphPreviewReady EventType = "graph.preview.ready"
	EventValidationStarted EventType = "validation.started"
	EventValidationLog     EventType = "validation.log"
	EventValidationFailed  EventType = "validation.failed"
	EventRepairStarted     EventType = "repair.started"
	EventRepairPatchReady  EventType = "repair.patch.ready"
	EventValidationPassed  EventType = "validation.passed"
	EventDraftSaved        EventType = "draft.saved"
	EventError             EventType = "error"
)

type TriggerType string

const (
	TriggerTypeManual   TriggerType = "manual"
	TriggerTypeSchedule TriggerType = "schedule"
)

type OutputTarget string

const (
	OutputTargetReturn  OutputTarget = "return"
	OutputTargetFeishu  OutputTarget = "feishu"
	OutputTargetEmail   OutputTarget = "email"
	OutputTargetWebhook OutputTarget = "webhook"
)

type ReviewStatus string

const (
	ReviewStatusDraft         ReviewStatus = "draft"
	ReviewStatusPendingReview ReviewStatus = "pending_review"
)

type NodeKind string

const (
	NodeKindSearch    NodeKind = "search"
	NodeKindTransform NodeKind = "transform"
	NodeKindOutput    NodeKind = "output"
)

type WorkflowGenerationSession struct {
	ID                  string                    `json:"id"`
	ConversationID      string                    `json:"conversation_id,omitempty"`
	UserID              string                    `json:"user_id,omitempty"`
	Status              SessionStatus             `json:"status"`
	Requirement         string                    `json:"requirement"`
	PlanVersion         int                       `json:"plan_version"`
	Plan                *WorkflowGenerationPlan   `json:"plan,omitempty"`
	Slots               []RequiredSlot            `json:"slots,omitempty"`
	DraftWorkflowID     string                    `json:"draft_workflow_id,omitempty"`
	ValidationProvider  ValidationProvider        `json:"validation_provider,omitempty"`
	ValidationRuns      []ValidationRunSummary    `json:"validation_runs,omitempty"`
	Events              []WorkflowGenerationEvent `json:"events,omitempty"`
	CreatedByUserPrompt bool                      `json:"created_by_user_prompt,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type WorkflowGenerationPlan struct {
	Version            int                `json:"version"`
	Title              string             `json:"title"`
	Intent             string             `json:"intent"`
	ReviewStatus       ReviewStatus       `json:"review_status,omitempty"`
	ResourceKind       string             `json:"resource_kind,omitempty"`
	OperationFrame     map[string]any     `json:"operation_frame,omitempty"`
	Trigger            WorkflowTrigger    `json:"trigger"`
	Inputs             []WorkflowIO       `json:"inputs,omitempty"`
	Nodes              []WorkflowPlanNode `json:"nodes"`
	Outputs            []WorkflowOutput   `json:"outputs"`
	ValidationStrategy ValidationStrategy `json:"validation_strategy"`
	Risks              []string           `json:"risks,omitempty"`
	RequiredSlots      []RequiredSlot     `json:"required_slots,omitempty"`
}

type WorkflowTrigger struct {
	Type     TriggerType `json:"type"`
	Schedule string      `json:"schedule,omitempty"`
	Summary  string      `json:"summary,omitempty"`
}

type WorkflowIO struct {
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type WorkflowPlanNode struct {
	ID          string         `json:"id"`
	Kind        NodeKind       `json:"kind"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Action      string         `json:"action,omitempty"`
	Inputs      []WorkflowIO   `json:"inputs,omitempty"`
	Outputs     []WorkflowIO   `json:"outputs,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
}

type WorkflowOutput struct {
	ID          string       `json:"id"`
	Target      OutputTarget `json:"target"`
	Description string       `json:"description,omitempty"`
	SecretRef   string       `json:"secret_ref,omitempty"`
}

type ValidationStrategy struct {
	Enabled  bool               `json:"enabled"`
	Provider ValidationProvider `json:"provider"`
	Scenario string             `json:"scenario,omitempty"`
	Network  string             `json:"network,omitempty"`
}

type RequiredSlot struct {
	ID          string   `json:"id"`
	Label       string   `json:"label,omitempty"`
	Question    string   `json:"question"`
	Type        string   `json:"type,omitempty"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"`
	Description string   `json:"description,omitempty"`
}

type ValidationRunSummary struct {
	ID        string             `json:"id"`
	Provider  ValidationProvider `json:"provider"`
	Status    string             `json:"status"`
	Scenario  string             `json:"scenario,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	StartedAt time.Time          `json:"started_at,omitempty"`
	EndedAt   time.Time          `json:"ended_at,omitempty"`
}

type WorkflowGenerationEvent struct {
	ID        string         `json:"id"`
	Sequence  int64          `json:"sequence"`
	Type      EventType      `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	NodeID    string         `json:"node_id,omitempty"`
	Status    string         `json:"status,omitempty"`
	Message   string         `json:"message,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type BuildPlanRequest struct {
	Requirement string            `json:"requirement"`
	Slots       map[string]string `json:"slots,omitempty"`
}

type RevisePlanRequest struct {
	Previous WorkflowGenerationPlan `json:"previous"`
	Message  string                 `json:"message"`
}

type PlanBuilder interface {
	BuildPlan(ctx context.Context, req BuildPlanRequest) (*WorkflowGenerationPlan, error)
	RevisePlan(ctx context.Context, req RevisePlanRequest) (*WorkflowGenerationPlan, error)
}
