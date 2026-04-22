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
	ID                 string              `json:"id"`
	Type               SessionType         `json:"type"`
	Mode               Mode                `json:"mode"`
	HostID             string              `json:"hostId,omitempty"`
	Messages           []Message           `json:"messages"`
	Context            ContextWindow       `json:"context"`
	Activity           ActivityStats       `json:"activity"`
	CurrentTurn        *TurnSnapshot       `json:"currentTurn,omitempty"`
	TurnHistory        []TurnSnapshot      `json:"turnHistory,omitempty"`
	PendingApprovals   []PendingApproval   `json:"pendingApprovals,omitempty"`
	PendingEvidence    []PendingEvidence   `json:"pendingEvidence,omitempty"`
	LatestCheckpoint   *CheckpointMetadata `json:"latestCheckpoint,omitempty"`
	CompactedSegments  []CompactedSegment  `json:"compactedSegments,omitempty"`
	ExternalReferences []ExternalReference `json:"externalReferences,omitempty"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
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
	if s.CurrentTurn != nil {
		if err := s.CurrentTurn.Validate(); err != nil {
			return fmt.Errorf("current turn: %w", err)
		}
	}
	for i := range s.Messages {
		if s.Messages[i].ToolResult != nil {
			if err := s.Messages[i].ToolResult.Validate(); err != nil {
				return fmt.Errorf("message[%d] tool result: %w", i, err)
			}
		}
	}
	for i := range s.TurnHistory {
		if err := s.TurnHistory[i].Validate(); err != nil {
			return fmt.Errorf("turn history[%d]: %w", i, err)
		}
	}
	for i := range s.PendingApprovals {
		if err := s.PendingApprovals[i].Validate(); err != nil {
			return fmt.Errorf("pending approval[%d]: %w", i, err)
		}
	}
	for i := range s.PendingEvidence {
		if err := s.PendingEvidence[i].Validate(); err != nil {
			return fmt.Errorf("pending evidence[%d]: %w", i, err)
		}
	}
	if s.LatestCheckpoint != nil {
		if err := s.LatestCheckpoint.Validate(); err != nil {
			return fmt.Errorf("latest checkpoint: %w", err)
		}
	}
	for i := range s.CompactedSegments {
		if err := s.CompactedSegments[i].Validate(); err != nil {
			return fmt.Errorf("compacted segment[%d]: %w", i, err)
		}
	}
	for i := range s.ExternalReferences {
		if err := s.ExternalReferences[i].Validate(); err != nil {
			return fmt.Errorf("external reference[%d]: %w", i, err)
		}
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
	ToolCallID         string                `json:"toolCallId"`
	Content            string                `json:"content"`
	Summary            string                `json:"summary,omitempty"`
	Display            *ToolDisplayPayload   `json:"display,omitempty"`
	Error              string                `json:"error,omitempty"`
	References         []ToolResultReference `json:"references,omitempty"`
	Spilled            bool                  `json:"spilled,omitempty"`
	ExternalReferences []ExternalReference   `json:"externalReferences,omitempty"`
}

// Validate checks the tool result payload.
func (r ToolResult) Validate() error {
	for i := range r.References {
		if err := r.References[i].Validate(); err != nil {
			return fmt.Errorf("reference[%d]: %w", i, err)
		}
	}
	for i := range r.ExternalReferences {
		if err := r.ExternalReferences[i].Validate(); err != nil {
			return fmt.Errorf("external reference[%d]: %w", i, err)
		}
	}
	return nil
}

// ToolProgressUpdate captures an incremental progress or partial-output update
// emitted while a long-running tool is still executing.
type ToolProgressUpdate struct {
	ToolCallID string    `json:"toolCallId,omitempty"`
	ToolName   string    `json:"toolName,omitempty"`
	Delta      string    `json:"delta,omitempty"`
	TotalRead  int       `json:"totalRead,omitempty"`
	Done       bool      `json:"done,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
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

// ToolResultReferenceKind describes the external carrier for a tool result.
type ToolResultReferenceKind string

const (
	ToolResultReferenceKindBlob ToolResultReferenceKind = "blob"
	ToolResultReferenceKindCard ToolResultReferenceKind = "card"
	ToolResultReferenceKindFile ToolResultReferenceKind = "file"
)

// ToolResultReference captures a structured external reference attached to a tool result.
type ToolResultReference struct {
	Kind        ToolResultReferenceKind `json:"kind"`
	URI         string                  `json:"uri,omitempty"`
	CardRef     string                  `json:"cardRef,omitempty"`
	FilePath    string                  `json:"filePath,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Summary     string                  `json:"summary,omitempty"`
	ContentType string                  `json:"contentType,omitempty"`
	Digest      string                  `json:"digest,omitempty"`
	Bytes       int64                   `json:"bytes,omitempty"`
}

// Validate checks the tool result reference.
func (r ToolResultReference) Validate() error {
	switch r.Kind {
	case ToolResultReferenceKindBlob, ToolResultReferenceKindCard, ToolResultReferenceKindFile:
	default:
		return fmt.Errorf("invalid tool result reference kind %q", r.Kind)
	}
	switch r.Kind {
	case ToolResultReferenceKindBlob:
		if r.URI == "" {
			return fmt.Errorf("blob reference uri is required")
		}
	case ToolResultReferenceKindCard:
		if r.CardRef == "" {
			return fmt.Errorf("card reference cardRef is required")
		}
	case ToolResultReferenceKindFile:
		if r.FilePath == "" {
			return fmt.Errorf("file reference filePath is required")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Long-running turn state model.
// ---------------------------------------------------------------------------

// TurnLifecycleState tracks the coarse state of a turn.
type TurnLifecycleState string

const (
	TurnLifecyclePending   TurnLifecycleState = "pending"
	TurnLifecycleRunning   TurnLifecycleState = "running"
	TurnLifecycleSuspended TurnLifecycleState = "suspended"
	TurnLifecycleResumable TurnLifecycleState = "resumable"
	TurnLifecycleCompleted TurnLifecycleState = "completed"
	TurnLifecycleFailed    TurnLifecycleState = "failed"
	TurnLifecycleCanceled  TurnLifecycleState = "canceled"
)

// IsValid reports whether the lifecycle state is canonical.
func (s TurnLifecycleState) IsValid() bool {
	switch s {
	case TurnLifecyclePending, TurnLifecycleRunning, TurnLifecycleSuspended,
		TurnLifecycleResumable, TurnLifecycleCompleted, TurnLifecycleFailed, TurnLifecycleCanceled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the lifecycle state is terminal.
func (s TurnLifecycleState) IsTerminal() bool {
	switch s {
	case TurnLifecycleCompleted, TurnLifecycleFailed, TurnLifecycleCanceled:
		return true
	default:
		return false
	}
}

// CanResume reports whether the turn can be resumed from this lifecycle state.
func (s TurnLifecycleState) CanResume() bool {
	switch s {
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		return true
	default:
		return false
	}
}

// TurnResumeState tracks why or how a turn is resumable.
type TurnResumeState string

const (
	TurnResumeStateNone            TurnResumeState = "none"
	TurnResumeStatePendingApproval TurnResumeState = "pending_approval"
	TurnResumeStatePendingEvidence TurnResumeState = "pending_evidence"
	TurnResumeStateCheckpointReady TurnResumeState = "checkpoint_ready"
	TurnResumeStateResumable       TurnResumeState = "resumable"
)

// IsValid reports whether the resume state is canonical.
func (s TurnResumeState) IsValid() bool {
	switch s {
	case TurnResumeStateNone, TurnResumeStatePendingApproval, TurnResumeStatePendingEvidence,
		TurnResumeStateCheckpointReady, TurnResumeStateResumable:
		return true
	default:
		return false
	}
}

// CheckpointMetadata captures the persistent marker for a turn or iteration.
type CheckpointMetadata struct {
	ID           string             `json:"id"`
	SessionID    string             `json:"sessionId"`
	TurnID       string             `json:"turnId"`
	Iteration    int                `json:"iteration"`
	Sequence     int                `json:"sequence"`
	Kind         string             `json:"kind,omitempty"`
	Source       string             `json:"source,omitempty"`
	Lifecycle    TurnLifecycleState `json:"lifecycle,omitempty"`
	ResumeState  TurnResumeState    `json:"resumeState,omitempty"`
	Incremental  bool               `json:"incremental,omitempty"`
	Compacted    bool               `json:"compacted,omitempty"`
	CreatedAt    time.Time          `json:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt"`
	Checksum     string             `json:"checksum,omitempty"`
	ExternalRefs []string           `json:"externalRefs,omitempty"`
}

// Validate checks that the checkpoint metadata is structurally sound.
func (m CheckpointMetadata) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("checkpoint id is required")
	}
	if m.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if m.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if m.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	if m.Sequence < 0 {
		return fmt.Errorf("sequence must be >= 0")
	}
	if !m.Lifecycle.IsValid() && m.Lifecycle != "" {
		return fmt.Errorf("invalid lifecycle %q", m.Lifecycle)
	}
	if !m.ResumeState.IsValid() && m.ResumeState != "" {
		return fmt.Errorf("invalid resume state %q", m.ResumeState)
	}
	return nil
}

// PendingApproval represents a structured approval gate waiting to be resumed.
type PendingApproval struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"sessionId"`
	TurnID     string     `json:"turnId"`
	Iteration  int        `json:"iteration"`
	ToolName   string     `json:"toolName"`
	ToolCallID string     `json:"toolCallId,omitempty"`
	HostID     string     `json:"hostId,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Status     string     `json:"status,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	DecidedAt  *time.Time `json:"decidedAt,omitempty"`
	Decision   string     `json:"decision,omitempty"`
}

// Validate checks the approval record.
func (p PendingApproval) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("approval id is required")
	}
	if p.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if p.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if p.ToolName == "" {
		return fmt.Errorf("tool name is required")
	}
	if p.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	return nil
}

// PendingEvidence represents a structured evidence gate waiting to be resumed.
type PendingEvidence struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"sessionId"`
	TurnID     string     `json:"turnId"`
	Iteration  int        `json:"iteration"`
	ToolName   string     `json:"toolName,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Status     string     `json:"status,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Decision   string     `json:"decision,omitempty"`
}

// Validate checks the evidence record.
func (p PendingEvidence) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("evidence id is required")
	}
	if p.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if p.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if p.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	return nil
}

// ExternalReference is a stable pointer to material that has been externalized.
type ExternalReference struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionId"`
	TurnID      string    `json:"turnId"`
	Iteration   int       `json:"iteration"`
	Kind        string    `json:"kind,omitempty"`
	URI         string    `json:"uri,omitempty"`
	CardRef     string    `json:"cardRef,omitempty"`
	FilePath    string    `json:"filePath,omitempty"`
	Title       string    `json:"title,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	Digest      string    `json:"digest,omitempty"`
	Bytes       int64     `json:"bytes,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Validate checks the external reference.
func (r ExternalReference) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("reference id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if r.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if r.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	return nil
}

// CompactedSegment describes a compacted region of turn history.
type CompactedSegment struct {
	ID                 string              `json:"id"`
	SessionID          string              `json:"sessionId"`
	TurnID             string              `json:"turnId"`
	Iteration          int                 `json:"iteration"`
	StartIndex         int                 `json:"startIndex"`
	EndIndex           int                 `json:"endIndex"`
	Summary            string              `json:"summary,omitempty"`
	ReferenceIDs       []string            `json:"referenceIds,omitempty"`
	ExternalReferences []ExternalReference `json:"externalReferences,omitempty"`
	Checkpoint         *CheckpointMetadata `json:"checkpoint,omitempty"`
	CreatedAt          time.Time           `json:"createdAt"`
}

// Validate checks the compacted segment.
func (s CompactedSegment) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("segment id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if s.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if s.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	if s.EndIndex >= 0 && s.StartIndex > s.EndIndex {
		return fmt.Errorf("start index must be <= end index")
	}
	if s.Checkpoint != nil {
		if err := s.Checkpoint.Validate(); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
	}
	for i := range s.ExternalReferences {
		if err := s.ExternalReferences[i].Validate(); err != nil {
			return fmt.Errorf("external reference[%d]: %w", i, err)
		}
	}
	return nil
}

// TurnSnapshot freezes the turn-level state at a stable boundary.
type TurnSnapshot struct {
	ID                    string              `json:"id"`
	SessionID             string              `json:"sessionId"`
	SessionType           SessionType         `json:"sessionType"`
	Mode                  Mode                `json:"mode"`
	Lifecycle             TurnLifecycleState  `json:"lifecycle"`
	ResumeState           TurnResumeState     `json:"resumeState"`
	Iteration             int                 `json:"iteration"`
	StartedAt             time.Time           `json:"startedAt"`
	UpdatedAt             time.Time           `json:"updatedAt"`
	CompletedAt           *time.Time          `json:"completedAt,omitempty"`
	StablePromptHash      string              `json:"stablePromptHash,omitempty"`
	StableToolFingerprint string              `json:"stableToolFingerprint,omitempty"`
	GovernanceSnapshot    string              `json:"governanceSnapshot,omitempty"`
	PromptSections        []string            `json:"promptSections,omitempty"`
	LatestCheckpoint      *CheckpointMetadata `json:"latestCheckpoint,omitempty"`
	Iterations            []IterationState    `json:"iterations,omitempty"`
	PendingApprovals      []PendingApproval   `json:"pendingApprovals,omitempty"`
	PendingEvidence       []PendingEvidence   `json:"pendingEvidence,omitempty"`
	HiddenTools           []string            `json:"hiddenTools,omitempty"`
	CompactedSegments     []CompactedSegment  `json:"compactedSegments,omitempty"`
	ExternalReferences    []ExternalReference `json:"externalReferences,omitempty"`
	FinalOutput           string              `json:"finalOutput,omitempty"`
	Error                 string              `json:"error,omitempty"`
}

// Validate checks the turn snapshot.
func (s TurnSnapshot) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("turn id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if !s.SessionType.IsValid() {
		return fmt.Errorf("invalid session type %q", s.SessionType)
	}
	if !s.Mode.IsValid() {
		return fmt.Errorf("invalid mode %q", s.Mode)
	}
	if !s.Lifecycle.IsValid() {
		return fmt.Errorf("invalid lifecycle %q", s.Lifecycle)
	}
	if !s.ResumeState.IsValid() {
		return fmt.Errorf("invalid resume state %q", s.ResumeState)
	}
	if s.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	if s.LatestCheckpoint != nil {
		if err := s.LatestCheckpoint.Validate(); err != nil {
			return fmt.Errorf("latest checkpoint: %w", err)
		}
	}
	for i := range s.Iterations {
		if err := s.Iterations[i].Validate(); err != nil {
			return fmt.Errorf("iteration[%d]: %w", i, err)
		}
	}
	for i := range s.PendingApprovals {
		if err := s.PendingApprovals[i].Validate(); err != nil {
			return fmt.Errorf("pending approval[%d]: %w", i, err)
		}
	}
	for i := range s.PendingEvidence {
		if err := s.PendingEvidence[i].Validate(); err != nil {
			return fmt.Errorf("pending evidence[%d]: %w", i, err)
		}
	}
	for i := range s.CompactedSegments {
		if err := s.CompactedSegments[i].Validate(); err != nil {
			return fmt.Errorf("compacted segment[%d]: %w", i, err)
		}
	}
	for i := range s.ExternalReferences {
		if err := s.ExternalReferences[i].Validate(); err != nil {
			return fmt.Errorf("external reference[%d]: %w", i, err)
		}
	}
	return nil
}

// IterationState captures a single model/tool iteration within a turn.
type IterationState struct {
	ID                 string               `json:"id"`
	SessionID          string               `json:"sessionId"`
	TurnID             string               `json:"turnId"`
	Iteration          int                  `json:"iteration"`
	Lifecycle          TurnLifecycleState   `json:"lifecycle"`
	ResumeState        TurnResumeState      `json:"resumeState"`
	MessagesForModel   []Message            `json:"messagesForModel,omitempty"`
	ToolCalls          []ToolCall           `json:"toolCalls,omitempty"`
	ToolProgress       []ToolProgressUpdate `json:"toolProgress,omitempty"`
	ToolResults        []ToolResult         `json:"toolResults,omitempty"`
	VisibleTools       []string             `json:"visibleTools,omitempty"`
	RefreshedTools     []string             `json:"refreshedTools,omitempty"`
	PromptDelta        string               `json:"promptDelta,omitempty"`
	TokenBudget        int                  `json:"tokenBudget,omitempty"`
	ResultBudget       int                  `json:"resultBudget,omitempty"`
	Checkpoint         *CheckpointMetadata  `json:"checkpoint,omitempty"`
	PendingApprovals   []PendingApproval    `json:"pendingApprovals,omitempty"`
	PendingEvidence    []PendingEvidence    `json:"pendingEvidence,omitempty"`
	CompactedSegments  []CompactedSegment   `json:"compactedSegments,omitempty"`
	ExternalReferences []ExternalReference  `json:"externalReferences,omitempty"`
	StartedAt          time.Time            `json:"startedAt"`
	UpdatedAt          time.Time            `json:"updatedAt"`
	CompletedAt        *time.Time           `json:"completedAt,omitempty"`
}

// Validate checks the iteration state.
func (s IterationState) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("iteration id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if s.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if s.Iteration < 0 {
		return fmt.Errorf("iteration must be >= 0")
	}
	if !s.Lifecycle.IsValid() {
		return fmt.Errorf("invalid lifecycle %q", s.Lifecycle)
	}
	if !s.ResumeState.IsValid() {
		return fmt.Errorf("invalid resume state %q", s.ResumeState)
	}
	if s.Checkpoint != nil {
		if err := s.Checkpoint.Validate(); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
	}
	for i := range s.ToolResults {
		if err := s.ToolResults[i].Validate(); err != nil {
			return fmt.Errorf("tool result[%d]: %w", i, err)
		}
	}
	for i := range s.PendingApprovals {
		if err := s.PendingApprovals[i].Validate(); err != nil {
			return fmt.Errorf("pending approval[%d]: %w", i, err)
		}
	}
	for i := range s.PendingEvidence {
		if err := s.PendingEvidence[i].Validate(); err != nil {
			return fmt.Errorf("pending evidence[%d]: %w", i, err)
		}
	}
	for i := range s.CompactedSegments {
		if err := s.CompactedSegments[i].Validate(); err != nil {
			return fmt.Errorf("compacted segment[%d]: %w", i, err)
		}
	}
	for i := range s.ExternalReferences {
		if err := s.ExternalReferences[i].Validate(); err != nil {
			return fmt.Errorf("external reference[%d]: %w", i, err)
		}
	}
	return nil
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
	SessionID   string     `json:"sessionId,omitempty"`
	TurnID      string     `json:"turnId,omitempty"`
	Type        string     `json:"type"`   // host_exec, multi_host, plan
	Status      string     `json:"status"` // pending, running, completed, failed, killed
	Description string     `json:"description"`
	HostIDs     []string   `json:"hostIds,omitempty"`
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Output      string     `json:"output,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
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
	SessionID    string            `json:"sessionId"`
	TurnID       string            `json:"turnId"`
	ApprovalID   string            `json:"approvalId,omitempty"`
	CheckpointID string            `json:"checkpointId,omitempty"`
	ResumeState  TurnResumeState   `json:"resumeState,omitempty"`
	Decision     string            `json:"decision,omitempty"` // approved, denied
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Validate checks that the resume request has required fields.
func (r ResumeRequest) Validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if r.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if !r.ResumeState.IsValid() && r.ResumeState != "" {
		return fmt.Errorf("invalid resume state %q", r.ResumeState)
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
	SessionType        SessionType       `json:"sessionType"`
	Mode               Mode              `json:"mode"`
	SessionID          string            `json:"sessionId,omitempty"`
	HostID             string            `json:"hostId,omitempty"`
	WorkspaceSessionID string            `json:"workspaceSessionId,omitempty"`
	VisibleTools       []string          `json:"visibleTools,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
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
