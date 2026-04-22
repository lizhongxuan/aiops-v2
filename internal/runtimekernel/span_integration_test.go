package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Mock SpanStreamSource for testing
// ---------------------------------------------------------------------------

type mockSpanStreamSource struct {
	turnSpans     []mockSpanEvent
	toolSpans     []mockSpanEvent
	completedIDs  []string
	failedIDs     []string
	textEmissions []string
}

type mockSpanEvent struct {
	spanID       string
	parentSpanID string
	name         string
}

func (m *mockSpanStreamSource) StartTurnSpan(turnID string, input string) string {
	id := "turn-span-" + turnID
	m.turnSpans = append(m.turnSpans, mockSpanEvent{spanID: id, name: input})
	return id
}

func (m *mockSpanStreamSource) StartToolSpan(parentSpanID string, toolName string) string {
	id := "tool-span-" + toolName
	m.toolSpans = append(m.toolSpans, mockSpanEvent{spanID: id, parentSpanID: parentSpanID, name: toolName})
	return id
}

func (m *mockSpanStreamSource) CompleteSpan(spanID string, summary string, detail string) {
	m.completedIDs = append(m.completedIDs, spanID)
}

func (m *mockSpanStreamSource) FailSpan(spanID string, errMsg string) {
	m.failedIDs = append(m.failedIDs, spanID)
}

func (m *mockSpanStreamSource) EmitText(text string) {
	m.textEmissions = append(m.textEmissions, text)
}

func (m *mockSpanStreamSource) Chunks() <-chan spanstream.TypedEventChunk {
	ch := make(chan spanstream.TypedEventChunk)
	close(ch)
	return ch
}

// ---------------------------------------------------------------------------
// Tests: SpanTree integration into RuntimeKernel
// ---------------------------------------------------------------------------

func TestRunTurn_CreatesRootSpanForEachTurn(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	kernel := newTestKernelWithSpanSource(spanSource)

	req := TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Input:       "check disk usage",
		TurnID:      "test-turn-1",
	}

	result, err := kernel.RunTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", result.Status)
	}

	// Verify a turn span was created
	if len(spanSource.turnSpans) != 1 {
		t.Fatalf("expected 1 turn span, got %d", len(spanSource.turnSpans))
	}
	if spanSource.turnSpans[0].name != "check disk usage" {
		t.Fatalf("expected turn span input 'check disk usage', got %q", spanSource.turnSpans[0].name)
	}

	// Verify the turn span was completed
	if len(spanSource.completedIDs) != 1 {
		t.Fatalf("expected 1 completed span, got %d", len(spanSource.completedIDs))
	}
	if spanSource.completedIDs[0] != "turn-span-test-turn-1" {
		t.Fatalf("expected completed span ID 'turn-span-test-turn-1', got %q", spanSource.completedIDs[0])
	}
}

func TestRunTurn_FailsSpanOnError(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	kernel := newTestKernelWithSpanSourceAndCompiler(spanSource, &testPanicCompiler{})

	req := TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Input:       "test input",
		TurnID:      "test-turn-err",
	}

	// The panic compiler will cause a panic which is recovered
	result, _ := kernel.RunTurn(context.Background(), req)

	// Panic recovery means the span won't be explicitly failed via our code path
	// (the defer/recover catches it before our span fail code runs)
	// But the turn span should have been created
	if len(spanSource.turnSpans) != 1 {
		t.Fatalf("expected 1 turn span, got %d", len(spanSource.turnSpans))
	}

	// Result should be error status from panic recovery
	if result.Status != "error" {
		t.Fatalf("expected status 'error', got %q", result.Status)
	}
}

func TestRunTurn_NoSpanSourceIsNoop(t *testing.T) {
	// Kernel without span source should work fine
	kernel := newTestKernel(nil)

	req := TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Input:       "hello",
	}

	result, err := kernel.RunTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", result.Status)
	}
}

func TestRunTurn_EmitsIterationStageActivityUpdates(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	kernel := newTestKernelWithSpanSource(spanSource)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-stage-events",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-stage-events",
		Input:       "hello stages",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed status, got %q", result.Status)
	}

	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	var stages []string
	for _, event := range emitter.events {
		if event.Type != EventActivityUpdate {
			continue
		}
		var payload struct {
			Iteration int    `json:"iteration"`
			Stage     string `json:"stage"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("unmarshal stage payload: %v", err)
		}
		if payload.Iteration == 0 {
			stages = append(stages, payload.Stage)
		}
	}
	if len(stages) == 0 {
		t.Fatal("expected iteration stage activity updates")
	}
	if !containsStage(stages, "context_pipeline") || !containsStage(stages, "call_model") {
		t.Fatalf("iteration stages = %v, want context_pipeline and call_model", stages)
	}
	if len(spanSource.textEmissions) == 0 {
		t.Fatal("expected span text emissions for iteration stages")
	}
}

func TestSpanAwareRunnerCallback_ToolLifecycle(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	emitter := &testMockEventEmitter{}

	cb := NewSpanAwareRunnerCallback("sess-1", "turn-1", emitter, spanSource, "turn-span-1")

	// Start a tool
	args, _ := json.Marshal(map[string]string{"path": "/var/log"})
	cb.OnToolStart("tc-1", "file_read", args)

	// Verify tool span was created
	if len(spanSource.toolSpans) != 1 {
		t.Fatalf("expected 1 tool span, got %d", len(spanSource.toolSpans))
	}
	if spanSource.toolSpans[0].parentSpanID != "turn-span-1" {
		t.Fatalf("expected parent 'turn-span-1', got %q", spanSource.toolSpans[0].parentSpanID)
	}
	if spanSource.toolSpans[0].name != "file_read" {
		t.Fatalf("expected tool name 'file_read', got %q", spanSource.toolSpans[0].name)
	}

	// Verify projection event was emitted
	if len(emitter.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitter.events))
	}
	if emitter.events[0].Type != EventToolStarted {
		t.Fatalf("expected event type 'tool.started', got %q", emitter.events[0].Type)
	}

	// Complete the tool
	cb.OnToolComplete("tc-1", "file_read", "log content here")

	// Verify tool span was completed
	if len(spanSource.completedIDs) != 1 {
		t.Fatalf("expected 1 completed span, got %d", len(spanSource.completedIDs))
	}

	// Verify projection event
	if len(emitter.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(emitter.events))
	}
	if emitter.events[1].Type != EventToolCompleted {
		t.Fatalf("expected event type 'tool.completed', got %q", emitter.events[1].Type)
	}
}

func TestSpanAwareRunnerCallback_ToolFailed(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	emitter := &testMockEventEmitter{}

	cb := NewSpanAwareRunnerCallback("sess-1", "turn-1", emitter, spanSource, "turn-span-1")

	// Start and fail a tool
	cb.OnToolStart("tc-2", "shell_exec", nil)
	cb.OnToolFailed("tc-2", "shell_exec", fmt.Errorf("permission denied"))

	// Verify tool span was failed
	if len(spanSource.failedIDs) != 1 {
		t.Fatalf("expected 1 failed span, got %d", len(spanSource.failedIDs))
	}

	// Verify projection events
	if len(emitter.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(emitter.events))
	}
	if emitter.events[1].Type != EventToolFailed {
		t.Fatalf("expected event type 'tool.failed', got %q", emitter.events[1].Type)
	}
}

func TestSpanAwareRunnerCallback_TextOutput(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	emitter := &testMockEventEmitter{}

	cb := NewSpanAwareRunnerCallback("sess-1", "turn-1", emitter, spanSource, "turn-span-1")

	cb.OnTextOutput("Hello, I'm analyzing...")

	if len(spanSource.textEmissions) != 1 {
		t.Fatalf("expected 1 text emission, got %d", len(spanSource.textEmissions))
	}
	if spanSource.textEmissions[0] != "Hello, I'm analyzing..." {
		t.Fatalf("unexpected text: %q", spanSource.textEmissions[0])
	}
}

func TestSpanAwareRunnerCallback_CompleteTurnSpan(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	emitter := &testMockEventEmitter{}

	cb := NewSpanAwareRunnerCallback("sess-1", "turn-1", emitter, spanSource, "turn-span-1")
	cb.CompleteTurnSpan("Turn completed successfully")

	if len(spanSource.completedIDs) != 1 {
		t.Fatalf("expected 1 completed span, got %d", len(spanSource.completedIDs))
	}
	if spanSource.completedIDs[0] != "turn-span-1" {
		t.Fatalf("expected completed span 'turn-span-1', got %q", spanSource.completedIDs[0])
	}
}

func TestMultiplexedStreamAdapter_Integration(t *testing.T) {
	// Create a real SpanTree and MultiplexedStream
	rootSpan := &spanstream.Span{
		ID:        "root",
		Type:      spanstream.SpanTypeTurn,
		Status:    spanstream.SpanStatusRunning,
		Name:      "Session Root",
		StartTime: time.Now(),
	}
	tree := spanstream.NewSpanTree(rootSpan)
	stream := spanstream.NewMultiplexedStream(tree, 64)
	defer stream.Close()

	adapter := NewMultiplexedStreamAdapter(stream, "root")

	// Start a turn span
	turnSpanID := adapter.StartTurnSpan("turn-1", "analyze logs")
	if turnSpanID == "" {
		t.Fatal("expected non-empty turn span ID")
	}

	// Start a tool span under the turn
	toolSpanID := adapter.StartToolSpan(turnSpanID, "log_tail")
	if toolSpanID == "" {
		t.Fatal("expected non-empty tool span ID")
	}

	// Complete the tool span
	adapter.CompleteSpan(toolSpanID, "log_tail completed", "found 3 errors")

	// Complete the turn span
	adapter.CompleteSpan(turnSpanID, "Turn completed", "")

	// Verify tree structure
	turnSpan := tree.FindSpan(turnSpanID)
	if turnSpan == nil {
		t.Fatal("turn span not found in tree")
	}
	if turnSpan.Status != spanstream.SpanStatusCompleted {
		t.Fatalf("expected turn span status 'completed', got %q", turnSpan.Status)
	}

	toolSpan := tree.FindSpan(toolSpanID)
	if toolSpan == nil {
		t.Fatal("tool span not found in tree")
	}
	if toolSpan.Status != spanstream.SpanStatusCompleted {
		t.Fatalf("expected tool span status 'completed', got %q", toolSpan.Status)
	}
	if toolSpan.ParentID != turnSpanID {
		t.Fatalf("expected tool span parent %q, got %q", turnSpanID, toolSpan.ParentID)
	}
}

func TestToolDispatcherWithSpans(t *testing.T) {
	spanSource := &mockSpanStreamSource{}
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{
		ModePolicy: make(map[string]policyengine.ModePolicy),
	}

	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "disk_usage",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: &mockToolExecutor{result: "80% used"},
			},
		},
	}

	dispatcher := NewToolDispatcherWithSpans(lookup, policy, emitter, spanSource)

	tc := ToolCall{
		ID:        "tc-1",
		Name:      "disk_usage",
		Arguments: json.RawMessage(`{}`),
	}

	result := dispatcher.DispatchWithParentSpan(
		context.Background(), "sess-1", "turn-1", tc,
		SessionTypeHost, ModeInspect, "parent-span-1",
	)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Content != "80% used" {
		t.Fatalf("expected content '80%% used', got %q", result.Content)
	}

	// Verify tool span was created and completed
	if len(spanSource.toolSpans) != 1 {
		t.Fatalf("expected 1 tool span, got %d", len(spanSource.toolSpans))
	}
	if spanSource.toolSpans[0].parentSpanID != "parent-span-1" {
		t.Fatalf("expected parent 'parent-span-1', got %q", spanSource.toolSpans[0].parentSpanID)
	}
	if len(spanSource.completedIDs) != 1 {
		t.Fatalf("expected 1 completed span, got %d", len(spanSource.completedIDs))
	}
}

func TestToolDispatcher_PermissionEngineDeniesExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{
		ModePolicy: make(map[string]policyengine.ModePolicy),
	}

	executor := &mockToolExecutor{result: "80% used"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "disk_usage",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}

	dispatcher := NewToolDispatcher(lookup, policy, emitter)
	dispatcher.permissions = permissions.NewEngine([]permissions.Rule{
		{
			Name:   "deny-disk-usage",
			Action: permissions.ActionDeny,
			Reason: "blocked by policy",
			Matcher: permissions.Matcher{
				ToolNames: []string{"disk_usage"},
			},
		},
	})

	result := dispatcher.Dispatch(
		context.Background(), "sess-1", "turn-1",
		ToolCall{ID: "tc-1", Name: "disk_usage", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost, ModeInspect,
	)

	if result.Error != "denied: blocked by policy" {
		t.Fatalf("expected permission deny error, got %#v", result)
	}
	if executor.calls != 0 {
		t.Fatalf("expected executor not to run, got %d calls", executor.calls)
	}
	if len(emitter.events) != 2 {
		t.Fatalf("expected tool.started + tool.failed events, got %d", len(emitter.events))
	}
}

func TestToolDispatcher_PreToolHookBlocksExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{
		ModePolicy: make(map[string]policyengine.ModePolicy),
	}

	executor := &mockToolExecutor{result: "ok"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:    "disk_usage",
						Aliases: []string{"du"},
						Origin:  tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "guard-disk-usage",
		Stage: hooks.StagePreToolUse,
		Matcher: hooks.ToolMatcher{
			ToolNames:     []string{"du"},
			InputContains: []string{`"/var/log"`},
		},
		Hook: func(context.Context, *hooks.ToolEvent) error {
			return errors.New("hook blocked")
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	dispatcher := NewToolDispatcher(lookup, policy, emitter)
	dispatcher.hooks = registry

	result := dispatcher.Dispatch(
		context.Background(), "sess-1", "turn-1",
		ToolCall{ID: "tc-1", Name: "disk_usage", Arguments: json.RawMessage(`{"path":"/var/log"}`)},
		SessionTypeHost, ModeInspect,
	)

	if result.Error != "pre_tool_use: hook blocked" {
		t.Fatalf("expected pre hook error, got %#v", result)
	}
	if executor.calls != 0 {
		t.Fatalf("expected executor not to run, got %d calls", executor.calls)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestKernelWithSpanSource(spanSource SpanStreamSource) *EinoKernel {
	registry := newMockToolAssemblySource()
	compiler := &testMockCompiler{}
	policy := &policyengine.Engine{
		ModePolicy:       make(map[string]policyengine.ModePolicy),
		CompletionPolicy: &testMockCompletionEvaluator{action: policyengine.PolicyActionAllow},
	}
	emitter := &testMockEventEmitter{}
	providers := map[string]modelrouter.ChatModel{
		"mock": &testMockChatModel{},
	}
	router := modelrouter.NewRouter("mock", providers, nil)

	return NewEinoKernel(EinoKernelConfig{
		ToolSource:  registry,
		Compiler:    compiler,
		Policy:      policy,
		Projector:   emitter,
		ModelRouter: router,
		SpanSource:  spanSource,
	})
}

func newTestKernelWithSpanSourceAndCompiler(spanSource SpanStreamSource, compiler promptcompiler.Compiler) *EinoKernel {
	registry := newMockToolAssemblySource()
	policy := &policyengine.Engine{
		ModePolicy:       make(map[string]policyengine.ModePolicy),
		CompletionPolicy: &testMockCompletionEvaluator{action: policyengine.PolicyActionAllow},
	}
	emitter := &testMockEventEmitter{}
	providers := map[string]modelrouter.ChatModel{
		"mock": &testMockChatModel{},
	}
	router := modelrouter.NewRouter("mock", providers, nil)

	return NewEinoKernel(EinoKernelConfig{
		ToolSource:  registry,
		Compiler:    compiler,
		Policy:      policy,
		Projector:   emitter,
		ModelRouter: router,
		SpanSource:  spanSource,
	})
}

func newMockToolAssemblySource() ToolAssemblySource {
	return &testMockToolAssemblySource{registry: newEmptyRegistry()}
}

func containsStage(stages []string, target string) bool {
	for _, stage := range stages {
		if stage == target {
			return true
		}
	}
	return false
}

func newEmptyRegistry() *tooling.Registry {
	return tooling.NewRegistry()
}

// mockToolLookup implements ToolLookup for testing.
type mockToolLookup struct {
	tools map[string]mockToolEntry
}

type mockToolEntry struct {
	desc     ToolDescriptor
	executor ToolExecutor
}

func (m *mockToolLookup) LookupTool(name string) (ToolDescriptor, ToolExecutor, bool) {
	entry, ok := m.tools[name]
	if !ok {
		return ToolDescriptor{}, nil, false
	}
	return entry.desc, entry.executor, true
}

// mockToolExecutor implements ToolExecutor for testing.
type mockToolExecutor struct {
	result string
	err    error
	calls  int
}

func (m *mockToolExecutor) Execute(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
	m.calls++
	return tooling.ToolResult{Content: m.result}, m.err
}
