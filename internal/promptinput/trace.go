package promptinput

import (
	"fmt"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
)

func buildTrace(req BuildRequest, promptMessages []*schema.Message, memories []MemoryItem, history []Message, runtimeMessages []*schema.Message) PromptInputTrace {
	items := make([]TraceItem, 0, len(promptMessages)+len(memories)+len(req.Compiled.Dynamic.ProtocolState.Items)+len(history))

	for _, msg := range promptMessages {
		if msg == nil {
			continue
		}
		promptLayer := stringExtra(msg.Extra, "prompt_layer")
		semanticRole := promptLayer
		if semanticRole == "" {
			semanticRole = stringExtra(msg.Extra, "semantic_role")
		}
		items = append(items, TraceItem{
			Source:       promptSource(promptLayer),
			SemanticRole: semanticRole,
			ProviderRole: string(msg.Role),
			PromptLayer:  promptLayer,
			Content:      msg.Content,
		})
	}

	for _, memory := range memories {
		items = append(items, TraceItem{
			Source:       "memory",
			SemanticRole: "memory",
			ProviderRole: string(schema.System),
			PromptLayer:  "memory",
			ID:           memory.ID,
			Content:      memory.Text,
		})
	}

	if req.OpsContextCapsule != "" {
		items = append(items, TraceItem{
			Source:       "ops_context",
			SemanticRole: "ops_context_capsule",
			ProviderRole: string(schema.System),
			PromptLayer:  "ops_context_capsule",
			Content:      req.OpsContextCapsule,
		})
	}

	for _, item := range req.Compiled.Dynamic.ProtocolState.Items {
		items = append(items, TraceItem{
			Source:       "protocol_state",
			SemanticRole: item.Kind,
			ID:           item.ID,
			Status:       item.Status,
			Content:      item.Text,
		})
	}

	for i, msg := range history {
		providerRole := msg.Role
		if i < len(runtimeMessages) && runtimeMessages[i] != nil {
			providerRole = string(runtimeMessages[i].Role)
		}
		items = append(items, TraceItem{
			Source:       "conversation",
			SemanticRole: conversationSemanticRole(msg),
			ProviderRole: providerRole,
			ID:           conversationTraceID(msg),
			Content:      msg.Content,
		})
	}

	return PromptInputTrace{
		Items:                  items,
		OpsContextCapsuleChars: len(req.OpsContextCapsule),
		SessionFactCount:       req.SessionFactCount,
		LettaHintCount:         req.LettaHintCount,
		MemoryItemCount:        len(memories),
		VisibleOpsManualTools:  visibleOpsManualTools(req.Tools),
		DroppedContextReasons:  append([]string(nil), req.DroppedContextReasons...),
		ContextGovernance:      cloneContextGovernanceTraceItems(req.ContextGovernance),
	}
}

func promptSource(promptLayer string) string {
	switch promptLayer {
	case "system", "developer", "tool_index":
		return "stable_prompt"
	case "runtime_policy":
		return "dynamic_prompt"
	default:
		return "prompt"
	}
}

func conversationSemanticRole(msg Message) string {
	switch msg.Role {
	case "assistant":
		if len(msg.ToolCalls) > 0 {
			return "assistant_tool_call"
		}
		return "assistant"
	case "tool":
		return "tool_result"
	default:
		return msg.Role
	}
}

func conversationTraceID(msg Message) string {
	if msg.ToolResult != nil && msg.ToolResult.ToolCallID != "" {
		return msg.ToolResult.ToolCallID
	}
	if len(msg.ToolCalls) == 1 {
		return msg.ToolCalls[0].ID
	}
	if len(msg.ToolCalls) > 1 {
		return fmt.Sprintf("%s+%d", msg.ToolCalls[0].ID, len(msg.ToolCalls)-1)
	}
	return ""
}

func stringExtra(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok {
		return ""
	}
	str, _ := value.(string)
	return str
}

func visibleOpsManualTools(tools []promptcompiler.Tool) []string {
	var out []string
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := tool.Metadata().Name
		switch name {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			out = append(out, name)
		}
	}
	return out
}

func cloneContextGovernanceTraceItems(items []ContextGovernanceTraceItem) []ContextGovernanceTraceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]ContextGovernanceTraceItem, 0, len(items))
	for _, item := range items {
		item.ReferenceIDs = append([]string(nil), item.ReferenceIDs...)
		if len(item.Budget) > 0 {
			budget := make(map[string]int, len(item.Budget))
			for key, value := range item.Budget {
				budget[key] = value
			}
			item.Budget = budget
		}
		out = append(out, item)
	}
	return out
}
