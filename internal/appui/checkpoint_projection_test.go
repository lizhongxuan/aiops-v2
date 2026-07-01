package appui

import (
	"reflect"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

func TestCheckpointProjectionIncludesApprovalWaitingState(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata:    map[string]string{metadataOpsRunID: "opsrun-turn-1", "aiops.target.summary": "service:checkout"},
		ExternalReferences: []runtimekernel.ExternalReference{{
			ID:        "evidence-coroot-1",
			Kind:      "blob",
			CreatedAt: now,
		}},
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:           "checkpoint-approval-1",
			SessionID:    "session-1",
			TurnID:       "turn-1",
			Iteration:    1,
			Sequence:     2,
			Kind:         "approval_needed",
			Lifecycle:    runtimekernel.TurnLifecycleSuspended,
			ResumeState:  runtimekernel.TurnResumeStatePendingApproval,
			ExternalRefs: []string{"evidence-coroot-1"},
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:             "approval-1",
			SessionID:      "session-1",
			TurnID:         "turn-1",
			Iteration:      1,
			ToolName:       "host.exec",
			ToolCallID:     "tool-call-1",
			HostID:         "host-a",
			ResourceScopes: []string{"service:checkout"},
			CreatedAt:      now,
		}},
	}
	run := &AgentRunView{
		ID:            "opsrun-turn-1",
		CurrentStepID: "step-approval",
		CheckpointID:  "checkpoint-approval-1",
		Steps: []AgentStepView{{
			ID:           "step-approval",
			Kind:         AgentStepKindApproval,
			CheckpointID: "checkpoint-approval-1",
		}},
	}

	checkpoints := BuildCheckpointSummariesFromTurn(turn, run)
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint summaries = %#v, want one approval checkpoint", checkpoints)
	}
	got := checkpoints[0]
	if got.ID != "checkpoint-approval-1" ||
		got.Kind != runtimekernel.CheckpointKindApprovalWaiting ||
		got.TurnID != "turn-1" ||
		got.StepID != "step-approval" ||
		got.ApprovalState != "waiting" ||
		!got.Resumable {
		t.Fatalf("approval checkpoint summary = %#v", got)
	}
	if !reflect.DeepEqual(got.TargetRefs, []string{"host-a", "service:checkout"}) {
		t.Fatalf("target refs = %#v, want host and service refs", got.TargetRefs)
	}
	if !reflect.DeepEqual(got.EvidenceRefs, []string{"evidence-coroot-1"}) {
		t.Fatalf("evidence refs = %#v, want external refs", got.EvidenceRefs)
	}
}

func TestCheckpointProjectionLinksCheckpointToAgentStep(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 10, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		Iteration:   2,
		StartedAt:   now,
		UpdatedAt:   now,
		Iterations: []runtimekernel.IterationState{{
			ID:          "iter-1",
			SessionID:   "session-1",
			TurnID:      "turn-1",
			Iteration:   2,
			Lifecycle:   runtimekernel.TurnLifecycleRunning,
			ResumeState: runtimekernel.TurnResumeStateNone,
			Checkpoint: &runtimekernel.CheckpointMetadata{
				ID:            "checkpoint-tool-1",
				SessionID:     "session-1",
				TurnID:        "turn-1",
				Iteration:     2,
				Sequence:      1,
				Kind:          "tool_result",
				CurrentStepID: "step-tool-call",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			StartedAt: now,
			UpdatedAt: now,
		}},
	}
	run := &AgentRunView{
		ID: "opsrun-turn-1",
		Steps: []AgentStepView{{
			ID:           "step-tool-call",
			Kind:         AgentStepKindToolCall,
			CheckpointID: "checkpoint-tool-1",
		}},
	}

	checkpoints := BuildCheckpointSummariesFromTurn(turn, run)
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint summaries = %#v, want one iteration checkpoint", checkpoints)
	}
	if got := checkpoints[0]; got.StepID != "step-tool-call" || got.Kind != runtimekernel.CheckpointKindAfterToolCall {
		t.Fatalf("linked checkpoint summary = %#v", got)
	}
}

func TestCheckpointProjectionKeepsResumeMetadataReadOnly(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 20, 0, 0, time.UTC)
	checkpoint := &runtimekernel.CheckpointMetadata{
		ID:           "checkpoint-resume-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Iteration:    3,
		Sequence:     4,
		Kind:         "resume_checkpoint",
		Lifecycle:    runtimekernel.TurnLifecycleRunning,
		ResumeState:  runtimekernel.TurnResumeStateCheckpointReady,
		TargetRefs:   []string{"service:checkout"},
		EvidenceRefs: []string{"evidence-1"},
		Resumable:    true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:               "turn-1",
		SessionID:        "session-1",
		SessionType:      runtimekernel.SessionTypeWorkspace,
		Mode:             runtimekernel.ModeChat,
		Lifecycle:        runtimekernel.TurnLifecycleRunning,
		ResumeState:      runtimekernel.TurnResumeStateCheckpointReady,
		Iteration:        3,
		StartedAt:        now,
		UpdatedAt:        now,
		LatestCheckpoint: checkpoint,
	}
	run := &AgentRunView{ID: "opsrun-turn-1", CheckpointID: "checkpoint-resume-1"}

	originalKind := checkpoint.Kind
	originalTargets := append([]string(nil), checkpoint.TargetRefs...)
	checkpoints := BuildCheckpointSummariesFromTurn(turn, run)
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint summaries = %#v, want one resume checkpoint", checkpoints)
	}
	checkpoints[0].TargetRefs[0] = "mutated"
	if checkpoint.Kind != originalKind || !reflect.DeepEqual(checkpoint.TargetRefs, originalTargets) {
		t.Fatalf("projection mutated runtime checkpoint: checkpoint=%#v originalTargets=%#v", checkpoint, originalTargets)
	}
}
