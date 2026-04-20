package runtimekernel

import (
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// SessionState carries the full state of a session.
// ---------------------------------------------------------------------------

// SessionState represents the full state of a session (host or workspace).
type SessionState struct {
	ID        string        `json:"id"`
	Type      SessionType   `json:"type"`
	Mode      Mode          `json:"mode"`
	HostID    string        `json:"hostId,omitempty"`
	Messages  []Message     `json:"messages"`
	Context   ContextWindow `json:"context"`
	Activity  ActivityStats `json:"activity"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// Validate checks that the session state has valid session type and mode.
func (s SessionState) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("session id is required")
	}
	if !s.Type.IsValid() {
		return fmt.Errorf("invalid session type %q", s.Type)
	}
	if !s.Mode.IsValid() {
		return fmt.Errorf("invalid mode %q", s.Mode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Message represents a single message in a session (compatible with frontend).
// ---------------------------------------------------------------------------

// Message represents a single message in a session conversation.
type Message struct {
	ID         string      `json:"id"`
	Role       string      `json:"role"` // user, assistant, system, tool
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"toolCalls,omitempty"`
	ToolResult *ToolResult `json:"toolResult,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// ToolCall represents a tool invocation request from the LLM.
// ---------------------------------------------------------------------------

// ToolCall represents a tool invocation request (aligned with Eino ToolCall).
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ---------------------------------------------------------------------------
// ToolResult represents the result of a tool execution.
// ---------------------------------------------------------------------------

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolCallID string              `json:"toolCallId"`
	Content    string              `json:"content"`
	Display    *ToolDisplayPayload `json:"display,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// ToolDisplayPayload is the structured UI output for tool results.
// ---------------------------------------------------------------------------

// ToolDisplayPayload is the structured UI output for tool results.
type ToolDisplayPayload struct {
	Type    string          `json:"type"`
	Title   string          `json:"title,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	CardRef string          `json:"cardRef,omitempty"`
}

// ---------------------------------------------------------------------------
// ContextWindow tracks token usage and truncation state.
// ---------------------------------------------------------------------------

// ContextWindow tracks token usage and truncation state for a session.
type ContextWindow struct {
	MaxTokens   int `json:"maxTokens"`
	UsedTokens  int `json:"usedTokens"`
	Messages    int `json:"messages"`
	TruncatedAt int `json:"truncatedAt,omitempty"`
}

// ---------------------------------------------------------------------------
// ActivityStats tracks runtime activity counters.
// ---------------------------------------------------------------------------

// ActivityStats tracks runtime activity counters (runtime.activity).
type ActivityStats struct {
	SearchCount    int `json:"searchCount"`
	BrowseCount    int `json:"browseCount"`
	CommandCount   int `json:"commandCount"`
	FileReadCount  int `json:"fileReadCount"`
	FileWriteCount int `json:"fileWriteCount"`
}

// ---------------------------------------------------------------------------
// ApprovalRecord represents an approval decision record.
// ---------------------------------------------------------------------------

// ApprovalRecord represents an approval decision record.
type ApprovalRecord struct {
	ID        string     `json:"id"`
	SessionID string     `json:"sessionId"`
	TurnID    string     `json:"turnId"`
	ToolName  string     `json:"toolName"`
	Command   string     `json:"command,omitempty"`
	HostID    string     `json:"hostId,omitempty"`
	Status    string     `json:"status"` // pending, approved, denied
	Operator  string     `json:"operator,omitempty"`
	Decision  string     `json:"decision,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
}

// Validate checks that the approval record has required fields.
func (a ApprovalRecord) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("approval id is required")
	}
	if a.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if a.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if a.ToolName == "" {
		return fmt.Errorf("tool name is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// WorkspaceTask represents a workspace task (reference: claude code/Task.ts).
// ---------------------------------------------------------------------------

// WorkspaceTask represents a workspace task with lifecycle management.
type WorkspaceTask struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`   // host_exec, multi_host, plan
	Status      string     `json:"status"` // pending, running, completed, failed, killed
	Description string     `json:"description"`
	HostIDs     []string   `json:"hostIds,omitempty"`
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Output      string     `json:"output,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// Validate checks that the workspace task has required fields.
func (t WorkspaceTask) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task id is required")
	}
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if t.Status == "" {
		return fmt.Errorf("task status is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// SessionType identifies the two user-visible session domains.
// ---------------------------------------------------------------------------

// SessionType identifies the only two user-visible session domains in V2.
type SessionType string

const (
	SessionTypeHost      SessionType = "host"
	SessionTypeWorkspace SessionType = "workspace"
)

var allSessionTypes = []SessionType{
	SessionTypeHost,
	SessionTypeWorkspace,
}

// AllSessionTypes returns the canonical V2 session types.
func AllSessionTypes() []SessionType {
	out := make([]SessionType, len(allSessionTypes))
	copy(out, allSessionTypes)
	return out
}

// IsValid reports whether the value is one of the canonical V2 session types.
func (s SessionType) IsValid() bool {
	switch s {
	case SessionTypeHost, SessionTypeWorkspace:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Mode identifies the four canonical runtime policies.
// ---------------------------------------------------------------------------

// Mode identifies the only four canonical runtime policies in V2.
type Mode string

const (
	ModeChat    Mode = "chat"
	ModeInspect Mode = "inspect"
	ModePlan    Mode = "plan"
	ModeExecute Mode = "execute"
)

var allModes = []Mode{
	ModeChat,
	ModeInspect,
	ModePlan,
	ModeExecute,
}

// AllModes returns the canonical V2 modes.
func AllModes() []Mode {
	out := make([]Mode, len(allModes))
	copy(out, allModes)
	return out
}

// IsValid reports whether the value is one of the canonical V2 modes.
func (m Mode) IsValid() bool {
	switch m {
	case ModeChat, ModeInspect, ModePlan, ModeExecute:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// TurnRequest / TurnResult / ResumeRequest / CancelRequest
// ---------------------------------------------------------------------------

// TurnRequest is the typed V2 input contract for a runtime turn.
type TurnRequest struct {
	SessionType SessionType       `json:"sessionType"`
	Mode        Mode              `json:"mode"`
	SessionID   string            `json:"sessionId,omitempty"`
	TurnID      string            `json:"turnId,omitempty"`
	Input       string            `json:"input,omitempty"`
	HostID      string            `json:"hostId,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Validate checks that the request uses canonical session and mode values.
func (r TurnRequest) Validate() error {
	if !r.SessionType.IsValid() {
		return fmt.Errorf("invalid session type %q", r.SessionType)
	}
	if !r.Mode.IsValid() {
		return fmt.Errorf("invalid mode %q", r.Mode)
	}
	return nil
}

// TurnResult is the typed V2 output contract for a completed or failed turn.
type TurnResult struct {
	SessionType SessionType `json:"sessionType"`
	Mode        Mode        `json:"mode"`
	SessionID   string      `json:"sessionId"`
	TurnID      string      `json:"turnId"`
	Status      string      `json:"status"`
	Output      string      `json:"output,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// Validate checks that the result keeps the V2 typed contract intact.
func (r TurnResult) Validate() error {
	if !r.SessionType.IsValid() {
		return fmt.Errorf("invalid session type %q", r.SessionType)
	}
	if !r.Mode.IsValid() {
		return fmt.Errorf("invalid mode %q", r.Mode)
	}
	if r.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if r.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	return nil
}

// ResumeRequest resumes a turn that was interrupted (e.g. by approval).
type ResumeRequest struct {
	SessionID  string            `json:"sessionId"`
	TurnID     string            `json:"turnId"`
	ApprovalID string            `json:"approvalId,omitempty"`
	Decision   string            `json:"decision,omitempty"` // approved, denied
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Validate checks that the resume request has required fields.
func (r ResumeRequest) Validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if r.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	return nil
}

// CancelRequest cancels an active turn.
type CancelRequest struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	Reason    string `json:"reason,omitempty"`
}

// Validate checks that the cancel request has required fields.
func (r CancelRequest) Validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if r.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// RuntimeContext carries typed runtime metadata for capability/policy decisions.
// ---------------------------------------------------------------------------

// RuntimeContext carries typed runtime metadata for capability and policy decisions.
type RuntimeContext struct {
	SessionType         SessionType       `json:"sessionType"`
	Mode                Mode              `json:"mode"`
	SessionID           string            `json:"sessionId,omitempty"`
	HostID              string            `json:"hostId,omitempty"`
	WorkspaceSessionID  string            `json:"workspaceSessionId,omitempty"`
	VisibleCapabilities []string          `json:"visibleCapabilities,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

// Validate checks that the context stays inside the V2 session and mode set.
func (c RuntimeContext) Validate() error {
	if !c.SessionType.IsValid() {
		return fmt.Errorf("invalid session type %q", c.SessionType)
	}
	if !c.Mode.IsValid() {
		return fmt.Errorf("invalid mode %q", c.Mode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// LifecycleEvent and EventType (Projection layer contract)
// ---------------------------------------------------------------------------

// EventType is the projection-layer event type enumeration.
type EventType string

const (
	EventToolStarted       EventType = "tool.started"
	EventToolProgress      EventType = "tool.progress"
	EventToolCompleted     EventType = "tool.completed"
	EventToolFailed        EventType = "tool.failed"
	EventApprovalNeeded    EventType = "approval.needed"
	EventApprovalDecided   EventType = "approval.decided"
	EventEvidenceCollected EventType = "evidence.collected"
	EventTurnComplete      EventType = "turn.complete"
	EventActivityUpdate    EventType = "activity.update"
	EventCardGenerated     EventType = "card.generated"
)

var allEventTypes = []EventType{
	EventToolStarted,
	EventToolProgress,
	EventToolCompleted,
	EventToolFailed,
	EventApprovalNeeded,
	EventApprovalDecided,
	EventEvidenceCollected,
	EventTurnComplete,
	EventActivityUpdate,
	EventCardGenerated,
}

// AllEventTypes returns all canonical event types.
func AllEventTypes() []EventType {
	out := make([]EventType, len(allEventTypes))
	copy(out, allEventTypes)
	return out
}

// IsValid reports whether the value is one of the canonical event types.
func (e EventType) IsValid() bool {
	switch e {
	case EventToolStarted, EventToolProgress, EventToolCompleted, EventToolFailed,
		EventApprovalNeeded, EventApprovalDecided, EventEvidenceCollected,
		EventTurnComplete, EventActivityUpdate, EventCardGenerated:
		return true
	default:
		return false
	}
}

// LifecycleEvent is the unified lifecycle event emitted by RuntimeKernel
// and consumed by the Projection layer.
type LifecycleEvent struct {
	Type      EventType       `json:"type"`
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Validate checks that the lifecycle event carries a supported shape.
func (e LifecycleEvent) Validate() error {
	if !e.Type.IsValid() {
		return fmt.Errorf("invalid event type %q", e.Type)
	}
	if e.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if e.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}
