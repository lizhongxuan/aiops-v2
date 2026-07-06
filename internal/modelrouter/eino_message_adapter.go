package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/promptinput"
	"github.com/cloudwego/eino/schema"
)

func ModelInputItemsToEinoMessages(items []promptinput.ModelInputItem) ([]*schema.Message, ProviderMessageAudit, error) {
	messages := make([]*schema.Message, 0, len(items))
	auditItems := make([]ProviderMessageAuditItem, 0, len(items))
	for idx, item := range items {
		if err := item.Validate(); err != nil {
			return nil, ProviderMessageAudit{}, fmt.Errorf("item[%d]: %w", idx, err)
		}
		msg := einoMessageFromModelInputItem(item)
		messages = append(messages, msg)
		auditItems = append(auditItems, ProviderMessageAuditItem{
			ItemID:              item.ID,
			ProviderRole:        string(item.ProviderRole),
			ToolCallID:          firstNonEmptyString(item.ToolCallID, item.ToolResultToolCallID()),
			ItemHash:            item.StableHash(),
			ProviderMessageHash: stableProviderHash(msg),
		})
	}
	return messages, ProviderMessageAudit{
		ProviderMessagesHash: stableProviderHash(messages),
		Items:                auditItems,
	}, nil
}

func ProviderMessageAuditFromModelInputItems(items []promptinput.ModelInputItem) (ProviderMessageAudit, error) {
	_, audit, err := ModelInputItemsToEinoMessages(items)
	return audit, err
}

func EinoInstructionMessagesText(messages []*schema.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		role := strings.TrimSpace(string(msg.Role))
		if role != "" {
			builder.WriteString("[")
			builder.WriteString(role)
			builder.WriteString("]\n")
		}
		builder.WriteString(msg.Content)
	}
	return strings.TrimSpace(builder.String())
}

func ModelInputItemsFromEinoMessages(messages []*schema.Message) []promptinput.ModelInputItem {
	items := make([]promptinput.ModelInputItem, 0, len(messages))
	for idx, msg := range messages {
		if msg == nil {
			continue
		}
		item := promptinput.ModelInputItem{
			ID:               fmt.Sprintf("provider-message-%d", idx),
			ProviderRole:     providerRoleFromEino(msg.Role),
			SemanticRole:     semanticRoleFromEinoMessage(msg),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			Name:             firstNonEmptyString(msg.Name, msg.ToolName),
			ToolCallID:       msg.ToolCallID,
			Source:           promptinput.ModelInputSource{Layer: sourceLayerFromEinoMessage(msg), Origin: "provider_message"},
			Phase:            "trace",
			CacheGroup:       "dynamic",
			Metadata:         map[string]string{},
		}
		for _, call := range msg.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, promptinput.ModelInputToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: json.RawMessage(call.Function.Arguments),
			})
		}
		if item.ProviderRole == promptinput.ProviderRoleTool {
			item.ToolResult = &promptinput.ModelInputToolResult{
				ToolCallID: msg.ToolCallID,
				Content:    msg.Content,
			}
		}
		for key, value := range msg.Extra {
			if text, ok := value.(string); ok {
				item.Metadata[key] = text
			}
		}
		if len(item.Metadata) == 0 {
			item.Metadata = nil
		}
		items = append(items, item)
	}
	return items
}

func providerRoleFromEino(role schema.RoleType) promptinput.ProviderRole {
	switch role {
	case schema.System:
		return promptinput.ProviderRoleSystem
	case schema.Assistant:
		return promptinput.ProviderRoleAssistant
	case schema.Tool:
		return promptinput.ProviderRoleTool
	case schema.User:
		return promptinput.ProviderRoleUser
	default:
		return promptinput.ProviderRoleUser
	}
}

func semanticRoleFromEinoMessage(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Extra != nil {
		if role, ok := msg.Extra["semantic_role"].(string); ok {
			return strings.TrimSpace(role)
		}
	}
	switch msg.Role {
	case schema.Tool:
		return "tool_result"
	default:
		return strings.TrimSpace(string(msg.Role))
	}
}

func sourceLayerFromEinoMessage(msg *schema.Message) string {
	if msg == nil || msg.Extra == nil {
		return ""
	}
	for _, key := range []string{"source_layer", "prompt_layer"} {
		if layer, ok := msg.Extra[key].(string); ok && strings.TrimSpace(layer) != "" {
			return strings.TrimSpace(layer)
		}
	}
	return ""
}

func einoMessageFromModelInputItem(item promptinput.ModelInputItem) *schema.Message {
	content := item.Content
	if item.ProviderRole == promptinput.ProviderRoleTool && item.ToolResult != nil && item.ToolResult.Content != "" {
		content = item.ToolResult.Content
	}
	var msg *schema.Message
	switch item.ProviderRole {
	case promptinput.ProviderRoleSystem, promptinput.ProviderRoleDeveloper:
		msg = schema.SystemMessage(content)
	case promptinput.ProviderRoleUser:
		msg = schema.UserMessage(content)
	case promptinput.ProviderRoleAssistant:
		msg = schema.AssistantMessage(content, schemaToolCallsFromModelInput(item.ToolCalls))
		msg.ReasoningContent = item.ReasoningContent
	case promptinput.ProviderRoleTool:
		msg = schema.ToolMessage(content, firstNonEmptyString(item.ToolCallID, item.ToolResultToolCallID()))
	default:
		msg = schema.UserMessage(content)
	}
	if item.Name != "" {
		msg.Name = item.Name
	}
	if msg.Extra == nil {
		msg.Extra = map[string]any{}
	}
	msg.Extra["model_input_item_id"] = item.ID
	msg.Extra["semantic_role"] = item.SemanticRole
	msg.Extra["source_layer"] = item.Source.Layer
	msg.Extra["source_section_id"] = item.Source.SectionID
	msg.Extra["phase"] = item.Phase
	msg.Extra["cache_group"] = item.CacheGroup
	for key, value := range item.Metadata {
		if _, exists := msg.Extra[key]; !exists {
			msg.Extra[key] = value
		}
	}
	return msg
}

func schemaToolCallsFromModelInput(toolCalls []promptinput.ModelInputToolCall) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, schema.ToolCall{
			ID:   call.ID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      call.Name,
				Arguments: string(call.Arguments),
			},
		})
	}
	return out
}

func stableProviderHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
