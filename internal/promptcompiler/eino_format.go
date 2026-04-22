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

	// Layer 1: System Prompt as system message
	if system := compiled.effectiveSystemPrompt(); system.Content != "" {
		messages = append(messages, schema.SystemMessage(system.Content))
	}

	// Layer 2: Developer Instructions as system message
	if developer := compiled.effectiveDeveloperInstructions(); developer.Content != "" {
		messages = append(messages, schema.SystemMessage(developer.Content))
	}

	// Layer 3: Tool Prompt Set as system message
	if tools := compiled.effectiveToolPromptSet(); tools.Content != "" {
		messages = append(messages, schema.SystemMessage(tools.Content))
	}

	// Layer 4: Runtime Policy Prompt as system message
	if policy := compiled.effectiveRuntimePolicyPrompt(); policy.Content != "" {
		messages = append(messages, schema.SystemMessage(policy.Content))
	}

	return messages
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
