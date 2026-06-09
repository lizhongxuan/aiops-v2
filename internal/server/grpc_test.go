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
