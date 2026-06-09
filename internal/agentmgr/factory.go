package agentmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agents"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

type toolAssemblySource interface {
	AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool
}

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

	// AssembledTools are the unified tool descriptors used by the shared
	// runtime loop for prompt compilation and dispatch lookup.
	AssembledTools []tooling.Tool

	// MaxIterations is the maximum ReAct loop iterations.
	MaxIterations int

	// HostID is the bound host (empty for planner/workspace-level agents).
	HostID string

	// MissionID is the workspace mission ID (empty for host sessions).
	MissionID string

	// SessionID is the isolated child session used by the runtime turn.
	SessionID string

	// Input is the child turn user input/task.
	Input string

	// Metadata is propagated to the runtime turn.
	Metadata map[string]string
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

	// definitionRegistry provides a shared definition source when configured.
	definitionRegistry *agents.Registry

	// registry provides the unified tool assembly path used by agents.
	registry toolAssemblySource

	// compiler compiles structured prompts for agents.
	compiler promptcompiler.Compiler

	// modelRouter resolves ChatModel instances by AgentKind.
	modelRouter *modelrouter.Router

	// policy provides policy evaluation (used for tool filtering context).
	policy *policyengine.Engine
}

type assembledToolSet struct {
	assembled []tooling.Tool
	runtime   []tool.BaseTool
}

// NewAgentFactory creates a new AgentFactory with the given dependencies.
func NewAgentFactory(
	registry toolAssemblySource,
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

// SetDefinitionRegistry configures a shared definition registry as the primary read source.
func (f *AgentFactory) SetDefinitionRegistry(registry *agents.Registry) {
	f.definitionRegistry = registry
}

// GetDefinition returns the registered definition for the given kind, or nil.
func (f *AgentFactory) GetDefinition(kind AgentKind) *AgentDefinition {
	if f.definitionRegistry != nil {
		if def, ok := f.definitionRegistry.Get(string(kind)); ok {
			converted := FromRegistryDefinition(def)
			return &converted
		}
	}
	return f.definitions[kind]
}

func compileContextWithAssembledTools(base promptcompiler.CompileContext, tools []tooling.Tool) promptcompiler.CompileContext {
	base.AssembledTools = tools
	return base
}

func buildEinoToolPool(tools []tooling.Tool) []tool.BaseTool {
	return tooling.AssembleEinoToolPool(tools)
}

func (f *AgentFactory) assembleToolSet(session, mode string, allowedTools []string) (assembledToolSet, error) {
	opts := tooling.AssembleOptions{}
	if len(allowedTools) > 0 {
		allowed := make(map[string]struct{}, len(allowedTools))
		for _, name := range allowedTools {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			allowed[name] = struct{}{}
		}
		opts.Filter = func(_ tooling.Tool, _ tooling.ToolContext, meta tooling.ToolMetadata) bool {
			if _, ok := allowed[meta.Name]; ok {
				return true
			}
			for _, alias := range meta.Aliases {
				if _, ok := allowed[alias]; ok {
					return true
				}
			}
			return false
		}
	}

	assembled := f.registry.AssembleToolsWithOptions(session, mode, opts)
	return assembledToolSet{
		assembled: assembled,
		runtime:   buildEinoToolPool(assembled),
	}, nil
}

// ---------------------------------------------------------------------------
// CreateHostAgent creates a ChatModelAgent config for a single-host session.
// ---------------------------------------------------------------------------

// CreateHostAgent creates an AgentConfig for a host session ChatModelAgent.
// It resolves the model, compiles the prompt with host context, and assembles
// tools visible in the host session for the given mode.
func (f *AgentFactory) CreateHostAgent(ctx context.Context, hostID string, mode string) (*AgentConfig, error) {
	return f.createHostAgent(ctx, hostID, mode, "", nil)
}

func (f *AgentFactory) CreateHostChildAgent(ctx context.Context, req hostops.SpawnHostChildRequest) (*AgentConfig, error) {
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return nil, fmt.Errorf("hostID is required for host child agent")
	}
	cfg, err := f.createHostAgent(ctx, hostID, "execute", strings.TrimSpace(req.MissionID), []string{hostChildPromptAsset(req)})
	if err != nil {
		return nil, err
	}
	cfg.SessionID = strings.TrimSpace(req.SessionID)
	cfg.Input = strings.TrimSpace(req.Task)
	return cfg, nil
}

func (f *AgentFactory) createHostAgent(ctx context.Context, hostID string, mode string, missionID string, skillAssets []string) (*AgentConfig, error) {
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
	var allowedTools []string
	if def := f.GetDefinition(AgentKindWorker); def != nil {
		allowedTools = def.Tools
	}
	toolSet, err := f.assembleToolSet("host", mode, allowedTools)
	if err != nil {
		return nil, fmt.Errorf("create host agent: assemble tools: %w", err)
	}

	// Compile prompt via PromptCompiler with host context.
	compileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:       "host",
		Mode:              mode,
		HostContext:       hostID,
		SkillPromptAssets: append([]string(nil), skillAssets...),
		AgentKind:         promptcompiler.AgentKindWorker,
	}, toolSet.assembled)
	instructions, err := f.compiler.CompileForEino(compileCtx)
	if err != nil {
		return nil, fmt.Errorf("create host agent: compile prompt: %w", err)
	}

	// Determine max iterations from definition if registered.
	maxIter := 25 // default
	if def := f.GetDefinition(AgentKindWorker); def != nil && def.MaxIterations > 0 {
		maxIter = def.MaxIterations
	}

	return &AgentConfig{
		Kind:           AgentKindWorker,
		Model:          model,
		Instructions:   instructions,
		Tools:          toolSet.runtime,
		AssembledTools: toolSet.assembled,
		MaxIterations:  maxIter,
		HostID:         hostID,
		MissionID:      missionID,
	}, nil
}

func hostChildPromptAsset(req hostops.SpawnHostChildRequest) string {
	hostID := strings.TrimSpace(req.HostID)
	display := agentFirstNonEmpty(req.HostDisplayName, req.HostAddress, hostID)
	task := strings.TrimSpace(req.Task)
	if task == "" {
		task = "按 manager 分派完成本机运维任务，并回报证据。"
	}
	return fmt.Sprintf(
		"你是 host-bound 运维子 Agent。\n你的绑定主机是 %s，hostId=%s。\n你只能对这个主机执行检查、配置、安装或诊断。\n如果任务需要其他主机信息，你只能向 manager 汇报需要协调，不能直接操作其他主机。\n当前任务：%s",
		display,
		hostID,
		task,
	)
}

func agentFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

	var plannerTools []string
	if def := f.GetDefinition(AgentKindPlanner); def != nil {
		plannerTools = def.Tools
	}
	plannerToolSet, err := f.assembleToolSet("workspace", "plan", plannerTools)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: assemble planner tools: %w", err)
	}
	plannerCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "plan",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, plannerToolSet.assembled)
	plannerInstructions, err := f.compiler.CompileForEino(plannerCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile planner prompt: %w", err)
	}

	plannerMaxIter := 10
	if def := f.GetDefinition(AgentKindPlanner); def != nil && def.MaxIterations > 0 {
		plannerMaxIter = def.MaxIterations
	}

	plannerCfg := AgentConfig{
		Kind:           AgentKindPlanner,
		Model:          plannerModel,
		Instructions:   plannerInstructions,
		Tools:          plannerToolSet.runtime,
		AssembledTools: plannerToolSet.assembled,
		MaxIterations:  plannerMaxIter,
		MissionID:      missionID,
	}

	// --- Executor Agent (uses execute mode tools) ---
	executorModel, err := f.modelRouter.GetModel(modelrouter.AgentKindPlanner, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: get executor model: %w", err)
	}

	executorToolSet, err := f.assembleToolSet("workspace", "execute", plannerTools)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: assemble executor tools: %w", err)
	}
	executorCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "execute",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, executorToolSet.assembled)
	executorInstructions, err := f.compiler.CompileForEino(executorCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile executor prompt: %w", err)
	}

	executorCfg := AgentConfig{
		Kind:           AgentKindPlanner,
		Model:          executorModel,
		Instructions:   executorInstructions,
		Tools:          executorToolSet.runtime,
		AssembledTools: executorToolSet.assembled,
		MaxIterations:  plannerMaxIter,
		MissionID:      missionID,
	}

	// --- Replanner Agent ---
	replannerModel, err := f.modelRouter.GetModel(modelrouter.AgentKindPlanner, modelrouter.ProviderConfig{})
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: get replanner model: %w", err)
	}

	replannerToolSet, err := f.assembleToolSet("workspace", "plan", plannerTools)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: assemble replanner tools: %w", err)
	}
	replannerCompileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType:      "workspace",
		Mode:             "plan",
		WorkspaceContext: missionID,
		AgentKind:        promptcompiler.AgentKindPlanner,
	}, replannerToolSet.assembled)
	replannerInstructions, err := f.compiler.CompileForEino(replannerCompileCtx)
	if err != nil {
		return nil, fmt.Errorf("create workspace agent: compile replanner prompt: %w", err)
	}

	replannerCfg := AgentConfig{
		Kind:           AgentKindPlanner,
		Model:          replannerModel,
		Instructions:   replannerInstructions,
		Tools:          replannerToolSet.runtime,
		AssembledTools: replannerToolSet.assembled,
		MaxIterations:  plannerMaxIter,
		MissionID:      missionID,
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
	var allowedTools []string
	if def := f.GetDefinition(AgentKindWorker); def != nil {
		allowedTools = def.Tools
	}
	toolSet, err := f.assembleToolSet("workspace", "execute", allowedTools)
	if err != nil {
		return nil, fmt.Errorf("create worker agent: assemble tools: %w", err)
	}
	compileCtx := compileContextWithAssembledTools(promptcompiler.CompileContext{
		SessionType: "workspace",
		Mode:        "execute",
		HostContext: hostContext,
		AgentKind:   promptcompiler.AgentKindWorker,
	}, toolSet.assembled)
	instructions, err := f.compiler.CompileForEino(compileCtx)
	if err != nil {
		return nil, fmt.Errorf("create worker agent: compile prompt: %w", err)
	}

	// Determine max iterations from definition.
	maxIter := 15 // default for workers
	if def := f.GetDefinition(AgentKindWorker); def != nil && def.MaxIterations > 0 {
		maxIter = def.MaxIterations
	}

	return &AgentConfig{
		Kind:           AgentKindWorker,
		Model:          model,
		Instructions:   instructions,
		Tools:          toolSet.runtime,
		AssembledTools: toolSet.assembled,
		MaxIterations:  maxIter,
		HostID:         hostID,
		Input:          task,
	}, nil
}
