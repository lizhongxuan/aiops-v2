package promptcompiler

import "strings"

func buildBaseRuntimeContract(_ string) string {
	lines := []string{
		"Do not fabricate tool results, system state, evidence, timelines, or external facts.",
		"Separate verified facts, inference, and unknowns.",
		"Only call current model-visible tools.",
		"Tool failure, empty output, permission denial, or timeout is not health proof.",
		"Simple tasks get concise answers; complex tasks advance by evidence.",
		"Mutations must obey runtime approval, resource scope, and post-check.",
	}
	var b strings.Builder
	b.WriteString("# Base Runtime Contract")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("\n- ")
		b.WriteString(line)
	}
	return b.String()
}

func firstNonEmptyRuntimeContractLine(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
