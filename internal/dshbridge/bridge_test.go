package dshbridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryEventStore struct {
	mu       sync.Mutex
	events   map[string]map[int64]DurableEvent
	terminal map[string]bool
}

func newMemoryEventStore() *memoryEventStore {
	return &memoryEventStore{events: make(map[string]map[int64]DurableEvent), terminal: make(map[string]bool)}
}

func (s *memoryEventStore) Cursor(_ context.Context, sessionID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.events[sessionID]
	if len(events) == 0 {
		return -1, nil
	}
	var cursor int64 = -1
	for seq := range events {
		if seq > cursor {
			cursor = seq
		}
	}
	return cursor, nil
}

func (s *memoryEventStore) Commit(_ context.Context, event DurableEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.events[event.SessionID]
	if events == nil {
		events = make(map[int64]DurableEvent)
		s.events[event.SessionID] = events
	}
	if prior, exists := events[event.Seq]; exists {
		if !reflect.DeepEqual(prior, event) {
			return errors.New("conflicting event replay")
		}
		return nil
	}
	var cursor int64 = -1
	for seq := range events {
		if seq > cursor {
			cursor = seq
		}
	}
	if event.Seq != cursor+1 {
		return errors.New("durable event sequence gap")
	}
	events[event.Seq] = event
	if event.Type == "turn/end" && event.TurnID != "" {
		s.terminal[event.SessionID+"\x00"+event.TurnID] = true
	}
	return nil
}

func (s *memoryEventStore) TurnComplete(_ context.Context, sessionID, turnID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal[sessionID+"\x00"+turnID], nil
}

type blockingDispatcher struct {
	calls   atomic.Int32
	entered chan ToolCall
	approve chan ToolResult
}

type drainingDispatcher struct {
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (d *drainingDispatcher) Dispatch(ctx context.Context, _ TurnBinding, _ ToolCall) (ToolResult, error) {
	d.entered <- struct{}{}
	<-ctx.Done()
	d.canceled <- struct{}{}
	<-d.release
	return ToolResult{}, ctx.Err()
}

func (d *blockingDispatcher) Dispatch(ctx context.Context, _ TurnBinding, call ToolCall) (ToolResult, error) {
	d.calls.Add(1)
	d.entered <- call
	select {
	case result := <-d.approve:
		return result, nil
	case <-ctx.Done():
		return ToolResult{}, ctx.Err()
	}
}

type testPeer struct {
	conn          net.Conn
	requests      chan rpcFrame
	notifications chan rpcFrame
	responses     chan rpcFrame
	writeMu       sync.Mutex
}

func newTestPeer(conn net.Conn) *testPeer {
	p := &testPeer{
		conn: conn, requests: make(chan rpcFrame, 16),
		notifications: make(chan rpcFrame, 32), responses: make(chan rpcFrame, 16),
	}
	go p.readLoop()
	return p
}

func (p *testPeer) readLoop() {
	reader := bufio.NewReader(p.conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var frame rpcFrame
		if json.Unmarshal(line, &frame) != nil {
			continue
		}
		switch {
		case frame.Method != "" && len(frame.ID) > 0:
			p.requests <- frame
		case frame.Method != "":
			p.notifications <- frame
		case len(frame.ID) > 0:
			p.responses <- frame
		}
	}
}

func (p *testPeer) nextRequest(t *testing.T, method string) rpcFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case frame := <-p.requests:
			if frame.Method == method {
				return frame
			}
			p.requests <- frame
			select {
			case <-time.After(5 * time.Millisecond):
			case <-deadline:
				t.Fatalf("timed out waiting for JSON-RPC %s", method)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for JSON-RPC %s", method)
		}
	}
}

func (p *testPeer) respond(t *testing.T, request rpcFrame, result any) {
	t.Helper()
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func (p *testPeer) notify(t *testing.T, method string, params any) {
	t.Helper()
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (p *testPeer) request(t *testing.T, id any, method string, params any) {
	t.Helper()
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func (p *testPeer) write(t *testing.T, frame any) {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := p.conn.Write(append(data, '\n')); err != nil {
		t.Fatalf("write fake DSH frame: %v", err)
	}
}

func (p *testPeer) response(t *testing.T, id string) rpcFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case frame := <-p.responses:
			var got string
			_ = json.Unmarshal(frame.ID, &got)
			if got == id {
				return frame
			}
		case <-deadline:
			t.Fatalf("timed out waiting for response id %q", id)
		}
	}
}

func startTestBridge(t *testing.T, events EventStore, dispatcher ToolDispatcher, stream func(string, string, json.RawMessage)) (*Bridge, *testPeer) {
	t.Helper()
	hostConn, peerConn := net.Pipe()
	peer := newTestPeer(peerConn)
	t.Cleanup(func() { _ = peerConn.Close() })
	bridge, err := New(hostConn, Config{
		CWD: "/tmp/aiops-dsh-fixture", Provider: "mock", Model: "existing-model-route",
		Tools: []ToolSchema{{
			Name: "go_read", Description: "Read through the Go runtime.",
			Parameters: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"],"additionalProperties":false}`),
		}},
		Dispatcher: dispatcher, Events: events,
		Callbacks: Callbacks{AssistantStream: stream},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	started := make(chan error, 1)
	go func() { started <- bridge.Start(ctx) }()
	initialize := peer.nextRequest(t, "initialize")
	var params struct {
		BridgeVersion string       `json:"aiopsBridgeVersion"`
		Tools         []ToolSchema `json:"aiopsTools"`
		Provider      string       `json:"provider"`
		Model         string       `json:"model"`
	}
	if err := json.Unmarshal(initialize.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.BridgeVersion != ProtocolVersion || params.Provider != "mock" || params.Model != "existing-model-route" || len(params.Tools) != 1 || params.Tools[0].Name != "go_read" {
		t.Fatalf("initialize params = %#v", params)
	}
	peer.respond(t, initialize, map[string]any{
		"serverInfo":         map[string]string{"name": "deepseek-harness-sdk-runtime", "version": "test"},
		"aiopsBridgeVersion": ProtocolVersion,
		"aiopsCapabilities":  []string{"session/cancel", "session/replay", "assistant.stream", "aiops.tool.dispatch"},
	})
	if err := <-started; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return bridge, peer
}

func promptFixture(t *testing.T, bridge *Bridge, peer *testPeer, binding TurnBinding) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		_, err := bridge.Prompt(context.Background(), binding, []json.RawMessage{json.RawMessage(`{"type":"text","text":"read service state"}`)})
		result <- err
	}()
	request := peer.nextRequest(t, "session/prompt")
	var params promptParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.SessionID != binding.SessionID || params.TurnID != binding.TurnID {
		t.Fatalf("prompt binding = %#v", params)
	}
	peer.respond(t, request, map[string]string{"messageId": "message-1"})
	if err := <-result; err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
}

func fixtureBinding(sessionID, turnID string) TurnBinding {
	return TurnBinding{
		SessionID: sessionID, TurnID: turnID, TargetBindingHash: "sha256:target-a",
		ToolSurfaceFingerprint: "sha256:surface-a", PermissionSnapshotHash: "sha256:permissions-a",
	}
}

func TestPromptReceiptAndAgentIdleDoNotCompleteTurnBeforeDurableEnd(t *testing.T) {
	events := newMemoryEventStore()
	streamFrames := make(chan json.RawMessage, 2)
	bridge, peer := startTestBridge(t, events, &blockingDispatcher{entered: make(chan ToolCall, 1), approve: make(chan ToolResult)}, func(_, _ string, frame json.RawMessage) {
		streamFrames <- frame
	})
	binding := fixtureBinding("session-stream", "turn-stream")
	promptFixture(t, bridge, peer, binding)

	waitResult := make(chan error, 1)
	go func() { waitResult <- bridge.WaitTurn(context.Background(), binding) }()
	waitRequest := peer.nextRequest(t, "session/wait")
	peer.respond(t, waitRequest, map[string]any{})
	select {
	case err := <-waitResult:
		t.Fatalf("WaitTurn resolved on queue receipt/idle: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	peer.notify(t, "assistant.stream", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
		"frame": map[string]any{"type": "chunk", "index": 0, "text": "reading"},
	})
	select {
	case frame := <-streamFrames:
		if !strings.Contains(string(frame), `"text":"reading"`) {
			t.Fatalf("stream frame = %s", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("assistant stream frame was not forwarded")
	}

	peer.notify(t, "session.event", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
		"event": map[string]any{"seq": 0, "time": 1, "type": "turn/start", "data": map[string]any{"turn": 1}},
	})
	peer.notify(t, "session.event", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
		"event": map[string]any{"seq": 1, "time": 2, "type": "turn/end", "data": map[string]any{"reason": map[string]string{"kind": "completed"}}},
	})
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatalf("WaitTurn() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("durable turn/end did not complete the Go turn")
	}
	cursor, err := events.Cursor(context.Background(), binding.SessionID)
	if err != nil || cursor != 1 {
		t.Fatalf("event cursor = %d, %v; assistant stream must not advance it", cursor, err)
	}
}

func TestGoToolDispatchStaysPendingUntilGoApprovalAndRejectsCallReplay(t *testing.T) {
	events := newMemoryEventStore()
	dispatcher := &blockingDispatcher{entered: make(chan ToolCall, 2), approve: make(chan ToolResult, 1)}
	bridge, peer := startTestBridge(t, events, dispatcher, nil)
	binding := fixtureBinding("session-approval", "turn-approval")
	promptFixture(t, bridge, peer, binding)
	params := map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID, "callId": "call-1",
		"toolName": "go_read", "arguments": map[string]string{"target": "service-a"},
	}
	peer.request(t, "dsh-call-1", "aiops.tool.dispatch", params)
	call := <-dispatcher.entered
	if call.CallID != "call-1" || call.Name != "go_read" || !strings.Contains(string(call.Arguments), "service-a") {
		t.Fatalf("Go dispatcher call = %#v", call)
	}
	hash := sha256.Sum256(call.Arguments)
	if call.ArgumentsHash != "sha256:"+hex.EncodeToString(hash[:]) {
		t.Fatalf("arguments hash = %s", call.ArgumentsHash)
	}
	select {
	case response := <-peer.responses:
		t.Fatalf("DSH got a tool result before Go approval: %#v", response)
	case <-time.After(30 * time.Millisecond):
	}
	dispatcher.approve <- ToolResult{
		Content: []json.RawMessage{json.RawMessage(`{"type":"text","text":"approved result"}`)},
		IsError: false,
	}
	response := peer.response(t, "dsh-call-1")
	var result ToolResult
	if err := json.Unmarshal(response.Result, &result); err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("approved tool result = %#v, %v", result, err)
	}

	peer.request(t, "dsh-call-2", "aiops.tool.dispatch", params)
	replayed := peer.response(t, "dsh-call-2")
	var rejected ToolResult
	if err := json.Unmarshal(replayed.Result, &rejected); err != nil || !rejected.IsError {
		t.Fatalf("replayed call result = %#v, %v; want fail-closed error", rejected, err)
	}
	if got := dispatcher.calls.Load(); got != 1 {
		t.Fatalf("Go dispatcher executions = %d; want exactly once", got)
	}
}

func TestCancelTurnCancelsAndDrainsGoToolBeforeReturning(t *testing.T) {
	events := newMemoryEventStore()
	dispatcher := &blockingDispatcher{entered: make(chan ToolCall, 1), approve: make(chan ToolResult)}
	bridge, peer := startTestBridge(t, events, dispatcher, nil)
	binding := fixtureBinding("session-cancel", "turn-cancel")
	promptFixture(t, bridge, peer, binding)
	peer.request(t, "dsh-call-cancel", "aiops.tool.dispatch", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID, "callId": "call-cancel",
		"toolName": "go_read", "arguments": map[string]string{"target": "service-a"},
	})
	<-dispatcher.entered
	if err := bridge.CancelTurn(context.Background(), fixtureBinding(binding.SessionID, "wrong-turn")); err == nil {
		t.Fatal("wrong turn cancellation was accepted")
	}
	canceled := make(chan error, 1)
	go func() { canceled <- bridge.CancelTurn(context.Background(), binding) }()
	request := peer.nextRequest(t, "session/cancel")
	var params map[string]string
	if err := json.Unmarshal(request.Params, &params); err != nil || params["sessionId"] != binding.SessionID || params["turnId"] != binding.TurnID {
		t.Fatalf("cancel request binding = %#v, %v", params, err)
	}
	peer.respond(t, request, map[string]bool{"accepted": true})
	select {
	case err := <-canceled:
		if err != nil {
			t.Fatalf("CancelTurn() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("CancelTurn returned before the local Go dispatch drained")
	}
	result := peer.response(t, "dsh-call-cancel")
	var toolResult ToolResult
	if err := json.Unmarshal(result.Result, &toolResult); err != nil || !toolResult.IsError {
		t.Fatalf("cancelled tool response = %#v, %v", toolResult, err)
	}
}

func TestDshToolCancelIsTurnBoundAndTurnEndWaitsForGoDispatchDrain(t *testing.T) {
	events := newMemoryEventStore()
	dispatcher := &drainingDispatcher{
		entered: make(chan struct{}, 1), canceled: make(chan struct{}, 1), release: make(chan struct{}),
	}
	bridge, peer := startTestBridge(t, events, dispatcher, nil)
	binding := fixtureBinding("session-tool-cancel", "turn-tool-cancel")
	promptFixture(t, bridge, peer, binding)
	peer.request(t, "dsh-cancelled-tool", "aiops.tool.dispatch", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID, "callId": "call-cancel",
		"toolName": "go_read", "arguments": map[string]string{"target": "service-a"},
	})
	<-dispatcher.entered
	peer.notify(t, "aiops.tool.cancel", map[string]string{
		"sessionId": binding.SessionID, "turnId": "wrong-turn", "callId": "call-cancel",
	})
	select {
	case <-dispatcher.canceled:
		t.Fatal("a mismatched DSH cancel interrupted Go tool execution")
	case <-time.After(30 * time.Millisecond):
	}
	peer.notify(t, "aiops.tool.cancel", map[string]string{
		"sessionId": binding.SessionID, "turnId": binding.TurnID, "callId": "call-cancel",
	})
	select {
	case <-dispatcher.canceled:
	case <-time.After(time.Second):
		t.Fatal("matching DSH cancel did not reach the Go tool context")
	}
	peer.notify(t, "session.event", map[string]any{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
		"event": map[string]any{"seq": 0, "time": 1, "type": "turn/end", "data": map[string]any{"reason": map[string]string{"kind": "cancelled"}}},
	})
	select {
	case response := <-peer.responses:
		var id string
		_ = json.Unmarshal(response.ID, &id)
		if id == "dsh-cancelled-tool" {
			t.Fatal("DSH tool response escaped before Go dispatch quiescence")
		}
	case <-time.After(30 * time.Millisecond):
	}
	cursor, _ := events.Cursor(context.Background(), binding.SessionID)
	if cursor != -1 {
		t.Fatalf("turn/end committed while Go tool was still draining: cursor=%d", cursor)
	}
	close(dispatcher.release)
	toolResponse := peer.response(t, "dsh-cancelled-tool")
	var result ToolResult
	if err := json.Unmarshal(toolResponse.Result, &result); err != nil || !result.IsError {
		t.Fatalf("cancelled tool response = %#v, %v", result, err)
	}
	deadline := time.After(time.Second)
	for {
		cursor, _ = events.Cursor(context.Background(), binding.SessionID)
		if cursor == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("turn/end was not committed after Go dispatch drained")
		case <-time.After(5 * time.Millisecond):
		}
	}
	complete, err := events.TurnComplete(context.Background(), binding.SessionID, binding.TurnID)
	if err != nil || !complete {
		t.Fatalf("turn completion = %v, %v", complete, err)
	}
}

func TestRecoverTurnReplaysDurableTailAndDeduplicatesThroughEventStore(t *testing.T) {
	events := newMemoryEventStore()
	binding := fixtureBinding("session-recover", "turn-recover")
	if err := events.Commit(context.Background(), DurableEvent{
		SessionID: binding.SessionID, TurnID: binding.TurnID, Seq: 0, Time: 1,
		Type: "turn/start", Data: json.RawMessage(`{"turn":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	bridge, peer := startTestBridge(t, events, &blockingDispatcher{entered: make(chan ToolCall, 1), approve: make(chan ToolResult)}, nil)
	recovered := make(chan error, 1)
	go func() { recovered <- bridge.RecoverTurn(context.Background(), binding) }()
	replay := peer.nextRequest(t, "session/replay")
	var replayParams struct {
		SessionID string `json:"sessionId"`
		AfterSeq  int64  `json:"afterSeq"`
	}
	if err := json.Unmarshal(replay.Params, &replayParams); err != nil || replayParams.SessionID != binding.SessionID || replayParams.AfterSeq != 0 {
		t.Fatalf("replay cursor = %#v, %v", replayParams, err)
	}
	peer.respond(t, replay, map[string]any{
		"sessionId": binding.SessionID,
		"events": []any{map[string]any{
			"seq": 1, "time": 2, "type": "turn/end", "data": map[string]any{"reason": map[string]string{"kind": "completed"}},
		}},
	})
	if err := <-recovered; err != nil {
		t.Fatalf("RecoverTurn() error = %v", err)
	}
	if err := bridge.WaitTurn(context.Background(), binding); err != nil {
		t.Fatalf("WaitTurn() after recovered terminal = %v", err)
	}
	cursor, err := events.Cursor(context.Background(), binding.SessionID)
	if err != nil || cursor != 1 {
		t.Fatalf("replayed cursor = %d, %v", cursor, err)
	}
}

func TestStartRejectsUnnegotiatedExtension(t *testing.T) {
	hostConn, peerConn := net.Pipe()
	peer := newTestPeer(peerConn)
	defer peerConn.Close()
	bridge, err := New(hostConn, Config{
		CWD: "/tmp/aiops-dsh-fixture", Provider: "mock", Model: "existing-model-route",
		Tools:      []ToolSchema{{Name: "go_read", Description: "read", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Dispatcher: &blockingDispatcher{entered: make(chan ToolCall, 1), approve: make(chan ToolResult)}, Events: newMemoryEventStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	started := make(chan error, 1)
	go func() { started <- bridge.Start(context.Background()) }()
	request := peer.nextRequest(t, "initialize")
	peer.respond(t, request, map[string]any{
		"aiopsBridgeVersion": "unknown", "aiopsCapabilities": []string{"session/cancel"},
	})
	if err := <-started; err == nil || !strings.Contains(err.Error(), "did not negotiate") {
		t.Fatalf("Start() error = %v; want fail-closed negotiation error", err)
	}
}
