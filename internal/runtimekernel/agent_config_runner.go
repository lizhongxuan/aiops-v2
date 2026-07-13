package runtimekernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentruntime"
	evidencecore "aiops-v2/internal/evidence"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/tooling"
)

// AgentConfigRunner executes assembled child-agent configs through the same
// RunTurn path used by the main AI Chat agent.
type AgentConfigRunner struct {
	cfg AgentConfigRunnerConfig
}

// AgentConfigRunnerConfig contains optional runtime dependencies shared with
// the main kernel. Nil dependencies fall back to no-op/default values.
type AgentConfigRunnerConfig struct {
	Policy           *policyengine.Engine
	Permissions      *permissions.Engine
	Hooks            *hooks.Registry
	Projector        EventEmitter
	Sessions         *SessionManager
	SessionRepo      SessionRepository
	SpanSource       SpanStreamSource
	Observer         Observer
	ResourceLockGate ToolResourceLockGate
	Compressor       *spanstream.ContextCompressor
	SpillRepo        ToolResultSpillRepository
	ArtifactRepo     ContextArtifactRepository
	SkillRegistry    *skills.Registry
	EvidenceService  *evidencecore.Service
}

// NewAgentConfigRunner constructs a runner for AgentManager.
func NewAgentConfigRunner(cfg AgentConfigRunnerConfig) *AgentConfigRunner {
	if cfg.Policy == nil {
		cfg.Policy = &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()}
	}
	if cfg.Projector == nil {
		cfg.Projector = noopEventEmitter{}
	}
	return &AgentConfigRunner{cfg: cfg}
}

// Run executes one child-agent turn and returns the final assistant output.
func (r *AgentConfigRunner) Run(ctx context.Context, config agentruntime.Config) (string, error) {
	if r == nil {
		return "", fmt.Errorf("agent config runner is required")
	}
	if config == nil {
		return "", fmt.Errorf("agent config is required")
	}
	model := config.RuntimeModel()
	if model == nil {
		return "", fmt.Errorf("agent model is required")
	}
	assembledTools := append([]tooling.Tool(nil), config.RuntimeAssembledTools()...)
	if len(assembledTools) == 0 {
		assembledTools = assembledToolsFromRuntimePool(config.RuntimeTools())
	}

	sessionType, mode := childRuntimeSession(config)
	sessionID := strings.TrimSpace(config.RuntimeSessionID())
	if sessionID == "" {
		sessionID = defaultChildSessionID(config)
	}
	input := strings.TrimSpace(config.RuntimeInput())
	if input == "" {
		input = "执行子 Agent 任务，并返回最终结果。"
	}
	runtimeMetadata := childRuntimeMetadata(config)

	provider := "agent-config"
	router := modelrouter.NewRouter(provider, map[string]modelrouter.ChatModel{provider: model}, nil)
	router.SetProviderConfigResolver(agentConfigProviderResolver{
		provider:        provider,
		model:           firstNonEmpty(strings.TrimSpace(config.RuntimeKind()), "child-agent"),
		reasoningEffort: firstMetadataValue(runtimeMetadata, "reasoningEffort", "reasoning_effort"),
	})
	instructionContext := classifyFixedAgentInstructions(config.RuntimeInstructions())

	kernel := NewRuntimeKernel(RuntimeKernelConfig{
		ToolSource:       fixedAgentToolSource{sessionType: sessionType, mode: mode, assembledTools: assembledTools, runtimeTools: config.RuntimeTools(), metadata: runtimeMetadata, config: config},
		Compiler:         fixedAgentCompiler{roleContent: instructionContext.Role, dynamicContent: instructionContext.Dynamic},
		Policy:           r.cfg.Policy,
		Permissions:      r.cfg.Permissions,
		Hooks:            r.cfg.Hooks,
		Projector:        r.cfg.Projector,
		ModelRouter:      router,
		Sessions:         r.cfg.Sessions,
		SessionRepo:      r.cfg.SessionRepo,
		SpanSource:       r.cfg.SpanSource,
		Observer:         r.cfg.Observer,
		ResourceLockGate: r.cfg.ResourceLockGate,
		Compressor:       r.cfg.Compressor,
		SpillRepo:        r.cfg.SpillRepo,
		ArtifactRepo:     r.cfg.ArtifactRepo,
		SkillRegistry:    r.cfg.SkillRegistry,
		EvidenceService:  r.cfg.EvidenceService,
	})

	result, err := kernel.RunTurn(ctx, TurnRequest{
		SessionType: sessionType,
		Mode:        mode,
		SessionID:   sessionID,
		TurnID:      "agent-turn-" + safeID(time.Now().UTC().Format("20060102150405.000000000")),
		Input:       input,
		HostID:      strings.TrimSpace(config.RuntimeHostID()),
		Metadata:    runtimeMetadata,
	})
	if err != nil {
		return "", err
	}
	if result.Status != "completed" {
		return result.Output, fmt.Errorf("agent turn %s: %s", result.Status, result.Error)
	}
	return result.Output, nil
}

type fixedAgentToolSource struct {
	sessionType    SessionType
	mode           Mode
	assembledTools []tooling.Tool
	runtimeTools   []tool.BaseTool
	metadata       map[string]string
	config         agentruntime.Config
}

func (s fixedAgentToolSource) CompileContext(session SessionType, mode Mode) promptcompiler.CompileContext {
	return s.compileContext(session, mode, nil)
}

func (s fixedAgentToolSource) CompileContextWithMetadata(session SessionType, mode Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.compileContext(session, mode, metadata).AssembledTools
}

func (s fixedAgentToolSource) AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool {
	if len(s.runtimeTools) > 0 {
		return append([]tool.BaseTool(nil), s.runtimeTools...)
	}
	return tooling.AssembleEinoToolPool(s.assembledTools)
}

func (s fixedAgentToolSource) AssembleToolPoolWithMetadata(session SessionType, mode Mode, _ map[string]string) []tool.BaseTool {
	return s.AssembleToolPool(session, mode)
}

func (s fixedAgentToolSource) compileContext(session SessionType, mode Mode, metadata map[string]string) promptcompiler.CompileContext {
	if !session.IsValid() {
		session = s.sessionType
	}
	if !mode.IsValid() {
		mode = s.mode
	}
	merged := copyStringMap(s.metadata)
	for k, v := range metadata {
		merged[k] = v
	}
	ctx := promptcompiler.CompileContext{
		SessionType:    string(session),
		Mode:           string(mode),
		AssembledTools: append([]tooling.Tool(nil), s.assembledTools...),
		HostContext:    strings.TrimSpace(s.config.RuntimeHostID()),
		WorkspaceContext: strings.TrimSpace(firstNonEmpty(
			s.config.RuntimeMissionID(),
			merged["missionId"],
			merged["missionID"],
		)),
		AgentKind: promptcompiler.AgentKindWorker,
	}
	if session == SessionTypeWorkspace && strings.EqualFold(strings.TrimSpace(s.config.RuntimeKind()), "planner") {
		ctx.AgentKind = promptcompiler.AgentKindPlanner
	}
	return ctx
}

type fixedAgentCompiler struct {
	roleContent    string
	dynamicContent string
}

func (c fixedAgentCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	roleContent := "你是一个子 Agent，按 manager 分派的任务执行并返回可核验结果。"
	if typedRole := strings.TrimSpace(c.roleContent); typedRole != "" {
		roleContent += "\n" + typedRole
	}
	systemContent := roleContent
	ctx.ExtraSections = append([]promptcompiler.PromptSection(nil), ctx.ExtraSections...)
	if dynamicContent := strings.TrimSpace(c.dynamicContent); dynamicContent != "" {
		ctx.ExtraSections = append(ctx.ExtraSections, promptcompiler.PromptSection{
			Title: "Delegated Agent Context", Content: dynamicContent,
		})
	}
	toolContent := toolPromptContent(ctx.AssembledTools)
	compiled := promptcompiler.CompiledPrompt{
		Stable: promptcompiler.StablePromptEnvelope{
			Content: systemContent + "\n\n" + toolContent,
			System:  promptcompiler.SystemPrompt{Content: systemContent, Role: roleContent},
			Tools:   promptcompiler.ToolPromptSet{Content: toolContent},
		},
		Dynamic: promptcompiler.DynamicPromptDelta{
			Content: ctx.RuntimePolicy,
			Policy:  promptcompiler.RuntimePolicyPrompt{Content: ctx.RuntimePolicy, Mode: ctx.Mode},
		},
		System: promptcompiler.SystemPrompt{Content: systemContent, Role: roleContent},
		Tools:  promptcompiler.ToolPromptSet{Content: toolContent},
		Policy: promptcompiler.RuntimePolicyPrompt{Content: ctx.RuntimePolicy, Mode: ctx.Mode},
	}
	compiled.Envelope = promptcompiler.PromptEnvelope{Sections: []promptcompiler.PromptCompiledSection{
		{
			ID:        "base.contract",
			Layer:     promptcompiler.PromptSectionKindStable,
			Role:      "system",
			Content:   systemContent,
			Stability: promptcompiler.PromptSectionKindStable,
			Source:    "child_agent",
			Required:  true,
		},
		{
			ID:        "tool.surface",
			Layer:     promptcompiler.PromptSectionKindStable,
			Role:      "system",
			Content:   toolContent,
			Stability: promptcompiler.PromptSectionKindStable,
			Source:    "tools",
			Required:  true,
		},
		{
			ID:        "runtime.state",
			Layer:     promptcompiler.PromptSectionKindDynamic,
			Role:      "system",
			Content:   ctx.RuntimePolicy,
			Stability: promptcompiler.PromptSectionKindDynamic,
			Source:    "runtime",
			Required:  true,
		},
	}}
	compiled.Fingerprint = promptcompiler.BuildPromptFingerprintForAdapter(compiled)
	compiled.PromptSections = promptcompiler.BuildPromptSectionTrace(compiled)
	compiled.EnvelopeV2 = promptcompiler.BuildPromptEnvelopeV2(compiled, ctx)
	if err := compiled.EnvelopeV2.Validate(); err != nil {
		return promptcompiler.CompiledPrompt{}, fmt.Errorf("compile child prompt envelope v2: %w", err)
	}
	return compiled, nil
}

type fixedAgentInstructionContext struct {
	Role    string
	Dynamic string
}

func classifyFixedAgentInstructions(messages []*schema.Message) fixedAgentInstructionContext {
	roleMessages := make([]*schema.Message, 0, len(messages))
	dynamicMessages := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil || strings.TrimSpace(message.Content) == "" {
			continue
		}
		sectionID := fixedAgentInstructionExtra(message, "source_section_id")
		layer := fixedAgentInstructionExtra(message, "source_layer")
		if layer == string(promptcompiler.LayerRoleProfileCore) {
			roleMessages = append(roleMessages, message)
			continue
		}
		if fixedAgentInstructionIsRebuilt(sectionID, layer) {
			continue
		}
		dynamicMessages = append(dynamicMessages, message)
	}
	return fixedAgentInstructionContext{
		Role:    modelrouter.EinoInstructionMessagesText(roleMessages),
		Dynamic: modelrouter.EinoInstructionMessagesText(dynamicMessages),
	}
}

func fixedAgentInstructionExtra(message *schema.Message, key string) string {
	if message == nil || message.Extra == nil {
		return ""
	}
	value, _ := message.Extra[key].(string)
	return strings.TrimSpace(value)
}

func fixedAgentInstructionIsRebuilt(sectionID, layer string) bool {
	if sectionID == "base.contract" || strings.HasPrefix(sectionID, "profile.") || sectionID == "tool.surface" || sectionID == "runtime.state" {
		return true
	}
	switch promptcompiler.PromptLogicalLayer(layer) {
	case promptcompiler.LayerAbsoluteSystemCore, promptcompiler.LayerStableRuntimeContract, promptcompiler.LayerTurnStableFacts:
		return true
	default:
		return false
	}
}

type agentConfigProviderResolver struct {
	provider        string
	model           string
	reasoningEffort string
}

func (r agentConfigProviderResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	return modelrouter.ProviderConfig{
		Provider:         r.provider,
		Model:            r.model,
		MaxContextTokens: DefaultMaxTokens,
		MaxTokens:        20000,
		ReasoningEffort:  r.reasoningEffort,
	}, true
}

type noopEventEmitter struct{}

func (noopEventEmitter) Emit(LifecycleEvent) {}

func childRuntimeSession(config agentruntime.Config) (SessionType, Mode) {
	if strings.TrimSpace(config.RuntimeHostID()) != "" {
		return SessionTypeHost, ModeExecute
	}
	if strings.EqualFold(strings.TrimSpace(config.RuntimeKind()), "planner") {
		return SessionTypeWorkspace, ModePlan
	}
	return SessionTypeWorkspace, ModeExecute
}

func defaultChildSessionID(config agentruntime.Config) string {
	parts := []string{"agent"}
	if hostID := strings.TrimSpace(config.RuntimeHostID()); hostID != "" {
		parts = append(parts, "host", hostID)
	} else if missionID := strings.TrimSpace(config.RuntimeMissionID()); missionID != "" {
		parts = append(parts, "mission", missionID)
	} else {
		parts = append(parts, "run", time.Now().UTC().Format("20060102150405.000000000"))
	}
	return safeID(strings.Join(parts, ":"))
}

func childRuntimeMetadata(config agentruntime.Config) map[string]string {
	metadata := copyStringMap(config.RuntimeMetadata())
	if kind := strings.TrimSpace(config.RuntimeKind()); kind != "" {
		metadata["agentKind"] = kind
	}
	if hostID := strings.TrimSpace(config.RuntimeHostID()); hostID != "" {
		metadata["hostId"] = hostID
	}
	if missionID := strings.TrimSpace(config.RuntimeMissionID()); missionID != "" {
		metadata["missionId"] = missionID
	}
	return metadata
}

func toolPromptContent(tools []tooling.Tool) string {
	var builder strings.Builder
	for _, t := range tools {
		if t == nil {
			continue
		}
		meta := t.Metadata()
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(name)
		if desc := strings.TrimSpace(meta.Description); desc != "" {
			builder.WriteString(": ")
			builder.WriteString(desc)
		}
	}
	return strings.TrimSpace(builder.String())
}

func promptSectionsText(sections []promptcompiler.PromptSection) string {
	var builder strings.Builder
	for _, section := range sections {
		if strings.TrimSpace(section.Content) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		if title := strings.TrimSpace(section.Title); title != "" {
			builder.WriteString(title)
			builder.WriteString("\n")
		}
		builder.WriteString(section.Content)
	}
	return strings.TrimSpace(builder.String())
}

func assembledToolsFromRuntimePool(tools []tool.BaseTool) []tooling.Tool {
	if len(tools) == 0 {
		return nil
	}
	assembled := make([]tooling.Tool, 0, len(tools))
	for _, runtimeTool := range tools {
		if runtimeTool == nil {
			continue
		}
		info, err := runtimeTool.Info(context.Background())
		if err != nil || info == nil || strings.TrimSpace(info.Name) == "" {
			continue
		}
		assembled = append(assembled, &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        info.Name,
				Description: info.Desc,
			},
		})
	}
	return assembled
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "agent"
	}
	var builder strings.Builder
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '-', ch == '_', ch == ':':
			builder.WriteRune(ch)
		default:
			builder.WriteByte('-')
		}
	}
	return builder.String()
}
