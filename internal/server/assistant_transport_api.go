package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

const assistantTransportPollInterval = 10 * time.Millisecond

type assistantTransportSessionSourceProvider interface {
	SessionSource() appui.SessionSource
}

func (s *HTTPServer) handleAssistantTransport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source := s.assistantTransportSessionSource()
	if source == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant transport session source is not configured"})
		return
	}

	req, err := decodeAssistantTransportRequest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	commands, err := assistantTransportCommandsFromRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	encoder := newAssistantTransportStreamEncoder(w)
	projector := appui.NewTransportProjector()
	handler := appui.NewTransportCommandHandler(s.ui.ChatService(), s.ui.ApprovalService(), s.ui.ChoiceService(), s.ui.MCPService())

	state := assistantTransportInitialState(req)
	prev := state
	for _, command := range commands {
		next, _, applyErr := handler.Apply(r.Context(), assistantTransportCloneState(state), command)
		if applyErr != nil {
			next.Status = appui.AiopsTransportStatusFailed
			next.LastError = strings.TrimSpace(applyErr.Error())
			next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := encoder.WriteStateOps(assistantTransportDiffStateOps(prev, next)); err != nil {
				return
			}
			_ = encoder.WriteError(next.LastError)
			return
		}
		if err := encoder.WriteStateOps(assistantTransportDiffStateOps(prev, next)); err != nil {
			return
		}
		prev = next
		state = next
	}

	shouldPoll := strings.TrimSpace(state.SessionID) != "" && (state.CurrentTurnID != "" || state.Status == appui.AiopsTransportStatusWorking || state.Status == appui.AiopsTransportStatusBlocked)
	if !shouldPoll {
		return
	}

	if _, err := s.streamAssistantTransportState(r.Context(), encoder, source, projector, s.ui.ChatService(), state); err != nil {
		return
	}
}

func (s *HTTPServer) assistantTransportSessionSource() appui.SessionSource {
	if s == nil || s.ui == nil {
		return nil
	}
	provider, ok := s.ui.(assistantTransportSessionSourceProvider)
	if !ok {
		return nil
	}
	return provider.SessionSource()
}

func assistantTransportInitialState(req *assistantTransportRequest) appui.AiopsTransportState {
	if req == nil {
		return appui.NewAiopsTransportState("", "")
	}
	state := req.State
	if strings.TrimSpace(state.SchemaVersion) == "" {
		state = appui.NewAiopsTransportState(strings.TrimSpace(state.SessionID), strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.ThreadID)))
	}
	if strings.TrimSpace(state.ThreadID) == "" {
		state.ThreadID = strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.SessionID))
	}
	if strings.TrimSpace(state.SessionID) == "" {
		state.SessionID = strings.TrimSpace(req.State.SessionID)
	}
	if state.Turns == nil {
		state.Turns = map[string]appui.AiopsTransportTurn{}
	}
	if state.TurnOrder == nil {
		state.TurnOrder = []string{}
	}
	if state.PendingApprovals == nil {
		state.PendingApprovals = map[string]appui.AiopsTransportApproval{}
	}
	if state.McpSurfaces == nil {
		state.McpSurfaces = map[string]appui.AiopsTransportMcpSurface{}
	}
	if state.Artifacts == nil {
		state.Artifacts = map[string]appui.AiopsTransportArtifact{}
	}
	if state.RuntimeLiveness.ActiveTurns == nil {
		state.RuntimeLiveness.ActiveTurns = map[string]bool{}
	}
	if state.RuntimeLiveness.ActiveAgents == nil {
		state.RuntimeLiveness.ActiveAgents = map[string]bool{}
	}
	if state.RuntimeLiveness.PendingApprovals == nil {
		state.RuntimeLiveness.PendingApprovals = map[string]bool{}
	}
	if state.RuntimeLiveness.PendingUserInputs == nil {
		state.RuntimeLiveness.PendingUserInputs = map[string]bool{}
	}
	if state.RuntimeLiveness.ActiveCommandStreams == nil {
		state.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
	}
	if strings.TrimSpace(state.UpdatedAt) == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return state
}

func assistantTransportCommandsFromRequest(req *assistantTransportRequest) ([]appui.TransportCommand, error) {
	if req == nil || len(req.Commands) == 0 {
		return nil, nil
	}
	state := assistantTransportInitialState(req)
	commands := make([]appui.TransportCommand, 0, len(req.Commands))
	for _, command := range req.Commands {
		next, err := assistantTransportCommandFromDecoded(command, req, state)
		if err != nil {
			return nil, err
		}
		commands = append(commands, next)
	}
	return commands, nil
}

func assistantTransportCommandFromDecoded(raw assistantTransportCommand, req *assistantTransportRequest, state appui.AiopsTransportState) (appui.TransportCommand, error) {
	switch command := raw.(type) {
	case *assistantTransportAddMessageCommand:
		text, sourceID, hostID, metadata, err := decodeAssistantTransportMessage(command.Message)
		if err != nil {
			return appui.TransportCommand{}, err
		}
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeAddMessage,
			AddMessage: &appui.TransportAddMessageCommand{
				SessionID: state.SessionID,
				ThreadID:  strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.ThreadID)),
				ParentID:  strings.TrimSpace(req.ParentID),
				SourceID:  sourceID,
				HostID:    strings.TrimSpace(hostID),
				Message:   appui.TransportUserMessage{Text: text},
				Metadata:  metadata,
			},
		}, nil
	case *assistantTransportRetryCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeRetry,
			Retry: &appui.TransportRetryCommand{
				SessionID: strings.TrimSpace(firstAssistantTransportValue(command.SessionID, state.SessionID)),
				TurnID:    strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
			},
		}, nil
	case *assistantTransportStopCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeStop,
			Stop: &appui.TransportStopCommand{
				SessionID: strings.TrimSpace(firstAssistantTransportValue(command.SessionID, state.SessionID)),
				TurnID:    strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
				Reason:    strings.TrimSpace(command.Reason),
			},
		}, nil
	case *assistantTransportApprovalDecisionCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeApprovalDecision,
			ApprovalDecision: &appui.TransportApprovalDecisionCommand{
				ApprovalID: strings.TrimSpace(command.ApprovalID),
				Decision:   strings.TrimSpace(command.Decision),
			},
		}, nil
	case *assistantTransportChoiceAnswerCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeChoiceAnswer,
			ChoiceAnswer: &appui.TransportChoiceAnswerCommand{
				RequestID: strings.TrimSpace(command.RequestID),
				Answer:    strings.TrimSpace(command.Answer),
			},
		}, nil
	case *assistantTransportMCPActionCommand:
		action := strings.TrimSpace(command.Action)
		if strings.EqualFold(action, "refresh") {
			return appui.TransportCommand{
				Type: appui.TransportCommandTypeMCPRefresh,
				MCPRefresh: &appui.TransportMCPRefreshCommand{
					SurfaceID: strings.TrimSpace(firstAssistantTransportValue(command.SurfaceID, command.Target)),
				},
			}, nil
		}
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPAction,
			MCPAction: &appui.TransportMCPActionCommand{
				SurfaceID: strings.TrimSpace(firstAssistantTransportValue(command.SurfaceID, command.Target)),
				ActionID:  action,
				Input:     cloneTransportAnyMap(command.Params),
			},
		}, nil
	case *assistantTransportMCPRefreshCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPRefresh,
			MCPRefresh: &appui.TransportMCPRefreshCommand{
				SurfaceID: strings.TrimSpace(command.SurfaceID),
			},
		}, nil
	case *assistantTransportMCPPinCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPPin,
			MCPPin: &appui.TransportMCPPinCommand{
				SurfaceID: strings.TrimSpace(command.SurfaceID),
				Pinned:    command.Pinned,
			},
		}, nil
	case *assistantTransportInsertArtifactCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeInsertArtifact,
			InsertArtifact: &appui.TransportInsertArtifactCommand{
				TurnID:   strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
				Artifact: command.Artifact,
			},
		}, nil
	default:
		return appui.TransportCommand{}, errors.New("assistant transport command is not supported")
	}
}

func decodeAssistantTransportMessage(raw json.RawMessage) (text string, sourceID string, hostID string, metadata map[string]string, err error) {
	var payload struct {
		ID       string            `json:"id"`
		Role     string            `json:"role"`
		HostID   string            `json:"hostId"`
		Metadata map[string]string `json:"metadata"`
		Content  []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", nil, err
	}
	parts := make([]string, 0, len(payload.Content)+len(payload.Parts))
	for _, item := range payload.Content {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") {
			if value := strings.TrimSpace(item.Text); value != "" {
				parts = append(parts, value)
			}
		}
	}
	for _, item := range payload.Parts {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") {
			if value := strings.TrimSpace(item.Text); value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, "\n"), strings.TrimSpace(payload.ID), strings.TrimSpace(payload.HostID), cloneTransportMetadata(payload.Metadata), nil
}

func (s *HTTPServer) streamAssistantTransportState(
	ctx context.Context,
	encoder *assistantTransportStreamEncoder,
	source appui.SessionSource,
	projector *appui.TransportProjector,
	chat appui.ChatService,
	state appui.AiopsTransportState,
) (appui.AiopsTransportState, error) {
	ticker := time.NewTicker(assistantTransportPollInterval)
	defer ticker.Stop()

	current := state
	lastFingerprint := ""
	for {
		session := source.Get(current.SessionID)
		waitingForAcceptedApproval := false
		if session != nil {
			latestTurn := assistantTransportLatestSessionTurn(session)
			waitingForAcceptedTurn := assistantTransportShouldWaitForAcceptedTurn(current, latestTurn)
			waitingForAcceptedApproval = assistantTransportShouldWaitForAcceptedApproval(current, latestTurn)
			fingerprint := assistantTransportTurnFingerprint(latestTurn)
			if !waitingForAcceptedTurn && !waitingForAcceptedApproval && fingerprint != "" && fingerprint != lastFingerprint {
				next, err := projectAssistantTransportSessionState(assistantTransportCloneState(current), session, projector)
				if err != nil {
					return current, err
				}
				next = s.attachAssistantTransportExperiencePackSuggestions(next, session)
				if err := encoder.WriteStateOps(assistantTransportDiffStateOps(current, next)); err != nil {
					return current, err
				}
				if assistantTransportShouldWriteError(next) {
					if err := encoder.WriteError(next.LastError); err != nil {
						return next, err
					}
				}
				current = next
				lastFingerprint = fingerprint
			}
			if assistantTransportSessionTurnShouldCloseStream(session) {
				if !waitingForAcceptedTurn && !waitingForAcceptedApproval && current.Status != appui.AiopsTransportStatusWorking && current.Status != appui.AiopsTransportStatusBlocked {
					return current, nil
				}
				if !waitingForAcceptedTurn && !waitingForAcceptedApproval && current.Status == appui.AiopsTransportStatusBlocked {
					return current, nil
				}
			}
		}

		select {
		case <-ctx.Done():
			if !waitingForAcceptedApproval && shouldCancelAssistantTransportOnContextDone(current, session) {
				_ = cancelAssistantTransportTurn(context.Background(), chat, current, session)
			}
			return current, ctx.Err()
		case <-ticker.C:
		}
	}
}

func assistantTransportShouldWaitForAcceptedTurn(state appui.AiopsTransportState, latest *runtimekernel.TurnSnapshot) bool {
	if latest == nil {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking && state.Status != appui.AiopsTransportStatusBlocked {
		return false
	}
	currentTurnID := strings.TrimSpace(state.CurrentTurnID)
	latestTurnID := strings.TrimSpace(latest.ID)
	if currentTurnID == "" || latestTurnID == "" || currentTurnID == latestTurnID {
		return false
	}
	return latest.Lifecycle.IsTerminal()
}

func assistantTransportShouldWaitForAcceptedApproval(state appui.AiopsTransportState, latest *runtimekernel.TurnSnapshot) bool {
	if latest == nil {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking &&
		state.Status != appui.AiopsTransportStatusFailed &&
		state.Status != appui.AiopsTransportStatusCanceled {
		return false
	}
	currentTurnID := strings.TrimSpace(state.CurrentTurnID)
	latestTurnID := strings.TrimSpace(latest.ID)
	if currentTurnID == "" || latestTurnID == "" || currentTurnID != latestTurnID {
		return false
	}
	if state.Status == appui.AiopsTransportStatusWorking && !state.RuntimeLiveness.ActiveTurns[latestTurnID] {
		return false
	}
	if latest.Lifecycle != runtimekernel.TurnLifecycleSuspended && latest.Lifecycle != runtimekernel.TurnLifecycleResumable {
		return false
	}
	pendingIDs := assistantTransportPendingApprovalIDs(latest)
	if len(pendingIDs) == 0 {
		return false
	}
	if !assistantTransportHasLocalApprovalDecision(state, latestTurnID, pendingIDs) {
		return false
	}
	for approvalID := range pendingIDs {
		if _, ok := state.PendingApprovals[approvalID]; ok {
			return false
		}
		if state.RuntimeLiveness.PendingApprovals[approvalID] {
			return false
		}
	}
	return true
}

func assistantTransportHasLocalApprovalDecision(state appui.AiopsTransportState, turnID string, pendingIDs map[string]bool) bool {
	turn, ok := state.Turns[strings.TrimSpace(turnID)]
	if !ok {
		return false
	}
	for _, block := range turn.Process {
		approvalID := strings.TrimSpace(block.ApprovalID)
		if approvalID == "" || !pendingIDs[approvalID] {
			continue
		}
		switch block.Status {
		case appui.AiopsTransportProcessStatusRunning,
			appui.AiopsTransportProcessStatusRejected,
			appui.AiopsTransportProcessStatusFailed:
			return true
		}
	}
	return false
}

func assistantTransportPendingApprovalIDs(turn *runtimekernel.TurnSnapshot) map[string]bool {
	pending := map[string]bool{}
	if turn == nil {
		return pending
	}
	for _, approval := range turn.PendingApprovals {
		if id := strings.TrimSpace(approval.ID); id != "" {
			pending[id] = true
		}
	}
	for _, evidence := range turn.PendingEvidence {
		if id := strings.TrimSpace(evidence.ID); id != "" {
			pending[id] = true
		}
	}
	return pending
}

func assistantTransportShouldWriteError(state appui.AiopsTransportState) bool {
	if state.Status != appui.AiopsTransportStatusFailed {
		return false
	}
	message := strings.TrimSpace(state.LastError)
	if message == "" {
		return false
	}
	normalized := strings.ToLower(message)
	if normalized == "approval denied" || normalized == "approval rejected" || normalized == "user denied approval" {
		return false
	}
	return true
}

func shouldCancelAssistantTransportOnContextDone(state appui.AiopsTransportState, session *runtimekernel.SessionState) bool {
	if session == nil || session.CurrentTurn == nil {
		return false
	}
	if session.CurrentTurn.Lifecycle.IsTerminal() {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking && state.Status != appui.AiopsTransportStatusBlocked {
		return false
	}
	return strings.TrimSpace(session.ID) != "" && strings.TrimSpace(session.CurrentTurn.ID) != ""
}

func cancelAssistantTransportTurn(ctx context.Context, chat appui.ChatService, state appui.AiopsTransportState, session *runtimekernel.SessionState) error {
	if chat == nil || session == nil || session.CurrentTurn == nil {
		return nil
	}
	_, err := chat.StopTurn(ctx, appui.StopCommand{
		SessionID: strings.TrimSpace(firstAssistantTransportValue(state.SessionID, session.ID)),
		TurnID:    strings.TrimSpace(firstAssistantTransportValue(state.CurrentTurnID, session.CurrentTurn.ID)),
		Reason:    "assistant transport client disconnected",
	})
	return err
}

func projectAssistantTransportSessionState(
	state appui.AiopsTransportState,
	session *runtimekernel.SessionState,
	projector *appui.TransportProjector,
) (appui.AiopsTransportState, error) {
	if projector == nil {
		projector = appui.NewTransportProjector()
	}
	next := state
	if session == nil {
		return next, nil
	}
	next.SessionID = strings.TrimSpace(firstAssistantTransportValue(session.ID, next.SessionID))
	if strings.TrimSpace(next.ThreadID) == "" {
		next.ThreadID = strings.TrimSpace(firstAssistantTransportValue(next.SessionID, session.ID))
	}
	if session.CurrentTurn != nil {
		return projector.ProjectTurnSnapshot(next, session.CurrentTurn)
	}
	if count := len(session.TurnHistory); count > 0 {
		latest := session.TurnHistory[count-1]
		return projector.ProjectTurnSnapshot(next, &latest)
	}
	next.Status = appui.AiopsTransportStatusIdle
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return next, nil
}

func (s *HTTPServer) attachAssistantTransportExperiencePackSuggestions(state appui.AiopsTransportState, session *runtimekernel.SessionState) appui.AiopsTransportState {
	turn := assistantTransportLatestSessionTurn(session)
	if turn == nil || turn.Lifecycle != runtimekernel.TurnLifecycleCompleted {
		state.ExperiencePackSuggestions = nil
		return state
	}
	projectedTurn, ok := state.Turns[strings.TrimSpace(turn.ID)]
	if !ok || !assistantTransportTurnHasExperiencePackValue(projectedTurn) {
		state.ExperiencePackSuggestions = nil
		return state
	}
	service := s.experiencePackService()
	if service == nil {
		state.ExperiencePackSuggestions = nil
		return state
	}
	caseID := assistantTransportSuggestionCaseID(state, session, turn)
	text := assistantTransportSuggestionText(projectedTurn)
	signals := assistantTransportSuggestionSignals(text)
	serviceName := assistantTransportDetectService(text)
	environment := assistantTransportDetectEnvironment(text)
	matches, _ := service.Retrieve(appui.ExperiencePackRetrieveRequest{
		CaseID:        caseID,
		ChatSessionID: strings.TrimSpace(state.SessionID),
		UserText:      text,
		Signals:       signals,
		Environment:   environment,
		Metadata: map[string]any{
			"threadId": strings.TrimSpace(state.ThreadID),
			"turnId":   strings.TrimSpace(turn.ID),
			"source":   "ai-chat",
		},
	})
	matchedPackID := ""
	if len(matches.Items) > 0 {
		matchedPackID = strings.TrimSpace(matches.Items[0].PackID)
		projectedTurn.AgentUiArtifacts = assistantTransportUpsertAgentArtifact(
			projectedTurn.AgentUiArtifacts,
			assistantTransportExperienceMatchArtifact(caseID, projectedTurn, matches.Items[0]),
		)
		state.Turns[strings.TrimSpace(turn.ID)] = projectedTurn
	}
	commands := assistantTransportExtractCommands(projectedTurn)
	result, err := service.EvaluateSuggestions(appui.ExperiencePackSuggestionEvaluateRequest{
		CaseID:                   caseID,
		Service:                  serviceName,
		Environment:              environment,
		Signals:                  signals,
		CommandCount:             assistantTransportEstimateCommandCount(projectedTurn),
		Outcome:                  "success",
		RedactionStatus:          "redacted",
		LLMOperationalValueScore: 0.8,
		MatchedPackID:            matchedPackID,
		MemoryGraphWritable:      true,
		ReusableStepCount:        assistantTransportEstimateReusableStepCount(projectedTurn),
		Metadata: map[string]any{
			"chatSessionId": strings.TrimSpace(state.SessionID),
			"threadId":      strings.TrimSpace(state.ThreadID),
			"turnId":        strings.TrimSpace(turn.ID),
			"source":        "ai-chat",
			"commands":      commands,
			"signals":       signals,
			"matchedPackId": matchedPackID,
		},
	})
	if err != nil || len(result.Items) == 0 {
		state.ExperiencePackSuggestions = nil
		return state
	}
	sourceRefs := []string{}
	if state.ThreadID != "" {
		sourceRefs = append(sourceRefs, state.ThreadID)
	}
	if state.SessionID != "" {
		sourceRefs = append(sourceRefs, state.SessionID)
	}
	if turn.ID != "" {
		sourceRefs = append(sourceRefs, turn.ID)
	}
	summary := assistantTransportSuggestionSummary(projectedTurn)
	state.ExperiencePackSuggestions = make([]appui.AiopsTransportExperiencePackSuggestion, 0, len(result.Items))
	for _, item := range result.Items {
		itemID := strings.TrimSpace(firstAssistantTransportValue(item.ID, item.Type))
		state.ExperiencePackSuggestions = append(state.ExperiencePackSuggestions, appui.AiopsTransportExperiencePackSuggestion{
			ID:          itemID,
			Type:        strings.TrimSpace(item.Type),
			Label:       strings.TrimSpace(item.Label),
			Reason:      strings.TrimSpace(item.Reason),
			CaseID:      caseID,
			PackID:      firstAssistantTransportValue(matchedPackID, assistantTransportSuggestionPackID(caseID, serviceName)),
			Title:       assistantTransportSuggestionTitle(serviceName),
			Summary:     summary,
			Service:     serviceName,
			Environment: environment,
			SourceRefs:  append([]string(nil), sourceRefs...),
			Metadata: map[string]any{
				"chatSessionId": strings.TrimSpace(state.SessionID),
				"threadId":      strings.TrimSpace(state.ThreadID),
				"turnId":        strings.TrimSpace(turn.ID),
				"suggestionId":  itemID,
				"commands":      commands,
				"signals":       signals,
				"matchedPackId": matchedPackID,
				"proofId":       assistantTransportProofID(projectedTurn),
			},
		})
	}
	return state
}

func assistantTransportExperienceMatchArtifact(caseID string, turn appui.AiopsTransportTurn, match appui.ExperiencePackMatch) appui.AiopsTransportAgentArtifact {
	packTitle := strings.TrimSpace(firstAssistantTransportValue(match.Skill.Name, match.PackID))
	confidence := match.Confidence
	return appui.AiopsTransportAgentArtifact{
		ID:              "experience-match-" + transportSafeID(firstAssistantTransportValue(match.PackID, turn.ID)),
		Type:            "experience_match",
		TitleZh:         "命中经验包",
		SummaryZh:       firstAssistantTransportValue(packTitle, "经验包") + " 已命中。AI 将先给出执行计划、风险范围、Runner 工作流和验证方式，确认后才会使用。",
		Status:          "ready",
		Source:          "ai-chat",
		CaseID:          caseID,
		RedactionStatus: "redacted",
		InlineData: map[string]any{
			"packId":              match.PackID,
			"skillName":           firstAssistantTransportValue(match.Skill.Name, match.Skill.Summary, match.PackID),
			"compatibilityStatus": match.CompatibilityStatus,
			"compatibilityGaps":   append([]string(nil), match.CompatibilityGaps...),
			"matchReasons":        append([]string(nil), match.MatchReasons...),
			"matchedSignals":      append([]string(nil), match.MatchedSignals...),
			"preconditionGaps":    append([]string(nil), match.PreconditionGaps...),
			"riskWarnings":        append([]string(nil), match.RiskWarnings...),
			"osVariant":           match.OSVariant,
			"runnerBinding":       match.RunnerBinding,
			"history":             match.History,
			"advancedRefs":        match.AdvancedRefs,
			"confidence":          confidence,
			"executionPlan":       assistantTransportExperienceExecutionPlan(match),
			"riskScope":           "执行前必须确认 HostLease、审批、Dry Run 和爆炸半径；环境不一致时仅生成 Runner 变体，不修改原经验包。",
			"validationItems":     []string{"环境预检查", "人工审批", "Dry Run", "受控执行", "恢复验证", "受控回滚"},
			"requiresReview":      true,
			"retrievalPipeline":   []string{"结构化条件过滤", "关键词/BM25", "向量语义检索", "环境指纹匹配", "GEP Gene signals_match"},
		},
		Actions: []map[string]any{
			{"id": "view-experience-pack", "label": "查看经验包", "href": "/settings/experience-packs", "mutation": false},
		},
	}
}

func assistantTransportExperienceExecutionPlan(match appui.ExperiencePackMatch) []string {
	steps := []string{
		"读取 Skill：确认为什么适用、前置条件、验证和回滚方式",
		"读取 GEP Gene：确认触发信号、安全约束、历史成功/失败和环境指纹",
	}
	if strings.TrimSpace(match.RunnerBinding.WorkflowID) != "" {
		steps = append(steps, "准备 Runner Workflow："+firstAssistantTransportValue(match.RunnerBinding.WorkflowName, match.RunnerBinding.WorkflowID))
	} else if match.CompatibilityStatus == "adapt_required" {
		steps = append(steps, "当前环境与经验包存在差异：只参考 Skill，先生成适配计划和 Runner 变体")
	} else if match.CompatibilityStatus == "reference_only" {
		steps = append(steps, "当前经验仅作参考：不使用原 Runner，每一步操作都需要用户审核")
	} else {
		steps = append(steps, "当前环境没有可直接执行的 Runner，先生成 Runner 变体")
	}
	steps = append(steps,
		"提交人工审核：确认目标、爆炸半径和回滚计划",
		"执行 Dry Run：只读验证参数和前置条件",
		"受控执行并完成恢复验证",
	)
	return steps
}

func assistantTransportUpsertAgentArtifact(items []appui.AiopsTransportAgentArtifact, artifact appui.AiopsTransportAgentArtifact) []appui.AiopsTransportAgentArtifact {
	id := strings.TrimSpace(artifact.ID)
	if id == "" {
		return items
	}
	for idx, item := range items {
		if strings.TrimSpace(item.ID) == id {
			next := append([]appui.AiopsTransportAgentArtifact(nil), items...)
			next[idx] = artifact
			return next
		}
	}
	next := append([]appui.AiopsTransportAgentArtifact(nil), items...)
	return append(next, artifact)
}

func assistantTransportTurnHasExperiencePackValue(turn appui.AiopsTransportTurn) bool {
	text := strings.ToLower(assistantTransportSuggestionText(turn))
	if text == "" {
		return false
	}
	keywords := []string{
		"运维", "排障", "故障", "告警", "修复", "恢复", "验证", "回滚", "部署",
		"备份", "dry run", "coroot", "redis", "mysql", "mysqldump", "postgres", "postgresql", "pg", "kubernetes", "p95", "latency",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func assistantTransportSuggestionText(turn appui.AiopsTransportTurn) string {
	parts := []string{}
	if turn.User != nil {
		parts = append(parts, turn.User.Text)
	}
	if turn.Final != nil {
		parts = append(parts, turn.Final.Text)
	}
	for _, block := range turn.Process {
		parts = append(parts, block.Text, block.Command, block.InputSummary, block.OutputPreview)
	}
	return strings.Join(parts, "\n")
}

func assistantTransportSuggestionSummary(turn appui.AiopsTransportTurn) string {
	if turn.Final != nil && strings.TrimSpace(turn.Final.Text) != "" {
		return trimAssistantTransportText(turn.Final.Text, 280)
	}
	if turn.User != nil && strings.TrimSpace(turn.User.Text) != "" {
		return trimAssistantTransportText(turn.User.Text, 280)
	}
	return "从 AI Chat 运维轨迹生成的候选经验。"
}

func assistantTransportEstimateCommandCount(turn appui.AiopsTransportTurn) int {
	if commands := assistantTransportExtractCommands(turn); len(commands) > 0 {
		return len(commands)
	}
	count := 0
	for _, block := range turn.Process {
		if block.Kind == appui.AiopsTransportProcessKindCommand || block.Kind == appui.AiopsTransportProcessKindTool {
			count++
		}
	}
	return count
}

func assistantTransportEstimateReusableStepCount(turn appui.AiopsTransportTurn) int {
	if commands := assistantTransportExtractCommands(turn); len(commands) > 0 {
		return len(commands)
	}
	count := 0
	for _, block := range turn.Process {
		if strings.TrimSpace(block.Text) != "" || strings.TrimSpace(block.Command) != "" {
			count++
		}
		if len(block.Steps) > 0 {
			count += len(block.Steps)
		}
	}
	return count
}

func assistantTransportExtractCommands(turn appui.AiopsTransportTurn) []string {
	candidates := []string{}
	for _, block := range turn.Process {
		for _, value := range []string{block.Command, block.InputSummary, block.OutputPreview, block.Text} {
			candidates = append(candidates, assistantTransportCommandLines(value)...)
		}
	}
	if turn.Final != nil {
		candidates = append(candidates, assistantTransportCommandLines(turn.Final.Text)...)
	}
	if turn.User != nil {
		candidates = append(candidates, assistantTransportCommandLines(turn.User.Text)...)
	}
	return assistantTransportUniqueCommands(candidates)
}

func assistantTransportCommandLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := []string{}
	for _, line := range lines {
		command := normalizeAssistantTransportCommandLine(line)
		if command != "" {
			result = append(result, command)
		}
	}
	return result
}

func normalizeAssistantTransportCommandLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	line = strings.Trim(line, "`")
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "$ ")
	line = strings.TrimPrefix(line, "# ")
	line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
	if len(line) > 2 && line[1] == '.' && line[0] >= '0' && line[0] <= '9' {
		line = strings.TrimSpace(line[2:])
	}
	if len(line) > 3 && line[2] == '.' && line[0] >= '0' && line[0] <= '9' && line[1] >= '0' && line[1] <= '9' {
		line = strings.TrimSpace(line[3:])
	}
	lower := strings.ToLower(line)
	prefixes := []string{
		"sudo ", "ssh ", "scp ", "rsync ", "systemctl ", "journalctl ", "docker ", "docker compose ", "kubectl ", "helm ",
		"psql ", "pg_ctl", "pg_basebackup", "initdb", "createuser ", "createdb ", "redis-cli ", "mysql ", "mysqldump ",
		"curl ", "wget ", "sed ", "awk ", "grep ", "cat ", "tee ", "mkdir ", "chown ", "chmod ", "cp ", "mv ", "tar ",
		"apt ", "apt-get ", "yum ", "dnf ", "brew ", "echo ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return line
		}
	}
	return ""
}

func assistantTransportUniqueCommands(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func assistantTransportProofID(turn appui.AiopsTransportTurn) string {
	seed := []string{turn.ID}
	if turn.Final != nil {
		seed = append(seed, turn.Final.ID)
	}
	if turn.User != nil {
		seed = append(seed, turn.User.ID)
	}
	return "proof-ai-chat-" + transportSafeID(strings.Join(seed, "-"))
}

func assistantTransportSuggestionCaseID(state appui.AiopsTransportState, session *runtimekernel.SessionState, turn *runtimekernel.TurnSnapshot) string {
	return "case-ai-chat-" + transportSafeID(firstAssistantTransportValue(state.ThreadID, state.SessionID, sessionID(session), turnID(turn), "session"))
}

func assistantTransportSuggestionPackID(caseID, serviceName string) string {
	base := caseID
	if serviceName != "" {
		base += "-" + serviceName
	}
	return "pack-" + transportSafeID(base)
}

func assistantTransportSuggestionTitle(serviceName string) string {
	switch serviceName {
	case "redis":
		return "Redis 运维排障经验包"
	case "mysql":
		return "MySQL 运维经验包"
	case "postgres":
		return "PostgreSQL 运维经验包"
	case "kubernetes":
		return "Kubernetes 运维经验包"
	default:
		return "AI Chat 运维经验包"
	}
}

func assistantTransportDetectService(text string) string {
	normalized := strings.ToLower(text)
	switch {
	case strings.Contains(normalized, "redis"):
		return "redis"
	case strings.Contains(normalized, "mysql"), strings.Contains(normalized, "mysqldump"), strings.Contains(normalized, "mariadb"):
		return "mysql"
	case strings.Contains(normalized, "postgres"), strings.Contains(normalized, "postgresql"), strings.Contains(normalized, "pg "):
		return "postgres"
	case strings.Contains(normalized, "kubernetes"), strings.Contains(normalized, "kubectl"), strings.Contains(normalized, "pod"):
		return "kubernetes"
	default:
		return "aiops"
	}
}

func assistantTransportDetectEnvironment(text string) string {
	normalized := strings.ToLower(text)
	switch {
	case strings.Contains(normalized, "prod"), strings.Contains(normalized, "生产"):
		return "prod"
	case strings.Contains(normalized, "staging"), strings.Contains(normalized, "预发"):
		return "staging"
	default:
		return "unknown"
	}
}

func assistantTransportSuggestionSignals(text string) []string {
	normalized := strings.ToLower(text)
	signals := []string{}
	for _, keyword := range []string{"redis", "used_memory_rss", "maxmemory", "payment-api", "p95", "slowlog", "big key", "mysql", "mysqldump", "backup", "备份", "aiops_biz", "orders", "coroot", "dry run", "rollback", "回滚", "恢复验证"} {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			signals = append(signals, keyword)
		}
	}
	if strings.Contains(normalized, "mysqldump") && !assistantTransportStringIn(signals, "备份") {
		signals = append(signals, "备份")
	}
	if len(signals) == 0 {
		signals = append(signals, "ai-chat-ops")
	}
	return signals
}

func assistantTransportStringIn(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sessionID(session *runtimekernel.SessionState) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.ID)
}

func turnID(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	return strings.TrimSpace(turn.ID)
}

func transportSafeID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func trimAssistantTransportText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func assistantTransportSessionTurnIsTerminal(session *runtimekernel.SessionState) bool {
	turn := assistantTransportLatestSessionTurn(session)
	if turn == nil {
		return false
	}
	return turn.Lifecycle.IsTerminal()
}

func assistantTransportSessionTurnShouldCloseStream(session *runtimekernel.SessionState) bool {
	turn := assistantTransportLatestSessionTurn(session)
	if turn == nil {
		return false
	}
	return turn.Lifecycle.IsTerminal() || turn.Lifecycle == runtimekernel.TurnLifecycleSuspended || turn.Lifecycle == runtimekernel.TurnLifecycleResumable
}

func assistantTransportLatestSessionTurn(session *runtimekernel.SessionState) *runtimekernel.TurnSnapshot {
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil {
		return session.CurrentTurn
	}
	if len(session.TurnHistory) == 0 {
		return nil
	}
	return &session.TurnHistory[len(session.TurnHistory)-1]
}

func assistantTransportTurnFingerprint(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(turn.ID),
		string(turn.Lifecycle),
		turn.UpdatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(turn.FinalOutput),
		strings.TrimSpace(turn.Error),
		assistantTransportTurnCompletedAtFingerprint(turn),
	}, "|")
}

func assistantTransportTurnCompletedAtFingerprint(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil || turn.CompletedAt == nil {
		return ""
	}
	return turn.CompletedAt.UTC().Format(time.RFC3339Nano)
}

func assistantTransportDiffStateOps(prev, next appui.AiopsTransportState) []assistantTransportStreamStateOp {
	ops := make([]assistantTransportStreamStateOp, 0, 16)
	appendSet := func(path []any, value any) {
		ops = append(ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpSet,
			Path:  path,
			Value: value,
		})
	}
	appendText := func(path []any, value string) {
		if value == "" {
			return
		}
		ops = append(ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpAppendText,
			Path:  path,
			Value: value,
		})
	}

	if prev.SchemaVersion != next.SchemaVersion {
		appendSet([]any{"schemaVersion"}, next.SchemaVersion)
	}
	if prev.SessionID != next.SessionID {
		appendSet([]any{"sessionId"}, next.SessionID)
	}
	if prev.ThreadID != next.ThreadID {
		appendSet([]any{"threadId"}, next.ThreadID)
	}
	if prev.CurrentTurnID != next.CurrentTurnID {
		appendSet([]any{"currentTurnId"}, next.CurrentTurnID)
	}
	if prev.Status != next.Status {
		appendSet([]any{"status"}, next.Status)
	}
	if prev.Seq != next.Seq {
		appendSet([]any{"seq"}, next.Seq)
	}
	if prev.UpdatedAt != next.UpdatedAt {
		appendSet([]any{"updatedAt"}, next.UpdatedAt)
	}
	if prev.LastError != next.LastError {
		appendSet([]any{"lastError"}, next.LastError)
	}
	if !reflect.DeepEqual(prev.TurnOrder, next.TurnOrder) {
		appendSet([]any{"turnOrder"}, next.TurnOrder)
	}
	if !reflect.DeepEqual(prev.PendingApprovals, next.PendingApprovals) {
		appendSet([]any{"pendingApprovals"}, next.PendingApprovals)
	}
	if !reflect.DeepEqual(prev.McpSurfaces, next.McpSurfaces) {
		appendSet([]any{"mcpSurfaces"}, next.McpSurfaces)
	}
	if !reflect.DeepEqual(prev.Artifacts, next.Artifacts) {
		appendSet([]any{"artifacts"}, next.Artifacts)
	}
	if !reflect.DeepEqual(prev.ExperiencePackSuggestions, next.ExperiencePackSuggestions) {
		appendSet([]any{"experiencePackSuggestions"}, next.ExperiencePackSuggestions)
	}
	if !reflect.DeepEqual(prev.RuntimeLiveness, next.RuntimeLiveness) {
		appendSet([]any{"runtimeLiveness"}, next.RuntimeLiveness)
	}

	seenTurns := map[string]struct{}{}
	for _, turnID := range next.TurnOrder {
		seenTurns[turnID] = struct{}{}
		nextTurn := next.Turns[turnID]
		prevTurn := prev.Turns[turnID]

		nextTurnForSet := nextTurn
		prevTurnForSet := prevTurn
		nextFinalText := ""
		prevFinalText := ""
		if prevTurnForSet.Final != nil {
			prevFinalText = prevTurnForSet.Final.Text
			finalCopy := *prevTurnForSet.Final
			finalCopy.Text = prevFinalText
			prevTurnForSet.Final = &finalCopy
		}
		if nextTurnForSet.Final != nil {
			nextFinalText = nextTurnForSet.Final.Text
			finalCopy := *nextTurnForSet.Final
			finalCopy.Text = prevFinalText
			nextTurnForSet.Final = &finalCopy
		}
		if !reflect.DeepEqual(prevTurnForSet, nextTurnForSet) {
			appendSet([]any{"turns", turnID}, nextTurnForSet)
		}
		if nextFinalText != prevFinalText {
			if strings.HasPrefix(nextFinalText, prevFinalText) {
				appendText([]any{"turns", turnID, "final", "text"}, nextFinalText[len(prevFinalText):])
			} else {
				appendSet([]any{"turns", turnID, "final", "text"}, "")
				appendText([]any{"turns", turnID, "final", "text"}, nextFinalText)
			}
		}
	}
	for turnID := range next.Turns {
		if _, ok := seenTurns[turnID]; ok {
			continue
		}
		nextTurn := next.Turns[turnID]
		prevTurn := prev.Turns[turnID]
		if !reflect.DeepEqual(prevTurn, nextTurn) {
			appendSet([]any{"turns", turnID}, nextTurn)
		}
	}

	return ops
}

func firstAssistantTransportValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneTransportMetadata(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTransportAnyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func assistantTransportCloneState(state appui.AiopsTransportState) appui.AiopsTransportState {
	cloned := state
	if len(state.TurnOrder) > 0 {
		cloned.TurnOrder = append([]string(nil), state.TurnOrder...)
	}
	cloned.Turns = make(map[string]appui.AiopsTransportTurn, len(state.Turns))
	for key, turn := range state.Turns {
		cloned.Turns[key] = assistantTransportCloneTurn(turn)
	}
	cloned.PendingApprovals = make(map[string]appui.AiopsTransportApproval, len(state.PendingApprovals))
	for key, approval := range state.PendingApprovals {
		cloned.PendingApprovals[key] = approval
	}
	cloned.McpSurfaces = make(map[string]appui.AiopsTransportMcpSurface, len(state.McpSurfaces))
	for key, surface := range state.McpSurfaces {
		cloned.McpSurfaces[key] = surface
	}
	cloned.Artifacts = make(map[string]appui.AiopsTransportArtifact, len(state.Artifacts))
	for key, artifact := range state.Artifacts {
		cloned.Artifacts[key] = artifact
	}
	if len(state.ExperiencePackSuggestions) > 0 {
		cloned.ExperiencePackSuggestions = make([]appui.AiopsTransportExperiencePackSuggestion, len(state.ExperiencePackSuggestions))
		for idx, suggestion := range state.ExperiencePackSuggestions {
			cloned.ExperiencePackSuggestions[idx] = assistantTransportCloneExperiencePackSuggestion(suggestion)
		}
	}
	cloned.RuntimeLiveness = appui.AiopsRuntimeLiveness{
		ActiveTurns:          cloneTransportBoolMap(state.RuntimeLiveness.ActiveTurns),
		ActiveAgents:         cloneTransportBoolMap(state.RuntimeLiveness.ActiveAgents),
		PendingApprovals:     cloneTransportBoolMap(state.RuntimeLiveness.PendingApprovals),
		PendingUserInputs:    cloneTransportBoolMap(state.RuntimeLiveness.PendingUserInputs),
		ActiveCommandStreams: cloneTransportBoolMap(state.RuntimeLiveness.ActiveCommandStreams),
	}
	return cloned
}

func assistantTransportCloneExperiencePackSuggestion(suggestion appui.AiopsTransportExperiencePackSuggestion) appui.AiopsTransportExperiencePackSuggestion {
	if len(suggestion.SourceRefs) > 0 {
		suggestion.SourceRefs = append([]string(nil), suggestion.SourceRefs...)
	}
	if len(suggestion.Metadata) > 0 {
		suggestion.Metadata = cloneTransportAnyMap(suggestion.Metadata)
	}
	return suggestion
}

func assistantTransportCloneTurn(turn appui.AiopsTransportTurn) appui.AiopsTransportTurn {
	cloned := turn
	if turn.User != nil {
		userCopy := *turn.User
		cloned.User = &userCopy
	}
	if turn.Intent != nil {
		intentCopy := *turn.Intent
		cloned.Intent = &intentCopy
	}
	if turn.Final != nil {
		finalCopy := *turn.Final
		cloned.Final = &finalCopy
	}
	if len(turn.Process) > 0 {
		cloned.Process = make([]appui.AiopsProcessBlock, len(turn.Process))
		for idx, block := range turn.Process {
			cloned.Process[idx] = assistantTransportCloneProcessBlock(block)
		}
	}
	if len(turn.AgentUiArtifacts) > 0 {
		cloned.AgentUiArtifacts = make([]appui.AiopsTransportAgentArtifact, len(turn.AgentUiArtifacts))
		for idx, artifact := range turn.AgentUiArtifacts {
			cloned.AgentUiArtifacts[idx] = assistantTransportCloneAgentArtifact(artifact)
		}
	}
	return cloned
}

func assistantTransportCloneAgentArtifact(artifact appui.AiopsTransportAgentArtifact) appui.AiopsTransportAgentArtifact {
	cloned := artifact
	cloned.InlineData = cloneTransportAnyMap(artifact.InlineData)
	cloned.Payload = cloneTransportAnyMap(artifact.Payload)
	cloned.Metadata = cloneTransportAnyMap(artifact.Metadata)
	if len(artifact.Actions) > 0 {
		cloned.Actions = make([]map[string]any, len(artifact.Actions))
		for idx, action := range artifact.Actions {
			cloned.Actions[idx] = cloneTransportAnyMap(action)
		}
	}
	return cloned
}

func assistantTransportCloneProcessBlock(block appui.AiopsProcessBlock) appui.AiopsProcessBlock {
	cloned := block
	if len(block.Steps) > 0 {
		cloned.Steps = append([]appui.AiopsTransportPlanStep(nil), block.Steps...)
	}
	if len(block.Queries) > 0 {
		cloned.Queries = append([]string(nil), block.Queries...)
	}
	if len(block.Results) > 0 {
		cloned.Results = append([]appui.AiopsSearchResult(nil), block.Results...)
	}
	if block.ExitCode != nil {
		exitCode := *block.ExitCode
		cloned.ExitCode = &exitCode
	}
	return cloned
}

func cloneTransportBoolMap(value map[string]bool) map[string]bool {
	if len(value) == 0 {
		return map[string]bool{}
	}
	cloned := make(map[string]bool, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
