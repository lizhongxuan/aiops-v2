package runtimekernel

import (
	"reflect"
	"testing"
	"time"
)

func TestCheckpointKindsDefineSemanticTypes(t *testing.T) {
	got := []string{
		CheckpointKindTurnStart,
		CheckpointKindToolSurfaceReady,
		CheckpointKindBeforeToolCall,
		CheckpointKindAfterToolCall,
		CheckpointKindApprovalWaiting,
		CheckpointKindApprovalResolved,
		CheckpointKindFinalResponse,
		CheckpointKindErrorRecovery,
	}
	want := []string{
		"checkpoint_turn_start",
		"checkpoint_tool_surface_ready",
		"checkpoint_before_tool_call",
		"checkpoint_after_tool_call",
		"checkpoint_approval_waiting",
		"checkpoint_approval_resolved",
		"checkpoint_final_response",
		"checkpoint_error_recovery",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("checkpoint kinds = %#v, want %#v", got, want)
	}
}

func TestNormalizeCheckpointKindMapsLegacyRuntimeKinds(t *testing.T) {
	tests := map[string]string{
		"assistant_response": CheckpointKindFinalResponse,
		"tool_result":        CheckpointKindAfterToolCall,
		"resume_tool_result": CheckpointKindAfterToolCall,
		"approval_needed":    CheckpointKindApprovalWaiting,
		"tool_denied":        CheckpointKindErrorRecovery,
		"model_call_failed":  CheckpointKindErrorRecovery,
		"turn_failed":        CheckpointKindErrorRecovery,
		"tool_progress":      CheckpointKindAfterToolCall,
	}
	for input, want := range tests {
		if got := NormalizeCheckpointKind(input); got != want {
			t.Fatalf("NormalizeCheckpointKind(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCheckpointMetadataCarriesReadOnlyProjectionFields(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	checkpoint := CheckpointMetadata{
		ID:                 "checkpoint-approval-1",
		SessionID:          "session-1",
		TurnID:             "turn-1",
		Iteration:          2,
		Sequence:           3,
		Kind:               CheckpointKindApprovalWaiting,
		Lifecycle:          TurnLifecycleSuspended,
		ResumeState:        TurnResumeStatePendingApproval,
		RunID:              "opsrun-turn-1",
		CurrentStepID:      "step-approval",
		ToolSurfaceSummary: "HostOps / Coroot RCA",
		TargetRefs:         []string{"service:checkout", "host:db-a"},
		EvidenceRefs:       []string{"evidence-coroot-1"},
		ApprovalState:      "waiting",
		Resumable:          true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("CheckpointMetadata.Validate() error = %v", err)
	}
	if checkpoint.RunID != "opsrun-turn-1" ||
		checkpoint.CurrentStepID != "step-approval" ||
		checkpoint.ToolSurfaceSummary != "HostOps / Coroot RCA" ||
		checkpoint.ApprovalState != "waiting" ||
		!checkpoint.Resumable ||
		!reflect.DeepEqual(checkpoint.TargetRefs, []string{"service:checkout", "host:db-a"}) ||
		!reflect.DeepEqual(checkpoint.EvidenceRefs, []string{"evidence-coroot-1"}) {
		t.Fatalf("checkpoint projection fields = %#v", checkpoint)
	}
}
