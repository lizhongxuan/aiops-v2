package promptcompiler

import (
	"strings"
	"testing"
)

func assertPromptContainsAll(t *testing.T, label string, content string, wants []string) {
	t.Helper()
	lower := strings.ToLower(content)
	for _, want := range wants {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Fatalf("%s missing %q in:\n%s", label, want, content)
		}
	}
}

func assertPromptOmitsAll(t *testing.T, label string, content string, blocked []string) {
	t.Helper()
	lower := strings.ToLower(content)
	for _, block := range blocked {
		if strings.Contains(lower, strings.ToLower(block)) {
			t.Fatalf("%s contains blocked phrase %q in:\n%s", label, block, content)
		}
	}
}

func TestSemanticPromptSimpleChatDoesNotForcePlan(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "system", compiled.System.Content, []string{
		"concise, direct answers",
		"simple factual lookups",
		"use tools silently",
	})
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"concise, direct, and friendly",
		"actionable answers",
		"structure proportional to the task",
		"simple",
		"direct",
		"do not pad simple tasks with a plan",
	})
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"always create a plan",
		"must call update_plan for every request",
	})
}

func TestSemanticPromptProgressUpdatesAreScoped(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"group related actions",
		"one brief progress update",
		"substantial work",
		"skip preambles for trivial reads",
		"quick factual lookups",
	})
}

func TestSemanticPromptResponsivenessPreambles(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Responsiveness",
		"progress update or preamble",
		"1-2 sentences",
		"immediate tangible next steps",
		"connect the next preamble",
		"momentum and clarity",
		"light, friendly, and curious",
		"I've explored the repo; now checking the API route definitions.",
		"Next, I'll patch the config and update the related tests.",
	})
}

func TestSemanticPromptQuickFactualLookupsStayCompact(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"quick factual lookups",
		"use tools silently",
		"only the key values",
		"do not narrate tool process",
		"optional follow-up menus",
	})
}

func TestSemanticPromptComplexToolInvestigationsKeepProgressUpdates(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"complex, ambiguous, multi-step, or AIOps/RCA",
		"before the first tool call",
		"multi-step investigations",
		"after each batch",
	})
}

func TestSemanticPromptExecuteModeIncludesApprovalBoundary(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "policy", compiled.Policy.Content, []string{
		"approval",
		"evidence",
		"execute",
	})
}

func TestSemanticPromptIncludesProtocolState(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		ProtocolState: ProtocolPromptState{Items: []ProtocolPromptItem{
			{Kind: "approval", ID: "approval-1", Status: "pending", Text: "exec_command pending approval"},
		}},
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "dynamic", compiled.Dynamic.Content, []string{
		"approval-1",
		"pending",
		"exec_command pending approval",
	})
}

func TestSemanticPromptExecuteModeRequiresEvidenceForLocalEvalBehavior(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"local agent",
		"eval",
		"gather local evidence",
		"do not only acknowledge",
	})
}

func TestSemanticPromptReadOnlyInspectionDoesNotRunTests(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"read-only local inspection",
		"do not execute build, test",
		"verification methods",
	})
}

func TestSemanticPromptExecCommandAvoidsShellPipelines(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"exec_command",
		"do not wrap commands",
		"pipes, redirection, or command chaining",
	})
}

func TestSemanticPromptExplicitPlanRequestUsesPlanningTool(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"explicitly requires a structured plan",
		"planning tool",
		"in_progress",
	})
}
