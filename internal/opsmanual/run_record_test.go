package opsmanual

import "testing"

func TestBuildRunRecordFromWorkflowResultRedactsParametersAndSetsDigest(t *testing.T) {
	record, err := BuildRunRecordFromWorkflowResult(WorkflowResult{
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
}
