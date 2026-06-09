package runtimekernel

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
)

func TestContextUsageAnalyzerCategorizesModelInput(t *testing.T) {
	secretToolPayload := strings.Repeat("tool-secret-payload ", 80)
	compiled := promptcompiler.CompiledPrompt{
		Stable: promptcompiler.StablePromptEnvelope{
			System:    promptcompiler.SystemPrompt{Content: "system prompt"},
			Developer: promptcompiler.DeveloperInstructions{Content: "developer rules"},
			Tools:     promptcompiler.ToolPromptSet{Content: strings.Repeat("tool schema ", 20)},
		},
		Dynamic: promptcompiler.DynamicPromptDelta{
			SkillPromptAssets: []string{strings.Repeat("skill asset ", 10)},
			ExtraSections: []promptcompiler.PromptSection{{
				Title:   "MCP Resources",
				Content: strings.Repeat("resource index ", 12),
			}, {
				Title:   "Artifact References",
				Content: strings.Repeat("artifact ref ", 9),
			}},
			Policy: promptcompiler.RuntimePolicyPrompt{Content: "runtime policy"},
		},
		System:    promptcompiler.SystemPrompt{Content: "system prompt"},
		Developer: promptcompiler.DeveloperInstructions{Content: "developer rules"},
		Tools:     promptcompiler.ToolPromptSet{Content: strings.Repeat("tool schema ", 20)},
		Policy:    promptcompiler.RuntimePolicyPrompt{Content: "runtime policy"},
	}
	history := []Message{
		{Role: "user", Content: "current question"},
		{Role: "tool", Content: secretToolPayload, ToolResult: &ToolResult{ToolCallID: "call-large", Content: secretToolPayload}},
	}

	result, err := buildPromptInputWithContextGovernance(history, compiled, []ContextGovernanceEvent{BuildContextGovernanceEvent(ContextGovernanceEvent{
		Layer: "L4",
		Kind:  "buffer.warning",
		Budget: ContextBudgetThresholds{
			MaxContextTokens:     32000,
			ReservedOutputTokens: 4000,
		},
	})})
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}

	usage := result.Trace.ContextUsage
	if usage.MaxContextTokens != 32000 || usage.ReservedOutputTokens != 4000 {
		t.Fatalf("usage budget = %#v, want max/reserved from governance", usage)
	}
	for _, want := range []string{"system", "developer", "tools", "skills", "mcp", "messages", "tool_results", "artifacts", "buffers"} {
		if categoryTokens(usage, want) == 0 {
			t.Fatalf("category %q should be present with tokens, usage=%#v", want, usage)
		}
	}
	if usage.EstimatedInputTokens == 0 {
		t.Fatalf("estimated input tokens should be set: %#v", usage)
	}
	if len(usage.TopContributors) == 0 {
		t.Fatalf("expected top contributors: %#v", usage)
	}
	if usage.TopContributors[0].Kind != "tool_results" || usage.TopContributors[0].ID != "call-large" {
		t.Fatalf("top contributor = %#v, want tool result call-large", usage.TopContributors[0])
	}
	if strings.Contains(strings.ToLower(usage.TopContributors[0].Action), "tool-secret-payload") {
		t.Fatalf("top contributor leaked raw content: %#v", usage.TopContributors[0])
	}
}

func TestAnalyzeContextUsageDoesNotRequireRawSensitiveContributors(t *testing.T) {
	usage := AnalyzeContextUsage(ContextUsageInput{
		Messages: []*schema.Message{
			{Role: schema.User, Content: "user-secret-content"},
			{Role: schema.Tool, Content: strings.Repeat("tool-secret-content ", 20), ToolCallID: "call-1"},
		},
	})
	for _, contributor := range usage.TopContributors {
		if strings.Contains(contributor.ID, "secret-content") || strings.Contains(contributor.Action, "secret-content") {
			t.Fatalf("contributor leaked raw content: %#v", contributor)
		}
	}
}

func categoryTokens(usage ContextUsage, name string) int {
	for _, category := range usage.Categories {
		if category.Name == name {
			return category.TokensEstimate
		}
	}
	return 0
}
