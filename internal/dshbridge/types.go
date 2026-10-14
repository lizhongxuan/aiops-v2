package dshbridge

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
)

const (
	ProtocolVersion = "aiops-dsh-bridge.v1"
	maxFrameBytes   = 8 << 20
)

var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)

// ToolSchema is copied from the Go-assembled, policy-filtered tool surface.
// It is the only set of names the DSH agent can see or call.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// TurnBinding contains host-owned identity and immutable admission facts.
// None of these values are accepted from model arguments.
type TurnBinding struct {
	SessionID              string
	TurnID                 string
	TargetBindingHash      string
	ToolSurfaceFingerprint string
	PermissionSnapshotHash string
}

// ToolCall is a validated DSH request paired with its trusted host binding.
type ToolCall struct {
	CallID        string
	Name          string
	Arguments     json.RawMessage
	ArgumentsHash string
}

// ToolResult is returned to DSH only after the Go runtime has settled the call.
type ToolResult struct {
	Content []json.RawMessage `json:"content"`
	IsError bool              `json:"isError"`
}

// ToolDispatcher is the only execution seam for model-requested tools. A
// production implementation must route to RuntimeKernel/ToolDispatcher and
// must keep a pending approval open until Go has persisted and resumed the
// matching approval. Returning an approval prompt as a successful tool result
// is not supported.
type ToolDispatcher interface {
	Dispatch(context.Context, TurnBinding, ToolCall) (ToolResult, error)
}

// EventStore commits durable DSH session events idempotently by (session, seq).
// Cursor returns -1 for an empty session. TurnComplete must be derived from a
// committed turn/end event attributed to that exact Go turn.
type EventStore interface {
	Cursor(context.Context, string) (int64, error)
	Commit(context.Context, DurableEvent) error
	TurnComplete(context.Context, string, string) (bool, error)
}

// DurableEvent is an event from the DSH session log. Stream frames are not
// represented here and must not advance this cursor.
type DurableEvent struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId,omitempty"`
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	Time      int64           `json:"time"`
	Data      json.RawMessage `json:"data"`
}

type Callbacks struct {
	// AssistantStream receives transient frames in arrival order. A frame is
	// not durable and cannot complete a turn.
	AssistantStream func(sessionID, turnID string, frame json.RawMessage)
}

type Config struct {
	CWD             string
	Provider        string
	Model           string
	ReasoningEffort string
	MaxTokens       int
	Tools           []ToolSchema
	Dispatcher      ToolDispatcher
	Events          EventStore
	Callbacks       Callbacks
}

type PromptReceipt struct {
	MessageID string `json:"messageId"`
}

type Bridge struct {
	conn       io.ReadWriteCloser
	config     Config
	tools      map[string]ToolSchema
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	closeOnce  sync.Once
	writeMu    sync.Mutex
	mu         sync.Mutex
	nextID     atomic.Uint64
	pending    map[string]chan rpcResponse
	active     map[string]*activeTurn
	dispatches map[string]context.CancelFunc
	lastErr    error
	started    bool
}

type activeTurn struct {
	binding   TurnBinding
	startSeq  int64
	done      chan struct{}
	terminal  bool
	canceling bool
	settling  bool
	calls     map[string]struct{}
	inFlight  sync.WaitGroup
}

type rpcFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

type promptParams struct {
	SessionID     string            `json:"sessionId"`
	TurnID        string            `json:"turnId"`
	ContentBlocks []json.RawMessage `json:"contentBlocks"`
}

type toolDispatchParams struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	CallID    string          `json:"callId"`
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolCancelParams struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	CallID    string `json:"callId"`
}

type eventNotification struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Event     json.RawMessage `json:"event"`
}

type streamNotification struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Frame     json.RawMessage `json:"frame"`
}

type replayResult struct {
	SessionID string            `json:"sessionId"`
	Events    []json.RawMessage `json:"events"`
}
