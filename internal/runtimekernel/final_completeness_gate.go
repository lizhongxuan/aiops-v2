package runtimekernel

import "strings"

const finalCompletenessRetryMetadataKey = "finalCompletenessRetry"

type FinalCompletenessDecision struct {
	Action  string
	Reasons []string
	Tail    string
}

func EvaluateFinalCompleteness(answer string) FinalCompletenessDecision {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return FinalCompletenessDecision{Action: "allow"}
	}

	reasons := make([]string, 0, 3)
	if endsWithOpenContinuation(trimmed) {
		reasons = append(reasons, "ends_with_continuation_marker")
	}
	if hasUnclosedFinalDelimiter(trimmed) {
		reasons = append(reasons, "unclosed_delimiter")
	}
	if hasUnclosedMarkdownFence(trimmed) {
		reasons = append(reasons, "unclosed_markdown_fence")
	}

	if len(reasons) == 0 {
		return FinalCompletenessDecision{Action: "allow"}
	}
	return FinalCompletenessDecision{
		Action:  "retry_complete_final",
		Reasons: reasons,
		Tail:    finalAnswerTail(trimmed, 160),
	}
}

func finalCompletenessRetryPrompt(decision FinalCompletenessDecision) string {
	reasons := strings.Join(decision.Reasons, ", ")
	if reasons == "" {
		reasons = "possible_truncation"
	}
	var builder strings.Builder
	builder.WriteString("## Final answer completeness guard\n")
	builder.WriteString("The previous final answer appears incomplete or truncated before the turn was completed. ")
	builder.WriteString("Reason: ")
	builder.WriteString(reasons)
	builder.WriteString(". Re-send one complete final answer using the existing evidence and conversation state. ")
	builder.WriteString("Do not repeat tool calls unless the existing evidence is actually insufficient. Finish every sentence, close any open delimiter or markdown block, and do not stop mid-value.")
	if strings.TrimSpace(decision.Tail) != "" {
		builder.WriteString("\n\nPrevious final answer tail:\n")
		builder.WriteString(decision.Tail)
	}
	return builder.String()
}

func endsWithOpenContinuation(text string) bool {
	last := lastRune(text)
	switch last {
	case '，', ',', '、', '：', ':', '；', ';', '（', '(', '【', '[', '{', '「', '“', '‘', '《':
		return true
	default:
		return false
	}
}

func hasUnclosedMarkdownFence(text string) bool {
	return strings.Count(text, "```")%2 == 1
}

func hasUnclosedFinalDelimiter(text string) bool {
	pairs := []struct {
		open  rune
		close rune
	}{
		{'（', '）'},
		{'(', ')'},
		{'【', '】'},
		{'[', ']'},
		{'{', '}'},
		{'「', '」'},
		{'“', '”'},
		{'‘', '’'},
		{'《', '》'},
	}
	for _, pair := range pairs {
		openCount, closeCount := 0, 0
		for _, r := range text {
			if r == pair.open {
				openCount++
			}
			if r == pair.close {
				closeCount++
			}
		}
		if openCount > closeCount {
			return true
		}
	}
	return false
}

func finalAnswerTail(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[len(runes)-maxRunes:])
}

func lastRune(text string) rune {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}
	return runes[len(runes)-1]
}
