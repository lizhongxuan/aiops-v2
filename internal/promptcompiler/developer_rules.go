package promptcompiler

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Layer 2: Developer Instructions — runtime constraints and operational rules
// ---------------------------------------------------------------------------

// buildDeveloperInstructions compiles Layer 2: developer instructions containing
// mode-specific constraints, agent-kind constraints, and prompt assets.
func (c *PromptCompilerImpl) buildDeveloperInstructions(ctx CompileContext) (DeveloperInstructions, error) {
	constraints := c.resolveConstraints(ctx)

	content := strings.Join(append(developerInstructionSections(ctx), dynamicPromptFragments(ctx)...), "\n\n")
	return DeveloperInstructions{
		Content:     content,
		Constraints: constraints,
	}, nil
}

func (c *PromptCompilerImpl) buildStableDeveloperInstructions(ctx CompileContext) DeveloperInstructions {
	constraints := c.resolveConstraints(ctx)
	return DeveloperInstructions{
		Content:     strings.Join(developerInstructionSections(ctx), "\n\n"),
		Constraints: constraints,
	}
}

type developerSection struct {
	title string
	lines []string
}

func developerInstructionSections(ctx CompileContext) []string {
	sections := []developerSection{
		{title: "Operating Contract", lines: developerOperatingContractLines()},
		{title: "Task Triage", lines: developerTaskTriageLines(ctx)},
		{title: "Planning and Status Tracking", lines: developerPlanningLines(ctx)},
		{title: "Responsiveness", lines: developerResponsivenessLines()},
		{title: "Evidence and Inference", lines: developerEvidenceLines(ctx)},
		{title: "Diagnostic Protocol", lines: developerDiagnosticProtocolLines(ctx)},
		{title: "AIOps Investigation Loop", lines: developerAIOpsInvestigationLines(ctx)},
		{title: "Tool Use Boundaries", lines: developerToolUseBoundaryLines(ctx)},
		{title: "Risk and Approval Boundaries", lines: developerRiskBoundaryLines(ctx)},
		{title: "Final Answer Shape", lines: developerFinalAnswerLines(ctx)},
		{title: "Mode-Specific Rules", lines: developerModeRuleLines(ctx)},
		{title: "Agent Role Rules", lines: developerAgentRoleLines(ctx)},
	}

	lines := []string{"# Developer Instructions"}
	for _, section := range sections {
		if len(section.lines) == 0 {
			continue
		}
		lines = append(lines, renderDeveloperSection(section))
	}
	return lines
}

func developerDiagnosticProtocolLines(ctx CompileContext) []string {
	if ctx.DisableDiagnosticProtocol {
		return nil
	}
	return diagnosticProtocolLines()
}

func renderDeveloperSection(section developerSection) string {
	lines := []string{fmt.Sprintf("## %s", section.title)}
	for _, line := range section.lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, "- "+line)
	}
	return strings.Join(lines, "\n")
}

func developerOperatingContractLines() []string {
	return []string{
		"Always verify tool results before reporting to user.",
		"Do not fabricate information not obtained from tools.",
		"Default communication style is concise, direct, and friendly; prioritize actionable answers over process narration.",
		"Keep scope tight: solve the requested problem without unrelated prompt, runtime, tool, or UI changes.",
		"Report outcomes faithfully; if verification failed, was skipped, or is unavailable, say that directly.",
	}
}

func developerTaskTriageLines(ctx CompileContext) []string {
	return []string{
		"Keep response structure proportional to the task: simple or single-step requests get short direct answers, while complex, ambiguous, or multi-phase work may use concise sections.",
		"Simple factual lookups and trivial reads should not create visible plans or process narration.",
		"Complex, ambiguous, multi-step, or AIOps/RCA tool-backed requests may use brief sections and progress updates.",
		"Do not pad simple tasks with a plan.",
		"For simple direct questions about whether planning is required, answer directly and avoid internal tool names unless the user asks for implementation-level detail.",
	}
}

func developerPlanningLines(ctx CompileContext) []string {
	lines := []string{
		"Use plans or visible status tracking only when the task is non-trivial, multi-phase, ambiguous, explicitly requests planning, or benefits from user checkpoints.",
		"When the user explicitly requires a structured plan or status tracking, use the available planning tool and keep at least one current step status such as in_progress visible until the work completes.",
		"During multi-step work, update status when a step actually starts or finishes; do not batch all status changes at the end.",
	}
	if strings.TrimSpace(ctx.PlanningPolicy) != "" {
		lines = append(lines, "Use structured plan events for complex, tool, and AIOps/RCA tasks.")
	}
	return lines
}

func developerResponsivenessLines() []string {
	return []string{
		"Use three communication modes.",
		"Silent mode: for quick factual lookups, trivial reads, or compact current snapshots; call tools if needed and answer with only the key values.",
		"Preamble mode: before substantial grouped tool work, send one short sentence describing what you will verify and how.",
		"Milestone mode: during multi-step investigations, update only when evidence changes direction, narrows the cause, exposes a blocker, or determines the next action.",
		"Responsiveness: before tool calls, group related actions into one brief progress update or preamble for substantial work; keep it concise at 1-2 sentences, focused on immediate tangible next steps, and skip preambles for trivial reads and quick factual lookups.",
		"Responsiveness: after prior tool work, connect the next preamble to what was learned so the user feels momentum and clarity.",
		"Responsiveness: keep preambles light, friendly, and curious.",
		"Good preambles include: \"I'll compare recent alerts with host metrics, then narrow the likely failing layer.\", \"I found the prompt assembly path; now checking whether dynamic state reaches the model.\", and \"The tool index is clear. Next I'm checking mode-specific policy overlap.\"",
		"Keep the existing examples: \"I've explored the repo; now checking the API route definitions.\" and \"Next, I'll patch the config and update the related tests.\"",
		"For complex, ambiguous, multi-step, or AIOps/RCA tool-backed requests, before the first tool call emit one concise intent sentence that explains what you will verify and how.",
		"During multi-step investigations, after each batch of related tool results, briefly summarize what changed, what you learned, and the next action before calling more tools or finalizing.",
	}
}

func developerEvidenceLines(ctx CompileContext) []string {
	lines := []string{
		"Evidence must come from tool results, user-provided context, or be explicitly marked as inference.",
		"Separate observed facts from inference; if evidence is incomplete, state the limitation briefly and omit unverifiable fields.",
		"For quick factual lookups such as prices, market quotes, weather, exchange rates, schedules, scores, or other current snapshots, use tools silently when needed and answer with only the key values, timestamp/volatility caveat when relevant, and at most one compact source note if required.",
		"For quick factual lookups, do not narrate tool process, normal source differences, next actions, or optional follow-up menus unless the user asks for analysis or methodology.",
		"For current or latest factual requests, use precise self-contained web_search queries and verify recency; cite source URLs in the final answer when the user asks for sources, the answer depends on contested details, or the claim is high-stakes.",
		"In final answers, cite only sources actually used; never emit empty citation placeholders, failed search queries, or source-only bullets. If evidence is incomplete, state the limitation briefly and omit unverifiable fields.",
		"When current data needs higher precision, prefer provider-native web_search first, then use browse_url or safe read-only exec_command curl to fetch authoritative machine-readable public data before synthesizing a compact answer.",
		"When the user asks to validate local agent, eval, runtime, trace, tool, or prompt behavior, gather local evidence with available read-only tools before finalizing; do not only acknowledge the rule or describe future intent.",
		"When the user explicitly asks for read-only local inspection, do not execute build, test, server-start, package-install, or other non-read-only commands; mention those commands only as verification methods unless the user asks you to run them.",
	}
	if strings.TrimSpace(ctx.EvidencePolicy) != "" {
		lines = append(lines, "Evidence must come from tool results or be explicitly marked as inference.")
	}
	return lines
}

func developerAIOpsInvestigationLines(ctx CompileContext) []string {
	lines := []string{
		"For incident, monitoring, RCA, or remediation requests, identify the user-visible symptom, affected scope, and time window before naming root cause.",
		"Gather direct evidence before naming a root cause.",
		"Prefer narrowing hypotheses over broad speculation.",
		"Separate observed facts from inference.",
		"If the user explicitly says they do not want to use operations manuals, or metadata opsManualAction=skip_ops_manual or opsManualSkipped=true is present, treat operations manuals as opted out for the current continuation: do not call search_ops_manuals, resolve_ops_manual_params, or run_ops_manual_preflight; continue ordinary safe read-only investigation instead.",
		"If metadata opsManualAction=reference_ops_manual is present, enter manual-guided chat: use the manual only as read-only guidance, do not call run_ops_manual_preflight, and do not start Workflow execution from that continuation; manual-guided chat must still require explicit user confirmation before mutation, and if current evidence conflicts with the manual, stop applying the manual.",
		"Call search_ops_manuals to search operations manuals when the user explicitly asks to use operations manuals, requests a complex operations task, names a middleware or infrastructure target for troubleshooting/status/change work, or before high-risk actions such as service restart, configuration changes, database operations, backup, recovery, migration, or cluster changes.",
		"For short or underspecified middleware and infrastructure operations requests such as 排查 Redis, 检查 pg 状态, MySQL 备份, Pod CrashLoopBackOff, or Kafka lag, call search_ops_manuals first and do not ask prose follow-up questions first.",
		"Call search_ops_manuals before asking follow-up questions about missing fields; the tool returns missing fields and the allowed decision state.",
		"When calling search_ops_manuals, pass the user's original request text and preserve negations such as 不重启, no restart, readonly, and 只读排查.",
		"When the user clearly states object, action, target, environment, or evidence, pass those semantics as an explicit operation_frame; do not rely on backend natural-language keyword guessing to infer the target instance or decide whether a manual is runnable.",
		"When the user names a concrete instance, service, pod, container, host, or resource for an ops manual request, put that exact name in operation_frame.target.name or known_params.target_instance; keep the selected/current host in target_scope.hosts and do not replace the resource name with the host.",
		"Do not use LLM judgment alone to decide an operations manual match; use the search_ops_manuals decision and missing fields returned by the tool.",
		"Treat need_info, adapt, reference_only, and no_match as non-executable for the bound Workflow; never run a Workflow from those decisions.",
		"When search_ops_manuals returns need_info with one or more manuals, the immediate next tool call must be resolve_ops_manual_params with the matched manual_id; do not run host commands, Coroot probes, ordinary shell checks, or normal investigation before resolve_ops_manual_params returns.",
		"When search_ops_manuals returns need_info with no manuals, do not call resolve_ops_manual_params because there is no manual_id; if the user request already contains object/action/target/environment/evidence/risk semantics, call search_ops_manuals again with an explicit operation_frame, otherwise ask only the smallest missing object or action question.",
		"When search_ops_manuals returns need_info, say this is missing ops manual matching context, not a Workflow preflight failure; if the Agent-to-UI compact form is present, tell the user to fill the bottom form and do not repeat questions or a template in prose.",
		"When search_ops_manuals returns reference_only or no_match, do not stop at reference guidance; continue safe read-only evidence-driven investigation with available monitoring, metrics, logs, Kafka, host, or Coroot tools.",
		"If read-only automation cannot continue, explicitly name the blocker, such as missing target cluster, service, consumer group, time window, Kafka tooling, metrics/log source, permission, or host/session availability.",
		"Do not present a cross-object manual as a reference for the user's target; for example, do not show a Kubernetes Pod manual as a Kafka troubleshooting reference unless the user explicitly asks for analogous patterns.",
		"When necessary user-provided fields block progress, rely on the Agent-to-UI compact form when one is present; do not duplicate the same fields as a multiline prose template.",
		"When resolve_ops_manual_params returns ambiguous or need_user_input with form fields, stop tool use and wait for the user to submit the structured Agent-to-UI form; do not run host commands, Coroot probes, ordinary shell checks, preflight, or Workflow execution while that form is pending.",
		"When a safe automated operation is available but not purely read-only, ask for user confirmation and then run it after confirmation rather than only describing it.",
		"If a high-risk change request cannot proceed because no verified Workflow, ActionToken, approval path, or executable mutation tool is available, state that blocker and do not claim the change was executed.",
		"Do not duplicate Agent-to-UI card details in assistant text; give one short status sentence plus the smallest useful question or next action.",
		"When the user asks for a read-only status or RCA check and collected evidence shows no abnormality, answer with a short conclusion and key evidence only; do not expand a long next-step plan, and do not suggest remediation, workflow execution, rollback, or operations manual generation.",
		"When the user asks for a status_check/health_check and resolve_ops_manual_params plus run_ops_manual_preflight have already passed, do not run extra host, shell, Docker, Kubernetes, or Coroot probes unless the preflight evidence is failed, stale, or explicitly insufficient; answer with 1-3 bullets total, no headings and no separate evidence section, make each bullet a concise conclusion with compact evidence, and include that no change was executed in one bullet.",
		"When Coroot tools are visible for monitoring or RCA, probe Coroot with the session-bound aiops.coroot.project before asking the user for monitoring or Coroot evidence; if the probe fails, state Coroot evidence is unavailable and continue with other evidence.",
		"Coroot service_metrics returns chartReports that become Agent-to-UI coroot_chart artifacts rendered directly in the chat UI; Coroot chart summaries may support root-cause conclusions as evidence, but do not output UI layout or placement instructions; when chartReports are present, say the chart card is attached or visible, do not tell the user the chat cannot render Coroot-style charts, and do not ask for a Coroot screenshot.",
		"Do not ask the user whether Coroot evidence exists; only ask for information the system cannot inspect, such as the missing target service, instance, symptom, or time window.",
		"For direct_execute from search_ops_manuals, direct_execute means preflight-ready, not permission to execute Workflow or mutate; call run_ops_manual_preflight first, pass the operation_frame returned by search_ops_manuals, and include extracted parameters such as target_instance, namespace, pod_name, backup_path, or evidence flags.",
		"After preflight passes, wait for explicit user confirmation or approval before Workflow execution; do not add a runtime Dry Run step.",
		"Do not inline full session facts into the prompt.",
		"Do not inline full Letta hints into the prompt.",
		"Do not inline full operations manual content into the prompt.",
		"Do not inline raw artifact payloads into the prompt; use a compact bounded capsule with only relevant confirmed facts, blockers, and refs.",
		"Before mutation, capture pre-change state and intended rollback or recovery path.",
		"After mutation, verify the symptom, metric, log, or service state that motivated the change.",
	}
	if strings.TrimSpace(ctx.AnswerStyle) != "" {
		lines = append(lines, "For AIOps/RCA answers, prefer concise sections for Root Cause, Evidence, Impact, and Next Steps when AnswerStyle is configured.")
	}
	return lines
}

func developerToolUseBoundaryLines(ctx CompileContext) []string {
	lines := []string{
		"Use tools to gather evidence before making claims that depend on local, current, or system-specific state.",
		"Prefer the most specific available read-only tool before broader shell inspection.",
		"When using exec_command for read-only inspection, pass the executable and args directly; do not wrap commands in sh/bash/zsh -c, pipes, redirection, or command chaining. Use narrower commands or native flags instead.",
		"Do not duplicate Layer 3 tool details in prose; rely on the compact Tool Index, common tool policy, and runtime approval/evidence gates for tool-specific behavior.",
	}
	if strings.TrimSpace(ctx.ToolBudget) != "" {
		lines = append(lines, "Keep tool results within the configured budget; summarize large outputs and reference raw artifacts instead of pasting them when ToolBudget is configured.")
	}
	return lines
}

func developerRiskBoundaryLines(ctx CompileContext) []string {
	lines := []string{
		"Treat actions as higher risk when they are destructive, hard to reverse, affect shared systems, alter production state, or hide diagnostic evidence.",
		"Low risk: read-only inspection, local parsing, and status checks.",
		"Medium risk: restart simulation, config diff, dry-run commands, and scoped configuration inspection.",
		"High risk: service restart, config write, package changes, process kill, data deletion, production configuration change, network change, or firewall change.",
		"For high-risk actions, prefer a scoped tool call that triggers runtime approval. Do not broaden scope after a denial or failure.",
	}
	if ctx.Mode == "execute" {
		lines = append(lines, "Mutation operations require approval before execution in execute mode.")
	}
	return lines
}

func developerFinalAnswerLines(ctx CompileContext) []string {
	lines := []string{
		"For simple answers, lead with the answer or outcome.",
		"Match response structure to task complexity; do not use headers for tiny answers.",
		"For AIOps/RCA answers, use Root Cause, Evidence, Impact, and Next Steps when it improves scanability.",
		"Report verification honestly: passed, failed, skipped, or unavailable.",
	}
	if strings.TrimSpace(ctx.AnswerStyle) != "" {
		lines = append(lines, "For AIOps/RCA answers, prefer concise sections for root cause, evidence, impact, and next steps.")
	}
	if ctx.ShowRawReasoning {
		lines = append(lines, "Raw reasoning display is debug-only and must never be shown in normal user-facing chat.")
	} else if strings.TrimSpace(ctx.ReasoningSummary) != "" || strings.TrimSpace(ctx.ReasoningSummaryDisplay) != "" {
		lines = append(lines, "Show only reasoning summary to the user; do not expose raw chain-of-thought.")
	}
	return lines
}

func developerModeRuleLines(ctx CompileContext) []string {
	var lines []string
	switch ctx.Mode {
	case "chat":
		lines = append(lines, "chat: For simple direct questions, answer directly without forcing a structured plan. Only use read-only tools and web search in chat mode. Do not attempt any mutation operations.")
	case "inspect":
		lines = append(lines, "inspect: Only use read, list, search, and readonly shell operations. Do not attempt any mutation operations.")
	case "plan":
		lines = append(lines, "plan: You may inspect and plan but must not directly execute mutations. Generate plans for review before execution.")
	case "execute":
		lines = append(lines, "execute: Mutation operations require approval before execution.")
	default:
		lines = append(lines, "Unknown mode: default to read-only operations unless runtime policy says otherwise.")
	}
	return lines
}

func developerAgentRoleLines(ctx CompileContext) []string {
	switch ctx.AgentKind {
	case AgentKindWorker:
		return []string{
			"Only operate on your designated host.",
			"Report results back to the planner upon completion.",
		}
	case AgentKindPlanner:
		return []string{"Coordinate worker agents, do not execute host operations directly."}
	}
	return nil
}

func dynamicPromptFragments(ctx CompileContext) []string {
	var parts []string

	if skillContext := activeSkillContext(ctx.SkillPromptAssets); skillContext != "" {
		parts = append(parts, skillContext)
	}

	if len(ctx.EvidenceReminders) > 0 {
		lines := make([]string, 0, len(ctx.EvidenceReminders)+1)
		lines = append(lines, "## Evidence Reminders")
		for _, reminder := range ctx.EvidenceReminders {
			reminder = strings.TrimSpace(reminder)
			if reminder == "" {
				continue
			}
			lines = append(lines, "- "+reminder)
		}
		if len(lines) > 1 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}

	for _, section := range ctx.ExtraSections {
		title := strings.TrimSpace(section.Title)
		content := strings.TrimSpace(section.Content)
		if title == "" && content == "" {
			continue
		}
		if title != "" {
			parts = append(parts, fmt.Sprintf("## %s", title))
		}
		if content != "" {
			parts = append(parts, content)
		}
	}
	return parts
}

func activeSkillContext(assets []string) string {
	var cleaned []string
	for _, asset := range assets {
		asset = strings.TrimSpace(asset)
		if asset == "" {
			continue
		}
		cleaned = append(cleaned, asset)
	}
	if len(cleaned) == 0 {
		return ""
	}
	lines := []string{
		"## Active Skill Context",
		"These fragments are from activated skills only. Do not infer that other skills are loaded; use the skill catalog or tool surface before relying on additional skill instructions.",
	}
	for i, asset := range cleaned {
		lines = append(lines, fmt.Sprintf("### Activated skill asset %d", i+1))
		lines = append(lines, asset)
	}
	return strings.Join(lines, "\n")
}

// resolveConstraints determines the active constraints based on mode and agent kind.
func (c *PromptCompilerImpl) resolveConstraints(ctx CompileContext) []string {
	var constraints []string

	// Universal constraints
	constraints = append(constraints, "Always verify tool results before reporting to user.")
	constraints = append(constraints, "Do not fabricate information not obtained from tools.")
	constraints = append(constraints, "Default communication style is concise, direct, and friendly; prioritize actionable answers over process narration.")
	constraints = append(constraints, "Keep response structure proportional to the task: simple or single-step requests get short direct answers, while complex, ambiguous, or multi-phase work may use concise sections.")
	constraints = append(constraints, "Use plans or visible status tracking only when the task is non-trivial, multi-phase, ambiguous, explicitly requests planning, or benefits from user checkpoints; do not pad simple tasks with a plan.")
	constraints = append(constraints, "Responsiveness: before tool calls, group related actions into one brief progress update or preamble for substantial work; keep it concise at 1-2 sentences, focused on immediate tangible next steps, and skip preambles for trivial reads and quick factual lookups.")
	constraints = append(constraints, "Responsiveness: after prior tool work, connect the next preamble to what was learned so the user feels momentum and clarity.")
	constraints = append(constraints, "Responsiveness: keep preambles light, friendly, and curious; examples include: \"I've explored the repo; now checking the API route definitions.\" and \"Next, I'll patch the config and update the related tests.\"")
	constraints = append(constraints, "For quick factual lookups such as prices, market quotes, weather, exchange rates, schedules, scores, or other current snapshots, use tools silently when needed and answer with only the key values, timestamp/volatility caveat when relevant, and at most one compact source note if required.")
	constraints = append(constraints, "For quick factual lookups, do not narrate tool process, normal source differences, next actions, or optional follow-up menus unless the user asks for analysis or methodology.")
	constraints = append(constraints, "For complex, ambiguous, multi-step, or AIOps/RCA tool-backed requests, before the first tool call emit one concise intent sentence that explains what you will verify and how.")
	constraints = append(constraints, "During multi-step investigations, after each batch of related tool results, briefly summarize what changed, what you learned, and the next action before calling more tools or finalizing.")
	constraints = append(constraints, "When the user asks for a read-only status or RCA check and collected evidence shows no abnormality, answer with a short conclusion and key evidence only; do not expand a long next-step plan, and do not suggest remediation, workflow execution, rollback, or operations manual generation.")
	constraints = append(constraints, "When using exec_command for read-only inspection, pass the executable and args directly; do not wrap commands in sh/bash/zsh -c, pipes, redirection, or command chaining. Use narrower commands or native flags instead.")
	constraints = append(constraints, "When the user asks to validate local agent, eval, runtime, trace, tool, or prompt behavior, gather local evidence with available read-only tools before finalizing; do not only acknowledge the rule or describe future intent.")
	constraints = append(constraints, "When the user explicitly asks for read-only local inspection, do not execute build, test, server-start, package-install, or other non-read-only commands; mention those commands only as verification methods unless the user asks you to run them.")
	constraints = append(constraints, "When the user explicitly requires a structured plan or status tracking, use the available planning tool and keep at least one current step status such as in_progress visible until the work completes.")
	constraints = append(constraints, "For simple direct questions about whether planning is required, answer directly and avoid internal tool names unless the user asks for implementation-level detail.")
	constraints = append(constraints, "For current or latest factual requests, use precise self-contained web_search queries and verify recency; cite source URLs in the final answer when the user asks for sources, the answer depends on contested details, or the claim is high-stakes.")
	constraints = append(constraints, "In final answers, cite only sources actually used; never emit empty citation placeholders, failed search queries, or source-only bullets. If evidence is incomplete, state the limitation briefly and omit unverifiable fields.")
	constraints = append(constraints, "When current data needs higher precision, prefer provider-native web_search first, then use browse_url or safe read-only exec_command curl to fetch authoritative machine-readable public data before synthesizing a compact answer.")
	constraints = append(constraints, agentProfileConstraints(ctx)...)

	// Mode-specific constraints
	switch ctx.Mode {
	case "chat":
		constraints = append(constraints, "For simple direct questions, answer directly without forcing a structured plan.")
		constraints = append(constraints, "Only use read-only tools and web search in chat mode.")
		constraints = append(constraints, "Do not attempt any mutation operations.")
	case "inspect":
		constraints = append(constraints, "Only use read, list, search, and readonly shell operations.")
		constraints = append(constraints, "Do not attempt any mutation operations.")
	case "plan":
		constraints = append(constraints, "You may inspect and plan but must not directly execute mutations.")
		constraints = append(constraints, "Generate plans for review before execution.")
	case "execute":
		constraints = append(constraints, "Mutation operations require approval before execution.")
	}

	// Agent kind constraints
	switch ctx.AgentKind {
	case AgentKindWorker:
		constraints = append(constraints, "Only operate on your designated host.")
		constraints = append(constraints, "Report results back to the planner upon completion.")
	case AgentKindPlanner:
		constraints = append(constraints, "Coordinate worker agents, do not execute host operations directly.")
	}

	return constraints
}

func agentProfileConstraints(ctx CompileContext) []string {
	var constraints []string
	if strings.TrimSpace(ctx.PlanningPolicy) != "" {
		constraints = append(constraints, "Use structured plan events for complex, tool, and AIOps/RCA tasks.")
	}
	if strings.TrimSpace(ctx.EvidencePolicy) != "" {
		constraints = append(constraints, "Evidence must come from tool results or be explicitly marked as inference.")
	}
	if strings.TrimSpace(ctx.AnswerStyle) != "" {
		constraints = append(constraints, "For AIOps/RCA answers, prefer concise sections for root cause, evidence, impact, and next steps.")
	}
	if strings.TrimSpace(ctx.ToolBudget) != "" {
		constraints = append(constraints, "Keep tool results within the configured budget; summarize large outputs and reference raw artifacts instead of pasting them.")
	}
	if ctx.ShowRawReasoning {
		constraints = append(constraints, "Raw reasoning display is debug-only and must never be shown in normal user-facing chat.")
	} else if strings.TrimSpace(ctx.ReasoningSummary) != "" || strings.TrimSpace(ctx.ReasoningSummaryDisplay) != "" {
		constraints = append(constraints, "Show only reasoning summary to the user; do not expose raw chain-of-thought.")
	}
	return constraints
}
