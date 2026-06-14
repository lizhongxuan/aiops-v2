package runtimekernel

import (
	"fmt"
	"strings"
	"time"
)

type MicrocompactOptions struct {
	SessionID                  string
	TurnID                     string
	Iteration                  int
	KeepRecentGroups           int
	SmallContextMode           bool
	CreatedAt                  time.Time
	LargeInlineResultMinTokens int
	LargeInlineResultMinBytes  int64
	PendingEvidenceToolCallIDs []string
	ApprovalBlockerToolCallIDs []string
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
	var allToolIndexes []int
	var compactIndexes []int
	protectedToolCallIDs := microcompactProtectedToolCallIDs(opts)
	for i, msg := range result {
		if msg.ToolResult != nil {
			cp := *msg.ToolResult
			result[i].ToolResult = &cp
			allToolIndexes = append(allToolIndexes, i)
		}
	}
	recentKeepIndexes := recentToolResultIndexes(allToolIndexes, keep)
	for i, msg := range result {
		if _, recent := recentKeepIndexes[i]; recent {
			continue
		}
		if isCompactableToolResult(msg, opts, protectedToolCallIDs) {
			compactIndexes = append(compactIndexes, i)
		}
	}

	if len(compactIndexes) == 0 {
		return MicrocompactResult{Messages: result}
	}

	events := make([]ContextGovernanceEvent, 0, len(compactIndexes))
	for _, idx := range compactIndexes {
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

func isCompactableToolResult(msg Message, opts MicrocompactOptions, protectedToolCallIDs map[string]struct{}) bool {
	if msg.ToolResult == nil || msg.ToolResult.Error != "" {
		return false
	}
	if _, ok := protectedToolCallIDs[msg.ToolResult.ToolCallID]; ok {
		return false
	}
	if msg.ToolResult.Spilled || len(msg.ToolResult.ExternalReferences) > 0 {
		return true
	}
	return isLargeInlineToolResult(*msg.ToolResult, opts)
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

func isLargeInlineToolResult(result ToolResult, opts MicrocompactOptions) bool {
	if strings.TrimSpace(result.Content) == "" {
		return false
	}
	minTokens := opts.LargeInlineResultMinTokens
	if minTokens <= 0 {
		minTokens = 1500
	}
	minBytes := opts.LargeInlineResultMinBytes
	if minBytes <= 0 {
		minBytes = 6000
	}
	bytes := result.InlineBytes
	if bytes <= 0 {
		bytes = int64(len(result.Content))
	}
	return len(result.Content)/4 >= minTokens || bytes >= minBytes
}

func microcompactProtectedToolCallIDs(opts MicrocompactOptions) map[string]struct{} {
	protected := make(map[string]struct{})
	for _, id := range opts.PendingEvidenceToolCallIDs {
		if strings.TrimSpace(id) != "" {
			protected[id] = struct{}{}
		}
	}
	for _, id := range opts.ApprovalBlockerToolCallIDs {
		if strings.TrimSpace(id) != "" {
			protected[id] = struct{}{}
		}
	}
	return protected
}

func recentToolResultIndexes(indexes []int, keep int) map[int]struct{} {
	recent := make(map[int]struct{})
	if keep <= 0 {
		return recent
	}
	start := len(indexes) - keep
	if start < 0 {
		start = 0
	}
	for _, idx := range indexes[start:] {
		recent[idx] = struct{}{}
	}
	return recent
}
