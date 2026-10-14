package dshbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// New creates a bridge over one DSH process's stdio. The caller must send
// stdout to this protocol endpoint only; diagnostics belong on stderr.
func New(conn io.ReadWriteCloser, config Config) (*Bridge, error) {
	if conn == nil || strings.TrimSpace(config.CWD) == "" || strings.TrimSpace(config.Provider) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("dsh bridge requires a connection, cwd, provider, and model")
	}
	if config.Dispatcher == nil || config.Events == nil {
		return nil, errors.New("dsh bridge requires the Go dispatcher and durable event store")
	}
	if len(config.Tools) == 0 {
		return nil, errors.New("dsh bridge requires a non-empty Go tool surface")
	}
	toolMap := make(map[string]ToolSchema, len(config.Tools))
	for _, tool := range config.Tools {
		if !toolNamePattern.MatchString(tool.Name) || strings.TrimSpace(tool.Description) == "" || !validObjectSchema(tool.Parameters) {
			return nil, fmt.Errorf("invalid Go tool schema %q", tool.Name)
		}
		if _, exists := toolMap[tool.Name]; exists {
			return nil, fmt.Errorf("duplicate Go tool schema %q", tool.Name)
		}
		toolMap[tool.Name] = tool
	}
	return &Bridge{
		conn: conn, config: config, tools: toolMap, done: make(chan struct{}),
		pending: make(map[string]chan rpcResponse), active: make(map[string]*activeTurn),
		dispatches: make(map[string]context.CancelFunc),
	}, nil
}

// Start starts the reader and requires the pinned extension and all P0
// capabilities before any session prompt can be sent.
func (b *Bridge) Start(parent context.Context) error {
	if parent == nil {
		return errors.New("dsh bridge requires a startup context")
	}
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("dsh bridge already started")
	}
	b.started = true
	b.ctx, b.cancel = context.WithCancel(parent)
	b.mu.Unlock()
	go b.readLoop()
	go func() {
		select {
		case <-parent.Done():
			b.fail(parent.Err())
		case <-b.done:
		}
	}()

	params := map[string]any{
		"cwd": b.config.CWD, "provider": b.config.Provider, "model": b.config.Model,
		"aiopsBridgeVersion": ProtocolVersion, "aiopsTools": b.config.Tools,
	}
	if b.config.ReasoningEffort != "" {
		params["reasoningEffort"] = b.config.ReasoningEffort
	}
	if b.config.MaxTokens > 0 {
		params["maxTokens"] = b.config.MaxTokens
	}
	var result struct {
		BridgeVersion string   `json:"aiopsBridgeVersion"`
		Capabilities  []string `json:"aiopsCapabilities"`
	}
	if err := b.request(parent, "initialize", params, &result); err != nil {
		b.fail(err)
		return err
	}
	if result.BridgeVersion != ProtocolVersion || !hasRequiredCapabilities(result.Capabilities) {
		err := errors.New("DSH server did not negotiate the required AIOps bridge capabilities")
		b.fail(err)
		return err
	}
	return nil
}

// Prompt sends one host-owned turn. The receipt acknowledges queue admission
// only; WaitTurn requires a committed durable turn/end event.
func (b *Bridge) Prompt(ctx context.Context, binding TurnBinding, content []json.RawMessage) (PromptReceipt, error) {
	if err := validateBinding(binding); err != nil {
		return PromptReceipt{}, err
	}
	cursor, err := b.config.Events.Cursor(ctx, binding.SessionID)
	if err != nil {
		return PromptReceipt{}, err
	}
	turn := &activeTurn{
		binding: binding, startSeq: cursor, done: make(chan struct{}),
		approvalChanged: make(chan struct{}, 1), calls: make(map[string]struct{}),
		pendingCalls: make(map[string]pendingToolCall),
	}
	b.mu.Lock()
	if _, exists := b.active[binding.SessionID]; exists {
		b.mu.Unlock()
		return PromptReceipt{}, errors.New("DSH session already has an active Go turn")
	}
	b.active[binding.SessionID] = turn
	b.mu.Unlock()

	var receipt PromptReceipt
	params := promptParams{SessionID: binding.SessionID, TurnID: binding.TurnID, ContentBlocks: content}
	if err := b.request(ctx, "session/prompt", params, &receipt); err != nil {
		// The peer may have queued the prompt before the request context ended.
		// Keep the binding until replay or a durable terminal event resolves it.
		return PromptReceipt{}, err
	}
	if receipt.MessageID == "" {
		return PromptReceipt{}, errors.New("DSH returned an empty prompt receipt")
	}
	return receipt, nil
}

// WaitTurn waits for a durable turn/end event. Prompt receipts and idle
// notifications never establish business completion. A durable Go approval
// suspends this call so RuntimeKernel can return its existing blocked result.
func (b *Bridge) WaitTurn(ctx context.Context, binding TurnBinding) error {
	turn, err := b.activeTurn(binding)
	if err != nil {
		complete, lookupErr := b.config.Events.TurnComplete(ctx, binding.SessionID, binding.TurnID)
		if lookupErr != nil {
			return lookupErr
		}
		if complete {
			return nil
		}
		return err
	}
	for {
		b.mu.Lock()
		terminal := turn.terminal
		pendingApproval := len(turn.pendingCalls) > 0
		b.mu.Unlock()
		if terminal {
			return nil
		}
		if pendingApproval {
			return ErrTurnSuspendedForApproval
		}
		select {
		case <-turn.done:
			return nil
		case <-turn.approvalChanged:
			// Recheck the pending set. Resolution can race this notification.
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return b.closedError()
		}
	}
}

// ResolveToolCall delivers the result of Go's already-approved and settled
// tool execution to the original DSH request. The exact host session, turn,
// and call must match; a result cannot be replayed or supplied while cancelling.
func (b *Bridge) ResolveToolCall(ctx context.Context, binding TurnBinding, callID string, result ToolResult) error {
	if err := validateBinding(binding); err != nil {
		return err
	}
	if ctx == nil || strings.TrimSpace(callID) == "" || !validToolResult(result) {
		return errors.New("DSH approval resolution requires a context, call ID, and valid Go tool result")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	turn := b.active[binding.SessionID]
	if turn == nil || turn.binding.TurnID != binding.TurnID || turn.terminal {
		b.mu.Unlock()
		return errors.New("Go turn binding is not active in this DSH session")
	}
	if turn.canceling || turn.settling {
		b.mu.Unlock()
		return errors.New("cannot resolve a DSH tool call while its Go turn is cancelling or settling")
	}
	pending, exists := turn.pendingCalls[callID]
	if !exists || pending.resolving {
		b.mu.Unlock()
		return errors.New("DSH tool call has no pending Go approval")
	}
	pending.resolving = true
	turn.pendingCalls[callID] = pending
	b.mu.Unlock()
	if err := b.writeFrame(map[string]any{"jsonrpc": "2.0", "id": pending.rpcID, "result": result}); err != nil {
		b.fail(err)
		return err
	}
	b.mu.Lock()
	if current, ok := turn.pendingCalls[callID]; ok && current.resolving {
		delete(turn.pendingCalls, callID)
		b.notifyApprovalChangedLocked(turn)
	}
	b.mu.Unlock()
	return nil
}

// CancelTurn cancels the exact host-bound turn and first cancels any local
// Go tool work so a remote cancellation cannot race past a still-running tool.
func (b *Bridge) CancelTurn(ctx context.Context, binding TurnBinding) error {
	turn, err := b.activeTurn(binding)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if turn.cancelInProgress {
		b.mu.Unlock()
		return errors.New("Go turn cancellation is already in progress")
	}
	for _, pending := range turn.pendingCalls {
		if pending.resolving {
			b.mu.Unlock()
			return errors.New("cannot cancel a DSH turn while a Go approval result is being delivered")
		}
	}
	if len(turn.pendingCalls) > 0 {
		if _, ok := b.config.Dispatcher.(PendingApprovalCanceler); !ok {
			b.mu.Unlock()
			return errors.New("Go dispatcher cannot revoke the pending approval; refusing to cancel DSH first")
		}
	}
	turn.canceling = true
	turn.cancelInProgress = true
	for key, cancel := range b.dispatches {
		if strings.HasPrefix(key, binding.SessionID+"\x00"+binding.TurnID+"\x00") {
			cancel()
		}
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		turn.cancelInProgress = false
		b.mu.Unlock()
	}()
	if err := b.waitForDispatches(ctx, turn); err != nil {
		return err
	}
	b.mu.Lock()
	pendingCalls := make([]pendingToolCall, 0, len(turn.pendingCalls))
	for _, pending := range turn.pendingCalls {
		pendingCalls = append(pendingCalls, pending)
	}
	canceler, canCancelPending := b.config.Dispatcher.(PendingApprovalCanceler)
	if len(pendingCalls) > 0 && !canCancelPending {
		b.mu.Unlock()
		return errors.New("Go dispatcher cannot revoke the pending approval; refusing to cancel DSH first")
	}
	b.mu.Unlock()
	for _, pending := range pendingCalls {
		if err := canceler.CancelPendingApproval(ctx, binding, pending.call); err != nil {
			return fmt.Errorf("revoke pending Go approval before DSH cancellation: %w", err)
		}
	}
	var result struct {
		Accepted bool `json:"accepted"`
	}
	requestErr := b.request(ctx, "session/cancel", map[string]string{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
	}, &result)
	if requestErr != nil {
		b.fail(fmt.Errorf("DSH cancellation outcome is ambiguous after Go approval revocation: %w", requestErr))
		return requestErr
	}
	if !result.Accepted {
		b.mu.Lock()
		clear(turn.pendingCalls)
		b.notifyApprovalChangedLocked(turn)
		b.mu.Unlock()
		for _, pending := range pendingCalls {
			b.sendToolError(pending.rpcID, "Go runtime revoked this approval because the turn is cancelling.")
		}
		return errors.New("DSH did not accept cancellation for the bound Go turn")
	}
	b.mu.Lock()
	clear(turn.pendingCalls)
	b.notifyApprovalChangedLocked(turn)
	b.mu.Unlock()
	return nil
}

// RecoverTurn rebinds the persisted Go turn, then replays DSH's durable tail
// after the last atomically committed host cursor. It never replays mutations.
func (b *Bridge) RecoverTurn(ctx context.Context, binding TurnBinding) error {
	if err := validateBinding(binding); err != nil {
		return err
	}
	cursor, err := b.config.Events.Cursor(ctx, binding.SessionID)
	if err != nil {
		return err
	}
	turn := &activeTurn{
		binding: binding, startSeq: cursor, done: make(chan struct{}),
		approvalChanged: make(chan struct{}, 1), calls: make(map[string]struct{}),
		pendingCalls: make(map[string]pendingToolCall),
	}
	b.mu.Lock()
	if _, exists := b.active[binding.SessionID]; exists {
		b.mu.Unlock()
		return errors.New("cannot recover while another Go turn is active")
	}
	b.active[binding.SessionID] = turn
	b.mu.Unlock()
	complete, err := b.config.Events.TurnComplete(ctx, binding.SessionID, binding.TurnID)
	if err != nil {
		return err
	}
	if complete {
		b.markTerminal(turn)
		return nil
	}
	var result replayResult
	if err := b.request(ctx, "session/replay", map[string]any{
		"sessionId": binding.SessionID, "afterSeq": cursor,
	}, &result); err != nil {
		return err
	}
	if result.SessionID != binding.SessionID {
		return errors.New("DSH replay returned a different session identity")
	}
	for _, raw := range result.Events {
		var event DurableEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return fmt.Errorf("decode replayed DSH event: %w", err)
		}
		event.SessionID = binding.SessionID
		if err := b.commitEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// Close cancels in-flight host dispatches, pending requests, and the stdio
// transport. The process owner remains responsible for quiescing its Go tools.
func (b *Bridge) Close() error {
	b.fail(errors.New("dsh bridge closed"))
	return nil
}

func (b *Bridge) notifyApprovalChangedLocked(turn *activeTurn) {
	select {
	case turn.approvalChanged <- struct{}{}:
	default:
	}
}
