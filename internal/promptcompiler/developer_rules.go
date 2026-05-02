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

	content := strings.Join(append(developerConstraintLines(constraints), dynamicPromptFragments(ctx)...), "\n")
	return DeveloperInstructions{
		Content:     content,
		Constraints: constraints,
	}, nil
}

func (c *PromptCompilerImpl) buildStableDeveloperInstructions(ctx CompileContext) DeveloperInstructions {
	constraints := c.resolveConstraints(ctx)
	return DeveloperInstructions{
		Content:     strings.Join(developerConstraintLines(constraints), "\n"),
		Constraints: constraints,
	}
}

func developerConstraintLines(constraints []string) []string {
	lines := make([]string, 0, len(constraints)+1)
	lines = append(lines, "# Developer Instructions")
	for _, constraint := range constraints {
		lines = append(lines, fmt.Sprintf("- %s", constraint))
	}
	return lines
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
	constraints = append(constraints, "For non-trivial or tool-backed requests, before the first tool call emit one concise intent sentence that explains what you will verify and how.")
	constraints = append(constraints, "After each tool result or batch of related tool results, briefly summarize what changed, what you learned, and the next action before calling more tools or finalizing.")
	constraints = append(constraints, "When using exec_command for read-only inspection, pass the executable and args directly; do not wrap commands in sh/bash/zsh -c, pipes, redirection, or command chaining. Use narrower commands or native flags instead.")
	constraints = append(constraints, "When the user asks to validate local agent, eval, runtime, trace, tool, or prompt behavior, gather local evidence with available read-only tools before finalizing; do not only acknowledge the rule or describe future intent.")
	constraints = append(constraints, "When the user explicitly asks for read-only local inspection, do not execute build, test, server-start, package-install, or other non-read-only commands; mention those commands only as verification methods unless the user asks you to run them.")
	constraints = append(constraints, "When the user explicitly requires a structured plan or status tracking, use the available planning tool and keep at least one current step status such as in_progress visible until the work completes.")
	constraints = append(constraints, "For simple direct questions about whether planning is required, answer directly and avoid internal tool names unless the user asks for implementation-level detail.")
	constraints = append(constraints, "For current or latest factual requests, use precise self-contained web_search queries, verify recency, and cite source URLs in the final answer.")
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
