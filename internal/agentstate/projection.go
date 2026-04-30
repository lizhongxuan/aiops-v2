package agentstate

import "strings"

// PromptItem is a small protocol-state projection that can later be adapted to
// promptcompiler.ProtocolPromptItem without making agentstate depend on prompts.
type PromptItem struct {
	Kind   string
	ID     string
	Status string
	Text   string
}

func ProjectPromptItems(state AgentState) []PromptItem {
	items := make([]PromptItem, 0, len(state.Items))
	for _, item := range state.Items {
		items = append(items, PromptItem{
			Kind:   string(item.Type),
			ID:     item.ID,
			Status: string(item.Status),
			Text:   strings.TrimSpace(item.Payload.Summary),
		})
	}
	return items
}
