package promptinput

import (
	"fmt"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

// Build converts compiled prompt fragments plus current-turn conversation
// context into provider-neutral model input items and a semantic trace.
func (Builder) Build(req BuildRequest) (BuildResult, error) {
	promptItems := compiledPromptModelInputItems(req.Compiled)
	opsContextItems := opsContextModelInputItems(req)
	memoryItems := memoryModelInputItems(req)
	history := MessagesForCurrentTurnModelInput(req.History)
	runtimeItems, err := MessagesToModelInputItems(history)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}

	resultItems := make([]ModelInputItem, 0, len(promptItems)+len(opsContextItems)+len(memoryItems)+len(runtimeItems))
	resultItems = append(resultItems, promptItems...)
	resultItems = append(resultItems, opsContextItems...)
	resultItems = append(resultItems, memoryItems...)
	resultItems = append(resultItems, runtimeItems...)
	for i := range resultItems {
		if err := resultItems[i].Validate(); err != nil {
			return BuildResult{}, fmt.Errorf("model input item[%d]: %w", i, err)
		}
	}
	return BuildResult{
		Items: resultItems,
		Trace: buildTrace(req, resultItems, memoryMessagesFromRequest(req), history),
	}, nil
}

func compiledPromptModelInputItems(compiled promptcompiler.CompiledPrompt) []ModelInputItem {
	sections := compiled.Envelope.Sections
	if len(sections) == 0 {
		sections = fallbackCompiledPromptSections(compiled)
	}
	out := make([]ModelInputItem, 0, len(sections))
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		role := ProviderRoleSystem
		if section.Role == "developer" {
			role = ProviderRoleDeveloper
		}
		layer := firstNonBlankPromptInputString(section.Layer, section.ID)
		out = append(out, ModelInputItem{
			ID:           firstNonBlankPromptInputString(section.ID, section.Layer, section.Source),
			ProviderRole: role,
			SemanticRole: firstNonBlankPromptInputString(layer, section.ID, section.Source),
			Content:      content,
			Source: ModelInputSource{
				Layer:     layer,
				SectionID: section.ID,
				Origin:    promptSource(layer),
			},
			Phase:      "prompt",
			CacheGroup: firstNonBlankPromptInputString(section.Stability, "stable"),
			Metadata: map[string]string{
				"prompt_layer":      layer,
				"prompt_section_id": section.ID,
			},
		})
	}
	return out
}

func fallbackCompiledPromptSections(compiled promptcompiler.CompiledPrompt) []promptcompiler.PromptCompiledSection {
	out := make([]promptcompiler.PromptCompiledSection, 0, 5)
	if content := strings.TrimSpace(compiled.System.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "system", Layer: "system", Role: "system", Content: content, Stability: "stable", Source: "system"})
	}
	if content := strings.TrimSpace(compiled.Developer.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "developer", Layer: "developer", Role: "developer", Content: content, Stability: "stable", Source: "developer"})
	}
	if content := strings.TrimSpace(compiled.Tools.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "tool_index", Layer: "tool_index", Role: "system", Content: content, Stability: "stable", Source: "tool"})
	}
	if content := strings.TrimSpace(compiled.Dynamic.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "dynamic_prompt", Layer: "dynamic_prompt", Role: "system", Content: content, Stability: "dynamic", Source: "runtime_context"})
	}
	if content := strings.TrimSpace(compiled.Policy.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "runtime_policy", Layer: "runtime_policy", Role: "system", Content: content, Stability: "dynamic", Source: "context"})
	}
	return out
}

func opsContextModelInputItems(req BuildRequest) []ModelInputItem {
	capsule := strings.TrimSpace(req.OpsContextCapsule)
	if capsule == "" {
		return nil
	}
	return []ModelInputItem{{
		ID:           "ops-context-capsule",
		ProviderRole: ProviderRoleSystem,
		SemanticRole: "ops_context_capsule",
		Content:      "Ops context capsule:\n" + capsule,
		Source:       ModelInputSource{Layer: "ops_context_capsule", Origin: "ops_context"},
		Phase:        "context",
		CacheGroup:   "dynamic",
		Metadata:     map[string]string{"prompt_layer": "ops_context_capsule"},
	}}
}

func memoryModelInputItems(req BuildRequest) []ModelInputItem {
	memories := memoryMessagesFromRequest(req)
	out := make([]ModelInputItem, 0, len(memories))
	for _, item := range memories {
		out = append(out, ModelInputItem{
			ID:           "memory-" + item.ID,
			ProviderRole: ProviderRoleSystem,
			SemanticRole: "memory",
			Content:      "Memory: " + item.Text,
			Source:       ModelInputSource{Layer: "memory", MessageID: item.ID, Origin: "memory"},
			Phase:        "memory",
			CacheGroup:   "dynamic",
			Metadata:     map[string]string{"memory_id": item.ID, "memory_scope": item.Scope},
		})
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

func MessagesToModelInputItems(history []Message) ([]ModelInputItem, error) {
	out := make([]ModelInputItem, 0, len(history))
	for idx, msg := range history {
		item := ModelInputItem{
			ID:           fmt.Sprintf("history-%d", idx),
			ProviderRole: providerRoleFromConversationRole(msg.Role),
			SemanticRole: conversationSemanticRole(msg),
			Content:      msg.Content,
			Source:       ModelInputSource{Layer: "history", Origin: "conversation"},
			Phase:        "history",
			CacheGroup:   "dynamic",
		}
		for _, call := range msg.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, ModelInputToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		if msg.ToolResult != nil {
			item.ProviderRole = ProviderRoleTool
			item.ToolCallID = msg.ToolResult.ToolCallID
			item.ToolResult = &ModelInputToolResult{ToolCallID: msg.ToolResult.ToolCallID, Content: msg.ToolResult.Content}
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func providerRoleFromConversationRole(role string) ProviderRole {
	switch strings.TrimSpace(role) {
	case "system":
		return ProviderRoleSystem
	case "assistant":
		return ProviderRoleAssistant
	case "tool":
		return ProviderRoleTool
	default:
		return ProviderRoleUser
	}
}

func firstNonBlankPromptInputString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "model-input-item"
}
