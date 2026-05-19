package opsmanual

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/memory"
)

func TestLearningSummaryFromRunRecordIsRedactedAndAllowlisted(t *testing.T) {
	record, err := BuildRunRecordFromWorkflowResult(WorkflowResult{
		ID:              "run-learning-1",
		ManualID:        "manual-redis-rca",
		WorkflowID:      "workflow-redis-rca",
		OpsManualFlowID: "flow-redis-learning",
		OperationFrame: OperationFrame{
			Target:      OperationTarget{Type: "redis", Name: "redis-primary-01"},
			Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
			Environment: EnvironmentProfile{Platform: "vm"},
		},
		Parameters: map[string]any{
			"target_instance": "redis-primary-01",
			"password":        "plain-password",
		},
		DryRunStatus:     "passed",
		ExecutionStatus:  "success",
		ValidationStatus: "passed",
		FailureReason:    "ignored token=raw-token password=raw-password",
		CompletedAt:      "2026-05-19T11:00:00Z",
	})
	if err != nil {
		t.Fatalf("BuildRunRecordFromWorkflowResult() error = %v", err)
	}

	summary := BuildRedactedLearningSummaryFromRunRecord(record, "applicable")
	serialized := summary.Text + " " + summary.TargetAlias
	for _, forbidden := range []string{"plain-password", "raw-token", "raw-password", "password=", "token="} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("summary leaked %q: %#v", forbidden, summary)
		}
	}
	if summary.ManualID != "manual-redis-rca" || summary.WorkflowID != "workflow-redis-rca" || summary.ObjectType != "redis" || summary.Action != "rca_or_repair" {
		t.Fatalf("summary = %#v, want allowlisted manual/workflow/object/action", summary)
	}
	if summary.TargetAlias != "redis-primary-01" || summary.UserFeedback != "applicable" || !summary.Redacted {
		t.Fatalf("summary = %#v, want target alias, feedback and redacted marker", summary)
	}
}

func TestLearningSummaryFromManualGuidedEventDoesNotCreateWorkflowRunSummary(t *testing.T) {
	summary := BuildRedactedLearningSummaryFromManualGuidedEvent(ManualGuidedChatEvent{
		ID:                "manual-guided-learning-1",
		ManualID:          "manual-redis-rca",
		WorkflowID:        "workflow-redis-rca",
		OpsManualFlowID:   "flow-reference",
		ReferenceMode:     "manual_guided_chat",
		StageSummary:      "只读参考手册完成排查，secret=raw-secret",
		MutationRequested: false,
		WorkflowRunID:     "",
		CreatedAt:         "2026-05-19T11:10:00Z",
	}, "reference_useful")
	if summary.WorkflowRunID != "" || summary.ResultSummary != "manual_guided_chat" {
		t.Fatalf("summary = %#v, want manual-guided summary without workflow run", summary)
	}
	if strings.Contains(summary.Text, "raw-secret") || strings.Contains(summary.Text, "secret=") {
		t.Fatalf("summary leaked secret: %#v", summary)
	}
}

func TestWriteRedactedLearningSummaryHonorsPolicyAndWritesMemoryHint(t *testing.T) {
	store := memory.NewJSONStore(memory.Config{Path: filepath.Join(t.TempDir(), "memory.json"), Enabled: true})
	record := RunRecord{
		ID:              "run-learning-write",
		ManualID:        "manual-redis-rca",
		WorkflowID:      "workflow-redis-rca",
		OpsManualFlowID: "flow-redis-learning",
		OperationFrame: OperationFrame{
			Target:      OperationTarget{Type: "redis", Name: "redis-primary-01"},
			Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
			Environment: EnvironmentProfile{Platform: "vm"},
		},
		RedactedParameters: map[string]any{"target_instance": "redis-primary-01"},
		ValidationStatus:   "passed",
		CompletedAt:        "2026-05-19T11:00:00Z",
	}

	skipped, err := WriteRedactedLearningSummary(context.Background(), store, LearningSummaryWriteRequest{
		RunRecord:     &record,
		AllowWrite:    false,
		SkippedReason: "policy_disabled",
		Scope:         memory.ScopeProject,
		ProjectID:     "proj",
	})
	if err != nil {
		t.Fatalf("WriteRedactedLearningSummary(skip) error = %v", err)
	}
	if skipped.Written || skipped.SkippedReason != "policy_disabled" {
		t.Fatalf("skipped = %#v, want explicit policy skip", skipped)
	}

	written, err := WriteRedactedLearningSummary(context.Background(), store, LearningSummaryWriteRequest{
		RunRecord:                   &record,
		UserFeedback:                "applicable",
		AllowWrite:                  true,
		EnvironmentProfileCandidate: true,
		Scope:                       memory.ScopeProject,
		ProjectID:                   "proj",
		Now:                         time.Date(2026, 5, 19, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("WriteRedactedLearningSummary(write) error = %v", err)
	}
	if !written.Written || written.Item.Kind != memory.KindOpsManualManualHint || !written.Item.Redacted || written.Item.Source != "memory_hint" {
		t.Fatalf("written = %#v, want redacted memory hint", written)
	}
	results, err := store.Search(context.Background(), memory.Query{Scope: memory.ScopeProject, ProjectID: "proj", Text: "redis", Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].ManualID != "manual-redis-rca" || results[0].TargetAlias != "redis-primary-01" {
		t.Fatalf("results = %#v, want persisted learning summary", results)
	}
}
