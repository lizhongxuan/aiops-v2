package runtimekernel

// ---------------------------------------------------------------------------
// Context assembly and trimming for the RuntimeKernel.
// Manages the ContextWindow to ensure UsedTokens does not exceed MaxTokens.
// Trimming preserves the most recent messages (priority: newest first).
// ---------------------------------------------------------------------------

// DefaultMaxTokens is the default context window size if not configured.
const DefaultMaxTokens = 128000

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
