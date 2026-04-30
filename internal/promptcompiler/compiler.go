package promptcompiler

import (
	"fmt"
	"strings"
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
	stableDeveloper := c.buildStableDeveloperInstructions(ctx)

	tools, err := c.buildToolPromptSet(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile tool prompt set: %w", err)
	}
	toolDelta := c.buildToolPromptDelta(ctx)

	policy, err := c.buildRuntimePolicyPrompt(ctx)
	if err != nil {
		return CompiledPrompt{}, fmt.Errorf("compile runtime policy prompt: %w", err)
	}

	stableContent := joinNonEmpty(system.Content, stableDeveloper.Content, tools.Content)
	dynamicParts := dynamicPromptFragments(ctx)
	if toolDelta.Content != "" {
		dynamicParts = append(dynamicParts, toolDelta.Content)
	}
	protocolState := normalizeProtocolState(ctx.ProtocolState)
	if protocolContent := renderProtocolPromptState(protocolState); protocolContent != "" {
		dynamicParts = append(dynamicParts, protocolContent)
	}
	dynamicContent := joinNonEmpty(append(dynamicParts, policy.Content)...)

	return CompiledPrompt{
		Stable: StablePromptEnvelope{
			Content:   stableContent,
			System:    system,
			Developer: stableDeveloper,
			Tools:     tools,
		},
		Dynamic: DynamicPromptDelta{
			Content:           dynamicContent,
			SkillPromptAssets: append([]string(nil), ctx.SkillPromptAssets...),
			EvidenceReminders: append([]string(nil), ctx.EvidenceReminders...),
			ExtraSections:     clonePromptSections(ctx.ExtraSections),
			ToolDelta:         toolDelta,
			ProtocolState:     protocolState,
			Policy:            policy,
		},
		System:    system,
		Developer: developer,
		Tools:     tools,
		Policy:    policy,
	}, nil
}

func joinNonEmpty(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "\n\n")
}

func clonePromptSections(sections []PromptSection) []PromptSection {
	if len(sections) == 0 {
		return nil
	}
	out := make([]PromptSection, 0, len(sections))
	for _, section := range sections {
		if strings.TrimSpace(section.Title) == "" && strings.TrimSpace(section.Content) == "" {
			continue
		}
		out = append(out, PromptSection{
			Title:   section.Title,
			Content: section.Content,
		})
	}
	return out
}

// CompileForEino is defined in eino_format.go — it compiles and converts
// to Eino Message format for adk.ChatModelAgent's Instruction field.
