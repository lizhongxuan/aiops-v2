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
		effectiveAllowed := appendMandatoryInitialTools(allowedTools)
		allowed := make(map[string]struct{}, len(effectiveAllowed))
		opts.EnabledTools = append(opts.EnabledTools, effectiveAllowed...)
		for _, name := range effectiveAllowed {
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

func appendMandatoryInitialTools(names []string) []string {
	seen := make(map[string]struct{}, len(names)+len(tooling.MandatoryInitialToolNames()))
	out := make([]string, 0, len(names)+len(tooling.MandatoryInitialToolNames()))
	for _, name := range append(append([]string(nil), names...), tooling.MandatoryInitialToolNames()...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// ---------------------------------------------------------------------------
// CreateHostAgent creates a ChatModelAgent config for a single-host session.
// ---------------------------------------------------------------------------

// CreateHostAgent creates an AgentConfig for a host session ChatModelAgent.
// It resolves the model, compiles the prompt with host context, and assembles
// tools visible in the host session for the given mode.
func (f *AgentFactory) CreateHostAgent(ctx context.Context, hostID string, mode string) (*AgentConfig, error) {
	return f.createHostAgent(ctx, hostID, mode, "", nil, nil)
}

// CreateHostChildAgent creates a host-bound child task config.
//
// A host child is not a separate runtime class. It reuses the full Host Agent
// runtime and adds one manager-assigned host task prompt asset plus lifecycle
// metadata so traces and UI can distinguish the child task instance.
func (f *AgentFactory) CreateHostChildAgent(ctx context.Context, req hostops.SpawnHostChildRequest) (*AgentConfig, error) {
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return nil, fmt.Errorf("hostID is required for host child agent")
	}
	source := strings.TrimSpace(req.ContextDecisionTraceID)
	if source == "" {
		source = strings.TrimSpace(req.ChildAgentID)
	}
	if source == "" {
		source = "spawn-request"
	}
	cfg, err := f.createHostAgent(ctx, hostID, "execute", strings.TrimSpace(req.MissionID), nil, []string{hostChildPromptAsset(req)})
	if err != nil {
		return nil, err
	}
	cfg.SessionID = strings.TrimSpace(req.SessionID)
	cfg.Input = strings.TrimSpace(req.Task)
	if cfg.Metadata == nil {
		cfg.Metadata = map[string]string{}
	}
	cfg.Metadata["hostTaskPromptAssetSource"] = "agent-message:" + source
	cfg.Metadata["runtimeBase"] = "host_agent"
	cfg.Metadata["agentRole"] = "host_child_task"
	return cfg, nil
}

func (f *AgentFactory) createHostAgent(ctx context.Context, hostID string, mode string, missionID string, skillAssets []string, hostTaskAssets []string) (*AgentConfig, error) {
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
		SessionType:          "host",
		Mode:                 mode,
		HostContext:          hostID,
		SkillPromptAssets:    append([]string(nil), skillAssets...),
		HostTaskPromptAssets: append([]string(nil), hostTaskAssets...),
		AgentKind:            promptcompiler.AgentKindWorker,
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
		Metadata:       map[string]string{"runtimeProfile": "host_agent_full_runtime"},
	}, nil
}

type HostChildPromptContext struct {
	MissionID            string
	ChildAgentID         string
	HostID               string
	HostDisplayName      string
	Role                 string
	Goal                 string
	PlanStepID           string
	Risk                 string
	Constraints          []string
	EvidenceRequirements []string
}

func hostChildPromptAsset(req hostops.SpawnHostChildRequest) string {
	return renderHostChildPrompt(normalizeHostChildPromptContext(req))
}

func normalizeHostChildPromptContext(req hostops.SpawnHostChildRequest) HostChildPromptContext {
	ctx := HostChildPromptContext{
		MissionID:            hostPromptClean(req.MissionID),
		ChildAgentID:         hostPromptClean(req.ChildAgentID),
		HostID:               hostPromptClean(req.HostID),
		HostDisplayName:      hostPromptClean(agentFirstNonEmpty(req.HostDisplayName, req.HostAddress, req.HostID)),
		Role:                 hostPromptClean(req.Role),
		Goal:                 hostPromptClean(req.Task),
		PlanStepID:           hostPromptClean(req.PlanStepID),
		Risk:                 hostPromptClean(string(req.RiskLevel)),
		Constraints:          hostPromptCleanList(req.Constraints),
		EvidenceRequirements: hostPromptCleanList(req.EvidenceRequirements),
	}
	if ctx.Role == "" {
		ctx.Role = "host_child"
	}
	if ctx.Goal == "" {
		ctx.Goal = "operate on the assigned host and report evidence to the manager"
	}
	if ctx.PlanStepID == "" {
		ctx.PlanStepID = "unspecified"
	}
	if ctx.Risk == "" {
		ctx.Risk = "unknown"
	}
	if len(ctx.EvidenceRequirements) == 0 {
		ctx.EvidenceRequirements = []string{"command_result"}
	}
	if ctx.HostDisplayName == "" {
		ctx.HostDisplayName = ctx.HostID
	}
	return ctx
}

func renderHostChildPrompt(ctx HostChildPromptContext) string {
	sections := []string{
		renderHostChildPromptSection("Host Agent Binding", "host_agent.binding.v1", []string{
			"mission_id: " + ctx.MissionID,
			"child_agent_id: " + ctx.ChildAgentID,
			"bound_host_id: " + ctx.HostID,
			"host_display_name: " + ctx.HostDisplayName,
			"role: " + ctx.Role,
		}),
		renderHostChildPromptSection("Assigned Subtask", "host_agent.assigned_subtask.v1", []string{
			"plan_step_id: " + ctx.PlanStepID,
			"risk: " + ctx.Risk,
			"goal: " + ctx.Goal,
			"constraints: " + joinPromptList(ctx.Constraints, "none"),
			"evidence_requirements: " + joinPromptList(ctx.EvidenceRequirements, "command_result"),
		}),
		renderHostChildPromptSection("Execution Protocol", "host_agent.execution_protocol.v1", []string{
			"Use HostCommandTool for automated host commands.",
			"Do not use human terminal output as HostTaskReport evidence.",
			"Operate only within the bound host scope.",
			"Ask the manager for coordination when another host or workspace-level decision is required.",
		}),
		renderHostChildPromptSection("Report Contract", "host_agent.report_contract.v1", []string{
			"Return HostTaskReport with status, command summaries, evidence refs, errors, blockers, and next steps.",
			"Allowed status values: completed, failed, blocked, needs_manager_coordination, needs_user_approval.",
		}),
		renderHostChildPromptSection("Stop And Block Conditions", "host_agent.stop_block_conditions.v1", []string{
			"Stop before cross-host operations, missing approval, missing evidence boundary, or unclear destructive scope.",
		}),
	}
	return strings.Join(sections, "\n\n")
}

func renderHostChildPromptSection(title, id string, lines []string) string {
	cleaned := make([]string, 0, len(lines)+2)
	cleaned = append(cleaned, "## "+title, "prompt_section_id: "+id)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

func hostPromptClean(value string) string {
	return hostops.RedactSensitiveText(strings.TrimSpace(value))
}

func hostPromptCleanList(values []string) []string {
	out := cleanStringList(values)
	for i := range out {
		out[i] = hostPromptClean(out[i])
	}
	return out
}

func joinPromptList(values []string, fallback string) string {
	values = cleanStringList(values)
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
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
// CreateWorkerAgent creates a compatibility host worker config.
// ---------------------------------------------------------------------------

// CreateWorkerAgent is a compatibility entry point for older worker-agent
// callers. New code should use CreateHostAgent for interactive host sessions or
// CreateHostChildAgent for manager-assigned host subtasks.
//
// Deprecated: use CreateHostAgent or CreateHostChildAgent.
func (f *AgentFactory) CreateWorkerAgent(ctx context.Context, hostID string, task string) (*AgentConfig, error) {
	if hostID == "" {
		return nil, fmt.Errorf("hostID is required for worker agent")
	}
	cfg, err := f.createHostAgent(ctx, hostID, "execute", "", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create worker agent: %w", err)
	}
	cfg.Input = strings.TrimSpace(task)
	if cfg.Metadata == nil {
		cfg.Metadata = map[string]string{}
	}
	cfg.Metadata["compatibilityEntry"] = "CreateWorkerAgent"
	return cfg, nil
}
