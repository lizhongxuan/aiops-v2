package opsmanual

import "testing"

func TestMigrateLegacyExperiencePackMapsMinimalDataToOpsManualCompatibility(t *testing.T) {
	result, err := MigrateLegacyExperiencePack(LegacyExperiencePack{
		ID:              "exp-redis-memory",
		Name:            "Redis memory repair",
		Summary:         "Diagnose and repair Redis memory pressure.",
		Status:          "verified",
		WorkflowID:      "wf-redis-memory",
		WorkflowVersion: "v3",
		WorkflowYAML:    "version: v3\nname: redis memory\n",
		TargetType:      "redis",
		Action:          "repair",
		RequiredInputs:  []string{"target_instance"},
		Validation:      []string{"memory stable"},
		CannotUseWhen:   []string{"target instance unknown"},
		RunRecords: []LegacyRunRecord{
			{WorkflowID: "wf-redis-memory", ExecutionStatus: "succeeded", ValidationStatus: "passed", CompletedAt: "2026-05-14T02:00:00Z"},
			{WorkflowID: "wf-redis-memory", ExecutionStatus: "failed", ValidationStatus: "failed", FailureReason: "timeout", CompletedAt: "2026-05-14T03:00:00Z"},
		},
	})
	if err != nil {
		t.Fatalf("MigrateLegacyExperiencePack() error = %v", err)
	}
	if result.Manual.ID != "manual-exp-redis-memory" || result.Manual.Status != ManualStatusVerified {
		t.Fatalf("Manual id/status = %q/%q, want compatible verified manual", result.Manual.ID, result.Manual.Status)
	}
	if result.Manual.WorkflowRef.WorkflowID != "wf-redis-memory" || result.Manual.WorkflowRef.WorkflowDigest == "" {
		t.Fatalf("WorkflowRef = %#v, want workflow id and digest", result.Manual.WorkflowRef)
	}
	if result.Manual.Operation.TargetType != "redis" || result.Manual.Operation.Action != "repair" {
		t.Fatalf("Operation = %#v, want legacy target/action", result.Manual.Operation)
	}
	if result.RunRecordSummary.SuccessCount != 1 || result.RunRecordSummary.FailureCount != 1 || result.RunRecordSummary.RecentResult != "failed" {
		t.Fatalf("RunRecordSummary = %#v, want migrated counts and recent failure", result.RunRecordSummary)
	}
}
