package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/permissions"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherObserverRecordsToolOutcome(t *testing.T) {
	t.Run("success records bounded result metadata", func(t *testing.T) {
		observer := &toolRecordingObserver{}
		emitter := &testMockEventEmitter{}
		largeResult := strings.Repeat("x", 3*1024)
		lookup := &mockToolLookup{
			tools: map[string]mockToolEntry{
				"read_log": {
					desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
						Name:      "read_log",
						RiskLevel: tooling.ToolRiskLow,
					}},
					executor: &mockToolExecutor{result: largeResult},
				},
			},
		}
		dispatcher := NewToolDispatcher(lookup, nil, emitter).WithObserver(observer)

		result := dispatcher.Dispatch(context.Background(), "sess-tool", "turn-tool", ToolCall{
			ID:        "call-read-log",
			Name:      "read_log",
			Arguments: json.RawMessage(`{"path":"/var/log/app.log"}`),
		}, SessionTypeHost, ModeInspect)

		if result.Error != "" {
			t.Fatalf("dispatch returned error: %s", result.Error)
		}
		if len(observer.toolCalls) != 1 {
			t.Fatalf("tool calls = %d, want 1", len(observer.toolCalls))
		}
		call := observer.toolCalls[0]
		if call.ToolName != "read_log" || call.ToolCallID != "call-read-log" || call.Risk != "low" {
			t.Fatalf("tool call attrs = %#v", call)
		}
		span := observer.spans[0]
		if span.status != "completed" {
			t.Fatalf("span status = %q, want completed", span.status)
		}
		if span.attrs["tool.outcome"] != "tool_result" {
			t.Fatalf("tool.outcome = %#v, want tool_result", span.attrs["tool.outcome"])
		}
		if span.attrs["tool.result_truncated"] != true || span.attrs["tool.raw_ref"] == "" {
			t.Fatalf("span attrs missing bounded result metadata: %#v", span.attrs)
		}
		if got, ok := span.attrs["tool.result_bytes"].(int); !ok || got != len([]byte(largeResult)) {
			t.Fatalf("tool.result_bytes = %#v, want %d", span.attrs["tool.result_bytes"], len([]byte(largeResult)))
		}
	})

	t.Run("failure records failed status", func(t *testing.T) {
		observer := &toolRecordingObserver{}
		emitter := &testMockEventEmitter{}
		lookup := &mockToolLookup{
			tools: map[string]mockToolEntry{
				"fragile_tool": {
					desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "fragile_tool"}},
					executor: &mockToolExecutor{err: assertErr("boom")},
				},
			},
		}
		dispatcher := NewToolDispatcher(lookup, nil, emitter).WithObserver(observer)

		result := dispatcher.Dispatch(context.Background(), "sess-tool", "turn-tool", ToolCall{
			ID:   "call-fragile",
			Name: "fragile_tool",
		}, SessionTypeHost, ModeExecute)

		if result.Error == "" {
			t.Fatalf("dispatch result = %#v, want error", result)
		}
		span := observer.spans[0]
		if span.status != "failed" || span.message != "boom" {
			t.Fatalf("span status/message = %q/%q, want failed/boom", span.status, span.message)
		}
		if span.attrs["error"] != "boom" || span.attrs["tool.outcome"] != "tool_failed" {
			t.Fatalf("failure span attrs = %#v", span.attrs)
		}
	})

	t.Run("approval block records blocked outcome", func(t *testing.T) {
		observer := &toolRecordingObserver{}
		emitter := &testMockEventEmitter{}
		executor := &mockToolExecutor{result: "restarted"}
		lookup := &mockToolLookup{
			tools: map[string]mockToolEntry{
				"restart_service": {
					desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "restart_service"}},
					executor: executor,
				},
			},
		}
		dispatcher := NewToolDispatcher(lookup, nil, emitter).
			WithObserver(observer).
			WithPermissions(permissions.NewEngine([]permissions.Rule{{
				Name:   "restart-needs-approval",
				Action: permissions.ActionAsk,
				Reason: "approval required",
				Matcher: permissions.Matcher{
					ToolNames: []string{"restart_service"},
				},
			}}))

		result := dispatcher.Dispatch(context.Background(), "sess-tool", "turn-tool", ToolCall{
			ID:   "call-restart",
			Name: "restart_service",
		}, SessionTypeHost, ModeExecute)

		if !result.Blocked || result.Outcome != "approval_needed" {
			t.Fatalf("dispatch result = %#v, want approval block", result)
		}
		if executor.calls != 0 {
			t.Fatalf("executor calls = %d, want 0", executor.calls)
		}
		span := observer.spans[0]
		if span.status != "blocked" || span.attrs["tool.outcome"] != "approval_needed" {
			t.Fatalf("approval span = %#v status=%q", span.attrs, span.status)
		}
	})
}

type toolRecordingObserver struct {
	toolCalls []ToolCallSpanAttrs
	spans     []*toolRecordingSpan
}

func (o *toolRecordingObserver) ContextWithTraceContext(ctx context.Context, _ TraceContextCarrier) context.Context {
	return normalizeObserverContext(ctx)
}

func (o *toolRecordingObserver) StartTurn(ctx context.Context, _ TurnSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (o *toolRecordingObserver) StartStage(ctx context.Context, _ StageSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (o *toolRecordingObserver) StartModelCall(ctx context.Context, _ ModelCallSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (o *toolRecordingObserver) StartToolCall(ctx context.Context, attrs ToolCallSpanAttrs) (context.Context, ObservedSpan) {
	o.toolCalls = append(o.toolCalls, attrs)
	span := &toolRecordingSpan{attrs: map[string]any{}}
	o.spans = append(o.spans, span)
	return normalizeObserverContext(ctx), span
}

type toolRecordingSpan struct {
	attrs   map[string]any
	status  string
	message string
}

func (s *toolRecordingSpan) SetAttributes(attrs map[string]any) {
	for key, value := range attrs {
		s.attrs[key] = value
	}
}

func (s *toolRecordingSpan) TraceContext() TraceContextCarrier { return nil }

func (s *toolRecordingSpan) SetStatus(status string, message string) {
	s.status = status
	s.message = message
}

func (s *toolRecordingSpan) End() {}
