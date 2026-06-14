package tooling

import (
	"sort"
	"strings"
	"unicode"
)

type PackTriggerMatch struct {
	Pack      string   `json:"pack"`
	ToolNames []string `json:"toolNames,omitempty"`
	Score     int      `json:"score"`
	MatchedBy []string `json:"matchedBy,omitempty"`
}

func MatchToolPacksByMetadata(tools []Tool, input string) []PackTriggerMatch {
	terms := triggerTerms(input)
	if len(terms) == 0 {
		return nil
	}
	normalizedInput := strings.ToLower(input)
	byPack := map[string]*PackTriggerMatch{}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if meta.Pack == "" || ToolHiddenFromDiscovery(meta) {
			continue
		}
		if meta.Layer != ToolLayerDeferred && !meta.DeferByDefault && !meta.EffectiveDiscovery().RequiresSelect {
			continue
		}
		score, matchedBy := scorePackTrigger(meta, terms, normalizedInput)
		if score == 0 {
			continue
		}
		match := byPack[meta.Pack]
		if match == nil {
			match = &PackTriggerMatch{Pack: meta.Pack}
			byPack[meta.Pack] = match
		}
		match.Score += score
		match.ToolNames = append(match.ToolNames, meta.Name)
		match.MatchedBy = append(match.MatchedBy, matchedBy...)
	}
	out := make([]PackTriggerMatch, 0, len(byPack))
	for _, match := range byPack {
		match.ToolNames = uniqueSorted(match.ToolNames)
		match.MatchedBy = uniqueSorted(match.MatchedBy)
		out = append(out, *match)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Pack < out[j].Pack
	})
	return out
}

func scorePackTrigger(meta ToolMetadata, terms map[string]struct{}, normalizedInput string) (int, []string) {
	d := meta.EffectiveDiscovery()
	candidates := append([]string(nil), meta.Triggers...)
	candidates = append(candidates, meta.SearchHint, d.CapabilityKind)
	candidates = append(candidates, d.DiscoveryTags...)
	candidates = append(candidates, d.ResourceTypes...)
	candidates = append(candidates, d.OperationKinds...)
	score := 0
	var matched []string
	for _, candidate := range candidates {
		normalizedCandidate := strings.ToLower(strings.TrimSpace(candidate))
		if shouldMatchTriggerBySubstring(normalizedCandidate) && strings.Contains(normalizedInput, normalizedCandidate) {
			score++
			matched = append(matched, normalizedCandidate)
			continue
		}
		candidateTerms := normalizedTriggerTerms(candidate)
		if len(candidateTerms) > 1 {
			if allTermsPresent(candidateTerms, terms) {
				score += len(candidateTerms)
				matched = append(matched, strings.Join(candidateTerms, " "))
			}
			continue
		}
		for _, term := range candidateTerms {
			if term == "" {
				continue
			}
			if _, ok := terms[term]; ok {
				score++
				matched = append(matched, term)
			}
		}
	}
	return score, matched
}

func normalizedTriggerTerms(input string) []string {
	var out []string
	for _, term := range strings.Fields(strings.ToLower(input)) {
		term = strings.Trim(term, ".,;:!?()[]{}\"'")
		if term != "" {
			out = append(out, term)
		}
	}
	return out
}

func allTermsPresent(candidates []string, terms map[string]struct{}) bool {
	for _, term := range candidates {
		if _, ok := terms[term]; !ok {
			return false
		}
	}
	return true
}

func shouldMatchTriggerBySubstring(candidate string) bool {
	if candidate == "" {
		return false
	}
	if strings.Contains(candidate, "://") {
		return true
	}
	for _, r := range candidate {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func triggerTerms(input string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, term := range normalizedTriggerTerms(input) {
		out[term] = struct{}{}
	}
	return out
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
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
	sort.Strings(out)
	return out
}
