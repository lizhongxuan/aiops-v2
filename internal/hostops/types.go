package hostops

import (
	"time"

	"aiops-v2/internal/opssemantic"
	"aiops-v2/internal/resourcebinding"
)

// HostMentionSource identifies how a textual @mention was interpreted.
type HostMentionSource string

const (
	HostMentionSourceInventory       HostMentionSource = "inventory"
	HostMentionSourceIPLiteral       HostMentionSource = "ip_literal"
	HostMentionSourceHostnameLiteral HostMentionSource = "hostname_literal"
	HostMentionSourceLocalAlias      HostMentionSource = "local_alias"
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
	HostChildAgentStatusBlocked          HostChildAgentStatus = "blocked"
	HostChildAgentStatusCompleted        HostChildAgentStatus = "completed"
	HostChildAgentStatusFailed           HostChildAgentStatus = "failed"
	HostChildAgentStatusCancelled        HostChildAgentStatus = "cancelled"
	HostChildAgentStatusQueued           HostChildAgentStatus = "queued"
	HostChildAgentStatusSuperseded       HostChildAgentStatus = "superseded"
)

type PlanStatus string

const (
	PlanStatusDraft             PlanStatus = "draft"
	PlanStatusWaitingAcceptance PlanStatus = "waiting_acceptance"
	PlanStatusAccepted          PlanStatus = "accepted"
	PlanStatusRunning           PlanStatus = "running"
	PlanStatusCompleted         PlanStatus = "completed"
	PlanStatusFailed            PlanStatus = "failed"
	PlanStatusCancelled         PlanStatus = "cancelled"
)

type PlanStepStatus string

const (
	PlanStepStatusPending   PlanStepStatus = "pending"
	PlanStepStatusRunning   PlanStepStatus = "running"
	PlanStepStatusCompleted PlanStepStatus = "completed"
	PlanStepStatusBlocked   PlanStepStatus = "blocked"
	PlanStepStatusFailed    PlanStepStatus = "failed"
	PlanStepStatusCancelled PlanStepStatus = "cancelled"
)

type HostOperationPlan struct {
	ID         string         `json:"id"`
	Version    int            `json:"version"`
	Status     PlanStatus     `json:"status"`
	Steps      []PlanStep     `json:"steps"`
	Revisions  []PlanRevision `json:"revisions,omitempty"`
	AcceptedAt *time.Time     `json:"acceptedAt,omitempty"`
	AcceptedBy string         `json:"acceptedBy,omitempty"`
}

type PlanStep struct {
	ID               string                    `json:"id"`
	Index            int                       `json:"index"`
	Title            string                    `json:"title"`
	Summary          string                    `json:"summary,omitempty"`
	Status           PlanStepStatus            `json:"status"`
	HostIDs          []string                  `json:"hostIds"`
	ChildAgentIDs    []string                  `json:"childAgentIds,omitempty"`
	ActionType       opssemantic.OpsActionType `json:"actionType"`
	RiskLevel        opssemantic.OpsRiskLevel  `json:"riskLevel"`
	EvidenceRequired []string                  `json:"evidenceRequired,omitempty"`
	ApprovalRequired bool                      `json:"approvalRequired"`
	StartedAt        *time.Time                `json:"startedAt,omitempty"`
	CompletedAt      *time.Time                `json:"completedAt,omitempty"`
}

type PlanRevision struct {
	ID                 string    `json:"id"`
	FromVersion        int       `json:"fromVersion"`
	ToVersion          int       `json:"toVersion"`
	Reason             string    `json:"reason"`
	AffectedHostIDs    []string  `json:"affectedHostIds,omitempty"`
	Changes            []string  `json:"changes"`
	RequiresAcceptance bool      `json:"requiresAcceptance"`
	CreatedAt          time.Time `json:"createdAt"`
}

type HostOperationMission struct {
	ID                           string                                `json:"id"`
	SessionID                    string                                `json:"sessionId,omitempty"`
	ThreadID                     string                                `json:"threadId"`
	UserTurnID                   string                                `json:"userTurnId"`
	ManagerAgentID               string                                `json:"managerAgentId,omitempty"`
	Status                       HostMissionStatus                     `json:"status"`
	SemanticTask                 opssemantic.OpsSemanticTask           `json:"semanticTask"`
	Plan                         HostOperationPlan                     `json:"plan"`
	PlanRequired                 bool                                  `json:"planRequired"`
	PlanAccepted                 bool                                  `json:"planAccepted"`
	Mentions                     []HostMention                         `json:"mentions"`
	RoleBindings                 []resourcebinding.ResourceRoleBinding `json:"roleBindings,omitempty"`
	RoleConflicts                []resourcebinding.RoleBindingConflict `json:"roleConflicts,omitempty"`
	RoleBindingAssignmentEnabled bool                                  `json:"roleBindingAssignmentEnabled,omitempty"`
	ChildAgentIDs                []string                              `json:"childAgentIds"`
	CreatedAt                    time.Time                             `json:"createdAt"`
	UpdatedAt                    time.Time                             `json:"updatedAt"`
}

type HostChildAgent struct {
	ID                string               `json:"id"`
	MissionID         string               `json:"missionId"`
	ParentAgentID     string               `json:"parentAgentId,omitempty"`
	SessionID         string               `json:"sessionId"`
	HostID            string               `json:"hostId"`
	HostAddress       string               `json:"hostAddress"`
	HostDisplayName   string               `json:"hostDisplayName,omitempty"`
	Role              string               `json:"role"`
	BoundRole         string               `json:"boundRole,omitempty"`
	RoleBindingHash   string               `json:"roleBindingHash,omitempty"`
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

type HostSubTask struct {
	ID                    string                       `json:"id,omitempty"`
	MissionID             string                       `json:"missionId"`
	PlanStepID            string                       `json:"planStepId"`
	HostAgentID           string                       `json:"hostAgentId"`
	HostID                string                       `json:"hostId"`
	BoundRole             string                       `json:"boundRole,omitempty"`
	RoleBindingHash       string                       `json:"roleBindingHash,omitempty"`
	Goal                  string                       `json:"goal"`
	Constraints           []string                     `json:"constraints,omitempty"`
	ActionType            opssemantic.OpsActionType    `json:"actionType,omitempty"`
	RiskLevel             opssemantic.OpsRiskLevel     `json:"riskLevel"`
	EvidenceRequirements  []string                     `json:"evidenceRequirements,omitempty"`
	SchedulingDirective   HostSubTaskScheduleDirective `json:"schedulingDirective,omitempty"`
	ManagerRevisionReason string                       `json:"managerRevisionReason,omitempty"`
}

type HostTaskCommandRecord struct {
	Command         string `json:"command"`
	RedactedCommand string `json:"redactedCommand,omitempty"`
	Status          string `json:"status"`
	ExitCode        int    `json:"exitCode,omitempty"`
	Summary         string `json:"summary,omitempty"`
	EvidenceRef     string `json:"evidenceRef,omitempty"`
}

type HostTaskReport struct {
	MissionID       string                  `json:"missionId"`
	PlanStepID      string                  `json:"planStepId"`
	HostAgentID     string                  `json:"hostAgentId"`
	HostID          string                  `json:"hostId"`
	BoundRole       string                  `json:"boundRole,omitempty"`
	RoleBindingHash string                  `json:"roleBindingHash,omitempty"`
	Status          string                  `json:"status"`
	Summary         string                  `json:"summary,omitempty"`
	Commands        []HostTaskCommandRecord `json:"commands,omitempty"`
	EvidenceRefs    []string                `json:"evidenceRefs,omitempty"`
	Evidence        []HostTaskEvidence      `json:"evidence,omitempty"`
	Errors          []string                `json:"errors,omitempty"`
	Blockers        []string                `json:"blockers,omitempty"`
	NextSteps       []string                `json:"nextSteps,omitempty"`
}

type HostTaskReportStatus string

const (
	HostTaskReportStatusCompleted                HostTaskReportStatus = "completed"
	HostTaskReportStatusFailed                   HostTaskReportStatus = "failed"
	HostTaskReportStatusBlocked                  HostTaskReportStatus = "blocked"
	HostTaskReportStatusBlockedApproval          HostTaskReportStatus = "blocked_approval"
	HostTaskReportStatusBlockedEvidence          HostTaskReportStatus = "blocked_evidence"
	HostTaskReportStatusCancelled                HostTaskReportStatus = "cancelled"
	HostTaskReportStatusTimeout                  HostTaskReportStatus = "timeout"
	HostTaskReportStatusNeedsManagerCoordination HostTaskReportStatus = "needs_manager_coordination"
	HostTaskReportStatusNeedsUserApproval        HostTaskReportStatus = "needs_user_approval"
)

type EvidenceSource string

const (
	EvidenceSourceHostCommandTool EvidenceSource = "host_command_tool"
	EvidenceSourceHumanTerminal   EvidenceSource = "human_terminal"
	EvidenceSourceArtifact        EvidenceSource = "artifact"
)

type RedactionStatus string

const (
	RedactionStatusUnknown     RedactionStatus = ""
	RedactionStatusApplied     RedactionStatus = "applied"
	RedactionStatusNotRequired RedactionStatus = "not_required"
)

type HostTaskEvidence struct {
	ID              string          `json:"id"`
	MissionID       string          `json:"missionId,omitempty"`
	PlanStepID      string          `json:"planStepId,omitempty"`
	HostAgentID     string          `json:"hostAgentId,omitempty"`
	HostID          string          `json:"hostId"`
	Source          EvidenceSource  `json:"source"`
	ArtifactRef     string          `json:"artifactRef,omitempty"`
	CommandRecordID string          `json:"commandRecordId,omitempty"`
	Summary         string          `json:"summary,omitempty"`
	RedactionStatus RedactionStatus `json:"redactionStatus,omitempty"`
}

type HostSubTaskStatus string

const (
	HostSubTaskStatusRunning         HostSubTaskStatus = "running"
	HostSubTaskStatusQueued          HostSubTaskStatus = "queued"
	HostSubTaskStatusCompleted       HostSubTaskStatus = "completed"
	HostSubTaskStatusBlockedApproval HostSubTaskStatus = "blocked_approval"
	HostSubTaskStatusBlockedEvidence HostSubTaskStatus = "blocked_evidence"
	HostSubTaskStatusFailed          HostSubTaskStatus = "failed"
	HostSubTaskStatusCancelled       HostSubTaskStatus = "cancelled"
	HostSubTaskStatusTimeout         HostSubTaskStatus = "timeout"
	HostSubTaskStatusSuperseded      HostSubTaskStatus = "superseded"
)

type HostSubTaskScheduleDirective string

const (
	HostSubTaskScheduleDefault   HostSubTaskScheduleDirective = ""
	HostSubTaskScheduleCancel    HostSubTaskScheduleDirective = "cancel"
	HostSubTaskScheduleSupersede HostSubTaskScheduleDirective = "supersede"
)

type HostSubTaskScheduleDecision struct {
	SubTaskID             string            `json:"subTaskId"`
	MissionID             string            `json:"missionId"`
	HostID                string            `json:"hostId"`
	PlanStepID            string            `json:"planStepId,omitempty"`
	Status                HostSubTaskStatus `json:"status"`
	ActiveSubTaskID       string            `json:"activeSubTaskId,omitempty"`
	SupersededSubTaskID   string            `json:"supersededSubTaskId,omitempty"`
	ManagerRevisionReason string            `json:"managerRevisionReason,omitempty"`
	ToolCallID            string            `json:"toolCallId,omitempty"`
	EvidenceRef           string            `json:"evidenceRef,omitempty"`
	BlockingReason        string            `json:"blockingReason,omitempty"`
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
