package spanstream

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EventChunkType identifies the type of a typed event chunk.
type EventChunkType string

const (
	ChunkTypeText         EventChunkType = "text"
	ChunkTypeSpanStart    EventChunkType = "span_start"
	ChunkTypeSpanProgress EventChunkType = "span_progress"
	ChunkTypeSpanComplete EventChunkType = "span_complete"
	ChunkTypeStatus       EventChunkType = "status"
	ChunkTypeSummary      EventChunkType = "summary"
)

// TypedEventChunk is a typed event block pushed from the engine to the frontend.
type TypedEventChunk struct {
	Type      EventChunkType  `json:"type"`
	SpanID    string          `json:"spanId,omitempty"`
	ParentID  string          `json:"parentId,omitempty"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

// MultiplexedStream manages multiplexed event streaming with span tracking.
type MultiplexedStream struct {
	chunks chan TypedEventChunk
	spans  sync.Map  // spanID → *Span
	tree   *SpanTree
	seq    atomic.Int64 // monotonic sequence for ordering
}

// NewMultiplexedStream creates a new MultiplexedStream with the given SpanTree
// and channel buffer size.
func NewMultiplexedStream(tree *SpanTree, bufferSize int) *MultiplexedStream {
	if bufferSize <= 0 {
		bufferSize = 256
	}
	return &MultiplexedStream{
		chunks: make(chan TypedEventChunk, bufferSize),
		tree:   tree,
	}
}

// Chunks returns the read-only channel for consuming events.
func (ms *MultiplexedStream) Chunks() <-chan TypedEventChunk {
	return ms.chunks
}

// EmitText emits a text streaming event (LLM output).
func (ms *MultiplexedStream) EmitText(text string) {
	data, _ := json.Marshal(text)
	chunk := TypedEventChunk{
		Type:      ChunkTypeText,
		Data:      data,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// StartSpan creates a new child span under the given parent and emits a SpanStart event.
// Returns the new span's ID.
func (ms *MultiplexedStream) StartSpan(parentID string, spanType SpanType, name string) string {
	spanID := ms.generateSpanID()

	span := &Span{
		ID:        spanID,
		ParentID:  parentID,
		Type:      spanType,
		Status:    SpanStatusRunning,
		Name:      name,
		StartTime: time.Now(),
	}

	// Register in the span map for quick lookup
	ms.spans.Store(spanID, span)

	// Add to the tree structure
	if ms.tree != nil {
		ms.tree.AddChild(parentID, span)
	}

	// Emit span_start event
	startData, _ := json.Marshal(struct {
		SpanType SpanType `json:"spanType"`
		Name     string   `json:"name"`
	}{
		SpanType: spanType,
		Name:     name,
	})

	chunk := TypedEventChunk{
		Type:      ChunkTypeSpanStart,
		SpanID:    spanID,
		ParentID:  parentID,
		Data:      startData,
		Timestamp: span.StartTime,
	}
	ms.emit(chunk)

	return spanID
}

// CompleteSpan marks a span as completed and emits a SpanComplete event.
func (ms *MultiplexedStream) CompleteSpan(spanID string, summary string, detail string) {
	// Update span in the map
	if val, ok := ms.spans.Load(spanID); ok {
		span := val.(*Span)
		span.Status = SpanStatusCompleted
		span.Summary = summary
		span.Detail = detail
		now := time.Now()
		span.EndTime = &now
	}

	// Update in the tree
	if ms.tree != nil {
		ms.tree.CompleteSpan(spanID, summary, detail)
	}

	// Emit span_complete event
	completeData, _ := json.Marshal(struct {
		Summary string `json:"summary"`
		Detail  string `json:"detail"`
	}{
		Summary: summary,
		Detail:  detail,
	})

	chunk := TypedEventChunk{
		Type:      ChunkTypeSpanComplete,
		SpanID:    spanID,
		Data:      completeData,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// FailSpan marks a span as failed and emits a SpanComplete event with error info.
func (ms *MultiplexedStream) FailSpan(spanID string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	// Update span in the map
	if val, ok := ms.spans.Load(spanID); ok {
		span := val.(*Span)
		span.Status = SpanStatusFailed
		span.Detail = errMsg
		now := time.Now()
		span.EndTime = &now
	}

	// Update in the tree
	if ms.tree != nil {
		ms.tree.FailSpan(spanID, errMsg)
	}

	// Emit span_complete event with failed status
	failData, _ := json.Marshal(struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}{
		Error:  errMsg,
		Status: "failed",
	})

	chunk := TypedEventChunk{
		Type:      ChunkTypeSpanComplete,
		SpanID:    spanID,
		Data:      failData,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// EmitProgress emits a span progress event.
func (ms *MultiplexedStream) EmitProgress(spanID string, progress json.RawMessage) {
	chunk := TypedEventChunk{
		Type:      ChunkTypeSpanProgress,
		SpanID:    spanID,
		Data:      progress,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// EmitStatus emits a status change event.
func (ms *MultiplexedStream) EmitStatus(status string) {
	data, _ := json.Marshal(status)
	chunk := TypedEventChunk{
		Type:      ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// EmitSummary emits a summary update event for a span.
func (ms *MultiplexedStream) EmitSummary(spanID string, summary string) {
	data, _ := json.Marshal(struct {
		Summary string `json:"summary"`
	}{
		Summary: summary,
	})
	chunk := TypedEventChunk{
		Type:      ChunkTypeSummary,
		SpanID:    spanID,
		Data:      data,
		Timestamp: time.Now(),
	}
	ms.emit(chunk)
}

// Close closes the chunks channel. No more events can be emitted after this.
func (ms *MultiplexedStream) Close() {
	close(ms.chunks)
}

// emit sends a chunk to the channel (non-blocking if full, drops oldest).
func (ms *MultiplexedStream) emit(chunk TypedEventChunk) {
	select {
	case ms.chunks <- chunk:
	default:
		// Channel full — drop oldest and retry to avoid blocking the main loop
		select {
		case <-ms.chunks:
		default:
		}
		select {
		case ms.chunks <- chunk:
		default:
		}
	}
}

// generateSpanID creates a unique span ID using a monotonic counter.
func (ms *MultiplexedStream) generateSpanID() string {
	seq := ms.seq.Add(1)
	return fmt.Sprintf("span-%d-%d", time.Now().UnixNano(), seq)
}
