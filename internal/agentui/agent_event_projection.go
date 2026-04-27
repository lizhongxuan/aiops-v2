package agentui

type RuntimeLiveness struct {
	ActiveTurns          map[string]bool `json:"activeTurns"`
	ActiveAgents         map[string]bool `json:"activeAgents"`
	PendingApprovals     map[string]bool `json:"pendingApprovals"`
	PendingUserInputs    map[string]bool `json:"pendingUserInputs"`
	ActiveCommandStreams map[string]bool `json:"activeCommandStreams"`
}

type AgentEventProjection struct {
	SessionID          string                     `json:"sessionId"`
	ThreadID           string                     `json:"threadId,omitempty"`
	CurrentTurnID      string                     `json:"currentTurnId,omitempty"`
	Status             string                     `json:"status"`
	LastSeq            int64                      `json:"lastSeq"`
	RuntimeLiveness    RuntimeLiveness            `json:"runtimeLiveness"`
	Timeline           []TimelineEntry            `json:"timeline"`
	Agents             []AgentProjection          `json:"agents"`
	Approvals          []ApprovalProjection       `json:"approvals"`
	Artifacts          []ArtifactProjection       `json:"artifacts"`
	Diff               *DiffProjection            `json:"diff,omitempty"`
	FinalMessages      map[string]AssistantFinal  `json:"finalMessages,omitempty"`
	ProcessGroups      map[string][]TimelineEntry `json:"processGroups,omitempty"`
	LastTerminalFailed bool                       `json:"-"`
}

type TimelineEntry struct {
	ID           string               `json:"id"`
	Kind         AgentEventKind       `json:"kind"`
	TurnID       string               `json:"turnId,omitempty"`
	AgentID      string               `json:"agentId,omitempty"`
	ToolCallID   string               `json:"toolCallId,omitempty"`
	DisplayKind  string               `json:"displayKind,omitempty"`
	Phase        AgentEventPhase      `json:"phase"`
	Status       AgentEventStatus     `json:"status"`
	Visibility   AgentEventVisibility `json:"visibility"`
	Title        string               `json:"title,omitempty"`
	Summary      string               `json:"summary,omitempty"`
	Detail       string               `json:"detail,omitempty"`
	Risk         string               `json:"risk,omitempty"`
	RawRef       string               `json:"rawRef,omitempty"`
	Foldable     bool                 `json:"foldable,omitempty"`
	AutoCollapse bool                 `json:"autoCollapse,omitempty"`
	Collapsed    bool                 `json:"collapsed,omitempty"`
	DurationMs   int64                `json:"durationMs,omitempty"`
	UpdatedAt    string               `json:"updatedAt,omitempty"`
	Seq          int64                `json:"seq"`
}

type AgentProjection struct {
	ID          string           `json:"id"`
	Handle      string           `json:"handle,omitempty"`
	Name        string           `json:"name,omitempty"`
	Role        string           `json:"role,omitempty"`
	Status      AgentEventStatus `json:"status"`
	LastAction  string           `json:"lastAction,omitempty"`
	LastSummary string           `json:"lastSummary,omitempty"`
	Stats       AgentStats       `json:"stats,omitempty"`
	StartedAt   string           `json:"startedAt,omitempty"`
	CompletedAt string           `json:"completedAt,omitempty"`
}

type ApprovalProjection struct {
	ID           string           `json:"id"`
	ApprovalType string           `json:"approvalType,omitempty"`
	Title        string           `json:"title,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Risk         string           `json:"risk,omitempty"`
	Decision     string           `json:"decision,omitempty"`
	Targets      []string         `json:"targets,omitempty"`
	Status       AgentEventStatus `json:"status"`
	UpdatedAt    string           `json:"updatedAt,omitempty"`
}

type ArtifactProjection struct {
	ID          string           `json:"id"`
	Kind        string           `json:"kind,omitempty"`
	Title       string           `json:"title,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	URI         string           `json:"uri,omitempty"`
	ContentType string           `json:"contentType,omitempty"`
	Status      AgentEventStatus `json:"status"`
	UpdatedAt   string           `json:"updatedAt,omitempty"`
}

type DiffProjection struct {
	Scope        string     `json:"scope,omitempty"`
	Files        []DiffFile `json:"files,omitempty"`
	FilesCount   int        `json:"filesCount,omitempty"`
	AddedLines   int        `json:"addedLines,omitempty"`
	RemovedLines int        `json:"removedLines,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	UpdatedAt    string     `json:"updatedAt,omitempty"`
}

type AssistantFinal struct {
	TurnID    string           `json:"turnId"`
	Text      string           `json:"text"`
	Status    AgentEventStatus `json:"status"`
	UpdatedAt string           `json:"updatedAt,omitempty"`
}
