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

	// HostContext contains host-specific context (hostname, OS, etc.).
	HostContext string

	// WorkspaceContext contains workspace-level context (mission, hosts, etc.).
	WorkspaceContext string

	// SkillPromptAssets are prompt fragments contributed by skill capabilities.
	SkillPromptAssets []string

	// Deprecated: compatibility-only legacy field from the pre-unified prompt path.
	// PromptCompiler ignores these assets; MCP guidance should come from assembled tools.
	MCPPromptAssets []string

	// ExtraSections are additional prompt sections injected by runtime/hook layers.
	ExtraSections []PromptSection

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
// Per Req 3.5: only capability, constraints, result shape, and approval note.
type ToolPromptEntry struct {
	// Capability describes what the tool can do.
	Capability string

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

// RuntimePolicyPrompt is Layer 4: policy constraints based on current mode.
type RuntimePolicyPrompt struct {
	// Content is the compiled runtime policy prompt text.
	Content string

	// Mode is the mode this policy was compiled for.
	Mode Mode
}

// ---------------------------------------------------------------------------
// CompiledPrompt is the four-layer output of the PromptCompiler.
// ---------------------------------------------------------------------------

// CompiledPrompt contains the four compiled prompt layers in order:
// System Prompt → Developer Instructions → Tool Prompt Set → Runtime Policy Prompt.
type CompiledPrompt struct {
	// System is Layer 1: environment and role.
	System SystemPrompt

	// Developer is Layer 2: runtime constraints.
	Developer DeveloperInstructions

	// Tools is Layer 3: capability descriptions.
	Tools ToolPromptSet

	// Policy is Layer 4: mode-specific policy constraints.
	Policy RuntimePolicyPrompt
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
