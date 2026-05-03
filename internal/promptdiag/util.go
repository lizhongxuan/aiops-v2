package promptdiag

import (
	"regexp"
	"sort"
	"strings"
)

func cleanStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = appendUnique(out, value)
	}
	return out
}

func appendUnique(values []string, more ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values)+len(more))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range more {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func missingStrings(expected, actual []string) []string {
	actualSet := map[string]struct{}{}
	for _, value := range actual {
		actualSet[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	var missing []string
	for _, value := range expected {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := actualSet[strings.ToLower(value)]; !ok {
			missing = append(missing, value)
		}
	}
	return missing
}

func cleanMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendTrace(values []TraceLink, trace TraceLink) []TraceLink {
	key := trace.JSONPath + "\x00" + trace.MarkdownPath + "\x00" + trace.DiffPath
	for _, existing := range values {
		existingKey := existing.JSONPath + "\x00" + existing.MarkdownPath + "\x00" + existing.DiffPath
		if existingKey == key {
			return values
		}
	}
	return append(values, trace)
}

func appendSuggestion(values []Suggestion, suggestion Suggestion) []Suggestion {
	key := strings.TrimSpace(suggestion.Area) + "\x00" + strings.TrimSpace(suggestion.Action) + "\x00" + boolKey(suggestion.LLMAssisted)
	if key == "\x00" {
		return values
	}
	for _, existing := range values {
		existingKey := strings.TrimSpace(existing.Area) + "\x00" + strings.TrimSpace(existing.Action) + "\x00" + boolKey(existing.LLMAssisted)
		if existingKey == key {
			return values
		}
	}
	return append(values, suggestion)
}

func traceTurnSummaries(traces []TraceLink) []TraceTurnSummary {
	type state struct {
		summary TraceTurnSummary
		seen    map[int]struct{}
	}
	byKey := map[string]*state{}
	var order []string
	for _, trace := range traces {
		key := trace.SessionID + "\x00" + trace.TurnID
		if key == "\x00" {
			key = trace.JSONPath
		}
		current := byKey[key]
		if current == nil {
			current = &state{
				summary: TraceTurnSummary{
					CaseID:    trace.CaseID,
					SessionID: trace.SessionID,
					TurnID:    trace.TurnID,
					FirstAt:   trace.CreatedAt,
					LastAt:    trace.CreatedAt,
				},
				seen: map[int]struct{}{},
			}
			byKey[key] = current
			order = append(order, key)
		}
		if current.summary.CaseID == "" {
			current.summary.CaseID = trace.CaseID
		}
		if _, ok := current.seen[trace.Iteration]; !ok {
			current.seen[trace.Iteration] = struct{}{}
			current.summary.Iterations = append(current.summary.Iterations, trace.Iteration)
		}
		if current.summary.FirstAt == "" || (trace.CreatedAt != "" && trace.CreatedAt < current.summary.FirstAt) {
			current.summary.FirstAt = trace.CreatedAt
		}
		if trace.CreatedAt > current.summary.LastAt {
			current.summary.LastAt = trace.CreatedAt
		}
	}
	out := make([]TraceTurnSummary, 0, len(order))
	for _, key := range order {
		summary := byKey[key].summary
		sort.Ints(summary.Iterations)
		out = append(out, summary)
	}
	return out
}

func traceIterationCount(turns []TraceTurnSummary) int {
	count := 0
	for _, turn := range turns {
		count += len(turn.Iterations)
	}
	return count
}

func boolKey(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

var unsafeFileName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFileName(value string) string {
	value = unsafeFileName.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "draft-case"
	}
	return value
}
