package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/terminal"
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

type HostExecRequest struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	TimeoutMs      int      `json:"timeoutMs,omitempty"`
	MaxOutputBytes int      `json:"maxOutputBytes,omitempty"`
}

type HostExecResponse struct {
	Status   string `json:"status"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

type hostTerminalPayload struct {
	Action    string `json:"action"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd,omitempty"`
	Shell     string `json:"shell,omitempty"`
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Data      string `json:"data,omitempty"`
	Signal    string `json:"signal,omitempty"`
	Status    string `json:"status,omitempty"`
	Code      int    `json:"code,omitempty"`
	Error     string `json:"error,omitempty"`
}

type HostAgentGRPCAuthenticator interface {
	AuthenticateHostAgentGRPC(ctx context.Context, hostID, token string) error
}

type HostAgentGRPCAuthenticatorFunc func(ctx context.Context, hostID, token string) error

func (fn HostAgentGRPCAuthenticatorFunc) AuthenticateHostAgentGRPC(ctx context.Context, hostID, token string) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, hostID, token)
}

type hostAgentGRPCRegisterPayload struct {
	Token string `json:"token"`
}

// ---------------------------------------------------------------------------
// GRPCServer manages Host Agent gRPC connections.
// ---------------------------------------------------------------------------

// GRPCServer manages bidirectional gRPC streams with Host Agents.
type GRPCServer struct {
	mu            sync.RWMutex
	agents        map[string]*hostConnection
	pending       map[string]chan *HostMessage
	terminals     map[string]chan *HostMessage
	authenticator HostAgentGRPCAuthenticator
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
	return NewGRPCServerWithAuthenticator(nil)
}

func NewGRPCServerWithAuthenticator(authenticator HostAgentGRPCAuthenticator) *GRPCServer {
	return &GRPCServer{
		agents:        make(map[string]*hostConnection),
		pending:       make(map[string]chan *HostMessage),
		terminals:     make(map[string]chan *HostMessage),
		authenticator: authenticator,
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
	if s.authenticator != nil {
		token := hostAgentGRPCRegisterToken(msg.Payload)
		if err := s.authenticator.AuthenticateHostAgentGRPC(ctx, hostID, token); err != nil {
			_ = stream.Send(&HostMessage{
				Type:  HostMsgError,
				ID:    hostID,
				Error: "unauthorized",
				Time:  time.Now().UnixMilli(),
			})
			return fmt.Errorf("host-agent %q grpc authentication failed: %w", hostID, err)
		}
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

func hostAgentGRPCRegisterToken(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body hostAgentGRPCRegisterPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Token)
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

func (s *GRPCServer) RunExec(ctx context.Context, hostID string, req HostExecRequest) (HostExecResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		return HostExecResponse{}, fmt.Errorf("command is required")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return HostExecResponse{}, err
	}
	msgID := fmt.Sprintf("exec-%d", time.Now().UTC().UnixNano())
	response, err := s.requestHost(ctx, strings.TrimSpace(hostID), &HostMessage{
		Type:    HostMsgExec,
		ID:      msgID,
		Payload: data,
	})
	if err != nil {
		return HostExecResponse{}, err
	}
	if response.Type == HostMsgError {
		return HostExecResponse{}, errors.New(firstNonEmpty(response.Error, "host-agent returned error"))
	}
	var payload HostExecResponse
	if len(response.Payload) > 0 {
		if err := json.Unmarshal(response.Payload, &payload); err != nil {
			return HostExecResponse{}, fmt.Errorf("decode host-agent exec response: %w", err)
		}
	}
	if strings.TrimSpace(response.Error) != "" && strings.TrimSpace(payload.Error) == "" {
		payload.Error = strings.TrimSpace(response.Error)
	}
	if strings.TrimSpace(payload.Status) == "" {
		payload.Status = "success"
	}
	return payload, nil
}

func (s *GRPCServer) OpenTerminal(ctx context.Context, req terminal.RemoteTerminalOpenRequest) (terminal.RemoteTerminalHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return nil, fmt.Errorf("host id is required")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("terminal session id is required")
	}
	events := make(chan *HostMessage, 32)
	s.registerTerminal(sessionID, events)
	handle := &grpcTerminalHandle{
		server:    s,
		hostID:    hostID,
		sessionID: sessionID,
		events:    events,
		emit:      req.Emit,
	}
	go handle.forwardEvents()

	payload, err := json.Marshal(hostTerminalPayload{
		Action:    "open",
		SessionID: sessionID,
		Cwd:       req.Cwd,
		Shell:     req.Shell,
		Cols:      req.Cols,
		Rows:      req.Rows,
	})
	if err != nil {
		s.unregisterTerminal(sessionID)
		return nil, err
	}
	msgID := fmt.Sprintf("terminal-open-%d", time.Now().UTC().UnixNano())
	response, err := s.requestHost(ctx, hostID, &HostMessage{
		Type:    HostMsgTerminal,
		ID:      msgID,
		Payload: payload,
	})
	if err != nil {
		s.unregisterTerminal(sessionID)
		return nil, err
	}
	if response.Type == HostMsgError {
		s.unregisterTerminal(sessionID)
		return nil, errors.New(firstNonEmpty(response.Error, "host-agent returned terminal error"))
	}
	return handle, nil
}

func (s *GRPCServer) requestHost(ctx context.Context, hostID string, msg *HostMessage) (*HostMessage, error) {
	if hostID == "" {
		return nil, fmt.Errorf("host id is required")
	}
	if msg == nil {
		return nil, fmt.Errorf("host message is required")
	}
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano())
	}
	ch := make(chan *HostMessage, 1)
	s.mu.Lock()
	if _, exists := s.pending[msg.ID]; exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("host message id already pending: %s", msg.ID)
	}
	s.pending[msg.ID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, msg.ID)
		s.mu.Unlock()
	}()

	if err := s.SendToHost(hostID, msg); err != nil {
		return nil, err
	}
	select {
	case response := <-ch:
		return response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *GRPCServer) registerTerminal(sessionID string, events chan *HostMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminals[sessionID] = events
}

func (s *GRPCServer) unregisterTerminal(sessionID string) {
	s.mu.Lock()
	events := s.terminals[sessionID]
	delete(s.terminals, sessionID)
	s.mu.Unlock()
	if events != nil {
		close(events)
	}
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
func (s *GRPCServer) handleHostResponse(_ string, msg *HostMessage) {
	if msg == nil || strings.TrimSpace(msg.ID) == "" {
		return
	}
	s.mu.RLock()
	ch := s.pending[msg.ID]
	s.mu.RUnlock()
	if ch == nil {
		if msg.Type == HostMsgTerminal {
			s.routeTerminalMessage(msg)
		}
		return
	}
	select {
	case ch <- msg:
	default:
	}
}

func (s *GRPCServer) routeTerminalMessage(msg *HostMessage) {
	var payload hostTerminalPayload
	if len(msg.Payload) == 0 || json.Unmarshal(msg.Payload, &payload) != nil || strings.TrimSpace(payload.SessionID) == "" {
		return
	}
	s.mu.RLock()
	ch := s.terminals[payload.SessionID]
	s.mu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- msg:
	default:
	}
}

type grpcTerminalHandle struct {
	server    *GRPCServer
	hostID    string
	sessionID string
	events    <-chan *HostMessage
	emit      func(terminal.Event)
	closeOnce sync.Once
}

func (h *grpcTerminalHandle) SendInput(data string) error {
	return h.send(hostTerminalPayload{Action: "input", SessionID: h.sessionID, Data: data})
}

func (h *grpcTerminalHandle) Resize(cols, rows int) error {
	return h.send(hostTerminalPayload{Action: "resize", SessionID: h.sessionID, Cols: cols, Rows: rows})
}

func (h *grpcTerminalHandle) Signal(name string) error {
	return h.send(hostTerminalPayload{Action: "signal", SessionID: h.sessionID, Signal: strings.TrimSpace(name)})
}

func (h *grpcTerminalHandle) Close() error {
	var err error
	h.closeOnce.Do(func() {
		err = h.send(hostTerminalPayload{Action: "close", SessionID: h.sessionID})
		h.server.unregisterTerminal(h.sessionID)
	})
	return err
}

func (h *grpcTerminalHandle) send(payload hostTerminalPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.server.SendToHost(h.hostID, &HostMessage{
		Type:    HostMsgTerminal,
		ID:      fmt.Sprintf("terminal-%s-%d", payload.Action, time.Now().UTC().UnixNano()),
		Payload: data,
	})
}

func (h *grpcTerminalHandle) forwardEvents() {
	for msg := range h.events {
		event, ok := terminalEventFromHostMessage(msg)
		if !ok {
			continue
		}
		if h.emit != nil {
			h.emit(event)
		}
		if event.Type == terminal.EventTypeExit || event.Type == terminal.EventTypeError {
			h.server.unregisterTerminal(h.sessionID)
			return
		}
	}
}

func terminalEventFromHostMessage(msg *HostMessage) (terminal.Event, bool) {
	var payload hostTerminalPayload
	if len(msg.Payload) == 0 || json.Unmarshal(msg.Payload, &payload) != nil {
		return terminal.Event{}, false
	}
	event := terminal.Event{
		SessionID: payload.SessionID,
		UpdatedAt: time.Now().UTC(),
	}
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "output":
		event.Type = terminal.EventTypeOutput
		event.Data = payload.Data
	case "status":
		event.Type = terminal.EventTypeStatus
		event.Status = terminal.SessionStatus(payload.Status)
	case "exit":
		event.Type = terminal.EventTypeExit
		event.Status = terminal.SessionStatusExited
		event.Code = payload.Code
		event.Signal = payload.Signal
	case "error":
		event.Type = terminal.EventTypeError
		event.Status = terminal.SessionStatusError
		event.Message = firstNonEmpty(payload.Error, msg.Error, "terminal error")
	default:
		return terminal.Event{}, false
	}
	return event, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
