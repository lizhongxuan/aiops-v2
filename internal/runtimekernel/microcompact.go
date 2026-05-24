package runtimekernel

import (
	"fmt"
	"strings"
	"time"
)

type MicrocompactOptions struct {
	SessionID        string
	TurnID           string
	Iteration        int
	KeepRecentGroups int
	SmallContextMode bool
	CreatedAt        time.Time
}

type MicrocompactResult struct {
	Messages []Message
	Events   []ContextGovernanceEvent
}

// MicrocompactMessages replaces old compactable tool result payloads with
// short model-visible snapshots while preserving the original message list
// shape. It does not mutate the input slice or delete session history.
func MicrocompactMessages(messages []Message, opts MicrocompactOptions) MicrocompactResult {
	keep := opts.KeepRecentGroups
	if keep <= 0 {
		if opts.SmallContextMode {
			keep = 2
		} else {
			keep = 5
		}
	}
	if opts.CreatedAt.IsZero() {
		opts.CreatedAt = time.Now().UTC()
	}

	result := append([]Message(nil), messages...)
	var toolIndexes []int
	for i, msg := range result {
		if msg.ToolResult != nil {
			cp := *msg.ToolResult
			result[i].ToolResult = &cp
		}
		if isCompactableToolResult(msg) {
			toolIndexes = append(toolIndexes, i)
		}
	}

	cutoff := len(toolIndexes) - keep
	if cutoff <= 0 {
		return MicrocompactResult{Messages: result}
	}

	events := make([]ContextGovernanceEvent, 0, cutoff)
	for pos, idx := range toolIndexes {
		if pos >= cutoff {
			continue
		}
		tr := result[idx].ToolResult
		refIDs := referenceIDsFromExternalReferences(tr.ExternalReferences)
		tr.Content = microcompactSnapshot(*tr, refIDs)
		result[idx].ToolResult = tr
		events = append(events, ContextGovernanceEvent{
			ID:           fmt.Sprintf("ctxgov-%s-%d-l3-%d", opts.TurnID, opts.Iteration, idx),
			Layer:        ContextGovernanceLayerL3,
			Kind:         "tool_result.microcompacted",
			SessionID:    opts.SessionID,
			TurnID:       opts.TurnID,
			Iteration:    opts.Iteration,
			ToolCallID:   tr.ToolCallID,
			ReferenceIDs: refIDs,
			CreatedAt:    opts.CreatedAt,
		})
	}
	return MicrocompactResult{Messages: result, Events: events}
}

func isCompactableToolResult(msg Message) bool {
	if msg.ToolResult == nil || msg.ToolResult.Error != "" {
		return false
	}
	return msg.ToolResult.Spilled || len(msg.ToolResult.ExternalReferences) > 0
}

func microcompactSnapshot(result ToolResult, refIDs []string) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = summarizeSnippet(result.Content)
	}
	if len(refIDs) == 0 {
		return "Old tool result compacted. Summary: " + summary + "."
	}
	return fmt.Sprintf("Old tool result compacted. Summary: %s. External refs: %s.", summary, strings.Join(refIDs, ", "))
}

func referenceIDsFromExternalReferences(refs []ExternalReference) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}
