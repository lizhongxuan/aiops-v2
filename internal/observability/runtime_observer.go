package observability

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type RuntimeObserver struct {
	tracer trace.Tracer
	cfg    Config
}

func NewRuntimeObserver(tracer trace.Tracer, cfg Config) RuntimeObserver {
	if tracer == nil {
		tracer = trace.NewNoopTracerProvider().Tracer("aiops-v2/runtime")
	}
	return RuntimeObserver{tracer: tracer, cfg: cfg}
}

func (o RuntimeObserver) ContextWithTraceContext(ctx context.Context, carrier runtimekernel.TraceContextCarrier) context.Context {
	base := normalizeContext(ctx)
	if len(carrier) == 0 {
		return base
	}
	mapCarrier := propagation.MapCarrier{}
	for key, value := range carrier {
		if key == "" || value == "" {
			continue
		}
		mapCarrier[key] = value
	}
	if len(mapCarrier) == 0 {
		return base
	}
	return propagation.TraceContext{}.Extract(base, mapCarrier)
}

func (o RuntimeObserver) StartTurn(ctx context.Context, attrs runtimekernel.TurnSpanAttrs) (context.Context, runtimekernel.ObservedSpan) {
	ctx, span := o.tracer.Start(normalizeContext(ctx), "agent.turn")
	span.SetAttributes(
		attribute.String("openinference.span.kind", "AGENT"),
		attribute.String("session.id", attrs.SessionID),
		attribute.String("turn.id", attrs.TurnID),
		attribute.String("client_turn_id", attrs.ClientTurnID),
		attribute.String("client_message_id", attrs.ClientMessageID),
		attribute.String("session_type", attrs.SessionType),
		attribute.String("mode", attrs.Mode),
		attribute.String("host_id", attrs.HostID),
		attribute.String("aiops.project", o.cfg.Project),
	)
	return ctx, observedSpan{span: span}
}

func (o RuntimeObserver) StartStage(ctx context.Context, attrs runtimekernel.StageSpanAttrs) (context.Context, runtimekernel.ObservedSpan) {
	ctx, span := o.tracer.Start(normalizeContext(ctx), "stage."+attrs.Stage)
	span.SetAttributes(
		attribute.String("openinference.span.kind", "CHAIN"),
		attribute.String("session.id", attrs.SessionID),
		attribute.String("turn.id", attrs.TurnID),
		attribute.String("stage", attrs.Stage),
		attribute.Int("iteration", attrs.Iteration),
	)
	return ctx, observedSpan{span: span}
}

func (o RuntimeObserver) StartModelCall(ctx context.Context, attrs runtimekernel.ModelCallSpanAttrs) (context.Context, runtimekernel.ObservedSpan) {
	ctx, span := o.tracer.Start(normalizeContext(ctx), "model_call")
	spanAttrs := []attribute.KeyValue{
		attribute.String("openinference.span.kind", "LLM"),
		attribute.String("session.id", attrs.SessionID),
		attribute.String("turn.id", attrs.TurnID),
		attribute.Int("iteration", attrs.Iteration),
		attribute.String("model.name", attrs.ModelName),
		attribute.String("prompt.stable_hash", attrs.PromptStableHash),
		attribute.String("visible_tools", strings.Join(attrs.VisibleTools, ",")),
		attribute.Int("input.message_count", attrs.MessageCount),
		attribute.String("trace.file", attrs.TraceFile),
		attribute.Bool("output.has_tool_calls", attrs.HasToolCalls),
		attribute.Int("output.tool_call_count", attrs.ToolCallCount),
	}
	if strings.TrimSpace(attrs.TraceDiffFile) != "" {
		spanAttrs = append(spanAttrs, attribute.String("trace.diff", attrs.TraceDiffFile))
	}
	spanAttrs = appendPromptFingerprintAttrs(spanAttrs, attrs.PromptFingerprint)
	span.SetAttributes(spanAttrs...)
	return ctx, observedSpan{span: span}
}

func appendPromptFingerprintAttrs(attrs []attribute.KeyValue, fp map[string]string) []attribute.KeyValue {
	mapping := map[string]string{
		"version":                   "prompt.version",
		"compilerVersion":           "prompt.compiler_version",
		"absoluteSystemHash":        "prompt.absolute_system_hash",
		"roleProfileHash":           "prompt.role_profile_hash",
		"stableRuntimeContractHash": "prompt.stable_runtime_contract_hash",
		"stablePrefixHash":          "prompt.stable_prefix_hash",
		"turnStableHash":            "prompt.turn_stable_hash",
		"turnPrefixHash":            "prompt.turn_prefix_hash",
		"conversationHistoryHash":   "prompt.conversation_history_hash",
		"dynamicContextHash":        "prompt.dynamic_context_hash",
		"currentUserInputHash":      "prompt.current_user_input_hash",
		"modelInputHash":            "prompt.model_input_hash",
		"systemHash":                "prompt.system_hash",
		"developerHash":             "prompt.developer_hash",
		"toolRegistryHash":          "prompt.tool_registry_hash",
		"runtimePolicyHash":         "prompt.runtime_policy_hash",
		"protocolStateHash":         "prompt.protocol_state_hash",
	}
	for source, attrName := range mapping {
		if value := strings.TrimSpace(fp[source]); value != "" {
			attrs = append(attrs, attribute.String(attrName, value))
		}
	}
	return attrs
}

func (o RuntimeObserver) StartToolCall(ctx context.Context, attrs runtimekernel.ToolCallSpanAttrs) (context.Context, runtimekernel.ObservedSpan) {
	ctx, span := o.tracer.Start(normalizeContext(ctx), "tool_call."+attrs.ToolName)
	span.SetAttributes(
		attribute.String("openinference.span.kind", "TOOL"),
		attribute.String("session.id", attrs.SessionID),
		attribute.String("turn.id", attrs.TurnID),
		attribute.String("tool.name", attrs.ToolName),
		attribute.String("tool.call_id", attrs.ToolCallID),
		attribute.String("tool.risk", attrs.Risk),
	)
	return ctx, observedSpan{span: span}
}

type observedSpan struct {
	span trace.Span
}

func (s observedSpan) TraceContext() runtimekernel.TraceContextCarrier {
	if s.span == nil || !s.span.SpanContext().IsValid() {
		return nil
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(trace.ContextWithSpan(context.Background(), s.span), carrier)
	out := make(runtimekernel.TraceContextCarrier, len(carrier))
	for key, value := range carrier {
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s observedSpan) SetAttributes(attrs map[string]any) {
	if s.span == nil {
		return
	}
	for key, value := range attrs {
		s.span.SetAttributes(attributeForValue(key, value))
	}
}

func (s observedSpan) SetStatus(status string, message string) {
	if s.span == nil {
		return
	}
	if strings.EqualFold(status, "failed") || strings.TrimSpace(message) != "" {
		s.span.SetStatus(codes.Error, message)
		return
	}
	s.span.SetStatus(codes.Ok, message)
}

func (s observedSpan) End() {
	if s.span != nil {
		s.span.End()
	}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func attributeForValue(key string, value any) attribute.KeyValue {
	switch typed := value.(type) {
	case string:
		return attribute.String(key, typed)
	case bool:
		return attribute.Bool(key, typed)
	case int:
		return attribute.Int(key, typed)
	case int64:
		return attribute.Int64(key, typed)
	case float64:
		return attribute.Float64(key, typed)
	default:
		return attribute.String(key, fmt.Sprint(typed))
	}
}
