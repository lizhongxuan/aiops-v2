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
	if compiled.System.Content != "" {
		messages = append(messages, schema.SystemMessage(compiled.System.Content))
	}

	// Layer 2: Developer Instructions as system message
	if compiled.Developer.Content != "" {
		messages = append(messages, schema.SystemMessage(compiled.Developer.Content))
	}

	// Layer 3: Tool Prompt Set as system message
	if compiled.Tools.Content != "" {
		messages = append(messages, schema.SystemMessage(compiled.Tools.Content))
	}

	// Layer 4: Runtime Policy Prompt as system message
	if compiled.Policy.Content != "" {
		messages = append(messages, schema.SystemMessage(compiled.Policy.Content))
	}

	return messages
}
