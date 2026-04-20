package agentmgr

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/capability"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// AgentConfig represents the assembled configuration for creating an agent.
// This is the output of the factory methods, containing all the pieces needed
// to construct an Eino ADK agent (ChatModelAgent or PlanExecuteAgent).
// ---------------------------------------------------------------------------

// AgentConfig holds the assembled configuration for an agent instance.
// It contains the model, instruction messages, tools, and iteration limits
// needed to create an adk.ChatModelAgent or PlanExecuteAgent.
type AgentConfig struct {
	// Kind identifies the agent type.
	Kind AgentKind

	// Model is the resolved ChatModel for this agent.
	Model modelrouter.ChatModel

	// Instructions are the compiled prompt messages (Eino format).
	Instructions []*schema.Message

	// Tools are the Eino-adapted tools available to this agent.
	Tools []tool.BaseTool

	// MaxIterations is the maximum ReAct loop iterations.
	MaxIterations int

	// HostID is the bound host (empty for planner/workspace-level agents).
	HostID string

	// MissionID is the workspace mission ID (empty for host sessions).
	MissionID string
}

// ---------------------------------------------------------------------------
// WorkspaceAgentConfig holds the PlanExecuteAgent configuration.
// ---------------------------------------------------------------------------

// WorkspaceAgentConfig holds the assembled configuration for a workspace
// PlanExecuteAgent, including Planner, Executor, and Replanner configs.
type WorkspaceAgentConfig struct {
	// Planner is the configuration for the Planner ChatModelAgent.
	Planner AgentConfig

	// Executor is the configuration for the Executor (spawns Worker agents).
	Executor AgentConfig

	// Replanner is the configuration for the Replanner ChatModelAgent.
	Replanner AgentConfig

	// MissionID is the workspace mission this agent serves.
	MissionID string
}

// ---------------------------------------------------------------------------
// AgentFactory creates agent configurations based on AgentDefinitions.
// It uses Registry, PromptCompiler, ModelRouter, and PolicyEngine to assemble
// the full agent configuration.
// ---------------------------------------------------------------------------

// AgentFactory creates Eino ADK agent configurations by assembling models,
// prompts, and tools from the system's core components.
type AgentFactory struct {
	// definitions maps AgentKind to its definition template.
	definitions map[AgentKind]*AgentDefinition

	// registry provides capability/tool lookup and assembly.
	registry *capability.Registry

	// compiler compiles structured prompts for agents.
	compiler promptcompiler.Compiler

	// modelRouter resolves ChatModel instances by AgentKind.
	modelRouter *modelrouter.Router

	// policy provides policy evaluation (used for tool filtering context).
	policy *policyengine.Engine
}

// NewAgentFactory creates a new AgentFactory with the given dependencies.
func NewAgentFactory(
	registry *capability.Registry,
	compiler promptcompiler.Compiler,
	modelRouter *modelrouter.Router,
	policy *policyengine.Engine,
) *AgentFactory {
	return &AgentFactory{
		definitions: make(map[AgentKind]*AgentDefinition),
		registry:    registry,
		compiler:    compiler,
		modelRouter: modelRouter,
		policy:      policy,
	}
}

// RegisterDefinition registers an AgentDefinition for a given AgentKind.
// This allows the factory to create agents of that kind.
func (f *AgentFactory) RegisterDefinition(def *AgentDefinition) error {
	if def == nil {
		return fmt.Errorf("agent definition is nil")
	}
	if err := def.Validate(); err != nil {
		return fmt.Errorf("register definition: %w", err)
	}
	f.definitions[def.Kind] = def
	return nil
}

// GetDefinition returns the registered definition for the given kind, or nil.
func (f *AgentFactory) GetDefinition(kind AgentKind) *AgentDefinition {
	return f.definitions[kind]
}

func compileContextWithAssembledTools(base promptcompiler.CompileContext, tools []tooling.Tool) promptcompiler.CompileContext {
	base.AssembledTools = tools
	return base
}

func buildEinoToolPool(tools []tooling.Tool) []tool.BaseTool {
	return tooling.AssembleEinoToolPool(tools)
}

// ---------------------------------------------------------------------------
// CreateHostAgent creates a ChatModelAgent config for a single-host session.
// ---------------------------------------------------------------------------

// CreateHostAgent creates an AgentConfig for a host session ChatModelAgent.
// It resolves the model, compiles the prompt with host context, and assembles
// tools visible in the host session for the given mode.
func (f *AgentFactory) CreateHostAgent(ctx context.Context, hostID string, mode string) (*AgentConfig, error) {
	if hostID == "" {
		return nil, fmt.Errorf("hostID is required for host agent")
	}

	// Resolve model via ModelRouter (use worker kind for host agents — single host).
	agentKind := modelrouter.AgentKindWorker
	providerCfg := modelrouter.ProviderConfig{}
	model, err := f.modelRouter.GetModel(agentKind, providerCfg)
	if err != nil {
		return nil, fmt.Errorf("create host agent: get model: %w", err)
	}

	// Assemble tools once and reuse them for prompt compilation and runtime.
	assembledTools := f.registry.AssembleTools("host", mode)
	tools := buildEinoToolPool(assembledTools)

	// Compile prompt via PromptCompiler with host context.
	compileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType: "host",
		Mode:        mode,
		HostContext: hostID,
		AgentKind:   promptcompiler.AgentKindWorker,
	}, assembledTools)
	instructions, err := f.compiler.CompileForEino(compileCtx)
	if err != nil {
		return nil, fmt.Errorf("create host agent: compile prompt: %w", err)
	}

	// Determine max iterations from definition if registered.
	maxIter := 25 // default
	if def := f.definitions[AgentKindWorker]; def != nil && def.MaxIterations > 0 {
		maxIter = def.MaxIterations
	}

	return &AgentConfig{
		Kind:          AgentKindWorker,
		Model:         model,
		Instructions:  instructions,
		Tools:         tools,
		MaxIterations: maxIter,
		HostID:        hostID,
	}, nil
}

// ---------------------------------------------------------------------------
// CreateWorkspaceAgent creates a PlanExecuteAgent config for workspace sessions.
// ---------------------------------------------------------------------------

// CreateWorkspaceAgent creates a WorkspaceAgentConfig for a workspace session
// PlanExecuteAgent. It assembles Planner, Executor, and Replanner configs.
func (f *AgentFactory) CreateWorkspaceAgent(ctx context.Context, missionID string) (*WorkspaceAgentConfig, error) {
	if missionID == "" {
		return nil, fmt.Errorf("missionID is required for workspace agent")
	}

	// --- Planner Agent ---
	plannerModel, err := f.modelRouter.GetModel(modelrouter.AgentKindPlanner, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: get planner model: %w", err)
	}

	plannerAssembledTools := f.registry.AssembleTools("workspace", "plan")
	plannerTools := buildEinoToolPool(plannerAssembledTools)
	plannerCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "plan",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, plannerAssembledTools)
	plannerInstructions, err := f.compiler.CompileForEino(plannerCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile planner prompt: %w", err)
	}

	plannerMaxIter := 10
	if def := f.definitions[AgentKindPlanner]; def != nil && def.MaxIterations > 0 {
		plannerMaxIter = def.MaxIterations
	}

	plannerCfg := AgentConfig{
		Kind:          AgentKindPlanner,
		Model:         plannerModel,
		Instructions:  plannerInstructions,
		Tools:         plannerTools,
		MaxIterations: plannerMaxIter,
		MissionID:     missionID,
	}

	// --- Executor Agent (uses execute mode tools) ---
	executorModel, err := f.modelRouter.GetModel(modelrouter.AgentKindPlanner, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: get executor model: %w", err)
	}

	executorAssembledTools := f.registry.AssembleTools("workspace", "execute")
	executorTools := buildEinoToolPool(executorAssembledTools)
	executorCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "execute",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, executorAssembledTools)
	executorInstructions, err := f.compiler.CompileForEino(executorCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile executor prompt: %w", err)
	}

	executorCfg := AgentConfig{
		Kind:          AgentKindPlanner,
		Model:         executorModel,
		Instructions:  executorInstructions,
		Tools:         executorTools,
		MaxIterations: plannerMaxIter,
		MissionID:     missionID,
	}

	// --- Replanner Agent ---
	replannerModel, err := f.modelRouter.GetModel(modelrouter.AgentKindPlanner, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: get replanner model: %w", err)
	}

	replannerAssembledTools := f.registry.AssembleTools("workspace", "plan")
	replannerTools := buildEinoToolPool(replannerAssembledTools)
	replannerCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "plan",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, replannerAssembledTools)
	replannerInstructions, err := f.compiler.CompileForEino(replannerCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile replanner prompt: %w", err)
	}

	replannerCfg := AgentConfig{
		Kind:          AgentKindPlanner,
		Model:         replannerModel,
		Instructions:  replannerInstructions,
		Tools:         replannerTools,
		MaxIterations: plannerMaxIter,
		MissionID:     missionID,
	}

	return &WorkspaceAgentConfig{
		Planner:   plannerCfg,
		Executor:  executorCfg,
		Replanner: replannerCfg,
		MissionID: missionID,
	}, nil
}

// ---------------------------------------------------------------------------
// CreateWorkerAgent creates a ChatModelAgent config for a specific host worker.
// ---------------------------------------------------------------------------

// CreateWorkerAgent creates an AgentConfig for a Worker ChatModelAgent bound
// to a specific host. It filters tools to only those accessible via the bound
// host's gRPC connection, and compiles a host-specific prompt with the task.
func (f *AgentFactory) CreateWorkerAgent(ctx context.Context, hostID string, task string) (*AgentConfig, error) {
	if hostID == "" {
		return nil, fmt.Errorf("hostID is required for worker agent")
	}

	// Resolve model (worker agents may use cheaper models).
	model, err := f.modelRouter.GetModel(modelrouter.AgentKindWorker, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create worker agent: get model: %w", err)
	}

	// Compile prompt with host-specific context and task instruction.
	hostContext := fmt.Sprintf("HostID: %s\nTask: %s", hostID, task)
	assembledTools := f.assembleToolsForHost(hostID, "execute")
	compileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType: "workspace",
		Mode:        "execute",
		HostContext: hostContext,
		AgentKind:   promptcompiler.AgentKindWorker,
	}, assembledTools)
	instructions, err := f.compiler.CompileForEino(compileCtx)
	if err != nil {
		return nil, fmt.Errorf("create worker agent: compile prompt: %w", err)
	}

	tools := buildEinoToolPool(assembledTools)

	// Determine max iterations from definition.
	maxIter := 15 // default for workers
	if def := f.definitions[AgentKindWorker]; def != nil && def.MaxIterations > 0 {
		maxIter = def.MaxIterations
	}

	return &AgentConfig{
		Kind:          AgentKindWorker,
		Model:         model,
		Instructions:  instructions,
		Tools:         tools,
		MaxIterations: maxIter,
		HostID:        hostID,
	}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers for host-based tool filtering.
// ---------------------------------------------------------------------------

// filterToolsByHost returns capability entries visible in workspace/execute mode
// that are allowed for the given hostID. Tools with no host restriction or
// matching the hostID are included.
func (f *AgentFactory) filterToolsByHost(hostID string, mode string) []capability.Entry {
	all := f.registry.VisibleCapabilities("workspace", mode)
	var filtered []capability.Entry
	for _, e := range all {
		// workspace:* capabilities are not host-specific tools
		if e.Kind == capability.KindWorkspace {
			continue
		}
		// Include tools that are not host-restricted or match this host
		filtered = append(filtered, e)
	}
	return filtered
}

// assembleToolsForHost returns unified tools accessible by the given hostID.
// It uses the registry's assembled tools and filters based on the worker's
// capability scope (if a definition is registered).
func (f *AgentFactory) assembleToolsForHost(hostID string, mode string) []tooling.Tool {
	// Get all tools for workspace/execute
	allTools := f.registry.AssembleTools("workspace", mode)

	// If we have a worker definition with specific capability scope, filter.
	def := f.definitions[AgentKindWorker]
	if def == nil || len(def.CapabilityScope.Kinds) == 0 {
		return allTools
	}

	// Build allowed kinds set from the worker definition.
	allowedKinds := make(map[capability.Kind]bool)
	for _, k := range def.CapabilityScope.Kinds {
		allowedKinds[k] = true
	}

	// Filter tools by checking their entry kind against allowed kinds.
	visible := f.filterToolsByHost(hostID, mode)
	var filtered []tooling.Tool
	for _, e := range visible {
		if !allowedKinds[e.Kind] {
			continue
		}
		if e.Tool != nil {
			meta := tooling.ToolMetadata{
				Name:        e.Name,
				Description: e.Description,
				Origin:      toolOriginForKind(e.Kind),
				IsMCP:       e.Kind == capability.KindMCPTool,
			}
			filtered = append(filtered, tooling.NewLegacyToolAdapter(agentCapabilityToolBridge{
				runtime:    e.Tool,
				visibility: e.Visibility,
			}, meta))
		}
	}
	return filtered
}

type agentCapabilityToolBridge struct {
	runtime    capability.ToolRuntime
	visibility capability.Visibility
}

func (b agentCapabilityToolBridge) Description() string { return b.runtime.Description() }

func (b agentCapabilityToolBridge) InputSchema() json.RawMessage { return b.runtime.InputSchema() }

func (b agentCapabilityToolBridge) Execute(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
	res, err := b.runtime.Execute(ctx, input)
	return tooling.ToolResult{
		ToolCallID: res.ToolCallID,
		Content:    res.Content,
		Display:    convertDisplay(res.Display),
		Error:      res.Error,
	}, err
}

func (b agentCapabilityToolBridge) IsEnabled(ctx tooling.ToolContext) bool {
	if len(b.visibility.SessionTypes) == 0 && len(b.visibility.Modes) == 0 {
		return true
	}

	sessionOK := len(b.visibility.SessionTypes) == 0
	for _, st := range b.visibility.SessionTypes {
		if st == capability.SessionType(ctx.SessionType) {
			sessionOK = true
			break
		}
	}

	modeOK := len(b.visibility.Modes) == 0
	for _, m := range b.visibility.Modes {
		if m == capability.Mode(ctx.Mode) {
			modeOK = true
			break
		}
	}

	return sessionOK && modeOK
}

func (b agentCapabilityToolBridge) CheckPermissions(ctx context.Context) error {
	return b.runtime.CheckPermissions(ctx)
}

func (b agentCapabilityToolBridge) IsReadOnly() bool { return b.runtime.IsReadOnly() }

func (b agentCapabilityToolBridge) IsDestructive() bool { return b.runtime.IsDestructive() }

func (b agentCapabilityToolBridge) IsConcurrencySafe() bool { return b.runtime.IsConcurrencySafe() }

func (b agentCapabilityToolBridge) Display() tooling.ToolDisplayPayload {
	display := b.runtime.Display()
	return tooling.ToolDisplayPayload{
		Type:    display.Type,
		Title:   display.Title,
		Data:    display.Data,
		CardRef: display.CardRef,
	}
}

func convertDisplay(display *capability.ToolDisplayPayload) *tooling.ToolDisplayPayload {
	if display == nil {
		return nil
	}
	return &tooling.ToolDisplayPayload{
		Type:    display.Type,
		Title:   display.Title,
		Data:    display.Data,
		CardRef: display.CardRef,
	}
}

func toolOriginForKind(kind capability.Kind) tooling.ToolOrigin {
	if kind == capability.KindMCPTool {
		return tooling.ToolOriginMCP
	}
	return tooling.ToolOriginBuiltin
}
