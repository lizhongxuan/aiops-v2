package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/agentrpc"
	"aiops-v2/internal/terminal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type grpcTestStream struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	incoming chan *HostMessage
	outgoing []*HostMessage
}

func newGRPCTestStream() *grpcTestStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &grpcTestStream{
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan *HostMessage, 32),
		outgoing: make([]*HostMessage, 0),
	}
}

func (s *grpcTestStream) Send(msg *HostMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *msg
	cp.Payload = append(json.RawMessage(nil), msg.Payload...)
	s.outgoing = append(s.outgoing, &cp)
	return nil
}

func (s *grpcTestStream) Recv() (*HostMessage, error) {
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

func (s *grpcTestStream) Context() context.Context {
	return s.ctx
}

func (s *grpcTestStream) enqueue(msg *HostMessage) {
	s.incoming <- msg
}

func (s *grpcTestStream) getOutgoing() []*HostMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*HostMessage, len(s.outgoing))
	copy(out, s.outgoing)
	return out
}

func TestGRPCRunExecWaitsForHostAgentResult(t *testing.T) {
	grpcSrv := NewGRPCServer()
	stream := newGRPCTestStream()
	stream.enqueue(&HostMessage{Type: HostMsgRegister, ID: "host-kme", Time: time.Now().UnixMilli()})
	done := make(chan error, 1)
	go func() {
		done <- grpcSrv.HandleStream(stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-done
	})
	waitForConnectedHost(t, grpcSrv, "host-kme")

	resultCh := make(chan HostExecResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := grpcSrv.RunExec(context.Background(), "host-kme", HostExecRequest{
			Command: "nproc",
			Args:    []string{},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	execMsg := waitForOutgoingExec(t, stream)
	var req HostExecRequest
	if err := json.Unmarshal(execMsg.Payload, &req); err != nil {
		t.Fatalf("exec payload is not HostExecRequest: %v\n%s", err, execMsg.Payload)
	}
	if req.Command != "nproc" {
		t.Fatalf("exec command = %q, want nproc", req.Command)
	}
	respData, err := json.Marshal(HostExecResponse{Status: "success", Stdout: "8\n", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal exec response: %v", err)
	}
	stream.enqueue(&HostMessage{Type: HostMsgExec, ID: execMsg.ID, Payload: respData, Time: time.Now().UnixMilli()})

	select {
	case err := <-errCh:
		t.Fatalf("RunExec() error = %v", err)
	case result := <-resultCh:
		if result.Stdout != "8\n" || result.ExitCode != 0 || result.Status != "success" {
			t.Fatalf("RunExec() = %#v, want stdout 8", result)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExec() did not return host-agent response")
	}
}

func TestGRPCTerminalBackendForwardsControlsAndOutput(t *testing.T) {
	grpcSrv := NewGRPCServer()
	stream := newGRPCTestStream()
	stream.enqueue(&HostMessage{Type: HostMsgRegister, ID: "host-terminal", Time: time.Now().UnixMilli()})
	done := make(chan error, 1)
	go func() {
		done <- grpcSrv.HandleStream(stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-done
	})
	waitForConnectedHost(t, grpcSrv, "host-terminal")

	terminalMgr := terminal.NewManager(terminal.WithRemoteBackend(grpcSrv))
	metaCh := make(chan terminal.SessionMetadata, 1)
	errCh := make(chan error, 1)
	go func() {
		meta, err := terminalMgr.CreateSession(context.Background(), terminal.CreateSessionRequest{
			HostID: "host-terminal",
			Cwd:    "/srv",
			Shell:  "/bin/bash",
			Cols:   120,
			Rows:   36,
		})
		if err != nil {
			errCh <- err
			return
		}
		metaCh <- meta
	}()

	openMsg := waitForOutgoingTerminalAction(t, stream, "open")
	var openPayload hostTerminalPayload
	if err := json.Unmarshal(openMsg.Payload, &openPayload); err != nil {
		t.Fatalf("decode open payload: %v", err)
	}
	if openPayload.SessionID == "" || openPayload.Cwd != "/srv" || openPayload.Shell != "/bin/bash" {
		t.Fatalf("open payload = %+v, want session cwd and shell", openPayload)
	}
	ackPayload, err := json.Marshal(hostTerminalPayload{Action: "status", SessionID: openPayload.SessionID, Status: "running"})
	if err != nil {
		t.Fatalf("marshal terminal ack: %v", err)
	}
	stream.enqueue(&HostMessage{Type: HostMsgTerminal, ID: openMsg.ID, Payload: ackPayload, Time: time.Now().UnixMilli()})

	var meta terminal.SessionMetadata
	select {
	case err := <-errCh:
		t.Fatalf("CreateSession() error = %v", err)
	case meta = <-metaCh:
		if meta.SessionID != openPayload.SessionID || meta.HostID != "host-terminal" {
			t.Fatalf("metadata = %+v, want gRPC terminal metadata", meta)
		}
	case <-time.After(time.Second):
		t.Fatal("CreateSession() did not receive terminal open ack")
	}

	session := terminalMgr.GetSession(meta.SessionID)
	events, release := session.Subscribe()
	defer release()
	<-events // ready

	outputPayload, err := json.Marshal(hostTerminalPayload{Action: "output", SessionID: meta.SessionID, Data: "hello from grpc\n"})
	if err != nil {
		t.Fatalf("marshal terminal output: %v", err)
	}
	stream.enqueue(&HostMessage{Type: HostMsgTerminal, ID: "terminal-output-1", Payload: outputPayload, Time: time.Now().UnixMilli()})
	select {
	case event := <-events:
		if event.Type != terminal.EventTypeOutput || event.Data != "hello from grpc\n" {
			t.Fatalf("event = %+v, want grpc output", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for grpc terminal output")
	}

	if err := session.SendInput("date\n"); err != nil {
		t.Fatalf("SendInput() error = %v", err)
	}
	session.Resize(140, 50)
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	inputMsg := waitForOutgoingTerminalAction(t, stream, "input")
	var inputPayload hostTerminalPayload
	if err := json.Unmarshal(inputMsg.Payload, &inputPayload); err != nil {
		t.Fatalf("decode input payload: %v", err)
	}
	if inputPayload.SessionID != meta.SessionID || inputPayload.Data != "date\n" {
		t.Fatalf("input payload = %+v, want session input", inputPayload)
	}
	resizeMsg := waitForOutgoingTerminalAction(t, stream, "resize")
	var resizePayload hostTerminalPayload
	if err := json.Unmarshal(resizeMsg.Payload, &resizePayload); err != nil {
		t.Fatalf("decode resize payload: %v", err)
	}
	if resizePayload.Cols != 140 || resizePayload.Rows != 50 {
		t.Fatalf("resize payload = %+v, want 140x50", resizePayload)
	}
	_ = waitForOutgoingTerminalAction(t, stream, "close")
}

func TestGRPCHandleStreamRejectsInvalidRegisterToken(t *testing.T) {
	grpcSrv := NewGRPCServerWithAuthenticator(HostAgentGRPCAuthenticatorFunc(func(_ context.Context, hostID, token string) error {
		if hostID == "host-secure" && token == "expected-token" {
			return nil
		}
		return errors.New("invalid token")
	}))
	stream := newGRPCTestStream()
	payload, err := json.Marshal(map[string]any{"token": "wrong-token"})
	if err != nil {
		t.Fatalf("marshal register payload: %v", err)
	}
	stream.enqueue(&HostMessage{Type: HostMsgRegister, ID: "host-secure", Payload: payload, Time: time.Now().UnixMilli()})

	err = grpcSrv.HandleStream(stream)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("HandleStream() error = %v, want authentication failure", err)
	}
	if grpcSrv.IsHostConnected("host-secure") {
		t.Fatal("host-secure should not be connected after failed authentication")
	}
	outgoing := stream.getOutgoing()
	if len(outgoing) == 0 || outgoing[0].Type != HostMsgError || outgoing[0].Error != "unauthorized" {
		t.Fatalf("outgoing = %#v, want unauthorized error message", outgoing)
	}
}

func TestGRPCHandleStreamAcceptsValidRegisterToken(t *testing.T) {
	grpcSrv := NewGRPCServerWithAuthenticator(HostAgentGRPCAuthenticatorFunc(func(_ context.Context, hostID, token string) error {
		if hostID == "host-secure" && token == "expected-token" {
			return nil
		}
		return errors.New("invalid token")
	}))
	stream := newGRPCTestStream()
	payload, err := json.Marshal(map[string]any{"token": "expected-token"})
	if err != nil {
		t.Fatalf("marshal register payload: %v", err)
	}
	stream.enqueue(&HostMessage{Type: HostMsgRegister, ID: "host-secure", Payload: payload, Time: time.Now().UnixMilli()})
	done := make(chan error, 1)
	go func() {
		done <- grpcSrv.HandleStream(stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-done
	})
	waitForConnectedHost(t, grpcSrv, "host-secure")
	outgoing := stream.getOutgoing()
	if len(outgoing) == 0 || outgoing[0].Type != HostMsgAck || outgoing[0].ID != "host-secure" {
		t.Fatalf("outgoing = %#v, want registration ack", outgoing)
	}
}

func TestAgentGRPCServiceExecRoundTrip(t *testing.T) {
	manager := NewGRPCServer()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	RegisterAgentGRPCService(grpcServer, manager)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	defer conn.Close()
	stream, err := agentrpc.NewAgentServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	registerEnvelope, err := hostMessageToStruct(&HostMessage{Type: HostMsgRegister, ID: "host-grpc", Time: time.Now().UnixMilli()})
	if err != nil {
		t.Fatalf("register envelope: %v", err)
	}
	if err := stream.Send(registerEnvelope); err != nil {
		t.Fatalf("send register: %v", err)
	}
	ackEnvelope, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv ack: %v", err)
	}
	ack, err := structToHostMessage(ackEnvelope)
	if err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ack.Type != HostMsgAck || ack.ID != "host-grpc" {
		t.Fatalf("ack = %#v, want host ack", ack)
	}

	resultCh := make(chan HostExecResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := manager.RunExec(context.Background(), "host-grpc", HostExecRequest{Command: "hostname"})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	execEnvelope, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv exec: %v", err)
	}
	execMsg, err := structToHostMessage(execEnvelope)
	if err != nil {
		t.Fatalf("decode exec: %v", err)
	}
	if execMsg.Type != HostMsgExec {
		t.Fatalf("message type = %q, want exec", execMsg.Type)
	}
	responsePayload, err := json.Marshal(HostExecResponse{Status: "success", Stdout: "host-grpc\n", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	responseEnvelope, err := hostMessageToStruct(&HostMessage{Type: HostMsgExec, ID: execMsg.ID, Payload: responsePayload, Time: time.Now().UnixMilli()})
	if err != nil {
		t.Fatalf("response envelope: %v", err)
	}
	if err := stream.Send(responseEnvelope); err != nil {
		t.Fatalf("send response: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("RunExec() error = %v", err)
	case result := <-resultCh:
		if result.Stdout != "host-grpc\n" {
			t.Fatalf("result = %#v, want grpc stdout", result)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExec() did not receive gRPC response")
	}
}

func waitForConnectedHost(t *testing.T, srv *GRPCServer, hostID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if srv.IsHostConnected(hostID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("host %s did not connect", hostID)
}

func waitForOutgoingExec(t *testing.T, stream *grpcTestStream) *HostMessage {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, msg := range stream.getOutgoing() {
			if msg.Type == HostMsgExec {
				return msg
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no outgoing exec message")
	return nil
}

func waitForOutgoingTerminalAction(t *testing.T, stream *grpcTestStream, action string) *HostMessage {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, msg := range stream.getOutgoing() {
			if msg.Type != HostMsgTerminal {
				continue
			}
			var payload hostTerminalPayload
			if err := json.Unmarshal(msg.Payload, &payload); err == nil && payload.Action == action {
				return msg
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no outgoing terminal %s message", action)
	return nil
}
