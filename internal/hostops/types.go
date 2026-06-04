package hostops

import "time"

// HostMentionSource identifies how a textual @mention was interpreted.
type HostMentionSource string

const (
	HostMentionSourceInventory       HostMentionSource = "inventory"
	HostMentionSourceIPLiteral       HostMentionSource = "ip_literal"
	HostMentionSourceHostnameLiteral HostMentionSource = "hostname_literal"
)

// HostMention is the server-side representation of one @host token.
type HostMention struct {
	TokenID     string            `json:"tokenId"`
	Raw         string            `json:"raw"`
	SpanStart   int               `json:"spanStart"`
	SpanEnd     int               `json:"spanEnd"`
	HostID      string            `json:"hostId,omitempty"`
	Address     string            `json:"address,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Source      HostMentionSource `json:"source"`
	Resolved    bool              `json:"resolved"`
	Confidence  float64           `json:"confidence"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type HostMissionStatus string

const (
	HostMissionStatusPlanning              HostMissionStatus = "planning"
	HostMissionStatusWaitingPlanAcceptance HostMissionStatus = "waiting_plan_acceptance"
	HostMissionStatusSpawningChildren      HostMissionStatus = "spawning_children"
	HostMissionStatusRunning               HostMissionStatus = "running"
	HostMissionStatusWaitingApproval       HostMissionStatus = "waiting_approval"
	HostMissionStatusCompleted             HostMissionStatus = "completed"
	HostMissionStatusFailed                HostMissionStatus = "failed"
	HostMissionStatusCancelled             HostMissionStatus = "cancelled"
)

type HostChildAgentStatus string

const (
	HostChildAgentStatusPlanned          HostChildAgentStatus = "planned"
	HostChildAgentStatusSpawning         HostChildAgentStatus = "spawning"
	HostChildAgentStatusRunning          HostChildAgentStatus = "running"
	HostChildAgentStatusWaiting          HostChildAgentStatus = "waiting"
	HostChildAgentStatusApprovalRequired HostChildAgentStatus = "approval_required"
	HostChildAgentStatusCompleted        HostChildAgentStatus = "completed"
	HostChildAgentStatusFailed           HostChildAgentStatus = "failed"
	HostChildAgentStatusCancelled        HostChildAgentStatus = "cancelled"
)

type HostOperationMission struct {
	ID             string            `json:"id"`
	ThreadID       string            `json:"threadId"`
	UserTurnID     string            `json:"userTurnId"`
	ManagerAgentID string            `json:"managerAgentId,omitempty"`
	Status         HostMissionStatus `json:"status"`
	PlanRequired   bool              `json:"planRequired"`
	PlanAccepted   bool              `json:"planAccepted"`
	Mentions       []HostMention     `json:"mentions"`
	ChildAgentIDs  []string          `json:"childAgentIds"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type HostChildAgent struct {
	ID                string               `json:"id"`
	MissionID         string               `json:"missionId"`
	ParentAgentID     string               `json:"parentAgentId,omitempty"`
	SessionID         string               `json:"sessionId"`
	HostID            string               `json:"hostId"`
	HostAddress       string               `json:"hostAddress"`
	Role              string               `json:"role"`
	Task              string               `json:"task"`
	Status            HostChildAgentStatus `json:"status"`
	PlanStepIDs       []string             `json:"planStepIds"`
	LastInputPreview  string               `json:"lastInputPreview"`
	LastOutputPreview string               `json:"lastOutputPreview"`
	Error             string               `json:"error,omitempty"`
	StartedAt         time.Time            `json:"startedAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
	CompletedAt       *time.Time           `json:"completedAt,omitempty"`
}

type TranscriptItemType string

const (
	TranscriptItemManagerMessage   TranscriptItemType = "manager_message"
	TranscriptItemUserFollowup     TranscriptItemType = "user_followup"
	TranscriptItemAssistantMessage TranscriptItemType = "assistant_message"
	TranscriptItemToolCall         TranscriptItemType = "tool_call"
	TranscriptItemToolResult       TranscriptItemType = "tool_result"
	TranscriptItemApproval         TranscriptItemType = "approval"
	TranscriptItemError            TranscriptItemType = "error"
)

type TranscriptItem struct {
	ID         string             `json:"id"`
	Type       TranscriptItemType `json:"type"`
	Content    string             `json:"content,omitempty"`
	ToolName   string             `json:"toolName,omitempty"`
	ApprovalID string             `json:"approvalId,omitempty"`
	Status     string             `json:"status,omitempty"`
	Payload    map[string]any     `json:"payload,omitempty"`
	CreatedAt  time.Time          `json:"createdAt"`
}
