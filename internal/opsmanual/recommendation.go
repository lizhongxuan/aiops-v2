package opsmanual

import "strings"

func EnrichSearchManualHitRecommendation(hit SearchManualHit) SearchManualHit {
	hit.HistoryStats = RecommendationHistoryStatsFromRunSummary(hit.RunRecordSummary)
	hit.Workflow = WorkflowRecommendationSummaryFromHit(hit)
	hit.ApplicabilityBoundary = searchHitApplicabilityBoundary(hit)
	hit.NotApplicableWhen = searchHitNotApplicableWhen(hit)
	hit.MatchReasons = searchHitMatchReasons(hit)
	hit.Confidence = searchHitConfidence(hit)
	hit.TraceOnly = hit.Confidence != "high"
	return hit
}

func RecommendationHistoryStatsFromRunSummary(summary RunRecordSummary) RecommendationHistoryStats {
	used := summary.UsedCount
	if used == 0 {
		used = summary.SuccessCount + summary.FailureCount
	}
	stats := RecommendationHistoryStats{
		UsedCount:    used,
		SkippedCount: summary.SkippedCount,
		SuccessCount: summary.SuccessCount,
		FailureCount: summary.FailureCount,
		RecentResult: strings.TrimSpace(summary.RecentResult),
		LatestStatus: strings.TrimSpace(summary.LatestStatus),
		LastRunAt:    strings.TrimSpace(summary.LastRunAt),
	}
	if used > 0 {
		stats.SuccessRate = (summary.SuccessCount * 100) / used
	}
	return stats
}

func WorkflowRecommendationSummaryFromHit(hit SearchManualHit) WorkflowRecommendationSummary {
	manual := hit.Manual
	riskLevel := strings.TrimSpace(manual.Operation.RiskLevel)
	requiredTarget := strings.TrimSpace(firstNonEmpty(manual.Operation.TargetType, manual.Applicability.Middleware))
	return WorkflowRecommendationSummary{
		ID:               strings.TrimSpace(firstNonEmpty(hit.BoundWorkflowID, manual.WorkflowRef.WorkflowID)),
		RiskLevel:        riskLevel,
		RequiredTarget:   requiredTarget,
		RequiresApproval: manual.RunnableConditions.RequiresApproval || riskLevelRequiresApproval(riskLevel) || len(manual.RiskPolicy.ApprovalRequiredWhen) > 0,
		RequiredParams:   cloneStrings(manual.RunnableConditions.RequiredParams),
	}
}

func searchHitConfidence(hit SearchManualHit) string {
	switch {
	case hit.UsableMode == DecisionDirectExecute &&
		hit.MatchLevel == "same_object_same_operation" &&
		len(hit.MissingFields) == 0 &&
		len(hit.EnvironmentDiffs) == 0 &&
		len(hit.BlockedReasons) == 0 &&
		hit.RunRecordSummary.SuccessCount > 0 &&
		!hit.RunRecordSummary.Suppressed:
		return "high"
	case hit.UsableMode == DecisionNeedInfo || hit.UsableMode == DecisionAdapt:
		return "medium"
	default:
		return "low"
	}
}

func searchHitMatchReasons(hit SearchManualHit) []string {
	reasons := []string{}
	if strings.TrimSpace(hit.MatchLevel) != "" {
		reasons = appendUnique(reasons, strings.TrimSpace(hit.MatchLevel))
	}
	for _, field := range hit.MatchedFields {
		if strings.TrimSpace(field) != "" {
			reasons = appendUnique(reasons, "matched_"+strings.TrimSpace(field))
		}
	}
	if hit.RunRecordSummary.SuccessCount > 0 && !hit.RunRecordSummary.Suppressed {
		reasons = appendUnique(reasons, "successful_run_history")
	}
	if strings.TrimSpace(firstNonEmpty(hit.BoundWorkflowID, hit.Manual.WorkflowRef.WorkflowID)) != "" {
		reasons = appendUnique(reasons, "workflow_bound")
	}
	if riskExplainable(hit.Manual) {
		reasons = appendUnique(reasons, "risk_explainable")
	}
	if len(hit.EnvironmentDiffs) == 0 && hit.MatchLevel == "same_object_same_operation" {
		reasons = appendUnique(reasons, "no_environment_conflict")
	}
	for _, source := range hit.HintSources {
		if strings.TrimSpace(source) != "" {
			reasons = appendUnique(reasons, strings.TrimSpace(source))
		}
	}
	return reasons
}

func searchHitApplicabilityBoundary(hit SearchManualHit) []string {
	manual := hit.Manual
	var out []string
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			out = appendUnique(out, key+"="+value)
		}
	}
	appendKV("target", firstNonEmpty(manual.Operation.TargetType, manual.Applicability.Middleware))
	appendKV("operation", manual.Operation.Action)
	appendKV("version", strings.Join(manual.Applicability.MiddlewareVersions, ","))
	appendKV("os", strings.Join(manual.Applicability.OS, ","))
	appendKV("platform", strings.Join(manual.Applicability.Platform, ","))
	appendKV("execution_surface", strings.Join(manual.Applicability.ExecutionSurface, ","))
	appendKV("risk", manual.Operation.RiskLevel)
	if manual.RiskPolicy.BlastRadius != "" {
		appendKV("blast_radius", manual.RiskPolicy.BlastRadius)
	}
	return out
}

func searchHitNotApplicableWhen(hit SearchManualHit) []string {
	var out []string
	for _, item := range hit.Manual.Diagnosis.NotApplicableWhen {
		out = appendUnique(out, strings.TrimSpace(item))
	}
	for _, item := range hit.Manual.CannotUseWhen {
		out = appendUnique(out, strings.TrimSpace(item))
	}
	for _, item := range hit.BlockedReasons {
		out = appendUnique(out, strings.TrimSpace(item))
	}
	return out
}

func riskExplainable(manual OpsManual) bool {
	return strings.TrimSpace(manual.Operation.RiskLevel) != "" ||
		manual.RiskPolicy.BlastRadius != "" ||
		manual.RiskPolicy.DataMutation ||
		manual.RiskPolicy.ServiceRestart != "" ||
		len(manual.RiskPolicy.ApprovalRequiredWhen) > 0
}

func riskLevelRequiresApproval(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "high", "critical":
		return true
	default:
		return false
	}
}
