package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/server"
)

// ---------------------------------------------------------------------------
// gRPC Compatibility Integration Tests
// Validates: Requirements 6.3
// ---------------------------------------------------------------------------

// mockHostStream implements server.HostAgentStream for testing.
type mockHostStream struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	incoming chan *server.HostMessage
	outgoing []*server.HostMessage
}

func newMockHostStream() *mockHostStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockHostStream{
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan *server.HostMessage, 32),
		outgoing: make([]*server.HostMessage, 0),
	}
}

func (s *mockHostStream) Send(msg *server.HostMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outgoing = append(s.outgoing, msg)
	return nil
}

func (s *mockHostStream) Recv() (*server.HostMessage, error) {
	select {
	case msg, ok := <-s.incoming:
		if !ok {
			return nil, errors.New("stream closed")
		}
		return msg, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *mockHostStream) Context() context.Context {
	return s.ctx
}

func (s *mockHostStream) enqueue(msg *server.HostMessage) {
	s.incoming <- msg
}

func (s *mockHostStream) closeIncoming() {
	close(s.incoming)
}

func (s *mockHostStream) getOutgoing() []*server.HostMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*server.HostMessage, len(s.outgoing))
	copy(out, s.outgoing)
	return out
}

// TestGRPCRegisterHandshake verifies the register → ack handshake protocol.
func TestGRPCRegisterHandshake(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Send register message
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-alpha",
		Time: time.Now().UnixMilli(),
	})

	// Send a heartbeat then close
	go func() {
		time.Sleep(50 * time.Millisecond)
		stream.enqueue(&server.HostMessage{
			Type: server.HostMsgHeartbeat,
			ID:   "hb-1",
			Time: time.Now().UnixMilli(),
		})
		time.Sleep(50 * time.Millisecond)
		stream.cancel()
	}()

	// HandleStream blocks until context is cancelled
	_ = grpcSrv.HandleStream(stream)

	// Verify ack was sent for register
	outgoing := stream.getOutgoing()
	if len(outgoing) < 1 {
		t.Fatalf("expected at least 1 outgoing message, got %d", len(outgoing))
	}

	// First response should be ack for register
	ack := outgoing[0]
	if ack.Type != server.HostMsgAck {
		t.Errorf("first response type = %q, want %q", ack.Type, server.HostMsgAck)
	}
	if ack.ID != "host-alpha" {
		t.Errorf("ack ID = %q, want %q", ack.ID, "host-alpha")
	}
	if ack.Time == 0 {
		t.Error("ack should have non-zero time")
	}
}

// TestGRPCHeartbeatProtocol verifies heartbeat → ack response.
func TestGRPCHeartbeatProtocol(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Register first
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-beta",
		Time: time.Now().UnixMilli(),
	})

	// Send multiple heartbeats
	go func() {
		time.Sleep(30 * time.Millisecond)
		for i := 0; i < 3; i++ {
			stream.enqueue(&server.HostMessage{
				Type: server.HostMsgHeartbeat,
				ID:   "hb-" + string(rune('1'+i)),
				Time: time.Now().UnixMilli(),
			})
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		stream.cancel()
	}()

	_ = grpcSrv.HandleStream(stream)

	outgoing := stream.getOutgoing()
	// Should have: 1 register ack + 3 heartbeat acks = 4
	if len(outgoing) < 4 {
		t.Fatalf("expected at least 4 outgoing messages, got %d", len(outgoing))
	}

	// All heartbeat responses should be ack type
	for i := 1; i < len(outgoing); i++ {
		if outgoing[i].Type != server.HostMsgAck {
			t.Errorf("outgoing[%d].type = %q, want %q", i, outgoing[i].Type, server.HostMsgAck)
		}
	}
}

// TestGRPCUnknownMessageType verifies error response for unknown message types.
func TestGRPCUnknownMessageType(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Register
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-gamma",
		Time: time.Now().UnixMilli(),
	})

	// Send unknown type
	go func() {
		time.Sleep(30 * time.Millisecond)
		stream.enqueue(&server.HostMessage{
			Type: "unknown_type",
			ID:   "msg-1",
			Time: time.Now().UnixMilli(),
		})
		time.Sleep(30 * time.Millisecond)
		stream.cancel()
	}()

	_ = grpcSrv.HandleStream(stream)

	outgoing := stream.getOutgoing()
	// Should have register ack + error response
	if len(outgoing) < 2 {
		t.Fatalf("expected at least 2 outgoing messages, got %d", len(outgoing))
	}

	// Find the error message
	var foundError bool
	for _, msg := range outgoing {
		if msg.Type == server.HostMsgError {
			foundError = true
			if msg.Error == "" {
				t.Error("error message should have non-empty Error field")
			}
			if msg.Time == 0 {
				t.Error("error message should have non-zero time")
			}
		}
	}
	if !foundError {
		t.Error("expected an error response for unknown message type")
	}
}

// TestGRPCRegisterMissingID verifies error when register has no host ID.
func TestGRPCRegisterMissingID(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Register without ID
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "",
		Time: time.Now().UnixMilli(),
	})

	err := grpcSrv.HandleStream(stream)
	if err == nil {
		t.Error("expected error for register without host ID")
	}
}

// TestGRPCFirstMessageMustBeRegister verifies that non-register first message is rejected.
func TestGRPCFirstMessageMustBeRegister(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Send heartbeat as first message (should fail)
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgHeartbeat,
		ID:   "hb-1",
		Time: time.Now().UnixMilli(),
	})

	err := grpcSrv.HandleStream(stream)
	if err == nil {
		t.Error("expected error when first message is not register")
	}
}

// TestGRPCHostConnectionTracking verifies host connection state management.
func TestGRPCHostConnectionTracking(t *testing.T) {
	grpcSrv := server.NewGRPCServer()

	// Initially no hosts connected
	if len(grpcSrv.ConnectedHosts()) != 0 {
		t.Errorf("expected 0 connected hosts, got %d", len(grpcSrv.ConnectedHosts()))
	}

	stream := newMockHostStream()

	// Register
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-delta",
		Time: time.Now().UnixMilli(),
	})

	done := make(chan struct{})
	go func() {
		_ = grpcSrv.HandleStream(stream)
		close(done)
	}()

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Host should be connected
	if !grpcSrv.IsHostConnected("host-delta") {
		t.Error("expected host-delta to be connected")
	}

	hosts := grpcSrv.ConnectedHosts()
	if len(hosts) != 1 || hosts[0] != "host-delta" {
		t.Errorf("connected hosts = %v, want [host-delta]", hosts)
	}

	// Disconnect
	stream.cancel()
	<-done

	// Host should be disconnected
	time.Sleep(20 * time.Millisecond)
	if grpcSrv.IsHostConnected("host-delta") {
		t.Error("expected host-delta to be disconnected after stream close")
	}
}

// TestGRPCSendToHost verifies sending commands to a connected host.
func TestGRPCSendToHost(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Register
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-epsilon",
		Time: time.Now().UnixMilli(),
	})

	done := make(chan struct{})
	go func() {
		_ = grpcSrv.HandleStream(stream)
		close(done)
	}()

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Send command to host
	payload, _ := json.Marshal(map[string]string{"cmd": "df -h"})
	err := grpcSrv.SendToHost("host-epsilon", &server.HostMessage{
		Type:    server.HostMsgExec,
		ID:      "cmd-001",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("SendToHost failed: %v", err)
	}

	// Verify message was sent
	time.Sleep(20 * time.Millisecond)
	outgoing := stream.getOutgoing()

	// Find the exec message (after the register ack)
	var foundExec bool
	for _, msg := range outgoing {
		if msg.Type == server.HostMsgExec {
			foundExec = true
			if msg.ID != "cmd-001" {
				t.Errorf("exec msg ID = %q, want %q", msg.ID, "cmd-001")
			}
			if msg.Time == 0 {
				t.Error("exec msg should have non-zero time")
			}
		}
	}
	if !foundExec {
		t.Error("expected exec message to be sent to host")
	}

	// Cleanup
	stream.cancel()
	<-done
}

// TestGRPCSendToDisconnectedHost verifies error when sending to non-existent host.
func TestGRPCSendToDisconnectedHost(t *testing.T) {
	grpcSrv := server.NewGRPCServer()

	err := grpcSrv.SendToHost("non-existent-host", &server.HostMessage{
		Type: server.HostMsgExec,
		ID:   "cmd-001",
	})
	if err == nil {
		t.Error("expected error when sending to disconnected host")
	}
}

// TestGRPCHostMessageWireFormat verifies the HostMessage JSON wire format.
func TestGRPCHostMessageWireFormat(t *testing.T) {
	// Verify HostMessage serializes to the expected wire format
	msg := &server.HostMessage{
		Type:    server.HostMsgExec,
		ID:      "cmd-123",
		Payload: json.RawMessage(`{"command":"ls -la","cwd":"/tmp"}`),
		Time:    1700000000000,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify round-trip
	var decoded server.HostMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Type != server.HostMsgExec {
		t.Errorf("type = %q, want %q", decoded.Type, server.HostMsgExec)
	}
	if decoded.ID != "cmd-123" {
		t.Errorf("id = %q, want %q", decoded.ID, "cmd-123")
	}
	if decoded.Time != 1700000000000 {
		t.Errorf("time = %d, want %d", decoded.Time, 1700000000000)
	}

	// Verify payload preserved
	var payload map[string]string
	if err := json.Unmarshal(decoded.Payload, &payload); err != nil {
		t.Fatalf("payload unmarshal failed: %v", err)
	}
	if payload["command"] != "ls -la" {
		t.Errorf("payload.command = %q, want %q", payload["command"], "ls -la")
	}
}

// TestGRPCMessageTypeConstants verifies all protocol message type constants.
func TestGRPCMessageTypeConstants(t *testing.T) {
	// Verify all expected message types are defined (protocol compatibility)
	expectedTypes := map[string]string{
		"register":  server.HostMsgRegister,
		"heartbeat": server.HostMsgHeartbeat,
		"terminal":  server.HostMsgTerminal,
		"exec":      server.HostMsgExec,
		"file":      server.HostMsgFile,
		"ack":       server.HostMsgAck,
		"error":     server.HostMsgError,
	}

	for expected, actual := range expectedTypes {
		if actual != expected {
			t.Errorf("HostMsg constant for %q = %q, want %q", expected, actual, expected)
		}
	}
}

// TestGRPCToolResponseRouting verifies that terminal/exec/file responses from
// Host Agent are handled without error.
func TestGRPCToolResponseRouting(t *testing.T) {
	grpcSrv := server.NewGRPCServer()
	stream := newMockHostStream()

	// Register
	stream.enqueue(&server.HostMessage{
		Type: server.HostMsgRegister,
		ID:   "host-zeta",
		Time: time.Now().UnixMilli(),
	})

	// Send tool responses (terminal, exec, file)
	go func() {
		time.Sleep(30 * time.Millisecond)

		// Terminal response
		stream.enqueue(&server.HostMessage{
			Type:    server.HostMsgTerminal,
			ID:      "term-001",
			Payload: json.RawMessage(`{"output":"total 4.0K\ndrwxr-xr-x 2 root root 4096"}`),
			Time:    time.Now().UnixMilli(),
		})

		// Exec response
		stream.enqueue(&server.HostMessage{
			Type:    server.HostMsgExec,
			ID:      "exec-001",
			Payload: json.RawMessage(`{"exitCode":0,"stdout":"OK"}`),
			Time:    time.Now().UnixMilli(),
		})

		// File response
		stream.enqueue(&server.HostMessage{
			Type:    server.HostMsgFile,
			ID:      "file-001",
			Payload: json.RawMessage(`{"path":"/etc/hosts","content":"127.0.0.1 localhost"}`),
			Time:    time.Now().UnixMilli(),
		})

		time.Sleep(30 * time.Millisecond)
		stream.cancel()
	}()

	// HandleStream should process all messages without error (context cancelled is expected)
	err := grpcSrv.HandleStream(stream)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected error: %v", err)
	}

	// No error responses should have been sent for valid message types
	outgoing := stream.getOutgoing()
	for _, msg := range outgoing {
		if msg.Type == server.HostMsgError {
			t.Errorf("unexpected error response: %s", msg.Error)
		}
	}
}
