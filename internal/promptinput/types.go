package promptinput

import (
	"encoding/json"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/specialinputmemory"
)

// ToolCall is the promptinput-local tool call shape. Keeping it local avoids
// a dependency cycle between runtimekernel and promptinput.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResult is the promptinput-local tool result shape.
type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content,omitempty"`
}

// Message is the conversation message shape consumed by the prompt input
// builder.
type Message struct {
	Role             string      `json:"role"`
	Content          string      `json:"content,omitempty"`
	ReasoningContent string      `json:"reasoningContent,omitempty"`
	ToolCalls        []ToolCall  `json:"toolCalls,omitempty"`
	ToolResult       *ToolResult `json:"toolResult,omitempty"`
}

// BuildRequest contains all structured inputs needed to build provider model
// messages and a semantic trace for those messages.
type BuildRequest struct {
	History     []Message
	Compiled    promptcompiler.CompiledPrompt
	State       agentstate.AgentState
	Tools       []promptcompiler.Tool
	Memories    []MemoryItem
	MaxMemories int

	OpsContextCapsule     string
	SessionFactCount      int
	LettaHintCount        int
	DroppedContextReasons []string
	ContextGovernance     []ContextGovernanceTraceItem

	VerificationReportRef  string
	VerificationStatus     string
	TaskDepth              *TaskDepthTrace
	EvidenceCoverage       *EvidenceCoverageTrace
	GenericityTrace        *GenericityTrace
	CompletionGate         *CompletionGateTrace
	SafetySignals          []SafetySignalTrace
	UnexpectedStateGate    *UnexpectedStateGateTrace
	ApprovalScope          *ApprovalScopeTrace
	ResourceBindings       []resourcebinding.ResourceBindingSnapshot
	ResourceRoleBindings   []resourcebinding.ResourceRoleBinding
	ResourceCapabilities   []resourcebinding.ResourceCapability
	ResourceEvidenceRefs   []resourcebinding.EvidenceRef
	SessionTargetSnapshot  *resourcebinding.SessionTargetSnapshot
	RoleBindingConflicts   []resourcebinding.RoleBindingConflict
	AgentAssemblySnapshot  *agentassembly.AgentAssemblySnapshot
	SpecialInputWorldState *specialinputmemory.SpecialInputWorldStateSection
}

// BuildResult is the provider-neutral model input plus its explainable trace.
type BuildResult struct {
	Items []ModelInputItem
	Trace PromptInputTrace
}

// PromptInputTrace records where each prompt input item came from.
type PromptInputTrace struct {
	Items                         []TraceItem                                       `json:"items"`
	PromptSections                []promptcompiler.PromptSectionTrace               `json:"promptSections,omitempty"`
	ChangedSections               []promptcompiler.ChangedPromptSection             `json:"changedSections,omitempty"`
	OpsContextCapsuleChars        int                                               `json:"opsContextCapsuleChars,omitempty"`
	SessionFactCount              int                                               `json:"sessionFactCount,omitempty"`
	LettaHintCount                int                                               `json:"lettaHintCount,omitempty"`
	MemoryItemCount               int                                               `json:"memoryItemCount,omitempty"`
	VisibleOpsManualTools         []string                                          `json:"visibleOpsManualTools,omitempty"`
	DroppedContextReasons         []string                                          `json:"droppedContextReasons,omitempty"`
	ContextDedupe                 *ContextDedupeTrace                               `json:"contextDedupe,omitempty"`
	ContextGovernance             []ContextGovernanceTraceItem                      `json:"contextGovernance,omitempty"`
	ContextUsage                  ContextUsage                                      `json:"contextUsage,omitempty"`
	AssemblySource                string                                            `json:"assembly_source,omitempty"`
	PromptCompilerSource          string                                            `json:"prompt_compiler_source,omitempty"`
	ToolSurfaceSource             string                                            `json:"tool_surface_source,omitempty"`
	AdapterName                   string                                            `json:"adapter_name,omitempty"`
	ToolSurfaceFingerprint        string                                            `json:"toolSurfaceFingerprint,omitempty"`
	ToolSurfacePolicySnapshotHash string                                            `json:"toolSurfacePolicySnapshotHash,omitempty"`
	ToolSurfaceSnapshot           *ToolSurfaceSnapshot                              `json:"toolSurfaceSnapshot,omitempty"`
	PublicWebBudget               *PublicWebBudgetTrace                             `json:"publicWebBudget,omitempty"`
	WebSearchPolicy               *WebSearchPolicyTrace                             `json:"webSearchPolicy,omitempty"`
	WebSearch                     *WebSearchTrace                                   `json:"webSearch,omitempty"`
	Final                         *FinalTrace                                       `json:"final,omitempty"`
	DeferredToolDirectory         []promptcompiler.DeferredToolDirectoryEntry       `json:"deferredToolDirectory,omitempty"`
	LoadedToolsDelta              []string                                          `json:"loadedToolsDelta,omitempty"`
	LoadedPacksDelta              []string                                          `json:"loadedPacksDelta,omitempty"`
	SkillIndexHash                string                                            `json:"skillIndexHash,omitempty"`
	LoadedSkillsDelta             []string                                          `json:"loadedSkillsDelta,omitempty"`
	ToolSearchEvents              []ToolSearchTraceEvent                            `json:"toolSearchEvents,omitempty"`
	ToolSelectionEvents           []ToolSelectionTraceEvent                         `json:"toolSelectionEvents,omitempty"`
	RejectedToolCalls             []RejectedToolCallTraceEvent                      `json:"rejectedToolCalls,omitempty"`
	DispatchDecisions             []DispatchDecisionTrace                           `json:"dispatchDecisions,omitempty"`
	SkillSearchEvents             []SkillSearchTraceEvent                           `json:"skillSearchEvents,omitempty"`
	SkillReadEvents               []SkillReadTraceEvent                             `json:"skillReadEvents,omitempty"`
	RejectedSkillActivations      []RejectedSkillActivationTraceEvent               `json:"rejectedSkillActivations,omitempty"`
	MCPInstructionDeltas          []MCPInstructionDeltaTrace                        `json:"mcpInstructionDeltas,omitempty"`
	ParallelDispatchGroups        []ParallelDispatchTraceGroup                      `json:"parallelDispatchGroups,omitempty"`
	FailedToolSummaries           []FailedToolSummary                               `json:"failedToolSummaries,omitempty"`
	AgentIndexHash                string                                            `json:"agentIndexHash,omitempty"`
	AgentIndexEntries             []AgentIndexEntryTrace                            `json:"agentIndexEntries,omitempty"`
	AgentIndexDropped             []DroppedAgentIndexEntryTrace                     `json:"agentIndexDropped,omitempty"`
	AgentIndexDelta               []string                                          `json:"agentIndexDelta,omitempty"`
	AgentDelegationDecision       *AgentDelegationDecisionTrace                     `json:"agentDelegationDecision,omitempty"`
	AgentAssignmentLint           []AgentAssignmentLintTrace                        `json:"agentAssignmentLint,omitempty"`
	AgentParallelTraceGroups      []AgentParallelTraceGroup                         `json:"agentParallelTraceGroups,omitempty"`
	ResourceBindings              []resourcebinding.ResourceBindingSnapshot         `json:"resourceBindings,omitempty"`
	ResourceRoleBindings          []resourcebinding.ResourceRoleBinding             `json:"resourceRoleBindings,omitempty"`
	ResourceCapabilities          []resourcebinding.ResourceCapability              `json:"resourceCapabilities,omitempty"`
	ResourceEvidenceRefs          []resourcebinding.EvidenceRef                     `json:"resourceEvidenceRefs,omitempty"`
	SessionTargetSnapshot         *resourcebinding.SessionTargetSnapshot            `json:"sessionTargetSnapshot,omitempty"`
	RoleBindingConflicts          []resourcebinding.RoleBindingConflict             `json:"roleBindingConflicts,omitempty"`
	AgentAssemblySnapshot         *agentassembly.AgentAssemblySnapshot              `json:"agentAssemblySnapshot,omitempty"`
	SpecialInputWorldState        *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState,omitempty"`
	ResourceLocks                 []ResourceLockTrace                               `json:"resourceLocks,omitempty"`
	OwnerWriteTraces              []OwnerWriteTrace                                 `json:"ownerWriteTraces,omitempty"`
	AgentFinalGate                *AgentFinalGateDecisionTrace                      `json:"agentFinalGate,omitempty"`
	AgentNotifications            []AgentNotificationTrace                          `json:"agentNotifications,omitempty"`
	VerificationAgentReport       *VerificationAgentReportTrace                     `json:"verificationAgentReport,omitempty"`
	VerificationReportRef         string                                            `json:"verificationReportRef,omitempty"`
	VerificationStatus            string                                            `json:"verificationStatus,omitempty"`
	TaskDepth                     *TaskDepthTrace                                   `json:"taskDepth,omitempty"`
	EvidenceCoverage              *EvidenceCoverageTrace                            `json:"evidenceCoverage,omitempty"`
	GenericityTrace               *GenericityTrace                                  `json:"genericityTrace,omitempty"`
	CompletionGate                *CompletionGateTrace                              `json:"completionGate,omitempty"`
	SafetySignals                 []SafetySignalTrace                               `json:"safetySignals,omitempty"`
	UnexpectedStateGate           *UnexpectedStateGateTrace                         `json:"unexpectedStateGate,omitempty"`
	ApprovalScope                 *ApprovalScopeTrace                               `json:"approvalScope,omitempty"`
	PlanModeState                 *PlanModeTraceState                               `json:"planModeState,omitempty"`
	PlanArtifactRef               string                                            `json:"planArtifactRef,omitempty"`
	PlanTransitions               []PlanTransitionTrace                             `json:"planTransitions,omitempty"`
	PlanRequirementDecision       *PlanRequirementDecisionTrace                     `json:"planRequirementDecision,omitempty"`
	PlanCompletionGate            *PlanCompletionGateTrace                          `json:"planCompletionGate,omitempty"`
	TaskClaims                    []TaskClaimTrace                                  `json:"taskClaims,omitempty"`
	PlanApprovalScope             *PlanApprovalScopeTrace                           `json:"planApprovalScope,omitempty"`
	PlanRejectionEvents           []PlanRejectionEventTrace                         `json:"planRejectionEvents,omitempty"`
	TaskTodoState                 *TaskTodoTraceState                               `json:"taskTodoState,omitempty"`
}

type ContextDedupeTrace struct {
	RepeatedUserMessageCount int `json:"repeatedUserMessageCount,omitempty"`
	SavedChars               int `json:"savedChars,omitempty"`
	RetainedDeltaChars       int `json:"retainedDeltaChars,omitempty"`
}

type PublicWebBudgetTrace struct {
	MaxSearchCalls        int  `json:"maxSearchCalls,omitempty"`
	MaxQueries            int  `json:"maxQueries,omitempty"`
	MaxResults            int  `json:"maxResults,omitempty"`
	MaxCallsPerTurn       int  `json:"maxCallsPerTurn,omitempty"`
	MaxQueriesPerCall     int  `json:"maxQueriesPerCall,omitempty"`
	MaxResultsPerDomain   int  `json:"maxResultsPerDomain,omitempty"`
	ExplicitUserRequested bool `json:"explicitUserRequested,omitempty"`
}

type WebSearchPolicyTrace struct {
	Level            string   `json:"level,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	ReasonCodes      []string `json:"reasonCodes,omitempty"`
	QuerySeeds       []string `json:"querySeeds,omitempty"`
	DisabledBy       string   `json:"disabledBy,omitempty"`
	RequireCitations bool     `json:"requireCitations,omitempty"`
}

type WebSearchTrace struct {
	Attempted     bool   `json:"attempted,omitempty"`
	RetryCount    int    `json:"retryCount,omitempty"`
	Adapter       string `json:"adapter,omitempty"`
	SourceCount   int    `json:"sourceCount,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
}

type FinalTrace struct {
	PublicWebLimitation bool `json:"publicWebLimitation,omitempty"`
}

type DispatchDecisionTrace struct {
	ToolName               string `json:"toolName,omitempty"`
	ToolCallID             string `json:"toolCallId,omitempty"`
	ToolSurfaceFingerprint string `json:"toolSurfaceFingerprint"`
	PermissionSnapshotHash string `json:"permissionSnapshotHash"`
	ArgumentsHash          string `json:"argumentsHash"`
}

type ToolSurfaceSnapshot struct {
	Fingerprint      string              `json:"fingerprint,omitempty"`
	VisibleTools     []string            `json:"visibleTools,omitempty"`
	DeferredTools    []string            `json:"deferredTools,omitempty"`
	HiddenTools      []string            `json:"hiddenTools,omitempty"`
	HiddenReasons    map[string][]string `json:"hiddenReasons,omitempty"`
	LoadedPacksDelta []string            `json:"loadedPacksDelta,omitempty"`
	PolicyHash       string              `json:"policyHash,omitempty"`
}

type OwnerWriteTrace struct {
	Responsibility string `json:"responsibility"`
	Owner          string `json:"owner"`
	Writer         string `json:"writer"`
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	Outcome        string `json:"outcome"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

// TraceItem is one semantic prompt-input trace entry.
type TraceItem struct {
	Source       string `json:"source"`
	SemanticRole string `json:"semanticRole"`
	ProviderRole string `json:"providerRole,omitempty"`
	PromptLayer  string `json:"promptLayer,omitempty"`
	ID           string `json:"id,omitempty"`
	Status       string `json:"status,omitempty"`
	Content      string `json:"content,omitempty"`
}

type ContextGovernanceTraceItem struct {
	ID                  string             `json:"id,omitempty"`
	Layer               string             `json:"layer"`
	Kind                string             `json:"kind"`
	Message             string             `json:"message,omitempty"`
	ToolCallID          string             `json:"toolCallId,omitempty"`
	ToolName            string             `json:"toolName,omitempty"`
	MaterializationTier string             `json:"materializationTier,omitempty"`
	OriginalBytes       int64              `json:"originalBytes,omitempty"`
	InlineBytes         int64              `json:"inlineBytes,omitempty"`
	Budget              map[string]int     `json:"budget,omitempty"`
	ReferenceIDs        []string           `json:"referenceIds,omitempty"`
	Resource            *ResourceTraceItem `json:"resource,omitempty"`
	RetryAttempt        int                `json:"retryAttempt,omitempty"`
	RetryMax            int                `json:"retryMax,omitempty"`
}

type ResourceRange = resourceio.Range

type ResourceTraceItem struct {
	URI         string        `json:"uri,omitempty"`
	Digest      string        `json:"digest,omitempty"`
	ContentType string        `json:"contentType,omitempty"`
	Bytes       int64         `json:"bytes,omitempty"`
	Range       ResourceRange `json:"range,omitempty"`
}

type ContextUsage struct {
	MaxContextTokens     int                    `json:"maxContextTokens"`
	ReservedOutputTokens int                    `json:"reservedOutputTokens"`
	EstimatedInputTokens int                    `json:"estimatedInputTokens"`
	Categories           []ContextUsageCategory `json:"categories"`
	TopContributors      []ContextContributor   `json:"topContributors,omitempty"`
}

type ContextUsageCategory struct {
	Name           string `json:"name"`
	Bytes          int    `json:"bytes"`
	TokensEstimate int    `json:"tokensEstimate"`
	Items          int    `json:"items,omitempty"`
}

type ContextContributor struct {
	Kind           string `json:"kind"`
	ID             string `json:"id,omitempty"`
	TokensEstimate int    `json:"tokensEstimate"`
	Bytes          int    `json:"bytes"`
	Action         string `json:"action,omitempty"`
}

type ToolSearchTraceEvent struct {
	Mode            string                     `json:"mode,omitempty"`
	Query           string                     `json:"query,omitempty"`
	Ranker          string                     `json:"ranker,omitempty"`
	MatchCount      int                        `json:"matchCount,omitempty"`
	RejectedCount   int                        `json:"rejectedCount,omitempty"`
	Matches         []string                   `json:"matches,omitempty"`
	RejectedReasons []ToolSearchRejectedReason `json:"rejectedReasons,omitempty"`
	Reason          string                     `json:"reason,omitempty"`
}

type ToolSearchRejectedReason struct {
	ToolName       string `json:"toolName,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Status         string `json:"status,omitempty"`
	Source         string `json:"source,omitempty"`
	MCPServerID    string `json:"mcpServerId,omitempty"`
	HealthStatus   string `json:"healthStatus,omitempty"`
	FilteredReason string `json:"filteredReason,omitempty"`
}

type ToolSelectionTraceEvent struct {
	Source           string            `json:"source,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	LoadedTools      []string          `json:"loadedTools,omitempty"`
	LoadedPacks      []string          `json:"loadedPacks,omitempty"`
	NotLoaded        []string          `json:"notLoaded,omitempty"`
	NotLoadedReasons map[string]string `json:"notLoadedReasons,omitempty"`
}

type RejectedToolCallTraceEvent struct {
	ToolName             string `json:"toolName"`
	ErrorType            string `json:"errorType"`
	Reason               string `json:"reason"`
	RequiredAction       string `json:"requiredAction,omitempty"`
	SuggestedSearchQuery string `json:"suggestedSearchQuery,omitempty"`
	TurnID               string `json:"turnId,omitempty"`
	ToolCallID           string `json:"toolCallId,omitempty"`
}

type SkillSearchTraceEvent struct {
	Mode       string   `json:"mode,omitempty"`
	Query      string   `json:"query,omitempty"`
	MatchCount int      `json:"matchCount,omitempty"`
	Matches    []string `json:"matches,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type SkillReadTraceEvent struct {
	Skill  string `json:"skill,omitempty"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
	Range  string `json:"range,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

type RejectedSkillActivationTraceEvent struct {
	SkillName      string `json:"skillName,omitempty"`
	Reason         string `json:"reason"`
	RequiredAction string `json:"requiredAction,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
}

type MCPInstructionDeltaTrace struct {
	ServerID string `json:"serverId,omitempty"`
	Action   string `json:"action,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Chars    int    `json:"chars,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

type ParallelDispatchTraceGroup struct {
	GroupID            string                         `json:"groupId"`
	Decision           string                         `json:"decision"`
	Reasons            []string                       `json:"reasons,omitempty"`
	ToolCalls          []ParallelDispatchToolCall     `json:"toolCalls,omitempty"`
	Excluded           []ParallelDispatchExcludedTool `json:"excluded,omitempty"`
	SharedResourceKeys []string                       `json:"sharedResourceKeys,omitempty"`
}

type ParallelDispatchToolCall struct {
	ToolCallID        string `json:"toolCallId,omitempty"`
	ToolName          string `json:"toolName"`
	SharedResourceKey string `json:"sharedResourceKey,omitempty"`
}

type ParallelDispatchExcludedTool struct {
	ToolCallID        string   `json:"toolCallId,omitempty"`
	ToolName          string   `json:"toolName"`
	Reasons           []string `json:"reasons,omitempty"`
	SharedResourceKey string   `json:"sharedResourceKey,omitempty"`
}

type FailedToolSummary struct {
	Tool          string `json:"tool"`
	FailureClass  string `json:"failureClass"`
	Attempts      int    `json:"attempts"`
	FinalStatus   string `json:"finalStatus"`
	SafeToRetry   bool   `json:"safeToRetry"`
	ModelGuidance string `json:"modelGuidance"`
}

type PlanModeTraceState struct {
	State            string `json:"state,omitempty"`
	PlanID           string `json:"planId,omitempty"`
	ArtifactStatus   string `json:"artifactStatus,omitempty"`
	ApprovalStatus   string `json:"approvalStatus,omitempty"`
	ReminderLevel    string `json:"reminderLevel,omitempty"`
	PendingQuestions int    `json:"pendingQuestions,omitempty"`
	OpenQuestions    int    `json:"openQuestions,omitempty"`
	RejectionReason  string `json:"rejectionReason,omitempty"`
}

type PlanTransitionTrace struct {
	PlanID string `json:"planId,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type PlanRequirementDecisionTrace struct {
	Required bool     `json:"required"`
	Decision string   `json:"decision,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Signals  []string `json:"signals,omitempty"`
}

type PlanCompletionGateTrace struct {
	Decision string   `json:"decision,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
}

type CompletionGateTrace struct {
	Decision string   `json:"decision,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
}

type TaskDepthTrace struct {
	Level               string   `json:"level,omitempty"`
	Reasons             []string `json:"reasons,omitempty"`
	RequiresPlan        bool     `json:"requiresPlan"`
	RequiresEvidence    bool     `json:"requiresEvidence"`
	RequiresValidation  bool     `json:"requiresValidation"`
	AnalysisOnly        bool     `json:"analysisOnly,omitempty"`
	ExecutionProhibited bool     `json:"executionProhibited,omitempty"`
}

type EvidenceCoverageTrace struct {
	Action             string   `json:"action,omitempty"`
	Coverage           float64  `json:"coverage,omitempty"`
	RequiredDimensions []string `json:"requiredDimensions,omitempty"`
	CoveredDimensions  []string `json:"coveredDimensions,omitempty"`
	MissingDimensions  []string `json:"missingDimensions,omitempty"`
	OpenQuestions      []string `json:"openQuestions,omitempty"`
	VerificationStatus string   `json:"verificationStatus,omitempty"`
	Reasons            []string `json:"reasons,omitempty"`
}

type GenericityTrace struct {
	CoreRuleDomainTerms []string `json:"coreRuleDomainTerms,omitempty"`
	AllowedFixtureTerms []string `json:"allowedFixtureTerms,omitempty"`
	AllowedPluginTerms  []string `json:"allowedPluginTerms,omitempty"`
	ResourceIDSource    string   `json:"resourceIdSource,omitempty"`
	Violations          []string `json:"violations,omitempty"`
}

type SafetySignalTrace struct {
	Category string   `json:"category,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Action   string   `json:"action,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
}

type UnexpectedStateGateTrace struct {
	Action         string   `json:"action,omitempty"`
	Sources        []string `json:"sources,omitempty"`
	AffectedScopes []string `json:"affectedScopes,omitempty"`
	BlockedAction  string   `json:"blockedAction,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

type ApprovalScopeTrace struct {
	GrantID        string   `json:"grantId,omitempty"`
	Status         string   `json:"status,omitempty"`
	AllowedActions []string `json:"allowedActions,omitempty"`
	ResourceScopes []string `json:"resourceScopes,omitempty"`
	RiskCeiling    string   `json:"riskCeiling,omitempty"`
	ExpiresAt      string   `json:"expiresAt,omitempty"`
	InputHash      string   `json:"inputHash,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

type TaskClaimTrace struct {
	TaskID string `json:"taskId,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type PlanApprovalScopeTrace struct {
	PlanID         string   `json:"planId,omitempty"`
	ApprovedScopes []string `json:"approvedScopes,omitempty"`
	DeniedScopes   []string `json:"deniedScopes,omitempty"`
}

type PlanRejectionEventTrace struct {
	PlanID string `json:"planId,omitempty"`
	Reason string `json:"reason,omitempty"`
	By     string `json:"by,omitempty"`
}

type TaskTodoTraceState struct {
	Items []TaskTodoTraceItem `json:"items,omitempty"`
}

type TaskTodoTraceItem struct {
	ID              string `json:"id,omitempty"`
	Status          string `json:"status,omitempty"`
	Owner           string `json:"owner,omitempty"`
	BlockedBy       string `json:"blockedBy,omitempty"`
	PendingEvidence string `json:"pendingEvidence,omitempty"`
}

type AgentIndexEntryTrace struct {
	Kind            string   `json:"kind,omitempty"`
	Name            string   `json:"name,omitempty"`
	Description     string   `json:"description,omitempty"`
	WhenToUse       string   `json:"whenToUse,omitempty"`
	CapabilityKinds []string `json:"capabilityKinds,omitempty"`
	ResourceTypes   []string `json:"resourceTypes,omitempty"`
	OperationKinds  []string `json:"operationKinds,omitempty"`
	MaxConcurrent   int      `json:"maxConcurrent,omitempty"`
	CostClass       string   `json:"costClass,omitempty"`
}

type DroppedAgentIndexEntryTrace struct {
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type AgentDelegationDecisionTrace struct {
	Action          string   `json:"action"`
	Reason          string   `json:"reason,omitempty"`
	CandidateAgent  string   `json:"candidateAgent,omitempty"`
	ExistingAgentID string   `json:"existingAgentId,omitempty"`
	RequiredFields  []string `json:"requiredFields,omitempty"`
}

type AgentAssignmentLintTrace struct {
	AgentID       string   `json:"agentId,omitempty"`
	Status        string   `json:"status"`
	MissingFields []string `json:"missingFields,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
}

type AgentParallelTraceGroup struct {
	MissionID      string                   `json:"missionId,omitempty"`
	RequestedCount int                      `json:"requestedCount,omitempty"`
	SpawnedInTurn  []string                 `json:"spawnedInTurn,omitempty"`
	Queued         []string                 `json:"queued,omitempty"`
	SerialReasons  []AgentSerialReasonTrace `json:"serialReasons,omitempty"`
}

type AgentSerialReasonTrace struct {
	AgentID string `json:"agentId,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type ResourceLockKeyTrace struct {
	ResourceType  string `json:"resourceType,omitempty"`
	ResourceID    string `json:"resourceId,omitempty"`
	OperationKind string `json:"operationKind,omitempty"`
}

type ResourceLockTrace struct {
	LeaseID string               `json:"leaseId,omitempty"`
	AgentID string               `json:"agentId,omitempty"`
	Action  string               `json:"action,omitempty"`
	Reason  string               `json:"reason,omitempty"`
	Holder  string               `json:"holder,omitempty"`
	Key     ResourceLockKeyTrace `json:"key,omitempty"`
}

type AgentFinalGateDecisionTrace struct {
	Action        string   `json:"action"`
	PendingAgents []string `json:"pendingAgents,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
}

type AgentNotificationTrace struct {
	AgentID    string          `json:"agentId"`
	Status     string          `json:"status"`
	Summary    string          `json:"summary,omitempty"`
	ResultRefs []string        `json:"resultRefs,omitempty"`
	Usage      AgentUsageTrace `json:"usage,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type AgentUsageTrace struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	ToolCalls    int `json:"toolCalls,omitempty"`
}

type VerificationAgentReportTrace struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary,omitempty"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	Counterchecks []string `json:"counterchecks,omitempty"`
	Blockers      []string `json:"blockers,omitempty"`
}

type MemoryItem struct {
	ID    string `json:"id"`
	Scope string `json:"scope,omitempty"`
	Text  string `json:"text"`
}

// Builder builds provider input and trace from promptcompiler output plus
// filtered conversation state.
type Builder struct{}
