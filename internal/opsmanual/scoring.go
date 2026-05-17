package opsmanual

import (
	"fmt"
	"math"
	"strings"
)

const (
	candidateMinScore     = 0.55
	strongSemanticScore   = 0.82
	directExecuteMinScore = 0.82
)

type VectorScorer interface {
	Score(query string, manual OpsManual) (float64, bool)
}

func calculateScoreBreakdown(manual OpsManual, frame OperationFrame, summary RunRecordSummary, scorer VectorScorer) ScoreBreakdown {
	structural := structuralScore(manual, frame)
	keyword := keywordScore(manual, frame)
	vector := 0.0
	if scorer != nil {
		if score, ok := scorer.Score(frame.RawText, manual); ok {
			vector = clamp01(score)
		}
	}
	runHistory := runHistoryScore(summary)
	penalty := scorePenalty(manual, frame, summary)
	final := structural*0.85 + keyword*0.10 + vector*0.03 + runHistory*0.02 - penalty
	return ScoreBreakdown{
		StructuralScore: roundScore(structural),
		KeywordScore:    roundScore(keyword),
		VectorScore:     roundScore(vector),
		RunHistoryScore: roundScore(runHistory),
		Penalty:         roundScore(penalty),
		FinalScore:      roundScore(clamp01(final)),
	}
}

func structuralScore(manual OpsManual, frame OperationFrame) float64 {
	score := 0.0
	manualTarget := manualTargetType(manual)
	if manualTarget != "" && equalFold(manualTarget, frame.Target.Type) {
		score += 0.30
	}
	if operationsCompatibleForSearch(manual.Operation.Action, frame.Operation.Action) {
		score += 0.25
	}
	if listMatches(manual.Applicability.ExecutionSurface, frame.Environment.ExecutionSurface) && frame.Environment.ExecutionSurface != "" {
		score += 0.12
	}
	if listMatches(manual.Applicability.Platform, frame.Environment.Platform) && frame.Environment.Platform != "" {
		score += 0.10
	}
	if (listMatches(manual.Applicability.OS, frame.Environment.OS) && frame.Environment.OS != "") ||
		(listMatches(manual.Applicability.Topology, frame.Environment.Runtime) && frame.Environment.Runtime != "") {
		score += 0.08
	}
	if len(missingFieldsForManual(manual, frame)) == 0 {
		score += 0.10
	}
	if !riskExceedsManual(frame.Risk.Level, firstNonEmpty(manual.RunnableConditions.MaxRiskLevel, manual.Operation.RiskLevel)) {
		score += 0.05
	}
	return clamp01(score)
}

func keywordScore(manual OpsManual, frame OperationFrame) float64 {
	query := tokenSet(frame.RawText + " " + frame.Target.Type + " " + frame.Operation.Action)
	if len(query) == 0 {
		return 0
	}
	profile := effectiveRetrievalProfile(manual)
	source := strings.Join(manual.Tags, " ") + " " + manual.SearchDoc + " " + profile.EmbeddingText + " " + strings.Join(profile.Keywords, " ")
	for key, aliases := range profile.Aliases {
		source += " " + key + " " + strings.Join(aliases, " ")
	}
	for _, alias := range objectAliases(manualTargetType(manual)) {
		source += " " + alias
	}
	for _, alias := range operationAliases(manual.Operation.Action) {
		source += " " + alias
	}
	corpus := tokenSet(source)
	if len(corpus) == 0 {
		return 0
	}
	matches := 0
	for token := range query {
		if corpus[token] {
			matches++
		}
	}
	return clamp01(float64(matches) / math.Min(float64(len(query)), 8))
}

func scorePenalty(manual OpsManual, frame OperationFrame, summary RunRecordSummary) float64 {
	penalty := 0.0
	if manualTarget := manualTargetType(manual); manualTarget != "" && frame.Target.Type != "" && !equalFold(manualTarget, frame.Target.Type) {
		penalty += 0.45
	}
	if manual.Operation.Action != "" && frame.Operation.Action != "" && !operationsCompatibleForSearch(manual.Operation.Action, frame.Operation.Action) {
		penalty += 0.35
	}
	if noRestartConflict(manual, frame) {
		penalty += 0.50
	}
	if riskExceedsManual(frame.Risk.Level, firstNonEmpty(manual.RunnableConditions.MaxRiskLevel, manual.Operation.RiskLevel)) {
		penalty += 0.50
	}
	if latestRunFailed(summary) {
		penalty += 0.30
	}
	for _, negative := range effectiveRetrievalProfile(manual).NegativeKeywords {
		if strings.Contains(normalizeText(frame.RawText), strings.ToLower(negative)) {
			penalty += 0.25
		}
	}
	return clamp01(penalty)
}

func runHistoryScore(summary RunRecordSummary) float64 {
	if latestRunFailed(summary) {
		return 0
	}
	score := float64(summary.SuccessCount) * 0.2
	if summary.RecentResult == "passed" || summary.RecentResult == "success" {
		score += 0.4
	}
	score -= float64(summary.FailureCount) * 0.2
	return clamp01(score)
}

func effectiveRetrievalProfile(manual OpsManual) RetrievalProfile {
	profile := manual.RetrievalProfile
	metaProfile := retrievalProfileFromMetadata(manual.Metadata)
	profile.Keywords = dedupe(append(cloneStrings(profile.Keywords), metaProfile.Keywords...))
	profile.NegativeKeywords = dedupe(append(cloneStrings(profile.NegativeKeywords), metaProfile.NegativeKeywords...))
	if profile.EmbeddingText == "" {
		profile.EmbeddingText = metaProfile.EmbeddingText
	}
	if profile.MinScore.Candidate == 0 {
		profile.MinScore.Candidate = metaProfile.MinScore.Candidate
	}
	if profile.MinScore.DirectExecute == 0 {
		profile.MinScore.DirectExecute = metaProfile.MinScore.DirectExecute
	}
	if profile.Aliases == nil {
		profile.Aliases = map[string][]string{}
	}
	for key, values := range metaProfile.Aliases {
		profile.Aliases[key] = dedupe(append(cloneStrings(profile.Aliases[key]), values...))
	}
	if len(profile.Aliases) == 0 {
		profile.Aliases = nil
	}
	return profile
}

func retrievalProfileFromMetadata(meta map[string]any) RetrievalProfile {
	raw, ok := meta["retrieval_profile"]
	if !ok {
		return RetrievalProfile{}
	}
	profileMap, ok := raw.(map[string]any)
	if !ok {
		return RetrievalProfile{}
	}
	return RetrievalProfile{
		Aliases:          metadataStringSliceMap(profileMap, "aliases"),
		Keywords:         metadataStringSliceFromAny(profileMap["keywords"]),
		NegativeKeywords: metadataStringSliceFromAny(firstAny(profileMap["negative_keywords"], profileMap["negativeKeywords"])),
		EmbeddingText:    fmt.Sprint(firstAny(profileMap["embedding_text"], profileMap["embeddingText"])),
		MinScore: ScoreThresholds{
			Candidate:     metadataFloat(profileMap, "candidate"),
			DirectExecute: metadataFloat(profileMap, "direct_execute"),
		},
	}
}

func metadataStringSliceMap(meta map[string]any, key string) map[string][]string {
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string][]string, len(rawMap))
	for mapKey, value := range rawMap {
		out[mapKey] = metadataStringSliceFromAny(value)
	}
	return out
}

func metadataStringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := []string{}
		for _, item := range typed {
			out = appendUnique(out, fmt.Sprint(item))
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func metadataFloat(meta map[string]any, key string) float64 {
	raw, ok := meta[key]
	if !ok {
		if minScore, ok := meta["min_score"].(map[string]any); ok {
			return metadataFloat(minScore, key)
		}
		return 0
	}
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return ""
}

func tokenSet(text string) map[string]bool {
	normalized := normalizeText(text)
	out := map[string]bool{}
	for _, field := range strings.Fields(normalized) {
		token := strings.Trim(field, " ._/\\-()[]{}")
		if len(token) < 2 {
			continue
		}
		out[token] = true
	}
	return out
}

func manualTargetType(manual OpsManual) string {
	return strings.TrimSpace(firstNonEmpty(manual.Operation.TargetType, manual.Applicability.Middleware))
}

func noRestartConflict(manual OpsManual, frame OperationFrame) bool {
	return equalFold(manual.Operation.Action, "restart") &&
		!frame.Risk.ServiceRestart &&
		(strings.Contains(normalizeText(frame.RawText), "no restart") ||
			strings.Contains(normalizeText(frame.RawText), "不重启") ||
			strings.Contains(normalizeText(frame.RawText), "readonly") ||
			strings.Contains(normalizeText(frame.RawText), "只读"))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func roundScore(value float64) float64 {
	return math.Round(value*1000) / 1000
}
