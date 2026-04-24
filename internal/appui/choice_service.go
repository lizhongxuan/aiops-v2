package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

type defaultChoiceService struct {
	runtime  RuntimeGateway
	sessions SessionSource
}

func NewChoiceService(runtime RuntimeGateway, sessions SessionSource) ChoiceService {
	return &defaultChoiceService{
		runtime:  runtime,
		sessions: sessions,
	}
}

func (s *defaultChoiceService) Answer(ctx context.Context, answer ChoiceAnswer) (ActionResult, error) {
	session, turn, checkpointID, err := resolveChoiceTarget(s.sessions, answer.RequestID)
	if err != nil {
		return ActionResult{}, err
	}

	resumeState := runtimekernel.TurnResumeStateResumable
	if turn != nil && turn.ResumeState.IsValid() && turn.ResumeState != runtimekernel.TurnResumeStateNone {
		resumeState = turn.ResumeState
	}
	result, err := s.runtime.ResumeTurn(ctx, runtimekernel.ResumeRequest{
		SessionID:    session.ID,
		TurnID:       currentTurnID(session),
		CheckpointID: checkpointID,
		ResumeState:  resumeState,
		Metadata:     buildChoiceMetadata(answer.RequestID, answer.Answers),
	})
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{
		Status:    result.Status,
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
	}, nil
}

func buildChoiceMetadata(requestID string, answers []any) map[string]string {
	metadata := map[string]string{}
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		metadata["choice.requestId"] = trimmed
	}
	metadata["resume.input"] = buildChoiceInput(requestID, answers)
	for idx, answer := range answers {
		trimmed := strings.TrimSpace(choiceAnswerText(answer))
		if trimmed == "" {
			continue
		}
		metadata[fmt.Sprintf("choice.answer.%d", idx)] = trimmed
	}
	return metadata
}

func buildChoiceInput(requestID string, answers []any) string {
	lines := make([]string, 0, len(answers)+1)
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		lines = append(lines, fmt.Sprintf("choice request %s", trimmed))
	}
	for idx, answer := range answers {
		text := strings.TrimSpace(choiceAnswerText(answer))
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("answer %d: %s", idx+1, text))
	}
	if len(lines) == 0 {
		return "choice response received"
	}
	return strings.Join(lines, "\n")
}

func choiceAnswerText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case map[string]any:
		return choiceMapText(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func choiceMapText(value map[string]any) string {
	parts := make([]string, 0, 3)
	if entries, ok := value["values"].([]any); ok && len(entries) > 0 {
		labels := make([]string, 0, len(entries))
		for _, entry := range entries {
			if item, ok := entry.(map[string]any); ok {
				label := strings.TrimSpace(fmt.Sprint(firstNonEmpty(
					asString(item["label"]),
					asString(item["value"]),
				)))
				if label != "" {
					labels = append(labels, label)
				}
			}
		}
		if len(labels) > 0 {
			parts = append(parts, strings.Join(labels, ", "))
		}
	}
	single := strings.TrimSpace(fmt.Sprint(firstNonEmpty(
		asString(value["label"]),
		asString(value["value"]),
	)))
	if single != "" {
		parts = append(parts, single)
	}
	if note := strings.TrimSpace(asString(value["note"])); note != "" {
		parts = append(parts, "note="+note)
	}
	if len(parts) == 0 {
		data, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(data)
	}
	return strings.Join(parts, "; ")
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprint(typed)
	}
}
