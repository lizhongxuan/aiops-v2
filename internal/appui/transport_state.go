package appui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/specialinputmemory"
)

const AiopsTransportSchemaVersion = "aiops.transport.v2"

const AiopsTransportAgentItemSchemaVersion = "aiops.transport.agent-item.v1"

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
	AiopsTransportProcessKindSubagent  AiopsTransportProcessKind = "subagent"
)

type AiopsTransportProcessStatus string

const (
	AiopsTransportProcessStatusQueued    AiopsTransportProcessStatus = "queued"
	AiopsTransportProcessStatusRunning   AiopsTransportProcessStatus = "running"
	AiopsTransportProcessStatusCompleted AiopsTransportProcessStatus = "completed"
	AiopsTransportProcessStatusFailed    AiopsTransportProcessStatus = "failed"
	AiopsTransportProcessStatusBlocked   AiopsTransportProcessStatus = "blocked"
	AiopsTransportProcessStatusRejected  AiopsTransportProcessStatus = "rejected"
	AiopsTransportProcessStatusSkipped   AiopsTransportProcessStatus = "skipped"
)

type AiopsTransportFinalStatus string

const (
	AiopsTransportFinalStatusRunning         AiopsTransportFinalStatus = "running"
	AiopsTransportFinalStatusCompleted       AiopsTransportFinalStatus = "completed"
	AiopsTransportFinalStatusFailed          AiopsTransportFinalStatus = "failed"
	AiopsTransportFinalStatusVerified        AiopsTransportFinalStatus = "verified"
	AiopsTransportFinalStatusPartial         AiopsTransportFinalStatus = "partial"
	AiopsTransportFinalStatusBlocked         AiopsTransportFinalStatus = "blocked"
	AiopsTransportFinalStatusNeedsEvidence   AiopsTransportFinalStatus = "needs_evidence"
	AiopsTransportFinalStatusApprovalDenied  AiopsTransportFinalStatus = "approval_denied"
	AiopsTransportFinalStatusToolUnavailable AiopsTransportFinalStatus = "tool_unavailable"
	AiopsTransportFinalStatusCancelled       AiopsTransportFinalStatus = "cancelled"
	AiopsTransportFinalStatusUnknown         AiopsTransportFinalStatus = "unknown"
)

type AiopsTransportBlockType string

const (
	AiopsTransportBlockTypeCommentary  AiopsTransportBlockType = "commentary"
	AiopsTransportBlockTypeFinalAnswer AiopsTransportBlockType = "final_answer"
	AiopsTransportBlockTypeArtifact    AiopsTransportBlockType = "artifact"
)

type AiopsTransportState struct {
	SchemaVersion       string                               `json:"schemaVersion"`
	SessionID           string                               `json:"sessionId"`
	ThreadID            string                               `json:"threadId"`
	Status              AiopsTransportStatus                 `json:"status"`
	CurrentTurnID       string                               `json:"currentTurnId,omitempty"`
	OpsRun              *AiopsTransportOpsRun                `json:"opsRun,omitempty"`
	Turns               map[string]AiopsTransportTurn        `json:"turns"`
	TurnOrder           []string                             `json:"turnOrder"`
	PendingApprovals    map[string]AiopsTransportApproval    `json:"pendingApprovals"`
	McpSurfaces         map[string]AiopsTransportMcpSurface  `json:"mcpSurfaces"`
	Artifacts           map[string]AiopsTransportArtifact    `json:"artifacts"`
	RuntimeLiveness     AiopsRuntimeLiveness                 `json:"runtimeLiveness"`
	HostMissions        map[string]AiopsTransportHostMission `json:"hostMissions,omitempty"`
	ChildAgents         map[string]AiopsTransportChildAgent  `json:"childAgents,omitempty"`
	SpecialInputContext *specialinputmemory.TransportContext `json:"specialInputContext,omitempty"`
	ActiveHostMissionID string                               `json:"activeHostMissionId,omitempty"`
	LastError           string                               `json:"lastError,omitempty"`
	Seq                 int64                                `json:"seq"`
	UpdatedAt           string                               `json:"updatedAt"`
}

type AiopsTransportOpsRun struct {
	ID                 string              `json:"id"`
	SessionID          string              `json:"sessionId,omitempty"`
	TurnID             string              `json:"turnId,omitempty"`
	ClientTurnID       string              `json:"clientTurnId,omitempty"`
	Source             string              `json:"source"`
	Status             string              `json:"status"`
	Title              string              `json:"title,omitempty"`
	RouteMode          string              `json:"routeMode,omitempty"`
	TargetSummary      string              `json:"targetSummary,omitempty"`
	ToolSurfaceSummary string              `json:"toolSurfaceSummary,omitempty"`
	EvidenceCount      int                 `json:"evidenceCount,omitempty"`
	CurrentStep        string              `json:"currentStep,omitempty"`
	CurrentStepID      string              `json:"currentStepId,omitempty"`
	CheckpointID       string              `json:"checkpointId,omitempty"`
	AgentRun           *AgentRunView       `json:"agentRun,omitempty"`
	PostRunSuggestions []PostRunSuggestion `json:"postRunSuggestions,omitempty"`
}

type AiopsTransportTurn struct {
	ID                      string                    `json:"id"`
	ClientTurnID            string                    `json:"clientTurnId,omitempty"`
	ClientMessageID         string                    `json:"clientMessageId,omitempty"`
	AgentItems              []AiopsTransportAgentItem `json:"agentItems,omitempty"`
	AgentItemsTruncated     bool                      `json:"agentItemsTruncated,omitempty"`
	AgentItemsOriginalCount int                       `json:"agentItemsOriginalCount,omitempty"`
	AgentItemsOriginalBytes int64                     `json:"agentItemsOriginalBytes,omitempty"`
	AgentItemsHash          string                    `json:"agentItemsHash,omitempty"`
	AgentItemsRef           string                    `json:"agentItemsRef,omitempty"`
	User                    *AiopsTransportMessage    `json:"user,omitempty"`
	Intent                  *AiopsTransportIntent     `json:"intent,omitempty"`
	// Process and Final are internal projection work fields. The wire transcript
	// is exclusively BlockOrder + BlocksByID.
	Process           []AiopsProcessBlock             `json:"-"`
	Timeline          []AiopsTransportTimelineItem    `json:"timeline,omitempty"`
	ContextGovernance []AiopsContextGovernanceEvent   `json:"contextGovernance,omitempty"`
	AgentUIArtifacts  []AiopsTransportAgentUIArtifact `json:"agentUiArtifacts,omitempty"`
	Final             *AiopsTransportFinal            `json:"-"`
	BlockOrder        []string                        `json:"blockOrder"`
	BlocksByID        map[string]AiopsTransportBlock  `json:"blocksById"`
	Status            AiopsTransportTurnStatus        `json:"status"`
	StartedAt         string                          `json:"startedAt,omitempty"`
	CompletedAt       string                          `json:"completedAt,omitempty"`
	UpdatedAt         string                          `json:"updatedAt,omitempty"`
}

// AiopsTransportBlock is the canonical ordered transcript unit. Process
// details are flattened for typed renderers; final/artifact payloads remain
// typed and never need to be inferred from assistant text.
type AiopsTransportBlock struct {
	Type AiopsTransportBlockType `json:"type"`
	AiopsProcessBlock
	FinalContract *AiopsTransportFinal           `json:"finalContract,omitempty"`
	Artifact      *AiopsTransportAgentUIArtifact `json:"artifact,omitempty"`
}

// AiopsTransportAgentItem is the versioned, privacy-bounded wire form of a
// canonical runtime TurnItem. It deliberately does not expose agentstate's
// open-ended JSON schema directly.
type AiopsTransportAgentItem struct {
	SchemaVersion string                         `json:"schemaVersion"`
	ID            string                         `json:"id"`
	Type          string                         `json:"type"`
	Status        string                         `json:"status"`
	Payload       AiopsTransportAgentItemPayload `json:"payload,omitempty"`
	CreatedAt     string                         `json:"createdAt,omitempty"`
	UpdatedAt     string                         `json:"updatedAt,omitempty"`
	Truncated     bool                           `json:"truncated,omitempty"`
	OriginalBytes int64                          `json:"originalBytes,omitempty"`
	ContentHash   string                         `json:"contentHash,omitempty"`
	Ref           string                         `json:"ref,omitempty"`
}

type AiopsTransportAgentItemPayload struct {
	Kind    string          `json:"kind,omitempty"`
	Summary string          `json:"summary,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type AiopsTransportTimelineItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status,omitempty"`
	Text        string `json:"text,omitempty"`
	PayloadKind string `json:"payloadKind,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
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
	ID                    string                           `json:"id"`
	Text                  string                           `json:"text"`
	Status                AiopsTransportFinalStatus        `json:"status"`
	SchemaVersion         string                           `json:"schemaVersion,omitempty"`
	Confidence            string                           `json:"confidence,omitempty"`
	AnswerText            string                           `json:"answerText,omitempty"`
	CheckedEvidenceRefs   []string                         `json:"checkedEvidenceRefs,omitempty"`
	UncheckedRequirements []string                         `json:"uncheckedRequirements,omitempty"`
	FailedToolImpacts     []AiopsTransportFailedToolImpact `json:"failedToolImpacts,omitempty"`
	ApprovedActions       []string                         `json:"approvedActions,omitempty"`
	PerformedActions      []string                         `json:"performedActions,omitempty"`
	PostChecks            []string                         `json:"postChecks,omitempty"`
	RequiredPostChecks    []string                         `json:"requiredPostChecks,omitempty"`
	Limitations           []string                         `json:"limitations,omitempty"`
	DurationMs            int64                            `json:"durationMs,omitempty"`
}

type AiopsTransportFailedToolImpact struct {
	ToolName     string `json:"toolName,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	FailureClass string `json:"failureClass,omitempty"`
	Impact       string `json:"impact,omitempty"`
}

type AiopsProcessBlock struct {
	ID                  string                      `json:"id"`
	Kind                AiopsTransportProcessKind   `json:"kind"`
	DisplayKind         string                      `json:"displayKind,omitempty"`
	Phase               string                      `json:"phase,omitempty"`
	StreamState         string                      `json:"streamState,omitempty"`
	CommentarySource    string                      `json:"commentarySource,omitempty"`
	Iteration           *int                        `json:"iteration,omitempty"`
	ToolCallIDs         []string                    `json:"toolCallIds,omitempty"`
	EvidenceBoundary    string                      `json:"evidenceBoundary,omitempty"`
	Status              AiopsTransportProcessStatus `json:"status"`
	Text                string                      `json:"text"`
	Command             string                      `json:"command,omitempty"`
	InputSummary        string                      `json:"inputSummary,omitempty"`
	OutputPreview       string                      `json:"outputPreview,omitempty"`
	FoldGroupID         string                      `json:"foldGroupId,omitempty"`
	FoldGroupKind       string                      `json:"foldGroupKind,omitempty"`
	Steps               []AiopsTransportPlanStep    `json:"steps,omitempty"`
	Queries             []string                    `json:"queries,omitempty"`
	Results             []AiopsSearchResult         `json:"results,omitempty"`
	Operation           string                      `json:"operation,omitempty"`
	URL                 string                      `json:"url,omitempty"`
	Adapter             string                      `json:"adapter,omitempty"`
	Backend             string                      `json:"backend,omitempty"`
	SourceCount         int                         `json:"sourceCount,omitempty"`
	ToolCallID          string                      `json:"toolCallId,omitempty"`
	CheckpointID        string                      `json:"checkpointId,omitempty"`
	ApprovalID          string                      `json:"approvalId,omitempty"`
	Source              string                      `json:"source,omitempty"`
	TargetSummary       string                      `json:"targetSummary,omitempty"`
	Risk                string                      `json:"risk,omitempty"`
	RiskSummary         string                      `json:"riskSummary,omitempty"`
	ExpectedEffect      string                      `json:"expectedEffect,omitempty"`
	Rollback            string                      `json:"rollback,omitempty"`
	Validation          string                      `json:"validation,omitempty"`
	Confidence          string                      `json:"confidence,omitempty"`
	Window              string                      `json:"window,omitempty"`
	RawRef              string                      `json:"rawRef,omitempty"`
	EvidenceRefs        []string                    `json:"evidenceRefs,omitempty"`
	Mock                bool                        `json:"mock,omitempty"`
	ExitCode            *int                        `json:"exitCode,omitempty"`
	DurationMs          int64                       `json:"durationMs,omitempty"`
	MaterializationTier string                      `json:"materializationTier,omitempty"`
	OriginalBytes       int64                       `json:"originalBytes,omitempty"`
	InlineBytes         int64                       `json:"inlineBytes,omitempty"`
	ExternalReferences  []AiopsExternalReference    `json:"externalReferences,omitempty"`
	UpdatedAt           string                      `json:"updatedAt,omitempty"`
}

type AiopsContextGovernanceEvent struct {
	ID              string         `json:"id,omitempty"`
	Layer           string         `json:"layer"`
	Kind            string         `json:"kind"`
	Message         string         `json:"message,omitempty"`
	Budget          map[string]any `json:"budget,omitempty"`
	ReferenceIDs    []string       `json:"referenceIds,omitempty"`
	CompactedIDs    []string       `json:"compactedIds,omitempty"`
	DroppedGroupIDs []string       `json:"droppedGroupIds,omitempty"`
	RetryAttempt    int            `json:"retryAttempt,omitempty"`
	RetryMax        int            `json:"retryMax,omitempty"`
	Timeout         bool           `json:"timeout,omitempty"`
	CreatedAt       string         `json:"createdAt,omitempty"`
}

type AiopsExternalReference struct {
	ID          string `json:"id"`
	Kind        string `json:"kind,omitempty"`
	URI         string `json:"uri,omitempty"`
	CardRef     string `json:"cardRef,omitempty"`
	FilePath    string `json:"filePath,omitempty"`
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Digest      string `json:"digest,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
}

type AiopsTransportPlanStep struct {
	ID               string   `json:"id"`
	Index            int      `json:"index,omitempty"`
	Text             string   `json:"text"`
	Status           string   `json:"status,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	Title            string   `json:"title,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	HostIDs          []string `json:"hostIds,omitempty"`
	ChildAgentIDs    []string `json:"childAgentIds,omitempty"`
	ApprovalRequired bool     `json:"approvalRequired,omitempty"`
}

type AiopsSearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	Text        string `json:"text,omitempty"`
	Fetched     bool   `json:"fetched,omitempty"`
	FetchError  string `json:"fetchError,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

type AiopsTransportApproval struct {
	ID             string `json:"id"`
	TurnID         string `json:"turnId,omitempty"`
	Type           string `json:"type,omitempty"`
	Status         string `json:"status,omitempty"`
	Command        string `json:"command,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Source         string `json:"source,omitempty"`
	TargetSummary  string `json:"targetSummary,omitempty"`
	ExpectedEffect string `json:"expectedEffect,omitempty"`
	Rollback       string `json:"rollback,omitempty"`
	Validation     string `json:"validation,omitempty"`
	RequestedAt    string `json:"requestedAt,omitempty"`
	ResolvedAt     string `json:"resolvedAt,omitempty"`
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

type AiopsTransportHostMission struct {
	ID                 string                      `json:"id"`
	TurnID             string                      `json:"turnId"`
	Status             string                      `json:"status"`
	PlanRequired       bool                        `json:"planRequired"`
	PlanAccepted       bool                        `json:"planAccepted"`
	MentionedHosts     []AiopsTransportHostMention `json:"mentionedHosts"`
	ChildAgentIDs      []string                    `json:"childAgentIds"`
	PlanSteps          []AiopsTransportPlanStep    `json:"planSteps,omitempty"`
	ManagerAgentID     string                      `json:"managerAgentId,omitempty"`
	ActiveChildAgentID string                      `json:"activeChildAgentId,omitempty"`
	CreatedAt          string                      `json:"createdAt,omitempty"`
	UpdatedAt          string                      `json:"updatedAt,omitempty"`
}

type AiopsTransportHostMention struct {
	TokenID     string `json:"tokenId"`
	Raw         string `json:"raw"`
	HostID      string `json:"hostId,omitempty"`
	Address     string `json:"address,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Source      string `json:"source"`
	Resolved    bool   `json:"resolved"`
}

type AiopsTransportChildAgent struct {
	ID                string   `json:"id"`
	MissionID         string   `json:"missionId"`
	ParentAgentID     string   `json:"parentAgentId,omitempty"`
	SessionID         string   `json:"sessionId"`
	HostID            string   `json:"hostId"`
	HostAddress       string   `json:"hostAddress,omitempty"`
	HostDisplayName   string   `json:"hostDisplayName"`
	Role              string   `json:"role,omitempty"`
	Task              string   `json:"task,omitempty"`
	CurrentStepTitle  string   `json:"currentStepTitle,omitempty"`
	Status            string   `json:"status"`
	PlanStepIDs       []string `json:"planStepIds,omitempty"`
	LastInputPreview  string   `json:"lastInputPreview,omitempty"`
	LastOutputPreview string   `json:"lastOutputPreview,omitempty"`
	Error             string   `json:"error,omitempty"`
	StartedAt         string   `json:"startedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	CompletedAt       string   `json:"completedAt,omitempty"`
	RuntimeProfile    string   `json:"runtimeProfile,omitempty"`
	ActiveSubtaskID   string   `json:"activeSubtaskId,omitempty"`
	QueueReason       string   `json:"queueReason,omitempty"`
	TraceSummary      string   `json:"traceSummary,omitempty"`
	AgentMessageRefs  []string `json:"agentMessageRefs,omitempty"`

	PromptSections       []AiopsTransportPromptSectionTrace  `json:"promptSections,omitempty"`
	ContextDecisions     []AiopsTransportContextDecision     `json:"contextDecisions,omitempty"`
	ToolSurface          []AiopsTransportToolSurfaceEntry    `json:"toolSurface,omitempty"`
	McpInstructionDeltas []AiopsTransportScopedTraceEntry    `json:"mcpInstructionDeltas,omitempty"`
	SkillActivationTrace []AiopsTransportScopedTraceEntry    `json:"skillActivationTrace,omitempty"`
	ApprovalTrace        []AiopsTransportScopedTraceEntry    `json:"approvalTrace,omitempty"`
	EvidenceTrace        []AiopsTransportEvidenceTrace       `json:"evidenceTrace,omitempty"`
	ReportTimeline       []AiopsTransportHostTaskReportTrace `json:"reportTimeline,omitempty"`
}

type AiopsTransportPromptSectionTrace struct {
	ID             string `json:"id"`
	Kind           string `json:"kind,omitempty"`
	Source         string `json:"source,omitempty"`
	Hash           string `json:"hash,omitempty"`
	TokensEstimate int    `json:"tokensEstimate,omitempty"`
	Cache          string `json:"cache,omitempty"`
	RetentionRank  string `json:"retentionRank,omitempty"`
	RetentionClass string `json:"retentionClass,omitempty"`
	CompactAction  string `json:"compactAction,omitempty"`
	CompactSchema  string `json:"compactSchema,omitempty"`
	SourceRef      string `json:"sourceRef,omitempty"`
	Redaction      string `json:"redaction,omitempty"`
	Purpose        string `json:"purpose,omitempty"`
}

type AiopsTransportContextDecision struct {
	Kind             string `json:"kind,omitempty"`
	Decision         string `json:"decision,omitempty"`
	Reason           string `json:"reason,omitempty"`
	RetentionRank    string `json:"retentionRank,omitempty"`
	CompactAction    string `json:"compactAction,omitempty"`
	SourceRef        string `json:"sourceRef,omitempty"`
	ArtifactRef      string `json:"artifactRef,omitempty"`
	ValidationStatus string `json:"validationStatus,omitempty"`
	SafetyImpact     string `json:"safetyImpact,omitempty"`
	BlockingReason   string `json:"blockingReason,omitempty"`
	Redaction        string `json:"redaction,omitempty"`
}

type AiopsTransportToolSurfaceEntry struct {
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
	Reason  string `json:"reason,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Risk    string `json:"risk,omitempty"`
}

type AiopsTransportScopedTraceEntry struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
	Scope  string `json:"scope,omitempty"`
	Source string `json:"source,omitempty"`
}

type AiopsTransportEvidenceTrace struct {
	ID        string `json:"id"`
	Source    string `json:"source,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Redaction string `json:"redaction,omitempty"`
	Status    string `json:"status,omitempty"`
}

type AiopsTransportHostTaskReportTrace struct {
	ID           string   `json:"id"`
	Status       string   `json:"status,omitempty"`
	HostID       string   `json:"hostId,omitempty"`
	PlanStepID   string   `json:"planStepId,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	Blockers     []string `json:"blockers,omitempty"`
	NextSteps    []string `json:"nextSteps,omitempty"`
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
		HostMissions:     map[string]AiopsTransportHostMission{},
		ChildAgents:      map[string]AiopsTransportChildAgent{},
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
