package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/spanstream"
)

// mockWSConn implements WebSocketConn for testing.
type mockWSConn struct {
	mu       sync.Mutex
	messages []interface{}
	closed   bool
	failNext bool
}

func newMockConn() *mockWSConn {
	return &mockWSConn{}
}

func (m *mockWSConn) WriteJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		return errors.New("write failed")
	}
	// Deep copy via JSON round-trip to avoid data races
	data, _ := json.Marshal(v)
	var copy interface{}
	_ = json.Unmarshal(data, &copy)
	m.messages = append(m.messages, copy)
	return nil
}

func (m *mockWSConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockWSConn) getMessages() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]interface{}, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockWSConn) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func TestWebSocketPusher_AddRemoveClient(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, false)

	if pusher.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", pusher.ClientCount())
	}

	pusher.RemoveClient("client-1")
	if pusher.ClientCount() != 0 {
		t.Fatalf("expected 0 clients, got %d", pusher.ClientCount())
	}
	if !conn.isClosed() {
		t.Fatal("expected connection to be closed")
	}
}

func TestWebSocketPusher_PushChunk_NewFormat(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, false)

	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeText,
		Data:      json.RawMessage(`"hello world"`),
		Timestamp: time.Now(),
	}

	pusher.PushChunk(chunk)

	msgs := conn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Verify it's the TypedEventChunk format (has "type" field with value "text")
	msgMap, ok := msgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", msgs[0])
	}
	if msgMap["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", msgMap["type"])
	}
}


func TestWebSocketPusher_PushChunk_LegacyFormat(t *testing.T) {
	pusher := NewWebSocketPusher(true)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, true) // legacy client

	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeText,
		Data:      json.RawMessage(`"hello legacy"`),
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	pusher.PushChunk(chunk)

	msgs := conn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msgMap, ok := msgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", msgs[0])
	}

	// Legacy format should have "type" = "message" for text chunks
	if msgMap["type"] != "message" {
		t.Fatalf("expected legacy type 'message', got %v", msgMap["type"])
	}
	// Should have "payload" field
	if msgMap["payload"] == nil {
		t.Fatal("expected payload field in legacy message")
	}
	// Should have "time" field
	if msgMap["time"] == nil {
		t.Fatal("expected time field in legacy message")
	}
}

func TestWebSocketPusher_PushChunk_MixedClients(t *testing.T) {
	pusher := NewWebSocketPusher(true)

	newConn := newMockConn()
	legacyConn := newMockConn()
	pusher.AddClient("new-client", newConn, false)
	pusher.AddClient("legacy-client", legacyConn, true)

	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeSpanStart,
		SpanID:    "span-123",
		ParentID:  "span-root",
		Data:      json.RawMessage(`{"spanType":"tool_call","name":"disk_usage"}`),
		Timestamp: time.Now(),
	}

	pusher.PushChunk(chunk)

	// New client gets TypedEventChunk format
	newMsgs := newConn.getMessages()
	if len(newMsgs) != 1 {
		t.Fatalf("expected 1 message for new client, got %d", len(newMsgs))
	}
	newMsg := newMsgs[0].(map[string]interface{})
	if newMsg["type"] != "span_start" {
		t.Fatalf("new client: expected type 'span_start', got %v", newMsg["type"])
	}
	if newMsg["spanId"] != "span-123" {
		t.Fatalf("new client: expected spanId 'span-123', got %v", newMsg["spanId"])
	}

	// Legacy client gets legacy format
	legacyMsgs := legacyConn.getMessages()
	if len(legacyMsgs) != 1 {
		t.Fatalf("expected 1 message for legacy client, got %d", len(legacyMsgs))
	}
	legacyMsg := legacyMsgs[0].(map[string]interface{})
	if legacyMsg["type"] != "tool_start" {
		t.Fatalf("legacy client: expected type 'tool_start', got %v", legacyMsg["type"])
	}
}

func TestWebSocketPusher_FailedClientRemoved(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	goodConn := newMockConn()
	badConn := newMockConn()
	badConn.failNext = true

	pusher.AddClient("good", goodConn, false)
	pusher.AddClient("bad", badConn, false)

	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      json.RawMessage(`"thinking"`),
		Timestamp: time.Now(),
	}

	pusher.PushChunk(chunk)

	// Bad client should be removed
	if pusher.ClientCount() != 1 {
		t.Fatalf("expected 1 client remaining, got %d", pusher.ClientCount())
	}
	if !badConn.isClosed() {
		t.Fatal("expected bad connection to be closed")
	}

	// Good client should have received the message
	msgs := goodConn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for good client, got %d", len(msgs))
	}
}

func TestWebSocketPusher_StartStreaming(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, false)

	tree := spanstream.NewSpanTree(&spanstream.Span{
		ID:        "root",
		Type:      spanstream.SpanTypeTurn,
		Status:    spanstream.SpanStatusRunning,
		StartTime: time.Now(),
	})
	stream := spanstream.NewMultiplexedStream(tree, 16)

	ctx, cancel := context.WithCancel(context.Background())

	// Start streaming in background
	done := make(chan struct{})
	go func() {
		pusher.StartStreaming(ctx, stream)
		close(done)
	}()

	// Emit some events
	stream.EmitText("hello")
	stream.EmitStatus("thinking")

	// Give time for events to be consumed
	time.Sleep(50 * time.Millisecond)

	// Cancel and wait
	cancel()
	<-done

	msgs := conn.getMessages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}
}

func TestWebSocketPusher_StartStreaming_StreamClose(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, false)

	tree := spanstream.NewSpanTree(&spanstream.Span{
		ID:        "root",
		Type:      spanstream.SpanTypeTurn,
		Status:    spanstream.SpanStatusRunning,
		StartTime: time.Now(),
	})
	stream := spanstream.NewMultiplexedStream(tree, 16)

	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		pusher.StartStreaming(ctx, stream)
		close(done)
	}()

	stream.EmitText("before close")
	time.Sleep(20 * time.Millisecond)

	// Close the stream — StartStreaming should return
	stream.Close()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("StartStreaming did not return after stream close")
	}
}

func TestConvertToLegacy_AllChunkTypes(t *testing.T) {
	tests := []struct {
		name         string
		chunk        spanstream.TypedEventChunk
		expectedType string
	}{
		{
			name: "text → message",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeText,
				Data:      json.RawMessage(`"hello"`),
				Timestamp: time.Now(),
			},
			expectedType: "message",
		},
		{
			name: "span_start → tool_start",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeSpanStart,
				SpanID:    "s1",
				Data:      json.RawMessage(`{"name":"test"}`),
				Timestamp: time.Now(),
			},
			expectedType: "tool_start",
		},
		{
			name: "span_progress → tool_progress",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeSpanProgress,
				SpanID:    "s1",
				Data:      json.RawMessage(`{"percent":50}`),
				Timestamp: time.Now(),
			},
			expectedType: "tool_progress",
		},
		{
			name: "span_complete → tool_complete",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeSpanComplete,
				SpanID:    "s1",
				Data:      json.RawMessage(`{"summary":"done"}`),
				Timestamp: time.Now(),
			},
			expectedType: "tool_complete",
		},
		{
			name: "status → status",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeStatus,
				Data:      json.RawMessage(`"idle"`),
				Timestamp: time.Now(),
			},
			expectedType: "status",
		},
		{
			name: "summary → summary",
			chunk: spanstream.TypedEventChunk{
				Type:      spanstream.ChunkTypeSummary,
				SpanID:    "s1",
				Data:      json.RawMessage(`{"summary":"compressed"}`),
				Timestamp: time.Now(),
			},
			expectedType: "summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := convertToLegacy(tt.chunk)
			if legacy.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, legacy.Type)
			}
			if legacy.Time != tt.chunk.Timestamp.UnixMilli() {
				t.Errorf("expected time %d, got %d", tt.chunk.Timestamp.UnixMilli(), legacy.Time)
			}
			if legacy.Payload == nil {
				t.Error("expected non-nil payload")
			}
		})
	}
}

func TestConvertToLegacy_SpanIDWrapping(t *testing.T) {
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeSpanStart,
		SpanID:    "span-42",
		Data:      json.RawMessage(`{"spanType":"tool_call","name":"ls"}`),
		Timestamp: time.Now(),
	}

	legacy := convertToLegacy(chunk)

	var payload struct {
		SpanID string          `json:"spanId"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(legacy.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.SpanID != "span-42" {
		t.Errorf("expected spanId 'span-42', got %q", payload.SpanID)
	}
	if payload.Data == nil {
		t.Error("expected non-nil data in wrapped payload")
	}
}

func TestWebSocketPusher_PushLegacyMessage(t *testing.T) {
	pusher := NewWebSocketPusher(false)

	conn := newMockConn()
	pusher.AddClient("client-1", conn, false)

	payload, _ := json.Marshal(map[string]string{"status": "ok"})
	pusher.PushLegacyMessage("custom_event", payload)

	msgs := conn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msgMap := msgs[0].(map[string]interface{})
	if msgMap["type"] != "custom_event" {
		t.Fatalf("expected type 'custom_event', got %v", msgMap["type"])
	}
}

func TestWebSocketPusher_RemoveNonexistentClient(t *testing.T) {
	pusher := NewWebSocketPusher(false)
	// Should not panic
	pusher.RemoveClient("nonexistent")
	if pusher.ClientCount() != 0 {
		t.Fatal("expected 0 clients")
	}
}
