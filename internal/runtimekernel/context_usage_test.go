package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
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
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-large", Name: "read_large_result"}}},
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
		Items: []promptinput.ModelInputItem{
			{ProviderRole: promptinput.ProviderRoleUser, Content: "user-secret-content"},
			{
				ProviderRole: promptinput.ProviderRoleTool,
				Content:      strings.Repeat("tool-secret-content ", 20),
				ToolCallID:   "call-1",
				ToolResult:   &promptinput.ModelInputToolResult{ToolCallID: "call-1", Content: strings.Repeat("tool-secret-content ", 20)},
			},
		},
	})
	for _, contributor := range usage.TopContributors {
		if strings.Contains(contributor.ID, "secret-content") || strings.Contains(contributor.Action, "secret-content") {
			t.Fatalf("contributor leaked raw content: %#v", contributor)
		}
	}
}

func TestContextUsageCategorizesHostTaskPromptAssetsSeparately(t *testing.T) {
	usage := AnalyzeContextUsage(ContextUsageInput{
		Compiled: promptcompiler.CompiledPrompt{
			Dynamic: promptcompiler.DynamicPromptDelta{
				SkillPromptAssets:    []string{strings.Repeat("skill capability ", 10)},
				HostTaskPromptAssets: []string{strings.Repeat("assigned host task ", 10)},
			},
		},
	})

	if categoryTokens(usage, "skills") == 0 {
		t.Fatalf("skills category should include skill assets: %#v", usage)
	}
	if categoryTokens(usage, "host_task") == 0 {
		t.Fatalf("host_task category should include host task prompt assets: %#v", usage)
	}
	for _, contributor := range usage.TopContributors {
		if contributor.Kind == "skills" && strings.Contains(contributor.ID, "host") {
			t.Fatalf("host task contributor was attributed to skills: %#v", contributor)
		}
	}
}

func TestContextUsageUsesBudgetedDynamicSources(t *testing.T) {
	rawSkill := strings.Repeat("raw skill body ", 1000)
	usage := AnalyzeContextUsage(ContextUsageInput{
		Compiled: promptcompiler.CompiledPrompt{
			Dynamic: promptcompiler.DynamicPromptDelta{
				Sources: []promptcompiler.DynamicContextSource{{
					ID:          promptcompiler.DynamicContextSourceSkill,
					Content:     "summary only",
					TokenBudget: 1000,
					Overflowed:  true,
					SourceRef:   "prompt_trace://dynamic.skill",
					EvidenceRef: "dynamic.skill:overflow",
				}},
				SkillPromptAssets: []string{rawSkill},
			},
		},
	})

	if got := categoryTokens(usage, "skills"); got == 0 || got > 20 {
		t.Fatalf("skills tokens = %d, want bounded source tokens instead of raw asset; usage=%#v", got, usage)
	}
	for _, contributor := range usage.TopContributors {
		if strings.Contains(contributor.Action, "raw skill body") {
			t.Fatalf("context usage leaked raw dynamic source overflow: %#v", contributor)
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
