package spanstream

import (
	"encoding/json"
	"sync"
	"time"
)

// SpanType identifies the type of a span node.
type SpanType string

const (
	SpanTypeTurn      SpanType = "turn"
	SpanTypeToolCall  SpanType = "tool_call"
	SpanTypeSearch    SpanType = "search"
	SpanTypeFileRead  SpanType = "file_read"
	SpanTypeShellExec SpanType = "shell_exec"
	SpanTypeSummary   SpanType = "summary"
)

// SpanStatus represents the execution status of a span.
type SpanStatus string

const (
	SpanStatusRunning   SpanStatus = "running"
	SpanStatusCompleted SpanStatus = "completed"
	SpanStatusFailed    SpanStatus = "failed"
)

// Span represents a node in the conversation tree.
type Span struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parentId,omitempty"`
	Type      SpanType       `json:"type"`
	Status    SpanStatus     `json:"status"`
	Name      string         `json:"name"`
	Summary   string         `json:"summary,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	StartTime time.Time      `json:"startTime"`
	EndTime   *time.Time     `json:"endTime,omitempty"`
	Children  []*Span        `json:"children,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SpanTree is a thread-safe tree structure for managing conversation spans.
type SpanTree struct {
	RootSpan *Span `json:"rootSpan"`
	mu       sync.RWMutex
}

// NewSpanTree creates a new SpanTree with the given root span.
func NewSpanTree(root *Span) *SpanTree {
	return &SpanTree{RootSpan: root}
}

// AddChild adds a child span under the specified parent span ID.
// Returns false if the parent span is not found.
func (st *SpanTree) AddChild(parentID string, child *Span) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	parent := st.findSpanLocked(st.RootSpan, parentID)
	if parent == nil {
		return false
	}
	child.ParentID = parentID
	parent.Children = append(parent.Children, child)
	return true
}

// FindSpan locates a span by ID within the tree.
func (st *SpanTree) FindSpan(id string) *Span {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.findSpanLocked(st.RootSpan, id)
}

// CompleteSpan marks a span as completed with summary and detail.
func (st *SpanTree) CompleteSpan(id string, summary string, detail string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	span := st.findSpanLocked(st.RootSpan, id)
	if span == nil {
		return false
	}
	span.Status = SpanStatusCompleted
	span.Summary = summary
	span.Detail = detail
	now := time.Now()
	span.EndTime = &now
	return true
}

// FailSpan marks a span as failed with an error message in detail.
func (st *SpanTree) FailSpan(id string, errMsg string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	span := st.findSpanLocked(st.RootSpan, id)
	if span == nil {
		return false
	}
	span.Status = SpanStatusFailed
	span.Detail = errMsg
	now := time.Now()
	span.EndTime = &now
	return true
}

// MarshalJSON serializes the SpanTree to JSON.
func (st *SpanTree) MarshalJSON() ([]byte, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	type alias SpanTree
	return json.Marshal(&struct {
		*alias
	}{alias: (*alias)(st)})
}

// UnmarshalJSON deserializes the SpanTree from JSON.
func (st *SpanTree) UnmarshalJSON(data []byte) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	type alias SpanTree
	aux := &struct {
		*alias
	}{alias: (*alias)(st)}
	return json.Unmarshal(data, aux)
}

// findSpanLocked recursively searches for a span by ID. Caller must hold lock.
func (st *SpanTree) findSpanLocked(node *Span, id string) *Span {
	if node == nil {
		return nil
	}
	if node.ID == id {
		return node
	}
	for _, child := range node.Children {
		if found := st.findSpanLocked(child, id); found != nil {
			return found
		}
	}
	return nil
}
