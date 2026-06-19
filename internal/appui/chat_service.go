package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/workflowgen"
)

type defaultChatService struct {
	runtime            RuntimeGateway
	sessions           SessionSource
	hosts              HostRepository
	hostOps            HostOpsService
	agentEvents        AgentEventService
	turnRunner         AsyncTurnRunner
	baseContext        context.Context
	workflowGeneration *WorkflowGenerationChatService
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
	return NewChatServiceWithContextAndHosts(baseContext, runtime, sessions, nil, agentEvents...)
}

func NewChatServiceWithHosts(runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextAndHosts(context.Background(), runtime, sessions, hosts, agentEvents...)
}

func NewChatServiceWithContextAndHosts(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextHostsAndHostOps(baseContext, runtime, sessions, hosts, nil, agentEvents...)
}

func NewChatServiceWithContextHostsAndHostOps(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, hostOps HostOpsService, agentEvents ...AgentEventService) ChatService {
	var eventService AgentEventService
	if len(agentEvents) > 0 {
		eventService = agentEvents[0]
	}
	if eventService == nil {
		eventService = NewAgentEventService(nil)
	}
	baseContext = normalizeBaseContext(baseContext)
	var workflowGeneration *WorkflowGenerationChatService
	if sessionStore, ok := sessions.(SessionStore); ok {
		workflowGeneration = NewWorkflowGenerationChatService(sessionStore, workflowgen.NewMemorySessionStore(), workflowgen.DeterministicPlanBuilder{}, workflowgen.RunnerGraphGenerator{}, eventService)
	}
	return &defaultChatService{
		runtime:            runtime,
		sessions:           sessions,
		hosts:              hosts,
		hostOps:            hostOps,
		agentEvents:        eventService,
		baseContext:        baseContext,
		workflowGeneration: workflowGeneration,
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
		Metadata:        cloneStringMetadata(cmd.Metadata),
	}
	if strings.TrimSpace(req.HostID) == "" && req.SessionType == runtimekernel.SessionTypeHost {
		req.HostID = serverLocalHostID
	}
	if req.SessionID == "" {
		req.SessionID = strings.TrimSpace(cmd.SessionID)
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	s.enrichTurnHostMetadata(&req)
	if s.workflowGeneration != nil {
		if response, handled, err := s.workflowGeneration.Handle(ctx, cmd, req); handled || err != nil {
			return response, err
		}
	}
	if response, handled, err := s.handleHostOpsRoute(ctx, cmd, &req); handled || err != nil {
		return response, err
	}
	if response, handled, err := s.handleGenericOpsRepair(ctx, cmd, req); handled || err != nil {
		return response, err
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

func (s *defaultChatService) handleHostOpsRoute(ctx context.Context, cmd ChatCommand, req *runtimekernel.TurnRequest) (TurnResponse, bool, error) {
	if s == nil || s.hostOps == nil || req == nil {
		return TurnResponse{}, false, nil
	}
	mentions := s.hostOpsMentionsForCommand(ctx, cmd, req.Input)
	decision := hostops.DetectRoute(req.Input, mentions)
	if decision.Kind != hostops.RouteKindHostOps {
		return TurnResponse{}, false, nil
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	missionID := "hostops:" + strings.TrimSpace(req.TurnID)
	req.Metadata["aiops.hostops.routeKind"] = string(decision.Kind)
	req.Metadata["aiops.hostops.planRequired"] = boolMetadataString(decision.PlanRequired)
	req.Metadata["aiops.hostops.serverDetectedMultiHost"] = boolMetadataString(decision.PlanRequired)
	req.Metadata["aiops.hostops.missionId"] = missionID
	req.Metadata["aiops.hostops.managerAgentId"] = firstNonEmptyString(strings.TrimSpace(req.Metadata["aiops.hostops.managerAgentId"]), "hostops-manager:"+req.TurnID)
	req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], hostops.ToolPackHostOps)
	if serialized, err := json.Marshal(decision.Mentions); err == nil {
		req.Metadata["aiops.hostops.mentions"] = string(serialized)
	}
	view, err := s.hostOps.CreateMission(ctx, HostMissionCreateCommand{
		ID:             missionID,
		ThreadID:       firstNonEmptyString(strings.TrimSpace(cmd.SessionID), strings.TrimSpace(req.SessionID)),
		SessionID:      strings.TrimSpace(req.SessionID),
		UserTurnID:     strings.TrimSpace(req.TurnID),
		ManagerAgentID: strings.TrimSpace(req.Metadata["aiops.hostops.managerAgentId"]),
		Goal:           req.Input,
		Mentions:       decision.Mentions,
	})
	if err != nil {
		return TurnResponse{}, true, err
	}
	s.writeHostOpsMissionTurn(*req, view)
	return TurnResponse{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "accepted",
	}, true, nil
}

func (s *defaultChatService) writeHostOpsMissionTurn(req runtimekernel.TurnRequest, view HostOperationView) {
	store, ok := s.sessions.(SessionStore)
	if !ok {
		return
	}
	now := time.Now().UTC()
	completedAt := now
	summary := hostOpsMissionSummary(view)
	turn := runtimekernel.TurnSnapshot{
		ID:              req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionID:       req.SessionID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Metadata:        cloneStringMetadata(req.Metadata),
		Lifecycle:       runtimekernel.TurnLifecycleCompleted,
		ResumeState:     runtimekernel.TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
		CompletedAt:     &completedAt,
		AgentItems:      hostOpsMissionTurnItems(req, view, summary, now),
		FinalOutput:     summary,
	}
	writeHostOpsMissionSessionTurn(store, req, summary, turn, now)
}

func hostOpsMissionTurnItems(req runtimekernel.TurnRequest, view HostOperationView, summary string, now time.Time) []agentstate.TurnItem {
	items := []agentstate.TurnItem{
		{
			ID:     req.TurnID + "-user",
			Type:   agentstate.TurnItemTypeUserMessage,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "turn",
				Summary: req.Input,
				Data:    mustJSON(map[string]any{"prompt": req.Input, "summary": req.Input}),
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	if view.Plan != nil && len(view.Plan.Steps) > 0 {
		items = append(items, hostOpsMissionPlanItem(req, view, now))
	}
	if len(view.ChildAgents) > 0 {
		items = append(items, hostOpsMissionChildrenToolItem(req, view, now))
	}
	items = append(items, agentstate.TurnItem{
		ID:     req.TurnID + "-final",
		Type:   agentstate.TurnItemTypeFinalAnswer,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "final",
			Summary: summary,
			Data:    mustJSON(map[string]any{"summary": summary}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return items
}

func hostOpsMissionPlanItem(req runtimekernel.TurnRequest, view HostOperationView, now time.Time) agentstate.TurnItem {
	steps := make([]map[string]any, 0, len(view.Plan.Steps))
	for _, step := range view.Plan.Steps {
		steps = append(steps, map[string]any{
			"id":               step.ID,
			"index":            step.Index,
			"title":            step.Title,
			"summary":          step.Summary,
			"status":           step.Status,
			"hostIds":          step.HostIDs,
			"childAgentIds":    step.ChildAgentIDs,
			"risk":             step.Risk,
			"approvalRequired": step.ApprovalRequired,
		})
	}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-hostops-plan",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "hostops.plan",
			Summary: "主机运维计划",
			Data: mustJSON(map[string]any{
				"title": "主机运维计划",
				"steps": steps,
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func hostOpsMissionChildrenToolItem(req runtimekernel.TurnRequest, view HostOperationView, now time.Time) agentstate.TurnItem {
	children := make([]AiopsTransportChildAgent, 0, len(view.ChildAgents))
	for _, childView := range view.ChildAgents {
		if child := transportChildAgentFromView(childView); child.ID != "" {
			children = append(children, child)
		}
	}
	toolCallID := req.TurnID + "-spawn-host-agent"
	outputPreview := mustJSON(map[string]any{"children": children})
	payload := transportToolPayload{
		ID:            toolCallID,
		ToolCallID:    toolCallID,
		ToolName:      hostops.ToolSpawnHostAgent,
		Name:          hostops.ToolSpawnHostAgent,
		DisplayKind:   "hostops.spawn_host_agent",
		InputSummary:  "启动 host-bound 主机 Agent",
		OutputSummary: hostOpsMissionSummary(view),
		OutputPreview: outputPreview,
	}
	return agentstate.TurnItem{
		ID:     toolCallID,
		Type:   agentstate.TurnItemTypeToolResult,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "hostops.spawn_host_agent",
			Summary: payload.OutputSummary,
			Data:    mustJSON(payload),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func writeHostOpsMissionSessionTurn(store SessionStore, req runtimekernel.TurnRequest, assistantText string, turn runtimekernel.TurnSnapshot, now time.Time) {
	session := store.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
	if session.HostID == "" {
		session.HostID = req.HostID
	}
	if session.CurrentTurn != nil {
		session.TurnHistory = append(session.TurnHistory, *session.CurrentTurn)
	}
	session.Messages = append(session.Messages,
		runtimekernel.Message{
			ID:              firstNonEmptyString(req.ClientMessageID, req.TurnID+":user"),
			ClientMessageID: req.ClientMessageID,
			ClientTurnID:    req.ClientTurnID,
			Role:            "user",
			Content:         req.Input,
			Timestamp:       now,
			Metadata:        cloneStringMetadata(req.Metadata),
		},
		runtimekernel.Message{
			ID:           req.TurnID + ":assistant",
			ClientTurnID: req.ClientTurnID,
			Role:         "assistant",
			Content:      assistantText,
			Timestamp:    now,
			Metadata:     cloneStringMetadata(req.Metadata),
		},
	)
	session.CurrentTurn = &turn
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	store.Update(session)
}

func hostOpsMissionSummary(view HostOperationView) string {
	if len(view.ChildAgents) > 0 {
		return fmt.Sprintf("已创建主机运维任务，并启动 %d 个 host-bound 主机 Agent。", len(view.ChildAgents))
	}
	if view.PlanRequired && !view.PlanAccepted {
		return "已创建多主机运维计划，等待确认后启动 host-bound 主机 Agent。"
	}
	return "已创建主机运维任务，等待主机 Agent 调度。"
}

func (s *defaultChatService) hostOpsMentionsForCommand(ctx context.Context, cmd ChatCommand, content string) []hostops.HostMention {
	mentions := hostOpsMentionsFromMetadata(cmd.Metadata["aiops.hostops.mentions"])
	if len(mentions) == 0 {
		mentions = hostops.ParseHostMentions(content)
	}
	if len(mentions) == 0 {
		return nil
	}
	if s == nil || s.hosts == nil {
		return filterHostOpsRouteMentions(mentions)
	}
	resolved, _ := hostops.NewResolver(hostRepositoryLookup{repo: s.hosts}).Resolve(ctx, mentions)
	return filterHostOpsRouteMentions(resolved)
}

func filterHostOpsRouteMentions(mentions []hostops.HostMention) []hostops.HostMention {
	out := make([]hostops.HostMention, 0, len(mentions))
	for _, mention := range mentions {
		if strings.TrimSpace(mention.HostID) != "" || mention.Resolved {
			out = append(out, mention)
			continue
		}
		if mention.Source == hostops.HostMentionSourceIPLiteral && strings.TrimSpace(mention.Address) != "" {
			out = append(out, mention)
		}
	}
	return out
}

type hostMentionMetadataItem struct {
	TokenID     string  `json:"tokenId"`
	Raw         string  `json:"raw"`
	Value       string  `json:"value"`
	Start       int     `json:"start"`
	End         int     `json:"end"`
	SpanStart   int     `json:"spanStart"`
	SpanEnd     int     `json:"spanEnd"`
	HostID      string  `json:"hostId"`
	Address     string  `json:"address"`
	DisplayName string  `json:"displayName"`
	Source      string  `json:"source"`
	Resolved    bool    `json:"resolved"`
	Confidence  float64 `json:"confidence"`
}

func hostOpsMentionsFromMetadata(raw string) []hostops.HostMention {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []hostMentionMetadataItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	now := time.Now().UTC()
	mentions := make([]hostops.HostMention, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item.Value)
		mentionRaw := strings.TrimSpace(item.Raw)
		if mentionRaw == "" && value != "" {
			mentionRaw = "@" + strings.TrimPrefix(value, "@")
		}
		if mentionRaw == "" && strings.TrimSpace(item.HostID) == "" && strings.TrimSpace(item.Address) == "" {
			continue
		}
		spanStart := item.SpanStart
		if spanStart == 0 {
			spanStart = item.Start
		}
		spanEnd := item.SpanEnd
		if spanEnd == 0 {
			spanEnd = item.End
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" && value != "" {
			displayName = strings.TrimPrefix(value, "@")
		}
		source := hostops.HostMentionSource(strings.TrimSpace(item.Source))
		if source == "" {
			source = hostops.HostMentionSourceHostnameLiteral
		}
		confidence := item.Confidence
		if confidence == 0 {
			confidence = 0.75
		}
		mentions = append(mentions, hostops.HostMention{
			TokenID:     strings.TrimSpace(item.TokenID),
			Raw:         mentionRaw,
			SpanStart:   spanStart,
			SpanEnd:     spanEnd,
			HostID:      strings.TrimSpace(item.HostID),
			Address:     strings.TrimSpace(item.Address),
			DisplayName: displayName,
			Source:      source,
			Resolved:    item.Resolved,
			Confidence:  confidence,
			CreatedAt:   now,
		})
	}
	return mentions
}

type hostRepositoryLookup struct {
	repo HostRepository
}

func (l hostRepositoryLookup) ListHosts(context.Context) ([]hostops.HostRecordView, error) {
	if l.repo == nil {
		return nil, nil
	}
	records, err := l.repo.ListHosts()
	if err != nil {
		return nil, err
	}
	hosts := make([]hostops.HostRecordView, 0, len(records))
	for _, record := range records {
		hosts = append(hosts, hostops.HostRecordView{
			ID:          strings.TrimSpace(record.ID),
			Address:     strings.TrimSpace(record.Address),
			Hostname:    strings.TrimSpace(record.Name),
			DisplayName: strings.TrimSpace(record.Name),
			Managed:     strings.TrimSpace(record.AgentURL) != "",
			Executable:  record.Executable,
			AgentURL:    strings.TrimSpace(record.AgentURL),
		})
	}
	return hosts, nil
}

func (s *defaultChatService) enrichTurnHostMetadata(req *runtimekernel.TurnRequest) {
	if s == nil || s.hosts == nil || req == nil || strings.TrimSpace(req.HostID) == "" {
		return
	}
	host, err := s.hosts.GetHost(strings.TrimSpace(req.HostID))
	if err != nil || host == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	setMetadataIfEmpty(req.Metadata, "aiops.host.metadataAvailable", "true")
	setMetadataIfEmpty(req.Metadata, "aiops.host.id", host.ID)
	setMetadataIfEmpty(req.Metadata, "aiops.host.label", firstNonEmpty(host.Name, host.ID))
	setMetadataIfEmpty(req.Metadata, "aiops.host.os", host.OS)
	setMetadataIfEmpty(req.Metadata, "aiops.host.arch", host.Arch)
	setMetadataIfEmpty(req.Metadata, "aiops.host.transport", host.Transport)
	setMetadataIfEmpty(req.Metadata, "aiops.host.status", host.Status)
	setMetadataIfEmpty(req.Metadata, "aiops.host.address", host.Address)
	setMetadataIfEmpty(req.Metadata, "aiops.host.sshUser", host.SSHUser)
	if host.SSHPort > 0 {
		setMetadataIfEmpty(req.Metadata, "aiops.host.sshPort", strconv.Itoa(host.SSHPort))
	}
}

func setMetadataIfEmpty(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || strings.TrimSpace(metadata[key]) != "" {
		return
	}
	metadata[key] = value
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
