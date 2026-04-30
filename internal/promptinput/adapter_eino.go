package promptinput

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// MessagesToSchema converts promptinput messages into Eino provider messages.
func MessagesToSchema(messages []Message) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		converted, err := messageToSchema(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func messageToSchema(msg Message) (*schema.Message, error) {
	switch msg.Role {
	case "system":
		return schema.SystemMessage(msg.Content), nil
	case "user":
		return schema.UserMessage(msg.Content), nil
	case "assistant":
		return schema.AssistantMessage(msg.Content, schemaToolCallsFromMessages(msg.ToolCalls)), nil
	case "tool":
		toolCallID := ""
		if msg.ToolResult != nil {
			toolCallID = msg.ToolResult.ToolCallID
		}
		return schema.ToolMessage(msg.Content, toolCallID), nil
	default:
		return nil, fmt.Errorf("unsupported promptinput message role %q", msg.Role)
	}
}

func schemaToolCallsFromMessages(toolCalls []ToolCall) []schema.ToolCall {
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
