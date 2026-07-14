package promptcompiler

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DynamicContextSourceEvidence         = "dynamic.evidence"
	DynamicContextSourceSkill            = "dynamic.skill"
	DynamicContextSourceHostTask         = "dynamic.host_task"
	DynamicContextSourceProtocol         = "dynamic.protocol"
	DynamicContextSourceMemory           = "dynamic.memory"
	DynamicContextSourceHistoryCompacted = "history.compacted"
)

func dynamicContextBudgetTokens(sourceID string) int {
	switch sourceID {
	case DynamicContextSourceEvidence:
		return 1000
	case DynamicContextSourceSkill:
		return 1000
	case DynamicContextSourceHostTask:
		return 800
	case DynamicContextSourceProtocol:
		return 500
	case DynamicContextSourceHistoryCompacted:
		return 1200
	case DynamicContextSourceMemory:
		return 500
	default:
		return 500
	}
}

func buildDynamicContextSources(ctx CompileContext, protocolContent string) []DynamicContextSource {
	var sources []DynamicContextSource
	if skillContext := joinNonEmpty(activeSkillContext(ctx.SkillPromptAssets), loadedSkillContext(ctx.LoadedSkillRefs)); skillContext != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceSkill, "Skill Context", skillContext))
	}
	if hostTaskContext := activeHostTaskContext(ctx.HostTaskPromptAssets); hostTaskContext != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceHostTask, "Host Task Context", hostTaskContext))
	}
	if evidenceContext := evidenceDynamicContext(ctx.EvidenceReminders, ctx.ExtraSections); evidenceContext != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceEvidence, "Evidence Context", evidenceContext))
	}
	if memoryContext := extraSectionsByDynamicSource(ctx.ExtraSections, DynamicContextSourceMemory); memoryContext != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceMemory, "Memory Context", memoryContext))
	}
	if compactedContext := extraSectionsByDynamicSource(ctx.ExtraSections, DynamicContextSourceHistoryCompacted); compactedContext != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceHistoryCompacted, "Compacted History", compactedContext))
	}
	if protocolContent = strings.TrimSpace(protocolContent); protocolContent != "" {
		sources = append(sources, boundedDynamicContextSource(DynamicContextSourceProtocol, "Protocol State", protocolContent))
	}
	return sources
}

func renderDynamicContextSources(sources []DynamicContextSource) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		rendered := renderDynamicContextSource(source)
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	return out
}

func renderDynamicContextSource(source DynamicContextSource) string {
	content := strings.TrimSpace(source.Content)
	if source.ID == "" || content == "" {
		return ""
	}
	lines := []string{
		"## " + firstNonEmpty(source.Title, source.ID),
		"source_id: " + source.ID,
		fmt.Sprintf("token_budget: %d", source.TokenBudget),
	}
	if source.Overflowed {
		lines = append(lines,
			"status: summarized_overflow",
			"summary: "+source.Summary,
			"source_ref: "+source.SourceRef,
			"evidence_ref: "+source.EvidenceRef,
			"raw_available_via_tool_or_trace: true",
		)
		return strings.Join(lines, "\n")
	}
	lines = append(lines, content)
	return strings.Join(lines, "\n")
}

func boundedDynamicContextSource(sourceID, title, raw string) DynamicContextSource {
	raw = strings.TrimSpace(raw)
	budget := dynamicContextBudgetTokens(sourceID)
	source := DynamicContextSource{
		ID:             sourceID,
		Title:          title,
		TokenBudget:    budget,
		TokensEstimate: estimateDynamicContextTokens(raw),
	}
	if raw == "" {
		return source
	}
	maxRunes := budget * 4
	if maxRunes <= 0 || utf8.RuneCountInString(raw) <= maxRunes {
		source.Content = raw
		return source
	}
	source.Overflowed = true
	source.RawAvailableViaTrace = true
	source.Summary = summarizeDynamicOverflow(raw, 320)
	source.SourceRef = "prompt_trace://" + sourceID
	source.EvidenceRef = sourceID + ":overflow"
	source.Content = source.Summary
	return source
}

func estimateDynamicContextTokens(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}
	tokens := utf8.RuneCountInString(content) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func summarizeDynamicOverflow(content string, maxRunes int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "..."
}

func evidenceDynamicContext(reminders []string, sections []PromptSection) string {
	parts := make([]string, 0, 2)
	if len(reminders) > 0 {
		lines := make([]string, 0, len(reminders)+1)
		lines = append(lines, "## Evidence Reminders")
		for _, reminder := range reminders {
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
	if extra := extraSectionsByDynamicSource(sections, DynamicContextSourceEvidence); extra != "" {
		parts = append(parts, extra)
	}
	return joinNonEmpty(parts...)
}

func extraSectionsByDynamicSource(sections []PromptSection, sourceID string) string {
	var parts []string
	for _, section := range sections {
		if dynamicSourceIDForExtraSection(section) != sourceID {
			continue
		}
		rendered := renderPromptSection(section)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return joinNonEmpty(parts...)
}

func dynamicSourceIDForExtraSection(section PromptSection) string {
	switch sourceType := strings.TrimSpace(section.SourceType); sourceType {
	case DynamicContextSourceEvidence, DynamicContextSourceSkill, DynamicContextSourceHostTask,
		DynamicContextSourceProtocol, DynamicContextSourceMemory, DynamicContextSourceHistoryCompacted:
		return sourceType
	}
	// Deprecated: compatibility-only classification for the read-only legacy
	// parity/trace view. The canonical EnvelopeV2 provider path consumes typed
	// PromptSection source facts and never uses this title adapter as control input.
	title := strings.ToLower(strings.TrimSpace(section.Title))
	switch {
	case strings.Contains(title, "memory") || strings.Contains(title, "letta") || strings.Contains(title, "session fact"):
		return DynamicContextSourceMemory
	case strings.Contains(title, "compact") || strings.Contains(title, "compacted") || strings.Contains(title, "history"):
		return DynamicContextSourceHistoryCompacted
	default:
		return DynamicContextSourceEvidence
	}
}

func renderPromptSection(section PromptSection) string {
	title := strings.TrimSpace(section.Title)
	content := strings.TrimSpace(section.Content)
	if title == "" && content == "" {
		return ""
	}
	if title == "" {
		return content
	}
	if content == "" {
		return "## " + title
	}
	return fmt.Sprintf("## %s\n%s", title, content)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
