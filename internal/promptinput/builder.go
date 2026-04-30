package promptinput

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
)

// Build converts compiled prompt fragments plus current-turn conversation
// context into provider model messages and a semantic prompt-input trace.
func (Builder) Build(req BuildRequest) (BuildResult, error) {
	promptMessages := promptcompiler.CompiledPromptToMessages(req.Compiled)
	memoryMessages := memoryMessages(req)

	history := MessagesForCurrentTurnModelInput(req.History)
	runtimeMessages, err := MessagesToSchema(history)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}

	resultMessages := make([]*schema.Message, 0, len(promptMessages)+len(memoryMessages)+len(runtimeMessages))
	resultMessages = append(resultMessages, promptMessages...)
	resultMessages = append(resultMessages, memoryMessages...)
	resultMessages = append(resultMessages, runtimeMessages...)
	return BuildResult{
		Messages: resultMessages,
		Trace:    buildTrace(req, promptMessages, memoryMessagesFromRequest(req), history, runtimeMessages),
	}, nil
}

func memoryMessages(req BuildRequest) []*schema.Message {
	memories := memoryMessagesFromRequest(req)
	out := make([]*schema.Message, 0, len(memories))
	for _, item := range memories {
		msg := schema.SystemMessage("Memory: " + item.Text)
		msg.Extra = map[string]any{"prompt_layer": "memory", "semantic_role": "memory", "memory_id": item.ID, "memory_scope": item.Scope}
		out = append(out, msg)
	}
	return out
}

func memoryMessagesFromRequest(req BuildRequest) []MemoryItem {
	limit := req.MaxMemories
	if limit <= 0 || limit > 3 {
		limit = 3
	}
	if len(req.Memories) <= limit {
		return append([]MemoryItem(nil), req.Memories...)
	}
	return append([]MemoryItem(nil), req.Memories[:limit]...)
}

// MessagesForCurrentTurnModelInput preserves prior stable conversation messages
// while dropping old tool-call/result noise before the latest user turn.
func MessagesForCurrentTurnModelInput(history []Message) []Message {
	lastUserIndex := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			lastUserIndex = i
			break
		}
	}
	if lastUserIndex <= 0 {
		return append([]Message(nil), history...)
	}

	out := make([]Message, 0, len(history))
	for i, msg := range history {
		if i >= lastUserIndex {
			out = append(out, msg)
			continue
		}
		if isStableConversationMessage(msg) {
			out = append(out, msg)
		}
	}
	return out
}

func isStableConversationMessage(msg Message) bool {
	switch msg.Role {
	case "system", "user":
		return strings.TrimSpace(msg.Content) != ""
	case "assistant":
		return len(msg.ToolCalls) == 0 && strings.TrimSpace(msg.Content) != ""
	default:
		return false
	}
}
