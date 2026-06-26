package promptcompiler

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Eino Format Conversion — CompiledPrompt → Eino *schema.Message format
// ---------------------------------------------------------------------------

// CompileForEino compiles and converts to Eino *schema.Message format,
// suitable for adk.ChatModelAgent's Instruction field.
// Produces one system message per section in the compiled envelope.
//
// Content is preserved exactly (round-trip semantic preservation).
func (c *PromptCompilerImpl) CompileForEino(ctx CompileContext) ([]*schema.Message, error) {
	compiled, err := c.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile for eino: %w", err)
	}

	return CompiledPromptToMessages(compiled), nil
}

// CompiledPromptToMessages converts a CompiledPrompt to Eino *schema.Message format.
// Each envelope section becomes a system message. Content is preserved exactly.
func CompiledPromptToMessages(compiled CompiledPrompt) []*schema.Message {
	if len(compiled.Envelope.Sections) > 0 {
		messages := make([]*schema.Message, 0, len(compiled.Envelope.Sections))
		for _, section := range compiled.Envelope.Sections {
			content := strings.TrimSpace(section.Content)
			if content == "" {
				continue
			}
			msg := semanticPromptMessage(content, section.ID, section.Source)
			msg.Extra["prompt_section_id"] = section.ID
			msg.Extra["prompt_layer"] = section.ID
			msg.Extra["semantic_role"] = firstNonEmptyEnvelopeString(section.Source, section.ID)
			messages = append(messages, msg)
		}
		return messages
	}
	messages := make([]*schema.Message, 0, 4)

	if system := compiled.effectiveSystemPrompt(); system.Content != "" {
		messages = append(messages, semanticPromptMessage(system.Content, "system", "system"))
	}

	if developer := compiled.effectiveDeveloperInstructions(); developer.Content != "" {
		messages = append(messages, semanticPromptMessage(developer.Content, "developer", "developer"))
	}

	if tools := compiled.effectiveToolPromptSet(); tools.Content != "" {
		messages = append(messages, semanticPromptMessage(tools.Content, "tool_index", "tool"))
	}

	if dynamicContent := compiled.effectiveDynamicContextContent(); dynamicContent != "" {
		messages = append(messages, semanticPromptMessage(dynamicContent, "dynamic_prompt", "runtime_context"))
	}

	if policy := compiled.effectiveRuntimePolicyPrompt(); policy.Content != "" {
		messages = append(messages, semanticPromptMessage(policy.Content, "runtime_policy", "context"))
	}

	return messages
}

func firstNonEmptyEnvelopeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func semanticPromptMessage(content, layer, semanticRole string) *schema.Message {
	msg := schema.SystemMessage(content)
	msg.Extra = map[string]any{
		"prompt_layer":  layer,
		"semantic_role": semanticRole,
	}
	return msg
}

func (c CompiledPrompt) effectiveSystemPrompt() SystemPrompt {
	if c.System.Content != "" || c.System.Role != "" || c.System.Environment != "" {
		return c.System
	}
	return c.Stable.System
}

func (c CompiledPrompt) effectiveDeveloperInstructions() DeveloperInstructions {
	if c.Developer.Content != "" || len(c.Developer.Constraints) > 0 {
		return c.Developer
	}
	return c.Stable.Developer
}

func (c CompiledPrompt) effectiveToolPromptSet() ToolPromptSet {
	if c.Tools.Content != "" || len(c.Tools.Entries) > 0 {
		return c.Tools
	}
	return c.Stable.Tools
}

func (c CompiledPrompt) effectiveRuntimePolicyPrompt() RuntimePolicyPrompt {
	if c.Policy.Content != "" || c.Policy.Mode != "" {
		return c.Policy
	}
	return c.Dynamic.Policy
}

func (c CompiledPrompt) effectiveDynamicContextContent() string {
	content := strings.TrimSpace(c.Dynamic.Content)
	if content == "" {
		return ""
	}
	policyContent := strings.TrimSpace(c.Policy.Content)
	if policyContent == "" {
		policyContent = strings.TrimSpace(c.Dynamic.Policy.Content)
	}
	if policyContent != "" && strings.HasSuffix(content, policyContent) {
		content = strings.TrimSpace(strings.TrimSuffix(content, policyContent))
	}
	return content
}
