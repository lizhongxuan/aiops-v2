package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type defaultChatService struct {
	runtime     RuntimeGateway
	sessions    SessionSource
	agentEvents AgentEventService
	turnRunner  AsyncTurnRunner
	baseContext context.Context
}

type AsyncTurnRunner interface {
	Start(ctx context.Context, req runtimekernel.TurnRequest)
}

type defaultAsyncTurnRunner struct {
	runtime     RuntimeGateway
	agentEvents AgentEventService
	baseContext context.Context
}

func NewChatService(runtime RuntimeGateway, sessions SessionSource, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContext(context.Background(), runtime, sessions, agentEvents...)
}

func NewChatServiceWithContext(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, agentEvents ...AgentEventService) ChatService {
	var eventService AgentEventService
	if len(agentEvents) > 0 {
		eventService = agentEvents[0]
	}
	if eventService == nil {
		eventService = NewAgentEventService(nil)
	}
	baseContext = normalizeBaseContext(baseContext)
	return &defaultChatService{
		runtime:     runtime,
		sessions:    sessions,
		agentEvents: eventService,
		baseContext: baseContext,
		turnRunner: defaultAsyncTurnRunner{
			runtime:     runtime,
			agentEvents: eventService,
			baseContext: baseContext,
		},
	}
}

func normalizeBaseContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
	if session := s.resolveCommandSession(sessionID); session != nil {
		if sessionID == "" {
			sessionID = session.ID
		}
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
	if req.SessionID == "" {
		req.SessionID = strings.TrimSpace(cmd.SessionID)
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	s.appendTurnAcceptedEvents(req)
	s.turnRunner.Start(ctx, req)
	return TurnResponse{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "accepted",
	}, nil
}

func (s *defaultChatService) appendTurnAcceptedEvents(req runtimekernel.TurnRequest) {
	if s == nil || s.agentEvents == nil {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turnPayload, _ := json.Marshal(TurnPayload{
		Prompt:          req.Input,
		Title:           req.Input,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Mode:            string(req.Mode),
		ReasoningEffort: req.Metadata["reasoningEffort"],
	})
	_, _ = s.agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.requested", req.TurnID),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		Kind:         AgentEventTurn,
		Phase:        AgentEventPhaseRequested,
		Status:       AgentEventStatusQueued,
		Visibility:   AgentEventVisibilityPrimary,
		Source:       AgentEventSourceUI,
		CreatedAt:    now,
		Payload:      turnPayload,
	})
	appendMainAgentEvent(ctx, s.agentEvents, req, AgentEventPhaseStarted, AgentEventStatusRunning, "正在启动 Agent", "")
}

func (r defaultAsyncTurnRunner) Start(_ context.Context, req runtimekernel.TurnRequest) {
	go r.run(req)
}

func (r defaultAsyncTurnRunner) run(req runtimekernel.TurnRequest) {
	if r.runtime == nil {
		return
	}
	ctx := normalizeBaseContext(r.baseContext)
	defer func() {
		if recovered := recover(); recovered != nil {
			appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", fmt.Sprintf("panic: %v", recovered))
			appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, fmt.Sprintf("panic: %v", recovered))
		}
	}()
	result, err := r.runtime.RunTurn(ctx, req)
	if err != nil {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", err.Error())
		appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, err.Error())
		return
	}
	if strings.EqualFold(result.Status, "cancelled") || strings.EqualFold(result.Status, string(AgentEventStatusCanceled)) {
		return
	}
	if strings.EqualFold(result.Status, "blocked") || strings.EqualFold(result.Status, string(AgentEventStatusBlocked)) {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseBlocked, AgentEventStatusBlocked, "", strings.TrimSpace(result.Error))
		return
	}
	if strings.EqualFold(result.Status, "failed") || strings.TrimSpace(result.Error) != "" {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", strings.TrimSpace(result.Error))
		appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, strings.TrimSpace(result.Error))
		return
	}
	appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseCompleted, AgentEventStatusCompleted, "", "任务已完成")
}

func appendTerminalAgentEvent(ctx context.Context, agentEvents AgentEventService, req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, message string) {
	if agentEvents == nil {
		return
	}
	ctx = normalizeBaseContext(ctx)
	payload, _ := json.Marshal(TurnPayload{Error: message, Summary: message})
	_, _ = agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.%s.async", req.TurnID, phase),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		Kind:         AgentEventTurn,
		Phase:        phase,
		Status:       status,
		Visibility:   AgentEventVisibilityPrimary,
		Source:       AgentEventSourceSystem,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:      payload,
	})
}

func appendMainAgentEvent(ctx context.Context, agentEvents AgentEventService, req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, lastAction string, lastSummary string) {
	if agentEvents == nil {
		return
	}
	payload, _ := json.Marshal(AgentPayload{
		Handle:      "main",
		Name:        "Main Agent",
		Role:        "primary",
		LastAction:  strings.TrimSpace(lastAction),
		LastSummary: strings.TrimSpace(lastSummary),
	})
	_, _ = agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:agent.main.%s", req.TurnID, phase),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		AgentID:      "agent-main",
		Kind:         AgentEventAgent,
		Phase:        phase,
		Status:       status,
		Visibility:   AgentEventVisibilitySecondary,
		Source:       AgentEventSourceSystem,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:      payload,
	})
}

func (s *defaultChatService) appendTerminalAgentEvent(req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, message string) {
	if s == nil {
		return
	}
	appendTerminalAgentEvent(s.baseContext, s.agentEvents, req, phase, status, message)
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
	if s == nil || s.sessions == nil {
		return nil
	}
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
	s.appendCanceledEvent(result, cmd.SessionID, cmd.TurnID, "任务已取消")
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) StopTurn(ctx context.Context, cmd StopCommand) (TurnResponse, error) {
	if sessionID := strings.TrimSpace(cmd.SessionID); sessionID != "" {
		if turnID := strings.TrimSpace(cmd.TurnID); turnID != "" {
			result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
				SessionID: sessionID,
				TurnID:    turnID,
				Reason:    cmd.Reason,
			})
			if err != nil {
				return TurnResponse{}, err
			}
			s.appendCanceledEvent(result, sessionID, turnID, "任务已停止")
			return mapTurnResponse(result), nil
		}
	}
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
	s.appendCanceledEvent(result, session.ID, turn.ID, "任务已停止")
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) appendCanceledEvent(result runtimekernel.TurnResult, fallbackSessionID string, fallbackTurnID string, message string) {
	if s == nil {
		return
	}
	if !strings.EqualFold(result.Status, "cancelled") && !strings.EqualFold(result.Status, string(AgentEventStatusCanceled)) {
		return
	}
	sessionID := strings.TrimSpace(result.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(fallbackSessionID)
	}
	turnID := strings.TrimSpace(result.TurnID)
	if turnID == "" {
		turnID = strings.TrimSpace(fallbackTurnID)
	}
	if sessionID == "" || turnID == "" {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	appendMainAgentEvent(ctx, s.agentEvents, runtimekernel.TurnRequest{
		SessionID:       sessionID,
		TurnID:          turnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
	}, AgentEventPhaseCanceled, AgentEventStatusCanceled, "", message)
	s.appendTerminalAgentEvent(runtimekernel.TurnRequest{
		SessionID:       sessionID,
		TurnID:          turnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
	}, AgentEventPhaseCanceled, AgentEventStatusCanceled, message)
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
