package promptcompiler

import (
	"github.com/cloudwego/eino/schema"

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

// CompileContext carries all inputs needed to compile a four-layer prompt.
type CompileContext struct {
	// SessionType identifies the session domain (host/workspace).
	SessionType SessionType

	// Mode identifies the current runtime policy (chat/inspect/plan/execute).
	Mode Mode

	// AssembledTools is the ordered set of tools assembled for this prompt.
	AssembledTools []Tool

	// RuntimePolicy is the active policy text for the current mode.
	RuntimePolicy string

	// PlanningPolicy controls whether structured plan events are preferred.
	PlanningPolicy string

	// EvidencePolicy controls source-bound evidence expectations.
	EvidencePolicy string

	// AnswerStyle controls final answer structure and density.
	AnswerStyle string

	// ToolBudget describes the tool result and dispatch budget profile.
	ToolBudget string

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

	// EvidenceReminders are per-iteration reminders that should stay outside the
	// stable prompt envelope.
	EvidenceReminders []string

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

// PromptSection is an additional prompt fragment injected into developer instructions.
type PromptSection struct {
	Title   string
	Content string
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
}

// ToolPromptDelta captures per-iteration changes that should not force a
// stable tool index rewrite.
type ToolPromptDelta struct {
	Content                string
	NewlyAvailable         []string
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
	Items []ProtocolPromptItem
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

	// SkillPromptAssets are prompt fragments that may change within a turn.
	SkillPromptAssets []string

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

// CompiledPrompt is the promptcompiler output. It preserves the legacy
// flattened four-layer view for compatibility and also exposes a stable /
// dynamic split for long-running turn reuse.
type CompiledPrompt struct {
	// Stable contains the layers that are expected to stay stable within a turn.
	Stable StablePromptEnvelope

	// Dynamic contains the layers that may change per turn.
	Dynamic DynamicPromptDelta

	// System is Layer 1: environment and role.
	System SystemPrompt

	// Developer is Layer 2: runtime constraints.
	Developer DeveloperInstructions

	// Tools is Layer 3: capability descriptions.
	Tools ToolPromptSet

	// Policy is Layer 4: mode-specific policy constraints.
	Policy RuntimePolicyPrompt

	// Fingerprint identifies prompt-layer changes without exposing prompt text.
	Fingerprint PromptFingerprint
}

// ---------------------------------------------------------------------------
// Compiler is the PromptCompiler interface.
// ---------------------------------------------------------------------------

// Compiler is the unique prompt truth source. It compiles structured inputs
// into a four-layer CompiledPrompt and can convert to Eino message format.
type Compiler interface {
	// Compile compiles the four-layer prompt from the given context.
	Compile(ctx CompileContext) (CompiledPrompt, error)

	// CompileForEino compiles and converts to Eino *schema.Message format,
	// suitable for adk.ChatModelAgent's Instruction field.
	CompileForEino(ctx CompileContext) ([]*schema.Message, error)
}
