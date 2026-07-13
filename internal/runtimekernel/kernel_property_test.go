package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"pgregory.net/rapid"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Mock implementations for testing
// ---------------------------------------------------------------------------

// testMockCompiler implements promptcompiler.Compiler for testing.
type testMockCompiler struct{}

func (m *testMockCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	stable := "system\n\ndev\n\ntools"
	compiled := promptcompiler.CompiledPrompt{
		Stable: promptcompiler.StablePromptEnvelope{
			Content:   stable,
			System:    promptcompiler.SystemPrompt{Content: "system"},
			Developer: promptcompiler.DeveloperInstructions{Content: "dev"},
			Tools:     promptcompiler.ToolPromptSet{Content: "tools"},
		},
		Dynamic: promptcompiler.DynamicPromptDelta{
			Content: "policy",
			Policy:  promptcompiler.RuntimePolicyPrompt{Content: "policy"},
		},
		System:    promptcompiler.SystemPrompt{Content: "system"},
		Developer: promptcompiler.DeveloperInstructions{Content: "dev"},
		Tools:     promptcompiler.ToolPromptSet{Content: "tools"},
		Policy:    promptcompiler.RuntimePolicyPrompt{Content: "policy"},
		Fingerprint: promptcompiler.PromptFingerprint{
			Version:           "prompt-fingerprint-v1",
			CompilerVersion:   "prompt-fingerprint-v1",
			StableHash:        "mock-stable-hash",
			SystemHash:        "mock-system-hash",
			DeveloperHash:     "mock-developer-hash",
			ToolRegistryHash:  "mock-tool-registry-hash",
			RuntimePolicyHash: "mock-runtime-policy-hash",
			ProtocolStateHash: "mock-protocol-state-hash",
		},
	}
	compiled.EnvelopeV2 = promptcompiler.BuildPromptEnvelopeV2(compiled, ctx)
	return compiled, nil
}

// testPanicCompiler panics during Compile (for panic recovery testing).
type testPanicCompiler struct{}

func (p *testPanicCompiler) Compile(_ promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	panic("compiler panic for testing")
}

// testMockChatModel implements model.ChatModel for testing.
type testMockChatModel struct{}

type testProviderConfigResolver struct {
	config modelrouter.ProviderConfig
}

func (r testProviderConfigResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	return r.config, true
}

func (m *testMockChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "mock response"}, nil
}

func (m *testMockChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *testMockChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

type promptTooLongSummaryModel struct{}

func (m *promptTooLongSummaryModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, errors.New("prompt too long: context length exceeded")
}

func (m *promptTooLongSummaryModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented")
}

func (m *promptTooLongSummaryModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

// testMockToolAssemblySource implements ToolAssemblySource for testing.
type testMockToolAssemblySource struct {
	registry *tooling.Registry
}

func (s *testMockToolAssemblySource) CompileContext(session SessionType, mode Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{
		SessionType:    string(session),
		Mode:           string(mode),
		AssembledTools: s.registry.AssembleTools(string(session), string(mode)),
	}
}

func (s *testMockToolAssemblySource) AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool {
	return s.registry.AssembleToolPool(string(session), string(mode))
}

func (s *testMockToolAssemblySource) CompileContextWithMetadata(session SessionType, mode Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.registry.CompileContextWithMetadata(string(session), string(mode), metadata)
}

func (s *testMockToolAssemblySource) AssembleToolPoolWithMetadata(session SessionType, mode Mode, metadata map[string]string) []tool.BaseTool {
	return s.registry.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

// testMockEventEmitter implements EventEmitter for testing.
type testMockEventEmitter struct {
	mu     sync.Mutex
	events []LifecycleEvent
}

func (e *testMockEventEmitter) Emit(event LifecycleEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

// testMockCompletionEvaluator implements policyengine.CompletionEvaluator.
type testMockCompletionEvaluator struct {
	action policyengine.PolicyAction
}

func (m *testMockCompletionEvaluator) CheckCompletion(_ context.Context, _ policyengine.TurnState) policyengine.PolicyDecision {
	return policyengine.PolicyDecision{Action: m.action}
}

// testMockTool implements tooling.Tool for testing.
type testMockTool struct {
	name        string
	description string
	readOnly    bool
	sessions    []string
	modes       []string
}

func (m *testMockTool) Metadata() tooling.ToolMetadata {
	return tooling.ToolMetadata{
		Name:        m.name,
		Description: m.description,
	}
}
func (m *testMockTool) InputSchema() json.RawMessage  { return json.RawMessage(`{}`) }
func (m *testMockTool) OutputSchema() json.RawMessage { return nil }
func (m *testMockTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return m.description
}
func (m *testMockTool) Prompt(tooling.PromptContext) string { return m.description }
func (m *testMockTool) IsEnabled(ctx tooling.ToolContext) bool {
	return matchToolingTestValue(m.sessions, ctx.SessionType) && matchToolingTestValue(m.modes, ctx.Mode)
}
func (m *testMockTool) IsReadOnly(json.RawMessage) bool        { return m.readOnly }
func (m *testMockTool) IsDestructive(json.RawMessage) bool     { return !m.readOnly }
func (m *testMockTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (m *testMockTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (m *testMockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (m *testMockTool) Execute(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func matchToolingTestValue(expected []string, actual string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		if candidate == actual {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genSessionType() *rapid.Generator[SessionType] {
	return rapid.SampledFrom(AllSessionTypes())
}

func genMode() *rapid.Generator[Mode] {
	return rapid.SampledFrom(AllModes())
}

func genNonEmptyString() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_]{0,19}`)
}

// ---------------------------------------------------------------------------
// Helper: create a fully wired RuntimeKernel for testing
// ---------------------------------------------------------------------------

func newTestKernel(compiler promptcompiler.Compiler) *RuntimeKernel {
	registry := tooling.NewRegistry()
	if compiler == nil {
		compiler = &testMockCompiler{}
	}
	policy := &policyengine.Engine{
		ModePolicy:       make(map[string]policyengine.ModePolicy),
		CompletionPolicy: &testMockCompletionEvaluator{action: policyengine.PolicyActionAllow},
	}
	emitter := &testMockEventEmitter{}
	providers := map[string]modelrouter.ChatModel{
		"mock": &testMockChatModel{},
	}
	router := modelrouter.NewRouter("mock", providers, nil)
	router.SetProviderConfigResolver(testProviderConfigResolver{config: modelrouter.ProviderConfig{Provider: "mock", Model: "mock", MaxContextTokens: 64000}})

	return NewRuntimeKernel(RuntimeKernelConfig{
		ToolSource:  &testMockToolAssemblySource{registry: registry},
		Compiler:    compiler,
		Policy:      policy,
		Projector:   emitter,
		ModelRouter: router,
	})
}

func newTestKernelWithHooks(compiler promptcompiler.Compiler, registry *hooks.Registry) *RuntimeKernel {
	kernel := newTestKernel(compiler)
	kernel.hooks = registry
	return kernel
}

func TestRunTurn_ExecutesTurnHooks(t *testing.T) {
	registry := hooks.NewRegistry()
	var calls []string

	if err := registry.RegisterTurn(hooks.TurnRegistration{
		Name:  "pre-turn",
		Stage: hooks.StagePreTurn,
		Hook: func(_ context.Context, event *hooks.TurnEvent) error {
			calls = append(calls, "pre:"+event.Input)
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTurn pre failed: %v", err)
	}
	if err := registry.RegisterTurn(hooks.TurnRegistration{
		Name:  "post-turn",
		Stage: hooks.StagePostTurn,
		Hook: func(_ context.Context, event *hooks.TurnEvent) error {
			calls = append(calls, "post:"+event.TurnID)
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTurn post failed: %v", err)
	}

	kernel := newTestKernelWithHooks(nil, registry)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-hooks-1",
		Input:       "hello hooks",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed status, got %q", result.Status)
	}

	want := []string{"pre:hello hooks", "post:turn-hooks-1"}
	if fmt.Sprintf("%v", calls) != fmt.Sprintf("%v", want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

// ---------------------------------------------------------------------------
// Property 1: Turn 管道执行顺序
// For any valid TurnRequest, RuntimeKernel should execute pipeline steps in
// fixed order: assemble_context → compile_prompt → assemble_tools →
// create_agent → runner_run → callback_events → projection → final_gate
//
// **Validates: Requirements 1.2**
// ---------------------------------------------------------------------------

func TestProperty1_TurnPipelineExecutionOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")
		input := genNonEmptyString().Draw(t, "input")

		kernel := newTestKernel(nil)

		recorder := &PipelineRecorder{}
		req := TurnRequest{
			SessionType: sessionType,
			Mode:        mode,
			Input:       input,
		}

		_, _ = kernel.RunTurnWithRecorder(context.Background(), req, recorder)

		// Verify pipeline step order
		expectedOrder := AllPipelineSteps()
		if len(recorder.Steps) != len(expectedOrder) {
			t.Fatalf("expected %d pipeline steps, got %d: %v",
				len(expectedOrder), len(recorder.Steps), recorder.Steps)
		}

		for i, step := range recorder.Steps {
			if step != expectedOrder[i] {
				t.Fatalf("step[%d]: expected %q, got %q (full: %v)",
					i, expectedOrder[i], step, recorder.Steps)
			}
		}
	})
}

func TestRunTurnFiltersOpsManualToolsWhenUserOptedOut(t *testing.T) {
	compiler := &recordingTestCompiler{delegate: &testMockCompiler{}}
	kernel := newTestKernel(compiler)
	registry := kernel.tools.(*testMockToolAssemblySource).registry
	for _, name := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight", "host_read"} {
		_ = registry.Register(&testMockTool{
			name:        name,
			description: name,
			readOnly:    true,
		})
	}

	_, err := kernel.RunTurnWithRecorder(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Input:       "已选择跳过运维手册，继续只读排查",
		Metadata: map[string]string{
			"opsManualAction":  "skip_ops_manual",
			"opsManualSkipped": "true",
		},
	}, &PipelineRecorder{})
	if err != nil {
		t.Fatalf("RunTurnWithRecorder error = %v", err)
	}
	if len(compiler.contexts) == 0 {
		t.Fatal("compiler did not record a compile context")
	}
	names := toolNames(compiler.contexts[0].AssembledTools)
	if fmt.Sprintf("%v", names) != "[host_read]" {
		t.Fatalf("assembled tools = %v, want [host_read]", names)
	}
}

type recordingTestCompiler struct {
	delegate promptcompiler.Compiler
	contexts []promptcompiler.CompileContext
}

func (c *recordingTestCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	cloned := ctx
	cloned.AssembledTools = append([]promptcompiler.Tool(nil), ctx.AssembledTools...)
	c.contexts = append(c.contexts, cloned)
	return c.delegate.Compile(ctx)
}

// ---------------------------------------------------------------------------
// Property 4: Panic 恢复保证
// For any panic during tool execution, RuntimeKernel should capture the panic
// and return a TurnResult with error status without crashing the process.
//
// **Validates: Requirements 1.7**
// ---------------------------------------------------------------------------

func TestProperty4_PanicRecoveryGuarantee(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")
		panicMsg := genNonEmptyString().Draw(t, "panicMsg")

		// Test RecoverTurn with a panicking function
		result, err := RecoverTurn("sess-test", "turn-test", sessionType, mode, func() (TurnResult, error) {
			panic(panicMsg)
		})

		// Should not return an error (panic is captured in result)
		if err != nil {
			t.Fatalf("RecoverTurn should not return error on panic, got: %v", err)
		}

		// Result should have error status
		if result.Status != "error" {
			t.Fatalf("expected status 'error', got %q", result.Status)
		}

		// Result should contain the panic message
		if result.Error == "" {
			t.Fatal("expected non-empty error message in result")
		}

		// Session info should be preserved
		if result.SessionID != "sess-test" {
			t.Fatalf("expected sessionID 'sess-test', got %q", result.SessionID)
		}
		if result.TurnID != "turn-test" {
			t.Fatalf("expected turnID 'turn-test', got %q", result.TurnID)
		}
		if result.SessionType != sessionType {
			t.Fatalf("expected sessionType %q, got %q", sessionType, result.SessionType)
		}
		if result.Mode != mode {
			t.Fatalf("expected mode %q, got %q", mode, result.Mode)
		}
	})
}

func TestProperty4_PanicRecoveryInRunTurn(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")

		kernel := newTestKernel(&testPanicCompiler{})

		req := TurnRequest{
			SessionType: sessionType,
			Mode:        mode,
			Input:       "test",
		}

		// RunTurn should not panic — it should recover and return error result
		result, err := kernel.RunTurn(context.Background(), req)

		// Should not propagate panic as error
		if err != nil {
			t.Fatalf("RunTurn should not return error on panic, got: %v", err)
		}

		// Result should indicate error
		if result.Status != "error" {
			t.Fatalf("expected status 'error', got %q", result.Status)
		}
		if result.Error == "" {
			t.Fatal("expected non-empty error in result")
		}
	})
}

// ---------------------------------------------------------------------------
// Property 2: 工具装配按 Session/Mode 过滤
// For any tool set and session type/mode combination:
// - All assembled tools must have visibility allowing current session/mode
// - Host session must not see workspace-only tools
// - Mode-restricted tools must only appear in the allowed mode
//
// **Validates: Requirements 1.4, 1.5**
// ---------------------------------------------------------------------------

func TestProperty2_ToolAssemblyVisibilityBySessionMode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")

		// Create registry with tools restricted by session type.
		registry := tooling.NewRegistry()

		// Register a workspace-only tool.
		_ = registry.Register(&testMockTool{
			name:        "workspace_dispatch",
			description: "workspace dispatch tool",
			readOnly:    true,
			sessions:    []string{"workspace"},
			modes:       []string{"chat", "inspect", "plan", "execute"},
		})

		// Register a host-visible tool.
		_ = registry.Register(&testMockTool{
			name:        "disk_usage",
			description: "disk usage tool",
			readOnly:    true,
			sessions:    []string{"host", "workspace"},
			modes:       []string{"chat", "inspect", "plan", "execute"},
		})

		// Register a mode-restricted tool (only execute mode).
		_ = registry.Register(&testMockTool{
			name:        "dangerous_exec",
			description: "execute-only tool",
			readOnly:    false,
			sessions:    []string{"host", "workspace"},
			modes:       []string{"execute"},
		})

		assembled := registry.AssembleTools(string(sessionType), string(mode))
		names := make(map[string]bool, len(assembled))
		for _, assembledTool := range assembled {
			names[assembledTool.Metadata().Name] = true
		}

		// Host session must NOT see workspace-only tools.
		if sessionType == SessionTypeHost {
			if names["workspace_dispatch"] {
				t.Fatalf("host session should not see workspace-only tool %q", "workspace_dispatch")
			}
		}

		// Workspace session MUST see workspace-only tools when visibility allows.
		if sessionType == SessionTypeWorkspace {
			if !names["workspace_dispatch"] {
				t.Fatal("workspace session should see workspace-only tool")
			}
		}

		if !names["disk_usage"] {
			t.Fatal("assembled tool set should include disk_usage for every session/mode")
		}

		if mode == ModeExecute {
			if !names["dangerous_exec"] {
				t.Fatal("execute mode should include dangerous_exec")
			}
		} else if names["dangerous_exec"] {
			t.Fatalf("non-execute mode %q should not include dangerous_exec", mode)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 3: 上下文窗口边界
// For any message sequence, UsedTokens should not exceed MaxTokens after
// trimming, and trimming preserves the most recent messages.
//
// **Validates: Requirements 1.6**
// ---------------------------------------------------------------------------

func TestProperty3_ContextWindowBoundary(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxTokens := rapid.IntRange(100, 10000).Draw(t, "maxTokens")
		numMessages := rapid.IntRange(1, 50).Draw(t, "numMessages")

		// Generate messages with varying content lengths
		messages := make([]Message, numMessages)
		for i := 0; i < numMessages; i++ {
			contentLen := rapid.IntRange(10, 500).Draw(t, "contentLen")
			content := make([]byte, contentLen)
			for j := range content {
				content[j] = 'a'
			}
			messages[i] = Message{
				ID:      "msg-" + rapid.StringMatching(`[0-9]{4}`).Draw(t, "msgID"),
				Role:    "user",
				Content: string(content),
			}
		}

		// Test TrimContext
		cw := &ContextWindow{MaxTokens: maxTokens}
		TrimContext(cw, messages)

		// Property: UsedTokens must not exceed MaxTokens
		// Exception: if only 1 message remains and it exceeds budget, we keep it
		if cw.UsedTokens > cw.MaxTokens && cw.Messages > 1 {
			t.Fatalf("UsedTokens (%d) exceeds MaxTokens (%d) with %d messages",
				cw.UsedTokens, cw.MaxTokens, cw.Messages)
		}

		// Test AssembleContext
		cw2 := &ContextWindow{MaxTokens: maxTokens}
		trimmed := AssembleContext(cw2, messages)

		// Property: UsedTokens must not exceed MaxTokens (same exception)
		if cw2.UsedTokens > cw2.MaxTokens && len(trimmed) > 1 {
			t.Fatalf("AssembleContext: UsedTokens (%d) exceeds MaxTokens (%d) with %d messages",
				cw2.UsedTokens, cw2.MaxTokens, len(trimmed))
		}

		// Property: trimmed messages should be a suffix of original (most recent preserved)
		if len(trimmed) > 0 && len(messages) > 0 {
			lastTrimmed := trimmed[len(trimmed)-1]
			lastOriginal := messages[len(messages)-1]
			if lastTrimmed.ID != lastOriginal.ID {
				t.Fatalf("trimming should preserve most recent messages: last trimmed=%q, last original=%q",
					lastTrimmed.ID, lastOriginal.ID)
			}
		}

		// Property: Messages count should match trimmed length
		if cw2.Messages != len(trimmed) {
			t.Fatalf("Messages count (%d) should match trimmed length (%d)",
				cw2.Messages, len(trimmed))
		}
	})
}

func TestSplitContextForCompaction(t *testing.T) {
	messages := []Message{
		{ID: "msg-1", Role: "user", Content: "aaaaa aaaaa aaaaa aaaaa"},
		{ID: "msg-2", Role: "assistant", Content: "bbbbb bbbbb bbbbb bbbbb"},
		{ID: "msg-3", Role: "tool", Content: "ccccc ccccc ccccc ccccc"},
		{ID: "msg-4", Role: "assistant", Content: "ddddd ddddd ddddd ddddd"},
	}

	cw := &ContextWindow{MaxTokens: 12}
	plan := SplitContextForCompaction(cw, messages)

	if !plan.Compacted {
		t.Fatal("expected compaction plan to be marked compacted")
	}
	if len(plan.Compactable) == 0 {
		t.Fatal("expected compactable prefix to be returned")
	}
	if len(plan.Retained) == 0 {
		t.Fatal("expected retained suffix to be returned")
	}
	if plan.Retained[len(plan.Retained)-1].ID != messages[len(messages)-1].ID {
		t.Fatalf("expected newest message to be preserved, got %q", plan.Retained[len(plan.Retained)-1].ID)
	}
	if cw.Messages != len(plan.Retained) {
		t.Fatalf("context messages = %d, want %d", cw.Messages, len(plan.Retained))
	}
	if cw.TruncatedAt != len(plan.Compactable) {
		t.Fatalf("TruncatedAt = %d, want %d", cw.TruncatedAt, len(plan.Compactable))
	}
	if cw.UsedTokens > cw.MaxTokens && cw.Messages > 1 {
		t.Fatalf("UsedTokens (%d) should not exceed MaxTokens (%d) when more than one message remains", cw.UsedTokens, cw.MaxTokens)
	}
}

func TestApplyContextPipelineDoesNotRetryPromptTooLong(t *testing.T) {
	compressor := spanstream.NewContextCompressor(&promptTooLongSummaryModel{}, 1)
	messages := []Message{
		testToolResultMessage("tr-1", "logs.search", strings.Repeat("old logs ", 20), "ref-old"),
		{ID: "assistant-1", Role: "assistant", Content: strings.Repeat("old analysis ", 20)},
		testToolResultMessage("tr-2", "metrics.query", strings.Repeat("old metrics ", 20), "ref-metrics"),
		{ID: "assistant-2", Role: "assistant", Content: strings.Repeat("more analysis ", 20)},
		testToolResultMessage("tr-3", "trace.query", strings.Repeat("old traces ", 20), "ref-traces"),
		{ID: "user-latest", Role: "user", Content: strings.Repeat("latest user constraint ", 20)},
	}
	cw := &ContextWindow{MaxTokens: 20}

	result, err := ApplyContextPipeline(context.Background(), cw, messages, ContextPipelineOptions{
		SessionID:    "sess-l5",
		TurnID:       "turn-l5",
		Iteration:    2,
		Compressor:   compressor,
		BudgetPolicy: DefaultContextBudgetPolicy(20000, 8000),
	})
	if err != nil {
		t.Fatalf("ApplyContextPipeline returned error: %v", err)
	}
	if len(result.CompactedSegments) == 0 || strings.Contains(result.CompactedSegments[0].Summary, "compressed after retry") {
		t.Fatalf("summary message = %#v, want local fallback summary", result.Messages)
	}
	var tooLongEvent *ContextGovernanceEvent
	for i := range result.GovernanceEvents {
		if result.GovernanceEvents[i].Layer == ContextGovernanceLayerL5 && result.GovernanceEvents[i].Kind == "context.compaction.failed" {
			tooLongEvent = &result.GovernanceEvents[i]
			break
		}
	}
	if tooLongEvent == nil {
		t.Fatalf("governance events = %#v, want single L5 failed event", result.GovernanceEvents)
	}
	if tooLongEvent.RetryAttempt != 0 || tooLongEvent.RetryMax != 0 {
		t.Fatalf("too long event = %#v, want no retry counters", tooLongEvent)
	}
	if len(tooLongEvent.DroppedGroupIDs) != 0 {
		t.Fatalf("too long event dropped groups = %#v, want none", tooLongEvent)
	}
}

func TestApplyContextPipelineEmitsBoundaryMarkerAndHardKeepReasons(t *testing.T) {
	messages := []Message{
		{ID: "old-user", Role: "user", Content: strings.Repeat("old user context ", 20)},
		{ID: "old-assistant", Role: "assistant", Content: strings.Repeat("old assistant context ", 20)},
		{ID: "old-user-2", Role: "user", Content: strings.Repeat("older user context ", 20)},
		{ID: "old-assistant-2", Role: "assistant", Content: strings.Repeat("older assistant context ", 20)},
		{ID: "old-tool-2", Role: "tool", ToolResult: &ToolResult{ToolCallID: "tr-older", Content: strings.Repeat("older tool result ", 20)}},
		{ID: "old-assistant-3", Role: "assistant", Content: strings.Repeat("more old assistant context ", 20)},
		testToolResultMessage("tr-old", "read.context", strings.Repeat("old tool result ", 20), "ref-old"),
		{ID: "latest-user", Role: "user", Content: "Continue from this latest quoted request."},
		{ID: "latest-assistant", Role: "assistant", Content: "I will continue from the latest request."},
	}
	cw := &ContextWindow{MaxTokens: 24}

	result, err := ApplyContextPipeline(context.Background(), cw, messages, ContextPipelineOptions{
		SessionID: "sess-boundary",
		TurnID:    "turn-boundary",
		Iteration: 1,
		PendingApprovals: []PendingApproval{{
			ID:         "approval-1",
			SessionID:  "sess-boundary",
			TurnID:     "turn-boundary",
			Iteration:  1,
			ToolName:   "write.action",
			ToolCallID: "call-approval",
		}},
		PendingEvidence: []PendingEvidence{{
			ID:         "evidence-1",
			SessionID:  "sess-boundary",
			TurnID:     "turn-boundary",
			Iteration:  1,
			ToolCallID: "tr-old",
		}},
		BudgetPolicy: DefaultContextBudgetPolicy(20000, 8000),
	})
	if err != nil {
		t.Fatalf("ApplyContextPipeline returned error: %v", err)
	}
	if len(result.Messages) == 0 || !IsCompactBoundaryMessage(result.Messages[0]) {
		t.Fatalf("first message = %#v, want compact boundary marker", result.Messages)
	}
	meta, ok := CompactBoundaryMetadataFromMessage(result.Messages[0])
	if !ok {
		t.Fatal("boundary metadata missing")
	}
	if meta.SegmentID == "" || meta.SummarySchemaVersion != CompactSummarySchemaVersionV1 || meta.PreservedTailCount == 0 {
		t.Fatalf("boundary metadata = %#v", meta)
	}

	var keepEvent *ContextGovernanceEvent
	for i := range result.GovernanceEvents {
		if result.GovernanceEvents[i].Kind == "context.compaction.hard_keep" {
			keepEvent = &result.GovernanceEvents[i]
			break
		}
	}
	if keepEvent == nil {
		t.Fatalf("governance events = %#v, want hard keep event", result.GovernanceEvents)
	}
	for _, reason := range []string{
		"recent_user_message",
		"pending_approval",
		"pending_evidence",
		"active_task",
		"compact_safety_minimum",
	} {
		if !containsString(keepEvent.DroppedGroupIDs, reason) {
			t.Fatalf("hard keep reasons = %#v, missing %q", keepEvent.DroppedGroupIDs, reason)
		}
	}
}

func TestApplyContextPipelineCompactionEvidenceHandoffSummary(t *testing.T) {
	fullStdout := "STDOUT:" + strings.Repeat("raw-output-line ", 80)
	fullManual := "MANUAL_CONTENT:" + strings.Repeat("manual-step ", 80)
	fullArtifactPayload := "ARTIFACT_PAYLOAD:" + strings.Repeat("payload-field ", 80)
	messages := []Message{
		{ID: "goal", Role: "user", Content: "Investigate checkout latency and keep the current target."},
		{ID: "decision", Role: "assistant", Content: "Decision: use read-only evidence before any mutation."},
		testToolResultMessage("tool-stdout", "exec_command", fullStdout, "evidence-stdout"),
		{ID: "inference", Role: "assistant", Content: "Inference (confidence: medium): upstream database latency may be involved."},
		testToolResultMessage("tool-manual", "search_ops_manuals", fullManual, "manual-ref"),
		testToolResultMessage("tool-artifact", "artifact.read", fullArtifactPayload, "artifact-ref"),
		{ID: "tail-user", Role: "user", Content: "Continue with only verified evidence."},
		{ID: "tail-assistant", Role: "assistant", Content: "I will continue with verified evidence only."},
	}
	cw := &ContextWindow{MaxTokens: 28}
	result, err := ApplyContextPipeline(context.Background(), cw, messages, ContextPipelineOptions{
		SessionID: "sess-handoff",
		TurnID:    "turn-handoff",
		Iteration: 2,
		Profile:   "evidence_rca",
		TargetRefs: []string{
			"service:checkout",
			"host:db-a",
		},
		PendingApprovals: []PendingApproval{{
			ID:         "approval-restart",
			ToolName:   "exec_command",
			ToolCallID: "call-restart",
			TargetRefs: []string{"host:db-a"},
		}},
		PendingEvidence: []PendingEvidence{{
			ID:         "evidence-db",
			ToolCallID: "tool-stdout",
		}},
		RejectedApprovals: []RejectedApproval{{
			ID:       "rejected-restart",
			ToolName: "exec_command",
			Decision: "denied",
		}},
		ToolPacksLoaded: []string{"coroot_metrics", "ops_manual_flow"},
		BudgetPolicy:    DefaultContextBudgetPolicy(20000, 8000),
	})
	if err != nil {
		t.Fatalf("ApplyContextPipeline returned error: %v", err)
	}
	if len(result.CompactedSegments) != 1 {
		t.Fatalf("compacted segments = %d, want 1", len(result.CompactedSegments))
	}
	summary, err := ParseCompactSummaryV1(result.CompactedSegments[0].Summary)
	if err != nil {
		t.Fatalf("ParseCompactSummaryV1() error = %v\nsummary=%s", err, result.CompactedSegments[0].Summary)
	}
	if summary.UserGoal == "" ||
		summary.CurrentProfile != "evidence_rca" ||
		!containsString(summary.TargetRefs, "service:checkout") ||
		len(summary.Decisions) == 0 ||
		len(summary.ConfirmedFacts) == 0 ||
		len(summary.Inferences) == 0 ||
		len(summary.PendingApprovals) != 1 ||
		len(summary.PendingEvidence) != 1 ||
		len(summary.RejectedApprovals) != 1 ||
		!containsString(summary.ToolPacksLoaded, "coroot_metrics") ||
		summary.NextStep.Action == "" {
		t.Fatalf("handoff summary incomplete: %#v", summary)
	}
	raw := result.CompactedSegments[0].Summary
	for _, forbidden := range []string{fullStdout, fullManual, fullArtifactPayload} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("handoff summary leaked full raw payload %q in %s", forbidden[:16], raw)
		}
	}
}

// ---------------------------------------------------------------------------
// Property 27: Workspace 请求分流
// For any workspace session request, it should be classified into exactly one
// category: state_query (read projection), single_host_readonly (current turn),
// or complex_task (PlanExecuteAgent).
//
// **Validates: Requirements 9.1**
// ---------------------------------------------------------------------------

func genRequestCategory() *rapid.Generator[RequestCategory] {
	return rapid.SampledFrom(AllRequestCategories())
}

func genWorkspaceTurnRequest() *rapid.Generator[TurnRequest] {
	return rapid.Custom[TurnRequest](func(t *rapid.T) TurnRequest {
		mode := genMode().Draw(t, "mode")
		input := rapid.SampledFrom([]string{
			// State query patterns
			"当前有哪些主机在线",
			"show status",
			"list running tasks",
			"有多少台服务器",
			// Single-host readonly patterns (with hostID)
			"check disk usage",
			"inspect logs",
			"read /var/log/syslog",
			// Complex task patterns
			"deploy new version to all servers",
			"execute cleanup on host-a and host-b",
			"fix the memory leak on production",
			// Ambiguous patterns
			"hello world",
			"what can you do",
		}).Draw(t, "input")

		hostID := rapid.SampledFrom([]string{"", "host-1", "host-2"}).Draw(t, "hostID")

		return TurnRequest{
			SessionType: SessionTypeWorkspace,
			Mode:        mode,
			Input:       input,
			HostID:      hostID,
		}
	})
}

func TestProperty27_WorkspaceRequestRouting(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		req := genWorkspaceTurnRequest().Draw(t, "request")

		router := NewWorkspaceRouter(nil)
		decision := router.ClassifyRequest(req)

		// Property: classification must be exactly one valid category
		if !decision.Category.IsValid() {
			t.Fatalf("invalid category %q for request %+v", decision.Category, req)
		}

		// Property: category must be one of the three valid values
		validCategories := AllRequestCategories()
		found := false
		for _, c := range validCategories {
			if decision.Category == c {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("category %q not in valid set %v", decision.Category, validCategories)
		}

		// Property: reason must be non-empty
		if decision.Reason == "" {
			t.Fatalf("routing decision must have a non-empty reason")
		}

		// Property: state_query should not have target hosts
		if decision.Category == CategoryStateQuery && len(decision.TargetHosts) > 0 {
			t.Fatalf("state_query should not have target hosts, got %v", decision.TargetHosts)
		}

		// Property: single_host_readonly must have exactly one target host
		if decision.Category == CategorySingleHostReadonly {
			if len(decision.TargetHosts) != 1 {
				t.Fatalf("single_host_readonly should have exactly 1 target host, got %d",
					len(decision.TargetHosts))
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 28: 任务状态机有效性
// For any WorkspaceTask state transition sequence, only valid transitions
// should be allowed (pending→running, running→completed/failed/killed).
// Terminal states have no outgoing transitions.
// Host Agent offline → tasks marked failed.
// Mission stop → all non-terminal tasks converge to terminal.
//
// **Validates: Requirements 9.3, 9.4, 9.5**
// ---------------------------------------------------------------------------

func genTaskStatus() *rapid.Generator[TaskStatus] {
	return rapid.SampledFrom(AllTaskStatuses())
}

func TestProperty28_TaskStateMachineValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genTaskStatus().Draw(t, "fromStatus")
		to := genTaskStatus().Draw(t, "toStatus")

		validTransitions := ValidTransitions(from)
		isValid := IsValidTransition(from, to)

		// Property: terminal states have no valid outgoing transitions
		if from.IsTerminal() {
			if len(validTransitions) != 0 {
				t.Fatalf("terminal state %q should have no valid transitions, got %v",
					from, validTransitions)
			}
			if isValid {
				t.Fatalf("transition from terminal state %q to %q should be invalid", from, to)
			}
		}

		// Property: if transition is valid, 'to' must be in ValidTransitions(from)
		if isValid {
			found := false
			for _, v := range validTransitions {
				if v == to {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("IsValidTransition(%q, %q) returned true but %q not in ValidTransitions(%q)=%v",
					from, to, to, from, validTransitions)
			}
		}

		// Property: if transition is invalid, 'to' must NOT be in ValidTransitions(from)
		if !isValid {
			for _, v := range validTransitions {
				if v == to {
					t.Fatalf("IsValidTransition(%q, %q) returned false but %q is in ValidTransitions(%q)=%v",
						from, to, to, from, validTransitions)
				}
			}
		}
	})
}

func TestProperty28_TaskStateMachine_HostOffline(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numTasks := rapid.IntRange(1, 10).Draw(t, "numTasks")
		hostID := genNonEmptyString().Draw(t, "hostID")

		tm := NewTaskManager()
		tm.SetHostOnline(hostID)

		// Create tasks in various states assigned to the host
		var nonTerminalIDs []string
		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			status := genTaskStatus().Draw(t, "taskStatus")

			task := &WorkspaceTask{
				ID:      taskID,
				Type:    "host_exec",
				HostIDs: []string{hostID},
			}
			_ = tm.AddTask(task)

			// Transition to desired state if possible
			if status == TaskStatusRunning {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
			} else if status == TaskStatusCompleted {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusCompleted, "")
			} else if status == TaskStatusFailed {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusFailed, "host error")
			} else if status == TaskStatusKilled {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusKilled, "killed")
			}
			// pending stays as-is

			currentTask := tm.GetTask(taskID)
			if currentTask != nil && !TaskStatus(currentTask.Status).IsTerminal() {
				nonTerminalIDs = append(nonTerminalIDs, taskID)
			}
		}

		// Mark host offline
		failedIDs := tm.SetHostOffline(hostID)

		// Property: all returned failed IDs should now be in failed state
		for _, id := range failedIDs {
			task := tm.GetTask(id)
			if task == nil {
				t.Fatalf("failed task %q not found", id)
			}
			if TaskStatus(task.Status) != TaskStatusFailed {
				t.Fatalf("task %q should be failed after host offline, got %q", id, task.Status)
			}
		}

		// Property: no non-terminal task assigned to the host should remain non-terminal
		for _, id := range nonTerminalIDs {
			task := tm.GetTask(id)
			if task != nil && !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("task %q should be terminal after host offline, got %q", id, task.Status)
			}
		}
	})
}

func TestProperty28_TaskStateMachine_MissionStop(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numTasks := rapid.IntRange(1, 10).Draw(t, "numTasks")

		tm := NewTaskManager()

		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			status := genTaskStatus().Draw(t, "taskStatus")

			task := &WorkspaceTask{
				ID:   taskID,
				Type: "multi_host",
			}
			_ = tm.AddTask(task)

			// Transition to desired state
			if status == TaskStatusRunning {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
			} else if status == TaskStatusCompleted {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusCompleted, "")
			} else if status == TaskStatusFailed {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusFailed, "error")
			} else if status == TaskStatusKilled {
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusKilled, "killed")
			}
		}

		// Stop mission
		tm.StopMission("test stop")

		// Property: after mission stop, ALL tasks must be in terminal state
		for _, task := range tm.ListTasks() {
			if !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("task %q should be terminal after mission stop, got %q",
					task.ID, task.Status)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 29: 预算/队列并发控制
// For any task queue and budget configuration, the number of simultaneously
// running tasks should never exceed the budget limit. After task completion,
// budget should be released and queue backfill should work correctly.
//
// **Validates: Requirements 9.6**
// ---------------------------------------------------------------------------

func TestProperty29_BudgetQueueConcurrencyControl(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxBudget := rapid.IntRange(1, 10).Draw(t, "maxBudget")
		numTasks := rapid.IntRange(1, 30).Draw(t, "numTasks")

		bc, err := NewBudgetController(maxBudget)
		if err != nil {
			t.Fatalf("failed to create BudgetController: %v", err)
		}

		// Try to acquire budget for all tasks
		var acquiredIDs []string
		var queuedIDs []string

		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			acquired, err := bc.TryAcquire(taskID)
			if err != nil {
				t.Fatalf("TryAcquire(%q) error: %v", taskID, err)
			}
			if acquired {
				acquiredIDs = append(acquiredIDs, taskID)
			} else {
				queuedIDs = append(queuedIDs, taskID)
			}

			// Property: running count must NEVER exceed maxBudget
			if bc.RunningCount() > maxBudget {
				t.Fatalf("running count %d exceeds maxBudget %d after acquiring %q",
					bc.RunningCount(), maxBudget, taskID)
			}
		}

		// Property: number of acquired tasks should equal min(numTasks, maxBudget)
		expectedAcquired := numTasks
		if expectedAcquired > maxBudget {
			expectedAcquired = maxBudget
		}
		if len(acquiredIDs) != expectedAcquired {
			t.Fatalf("expected %d acquired tasks, got %d", expectedAcquired, len(acquiredIDs))
		}

		// Property: number of queued tasks should equal max(0, numTasks - maxBudget)
		expectedQueued := numTasks - maxBudget
		if expectedQueued < 0 {
			expectedQueued = 0
		}
		if len(queuedIDs) != expectedQueued {
			t.Fatalf("expected %d queued tasks, got %d", expectedQueued, len(queuedIDs))
		}

		// Release tasks and verify queue backfill
		promotedCount := 0
		for _, id := range acquiredIDs {
			promoted, err := bc.Release(id)
			if err != nil {
				t.Fatalf("Release(%q) error: %v", id, err)
			}
			if promoted != "" {
				promotedCount++
				// Property: promoted task should now be running
				if !bc.IsRunning(promoted) {
					t.Fatalf("promoted task %q should be running", promoted)
				}
			}

			// Property: running count must NEVER exceed maxBudget after release
			if bc.RunningCount() > maxBudget {
				t.Fatalf("running count %d exceeds maxBudget %d after releasing %q",
					bc.RunningCount(), maxBudget, id)
			}
		}

		// Property: number of promoted tasks should equal min(queued, acquired)
		expectedPromoted := len(queuedIDs)
		if expectedPromoted > len(acquiredIDs) {
			expectedPromoted = len(acquiredIDs)
		}
		if promotedCount != expectedPromoted {
			t.Fatalf("expected %d promoted tasks, got %d", expectedPromoted, promotedCount)
		}
	})
}

func TestProperty29_BudgetNeverExceedsMax(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxBudget := rapid.IntRange(1, 5).Draw(t, "maxBudget")
		numOps := rapid.IntRange(5, 50).Draw(t, "numOps")

		bc, err := NewBudgetController(maxBudget)
		if err != nil {
			t.Fatalf("failed to create BudgetController: %v", err)
		}

		var runningIDs []string
		nextID := 0

		for i := 0; i < numOps; i++ {
			// Randomly acquire or release
			action := rapid.SampledFrom([]string{"acquire", "release"}).Draw(t, "action")

			switch action {
			case "acquire":
				taskID := fmt.Sprintf("t-%d", nextID)
				nextID++
				acquired, err := bc.TryAcquire(taskID)
				if err != nil {
					continue // skip duplicates
				}
				if acquired {
					runningIDs = append(runningIDs, taskID)
				}
			case "release":
				if len(runningIDs) > 0 {
					idx := rapid.IntRange(0, len(runningIDs)-1).Draw(t, "releaseIdx")
					id := runningIDs[idx]
					runningIDs = append(runningIDs[:idx], runningIDs[idx+1:]...)
					promoted, _ := bc.Release(id)
					if promoted != "" {
						runningIDs = append(runningIDs, promoted)
					}
				}
			}

			// INVARIANT: running count must NEVER exceed maxBudget
			if bc.RunningCount() > maxBudget {
				t.Fatalf("INVARIANT VIOLATED: running count %d > maxBudget %d at op %d",
					bc.RunningCount(), maxBudget, i)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 30: Reconcile 安全性
// After restart, reconcile should mark all non-terminal tasks as failed.
// It must NEVER restore already-failed tasks to running state.
//
// **Validates: Requirements 9.7**
// ---------------------------------------------------------------------------

func TestProperty30_ReconcileSafety(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numTasks := rapid.IntRange(1, 20).Draw(t, "numTasks")
		maxBudget := rapid.IntRange(1, 5).Draw(t, "maxBudget")

		tm := NewTaskManager()
		bc, err := NewBudgetController(maxBudget)
		if err != nil {
			t.Fatalf("failed to create BudgetController: %v", err)
		}

		// Create tasks in various states (simulating pre-restart state)
		var preReconcileStates = make(map[string]TaskStatus)
		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			targetStatus := genTaskStatus().Draw(t, "targetStatus")

			task := &WorkspaceTask{
				ID:   taskID,
				Type: "host_exec",
			}
			_ = tm.AddTask(task)

			// Transition to target state
			switch targetStatus {
			case TaskStatusRunning:
				_ = tm.Transition(taskID, TaskStatusRunning, "")
			case TaskStatusCompleted:
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusCompleted, "")
			case TaskStatusFailed:
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusFailed, "pre-restart failure")
			case TaskStatusKilled:
				_ = tm.Transition(taskID, TaskStatusRunning, "")
				_ = tm.Transition(taskID, TaskStatusKilled, "killed")
			}

			// Record actual state after transitions
			actualTask := tm.GetTask(taskID)
			preReconcileStates[taskID] = TaskStatus(actualTask.Status)
		}

		// Also put some tasks in the budget controller
		for i := 0; i < numTasks && i < maxBudget; i++ {
			_, _ = bc.TryAcquire(fmt.Sprintf("task-%d", i))
		}

		// Execute reconcile
		summary, err := Reconcile(tm, bc)
		if err != nil {
			t.Fatalf("Reconcile error: %v", err)
		}

		// Property: reconcile should examine all tasks
		if summary.TotalTasks != numTasks {
			t.Fatalf("expected TotalTasks=%d, got %d", numTasks, summary.TotalTasks)
		}

		// Property: after reconcile, ALL tasks must be in terminal state
		for _, task := range tm.ListTasks() {
			if !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("task %q should be terminal after reconcile, got %q",
					task.ID, task.Status)
			}
		}

		// Property: already-terminal tasks should NOT be modified
		for taskID, preStatus := range preReconcileStates {
			if preStatus.IsTerminal() {
				task := tm.GetTask(taskID)
				if TaskStatus(task.Status) != preStatus {
					t.Fatalf("terminal task %q was modified: was %q, now %q",
						taskID, preStatus, task.Status)
				}
			}
		}

		// Property: already-failed tasks must NEVER be restored to running
		for taskID, preStatus := range preReconcileStates {
			if preStatus == TaskStatusFailed {
				task := tm.GetTask(taskID)
				if TaskStatus(task.Status) == TaskStatusRunning {
					t.Fatalf("CRITICAL: failed task %q was restored to running after reconcile", taskID)
				}
			}
		}

		// Property: budget controller should be reset (no running tasks)
		if bc.RunningCount() != 0 {
			t.Fatalf("budget controller should have 0 running after reconcile, got %d",
				bc.RunningCount())
		}
		if bc.QueueLen() != 0 {
			t.Fatalf("budget controller should have 0 queued after reconcile, got %d",
				bc.QueueLen())
		}
	})
}

// ---------------------------------------------------------------------------
// Property 33: Workspace 五项运维语义保持
// For any workspace runtime scenario, the five operational semantics must
// be correctly preserved:
//   - stop: mission stop converges all non-terminal tasks
//   - offline: host offline marks tasks as failed
//   - reconcile: restart recovery marks non-terminal as failed, never restores failed→running
//   - budget: concurrent running never exceeds budget
//   - queue: completed tasks release budget and queue backfills
//
// **Validates: Requirements 12.5**
// ---------------------------------------------------------------------------

func TestProperty33_WorkspaceFiveOperationalSemantics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxBudget := rapid.IntRange(1, 5).Draw(t, "maxBudget")
		numTasks := rapid.IntRange(3, 15).Draw(t, "numTasks")
		numHosts := rapid.IntRange(1, 4).Draw(t, "numHosts")

		tm := NewTaskManager()
		bc, err := NewBudgetController(maxBudget)
		if err != nil {
			t.Fatalf("failed to create BudgetController: %v", err)
		}

		// Setup hosts
		hosts := make([]string, numHosts)
		for i := 0; i < numHosts; i++ {
			hosts[i] = fmt.Sprintf("host-%d", i)
			tm.SetHostOnline(hosts[i])
		}

		// Create tasks assigned to random hosts
		taskIDs := make([]string, numTasks)
		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			taskIDs[i] = taskID
			hostIdx := rapid.IntRange(0, numHosts-1).Draw(t, "hostIdx")
			task := &WorkspaceTask{
				ID:      taskID,
				Type:    "host_exec",
				HostIDs: []string{hosts[hostIdx]},
			}
			_ = tm.AddTask(task)
		}

		// Semantic 1: Budget — acquire budget for tasks
		var runningTasks []string
		for _, id := range taskIDs {
			acquired, err := bc.TryAcquire(id)
			if err != nil {
				continue
			}
			if acquired {
				_ = tm.Transition(id, TaskStatusRunning, "")
				runningTasks = append(runningTasks, id)
			}
			// INVARIANT: budget never exceeded
			if bc.RunningCount() > maxBudget {
				t.Fatalf("budget semantic violated: running %d > max %d",
					bc.RunningCount(), maxBudget)
			}
		}

		// Semantic 2: Queue backfill — release one running task
		if len(runningTasks) > 0 {
			releasedID := runningTasks[0]
			_ = tm.Transition(releasedID, TaskStatusCompleted, "")
			promoted, _ := bc.Release(releasedID)

			// If there was a queued task, it should be promoted
			if promoted != "" {
				_ = tm.Transition(promoted, TaskStatusRunning, "")
				// INVARIANT: budget still not exceeded
				if bc.RunningCount() > maxBudget {
					t.Fatalf("queue backfill violated budget: running %d > max %d",
						bc.RunningCount(), maxBudget)
				}
			}
		}

		// Semantic 3: Offline — take a random host offline
		offlineHostIdx := rapid.IntRange(0, numHosts-1).Draw(t, "offlineHostIdx")
		offlineHost := hosts[offlineHostIdx]
		failedByOffline := tm.SetHostOffline(offlineHost)

		// All tasks on offline host should be terminal
		for _, id := range failedByOffline {
			task := tm.GetTask(id)
			if !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("offline semantic violated: task %q on offline host %q is %q",
					id, offlineHost, task.Status)
			}
		}

		// Semantic 4: Stop — stop mission converges all remaining
		killedIDs := tm.StopMission("semantic test stop")
		_ = killedIDs

		// After stop, ALL tasks must be terminal
		for _, task := range tm.ListTasks() {
			if !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("stop semantic violated: task %q is %q after mission stop",
					task.ID, task.Status)
			}
		}

		// Semantic 5: Reconcile — reset and reconcile
		// Create fresh tasks to simulate post-restart state
		tm2 := NewTaskManager()
		bc2, _ := NewBudgetController(maxBudget)
		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("recon-task-%d", i)
			task := &WorkspaceTask{ID: taskID, Type: "host_exec"}
			_ = tm2.AddTask(task)
			// Some running, some pending
			if i%2 == 0 {
				_ = tm2.Transition(taskID, TaskStatusRunning, "")
			}
		}

		summary, err := Reconcile(tm2, bc2)
		if err != nil {
			t.Fatalf("reconcile error: %v", err)
		}

		// After reconcile, all tasks must be terminal
		for _, task := range tm2.ListTasks() {
			if !TaskStatus(task.Status).IsTerminal() {
				t.Fatalf("reconcile semantic violated: task %q is %q after reconcile",
					task.ID, task.Status)
			}
			// CRITICAL: no task should be in running state
			if TaskStatus(task.Status) == TaskStatusRunning {
				t.Fatalf("reconcile semantic violated: task %q is running after reconcile", task.ID)
			}
		}

		// Reconcile should have processed all tasks
		if summary.TotalTasks != numTasks {
			t.Fatalf("reconcile should examine all %d tasks, got %d",
				numTasks, summary.TotalTasks)
		}
	})
}
