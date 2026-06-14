package appui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
	projectedTurn.ContextGovernance = projectContextGovernanceEvents(turn.ContextGovernanceEvents)
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
	resultPayloads := transportToolResultJSONPayloads(turn)
	for _, item := range turn.AgentItems {
		projectedTurn = projectTurnItem(projectedTurn, &next, turnID, item, resultPreviews, resultPayloads)
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
		if artifact, ok := transportRCAArtifactFromFinalPayload(turnID, projectedTurn.Final.ID, finalText); ok {
			projectedTurn.AgentUIArtifacts = upsertTransportAgentUIArtifact(projectedTurn.AgentUIArtifacts, artifact)
		}
	}

	next = projectHostOpsMissionFromTurn(next, turnID, projectedTurn, turn)
	hostOpsBlocked := hostOpsProjectionBlocked(next, turnID)

	projectedTurn.Status = mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, len(next.PendingApprovals) > 0 || hostOpsBlocked)
	if hostOpsBlocked && projectedTurn.Status == AiopsTransportTurnStatusWorking {
		projectedTurn.Status = AiopsTransportTurnStatusBlocked
	}
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

func transportToolResultJSONPayloads(turn *runtimekernel.TurnSnapshot) map[string]json.RawMessage {
	payloads := map[string]json.RawMessage{}
	if turn == nil {
		return payloads
	}
	for _, iteration := range turn.Iterations {
		for _, result := range iteration.ToolResults {
			toolCallID := strings.TrimSpace(result.ToolCallID)
			content := strings.TrimSpace(result.Content)
			if toolCallID == "" || content == "" {
				continue
			}
			if _, exists := payloads[toolCallID]; exists {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(content), &obj); err != nil || len(obj) == 0 {
				continue
			}
			payloads[toolCallID] = append(json.RawMessage(nil), []byte(content)...)
		}
	}
	return payloads
}

func projectContextGovernanceEvents(events []runtimekernel.ContextGovernanceEvent) []AiopsContextGovernanceEvent {
	if len(events) == 0 {
		return nil
	}
	sorted := runtimekernel.SortContextGovernanceEvents(events)
	out := make([]AiopsContextGovernanceEvent, 0, len(sorted))
	seen := map[string]struct{}{}
	for _, event := range sorted {
		if event.Layer == "" || strings.TrimSpace(event.Kind) == "" {
			continue
		}
		key := strings.TrimSpace(event.ID)
		if key == "" {
			key = fmt.Sprintf("%s:%s:%s", event.Layer, event.Kind, event.CreatedAt.UTC().Format(time.RFC3339Nano))
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, AiopsContextGovernanceEvent{
			ID:              strings.TrimSpace(event.ID),
			Layer:           string(event.Layer),
			Kind:            strings.TrimSpace(event.Kind),
			Message:         strings.TrimSpace(event.Message),
			Budget:          projectContextBudget(event.Budget),
			ReferenceIDs:    cleanTransportStringList(event.ReferenceIDs),
			CompactedIDs:    cleanTransportStringList(event.CompactedIDs),
			DroppedGroupIDs: cleanTransportStringList(event.DroppedGroupIDs),
			RetryAttempt:    event.RetryAttempt,
			RetryMax:        event.RetryMax,
			Timeout:         event.Timeout,
			CreatedAt:       transportTimestamp(event.CreatedAt),
		})
	}
	return out
}

func projectContextBudget(budget runtimekernel.ContextBudgetThresholds) map[string]any {
	if budget.MaxContextTokens == 0 &&
		budget.ReservedOutputTokens == 0 &&
		budget.EffectiveContextWindow == 0 &&
		budget.WarningThreshold == 0 &&
		budget.AutoCompactThreshold == 0 &&
		budget.BlockingLimit == 0 {
		return nil
	}
	return map[string]any{
		"maxContextTokens":       budget.MaxContextTokens,
		"reservedOutputTokens":   budget.ReservedOutputTokens,
		"effectiveContextWindow": budget.EffectiveContextWindow,
		"warningThreshold":       budget.WarningThreshold,
		"autoCompactThreshold":   budget.AutoCompactThreshold,
		"blockingLimit":          budget.BlockingLimit,
		"smallContextMode":       budget.SmallContextMode,
	}
}

func projectExternalReferences(refs []runtimekernel.ExternalReference) []AiopsExternalReference {
	if len(refs) == 0 {
		return nil
	}
	out := make([]AiopsExternalReference, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, AiopsExternalReference{
			ID:          id,
			Kind:        strings.TrimSpace(ref.Kind),
			URI:         strings.TrimSpace(ref.URI),
			CardRef:     strings.TrimSpace(ref.CardRef),
			FilePath:    strings.TrimSpace(ref.FilePath),
			Title:       strings.TrimSpace(ref.Title),
			Summary:     strings.TrimSpace(ref.Summary),
			ContentType: strings.TrimSpace(ref.ContentType),
			Digest:      strings.TrimSpace(ref.Digest),
			Bytes:       ref.Bytes,
		})
	}
	return out
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
	resultPayloads map[string]json.RawMessage,
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
				ID               string   `json:"id"`
				Index            int      `json:"index"`
				Text             string   `json:"text"`
				Title            string   `json:"title"`
				Status           string   `json:"status"`
				Summary          string   `json:"summary"`
				Risk             string   `json:"risk"`
				HostIDs          []string `json:"hostIds"`
				ChildAgentIDs    []string `json:"childAgentIds"`
				ApprovalRequired bool     `json:"approvalRequired"`
			} `json:"steps"`
		}
		if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &payload) == nil {
			if title := strings.TrimSpace(payload.Title); title != "" {
				block.Text = title
			}
			for _, step := range payload.Steps {
				text := firstNonEmptyString(strings.TrimSpace(step.Text), strings.TrimSpace(step.Title))
				if text != "" {
					block.Steps = append(block.Steps, AiopsTransportPlanStep{
						ID:               strings.TrimSpace(step.ID),
						Index:            step.Index,
						Text:             text,
						Title:            strings.TrimSpace(step.Title),
						Status:           strings.TrimSpace(step.Status),
						Summary:          strings.TrimSpace(step.Summary),
						Risk:             strings.TrimSpace(step.Risk),
						HostIDs:          cleanTransportStringList(step.HostIDs),
						ChildAgentIDs:    cleanTransportStringList(step.ChildAgentIDs),
						ApprovalRequired: step.ApprovalRequired,
					})
				}
			}
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
		tool := decodeTransportToolPayload(item.Payload)
		if item.Type == agentstate.TurnItemTypeToolResult {
			artifactTool := tool
			if len(artifactTool.OutputPreview) == 0 {
				if raw := resultPayloads[strings.TrimSpace(artifactTool.ToolCallID)]; len(raw) > 0 {
					artifactTool.OutputPreview = raw
				}
			}
			if artifact, ok := transportOpsManualSearchArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportOpsManualPreflightArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportOpsManualParamResolutionArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportCorootServiceMetricsArtifactFromToolPayload(turnID, item.ID, artifactTool, transportTurnUserText(turn)); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportGenericAgentUIArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportRunnerWorkflowGenerationArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			projectHostOpsToolPayload(state, turnID, tool, firstNonZeroTime(item.UpdatedAt, item.CreatedAt))
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
		if shouldSuppressOpsManualSearchProcessBlock(tool, outputPreview) {
			return turn
		}
		toolText, outputPreview = compactOpsManualSearchProcessText(tool.DisplayKind, toolText, outputPreview)
		block := AiopsProcessBlock{
			ID:                  TransportProcessBlockStableID(turnID, string(blockKind), sourceID),
			Kind:                blockKind,
			DisplayKind:         displayKindForTransportToolBlock(blockKind, tool.DisplayKind, item.Payload.Kind, tool.ToolName),
			Status:              mapItemStatusToTransportProcessStatus(item.Status),
			Text:                toolText,
			Source:              strings.TrimSpace(tool.ToolName),
			InputSummary:        tool.InputSummary,
			OutputPreview:       sanitizeOutputPreview(outputPreview),
			RawRef:              tool.RawRef,
			EvidenceRefs:        cleanTransportStringList(tool.EvidenceRefs),
			Mock:                tool.Mock,
			ExitCode:            tool.ExitCode,
			DurationMs:          tool.DurationMs,
			MaterializationTier: strings.TrimSpace(tool.MaterializationTier),
			OriginalBytes:       tool.OriginalBytes,
			InlineBytes:         tool.InlineBytes,
			ExternalReferences:  projectExternalReferences(tool.ExternalReferences),
			UpdatedAt:           transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
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
		case AiopsTransportProcessKindSubagent:
			if block.Text == "" {
				block.Text = firstNonEmptyString(tool.OutputSummary, tool.InputSummary, "host subagent update")
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

func projectHostOpsMissionFromTurn(state AiopsTransportState, turnID string, projectedTurn AiopsTransportTurn, snapshot *runtimekernel.TurnSnapshot) AiopsTransportState {
	if snapshot == nil || !isHostOpsTurnMetadata(snapshot.Metadata) {
		return state
	}
	if state.HostMissions == nil {
		state.HostMissions = map[string]AiopsTransportHostMission{}
	}
	if state.ChildAgents == nil {
		state.ChildAgents = map[string]AiopsTransportChildAgent{}
	}
	missionID := strings.TrimSpace(snapshot.Metadata["aiops.hostops.missionId"])
	if missionID == "" {
		missionID = "hostops:" + turnID
	}
	now := transportTimestamp(firstNonZeroTime(snapshot.UpdatedAt, snapshot.StartedAt))
	mission := state.HostMissions[missionID]
	if mission.ID == "" {
		mission.ID = missionID
		mission.CreatedAt = transportTimestamp(snapshot.StartedAt)
	}
	mission.TurnID = turnID
	mission.ManagerAgentID = firstNonEmptyString(strings.TrimSpace(snapshot.Metadata["aiops.hostops.managerAgentId"]), mission.ManagerAgentID)
	mission.PlanRequired = metadataBool(snapshot.Metadata, "aiops.hostops.planRequired") || mission.PlanRequired
	mission.PlanAccepted = metadataBool(snapshot.Metadata, "aiops.hostops.planAccepted") || mission.PlanAccepted
	if mission.Status == "" {
		if mission.PlanRequired && !mission.PlanAccepted {
			mission.Status = "waiting_plan_acceptance"
		} else {
			mission.Status = "planning"
		}
	}
	if mentions := decodeTransportHostMentionsMetadata(snapshot.Metadata["aiops.hostops.mentions"]); len(mentions) > 0 {
		mission.MentionedHosts = mentions
	}
	if steps := latestHostOpsPlanSteps(projectedTurn.Process); len(steps) > 0 {
		mission.PlanSteps = steps
	}
	for childID, child := range state.ChildAgents {
		if child.MissionID == missionID && !transportStringSliceContains(mission.ChildAgentIDs, childID) {
			mission.ChildAgentIDs = append(mission.ChildAgentIDs, childID)
		}
	}
	if hostOpsMissionBlocked(mission, state.ChildAgents) {
		mission.Status = "waiting_approval"
	} else if len(mission.ChildAgentIDs) > 0 && mission.Status != "completed" && mission.Status != "failed" && mission.Status != "cancelled" {
		mission.Status = "running"
	}
	mission.UpdatedAt = now
	state.HostMissions[missionID] = mission
	state.ActiveHostMissionID = missionID
	return state
}

func hostOpsProjectionBlocked(state AiopsTransportState, turnID string) bool {
	for _, mission := range state.HostMissions {
		if strings.TrimSpace(mission.TurnID) != strings.TrimSpace(turnID) {
			continue
		}
		if hostOpsMissionBlocked(mission, state.ChildAgents) {
			return true
		}
	}
	return false
}

func hostOpsMissionBlocked(mission AiopsTransportHostMission, children map[string]AiopsTransportChildAgent) bool {
	switch strings.ToLower(strings.TrimSpace(mission.Status)) {
	case "waiting_approval", "approval_required", "blocked":
		return true
	}
	for _, step := range mission.PlanSteps {
		if step.ApprovalRequired && strings.EqualFold(strings.TrimSpace(step.Status), "blocked") {
			return true
		}
	}
	for _, childID := range mission.ChildAgentIDs {
		child, ok := children[childID]
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(child.Status)) {
		case "approval_required", "blocked":
			return true
		}
	}
	return false
}

func isHostOpsTurnMetadata(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.TrimSpace(metadata["aiops.hostops.routeKind"]) == "host_ops" {
		return true
	}
	return strings.TrimSpace(metadata["aiops.hostops.mentions"]) != ""
}

func metadataBool(metadata map[string]string, key string) bool {
	switch strings.ToLower(strings.TrimSpace(metadata[key])) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}

func decodeTransportHostMentionsMetadata(raw string) []AiopsTransportHostMention {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var mentions []AiopsTransportHostMention
	if err := json.Unmarshal([]byte(raw), &mentions); err != nil {
		return nil
	}
	out := make([]AiopsTransportHostMention, 0, len(mentions))
	for _, mention := range mentions {
		if strings.TrimSpace(mention.Raw) == "" && strings.TrimSpace(mention.HostID) == "" && strings.TrimSpace(mention.Address) == "" {
			continue
		}
		out = append(out, AiopsTransportHostMention{
			TokenID:     strings.TrimSpace(mention.TokenID),
			Raw:         strings.TrimSpace(mention.Raw),
			HostID:      strings.TrimSpace(mention.HostID),
			Address:     strings.TrimSpace(mention.Address),
			DisplayName: strings.TrimSpace(mention.DisplayName),
			Source:      strings.TrimSpace(mention.Source),
			Resolved:    mention.Resolved,
		})
	}
	return out
}

func latestHostOpsPlanSteps(blocks []AiopsProcessBlock) []AiopsTransportPlanStep {
	for i := len(blocks) - 1; i >= 0; i-- {
		block := blocks[i]
		if block.Kind == AiopsTransportProcessKindPlan && len(block.Steps) > 0 {
			return append([]AiopsTransportPlanStep(nil), block.Steps...)
		}
	}
	return nil
}

func projectHostOpsToolPayload(state *AiopsTransportState, turnID string, tool transportToolPayload, updatedAt time.Time) {
	if state == nil || !isHostOpsToolPayload(tool) {
		return
	}
	payload := tool.OutputPreview
	if len(payload) == 0 {
		payload = tool.DisplayData
	}
	if len(payload) == 0 {
		return
	}
	var decoded struct {
		Children []AiopsTransportChildAgent `json:"children"`
		Child    AiopsTransportChildAgent   `json:"child"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return
	}
	children := decoded.Children
	if strings.TrimSpace(decoded.Child.ID) != "" {
		children = append(children, decoded.Child)
	}
	if len(children) == 0 {
		return
	}
	if state.ChildAgents == nil {
		state.ChildAgents = map[string]AiopsTransportChildAgent{}
	}
	if state.HostMissions == nil {
		state.HostMissions = map[string]AiopsTransportHostMission{}
	}
	for _, child := range children {
		child = normalizeProjectedHostChild(child, turnID, updatedAt)
		if child.ID == "" {
			continue
		}
		state.ChildAgents[child.ID] = child
		projectHostChildLiveness(state, child)
		if child.MissionID != "" {
			mission := state.HostMissions[child.MissionID]
			if mission.ID == "" {
				mission.ID = child.MissionID
				mission.TurnID = turnID
				mission.CreatedAt = child.StartedAt
			}
			if !transportStringSliceContains(mission.ChildAgentIDs, child.ID) {
				mission.ChildAgentIDs = append(mission.ChildAgentIDs, child.ID)
			}
			mission.ActiveChildAgentID = child.ID
			mission.UpdatedAt = firstNonEmptyString(child.UpdatedAt, transportTimestamp(updatedAt))
			state.HostMissions[child.MissionID] = mission
		}
	}
}

func isHostOpsToolPayload(tool transportToolPayload) bool {
	displayKind := strings.ToLower(strings.TrimSpace(tool.DisplayKind))
	toolName := strings.ToLower(strings.TrimSpace(tool.ToolName))
	return strings.HasPrefix(displayKind, "hostops.") ||
		toolName == "spawn_host_agent" ||
		toolName == "send_host_agent_message" ||
		toolName == "wait_host_agents" ||
		toolName == "stop_host_agent"
}

func normalizeProjectedHostChild(child AiopsTransportChildAgent, turnID string, updatedAt time.Time) AiopsTransportChildAgent {
	child.ID = strings.TrimSpace(child.ID)
	child.MissionID = strings.TrimSpace(child.MissionID)
	if child.MissionID == "" {
		child.MissionID = "hostops:" + turnID
	}
	child.ParentAgentID = strings.TrimSpace(child.ParentAgentID)
	child.SessionID = strings.TrimSpace(child.SessionID)
	child.HostID = strings.TrimSpace(child.HostID)
	child.HostAddress = strings.TrimSpace(child.HostAddress)
	child.HostDisplayName = firstNonEmptyString(strings.TrimSpace(child.HostDisplayName), child.HostAddress, child.HostID)
	child.Role = strings.TrimSpace(child.Role)
	child.Task = strings.TrimSpace(child.Task)
	child.CurrentStepTitle = strings.TrimSpace(child.CurrentStepTitle)
	child.Status = strings.TrimSpace(child.Status)
	if child.Status == "" {
		child.Status = "running"
	}
	child.PlanStepIDs = cleanTransportStringList(child.PlanStepIDs)
	child.LastInputPreview = strings.TrimSpace(child.LastInputPreview)
	child.LastOutputPreview = strings.TrimSpace(child.LastOutputPreview)
	child.Error = strings.TrimSpace(child.Error)
	if child.StartedAt == "" {
		child.StartedAt = transportTimestamp(updatedAt)
	}
	child.UpdatedAt = firstNonEmptyString(strings.TrimSpace(child.UpdatedAt), transportTimestamp(updatedAt))
	return child
}

func projectHostChildLiveness(state *AiopsTransportState, child AiopsTransportChildAgent) {
	if state == nil || strings.TrimSpace(child.ID) == "" {
		return
	}
	if state.RuntimeLiveness.ActiveAgents == nil {
		state.RuntimeLiveness.ActiveAgents = map[string]bool{}
	}
	switch strings.ToLower(strings.TrimSpace(child.Status)) {
	case "completed", "failed", "cancelled", "canceled":
		delete(state.RuntimeLiveness.ActiveAgents, child.ID)
	default:
		state.RuntimeLiveness.ActiveAgents[child.ID] = true
	}
}

func transportStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	ID                  string                            `json:"id"`
	ToolCallID          string                            `json:"toolCallId"`
	ToolName            string                            `json:"toolName"`
	Name                string                            `json:"name"`
	DisplayKind         string                            `json:"displayKind"`
	InputSummary        string                            `json:"inputSummary"`
	OutputSummary       string                            `json:"outputSummary"`
	Arguments           json.RawMessage                   `json:"arguments"`
	OutputPreview       json.RawMessage                   `json:"outputPreview"`
	DisplayData         json.RawMessage                   `json:"displayData"`
	RawRef              string                            `json:"rawRef"`
	EvidenceRefs        []string                          `json:"evidenceRefs"`
	MaterializationTier string                            `json:"materializationTier"`
	OriginalBytes       int64                             `json:"originalBytes"`
	InlineBytes         int64                             `json:"inlineBytes"`
	ExternalReferences  []runtimekernel.ExternalReference `json:"externalReferences"`
	Mock                bool                              `json:"mock"`
	ExitCode            *int                              `json:"exitCode"`
	DurationMs          int64                             `json:"durationMs"`
	Error               string                            `json:"error"`
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
	if !isActionableOpsManualSearchPayload(payload) {
		return AiopsTransportAgentUIArtifact{}, false
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

func isOpsManualNoMatchDecision(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "no_match":
		return true
	default:
		return false
	}
}

func isActionableOpsManualSearchPayload(payload map[string]any) bool {
	decision := strings.TrimSpace(jsonStringValueFromMap(payload, "decision"))
	if isOpsManualNoMatchDecision(decision) {
		return false
	}
	return opsManualSearchPayloadHasManual(payload)
}

func opsManualSearchPayloadHasManual(payload map[string]any) bool {
	for _, key := range []string{"manuals", "hits", "matches"} {
		if values, ok := payload[key].([]any); ok && len(values) > 0 {
			return true
		}
	}
	if manual, ok := payload["manual"]; ok && manual != nil {
		return true
	}
	if manualID := strings.TrimSpace(jsonStringValueFromMap(payload, "manual_id")); manualID != "" {
		return true
	}
	return false
}

func transportOpsManualPreflightArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "ops_manual_preflight_result" {
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
	status := strings.TrimSpace(firstNonEmptyString(jsonStringValueFromMap(payload, "status"), jsonStringValueFromMap(payload, "preflight_status"), "unknown"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-preflight:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "result"),
		Type:            "ops_manual_preflight_result",
		Title:           "Ops manual preflight result",
		TitleZh:         "运维手册预检结果",
		Summary:         status,
		SummaryZh:       opsManualPreflightSummaryZh(status),
		Status:          status,
		Severity:        opsManualPreflightSeverity(status),
		Source:          "tool:run_ops_manual_preflight",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         opsManualPreflightArtifactActions(status, jsonStringValueFromMap(payload, "next_action")),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func transportOpsManualParamResolutionArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "ops_manual_param_resolution" && strings.TrimSpace(tool.DisplayKind) != "ops_manual_param_form" {
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
	status := strings.TrimSpace(firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "unknown"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-param-resolution:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "result"),
		Type:            strings.TrimSpace(tool.DisplayKind),
		Title:           "Ops manual parameter resolution",
		TitleZh:         "运维手册参数解析",
		Summary:         status,
		SummaryZh:       opsManualParamResolutionSummaryZh(status),
		Status:          status,
		Severity:        opsManualParamResolutionSeverity(status),
		Source:          "tool:resolve_ops_manual_params",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         opsManualParamResolutionArtifactActions(status),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func transportGenericAgentUIArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "rca_report" {
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
	if strings.TrimSpace(jsonStringValueFromMap(payload, "type")) != "rca_report" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata := asStringAnyMap(payload["metadata"])
	if metadata == nil {
		metadata = map[string]any{}
	}
	for _, key := range []string{"caseId", "evidenceRef", "promptTraceId"} {
		if value := jsonStringValueFromMap(payload, key); value != "" {
			metadata[key] = value
		}
	}
	return AiopsTransportAgentUIArtifact{
		ID:              "agent-ui:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "artifact"),
		Type:            "rca_report",
		Title:           jsonStringValueFromMap(payload, "title"),
		TitleZh:         firstNonEmptyString(jsonStringValueFromMap(payload, "titleZh"), "根因分析"),
		Summary:         jsonStringValueFromMap(payload, "summary"),
		SummaryZh:       jsonStringValueFromMap(payload, "summaryZh"),
		Status:          firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "ok"),
		Severity:        firstNonEmptyString(jsonStringValueFromMap(payload, "severity"), "info"),
		Source:          firstNonEmptyString(jsonStringValueFromMap(payload, "source"), "aiops"),
		PermissionScope: firstNonEmptyString(jsonStringValueFromMap(payload, "permissionScope"), "read"),
		RedactionStatus: firstNonEmptyString(jsonStringValueFromMap(payload, "redactionStatus"), "redacted"),
		InlineData:      asStringAnyMap(payload["inlineData"]),
		Metadata:        metadata,
		Actions:         asStringAnyMapList(payload["actions"]),
		CreatedAt:       firstNonEmptyString(jsonStringValueFromMap(payload, "createdAt"), now),
		UpdatedAt:       firstNonEmptyString(jsonStringValueFromMap(payload, "updatedAt"), now),
	}, true
}

func transportRCAArtifactFromFinalPayload(turnID, itemID string, content string) (AiopsTransportAgentUIArtifact, bool) {
	content = strings.TrimSpace(content)
	if content == "" || !strings.HasPrefix(content, "{") {
		return AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return AiopsTransportAgentUIArtifact{}, false
	}
	if strings.TrimSpace(jsonStringValueFromMap(payload, "schemaVersion")) != "aiops.rca_report/v1" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	evidenceRefs := transportStringList(payload["evidenceRefs"])
	rawRefs := transportAnyList(payload["rawRefs"])
	status := firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "inconclusive")
	if (status == "ok" || status == "partial") && len(evidenceRefs) == 0 && len(rawRefs) == 0 {
		return AiopsTransportAgentUIArtifact{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata := map[string]any{}
	if len(evidenceRefs) > 0 {
		metadata["evidenceRefs"] = evidenceRefs
	}
	if len(rawRefs) > 0 {
		metadata["rawRefs"] = rawRefs
	}
	summaryZh := firstNonEmptyString(
		jsonStringValueFromMap(payload, "summaryZh"),
		nestedJSONStringValue(payload, "conclusion", "summaryZh"),
		nestedJSONStringValue(payload, "conclusion", "summary"),
	)
	return AiopsTransportAgentUIArtifact{
		ID:              "agent-ui:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "final-rca"),
		Type:            "rca_report",
		TitleZh:         "根因分析",
		Summary:         jsonStringValueFromMap(payload, "summary"),
		SummaryZh:       summaryZh,
		Status:          status,
		Severity:        firstNonEmptyString(jsonStringValueFromMap(payload, "severity"), "info"),
		Source:          firstNonEmptyString(jsonStringValueFromMap(payload, "source"), "aiops"),
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func asStringAnyMap(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return nil
}

func nestedJSONStringValue(payload map[string]any, parent, key string) string {
	child, ok := payload[parent].(map[string]any)
	if !ok {
		return ""
	}
	return jsonStringValueFromMap(child, key)
}

func transportStringList(value any) []string {
	items := transportAnyList(value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return cleanTransportStringList(out)
}

func transportAnyList(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

func asStringAnyMapList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			out = append(out, record)
		}
	}
	return out
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

func opsManualPreflightSummaryZh(status string) string {
	switch strings.TrimSpace(status) {
	case "passed":
		return "预检已通过，可以确认或审批后执行。"
	case "blocked":
		return "预检被阻断，需要补充参数、权限或环境适配。"
	case "failed":
		return "预检失败，不能执行绑定工作流。"
	case "not_applicable":
		return "该手册没有预检探针，需要人工确认或审批后执行。"
	default:
		return "已完成运维手册预检。"
	}
}

func opsManualParamResolutionSummaryZh(status string) string {
	switch strings.TrimSpace(status) {
	case "resolved":
		return "参数已自动补齐，可进入预检。"
	case "ambiguous":
		return "发现多个候选，需要用户选择。"
	case "need_user_input":
		return "仍缺少少量无法自动获取的参数。"
	default:
		return "已完成运维手册参数解析。"
	}
}

func opsManualParamResolutionSeverity(status string) string {
	switch strings.TrimSpace(status) {
	case "resolved":
		return "success"
	case "ambiguous", "need_user_input":
		return "warning"
	default:
		return "neutral"
	}
}

func opsManualParamResolutionArtifactActions(status string) []map[string]any {
	switch strings.TrimSpace(status) {
	case "resolved":
		return []map[string]any{{"id": "run_preflight", "label": "运行预检", "kind": "panel"}}
	case "ambiguous", "need_user_input":
		return []map[string]any{{"id": "fill_params", "label": "补充参数", "kind": "form"}}
	default:
		return nil
	}
}

func transportRunnerWorkflowGenerationArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "runner_workflow_generation" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	data := tool.OutputPreview
	if len(data) == 0 {
		data = tool.DisplayData
	}
	if len(data) == 0 {
		return AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return AiopsTransportAgentUIArtifact{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "plan_ready")
	return AiopsTransportAgentUIArtifact{
		ID:              "runner-workflow-generation:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "artifact"),
		Type:            "runner_workflow_generation",
		Title:           firstNonEmptyString(jsonStringValueFromMap(payload, "workflowTitle"), jsonStringValueFromMap(payload, "title"), "Runner Workflow generation"),
		TitleZh:         "Runner Workflow 生成进度",
		Summary:         firstNonEmptyString(jsonStringValueFromMap(payload, "summary"), jsonStringValueFromMap(payload, "requirement")),
		SummaryZh:       firstNonEmptyString(jsonStringValueFromMap(payload, "summaryZh"), "初始生成大纲已生成，等待确认后生成草稿。"),
		Status:          status,
		Severity:        "info",
		Source:          "aiops.workflow_generation",
		PermissionScope: "draft",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         asStringAnyMapList(payload["actions"]),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func transportCorootServiceMetricsArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload, userQuery string) (AiopsTransportAgentUIArtifact, bool) {
	if !isCorootServiceMetricsToolName(tool.ToolName) {
		return AiopsTransportAgentUIArtifact{}, false
	}
	data := corootDisplayDataForTransportArtifact(tool)
	if len(data) == 0 {
		return AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return AiopsTransportAgentUIArtifact{}, false
	}
	if strings.TrimSpace(jsonStringValueFromMap(payload, "tool")) != "coroot.service_metrics" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	series := corootChartSeriesFromMetrics(payload["metrics"])
	chartReports := transportAnyList(payload["chartReports"])
	if len(series) == 0 && len(chartReports) == 0 {
		return AiopsTransportAgentUIArtifact{}, false
	}
	project := jsonStringValueFromMap(payload, "project")
	service := jsonStringValueFromMap(payload, "service")
	rawRef := asStringAnyMap(payload["rawRef"])
	dataRef := jsonStringValueFromMap(rawRef, "uri")
	status := firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "ready")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	visualKind := "timeseries"
	if len(chartReports) > 0 {
		visualKind = "coroot_report_charts"
	}
	visual := map[string]any{
		"kind": visualKind,
	}
	if len(series) > 0 {
		visual["series"] = series
	}
	if len(chartReports) > 0 {
		visual["reports"] = chartReports
	}
	card := map[string]any{
		"uiKind": "readonly_chart",
		"title":  firstNonEmptyString(service, "Coroot service") + " Coroot charts",
		"visual": visual,
	}
	inlineData := cloneStringAnyMap(payload)
	defaultReportName := transportCorootPreferredReportName(chartReports, firstNonEmptyString(userQuery, tool.InputSummary))
	if defaultReportName != "" {
		inlineData["defaultReportName"] = defaultReportName
	}
	inlineData["mcpCard"] = card
	chartSummary := transportCorootChartSummaryFromPayload(payload, service, defaultReportName)
	placementTopic := transportCorootTopicFromName(defaultReportName)
	if placementTopic == "" {
		placementTopic = transportCorootTopicFromChartSummary(chartSummary)
	}
	metadata := map[string]any{
		"project":    project,
		"service":    service,
		"toolCallId": strings.TrimSpace(tool.ToolCallID),
		"placement": map[string]any{
			"supports":        []string{"root_cause"},
			"preferredAfter":  []string{"root_cause"},
			"preferredBefore": []string{"evidence"},
			"topic":           placementTopic,
			"priority":        "primary",
			"service":         service,
		},
	}
	if len(chartSummary) > 0 {
		metadata["chartSummary"] = chartSummary
	}
	if len(rawRef) > 0 {
		metadata["rawRef"] = rawRef
	}
	artifactID := "coroot-chart:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "service-metrics")
	if strings.TrimSpace(project) != "" || strings.TrimSpace(service) != "" {
		artifactID = stableTransportID("coroot-chart", turnID, firstNonEmptyString(project, "project"), firstNonEmptyString(service, "service"))
	}
	return AiopsTransportAgentUIArtifact{
		ID:              artifactID,
		Type:            "coroot_chart",
		Title:           "Coroot service charts",
		TitleZh:         firstNonEmptyString(service, "服务") + " Coroot 图表",
		Summary:         "Coroot service charts and metrics",
		SummaryZh:       "Coroot 服务原生图表与指标趋势",
		Status:          transportCorootArtifactStatus(status),
		Severity:        transportCorootArtifactSeverity(corootFirstNonNil(payload["chartReports"], payload["metrics"])),
		DataRef:         dataRef,
		InlineData:      inlineData,
		Metadata:        metadata,
		Source:          "coroot",
		PermissionScope: "read",
		RedactionStatus: "none",
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func corootDisplayDataForTransportArtifact(tool transportToolPayload) json.RawMessage {
	if len(tool.DisplayData) > 0 {
		return tool.DisplayData
	}
	return tool.OutputPreview
}

func transportCorootChartSummaryFromPayload(payload map[string]any, service string, defaultReportName string) map[string]any {
	summary := cloneStringAnyMap(asStringAnyMap(payload["chartSummary"]))
	if len(summary) == 0 {
		summary = map[string]any{}
		if metricSummaries := transportCorootMetricSummaries(payload["metrics"]); len(metricSummaries) > 0 {
			summary["metricSummaries"] = metricSummaries
		}
		if reports := transportCorootReportSummaries(payload["chartReports"]); len(reports) > 0 {
			summary["reports"] = reports
		}
	}
	if strings.TrimSpace(service) != "" {
		summary["service"] = strings.TrimSpace(service)
	}
	if strings.TrimSpace(defaultReportName) != "" {
		summary["defaultReportName"] = strings.TrimSpace(defaultReportName)
	}
	return summary
}

func transportCorootMetricSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, metric := range asStringAnyMapList(value) {
		name := jsonStringValueFromMap(metric, "name")
		item := map[string]any{
			"name":  name,
			"topic": transportCorootTopicFromName(firstNonEmptyString(name, jsonStringValueFromMap(metric, "chartTitle"))),
		}
		for _, key := range []string{"status", "value", "unit", "chartTitle"} {
			if text := jsonStringValueFromMap(metric, key); text != "" {
				item[key] = text
			}
		}
		series := asStringAnyMapList(metric["series"])
		if len(series) > 0 {
			item["seriesCount"] = len(series)
			pointCount := 0
			var seriesNames []string
			for _, seriesMap := range series {
				pointCount += len(transportAnyList(seriesMap["values"]))
				seriesNames = appendTransportUniqueString(seriesNames, jsonStringValueFromMap(seriesMap, "name"), 5)
			}
			if pointCount > 0 {
				item["pointCount"] = pointCount
			}
			if len(seriesNames) > 0 {
				item["seriesNames"] = seriesNames
			}
		} else if pointCount := len(transportAnyList(metric["values"])); pointCount > 0 {
			item["seriesCount"] = 1
			item["pointCount"] = pointCount
		}
		out = append(out, item)
	}
	return out
}

func transportCorootReportSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, report := range asStringAnyMapList(value) {
		name := jsonStringValueFromMap(report, "name")
		item := map[string]any{
			"name":  name,
			"topic": transportCorootTopicFromName(name),
		}
		if status := jsonStringValueFromMap(report, "status"); status != "" {
			item["status"] = status
		}
		chartCount := 0
		seriesCount := 0
		pointCount := 0
		var titles []string
		var seriesNames []string
		for _, widget := range asStringAnyMapList(report["widgets"]) {
			if chart := asStringAnyMap(widget["chart"]); len(chart) > 0 {
				chartCount++
				title := firstNonEmptyString(jsonStringValueFromMap(widget, "title"), jsonStringValueFromMap(chart, "title"))
				titles = appendTransportUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = transportCorootTopicFromName(title)
				}
				sc, pc, names := transportCorootSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendTransportUniqueString(seriesNames, name, 5)
				}
			}
			group := asStringAnyMap(widget["chart_group"])
			if len(group) == 0 {
				continue
			}
			groupTitle := jsonStringValueFromMap(group, "title")
			for _, chart := range asStringAnyMapList(group["charts"]) {
				chartCount++
				title := firstNonEmptyString(groupTitle, jsonStringValueFromMap(chart, "title"))
				titles = appendTransportUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = transportCorootTopicFromName(title)
				}
				sc, pc, names := transportCorootSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendTransportUniqueString(seriesNames, name, 5)
				}
			}
		}
		if chartCount > 0 {
			item["chartCount"] = chartCount
		}
		if seriesCount > 0 {
			item["seriesCount"] = seriesCount
		}
		if pointCount > 0 {
			item["pointCount"] = pointCount
		}
		if len(titles) > 0 {
			item["titles"] = titles
		}
		if len(seriesNames) > 0 {
			item["seriesNames"] = seriesNames
		}
		out = append(out, item)
	}
	return out
}

func transportCorootSeriesCounts(chart map[string]any) (int, int, []string) {
	seriesCount := 0
	pointCount := 0
	var names []string
	for _, series := range asStringAnyMapList(chart["series"]) {
		seriesCount++
		pointCount += len(transportAnyList(series["data"]))
		names = appendTransportUniqueString(names, jsonStringValueFromMap(series, "name"), 5)
	}
	if threshold := asStringAnyMap(chart["threshold"]); len(threshold) > 0 {
		pointCount += len(transportAnyList(threshold["data"]))
	}
	return seriesCount, pointCount, names
}

func transportCorootTopicFromChartSummary(summary map[string]any) string {
	for _, key := range []string{"defaultReportName", "topic"} {
		if topic := transportCorootTopicFromName(jsonStringValueFromMap(summary, key)); topic != "" {
			return topic
		}
	}
	for _, report := range asStringAnyMapList(summary["reports"]) {
		if topic := jsonStringValueFromMap(report, "topic"); topic != "" {
			return topic
		}
		if topic := transportCorootTopicFromName(jsonStringValueFromMap(report, "name")); topic != "" {
			return topic
		}
	}
	for _, metric := range asStringAnyMapList(summary["metricSummaries"]) {
		if topic := jsonStringValueFromMap(metric, "topic"); topic != "" {
			return topic
		}
		if topic := transportCorootTopicFromName(jsonStringValueFromMap(metric, "name")); topic != "" {
			return topic
		}
	}
	return ""
}

func transportCorootTopicFromName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(normalized, "net"), strings.Contains(normalized, "network"), strings.Contains(normalized, "tcp"):
		return "net"
	case strings.Contains(normalized, "cpu"):
		return "cpu"
	case strings.Contains(normalized, "memory"), strings.Contains(normalized, "mem"), strings.Contains(normalized, "rss"):
		return "memory"
	case strings.Contains(normalized, "instances"), strings.Contains(normalized, "instance"):
		return "instances"
	default:
		return ""
	}
}

func appendTransportUniqueString(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || (limit > 0 && len(values) >= limit) {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func transportTurnUserText(turn AiopsTransportTurn) string {
	if turn.User == nil {
		return ""
	}
	return strings.TrimSpace(turn.User.Text)
}

func transportCorootPreferredReportName(chartReports []any, query string) string {
	reportNames := make([]string, 0, len(chartReports))
	for _, report := range chartReports {
		name := jsonStringValueFromMap(asStringAnyMap(report), "name")
		if name != "" {
			reportNames = append(reportNames, name)
		}
	}
	if len(reportNames) == 0 {
		return ""
	}
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	preferredTokens := []string{}
	switch {
	case transportContainsAny(normalizedQuery, "cpu", "处理器"):
		preferredTokens = append(preferredTokens, "cpu")
	case transportContainsAny(normalizedQuery, "memory", "mem", "内存", "rss"):
		preferredTokens = append(preferredTokens, "memory")
	case transportContainsAny(normalizedQuery, "network", "net", "tcp", "网络", "连接"):
		preferredTokens = append(preferredTokens, "net")
	case transportContainsAny(normalizedQuery, "logs", "log", "日志"):
		preferredTokens = append(preferredTokens, "logs", "log")
	case transportContainsAny(normalizedQuery, "instances", "instance", "实例", "restart", "重启", "pod", "容器"):
		preferredTokens = append(preferredTokens, "instances", "instance")
	}
	for _, token := range preferredTokens {
		if name := transportFindCorootReportByToken(reportNames, token); name != "" {
			return name
		}
	}
	if transportContainsAny(normalizedQuery, "根因", "异常", "服务", "情况", "health", "健康", "status") {
		if name := transportFindCorootReportByToken(reportNames, "cpu"); name != "" {
			return name
		}
	}
	return reportNames[0]
}

func transportFindCorootReportByToken(reportNames []string, token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, name := range reportNames {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == token || strings.Contains(normalized, token) {
			return name
		}
	}
	return ""
}

func transportContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func isCorootServiceMetricsToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case "coroot.service_metrics", "coroot_service_metrics":
		return true
	default:
		return false
	}
}

func corootChartSeriesFromMetrics(value any) []map[string]any {
	var out []map[string]any
	for _, item := range transportAnyList(value) {
		metric, ok := item.(map[string]any)
		if !ok {
			continue
		}
		metricName := strings.ToLower(strings.TrimSpace(jsonStringValueFromMap(metric, "name")))
		if metricName != "cpu" && metricName != "memory" {
			continue
		}
		metricLabel := strings.ToUpper(metricName)
		unit := jsonStringValueFromMap(metric, "unit")
		for _, rawSeries := range transportAnyList(metric["series"]) {
			seriesMap, ok := rawSeries.(map[string]any)
			if !ok {
				continue
			}
			data := corootChartDataFromValues(seriesMap["values"])
			if len(data) == 0 {
				continue
			}
			name := firstNonEmptyString(jsonStringValueFromMap(seriesMap, "name"), jsonStringValueFromMap(metric, "chartTitle"), metricLabel)
			out = append(out, map[string]any{
				"name": firstNonEmptyString(metricLabel+" / "+name, metricLabel),
				"unit": unit,
				"data": data,
			})
		}
		if len(transportAnyList(metric["series"])) > 0 {
			continue
		}
		data := corootChartDataFromValues(metric["values"])
		if len(data) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"name": firstNonEmptyString(metricLabel+" / "+jsonStringValueFromMap(metric, "chartTitle"), metricLabel),
			"unit": unit,
			"data": data,
		})
	}
	return out
}

func corootChartDataFromValues(value any) []map[string]any {
	var out []map[string]any
	for _, item := range transportAnyList(value) {
		switch point := item.(type) {
		case []any:
			if len(point) < 2 {
				continue
			}
			timestamp, okTS := corootFloatValue(point[0])
			metricValue, okValue := corootFloatValue(point[1])
			if !okTS || !okValue {
				continue
			}
			out = append(out, map[string]any{"timestamp": timestamp, "value": metricValue})
		case map[string]any:
			metricValue, okValue := corootFloatValue(point["value"])
			if !okValue {
				continue
			}
			row := map[string]any{"value": metricValue}
			if timestamp, okTS := corootFloatValue(corootFirstNonNil(point["timestamp"], point["ts"], point["time"])); okTS {
				row["timestamp"] = timestamp
			}
			out = append(out, row)
		}
	}
	return out
}

func transportCorootArtifactStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "success", "ready":
		return "ready"
	case "warning", "error", "blocked", "running":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "ready"
	}
}

func transportCorootArtifactSeverity(metrics any) string {
	severity := "info"
	for _, item := range transportAnyList(metrics) {
		metric, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(jsonStringValueFromMap(metric, "status"))) {
		case "critical", "error":
			return "critical"
		case "warning":
			severity = "warning"
		}
	}
	return severity
}

func corootFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func corootFirstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func opsManualPreflightSeverity(status string) string {
	switch strings.TrimSpace(status) {
	case "passed":
		return "success"
	case "blocked":
		return "warning"
	case "failed":
		return "error"
	case "not_applicable":
		return "info"
	default:
		return "neutral"
	}
}

func opsManualPreflightArtifactActions(status string, nextAction string) []map[string]any {
	nextAction = strings.TrimSpace(nextAction)
	switch strings.TrimSpace(status) {
	case "passed", "not_applicable":
		switch nextAction {
		case "request_approval":
			return []map[string]any{{"id": "request_approval", "label": "发起审批", "kind": "confirm"}}
		case "execute_workflow":
			return []map[string]any{{"id": "execute_workflow", "label": "执行 Workflow", "kind": "confirm"}}
		default:
			return []map[string]any{{"id": "confirm_execution", "label": "确认执行", "kind": "confirm"}}
		}
	case "blocked":
		if nextAction == "request_permission" {
			return []map[string]any{{"id": "request_permission", "label": "申请权限", "kind": "panel"}}
		}
		if nextAction == "generate_workflow_variant" {
			return []map[string]any{{"id": "generate_variant", "label": "生成适配工作流", "kind": "confirm"}}
		}
		return []map[string]any{{"id": "collect_context", "label": "补充上下文", "kind": "form"}}
	case "failed":
		return []map[string]any{{"id": "fallback_guide", "label": "查看降级步骤", "kind": "panel"}}
	default:
		return nil
	}
}

func opsManualSearchSummaryZh(decision string) string {
	switch strings.TrimSpace(decision) {
	case "direct_execute":
		return "已找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。"
	case "adapt":
		return "找到相似运维手册，但当前环境存在差异，需要先生成变体并校验。"
	case "reference_only":
		return "没有可直接运行的 Workflow，可继续只读自动化排查。"
	case "need_info":
		return "识别到相关运维手册，但还缺少少量关键上下文。"
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
			{"id": "run_preflight", "label": "运行预检", "kind": "panel"},
		}
	case "adapt":
		return []map[string]any{
			{"id": "generate_variant", "label": "生成适配工作流", "kind": "confirm"},
			{"id": "review_gaps", "label": "查看差异", "kind": "panel"},
		}
	case "reference_only":
		return nil
	case "need_info":
		return nil
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
	case strings.HasPrefix(kind, "hostops."), name == "spawn_host_agent", name == "send_host_agent_message", name == "wait_host_agents", name == "stop_host_agent":
		return AiopsTransportProcessKindSubagent
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

func shouldSuppressOpsManualSearchProcessBlock(tool transportToolPayload, outputPreview string) bool {
	if strings.TrimSpace(tool.DisplayKind) != "ops_manual_search_result" && strings.TrimSpace(tool.ToolName) != "search_ops_manuals" {
		return false
	}
	outputPreview = strings.TrimSpace(outputPreview)
	if outputPreview == "" {
		return true
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(outputPreview), &payload); err != nil || len(payload) == 0 {
		return true
	}
	return !isActionableOpsManualSearchPayload(payload)
}

func compactOpsManualSearchProcessText(displayKind, text, outputPreview string) (string, string) {
	if strings.TrimSpace(displayKind) != "ops_manual_search_result" {
		return text, outputPreview
	}
	payload := map[string]any{}
	if strings.TrimSpace(outputPreview) != "" {
		_ = json.Unmarshal([]byte(outputPreview), &payload)
	}
	decision := strings.TrimSpace(jsonStringValueFromMap(payload, "decision"))
	summary := strings.TrimSpace(jsonStringValueFromMap(payload, "summary"))
	if decision == "" && summary == "" {
		return firstNonEmptyString(summary, text, "运维手册检索完成"), ""
	}
	label := "运维手册匹配"
	if decision != "" {
		label += "：" + opsManualSearchDecisionLabel(decision)
	}
	if summary != "" {
		label += "，" + summary
	}
	return label, ""
}

func opsManualSearchDecisionLabel(decision string) string {
	switch strings.TrimSpace(decision) {
	case "need_info", "need_more_info", "missing_info":
		return "手册缺上下文"
	case "direct_execute", "direct", "executable":
		return "可进入预检"
	case "adapt", "adapt_required", "generate_variant":
		return "需适配 Workflow"
	case "reference_only", "reference":
		return "仅参考"
	case "no_match":
		return "无可用手册"
	default:
		return strings.TrimSpace(decision)
	}
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

func jsonStringValueFromMap(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func cleanTransportStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		cleaned = append(cleaned, text)
	}
	return cleaned
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
