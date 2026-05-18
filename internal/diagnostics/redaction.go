package diagnostics

import (
	"regexp"
)

const redactedValue = "[REDACTED]"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)("(?:password|passwd|pwd|token|secret|api[_-]?key)"\s*:\s*")([^"]+)`),
	regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|secret|api[_-]?key|authorization)\b\s*[:=]\s*Bearer\s+)([^\s,;]+)`),
	regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|secret|api[_-]?key)\b\s*[:=]\s*)([^\s,;]+)`),
	regexp.MustCompile(`(?i)(\bAuthorization\s*:\s*(?:Bearer|Basic)\s+)([^\s,;]+)`),
	regexp.MustCompile(`(?i)((?:^|\s)(?:-a|--pass(?:word)?|--token|--secret)\s+)([^\s,;]+)`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`),
}

var urlCredentialPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)([^@\s,;]*?:)([^@\s,;]+)(@)`)

func RedactSensitiveText(text string) string {
	if text == "" {
		return ""
	}
	redacted := urlCredentialPattern.ReplaceAllString(text, "${1}${2}"+redactedValue+"${4}")
	for _, pattern := range sensitivePatterns {
		if pattern.NumSubexp() >= 2 {
			redacted = pattern.ReplaceAllString(redacted, "${1}"+redactedValue)
		} else {
			redacted = pattern.ReplaceAllString(redacted, redactedValue)
		}
	}
	return redacted
}

func RedactTrace(trace DiagnosticTrace) DiagnosticTrace {
	trace.ScopeSummary = RedactSensitiveText(trace.ScopeSummary)
	trace.Hypotheses = redactStrings(trace.Hypotheses)
	trace.ObservedEvidence = redactStrings(trace.ObservedEvidence)
	trace.RefutingEvidence = redactStrings(trace.RefutingEvidence)
	trace.MissingEvidence = redactStrings(trace.MissingEvidence)
	trace.ConfidenceReason = RedactSensitiveText(trace.ConfidenceReason)
	for i := range trace.ToolFailures {
		trace.ToolFailures[i].ToolName = RedactSensitiveText(trace.ToolFailures[i].ToolName)
		trace.ToolFailures[i].Detail = RedactSensitiveText(trace.ToolFailures[i].Detail)
	}
	return trace
}

func redactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	redacted := make([]string, 0, len(values))
	for _, value := range values {
		redacted = append(redacted, RedactSensitiveText(value))
	}
	return redacted
}
