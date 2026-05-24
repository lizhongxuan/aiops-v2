package runtimekernel

import (
	"sort"
	"time"
)

// ContextGovernanceLayer is the L1-L5 governance layer that made a decision.
type ContextGovernanceLayer string

const (
	ContextGovernanceLayerL1 ContextGovernanceLayer = "L1"
	ContextGovernanceLayerL2 ContextGovernanceLayer = "L2"
	ContextGovernanceLayerL3 ContextGovernanceLayer = "L3"
	ContextGovernanceLayerL4 ContextGovernanceLayer = "L4"
	ContextGovernanceLayerL5 ContextGovernanceLayer = "L5"
)

// ContextGovernanceEvent is a redaction-safe state event. It carries IDs,
// counts, thresholds, and user-facing status text, but not raw tool content.
type ContextGovernanceEvent struct {
	ID              string                  `json:"id"`
	Layer           ContextGovernanceLayer  `json:"layer"`
	Kind            string                  `json:"kind"`
	SessionID       string                  `json:"sessionId,omitempty"`
	TurnID          string                  `json:"turnId,omitempty"`
	Iteration       int                     `json:"iteration,omitempty"`
	ToolCallID      string                  `json:"toolCallId,omitempty"`
	ToolName        string                  `json:"toolName,omitempty"`
	Message         string                  `json:"message,omitempty"`
	Budget          ContextBudgetThresholds `json:"budget,omitempty"`
	ReferenceIDs    []string                `json:"referenceIds,omitempty"`
	CompactedIDs    []string                `json:"compactedIds,omitempty"`
	DroppedGroupIDs []string                `json:"droppedGroupIds,omitempty"`
	RetryAttempt    int                     `json:"retryAttempt,omitempty"`
	RetryMax        int                     `json:"retryMax,omitempty"`
	Timeout         bool                    `json:"timeout,omitempty"`
	CreatedAt       time.Time               `json:"createdAt,omitempty"`
}

// ContextGovernanceTraceItem is the minimal payload intended for prompt trace.
type ContextGovernanceTraceItem struct {
	ID              string                  `json:"id,omitempty"`
	Layer           ContextGovernanceLayer  `json:"layer"`
	Kind            string                  `json:"kind"`
	Message         string                  `json:"message,omitempty"`
	Budget          ContextBudgetThresholds `json:"budget,omitempty"`
	ReferenceIDs    []string                `json:"referenceIds,omitempty"`
	CompactedIDs    []string                `json:"compactedIds,omitempty"`
	DroppedGroupIDs []string                `json:"droppedGroupIds,omitempty"`
	RetryAttempt    int                     `json:"retryAttempt,omitempty"`
	RetryMax        int                     `json:"retryMax,omitempty"`
	Timeout         bool                    `json:"timeout,omitempty"`
	CreatedAt       time.Time               `json:"createdAt,omitempty"`
}

// TracePayload returns the redaction-safe trace representation of the event.
func (e ContextGovernanceEvent) TracePayload() ContextGovernanceTraceItem {
	return ContextGovernanceTraceItem{
		ID:              e.ID,
		Layer:           e.Layer,
		Kind:            e.Kind,
		Message:         e.Message,
		Budget:          e.Budget,
		ReferenceIDs:    append([]string(nil), e.ReferenceIDs...),
		CompactedIDs:    append([]string(nil), e.CompactedIDs...),
		DroppedGroupIDs: append([]string(nil), e.DroppedGroupIDs...),
		RetryAttempt:    e.RetryAttempt,
		RetryMax:        e.RetryMax,
		Timeout:         e.Timeout,
		CreatedAt:       e.CreatedAt,
	}
}

// BuildContextGovernanceEvent creates a canonical event with UTC time and
// stable copied slices.
func BuildContextGovernanceEvent(event ContextGovernanceEvent) ContextGovernanceEvent {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	event.ReferenceIDs = append([]string(nil), event.ReferenceIDs...)
	event.CompactedIDs = append([]string(nil), event.CompactedIDs...)
	event.DroppedGroupIDs = append([]string(nil), event.DroppedGroupIDs...)
	return event
}

// SortContextGovernanceEvents returns events in stable chronological order.
func SortContextGovernanceEvents(events []ContextGovernanceEvent) []ContextGovernanceEvent {
	out := append([]ContextGovernanceEvent(nil), events...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.ID < right.ID
		}
		if left.CreatedAt.IsZero() {
			return false
		}
		if right.CreatedAt.IsZero() {
			return true
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	return out
}
