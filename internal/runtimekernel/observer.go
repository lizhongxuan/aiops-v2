package runtimekernel

import "context"

// Observer is the runtime-owned tracing hook surface.
// It intentionally does not mention OpenTelemetry or any vendor-specific SDK.
type Observer interface {
	ContextWithTraceContext(context.Context, TraceContextCarrier) context.Context
	StartTurn(context.Context, TurnSpanAttrs) (context.Context, ObservedSpan)
	StartStage(context.Context, StageSpanAttrs) (context.Context, ObservedSpan)
	StartModelCall(context.Context, ModelCallSpanAttrs) (context.Context, ObservedSpan)
	StartToolCall(context.Context, ToolCallSpanAttrs) (context.Context, ObservedSpan)
}

// ObservedSpan is the minimal span API needed by the runtime.
type ObservedSpan interface {
	TraceContext() TraceContextCarrier
	SetAttributes(attrs map[string]any)
	SetStatus(status string, message string)
	End()
}

// TraceContextCarrier stores a vendor-neutral trace context for suspended
// turns. OpenTelemetry observers use W3C propagation keys such as traceparent.
type TraceContextCarrier map[string]string

type TurnSpanAttrs struct {
	SessionID       string
	TurnID          string
	ClientTurnID    string
	ClientMessageID string
	SessionType     string
	Mode            string
	HostID          string
	Input           string
}

type StageSpanAttrs struct {
	SessionID string
	TurnID    string
	Stage     string
	Iteration int
}

type ModelCallSpanAttrs struct {
	SessionID         string
	TurnID            string
	Iteration         int
	ModelName         string
	PromptStableHash  string
	PromptFingerprint map[string]string
	VisibleTools      []string
	MessageCount      int
	TraceFile         string
	TraceDiffFile     string
	HasToolCalls      bool
	ToolCallCount     int
}

type ToolCallSpanAttrs struct {
	SessionID       string
	TurnID          string
	ToolName        string
	ToolCallID      string
	Risk            string
	Outcome         string
	ResultBytes     int
	ResultTruncated bool
	RawRef          string
	Error           string
}

type NoopObserver struct{}

func (NoopObserver) ContextWithTraceContext(ctx context.Context, _ TraceContextCarrier) context.Context {
	return normalizeObserverContext(ctx)
}

func (NoopObserver) StartTurn(ctx context.Context, _ TurnSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (NoopObserver) StartStage(ctx context.Context, _ StageSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (NoopObserver) StartModelCall(ctx context.Context, _ ModelCallSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func (NoopObserver) StartToolCall(ctx context.Context, _ ToolCallSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

type noopObservedSpan struct{}

func (noopObservedSpan) TraceContext() TraceContextCarrier { return nil }
func (noopObservedSpan) SetAttributes(map[string]any)      {}
func (noopObservedSpan) SetStatus(string, string)          {}
func (noopObservedSpan) End()                              {}

func normalizeObserverContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func copyTraceContextCarrier(carrier TraceContextCarrier) TraceContextCarrier {
	if len(carrier) == 0 {
		return nil
	}
	out := make(TraceContextCarrier, len(carrier))
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
