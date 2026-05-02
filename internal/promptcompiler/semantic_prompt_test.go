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
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"simple",
		"direct",
	})
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"always create a plan",
		"must call update_plan for every request",
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
