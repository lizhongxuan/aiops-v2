package appui

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type choiceRuntimeStub struct {
	resumeReq runtimekernel.ResumeRequest
}

func (s *choiceRuntimeStub) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (s *choiceRuntimeStub) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	s.resumeReq = req
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeExecute,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (s *choiceRuntimeStub) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestChoiceService_AnswerResumesMatchingCheckpoint(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-choice", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-choice",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleResumable,
		ResumeState: runtimekernel.TurnResumeStateResumable,
		Iteration:   2,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:          "choice-1",
			SessionID:   session.ID,
			TurnID:      "turn-choice",
			Iteration:   2,
			Sequence:    3,
			Kind:        "choice_needed",
			Lifecycle:   runtimekernel.TurnLifecycleResumable,
			ResumeState: runtimekernel.TurnResumeStateResumable,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	sessions.Update(session)

	runtime := &choiceRuntimeStub{}
	service := NewChoiceService(runtime, sessions)
	result, err := service.Answer(context.Background(), ChoiceAnswer{
		RequestID: "choice-1",
		Answers: []any{
			map[string]any{"value": "continue", "label": "Continue"},
			map[string]any{"value": "with-verification", "label": "With verification", "note": "verify first"},
		},
	})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	if runtime.resumeReq.SessionID != session.ID {
		t.Fatalf("ResumeTurn sessionId = %q, want %q", runtime.resumeReq.SessionID, session.ID)
	}
	if runtime.resumeReq.TurnID != "turn-choice" {
		t.Fatalf("ResumeTurn turnId = %q, want turn-choice", runtime.resumeReq.TurnID)
	}
	if runtime.resumeReq.CheckpointID != "choice-1" {
		t.Fatalf("ResumeTurn checkpointId = %q, want choice-1", runtime.resumeReq.CheckpointID)
	}
	if runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStateResumable {
		t.Fatalf("ResumeTurn resumeState = %q, want resumable", runtime.resumeReq.ResumeState)
	}
	if got := runtime.resumeReq.Metadata["choice.requestId"]; got != "choice-1" {
		t.Fatalf("ResumeTurn metadata[choice.requestId] = %q, want choice-1", got)
	}
	if got := runtime.resumeReq.Metadata["choice.answer.0"]; got != "Continue" {
		t.Fatalf("ResumeTurn metadata[choice.answer.0] = %q, want normalized choice text", got)
	}
	if got := runtime.resumeReq.Metadata["resume.input"]; got == "" {
		t.Fatal("ResumeTurn metadata[resume.input] is empty")
	}
	if result.Status != "completed" {
		t.Fatalf("ActionResult status = %q, want completed", result.Status)
	}
}
