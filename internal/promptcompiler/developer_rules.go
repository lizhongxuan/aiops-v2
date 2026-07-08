package promptcompiler

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Layer 2: Developer Instructions — runtime constraints and operational rules
// ---------------------------------------------------------------------------

// buildDeveloperInstructions compiles Layer 2: developer instructions containing
// mode-specific constraints, agent-kind constraints, and prompt assets.
func (c *PromptCompilerImpl) buildDeveloperInstructions(ctx CompileContext) (DeveloperInstructions, error) {
	blocks := developerInstructionBlocks(ctx)
	content := strings.Join(append(blocks, dynamicPromptFragments(ctx)...), "\n\n")
	return DeveloperInstructions{
		Content:     content,
		Constraints: developerConstraintsFromBlocks(blocks),
	}, nil
}

func (c *PromptCompilerImpl) buildStableDeveloperInstructions(ctx CompileContext) DeveloperInstructions {
	blocks := developerInstructionBlocks(ctx)
	return DeveloperInstructions{
		Content:     strings.Join(blocks, "\n\n"),
		Constraints: developerConstraintsFromBlocks(blocks),
	}
}

type developerSection struct {
	title string
	lines []string
}

const genericReasoningFallbackPolicy = "Reasoning fallback policy: decompose the goal, list assumptions, gather evidence before conclusions, cover key claims with evidence, and state the blocker when progress cannot continue. Do not expose raw reasoning."

func developerInstructionBlocks(ctx CompileContext) []string {
	sections := []developerSection{
		{title: "Operating Contract", lines: developerThinOperatingContractLines()},
		{title: "Task Scope", lines: developerThinTaskScopeLines(ctx)},
		{title: "Profile Rules", lines: developerThinProfileLines(ctx)},
		{title: "Tool Boundaries", lines: developerThinToolBoundaryLines(ctx)},
		{title: "Completion and Final Answer", lines: developerThinFinalAnswerLines(ctx)},
		{title: "Mode Rules", lines: developerThinModeRuleLines(ctx)},
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

func developerThinOperatingContractLines() []string {
	return []string{
		"Use the current runtime state, model-visible tools, user input, and registered evidence as the only sources of operational truth.",
		"Do not fabricate tool results, system state, evidence, timelines, external facts, or verification status.",
		"Progress updates are not final answers; keep them short and put complete RCA, evidence interpretation, remediation guidance, and caveats in the final answer.",
		"Do not encode product, environment, resource, address, credential, or incident examples as core rules.",
	}
}

func developerThinTaskScopeLines(ctx CompileContext) []string {
	lines := []string{
		"Keep response structure proportional to the task; simple requests get direct answers and complex tasks gather evidence before conclusions.",
		"Use a short plan only for multi-step, risky, ambiguous, or multi-agent work.",
		"Use structured planning state when available; keep in_progress plan items current for complex work.",
	}
	if normalizePromptProfile(ctx.Profile) != PromptProfileHostWorker && normalizePromptProfile(ctx.Profile) != PromptProfileHostManager {
		lines = append(lines,
			"For quick factual lookups, use tools silently when needed, answer with only the key values, and do not narrate tool process or optional follow-up menus.",
			"For complex, ambiguous, multi-step, or AIOps/RCA work, send at most a short prelude before the first tool call and short updates after each batch in multi-step investigations.",
			"For current or latest public facts, use precise source-bound lookup; cite source links in final answers and prefer authoritative machine-readable data such as safe curl only when available through visible tools.",
		)
	}
	if strings.TrimSpace(string(ctx.TaskDepth.Level)) != "" {
		lines = append(lines, fmt.Sprintf("Current task depth: %s.", ctx.TaskDepth.Level))
	}
	if ctx.TaskDepth.AnalysisOnly {
		lines = append(lines, "Current task is analysis-only: do not claim execution or verification.")
	}
	if effort := strings.TrimSpace(ctx.ReasoningEffort); effort != "" {
		lines = append(lines,
			fmt.Sprintf("Reasoning depth: %s.", effort),
			genericReasoningFallbackPolicy,
		)
	}
	return lines
}

func developerThinProfileLines(ctx CompileContext) []string {
	fragment, ok := profileFragment(normalizePromptProfile(ctx.Profile), strings.TrimSpace(ctx.HostContext))
	if !ok {
		return nil
	}
	return append([]string(nil), fragment.Lines...)
}

func developerThinToolBoundaryLines(ctx CompileContext) []string {
	profile := normalizePromptProfile(ctx.Profile)
	lines := []string{
		"Only call tools visible in the current runtime tool surface; hidden tools are unavailable.",
		"Tool failure, empty output, denial, timeout, or unavailable evidence is not proof of healthy target state.",
		"Failed, unloaded, hidden, or not-yet-selected tools do not count as checked evidence.",
		"For resource inspection, report concrete requested values, not only health or normality.",
		"For current host, current environment, local runtime, or selected resource facts, use environment-bound tools and do not use web_search or browse_url unless explicitly asked.",
	}
	if profile != PromptProfileHostManager && profile != PromptProfileWorkflowAgent {
		lines = append(lines, "When exec_command is visible for read-only inspection, pass executable and args directly; do not wrap commands in sh/bash/zsh -c, pipes, redirection, or command chaining.")
	}
	if strings.TrimSpace(ctx.SessionType) == "workspace" || strings.TrimSpace(ctx.HostContext) == "" {
		lines = append(lines, "If there is no explicit host or resource binding, do not provide mutation commands; ask the user to select a target or use @host/@ip before operations.")
	}
	if strings.TrimSpace(ctx.ToolBudget) != "" {
		lines = append(lines, "Summarize large tool outputs and reference raw artifacts instead of pasting them.")
	}
	return lines
}

func developerThinFinalAnswerLines(ctx CompileContext) []string {
	lines := []string{
		"For simple answers, lead with the answer or outcome.",
		"For ordinary evidence/advisory answers, keep the final answer to the conclusion, key mechanism, evidence boundary, and next read-only checks.",
		"Never emit empty citation placeholders, failed search queries, or unverifiable fields.",
		"Report verification honestly: passed, failed, skipped, partial, blocked, or unavailable.",
		"If verification status is PARTIAL or FAIL, state the blocker, checked contract, expected vs actual, and available evidence reference; call the outcome partially verified or blocked.",
		"Never characterize incomplete, unverified, blocked, or budget-limited work as complete.",
	}
	if strings.TrimSpace(ctx.AnswerStyle) != "" {
		lines = append(lines, "When AnswerStyle is configured, prefer concise sections only when they improve scanability.")
	}
	if strings.EqualFold(strings.TrimSpace(ctx.AnswerStyle), "concise") {
		lines = append(lines, "For AnswerStyle=concise, use the shortest complete answer that preserves evidence limits, approval boundaries, and verification status.")
	}
	return lines
}

func developerThinModeRuleLines(ctx CompileContext) []string {
	switch ctx.Mode {
	case "chat":
		return []string{"chat: default to direct answers and read-only tools; mutating operations are forbidden."}
	case "inspect":
		return []string{"inspect: use read-only evidence collection only."}
	case "plan":
		return []string{"plan: inspect and plan only; do not execute mutations."}
	case "execute":
		lines := []string{"execute: mutating actions require scoped runtime approval."}
		if normalizePromptProfile(ctx.Profile) != PromptProfileHostManager {
			lines = append(lines,
				"When validating local agent, eval, runtime, trace, tool, or prompt behavior, gather local evidence and do not only acknowledge the rule.",
				"When the user explicitly asks for read-only local inspection, do not execute build, test, server-start, package-install, or other non-read-only commands; mention them only as verification methods.",
			)
		}
		return lines
	default:
		return []string{"unknown mode: default to read-only behavior."}
	}
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

func developerConstraintsFromBlocks(blocks []string) []string {
	var constraints []string
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- ") {
				continue
			}
			constraint := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if constraint != "" {
				constraints = append(constraints, constraint)
			}
		}
	}
	return constraints
}

func dynamicPromptFragments(ctx CompileContext) []string {
	return renderDynamicContextSources(buildDynamicContextSources(ctx, ""))
}

func activeHostTaskContext(assets []string) string {
	lines := []string{
		"## Active Host Task Context",
		"These fragments are assigned host-bound task context from manager-to-host agent messages.",
		"They are not skill instructions. Do not infer that additional skills are loaded.",
	}
	for i, asset := range assets {
		asset = strings.TrimSpace(asset)
		if asset == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("### Host task asset %d", i+1), asset)
	}
	if len(lines) == 3 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func loadedSkillContext(refs []LoadedSkillPromptRef) string {
	lines := []string{"## Newly loaded skills"}
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		source := strings.TrimSpace(ref.Source)
		if source == "" {
			source = "skill_read"
		}
		line := fmt.Sprintf("- %s: loaded by %s", name, source)
		if reason := strings.TrimSpace(ref.Reason); reason != "" {
			line += "; reason=" + reason
		}
		if refRange := strings.TrimSpace(ref.Range); refRange != "" {
			line += "; range=" + refRange
		}
		if hash := strings.TrimSpace(ref.Hash); hash != "" {
			line += "; hash=" + hash
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
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
		lines = append(lines, renderActivatedSkillAsset(asset, i+1))
	}
	return strings.Join(lines, "\n")
}

const progressiveSkillAssetThresholdRunes = 1600

func renderActivatedSkillAsset(asset string, index int) string {
	asset = strings.TrimSpace(asset)
	if asset == "" {
		return ""
	}
	if !shouldProgressivelyDiscloseSkillAsset(asset) {
		return asset
	}
	summary := skillAssetCapabilitySummary(asset)
	if summary == "" {
		summary = "Long skill body is available through explicit skill_read or prompt trace; it is not inlined in the initial prompt."
	}
	lines := []string{
		"summary_type: progressive_disclosure",
		fmt.Sprintf("name: activated-skill-%d", index),
		"trigger: active_skill",
		"capability_summary: " + summary,
		fmt.Sprintf("entry_tool_or_source_ref: skill_read or prompt_trace://dynamic.skill/asset-%d", index),
		"full_body_policy: Do not rely on omitted long body text unless it was explicitly loaded by skill_read or recovered from trace.",
	}
	return strings.Join(lines, "\n")
}

func shouldProgressivelyDiscloseSkillAsset(asset string) bool {
	asset = strings.TrimSpace(asset)
	if asset == "" {
		return false
	}
	if utf8.RuneCountInString(asset) > progressiveSkillAssetThresholdRunes {
		return true
	}
	lower := strings.ToLower(asset)
	if strings.Contains(lower, "skill.md") || strings.Contains(lower, "when_to_use:") || strings.Contains(lower, "when to use:") {
		return true
	}
	return strings.Contains(lower, "allowed_tools:") || strings.Contains(lower, "denied_tools:")
}

func skillAssetCapabilitySummary(asset string) string {
	asset = strings.TrimSpace(asset)
	if asset == "" {
		return ""
	}
	lines := strings.Split(asset, "\n")
	for _, prefix := range []string{"description:", "when_to_use:", "whenToUse:", "when to use:"} {
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
				value := strings.Trim(strings.TrimSpace(line[len(prefix):]), `"'`)
				if value != "" {
					return summarizeDynamicOverflow(value, 220)
				}
			}
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" || strings.HasPrefix(line, "```") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		if line != "" {
			return summarizeDynamicOverflow(line, 220)
		}
	}
	return summarizeDynamicOverflow(asset, 220)
}
