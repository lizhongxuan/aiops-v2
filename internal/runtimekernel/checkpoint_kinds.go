package runtimekernel

import "strings"

const (
	CheckpointKindTurnStart        = "checkpoint_turn_start"
	CheckpointKindToolSurfaceReady = "checkpoint_tool_surface_ready"
	CheckpointKindBeforeToolCall   = "checkpoint_before_tool_call"
	CheckpointKindAfterToolCall    = "checkpoint_after_tool_call"
	CheckpointKindApprovalWaiting  = "checkpoint_approval_waiting"
	CheckpointKindApprovalResolved = "checkpoint_approval_resolved"
	CheckpointKindFinalResponse    = "checkpoint_final_response"
	CheckpointKindErrorRecovery    = "checkpoint_error_recovery"
)

// NormalizeCheckpointKind maps legacy runtime checkpoint markers onto the
// semantic read-model kinds used by AgentRun and prompt trace projections.
func NormalizeCheckpointKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case CheckpointKindTurnStart,
		CheckpointKindToolSurfaceReady,
		CheckpointKindBeforeToolCall,
		CheckpointKindAfterToolCall,
		CheckpointKindApprovalWaiting,
		CheckpointKindApprovalResolved,
		CheckpointKindFinalResponse,
		CheckpointKindErrorRecovery:
		return strings.ToLower(strings.TrimSpace(kind))
	case "assistant_response":
		return CheckpointKindFinalResponse
	case "tool_result", "resume_tool_result", "tool_progress":
		return CheckpointKindAfterToolCall
	case "approval_needed", "pending_approval":
		return CheckpointKindApprovalWaiting
	case "approval_resolved", "approval_granted":
		return CheckpointKindApprovalResolved
	case "resume_checkpoint":
		return CheckpointKindTurnStart
	case "tool_failed", "tool_denied", "turn_failed", "model_call_failed", "iteration_limit":
		return CheckpointKindErrorRecovery
	default:
		if strings.TrimSpace(kind) == "" {
			return ""
		}
		return strings.TrimSpace(kind)
	}
}

func CheckpointIsResumable(lifecycle TurnLifecycleState, resumeState TurnResumeState) bool {
	switch lifecycle {
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		return true
	}
	switch resumeState {
	case TurnResumeStatePendingApproval, TurnResumeStatePendingEvidence, TurnResumeStateCheckpointReady, TurnResumeStateResumable:
		return true
	default:
		return false
	}
}
