package promptcompiler

import (
	"strings"
	"testing"

	"aiops-v2/internal/taskdepth"
)

func TestTaskDepthContractSectionPlacementAndContent(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{
		TaskDepth: taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
	}), "\n\n")
	triage := strings.Index(developer, "## Task Triage")
	depth := strings.Index(developer, "## Task Depth Contract")
	planning := strings.Index(developer, "## Planning and Status Tracking")
	if triage == -1 || depth == -1 || planning == -1 {
		t.Fatalf("missing sections:\n%s", developer)
	}
	if !(triage < depth && depth < planning) {
		t.Fatalf("section order invalid: triage=%d depth=%d planning=%d", triage, depth, planning)
	}
	for _, want := range []string{
		"Classify the user's request before answering",
		"do not finalize in the first assistant response",
		"gather direct evidence before naming a root cause",
		"Conciseness controls user-facing wording only",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("developer instructions missing %q:\n%s", want, developer)
		}
	}
}

func TestCompletionGateSectionContent(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	completion := strings.Index(developer, "## Completion Gate")
	finalAnswer := strings.Index(developer, "## Final Answer Shape")
	if completion == -1 || finalAnswer == -1 {
		t.Fatalf("missing completion or final section:\n%s", developer)
	}
	if !(completion < finalAnswer) {
		t.Fatalf("completion gate should appear before final answer shape")
	}
	for _, want := range []string{
		"verified conclusion",
		"blocker",
		"budget-limited synthesis",
		"Never characterize incomplete, unverified, or blocked work as completed",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("completion gate missing %q:\n%s", want, developer)
		}
	}
}

func TestResourceInspectionAnswerMustReportConcreteValues(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	for _, want := range []string{
		"resource inspection",
		"concrete requested values",
		"not only health or normality",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("developer instructions missing %q:\n%s", want, developer)
		}
	}
}

func TestCurrentEnvironmentFactsMustUseEnvironmentBoundTools(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	for _, want := range []string{
		"Current host, current environment, local runtime, selected resource",
		"do not use web_search or browse_url",
		"environment-bound tools",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("developer instructions missing %q:\n%s", want, developer)
		}
	}
}

func TestReasoningEffortFallbackPolicy(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{ReasoningEffort: "high"}), "\n\n")
	if !strings.Contains(developer, "Reasoning depth: high") {
		t.Fatalf("developer instructions missing reasoning effort fallback:\n%s", developer)
	}
	if strings.Contains(strings.ToLower(developer), "show raw reasoning") {
		t.Fatalf("reasoning fallback must not expose raw reasoning:\n%s", developer)
	}
}

func TestReasoningEffortFallbackPolicyVisibleWhenProviderUnsupported(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{ReasoningEffort: "high"}), "\n\n")
	policy := extractReasoningFallbackPolicyLine(developer)
	if policy == "" {
		t.Fatalf("developer instructions missing unsupported-provider fallback policy:\n%s", developer)
	}
	for _, want := range []string{
		"decompose the goal",
		"list assumptions",
		"gather evidence before conclusions",
		"cover key claims with evidence",
		"state the blocker",
	} {
		if !strings.Contains(strings.ToLower(policy), want) {
			t.Fatalf("fallback policy missing %q:\n%s", want, policy)
		}
	}
	for _, forbidden := range []string{
		"aiops",
		"rca",
		"incident",
		"host",
		"service",
		"pod",
		"kubernetes",
		"metric",
		"log",
		"alert",
		"monitoring",
		"coroot",
	} {
		if strings.Contains(strings.ToLower(policy), forbidden) {
			t.Fatalf("fallback policy contains domain term %q:\n%s", forbidden, policy)
		}
	}
}

func extractReasoningFallbackPolicyLine(developer string) string {
	for _, line := range strings.Split(developer, "\n") {
		if strings.Contains(strings.ToLower(line), "reasoning fallback policy") {
			return line
		}
	}
	return ""
}
