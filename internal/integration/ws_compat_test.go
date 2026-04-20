package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/server"
	"aiops-v2/internal/spanstream"
)

// ---------------------------------------------------------------------------
// WebSocket Compatibility Integration Tests
// Validates: Requirements 6.2
// ---------------------------------------------------------------------------

// mockWSConn implements server.WebSocketConn for testing.
type mockWSConn struct {
	mu       sync.Mutex
	messages []interface{}
	closed   bool
}

func newMockWSConn() *mockWSConn {
	return &mockWSConn{
		messages: make([]interface{}, 0),
	}
}

func (c *mockWSConn) WriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Deep copy via JSON round-trip to avoid data races
	data, _ := json.Marshal(v)
	var copy interface{}
	_ = json.Unmarshal(data, &copy)
	c.messages = append(c.messages, copy)
	return nil
}

func (c *mockWSConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *mockWSConn) getMessages() []interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]interface{}, len(c.messages))
	copy(out, c.messages)
	return out
}

func (c *mockWSConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// TestWebSocketPusherBroadcastTypedChunk verifies that TypedEventChunk is pushed
// to connected clients in the correct format.
func TestWebSocketPusherBroadcastTypedChunk(t *testing.T) {
	pusher := server.NewWebSocketPusher(false)

	conn := newMockWSConn()
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

	// Verify the message structure matches TypedEventChunk format
	msgMap, ok := msgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", msgs[0])
	}

	if msgMap["type"] != string(spanstream.ChunkTypeText) {
		t.Errorf("type = %v, want %q", msgMap["type"], spanstream.ChunkTypeText)
	}
	if msgMap["data"] != "hello world" {
		t.Errorf("data = %v, want %q", msgMap["data"], "hello world")
	}
	if _, ok := msgMap["timestamp"]; !ok {
		t.Error("expected 'timestamp' field in message")
	}
}

// TestWebSocketPusherLegacyFormat verifies legacy message format for backward compatibility.
func TestWebSocketPusherLegacyFormat(t *testing.T) {
	pusher := server.NewWebSocketPusher(true)

	conn := newMockWSConn()
	pusher.AddClient("legacy-client", conn, true)

	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeText,
		Data:      json.RawMessage(`"streaming text"`),
		Timestamp: time.Now(),
	}

	pusher.PushChunk(chunk)

	msgs := conn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Legacy format should have type, payload, time fields
	msgMap, ok := msgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", msgs[0])
	}

	if msgMap["type"] != "message" {
		t.Errorf("legacy type = %v, want %q", msgMap["type"], "message")
	}
	if _, ok := msgMap["payload"]; !ok {
		t.Error("expected 'payload' field in legacy message")
	}
	if _, ok := msgMap["time"]; !ok {
		t.Error("expected 'time' field in legacy message")
	}
}

// TestWebSocketPusherSpanEvents verifies span lifecycle events are pushed correctly.
func TestWebSocketPusherSpanEvents(t *testing.T) {
	pusher := server.NewWebSocketPusher(true)

	legacyConn := newMockWSConn()
	modernConn := newMockWSConn()
	pusher.AddClient("legacy", legacyConn, true)
	pusher.AddClient("modern", modernConn, false)

	// Emit span_start
	startData, _ := json.Marshal(struct {
		SpanType string `json:"spanType"`
		Name     string `json:"name"`
	}{
		SpanType: "tool_call",
		Name:     "host.disk_usage",
	})

	startChunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeSpanStart,
		SpanID:    "span-001",
		ParentID:  "turn-001",
		Data:      startData,
		Timestamp: time.Now(),
	}
	pusher.PushChunk(startChunk)

	// Emit span_complete
	completeData, _ := json.Marshal(struct {
		Summary string `json:"summary"`
		Detail  string `json:"detail"`
	}{
		Summary: "disk usage checked",
		Detail:  "/dev/sda1: 80% used",
	})

	completeChunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeSpanComplete,
		SpanID:    "span-001",
		Data:      completeData,
		Timestamp: time.Now(),
	}
	pusher.PushChunk(completeChunk)

	// Verify modern client receives TypedEventChunk format
	modernMsgs := modernConn.getMessages()
	if len(modernMsgs) != 2 {
		t.Fatalf("modern client: expected 2 messages, got %d", len(modernMsgs))
	}

	msg0 := modernMsgs[0].(map[string]interface{})
	if msg0["type"] != string(spanstream.ChunkTypeSpanStart) {
		t.Errorf("modern msg[0].type = %v, want %q", msg0["type"], spanstream.ChunkTypeSpanStart)
	}
	if msg0["spanId"] != "span-001" {
		t.Errorf("modern msg[0].spanId = %v, want %q", msg0["spanId"], "span-001")
	}

	msg1 := modernMsgs[1].(map[string]interface{})
	if msg1["type"] != string(spanstream.ChunkTypeSpanComplete) {
		t.Errorf("modern msg[1].type = %v, want %q", msg1["type"], spanstream.ChunkTypeSpanComplete)
	}

	// Verify legacy client receives legacy format
	legacyMsgs := legacyConn.getMessages()
	if len(legacyMsgs) != 2 {
		t.Fatalf("legacy client: expected 2 messages, got %d", len(legacyMsgs))
	}

	legMsg0 := legacyMsgs[0].(map[string]interface{})
	if legMsg0["type"] != "tool_start" {
		t.Errorf("legacy msg[0].type = %v, want %q", legMsg0["type"], "tool_start")
	}
	if _, ok := legMsg0["time"]; !ok {
		t.Error("legacy msg[0] missing 'time' field")
	}

	legMsg1 := legacyMsgs[1].(map[string]interface{})
	if legMsg1["type"] != "tool_complete" {
		t.Errorf("legacy msg[1].type = %v, want %q", legMsg1["type"], "tool_complete")
	}
}

// TestWebSocketPusherClientManagement verifies add/remove client lifecycle.
func TestWebSocketPusherClientManagement(t *testing.T) {
	pusher := server.NewWebSocketPusher(false)

	conn1 := newMockWSConn()
	conn2 := newMockWSConn()

	pusher.AddClient("c1", conn1, false)
	pusher.AddClient("c2", conn2, false)

	if pusher.ClientCount() != 2 {
		t.Errorf("client count = %d, want 2", pusher.ClientCount())
	}

	// Remove one client
	pusher.RemoveClient("c1")

	if pusher.ClientCount() != 1 {
		t.Errorf("client count = %d, want 1", pusher.ClientCount())
	}

	if !conn1.isClosed() {
		t.Error("expected conn1 to be closed after removal")
	}

	// Push should only go to remaining client
	chunk := spanstream.TypedEventChunk{
		Type:      spanstream.ChunkTypeStatus,
		Data:      json.RawMessage(`"idle"`),
		Timestamp: time.Now(),
	}
	pusher.PushChunk(chunk)

	if len(conn1.getMessages()) != 0 {
		t.Error("removed client should not receive messages")
	}
	if len(conn2.getMessages()) != 1 {
		t.Errorf("remaining client should receive 1 message, got %d", len(conn2.getMessages()))
	}
}

// TestWebSocketPusherStartStreaming verifies streaming from MultiplexedStream.
func TestWebSocketPusherStartStreaming(t *testing.T) {
	pusher := server.NewWebSocketPusher(false)

	conn := newMockWSConn()
	pusher.AddClient("stream-client", conn, false)

	tree := spanstream.NewSpanTree(&spanstream.Span{
		ID:        "root",
		Type:      spanstream.SpanTypeTurn,
		Status:    spanstream.SpanStatusRunning,
		Name:      "test turn",
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

	// Emit events
	stream.EmitText("hello")
	stream.EmitStatus("processing")

	// Give time for events to propagate
	time.Sleep(50 * time.Millisecond)

	// Stop streaming
	cancel()
	<-done

	msgs := conn.getMessages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// First message should be text type
	msg0 := msgs[0].(map[string]interface{})
	if msg0["type"] != string(spanstream.ChunkTypeText) {
		t.Errorf("msg[0].type = %v, want %q", msg0["type"], spanstream.ChunkTypeText)
	}

	// Second message should be status type
	msg1 := msgs[1].(map[string]interface{})
	if msg1["type"] != string(spanstream.ChunkTypeStatus) {
		t.Errorf("msg[1].type = %v, want %q", msg1["type"], spanstream.ChunkTypeStatus)
	}
}

// TestWebSocketPusherLegacyMessage verifies PushLegacyMessage for non-chunk messages.
func TestWebSocketPusherLegacyMessage(t *testing.T) {
	pusher := server.NewWebSocketPusher(true)

	conn := newMockWSConn()
	pusher.AddClient("legacy-direct", conn, true)

	payload, _ := json.Marshal(map[string]string{"action": "refresh"})
	pusher.PushLegacyMessage("notification", payload)

	msgs := conn.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msgMap := msgs[0].(map[string]interface{})
	if msgMap["type"] != "notification" {
		t.Errorf("type = %v, want %q", msgMap["type"], "notification")
	}
	if _, ok := msgMap["time"]; !ok {
		t.Error("expected 'time' field in legacy message")
	}

	payloadMap, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected payload to be map, got %T", msgMap["payload"])
	}
	if payloadMap["action"] != "refresh" {
		t.Errorf("payload.action = %v, want %q", payloadMap["action"], "refresh")
	}
}
