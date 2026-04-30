package promptcompiler

import (
	"fmt"
	"strings"
)

func normalizeProtocolState(state ProtocolPromptState) ProtocolPromptState {
	if len(state.Items) == 0 {
		return ProtocolPromptState{}
	}
	items := make([]ProtocolPromptItem, 0, len(state.Items))
	for _, item := range state.Items {
		kind := strings.TrimSpace(item.Kind)
		text := strings.TrimSpace(item.Text)
		if kind == "" && text == "" {
			continue
		}
		items = append(items, ProtocolPromptItem{
			Kind:   kind,
			ID:     strings.TrimSpace(item.ID),
			Status: strings.TrimSpace(item.Status),
			Text:   text,
		})
	}
	return ProtocolPromptState{Items: items}
}

func renderProtocolPromptState(state ProtocolPromptState) string {
	if len(state.Items) == 0 {
		return ""
	}
	lines := []string{
		"## Protocol State",
		"Treat these as protocol-level state items, not conversational prose.",
	}
	for _, item := range state.Items {
		attrs := []string{}
		if item.Kind != "" {
			attrs = append(attrs, "kind="+item.Kind)
		}
		if item.ID != "" {
			attrs = append(attrs, "id="+item.ID)
		}
		if item.Status != "" {
			attrs = append(attrs, "status="+item.Status)
		}
		line := "- " + strings.Join(attrs, " ")
		if strings.TrimSpace(line) == "-" {
			line = "- item"
		}
		if item.Text != "" {
			line = fmt.Sprintf("%s: %s", line, item.Text)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
