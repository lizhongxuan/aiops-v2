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

const (
	assistantTransportPollInitialInterval = 10 * time.Millisecond
	assistantTransportPollMaxInterval     = 250 * time.Millisecond
)

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
	state.SchemaVersion = appui.AiopsTransportSchemaVersion
	if strings.TrimSpace(state.ThreadID) == "" {
		state.ThreadID = strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.SessionID))
	}
	if strings.TrimSpace(state.SessionID) == "" {
		state.SessionID = strings.TrimSpace(req.State.SessionID)
	}
	if state.Turns == nil {
		state.Turns = map[string]appui.AiopsTransportTurn{}
	}
	for turnID, turn := range state.Turns {
		state.Turns[turnID] = appui.EnsureAiopsTransportTurnBlocks(turn)
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
	current := state
	lastFingerprint := ""
	pollInterval := assistantTransportPollInitialInterval

	for {
		session := source.Get(current.SessionID)
		waitingForAcceptedApproval := false
		changed := false
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
				changed = true
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
		pollInterval = assistantTransportNextPollInterval(pollInterval, changed)
		timer := time.NewTimer(pollInterval)

		select {
		case <-ctx.Done():
			timer.Stop()
			if !waitingForAcceptedApproval && shouldCancelAssistantTransportOnContextDone(current, session) {
				_ = cancelAssistantTransportTurn(context.Background(), chat, current, session)
			}
			return current, ctx.Err()
		case <-timer.C:
		}
	}
}

func assistantTransportNextPollInterval(current time.Duration, changed bool) time.Duration {
	if changed || current <= 0 {
		return assistantTransportPollInitialInterval
	}
	next := current * 2
	if next > assistantTransportPollMaxInterval {
		return assistantTransportPollMaxInterval
	}
	return next
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
	if !reflect.DeepEqual(prev.RuntimeLiveness, next.RuntimeLiveness) {
		appendSet([]any{"runtimeLiveness"}, next.RuntimeLiveness)
	}

	seenTurns := map[string]struct{}{}
	for _, turnID := range next.TurnOrder {
		seenTurns[turnID] = struct{}{}
		prevTurn, prevExists := prev.Turns[turnID]
		ops = append(ops, assistantTransportDiffTurnOps(turnID, prevTurn, next.Turns[turnID], prevExists)...)
	}
	for turnID := range next.Turns {
		if _, ok := seenTurns[turnID]; ok {
			continue
		}
		prevTurn, prevExists := prev.Turns[turnID]
		ops = append(ops, assistantTransportDiffTurnOps(turnID, prevTurn, next.Turns[turnID], prevExists)...)
	}

	return ops
}

func assistantTransportDiffTurnOps(turnID string, prevTurn, nextTurn appui.AiopsTransportTurn, prevExists bool) []assistantTransportStreamStateOp {
	appendSet := func(ops *[]assistantTransportStreamStateOp, path []any, value any) {
		*ops = append(*ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpSet,
			Path:  path,
			Value: value,
		})
	}

	ops := []assistantTransportStreamStateOp{}
	nextTurn = appui.EnsureAiopsTransportTurnBlocks(nextTurn)
	if !prevExists {
		appendSet(&ops, []any{"turns", turnID}, nextTurn)
		return ops
	}
	prevTurn = appui.EnsureAiopsTransportTurnBlocks(prevTurn)

	if prevTurn.ID != nextTurn.ID {
		appendSet(&ops, []any{"turns", turnID, "id"}, nextTurn.ID)
	}
	if !reflect.DeepEqual(prevTurn.User, nextTurn.User) {
		appendSet(&ops, []any{"turns", turnID, "user"}, nextTurn.User)
	}
	if !reflect.DeepEqual(prevTurn.Intent, nextTurn.Intent) {
		appendSet(&ops, []any{"turns", turnID, "intent"}, nextTurn.Intent)
	}
	if prevTurn.Status != nextTurn.Status {
		appendSet(&ops, []any{"turns", turnID, "status"}, nextTurn.Status)
	}
	if prevTurn.StartedAt != nextTurn.StartedAt {
		appendSet(&ops, []any{"turns", turnID, "startedAt"}, nextTurn.StartedAt)
	}
	if prevTurn.CompletedAt != nextTurn.CompletedAt {
		appendSet(&ops, []any{"turns", turnID, "completedAt"}, nextTurn.CompletedAt)
	}
	if prevTurn.UpdatedAt != nextTurn.UpdatedAt {
		appendSet(&ops, []any{"turns", turnID, "updatedAt"}, nextTurn.UpdatedAt)
	}
	if !reflect.DeepEqual(prevTurn.BlockOrder, nextTurn.BlockOrder) {
		appendSet(&ops, []any{"turns", turnID, "blockOrder"}, nextTurn.BlockOrder)
	}

	seenBlocks := map[string]struct{}{}
	for _, blockID := range nextTurn.BlockOrder {
		seenBlocks[blockID] = struct{}{}
		prevBlock, prevBlockExists := prevTurn.BlocksByID[blockID]
		ops = append(ops, assistantTransportDiffBlockOps(turnID, blockID, prevBlock, nextTurn.BlocksByID[blockID], prevBlockExists)...)
	}
	for blockID, nextBlock := range nextTurn.BlocksByID {
		if _, ok := seenBlocks[blockID]; ok {
			continue
		}
		prevBlock, prevBlockExists := prevTurn.BlocksByID[blockID]
		ops = append(ops, assistantTransportDiffBlockOps(turnID, blockID, prevBlock, nextBlock, prevBlockExists)...)
	}

	return ops
}

func assistantTransportDiffBlockOps(turnID, blockID string, prevBlock, nextBlock appui.AiopsTranscriptBlock, prevExists bool) []assistantTransportStreamStateOp {
	appendSet := func(ops *[]assistantTransportStreamStateOp, path []any, value any) {
		*ops = append(*ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpSet,
			Path:  path,
			Value: value,
		})
	}
	appendText := func(ops *[]assistantTransportStreamStateOp, path []any, value string) {
		if value == "" {
			return
		}
		*ops = append(*ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpAppendText,
			Path:  path,
			Value: value,
		})
	}

	ops := []assistantTransportStreamStateOp{}
	blockPath := []any{"turns", turnID, "blocksById", blockID}
	if !prevExists {
		appendSet(&ops, blockPath, nextBlock)
		return ops
	}

	nextBlockForSet := assistantTransportBlockForMetadataSet(prevBlock, nextBlock)
	if !reflect.DeepEqual(prevBlock, nextBlockForSet) {
		appendSet(&ops, blockPath, nextBlockForSet)
	}

	if prevBlock.Text != nil && nextBlock.Text != nil {
		assistantTransportAppendTextDiff(&ops, append(blockPath, "text", "text"), prevBlock.Text.Text, nextBlock.Text.Text, appendSet, appendText)
	}
	if prevBlock.Tool != nil && nextBlock.Tool != nil {
		assistantTransportAppendTextDiff(&ops, append(blockPath, "tool", "output", "stdout"), prevBlock.Tool.Output.Stdout, nextBlock.Tool.Output.Stdout, appendSet, appendText)
		assistantTransportAppendTextDiff(&ops, append(blockPath, "tool", "output", "stderr"), prevBlock.Tool.Output.Stderr, nextBlock.Tool.Output.Stderr, appendSet, appendText)
		assistantTransportAppendTextDiff(&ops, append(blockPath, "tool", "output", "text"), prevBlock.Tool.Output.Text, nextBlock.Tool.Output.Text, appendSet, appendText)
	}
	if prevBlock.Thinking != nil && nextBlock.Thinking != nil {
		assistantTransportAppendTextDiff(&ops, append(blockPath, "thinking", "text"), prevBlock.Thinking.Text, nextBlock.Thinking.Text, appendSet, appendText)
	}

	return ops
}

func assistantTransportBlockForMetadataSet(prevBlock, nextBlock appui.AiopsTranscriptBlock) appui.AiopsTranscriptBlock {
	block := nextBlock
	if prevBlock.Text != nil && block.Text != nil {
		textCopy := *block.Text
		textCopy.Text = prevBlock.Text.Text
		block.Text = &textCopy
	}
	if prevBlock.Tool != nil && block.Tool != nil {
		toolCopy := *block.Tool
		outputCopy := toolCopy.Output
		outputCopy.Stdout = prevBlock.Tool.Output.Stdout
		outputCopy.Stderr = prevBlock.Tool.Output.Stderr
		outputCopy.Text = prevBlock.Tool.Output.Text
		toolCopy.Output = outputCopy
		block.Tool = &toolCopy
	}
	if prevBlock.Thinking != nil && block.Thinking != nil {
		thinkingCopy := *block.Thinking
		thinkingCopy.Text = prevBlock.Thinking.Text
		block.Thinking = &thinkingCopy
	}
	return block
}

func assistantTransportAppendTextDiff(
	ops *[]assistantTransportStreamStateOp,
	path []any,
	prevText string,
	nextText string,
	appendSet func(*[]assistantTransportStreamStateOp, []any, any),
	appendText func(*[]assistantTransportStreamStateOp, []any, string),
) {
	if prevText == nextText {
		return
	}
	if strings.HasPrefix(nextText, prevText) {
		appendText(ops, path, nextText[len(prevText):])
		return
	}
	appendSet(ops, path, "")
	appendText(ops, path, nextText)
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
		cloned.McpSurfaces[key] = assistantTransportCloneMcpSurface(surface)
	}
	cloned.Artifacts = make(map[string]appui.AiopsTransportArtifact, len(state.Artifacts))
	for key, artifact := range state.Artifacts {
		cloned.Artifacts[key] = assistantTransportCloneArtifact(artifact)
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

func assistantTransportCloneMcpSurface(surface appui.AiopsTransportMcpSurface) appui.AiopsTransportMcpSurface {
	cloned := surface
	if len(surface.Cards) > 0 {
		cloned.Cards = make([]appui.AiopsAgentUICard, len(surface.Cards))
		for idx, card := range surface.Cards {
			cloned.Cards[idx] = assistantTransportCloneAgentUICard(card)
		}
	}
	if surface.App != nil {
		appCopy := *surface.App
		if len(surface.App.Permissions) > 0 {
			appCopy.Permissions = append([]string(nil), surface.App.Permissions...)
		}
		cloned.App = &appCopy
	}
	if len(surface.Actions) > 0 {
		cloned.Actions = assistantTransportCloneActionBindings(surface.Actions)
	}
	if len(surface.ArtifactIDs) > 0 {
		cloned.ArtifactIDs = append([]string(nil), surface.ArtifactIDs...)
	}
	return cloned
}

func assistantTransportCloneArtifact(artifact appui.AiopsTransportArtifact) appui.AiopsTransportArtifact {
	cloned := artifact
	if artifact.PreviewData != nil {
		previewCopy := *artifact.PreviewData
		previewCopy.Metadata = cloneTransportMetadata(artifact.PreviewData.Metadata)
		cloned.PreviewData = &previewCopy
	}
	if len(artifact.Actions) > 0 {
		cloned.Actions = assistantTransportCloneActionBindings(artifact.Actions)
	}
	return cloned
}

func assistantTransportCloneAgentUICard(card appui.AiopsAgentUICard) appui.AiopsAgentUICard {
	cloned := card
	if len(card.Actions) > 0 {
		cloned.Actions = assistantTransportCloneActionBindings(card.Actions)
	}
	return cloned
}

func assistantTransportCloneActionBindings(actions []appui.AiopsTransportActionBinding) []appui.AiopsTransportActionBinding {
	cloned := make([]appui.AiopsTransportActionBinding, len(actions))
	for idx, action := range actions {
		cloned[idx] = action
		cloned[idx].Params = cloneTransportAnyMap(action.Params)
	}
	return cloned
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
	if len(turn.BlockOrder) > 0 {
		cloned.BlockOrder = append([]string(nil), turn.BlockOrder...)
	} else if turn.BlockOrder != nil {
		cloned.BlockOrder = []string{}
	}
	cloned.BlocksByID = make(map[string]appui.AiopsTranscriptBlock, len(turn.BlocksByID))
	for blockID, block := range turn.BlocksByID {
		cloned.BlocksByID[blockID] = assistantTransportCloneTranscriptBlock(block)
	}
	return cloned
}

func assistantTransportCloneTranscriptBlock(block appui.AiopsTranscriptBlock) appui.AiopsTranscriptBlock {
	cloned := block
	if block.Text != nil {
		textCopy := *block.Text
		cloned.Text = &textCopy
	}
	if block.Tool != nil {
		toolCopy := *block.Tool
		if block.Tool.ExitCode != nil {
			exitCode := *block.Tool.ExitCode
			toolCopy.ExitCode = &exitCode
		}
		cloned.Tool = &toolCopy
	}
	if block.Aggregate != nil {
		aggregateCopy := *block.Aggregate
		if len(block.Aggregate.ChildBlockIDs) > 0 {
			aggregateCopy.ChildBlockIDs = append([]string(nil), block.Aggregate.ChildBlockIDs...)
		}
		cloned.Aggregate = &aggregateCopy
	}
	if block.Approval != nil {
		approvalCopy := *block.Approval
		cloned.Approval = &approvalCopy
	}
	if block.Thinking != nil {
		thinkingCopy := *block.Thinking
		cloned.Thinking = &thinkingCopy
	}
	if block.Artifact != nil {
		artifactCopy := *block.Artifact
		cloned.Artifact = &artifactCopy
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
