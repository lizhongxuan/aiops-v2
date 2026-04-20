package runtimekernel

import (
	"encoding/json"
	"fmt"
	"time"

	"aiops-v2/internal/spanstream"
)

// ---------------------------------------------------------------------------
// SpanStreamSource — interface to avoid circular imports with spanstream pkg.
// EinoKernel uses this to create and manage spans during turn execution.
// ---------------------------------------------------------------------------

// SpanStreamSource provides span lifecycle management for the EinoKernel.
// Implemented by an adapter wrapping *spanstream.MultiplexedStream.
type SpanStreamSource interface {
	// StartTurnSpan creates a root span for a new turn and returns its ID.
	StartTurnSpan(turnID string, input string) string

	// StartToolSpan creates a child span under the given parent for a tool call.
	StartToolSpan(parentSpanID string, toolName string) string

	// CompleteSpan marks a span as completed with summary and detail.
	CompleteSpan(spanID string, summary string, detail string)

	// FailSpan marks a span as failed with an error message.
	FailSpan(spanID string, errMsg string)

	// EmitText emits a text streaming event (LLM output).
	EmitText(text string)

	// Chunks returns the read-only channel for consuming typed event chunks.
	Chunks() <-chan spanstream.TypedEventChunk
}

// ---------------------------------------------------------------------------
// MultiplexedStreamAdapter adapts *spanstream.MultiplexedStream to SpanStreamSource.
// ---------------------------------------------------------------------------

// MultiplexedStreamAdapter wraps a MultiplexedStream to implement SpanStreamSource.
type MultiplexedStreamAdapter struct {
	stream     *spanstream.MultiplexedStream
	rootSpanID string // the tree's root span ID, used as parent for turn spans
}

// NewMultiplexedStreamAdapter creates a new adapter around the given stream.
// rootSpanID is the ID of the tree's root span (used as parent for turn spans).
func NewMultiplexedStreamAdapter(stream *spanstream.MultiplexedStream, rootSpanID string) *MultiplexedStreamAdapter {
	return &MultiplexedStreamAdapter{stream: stream, rootSpanID: rootSpanID}
}

// StartTurnSpan creates a root span for a turn. The turn span is added as a
// child of the tree's root span.
func (a *MultiplexedStreamAdapter) StartTurnSpan(turnID string, input string) string {
	name := fmt.Sprintf("Turn: %s", truncateString(input, 50))
	return a.stream.StartSpan(a.rootSpanID, spanstream.SpanTypeTurn, name)
}

// StartToolSpan creates a child span for a tool call under the given parent.
func (a *MultiplexedStreamAdapter) StartToolSpan(parentSpanID string, toolName string) string {
	name := fmt.Sprintf("Tool: %s", toolName)
	return a.stream.StartSpan(parentSpanID, spanstream.SpanTypeToolCall, name)
}

// CompleteSpan marks a span as completed.
func (a *MultiplexedStreamAdapter) CompleteSpan(spanID string, summary string, detail string) {
	a.stream.CompleteSpan(spanID, summary, detail)
}

// FailSpan marks a span as failed.
func (a *MultiplexedStreamAdapter) FailSpan(spanID string, errMsg string) {
	a.stream.FailSpan(spanID, fmt.Errorf("%s", errMsg))
}

// EmitText emits a text streaming event.
func (a *MultiplexedStreamAdapter) EmitText(text string) {
	a.stream.EmitText(text)
}

// Chunks returns the event channel from the underlying stream.
func (a *MultiplexedStreamAdapter) Chunks() <-chan spanstream.TypedEventChunk {
	return a.stream.Chunks()
}

// ---------------------------------------------------------------------------
// SpanAwareRunnerCallback extends RunnerCallback with span tracking.
// It creates child spans for each tool call and maps agent events to spans.
// ---------------------------------------------------------------------------

// SpanAwareRunnerCallback bridges adk.Runner events to both the Projection
// layer and the SpanTree. Each tool call creates a child span under the
// turn's root span.
type SpanAwareRunnerCallback struct {
	sessionID    string
	turnID       string
	projector    EventEmitter
	spanSource   SpanStreamSource
	turnSpanID   string            // root span for this turn
	toolSpanIDs  map[string]string // toolCallID → spanID
}

// NewSpanAwareRunnerCallback creates a callback that tracks spans for tool calls.
func NewSpanAwareRunnerCallback(
	sessionID, turnID string,
	projector EventEmitter,
	spanSource SpanStreamSource,
	turnSpanID string,
) *SpanAwareRunnerCallback {
	return &SpanAwareRunnerCallback{
		sessionID:   sessionID,
		turnID:      turnID,
		projector:   projector,
		spanSource:  spanSource,
		turnSpanID:  turnSpanID,
		toolSpanIDs: make(map[string]string),
	}
}

// OnToolStart is called when a tool execution begins. Creates a child span.
func (cb *SpanAwareRunnerCallback) OnToolStart(toolCallID, toolName string, args json.RawMessage) {
	// Create child span under the turn root span
	if cb.spanSource != nil {
		spanID := cb.spanSource.StartToolSpan(cb.turnSpanID, toolName)
		cb.toolSpanIDs[toolCallID] = spanID
	}

	// Emit projection event (same as RunnerCallback)
	payload, _ := json.Marshal(map[string]interface{}{
		"id":       toolCallID,
		"toolName": toolName,
		"args":     args,
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolStarted,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// OnToolComplete is called when a tool execution completes. Completes the span.
func (cb *SpanAwareRunnerCallback) OnToolComplete(toolCallID, toolName, result string) {
	// Complete the tool span
	if cb.spanSource != nil {
		if spanID, ok := cb.toolSpanIDs[toolCallID]; ok {
			summary := fmt.Sprintf("%s completed", toolName)
			cb.spanSource.CompleteSpan(spanID, summary, result)
			delete(cb.toolSpanIDs, toolCallID)
		}
	}

	// Emit projection event
	payload, _ := json.Marshal(map[string]string{
		"id":       toolCallID,
		"toolName": toolName,
		"result":   result,
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolCompleted,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// OnToolFailed is called when a tool execution fails. Fails the span.
func (cb *SpanAwareRunnerCallback) OnToolFailed(toolCallID, toolName string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	// Fail the tool span
	if cb.spanSource != nil {
		if spanID, ok := cb.toolSpanIDs[toolCallID]; ok {
			cb.spanSource.FailSpan(spanID, errMsg)
			delete(cb.toolSpanIDs, toolCallID)
		}
	}

	// Emit projection event
	payload, _ := json.Marshal(map[string]string{
		"id":       toolCallID,
		"toolName": toolName,
		"error":    errMsg,
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolFailed,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// OnTextOutput is called when the LLM produces text output. Emits to span stream.
func (cb *SpanAwareRunnerCallback) OnTextOutput(text string) {
	if cb.spanSource != nil {
		cb.spanSource.EmitText(text)
	}
}

// CompleteTurnSpan marks the turn's root span as completed.
func (cb *SpanAwareRunnerCallback) CompleteTurnSpan(summary string) {
	if cb.spanSource != nil && cb.turnSpanID != "" {
		cb.spanSource.CompleteSpan(cb.turnSpanID, summary, "")
	}
}

// FailTurnSpan marks the turn's root span as failed.
func (cb *SpanAwareRunnerCallback) FailTurnSpan(errMsg string) {
	if cb.spanSource != nil && cb.turnSpanID != "" {
		cb.spanSource.FailSpan(cb.turnSpanID, errMsg)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// truncateString truncates a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
