package runtimekernel

import (
	"fmt"
	"time"

	runtimestate "aiops-v2/internal/runtimekernel/state"
)

const admissionTargetRequiredFinalText = "缺少明确且已验证的执行目标，因此未执行任何操作。请先选择或绑定目标后重试。"

func (k *RuntimeKernel) completeAdmissionTargetRequiredTurn(session *SessionState, snapshot *TurnSnapshot) (string, error) {
	if session == nil || snapshot == nil {
		return "", fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
		return "", err
	}
	now := time.Now()
	finalText := admissionTargetRequiredFinalText
	facts := FinalRuntimeFacts{
		CompletionStatus: FinalCompletionStatusBlocked,
		PostcheckStatus:  FinalPostcheckStatusNotRequired,
		RollbackStatus:   FinalRollbackStatusNotRequired,
		FailureCodes: []string{
			"exec_command_not_allowed",
			"mutation_intent_requires_explicit_target_binding",
			"no_explicit_target_binding",
		},
		EvidenceState: FinalEvidenceState{
			Confidence:                  FinalEvidenceConfidenceLow,
			ExecCommandAllowed:          false,
			TargetBound:                 false,
			MutationIntentWithoutTarget: true,
		},
	}
	facts.EvidenceDecision = VerifyFinalEvidenceFacts(facts.EvidenceState)
	finalContract := BuildFinalContract(finalText, facts)
	message := Message{
		ID:        fmt.Sprintf("msg-%d", now.UnixNano()),
		Role:      "assistant",
		Content:   finalText,
		Timestamp: now,
	}
	session.Messages = append(session.Messages, message)
	snapshot.Lifecycle = TurnLifecycleCompleted
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = ""
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), "admission_target_required", TurnLifecycleCompleted, TurnResumeStateNone)
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	commitFinalAssistantOutput(snapshot, assistantOutputCommitInput{
		TurnID:           snapshot.ID,
		Iteration:        snapshot.Iteration,
		MessageID:        message.ID,
		AssistantText:    finalText,
		EvidenceBoundary: "blocked",
		BoundaryAction:   FinalMessageBoundaryBlock,
		FinalContract:    &finalContract,
	})
	snapshot.FinalOutput = FinalTextFromAssistantMessage(snapshot)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteAssistantMessage, OwnerRuntimeKernel)
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	syncActiveTurnState(session, snapshot)
	k.persistTurnSnapshot(session, snapshot)
	return finalText, nil
}
