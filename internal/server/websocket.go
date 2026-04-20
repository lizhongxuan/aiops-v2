// Package server provides HTTP/WebSocket/gRPC API compatibility layer.
package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"aiops-v2/internal/spanstream"
)

// WebSocketConn abstracts a WebSocket connection for testability.
type WebSocketConn interface {
	// WriteJSON writes a JSON-encoded message to the connection.
	WriteJSON(v interface{}) error
	// Close closes the connection.
	Close() error
}

// LegacyMessage represents the legacy WebSocket message format for backward compatibility.
type LegacyMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Time    int64           `json:"time"` // Unix milliseconds
}

// WebSocketPusher consumes TypedEventChunk from a MultiplexedStream and pushes
// events to connected WebSocket clients. It supports both the new TypedEventChunk
// format and a legacy message format for backward compatibility.
type WebSocketPusher struct {
	mu      sync.RWMutex
	clients map[string]*wsClient
	legacy  bool // if true, also send legacy format as fallback
}

// wsClient tracks a single WebSocket client connection and its preferences.
type wsClient struct {
	id         string
	conn       WebSocketConn
	useLegacy  bool // client prefers legacy format
	done       chan struct{}
}

// NewWebSocketPusher creates a new WebSocketPusher.
// If legacy is true, clients that haven't opted into the new format will receive
// legacy-formatted messages.
func NewWebSocketPusher(legacy bool) *WebSocketPusher {
	return &WebSocketPusher{
		clients: make(map[string]*wsClient),
		legacy:  legacy,
	}
}

// AddClient registers a WebSocket client connection.
// useLegacy indicates whether this client expects legacy message format.
func (p *WebSocketPusher) AddClient(id string, conn WebSocketConn, useLegacy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[id] = &wsClient{
		id:        id,
		conn:      conn,
		useLegacy: useLegacy,
		done:      make(chan struct{}),
	}
}

// RemoveClient unregisters and closes a WebSocket client connection.
func (p *WebSocketPusher) RemoveClient(id string) {
	p.mu.Lock()
	client, ok := p.clients[id]
	if ok {
		delete(p.clients, id)
	}
	p.mu.Unlock()

	if ok {
		close(client.done)
		_ = client.conn.Close()
	}
}


// ClientCount returns the number of connected clients.
func (p *WebSocketPusher) ClientCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients)
}

// StartStreaming begins consuming events from the MultiplexedStream and pushing
// them to all connected clients. It blocks until the context is cancelled or
// the stream's channel is closed.
func (p *WebSocketPusher) StartStreaming(ctx context.Context, stream *spanstream.MultiplexedStream) {
	chunks := stream.Chunks()
	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			p.broadcast(chunk)
		}
	}
}

// PushChunk pushes a single TypedEventChunk to all connected clients.
// This can be used for direct pushing without StartStreaming.
func (p *WebSocketPusher) PushChunk(chunk spanstream.TypedEventChunk) {
	p.broadcast(chunk)
}

// broadcast sends a chunk to all connected clients, converting to legacy format
// for clients that require it.
func (p *WebSocketPusher) broadcast(chunk spanstream.TypedEventChunk) {
	p.mu.RLock()
	clients := make([]*wsClient, 0, len(p.clients))
	for _, c := range p.clients {
		clients = append(clients, c)
	}
	p.mu.RUnlock()

	var legacyMsg *LegacyMessage
	if p.legacy {
		legacyMsg = convertToLegacy(chunk)
	}

	var failed []string
	for _, client := range clients {
		var err error
		if client.useLegacy && legacyMsg != nil {
			err = client.conn.WriteJSON(legacyMsg)
		} else {
			err = client.conn.WriteJSON(chunk)
		}
		if err != nil {
			failed = append(failed, client.id)
		}
	}

	// Remove failed clients
	if len(failed) > 0 {
		for _, id := range failed {
			p.RemoveClient(id)
		}
	}
}

// convertToLegacy converts a TypedEventChunk to the legacy WebSocket message format.
// Legacy format: {"type": "<legacy_type>", "payload": <data>, "time": <unix_ms>}
func convertToLegacy(chunk spanstream.TypedEventChunk) *LegacyMessage {
	msg := &LegacyMessage{
		Time: chunk.Timestamp.UnixMilli(),
	}

	switch chunk.Type {
	case spanstream.ChunkTypeText:
		msg.Type = "message"
		msg.Payload = chunk.Data

	case spanstream.ChunkTypeSpanStart:
		msg.Type = "tool_start"
		// Wrap with spanId for legacy clients
		payload := wrapWithSpanID(chunk.SpanID, chunk.Data)
		msg.Payload = payload

	case spanstream.ChunkTypeSpanProgress:
		msg.Type = "tool_progress"
		payload := wrapWithSpanID(chunk.SpanID, chunk.Data)
		msg.Payload = payload

	case spanstream.ChunkTypeSpanComplete:
		msg.Type = "tool_complete"
		payload := wrapWithSpanID(chunk.SpanID, chunk.Data)
		msg.Payload = payload

	case spanstream.ChunkTypeStatus:
		msg.Type = "status"
		msg.Payload = chunk.Data

	case spanstream.ChunkTypeSummary:
		msg.Type = "summary"
		payload := wrapWithSpanID(chunk.SpanID, chunk.Data)
		msg.Payload = payload

	default:
		// Unknown chunk type — pass through as-is
		msg.Type = string(chunk.Type)
		msg.Payload = chunk.Data
	}

	return msg
}

// wrapWithSpanID wraps data with a spanId field for legacy format.
func wrapWithSpanID(spanID string, data json.RawMessage) json.RawMessage {
	wrapper := struct {
		SpanID string          `json:"spanId"`
		Data   json.RawMessage `json:"data"`
	}{
		SpanID: spanID,
		Data:   data,
	}
	result, _ := json.Marshal(wrapper)
	return result
}

// PushLegacyMessage sends a raw legacy message to all legacy clients.
// This is useful for sending messages that don't originate from TypedEventChunk.
func (p *WebSocketPusher) PushLegacyMessage(msgType string, payload json.RawMessage) {
	msg := &LegacyMessage{
		Type:    msgType,
		Payload: payload,
		Time:    time.Now().UnixMilli(),
	}

	p.mu.RLock()
	clients := make([]*wsClient, 0, len(p.clients))
	for _, c := range p.clients {
		clients = append(clients, c)
	}
	p.mu.RUnlock()

	var failed []string
	for _, client := range clients {
		if err := client.conn.WriteJSON(msg); err != nil {
			failed = append(failed, client.id)
		}
	}

	if len(failed) > 0 {
		for _, id := range failed {
			p.RemoveClient(id)
		}
	}
}
