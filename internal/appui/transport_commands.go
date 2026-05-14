package appui

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type TransportCommandType string

const (
	TransportCommandTypeAddMessage       TransportCommandType = "add-message"
	TransportCommandTypeStop             TransportCommandType = "aiops.stop"
	TransportCommandTypeRetry            TransportCommandType = "aiops.retry"
	TransportCommandTypeApprovalDecision TransportCommandType = "aiops.approval-decision"
	TransportCommandTypeChoiceAnswer     TransportCommandType = "aiops.choice-answer"
	TransportCommandTypeMCPAction        TransportCommandType = "aiops.mcp-action"
	TransportCommandTypeMCPRefresh       TransportCommandType = "aiops.mcp-refresh"
	TransportCommandTypeMCPPin           TransportCommandType = "aiops.mcp-pin"
	TransportCommandTypeInsertArtifact   TransportCommandType = "aiops.insert-agent-ui-artifact"
)

type TransportCommand struct {
	Type             TransportCommandType
	AddMessage       *TransportAddMessageCommand
	Stop             *TransportStopCommand
	Retry            *TransportRetryCommand
	ApprovalDecision *TransportApprovalDecisionCommand
	ChoiceAnswer     *TransportChoiceAnswerCommand
	MCPAction        *TransportMCPActionCommand
	MCPRefresh       *TransportMCPRefreshCommand
	MCPPin           *TransportMCPPinCommand
	InsertArtifact   *TransportInsertArtifactCommand
}

type TransportUserMessage struct {
	Text string
}

type TransportAddMessageCommand struct {
	SessionID       string
	ThreadID        string
	ParentID        string
	SourceID        string
	HostID          string
	ClientMessageID string
	ClientTurnID    string
	Message         TransportUserMessage
	Metadata        map[string]string
}

type TransportStopCommand struct {
	SessionID string
	TurnID    string
	Reason    string
}

type TransportRetryCommand struct {
	SessionID string
	TurnID    string
}

type TransportApprovalDecisionCommand struct {
	ApprovalID string
	Decision   string
}

type TransportChoiceAnswerCommand struct {
	RequestID string
	Answer    string
}

type TransportMCPActionCommand struct {
	SurfaceID string
	ActionID  string
	Input     map[string]any
}

type TransportMCPRefreshCommand struct {
	SurfaceID string
}

type TransportMCPPinCommand struct {
	SurfaceID string
	Pinned    bool
}

type TransportInsertArtifactCommand struct {
	TurnID   string
	Artifact AiopsTransportAgentArtifact
}

type TransportCommandHandler struct {
	chat      ChatService
	approvals ApprovalService
	choices   ChoiceService
	mcps      MCPService
}

type asyncApprovalDecisionService interface {
	DecideAsync(ctx context.Context, decision ApprovalDecision) (ActionResult, error)
}

type TransportCommandResult struct {
	SessionID string
	TurnID    string
	Status    string
}

func NewTransportCommandHandler(chat ChatService, approvals ApprovalService, choices ChoiceService, mcps MCPService) *TransportCommandHandler {
	return &TransportCommandHandler{
		chat:      chat,
		approvals: approvals,
		choices:   choices,
		mcps:      mcps,
	}
}

func (h *TransportCommandHandler) Apply(ctx context.Context, state AiopsTransportState, command TransportCommand) (AiopsTransportState, TransportCommandResult, error) {
	next := ensureAiopsTransportState(state)
	switch command.Type {
	case TransportCommandTypeAddMessage:
		return h.applyAddMessage(ctx, next, command.AddMessage)
	case TransportCommandTypeRetry:
		return h.applyRetry(ctx, next, command.Retry)
	case TransportCommandTypeStop:
		return h.applyStop(ctx, next, command.Stop)
	case TransportCommandTypeApprovalDecision:
		return h.applyApprovalDecision(ctx, next, command.ApprovalDecision)
	case TransportCommandTypeChoiceAnswer:
		return h.applyChoiceAnswer(ctx, next, command.ChoiceAnswer)
	case TransportCommandTypeMCPAction:
		return h.applyMCPAction(ctx, next, command.MCPAction)
	case TransportCommandTypeMCPRefresh:
		return h.applyMCPRefresh(ctx, next, command.MCPRefresh)
	case TransportCommandTypeMCPPin:
		return h.applyMCPPin(next, command.MCPPin), TransportCommandResult{}, nil
	case TransportCommandTypeInsertArtifact:
		return h.applyInsertArtifact(next, command.InsertArtifact), TransportCommandResult{}, nil
	default:
		return next, TransportCommandResult{}, nil
	}
}

func (h *TransportCommandHandler) applyAddMessage(ctx context.Context, state AiopsTransportState, command *TransportAddMessageCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	messageText := strings.TrimSpace(command.Message.Text)
	resp, err := h.chat.SendMessage(ctx, ChatCommand{
		SessionID:       strings.TrimSpace(firstNonEmptyString(command.SessionID, state.SessionID)),
		Content:         messageText,
		HostID:          strings.TrimSpace(command.HostID),
		ClientMessageID: strings.TrimSpace(command.ClientMessageID),
		ClientTurnID:    strings.TrimSpace(command.ClientTurnID),
		Metadata:        cloneStringMetadata(command.Metadata),
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.SessionID = firstNonEmptyString(resp.SessionID, state.SessionID)
	state.ThreadID = firstNonEmptyString(strings.TrimSpace(command.ThreadID), state.ThreadID, state.SessionID)
	state.Status = AiopsTransportStatusWorking
	state.CurrentTurnID = firstNonEmptyString(resp.TurnID, state.CurrentTurnID)
	if state.CurrentTurnID != "" && messageText != "" {
		turnID := state.CurrentTurnID
		turn := state.Turns[turnID]
		if turn.ID == "" {
			turn.ID = turnID
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		turn.Status = AiopsTransportTurnStatusSubmitted
		turn.StartedAt = firstNonEmptyString(turn.StartedAt, now)
		turn.User = &AiopsTransportMessage{
			ID:        firstNonEmptyString(resp.ClientMessageID, command.SourceID, turnID+":user"),
			Text:      messageText,
			CreatedAt: now,
		}
		state.TurnOrder = appendTurnOrder(state.TurnOrder, turnID)
		state.Turns[turnID] = turn
		state.RuntimeLiveness.ActiveTurns[turnID] = true
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func (h *TransportCommandHandler) applyRetry(ctx context.Context, state AiopsTransportState, command *TransportRetryCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	turnID := strings.TrimSpace(firstNonEmptyString(command.TurnID, state.CurrentTurnID))
	turn, ok := state.Turns[turnID]
	if !ok || turn.User == nil || strings.TrimSpace(turn.User.Text) == "" {
		return state, TransportCommandResult{}, fmt.Errorf("retry turn %q has no user message", turnID)
	}
	resp, err := h.chat.SendMessage(ctx, ChatCommand{
		SessionID: strings.TrimSpace(firstNonEmptyString(command.SessionID, state.SessionID)),
		Content:   strings.TrimSpace(turn.User.Text),
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.SessionID = firstNonEmptyString(resp.SessionID, state.SessionID)
	state.Status = AiopsTransportStatusWorking
	state.CurrentTurnID = firstNonEmptyString(resp.TurnID, state.CurrentTurnID)
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func (h *TransportCommandHandler) applyStop(ctx context.Context, state AiopsTransportState, command *TransportStopCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	resp, err := h.chat.StopTurn(ctx, StopCommand{
		SessionID: strings.TrimSpace(firstNonEmptyString(command.SessionID, state.SessionID)),
		TurnID:    strings.TrimSpace(firstNonEmptyString(command.TurnID, state.CurrentTurnID)),
		Reason:    strings.TrimSpace(command.Reason),
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.Status = AiopsTransportStatusCanceled
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	turnID := strings.TrimSpace(firstNonEmptyString(resp.TurnID, command.TurnID, state.CurrentTurnID))
	if turn := state.Turns[turnID]; turn.ID != "" {
		turn.Status = AiopsTransportTurnStatusCanceled
		if turn.Final != nil {
			turn.Final.Status = AiopsTransportFinalStatusFailed
		}
		state.Turns[turnID] = turn
	}
	if turnID != "" {
		delete(state.RuntimeLiveness.ActiveTurns, turnID)
	}
	state.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
	state.RuntimeLiveness.PendingApprovals = map[string]bool{}
	state.PendingApprovals = map[string]AiopsTransportApproval{}
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func (h *TransportCommandHandler) applyApprovalDecision(ctx context.Context, state AiopsTransportState, command *TransportApprovalDecisionCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.approvals == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	approvalID := strings.TrimSpace(command.ApprovalID)
	approval := state.PendingApprovals[approvalID]
	decision := ApprovalDecision{
		ID:       approvalID,
		Decision: strings.TrimSpace(command.Decision),
	}
	result, err := h.applyApprovalDecisionCommand(ctx, decision)
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	delete(state.PendingApprovals, approvalID)
	delete(state.RuntimeLiveness.PendingApprovals, approvalID)
	turnID := strings.TrimSpace(firstNonEmptyString(result.TurnID, approval.TurnID, state.CurrentTurnID))
	if isTransportRejectedDecision(command.Decision) || strings.EqualFold(result.Status, "failed") || strings.EqualFold(result.Status, "denied") {
		state.Status = AiopsTransportStatusFailed
		if turnID != "" {
			delete(state.RuntimeLiveness.ActiveTurns, turnID)
			turn := state.Turns[turnID]
			state.Turns[turnID] = markApprovalDecisionOnTurn(turn, approvalID, true)
		}
	} else {
		state.Status = AiopsTransportStatusWorking
		if turnID != "" {
			state.CurrentTurnID = turnID
			state.RuntimeLiveness.ActiveTurns[turnID] = true
			turn := state.Turns[turnID]
			state.Turns[turnID] = markApprovalDecisionOnTurn(turn, approvalID, false)
		}
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: result.SessionID, TurnID: result.TurnID, Status: result.Status}, nil
}

func (h *TransportCommandHandler) applyApprovalDecisionCommand(ctx context.Context, decision ApprovalDecision) (ActionResult, error) {
	if h == nil || h.approvals == nil {
		return ActionResult{}, nil
	}
	if async, ok := h.approvals.(asyncApprovalDecisionService); ok {
		return async.DecideAsync(ctx, decision)
	}
	return h.approvals.Decide(ctx, decision)
}

func markApprovalDecisionOnTurn(turn AiopsTransportTurn, approvalID string, rejected bool) AiopsTransportTurn {
	if turn.ID == "" {
		return turn
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if rejected {
		turn.Status = AiopsTransportTurnStatusFailed
		if turn.Final != nil {
			turn.Final.Status = AiopsTransportFinalStatusFailed
		}
	} else {
		turn.Status = AiopsTransportTurnStatusWorking
	}
	turn.UpdatedAt = now
	for idx := range turn.Process {
		if strings.TrimSpace(turn.Process[idx].ApprovalID) != strings.TrimSpace(approvalID) {
			continue
		}
		if rejected {
			turn.Process[idx].Status = AiopsTransportProcessStatusRejected
		} else {
			turn.Process[idx].Status = AiopsTransportProcessStatusRunning
		}
		turn.Process[idx].UpdatedAt = now
	}
	return turn
}

func (h *TransportCommandHandler) applyChoiceAnswer(ctx context.Context, state AiopsTransportState, command *TransportChoiceAnswerCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.choices == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	result, err := h.choices.Answer(ctx, ChoiceAnswer{
		RequestID: strings.TrimSpace(command.RequestID),
		Answers:   []any{strings.TrimSpace(command.Answer)},
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.Status = AiopsTransportStatusWorking
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: result.SessionID, TurnID: result.TurnID, Status: result.Status}, nil
}

func (h *TransportCommandHandler) applyMCPAction(ctx context.Context, state AiopsTransportState, command *TransportMCPActionCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.mcps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	surfaceID := strings.TrimSpace(command.SurfaceID)
	actionID := strings.TrimSpace(command.ActionID)
	if err := validateTransportMCPSurface(state, surfaceID, actionID); err != nil {
		return state, TransportCommandResult{}, err
	}
	_, err := h.mcps.Act(ctx, surfaceID, actionID)
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = touchMcpSurface(state, surfaceID)
	return state, TransportCommandResult{Status: "completed"}, nil
}

func (h *TransportCommandHandler) applyMCPRefresh(ctx context.Context, state AiopsTransportState, command *TransportMCPRefreshCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.mcps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	surfaceID := strings.TrimSpace(command.SurfaceID)
	if err := validateTransportMCPSurface(state, surfaceID, "refresh"); err != nil {
		return state, TransportCommandResult{}, err
	}
	_, err := h.mcps.Act(ctx, surfaceID, "refresh")
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = touchMcpSurface(state, surfaceID)
	return state, TransportCommandResult{Status: "completed"}, nil
}

func (h *TransportCommandHandler) applyMCPPin(state AiopsTransportState, command *TransportMCPPinCommand) AiopsTransportState {
	if command == nil {
		return state
	}
	surfaceID := strings.TrimSpace(command.SurfaceID)
	if surfaceID == "" {
		return state
	}
	state = touchMcpSurface(state, surfaceID)
	surface := state.McpSurfaces[surfaceID]
	surface.Pinned = command.Pinned
	state.McpSurfaces[surfaceID] = surface
	return state
}

func (h *TransportCommandHandler) applyInsertArtifact(state AiopsTransportState, command *TransportInsertArtifactCommand) AiopsTransportState {
	if command == nil {
		return state
	}
	artifact := command.Artifact
	artifact.ID = strings.TrimSpace(artifact.ID)
	artifact.Type = strings.TrimSpace(artifact.Type)
	if artifact.ID == "" {
		artifact.ID = fmt.Sprintf("agent-ui-artifact-%d", time.Now().UnixNano())
	}
	if artifact.Type == "" {
		artifact.Type = "experience_pack_candidate"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(artifact.CreatedAt) == "" {
		artifact.CreatedAt = now
	}
	artifact.UpdatedAt = now

	turnID := strings.TrimSpace(firstNonEmptyString(command.TurnID, state.CurrentTurnID))
	if turnID == "" {
		turnID = "local-artifacts"
	}
	turn := state.Turns[turnID]
	if turn.ID == "" {
		turn.ID = turnID
		turn.Status = AiopsTransportTurnStatusCompleted
		turn.StartedAt = now
		turn.CompletedAt = now
	}
	turn.UpdatedAt = now
	turn.AgentUiArtifacts = upsertAgentArtifact(turn.AgentUiArtifacts, artifact)
	state.TurnOrder = appendTurnOrder(state.TurnOrder, turnID)
	state.Turns[turnID] = turn
	state.UpdatedAt = now
	state.Seq++
	return state
}

func upsertAgentArtifact(items []AiopsTransportAgentArtifact, artifact AiopsTransportAgentArtifact) []AiopsTransportAgentArtifact {
	for idx, item := range items {
		if strings.TrimSpace(item.ID) == artifact.ID {
			next := append([]AiopsTransportAgentArtifact(nil), items...)
			next[idx] = artifact
			return next
		}
	}
	next := append([]AiopsTransportAgentArtifact(nil), items...)
	next = append(next, artifact)
	return next
}

func touchMcpSurface(state AiopsTransportState, surfaceID string) AiopsTransportState {
	surfaceID = strings.TrimSpace(surfaceID)
	if surfaceID == "" {
		return state
	}
	surface := state.McpSurfaces[surfaceID]
	if surface.ID == "" {
		surface.ID = surfaceID
		surface.Title = surfaceID
	}
	surface.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.McpSurfaces[surfaceID] = surface
	state.UpdatedAt = surface.UpdatedAt
	return state
}

func validateTransportMCPSurface(state AiopsTransportState, surfaceID string, actionID string) error {
	if strings.TrimSpace(surfaceID) == "" {
		return fmt.Errorf("mcp surface id is required")
	}
	if strings.TrimSpace(actionID) == "" {
		return fmt.Errorf("mcp action id is required")
	}
	surface, ok := state.McpSurfaces[surfaceID]
	if !ok || strings.TrimSpace(surface.ID) == "" {
		return fmt.Errorf("mcp surface %q is not available in transport state", surfaceID)
	}
	return nil
}

func isTransportRejectedDecision(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accept", "accept_session", "approve", "approved", "approved_for_session", "allow", "yes":
		return false
	default:
		return true
	}
}
