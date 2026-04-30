package promptcompiler

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Eino Format Conversion — CompiledPrompt → Eino *schema.Message format
// ---------------------------------------------------------------------------

// CompileForEino compiles and converts to Eino *schema.Message format,
// suitable for adk.ChatModelAgent's Instruction field.
// Produces exactly 4 system messages, one per layer, in order:
//   - Message[0]: System Prompt (Layer 1)
//   - Message[1]: Developer Instructions (Layer 2)
//   - Message[2]: Tool Prompt Set (Layer 3)
//   - Message[3]: Runtime Policy Prompt (Layer 4)
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
// Each layer becomes a system message. Content is preserved exactly.
func CompiledPromptToMessages(compiled CompiledPrompt) []*schema.Message {
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

	if policy := compiled.effectiveRuntimePolicyPrompt(); policy.Content != "" {
		messages = append(messages, semanticPromptMessage(policy.Content, "runtime_policy", "context"))
	}

	return messages
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
