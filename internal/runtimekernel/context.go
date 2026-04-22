package runtimekernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/spanstream"
)

// ---------------------------------------------------------------------------
// Context assembly and trimming for the RuntimeKernel.
// Manages the ContextWindow to ensure UsedTokens does not exceed MaxTokens.
// Trimming preserves the most recent messages (priority: newest first).
// ---------------------------------------------------------------------------

// DefaultMaxTokens is the default context window size if not configured.
const DefaultMaxTokens = 128000

// ContextCompactionPlan describes the split between the compactable prefix and
// the retained suffix of a message history.
type ContextCompactionPlan struct {
	Compacted    bool
	Compactable  []Message
	Retained     []Message
	TrimmedCount int
}

// ContextPipelineOptions controls compaction-time behavior for a turn iteration.
type ContextPipelineOptions struct {
	SessionID        string
	TurnID           string
	Iteration        int
	Compressor       *spanstream.ContextCompressor
	PendingApprovals []PendingApproval
	PendingEvidence  []PendingEvidence
}

// ContextPipelineResult contains the compacted view that is safe to show the model.
type ContextPipelineResult struct {
	Messages           []Message
	CompactedSegments  []CompactedSegment
	ExternalReferences []ExternalReference
}

// EstimateTokens provides a rough token estimate for a message.
// In production, this would use a proper tokenizer (tiktoken, etc.).
// For now, we use a simple heuristic: ~4 chars per token.
func EstimateTokens(msg Message) int {
	chars := len(msg.Content)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Name) + len(tc.Arguments)
	}
	if msg.ToolResult != nil {
		chars += len(msg.ToolResult.Content)
	}
	tokens := chars / 4
	if tokens < 1 && chars > 0 {
		tokens = 1
	}
	return tokens
}

// TrimContext ensures the context window stays within MaxTokens by removing
// the oldest messages first. The most recent messages are preserved with
// highest priority.
//
// After trimming:
//   - ContextWindow.UsedTokens <= ContextWindow.MaxTokens
//   - ContextWindow.Messages reflects the actual message count
//   - ContextWindow.TruncatedAt records how many messages were removed
func TrimContext(cw *ContextWindow, messages []Message) {
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	// Recalculate total tokens
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)

	// No trimming needed
	if totalTokens <= cw.MaxTokens {
		cw.TruncatedAt = 0
		return
	}

	// Trim from the front (oldest messages) until within budget
	trimmed := 0
	for totalTokens > cw.MaxTokens && trimmed < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[trimmed])
		trimmed++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - trimmed
	cw.TruncatedAt = trimmed
}

// AssembleContext creates a trimmed message slice from the session messages,
// respecting the context window limits. Returns the messages that fit within
// the window (most recent messages preserved).
func AssembleContext(cw *ContextWindow, messages []Message) []Message {
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	// Calculate total tokens
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}

	// If within budget, return all messages
	if totalTokens <= cw.MaxTokens {
		cw.UsedTokens = totalTokens
		cw.Messages = len(messages)
		cw.TruncatedAt = 0
		return messages
	}

	// Trim from front (oldest) to fit within budget
	startIdx := 0
	for totalTokens > cw.MaxTokens && startIdx < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[startIdx])
		startIdx++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - startIdx
	cw.TruncatedAt = startIdx

	return messages[startIdx:]
}

// SplitContextForCompaction prepares the context window for a compaction
// pipeline by returning the oldest messages that should be summarized and the
// newest messages that should remain in the live window.
//
// The returned plan mirrors TrimContext/AssembleContext token accounting, but
// exposes the compactable prefix so a caller can summarize it before
// reattaching the retained suffix.
func SplitContextForCompaction(cw *ContextWindow, messages []Message) ContextCompactionPlan {
	plan := ContextCompactionPlan{
		Compactable: append([]Message(nil), messages...),
		Retained:    append([]Message(nil), messages...),
	}

	if cw == nil {
		plan.Compactable = nil
		plan.TrimmedCount = 0
		plan.Compacted = false
		return plan
	}

	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)

	if totalTokens <= cw.MaxTokens {
		cw.TruncatedAt = 0
		plan.Compactable = nil
		plan.TrimmedCount = 0
		plan.Compacted = false
		return plan
	}

	startIdx := 0
	for totalTokens > cw.MaxTokens && startIdx < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[startIdx])
		startIdx++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - startIdx
	cw.TruncatedAt = startIdx

	plan.Compactable = append([]Message(nil), messages[:startIdx]...)
	plan.Retained = append([]Message(nil), messages[startIdx:]...)
	plan.TrimmedCount = startIdx
	plan.Compacted = startIdx > 0
	return plan
}

// ApplyContextPipeline upgrades the old trim-only behavior into a compaction-first
// pipeline. It preserves a hard-kept recent suffix, compacts an older prefix into
// a synthetic summary message, and only falls back to additional suffix trimming
// after the summary has been introduced.
func ApplyContextPipeline(ctx context.Context, cw *ContextWindow, messages []Message, opts ContextPipelineOptions) (ContextPipelineResult, error) {
	plan := SplitContextForCompaction(cw, messages)
	if !plan.Compacted {
		return ContextPipelineResult{Messages: plan.Retained}, nil
	}

	hardKeepCount := 4
	if len(opts.PendingApprovals) > 0 || len(opts.PendingEvidence) > 0 {
		hardKeepCount = 6
	}
	minRetained := hardKeepCount
	if len(messages) < minRetained {
		minRetained = len(messages)
	}
	if extra := minRetained - len(plan.Retained); extra > 0 && len(plan.Compactable) > 0 {
		if extra > len(plan.Compactable) {
			extra = len(plan.Compactable)
		}
		start := len(plan.Compactable) - extra
		plan.Retained = append(append([]Message(nil), plan.Compactable[start:]...), plan.Retained...)
		plan.Compactable = append([]Message(nil), plan.Compactable[:start]...)
		plan.TrimmedCount = len(plan.Compactable)
	}
	if len(plan.Compactable) == 0 {
		result := append([]Message(nil), plan.Retained...)
		recomputeContextWindow(cw, result)
		return ContextPipelineResult{Messages: result}, nil
	}

	refs := collectMessageReferences(plan.Compactable)
	summary := heuristicCompactionSummary(plan.Compactable)
	if opts.Compressor != nil {
		compressorMessages := make([]spanstream.Message, 0, len(plan.Compactable))
		for _, msg := range plan.Compactable {
			compressorMessages = append(compressorMessages, spanstream.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
		if compressed, err := opts.Compressor.Compress(ctx, nil, compressorMessages); err == nil && strings.TrimSpace(compressed) != "" {
			summary = compressed
		}
	}
	segmentID := fmt.Sprintf("cmp-%s-%d-%d", opts.TurnID, opts.Iteration, plan.TrimmedCount)
	summaryMsg := Message{
		ID:        segmentID + "-summary",
		Role:      "system",
		Content:   buildSummaryMessage(summary, refs),
		Timestamp: time.Now(),
	}
	summaryTokens := EstimateTokens(summaryMsg)
	if cw != nil && cw.MaxTokens > 0 && summaryTokens > cw.MaxTokens/3 && cw.MaxTokens > 16 {
		summaryMsg.Content = truncateForBudget(summaryMsg.Content, cw.MaxTokens/3)
	}

	retained := append([]Message(nil), plan.Retained...)
	resultMessages := append([]Message{summaryMsg}, retained...)
	if cw != nil {
		maxTokens := cw.MaxTokens
		if maxTokens <= 0 {
			maxTokens = DefaultMaxTokens
		}
		totalTokens := 0
		for _, msg := range resultMessages {
			totalTokens += EstimateTokens(msg)
		}
		for totalTokens > maxTokens && len(retained) > minRetained {
			totalTokens -= EstimateTokens(retained[0])
			retained = retained[1:]
		}
		resultMessages = append([]Message{summaryMsg}, retained...)
		recomputeContextWindow(cw, resultMessages)
		cw.TruncatedAt = plan.TrimmedCount
	}

	segment := CompactedSegment{
		ID:                 segmentID,
		SessionID:          opts.SessionID,
		TurnID:             opts.TurnID,
		Iteration:          opts.Iteration,
		StartIndex:         0,
		EndIndex:           plan.TrimmedCount - 1,
		Summary:            summary,
		ReferenceIDs:       referenceIDs(refs),
		ExternalReferences: refs,
		CreatedAt:          time.Now(),
	}
	return ContextPipelineResult{
		Messages:           resultMessages,
		CompactedSegments:  []CompactedSegment{segment},
		ExternalReferences: refs,
	}, nil
}

func recomputeContextWindow(cw *ContextWindow, messages []Message) {
	if cw == nil {
		return
	}
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)
	cw.TruncatedAt = 0
}

func collectMessageReferences(messages []Message) []ExternalReference {
	out := make([]ExternalReference, 0)
	seen := make(map[string]struct{})
	for _, msg := range messages {
		if msg.ToolResult == nil {
			continue
		}
		for _, ref := range msg.ToolResult.ExternalReferences {
			if ref.ID == "" {
				continue
			}
			if _, ok := seen[ref.ID]; ok {
				continue
			}
			seen[ref.ID] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}

func referenceIDs(refs []ExternalReference) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func heuristicCompactionSummary(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	first := messages[0]
	last := messages[len(messages)-1]
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Compressed %d earlier messages.", len(messages)))
	if snippet := summarizeSnippet(first.Content); snippet != "" {
		builder.WriteString(" Started with ")
		builder.WriteString(first.Role)
		builder.WriteString(": ")
		builder.WriteString(snippet)
		builder.WriteString(".")
	}
	if len(messages) > 1 {
		if snippet := summarizeSnippet(last.Content); snippet != "" {
			builder.WriteString(" Most recent compacted context was ")
			builder.WriteString(last.Role)
			builder.WriteString(": ")
			builder.WriteString(snippet)
			builder.WriteString(".")
		}
	}
	return builder.String()
}

func summarizeSnippet(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 120
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func buildSummaryMessage(summary string, refs []ExternalReference) string {
	if len(refs) == 0 {
		return "Earlier context summary: " + summary
	}
	return fmt.Sprintf("Earlier context summary: %s Refer to external refs: %s.", summary, strings.Join(referenceIDs(refs), ", "))
}

func truncateForBudget(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return value[:maxChars]
	}
	return value[:maxChars-3] + "..."
}
