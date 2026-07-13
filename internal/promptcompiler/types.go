package promptcompiler

import (
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Type aliases to avoid circular imports with runtimekernel.
// ---------------------------------------------------------------------------

// SessionType mirrors runtimekernel.SessionType.
type SessionType = string

// Mode mirrors runtimekernel.Mode.
type Mode = string

// Tool mirrors the assembled tool contract consumed by PromptCompiler.
type Tool = tooling.Tool

// ---------------------------------------------------------------------------
// AgentKind identifies the type of agent being compiled for.
// ---------------------------------------------------------------------------

// AgentKind identifies the agent type, determining prompt template and capability scope.
type AgentKind string

const (
	AgentKindPlanner AgentKind = "planner"
	AgentKindWorker  AgentKind = "worker"
)

// ---------------------------------------------------------------------------
// CompileContext is the structured input to the PromptCompiler.
// ---------------------------------------------------------------------------

// CompileContext carries all inputs needed to compile the section-first model
// prompt and its compatibility views.
type CompileContext struct {
	// SessionType identifies the session domain (host/workspace).
	SessionType SessionType

	// Mode identifies the current runtime policy (chat/inspect/plan/execute).
	Mode Mode

	// AssembledTools is the ordered set of tools assembled for this prompt.
	AssembledTools []Tool

	// DeferredToolCatalog is the full discoverable catalog used only to render
	// compact, non-callable deferred family summaries. It must never inject
	// deferred tool schemas into the initial model input.
	DeferredToolCatalog []Tool

	// MCPHealthSnapshot maps generic external tool source/server ids to health
	// states used in compact deferred directory summaries.
	MCPHealthSnapshot map[string]string

	// RuntimePolicy is the active policy text for the current mode.
	RuntimePolicy string

	// Profile is the runtime-selected prompt/tool profile for this turn.
	Profile string

	// PlanningPolicy controls whether structured plan events are preferred.
	PlanningPolicy string

	// EvidencePolicy controls source-bound evidence expectations.
	EvidencePolicy string

	// AnswerStyle controls final answer structure and density.
	AnswerStyle string

	// ToolBudget describes the tool result and dispatch budget profile.
	ToolBudget string

	// RuntimeState carries compact per-turn state that should be visible to the
	// model without expanding into policy prose.
	WebState               string
	OpsGraphState          string
	CorootState            string
	OpsManusState          string
	PendingApprovals       int
	PendingEvidence        int
	VisibleToolFingerprint string
	UserConstraints        []string
	TimeoutRecoveryState   string

	// TaskDepth describes the current turn's complexity and completion gates.
	TaskDepth taskdepth.Profile

	// ReasoningEffort carries the configured provider reasoning effort or prompt fallback policy.
	ReasoningEffort string

	// ReasoningSummary controls whether user-visible reasoning summaries are emitted.
	ReasoningSummary string

	// ReasoningSummaryDisplay controls how reasoning summaries are exposed in UI.
	ReasoningSummaryDisplay string

	// ShowRawReasoning is a debug-only flag; it must default to false.
	ShowRawReasoning bool

	// DisableDiagnosticProtocol disables the diagnosis prompt profile only.
	// Trace redaction and safety guardrails must remain active outside prompt compilation.
	DisableDiagnosticProtocol bool

	// HostContext contains host-specific context (hostname, OS, etc.).
	HostContext string

	// WorkspaceContext contains workspace-level context (mission, hosts, etc.).
	WorkspaceContext string

	// SkillPromptAssets are prompt fragments contributed by skill capabilities.
	SkillPromptAssets []string

	// HostTaskPromptAssets are assigned host-bound task fragments from
	// manager-to-host agent messages. They are not skill instructions.
	HostTaskPromptAssets []string

	// LoadedSkillRefs are compact markers for skill bodies loaded this session.
	LoadedSkillRefs []LoadedSkillPromptRef

	// EvidenceReminders are per-iteration reminders that should stay outside the
	// stable prompt envelope.
	EvidenceReminders []string

	// HostOpsManager marks the current turn as a host operation manager route.
	HostOpsManager bool

	// HostOpsPlanRequired requires a structured plan before host mutations.
	HostOpsPlanRequired bool

	// Deprecated: compatibility-only legacy field from the pre-unified prompt path.
	// PromptCompiler ignores these assets; MCP guidance should come from assembled tools.
	MCPPromptAssets []string

	// ExtraSections are additional prompt sections injected by runtime/hook layers.
	ExtraSections []PromptSection

	// ToolDelta carries per-iteration availability/approval changes for Layer 3.
	ToolDelta ToolPromptDelta

	// ProtocolState carries dynamic protocol items such as plan/todo/approval
	// state. It is rendered separately from ad hoc text fragments so prompt
	// traces can show state changes between model calls.
	ProtocolState ProtocolPromptState

	// AgentKind identifies the type of agent being compiled for.
	AgentKind AgentKind
}

type LoadedSkillPromptRef struct {
	Name   string
	Source string
	Reason string
	Range  string
	Hash   string
}

// PromptSection is an additional prompt fragment injected into developer instructions.
type PromptSection struct {
	Title       string
	Content     string
	SourceType  string
	SourceRef   string
	RetrievedAt string
	TrustLevel  string
}

// DynamicContextSource is a bounded prompt-facing dynamic context block.
// Content is already budgeted; overflow raw text is represented by summary and
// source/evidence refs instead of being inlined into the model prompt.
type DynamicContextSource struct {
	ID                   string `json:"id"`
	Title                string `json:"title,omitempty"`
	Content              string `json:"content,omitempty"`
	TokenBudget          int    `json:"tokenBudget,omitempty"`
	TokensEstimate       int    `json:"tokensEstimate,omitempty"`
	Overflowed           bool   `json:"overflowed,omitempty"`
	Summary              string `json:"summary,omitempty"`
	SourceRef            string `json:"sourceRef,omitempty"`
	EvidenceRef          string `json:"evidenceRef,omitempty"`
	RawAvailableViaTrace bool   `json:"rawAvailableViaTrace,omitempty"`
}

// PromptCompiledSection is the section-first model input unit produced by the
// compiler. Compatibility four-layer fields are derived from these sections
// during migration, but callers should prefer this ordered envelope for model
// input assembly.
type PromptCompiledSection struct {
	ID           string
	Layer        string
	LogicalLayer PromptLogicalLayer
	BundleRef    string
	Role         string
	Content      string
	Stability    string
	MaxTokens    int
	Source       string
	Required     bool
}

// PromptEnvelope is the ordered prompt product consumed by model input
// builders. It keeps behavior sections explicit so runtime state, profile, tool
// surface, and dynamic context are not reconstructed by separate paths.
type PromptEnvelope struct {
	Sections []PromptCompiledSection
}

// ---------------------------------------------------------------------------
// Four-layer Prompt structures
// ---------------------------------------------------------------------------

// SystemPrompt is Layer 1: environment and role definition.
type SystemPrompt struct {
	// Content is the compiled system prompt text.
	Content string

	// Role describes the agent's role identity.
	Role string

	// Environment describes the runtime environment context.
	Environment string
}

// DeveloperInstructions is Layer 2: runtime constraints and operational rules.
type DeveloperInstructions struct {
	// Content is the compiled developer instructions text.
	Content string

	// Constraints lists the active operational constraints.
	Constraints []string
}

// ToolPromptEntry describes a single tool's prompt information.
type ToolPromptEntry struct {
	// Capability describes what the tool can do.
	Capability string

	// Governance summarizes risk, approval, budgeting, and failure policy.
	Governance string

	// UsagePolicy describes when the model should choose this tool.
	UsagePolicy string

	// Guidance carries tool-specific prompt guidance rendered only when the
	// tool is visible in the current prompt.
	Guidance string

	// Example gives one compact usage example.
	Example string

	// FailureHandling describes how to recover when the tool fails or returns
	// insufficient evidence.
	FailureHandling string

	// Constraints describes usage limitations and preconditions.
	Constraints string

	// ResultShape describes the expected output format.
	ResultShape string

	// ApprovalNote describes the approval requirements for this tool.
	ApprovalNote string
}

// ToolPromptSet is Layer 3: capability descriptions for all visible tools.
type ToolPromptSet struct {
	// Content is the compiled tool prompt text (concatenation of all entries).
	Content string

	// Entries contains per-tool prompt information.
	Entries []ToolPromptEntry

	// DeferredDirectory contains compact, non-callable summaries for deferred
	// tool families and MCP packs. It is traceable but does not expose schemas.
	DeferredDirectory []DeferredToolDirectoryEntry
}

// DeferredToolDirectoryEntry is a compact prompt-facing and trace-facing
// summary of tools that can be discovered later via tool_search/select.
type DeferredToolDirectoryEntry struct {
	Pack              string   `json:"pack"`
	Capability        string   `json:"capability,omitempty"`
	Source            string   `json:"source,omitempty"`
	MCPServerID       string   `json:"mcpServerId,omitempty"`
	HealthStatus      string   `json:"healthStatus,omitempty"`
	RequiresHealth    bool     `json:"requiresHealth,omitempty"`
	RequiresApproval  bool     `json:"requiresApproval,omitempty"`
	RequiresSelect    bool     `json:"requiresSelect,omitempty"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	ToolCount         int      `json:"toolCount,omitempty"`
	ResourceTypes     []string `json:"resourceTypes,omitempty"`
	OperationKinds    []string `json:"operationKinds,omitempty"`
}

// ToolPromptDelta captures per-iteration changes that should not force a
// stable tool index rewrite.
type ToolPromptDelta struct {
	Content                string
	NewlyAvailable         []string
	NewlyAvailablePacks    []string
	TemporarilyUnavailable []string
	ApprovalRequired       []string
}

// ProtocolPromptItem is one dynamic protocol-state item visible to the model.
type ProtocolPromptItem struct {
	Kind   string
	ID     string
	Status string
	Text   string
}

// ProtocolPromptState groups dynamic state items that should be visible as
// state rather than free-form appended prompt text.
type ProtocolPromptState struct {
	Items             []ProtocolPromptItem
	PlanMode          *PlanModePromptState
	TaskTodo          *TaskTodoPromptState
	FailureSwitchPath *FailureSwitchPathPromptState
}

// PlanModePromptState is a compact prompt-facing state shape. It avoids a
// runtimekernel dependency while preserving the fields needed for plan-mode
// reminders and resume after compaction.
type PlanModePromptState struct {
	State                   string
	PlanID                  string
	ArtifactStatus          string
	ApprovalStatus          string
	ReminderLevel           string
	FullInstructionInjected bool
	PendingQuestions        int
	OpenQuestions           int
	RejectionReason         string
}

type TaskTodoPromptState struct {
	Items []TaskTodoPromptItem
}

type TaskTodoPromptItem struct {
	ID              string
	Status          string
	Owner           string
	BlockedBy       string
	PendingEvidence string
}

type FailureSwitchPathPromptState struct {
	Signature        string
	SeenCount        int
	Action           string
	SwitchPathReason string
}

// RuntimePolicyPrompt is Layer 4: policy constraints based on current mode.
type RuntimePolicyPrompt struct {
	// Content is the compiled runtime policy prompt text.
	Content string

	// Mode is the mode this policy was compiled for.
	Mode Mode
}

// ---------------------------------------------------------------------------
// Stable / Dynamic prompt structures
// ---------------------------------------------------------------------------

// StablePromptEnvelope groups the prompt layers that usually stay stable within
// a turn. It intentionally excludes runtime policy so the caller can cache or
// reuse it.
type StablePromptEnvelope struct {
	// Content is the concatenated stable prompt text.
	Content string

	// System is Layer 1: environment and role.
	System SystemPrompt

	// Developer is Layer 2: runtime constraints.
	Developer DeveloperInstructions

	// Tools is Layer 3: capability descriptions.
	Tools ToolPromptSet
}

// StablePrompt is kept as a compatibility alias while the runtime migrates to
// the more explicit envelope/delta naming.
type StablePrompt = StablePromptEnvelope

// DynamicPromptDelta groups the prompt layers that may change per turn.
type DynamicPromptDelta struct {
	// Content is the concatenated dynamic prompt text.
	Content string

	// Sources are the bounded source-id blocks used to render Content.
	Sources []DynamicContextSource

	// SkillPromptAssets are prompt fragments that may change within a turn.
	SkillPromptAssets []string

	// HostTaskPromptAssets are host-bound task fragments that may change within
	// a turn. They are separated from skill prompt assets to avoid skill-cache
	// and skill-trace pollution.
	HostTaskPromptAssets []string

	// EvidenceReminders are runtime-generated reminders that should stay out of
	// the stable prompt envelope.
	EvidenceReminders []string

	// ExtraSections are hook/runtime-injected sections that may change within a turn.
	ExtraSections []PromptSection

	// ToolDelta captures availability/approval changes for the current iteration.
	ToolDelta ToolPromptDelta

	// ProtocolState captures dynamic item/plan/todo/approval state.
	ProtocolState ProtocolPromptState

	// Policy is Layer 4: mode-specific policy constraints.
	Policy RuntimePolicyPrompt
}

// DynamicPrompt is kept as a compatibility alias while the runtime migrates to
// the more explicit envelope/delta naming.
type DynamicPrompt = DynamicPromptDelta

// PromptFingerprint is a privacy-safe digest of compiled prompt layers.
// It intentionally carries hashes and versions only, never full prompt text.
type PromptFingerprint struct {
	Version           string `json:"version,omitempty"`
	CompilerVersion   string `json:"compilerVersion,omitempty"`
	StableHash        string `json:"stableHash,omitempty"`
	SystemHash        string `json:"systemHash,omitempty"`
	DeveloperHash     string `json:"developerHash,omitempty"`
	ToolRegistryHash  string `json:"toolRegistryHash,omitempty"`
	RuntimePolicyHash string `json:"runtimePolicyHash,omitempty"`
	ProtocolStateHash string `json:"protocolStateHash,omitempty"`
}

// PromptSectionTrace is a redaction-safe prompt-section summary. It stores
// only stable identifiers, hashes, and size estimates, never section text.
type PromptSectionTrace struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	Hash           string `json:"hash"`
	Bytes          int    `json:"bytes"`
	TokensEstimate int    `json:"tokensEstimate"`
	TokenEstimate  int    `json:"tokenEstimate,omitempty"`
	Cache          string `json:"cache,omitempty"`
	RetentionRank  string `json:"retentionRank,omitempty"`
	RetentionClass string `json:"retentionClass,omitempty"`
	CompactAction  string `json:"compactAction,omitempty"`
	Action         string `json:"action,omitempty"`
	CompactSchema  string `json:"compactSchema,omitempty"`
	SourceRef      string `json:"sourceRef,omitempty"`
	Redaction      string `json:"redaction,omitempty"`
	Purpose        string `json:"purpose,omitempty"`
}

// ChangedPromptSection records why one prompt section hash changed.
type ChangedPromptSection struct {
	ID           string `json:"id"`
	Reason       string `json:"reason"`
	PreviousHash string `json:"previousHash,omitempty"`
	CurrentHash  string `json:"currentHash,omitempty"`
}

const (
	PromptSectionKindStable  = "stable"
	PromptSectionKindDynamic = "dynamic"

	PromptSectionCacheMiss = "miss"
	PromptSectionCacheHit  = "hit"
	// PromptSectionCacheInvalidated marks a known section whose hash changed
	// since the previous model input in the same session.
	PromptSectionCacheInvalidated = "invalidated"

	PromptSectionChangeInitial               = "initial"
	PromptSectionChangeSystemRoleChanged     = "system_role_changed"
	PromptSectionChangeDeveloperRulesChanged = "developer_core_rules_changed"
	PromptSectionChangeToolsIndexChanged     = "tools_index_changed"
	PromptSectionChangeRuntimePolicyChanged  = "runtime_policy_changed"
	PromptSectionChangeProtocolStateChanged  = "protocol_state_changed"
	PromptSectionChangeDynamicAssetsChanged  = "context_dynamic_assets_changed"
	PromptSectionChangeSectionAdded          = "section_added"
	PromptSectionChangeSectionRemoved        = "section_removed"
	PromptSectionChangeSectionContentChanged = "section_content_changed"

	RetentionRankP0 = "P0"
	RetentionRankP1 = "P1"
	RetentionRankP2 = "P2"
	RetentionRankP3 = "P3"
	RetentionRankP4 = "P4"

	RetentionClassMustKeep    = "must_keep"
	RetentionClassSummarize   = "summarize"
	RetentionClassExternalize = "externalize"
	RetentionClassDropIfStale = "drop_if_stale"

	CompactActionKeptOriginal = "kept_original"
	CompactActionSummarized   = "summarized"
	CompactActionExternalized = "externalized"
	CompactActionDropped      = "dropped"
	CompactActionBlocked      = "blocked"
)

// CompiledPrompt is the promptcompiler output. Envelope is the section-first
// prompt product used by model input assembly; stable/dynamic and four-layer
// fields are derived compatibility views for tests, trace labels, and thin
// adapters that still need typed access while the public API is retired.
type CompiledPrompt struct {
	// Envelope is the section-first prompt product consumed by model input assembly.
	Envelope PromptEnvelope

	// EnvelopeV2 is the validated L0-L6 shadow product. Task 10 does not
	// change provider input; the promptinput cutover consumes it later.
	EnvelopeV2 PromptEnvelopeV2

	// Stable is a derived compatibility view of stable envelope sections.
	Stable StablePromptEnvelope

	// Dynamic is a derived compatibility view of dynamic envelope sections.
	Dynamic DynamicPromptDelta

	// System is a derived compatibility view of base/profile content.
	System SystemPrompt

	// Developer is a derived compatibility view of profile content.
	Developer DeveloperInstructions

	// Tools is a derived compatibility view of tool.surface.
	Tools ToolPromptSet

	// Policy is a derived compatibility view of runtime.state.
	Policy RuntimePolicyPrompt

	// Fingerprint identifies prompt-layer changes without exposing prompt text.
	Fingerprint PromptFingerprint

	// PromptSections summarize prompt sections without exposing raw section text.
	PromptSections []PromptSectionTrace

	// ChangedSections optionally carries a caller-computed section diff.
	ChangedSections []ChangedPromptSection
}

// ---------------------------------------------------------------------------
// Compiler is the PromptCompiler interface.
// ---------------------------------------------------------------------------

// Compiler is the unique prompt truth source. It compiles structured inputs
// into a section-first CompiledPrompt. Provider-specific message conversion
// belongs to modelrouter provider adapters.
type Compiler interface {
	// Compile compiles the section-first prompt from the given context.
	Compile(ctx CompileContext) (CompiledPrompt, error)
}
