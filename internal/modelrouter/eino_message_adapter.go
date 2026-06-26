package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

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
