package observability

import (
	"context"
	"strings"
	"testing"

	"aiops-v2/internal/runtimekernel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRuntimeObserverCreatesTurnAndModelSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{Project: "local"})

	ctx, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{
		SessionID:   "sess-1",
		TurnID:      "turn-1",
		SessionType: "host",
		Mode:        "chat",
		Input:       "check disk",
	})
	_, model := observer.StartModelCall(ctx, runtimekernel.ModelCallSpanAttrs{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		Iteration:        0,
		ModelName:        "local-model",
		PromptStableHash: "hash-1",
		VisibleTools:     []string{"read_file"},
		MessageCount:     3,
		TraceFile:        ".data/model-input-traces/sess-1/turn-1/iteration-000.md",
		TraceDiffFile:    ".data/model-input-traces/sess-1/turn-1/input.diff.md",
	})
	model.SetAttributes(map[string]any{
		"output.has_tool_calls":  true,
		"output.tool_call_count": 1,
	})
	model.SetStatus("completed", "")
	model.End()
	turn.SetStatus("completed", "")
	turn.End()

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(spans))
	}
	if spans[0].Name() != "model_call" {
		t.Fatalf("first ended span = %q, want model_call", spans[0].Name())
	}
	if spans[1].Name() != "agent.turn" {
		t.Fatalf("second ended span = %q, want agent.turn", spans[1].Name())
	}
	if spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() {
		t.Fatalf("model parent = %s, want turn span %s", spans[0].Parent().SpanID(), spans[1].SpanContext().SpanID())
	}
	assertSpanAttribute(t, spans[0].Attributes(), "trace.file", ".data/model-input-traces/sess-1/turn-1/iteration-000.md")
	assertSpanAttribute(t, spans[0].Attributes(), "trace.diff", ".data/model-input-traces/sess-1/turn-1/input.diff.md")
	assertSpanAttribute(t, spans[0].Attributes(), "prompt.stable_hash", "hash-1")
}

func TestRuntimeObserverCreatesToolSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{})

	ctx, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{SessionID: "sess-1", TurnID: "turn-1"})
	_, tool := observer.StartToolCall(ctx, runtimekernel.ToolCallSpanAttrs{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		ToolName:   "read_file",
		ToolCallID: "call-1",
		Risk:       "low",
	})
	tool.SetAttributes(map[string]any{
		"tool.outcome":          "tool_result",
		"tool.result_bytes":     12,
		"tool.result_truncated": false,
	})
	tool.End()
	turn.End()

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(spans))
	}
	if spans[0].Name() != "tool_call.read_file" {
		t.Fatalf("tool span name = %q", spans[0].Name())
	}
	assertSpanAttribute(t, spans[0].Attributes(), "tool.name", "read_file")
	assertSpanAttribute(t, spans[0].Attributes(), "tool.outcome", "tool_result")
}

func TestRuntimeObserverRecordsPromptFingerprintAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{})

	ctx, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{SessionID: "sess-1", TurnID: "turn-1"})
	_, model := observer.StartModelCall(ctx, runtimekernel.ModelCallSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		PromptFingerprint: map[string]string{
			"version":           "prompt-fingerprint-v1",
			"systemHash":        "system-hash",
			"developerHash":     "developer-hash",
			"toolRegistryHash":  "tool-hash",
			"runtimePolicyHash": "policy-hash",
			"protocolStateHash": "protocol-hash",
		},
	})
	model.End()
	turn.End()

	modelSpan := findEndedSpan(t, recorder.Ended(), "model_call")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.version", "prompt-fingerprint-v1")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.system_hash", "system-hash")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.developer_hash", "developer-hash")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.tool_registry_hash", "tool-hash")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.runtime_policy_hash", "policy-hash")
	assertSpanAttribute(t, modelSpan.Attributes(), "prompt.protocol_state_hash", "protocol-hash")
}

func TestRuntimeObserverCanRehydrateTurnTraceContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{})

	_, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
	})
	carrier := turn.TraceContext()
	if carrier["traceparent"] == "" {
		t.Fatalf("trace carrier missing traceparent: %#v", carrier)
	}
	turn.End()

	resumedCtx := observer.ContextWithTraceContext(context.Background(), carrier)
	_, tool := observer.StartToolCall(resumedCtx, runtimekernel.ToolCallSpanAttrs{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		ToolName:   "exec_command",
		ToolCallID: "call-1",
	})
	tool.End()

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(spans))
	}
	turnSpan := findEndedSpan(t, spans, "agent.turn")
	toolSpan := findEndedSpan(t, spans, "tool_call.exec_command")
	if toolSpan.SpanContext().TraceID() != turnSpan.SpanContext().TraceID() {
		t.Fatalf("tool trace id = %s, want turn trace id %s", toolSpan.SpanContext().TraceID(), turnSpan.SpanContext().TraceID())
	}
	if toolSpan.Parent().SpanID() != turnSpan.SpanContext().SpanID() {
		t.Fatalf("tool parent = %s, want turn span %s", toolSpan.Parent().SpanID(), turnSpan.SpanContext().SpanID())
	}
}

func TestRuntimeObserverOmitsEmptyTraceDiffAttribute(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{})

	ctx, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
	})
	_, model := observer.StartModelCall(ctx, runtimekernel.ModelCallSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		TraceFile: ".data/model-input-traces/sess-1/turn-1/iteration-000.md",
	})
	model.End()
	turn.End()

	modelSpan := findEndedSpan(t, recorder.Ended(), "model_call")
	if hasSpanAttribute(modelSpan.Attributes(), "trace.diff") {
		t.Fatal("model_call should omit trace.diff when no diff file exists")
	}
}

func TestRuntimeObserverDoesNotRecordFullPromptByDefault(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	observer := NewRuntimeObserver(tp.Tracer("test"), Config{})

	_, turn := observer.StartTurn(context.Background(), runtimekernel.TurnSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Input:     "secret-token-123 should not be written to span attributes",
	})
	turn.End()

	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			if strings.Contains(attr.Value.AsString(), "secret-token-123") {
				t.Fatalf("sensitive prompt text leaked into attribute %s", attr.Key)
			}
		}
	}
}

func hasSpanAttribute(attrs []attribute.KeyValue, key string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return true
		}
	}
	return false
}

func findEndedSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("missing span %q", name)
	return nil
}

func assertSpanAttribute(t *testing.T, attrs []attribute.KeyValue, key string, want string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if attr.Value.AsString() != want {
				t.Fatalf("%s = %q, want %q", key, attr.Value.AsString(), want)
			}
			return
		}
	}
	t.Fatalf("missing span attribute %s", key)
}
