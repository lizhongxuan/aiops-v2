package promptinput

import (
	"fmt"
	"regexp"
	"strings"
)

// RenderMarkdown renders a human-readable semantic trace for a model input.
func RenderMarkdown(trace PromptInputTrace) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Prompt Input Trace")
	fmt.Fprintln(&b)
	if len(trace.Items) == 0 {
		fmt.Fprintln(&b, "_No prompt input trace items._")
		return b.String()
	}

	fmt.Fprintln(&b, "| # | source | semantic | provider | layer | id | status | content |")
	fmt.Fprintln(&b, "|---:|---|---|---|---|---|---|---|")
	for i, item := range trace.Items {
		fmt.Fprintf(
			&b,
			"| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			i,
			escapeMarkdownCell(item.Source),
			escapeMarkdownCell(item.SemanticRole),
			escapeMarkdownCell(item.ProviderRole),
			escapeMarkdownCell(item.PromptLayer),
			escapeMarkdownCell(item.ID),
			escapeMarkdownCell(item.Status),
			escapeMarkdownCell(redactSecrets(item.Content)),
		)
	}
	return b.String()
}

// RenderDiffMarkdown renders a redacted human-readable semantic diff.
func RenderDiffMarkdown(diff TraceDiff) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Prompt Input Diff")
	renderDiffItems(&b, "Added", "+", diff.Added)
	renderDiffItems(&b, "Removed", "-", diff.Removed)
	return b.String()
}

func renderDiffItems(b *strings.Builder, title, marker string, items []TraceItem) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	if len(items) == 0 {
		fmt.Fprintln(b, "_None._")
		return
	}
	for _, item := range items {
		fmt.Fprintf(
			b,
			"%s `%s/%s`",
			marker,
			item.Source,
			item.SemanticRole,
		)
		if item.ID != "" {
			fmt.Fprintf(b, " id=`%s`", escapeBackticks(item.ID))
		}
		if item.Status != "" {
			fmt.Fprintf(b, " status=`%s`", escapeBackticks(item.Status))
		}
		content := strings.TrimSpace(redactSecrets(item.Content))
		if content != "" {
			fmt.Fprintf(b, "\n\n```text\n%s\n```\n", content)
		} else {
			fmt.Fprintln(b)
		}
	}
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func escapeBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

var (
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	bearerPattern           = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	openAIKeyPattern        = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
)

func redactSecrets(content string) string {
	content = secretAssignmentPattern.ReplaceAllString(content, "$1=[REDACTED]")
	content = bearerPattern.ReplaceAllString(content, "Bearer [REDACTED]")
	content = openAIKeyPattern.ReplaceAllString(content, "sk-[REDACTED]")
	return content
}
