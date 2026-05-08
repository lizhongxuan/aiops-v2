package appui

import (
	"encoding/json"
	"fmt"
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

	projectedTurn := removeAggregateBlocks(EnsureAiopsTransportTurnBlocks(next.Turns[turnID]))
	if projectedTurn.ID == "" {
		projectedTurn.ID = turnID
	}
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

	hasFinalAnswerItem := false
	for _, item := range turn.AgentItems {
		if item.Type == agentstate.TurnItemTypeFinalAnswer {
			hasFinalAnswerItem = true
		}
		projectedTurn = projectTurnItem(projectedTurn, &next, turnID, item)
	}

	if turn.Lifecycle.IsTerminal() {
		projectedTurn = normalizeTerminalTranscriptBlocks(projectedTurn, turn.Lifecycle, turn.Error)
		pruneTransportPendingApprovalsForTurn(&next, turnID, map[string]bool{})
	} else {
		projectedTurn = projectSnapshotPendingApprovals(projectedTurn, &next, turnID, turn.PendingApprovals)
		projectedTurn = projectSnapshotPendingEvidence(projectedTurn, &next, turnID, turn.PendingEvidence)
	}

	if finalText := strings.TrimSpace(turn.FinalOutput); finalText != "" && !hasFinalAnswerItem {
		projectedTurn = projectSyntheticFinalTextBlock(projectedTurn, turnID, finalText, turn.UpdatedAt, turn.Lifecycle.IsTerminal())
	}
	projectedTurn.Status = mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, len(next.PendingApprovals) > 0)
	if len(projectedTurn.BlockOrder) == 0 && !turn.Lifecycle.IsTerminal() {
		projectedTurn = projectThinkingBlock(projectedTurn, turnID, "thinking", transportTimestamp(turn.UpdatedAt))
	}
	projectedTurn = aggregateTranscriptBlocks(projectedTurn)

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
		requestedAt := transportTimestamp(approval.CreatedAt)
		updatedAt := firstNonEmptyString(transportTimestamp(approval.UpdatedAt), requestedAt)
		turn = upsertApprovalTranscriptBlock(turn, turnID, approvalID, approvalType, firstNonEmptyString(reason, command, "等待审批"), command, status, requestedAt, "", updatedAt)
		state.PendingApprovals[approvalID] = AiopsTransportApproval{
			ID:          approvalID,
			TurnID:      firstNonEmptyString(strings.TrimSpace(approval.TurnID), turnID),
			Type:        approvalType,
			Status:      status,
			Command:     command,
			Reason:      reason,
			RequestedAt: requestedAt,
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
		command := firstNonEmptyString(findCommandForToolCall(turn, evidence.ToolCallID), strings.TrimSpace(evidence.ToolName), strings.TrimSpace(evidence.Reason))
		reason := strings.TrimSpace(evidence.Reason)
		requestedAt := transportTimestamp(evidence.CreatedAt)
		updatedAt := firstNonEmptyString(transportTimestamp(evidence.UpdatedAt), requestedAt)
		turn = attachApprovalToMatchingTool(turn, evidence.ToolCallID, evidenceID, status, updatedAt)
		turn = upsertApprovalTranscriptBlock(turn, turnID, evidenceID, "command", firstNonEmptyString(reason, command, "等待审批"), command, status, requestedAt, "", updatedAt)
		state.PendingApprovals[evidenceID] = AiopsTransportApproval{
			ID:          evidenceID,
			TurnID:      firstNonEmptyString(strings.TrimSpace(evidence.TurnID), turnID),
			Type:        "command",
			Status:      status,
			Command:     command,
			Reason:      reason,
			RequestedAt: requestedAt,
		}
		state.RuntimeLiveness.PendingApprovals[evidenceID] = true
	}
	return turn
}

func projectTurnItem(turn AiopsTransportTurn, state *AiopsTransportState, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
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
		turn = projectPlanTextBlock(turn, turnID, item)
	case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
		turn = projectToolTranscriptBlock(turn, state, turnID, item)
	case agentstate.TurnItemTypeEvidence:
		turn = projectEvidenceTranscriptBlock(turn, turnID, item)
	case agentstate.TurnItemTypeApproval:
		turn = projectApprovalItemBlock(turn, state, turnID, item)
	case agentstate.TurnItemTypeFinalAnswer:
		status := AiopsTranscriptTextStatusStreaming
		if item.Status == agentstate.ItemStatusCompleted {
			status = AiopsTranscriptTextStatusCompleted
		}
		turn = projectTextBlock(turn, turnID, item, strings.TrimSpace(item.Payload.Summary), status)
	case agentstate.TurnItemTypeModelCall:
		text := strings.TrimSpace(item.Payload.Summary)
		if isGenericModelSummary(text) {
			if item.Status == agentstate.ItemStatusRunning {
				turn = projectThinkingBlock(turn, turnID, item.ID, transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)))
			}
			return turn
		}
		status := AiopsTranscriptTextStatusStreaming
		if item.Status == agentstate.ItemStatusCompleted {
			status = AiopsTranscriptTextStatusCompleted
		}
		turn = projectTextBlock(turn, turnID, item, text, status)
	case agentstate.TurnItemTypeError:
		if state != nil {
			state.LastError = strings.TrimSpace(item.Payload.Summary)
		}
		turn = projectErrorTextBlock(turn, turnID, item)
	}
	return turn
}

func projectPlanTextBlock(turn AiopsTransportTurn, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
	text := strings.TrimSpace(item.Payload.Summary)
	var payload struct {
		Title string `json:"title"`
		Steps []struct {
			Text    string `json:"text"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
		} `json:"steps"`
	}
	if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &payload) == nil {
		text = firstNonEmptyString(strings.TrimSpace(payload.Title), text)
		if len(payload.Steps) > 0 {
			lines := []string{text}
			for _, step := range payload.Steps {
				if stepText := strings.TrimSpace(firstNonEmptyString(step.Text, step.Summary)); stepText != "" {
					lines = append(lines, stepText)
				}
			}
			text = strings.Join(nonEmptyStrings(lines...), "\n")
		}
	}
	return projectTextBlock(turn, turnID, item, text, AiopsTranscriptTextStatusCompleted)
}

func projectTextBlock(turn AiopsTransportTurn, turnID string, item agentstate.TurnItem, text string, status AiopsTranscriptTextStatus) AiopsTransportTurn {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return turn
	}
	blockID := TransportProcessBlockStableID(turnID, "text", item.ID)
	block := AiopsTranscriptBlock{
		ID:   blockID,
		Type: AiopsTranscriptBlockTypeText,
		Text: &AiopsTextBlock{
			Role:   "assistant",
			Text:   trimmed,
			Status: status,
		},
		CreatedAt: transportTimestamp(item.CreatedAt),
		UpdatedAt: transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
	}
	return UpsertAiopsTranscriptBlock(turn, block)
}

func projectSyntheticFinalTextBlock(turn AiopsTransportTurn, turnID, text string, updatedAt time.Time, terminal bool) AiopsTransportTurn {
	status := AiopsTranscriptTextStatusStreaming
	if terminal {
		status = AiopsTranscriptTextStatusCompleted
	}
	item := agentstate.TurnItem{
		ID:        "final-output",
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
	}
	return projectTextBlock(turn, turnID, item, text, status)
}

func projectErrorTextBlock(turn AiopsTransportTurn, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
	return projectTextBlock(turn, turnID, item, strings.TrimSpace(item.Payload.Summary), AiopsTranscriptTextStatusCompleted)
}

func projectThinkingBlock(turn AiopsTransportTurn, turnID, sourceID, updatedAt string) AiopsTransportTurn {
	blockID := TransportProcessBlockStableID(turnID, "thinking", sourceID)
	return UpsertAiopsTranscriptBlock(turn, AiopsTranscriptBlock{
		ID:   blockID,
		Type: AiopsTranscriptBlockTypeThinking,
		Thinking: &AiopsThinkingBlock{
			Text:   "正在思考",
			Status: string(AiopsTransportProcessStatusRunning),
		},
		UpdatedAt: updatedAt,
	})
}

func projectToolTranscriptBlock(turn AiopsTransportTurn, state *AiopsTransportState, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
	tool := decodeTransportToolPayload(item.Payload)
	toolKind := detectTranscriptToolKind(item.Payload.Kind, tool.DisplayKind, tool.ToolName)
	sourceID := firstNonEmptyString(tool.ToolCallID, normalizeTransportToolSourceID(tool.ToolName, tool.InputSummary), item.ID)
	blockID := TransportProcessBlockStableID(turnID, "tool", sourceID)
	status := mapItemStatusToTransportProcessStatus(item.Status)
	outputText := outputTextForTranscriptTool(toolKind, tool)
	command := commandForTranscriptTool(toolKind, tool)
	summary := summaryForTranscriptTool(toolKind, status, command, tool)
	block := AiopsTranscriptBlock{
		ID:   blockID,
		Type: AiopsTranscriptBlockTypeTool,
		Tool: &AiopsToolBlock{
			ToolKind:     toolKind,
			ToolName:     tool.ToolName,
			Title:        titleForTranscriptTool(toolKind),
			Summary:      summary,
			Status:       status,
			Command:      command,
			InputSummary: tool.InputSummary,
			Output: AiopsToolOutput{
				Stdout:    stdoutForTranscriptTool(toolKind, outputText),
				Stderr:    stderrForTranscriptTool(tool, outputText),
				Text:      outputText,
				Truncated: outputWasTruncated(outputText, tool),
				RawRef:    strings.TrimSpace(tool.RawRef),
			},
			ExitCode:    tool.ExitCode,
			DurationMs:  tool.DurationMs,
			StartedAt:   transportTimestamp(item.CreatedAt),
			CompletedAt: completedAtForToolItem(item),
		},
		CreatedAt: transportTimestamp(item.CreatedAt),
		UpdatedAt: transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
	}
	if sourceID != "" && state != nil {
		if item.Status == agentstate.ItemStatusRunning {
			state.RuntimeLiveness.ActiveCommandStreams[sourceID] = true
		} else {
			delete(state.RuntimeLiveness.ActiveCommandStreams, sourceID)
		}
	}
	return UpsertAiopsTranscriptBlock(turn, block)
}

func projectEvidenceTranscriptBlock(turn AiopsTransportTurn, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
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
	text := firstNonEmptyString(strings.TrimSpace(payload.Summary), strings.TrimSpace(item.Payload.Summary), strings.TrimSpace(payload.Title))
	if text == "" {
		return turn
	}
	blockID := TransportProcessBlockStableID(turnID, "tool", firstNonEmptyString(payload.ID, item.ID))
	details := []string{}
	for _, value := range []string{payload.Source, payload.Confidence, payload.Window, payload.RawRef} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			details = append(details, trimmed)
		}
	}
	output := strings.Join(details, "\n")
	block := AiopsTranscriptBlock{
		ID:   blockID,
		Type: AiopsTranscriptBlockTypeTool,
		Tool: &AiopsToolBlock{
			ToolKind:     AiopsTranscriptToolKindOther,
			ToolName:     "evidence",
			Title:        firstNonEmptyString(payload.Title, "Evidence"),
			Summary:      text,
			Status:       mapItemStatusToTransportProcessStatus(item.Status),
			InputSummary: text,
			Output:       AiopsToolOutput{Text: output},
		},
		CreatedAt: transportTimestamp(item.CreatedAt),
		UpdatedAt: transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
	}
	return UpsertAiopsTranscriptBlock(turn, block)
}

func projectApprovalItemBlock(turn AiopsTransportTurn, state *AiopsTransportState, turnID string, item agentstate.TurnItem) AiopsTransportTurn {
	var payload struct {
		ApprovalID   string `json:"approvalId"`
		ApprovalType string `json:"approvalType"`
		Command      string `json:"command"`
		Reason       string `json:"reason"`
	}
	_ = json.Unmarshal(item.Payload.Data, &payload)
	approvalID := firstNonEmptyString(strings.TrimSpace(payload.ApprovalID), item.ID)
	status := string(mapItemStatusToTransportProcessStatus(item.Status))
	if item.Status == agentstate.ItemStatusBlocked {
		status = string(AiopsTransportProcessStatusBlocked)
	}
	command := strings.TrimSpace(payload.Command)
	reason := strings.TrimSpace(payload.Reason)
	requestedAt := transportTimestamp(item.CreatedAt)
	updatedAt := transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt))
	turn = upsertApprovalTranscriptBlock(turn, turnID, approvalID, strings.TrimSpace(payload.ApprovalType), firstNonEmptyString(strings.TrimSpace(item.Payload.Summary), reason, command, "等待审批"), command, status, requestedAt, resolvedAtForApprovalItem(item), updatedAt)
	if state != nil {
		if item.Status == agentstate.ItemStatusBlocked {
			state.PendingApprovals[approvalID] = AiopsTransportApproval{
				ID:          approvalID,
				TurnID:      turnID,
				Type:        strings.TrimSpace(payload.ApprovalType),
				Status:      status,
				Command:     command,
				Reason:      reason,
				RequestedAt: requestedAt,
			}
			state.RuntimeLiveness.PendingApprovals[approvalID] = true
		} else {
			delete(state.PendingApprovals, approvalID)
			delete(state.RuntimeLiveness.PendingApprovals, approvalID)
		}
	}
	return turn
}

func upsertApprovalTranscriptBlock(turn AiopsTransportTurn, turnID, approvalID, approvalKind, summary, command, status, requestedAt, resolvedAt, updatedAt string) AiopsTransportTurn {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return turn
	}
	if approvalKind == "" {
		approvalKind = "tool"
	}
	if status == "" {
		status = string(AiopsTransportProcessStatusBlocked)
	}
	block := AiopsTranscriptBlock{
		ID:   TransportProcessBlockStableID(turnID, "approval", approvalID),
		Type: AiopsTranscriptBlockTypeApproval,
		Approval: &AiopsApprovalBlock{
			ApprovalID:   approvalID,
			ApprovalKind: approvalKind,
			Title:        "等待审批",
			Summary:      firstNonEmptyString(summary, command, "等待审批"),
			Command:      command,
			Status:       status,
			RequestedAt:  requestedAt,
			ResolvedAt:   resolvedAt,
		},
		CreatedAt: requestedAt,
		UpdatedAt: updatedAt,
	}
	return UpsertAiopsTranscriptBlock(turn, block)
}

func ensureAiopsTransportState(state AiopsTransportState) AiopsTransportState {
	if state.SchemaVersion == "" {
		state = NewAiopsTransportState(state.SessionID, state.ThreadID)
	}
	state.SchemaVersion = AiopsTransportSchemaVersion
	if state.Turns == nil {
		state.Turns = map[string]AiopsTransportTurn{}
	}
	if state.TurnOrder == nil {
		state.TurnOrder = []string{}
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
	for id, turn := range state.Turns {
		state.Turns[id] = EnsureAiopsTransportTurnBlocks(turn)
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

func normalizeTerminalTranscriptBlocks(turn AiopsTransportTurn, lifecycle runtimekernel.TurnLifecycleState, errorText string) AiopsTransportTurn {
	status := terminalProcessStatus(lifecycle, errorText)
	turn = EnsureAiopsTransportTurnBlocks(turn)
	for id, block := range turn.BlocksByID {
		switch block.Type {
		case AiopsTranscriptBlockTypeTool:
			if block.Tool == nil {
				continue
			}
			switch block.Tool.Status {
			case AiopsTransportProcessStatusBlocked, AiopsTransportProcessStatusRunning, AiopsTransportProcessStatusQueued:
				nextTool := *block.Tool
				nextTool.Status = status
				block.Tool = &nextTool
				block.UpdatedAt = firstNonEmptyString(block.UpdatedAt, time.Now().UTC().Format(time.RFC3339Nano))
				turn.BlocksByID[id] = block
			}
		case AiopsTranscriptBlockTypeApproval:
			if block.Approval != nil && status != AiopsTransportProcessStatusCompleted {
				nextApproval := *block.Approval
				nextApproval.Status = string(status)
				block.Approval = &nextApproval
				turn.BlocksByID[id] = block
			}
		case AiopsTranscriptBlockTypeThinking:
			if block.Thinking != nil {
				delete(turn.BlocksByID, id)
				turn.BlockOrder = removeString(turn.BlockOrder, id)
			}
		}
	}
	return turn
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

func detectTranscriptToolKind(envelopeKind, displayKind, toolName string) AiopsTranscriptToolKind {
	kind := strings.ToLower(firstNonEmptyString(displayKind, envelopeKind))
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case strings.Contains(kind, "browser.search"), name == "web_search":
		return AiopsTranscriptToolKindSearch
	case kind == "command", name == "exec_command":
		return AiopsTranscriptToolKindCommand
	case strings.Contains(kind, "list"):
		return AiopsTranscriptToolKindList
	case strings.HasPrefix(kind, "file"), strings.Contains(name, "file"):
		return AiopsTranscriptToolKindFile
	case strings.Contains(kind, "mcp"):
		return AiopsTranscriptToolKindMCP
	case strings.Contains(kind, "browser"):
		return AiopsTranscriptToolKindBrowser
	default:
		return AiopsTranscriptToolKindOther
	}
}

func isTransportSearchTool(tool transportToolPayload) bool {
	return detectTranscriptToolKind(tool.DisplayKind, tool.DisplayKind, tool.ToolName) == AiopsTranscriptToolKindSearch
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

func titleForTranscriptTool(kind AiopsTranscriptToolKind) string {
	switch kind {
	case AiopsTranscriptToolKindCommand:
		return "Shell"
	case AiopsTranscriptToolKindSearch:
		return "Search"
	case AiopsTranscriptToolKindFile:
		return "File"
	case AiopsTranscriptToolKindList:
		return "List"
	case AiopsTranscriptToolKindMCP:
		return "MCP"
	case AiopsTranscriptToolKindBrowser:
		return "Browser"
	default:
		return "Tool"
	}
}

func commandForTranscriptTool(kind AiopsTranscriptToolKind, tool transportToolPayload) string {
	if kind != AiopsTranscriptToolKindCommand {
		return ""
	}
	return firstNonEmptyString(strings.TrimSpace(tool.InputSummary), strings.TrimSpace(tool.ToolName))
}

func summaryForTranscriptTool(kind AiopsTranscriptToolKind, status AiopsTransportProcessStatus, command string, tool transportToolPayload) string {
	switch kind {
	case AiopsTranscriptToolKindCommand:
		switch status {
		case AiopsTransportProcessStatusRunning, AiopsTransportProcessStatusQueued:
			return "正在运行 " + firstNonEmptyString(command, strings.TrimSpace(tool.ToolName), "命令")
		case AiopsTransportProcessStatusFailed:
			return "运行失败 " + firstNonEmptyString(command, strings.TrimSpace(tool.ToolName), "命令")
		default:
			return "已运行 " + firstNonEmptyString(command, strings.TrimSpace(tool.ToolName), "命令")
		}
	case AiopsTranscriptToolKindSearch:
		if status == AiopsTransportProcessStatusRunning || status == AiopsTransportProcessStatusQueued {
			return "正在搜索 " + firstNonEmptyString(tool.InputSummary, "网页")
		}
		return "已搜索 1 次"
	case AiopsTranscriptToolKindFile:
		if isFileEditSummary(tool.OutputSummary, tool.InputSummary) {
			return "已编辑 1 个文件"
		}
		return "已探索 1 个文件"
	case AiopsTranscriptToolKindList:
		return "已列出 1 个列表"
	case AiopsTranscriptToolKindMCP:
		return "已调用 MCP 工具"
	case AiopsTranscriptToolKindBrowser:
		return "已浏览 1 次"
	default:
		return firstNonEmptyString(strings.TrimSpace(tool.OutputSummary), strings.TrimSpace(tool.InputSummary), strings.TrimSpace(tool.ToolName), "已处理 1 个操作")
	}
}

func outputTextForTranscriptTool(kind AiopsTranscriptToolKind, tool transportToolPayload) string {
	if kind == AiopsTranscriptToolKindSearch {
		if lines := searchResultLines(tool.OutputPreview); len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
		return cleanProviderNativeSearchSummary(firstNonEmptyString(tool.OutputSummary, tool.Error))
	}
	return sanitizeOutputPreview(firstNonEmptyString(jsonStringValue(tool.OutputPreview), tool.OutputSummary, tool.Error))
}

func stdoutForTranscriptTool(kind AiopsTranscriptToolKind, output string) string {
	if kind == AiopsTranscriptToolKindCommand {
		return output
	}
	return ""
}

func stderrForTranscriptTool(tool transportToolPayload, output string) string {
	if strings.TrimSpace(tool.Error) == "" {
		return ""
	}
	return output
}

func outputWasTruncated(output string, tool transportToolPayload) bool {
	raw := firstNonEmptyString(jsonStringValue(tool.OutputPreview), tool.OutputSummary, tool.Error)
	return len([]rune(raw)) > len([]rune(output))
}

func completedAtForToolItem(item agentstate.TurnItem) string {
	switch item.Status {
	case agentstate.ItemStatusCompleted, agentstate.ItemStatusFailed, agentstate.ItemStatusCancelled:
		return transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt))
	default:
		return ""
	}
}

func resolvedAtForApprovalItem(item agentstate.TurnItem) string {
	if item.Status == agentstate.ItemStatusBlocked || item.Status == agentstate.ItemStatusPending || item.Status == agentstate.ItemStatusRunning {
		return ""
	}
	return transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt))
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

func searchResultLines(raw json.RawMessage) []string {
	results := decodeTransportSearchResults(raw)
	if len(results) == 0 {
		return nil
	}
	lines := make([]string, 0, len(results))
	for _, result := range results {
		line := strings.TrimSpace(strings.Join(nonEmptyStrings(result.Title, result.URL, result.Snippet), " "))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
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

func aggregateTranscriptBlocks(turn AiopsTransportTurn) AiopsTransportTurn {
	turn = removeAggregateBlocks(EnsureAiopsTransportTurnBlocks(turn))
	nextOrder := make([]string, 0, len(turn.BlockOrder))
	flushRun := func(run []string) {
		if len(run) == 0 {
			return
		}
		if len(run) == 1 {
			nextOrder = append(nextOrder, run[0])
			return
		}
		blocks := make([]AiopsTranscriptBlock, 0, len(run))
		for _, id := range run {
			blocks = append(blocks, turn.BlocksByID[id])
		}
		summary, counts := aggregateSummaryForBlocks(blocks)
		aggregateID := TransportProcessBlockStableID(turn.ID, "aggregate", strings.Join(run, "-"))
		turn.BlocksByID[aggregateID] = AiopsTranscriptBlock{
			ID:   aggregateID,
			Type: AiopsTranscriptBlockTypeAggregate,
			Aggregate: &AiopsAggregateBlock{
				Summary:       summary,
				Status:        string(AiopsTransportProcessStatusCompleted),
				ChildBlockIDs: append([]string(nil), run...),
				Counts:        counts,
			},
			UpdatedAt: latestBlockUpdatedAt(blocks),
		}
		nextOrder = append(nextOrder, aggregateID)
	}

	run := []string{}
	for _, id := range turn.BlockOrder {
		block, ok := turn.BlocksByID[id]
		if !ok {
			continue
		}
		if isAggregatableToolBlock(block) {
			run = append(run, id)
			continue
		}
		flushRun(run)
		run = nil
		nextOrder = append(nextOrder, id)
	}
	flushRun(run)
	turn.BlockOrder = nextOrder
	return turn
}

func removeAggregateBlocks(turn AiopsTransportTurn) AiopsTransportTurn {
	turn = EnsureAiopsTransportTurnBlocks(turn)
	nextOrder := make([]string, 0, len(turn.BlockOrder))
	for _, id := range turn.BlockOrder {
		block := turn.BlocksByID[id]
		if block.Type == AiopsTranscriptBlockTypeAggregate && block.Aggregate != nil {
			for _, childID := range block.Aggregate.ChildBlockIDs {
				if _, ok := turn.BlocksByID[childID]; ok {
					nextOrder = append(nextOrder, childID)
				}
			}
			delete(turn.BlocksByID, id)
			continue
		}
		nextOrder = append(nextOrder, id)
	}
	turn.BlockOrder = nextOrder
	return turn
}

func isAggregatableToolBlock(block AiopsTranscriptBlock) bool {
	if block.Type != AiopsTranscriptBlockTypeTool || block.Tool == nil {
		return false
	}
	if block.Tool.Status != AiopsTransportProcessStatusCompleted {
		return false
	}
	return block.Tool.ToolKind != AiopsTranscriptToolKindCommand || len([]rune(block.Tool.Output.Text)) < 200
}

func aggregateSummaryForBlocks(blocks []AiopsTranscriptBlock) (string, AiopsAggregateCounts) {
	counts := AiopsAggregateCounts{}
	for _, block := range blocks {
		if block.Tool == nil {
			continue
		}
		switch block.Tool.ToolKind {
		case AiopsTranscriptToolKindCommand:
			counts.Command++
		case AiopsTranscriptToolKindSearch:
			counts.Search++
		case AiopsTranscriptToolKindList:
			counts.List++
		case AiopsTranscriptToolKindFile:
			if isFileEditSummary(block.Tool.Summary, block.Tool.InputSummary) {
				counts.FileEdit++
			} else {
				counts.FileRead++
			}
		case AiopsTranscriptToolKindMCP:
			counts.MCP++
		case AiopsTranscriptToolKindBrowser:
			counts.Browser++
		default:
			counts.Other++
		}
	}
	parts := []string{}
	if counts.FileRead > 0 {
		parts = append(parts, fmt.Sprintf("已探索 %d 个文件", counts.FileRead))
	}
	if counts.FileEdit > 0 {
		parts = append(parts, fmt.Sprintf("已编辑 %d 个文件", counts.FileEdit))
	}
	if counts.Search > 0 {
		parts = append(parts, fmt.Sprintf("%d 次搜索", counts.Search))
	}
	if counts.List > 0 {
		parts = append(parts, fmt.Sprintf("%d 个列表", counts.List))
	}
	if counts.Command > 0 {
		parts = append(parts, fmt.Sprintf("已运行 %d 条命令", counts.Command))
	}
	if counts.Browser > 0 {
		parts = append(parts, fmt.Sprintf("已浏览 %d 次", counts.Browser))
	}
	if counts.MCP > 0 {
		parts = append(parts, fmt.Sprintf("已调用 %d 个 MCP 工具", counts.MCP))
	}
	if counts.Other > 0 {
		parts = append(parts, fmt.Sprintf("已处理 %d 个操作", counts.Other))
	}
	return strings.Join(parts, ","), counts
}

func isFileEditSummary(values ...string) bool {
	combined := strings.ToLower(strings.Join(values, " "))
	return strings.Contains(combined, "edit") || strings.Contains(combined, "write") || strings.Contains(combined, "修改") || strings.Contains(combined, "编辑")
}

func attachApprovalToMatchingTool(turn AiopsTransportTurn, toolCallID, approvalID, status, updatedAt string) AiopsTransportTurn {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return turn
	}
	for id, block := range turn.BlocksByID {
		if block.Type != AiopsTranscriptBlockTypeTool || block.Tool == nil {
			continue
		}
		if !strings.Contains(id, toolCallID) {
			continue
		}
		nextTool := *block.Tool
		nextTool.ApprovalID = approvalID
		nextTool.Status = AiopsTransportProcessStatus(status)
		block.Tool = &nextTool
		block.UpdatedAt = updatedAt
		turn.BlocksByID[id] = block
	}
	return turn
}

func findCommandForToolCall(turn AiopsTransportTurn, toolCallID string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	for id, block := range turn.BlocksByID {
		if toolCallID != "" && !strings.Contains(id, toolCallID) {
			continue
		}
		if block.Type == AiopsTranscriptBlockTypeTool && block.Tool != nil && block.Tool.ToolKind == AiopsTranscriptToolKindCommand {
			return firstNonEmptyString(block.Tool.Command, block.Tool.InputSummary, block.Tool.Summary)
		}
	}
	return ""
}

func latestBlockUpdatedAt(blocks []AiopsTranscriptBlock) string {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].UpdatedAt != "" {
			return blocks[i].UpdatedAt
		}
	}
	return ""
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

func isGenericModelSummary(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return normalized == "" || normalized == "model response received" || normalized == "calling model"
}

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

func isHTMLContent(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "<!DOCTYPE") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<HTML")
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
var multiSpacePattern = regexp.MustCompile(`\s{2,}`)
var providerNativeSearchQueryPattern = regexp.MustCompile(`provider-native web_search completed for query\s+"([^"]+)"`)

func stripHTMLTags(s string) string {
	text := htmlTagPattern.ReplaceAllString(s, " ")
	text = multiSpacePattern.ReplaceAllString(text, " ")
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

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func removeString(values []string, needle string) []string {
	out := values[:0]
	for _, value := range values {
		if value != needle {
			out = append(out, value)
		}
	}
	return out
}
