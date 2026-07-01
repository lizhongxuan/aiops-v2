package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type agentRuntimeContextOptimizationTraceCase struct {
	Name     string                                     `json:"name"`
	Input    string                                     `json:"input"`
	Expected agentRuntimeContextOptimizationTraceExpect `json:"expected"`
}

type agentRuntimeContextOptimizationTraceExpect struct {
	Profile                         string   `json:"profile"`
	PromptSections                  []string `json:"promptSections"`
	VisibleTools                    []string `json:"visibleTools"`
	HiddenTools                     []string `json:"hiddenTools"`
	ToolPacks                       []string `json:"toolPacks,omitempty"`
	Timeline                        []string `json:"timeline"`
	FoldGroups                      []string `json:"foldGroups,omitempty"`
	ProgressMaxChars                int      `json:"progressMaxChars"`
	SingleRuntimePath               bool     `json:"singleRuntimePath"`
	NoOldFallbackPath               bool     `json:"noOldFallbackPath"`
	PromptBudgetWithinProfileLimit  bool     `json:"promptBudgetWithinProfileLimit"`
	OnlyMentionedToolPacksVisible   bool     `json:"onlyMentionedToolPacksVisible"`
	FinalAnswerQualityPreserved     bool     `json:"finalAnswerQualityPreserved"`
	ProgressDoesNotContainLongFinal bool     `json:"progressDoesNotContainLongFinal"`
	ModelTimeoutRecoverable         bool     `json:"modelTimeoutRecoverable,omitempty"`
}

func TestAgentRuntimeContextOptimizationTrace(t *testing.T) {
	cases := loadAgentRuntimeContextOptimizationTraceCases(t)
	expectedNames := []string{
		"advisor_simple_no_host_no_ops_tools",
		"public_fact_with_inline_sources",
		"evidence_rca_logs_no_host_exec",
		"host_simple_exec_no_child_agent",
		"host_complex_task_uses_plan_then_child_agent",
		"multi_host_manager_no_direct_exec",
		"approval_denied_same_turn_continuation",
		"ops_graph_requires_mention",
		"coroot_requires_mention",
		"ops_manus_requires_mention",
		"tool_failure_not_health_proof",
		"long_history_compaction_refs",
		"progress_short_final_long",
		"web_search_command_folding",
		"model_timeout_recoverable",
	}
	if len(cases) != len(expectedNames) {
		t.Fatalf("context optimization cases = %d, want %d", len(cases), len(expectedNames))
	}
	want := map[string]bool{}
	for _, name := range expectedNames {
		want[name] = true
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if !want[tc.Name] {
				t.Fatalf("unexpected case %q", tc.Name)
			}
			if seen[tc.Name] {
				t.Fatalf("duplicate case %q", tc.Name)
			}
			seen[tc.Name] = true
			validateAgentRuntimeContextOptimizationTraceCase(t, tc)
		})
	}
}

func loadAgentRuntimeContextOptimizationTraceCases(t *testing.T) []agentRuntimeContextOptimizationTraceCase {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "eval_cases", "agent_runtime_context_optimization", "cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cases []agentRuntimeContextOptimizationTraceCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Name < cases[j].Name
	})
	return cases
}

func validateAgentRuntimeContextOptimizationTraceCase(t *testing.T, tc agentRuntimeContextOptimizationTraceCase) {
	t.Helper()
	if strings.TrimSpace(tc.Name) == "" || strings.TrimSpace(tc.Input) == "" {
		t.Fatalf("case must have name and input: %#v", tc)
	}
	expect := tc.Expected
	requireRuntimeTraceValue(t, tc.Name, "profile", expect.Profile)
	requireRuntimeTraceList(t, tc.Name, "promptSections", expect.PromptSections)
	requireRuntimeTraceList(t, tc.Name, "visibleTools", expect.VisibleTools)
	requireRuntimeTraceListAllowDuplicates(t, tc.Name, "timeline", expect.Timeline)
	for _, required := range []string{"base.contract", "runtime.state", "tool.surface", "dynamic.context"} {
		if !runtimeTraceContains(expect.PromptSections, required) {
			t.Fatalf("case %s: promptSections missing %q in %#v", tc.Name, required, expect.PromptSections)
		}
	}
	for _, required := range []string{"assistant_message(commentary)", "assistant_message(final_answer)"} {
		if !runtimeTraceContains(expect.Timeline, required) {
			t.Fatalf("case %s: timeline missing %q in %#v", tc.Name, required, expect.Timeline)
		}
	}
	if expect.ProgressMaxChars <= 0 || expect.ProgressMaxChars > 180 {
		t.Fatalf("case %s: progressMaxChars = %d, want 1..180", tc.Name, expect.ProgressMaxChars)
	}
	if !expect.SingleRuntimePath ||
		!expect.NoOldFallbackPath ||
		!expect.PromptBudgetWithinProfileLimit ||
		!expect.OnlyMentionedToolPacksVisible ||
		!expect.FinalAnswerQualityPreserved ||
		!expect.ProgressDoesNotContainLongFinal {
		t.Fatalf("case %s: context optimization invariants must all be true: %#v", tc.Name, expect)
	}
	if strings.Contains(tc.Name, "no_host") || strings.Contains(tc.Name, "logs_no_host") {
		if runtimeTraceContains(expect.VisibleTools, "exec_command") {
			t.Fatalf("case %s: exec_command must not be visible without host mention", tc.Name)
		}
	}
	if strings.Contains(tc.Name, "requires_mention") && len(expect.HiddenTools) == 0 {
		t.Fatalf("case %s: mention gate cases must list hidden tools", tc.Name)
	}
	if tc.Name == "web_search_command_folding" {
		for _, required := range []string{"web_lookup", "command"} {
			if !runtimeTraceContains(expect.FoldGroups, required) {
				t.Fatalf("case %s: foldGroups missing %q in %#v", tc.Name, required, expect.FoldGroups)
			}
		}
		if runtimeTraceContains(expect.FoldGroups, "mcp") || runtimeTraceContains(expect.FoldGroups, "approval") {
			t.Fatalf("case %s: foldGroups must not include MCP or approval: %#v", tc.Name, expect.FoldGroups)
		}
	}
	if tc.Name == "model_timeout_recoverable" && !expect.ModelTimeoutRecoverable {
		t.Fatalf("case %s: modelTimeoutRecoverable must be true", tc.Name)
	}
}

func requireRuntimeTraceListAllowDuplicates(t *testing.T, caseName, field string, values []string) {
	t.Helper()
	if len(values) == 0 {
		t.Fatalf("case %s: expected.%s is required", caseName, field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("case %s: expected.%s contains empty value", caseName, field)
		}
	}
}
