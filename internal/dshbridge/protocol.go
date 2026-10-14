package dshbridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func (b *Bridge) readLoop() {
	scanner := bufio.NewScanner(b.conn)
	scanner.Buffer(make([]byte, 64<<10), maxFrameBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		var frame rpcFrame
		if err := json.Unmarshal(line, &frame); err != nil || frame.JSONRPC != "2.0" {
			b.fail(errors.New("invalid JSON-RPC frame from DSH stdout"))
			return
		}
		switch {
		case frame.Method != "" && len(frame.ID) > 0:
			go b.handleIncomingRequest(frame)
		case frame.Method != "":
			if err := b.handleNotification(frame.Method, frame.Params); err != nil {
				b.fail(err)
				return
			}
		case len(frame.ID) > 0:
			b.handleResponse(frame)
		default:
			b.fail(errors.New("unrecognized JSON-RPC frame from DSH stdout"))
			return
		}
	}
	if err := scanner.Err(); err != nil {
		b.fail(err)
	} else {
		b.fail(io.EOF)
	}
}

func (b *Bridge) handleResponse(frame rpcFrame) {
	var id string
	if err := json.Unmarshal(frame.ID, &id); err != nil || id == "" {
		b.fail(errors.New("DSH returned a response with an invalid request id"))
		return
	}
	b.mu.Lock()
	ch := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if ch == nil {
		return
	}
	if frame.Error != nil {
		ch <- rpcResponse{err: fmt.Errorf("DSH JSON-RPC error %d: %s", frame.Error.Code, frame.Error.Message)}
		return
	}
	ch <- rpcResponse{result: frame.Result}
}

func (b *Bridge) handleNotification(method string, raw json.RawMessage) error {
	switch method {
	case "session.event":
		var payload eventNotification
		if err := json.Unmarshal(raw, &payload); err != nil || payload.SessionID == "" {
			return errors.New("invalid DSH session.event notification")
		}
		var event DurableEvent
		if err := json.Unmarshal(payload.Event, &event); err != nil || event.Type == "" || event.Seq < 0 {
			return errors.New("invalid DSH durable session event")
		}
		event.SessionID = payload.SessionID
		event.TurnID = payload.TurnID
		return b.commitEvent(event)
	case "assistant.stream":
		var payload streamNotification
		if err := json.Unmarshal(raw, &payload); err != nil || payload.SessionID == "" || payload.TurnID == "" || len(payload.Frame) == 0 {
			return errors.New("invalid DSH assistant.stream notification")
		}
		b.mu.Lock()
		turn := b.active[payload.SessionID]
		valid := turn != nil && turn.binding.TurnID == payload.TurnID && !turn.terminal
		b.mu.Unlock()
		if valid && b.config.Callbacks.AssistantStream != nil {
			b.config.Callbacks.AssistantStream(payload.SessionID, payload.TurnID, append(json.RawMessage(nil), payload.Frame...))
		}
	case "aiops.tool.cancel":
		var payload toolCancelParams
		if err := json.Unmarshal(raw, &payload); err != nil || payload.SessionID == "" || payload.TurnID == "" || payload.CallID == "" {
			return errors.New("invalid DSH aiops.tool.cancel notification")
		}
		b.cancelOneDispatch(payload)
	default:
		// Status/child notifications never establish business completion.
	}
	return nil
}

func (b *Bridge) handleIncomingRequest(frame rpcFrame) {
	var id any
	if err := json.Unmarshal(frame.ID, &id); err != nil || id == nil {
		b.fail(errors.New("DSH tool request has an invalid JSON-RPC id"))
		return
	}
	if frame.Method != "aiops.tool.dispatch" {
		b.sendError(frame.ID, -32601, "method not supported")
		return
	}
	var params toolDispatchParams
	if err := json.Unmarshal(frame.Params, &params); err != nil || params.SessionID == "" || params.TurnID == "" || params.CallID == "" || params.ToolName == "" || !validJSONObject(params.Arguments) {
		b.sendError(frame.ID, -32602, "invalid tool dispatch request")
		return
	}
	if _, allowed := b.tools[params.ToolName]; !allowed {
		b.sendError(frame.ID, -32602, "tool is outside the negotiated Go surface")
		return
	}
	b.mu.Lock()
	turn := b.active[params.SessionID]
	if turn == nil || turn.binding.TurnID != params.TurnID || turn.terminal || turn.canceling || turn.settling {
		b.mu.Unlock()
		b.sendToolError(frame.ID, "Go runtime rejected an unbound tool request.")
		return
	}
	if _, duplicate := turn.calls[params.CallID]; duplicate {
		b.mu.Unlock()
		b.sendToolError(frame.ID, "Go runtime rejected a repeated tool request.")
		return
	}
	turn.calls[params.CallID] = struct{}{}
	turn.inFlight.Add(1)
	callCtx, callCancel := context.WithCancel(b.ctx)
	key := dispatchKey(params.SessionID, params.TurnID, params.CallID)
	b.dispatches[key] = callCancel
	b.mu.Unlock()
	defer func() {
		callCancel()
		turn.inFlight.Done()
		b.mu.Lock()
		delete(b.dispatches, key)
		b.mu.Unlock()
	}()

	hash := sha256.Sum256(params.Arguments)
	result, err := b.config.Dispatcher.Dispatch(callCtx, turn.binding, ToolCall{
		CallID: params.CallID, Name: params.ToolName,
		Arguments: append(json.RawMessage(nil), params.Arguments...), ArgumentsHash: "sha256:" + hex.EncodeToString(hash[:]),
	})
	if err != nil || !validToolResult(result) {
		b.sendToolError(frame.ID, "Go runtime rejected or failed this tool call.")
		return
	}
	if callCtx.Err() != nil {
		b.sendToolError(frame.ID, "Go runtime cancelled this tool call.")
		return
	}
	b.sendResult(frame.ID, result)
}

func (b *Bridge) commitEvent(event DurableEvent) error {
	if event.SessionID == "" || event.Seq < 0 || event.Type == "" {
		return errors.New("invalid durable event identity")
	}
	b.mu.Lock()
	turn := b.active[event.SessionID]
	if event.TurnID != "" {
		if turn == nil || turn.binding.TurnID != event.TurnID {
			b.mu.Unlock()
			return errors.New("DSH event turn binding does not match the active Go turn")
		}
	} else if turn != nil && event.Seq > turn.startSeq {
		event.TurnID = turn.binding.TurnID
	}
	b.mu.Unlock()
	if event.Type == "turn/end" && event.TurnID != "" && turn != nil && turn.binding.TurnID == event.TurnID {
		if err := b.cancelAndWait(turn); err != nil {
			return fmt.Errorf("wait for Go tool dispatch to quiesce at turn end: %w", err)
		}
	}
	if err := b.config.Events.Commit(b.ctx, event); err != nil {
		return fmt.Errorf("commit DSH durable event: %w", err)
	}
	if event.Type == "turn/end" && event.TurnID != "" && turn != nil && turn.binding.TurnID == event.TurnID {
		b.markTerminal(turn)
	}
	return nil
}

func (b *Bridge) markTerminal(turn *activeTurn) {
	b.mu.Lock()
	if !turn.terminal {
		turn.terminal = true
		close(turn.done)
		if b.active[turn.binding.SessionID] == turn {
			delete(b.active, turn.binding.SessionID)
		}
	}
	b.mu.Unlock()
}

func (b *Bridge) isTerminal(turn *activeTurn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return turn.terminal
}

func (b *Bridge) activeTurn(binding TurnBinding) (*activeTurn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	turn := b.active[binding.SessionID]
	if turn == nil || turn.binding.TurnID != binding.TurnID {
		return nil, errors.New("Go turn binding is not active in this DSH session")
	}
	return turn, nil
}

func (b *Bridge) cancelOneDispatch(params toolCancelParams) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if turn := b.active[params.SessionID]; turn == nil || turn.binding.TurnID != params.TurnID {
		return
	}
	if cancel := b.dispatches[dispatchKey(params.SessionID, params.TurnID, params.CallID)]; cancel != nil {
		cancel()
	}
}

func (b *Bridge) cancelAndWait(turn *activeTurn) error {
	b.mu.Lock()
	turn.settling = true
	for key, cancel := range b.dispatches {
		if strings.HasPrefix(key, turn.binding.SessionID+"\x00"+turn.binding.TurnID+"\x00") {
			cancel()
		}
	}
	b.mu.Unlock()
	return b.waitForDispatches(b.ctx, turn)
}

func (b *Bridge) waitForDispatches(ctx context.Context, turn *activeTurn) error {
	quiescent := make(chan struct{})
	go func() {
		turn.inFlight.Wait()
		close(quiescent)
	}()
	select {
	case <-quiescent:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return b.closedError()
	}
}

func (b *Bridge) cancelDispatches(binding TurnBinding) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, cancel := range b.dispatches {
		if strings.HasPrefix(key, binding.SessionID+"\x00"+binding.TurnID+"\x00") {
			cancel()
		}
	}
}

func (b *Bridge) request(ctx context.Context, method string, params any, out any) error {
	b.mu.Lock()
	if !b.started || b.ctx == nil {
		b.mu.Unlock()
		return errors.New("dsh bridge has not started")
	}
	b.mu.Unlock()
	id := fmt.Sprintf("go-%d", b.nextID.Add(1))
	ch := make(chan rpcResponse, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()
	if err := b.writeFrame(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return err
	}
	select {
	case response := <-ch:
		if response.err != nil {
			return response.err
		}
		if out == nil {
			return nil
		}
		if len(response.result) == 0 || string(response.result) == "null" {
			return errors.New("DSH returned an empty JSON-RPC result")
		}
		if err := json.Unmarshal(response.result, out); err != nil {
			return fmt.Errorf("decode DSH %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return ctx.Err()
	case <-b.done:
		return b.closedError()
	}
}

func (b *Bridge) writeFrame(frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(data) > maxFrameBytes {
		return errors.New("JSON-RPC frame exceeds the configured size limit")
	}
	data = append(data, '\n')
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	for len(data) > 0 {
		n, writeErr := b.conn.Write(data)
		if writeErr != nil {
			return writeErr
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (b *Bridge) sendResult(id json.RawMessage, result any) {
	if err := b.writeFrame(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		b.fail(err)
	}
}

func (b *Bridge) sendError(id json.RawMessage, code int, message string) {
	if err := b.writeFrame(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: message}}); err != nil {
		b.fail(err)
	}
}

func (b *Bridge) sendToolError(id json.RawMessage, message string) {
	block, _ := json.Marshal(map[string]string{"type": "text", "text": message})
	b.sendResult(id, ToolResult{Content: []json.RawMessage{block}, IsError: true})
}

func (b *Bridge) fail(err error) {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.lastErr = err
		cancel := b.cancel
		pending := b.pending
		b.pending = make(map[string]chan rpcResponse)
		for _, cancelDispatch := range b.dispatches {
			cancelDispatch()
		}
		b.dispatches = make(map[string]context.CancelFunc)
		close(b.done)
		b.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		_ = b.conn.Close()
		for _, ch := range pending {
			select {
			case ch <- rpcResponse{err: err}:
			default:
			}
		}
	})
}

func (b *Bridge) closedError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lastErr != nil {
		return b.lastErr
	}
	return errors.New("dsh bridge is closed")
}

func validateBinding(binding TurnBinding) error {
	if strings.TrimSpace(binding.SessionID) == "" || strings.TrimSpace(binding.TurnID) == "" ||
		strings.TrimSpace(binding.TargetBindingHash) == "" || strings.TrimSpace(binding.ToolSurfaceFingerprint) == "" ||
		strings.TrimSpace(binding.PermissionSnapshotHash) == "" {
		return errors.New("Go turn binding requires session, turn, target, tool surface, and permission snapshot identities")
	}
	return nil
}

func validObjectSchema(raw json.RawMessage) bool {
	var schema struct {
		Type string `json:"type"`
	}
	return len(raw) > 0 && json.Unmarshal(raw, &schema) == nil && schema.Type == "object"
}

func validJSONObject(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value != nil
}

func validToolResult(result ToolResult) bool {
	if result.Content == nil {
		return false
	}
	for _, raw := range result.Content {
		var block struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &block) != nil || block.Type == "" {
			return false
		}
	}
	return true
}

func hasRequiredCapabilities(capabilities []string) bool {
	required := map[string]bool{
		"session/cancel": false, "session/replay": false,
		"assistant.stream": false, "aiops.tool.dispatch": false,
	}
	for _, capability := range capabilities {
		if _, exists := required[capability]; exists {
			required[capability] = true
		}
	}
	for _, found := range required {
		if !found {
			return false
		}
	}
	return true
}

func dispatchKey(sessionID, turnID, callID string) string {
	return sessionID + "\x00" + turnID + "\x00" + callID
}
