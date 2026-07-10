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
		"response structure proportional to the task",
		"simple requests get direct answers",
		"Use a short plan only for multi-step",
		"quick factual lookups",
		"only the key values",
	})
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"always create a plan",
		"must call update_plan for every request",
	})
}

func TestSemanticPromptDeveloperInstructionsUseClaudeCodeStyleSections(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType:    "host",
		Mode:           "execute",
		PlanningPolicy: "structured_events",
		EvidencePolicy: "tool_sourced",
		AnswerStyle:    "aiops_rca",
		ToolBudget:     "bounded",
		AgentKind:      AgentKindWorker,
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"## Operating Contract",
		"## Task Scope",
		"## Tool Boundaries",
		"## Completion and Final Answer",
		"## Mode Rules",
		"registered evidence",
		"Use structured planning state when available",
		"tool failure, empty output, denial, timeout",
		"mutating actions require scoped runtime approval",
		"final answer",
	})
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"## AIOps Investigation Loop",
		"## Risk and Approval Boundaries",
		"## Final Answer Shape",
	})
}

func TestSemanticPromptResponsivenessUsesCommunicationModes(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"quick factual lookups",
		"use tools silently",
		"short prelude before the first tool call",
		"short updates after each batch",
		"multi-step investigations",
	})
}

func TestSemanticPromptAIOpsInvestigationLoopIsOperational(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		AnswerStyle: "aiops_rca",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"complex tasks gather evidence before conclusions",
		"complete RCA",
		"evidence interpretation",
		"remediation guidance",
		"verification status",
	})
}

func TestSemanticPromptCleanReadOnlyStatusChecksStayCompact(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		AnswerStyle: "aiops_rca",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"For simple answers, lead with the answer or outcome.",
		"ordinary evidence/advisory answers",
		"conclusion",
		"evidence boundary",
		"next read-only checks",
	})
}

func TestSemanticPromptDoesNotInlineProviderSpecificCorootRules(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"Coroot service_metrics returns chartReports",
		"Agent-to-UI coroot_chart artifacts",
		"Coroot chart summaries",
	})
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Only call tools visible in the current runtime tool surface",
		"Failed, unloaded, hidden, or not-yet-selected tools do not count as checked evidence",
	})
}

func TestSemanticPromptOpsManualWorkflowRulesOmittedFromDeveloperPrompt(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptOmitsAll(t, "developer", compiled.Developer.Content, []string{
		"search_ops_manuals",
		"run_ops_manual_preflight",
		"resolve_ops_manual_params",
		"direct_execute means preflight-ready",
		"Workflow execution",
	})
}

func TestDeveloperRulesDoNotCarryOpsManualPreflightWorkflow(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	text := compiledEnvelopeTextForTest(compiled)
	for _, forbidden := range []string{"direct_execute means preflight-ready", "Workflow execution", "run_ops_manual_preflight", "resolve_ops_manual_params", "proceed to Dry Run only after preflight passed"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("developer prompt still carries OpsManual workflow rule %q:\n%s", forbidden, text)
		}
	}
}

func TestSemanticPromptRiskBoundariesUseBlastRadius(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		Profile:     PromptProfileHostWorker,
		HostContext: "host-1",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"runtime approval",
		"verification after the action",
		"Report verification honestly",
		"Never characterize incomplete",
	})
}

func TestSemanticPromptProgressUpdatesAreScoped(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Progress updates are not final answers",
		"quick factual lookups",
		"do not narrate tool process",
		"short prelude",
		"short updates",
	})
}

func TestSemanticPromptUserVisibleRepliesMatchUserLanguage(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Match the user's language",
		"translate fixed tool/status labels",
		"do not expose internal English identifiers",
	})
}

func TestSemanticPromptKeepsRCAInFinalAnswerWithInlineSources(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Progress updates are not final answers",
		"complete RCA, evidence interpretation, remediation guidance, and caveats in the final answer",
		"cite source links in final answers",
		"precise source-bound lookup",
	})
}

func TestSemanticPromptResponsivenessPreambles(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"short prelude before the first tool call",
		"short updates after each batch",
		"multi-step investigations",
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

func TestFinalEvidenceVerifierRequirementInDeveloperRules(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "inspect",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if !strings.Contains(compiled.Stable.Developer.Content, "Failed, unloaded, hidden, or not-yet-selected tools do not count as checked evidence") {
		t.Fatalf("developer rules missing final evidence verifier requirement:\n%s", compiled.Stable.Developer.Content)
	}
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

func TestSemanticPromptRequiresExplicitHostBindingBeforeMutationAdvice(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "workspace", Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"no explicit host or resource binding",
		"do not provide mutation commands",
		"ask the user to select a target or use @host/@ip",
	})
}

func TestSemanticPromptExplicitPlanRequestUsesPlanningTool(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Use structured planning state when available",
		"multi-step, risky, ambiguous, or multi-agent work",
		"in_progress",
	})
}
