package appui

import (
	"fmt"
	"strings"
	"time"
)

const AiopsTransportSchemaVersion = "aiops.transport.v2"

type AiopsTransportStatus string

const (
	AiopsTransportStatusIdle     AiopsTransportStatus = "idle"
	AiopsTransportStatusWorking  AiopsTransportStatus = "working"
	AiopsTransportStatusBlocked  AiopsTransportStatus = "blocked"
	AiopsTransportStatusFailed   AiopsTransportStatus = "failed"
	AiopsTransportStatusCanceled AiopsTransportStatus = "canceled"
)

type AiopsTransportTurnStatus string

const (
	AiopsTransportTurnStatusSubmitted AiopsTransportTurnStatus = "submitted"
	AiopsTransportTurnStatusWorking   AiopsTransportTurnStatus = "working"
	AiopsTransportTurnStatusBlocked   AiopsTransportTurnStatus = "blocked"
	AiopsTransportTurnStatusCompleted AiopsTransportTurnStatus = "completed"
	AiopsTransportTurnStatusFailed    AiopsTransportTurnStatus = "failed"
	AiopsTransportTurnStatusCanceled  AiopsTransportTurnStatus = "canceled"
)

type AiopsTransportProcessStatus string

const (
	AiopsTransportProcessStatusQueued    AiopsTransportProcessStatus = "queued"
	AiopsTransportProcessStatusRunning   AiopsTransportProcessStatus = "running"
	AiopsTransportProcessStatusCompleted AiopsTransportProcessStatus = "completed"
	AiopsTransportProcessStatusFailed    AiopsTransportProcessStatus = "failed"
	AiopsTransportProcessStatusBlocked   AiopsTransportProcessStatus = "blocked"
	AiopsTransportProcessStatusRejected  AiopsTransportProcessStatus = "rejected"
)

type AiopsTransportState struct {
	SchemaVersion    string                              `json:"schemaVersion"`
	SessionID        string                              `json:"sessionId"`
	ThreadID         string                              `json:"threadId"`
	Status           AiopsTransportStatus                `json:"status"`
	CurrentTurnID    string                              `json:"currentTurnId,omitempty"`
	Turns            map[string]AiopsTransportTurn       `json:"turns"`
	TurnOrder        []string                            `json:"turnOrder"`
	PendingApprovals map[string]AiopsTransportApproval   `json:"pendingApprovals"`
	McpSurfaces      map[string]AiopsTransportMcpSurface `json:"mcpSurfaces"`
	Artifacts        map[string]AiopsTransportArtifact   `json:"artifacts"`
	RuntimeLiveness  AiopsRuntimeLiveness                `json:"runtimeLiveness"`
	LastError        string                              `json:"lastError,omitempty"`
	Seq              int64                               `json:"seq"`
	UpdatedAt        string                              `json:"updatedAt"`
}

type AiopsTransportTurn struct {
	ID          string                          `json:"id"`
	User        *AiopsTransportMessage          `json:"user,omitempty"`
	Intent      *AiopsTransportIntent           `json:"intent,omitempty"`
	BlockOrder  []string                        `json:"blockOrder"`
	BlocksByID  map[string]AiopsTranscriptBlock `json:"blocksById"`
	Status      AiopsTransportTurnStatus        `json:"status"`
	StartedAt   string                          `json:"startedAt,omitempty"`
	CompletedAt string                          `json:"completedAt,omitempty"`
	UpdatedAt   string                          `json:"updatedAt,omitempty"`
}

type AiopsTransportMessage struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type AiopsTransportIntent struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

type AiopsTransportPlanStep struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Status  string `json:"status,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type AiopsSearchResult struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type AiopsTranscriptBlockType string

const (
	AiopsTranscriptBlockTypeText      AiopsTranscriptBlockType = "text"
	AiopsTranscriptBlockTypeTool      AiopsTranscriptBlockType = "tool"
	AiopsTranscriptBlockTypeAggregate AiopsTranscriptBlockType = "aggregate"
	AiopsTranscriptBlockTypeApproval  AiopsTranscriptBlockType = "approval"
	AiopsTranscriptBlockTypeThinking  AiopsTranscriptBlockType = "thinking"
	AiopsTranscriptBlockTypeArtifact  AiopsTranscriptBlockType = "artifact"
)

type AiopsTranscriptBlock struct {
	ID        string                   `json:"id"`
	Type      AiopsTranscriptBlockType `json:"type"`
	Text      *AiopsTextBlock          `json:"text,omitempty"`
	Tool      *AiopsToolBlock          `json:"tool,omitempty"`
	Aggregate *AiopsAggregateBlock     `json:"aggregate,omitempty"`
	Approval  *AiopsApprovalBlock      `json:"approval,omitempty"`
	Thinking  *AiopsThinkingBlock      `json:"thinking,omitempty"`
	Artifact  *AiopsArtifactBlock      `json:"artifact,omitempty"`
	CreatedAt string                   `json:"createdAt,omitempty"`
	UpdatedAt string                   `json:"updatedAt,omitempty"`
}

type AiopsTranscriptTextStatus string

const (
	AiopsTranscriptTextStatusStreaming AiopsTranscriptTextStatus = "streaming"
	AiopsTranscriptTextStatusCompleted AiopsTranscriptTextStatus = "completed"
)

type AiopsTextBlock struct {
	Role   string                    `json:"role"`
	Text   string                    `json:"text"`
	Status AiopsTranscriptTextStatus `json:"status"`
}

type AiopsTranscriptToolKind string

const (
	AiopsTranscriptToolKindCommand AiopsTranscriptToolKind = "command"
	AiopsTranscriptToolKindSearch  AiopsTranscriptToolKind = "search"
	AiopsTranscriptToolKindFile    AiopsTranscriptToolKind = "file"
	AiopsTranscriptToolKindMCP     AiopsTranscriptToolKind = "mcp"
	AiopsTranscriptToolKindBrowser AiopsTranscriptToolKind = "browser"
	AiopsTranscriptToolKindList    AiopsTranscriptToolKind = "list"
	AiopsTranscriptToolKindOther   AiopsTranscriptToolKind = "other"
)

type AiopsToolOutput struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	RawRef    string `json:"rawRef,omitempty"`
}

type AiopsToolBlock struct {
	ToolKind     AiopsTranscriptToolKind     `json:"toolKind"`
	ToolName     string                      `json:"toolName,omitempty"`
	Title        string                      `json:"title"`
	Summary      string                      `json:"summary"`
	Status       AiopsTransportProcessStatus `json:"status"`
	Command      string                      `json:"command,omitempty"`
	InputSummary string                      `json:"inputSummary,omitempty"`
	Output       AiopsToolOutput             `json:"output"`
	ExitCode     *int                        `json:"exitCode,omitempty"`
	DurationMs   int64                       `json:"durationMs,omitempty"`
	StartedAt    string                      `json:"startedAt,omitempty"`
	CompletedAt  string                      `json:"completedAt,omitempty"`
	ApprovalID   string                      `json:"approvalId,omitempty"`
}

type AiopsAggregateCounts struct {
	Command  int `json:"command,omitempty"`
	Search   int `json:"search,omitempty"`
	FileRead int `json:"fileRead,omitempty"`
	FileEdit int `json:"fileEdit,omitempty"`
	List     int `json:"list,omitempty"`
	MCP      int `json:"mcp,omitempty"`
	Browser  int `json:"browser,omitempty"`
	Other    int `json:"other,omitempty"`
}

type AiopsAggregateBlock struct {
	Summary       string               `json:"summary"`
	Status        string               `json:"status"`
	ChildBlockIDs []string             `json:"childBlockIds"`
	Counts        AiopsAggregateCounts `json:"counts"`
}

type AiopsApprovalBlock struct {
	ApprovalID   string `json:"approvalId"`
	ApprovalKind string `json:"approvalKind"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Command      string `json:"command,omitempty"`
	Status       string `json:"status"`
	RequestedAt  string `json:"requestedAt"`
	ResolvedAt   string `json:"resolvedAt,omitempty"`
}

type AiopsThinkingBlock struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

type AiopsArtifactBlock struct {
	ArtifactID string `json:"artifactId"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
}

type AiopsTransportLifecycleState string

const (
	AiopsTransportLifecycleCreated  AiopsTransportLifecycleState = "created"
	AiopsTransportLifecycleLoading  AiopsTransportLifecycleState = "loading"
	AiopsTransportLifecycleReady    AiopsTransportLifecycleState = "ready"
	AiopsTransportLifecycleFailed   AiopsTransportLifecycleState = "failed"
	AiopsTransportLifecycleDisposed AiopsTransportLifecycleState = "disposed"
)

type AiopsTransportApproval struct {
	ID          string `json:"id"`
	TurnID      string `json:"turnId,omitempty"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status,omitempty"`
	Command     string `json:"command,omitempty"`
	Reason      string `json:"reason,omitempty"`
	RequestedAt string `json:"requestedAt,omitempty"`
	ResolvedAt  string `json:"resolvedAt,omitempty"`
}

type AiopsTransportMcpSurface struct {
	ID          string                        `json:"id"`
	Kind        string                        `json:"kind,omitempty"`
	Title       string                        `json:"title,omitempty"`
	Status      string                        `json:"status,omitempty"`
	Lifecycle   AiopsTransportLifecycleState  `json:"lifecycle,omitempty"`
	Pinned      bool                          `json:"pinned,omitempty"`
	Cards       []AiopsAgentUICard            `json:"cards,omitempty"`
	App         *AiopsIframeAppSurface        `json:"app,omitempty"`
	Actions     []AiopsTransportActionBinding `json:"actions,omitempty"`
	ArtifactIDs []string                      `json:"artifactIds,omitempty"`
	UpdatedAt   string                        `json:"updatedAt,omitempty"`
}

type AiopsTransportArtifact struct {
	ID          string                        `json:"id"`
	TurnID      string                        `json:"turnId,omitempty"`
	Kind        string                        `json:"kind,omitempty"`
	Title       string                        `json:"title,omitempty"`
	Preview     string                        `json:"preview,omitempty"`
	PreviewData *AiopsArtifactPreview         `json:"previewData,omitempty"`
	RawRef      string                        `json:"rawRef,omitempty"`
	Lifecycle   AiopsTransportLifecycleState  `json:"lifecycle,omitempty"`
	Actions     []AiopsTransportActionBinding `json:"actions,omitempty"`
	CreatedAt   string                        `json:"createdAt,omitempty"`
	ModifiedAt  string                        `json:"modifiedAt,omitempty"`
}

type AiopsAgentUICard struct {
	ID         string                        `json:"id"`
	Kind       string                        `json:"kind,omitempty"`
	Title      string                        `json:"title,omitempty"`
	Summary    string                        `json:"summary,omitempty"`
	Status     string                        `json:"status,omitempty"`
	ArtifactID string                        `json:"artifactId,omitempty"`
	SurfaceID  string                        `json:"surfaceId,omitempty"`
	Actions    []AiopsTransportActionBinding `json:"actions,omitempty"`
}

type AiopsArtifactPreview struct {
	ContentType string            `json:"contentType,omitempty"`
	Text        string            `json:"text,omitempty"`
	URL         string            `json:"url,omitempty"`
	RawRef      string            `json:"rawRef,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type AiopsIframeAppSurface struct {
	URL         string   `json:"url,omitempty"`
	Sandbox     string   `json:"sandbox,omitempty"`
	Height      int      `json:"height,omitempty"`
	Width       int      `json:"width,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

type AiopsTransportActionBinding struct {
	ID               string         `json:"id"`
	Label            string         `json:"label,omitempty"`
	Command          string         `json:"command,omitempty"`
	Target           string         `json:"target,omitempty"`
	Params           map[string]any `json:"params,omitempty"`
	RequiresApproval bool           `json:"requiresApproval,omitempty"`
}

type AiopsRuntimeLiveness struct {
	ActiveTurns          map[string]bool `json:"activeTurns"`
	ActiveAgents         map[string]bool `json:"activeAgents"`
	PendingApprovals     map[string]bool `json:"pendingApprovals"`
	PendingUserInputs    map[string]bool `json:"pendingUserInputs"`
	ActiveCommandStreams map[string]bool `json:"activeCommandStreams"`
}

func NewAiopsTransportState(sessionID, threadID string) AiopsTransportState {
	return AiopsTransportState{
		SchemaVersion:    AiopsTransportSchemaVersion,
		SessionID:        strings.TrimSpace(sessionID),
		ThreadID:         strings.TrimSpace(threadID),
		Status:           AiopsTransportStatusIdle,
		Turns:            map[string]AiopsTransportTurn{},
		TurnOrder:        []string{},
		PendingApprovals: map[string]AiopsTransportApproval{},
		McpSurfaces:      map[string]AiopsTransportMcpSurface{},
		Artifacts:        map[string]AiopsTransportArtifact{},
		RuntimeLiveness: AiopsRuntimeLiveness{
			ActiveTurns:          map[string]bool{},
			ActiveAgents:         map[string]bool{},
			PendingApprovals:     map[string]bool{},
			PendingUserInputs:    map[string]bool{},
			ActiveCommandStreams: map[string]bool{},
		},
		Seq:       0,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func TransportTurnStableID(threadID, turnID string) string {
	return stableTransportID("turn", threadID, turnID)
}

func TransportProcessBlockStableID(turnID, kind, sourceID string) string {
	return stableTransportID("block", turnID, kind, sourceID)
}

func EnsureAiopsTransportTurnBlocks(turn AiopsTransportTurn) AiopsTransportTurn {
	if turn.BlockOrder == nil {
		turn.BlockOrder = []string{}
	}
	if turn.BlocksByID == nil {
		turn.BlocksByID = map[string]AiopsTranscriptBlock{}
	}
	return turn
}

func UpsertAiopsTranscriptBlock(turn AiopsTransportTurn, block AiopsTranscriptBlock) AiopsTransportTurn {
	turn = EnsureAiopsTransportTurnBlocks(turn)
	block.ID = strings.TrimSpace(block.ID)
	if block.ID == "" {
		return turn
	}
	turn.BlocksByID[block.ID] = block
	for _, existing := range turn.BlockOrder {
		if existing == block.ID {
			return turn
		}
	}
	turn.BlockOrder = append(turn.BlockOrder, block.ID)
	return turn
}

func ReplaceVisibleBlocksWithAggregate(turn AiopsTransportTurn, childIDs []string, aggregate AiopsTranscriptBlock) AiopsTransportTurn {
	turn = EnsureAiopsTransportTurnBlocks(turn)
	aggregate.ID = strings.TrimSpace(aggregate.ID)
	if aggregate.ID == "" || len(childIDs) == 0 {
		return turn
	}
	childSet := map[string]bool{}
	for _, id := range childIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			childSet[trimmed] = true
		}
	}
	if len(childSet) == 0 {
		return turn
	}
	nextOrder := make([]string, 0, len(turn.BlockOrder)-len(childSet)+1)
	inserted := false
	for _, id := range turn.BlockOrder {
		if !childSet[id] {
			nextOrder = append(nextOrder, id)
			continue
		}
		if !inserted {
			nextOrder = append(nextOrder, aggregate.ID)
			inserted = true
		}
	}
	if !inserted {
		nextOrder = append(nextOrder, aggregate.ID)
	}
	turn.BlockOrder = nextOrder
	turn.BlocksByID[aggregate.ID] = aggregate
	return turn
}

func stableTransportID(prefix string, parts ...string) string {
	cleaned := make([]string, 0, len(parts)+1)
	if p := transportStablePart(prefix); p != "" {
		cleaned = append(cleaned, p)
	}
	for _, part := range parts {
		if p := transportStablePart(part); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return "transport:unknown"
	}
	return fmt.Sprintf("%s", strings.Join(cleaned, ":"))
}

func transportStablePart(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "_", "\\", "_")
	return replacer.Replace(trimmed)
}
