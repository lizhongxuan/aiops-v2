package appui

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

type TransportProjector struct{}

func NewTransportProjector() *TransportProjector {
	return &TransportProjector{}
}

func (p *TransportProjector) ProjectTurnSnapshot(state AiopsTransportState, turn *runtimekernel.TurnSnapshot) (AiopsTransportState, error) {
	next := ensureAiopsTransportState(state)
	if turn == nil {
		return next, nil
	}

	turnID := strings.TrimSpace(turn.ID)
	if turnID == "" {
		turnID = TransportTurnStableID(next.ThreadID, "current")
	}
	next.CurrentTurnID = turnID
	next.TurnOrder = appendTurnOrder(next.TurnOrder, turnID)

	projectedTurn := next.Turns[turnID]
	if projectedTurn.ID == "" {
		projectedTurn.ID = turnID
	}
	projectedTurn.Process = nil
	projectedTurn.StartedAt = firstNonEmptyString(projectedTurn.StartedAt, transportTimestamp(turn.StartedAt))
	projectedTurn.UpdatedAt = firstNonEmptyString(transportTimestamp(turn.UpdatedAt), projectedTurn.UpdatedAt)
	if turn.CompletedAt != nil {
		projectedTurn.CompletedAt = transportTimestamp(*turn.CompletedAt)
	} else if turn.Lifecycle.IsTerminal() {
		projectedTurn.CompletedAt = transportTimestamp(turn.UpdatedAt)
	}

	activeApprovalIDs := map[string]bool{}
	if !turn.Lifecycle.IsTerminal() {
		activeApprovalIDs = snapshotPendingApprovalIDs(turn.PendingApprovals, turn.PendingEvidence)
	}
	pruneTransportPendingApprovalsForTurn(&next, turnID, activeApprovalIDs)
	resultPreviews := transportToolResultPreviews(turn)
	for _, item := range turn.AgentItems {
		projectedTurn = projectTurnItem(projectedTurn, &next, turnID, item, resultPreviews)
	}
	if turn.Lifecycle.IsTerminal() {
		projectedTurn.Process = normalizeTerminalProcessBlocks(projectedTurn.Process, turn.Lifecycle, turn.Error)
		pruneTransportPendingApprovalsForTurn(&next, turnID, map[string]bool{})
	} else {
		projectedTurn = projectSnapshotPendingApprovals(projectedTurn, &next, turnID, turn.PendingApprovals)
		projectedTurn = projectSnapshotPendingEvidence(projectedTurn, &next, turnID, turn.PendingEvidence)
	}
	if finalText := strings.TrimSpace(turn.FinalOutput); finalText != "" {
		finalStatus := mapTurnStatusToFinalStatus(mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, len(next.PendingApprovals) > 0))
		if projectedTurn.Final == nil {
			projectedTurn.Final = &AiopsTransportFinal{ID: TransportProcessBlockStableID(turnID, "final", "output")}
		}
		if strings.TrimSpace(projectedTurn.Final.ID) == "" {
			projectedTurn.Final.ID = TransportProcessBlockStableID(turnID, "final", "output")
		}
		projectedTurn.Final.Text = finalText
		projectedTurn.Final.Status = finalStatus
		projectedTurn = projectAssistantFinalProcessBlock(
			projectedTurn,
			turnID,
			projectedTurn.Final.ID,
			finalText,
			mapFinalStatusToTransportProcessStatus(finalStatus),
			turn.UpdatedAt,
		)
	}

	projectedTurn.Status = mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, len(next.PendingApprovals) > 0)
	if projectedTurn.Final != nil && projectedTurn.Final.Status == "" {
		projectedTurn.Final.Status = mapTurnStatusToFinalStatus(projectedTurn.Status)
	}
	next.Turns[turnID] = projectedTurn

	applyTurnLiveness(&next, turnID, projectedTurn.Status)
	if errText := firstNonEmptyString(strings.TrimSpace(turn.Error), strings.TrimSpace(next.LastError)); errText != "" && projectedTurn.Status == AiopsTransportTurnStatusFailed {
		next.LastError = errText
	}
	next.Status = mapTurnStatusToTransportStatus(projectedTurn.Status)
	next.Seq += int64(len(turn.AgentItems))
	next.UpdatedAt = firstNonEmptyString(transportTimestamp(turn.UpdatedAt), next.UpdatedAt)

	return next, nil
}

func snapshotPendingApprovalIDs(approvals []runtimekernel.PendingApproval, evidenceItems []runtimekernel.PendingEvidence) map[string]bool {
	ids := make(map[string]bool, len(approvals)+len(evidenceItems))
	for _, approval := range approvals {
		if id := strings.TrimSpace(approval.ID); id != "" {
			ids[id] = true
		}
	}
	for _, evidence := range evidenceItems {
		if id := strings.TrimSpace(evidence.ID); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func transportToolResultPreviews(turn *runtimekernel.TurnSnapshot) map[string]string {
	previews := map[string]string{}
	if turn == nil {
		return previews
	}
	for _, iteration := range turn.Iterations {
		for _, result := range iteration.ToolResults {
			toolCallID := strings.TrimSpace(result.ToolCallID)
			if toolCallID == "" || strings.TrimSpace(result.Content) == "" {
				continue
			}
			if _, exists := previews[toolCallID]; exists {
				continue
			}
			_, outputPreview, _ := summarizeToolResultForEvent(turn.ID, toolCallID, result.Content)
			if preview := jsonStringValue(outputPreview); preview != "" {
				previews[toolCallID] = preview
			}
		}
	}
	return previews
}

func pruneTransportPendingApprovalsForTurn(state *AiopsTransportState, turnID string, activeIDs map[string]bool) {
	if state == nil {
		return
	}
	for approvalID, approval := range state.PendingApprovals {
		if strings.TrimSpace(approval.TurnID) != strings.TrimSpace(turnID) {
			continue
		}
		if activeIDs[approvalID] {
			continue
		}
		delete(state.PendingApprovals, approvalID)
		delete(state.RuntimeLiveness.PendingApprovals, approvalID)
	}
}

func projectSnapshotPendingApprovals(turn AiopsTransportTurn, state *AiopsTransportState, turnID string, approvals []runtimekernel.PendingApproval) AiopsTransportTurn {
	if state == nil {
		return turn
	}
	for _, approval := range approvals {
		approvalID := strings.TrimSpace(approval.ID)
		if approvalID == "" {
			continue
		}
		status := strings.TrimSpace(approval.Status)
		if status == "" || status == "pending" {
			status = string(AiopsTransportProcessStatusBlocked)
		}
		command := strings.TrimSpace(approval.Command)
		reason := strings.TrimSpace(approval.Reason)
		approvalType := "tool"
		if command != "" || strings.EqualFold(strings.TrimSpace(approval.ToolName), "exec_command") {
			approvalType = "command"
		}
		updatedAt := firstNonEmptyString(transportTimestamp(approval.UpdatedAt), transportTimestamp(approval.CreatedAt))
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindApproval), approvalID),
			Kind:        AiopsTransportProcessKindApproval,
			DisplayKind: "approval",
			Status:      AiopsTransportProcessStatusBlocked,
			Text:        firstNonEmptyString(reason, command, "等待审批"),
			Command:     command,
			ApprovalID:  approvalID,
			UpdatedAt:   updatedAt,
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
		state.PendingApprovals[approvalID] = AiopsTransportApproval{
			ID:          approvalID,
			TurnID:      firstNonEmptyString(strings.TrimSpace(approval.TurnID), turnID),
			Type:        approvalType,
			Status:      status,
			Command:     command,
			Reason:      reason,
			RequestedAt: transportTimestamp(approval.CreatedAt),
		}
		state.RuntimeLiveness.PendingApprovals[approvalID] = true
	}
	return turn
}

func projectSnapshotPendingEvidence(turn AiopsTransportTurn, state *AiopsTransportState, turnID string, evidenceItems []runtimekernel.PendingEvidence) AiopsTransportTurn {
	if state == nil {
		return turn
	}
	for _, evidence := range evidenceItems {
		evidenceID := strings.TrimSpace(evidence.ID)
		if evidenceID == "" {
			continue
		}
		status := strings.TrimSpace(evidence.Status)
		if status == "" || status == "pending" {
			status = string(AiopsTransportProcessStatusBlocked)
		}
		command := pendingEvidenceCommand(turn.Process, evidence)
		reason := strings.TrimSpace(evidence.Reason)
		updatedAt := firstNonEmptyString(transportTimestamp(evidence.UpdatedAt), transportTimestamp(evidence.CreatedAt))
		for i := range turn.Process {
			if !pendingEvidenceMatchesProcessBlock(turn.Process[i], evidence) {
				continue
			}
			if turn.Process[i].Kind == AiopsTransportProcessKindCommand && turn.Process[i].Status == AiopsTransportProcessStatusBlocked {
				turn.Process[i].ApprovalID = evidenceID
				if strings.TrimSpace(turn.Process[i].Command) == "" {
					turn.Process[i].Command = command
				}
				if strings.TrimSpace(turn.Process[i].Text) == "" {
					turn.Process[i].Text = command
				}
			}
		}
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindApproval), evidenceID),
			Kind:        AiopsTransportProcessKindApproval,
			DisplayKind: "approval",
			Status:      AiopsTransportProcessStatusBlocked,
			Text:        firstNonEmptyString(reason, command, "等待审批"),
			Command:     command,
			ApprovalID:  evidenceID,
			UpdatedAt:   updatedAt,
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
		state.PendingApprovals[evidenceID] = AiopsTransportApproval{
			ID:          evidenceID,
			TurnID:      firstNonEmptyString(strings.TrimSpace(evidence.TurnID), turnID),
			Type:        "command",
			Status:      status,
			Command:     command,
			Reason:      reason,
			RequestedAt: transportTimestamp(evidence.CreatedAt),
		}
		state.RuntimeLiveness.PendingApprovals[evidenceID] = true
	}
	return turn
}

func projectTurnItem(
	turn AiopsTransportTurn,
	state *AiopsTransportState,
	turnID string,
	item agentstate.TurnItem,
	resultPreviews map[string]string,
) AiopsTransportTurn {
	switch item.Type {
	case agentstate.TurnItemTypeUserMessage:
		text := firstNonEmptyString(strings.TrimSpace(item.Payload.Summary), decodeUserMessageText(item.Payload.Data))
		if text != "" {
			turn.User = &AiopsTransportMessage{
				ID:        item.ID,
				Text:      text,
				CreatedAt: transportTimestamp(item.CreatedAt),
			}
		}
	case agentstate.TurnItemTypePlan:
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindPlan), item.ID),
			Kind:        AiopsTransportProcessKindPlan,
			DisplayKind: "plan",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        strings.TrimSpace(item.Payload.Summary),
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		var payload struct {
			Title string `json:"title"`
			Steps []struct {
				ID      string `json:"id"`
				Text    string `json:"text"`
				Status  string `json:"status"`
				Summary string `json:"summary"`
			} `json:"steps"`
		}
		if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &payload) == nil {
			if title := strings.TrimSpace(payload.Title); title != "" {
				block.Text = title
			}
			for _, step := range payload.Steps {
				if text := strings.TrimSpace(step.Text); text != "" {
					block.Steps = append(block.Steps, AiopsTransportPlanStep{
						ID:      strings.TrimSpace(step.ID),
						Text:    text,
						Status:  strings.TrimSpace(step.Status),
						Summary: strings.TrimSpace(step.Summary),
					})
				}
			}
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
		tool := decodeTransportToolPayload(item.Payload)
		if item.Type == agentstate.TurnItemTypeToolResult {
			if artifact, ok := transportOpsManualSearchArtifactFromToolPayload(turnID, item.ID, tool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
		}
		blockKind := detectTransportToolBlockKind(item.Payload.Kind, tool.DisplayKind, tool.ToolName)
		sourceID := firstNonEmptyString(tool.ToolCallID, normalizeTransportToolSourceID(tool.ToolName, tool.InputSummary), item.ID)

		// For search blocks, Text is a query string (not HTML) — skip sanitization.
		// For other tool blocks, sanitize both Text and OutputPreview to strip HTML/truncate.
		toolText := summarizeTransportToolText(blockKind, tool, item.Payload)
		if blockKind != AiopsTransportProcessKindSearch {
			toolText = sanitizeOutputPreview(toolText)
		}

		outputPreview := outputPreviewForTransportToolBlock(blockKind, tool)
		if outputPreview == "" && item.Type == agentstate.TurnItemTypeToolResult {
			outputPreview = resultPreviews[tool.ToolCallID]
		}
		block := AiopsProcessBlock{
			ID:            TransportProcessBlockStableID(turnID, string(blockKind), sourceID),
			Kind:          blockKind,
			DisplayKind:   displayKindForTransportToolBlock(blockKind, tool.DisplayKind, item.Payload.Kind, tool.ToolName),
			Status:        mapItemStatusToTransportProcessStatus(item.Status),
			Text:          toolText,
			InputSummary:  tool.InputSummary,
			OutputPreview: sanitizeOutputPreview(outputPreview),
			RawRef:        tool.RawRef,
			ExitCode:      tool.ExitCode,
			DurationMs:    tool.DurationMs,
			UpdatedAt:     transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		switch blockKind {
		case AiopsTransportProcessKindSearch:
			query := firstNonEmptyString(tool.InputSummary, strings.TrimSpace(item.Payload.Summary))
			if query != "" {
				block.Queries = []string{query}
			}
			block.Results = decodeTransportSearchResults(tool.OutputPreview)
		case AiopsTransportProcessKindCommand:
			block.Command = firstNonEmptyString(tool.InputSummary, tool.ToolName)
			if block.Text == "" || block.Text == tool.OutputSummary {
				block.Text = block.Command
			}
		}
		if sourceID != "" {
			if item.Status == agentstate.ItemStatusRunning {
				state.RuntimeLiveness.ActiveCommandStreams[sourceID] = true
			} else {
				delete(state.RuntimeLiveness.ActiveCommandStreams, sourceID)
			}
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeEvidence:
		var payload struct {
			ID         string `json:"id"`
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Summary    string `json:"summary"`
			Source     string `json:"source"`
			Confidence string `json:"confidence"`
			Window     string `json:"window"`
			RawRef     string `json:"rawRef"`
		}
		_ = json.Unmarshal(item.Payload.Data, &payload)
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindEvidence), firstNonEmptyString(payload.ID, item.ID)),
			Kind:        AiopsTransportProcessKindEvidence,
			DisplayKind: "evidence",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        firstNonEmptyString(strings.TrimSpace(payload.Summary), strings.TrimSpace(item.Payload.Summary), strings.TrimSpace(payload.Title)),
			Source:      strings.TrimSpace(payload.Source),
			Confidence:  strings.TrimSpace(payload.Confidence),
			Window:      strings.TrimSpace(payload.Window),
			RawRef:      strings.TrimSpace(payload.RawRef),
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeApproval:
		var payload struct {
			ApprovalID   string `json:"approvalId"`
			ApprovalType string `json:"approvalType"`
			Command      string `json:"command"`
			Reason       string `json:"reason"`
		}
		_ = json.Unmarshal(item.Payload.Data, &payload)
		approvalID := firstNonEmptyString(strings.TrimSpace(payload.ApprovalID), item.ID)
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindApproval), approvalID),
			Kind:        AiopsTransportProcessKindApproval,
			DisplayKind: "approval",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        firstNonEmptyString(strings.TrimSpace(item.Payload.Summary), strings.TrimSpace(payload.Reason), strings.TrimSpace(payload.Command)),
			Command:     strings.TrimSpace(payload.Command),
			ApprovalID:  approvalID,
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
		if item.Status == agentstate.ItemStatusBlocked {
			state.PendingApprovals[approvalID] = AiopsTransportApproval{
				ID:          approvalID,
				TurnID:      turnID,
				Type:        strings.TrimSpace(payload.ApprovalType),
				Status:      string(block.Status),
				Command:     strings.TrimSpace(payload.Command),
				Reason:      strings.TrimSpace(payload.Reason),
				RequestedAt: transportTimestamp(item.CreatedAt),
			}
			state.RuntimeLiveness.PendingApprovals[approvalID] = true
		} else {
			delete(state.PendingApprovals, approvalID)
			delete(state.RuntimeLiveness.PendingApprovals, approvalID)
		}
	case agentstate.TurnItemTypeFinalAnswer:
		text := strings.TrimSpace(item.Payload.Summary)
		turn.Final = &AiopsTransportFinal{
			ID:     item.ID,
			Text:   text,
			Status: mapItemStatusToTransportFinalStatus(item.Status),
		}
		turn = projectAssistantFinalProcessBlock(
			turn,
			turnID,
			item.ID,
			text,
			mapItemStatusToTransportProcessStatus(item.Status),
			firstNonZeroTime(item.UpdatedAt, item.CreatedAt),
		)
	case agentstate.TurnItemTypeModelCall:
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindReasoning), item.ID),
			Kind:        AiopsTransportProcessKindReasoning,
			DisplayKind: "reasoning",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        strings.TrimSpace(item.Payload.Summary),
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeError:
		state.LastError = strings.TrimSpace(item.Payload.Summary)
	}

	return turn
}

func projectAssistantFinalProcessBlock(
	turn AiopsTransportTurn,
	turnID string,
	sourceID string,
	text string,
	status AiopsTransportProcessStatus,
	updatedAt time.Time,
) AiopsTransportTurn {
	text = strings.TrimSpace(text)
	if text == "" {
		return turn
	}
	block := AiopsProcessBlock{
		ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindAssistant), firstNonEmptyString(sourceID, "output")),
		Kind:        AiopsTransportProcessKindAssistant,
		DisplayKind: "assistant.final",
		Status:      status,
		Text:        text,
		UpdatedAt:   transportTimestamp(updatedAt),
	}
	turn.Process = upsertTransportProcessBlock(turn.Process, block)
	return turn
}

func ensureAiopsTransportState(state AiopsTransportState) AiopsTransportState {
	if state.SchemaVersion == "" {
		state = NewAiopsTransportState(state.SessionID, state.ThreadID)
	}
	if state.Turns == nil {
		state.Turns = map[string]AiopsTransportTurn{}
	}
	if state.PendingApprovals == nil {
		state.PendingApprovals = map[string]AiopsTransportApproval{}
	}
	if state.McpSurfaces == nil {
		state.McpSurfaces = map[string]AiopsTransportMcpSurface{}
	}
	if state.Artifacts == nil {
		state.Artifacts = map[string]AiopsTransportArtifact{}
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
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return state
}

func appendTurnOrder(order []string, turnID string) []string {
	for _, existing := range order {
		if existing == turnID {
			return order
		}
	}
	return append(order, turnID)
}

func upsertTransportProcessBlock(blocks []AiopsProcessBlock, block AiopsProcessBlock) []AiopsProcessBlock {
	for i := range blocks {
		if blocks[i].ID == block.ID {
			blocks[i] = mergeTransportProcessBlock(blocks[i], block)
			return blocks
		}
	}
	return append(blocks, block)
}

func normalizeTerminalProcessBlocks(blocks []AiopsProcessBlock, lifecycle runtimekernel.TurnLifecycleState, errorText string) []AiopsProcessBlock {
	status := terminalProcessStatus(lifecycle, errorText)
	for i := range blocks {
		switch blocks[i].Status {
		case AiopsTransportProcessStatusBlocked, AiopsTransportProcessStatusRunning, AiopsTransportProcessStatusQueued:
			blocks[i].Status = status
		}
	}
	return blocks
}

func terminalProcessStatus(lifecycle runtimekernel.TurnLifecycleState, errorText string) AiopsTransportProcessStatus {
	if lifecycle == runtimekernel.TurnLifecycleCanceled || isApprovalDeniedError(errorText) {
		return AiopsTransportProcessStatusRejected
	}
	if lifecycle == runtimekernel.TurnLifecycleFailed {
		return AiopsTransportProcessStatusFailed
	}
	return AiopsTransportProcessStatusCompleted
}

func isApprovalDeniedError(errorText string) bool {
	normalized := strings.ToLower(strings.TrimSpace(errorText))
	return normalized == "approval denied" || normalized == "approval rejected" || normalized == "user denied approval"
}

func mergeTransportProcessBlock(existing, next AiopsProcessBlock) AiopsProcessBlock {
	if existing.Kind == AiopsTransportProcessKindCommand && next.Kind == AiopsTransportProcessKindCommand {
		if strings.TrimSpace(existing.Command) != "" {
			next.Command = existing.Command
		}
		if strings.TrimSpace(existing.InputSummary) != "" {
			next.InputSummary = existing.InputSummary
		}
		if strings.TrimSpace(next.Text) == "" || strings.TrimSpace(next.Text) == strings.TrimSpace(next.OutputPreview) {
			next.Text = firstNonEmptyString(existing.Text, next.Command, next.Text)
		}
	}
	return next
}

func mapTurnLifecycleToTransportTurnStatus(lifecycle runtimekernel.TurnLifecycleState, resume runtimekernel.TurnResumeState, blocked bool) AiopsTransportTurnStatus {
	switch lifecycle {
	case runtimekernel.TurnLifecycleCompleted:
		return AiopsTransportTurnStatusCompleted
	case runtimekernel.TurnLifecycleFailed:
		return AiopsTransportTurnStatusFailed
	case runtimekernel.TurnLifecycleCanceled:
		return AiopsTransportTurnStatusCanceled
	case runtimekernel.TurnLifecyclePending:
		return AiopsTransportTurnStatusSubmitted
	case runtimekernel.TurnLifecycleSuspended, runtimekernel.TurnLifecycleResumable:
		if blocked || resume == runtimekernel.TurnResumeStatePendingApproval || resume == runtimekernel.TurnResumeStatePendingEvidence {
			return AiopsTransportTurnStatusBlocked
		}
		return AiopsTransportTurnStatusWorking
	default:
		return AiopsTransportTurnStatusWorking
	}
}

func mapTurnStatusToTransportStatus(status AiopsTransportTurnStatus) AiopsTransportStatus {
	switch status {
	case AiopsTransportTurnStatusBlocked:
		return AiopsTransportStatusBlocked
	case AiopsTransportTurnStatusFailed:
		return AiopsTransportStatusFailed
	case AiopsTransportTurnStatusCanceled:
		return AiopsTransportStatusCanceled
	case AiopsTransportTurnStatusCompleted:
		return AiopsTransportStatusIdle
	default:
		return AiopsTransportStatusWorking
	}
}

func mapTurnStatusToFinalStatus(status AiopsTransportTurnStatus) AiopsTransportFinalStatus {
	switch status {
	case AiopsTransportTurnStatusFailed, AiopsTransportTurnStatusCanceled:
		return AiopsTransportFinalStatusFailed
	case AiopsTransportTurnStatusCompleted:
		return AiopsTransportFinalStatusCompleted
	default:
		return AiopsTransportFinalStatusRunning
	}
}

func mapFinalStatusToTransportProcessStatus(status AiopsTransportFinalStatus) AiopsTransportProcessStatus {
	switch status {
	case AiopsTransportFinalStatusCompleted:
		return AiopsTransportProcessStatusCompleted
	case AiopsTransportFinalStatusFailed:
		return AiopsTransportProcessStatusFailed
	default:
		return AiopsTransportProcessStatusRunning
	}
}

func mapItemStatusToTransportProcessStatus(status agentstate.ItemStatus) AiopsTransportProcessStatus {
	switch status {
	case agentstate.ItemStatusPending:
		return AiopsTransportProcessStatusQueued
	case agentstate.ItemStatusRunning:
		return AiopsTransportProcessStatusRunning
	case agentstate.ItemStatusCompleted:
		return AiopsTransportProcessStatusCompleted
	case agentstate.ItemStatusBlocked:
		return AiopsTransportProcessStatusBlocked
	case agentstate.ItemStatusFailed, agentstate.ItemStatusCancelled:
		return AiopsTransportProcessStatusFailed
	default:
		return AiopsTransportProcessStatusQueued
	}
}

func mapItemStatusToTransportFinalStatus(status agentstate.ItemStatus) AiopsTransportFinalStatus {
	switch status {
	case agentstate.ItemStatusCompleted:
		return AiopsTransportFinalStatusCompleted
	case agentstate.ItemStatusFailed, agentstate.ItemStatusCancelled:
		return AiopsTransportFinalStatusFailed
	default:
		return AiopsTransportFinalStatusRunning
	}
}

func applyTurnLiveness(state *AiopsTransportState, turnID string, status AiopsTransportTurnStatus) {
	if state == nil || turnID == "" {
		return
	}
	switch status {
	case AiopsTransportTurnStatusSubmitted, AiopsTransportTurnStatusWorking:
		state.RuntimeLiveness.ActiveTurns[turnID] = true
	default:
		delete(state.RuntimeLiveness.ActiveTurns, turnID)
	}
	if status != AiopsTransportTurnStatusBlocked {
		state.RuntimeLiveness.PendingApprovals = cloneBoolMap(state.RuntimeLiveness.PendingApprovals)
	}
}

func decodeUserMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Prompt string `json:"prompt"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return firstNonEmptyString(strings.TrimSpace(payload.Prompt), strings.TrimSpace(payload.Text))
}

type transportToolPayload struct {
	ID            string          `json:"id"`
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	Name          string          `json:"name"`
	DisplayKind   string          `json:"displayKind"`
	InputSummary  string          `json:"inputSummary"`
	OutputSummary string          `json:"outputSummary"`
	Arguments     json.RawMessage `json:"arguments"`
	OutputPreview json.RawMessage `json:"outputPreview"`
	RawRef        string          `json:"rawRef"`
	ExitCode      *int            `json:"exitCode"`
	DurationMs    int64           `json:"durationMs"`
	Error         string          `json:"error"`
}

func transportOpsManualSearchArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "ops_manual_search_result" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	data := tool.OutputPreview
	if len(data) == 0 {
		return AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return AiopsTransportAgentUIArtifact{}, false
	}
	decision, _ := payload["decision"].(string)
	decision = strings.TrimSpace(decision)
	if decision == "" {
		decision = "unknown"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-search:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "result"),
		Type:            "ops_manual_search_result",
		Title:           "Ops manual search result",
		TitleZh:         "运维手册检索结果",
		Summary:         decision,
		SummaryZh:       opsManualSearchSummaryZh(decision),
		Status:          decision,
		Severity:        opsManualSearchSeverity(decision),
		Source:          "tool:search_ops_manuals",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         opsManualSearchArtifactActions(decision),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func upsertTransportAgentUIArtifact(items []AiopsTransportAgentUIArtifact, artifact AiopsTransportAgentUIArtifact) []AiopsTransportAgentUIArtifact {
	if strings.TrimSpace(artifact.ID) == "" {
		return items
	}
	for idx := range items {
		if items[idx].ID == artifact.ID {
			items[idx] = artifact
			return items
		}
	}
	return append(items, artifact)
}

func opsManualSearchSummaryZh(decision string) string {
	switch strings.TrimSpace(decision) {
	case "direct_execute":
		return "已找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。"
	case "adapt":
		return "找到相似运维手册，但当前环境存在差异，需要先生成变体并校验。"
	case "reference_only":
		return "找到可参考的运维手册，不能直接运行工作流，需要按步骤确认后执行。"
	case "need_info":
		return "识别到相关运维手册，但还缺少目标实例、环境、执行面或证据。"
	case "no_match":
		return "没有找到合适的运维手册。"
	default:
		return "已完成运维手册检索判定。"
	}
}

func opsManualSearchSeverity(decision string) string {
	switch strings.TrimSpace(decision) {
	case "direct_execute":
		return "success"
	case "adapt":
		return "warning"
	case "reference_only", "need_info":
		return "info"
	default:
		return "neutral"
	}
}

func opsManualSearchArtifactActions(decision string) []map[string]any {
	switch strings.TrimSpace(decision) {
	case "direct_execute":
		return []map[string]any{
			{"id": "fill_parameters", "label": "填写参数", "kind": "panel"},
			{"id": "dry_run", "label": "Dry Run", "kind": "panel"},
		}
	case "adapt":
		return []map[string]any{
			{"id": "generate_variant", "label": "生成适配工作流", "kind": "confirm"},
			{"id": "review_gaps", "label": "查看差异", "kind": "panel"},
		}
	case "reference_only":
		return []map[string]any{{"id": "step_by_step", "label": "逐步参考", "kind": "panel"}}
	case "need_info":
		return []map[string]any{{"id": "collect_context", "label": "补充上下文", "kind": "form"}}
	default:
		return nil
	}
}

func decodeTransportToolPayload(envelope agentstate.PayloadEnvelope) transportToolPayload {
	var payload transportToolPayload
	_ = json.Unmarshal(envelope.Data, &payload)
	if payload.ToolCallID == "" {
		payload.ToolCallID = strings.TrimSpace(payload.ID)
	}
	if payload.ToolName == "" {
		payload.ToolName = strings.TrimSpace(payload.Name)
	}
	if payload.DisplayKind == "" {
		payload.DisplayKind = strings.TrimSpace(envelope.Kind)
	}
	if payload.OutputSummary == "" && payload.Error == "" {
		payload.OutputSummary = strings.TrimSpace(envelope.Summary)
	}
	if payload.InputSummary == "" || transportToolSummaryLooksGeneric(payload.ToolName, payload.InputSummary) {
		payload.InputSummary = summarizeAgentToolInput(payload.ToolName, payload.Arguments)
	}
	if isTransportSearchTool(payload) && (payload.InputSummary == "" || transportToolSummaryLooksGeneric(payload.ToolName, payload.InputSummary)) {
		payload.InputSummary = cleanProviderNativeSearchSummary(firstNonEmptyString(payload.OutputSummary, strings.TrimSpace(envelope.Summary)))
	}
	if payload.InputSummary == "" || transportToolSummaryLooksGeneric(payload.ToolName, payload.InputSummary) {
		summary := strings.TrimSpace(envelope.Summary)
		if payload.OutputSummary == "" && !transportToolSummaryLooksGeneric(payload.ToolName, summary) {
			payload.InputSummary = summary
		}
	}
	return payload
}

func detectTransportToolBlockKind(envelopeKind, displayKind, toolName string) AiopsTransportProcessKind {
	kind := strings.ToLower(firstNonEmptyString(displayKind, envelopeKind))
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case strings.Contains(kind, "browser.search"), name == "web_search":
		return AiopsTransportProcessKindSearch
	case kind == "command", name == "exec_command":
		return AiopsTransportProcessKindCommand
	case strings.HasPrefix(kind, "file"), strings.Contains(name, "file"):
		return AiopsTransportProcessKindFile
	default:
		return AiopsTransportProcessKindTool
	}
}

func isTransportSearchTool(tool transportToolPayload) bool {
	return detectTransportToolBlockKind(tool.DisplayKind, tool.DisplayKind, tool.ToolName) == AiopsTransportProcessKindSearch
}

func normalizeTransportToolSourceID(toolName, inputSummary string) string {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	inputSummary = strings.TrimSpace(inputSummary)
	if toolName == "" || inputSummary == "" {
		return ""
	}
	if toolName == "web_search" {
		return toolName + ":" + inputSummary
	}
	return ""
}

func displayKindForTransportToolBlock(blockKind AiopsTransportProcessKind, values ...string) string {
	if blockKind == AiopsTransportProcessKindSearch {
		return "web_search"
	}
	return firstNonEmptyString(values...)
}

func summarizeTransportToolText(blockKind AiopsTransportProcessKind, tool transportToolPayload, envelope agentstate.PayloadEnvelope) string {
	if blockKind == AiopsTransportProcessKindSearch {
		query := firstNonEmptyString(tool.InputSummary, strings.TrimSpace(envelope.Summary))
		if strings.EqualFold(strings.TrimSpace(tool.OutputSummary), query) {
			return query
		}
		output := cleanProviderNativeSearchSummary(tool.OutputSummary)
		return firstNonEmptyString(output, query, tool.ToolName)
	}
	if blockKind == AiopsTransportProcessKindCommand {
		return firstNonEmptyString(tool.InputSummary, tool.ToolName)
	}
	return firstNonEmptyString(tool.OutputSummary, tool.InputSummary, strings.TrimSpace(envelope.Summary), tool.ToolName)
}

func outputPreviewForTransportToolBlock(blockKind AiopsTransportProcessKind, tool transportToolPayload) string {
	if blockKind == AiopsTransportProcessKindSearch {
		return cleanProviderNativeSearchSummary(firstNonEmptyString(tool.OutputSummary, tool.Error))
	}
	return firstNonEmptyString(jsonStringValue(tool.OutputPreview), tool.Error)
}

func jsonStringValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(string(raw))
}

func cleanProviderNativeSearchSummary(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if query := extractProviderNativeSearchQuery(text); query != "" {
		return query
	}
	var payload struct {
		Content string `json:"content"`
		Query   string `json:"query"`
	}
	if json.Unmarshal([]byte(text), &payload) == nil {
		if query := strings.TrimSpace(payload.Query); query != "" {
			return query
		}
		if query := extractProviderNativeSearchQuery(strings.TrimSpace(payload.Content)); query != "" {
			return query
		}
		if strings.TrimSpace(payload.Content) != "" {
			text = strings.TrimSpace(payload.Content)
		}
	}
	text = strings.TrimPrefix(text, "provider-native web_search completed for query ")
	text = strings.TrimSpace(text)
	if strings.Contains(text, "; provider returned no textual summary") {
		text = strings.TrimSpace(strings.Split(text, "; provider returned no textual summary")[0])
	}
	text = strings.Trim(text, "\"'")
	return text
}

func extractProviderNativeSearchQuery(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\"`, `"`))
	if value == "" {
		return ""
	}
	match := providerNativeSearchQueryPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func transportToolSummaryLooksGeneric(toolName, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if value == "" {
		return true
	}
	switch value {
	case toolName, "web_search", "search_web", "browse_url", "exec_command":
		return true
	}
	return false
}

func pendingEvidenceCommand(blocks []AiopsProcessBlock, evidence runtimekernel.PendingEvidence) string {
	for _, block := range blocks {
		if !pendingEvidenceMatchesProcessBlock(block, evidence) {
			continue
		}
		if block.Kind == AiopsTransportProcessKindCommand {
			return firstNonEmptyString(strings.TrimSpace(block.Command), strings.TrimSpace(block.InputSummary), strings.TrimSpace(block.Text))
		}
	}
	return firstNonEmptyString(strings.TrimSpace(evidence.ToolName), strings.TrimSpace(evidence.Reason))
}

func pendingEvidenceMatchesProcessBlock(block AiopsProcessBlock, evidence runtimekernel.PendingEvidence) bool {
	toolCallID := strings.TrimSpace(evidence.ToolCallID)
	if toolCallID == "" {
		return block.Kind == AiopsTransportProcessKindCommand && block.Status == AiopsTransportProcessStatusBlocked
	}
	return strings.Contains(block.ID, toolCallID) || strings.Contains(block.RawRef, toolCallID)
}

// sanitizeOutputPreview sanitizes content for display as an output preview.
// HTML content is stripped of tags and truncated to 200 runes with "…" appended.
// Non-HTML content is truncated to 500 runes with "…" appended.
func sanitizeOutputPreview(content string) string {
	if isHTMLContent(content) {
		stripped := stripHTMLTags(content)
		runes := []rune(stripped)
		if len(runes) > 200 {
			return string(runes[:200]) + "…"
		}
		return stripped
	}
	runes := []rune(content)
	if len(runes) > 500 {
		return string(runes[:500]) + "…"
	}
	return content
}

// isHTMLContent detects whether a string contains HTML content by checking
// if it starts with a DOCTYPE declaration or html tags (case variations),
// supporting leading whitespace.
func isHTMLContent(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "<!DOCTYPE") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<HTML")
}

// htmlTagPattern matches any HTML tag (opening, closing, or self-closing).
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// multiSpacePattern matches two or more consecutive whitespace characters.
var multiSpacePattern = regexp.MustCompile(`\s{2,}`)

var providerNativeSearchQueryPattern = regexp.MustCompile(`provider-native web_search completed for query\s+"([^"]+)"`)

// stripHTMLTags removes all HTML tags from the input string, preserves text
// content between tags, collapses multiple whitespace into a single space,
// and trims leading/trailing whitespace.
func stripHTMLTags(s string) string {
	// Remove all HTML tags
	text := htmlTagPattern.ReplaceAllString(s, " ")
	// Collapse multiple whitespace into a single space
	text = multiSpacePattern.ReplaceAllString(text, " ")
	// Trim leading/trailing whitespace
	return strings.TrimSpace(text)
}

func decodeTransportSearchResults(raw json.RawMessage) []AiopsSearchResult {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Results []AiopsSearchResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload.Results
}

func transportTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339Nano)
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
