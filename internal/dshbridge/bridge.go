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
	turn := &activeTurn{binding: binding, startSeq: cursor, done: make(chan struct{}), calls: make(map[string]struct{})}
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

// WaitTurn waits for DSH idle and then a durable turn/end event. Idle and the
// prompt receipt alone never establish business completion.
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
	if b.isTerminal(turn) {
		return nil
	}
	var ignored map[string]any
	if err := b.request(ctx, "session/wait", map[string]string{"sessionId": binding.SessionID}, &ignored); err != nil {
		return err
	}
	select {
	case <-turn.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return b.closedError()
	}
}

// CancelTurn cancels the exact host-bound turn and first cancels any local
// Go tool work so a remote cancellation cannot race past a still-running tool.
func (b *Bridge) CancelTurn(ctx context.Context, binding TurnBinding) error {
	turn, err := b.activeTurn(binding)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if turn.canceling {
		b.mu.Unlock()
		return errors.New("Go turn cancellation is already in progress")
	}
	turn.canceling = true
	for key, cancel := range b.dispatches {
		if strings.HasPrefix(key, binding.SessionID+"\x00"+binding.TurnID+"\x00") {
			cancel()
		}
	}
	b.mu.Unlock()
	var result struct {
		Accepted bool `json:"accepted"`
	}
	requestErr := b.request(ctx, "session/cancel", map[string]string{
		"sessionId": binding.SessionID, "turnId": binding.TurnID,
	}, &result)
	if err := b.waitForDispatches(ctx, turn); err != nil {
		return err
	}
	if requestErr != nil {
		return requestErr
	}
	if !result.Accepted {
		return errors.New("DSH did not accept cancellation for the bound Go turn")
	}
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
	turn := &activeTurn{binding: binding, startSeq: cursor, done: make(chan struct{}), calls: make(map[string]struct{})}
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
