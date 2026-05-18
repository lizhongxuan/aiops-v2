package appui

import (
	"fmt"
	"strings"
	"time"
)

const AiopsTransportSchemaVersion = "aiops.transport.v1"

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

type AiopsTransportProcessKind string

const (
	AiopsTransportProcessKindPlan      AiopsTransportProcessKind = "plan"
	AiopsTransportProcessKindAssistant AiopsTransportProcessKind = "assistant"
	AiopsTransportProcessKindReasoning AiopsTransportProcessKind = "reasoning"
	AiopsTransportProcessKindSearch    AiopsTransportProcessKind = "search"
	AiopsTransportProcessKindCommand   AiopsTransportProcessKind = "command"
	AiopsTransportProcessKindFile      AiopsTransportProcessKind = "file"
	AiopsTransportProcessKindTool      AiopsTransportProcessKind = "tool"
	AiopsTransportProcessKindEvidence  AiopsTransportProcessKind = "evidence"
	AiopsTransportProcessKindApproval  AiopsTransportProcessKind = "approval"
	AiopsTransportProcessKindMCP       AiopsTransportProcessKind = "mcp"
	AiopsTransportProcessKindSystem    AiopsTransportProcessKind = "system"
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

type AiopsTransportFinalStatus string

const (
	AiopsTransportFinalStatusRunning   AiopsTransportFinalStatus = "running"
	AiopsTransportFinalStatusCompleted AiopsTransportFinalStatus = "completed"
	AiopsTransportFinalStatusFailed    AiopsTransportFinalStatus = "failed"
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
	ID               string                          `json:"id"`
	User             *AiopsTransportMessage          `json:"user,omitempty"`
	Intent           *AiopsTransportIntent           `json:"intent,omitempty"`
	Process          []AiopsProcessBlock             `json:"process,omitempty"`
	AgentUIArtifacts []AiopsTransportAgentUIArtifact `json:"agentUiArtifacts,omitempty"`
	Final            *AiopsTransportFinal            `json:"final,omitempty"`
	Status           AiopsTransportTurnStatus        `json:"status"`
	StartedAt        string                          `json:"startedAt,omitempty"`
	CompletedAt      string                          `json:"completedAt,omitempty"`
	UpdatedAt        string                          `json:"updatedAt,omitempty"`
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

type AiopsTransportFinal struct {
	ID     string                    `json:"id"`
	Text   string                    `json:"text"`
	Status AiopsTransportFinalStatus `json:"status"`
}

type AiopsProcessBlock struct {
	ID            string                      `json:"id"`
	Kind          AiopsTransportProcessKind   `json:"kind"`
	DisplayKind   string                      `json:"displayKind,omitempty"`
	Status        AiopsTransportProcessStatus `json:"status"`
	Text          string                      `json:"text"`
	Command       string                      `json:"command,omitempty"`
	InputSummary  string                      `json:"inputSummary,omitempty"`
	OutputPreview string                      `json:"outputPreview,omitempty"`
	Steps         []AiopsTransportPlanStep    `json:"steps,omitempty"`
	Queries       []string                    `json:"queries,omitempty"`
	Results       []AiopsSearchResult         `json:"results,omitempty"`
	ApprovalID    string                      `json:"approvalId,omitempty"`
	Source        string                      `json:"source,omitempty"`
	Confidence    string                      `json:"confidence,omitempty"`
	Window        string                      `json:"window,omitempty"`
	RawRef        string                      `json:"rawRef,omitempty"`
	EvidenceRefs  []string                    `json:"evidenceRefs,omitempty"`
	Mock          bool                        `json:"mock,omitempty"`
	ExitCode      *int                        `json:"exitCode,omitempty"`
	DurationMs    int64                       `json:"durationMs,omitempty"`
	UpdatedAt     string                      `json:"updatedAt,omitempty"`
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
	ID        string `json:"id"`
	Kind      string `json:"kind,omitempty"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	Pinned    bool   `json:"pinned,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type AiopsTransportArtifact struct {
	ID         string `json:"id"`
	TurnID     string `json:"turnId,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Title      string `json:"title,omitempty"`
	Preview    string `json:"preview,omitempty"`
	RawRef     string `json:"rawRef,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type AiopsTransportAgentUIArtifact struct {
	ID              string           `json:"id"`
	Type            string           `json:"type"`
	Title           string           `json:"title,omitempty"`
	TitleZh         string           `json:"titleZh,omitempty"`
	Summary         string           `json:"summary,omitempty"`
	SummaryZh       string           `json:"summaryZh,omitempty"`
	Status          string           `json:"status,omitempty"`
	Severity        string           `json:"severity,omitempty"`
	DataRef         string           `json:"dataRef,omitempty"`
	InlineData      map[string]any   `json:"inlineData,omitempty"`
	Payload         map[string]any   `json:"payload,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	Actions         []map[string]any `json:"actions,omitempty"`
	Source          string           `json:"source,omitempty"`
	PermissionScope string           `json:"permissionScope,omitempty"`
	RedactionStatus string           `json:"redactionStatus,omitempty"`
	CreatedAt       string           `json:"createdAt,omitempty"`
	UpdatedAt       string           `json:"updatedAt,omitempty"`
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
