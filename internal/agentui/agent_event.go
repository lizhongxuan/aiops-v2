package agentui

import (
	"encoding/json"
	"fmt"
)

type AgentEventKind string
type AgentEventPhase string
type AgentEventStatus string
type AgentEventVisibility string
type AgentEventSource string

const (
	AgentEventTurn      AgentEventKind = "turn"
	AgentEventAgent     AgentEventKind = "agent"
	AgentEventAssistant AgentEventKind = "assistant"
	AgentEventTool      AgentEventKind = "tool"
	AgentEventApproval  AgentEventKind = "approval"
	AgentEventArtifact  AgentEventKind = "artifact"
	AgentEventDiff      AgentEventKind = "diff"
	AgentEventBrowser   AgentEventKind = "browser"
	AgentEventSystem    AgentEventKind = "system"
)

const (
	AgentEventPhaseRequested AgentEventPhase = "requested"
	AgentEventPhaseStarted   AgentEventPhase = "started"
	AgentEventPhaseDelta     AgentEventPhase = "delta"
	AgentEventPhaseUpdated   AgentEventPhase = "updated"
	AgentEventPhaseCompleted AgentEventPhase = "completed"
	AgentEventPhaseFailed    AgentEventPhase = "failed"
	AgentEventPhaseCanceled  AgentEventPhase = "canceled"
	AgentEventPhaseBlocked   AgentEventPhase = "blocked"
	AgentEventPhaseResolved  AgentEventPhase = "resolved"
)

const (
	AgentEventStatusQueued    AgentEventStatus = "queued"
	AgentEventStatusRunning   AgentEventStatus = "running"
	AgentEventStatusWaiting   AgentEventStatus = "waiting"
	AgentEventStatusBlocked   AgentEventStatus = "blocked"
	AgentEventStatusCompleted AgentEventStatus = "completed"
	AgentEventStatusFailed    AgentEventStatus = "failed"
	AgentEventStatusCanceled  AgentEventStatus = "canceled"
	AgentEventStatusSkipped   AgentEventStatus = "skipped"
)

const (
	AgentEventVisibilityPrimary   AgentEventVisibility = "primary"
	AgentEventVisibilitySecondary AgentEventVisibility = "secondary"
	AgentEventVisibilityDebug     AgentEventVisibility = "debug"
	AgentEventVisibilityHidden    AgentEventVisibility = "hidden"
)

const (
	AgentEventSourceRuntime    AgentEventSource = "runtime"
	AgentEventSourceTool       AgentEventSource = "tool"
	AgentEventSourceMCP        AgentEventSource = "mcp"
	AgentEventSourceApproval   AgentEventSource = "approval"
	AgentEventSourceUI         AgentEventSource = "ui"
	AgentEventSourceProjection AgentEventSource = "projection"
	AgentEventSourceSystem     AgentEventSource = "system"
)

type AgentEvent struct {
	EventID       string               `json:"eventId"`
	Seq           int64                `json:"seq"`
	SessionID     string               `json:"sessionId"`
	ThreadID      string               `json:"threadId,omitempty"`
	TurnID        string               `json:"turnId,omitempty"`
	ClientTurnID  string               `json:"clientTurnId,omitempty"`
	AgentID       string               `json:"agentId,omitempty"`
	ParentAgentID string               `json:"parentAgentId,omitempty"`
	Kind          AgentEventKind       `json:"kind"`
	Phase         AgentEventPhase      `json:"phase"`
	Status        AgentEventStatus     `json:"status"`
	Visibility    AgentEventVisibility `json:"visibility"`
	Source        AgentEventSource     `json:"source"`
	CreatedAt     string               `json:"createdAt"`
	StartedAt     string               `json:"startedAt,omitempty"`
	CompletedAt   string               `json:"completedAt,omitempty"`
	DurationMs    int64                `json:"durationMs,omitempty"`
	Payload       json.RawMessage      `json:"payload,omitempty"`
}

type AgentConfig struct {
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	CWD             string   `json:"cwd,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	Permission      string   `json:"permission,omitempty"`
}

type AgentStats struct {
	CommandsRun  int `json:"commandsRun,omitempty"`
	FilesRead    int `json:"filesRead,omitempty"`
	FilesChanged int `json:"filesChanged,omitempty"`
	ToolsCalled  int `json:"toolsCalled,omitempty"`
}

type TurnPayload struct {
	Title           string `json:"title,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Mode            string `json:"mode,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Error           string `json:"error,omitempty"`
}

type AgentPayload struct {
	Handle          string      `json:"handle,omitempty"`
	Name            string      `json:"name,omitempty"`
	Role            string      `json:"role,omitempty"`
	LastAction      string      `json:"lastAction,omitempty"`
	LastSummary     string      `json:"lastSummary,omitempty"`
	BlockedReason   string      `json:"blockedReason,omitempty"`
	RequestedConfig AgentConfig `json:"requestedConfig,omitempty"`
	EffectiveConfig AgentConfig `json:"effectiveConfig,omitempty"`
	Stats           AgentStats  `json:"stats,omitempty"`
}

type AssistantPayload struct {
	Text      string `json:"text,omitempty"`
	Delta     string `json:"delta,omitempty"`
	Channel   string `json:"channel,omitempty"`
	MessageID string `json:"messageId,omitempty"`
}

type ToolPayload struct {
	ToolCallID    string `json:"toolCallId,omitempty"`
	ToolName      string `json:"toolName,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	InputSummary  string `json:"inputSummary,omitempty"`
	OutputSummary string `json:"outputSummary,omitempty"`
	Error         string `json:"error,omitempty"`
	ArtifactID    string `json:"artifactId,omitempty"`
	ExitCode      *int   `json:"exitCode,omitempty"`
}

type ApprovalPayload struct {
	ApprovalID   string   `json:"approvalId,omitempty"`
	ApprovalType string   `json:"approvalType,omitempty"`
	Title        string   `json:"title,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Decision     string   `json:"decision,omitempty"`
	Targets      []string `json:"targets,omitempty"`
}

type ArtifactPayload struct {
	ArtifactID  string `json:"artifactId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	URI         string `json:"uri,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

type DiffFile struct {
	Path         string `json:"path"`
	Status       string `json:"status,omitempty"`
	AddedLines   int    `json:"addedLines,omitempty"`
	RemovedLines int    `json:"removedLines,omitempty"`
}

type DiffPayload struct {
	Scope        string     `json:"scope,omitempty"`
	Files        []DiffFile `json:"files,omitempty"`
	FilesCount   int        `json:"filesCount,omitempty"`
	AddedLines   int        `json:"addedLines,omitempty"`
	RemovedLines int        `json:"removedLines,omitempty"`
	Summary      string     `json:"summary,omitempty"`
}

type BrowserPayload struct {
	Action     string `json:"action,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Screenshot string `json:"screenshot,omitempty"`
}

func (e AgentEvent) Validate() error {
	if e.EventID == "" {
		return fmt.Errorf("event id is required")
	}
	if e.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if e.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if !e.Kind.IsValid() {
		return fmt.Errorf("invalid kind %q", e.Kind)
	}
	if e.Phase == "" {
		return fmt.Errorf("phase is required")
	}
	if !e.Phase.IsValid() {
		return fmt.Errorf("invalid phase %q", e.Phase)
	}
	if e.Status == "" {
		return fmt.Errorf("status is required")
	}
	if !e.Status.IsValid() {
		return fmt.Errorf("invalid status %q", e.Status)
	}
	if e.Visibility == "" {
		return fmt.Errorf("visibility is required")
	}
	if !e.Visibility.IsValid() {
		return fmt.Errorf("invalid visibility %q", e.Visibility)
	}
	if e.Source == "" {
		return fmt.Errorf("source is required")
	}
	if e.CreatedAt == "" {
		return fmt.Errorf("createdAt is required")
	}
	return nil
}

func (k AgentEventKind) IsValid() bool {
	switch k {
	case AgentEventTurn, AgentEventAgent, AgentEventAssistant, AgentEventTool, AgentEventApproval,
		AgentEventArtifact, AgentEventDiff, AgentEventBrowser, AgentEventSystem:
		return true
	default:
		return false
	}
}

func (p AgentEventPhase) IsValid() bool {
	switch p {
	case AgentEventPhaseRequested, AgentEventPhaseStarted, AgentEventPhaseDelta, AgentEventPhaseUpdated,
		AgentEventPhaseCompleted, AgentEventPhaseFailed, AgentEventPhaseCanceled, AgentEventPhaseBlocked,
		AgentEventPhaseResolved:
		return true
	default:
		return false
	}
}

func (s AgentEventStatus) IsValid() bool {
	switch s {
	case AgentEventStatusQueued, AgentEventStatusRunning, AgentEventStatusWaiting, AgentEventStatusBlocked,
		AgentEventStatusCompleted, AgentEventStatusFailed, AgentEventStatusCanceled, AgentEventStatusSkipped:
		return true
	default:
		return false
	}
}

func (v AgentEventVisibility) IsValid() bool {
	switch v {
	case AgentEventVisibilityPrimary, AgentEventVisibilitySecondary, AgentEventVisibilityDebug, AgentEventVisibilityHidden:
		return true
	default:
		return false
	}
}
