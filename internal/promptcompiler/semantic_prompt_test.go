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
		"## Task Triage",
		"## Planning and Status Tracking",
		"## Responsiveness",
		"## Evidence and Inference",
		"## AIOps Investigation Loop",
		"## Tool Use Boundaries",
		"## Risk and Approval Boundaries",
		"## Final Answer Shape",
		"## Mode-Specific Rules",
		"## Agent Role Rules",
		"verify tool results",
		"structured plan events",
		"Evidence must come from tool results",
		"Layer 3 tool details",
		"symptom, affected scope, and time window",
		"before mutation, capture pre-change state",
		"after mutation, verify",
		"Low risk",
		"Medium risk",
		"High risk",
		"Only operate on your designated host",
		"Root Cause",
		"Evidence",
		"Impact",
		"Next Steps",
	})
}

func TestSemanticPromptResponsivenessUsesCommunicationModes(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"Use three communication modes",
		"Silent mode",
		"Preamble mode",
		"Milestone mode",
		"quick factual lookups",
		"trivial reads",
		"substantial grouped tool work",
		"evidence changes direction",
		"narrows the cause",
		"exposes a blocker",
		"I'll compare recent alerts with host metrics",
		"I found the prompt assembly path",
		"The tool index is clear",
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
		"user-visible symptom",
		"affected scope",
		"time window",
		"direct evidence",
		"narrowing hypotheses",
		"observed facts from inference",
		"opsManualAction=skip_ops_manual",
		"opsManualSkipped=true",
		"do not call search_ops_manuals, resolve_ops_manual_params, or run_ops_manual_preflight",
		"ordinary safe read-only investigation",
		"opsManualAction=reference_ops_manual",
		"manual-guided chat",
		"must still require explicit user confirmation before mutation",
		"if current evidence conflicts with the manual, stop applying the manual",
		"direct_execute means preflight-ready",
		"not permission to execute Workflow or mutate",
		"Do not inline full session facts",
		"Do not inline full Letta hints",
		"Do not inline full operations manual content",
		"Do not inline raw artifact payloads",
		"pre-change state",
		"rollback or recovery path",
		"symptom, metric, log, or service state",
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
		"read-only status or RCA check",
		"no abnormality",
		"short conclusion",
		"do not expand a long next-step plan",
		"do not suggest remediation, workflow execution, rollback, or operations manual generation",
		"resolve_ops_manual_params plus run_ops_manual_preflight have already passed",
		"do not run extra host, shell, Docker, Kubernetes, or observability probes",
		"1-3 bullets total",
		"no headings and no separate evidence section",
		"concise conclusion with compact evidence",
		"no change was executed in one bullet",
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
		"dynamically available observability tools",
		"read-only evidence sources",
	})
}

func TestSemanticPromptOpsManualSearchTriggerRules(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"search_ops_manuals",
		"operations manuals",
		"complex operations task",
		"high-risk actions",
		"service restart",
		"configuration changes",
		"database operations",
		"backup",
		"recovery",
		"migration",
		"cluster changes",
		"middleware or infrastructure target",
		"short or underspecified",
		"排查 Redis",
		"do not ask prose follow-up questions first",
		"before asking follow-up questions",
		"missing fields",
		"original request text",
		"preserve negations",
		"不重启",
		"no restart",
		"operation_frame.target.name or known_params.target_instance",
		"keep the selected/current host in target_scope.hosts",
		"LLM judgment alone",
		"no verified Workflow, ActionToken, approval path, or executable mutation tool",
		"do not claim the change was executed",
		"need_info",
		"need_info with one or more manuals",
		"immediate next tool call must be resolve_ops_manual_params with the matched manual_id",
		"do not run host commands, monitoring probes, ordinary shell checks, or normal investigation before resolve_ops_manual_params returns",
		"need_info with no manuals",
		"do not call resolve_ops_manual_params because there is no manual_id",
		"call search_ops_manuals again with an explicit operation_frame",
		"ask only the smallest missing object or action question",
		"missing ops manual matching context, not a Workflow preflight failure",
		"fill the bottom form",
		"do not repeat questions or a template in prose",
		"Do not duplicate Agent-to-UI card details",
		"one short status sentence plus the smallest useful question or next action",
		"dynamically available observability tools are visible",
		"Do not ask the user whether configured observability evidence exists",
		"system cannot inspect",
		"adapt",
		"reference_only",
		"no_match",
		"non-executable",
		"never run a Workflow from those decisions",
		"reference_only or no_match",
		"continue safe read-only evidence-driven investigation",
		"Kafka",
		"metrics/log source",
		"host/session availability",
		"Do not present a cross-object manual",
		"Kubernetes Pod manual as a Kafka troubleshooting reference",
		"Agent-to-UI compact form",
		"do not duplicate the same fields as a multiline prose template",
		"resolve_ops_manual_params returns ambiguous or need_user_input",
		"stop tool use and wait for the user to submit the structured Agent-to-UI form",
		"do not run host commands, monitoring probes, ordinary shell checks, preflight, or Workflow execution while that form is pending",
		"ask for user confirmation and then run it after confirmation",
		"direct_execute",
		"run_ops_manual_preflight",
		"pass the operation_frame",
		"extracted parameters",
		"After preflight passes",
		"user confirmation or approval",
		"do not add a runtime Dry Run step",
	})
}

func TestDeveloperRulesDirectExecuteUsesPreflightThenConfirmation(t *testing.T) {
	lines := developerAIOpsInvestigationLines(CompileContext{})
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "direct_execute means preflight-ready") {
		t.Fatalf("missing direct_execute preflight-ready rule:\n%s", text)
	}
	if !strings.Contains(text, "After preflight passes, wait for explicit user confirmation or approval before Workflow execution") {
		t.Fatalf("missing confirmation after preflight rule:\n%s", text)
	}
	if strings.Contains(text, "proceed to Dry Run only after preflight passed") {
		t.Fatalf("runtime prompt still requires Dry Run:\n%s", text)
	}
}

func TestSemanticPromptRiskBoundariesUseBlastRadius(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	assertPromptContainsAll(t, "developer", compiled.Developer.Content, []string{
		"destructive",
		"hard to reverse",
		"shared systems",
		"production state",
		"hide diagnostic evidence",
		"Low risk",
		"Medium risk",
		"High risk",
		"runtime approval",
		"Do not broaden scope after a denial or failure",
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
