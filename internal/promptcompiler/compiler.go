package promptcompiler

import (
	"fmt"
)

// ---------------------------------------------------------------------------
// PromptCompilerImpl implements the Compiler interface.
// ---------------------------------------------------------------------------

// PromptCompilerImpl is the concrete implementation of the PromptCompiler.
// It is the unique prompt truth source, compiling structured inputs into
// a four-layer CompiledPrompt.
//
// Layer compilation rules are defined in separate files:
//   - system_rules.go: Layer 1 (System Prompt)
//   - developer_rules.go: Layer 2 (Developer Instructions)
//   - tool_registry.go: Layer 3 (Tool Prompt Set)
//   - runtime_policy_prompt.go: Layer 4 (Runtime Policy Prompt)
type PromptCompilerImpl struct{}

// NewCompiler creates a new PromptCompilerImpl.
func NewCompiler() *PromptCompilerImpl {
	return &PromptCompilerImpl{}
}

// Compile compiles the four-layer prompt from the given context.
// Layer order: System Prompt → Developer Instructions → Tool Prompt Set → Runtime Policy Prompt.
func (c *PromptCompilerImpl) Compile(ctx CompileContext) (CompiledPrompt, error) {
	system, err := c.buildSystemPrompt(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile system prompt: %w", err)
	}

	developer, err := c.buildDeveloperInstructions(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile developer instructions: %w", err)
	}

	tools, err := c.buildToolPromptSet(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile tool prompt set: %w", err)
	}

	policy, err := c.buildRuntimePolicyPrompt(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile runtime policy prompt: %w", err)
	}

	return CompiledPrompt{
		System:    system,
		Developer: developer,
		Tools:     tools,
		Policy:    policy,
	}, nil
}

// CompileForEino is defined in eino_format.go — it compiles and converts
// to Eino Message format for adk.ChatModelAgent's Instruction field.
