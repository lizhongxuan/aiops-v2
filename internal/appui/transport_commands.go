package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

type TransportCommandType string

const (
	TransportCommandTypeAddMessage          TransportCommandType = "add-message"
	TransportCommandTypeStop                TransportCommandType = "aiops.stop"
	TransportCommandTypeRetry               TransportCommandType = "aiops.retry"
	TransportCommandTypeApprovalDecision    TransportCommandType = "aiops.approval-decision"
	TransportCommandTypeChoiceAnswer        TransportCommandType = "aiops.choice-answer"
	TransportCommandTypeMCPAction           TransportCommandType = "aiops.mcp-action"
	TransportCommandTypeMCPRefresh          TransportCommandType = "aiops.mcp-refresh"
	TransportCommandTypeMCPPin              TransportCommandType = "aiops.mcp-pin"
	TransportCommandTypeSpecialInputClear   TransportCommandType = "aiops.special-input-clear"
	TransportCommandTypeSpecialInputConfirm TransportCommandType = "aiops.special-input-confirm"
	TransportCommandTypeHostPlanAccept      TransportCommandType = "aiops.host-plan-accept"
	TransportCommandTypeHostPlanRevise      TransportCommandType = "aiops.host-plan-revise"
	TransportCommandTypeChildAgentMessage   TransportCommandType = "aiops.child-agent-message"
	TransportCommandTypeChildAgentStop      TransportCommandType = "aiops.child-agent-stop"
)

type TransportCommand struct {
	Type                TransportCommandType
	AddMessage          *TransportAddMessageCommand
	Stop                *TransportStopCommand
	Retry               *TransportRetryCommand
	ApprovalDecision    *TransportApprovalDecisionCommand
	ChoiceAnswer        *TransportChoiceAnswerCommand
	MCPAction           *TransportMCPActionCommand
	MCPRefresh          *TransportMCPRefreshCommand
	MCPPin              *TransportMCPPinCommand
	SpecialInputClear   *TransportSpecialInputClearCommand
	SpecialInputConfirm *TransportSpecialInputConfirmCommand
	HostPlanAccept      *TransportHostPlanAcceptCommand
	HostPlanRevise      *TransportHostPlanReviseCommand
	ChildAgentMessage   *TransportChildAgentMessageCommand
	ChildAgentStop      *TransportChildAgentStopCommand
}

type TransportUserMessage struct {
	Text string
}

type TransportAddMessageCommand struct {
	SessionID       string
	SessionType     string
	Mode            string
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
	SessionID  string
	TurnID     string
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

type TransportSpecialInputClearCommand struct {
	SessionID    string
	ResourceKind string
	ResourceID   string
	CanonicalKey string
}

type TransportSpecialInputConfirmCommand struct {
	SessionID    string
	ResourceKind string
	ResourceID   string
	CanonicalKey string
}

type TransportHostPlanAcceptCommand struct {
	MissionID string
	PlanID    string
}

type TransportHostPlanReviseCommand struct {
	MissionID   string
	Instruction string
}

type TransportChildAgentMessageCommand struct {
	ChildAgentID string
	Content      string
}

type TransportChildAgentStopCommand struct {
	ChildAgentID string
}

type TransportCommandHandler struct {
	chat      ChatService
	approvals ApprovalService
	choices   ChoiceService
	mcps      MCPService
	hostOps   HostOpsService
}

type asyncApprovalDecisionService interface {
	DecideAsync(ctx context.Context, decision ApprovalDecision) (ActionResult, error)
}

func (h *TransportCommandHandler) WithHostOpsService(service HostOpsService) *TransportCommandHandler {
	if h != nil {
		h.hostOps = service
	}
	return h
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
	case TransportCommandTypeSpecialInputClear:
		return h.applySpecialInputClear(ctx, next, command.SpecialInputClear)
	case TransportCommandTypeSpecialInputConfirm:
		return h.applySpecialInputConfirm(ctx, next, command.SpecialInputConfirm)
	case TransportCommandTypeHostPlanAccept:
		return h.applyHostPlanAccept(ctx, next, command.HostPlanAccept)
	case TransportCommandTypeHostPlanRevise:
		return h.applyHostPlanRevise(ctx, next, command.HostPlanRevise)
	case TransportCommandTypeChildAgentMessage:
		return h.applyChildAgentMessage(ctx, next, command.ChildAgentMessage)
	case TransportCommandTypeChildAgentStop:
		return h.applyChildAgentStop(ctx, next, command.ChildAgentStop)
	default:
		return next, TransportCommandResult{}, nil
	}
}

func (h *TransportCommandHandler) applySpecialInputConfirm(ctx context.Context, state AiopsTransportState, command *TransportSpecialInputConfirmCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil {
		return state, TransportCommandResult{}, nil
	}
	sessionID := strings.TrimSpace(state.SessionID)
	if command != nil && strings.TrimSpace(command.SessionID) != "" {
		sessionID = strings.TrimSpace(command.SessionID)
	}
	resp, err := h.chat.SendMessage(ctx, ChatCommand{
		SessionID: sessionID,
		Content:   "确认",
		Metadata: map[string]string{
			"aiops.specialInput.command": "confirm",
		},
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.SessionID = firstNonEmptyString(resp.SessionID, state.SessionID)
	state.Status = AiopsTransportStatusIdle
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func (h *TransportCommandHandler) applySpecialInputClear(ctx context.Context, state AiopsTransportState, command *TransportSpecialInputClearCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil {
		return state, TransportCommandResult{}, nil
	}
	sessionID := strings.TrimSpace(state.SessionID)
	if command != nil && strings.TrimSpace(command.SessionID) != "" {
		sessionID = strings.TrimSpace(command.SessionID)
	}
	resp, err := h.chat.SendMessage(ctx, ChatCommand{
		SessionID: sessionID,
		Content:   "忘记当前主机",
		Metadata: map[string]string{
			"aiops.specialInput.command": "clear",
		},
	})
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state.SessionID = firstNonEmptyString(resp.SessionID, state.SessionID)
	state.SpecialInputContext = nil
	state.Status = AiopsTransportStatusIdle
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func (h *TransportCommandHandler) applyAddMessage(ctx context.Context, state AiopsTransportState, command *TransportAddMessageCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.chat == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	messageText := strings.TrimSpace(command.Message.Text)
	route := buildChatRuntimeTransportRoute(messageText, command.Metadata)
	hostID := strings.TrimSpace(command.HostID)
	if hostID == "" {
		hostID = strings.TrimSpace(route.metadata["aiops.target.hostId"])
	}
	resp, err := h.chat.SendMessage(ctx, ChatCommand{
		SessionID:       strings.TrimSpace(firstNonEmptyString(command.SessionID, state.SessionID)),
		SessionType:     strings.TrimSpace(command.SessionType),
		Mode:            strings.TrimSpace(command.Mode),
		Content:         messageText,
		HostID:          hostID,
		ClientMessageID: strings.TrimSpace(command.ClientMessageID),
		ClientTurnID:    strings.TrimSpace(command.ClientTurnID),
		Metadata:        route.metadata,
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
	if route.decision.Kind == hostops.RouteKindHostOps {
		state = addHostOpsMissionFromRoute(state, route, resp.TurnID)
		if h.hostOps != nil {
			missionID := strings.TrimSpace(route.metadata["aiops.hostops.missionId"])
			if missionID == "" {
				missionID = "hostops:" + strings.TrimSpace(resp.TurnID)
			}
			if view, getErr := h.hostOps.GetMission(ctx, missionID); getErr == nil {
				state = mergeHostOpsMissionView(state, view, resp.TurnID)
			}
		}
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state, TransportCommandResult{SessionID: resp.SessionID, TurnID: resp.TurnID, Status: resp.Status}, nil
}

func buildChatRuntimeTransportRoute(messageText string, metadata map[string]string) hostOpsTransportRoute {
	nextMetadata := cloneStringMetadata(metadata)
	structuredMentions := parseInputMentions(messageText, nextMetadata)
	mentions := filterHostOpsRouteMentions(hostOpsMentionsFromMetadata(nextMetadata["aiops.hostops.mentions"]))
	if structuredMentions.Present {
		if structuredMentions.Invalid {
			mentions = nil
		} else {
			mentions = filterHostOpsRouteMentions(inputMentionHostHintsToHostMentions(structuredMentions.Hosts))
		}
	} else if len(mentions) == 0 {
		if inputMentionStrictMode() {
			mentions = nil
		} else {
			mentions = filterHostOpsRouteMentions(hostops.ParseHostMentions(messageText))
		}
	}
	mentionSource, mentionValidation := mentionSourceForCommand(ChatCommand{Content: messageText, Metadata: nextMetadata}, messageText, structuredMentions, mentions)
	evidence := ExtractUserEvidence(messageText)
	chatRoute := BuildChatRuntimeRoute(messageText, mentions, evidence)
	envelope := BuildEvidenceEnvelope(messageText, nil, nil)
	intentFrame := BuildIntentFrame(messageText, envelope, nil)
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(intentFrame, chatRoute)
	activeRoute, routingMode := selectActiveChatRuntimeRoute(chatRoute, intentRoute, intentFrame, intentFrameRoutingTraceOnly)
	applyStructuredMentionRouteHints(&activeRoute, structuredMentions)
	req := &runtimekernel.TurnRequest{Input: messageText, Metadata: nextMetadata}
	applyChatRuntimeRouteMetadata(req, activeRoute)
	applyIntentFrameRouteMetadata(req, chatRoute, intentRoute, activeRoute, intentFrame, routingMode)
	req.Metadata["aiops.route.activeSource"] = routingMode
	applyChatRuntimeToolSurfaceMetadata(req, activeRoute)
	applyStructuredCapabilityMetadata(req.Metadata, structuredMentions)
	applyUserEvidenceMetadata(req, evidence)
	applyInputMentionDiagnosticValues(req, mentionSource, mentionValidation)
	applyChatRuntimeRouteHostBinding(req, activeRoute, mentions)
	applyWorkflowAgentRuntimeMetadata(req)

	decision := hostops.RouteDecision{Kind: hostops.RouteKindNormalChat, Mentions: append([]hostops.HostMention(nil), mentions...), Reason: strings.Join(activeRoute.Reasons, "; ")}
	_, addWorkflowRequest := parseAddWorkflowMention(messageText)
	_, plainWorkflowRequest := parsePlainWorkflowWritingRequest(messageText)
	if !addWorkflowRequest && !plainWorkflowRequest && activeRoute.Mode == ChatRouteMultiHostOps {
		decision = hostops.RouteDecision{
			Kind:         hostops.RouteKindHostOps,
			Mentions:     append([]hostops.HostMention(nil), mentions...),
			PlanRequired: true,
			Reason:       "multi-host operation requires plan mode",
		}
		req.Metadata["aiops.hostops.routeKind"] = string(decision.Kind)
		req.Metadata["aiops.hostops.planRequired"] = boolMetadataString(decision.PlanRequired)
		req.Metadata["aiops.hostops.serverDetectedMultiHost"] = boolMetadataString(decision.PlanRequired)
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], hostops.ToolPackHostOps)
		if serialized, err := json.Marshal(decision.Mentions); err == nil {
			req.Metadata["aiops.hostops.mentions"] = string(serialized)
		}
	}
	return hostOpsTransportRoute{decision: decision, metadata: req.Metadata}
}

type hostOpsTransportRoute struct {
	decision hostops.RouteDecision
	metadata map[string]string
}

func appendMetadataListValue(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	values := strings.FieldsFunc(current, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	for _, value := range values {
		if strings.TrimSpace(value) == next {
			return strings.TrimSpace(current)
		}
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	return strings.TrimSpace(current) + "," + next
}

func addHostOpsMissionFromRoute(state AiopsTransportState, route hostOpsTransportRoute, turnID string) AiopsTransportState {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		turnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}
	missionID := strings.TrimSpace(route.metadata["aiops.hostops.missionId"])
	if missionID == "" {
		missionID = "hostops:" + turnID
	}
	if state.HostMissions == nil {
		state.HostMissions = map[string]AiopsTransportHostMission{}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := string(hostops.HostMissionStatusPlanning)
	if route.decision.PlanRequired {
		status = string(hostops.HostMissionStatusWaitingPlanAcceptance)
	}
	mission := state.HostMissions[missionID]
	if mission.ID == "" {
		mission.ID = missionID
		mission.CreatedAt = now
	}
	mission.TurnID = turnID
	mission.Status = status
	mission.PlanRequired = route.decision.PlanRequired
	mission.PlanAccepted = false
	mission.MentionedHosts = transportHostMentionsFromHostOps(route.decision.Mentions)
	mission.UpdatedAt = now
	state.HostMissions[missionID] = mission
	state.ActiveHostMissionID = missionID
	return state
}

func transportHostMentionsFromHostOps(mentions []hostops.HostMention) []AiopsTransportHostMention {
	result := make([]AiopsTransportHostMention, 0, len(mentions))
	for _, mention := range mentions {
		result = append(result, AiopsTransportHostMention{
			TokenID:     mention.TokenID,
			Raw:         mention.Raw,
			HostID:      mention.HostID,
			Address:     mention.Address,
			DisplayName: mention.DisplayName,
			Source:      string(mention.Source),
			Resolved:    mention.Resolved,
		})
	}
	return result
}

func mergeHostOpsMissionView(state AiopsTransportState, view HostOperationView, turnID string) AiopsTransportState {
	missionID := strings.TrimSpace(view.ID)
	if missionID == "" {
		return state
	}
	if state.HostMissions == nil {
		state.HostMissions = map[string]AiopsTransportHostMission{}
	}
	if state.ChildAgents == nil {
		state.ChildAgents = map[string]AiopsTransportChildAgent{}
	}
	mission := state.HostMissions[missionID]
	mission.ID = missionID
	mission.TurnID = firstNonEmptyString(strings.TrimSpace(view.UserTurnID), strings.TrimSpace(turnID), mission.TurnID)
	mission.Status = firstNonEmptyString(strings.TrimSpace(view.Status), mission.Status)
	mission.PlanRequired = view.PlanRequired
	mission.PlanAccepted = view.PlanAccepted
	mission.ManagerAgentID = firstNonEmptyString(strings.TrimSpace(view.ManagerAgentID), mission.ManagerAgentID)
	mission.MentionedHosts = transportHostMentionsFromView(view.MentionedHosts)
	mission.PlanSteps = transportPlanStepsFromView(view.Plan)
	mission.CreatedAt = firstNonEmptyString(strings.TrimSpace(view.CreatedAt), mission.CreatedAt)
	mission.UpdatedAt = firstNonEmptyString(strings.TrimSpace(view.UpdatedAt), mission.UpdatedAt)
	mission.ChildAgentIDs = mission.ChildAgentIDs[:0]
	for _, childView := range view.ChildAgents {
		child := transportChildAgentFromView(childView)
		if child.ID == "" {
			continue
		}
		state.ChildAgents[child.ID] = child
		mission.ChildAgentIDs = append(mission.ChildAgentIDs, child.ID)
		mission.ActiveChildAgentID = child.ID
	}
	state.HostMissions[missionID] = mission
	state.ActiveHostMissionID = missionID
	return state
}

func transportHostMentionsFromView(mentions []HostMentionView) []AiopsTransportHostMention {
	result := make([]AiopsTransportHostMention, 0, len(mentions))
	for _, mention := range mentions {
		result = append(result, AiopsTransportHostMention{
			Raw:         strings.TrimSpace(mention.Raw),
			HostID:      strings.TrimSpace(mention.HostID),
			Address:     strings.TrimSpace(mention.Address),
			DisplayName: strings.TrimSpace(mention.DisplayName),
			Source:      strings.TrimSpace(mention.Source),
			Resolved:    mention.Resolved,
		})
	}
	return result
}

func transportPlanStepsFromView(plan *HostPlanView) []AiopsTransportPlanStep {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	steps := make([]AiopsTransportPlanStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		title := strings.TrimSpace(step.Title)
		steps = append(steps, AiopsTransportPlanStep{
			ID:               strings.TrimSpace(step.ID),
			Index:            step.Index,
			Text:             title,
			Title:            title,
			Summary:          strings.TrimSpace(step.Summary),
			Status:           strings.TrimSpace(step.Status),
			Risk:             strings.TrimSpace(step.Risk),
			HostIDs:          append([]string(nil), step.HostIDs...),
			ChildAgentIDs:    append([]string(nil), step.ChildAgentIDs...),
			ApprovalRequired: step.ApprovalRequired,
		})
	}
	return steps
}

func transportChildAgentFromView(view HostChildAgentView) AiopsTransportChildAgent {
	return AiopsTransportChildAgent{
		ID:                strings.TrimSpace(view.ID),
		MissionID:         strings.TrimSpace(view.MissionID),
		ParentAgentID:     strings.TrimSpace(view.ParentAgentID),
		SessionID:         strings.TrimSpace(view.SessionID),
		HostID:            strings.TrimSpace(view.HostID),
		HostAddress:       strings.TrimSpace(view.HostAddress),
		HostDisplayName:   strings.TrimSpace(view.HostDisplayName),
		Role:              strings.TrimSpace(view.Role),
		Task:              strings.TrimSpace(view.Task),
		CurrentStepTitle:  strings.TrimSpace(view.CurrentStepTitle),
		Status:            strings.TrimSpace(view.Status),
		PlanStepIDs:       append([]string(nil), view.PlanStepIDs...),
		LastInputPreview:  strings.TrimSpace(view.LastInputPreview),
		LastOutputPreview: strings.TrimSpace(view.LastOutputPreview),
		Error:             strings.TrimSpace(view.Error),
		StartedAt:         strings.TrimSpace(view.StartedAt),
		UpdatedAt:         strings.TrimSpace(view.UpdatedAt),
		CompletedAt:       strings.TrimSpace(view.CompletedAt),
	}
}

func boolMetadataString(value bool) string {
	if value {
		return "true"
	}
	return "false"
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
		SessionID: strings.TrimSpace(firstNonEmptyString(command.SessionID, state.SessionID)),
		TurnID:    strings.TrimSpace(firstNonEmptyString(command.TurnID, approval.TurnID, state.CurrentTurnID)),
		ID:        approvalID,
		Decision:  strings.TrimSpace(command.Decision),
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

func (h *TransportCommandHandler) applyHostPlanAccept(ctx context.Context, state AiopsTransportState, command *TransportHostPlanAcceptCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.hostOps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	view, err := h.hostOps.AcceptPlan(ctx, strings.TrimSpace(command.MissionID), strings.TrimSpace(command.PlanID))
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = updateHostMissionStatus(state, view.ID, view.Status, true)
	return state, TransportCommandResult{Status: view.Status}, nil
}

func (h *TransportCommandHandler) applyHostPlanRevise(ctx context.Context, state AiopsTransportState, command *TransportHostPlanReviseCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.hostOps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	view, err := h.hostOps.RevisePlan(ctx, strings.TrimSpace(command.MissionID), strings.TrimSpace(command.Instruction))
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = updateHostMissionStatus(state, view.ID, view.Status, false)
	return state, TransportCommandResult{Status: view.Status}, nil
}

func (h *TransportCommandHandler) applyChildAgentMessage(ctx context.Context, state AiopsTransportState, command *TransportChildAgentMessageCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.hostOps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	view, err := h.hostOps.SendChildMessage(ctx, strings.TrimSpace(command.ChildAgentID), strings.TrimSpace(command.Content))
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = updateChildAgentStatus(state, view.ID, view.Status)
	return state, TransportCommandResult{Status: view.Status}, nil
}

func (h *TransportCommandHandler) applyChildAgentStop(ctx context.Context, state AiopsTransportState, command *TransportChildAgentStopCommand) (AiopsTransportState, TransportCommandResult, error) {
	if h == nil || h.hostOps == nil || command == nil {
		return state, TransportCommandResult{}, nil
	}
	view, err := h.hostOps.StopChildAgent(ctx, strings.TrimSpace(command.ChildAgentID))
	if err != nil {
		return state, TransportCommandResult{}, err
	}
	state = updateChildAgentStatus(state, view.ID, view.Status)
	return state, TransportCommandResult{Status: view.Status}, nil
}

func updateHostMissionStatus(state AiopsTransportState, missionID string, status string, accepted bool) AiopsTransportState {
	missionID = strings.TrimSpace(missionID)
	if missionID == "" {
		return state
	}
	if state.HostMissions == nil {
		state.HostMissions = map[string]AiopsTransportHostMission{}
	}
	mission := state.HostMissions[missionID]
	if mission.ID == "" {
		mission.ID = missionID
	}
	if strings.TrimSpace(status) != "" {
		mission.Status = strings.TrimSpace(status)
	}
	if accepted {
		mission.PlanAccepted = true
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mission.UpdatedAt = now
	state.HostMissions[missionID] = mission
	state.UpdatedAt = now
	return state
}

func updateChildAgentStatus(state AiopsTransportState, childAgentID string, status string) AiopsTransportState {
	childAgentID = strings.TrimSpace(childAgentID)
	if childAgentID == "" {
		return state
	}
	if state.ChildAgents == nil {
		state.ChildAgents = map[string]AiopsTransportChildAgent{}
	}
	child := state.ChildAgents[childAgentID]
	if child.ID == "" {
		child.ID = childAgentID
	}
	if strings.TrimSpace(status) != "" {
		child.Status = strings.TrimSpace(status)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	child.UpdatedAt = now
	state.ChildAgents[childAgentID] = child
	state.UpdatedAt = now
	return state
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
