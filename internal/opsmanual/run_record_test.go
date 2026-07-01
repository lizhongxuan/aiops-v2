package opsmanual

import "testing"

func TestBuildRunRecordFromWorkflowResultRedactsParametersAndSetsDigest(t *testing.T) {
	record, err := BuildRunRecordFromWorkflowResult(WorkflowResult{
		ID:              "run-flow-1",
		OpsManualFlowID: "flow-redis-1",
		SessionID:       "sess-1",
		ManualID:        "manual-redis",
		WorkflowID:      "wf-redis",
		WorkflowVersion: "v2",
		WorkflowYAML:    "version: v2\nname: redis\n",
		Parameters: map[string]any{
			"target_instance": "redis-1",
			"password":        "plain",
			"api_token":       "token",
			"nested": map[string]any{
				"secret_key": "hidden",
				"safe":       "visible",
			},
		},
		ExecutionStatus:  "failed",
		ValidationStatus: "failed",
		FailureReason:    "redis ping failed",
		StartedAt:        "2026-05-14T01:00:00Z",
		CompletedAt:      "2026-05-14T01:02:00Z",
	})
	if err != nil {
		t.Fatalf("BuildRunRecordFromWorkflowResult() error = %v", err)
	}
	if record.WorkflowDigest == "" {
		t.Fatalf("WorkflowDigest is empty, want computed digest")
	}
	if record.OpsManualFlowID != "flow-redis-1" || record.SessionID != "sess-1" {
		t.Fatalf("record flow/session = %#v, want flow and session association", record)
	}
	if record.RedactedParameters["target_instance"] != "redis-1" {
		t.Fatalf("target_instance = %#v, want preserved", record.RedactedParameters["target_instance"])
	}
	if record.RedactedParameters["password"] != RedactedValue || record.RedactedParameters["api_token"] != RedactedValue {
		t.Fatalf("RedactedParameters = %#v, want sensitive top-level values redacted", record.RedactedParameters)
	}
	nested, ok := record.RedactedParameters["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested = %#v, want map[string]any", record.RedactedParameters["nested"])
	}
	if nested["secret_key"] != RedactedValue || nested["safe"] != "visible" {
		t.Fatalf("nested = %#v, want recursive secret redaction only", nested)
	}
	if record.ValidationStatus != "failed" || record.FailureReason != "redis ping failed" {
		t.Fatalf("record status/failure = %#v, want validation status and failure reason", record)
	}
}

func TestSummarizeRunRecordsCountsValidationAndRecentResult(t *testing.T) {
	summary := SummarizeRunRecords([]RunRecord{
		{ID: "old-success", ValidationStatus: "passed", ExecutionStatus: "succeeded", CompletedAt: "2026-05-14T01:00:00Z"},
		{ID: "new-failure", ValidationStatus: "failed", ExecutionStatus: "failed", FailureReason: "timeout", CompletedAt: "2026-05-14T03:00:00Z"},
		{ID: "dry-run", DryRunStatus: "passed", StartedAt: "2026-05-14T02:00:00Z"},
	})
	if summary.SuccessCount != 1 || summary.FailureCount != 1 {
		t.Fatalf("summary counts = %#v, want one success and one failure", summary)
	}
	if summary.RecentResult != "failed" || summary.LastRunAt != "2026-05-14T03:00:00Z" {
		t.Fatalf("summary recent = %#v, want newest validation result", summary)
	}
	if summary.LatestStatus != "failed" || summary.ConsecutiveFailures != 1 || !summary.Suppressed {
		t.Fatalf("summary suppression = %#v, want latest failed suppression", summary)
	}
	if summary.SuppressedReason == "" {
		t.Fatalf("suppressed reason empty: %#v", summary)
	}
}

func TestSummarizeRunRecordsTracksConsecutiveFailuresAndRecovery(t *testing.T) {
	failed := SummarizeRunRecords([]RunRecord{
		{ID: "new-failure", ValidationStatus: "failed", ExecutionStatus: "failed", CompletedAt: "2026-05-14T04:00:00Z"},
		{ID: "old-failure", ValidationStatus: "failed", ExecutionStatus: "failed", CompletedAt: "2026-05-14T03:00:00Z"},
		{ID: "old-success", ValidationStatus: "passed", ExecutionStatus: "succeeded", CompletedAt: "2026-05-14T02:00:00Z"},
	})
	if failed.ConsecutiveFailures != 2 || !failed.Suppressed {
		t.Fatalf("failed summary = %#v, want two consecutive failures and suppression", failed)
	}
	if failed.SuppressedReason != "consecutive failures: 2" {
		t.Fatalf("suppressed reason = %q", failed.SuppressedReason)
	}

	recovered := SummarizeRunRecords([]RunRecord{
		{ID: "new-success", ValidationStatus: "passed", ExecutionStatus: "succeeded", CompletedAt: "2026-05-14T05:00:00Z"},
		{ID: "old-failure", ValidationStatus: "failed", ExecutionStatus: "failed", CompletedAt: "2026-05-14T04:00:00Z"},
	})
	if recovered.LatestStatus != "passed" || recovered.ConsecutiveFailures != 0 || recovered.Suppressed {
		t.Fatalf("recovered summary = %#v, want latest passed and no suppression", recovered)
	}
}

func TestOpsManualUsageStatsDistinguishUsedSkippedSucceededFailed(t *testing.T) {
	summary := SummarizeRunRecords([]RunRecord{
		{ID: "used-success", ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-14T04:00:00Z"},
		{ID: "used-failed", ExecutionStatus: "failed", FailureReason: "timeout", CompletedAt: "2026-05-14T03:00:00Z"},
		{ID: "skipped", ExecutionStatus: "skipped", UserFeedback: "user skipped recommendation", CompletedAt: "2026-05-14T02:00:00Z"},
	})
	if summary.UsedCount != 2 || summary.SkippedCount != 1 {
		t.Fatalf("summary usage counts = %#v, want two used and one skipped", summary)
	}
	if summary.SuccessCount != 1 || summary.FailureCount != 1 {
		t.Fatalf("summary success/failure = %#v, want skipped excluded from success and failure", summary)
	}
}
