package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// gRPC 双向流适配 — 保持 Host Agent 的 register、heartbeat、terminal、exec、
// file 操作协议不变 (Requirements: 6.3)
//
// This file defines the gRPC service stubs that maintain protocol compatibility
// with existing Host Agents. The actual gRPC transport (protobuf definitions,
// generated code) remains unchanged; this layer adapts the new RuntimeKernel
// to the existing protocol.
// ---------------------------------------------------------------------------

// HostAgentStream abstracts a gRPC bidirectional stream connection to a Host Agent.
type HostAgentStream interface {
	// Send sends a message to the Host Agent.
	Send(msg *HostMessage) error
	// Recv receives a message from the Host Agent.
	Recv() (*HostMessage, error)
	// Context returns the stream context.
	Context() context.Context
}

// HostMessage is the protocol message exchanged with Host Agents over gRPC.
// It preserves the existing wire format.
type HostMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
	Time    int64           `json:"time"`
}

// HostMessageType constants — unchanged from existing protocol.
const (
	HostMsgRegister  = "register"
	HostMsgHeartbeat = "heartbeat"
	HostMsgTerminal  = "terminal"
	HostMsgExec      = "exec"
	HostMsgFile      = "file"
	HostMsgAck       = "ack"
	HostMsgError     = "error"
)

// ---------------------------------------------------------------------------
// GRPCServer manages Host Agent gRPC connections.
// ---------------------------------------------------------------------------

// GRPCServer manages bidirectional gRPC streams with Host Agents.
type GRPCServer struct {
	mu     sync.RWMutex
	agents map[string]*hostConnection
}

// hostConnection tracks a single Host Agent's gRPC stream.
type hostConnection struct {
	hostID     string
	stream     HostAgentStream
	registered time.Time
	lastPing   time.Time
	done       chan struct{}
}

// NewGRPCServer creates a new GRPCServer.
func NewGRPCServer() *GRPCServer {
	return &GRPCServer{
		agents: make(map[string]*hostConnection),
	}
}

// HandleStream processes a bidirectional gRPC stream from a Host Agent.
// It maintains the existing register/heartbeat/terminal/exec/file protocol.
func (s *GRPCServer) HandleStream(stream HostAgentStream) error {
	ctx := stream.Context()

	// First message must be register
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv register: %w", err)
	}
	if msg.Type != HostMsgRegister {
		return fmt.Errorf("expected register message, got %q", msg.Type)
	}

	hostID := msg.ID
	if hostID == "" {
		return fmt.Errorf("register message missing host ID")
	}

	conn := &hostConnection{
		hostID:     hostID,
		stream:     stream,
		registered: time.Now(),
		lastPing:   time.Now(),
		done:       make(chan struct{}),
	}

	s.mu.Lock()
	s.agents[hostID] = conn
	s.mu.Unlock()

	// Send ack
	_ = stream.Send(&HostMessage{
		Type: HostMsgAck,
		ID:   hostID,
		Time: time.Now().UnixMilli(),
	})

	defer func() {
		s.mu.Lock()
		delete(s.agents, hostID)
		s.mu.Unlock()
		close(conn.done)
	}()

	// Message loop — process heartbeat, terminal, exec, file
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		switch msg.Type {
		case HostMsgHeartbeat:
			conn.lastPing = time.Now()
			_ = stream.Send(&HostMessage{
				Type: HostMsgAck,
				ID:   msg.ID,
				Time: time.Now().UnixMilli(),
			})

		case HostMsgTerminal, HostMsgExec, HostMsgFile:
			// These are responses from Host Agent to tool calls.
			// Route them back to the pending request handler.
			s.handleHostResponse(hostID, msg)

		default:
			_ = stream.Send(&HostMessage{
				Type:  HostMsgError,
				Error: fmt.Sprintf("unknown message type: %s", msg.Type),
				Time:  time.Now().UnixMilli(),
			})
		}
	}
}

// SendToHost sends a command message to a specific Host Agent.
func (s *GRPCServer) SendToHost(hostID string, msg *HostMessage) error {
	s.mu.RLock()
	conn, ok := s.agents[hostID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("host %q not connected", hostID)
	}

	msg.Time = time.Now().UnixMilli()
	return conn.stream.Send(msg)
}

// IsHostConnected reports whether a Host Agent is currently connected.
func (s *GRPCServer) IsHostConnected(hostID string) bool {
	s.mu.RLock()
	_, ok := s.agents[hostID]
	s.mu.RUnlock()
	return ok
}

// ConnectedHosts returns the list of currently connected Host Agent IDs.
func (s *GRPCServer) ConnectedHosts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hosts := make([]string, 0, len(s.agents))
	for id := range s.agents {
		hosts = append(hosts, id)
	}
	return hosts
}

// handleHostResponse routes a response from a Host Agent to the appropriate handler.
// In production this would use a pending-request map with channels.
func (s *GRPCServer) handleHostResponse(_ string, _ *HostMessage) {
	// Stub: in production, this routes responses to pending tool call waiters.
	// The protocol format is preserved — no changes to wire format.
}
