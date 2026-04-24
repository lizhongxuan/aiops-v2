package appui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type defaultChatService struct {
	runtime  RuntimeGateway
	sessions SessionSource
}

func NewChatService(runtime RuntimeGateway, sessions SessionSource) ChatService {
	return &defaultChatService{
		runtime:  runtime,
		sessions: sessions,
	}
}

func (s *defaultChatService) SendMessage(ctx context.Context, cmd ChatCommand) (TurnResponse, error) {
	content := strings.TrimSpace(cmd.Content)
	if req, ok := s.buildPendingEvidenceResumeRequest(cmd, content); ok {
		result, err := s.runtime.ResumeTurn(ctx, req)
		if err != nil {
			return TurnResponse{}, err
		}
		return mapTurnResponse(result), nil
	}
	sessionID := strings.TrimSpace(cmd.SessionID)
	if sessionID == "" {
		if session := s.resolveCommandSession(""); session != nil {
			sessionID = session.ID
			if strings.TrimSpace(cmd.SessionType) == "" {
				cmd.SessionType = string(session.Type)
			}
			if strings.TrimSpace(cmd.Mode) == "" {
				cmd.Mode = string(session.Mode)
			}
			if strings.TrimSpace(cmd.HostID) == "" {
				cmd.HostID = strings.TrimSpace(session.HostID)
			}
		}
	}
	req := runtimekernel.TurnRequest{
		SessionType:     mapSessionType(cmd.SessionType),
		Mode:            mapMode(cmd.Mode),
		SessionID:       sessionID,
		TurnID:          fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		ClientTurnID:    strings.TrimSpace(cmd.ClientTurnID),
		ClientMessageID: strings.TrimSpace(cmd.ClientMessageID),
		Input:           content,
		HostID:          cmd.HostID,
		Metadata:        cmd.Metadata,
	}
	result, err := s.runtime.RunTurn(ctx, req)
	if err != nil {
		return TurnResponse{}, err
	}
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) buildPendingEvidenceResumeRequest(cmd ChatCommand, content string) (runtimekernel.ResumeRequest, bool) {
	if s == nil || s.sessions == nil || content == "" {
		return runtimekernel.ResumeRequest{}, false
	}
	session := s.resolveCommandSession(cmd.SessionID)
	if session == nil || session.CurrentTurn == nil {
		return runtimekernel.ResumeRequest{}, false
	}
	turn := session.CurrentTurn
	if !turn.Lifecycle.CanResume() {
		return runtimekernel.ResumeRequest{}, false
	}
	evidence, ok := firstPendingEvidence(turn, session)
	if !ok && turn.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		return runtimekernel.ResumeRequest{}, false
	}
	metadata := cloneStringMetadata(cmd.Metadata)
	metadata["resume.input"] = content
	if evidence.ID != "" {
		metadata["evidence.id"] = evidence.ID
	}
	if evidence.ToolCallID != "" {
		metadata["evidence.toolCallId"] = evidence.ToolCallID
	}
	if evidence.ToolName != "" {
		metadata["evidence.toolName"] = evidence.ToolName
	}
	return runtimekernel.ResumeRequest{
		SessionID:    session.ID,
		TurnID:       turn.ID,
		CheckpointID: evidence.ID,
		ResumeState:  runtimekernel.TurnResumeStatePendingEvidence,
		Metadata:     metadata,
	}, true
}

func (s *defaultChatService) resolveCommandSession(sessionID string) *runtimekernel.SessionState {
	targetID := strings.TrimSpace(sessionID)
	if targetID != "" {
		return s.sessions.Get(targetID)
	}
	return s.sessions.GetLatest()
}

func firstPendingEvidence(turn *runtimekernel.TurnSnapshot, session *runtimekernel.SessionState) (runtimekernel.PendingEvidence, bool) {
	if turn != nil && len(turn.PendingEvidence) > 0 {
		return turn.PendingEvidence[0], true
	}
	if session != nil && len(session.PendingEvidence) > 0 {
		return session.PendingEvidence[0], true
	}
	return runtimekernel.PendingEvidence{}, false
}

func cloneStringMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(metadata)+4)
	for key, value := range metadata {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out[trimmed] = value
		}
	}
	return out
}

func (s *defaultChatService) ResumeTurn(ctx context.Context, cmd ResumeCommand) (TurnResponse, error) {
	result, err := s.runtime.ResumeTurn(ctx, runtimekernel.ResumeRequest{
		SessionID:    cmd.SessionID,
		TurnID:       cmd.TurnID,
		ApprovalID:   cmd.ApprovalID,
		CheckpointID: cmd.CheckpointID,
		ResumeState:  runtimekernel.TurnResumeState(cmd.ResumeState),
		Decision:     cmd.Decision,
		Metadata:     cmd.Metadata,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) CancelTurn(ctx context.Context, cmd CancelCommand) (TurnResponse, error) {
	result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
		SessionID: cmd.SessionID,
		TurnID:    cmd.TurnID,
		Reason:    cmd.Reason,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) StopTurn(ctx context.Context, cmd StopCommand) (TurnResponse, error) {
	session, turn, err := resolveTurnTarget(s.sessions, cmd.SessionID, cmd.TurnID)
	if err != nil {
		return TurnResponse{}, err
	}
	result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Reason:    cmd.Reason,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	return mapTurnResponse(result), nil
}

func mapSessionType(value string) runtimekernel.SessionType {
	if value == string(runtimekernel.SessionTypeWorkspace) {
		return runtimekernel.SessionTypeWorkspace
	}
	return runtimekernel.SessionTypeHost
}

func mapMode(value string) runtimekernel.Mode {
	switch value {
	case string(runtimekernel.ModeInspect):
		return runtimekernel.ModeInspect
	case string(runtimekernel.ModePlan):
		return runtimekernel.ModePlan
	case string(runtimekernel.ModeExecute):
		return runtimekernel.ModeExecute
	default:
		return runtimekernel.ModeChat
	}
}

func mapTurnResponse(result runtimekernel.TurnResult) TurnResponse {
	return TurnResponse{
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
	}
}
