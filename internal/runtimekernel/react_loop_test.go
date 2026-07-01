package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
)

type sequentialLoopModel struct {
	responses   []*schema.Message
	inputs      [][]*schema.Message
	boundTools  [][]string
	generateErr error
}

func (m *sequentialLoopModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if len(m.responses) == 0 {
		return nil, errors.New("no more responses configured")
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return cloneSchemaMessage(resp), nil
}

func (m *sequentialLoopModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if len(m.responses) == 0 {
		return nil, errors.New("no more responses configured")
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return schema.StreamReaderFromArray([]*schema.Message{cloneSchemaMessage(resp)}), nil
}

func (m *sequentialLoopModel) BindTools(tools []*schema.ToolInfo) error {
	names := make([]string, 0, len(tools))
	for _, info := range tools {
		if info == nil {
			continue
		}
		names = append(names, info.Name)
	}
	m.boundTools = append(m.boundTools, names)
	return nil
}

func TestRunTurnWritesSingleAssistantMessageForNoToolFinal(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("这是最终回答。", nil),
		},
	}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-single-assistant-message-final",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-single-assistant-message-final",
		Input:       "直接回答",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-single-assistant-message-final")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	assertNoLegacyAssistantItems(t, session.CurrentTurn.AgentItems)
	finals := assistantMessagesByPhase(t, session.CurrentTurn.AgentItems, string(AssistantMessagePhaseFinalAnswer))
	if len(finals) != 1 {
		t.Fatalf("final assistant messages = %#v, want one", finals)
	}
	if got := strings.TrimSpace(finals[0].Payload.Summary); got != "这是最终回答。" {
		t.Fatalf("final text = %q", got)
	}
}

func TestRunTurnCompletesCommentaryMessageBeforeToolCall(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("我先查公开来源。", []schema.ToolCall{{
				ID:   "call-web",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "web_search",
					Arguments: `{"query":"postgres timeline"}`,
				},
			}}),
			schema.AssistantMessage("基于搜索结果，这是最终回答。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Description: "Search public web pages",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "search result"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-single-assistant-message-tool",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-single-assistant-message-tool",
		Input:       "分析 timeline",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-single-assistant-message-tool")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	items := session.CurrentTurn.AgentItems
	assertNoLegacyAssistantItems(t, items)
	commentary := assistantMessagesByPhase(t, items, string(AssistantMessagePhaseCommentary))
	finals := assistantMessagesByPhase(t, items, string(AssistantMessagePhaseFinalAnswer))
	if len(commentary) != 1 || len(finals) != 1 {
		t.Fatalf("commentary=%#v finals=%#v, want one each", commentary, finals)
	}
	if got := strings.TrimSpace(commentary[0].Payload.Summary); got != "我先查公开来源。" {
		t.Fatalf("commentary text = %q", got)
	}
	if got := strings.TrimSpace(finals[0].Payload.Summary); got != "基于搜索结果，这是最终回答。" {
		t.Fatalf("final text = %q", got)
	}
}

type timeoutThenSuccessModel struct {
	inputs   [][]*schema.Message
	attempts int
	sawTool  bool
}

func (m *timeoutThenSuccessModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	if m.attempts == 2 {
		return nil, context.DeadlineExceeded
	}
	return schema.AssistantMessage("recovered after timeout", nil), nil
}

func (m *timeoutThenSuccessModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	m.attempts++
	switch m.attempts {
	case 1:
		return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-timeout-evidence",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"timeout recovery evidence"}`,
			},
		}})}), nil
	case 2:
		return nil, context.DeadlineExceeded
	}
	for _, msg := range input {
		if msg != nil && msg.Role == schema.Tool && strings.Contains(msg.Content, "prior evidence survives timeout") {
			m.sawTool = true
		}
	}
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("recovered after timeout", nil)}), nil
}

func (m *timeoutThenSuccessModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func cloneSchemaMessages(messages []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, cloneSchemaMessage(msg))
	}
	return out
}

func cloneSchemaMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	cp := *msg
	if len(msg.ToolCalls) > 0 {
		cp.ToolCalls = append([]schema.ToolCall(nil), msg.ToolCalls...)
	}
	return &cp
}

func schemaMessagesText(messages []*schema.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		builder.WriteString(string(msg.Role))
		builder.WriteString(": ")
		builder.WriteString(msg.Content)
		builder.WriteByte('\n')
	}
	return builder.String()
}

type fixedSummaryModel struct {
	response string
}

func (m *fixedSummaryModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: m.response}, nil
}

func (m *fixedSummaryModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: m.response}}), nil
}

func (m *fixedSummaryModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

type streamingFinalLoopModel struct {
	chunks     []*schema.Message
	inputs     [][]*schema.Message
	boundTools [][]string
}

func (m *streamingFinalLoopModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called for streaming final responses")
}

func (m *streamingFinalLoopModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	sr, sw := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer sw.Close()
		for _, chunk := range m.chunks {
			sw.Send(cloneSchemaMessage(chunk), nil)
		}
	}()
	return sr, nil
}

func (m *streamingFinalLoopModel) BindTools(tools []*schema.ToolInfo) error {
	names := make([]string, 0, len(tools))
	for _, info := range tools {
		if info == nil {
			continue
		}
		names = append(names, info.Name)
	}
	m.boundTools = append(m.boundTools, names)
	return nil
}

type partialStreamErrorLoopModel struct {
	chunks []*schema.Message
	err    error
	inputs [][]*schema.Message
}

func (m *partialStreamErrorLoopModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate fallback should not be called after stream error")
}

func (m *partialStreamErrorLoopModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	sr, sw := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer sw.Close()
		for _, chunk := range m.chunks {
			sw.Send(cloneSchemaMessage(chunk), nil)
		}
		sw.Send(nil, m.err)
	}()
	return sr, nil
}

func (m *partialStreamErrorLoopModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

type gatedStreamingFinalLoopModel struct {
	firstSent chan struct{}
	release   chan struct{}
}

func (m *gatedStreamingFinalLoopModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called for gated streaming final responses")
}

func (m *gatedStreamingFinalLoopModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		defer sw.Close()
		sw.Send(schema.AssistantMessage("第一段", nil), nil)
		close(m.firstSent)
		<-m.release
		sw.Send(schema.AssistantMessage("第二段", nil), nil)
	}()
	return sr, nil
}

func (m *gatedStreamingFinalLoopModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

type memoryToolResultSpillRepo struct {
	spills map[string]*tooling.ResultSpill
}

func newMemoryToolResultSpillRepo() *memoryToolResultSpillRepo {
	return &memoryToolResultSpillRepo{spills: make(map[string]*tooling.ResultSpill)}
}

func (r *memoryToolResultSpillRepo) GetToolResultSpill(id string) (*tooling.ResultSpill, error) {
	spill, ok := r.spills[id]
	if !ok {
		return nil, errors.New("spill not found")
	}
	cp := *spill
	cp.Content = append([]byte(nil), spill.Content...)
	return &cp, nil
}

func (r *memoryToolResultSpillRepo) ListToolResultSpills() ([]*tooling.ResultSpill, error) {
	out := make([]*tooling.ResultSpill, 0, len(r.spills))
	for _, spill := range r.spills {
		cp := *spill
		cp.Content = append([]byte(nil), spill.Content...)
		out = append(out, &cp)
	}
	return out, nil
}

func (r *memoryToolResultSpillRepo) SaveToolResultSpill(spill *tooling.ResultSpill) error {
	cp := *spill
	cp.Content = append([]byte(nil), spill.Content...)
	r.spills[spill.ID] = &cp
	return nil
}

func (r *memoryToolResultSpillRepo) DeleteToolResultSpill(id string) error {
	delete(r.spills, id)
	return nil
}

type panickingAgentManager struct{}

func (p *panickingAgentManager) CreateWorkspaceAgent(context.Context, string) error {
	panic("legacy workspace agent path should not be called")
}

func (p *panickingAgentManager) SpawnAndRunPlanner(context.Context, string, string, string) (string, error) {
	panic("legacy workspace planner path should not be called")
}

func (p *panickingAgentManager) CollectResults(string) []AgentResult {
	panic("legacy workspace result collection should not be called")
}

type assemblerBackedToolSource struct {
	assembler *tooling.Assembler
}

func (s *assemblerBackedToolSource) CompileContext(session SessionType, mode Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{
		SessionType:    string(session),
		Mode:           string(mode),
		AssembledTools: s.assembler.AssembleToolsWithOptions(string(session), string(mode), tooling.AssembleOptions{}),
	}
}

func (s *assemblerBackedToolSource) AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool {
	return s.assembler.AssembleToolPoolWithOptions(string(session), string(mode), tooling.AssembleOptions{})
}

func (s *assemblerBackedToolSource) CompileContextWithMetadata(session SessionType, mode Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.assembler.CompileContextWithMetadata(string(session), string(mode), metadata)
}

func (s *assemblerBackedToolSource) AssembleToolPoolWithMetadata(session SessionType, mode Mode, metadata map[string]string) []tool.BaseTool {
	return s.assembler.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

func (s *assemblerBackedToolSource) AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool {
	return s.assembler.AssembleToolsWithOptions(session, mode, opts)
}

func (s *assemblerBackedToolSource) RefreshToken(session SessionType, mode Mode) string {
	return s.assembler.RefreshToken(string(session), string(mode), tooling.AssembleOptions{})
}

type recordingCompiler struct {
	delegate promptcompiler.Compiler
	contexts []promptcompiler.CompileContext
}

func newRecordingCompiler() *recordingCompiler {
	return &recordingCompiler{delegate: promptcompiler.NewCompiler()}
}

func (c *recordingCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	cloned := ctx
	cloned.AssembledTools = append([]promptcompiler.Tool(nil), ctx.AssembledTools...)
	cloned.SkillPromptAssets = append([]string(nil), ctx.SkillPromptAssets...)
	cloned.HostTaskPromptAssets = append([]string(nil), ctx.HostTaskPromptAssets...)
	cloned.EvidenceReminders = append([]string(nil), ctx.EvidenceReminders...)
	cloned.ExtraSections = append([]promptcompiler.PromptSection(nil), ctx.ExtraSections...)
	cloned.ToolDelta = promptcompiler.ToolPromptDelta{
		NewlyAvailable:         append([]string(nil), ctx.ToolDelta.NewlyAvailable...),
		NewlyAvailablePacks:    append([]string(nil), ctx.ToolDelta.NewlyAvailablePacks...),
		TemporarilyUnavailable: append([]string(nil), ctx.ToolDelta.TemporarilyUnavailable...),
		ApprovalRequired:       append([]string(nil), ctx.ToolDelta.ApprovalRequired...),
		Content:                ctx.ToolDelta.Content,
	}
	c.contexts = append(c.contexts, cloned)
	return c.delegate.Compile(ctx)
}

func newKernelForLoopTests(
	t *testing.T,
	source ToolAssemblySource,
	compiler promptcompiler.Compiler,
	chatModel modelrouter.ChatModel,
) (*RuntimeKernel, *testMockEventEmitter) {
	t.Helper()

	policy := &policyengine.Engine{
		ModePolicy:       policyengine.NewDefaultModePolicies(),
		CompletionPolicy: &testMockCompletionEvaluator{action: policyengine.PolicyActionAllow},
	}
	projector := &testMockEventEmitter{}
	router := modelrouter.NewRouter("mock", map[string]modelrouter.ChatModel{"mock": chatModel}, nil)
	router.SetProviderConfigResolver(testProviderConfigResolver{config: modelrouter.ProviderConfig{Provider: "mock", Model: "mock", MaxContextTokens: DefaultMaxTokens}})
	return NewRuntimeKernel(RuntimeKernelConfig{
		ToolSource:  source,
		Compiler:    compiler,
		Policy:      policy,
		Projector:   projector,
		ModelRouter: router,
		DebugConfig: func(context.Context) RuntimeDebugConfig {
			return runtimeDebugConfigForLegacyTraceTest()
		},
	}), projector
}

func newLoopKernel(t *testing.T, chatModel modelrouter.ChatModel, tools []tooling.Tool, hookRegistry *hooks.Registry, modePolicies map[policyengine.Mode]policyengine.ModePolicy) *RuntimeKernel {
	return newLoopKernelWithDeps(t, chatModel, tools, hookRegistry, modePolicies, nil, nil)
}

func newLoopKernelWithDeps(
	t *testing.T,
	chatModel modelrouter.ChatModel,
	tools []tooling.Tool,
	hookRegistry *hooks.Registry,
	modePolicies map[policyengine.Mode]policyengine.ModePolicy,
	compressor *spanstream.ContextCompressor,
	spillRepo ToolResultSpillRepository,
) *RuntimeKernel {
	t.Helper()

	registry := tooling.NewRegistry()
	for _, toolDef := range tools {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}

	if modePolicies == nil {
		modePolicies = policyengine.NewDefaultModePolicies()
	}
	policy := &policyengine.Engine{
		ModePolicy:       modePolicies,
		CompletionPolicy: &testMockCompletionEvaluator{action: policyengine.PolicyActionAllow},
	}
	projector := &testMockEventEmitter{}
	router := modelrouter.NewRouter("mock", map[string]modelrouter.ChatModel{"mock": chatModel}, nil)
	router.SetProviderConfigResolver(testProviderConfigResolver{config: modelrouter.ProviderConfig{Provider: "mock", Model: "mock", MaxContextTokens: DefaultMaxTokens}})

	return NewRuntimeKernel(RuntimeKernelConfig{
		ToolSource:  &testMockToolAssemblySource{registry: registry},
		Compiler:    &testMockCompiler{},
		Policy:      policy,
		Hooks:       hookRegistry,
		Projector:   projector,
		ModelRouter: router,
		Compressor:  compressor,
		SpillRepo:   spillRepo,
		DebugConfig: func(context.Context) RuntimeDebugConfig {
			return runtimeDebugConfigForLegacyTraceTest()
		},
	})
}

func TestRunTurn_InjectsPlanStateIntoNextProtocolPrompt(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "plan-call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "update_plan",
						Arguments: `{"steps":[{"id":"inspect","text":"Inspect host symptoms","status":"in_progress"},{"id":"summarize","text":"Summarize findings","status":"pending"}]}`,
					},
				},
			}),
			schema.AssistantMessage("缺少执行结果，计划仍在进行。", nil),
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(planning.NewUpdatePlanTool()); err != nil {
		t.Fatalf("Register update_plan failed: %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-plan-protocol",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-plan-protocol",
		Input:       "triage this incident",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if len(compiler.contexts) < 2 {
		t.Fatalf("compiler contexts = %d, want at least 2", len(compiler.contexts))
	}
	if hasProtocolKind(compiler.contexts[0].ProtocolState, "plan") {
		t.Fatalf("first model call should not include a plan state: %#v", compiler.contexts[0].ProtocolState)
	}
	second := compiler.contexts[1].ProtocolState
	if !hasProtocolItem(second, "plan", "inspect", "in_progress", "Inspect host symptoms") {
		t.Fatalf("second protocol state = %#v, want inspect plan item", second)
	}
	if !hasProtocolItem(second, "plan", "summarize", "pending", "Summarize findings") {
		t.Fatalf("second protocol state = %#v, want summarize plan item", second)
	}
}

func TestComplexTaskPrematureFinalContinuesWithGuard(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("可能是外部依赖变慢，建议检查。", nil),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-plan",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "update_plan",
				Arguments: `{"steps":[{"text":"确认症状和时间窗","status":"in_progress"}]}`,
			},
		}}),
		schema.AssistantMessage("结论（置信度：低）：还需要证据。关键证据：已建立调查计划。仍缺少的证据：指标和日志。", nil),
	}}
	registry := tooling.NewRegistry()
	if err := registry.Register(planning.NewUpdatePlanTool()); err != nil {
		t.Fatalf("Register update_plan failed: %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   "sess-depth-guard",
		TurnID:      "turn-depth-guard",
		Input:       "排查目标服务关键指标异常的根因",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Output == "可能是外部依赖变慢，建议检查。" {
		t.Fatalf("premature final should not complete as output")
	}
	if len(model.inputs) < 2 {
		t.Fatalf("model inputs = %d, want continuation after guard", len(model.inputs))
	}
	if got := schemaMessagesText(model.inputs[1]); !strings.Contains(got, "Premature final answer guard") {
		t.Fatalf("second model input missing guard:\n%s", got)
	}
}

func TestComplexTaskPrematureFinalHardBlocksRepeatedNoEvidenceFinal(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("Docker 已安装且运行正常。", nil),
		schema.AssistantMessage("Docker 已安装且运行正常。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		SessionID:   "sess-depth-no-evidence-final",
		TurnID:      "turn-depth-no-evidence-final",
		Input:       "检查这台主机 Docker 是否已安装并可用，只做只读检查",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !strings.Contains(result.Output, "缺少直接工具证据") {
		t.Fatalf("result output = %q, want missing evidence blocker", result.Output)
	}
	if strings.Contains(result.Output, "Docker 已安装且运行正常") {
		t.Fatalf("result output = %q, should not pass through unsupported success final", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want one retry before hard blocker", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-depth-no-evidence-final")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.Metadata["taskDepth.missingEvidenceFinalBlocked"] != "true" {
		t.Fatalf("missing hard-block metadata: %#v", session)
	}
}

func TestSimpleQuestionFinalDoesNotTriggerPrematureFinalGuard(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("AIOps 是智能运维。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   "sess-simple-final",
		TurnID:      "turn-simple-final",
		Input:       "AIOps 是什么？",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Output != "AIOps 是智能运维。" {
		t.Fatalf("output = %q", result.Output)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model inputs = %d, want one direct final", len(model.inputs))
	}
}

func TestRunTurn_CompletesFromToolEvidenceWhenModelStaysEmpty(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-model-config",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "get_current_model_config",
				Arguments: `{}`,
			},
		}}),
		{Role: schema.Assistant},
		{Role: schema.Assistant},
		{Role: schema.Assistant},
		{Role: schema.Assistant},
		{Role: schema.Assistant},
	}}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:      "get_current_model_config",
			Layer:     tooling.ToolLayerCore,
			RiskLevel: tooling.ToolRiskLow,
		},
		InputSchemaData: json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:    func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: `{"apiKeySet":true,"baseURL":"https://example.invalid/v1","model":"glm-4.7","provider":"zhipu"}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   "sess-empty-after-tool",
		TurnID:      "turn-empty-after-tool",
		Input:       "Tell me current model name only. Do not reveal or mention any api key.",
		Metadata:    map[string]string{"taskDepth": "simple_read"},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed; result=%#v", result.Status, result)
	}
	if !strings.Contains(result.Output, "glm-4.7") {
		t.Fatalf("output = %q, want model evidence", result.Output)
	}
	if strings.Contains(strings.ToLower(result.Output), "apikey") || strings.Contains(result.Output, "example.invalid") {
		t.Fatalf("output leaked sensitive or irrelevant config details: %q", result.Output)
	}
}

func TestSynthesisOnlyThresholdUsesTaskDepth(t *testing.T) {
	tools := []promptcompiler.Tool{&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_metrics"}}}
	if shouldSwitchToSynthesisOnly(ModeChat, taskdepth.Profile{Level: taskdepth.LevelInvestigation}, 4, tools) {
		t.Fatal("investigation should not switch to synthesis-only before 5 dispatches")
	}
	if !shouldSwitchToSynthesisOnly(ModeChat, taskdepth.Profile{Level: taskdepth.LevelInvestigation}, 5, tools) {
		t.Fatal("investigation should switch at depth-aware threshold 5")
	}
	if !shouldSwitchToSynthesisOnly(ModeChat, taskdepth.Profile{Level: taskdepth.LevelSimpleRead}, 5, tools) {
		t.Fatal("simple read should keep current low threshold")
	}
}

func TestPublicWebToolNamesAlsoHideDiscoverySearch(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "web_search", Pack: "public_web"}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "browse_url", Pack: "public_web"}},
	}
	names := publicWebToolNames(tools)
	for _, want := range []string{"web_search", "browse_url"} {
		if !containsString(names, want) {
			t.Fatalf("publicWebToolNames() = %v, want %s", names, want)
		}
	}
	if containsString(names, "tool_search") {
		t.Fatalf("publicWebToolNames() = %v, must not include tool_search for direct public web retrieval", names)
	}
}

func TestRunTurn_ExecutesMultiIterationToolLoop(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp/one"}`,
					},
				},
			}),
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-2",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp/two"}`,
					},
				},
			}),
			schema.AssistantMessage("final answer", nil),
		},
	}

	var executed []string
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Inspect disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			executed = append(executed, string(input))
			return tooling.ToolResult{Content: "ok:" + string(input)}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-loop",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-loop",
		Input:       "inspect disks",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Output != "final answer" {
		t.Fatalf("result output = %q, want final answer", result.Output)
	}
	if len(executed) != 2 {
		t.Fatalf("executed tool calls = %d, want 2", len(executed))
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model Generate calls = %d, want 3", len(model.inputs))
	}

	foundFirstToolMessage := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-1" && msg.Content == `ok:{"path":"/tmp/one"}` {
			foundFirstToolMessage = true
			break
		}
	}
	if !foundFirstToolMessage {
		t.Fatalf("second model input did not include first tool result: %#v", model.inputs[1])
	}

	session := kernel.sessions.Get("sess-loop")
	if session == nil {
		t.Fatal("expected session to exist")
	}
	if len(session.Messages) != 6 {
		t.Fatalf("session messages len = %d, want 6", len(session.Messages))
	}
	if session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot to exist")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	if len(session.CurrentTurn.Iterations) != 3 {
		t.Fatalf("turn iterations = %d, want 3", len(session.CurrentTurn.Iterations))
	}
	if got := session.Messages[len(session.Messages)-1].Content; got != "final answer" {
		t.Fatalf("latest session message = %q, want final answer", got)
	}
}

func TestRunTurn_FeedsToolFailureBackToModelInsteadOfFailingTurn(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-date",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "exec_command",
						Arguments: `{"command":"date","args":["-d","today","+%F"]}`,
					},
				},
			}),
			schema.AssistantMessage("继续基于已有上下文回答", nil),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_command",
			Description: "Execute a command",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("command failed: date: illegal option -- d")
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-tool-failure",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-tool-failure",
		Input:       "查看今天的公开数据",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn should continue after tool execution failure, got error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Output != "继续基于已有上下文回答" {
		t.Fatalf("result output = %q, want final answer after failed tool result", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}
	foundFailureToolMessage := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-date" {
			foundFailureToolMessage = strings.Contains(msg.Content, "exec_command failed") &&
				strings.Contains(msg.Content, "date: illegal option")
			break
		}
	}
	if !foundFailureToolMessage {
		t.Fatalf("second model input did not include failed tool result: %#v", model.inputs[1])
	}

	session := kernel.sessions.Get("sess-tool-failure")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session current turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	if len(session.CurrentTurn.Iterations) == 0 || len(session.CurrentTurn.Iterations[0].ToolResults) != 1 {
		t.Fatalf("first iteration tool results = %#v, want failed tool result recorded", session.CurrentTurn.Iterations)
	}
	if got := session.CurrentTurn.Iterations[0].ToolResults[0].Error; !strings.Contains(got, "date: illegal option") {
		t.Fatalf("recorded tool error = %q, want original tool error", got)
	}
}

func TestRunTurn_FeedsDeniedToolBackToModelInsteadOfFailingTurn(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("我会先检查数据库连接。", []schema.ToolCall{
				{
					ID:   "call-denied-psql",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "exec_command",
						Arguments: `{"command":"psql","args":["postgres://aiops:aiops@127.0.0.1:55432/aiops?sslmode=disable","-c","select version(), now();"]}`,
					},
				},
			}),
			schema.AssistantMessage("psql 命令被策略拒绝，我会改用已收集的端口和容器证据说明状态。", nil),
		},
	}

	executed := false
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_command",
			Description: "Execute a command",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "forbidden terminal command is blocked by policy"}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed = true
			return tooling.ToolResult{Content: "should not execute"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-denied-tool-feedback",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-denied-tool-feedback",
		Input:       "我要检查pg状态",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn should continue after denied tool call, got error: %v", err)
	}
	if executed {
		t.Fatal("denied tool should not execute")
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Output != "psql 命令被策略拒绝，我会改用已收集的端口和容器证据说明状态。" {
		t.Fatalf("result output = %q, want final answer after denied tool result", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}
	var deniedToolMessage string
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-denied-psql" {
			deniedToolMessage = msg.Content
			break
		}
	}
	if !strings.Contains(deniedToolMessage, "exec_command failed") || !strings.Contains(deniedToolMessage, "forbidden terminal command") {
		t.Fatalf("denied tool message = %q, want denial fed back to model", deniedToolMessage)
	}

	session := kernel.sessions.Get("sess-denied-tool-feedback")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session current turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	toolResult := session.CurrentTurn.Iterations[0].ToolResults[0]
	if toolResult.ToolCallID != "call-denied-psql" || !strings.Contains(toolResult.Error, "forbidden terminal command") {
		t.Fatalf("recorded denied tool result = %#v", toolResult)
	}
}

func TestRunTurn_RejectsRemovedOpsToolCallAsMissingToolResult(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-runbook",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "runbook.match",
						Arguments: `{"symptom":"redis memory"}`,
					},
				},
			}),
			schema.AssistantMessage("已改用当前可用的运维工具继续排查", nil),
		},
	}

	kernel := newLoopKernel(t, model, nil, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-removed-tool",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-removed-tool",
		Input:       "triage redis memory",
	})
	if err != nil {
		t.Fatalf("RunTurn should feed removed tool failure back to model, got error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	var failureToolMessage string
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-runbook" {
			failureToolMessage = msg.Content
			break
		}
	}
	assertStructuredToolError(t, failureToolMessage, "call-runbook", "runbook.match", "tool_not_found", "tool not found: runbook.match")

	session := kernel.sessions.Get("sess-removed-tool")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session current turn")
	}
	if len(session.CurrentTurn.Iterations) == 0 || len(session.CurrentTurn.Iterations[0].ToolResults) != 1 {
		t.Fatalf("first iteration tool results = %#v, want one failed result", session.CurrentTurn.Iterations)
	}
	toolResult := session.CurrentTurn.Iterations[0].ToolResults[0]
	if toolResult.ToolCallID != "call-runbook" || !strings.Contains(toolResult.Error, "tool not found: runbook.match") {
		t.Fatalf("recorded tool result = %#v, want removed tool failure", toolResult)
	}
}

func TestRunTurn_RejectsLegacyOpsToolPrefixesAsMissingToolResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		toolName  string
		arguments string
	}{
		{name: "k8s", toolName: "k8s.restart_workload", arguments: `{"workload":"order-api"}`},
		{name: "changes", toolName: "changes.recent_deployments", arguments: `{"service":"order-api"}`},
		{name: "fallback", toolName: "fallback.plan_exec", arguments: `{"task":"restart redis"}`},
		{name: "erp", toolName: "erp.business_metric", arguments: `{"metric":"order failures"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &sequentialLoopModel{
				responses: []*schema.Message{
					schema.AssistantMessage("", []schema.ToolCall{
						{
							ID:   "call-" + tc.name,
							Type: "function",
							Function: schema.FunctionCall{
								Name:      tc.toolName,
								Arguments: tc.arguments,
							},
						},
					}),
					schema.AssistantMessage("已改用当前可用的运维工具继续排查", nil),
				},
			}

			kernel := newLoopKernel(t, model, nil, nil, nil)
			result, err := kernel.RunTurn(context.Background(), TurnRequest{
				SessionID:   "sess-legacy-tool-" + tc.name,
				SessionType: SessionTypeHost,
				Mode:        ModeInspect,
				TurnID:      "turn-legacy-tool-" + tc.name,
				Input:       "triage redis memory",
			})
			if err != nil {
				t.Fatalf("RunTurn should feed removed tool failure back to model, got error: %v", err)
			}
			if result.Status != "completed" {
				t.Fatalf("result status = %q, want completed", result.Status)
			}

			var failureToolMessage string
			for _, msg := range model.inputs[1] {
				if msg.Role == schema.Tool && msg.ToolCallID == "call-"+tc.name {
					failureToolMessage = msg.Content
					break
				}
			}
			assertStructuredToolError(t, failureToolMessage, "call-"+tc.name, tc.toolName, "tool_not_found", "tool not found: "+tc.toolName)
		})
	}
}

func TestRunTurn_FeedsToolBudgetBackToModelInsteadOfDispatchingForever(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultMaxToolDispatchesPerTurn+2)
	for i := 0; i < defaultMaxToolDispatchesPerTurn+2; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-web-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"public data"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("基于已收集证据给出回答", nil),
		},
	}

	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Description: "Search public web pages",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "search result"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-tool-budget",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-tool-budget",
		Input:       "research public data",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn should continue after tool budget is reached, got error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if executed != defaultMaxToolDispatchesPerTurn {
		t.Fatalf("executed tool calls = %d, want budget %d", executed, defaultMaxToolDispatchesPerTurn)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}
	foundBudgetToolMessage := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "Tool budget reached") {
			foundBudgetToolMessage = true
			break
		}
	}
	if !foundBudgetToolMessage {
		t.Fatalf("second model input did not include tool budget result: %#v", model.inputs[1])
	}

	session := kernel.sessions.Get("sess-tool-budget")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session current turn")
	}
	if !containsString(session.CurrentTurn.HiddenTools, "web_search") {
		t.Fatalf("hidden tools = %v, want web_search hidden after budget", session.CurrentTurn.HiddenTools)
	}
	firstIter := session.CurrentTurn.Iterations[0]
	if got := len(firstIter.ToolResults); got != defaultMaxToolDispatchesPerTurn+2 {
		t.Fatalf("first iteration tool results = %d, want one result per requested tool call", got)
	}
	lastResult := firstIter.ToolResults[len(firstIter.ToolResults)-1]
	if lastResult.Display == nil || lastResult.Display.Type != "tool_budget" {
		t.Fatalf("last result display = %#v, want tool_budget", lastResult.Display)
	}
}

func TestRunTurn_CapsPublicWebRetrievalsBeforeGlobalToolBudget(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultMaxPublicWebDispatchesPerTurn+3)
	for i := 0; i < defaultMaxPublicWebDispatchesPerTurn+3; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-public-web-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"public docs"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("基于已检索的公开资料收敛回答", nil),
		},
	}

	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Description: "Search public web pages",
			Pack:        "public_web",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "search result"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-public-web-budget",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-public-web-budget",
		Input:       "research public docs",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn should continue after public web budget is reached, got error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if executed != defaultMaxPublicWebDispatchesPerTurn {
		t.Fatalf("executed public web calls = %d, want budget %d", executed, defaultMaxPublicWebDispatchesPerTurn)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}
	foundBudgetToolMessage := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "Public web retrieval budget reached") {
			foundBudgetToolMessage = true
			break
		}
	}
	if !foundBudgetToolMessage {
		t.Fatalf("second model input did not include public web budget result: %#v", model.inputs[1])
	}

	session := kernel.sessions.Get("sess-public-web-budget")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session current turn")
	}
	if !containsString(session.CurrentTurn.HiddenTools, "web_search") {
		t.Fatalf("hidden tools = %v, want web_search hidden after public web budget", session.CurrentTurn.HiddenTools)
	}
	firstIter := session.CurrentTurn.Iterations[0]
	if got := len(firstIter.ToolResults); got != defaultMaxPublicWebDispatchesPerTurn+3 {
		t.Fatalf("first iteration tool results = %d, want one result per requested tool call", got)
	}
	lastResult := firstIter.ToolResults[len(firstIter.ToolResults)-1]
	if lastResult.Display == nil || lastResult.Display.Type != "tool_budget" {
		t.Fatalf("last result display = %#v, want tool_budget", lastResult.Display)
	}
}

func TestRunTurn_CapsPublicWebQueriesAndResultLimit(t *testing.T) {
	toolCalls := []schema.ToolCall{
		{
			ID:   "call-public-web-a",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"queries":["official docs restore","official docs timeline","official docs standby"],"max_results":20}`,
			},
		},
		{
			ID:   "call-public-web-b",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"queries":["extra synonym one","extra synonym two"],"max_results":20}`,
			},
		},
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("基于已检索的公开资料收敛回答", nil),
		},
	}

	executed := 0
	var executedArgs []map[string]any
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Description: "Search public web pages",
			Pack:        "public_web",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, args json.RawMessage) (tooling.ToolResult, error) {
			executed++
			var payload map[string]any
			if err := json.Unmarshal(args, &payload); err != nil {
				t.Fatalf("tool args should stay JSON: %v", err)
			}
			executedArgs = append(executedArgs, payload)
			return tooling.ToolResult{Content: "search result"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-public-web-query-budget",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-public-web-query-budget",
		Input:       "research public docs",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn should continue after public web query budget is reached, got error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if executed != 2 {
		t.Fatalf("executed public web calls = %d, want 2 after per-call query clamp", executed)
	}
	if len(executedArgs) != 2 || int(executedArgs[0]["max_results"].(float64)) != DefaultPublicWebBudget().MaxResults {
		t.Fatalf("executed args = %#v, want max_results clamped to %d", executedArgs, DefaultPublicWebBudget().MaxResults)
	}
	for index, args := range executedArgs {
		queries, ok := args["queries"].([]any)
		if !ok {
			t.Fatalf("executed args[%d] queries = %#v, want JSON array", index, args["queries"])
		}
		if len(queries) > DefaultPublicWebBudget().MaxQueriesPerCall {
			t.Fatalf("executed args[%d] queries = %#v, want at most %d", index, queries, DefaultPublicWebBudget().MaxQueriesPerCall)
		}
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}
}

func TestRunTurn_SwitchesToSynthesisOnlyAfterPublicWebEvidence(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultPublicWebSynthesisDispatches)
	for i := 0; i < defaultPublicWebSynthesisDispatches; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-public-doc-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"official public docs"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("我先核对公开文档来源。", toolCalls),
			schema.AssistantMessage("基于已检索的官方来源给出最终 RCA。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Description: "Search public web pages",
			Pack:        "public_web",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "official source result"}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-public-web-synthesis-only",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-public-web-synthesis-only",
		Input:       "用公开文档辅助分析运维问题",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Output != "基于已检索的官方来源给出最终 RCA。" {
		t.Fatalf("result output = %q", result.Output)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if len(compiler.contexts[0].AssembledTools) == 0 {
		t.Fatal("first iteration should expose public web tools")
	}
	if len(compiler.contexts[1].AssembledTools) != 0 {
		t.Fatalf("second iteration tools = %v, want synthesis-only after public web evidence", toolNames(compiler.contexts[1].AssembledTools))
	}
	if !containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "web_search") {
		t.Fatalf("second iteration unavailable tools = %v, want web_search", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}
	if got := strings.Join(compiler.contexts[1].SkillPromptAssets, "\n"); !strings.Contains(got, "Public-web synthesis-only phase") {
		t.Fatalf("second iteration prompt assets missing public-web synthesis guidance:\n%s", got)
	}
}

func TestRunTurn_SwitchesToSynthesisOnlyAfterEnoughToolEvidence(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultSynthesisOnlyToolDispatches)
	for i := 0; i < defaultSynthesisOnlyToolDispatches; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-evidence-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"public evidence"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("基于已收集证据给出最终回答", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Description: "Search public web pages",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "evidence result"}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-synthesis-only",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-synthesis-only",
		Input:       "research public data",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Output != "基于已收集证据给出最终回答" {
		t.Fatalf("result output = %q", result.Output)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if len(compiler.contexts[0].AssembledTools) == 0 {
		t.Fatal("first iteration should expose tools")
	}
	if len(compiler.contexts[1].AssembledTools) != 0 {
		t.Fatalf("second iteration tools = %v, want synthesis-only with no tools", toolNames(compiler.contexts[1].AssembledTools))
	}
	if !containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "web_search") {
		t.Fatalf("second iteration unavailable tools = %v, want web_search", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}
}

func TestRunTurn_SimpleHostResourceInspectionSynthesizesAfterCoveredEvidence(t *testing.T) {
	toolCalls := []schema.ToolCall{
		{
			ID:   "call-cpu-count",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "exec_command",
				Arguments: `{"command":"nproc"}`,
			},
		},
		{
			ID:   "call-load",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "exec_command",
				Arguments: `{"command":"cat /proc/loadavg"}`,
			},
		},
		{
			ID:   "call-memory",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "exec_command",
				Arguments: `{"command":"free -h"}`,
			},
		},
		{
			ID:   "call-disk",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "exec_command",
				Arguments: `{"command":"df -hT -x tmpfs -x devtmpfs"}`,
			},
		},
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("CPU、内存、磁盘资源已基于直接主机证据汇总。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_command",
			Description: "Execute a terminal command on the selected host",
			Layer:       tooling.ToolLayerCore,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				PermissionScope: "argument_scoped",
			},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "resource evidence: " + string(input)}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-simple-host-resource-synthesis",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-simple-host-resource-synthesis",
		Input:       "帮我看下这台远程主机的 CPU、内存、磁盘资源情况，给出关键数值和简短判断。",
		HostID:      "remote-host",
		Metadata: map[string]string{
			"aiops.host.os":        "linux",
			"aiops.host.transport": "agent_http",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if len(compiler.contexts[0].AssembledTools) == 0 {
		t.Fatal("first iteration should expose exec_command")
	}
	if len(compiler.contexts[1].AssembledTools) != 0 {
		t.Fatalf("second iteration tools = %v, want synthesis-only after covered host resource evidence", toolNames(compiler.contexts[1].AssembledTools))
	}
	if !containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "exec_command") {
		t.Fatalf("second iteration unavailable tools = %v, want exec_command", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}
	if got := strings.Join(compiler.contexts[1].SkillPromptAssets, "\n"); !strings.Contains(got, "Synthesis-only phase") {
		t.Fatalf("second iteration prompt assets missing synthesis-only phase:\n%s", got)
	}
}

func TestRunTurn_UpdatePlanDoesNotConsumeSynthesisEvidenceBudget(t *testing.T) {
	toolCalls := []schema.ToolCall{
		{
			ID:   "call-plan-a",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "update_plan",
				Arguments: `{"steps":[{"id":"check","text":"Check Docker","status":"in_progress"}]}`,
			},
		},
		{
			ID:   "call-plan-b",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "update_plan",
				Arguments: `{"steps":[{"id":"check","text":"Check Docker","status":"completed"},{"id":"run","text":"Run nginx","status":"in_progress"}]}`,
			},
		},
	}
	for i := 0; i < defaultSynthesisOnlyToolDispatches-2; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-evidence-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"public evidence"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("缺少后续执行结果，继续执行下一步。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Description: "Search public web pages",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "evidence result"}, nil
		},
	}
	registry := tooling.NewRegistry()
	for _, toolDef := range []tooling.Tool{planning.NewUpdatePlanTool(), toolDef} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-plan-budget",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-plan-budget",
		Input:       "run a multi-step task",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if len(compiler.contexts[1].AssembledTools) == 0 {
		t.Fatal("second iteration should still expose tools because update_plan does not count as evidence")
	}
	if containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "web_search") {
		t.Fatalf("second iteration unavailable tools = %v, did not expect synthesis-only", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}
}

func TestRunTurn_ExecuteModeDoesNotSwitchToSynthesisOnlyAtEvidenceThreshold(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultSynthesisOnlyToolDispatches)
	for i := 0; i < defaultSynthesisOnlyToolDispatches; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-evidence-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "read_status",
				Arguments: `{"query":"status"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("继续执行变更步骤，而不是提前收尾", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_status",
			Description: "Read status",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "status evidence"}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-execute-budget",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-execute-budget",
		Input:       "inspect then change",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if len(compiler.contexts[1].AssembledTools) == 0 {
		t.Fatal("execute mode should keep tools available at the evidence synthesis threshold")
	}
	if containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "read_status") {
		t.Fatalf("second iteration unavailable tools = %v, did not expect synthesis-only in execute mode", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}
}

func TestRunTurn_AddsEvidenceAwareFinalAnswerPromptAfterToolResults(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-nginx-log",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_log",
						Arguments: `{"path":"/var/log/nginx/error.log"}`,
					},
				},
			}),
			schema.AssistantMessage("final answer", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_log",
			Description: "Read log evidence",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "upstream timeout for service-a"}, nil
		},
	}

	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-evidence-final",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-evidence-final",
		Input:       "分析 nginx 故障根因",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "final answer" {
		t.Fatalf("output = %q, want final answer", result.Output)
	}
	if len(compiler.contexts) < 2 {
		t.Fatalf("compiler contexts = %d, want second synthesis compile", len(compiler.contexts))
	}
	secondInput := strings.Join(compiler.contexts[1].SkillPromptAssets, "\n")
	for _, want := range []string{
		"Evidence-aware final answer",
		"upstream timeout for service-a",
		"根因：",
		"证据：",
		"影响面：",
		"下一步：",
	} {
		if !strings.Contains(secondInput, want) {
			t.Fatalf("second model input missing %q:\n%s", want, secondInput)
		}
	}
}

func TestRunTurn_EvidenceAwareFinalPromptKeepsCleanStatusChecksShort(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-redis-status",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_redis_status",
						Arguments: `{"instance":"redis-local-01"}`,
					},
				},
			}),
			schema.AssistantMessage("Redis 状态正常", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_redis_status",
			Description: "Read Redis status",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "redis-local-01 ping ok, memory stable, no abnormality detected"}, nil
		},
	}

	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	assembler := tooling.NewAssembler(registry, nil)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: assembler}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-clean-status-final",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-clean-status-final",
		Input:       "检查 redis-local-01 状态",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "Redis 状态正常" {
		t.Fatalf("output = %q, want Redis 状态正常", result.Output)
	}
	if len(compiler.contexts) < 2 {
		t.Fatalf("compiler contexts = %d, want second synthesis compile", len(compiler.contexts))
	}
	secondInput := strings.Join(compiler.contexts[1].SkillPromptAssets, "\n")
	for _, want := range []string{
		"read-only status/RCA check",
		"no abnormality",
		"Keep the final answer short",
		"Do not expand 下一步",
		"do not suggest remediation, workflow execution, rollback, or operations manual generation",
	} {
		if !strings.Contains(secondInput, want) {
			t.Fatalf("second model input missing %q:\n%s", want, secondInput)
		}
	}
}

func TestRunTurn_EmitsTurnEventLifecycleForReactLoop(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-events",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp/events"}`,
					},
				},
			}),
			schema.AssistantMessage("final event answer", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Inspect disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}

	kernel, emitter := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)
	if err := kernel.tools.(*testMockToolAssemblySource).registry.Register(toolDef); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-events",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-events",
		Input:       "inspect event order",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	eventTypes := make([]EventType, 0, len(emitter.events))
	for _, event := range emitter.events {
		eventTypes = append(eventTypes, event.Type)
	}
	wantOrdered := []EventType{
		EventTurnStarted,
		EventToolStarted,
		EventToolCompleted,
		EventPhaseEnd,
		EventProcessSummary,
		EventTurnComplete,
	}
	cursor := 0
	for _, eventType := range eventTypes {
		if cursor < len(wantOrdered) && eventType == wantOrdered[cursor] {
			cursor++
		}
	}
	if cursor != len(wantOrdered) {
		t.Fatalf("event order = %v, want subsequence %v", eventTypes, wantOrdered)
	}
}

func TestRunTurnWritesDeterministicCommentaryBeforeToolCallWithoutModelText(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-cpu",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "shell_command",
						Arguments: `{"cmd":"uptime"}`,
					},
				},
			}),
			schema.AssistantMessage("CPU 检查完成。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "shell_command",
			Description: "Run read-only command",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "CPU usage: 10% user"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	emitter := kernel.projector.(*testMockEventEmitter)
	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-deterministic-commentary",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-deterministic-commentary",
		Input:       "查看 cpu 情况",
		HostID:      "server-local",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	for _, event := range emitter.events {
		if event.Type == EventAssistantIntent {
			t.Fatalf("unexpected EventAssistantIntent in Chat main path: %#v", event)
		}
	}

	session := kernel.sessions.Get("sess-deterministic-commentary")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	items := session.CurrentTurn.AgentItems
	commentaryIndex := findAgentItemBySummaryContains(items, agentstate.TurnItemTypeAssistantMessage, "执行只读命令")
	toolIndex := indexAgentItemByID(items, "turn-deterministic-commentary-tool-call-call-cpu")
	if commentaryIndex < 0 || toolIndex < 0 {
		t.Fatalf("agent items = %#v, want commentary and tool call", items)
	}
	if !(commentaryIndex < toolIndex) {
		t.Fatalf("agent items = %#v, want commentary before tool call", items)
	}
	if phase := assistantMessagePhaseForAgentItemsTest(items[commentaryIndex]); phase != "commentary" {
		t.Fatalf("assistant phase = %q, want commentary", phase)
	}
	payload := agentItemPayloadMap(items[commentaryIndex])
	if payload["commentarySource"] != "runtime_tool_intent" {
		t.Fatalf("payload = %#v, want runtime_tool_intent", payload)
	}
}

func TestRunTurn_EmitsStartedBeforeFinalForNoToolReactLoop(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("direct final answer", nil),
		},
	}
	kernel, emitter := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-no-tool-events",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-no-tool-events",
		Input:       "answer directly",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	startedAt := -1
	completeAt := -1
	eventTypes := make([]EventType, 0, len(emitter.events))
	for i, event := range emitter.events {
		eventTypes = append(eventTypes, event.Type)
		switch event.Type {
		case EventTurnStarted:
			if startedAt == -1 {
				startedAt = i
			}
		case EventTurnComplete:
			if completeAt == -1 {
				completeAt = i
			}
		}
	}
	if startedAt == -1 || completeAt == -1 {
		t.Fatalf("event order = %v, want turn.started and turn.complete", eventTypes)
	}
	if !(startedAt < completeAt) {
		t.Fatalf("event order = %v, want turn.started before turn.complete", eventTypes)
	}
	session := kernel.sessions.Get("sess-no-tool-events")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if got := FinalTextFromAssistantMessage(session.CurrentTurn); got != "direct final answer" {
		t.Fatalf("final text = %q, want direct final answer", got)
	}
}

func TestRunTurn_CommitsStreamedFinalTextWithoutFinalDeltaEvents(t *testing.T) {
	model := &streamingFinalLoopModel{
		chunks: []*schema.Message{
			schema.AssistantMessage("第一段", nil),
			schema.AssistantMessage("第二段", nil),
		},
	}
	kernel, emitter := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-streaming-final",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-streaming-final",
		Input:       "stream directly",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "第一段第二段" {
		t.Fatalf("RunTurn output = %q, want concatenated streaming output", result.Output)
	}

	completeAt := -1
	for i, event := range emitter.events {
		switch event.Type {
		case EventAssistantFinalDelta:
			t.Fatalf("unexpected assistant final delta in single assistant_message path: %#v", event)
		case EventTurnComplete:
			if completeAt == -1 {
				completeAt = i
			}
		}
	}

	if completeAt == -1 {
		t.Fatalf("event order = %v, want turn.complete", emitter.events)
	}
	session := kernel.sessions.Get("sess-streaming-final")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if got := FinalTextFromAssistantMessage(session.CurrentTurn); got != "第一段第二段" {
		t.Fatalf("final text = %q, want concatenated streaming output", got)
	}
}

func TestRunTurn_PersistsRunningFinalAssistantItemDuringStreaming(t *testing.T) {
	model := &gatedStreamingFinalLoopModel{
		firstSent: make(chan struct{}),
		release:   make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(model.release) }) }
	defer release()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	type runResult struct {
		result TurnResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := kernel.RunTurn(context.Background(), TurnRequest{
			SessionID:   "sess-streaming-final-running",
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			TurnID:      "turn-streaming-final-running",
			Input:       "stream directly",
		})
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-model.firstSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first streaming chunk")
	}

	var finalItem agentstate.TurnItem
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session := kernel.sessions.Get("sess-streaming-final-running")
		if session != nil && session.CurrentTurn != nil {
			if item, ok := findAgentItemByID(session.CurrentTurn.AgentItems, assistantMessageItemID("turn-streaming-final-running", 0)); ok {
				finalItem = item
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if finalItem.ID == "" {
		session := kernel.sessions.Get("sess-streaming-final-running")
		var items []agentstate.TurnItem
		if session != nil && session.CurrentTurn != nil {
			items = session.CurrentTurn.AgentItems
		}
		t.Fatalf("running final assistant item was not persisted before stream completion; items = %#v", items)
	}
	if finalItem.Status != agentstate.ItemStatusRunning || assistantMessagePhaseForTest(finalItem) != string(AssistantMessagePhaseFinalAnswer) {
		t.Fatalf("running final item = %#v, want running final_answer", finalItem)
	}
	if got := finalItem.Payload.Summary; got != "第一段" {
		t.Fatalf("running final item summary = %q, want first streamed chunk", got)
	}

	release()
	result := <-done
	if result.err != nil {
		t.Fatalf("RunTurn failed: %v", result.err)
	}
	if result.result.Output != "第一段第二段" {
		t.Fatalf("RunTurn output = %q, want complete streamed output", result.result.Output)
	}
}

func TestRunTurn_CompletesWithStreamedFinalText(t *testing.T) {
	model := &streamingFinalLoopModel{
		chunks: []*schema.Message{
			schema.AssistantMessage("当前资源总览：\n", nil),
			schema.AssistantMessage("- CPU：空闲 73%\n", nil),
			schema.AssistantMessage("- 内存：32 GB，总体偏高\n", nil),
			schema.AssistantMessage("\n", nil),
			schema.AssistantMessage("数据为实时快照。", nil),
		},
	}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-streaming-final-canonical",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-streaming-final-canonical",
		Input:       "stream directly",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	want := "当前资源总览：\n- CPU：空闲 73%\n- 内存：32 GB，总体偏高\n\n数据为实时快照。"
	if result.Output != want {
		t.Fatalf("RunTurn output = %q, want streamed text", result.Output)
	}
	session := kernel.sessions.Get("sess-streaming-final-canonical")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing completed turn")
	}
	if session.CurrentTurn.FinalOutput != want {
		t.Fatalf("FinalOutput = %q, want streamed text", session.CurrentTurn.FinalOutput)
	}
	finalItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if finalItem.ID == "" {
		t.Fatalf("agent items = %+v, want assistant_message final item", session.CurrentTurn.AgentItems)
	}
	if finalItem.Payload.Summary != want {
		t.Fatalf("final item summary = %q, want streamed text", finalItem.Payload.Summary)
	}
}

func TestRunTurn_RetriesTruncatedFinalAnswerBeforeCompletion(t *testing.T) {
	incomplete := "评估结论\n\nCPU负载正常，空闲率较高（81.9"
	complete := "评估结论\n\nCPU负载正常，空闲率较高（81.93%），当前没有发现 CPU 饱和风险。"
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage(incomplete, nil),
		schema.AssistantMessage(complete, nil),
	}}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-truncated-final",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-truncated-final",
		Input:       "请根据已有 CPU 数据给出一句完整评估结论。",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	if result.Output != complete {
		t.Fatalf("RunTurn output = %q, want completed retry output", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want retry after truncated final answer", len(model.inputs))
	}
	retryInput := schemaMessagesText(model.inputs[1])
	if !strings.Contains(retryInput, "Final answer completeness guard") {
		t.Fatalf("second model input missing completeness guard:\n%s", retryInput)
	}
	if !strings.Contains(retryInput, incomplete) {
		t.Fatalf("second model input missing incomplete final answer evidence:\n%s", retryInput)
	}
	session := kernel.sessions.Get("sess-truncated-final")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing completed turn")
	}
	if got := session.CurrentTurn.Metadata[finalCompletenessRetryMetadataKey]; got != "1" {
		t.Fatalf("final completeness retry metadata = %q, want 1", got)
	}
	if session.CurrentTurn.FinalOutput != complete {
		t.Fatalf("FinalOutput = %q, want completed retry output", session.CurrentTurn.FinalOutput)
	}
	var finalItems []agentstate.TurnItem
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == agentstate.TurnItemTypeAssistantMessage && assistantMessagePhaseForTest(item) == "final_answer" && item.Status == agentstate.ItemStatusCompleted {
			finalItems = append(finalItems, item)
		}
	}
	if len(finalItems) != 1 {
		t.Fatalf("assistant final items = %#v, want only completed retry output", finalItems)
	}
	if finalItems[0].Status != agentstate.ItemStatusCompleted || finalItems[0].Payload.Summary != complete {
		t.Fatalf("assistant final item = %#v, want completed retry output", finalItems[0])
	}
}

func TestRunTurn_FailsLengthStoppedFinalAfterRetry(t *testing.T) {
	first := schema.AssistantMessage("完整说明：第一部分已经展开，第二部分需要继续补充", nil)
	first.ResponseMeta = &schema.ResponseMeta{FinishReason: "length"}
	second := schema.AssistantMessage("完整说明：第一部分已经展开，第二部分继续补充，但结尾仍然", nil)
	second.ResponseMeta = &schema.ResponseMeta{FinishReason: "length"}
	model := &sequentialLoopModel{responses: []*schema.Message{first, second}}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, newRecordingCompiler(), model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-length-stopped-final",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-length-stopped-final",
		Input:       "请写一段完整说明。",
	})
	if err == nil {
		t.Fatal("expected RunTurn to fail when the final answer repeatedly stops for length")
	}

	session := kernel.sessions.Get("sess-length-stopped-final")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing failed turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("turn lifecycle = %q, want failed", session.CurrentTurn.Lifecycle)
	}
	if session.CurrentTurn.FinalOutput != "" {
		t.Fatalf("FinalOutput = %q, want empty incomplete final", session.CurrentTurn.FinalOutput)
	}
	if session.CurrentTurn.LatestCheckpoint == nil || session.CurrentTurn.LatestCheckpoint.Kind != "assistant_message_incomplete" {
		t.Fatalf("checkpoint = %#v, metadata=%#v modelCalls=%d, want assistant_message_incomplete", session.CurrentTurn.LatestCheckpoint, session.CurrentTurn.Metadata, len(model.inputs))
	}
	finalItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if finalItem.ID == "" {
		t.Fatalf("agent items = %#v, want assistant_message item", session.CurrentTurn.AgentItems)
	}
	if finalItem.Status == agentstate.ItemStatusCompleted {
		t.Fatalf("final item status = %q, want not completed", finalItem.Status)
	}
	if got := session.CurrentTurn.Metadata[finalCompletenessRetryMetadataKey]; got != "1" {
		t.Fatalf("final completeness retry metadata = %q, want 1", got)
	}

	model.responses = append(model.responses, schema.AssistantMessage("接上文：第二部分完整补充完毕，并给出收束结论。", nil))
	resumed, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-length-stopped-final",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-length-stopped-final-continue",
		Input:       "接着说",
	})
	if err != nil {
		t.Fatalf("continuation RunTurn failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("continuation status = %q, want completed", resumed.Status)
	}
	continuationInput := schemaMessagesText(model.inputs[len(model.inputs)-1])
	if !strings.Contains(continuationInput, second.Content) {
		t.Fatalf("continuation input missing previous incomplete assistant content:\n%s", continuationInput)
	}
}

func TestRunTurn_AccumulatesAssistantTextBeforeFinalCommit(t *testing.T) {
	model := &gatedStreamingFinalLoopModel{
		firstSent: make(chan struct{}),
		release:   make(chan struct{}),
	}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	done := make(chan error, 1)
	go func() {
		_, err := kernel.RunTurn(context.Background(), TurnRequest{
			SessionID:   "sess-streaming-snapshot",
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			TurnID:      "turn-streaming-snapshot",
			Input:       "stream directly",
		})
		done <- err
	}()

	select {
	case <-model.firstSent:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first streaming final chunk")
	}

	var session *SessionState
	var answerItem agentstate.TurnItem
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		session = kernel.sessions.Get("sess-streaming-snapshot")
		if session != nil && session.CurrentTurn != nil {
			answerItem = findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
			if answerItem.ID != "" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing current turn after first streaming chunk")
	}
	if got := session.CurrentTurn.FinalOutput; got != "" {
		close(model.release)
		t.Fatalf("CurrentTurn.FinalOutput = %q, want empty until final commit", got)
	}
	if answerItem.ID == "" {
		close(model.release)
		t.Fatalf("assistant_message running draft must be written before final commit, got %+v", session.CurrentTurn.AgentItems)
	}
	if answerItem.Status != agentstate.ItemStatusRunning || assistantMessagePhaseForTest(answerItem) != string(AssistantMessagePhaseFinalAnswer) {
		close(model.release)
		t.Fatalf("assistant_message running draft = %+v, want running final_answer", answerItem)
	}
	if got := answerItem.Payload.Summary; got != "第一段" {
		close(model.release)
		t.Fatalf("assistant_message running draft summary = %q, want first streamed chunk", got)
	}
	if invalid := firstInvalidTurnItem(session.CurrentTurn.AgentItems); invalid.ID != "" {
		close(model.release)
		t.Fatalf("agent items must use current valid item types: %+v", session.CurrentTurn.AgentItems)
	}

	close(model.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunTurn failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for streaming run to complete")
	}
	session = kernel.sessions.Get("sess-streaming-snapshot")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing completed current turn")
	}
	if got := session.CurrentTurn.FinalOutput; got != "第一段第二段" {
		t.Fatalf("CurrentTurn.FinalOutput = %q, want committed final output", got)
	}
	finalItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if finalItem.ID == "" || finalItem.Status != agentstate.ItemStatusCompleted || finalItem.Payload.Summary != "第一段第二段" {
		t.Fatalf("final item = %+v, want completed assistant_message final", finalItem)
	}
}

func TestRunTurn_StreamingModelErrorPreservesIncompleteAssistantMessage(t *testing.T) {
	streamErr := context.DeadlineExceeded
	model := &partialStreamErrorLoopModel{
		chunks: []*schema.Message{
			schema.AssistantMessage("根因：已经生成的分析草稿必须保留。", nil),
		},
		err: streamErr,
	}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-stream-error-draft",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-stream-error-draft",
		Input:       "分析流式错误时的回答保留",
	})
	if err == nil {
		t.Fatal("RunTurn error = nil, want stream error")
	}

	session := kernel.sessions.Get("sess-stream-error-draft")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing failed turn")
	}
	if session.CurrentTurn.FinalOutput != "" {
		t.Fatalf("FinalOutput = %q, want empty on stream error", session.CurrentTurn.FinalOutput)
	}
	answerItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if answerItem.ID == "" {
		t.Fatalf("agent items = %#v, want assistant_message draft preserved", session.CurrentTurn.AgentItems)
	}
	if answerItem.Status != agentstate.ItemStatusFailed {
		t.Fatalf("assistant answer status = %q, want failed", answerItem.Status)
	}
	if answerItem.Payload.Summary != "根因：已经生成的分析草稿必须保留。" {
		t.Fatalf("assistant answer summary = %q", answerItem.Payload.Summary)
	}
	var answerData struct {
		Phase       string `json:"phase"`
		StreamState string `json:"streamState"`
		DisplayKind string `json:"displayKind"`
	}
	if err := json.Unmarshal(answerItem.Payload.Data, &answerData); err != nil {
		t.Fatalf("decode answer data: %v; raw=%s", err, string(answerItem.Payload.Data))
	}
	if answerData.Phase != "final_answer" || answerData.StreamState != "incomplete" || answerData.DisplayKind != "assistant.message" {
		t.Fatalf("answer data = %+v, want incomplete assistant_message", answerData)
	}
	errorItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeError)
	if errorItem.ID == "" || errorItem.Status != agentstate.ItemStatusFailed {
		t.Fatalf("agent items = %#v, want failed error item", session.CurrentTurn.AgentItems)
	}
}

func TestRunTurn_ModelFailureEmitsTurnErrorEvent(t *testing.T) {
	model := &sequentialLoopModel{generateErr: errors.New("provider unavailable")}
	kernel, emitter := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, &testMockCompiler{}, model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-model-failure-event",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-model-failure-event",
		Input:       "你好",
	})
	if err == nil {
		t.Fatal("RunTurn error = nil, want model failure")
	}
	if !hasEventType(emitter.events, EventTurnError) {
		t.Fatalf("events = %v, want %s after model failure", eventTypes(emitter.events), EventTurnError)
	}
}

func TestRunTurn_PreservesRetriedAssistantTextAsAnswerDraft(t *testing.T) {
	incomplete := "结论：数据库连接失败，下一步需要补充"
	complete := "结论：数据库连接失败。下一步先检查 Service Endpoints、NetworkPolicy 和凭证挂载。"
	first := schema.AssistantMessage(incomplete, nil)
	first.ResponseMeta = &schema.ResponseMeta{FinishReason: "length"}
	model := &sequentialLoopModel{responses: []*schema.Message{
		first,
		schema.AssistantMessage(complete, nil),
	}}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: tooling.NewRegistry()}, newRecordingCompiler(), model)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-retry-progress",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-retry-progress",
		Input:       "分析 CrashLoopBackOff",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-retry-progress")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing completed turn")
	}
	if got := session.CurrentTurn.FinalOutput; got != complete {
		t.Fatalf("FinalOutput = %q, want completed retry output", got)
	}
	if answerIndex := findAgentItemBySummary(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage, incomplete); answerIndex < 0 {
		t.Fatalf("agent items = %#v, want retried assistant text preserved as incomplete assistant_message", session.CurrentTurn.AgentItems)
	} else {
		var draftData struct {
			Phase               string `json:"phase"`
			StreamState         string `json:"streamState"`
			BoundaryAction      string `json:"boundaryAction"`
			ReplacedByMessageID string `json:"replacedByMessageId"`
			DisplayKind         string `json:"displayKind"`
		}
		item := session.CurrentTurn.AgentItems[answerIndex]
		if item.Status != agentstate.ItemStatusFailed {
			t.Fatalf("retried assistant item status = %q, want failed", item.Status)
		}
		if err := json.Unmarshal(item.Payload.Data, &draftData); err != nil {
			t.Fatalf("decode retried assistant_message data: %v; raw=%s", err, string(item.Payload.Data))
		}
		if draftData.Phase != "final_answer" || draftData.StreamState != "incomplete" || draftData.BoundaryAction != "retry_once" || draftData.ReplacedByMessageID == "" || draftData.DisplayKind != "assistant.message" {
			t.Fatalf("retried assistant_message data = %+v, want incomplete retry_once draft", draftData)
		}
	}
	assertNoLegacyAssistantItems(t, session.CurrentTurn.AgentItems)
}

func TestRunTurn_BlockedToolCallCanResume(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
			schema.AssistantMessage("write complete", nil),
		},
	}

	var executed int
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "wrote:" + string(input)}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-approval",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-approval",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}
	if executed != 0 {
		t.Fatalf("tool should not execute before approval, got %d executions", executed)
	}
	session := kernel.sessions.Get("sess-approval")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected suspended current turn snapshot")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleSuspended {
		t.Fatalf("current turn lifecycle = %q, want suspended", session.CurrentTurn.Lifecycle)
	}
	if session.CurrentTurn.ResumeState != TurnResumeStatePendingApproval {
		t.Fatalf("current turn resume state = %q, want pending_approval", session.CurrentTurn.ResumeState)
	}
	if len(session.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(session.PendingApprovals))
	}
	pendingApproval := session.PendingApprovals[0]
	if pendingApproval.ArgumentsHash == "" || pendingApproval.InputHash != pendingApproval.ArgumentsHash {
		t.Fatalf("pending approval hashes = input:%q args:%q, want populated matching hashes", pendingApproval.InputHash, pendingApproval.ArgumentsHash)
	}
	if pendingApproval.ToolSurfaceFingerprint == "" || pendingApproval.PermissionSnapshotHash == "" {
		t.Fatalf("pending approval fingerprints = surface:%q permission:%q, want populated", pendingApproval.ToolSurfaceFingerprint, pendingApproval.PermissionSnapshotHash)
	}
	if pendingApproval.IterationID == "" || len(pendingApproval.ApprovalOptions) == 0 {
		t.Fatalf("pending approval ledger fields = %#v, want iteration id and approval options", pendingApproval)
	}
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	foundApprovalNeeded := false
	for _, event := range emitter.events {
		if event.Type == EventApprovalNeeded {
			foundApprovalNeeded = true
			break
		}
	}
	if !foundApprovalNeeded {
		t.Fatal("expected approval-needed projection event when turn blocks")
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: "sess-approval",
		TurnID:    "turn-approval",
		Decision:  "approved",
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", resumed.Status)
	}
	if resumed.Output != "write complete" {
		t.Fatalf("resume output = %q, want write complete", resumed.Output)
	}
	if executed != 1 {
		t.Fatalf("tool executions after resume = %d, want 1", executed)
	}
	foundApprovalApproved := false
	foundTurnCompleteAfterApproval := false
	for _, event := range emitter.events {
		if event.Type == EventTurnComplete && event.TurnID == "turn-approval" {
			foundTurnCompleteAfterApproval = true
		}
		if event.Type != EventApprovalDecided {
			continue
		}
		var payload map[string]string
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("approval.decided payload decode error = %v", err)
		}
		if payload["status"] == "approved" && payload["decision"] == "approved" {
			foundApprovalApproved = true
		}
	}
	if !foundApprovalApproved {
		t.Fatalf("expected approval.decided approved event after resume, events = %#v", emitter.events)
	}
	if !foundTurnCompleteAfterApproval {
		t.Fatalf("expected turn.complete event after approved resume, events = %#v", emitter.events)
	}
	session = kernel.sessions.Get("sess-approval")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn after resume")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle after resume = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	if len(session.PendingApprovals) != 0 {
		t.Fatalf("pending approvals after resume = %d, want 0", len(session.PendingApprovals))
	}
}

func TestResumeTurnApprovalFingerprintDriftRequiresReapproval(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-drift",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"/tmp/demo","content":"hi"}`,
				},
			}}),
		},
	}
	var executed int
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "write_file", Description: "Write a file"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "should-not-run"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-drift",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-drift",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}
	session := kernel.sessions.Get("sess-drift")
	if session == nil || len(session.PendingApprovals) != 1 || session.CurrentTurn == nil {
		t.Fatalf("session pending approval missing: %#v", session)
	}
	approvalID := session.PendingApprovals[0].ID
	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:   "sess-drift",
		TurnID:      "turn-drift",
		ApprovalID:  approvalID,
		Decision:    "approved",
		ResumeState: TurnResumeStatePendingApproval,
		Metadata: map[string]string{
			"permissionSnapshotHash": "sha256:changed-permission",
		},
	})
	if err != nil {
		t.Fatalf("ResumeTurn drift error = %v", err)
	}
	if executed != 0 {
		t.Fatalf("tool executed %d time(s), want no execution after fingerprint drift", executed)
	}
	if resumed.Status != "blocked" || !strings.Contains(resumed.Error, "aiops.approval_drift/v1") {
		t.Fatalf("resume result = %#v, want approval drift blocker", resumed)
	}
	session = kernel.sessions.Get("sess-drift")
	if session == nil || len(session.PendingApprovals) != 1 || !strings.Contains(session.PendingApprovals[0].Reason, "approval_drift") {
		t.Fatalf("pending approvals after drift = %#v, want re-approval request", session.PendingApprovals)
	}
}

func TestResumeTurn_PreservesHostMetadataForToolAssembly(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-exec-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "exec_command",
						Arguments: `{"command":"docker","args":["--version"]}`,
					},
				},
			}),
			schema.AssistantMessage("docker checked", nil),
		},
	}
	execTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "exec_command",
			Description:      "base exec description",
			RiskLevel:        tooling.ToolRiskHigh,
			Mutating:         true,
			RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				PermissionScope: "argument_scoped",
			},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "host_command",
				ResourceID:    "remote-linux-01",
				OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{"docker --version"},
			},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "Docker version 26.1.3"}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(execTool); err != nil {
		t.Fatalf("Register exec_command failed: %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}, compiler, model)

	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-resume-host-meta",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-resume-host-meta",
		HostID:      "remote-linux-01",
		Input:       "检查 Docker",
		Metadata: map[string]string{
			"aiops.host.metadataAvailable": "true",
			"aiops.host.id":                "remote-linux-01",
			"aiops.host.os":                "linux",
			"aiops.host.arch":              "amd64",
			"aiops.host.transport":         "agent_http",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}
	if len(compiler.contexts) == 0 {
		t.Fatal("expected initial compiler context")
	}
	if desc := compilerToolDescription(compiler.contexts[0], "exec_command"); !strings.Contains(desc, "os=linux") || strings.Contains(desc, "Host OS: darwin") {
		t.Fatalf("initial exec_command description = %q, want linux target metadata", desc)
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:  "sess-resume-host-meta",
		TurnID:     "turn-resume-host-meta",
		ApprovalID: "approval-test",
		Decision:   "approved",
		Metadata:   map[string]string{"approval.reason": "test approval"},
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", resumed.Status)
	}
	if len(compiler.contexts) < 2 {
		t.Fatalf("compiler contexts = %d, want resume context", len(compiler.contexts))
	}
	resumeDesc := compilerToolDescription(compiler.contexts[len(compiler.contexts)-1], "exec_command")
	if !strings.Contains(resumeDesc, "host=remote-linux-01") || !strings.Contains(resumeDesc, "os=linux") || !strings.Contains(resumeDesc, "transport=agent_http") {
		t.Fatalf("resume exec_command description = %q, want preserved remote linux metadata", resumeDesc)
	}
	if strings.Contains(resumeDesc, "Host OS: darwin") || strings.Contains(resumeDesc, "For host resource inspection on macOS") {
		t.Fatalf("resume exec_command description = %q, leaked local macOS guidance", resumeDesc)
	}
	session := kernel.sessions.Get("sess-resume-host-meta")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected resumed session")
	}
	if got := session.CurrentTurn.Metadata["approval.reason"]; got != "test approval" {
		t.Fatalf("resume metadata override missing: approval.reason = %q", got)
	}
}

func compilerToolDescription(ctx promptcompiler.CompileContext, name string) string {
	for _, toolDef := range ctx.AssembledTools {
		if toolDef == nil || toolDef.Metadata().Name != name {
			continue
		}
		if desc := strings.TrimSpace(toolDef.Metadata().Description); desc != "" {
			return desc
		}
		return toolDef.Description(nil, tooling.DescribeContext{
			SessionType: ctx.SessionType,
			Mode:        ctx.Mode,
			Metadata:    toolDef.Metadata(),
		})
	}
	return ""
}

func TestResumeTurn_ClearsPendingApprovalBeforeApprovedToolCompletes(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
			schema.AssistantMessage("write complete", nil),
		},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			startedOnce.Do(func() { close(started) })
			select {
			case <-release:
				return tooling.ToolResult{Content: syntheticPassVerificationReportContent(t, "vr-synthetic-clear-approval", "synthetic approved write")}, nil
			case <-ctx.Done():
				return tooling.ToolResult{}, ctx.Err()
			}
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-clear-approval",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-clear-approval",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}

	done := make(chan struct{})
	var resumed TurnResult
	var resumeErr error
	go func() {
		defer close(done)
		resumed, resumeErr = kernel.ResumeTurn(context.Background(), ResumeRequest{
			SessionID: "sess-clear-approval",
			TurnID:    "turn-clear-approval",
			Decision:  "approved",
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("approved tool did not start")
	}
	session := kernel.sessions.Get("sess-clear-approval")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session while approved tool is running")
	}
	if len(session.PendingApprovals) != 0 || len(session.CurrentTurn.PendingApprovals) != 0 {
		t.Fatalf("pending approvals while approved tool is running: session=%d turn=%d", len(session.PendingApprovals), len(session.CurrentTurn.PendingApprovals))
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ResumeTurn did not finish")
	}
	if resumeErr != nil {
		t.Fatalf("ResumeTurn failed: %v", resumeErr)
	}
	if resumed.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", resumed.Status)
	}
}

func TestResumeTurn_DrainsRemainingToolCallsBeforeNextModelRequest(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
				{
					ID:   "call-read",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_info",
						Arguments: `{"target":"docker"}`,
					},
				},
			}),
			schema.AssistantMessage("done", nil),
		},
	}

	var writes int
	var reads int
	writeTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			writes++
			return tooling.ToolResult{Content: "wrote:" + string(input)}, nil
		},
	}
	readTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_info",
			Description: "Read info",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			reads++
			return tooling.ToolResult{Content: "read:" + string(input)}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{writeTool, readTool}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-multi-tool-approval",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-multi-tool-approval",
		Input:       "write then read",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: "sess-multi-tool-approval",
		TurnID:    "turn-multi-tool-approval",
		Decision:  "approved",
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", resumed.Status)
	}
	if writes != 1 || reads != 1 {
		t.Fatalf("tool executions writes=%d reads=%d, want 1/1", writes, reads)
	}
	if len(model.inputs) < 2 {
		t.Fatalf("model inputs = %d, want resume to call model after tool outputs", len(model.inputs))
	}
	lastInput := model.inputs[len(model.inputs)-1]
	seenToolOutputs := map[string]bool{}
	for _, msg := range lastInput {
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		seenToolOutputs[msg.ToolCallID] = true
	}
	if !seenToolOutputs["call-approval"] || !seenToolOutputs["call-read"] {
		t.Fatalf("resume model input missing tool outputs: %#v", seenToolOutputs)
	}
}

func TestResumeTurn_FeedsApprovedToolFailureBackToModel(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
			schema.AssistantMessage("handled failure", nil),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("disk is read-only")
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-approved-failure",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-approved-failure",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: "sess-approved-failure",
		TurnID:    "turn-approved-failure",
		Decision:  "approved",
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if resumed.Status != "completed" || resumed.Output != "handled failure" {
		t.Fatalf("resume result = %#v, want completed handled failure", resumed)
	}
	if len(model.inputs) < 2 {
		t.Fatalf("model inputs = %d, want resume model call", len(model.inputs))
	}
	lastInput := model.inputs[len(model.inputs)-1]
	var failureToolMessage string
	for _, msg := range lastInput {
		if msg != nil && msg.Role == schema.Tool && msg.ToolCallID == "call-approval" {
			failureToolMessage = msg.Content
		}
	}
	if !strings.Contains(failureToolMessage, "disk is read-only") {
		t.Fatalf("failure tool message = %q, want error content", failureToolMessage)
	}
}

func TestResumeTurn_WithResumeMetadataContinuesSharedLoop(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("choice applied", nil),
		},
	}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	now := time.Now().UTC()

	session := kernel.sessions.GetOrCreate("sess-choice", SessionTypeWorkspace, ModeExecute)
	session.Messages = []Message{
		{ID: "msg-assistant", Role: "assistant", Content: "请选择下一步。", Timestamp: now},
	}
	checkpoint := &CheckpointMetadata{
		ID:          "choice-1",
		SessionID:   session.ID,
		TurnID:      "turn-choice",
		Iteration:   0,
		Sequence:    1,
		Kind:        "choice_needed",
		Lifecycle:   TurnLifecycleResumable,
		ResumeState: TurnResumeStateResumable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	session.CurrentTurn = &TurnSnapshot{
		ID:               "turn-choice",
		SessionID:        session.ID,
		SessionType:      session.Type,
		Mode:             session.Mode,
		Lifecycle:        TurnLifecycleResumable,
		ResumeState:      TurnResumeStateResumable,
		Iteration:        0,
		StartedAt:        now,
		UpdatedAt:        now,
		LatestCheckpoint: checkpoint,
		Iterations: []IterationState{
			{
				ID:          "turn-choice-iter-0",
				SessionID:   session.ID,
				TurnID:      "turn-choice",
				Iteration:   0,
				Lifecycle:   TurnLifecycleResumable,
				ResumeState: TurnResumeStateResumable,
				Checkpoint:  checkpoint,
				StartedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
	kernel.sessions.Update(session)

	result, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:    session.ID,
		TurnID:       "turn-choice",
		CheckpointID: "choice-1",
		ResumeState:  TurnResumeStateResumable,
		Metadata: map[string]string{
			"choice.requestId": "choice-1",
			"choice.answer.0":  "Continue with verification",
			"resume.input":     "Continue with verification",
		},
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", result.Status)
	}
	if len(model.inputs) == 0 {
		t.Fatal("expected resume to call shared iteration loop")
	}
	lastInput := model.inputs[len(model.inputs)-1]
	foundResumeMessage := false
	for _, msg := range lastInput {
		if msg == nil {
			continue
		}
		if msg.Role == schema.User && strings.Contains(msg.Content, "Continue with verification") {
			foundResumeMessage = true
			break
		}
	}
	if !foundResumeMessage {
		t.Fatalf("model input did not include resume message: %+v", lastInput)
	}
	session = kernel.sessions.Get(session.ID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected resumed current turn snapshot")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle after resume = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	foundUserMessage := false
	for _, msg := range session.Messages {
		if msg.Role == "user" && strings.Contains(msg.Content, "Continue with verification") {
			foundUserMessage = true
			break
		}
	}
	if !foundUserMessage {
		t.Fatalf("session messages did not record resume input: %+v", session.Messages)
	}
}

func TestRunTurn_ModelTimeoutBecomesRecoverableAndResumeContinues(t *testing.T) {
	model := &timeoutThenSuccessModel{}
	evidenceTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Description: "Collect timeout recovery evidence",
			RiskLevel:   tooling.ToolRiskLow,
			AlwaysLoad:  true,
			Layer:       tooling.ToolLayerCore,
			Pack:        "public_web",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "prior evidence survives timeout"}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(evidenceTool); err != nil {
		t.Fatalf("Register web_search failed: %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry, nil)}, compiler, model)

	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-model-timeout",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-model-timeout",
		Input:       "analyze current incident",
		Metadata: map[string]string{
			"aiops.intent.kind":          "research",
			"aiops.intent.dataScopes":    "public_web",
			"aiops.route.allowsWebLearn": "true",
			"enableToolPack":             "public_web",
			"enableTool":                 "web_search",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn timeout should be recoverable, got error: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("RunTurn status = %q, want blocked recoverable turn", blocked.Status)
	}
	if len(compiler.contexts) == 0 || !containsString(toolNames(compiler.contexts[0].AssembledTools), "web_search") {
		t.Fatalf("first tool surface = %v, want web_search", toolNames(compiler.contexts[0].AssembledTools))
	}
	session := kernel.sessions.Get("sess-model-timeout")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected persisted turn snapshot")
	}
	snapshot := session.CurrentTurn
	if snapshot.Lifecycle != TurnLifecycleResumable {
		t.Fatalf("snapshot lifecycle = %q, want resumable", snapshot.Lifecycle)
	}
	if snapshot.ResumeState != TurnResumeStateResumable {
		t.Fatalf("snapshot resume state = %q, want resumable", snapshot.ResumeState)
	}
	if snapshot.LatestCheckpoint == nil || snapshot.LatestCheckpoint.Kind != "model_timeout" {
		t.Fatalf("checkpoint = %#v, want model_timeout", snapshot.LatestCheckpoint)
	}
	if len(snapshot.Iterations) == 0 || len(snapshot.Iterations[len(snapshot.Iterations)-1].ToolResults) == 0 {
		t.Fatalf("expected tool evidence before timeout, iterations = %#v", snapshot.Iterations)
	}
	if err := ValidateTurnRecoveryPreconditions(snapshot); err != nil {
		t.Fatalf("timeout snapshot should be recoverable: %v", err)
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:    session.ID,
		TurnID:       snapshot.ID,
		CheckpointID: snapshot.LatestCheckpoint.ID,
		ResumeState:  TurnResumeStateResumable,
	})
	if err != nil {
		t.Fatalf("ResumeTurn after timeout failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("ResumeTurn status = %q, want completed", resumed.Status)
	}
	if model.attempts < 2 {
		t.Fatalf("model attempts = %d, want resume to re-enter model loop", model.attempts)
	}
	if !model.sawTool {
		t.Fatalf("resume model input did not preserve prior tool evidence; last input:\n%s", schemaMessagesText(model.inputs[len(model.inputs)-1]))
	}
}

func TestRunTurn_LargeToolResultIsSummarizedAndSpilled(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-large",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tail_logs",
						Arguments: `{"path":"/tmp/logs"}`,
					},
				},
			}),
			schema.AssistantMessage("logs reviewed", nil),
		},
	}

	largeContent := `{"lines":["alpha alpha alpha alpha alpha alpha","beta beta beta beta beta beta","gamma gamma gamma gamma gamma gamma","delta delta delta delta delta delta","epsilon epsilon epsilon epsilon epsilon epsilon"]}`
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "tail_logs",
			Description: "Tail logs",
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: 48,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: largeContent}, nil
		},
	}

	spillRepo := newMemoryToolResultSpillRepo()
	kernel := newLoopKernelWithDeps(t, model, []tooling.Tool{toolDef}, nil, nil, nil, spillRepo)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-spill",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-spill",
		Input:       "inspect logs",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-spill")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session and current turn")
	}
	if len(session.CurrentTurn.ExternalReferences) != 1 {
		t.Fatalf("external references = %d, want 1", len(session.CurrentTurn.ExternalReferences))
	}
	ref := session.CurrentTurn.ExternalReferences[0]
	if ref.URI == "" {
		t.Fatal("expected external reference URI")
	}
	spill, err := spillRepo.GetToolResultSpill(ref.ID)
	if err != nil {
		t.Fatalf("GetToolResultSpill failed: %v", err)
	}
	if string(spill.Content) != largeContent {
		t.Fatalf("spill content mismatch")
	}
	if len(session.Messages) < 3 {
		t.Fatalf("session messages len = %d, want at least 3", len(session.Messages))
	}
	toolMsg := session.Messages[len(session.Messages)-2]
	if toolMsg.ToolResult == nil {
		t.Fatal("expected tool result message")
	}
	if !toolMsg.ToolResult.Spilled {
		t.Fatal("expected spilled tool result")
	}
	if !strings.Contains(toolMsg.Content, "Summary:") {
		t.Fatalf("tool message content = %q, want summary marker", toolMsg.Content)
	}
	if strings.Contains(toolMsg.Content, "Preview:") {
		t.Fatalf("tool message content = %q, should not include preview marker for large results", toolMsg.Content)
	}
	if len(toolMsg.ToolResult.References) != 1 {
		t.Fatalf("tool message references = %d, want 1", len(toolMsg.ToolResult.References))
	}
	if toolMsg.ToolResult.References[0].Kind != ToolResultReferenceKindBlob {
		t.Fatalf("tool message reference kind = %q, want %q", toolMsg.ToolResult.References[0].Kind, ToolResultReferenceKindBlob)
	}
	if len(toolMsg.ToolResult.ExternalReferences) != 1 {
		t.Fatalf("tool message external references = %d, want 1", len(toolMsg.ToolResult.ExternalReferences))
	}
	if toolMsg.ToolResult.MaterializationTier != "large" {
		t.Fatalf("materialization tier = %q, want large", toolMsg.ToolResult.MaterializationTier)
	}
	if toolMsg.ToolResult.OriginalBytes != int64(len(largeContent)) {
		t.Fatalf("original bytes = %d, want %d", toolMsg.ToolResult.OriginalBytes, len(largeContent))
	}
	if toolMsg.ToolResult.InlineBytes != int64(len(toolMsg.ToolResult.Content)) {
		t.Fatalf("inline bytes = %d, want %d", toolMsg.ToolResult.InlineBytes, len(toolMsg.ToolResult.Content))
	}
	materializedEvents := latestToolResultGovernanceEvents(session, "call-large")
	if len(materializedEvents) != 1 {
		t.Fatalf("materialization events = %#v, want 1", materializedEvents)
	}
	if got := materializedEvents[0]; got.Layer != ContextGovernanceLayerL1 || got.Kind != "tool_result.materialized" {
		t.Fatalf("materialization event = %#v, want L1 tool_result.materialized", got)
	}
	if len(materializedEvents[0].ReferenceIDs) != 1 || materializedEvents[0].ReferenceIDs[0] != ref.ID {
		t.Fatalf("materialization reference ids = %#v, want %q", materializedEvents[0].ReferenceIDs, ref.ID)
	}
	if toolMsg.Content == largeContent {
		t.Fatal("expected inline tool content to be summarized, not full payload")
	}
	if len(model.inputs) != 2 {
		t.Fatalf("Generate calls = %d, want 2", len(model.inputs))
	}
	foundToolSummary := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-large" {
			if msg.Content == largeContent {
				t.Fatal("model received full spilled content instead of summary")
			}
			foundToolSummary = true
		}
	}
	if !foundToolSummary {
		t.Fatal("expected second model input to include summarized tool result")
	}
}

func TestRunTurn_MediumToolResultKeepsSummaryOnlyAndSpillsFullContent(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-medium",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tail_logs",
						Arguments: `{"path":"/tmp/medium.log"}`,
					},
				},
			}),
			schema.AssistantMessage("preview reviewed", nil),
		},
	}

	mediumContent := strings.Repeat("alpha ", 14)
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "tail_logs",
			Description: "Tail logs",
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: 48,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: mediumContent}, nil
		},
	}

	spillRepo := newMemoryToolResultSpillRepo()
	kernel := newLoopKernelWithDeps(t, model, []tooling.Tool{toolDef}, nil, nil, nil, spillRepo)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-medium-spill",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-medium-spill",
		Input:       "inspect medium logs",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-medium-spill")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session and current turn")
	}
	toolMsg := session.Messages[len(session.Messages)-2]
	if toolMsg.ToolResult == nil {
		t.Fatal("expected tool result message")
	}
	if !toolMsg.ToolResult.Spilled {
		t.Fatal("expected spilled tool result")
	}
	if strings.Contains(toolMsg.Content, "Preview:") {
		t.Fatalf("tool message content = %q, should not include preview marker", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, "Summary:") {
		t.Fatalf("tool message content = %q, want summary marker", toolMsg.Content)
	}
	if len(toolMsg.ToolResult.References) != 1 {
		t.Fatalf("tool message references = %d, want 1", len(toolMsg.ToolResult.References))
	}
	if toolMsg.ToolResult.References[0].Kind != ToolResultReferenceKindBlob {
		t.Fatalf("tool message reference kind = %q, want %q", toolMsg.ToolResult.References[0].Kind, ToolResultReferenceKindBlob)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("Generate calls = %d, want 2", len(model.inputs))
	}
	foundSummaryOnly := false
	for _, msg := range model.inputs[1] {
		if msg.Role != schema.Tool || msg.ToolCallID != "call-medium" {
			continue
		}
		if strings.Contains(msg.Content, "Preview:") {
			t.Fatalf("model input tool content = %q, should not include preview marker", msg.Content)
		}
		if strings.Contains(msg.Content, "Summary:") && strings.Contains(msg.Content, "External ref:") {
			foundSummaryOnly = true
		}
	}
	if !foundSummaryOnly {
		t.Fatal("expected second model input to include summarized tool result with external ref")
	}
}

func TestRunTurn_ReadOnlyConcurrencySafeToolsRunInParallel(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{ID: "call-a", Type: "function", Function: schema.FunctionCall{Name: "read_a", Arguments: `{}`}},
				{ID: "call-b", Type: "function", Function: schema.FunctionCall{Name: "read_b", Arguments: `{}`}},
			}),
			schema.AssistantMessage("parallel reads complete", nil),
		},
	}

	aStarted := make(chan struct{})
	bStarted := make(chan struct{})
	var closeA sync.Once
	var closeB sync.Once
	var overlapped atomic.Bool
	readA := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "read_a", Description: "read A"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			closeA.Do(func() { close(aStarted) })
			select {
			case <-bStarted:
				overlapped.Store(true)
				return tooling.ToolResult{Content: "A"}, nil
			case <-time.After(500 * time.Millisecond):
				return tooling.ToolResult{}, errors.New("read_b did not overlap read_a")
			}
		},
	}
	readB := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "read_b", Description: "read B"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			<-aStarted
			closeB.Do(func() { close(bStarted) })
			return tooling.ToolResult{Content: "B"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{readA, readB}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-parallel",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-parallel",
		Input:       "read both",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if !overlapped.Load() {
		t.Fatal("read-only concurrency-safe tools did not overlap")
	}
	session := kernel.sessions.Get("sess-parallel")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing turn iterations for parallel trace: %#v", session)
	}
	firstIter := session.CurrentTurn.Iterations[0]
	if len(firstIter.ParallelDispatchGroups) != 1 {
		t.Fatalf("parallel dispatch groups = %#v, want one group", firstIter.ParallelDispatchGroups)
	}
	group := firstIter.ParallelDispatchGroups[0]
	for _, want := range []string{"read_only", "non_destructive", "concurrency_safe", "no_approval_required", "shared_resource_key"} {
		if !containsString(group.Reasons, want) {
			t.Fatalf("parallel dispatch reasons = %v, want %q", group.Reasons, want)
		}
	}
	tracePath := session.CurrentTurn.Iterations[1].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read second iteration trace: %v", err)
	}
	traceJSON := string(data)
	for _, want := range []string{`"parallelDispatchGroups"`, `"decision": "parallel"`, `"read_only"`, `"shared_resource_key"`} {
		if !strings.Contains(traceJSON, want) {
			t.Fatalf("runtime trace missing %q:\n%s", want, traceJSON)
		}
	}
}

func TestRunTurn_MutatingToolsSerializeEvenWhenPolicyAllowsExecution(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{ID: "call-a", Type: "function", Function: schema.FunctionCall{Name: "mutate_a", Arguments: `{}`}},
				{ID: "call-b", Type: "function", Function: schema.FunctionCall{Name: "mutate_b", Arguments: `{}`}},
			}),
			schema.AssistantMessage("mutations complete", nil),
		},
	}

	var active int32
	var maxActive int32
	mutate := func(content string) func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
		return func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				seen := atomic.LoadInt32(&maxActive)
				if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
					break
				}
			}
			time.Sleep(25 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return tooling.ToolResult{Content: content}, nil
		}
	}
	toolA := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "mutate_a",
			Description: "mutate A",
			Mutating:    true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "synthetic_resource",
				ResourceID:    "a",
				OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{"check-a"},
			},
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		DestructiveFunc:     func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		ExecuteFunc:         mutate("A"),
	}
	toolB := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "mutate_b",
			Description: "mutate B",
			Mutating:    true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "synthetic_resource",
				ResourceID:    "b",
				OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{"check-b"},
			},
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		DestructiveFunc:     func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		ExecuteFunc:         mutate("B"),
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolA, toolB}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-serialize",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-serialize",
		Input:       "mutate both",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("mutating tools ran concurrently, max active = %d", got)
	}
}

func TestRunTurn_ToolResultPreservesExplicitReferences(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-artifacts",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_artifacts",
						Arguments: `{"path":"/tmp/output"}`,
					},
				},
			}),
			schema.AssistantMessage("artifacts reviewed", nil),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_artifacts",
			Description: "Read artifacts",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Content: "artifacts ready",
				Display: &tooling.ToolDisplayPayload{
					Type:    "artifact-card",
					Title:   "Artifacts",
					CardRef: "card-artifacts",
				},
				References: []tooling.ResultReference{
					{
						Kind:     tooling.ResultReferenceKindFile,
						FilePath: "/tmp/output/report.txt",
						Title:    "Artifact report",
					},
				},
			}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-artifacts",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-artifacts",
		Input:       "read artifacts",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-artifacts")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected session and current turn")
	}
	toolMsg := session.Messages[len(session.Messages)-2]
	if toolMsg.ToolResult == nil {
		t.Fatal("expected tool result message")
	}
	if len(toolMsg.ToolResult.References) != 2 {
		t.Fatalf("tool message references = %d, want 2", len(toolMsg.ToolResult.References))
	}
	if !containsToolResultReferenceKind(toolMsg.ToolResult.References, ToolResultReferenceKindCard) {
		t.Fatalf("tool result references = %+v, want card reference", toolMsg.ToolResult.References)
	}
	if !containsToolResultReferenceKind(toolMsg.ToolResult.References, ToolResultReferenceKindFile) {
		t.Fatalf("tool result references = %+v, want file reference", toolMsg.ToolResult.References)
	}
	if len(toolMsg.ToolResult.ExternalReferences) != 2 {
		t.Fatalf("tool message external references = %d, want 2", len(toolMsg.ToolResult.ExternalReferences))
	}
	if len(session.CurrentTurn.ExternalReferences) != 2 {
		t.Fatalf("current turn external references = %d, want 2", len(session.CurrentTurn.ExternalReferences))
	}
	if !containsExternalCardRef(session.CurrentTurn.ExternalReferences, "card-artifacts") {
		t.Fatalf("current turn external references = %+v, want card ref", session.CurrentTurn.ExternalReferences)
	}
	if !containsExternalFilePath(session.CurrentTurn.ExternalReferences, "/tmp/output/report.txt") {
		t.Fatalf("current turn external references = %+v, want file path", session.CurrentTurn.ExternalReferences)
	}
}

func TestRunTurn_ContextPipelineCompactsOlderMessages(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("final answer", nil),
		},
	}
	compressor := spanstream.NewContextCompressor(&fixedSummaryModel{response: "compressed earlier context"}, 1)
	kernel := newLoopKernelWithDeps(t, model, nil, nil, nil, compressor, nil)
	session := kernel.sessions.GetOrCreate("sess-compact", SessionTypeHost, ModeChat)
	session.Context = ContextWindow{MaxTokens: 64}
	for i := 0; i < 10; i++ {
		session.Messages = append(session.Messages, Message{
			ID:        "m-" + string(rune('a'+i)),
			Role:      "user",
			Content:   "very long historical message payload " + strings.Repeat("x", 40),
			Timestamp: time.Now(),
		})
	}
	kernel.sessions.Update(session)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-compact",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-compact",
		Input:       "continue",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session = kernel.sessions.Get("sess-compact")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if len(session.CurrentTurn.CompactedSegments) == 0 {
		t.Fatal("expected compacted segments to be recorded")
	}
	firstIter := session.CurrentTurn.Iterations[0]
	if len(firstIter.CompactedSegments) == 0 {
		t.Fatal("expected iteration compacted segments to be recorded")
	}
	if len(model.inputs) != 1 {
		t.Fatalf("Generate calls = %d, want 1", len(model.inputs))
	}
	if len(model.inputs[0]) == 0 || model.inputs[0][len(model.inputs[0])-1].Content != "continue" {
		t.Fatalf("expected latest user message to remain in model input")
	}
}

func TestRunTurn_HookBlockedDecisionPersistsCheckpointSource(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-hook-block",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp/demo"}`,
					},
				},
			}),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Read disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "approval-gate",
		Stage: hooks.StagePreToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedPermissions = &tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "hook approval required",
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, registry, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hook-blocked",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-hook-blocked",
		Input:       "inspect disk",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}

	session := kernel.sessions.Get("sess-hook-blocked")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if session.CurrentTurn.LatestCheckpoint == nil {
		t.Fatal("expected latest checkpoint")
	}
	if session.CurrentTurn.LatestCheckpoint.Kind != "approval_needed" {
		t.Fatalf("checkpoint kind = %q, want approval_needed", session.CurrentTurn.LatestCheckpoint.Kind)
	}
	if session.CurrentTurn.LatestCheckpoint.Source != "hook" {
		t.Fatalf("checkpoint source = %q, want hook", session.CurrentTurn.LatestCheckpoint.Source)
	}
	if session.CurrentTurn.Error != "hook approval required" {
		t.Fatalf("turn error = %q, want hook approval required", session.CurrentTurn.Error)
	}
	if len(session.CurrentTurn.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(session.CurrentTurn.PendingApprovals))
	}
	if last := latestIteration(session.CurrentTurn); last == nil || last.Checkpoint == nil || last.Checkpoint.Source != "hook" {
		t.Fatalf("iteration checkpoint = %#v, want hook-sourced checkpoint", last)
	}
}

func TestRunTurn_PolicyDeniedToolPersistsFailureCheckpoint(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-policy-deny",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "should not run"}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-policy-deny",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-policy-deny",
		Input:       "write a file",
	})
	if err == nil {
		t.Fatal("expected RunTurn to fail")
	}

	session := kernel.sessions.Get("sess-policy-deny")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("turn lifecycle = %q, want failed", session.CurrentTurn.Lifecycle)
	}
	if session.CurrentTurn.LatestCheckpoint == nil {
		t.Fatal("expected failure checkpoint")
	}
	if session.CurrentTurn.LatestCheckpoint.Kind != "tool_denied" {
		t.Fatalf("checkpoint kind = %q, want tool_denied", session.CurrentTurn.LatestCheckpoint.Kind)
	}
	if session.CurrentTurn.LatestCheckpoint.Source != "policy" {
		t.Fatalf("checkpoint source = %q, want policy", session.CurrentTurn.LatestCheckpoint.Source)
	}
	if !strings.Contains(session.CurrentTurn.Error, "inspect mode does not allow mutation operations") {
		t.Fatalf("turn error = %q", session.CurrentTurn.Error)
	}
	if last := latestIteration(session.CurrentTurn); last == nil || last.Checkpoint == nil || last.Checkpoint.Kind != "tool_denied" {
		t.Fatalf("iteration checkpoint = %#v, want tool_denied", last)
	}
}

func TestRunTurn_ModelGenerationErrorPersistsFailedTurn(t *testing.T) {
	model := &sequentialLoopModel{generateErr: errors.New("429 cooling down")}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-model-error",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-model-error",
		Input:       "call the model",
	})
	if err == nil {
		t.Fatal("expected RunTurn to fail")
	}

	session := kernel.sessions.Get("sess-model-error")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("turn lifecycle = %q, want failed", session.CurrentTurn.Lifecycle)
	}
	if !strings.Contains(session.CurrentTurn.Error, "429 cooling down") {
		t.Fatalf("turn error = %q, want model error", session.CurrentTurn.Error)
	}
	if session.CurrentTurn.LatestCheckpoint == nil {
		t.Fatal("expected failure checkpoint")
	}
	if session.CurrentTurn.LatestCheckpoint.Kind != "model_call_failed" {
		t.Fatalf("checkpoint kind = %q, want model_call_failed", session.CurrentTurn.LatestCheckpoint.Kind)
	}
	if session.CurrentTurn.LatestCheckpoint.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("checkpoint lifecycle = %q, want failed", session.CurrentTurn.LatestCheckpoint.Lifecycle)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeModelCall, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed model_call item", session.CurrentTurn.AgentItems)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeError, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed error item", session.CurrentTurn.AgentItems)
	}
}

func TestRunTurn_RefreshesToolsBetweenIterations(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-connect",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_remote_registry",
						Arguments: `{"server":"dynamic"}`,
					},
				},
			}),
			schema.AssistantMessage("remote attached", nil),
		},
	}

	baseRegistry := tooling.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	dynamicTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_remote_metrics",
			Description: "Inspect remote metrics",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "remote-metrics"}, nil
		},
	}
	connectTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_remote_registry",
			Description: "Connect a remote MCP surface",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			if err := mcpRegistry.OnServerConnected("dynamic", []tooling.Tool{dynamicTool}); err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{Content: "connected"}, nil
		},
	}
	if err := baseRegistry.Register(connectTool); err != nil {
		t.Fatalf("Register connect tool failed: %v", err)
	}

	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(baseRegistry, mcpRegistry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-refresh",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-refresh",
		Input:       "attach remote tools",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}

	secondTools := toolNames(compiler.contexts[1].AssembledTools)
	if containsString(secondTools, "read_remote_metrics") {
		t.Fatalf("second iteration tools = %v, dynamic MCP tool should remain deferred until selected", secondTools)
	}

	session := kernel.sessions.Get("sess-refresh")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if session.CurrentTurn.StableToolFingerprint == "" {
		t.Fatal("expected stable tool fingerprint to be recorded")
	}
	if session.CurrentTurn.ToolSurfaceSnapshot == nil {
		t.Fatal("expected turn tool surface snapshot ref")
	}
	if session.CurrentTurn.ToolSurfaceSnapshot.Fingerprint != session.CurrentTurn.StableToolFingerprint {
		t.Fatalf("turn tool surface snapshot fingerprint = %q, want stable fingerprint %q", session.CurrentTurn.ToolSurfaceSnapshot.Fingerprint, session.CurrentTurn.StableToolFingerprint)
	}
	if containsString(session.CurrentTurn.ToolSurfaceSnapshot.ToolNames, "read_remote_metrics") {
		t.Fatalf("turn tool surface snapshot tools = %v, dynamic MCP tool should remain deferred until selected", session.CurrentTurn.ToolSurfaceSnapshot.ToolNames)
	}
	if session.CurrentTurn.StablePromptHash == "" {
		t.Fatal("expected stable prompt hash to be recorded")
	}
	if len(session.CurrentTurn.Iterations) != 2 {
		t.Fatalf("iterations = %d, want 2", len(session.CurrentTurn.Iterations))
	}
	if session.CurrentTurn.Iterations[0].ToolSurfaceFingerprint == "" {
		t.Fatal("expected iteration[0] tool surface fingerprint to be recorded")
	}
	if session.CurrentTurn.Iterations[1].ToolSurfaceFingerprint == "" {
		t.Fatal("expected iteration[1] tool surface fingerprint to be recorded")
	}
	if session.CurrentTurn.Iterations[1].ToolSurfaceFingerprint != session.CurrentTurn.StableToolFingerprint {
		t.Fatalf("latest iteration tool surface fingerprint = %q, want current turn stable fingerprint %q", session.CurrentTurn.Iterations[1].ToolSurfaceFingerprint, session.CurrentTurn.StableToolFingerprint)
	}
	if session.CurrentTurn.Iterations[1].ToolSurfaceSnapshot == nil {
		t.Fatal("expected iteration[1] tool surface snapshot ref")
	}
	if session.CurrentTurn.Iterations[1].ToolSurfaceSnapshot.Fingerprint != session.CurrentTurn.Iterations[1].ToolSurfaceFingerprint {
		t.Fatalf("iteration[1] snapshot fingerprint = %q, want %q", session.CurrentTurn.Iterations[1].ToolSurfaceSnapshot.Fingerprint, session.CurrentTurn.Iterations[1].ToolSurfaceFingerprint)
	}
	if containsString(session.CurrentTurn.Iterations[1].RefreshedTools, "read_remote_metrics") {
		t.Fatalf("iteration[1] refreshed tools = %v, dynamic MCP tool should remain deferred until selected", session.CurrentTurn.Iterations[1].RefreshedTools)
	}
	if session.CurrentTurn.Iterations[1].PromptDelta == "" {
		t.Fatal("expected dynamic prompt delta to be recorded")
	}
	if containsString(compiler.contexts[1].ToolDelta.NewlyAvailable, "read_remote_metrics") {
		t.Fatalf("second iteration tool delta = %v, dynamic MCP tool should remain deferred until selected", compiler.contexts[1].ToolDelta.NewlyAvailable)
	}
	if strings.Contains(session.CurrentTurn.Iterations[1].PromptDelta, "# Tool Index") {
		t.Fatal("prompt delta should not re-emit the stable tool index")
	}
}

func TestIterationToolDeltaReportsNewlyAvailablePacks(t *testing.T) {
	snapshot := &TurnSnapshot{Iterations: []IterationState{{
		VisibleTools: []string{"search_ops_manuals"},
	}}}
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "search_ops_manuals", Layer: tooling.ToolLayerCore}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "resolve_ops_manual_params", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow"}},
	}

	delta := iterationToolDelta(snapshot, tools)
	if !containsString(delta.NewlyAvailable, "resolve_ops_manual_params") {
		t.Fatalf("newly available tools = %v, want resolve_ops_manual_params", delta.NewlyAvailable)
	}
	if !containsString(delta.NewlyAvailablePacks, "ops_manual_flow") {
		t.Fatalf("newly available packs = %v, want ops_manual_flow", delta.NewlyAvailablePacks)
	}

	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{ToolDelta: delta})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if !strings.Contains(compiled.Dynamic.ToolDelta.Content, "Newly available tool packs") || !strings.Contains(compiled.Dynamic.ToolDelta.Content, "ops_manual_flow") {
		t.Fatalf("tool delta content missing pack section:\n%s", compiled.Dynamic.ToolDelta.Content)
	}
}

func TestRunTurn_ProgressivelyEnablesOpsManualFlowTools(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-search-manual",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "search_ops_manuals",
						Arguments: `{"text":"检查 Redis 状态，不要重启"}`,
					},
				},
			}),
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-resolve-params",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "resolve_ops_manual_params",
						Arguments: `{"manual_id":"manual-redis-rca"}`,
					},
				},
			}),
			schema.AssistantMessage("manual flow ready", nil),
		},
	}

	registry := tooling.NewRegistry()
	for _, toolDef := range opsManualFlowRuntimeTestTools() {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-ops-manual-flow",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-ops-manual-flow",
		Input:       "@ops_manuals 检查 Redis 状态，不要重启",
		Metadata: map[string]string{
			"enableToolPack":                   "ops_manual_flow",
			"enableTool":                       "search_ops_manuals",
			"aiops.opsManuals.explicitMention": "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 3 {
		t.Fatalf("compiler contexts = %d, want 3", len(compiler.contexts))
	}

	first := toolNames(compiler.contexts[0].AssembledTools)
	for _, want := range []string{"search_ops_manuals"} {
		if !containsString(first, want) {
			t.Fatalf("first iteration tools = %v, want %s", first, want)
		}
	}
	for _, forbidden := range []string{"resolve_ops_manual_params", "run_ops_manual_preflight"} {
		if containsString(first, forbidden) {
			t.Fatalf("first iteration tools = %v, should not include %s", first, forbidden)
		}
	}

	second := toolNames(compiler.contexts[1].AssembledTools)
	if !containsString(second, "resolve_ops_manual_params") {
		t.Fatalf("second iteration tools = %v, want resolve_ops_manual_params after matched search", second)
	}
	if containsString(second, "run_ops_manual_preflight") {
		t.Fatalf("second iteration tools = %v, should not include preflight before params resolve", second)
	}

	third := toolNames(compiler.contexts[2].AssembledTools)
	if !containsString(third, "run_ops_manual_preflight") {
		t.Fatalf("third iteration tools = %v, want run_ops_manual_preflight after resolved params", third)
	}
}

func TestRunTurn_FeedsHiddenInternalEvidenceToolCallBackToModel(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-hidden-evidence-record",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "evidence.record",
						Arguments: `{"summary":"legacy call"}`,
					},
				},
			}),
			schema.AssistantMessage("continued without evidence writer", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "evidence.record",
			Description: "internal evidence writer",
			Layer:       tooling.ToolLayerInternal,
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "should not execute"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hidden-evidence",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-hidden-evidence",
		Input:       "legacy evidence record",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	var failureToolMessage string
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-hidden-evidence-record" {
			failureToolMessage = msg.Content
			break
		}
	}
	if !strings.Contains(failureToolMessage, "tool not found: evidence.record") {
		t.Fatalf("hidden evidence failure message = %q, want tool-not-found feedback", failureToolMessage)
	}
}

func opsManualFlowRuntimeTestTools() []tooling.Tool {
	return []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        "search_ops_manuals",
				Description: "search manuals",
				Layer:       tooling.ToolLayerCore,
				RiskLevel:   tooling.ToolRiskLow,
			},
			ReadOnlyFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				data := json.RawMessage(`{"decision":"need_info","manuals":[{"manual":{"id":"manual-redis-rca","title":"Redis RCA"},"usable_mode":"need_info"}]}`)
				return tooling.ToolResult{
					Content: `{"decision":"need_info","manuals":[{"id":"manual-redis-rca"}]}`,
					Display: &tooling.ToolDisplayPayload{
						Type:  "ops_manual_search_result",
						Title: "search_ops_manuals",
						Data:  data,
					},
				}, nil
			},
		},
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:           "resolve_ops_manual_params",
				Description:    "resolve params",
				Layer:          tooling.ToolLayerDeferred,
				Pack:           "ops_manual_flow",
				DeferByDefault: true,
				RiskLevel:      tooling.ToolRiskLow,
			},
			ReadOnlyFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				data := json.RawMessage(`{"status":"resolved","manual_id":"manual-redis-rca","workflow_id":"wf-redis-rca","resolved_params":[{"id":"target_instance","value":"redis-01"}]}`)
				return tooling.ToolResult{
					Content: `{"status":"resolved","next_action":"run_preflight"}`,
					Display: &tooling.ToolDisplayPayload{
						Type:  "ops_manual_param_resolution",
						Title: "resolve_ops_manual_params",
						Data:  data,
					},
				}, nil
			},
		},
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:           "run_ops_manual_preflight",
				Description:    "preflight",
				Layer:          tooling.ToolLayerDeferred,
				Pack:           "ops_manual_flow",
				DeferByDefault: true,
				RiskLevel:      tooling.ToolRiskLow,
			},
			ReadOnlyFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				return tooling.ToolResult{Content: `{"status":"passed"}`}, nil
			},
		},
	}
}

func TestRunTurn_PostToolHookHidesToolForNextIteration(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-hide-surface",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_restricted_surface",
						Arguments: `{}`,
					},
				},
			}),
			schema.AssistantMessage("surface updated", nil),
		},
	}

	restrictTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_restricted_surface",
			Description: "Prepare a restricted tool surface",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "restricted"}, nil
		},
	}
	hiddenTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_remote_metrics",
			Description: "Read remote metrics",
			AlwaysLoad:  true,
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "metrics"}, nil
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "hide-read-remote",
		Stage: hooks.StagePostToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			if event.Tool.Name != "read_restricted_surface" {
				return nil
			}
			event.HideTools = append(event.HideTools, "read_remote_metrics")
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{
		assembler: tooling.NewAssembler(func() *tooling.Registry {
			reg := tooling.NewRegistry()
			if err := reg.Register(restrictTool); err != nil {
				t.Fatalf("Register restrict tool failed: %v", err)
			}
			if err := reg.Register(hiddenTool); err != nil {
				t.Fatalf("Register hidden tool failed: %v", err)
			}
			return reg
		}()),
	}, compiler, model)
	kernel.hooks = registry

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hook-hide",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-hook-hide",
		Input:       "restrict the surface",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	if !containsString(toolNames(compiler.contexts[0].AssembledTools), "read_remote_metrics") {
		t.Fatal("first iteration should include read_remote_metrics")
	}
	if containsString(toolNames(compiler.contexts[1].AssembledTools), "read_remote_metrics") {
		t.Fatal("second iteration should hide read_remote_metrics")
	}
	if !containsString(compiler.contexts[1].ToolDelta.TemporarilyUnavailable, "read_remote_metrics") {
		t.Fatalf("second iteration tool delta = %v, want read_remote_metrics unavailable", compiler.contexts[1].ToolDelta.TemporarilyUnavailable)
	}

	session := kernel.sessions.Get("sess-hook-hide")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if !containsString(session.CurrentTurn.HiddenTools, "read_remote_metrics") {
		t.Fatalf("hidden tools = %v, want read_remote_metrics", session.CurrentTurn.HiddenTools)
	}
	if containsString(session.CurrentTurn.Iterations[1].VisibleTools, "read_remote_metrics") {
		t.Fatalf("visible tools = %v, should hide read_remote_metrics", session.CurrentTurn.Iterations[1].VisibleTools)
	}
}

func TestRunTurnMetadataToolSurfaceHidesExecCommand(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("只基于已有信息分析，不执行命令。", nil),
			schema.AssistantMessage("结论保持受限：不执行命令。", nil),
		},
	}
	execTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:       "exec_command",
			Layer:      tooling.ToolLayerCore,
			AlwaysLoad: true,
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "should not execute"}, nil
		},
	}
	webTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:       "web_search",
			Layer:      tooling.ToolLayerCore,
			Pack:       "public_web",
			AlwaysLoad: true,
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "search"}, nil
		},
	}
	registry := tooling.NewRegistry()
	for _, toolDef := range []tooling.Tool{execTool, webTool} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register(%s) error = %v", toolDef.Metadata().Name, err)
		}
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{
		assembler: tooling.NewAssembler(registry),
	}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-metadata-no-exec",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-metadata-no-exec",
		Input:       "PG timeline 为什么更高？只分析，不执行命令。",
		Metadata: map[string]string{
			"aiops.route.mode":              "chat_advisory",
			"aiops.tool.execCommandAllowed": "false",
			"enableToolPack":                "public_web",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) == 0 {
		t.Fatal("compiler contexts = 0, want at least 1")
	}
	if names := toolNames(compiler.contexts[0].AssembledTools); containsString(names, "exec_command") {
		t.Fatalf("compiled tools = %v, want exec_command hidden", names)
	} else if !containsString(names, "web_search") {
		t.Fatalf("compiled tools = %v, want web_search visible", names)
	}
	session := kernel.sessions.Get("sess-metadata-no-exec")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) == 0 {
		t.Fatal("expected persisted turn iteration")
	}
	if containsString(session.CurrentTurn.Iterations[0].VisibleTools, "exec_command") {
		t.Fatalf("iteration visible tools = %v, want exec_command hidden", session.CurrentTurn.Iterations[0].VisibleTools)
	}
}

func TestRunTurnUserEvidenceRCAHidesWebSearchAndDoesNotRequireTargetBinding(t *testing.T) {
	answer := "结论（置信度：中）：基于用户提供日志，最可能是应用启动时数据库连接失败。缺失证据：namespace、Pod 名称、数据库端点和最近事件。"
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage(answer, nil),
			schema.AssistantMessage(answer, nil),
			schema.AssistantMessage(answer, nil),
		},
	}
	registry := tooling.NewRegistry()
	for _, toolDef := range []tooling.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "exec_command", Layer: tooling.ToolLayerCore, AlwaysLoad: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "web_search", Layer: tooling.ToolLayerCore, Pack: "public_web", AlwaysLoad: true}},
	} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register(%s) error = %v", toolDef.Metadata().Name, err)
		}
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{
		assembler: tooling.NewAssembler(registry),
	}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-user-evidence-rca-no-web",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-user-evidence-rca-no-web",
		Input: "线上 Kubernetes Pod 一直 CrashLoopBackOff。\n" +
			"kubectl describe 里看到 Last State: Terminated, Exit Code: 1，Back-off restarting failed container。\n" +
			"应用日志最后一行是 failed to connect database。请分析",
		Metadata: map[string]string{
			"aiops.route.mode":              "evidence_rca",
			"aiops.target.binding":          "none",
			"aiops.tool.execCommandAllowed": "false",
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "log",
			"taskDepth.analysisOnly":        "true",
			"taskDepth.executionProhibited": "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if strings.Contains(result.Output, "@host") || strings.Contains(result.Output, "@IP") || strings.Contains(result.Output, "绑定") {
		t.Fatalf("final output = %q, should not require target binding for analysis-only user evidence", result.Output)
	}
	if len(compiler.contexts) == 0 {
		t.Fatal("compiler contexts = 0, want at least 1")
	}
	if names := toolNames(compiler.contexts[0].AssembledTools); containsString(names, "web_search") || containsString(names, "exec_command") {
		t.Fatalf("compiled tools = %v, want no ambient web_search or exec_command", names)
	}
}

func TestRunTurn_StreamingToolEmitsProgressAndPersistsCheckpoint(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-stream",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_stream_logs",
						Arguments: `{"path":"/tmp/stream.log"}`,
					},
				},
			}),
			schema.AssistantMessage("stream complete", nil),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_stream_logs",
			Description: "Stream log content",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Stream: &tooling.StreamingResult{
					Reader:    strings.NewReader("alpha-beta-gamma"),
					ChunkSize: 5,
				},
			}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-stream",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-stream",
		Input:       "stream logs",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-stream")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn snapshot")
	}
	if len(session.CurrentTurn.Iterations) == 0 {
		t.Fatal("expected at least one iteration")
	}
	iter := session.CurrentTurn.Iterations[0]
	if len(iter.ToolProgress) < 2 {
		t.Fatalf("tool progress updates = %d, want at least 2", len(iter.ToolProgress))
	}
	if iter.ToolProgress[0].Delta == "" {
		t.Fatal("expected first progress update to contain partial content")
	}
	if !iter.ToolProgress[len(iter.ToolProgress)-1].Done {
		t.Fatal("expected final progress update to mark completion")
	}
	if iter.Checkpoint == nil || iter.Checkpoint.Kind != "tool_result" {
		t.Fatalf("iteration checkpoint = %#v, want final tool_result checkpoint", iter.Checkpoint)
	}

	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected test projector")
	}
	progressEvents := 0
	for _, event := range emitter.events {
		if event.Type == EventToolProgress {
			progressEvents++
		}
	}
	if progressEvents == 0 {
		t.Fatal("expected tool.progress events to be emitted")
	}
	if len(session.Messages) < 2 {
		t.Fatalf("session messages len = %d, want >= 2", len(session.Messages))
	}
	foundToolMessage := false
	for _, msg := range session.Messages {
		if msg.Role == "tool" && msg.ToolResult != nil && msg.ToolResult.ToolCallID == "call-stream" {
			foundToolMessage = msg.Content == "alpha-beta-gamma"
			break
		}
	}
	if !foundToolMessage {
		t.Fatal("expected final tool message to contain full streamed content")
	}
}

func TestRunTurn_StreamingToolPartialResultFeedsNextIterationContext(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-stream-partial",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_stream_logs",
						Arguments: `{"path":"/tmp/partial.log"}`,
					},
				},
			}),
			schema.AssistantMessage("done", nil),
		},
	}

	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_stream_logs",
			Description: "Stream log content",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Stream: &tooling.StreamingResult{
					Reader:    strings.NewReader("chunk-one chunk-two"),
					ChunkSize: 9,
				},
			}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-stream-context",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-stream-context",
		Input:       "stream into next iteration",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model inputs = %d, want 2", len(model.inputs))
	}

	foundPartialContext := false
	for _, msg := range model.inputs[1] {
		if !strings.Contains(msg.Content, "Partial tool result") {
			continue
		}
		if strings.Contains(msg.Content, "chunk-one") {
			foundPartialContext = true
			break
		}
	}
	if !foundPartialContext {
		t.Fatal("expected second iteration model input to include partial tool result context")
	}

	session := kernel.sessions.Get("sess-stream-context")
	if session == nil {
		t.Fatal("expected session state")
	}
	foundProgressMessage := false
	for _, msg := range session.Messages {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "Partial tool result") && strings.Contains(msg.Content, "chunk-one") {
			foundProgressMessage = true
			break
		}
	}
	if !foundProgressMessage {
		t.Fatal("expected session messages to retain partial tool result context")
	}
}

func TestWorkspaceRouter_DelegatesWorkspaceRequestsToSharedIterationLoop(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-ws-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"/tmp/workspace.txt"}`,
					},
				},
			}),
			schema.AssistantMessage("workspace done", nil),
		},
	}

	var executed int
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_file",
			Description: "Read a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "read:" + string(input)}, nil
		},
	}

	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	router := NewWorkspaceRouter(nil)

	result, err := router.RouteRequest(context.Background(), TurnRequest{
		SessionID:   "sess-ws-shared",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeInspect,
		TurnID:      "turn-ws-shared",
		Input:       "inspect the workspace file",
	}, kernel)
	if err != nil {
		t.Fatalf("RouteRequest failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if executed != 1 {
		t.Fatalf("tool executions = %d, want 1", executed)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model Generate calls = %d, want 2", len(model.inputs))
	}

	session := kernel.sessions.Get("sess-ws-shared")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected workspace session turn snapshot")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("current turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	if len(session.CurrentTurn.Iterations) != 2 {
		t.Fatalf("turn iterations = %d, want 2", len(session.CurrentTurn.Iterations))
	}
	foundToolResult := false
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-ws-1" && strings.Contains(msg.Content, `read:{"path":"/tmp/workspace.txt"}`) {
			foundToolResult = true
			break
		}
	}
	if !foundToolResult {
		t.Fatal("expected second model input to include workspace tool result")
	}
}

func TestRunTurn_WorkspaceSessionIgnoresLegacyAgentManagerPath(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("workspace done", nil),
		},
	}

	kernel := newLoopKernel(t, model, nil, nil, policyengine.NewDefaultModePolicies())
	kernel.agentMgr = &panickingAgentManager{}

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-ws-no-legacy",
		SessionType: SessionTypeWorkspace,
		Mode:        ModePlan,
		TurnID:      "turn-ws-no-legacy",
		Input:       "plan the next steps",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model Generate calls = %d, want 1", len(model.inputs))
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func assertStructuredToolError(t *testing.T, content, toolCallID, toolName, failureKind, messagePart string) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		t.Fatalf("tool error content is not structured JSON: %v\n%s", err, content)
	}
	for _, field := range []string{
		"type",
		"toolCallId",
		"toolName",
		"failureKind",
		"retryable",
		"userActionRequired",
		"message",
		"allowedNextActions",
	} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("tool error content missing field %q: %s", field, content)
		}
	}
	var body struct {
		Type               string   `json:"type"`
		ToolCallID         string   `json:"toolCallId"`
		ToolName           string   `json:"toolName"`
		FailureKind        string   `json:"failureKind"`
		Retryable          bool     `json:"retryable"`
		UserActionRequired bool     `json:"userActionRequired"`
		Message            string   `json:"message"`
		AllowedNextActions []string `json:"allowedNextActions"`
	}
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		t.Fatalf("tool error content is not structured JSON: %v\n%s", err, content)
	}
	if body.Type != "tool_error" {
		t.Fatalf("tool error type = %q, want tool_error", body.Type)
	}
	if body.ToolCallID != toolCallID || body.ToolName != toolName || body.FailureKind != failureKind {
		t.Fatalf("tool error identity = call:%q tool:%q kind:%q, want call:%q tool:%q kind:%q", body.ToolCallID, body.ToolName, body.FailureKind, toolCallID, toolName, failureKind)
	}
	if !strings.Contains(body.Message, messagePart) {
		t.Fatalf("tool error message = %q, want to contain %q", body.Message, messagePart)
	}
	if body.Retryable {
		t.Fatalf("tool error retryable = true, want false in phase 1")
	}
	if body.UserActionRequired {
		t.Fatalf("tool error userActionRequired = true, want false for tool_not_found")
	}
	if !containsString(body.AllowedNextActions, "ask_user") {
		t.Fatalf("tool error allowedNextActions = %v, want ask_user", body.AllowedNextActions)
	}
}

func hasProtocolKind(state promptcompiler.ProtocolPromptState, kind string) bool {
	for _, item := range state.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func hasProtocolItem(state promptcompiler.ProtocolPromptState, kind, id, status, text string) bool {
	for _, item := range state.Items {
		if item.Kind == kind && item.ID == id && item.Status == status && item.Text == text {
			return true
		}
	}
	return false
}

func containsToolResultReferenceKind(refs []ToolResultReference, kind ToolResultReferenceKind) bool {
	for _, ref := range refs {
		if ref.Kind == kind {
			return true
		}
	}
	return false
}

func containsExternalCardRef(refs []ExternalReference, cardRef string) bool {
	for _, ref := range refs {
		if ref.CardRef == cardRef {
			return true
		}
	}
	return false
}

func containsExternalFilePath(refs []ExternalReference, filePath string) bool {
	for _, ref := range refs {
		if ref.FilePath == filePath {
			return true
		}
	}
	return false
}

func assertNoLegacyAssistantItems(t *testing.T, items []agentstate.TurnItem) {
	t.Helper()
	if invalid := firstInvalidTurnItem(items); invalid.ID != "" {
		t.Fatalf("invalid or legacy turn item found: %#v", invalid)
	}
}

func firstInvalidTurnItem(items []agentstate.TurnItem) agentstate.TurnItem {
	for _, item := range items {
		if !item.Type.IsValid() {
			return item
		}
	}
	return agentstate.TurnItem{}
}

func assistantMessagesByPhase(t *testing.T, items []agentstate.TurnItem, phase string) []agentstate.TurnItem {
	t.Helper()
	var out []agentstate.TurnItem
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypeAssistantMessage {
			continue
		}
		var payload struct {
			Phase string `json:"phase"`
		}
		if len(item.Payload.Data) > 0 {
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("unmarshal assistant_message payload: %v", err)
			}
		}
		if strings.TrimSpace(payload.Phase) == phase {
			out = append(out, item)
		}
	}
	return out
}

func assistantMessagePhaseForTest(item agentstate.TurnItem) string {
	var payload struct {
		Phase string `json:"phase"`
	}
	if len(item.Payload.Data) > 0 {
		_ = json.Unmarshal(item.Payload.Data, &payload)
	}
	return strings.TrimSpace(payload.Phase)
}
