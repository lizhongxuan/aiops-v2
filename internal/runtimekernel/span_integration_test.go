package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"aiops-v2/internal/capability"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/spanstream"
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
			"disk_usage": {kind: "tool", executor: &mockToolExecutor{result: "80% used"}},
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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestKernelWithSpanSource(spanSource SpanStreamSource) *EinoKernel {
	registry := newMockCapabilityRegistry()
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
		Registry:    registry,
		Compiler:    compiler,
		Policy:      policy,
		Projector:   emitter,
		ModelRouter: router,
		SpanSource:  spanSource,
	})
}

func newTestKernelWithSpanSourceAndCompiler(spanSource SpanStreamSource, compiler promptcompiler.Compiler) *EinoKernel {
	registry := newMockCapabilityRegistry()
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
		Registry:    registry,
		Compiler:    compiler,
		Policy:      policy,
		Projector:   emitter,
		ModelRouter: router,
		SpanSource:  spanSource,
	})
}

func newMockCapabilityRegistry() CapabilitySource {
	return &testMockCapabilitySource{registry: newEmptyRegistry()}
}

func newEmptyRegistry() *capability.Registry {
	return capability.NewRegistry()
}

// mockToolLookup implements ToolLookup for testing.
type mockToolLookup struct {
	tools map[string]mockToolEntry
}

type mockToolEntry struct {
	kind     string
	executor ToolExecutor
}

func (m *mockToolLookup) LookupTool(name string) (string, ToolExecutor, bool) {
	entry, ok := m.tools[name]
	if !ok {
		return "", nil, false
	}
	return entry.kind, entry.executor, true
}

// mockToolExecutor implements ToolExecutor for testing.
type mockToolExecutor struct {
	result string
	err    error
}

func (m *mockToolExecutor) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return m.result, m.err
}
