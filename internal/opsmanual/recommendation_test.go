package opsmanual

import "testing"

func TestWorkflowHighConfidenceRequiresTargetCompatibility(t *testing.T) {
	base := SearchManualHit{
		Manual: OpsManual{
			ID:    "manual-redis-repair",
			Title: "Redis 修复手册",
			WorkflowRef: WorkflowRef{
				WorkflowID: "workflow-redis-repair",
			},
			Operation: OperationProfile{
				TargetType: "redis",
				Action:     "repair",
				RiskLevel:  "medium",
			},
			Applicability: ApplicabilityProfile{
				Middleware: "redis",
			},
		},
		BoundWorkflowID:  "workflow-redis-repair",
		MatchLevel:       "same_object_same_operation",
		UsableMode:       DecisionDirectExecute,
		RunRecordSummary: RunRecordSummary{SuccessCount: 2},
	}

	compatible := EnrichSearchManualHitRecommendation(base)
	if compatible.Confidence != "high" || compatible.TraceOnly {
		t.Fatalf("compatible confidence/trace_only = %q/%v, want high/false", compatible.Confidence, compatible.TraceOnly)
	}
	if compatible.Workflow.ID != "workflow-redis-repair" || compatible.Workflow.RequiredTarget != "redis" {
		t.Fatalf("compatible workflow = %#v, want workflow summary with required target", compatible.Workflow)
	}

	incompatible := base
	incompatible.EnvironmentDiffs = []string{"target_type differs: mysql != redis"}
	incompatible = EnrichSearchManualHitRecommendation(incompatible)
	if incompatible.Confidence == "high" || !incompatible.TraceOnly {
		t.Fatalf("incompatible confidence/trace_only = %q/%v, want non-high/true", incompatible.Confidence, incompatible.TraceOnly)
	}
	if containsRecommendationString(incompatible.MatchReasons, "no_environment_conflict") {
		t.Fatalf("incompatible match reasons = %#v, must not claim no environment conflict", incompatible.MatchReasons)
	}
}

func containsRecommendationString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
