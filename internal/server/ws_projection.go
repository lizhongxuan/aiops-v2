package server

import (
	"encoding/json"
	"time"

	"aiops-v2/internal/projection"
	"aiops-v2/internal/spanstream"
)

// ---------------------------------------------------------------------------
// ProjectionSubscriber adapts the Projection layer's Subscriber interface to
// push events through the WebSocketPusher. This keeps the existing WebSocket
// protocol unchanged while routing Projection events to connected clients.
// (Requirements: 6.2)
// ---------------------------------------------------------------------------

// ProjectionSubscriber implements projection.Subscriber and pushes projected
// data to WebSocket clients via the WebSocketPusher.
type ProjectionSubscriber struct {
	pusher *WebSocketPusher
}

// NewProjectionSubscriber creates a ProjectionSubscriber wired to the given pusher.
func NewProjectionSubscriber(pusher *WebSocketPusher) *ProjectionSubscriber {
	return &ProjectionSubscriber{pusher: pusher}
}

// OnToolInvocation pushes tool invocation events as TypedEventChunk via WebSocket.
func (ps *ProjectionSubscriber) OnToolInvocation(inv projection.ToolInvocation) {
	data, _ := json.Marshal(inv)

	// Map tool invocation status to span chunk type
	var chunkType spanstream.EventChunkType
	switch inv.Status {
	case projection.ToolInvocationStarted:
		chunkType = spanstream.ChunkTypeSpanStart
	case projection.ToolInvocationProgress:
		chunkType = spanstream.ChunkTypeSpanProgress
	case projection.ToolInvocationCompleted, projection.ToolInvocationFailed:
		chunkType = spanstream.ChunkTypeSpanComplete
	default:
		chunkType = spanstream.ChunkTypeStatus
	}

	chunk := spanstream.TypedEventChunk{
		Type:      chunkType,
		SpanID:    inv.ID,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}

// OnActivity pushes activity stats updates via WebSocket.
func (ps *ProjectionSubscriber) OnActivity(activity projection.ActivityStats) {
	data, _ := json.Marshal(activity)
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}

// OnCard pushes card generation events via WebSocket.
func (ps *ProjectionSubscriber) OnCard(card projection.Card) {
	data, _ := json.Marshal(card)
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}

// OnApproval pushes approval events via WebSocket.
func (ps *ProjectionSubscriber) OnApproval(approval projection.Approval) {
	data, _ := json.Marshal(approval)
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}

// OnEvidence pushes evidence events via WebSocket.
func (ps *ProjectionSubscriber) OnEvidence(evidence projection.Evidence) {
	data, _ := json.Marshal(evidence)
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}

// OnSnapshot pushes snapshot events via WebSocket.
func (ps *ProjectionSubscriber) OnSnapshot(snapshot projection.Snapshot) {
	data, _ := json.Marshal(snapshot)
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      data,
		Timestamp: time.Now(),
	}
	ps.pusher.PushChunk(chunk)
}
