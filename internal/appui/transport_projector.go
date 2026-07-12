package appui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/specialinputmemory"
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
	if turn.SpecialInputReadPlan != nil {
		next.SpecialInputContext = projectSpecialInputContextFromTurn(turn)
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
	projectedTurn.ClientTurnID = turn.ClientTurnID
	projectedTurn.ClientMessageID = turn.ClientMessageID
	agentItems := projectTransportAgentItems(turn.AgentItems)
	projectedTurn.AgentItems = agentItems.Items
	projectedTurn.AgentItemsTruncated = agentItems.Truncated
	projectedTurn.AgentItemsOriginalCount = agentItems.OriginalCount
	projectedTurn.AgentItemsOriginalBytes = agentItems.OriginalBytes
	projectedTurn.AgentItemsHash = agentItems.Hash
	projectedTurn.AgentItemsRef = agentItems.Ref
	projectionItems := redactTransportProjectionTurnItems(turn.AgentItems)
	projectedTurn.Process = nil
	projectedTurn.Timeline = projectTurnTimeline(projectionItems)
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
	resultPreviews, resultPayloads := transportToolResultFacts(turn)
	for _, item := range projectionItems {
		projectedTurn = projectTurnItem(projectedTurn, &next, turnID, item, resultPreviews, resultPayloads, turn.Metadata)
	}
	if turn.Lifecycle.IsTerminal() {
		projectedTurn.Process = normalizeTerminalProcessBlocks(projectedTurn.Process, turn.Lifecycle, turn.Error)
		pruneTransportPendingApprovalsForTurn(&next, turnID, map[string]bool{})
	} else {
		projectedTurn = projectSnapshotPendingApprovals(projectedTurn, &next, turnID, turn.PendingApprovals)
		projectedTurn = projectSnapshotPendingEvidence(projectedTurn, &next, turnID, turn.PendingEvidence)
	}
	projectedTurn = projectCheckpointProcessBlocks(projectedTurn, turnID, turn)
	if projectedTurn.Final != nil {
		if rcaProjectionAllowed(turn.Metadata) {
			if artifact, ok := transportRCAArtifactFromFinalPayload(turnID, projectedTurn.Final.ID, projectedTurn.Final.Text); ok {
				projectedTurn.AgentUIArtifacts = upsertTransportAgentUIArtifact(projectedTurn.AgentUIArtifacts, artifact)
			}
		}
	}

	if isHostOpsTurnMetadata(turn.Metadata) {
		next = projectHostOpsMissionFromTurn(next, turnID, projectedTurn, turn)
	} else if strings.TrimSpace(turn.Metadata["aiops.route.mode"]) == string(ChatRouteHostBoundOps) {
		next = clearStaleHostOpsMissionForTurn(next, turnID)
	}
	hostOpsBlocked := hostOpsProjectionBlocked(next, turnID)

	projectedTurn.Status = mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, len(next.PendingApprovals) > 0 || hostOpsBlocked)
	if hostOpsBlocked && projectedTurn.Status == AiopsTransportTurnStatusWorking {
		projectedTurn.Status = AiopsTransportTurnStatusBlocked
	}
	if projectedTurn.Final != nil && projectedTurn.Final.Status == "" {
		projectedTurn.Final.Status = mapTurnStatusToFinalStatus(projectedTurn.Status)
	}
	next.Turns[turnID] = projectedTurn
	if opsRun := projectOpsRunFromTurn(turn, projectedTurn); opsRun != nil {
		next.OpsRun = opsRun
	}

	applyTurnLiveness(&next, turnID, projectedTurn.Status)
	if errText := firstNonEmptyString(strings.TrimSpace(turn.Error), strings.TrimSpace(next.LastError)); errText != "" && projectedTurn.Status == AiopsTransportTurnStatusFailed {
		next.LastError = sanitizeUserVisibleRuntimeErrorText(errText)
	}
	next.Status = mapTurnStatusToTransportStatus(projectedTurn.Status)
	next.Seq += int64(len(turn.AgentItems))
	next.UpdatedAt = firstNonEmptyString(transportTimestamp(turn.UpdatedAt), next.UpdatedAt)

	return next, nil
}

func projectSpecialInputContextFromTurn(turn *runtimekernel.TurnSnapshot) *specialinputmemory.TransportContext {
	if turn == nil || turn.SpecialInputReadPlan == nil {
		return nil
	}
	return specialinputmemory.ProjectTransportContext(*turn.SpecialInputReadPlan)
}

func projectOpsRunFromTurn(turn *runtimekernel.TurnSnapshot, projectedTurn AiopsTransportTurn) *AiopsTransportOpsRun {
	if turn == nil {
		return nil
	}
	view := chatRunTraceViewFromMetadata(turn.Metadata, userPromptForOpsRun(projectedTurn))
	if strings.TrimSpace(view.ID) == "" {
		return nil
	}
	status := mapTurnLifecycleToTransportTurnStatus(turn.Lifecycle, turn.ResumeState, false)
	checkpointID := ""
	if turn.LatestCheckpoint != nil {
		checkpointID = strings.TrimSpace(turn.LatestCheckpoint.ID)
	}
	opsRun := &AiopsTransportOpsRun{
		ID:                 view.ID,
		SessionID:          firstNonEmptyString(view.SessionID, turn.SessionID),
		TurnID:             firstNonEmptyString(view.TurnID, turn.ID),
		ClientTurnID:       firstNonEmptyString(view.ClientTurnID, turn.ClientTurnID),
		Source:             firstNonEmptyString(view.Source, opsRunSourceChat),
		Status:             string(status),
		Title:              view.Title,
		RouteMode:          view.RouteMode,
		TargetSummary:      view.TargetSummary,
		ToolSurfaceSummary: view.ToolSurfaceSummary,
		EvidenceCount:      countProjectedEvidenceRefs(projectedTurn),
		CurrentStep:        firstNonEmptyString(view.CurrentStep, currentStepForProjectedTurn(projectedTurn, status)),
		CheckpointID:       checkpointID,
	}
	opsRun.AgentRun = buildAgentRunViewFromOpsRunAndTurn(*opsRun, projectedTurn, turn.Iteration)
	if opsRun.AgentRun != nil {
		opsRun.CurrentStepID = opsRun.AgentRun.CurrentStepID
		opsRun.PostRunSuggestions = BuildPostRunSuggestionsFromAgentRun(*opsRun.AgentRun)
	}
	return opsRun
}

func projectCheckpointProcessBlocks(projectedTurn AiopsTransportTurn, turnID string, turn *runtimekernel.TurnSnapshot) AiopsTransportTurn {
	checkpoints := BuildCheckpointSummariesFromTurn(turn, nil)
	if len(checkpoints) == 0 {
		return projectedTurn
	}
	existing := map[string]struct{}{}
	for _, block := range projectedTurn.Process {
		if id := strings.TrimSpace(block.CheckpointID); id != "" {
			existing[id] = struct{}{}
		}
	}
	for _, checkpoint := range checkpoints {
		if strings.TrimSpace(checkpoint.ID) == "" {
			continue
		}
		if !checkpointVisibleInChat(checkpoint) {
			continue
		}
		if _, ok := existing[checkpoint.ID]; ok {
			continue
		}
		projectedTurn.Process = append(projectedTurn.Process, AiopsProcessBlock{
			ID:                  TransportProcessBlockStableID(turnID, "checkpoint", checkpoint.ID),
			Kind:                AiopsTransportProcessKindSystem,
			DisplayKind:         "checkpoint." + checkpoint.Kind,
			Status:              checkpointProcessStatus(checkpoint),
			Text:                checkpointProcessText(checkpoint),
			InputSummary:        checkpoint.ToolSurfaceSummary,
			CheckpointID:        checkpoint.ID,
			TargetSummary:       strings.Join(checkpoint.TargetRefs, ", "),
			EvidenceRefs:        append([]string(nil), checkpoint.EvidenceRefs...),
			UpdatedAt:           transportTimestamp(checkpoint.CreatedAt),
			MaterializationTier: "metadata",
		})
		existing[checkpoint.ID] = struct{}{}
	}
	return projectedTurn
}

func checkpointVisibleInChat(checkpoint PromptTraceCheckpointSummary) bool {
	return false
}

func checkpointProcessStatus(checkpoint PromptTraceCheckpointSummary) AiopsTransportProcessStatus {
	switch checkpoint.Kind {
	case runtimekernel.CheckpointKindApprovalWaiting:
		return AiopsTransportProcessStatusBlocked
	case runtimekernel.CheckpointKindErrorRecovery:
		return AiopsTransportProcessStatusFailed
	default:
		if checkpoint.Resumable {
			return AiopsTransportProcessStatusRunning
		}
		return AiopsTransportProcessStatusCompleted
	}
}

func checkpointProcessText(checkpoint PromptTraceCheckpointSummary) string {
	switch checkpoint.Kind {
	case runtimekernel.CheckpointKindApprovalWaiting:
		return "等待审批"
	case runtimekernel.CheckpointKindApprovalResolved:
		return "审批已恢复"
	case runtimekernel.CheckpointKindAfterToolCall:
		return "工具调用后"
	case runtimekernel.CheckpointKindBeforeToolCall:
		return "工具调用前"
	case runtimekernel.CheckpointKindToolSurfaceReady:
		return "工具面就绪"
	case runtimekernel.CheckpointKindFinalResponse:
		return "最终响应"
	case runtimekernel.CheckpointKindErrorRecovery:
		return "工具失败后继续分析"
	case runtimekernel.CheckpointKindTurnStart:
		return "轮次开始"
	default:
		return "运行状态已更新"
	}
}

func userPromptForOpsRun(turn AiopsTransportTurn) string {
	if turn.User == nil {
		return ""
	}
	return turn.User.Text
}

func countProjectedEvidenceRefs(turn AiopsTransportTurn) int {
	seen := map[string]struct{}{}
	for _, block := range turn.Process {
		for _, ref := range block.EvidenceRefs {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				seen[ref] = struct{}{}
			}
		}
	}
	for _, artifact := range turn.AgentUIArtifacts {
		if ref, ok := artifact.Metadata["evidenceRef"].(string); ok && strings.TrimSpace(ref) != "" {
			seen[strings.TrimSpace(ref)] = struct{}{}
		}
	}
	return len(seen)
}

func projectTurnTimeline(items []agentstate.TurnItem) []AiopsTransportTimelineItem {
	if len(items) == 0 {
		return nil
	}
	timeline := make([]AiopsTransportTimelineItem, 0, len(items))
	for _, item := range items {
		if isUserProvidedEvidenceItem(item) {
			continue
		}
		id := strings.TrimSpace(item.ID)
		itemType := strings.TrimSpace(string(item.Type))
		if id == "" || itemType == "" {
			continue
		}
		text := strings.TrimSpace(item.Payload.Summary)
		if item.Type == agentstate.TurnItemTypeAssistantMessage && assistantMessageProjectionData(item).Phase == "final_answer" {
			text = sanitizeUserVisibleFinalAnswerText(text)
		} else {
			text = sanitizeUserVisibleProcessText(text)
		}
		timeline = append(timeline, AiopsTransportTimelineItem{
			ID:          id,
			Type:        itemType,
			Status:      strings.TrimSpace(string(item.Status)),
			Text:        text,
			PayloadKind: strings.TrimSpace(item.Payload.Kind),
			CreatedAt:   transportTimestamp(item.CreatedAt),
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		})
	}
	return timeline
}

func finalDurationMsFromAgentItem(item agentstate.TurnItem) int64 {
	if len(item.Payload.Data) == 0 {
		return 0
	}
	var payload struct {
		DurationMs int64 `json:"durationMs"`
	}
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		return 0
	}
	return payload.DurationMs
}

func currentStepForProjectedTurn(turn AiopsTransportTurn, status AiopsTransportTurnStatus) string {
	for i := len(turn.Process) - 1; i >= 0; i-- {
		block := turn.Process[i]
		if block.Status == AiopsTransportProcessStatusRunning || block.Status == AiopsTransportProcessStatusBlocked {
			return block.Text
		}
	}
	switch status {
	case AiopsTransportTurnStatusSubmitted, AiopsTransportTurnStatusWorking:
		return "处理中"
	case AiopsTransportTurnStatusBlocked:
		return "等待用户确认"
	case AiopsTransportTurnStatusCompleted:
		return "已结束"
	case AiopsTransportTurnStatusFailed:
		return "处理失败"
	case AiopsTransportTurnStatusCanceled:
		return "已停止"
	default:
		return ""
	}
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

func transportToolResultFacts(turn *runtimekernel.TurnSnapshot) (map[string]string, map[string]json.RawMessage) {
	previews := map[string]string{}
	payloads := map[string]json.RawMessage{}
	if turn == nil {
		return previews, payloads
	}
	for _, iteration := range turn.Iterations {
		for _, result := range iteration.ToolResults {
			toolCallID := strings.TrimSpace(result.ToolCallID)
			content := strings.TrimSpace(result.Content)
			if toolCallID == "" || content == "" {
				continue
			}
			safeContent := redactTransportAgentText(content)
			var value any
			if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
				value = decodeTransportAgentPayload(json.RawMessage(content))
				value = redactTransportAgentValue(value)
				if safeRaw, err := json.Marshal(value); err == nil {
					safeContent = string(safeRaw)
					if object, ok := value.(map[string]any); ok && len(object) > 0 && len(payloads[toolCallID]) == 0 {
						payloads[toolCallID] = append(json.RawMessage(nil), safeRaw...)
					}
				}
			}
			if _, exists := previews[toolCallID]; !exists {
				_, outputPreview, _ := summarizeToolResultForEvent(turn.ID, toolCallID, safeContent)
				if preview := jsonStringValue(outputPreview); preview != "" {
					previews[toolCallID] = preview
				}
			}
		}
	}
	return previews, payloads
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
		targetSummary := approvalTargetSummary(approval)
		risk := strings.TrimSpace(approval.Risk)
		approvalType := "tool"
		if command != "" || strings.EqualFold(strings.TrimSpace(approval.ToolName), "exec_command") {
			approvalType = "command"
		}
		updatedAt := firstNonEmptyString(transportTimestamp(approval.UpdatedAt), transportTimestamp(approval.CreatedAt))
		block := AiopsProcessBlock{
			ID:             TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindApproval), approvalID),
			Kind:           AiopsTransportProcessKindApproval,
			DisplayKind:    "approval",
			Status:         AiopsTransportProcessStatusBlocked,
			Text:           firstNonEmptyString(reason, command, "等待审批"),
			Command:        command,
			ApprovalID:     approvalID,
			Source:         strings.TrimSpace(approval.Source),
			TargetSummary:  targetSummary,
			Risk:           risk,
			RiskSummary:    approvalRiskSummary(risk, reason),
			ExpectedEffect: strings.TrimSpace(approval.ExpectedEffect),
			Rollback:       strings.TrimSpace(approval.Rollback),
			Validation:     strings.TrimSpace(approval.Validation),
			UpdatedAt:      updatedAt,
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
		state.PendingApprovals[approvalID] = AiopsTransportApproval{
			ID:             approvalID,
			TurnID:         firstNonEmptyString(strings.TrimSpace(approval.TurnID), turnID),
			Type:           approvalType,
			Status:         status,
			Command:        command,
			Reason:         reason,
			Risk:           risk,
			Source:         strings.TrimSpace(approval.Source),
			TargetSummary:  targetSummary,
			ExpectedEffect: strings.TrimSpace(approval.ExpectedEffect),
			Rollback:       strings.TrimSpace(approval.Rollback),
			Validation:     strings.TrimSpace(approval.Validation),
			RequestedAt:    transportTimestamp(approval.CreatedAt),
		}
		state.RuntimeLiveness.PendingApprovals[approvalID] = true
	}
	return turn
}

func approvalTargetSummary(approval runtimekernel.PendingApproval) string {
	values := make([]string, 0, 1+len(approval.ResourceScopes))
	if hostID := strings.TrimSpace(approval.HostID); hostID != "" {
		values = append(values, "host:"+hostID)
	}
	for _, scope := range approval.ResourceScopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			values = append(values, scope)
		}
	}
	cleaned := cleanTransportStringList(values)
	seen := map[string]bool{}
	out := make([]string, 0, len(cleaned))
	for _, value := range cleaned {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return strings.Join(out, "；")
}

func approvalRiskSummary(risk string, reason string) string {
	risk = strings.TrimSpace(risk)
	reason = strings.TrimSpace(reason)
	if risk == "" {
		return reason
	}
	if reason == "" {
		return "风险等级：" + risk
	}
	return "风险等级：" + risk + "；" + reason
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
	metadata map[string]string,
) AiopsTransportTurn {
	if isUserProvidedEvidenceItem(item) {
		return turn
	}
	switch item.Type {
	case agentstate.TurnItemTypeUserMessage:
		text := firstNonEmptyString(decodeUserMessageText(item.Payload.Data), strings.TrimSpace(item.Payload.Summary))
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
			if artifact, ok := transportGenericAgentUIArtifactFromToolPayload(turnID, item.ID, artifactTool, rcaProjectionAllowed(metadata)); ok {
				turn.AgentUIArtifacts = upsertTransportAgentUIArtifact(turn.AgentUIArtifacts, artifact)
			}
			if artifact, ok := transportWorkflowEditorArtifactFromToolPayload(turnID, item.ID, artifactTool); ok {
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
		if blockKind == AiopsTransportProcessKindCommand {
			outputPreview = commandTerminalOutputPreview(outputPreview, tool.ExitCode, mapItemStatusToTransportProcessStatus(item.Status))
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
			ToolCallID:          strings.TrimSpace(tool.ToolCallID),
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
			searchResultRaw := tool.OutputPreview
			if len(searchResultRaw) == 0 {
				searchResultRaw = resultPayloads[strings.TrimSpace(tool.ToolCallID)]
			}
			request := decodeTransportSearchRequest(tool.Arguments)
			envelope, hasEnvelope := decodeTransportSearchEnvelope(searchResultRaw)
			block.Operation = firstNonEmptyString(envelope.Operation, request.Operation)
			block.URL = firstNonEmptyString(envelope.URL, request.URL)
			block.Adapter = strings.TrimSpace(envelope.Source)
			block.Backend = strings.TrimSpace(envelope.Backend)
			block.Results = decodeTransportSearchResults(searchResultRaw)
			if len(block.Results) > 0 {
				block.SourceCount = len(block.Results)
			} else if hasEnvelope && envelope.SourceCount >= 0 {
				block.SourceCount = envelope.SourceCount
			}
			if block.Operation == "" && block.URL != "" {
				block.Operation = "open"
			}
			if block.Operation == "" {
				block.Operation = "search"
			}
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
		applyTransportFoldGroup(turnID, &block)
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
	case agentstate.TurnItemTypeApproval, agentstate.TurnItemTypeApprovalRequested, agentstate.TurnItemTypeApprovalDecided:
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
	case agentstate.TurnItemTypeFinalResponse:
		text := sanitizeUserVisibleFinalAnswerText(item.Payload.Summary)
		if text == "" {
			return turn
		}
		message := assistantMessageProjectionData(item)
		durationMs := firstNonZeroInt64(message.DurationMs, finalDurationMsFromAgentItem(item))
		turn.Final = transportFinalFromAgentItem(item.ID, text, mapItemStatusToTransportFinalStatus(item.Status), durationMs, message.FinalContract)
	case agentstate.TurnItemTypeAssistantMessage:
		message := assistantMessageProjectionData(item)
		switch message.Phase {
		case "final_answer":
			text := sanitizeUserVisibleFinalAnswerText(item.Payload.Summary)
			if text == "" {
				return turn
			}
			durationMs := firstNonZeroInt64(message.DurationMs, finalDurationMsFromAgentItem(item))
			turn.Final = transportFinalFromAgentItem(item.ID, text, mapItemStatusToTransportFinalStatus(item.Status), durationMs, message.FinalContract)
		default:
			text := sanitizeUserVisibleProcessText(item.Payload.Summary)
			if text == "" {
				return turn
			}
			durationMs := firstNonZeroInt64(message.DurationMs, finalDurationMsFromAgentItem(item))
			displayKind := firstNonEmptyString(message.DisplayKind, "assistant.message")
			block := AiopsProcessBlock{
				ID:               TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindAssistant), firstNonEmptyString(item.ID, "output")),
				Kind:             AiopsTransportProcessKindAssistant,
				DisplayKind:      displayKind,
				Phase:            firstNonEmptyString(message.Phase, "commentary"),
				StreamState:      strings.TrimSpace(message.StreamState),
				CommentarySource: strings.TrimSpace(message.CommentarySource),
				ToolCallIDs:      append([]string(nil), message.ToolCallIDs...),
				EvidenceBoundary: strings.TrimSpace(message.EvidenceBoundary),
				Status:           mapItemStatusToTransportProcessStatus(item.Status),
				Text:             text,
				DurationMs:       durationMs,
				UpdatedAt:        transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
			}
			turn.Process = upsertTransportProcessBlock(turn.Process, block)
		}
	case agentstate.TurnItemTypeModelCall:
		text := modelCallProcessText(item.Status, strings.TrimSpace(item.Payload.Summary))
		if strings.TrimSpace(text) == "" {
			return turn
		}
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindReasoning), item.ID),
			Kind:        AiopsTransportProcessKindReasoning,
			DisplayKind: "reasoning",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        text,
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	case agentstate.TurnItemTypeError:
		text := strings.TrimSpace(item.Payload.Summary)
		if text == "" {
			return turn
		}
		visibleText := sanitizeUserVisibleRuntimeErrorText(text)
		state.LastError = visibleText
		block := AiopsProcessBlock{
			ID:          TransportProcessBlockStableID(turnID, string(AiopsTransportProcessKindSystem), item.ID),
			Kind:        AiopsTransportProcessKindSystem,
			DisplayKind: "runtime.error",
			Status:      mapItemStatusToTransportProcessStatus(item.Status),
			Text:        visibleText,
			UpdatedAt:   transportTimestamp(firstNonZeroTime(item.UpdatedAt, item.CreatedAt)),
		}
		turn.Process = upsertTransportProcessBlock(turn.Process, block)
	}

	return turn
}

func transportFinalFromAgentItem(id string, text string, fallbackStatus AiopsTransportFinalStatus, durationMs int64, contract runtimekernel.FinalContract) *AiopsTransportFinal {
	final := &AiopsTransportFinal{
		ID:         strings.TrimSpace(id),
		Text:       strings.TrimSpace(text),
		Status:     fallbackStatus,
		DurationMs: durationMs,
	}
	if strings.TrimSpace(contract.SchemaVersion) == "" {
		return final
	}
	final.SchemaVersion = strings.TrimSpace(contract.SchemaVersion)
	final.Status = mapFinalContractStatusToTransportStatus(contract.Status, fallbackStatus)
	final.Confidence = strings.TrimSpace(contract.Confidence)
	final.AnswerText = strings.TrimSpace(contract.AnswerText)
	final.CheckedEvidenceRefs = cleanTransportStringList(contract.CheckedEvidenceRefs)
	final.UncheckedRequirements = cleanTransportStringList(contract.UncheckedRequirements)
	final.FailedToolImpacts = transportFailedToolImpacts(contract.FailedToolImpacts)
	final.ApprovedActions = cleanTransportStringList(contract.ApprovedActions)
	final.PerformedActions = cleanTransportStringList(contract.PerformedActions)
	final.PostChecks = cleanTransportStringList(contract.PostChecks)
	final.Limitations = cleanTransportStringList(contract.Limitations)
	return final
}

func transportFailedToolImpacts(items []runtimekernel.FailedToolImpact) []AiopsTransportFailedToolImpact {
	out := make([]AiopsTransportFailedToolImpact, 0, len(items))
	for _, item := range items {
		impact := AiopsTransportFailedToolImpact{
			ToolName:     strings.TrimSpace(item.ToolName),
			ToolCallID:   strings.TrimSpace(item.ToolCallID),
			FailureClass: strings.TrimSpace(item.FailureClass),
			Impact:       strings.TrimSpace(item.Impact),
		}
		if impact.ToolName == "" && impact.ToolCallID == "" && impact.FailureClass == "" && impact.Impact == "" {
			continue
		}
		out = append(out, impact)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type assistantMessageProjectionPayload struct {
	DisplayKind      string                      `json:"displayKind"`
	Phase            string                      `json:"phase"`
	StreamState      string                      `json:"streamState"`
	EvidenceBoundary string                      `json:"evidenceBoundary"`
	DurationMs       int64                       `json:"durationMs"`
	CommentarySource string                      `json:"commentarySource"`
	ToolCallIDs      []string                    `json:"toolCallIds"`
	FinalContract    runtimekernel.FinalContract `json:"finalContract"`
}

func assistantMessageProjectionData(item agentstate.TurnItem) assistantMessageProjectionPayload {
	if len(item.Payload.Data) == 0 {
		return assistantMessageProjectionPayload{}
	}
	var payload assistantMessageProjectionPayload
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		return assistantMessageProjectionPayload{}
	}
	payload.DisplayKind = strings.TrimSpace(payload.DisplayKind)
	payload.Phase = strings.TrimSpace(payload.Phase)
	payload.StreamState = strings.TrimSpace(payload.StreamState)
	payload.EvidenceBoundary = strings.TrimSpace(payload.EvidenceBoundary)
	payload.CommentarySource = strings.TrimSpace(payload.CommentarySource)
	payload.ToolCallIDs = compactStrings(payload.ToolCallIDs)
	return payload
}

func isUserProvidedEvidenceItem(item agentstate.TurnItem) bool {
	return item.Type == agentstate.TurnItemTypeEvidence &&
		strings.EqualFold(strings.TrimSpace(item.Payload.Kind), "user_provided")
}

func modelCallProcessText(status agentstate.ItemStatus, summary string) string {
	text := strings.TrimSpace(summary)
	switch strings.ToLower(text) {
	case "", "calling model":
		switch status {
		case agentstate.ItemStatusPending:
			return "排队等待模型返回"
		case agentstate.ItemStatusRunning:
			return "正在等待模型返回"
		case agentstate.ItemStatusFailed:
			return "模型调用失败"
		case agentstate.ItemStatusCancelled:
			return "模型调用已取消"
		default:
			return ""
		}
	case "model response received":
		return ""
	default:
		return summary
	}
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

func clearStaleHostOpsMissionForTurn(state AiopsTransportState, turnID string) AiopsTransportState {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" || len(state.HostMissions) == 0 {
		return state
	}
	for missionID, mission := range state.HostMissions {
		if strings.TrimSpace(mission.TurnID) != turnID {
			continue
		}
		for _, childID := range mission.ChildAgentIDs {
			delete(state.ChildAgents, strings.TrimSpace(childID))
		}
		delete(state.HostMissions, missionID)
		if state.ActiveHostMissionID == missionID {
			state.ActiveHostMissionID = ""
		}
	}
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
	if strings.TrimSpace(metadata["aiops.route.mode"]) == string(ChatRouteHostBoundOps) {
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

func rcaProjectionAllowed(metadata map[string]string) bool {
	if strings.TrimSpace(metadata[metadataCorootSkipReason]) != "" {
		return false
	}
	return metadataBool(metadata, metadataCorootRCADisplayAllowed) || metadataBool(metadata, metadataCorootExplicitRCA)
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
	var ok bool
	block, ok = sanitizeTransportProcessBlock(block)
	if !ok {
		return blocks
	}
	for i := range blocks {
		if blocks[i].ID == block.ID {
			blocks[i] = mergeTransportProcessBlock(blocks[i], block)
			return blocks
		}
	}
	return append(blocks, block)
}

func sanitizeTransportProcessBlock(block AiopsProcessBlock) (AiopsProcessBlock, bool) {
	block.Text = sanitizeUserVisibleProcessText(redactTransportAgentText(block.Text))
	block.Command = sanitizeUserVisibleProcessText(redactTransportAgentText(block.Command))
	block.InputSummary = sanitizeUserVisibleProcessText(redactTransportAgentText(block.InputSummary))
	block.OutputPreview = sanitizeUserVisibleProcessText(redactTransportAgentText(block.OutputPreview))
	for i := range block.Steps {
		block.Steps[i].Text = sanitizeUserVisibleProcessText(redactTransportAgentText(block.Steps[i].Text))
		block.Steps[i].Title = sanitizeUserVisibleProcessText(redactTransportAgentText(block.Steps[i].Title))
		block.Steps[i].Summary = sanitizeUserVisibleProcessText(redactTransportAgentText(block.Steps[i].Summary))
	}
	if block.Text == "" &&
		block.Command == "" &&
		block.InputSummary == "" &&
		block.OutputPreview == "" &&
		len(block.Steps) == 0 &&
		len(block.Queries) == 0 &&
		len(block.Results) == 0 {
		return block, false
	}
	return block, true
}

func normalizeTerminalProcessBlocks(blocks []AiopsProcessBlock, lifecycle runtimekernel.TurnLifecycleState, errorText string) []AiopsProcessBlock {
	status := terminalProcessStatus(lifecycle, errorText)
	for i := range blocks {
		switch blocks[i].Status {
		case AiopsTransportProcessStatusBlocked, AiopsTransportProcessStatusRunning, AiopsTransportProcessStatusQueued:
			blocks[i].Status = status
		}
		if lifecycle == runtimekernel.TurnLifecycleCanceled && blocks[i].Kind == AiopsTransportProcessKindReasoning && isModelWaitingProcessText(blocks[i].Text) {
			blocks[i].Text = "模型调用已取消"
		}
	}
	return dedupeTerminalRuntimeErrorBlocks(blocks)
}

func dedupeTerminalRuntimeErrorBlocks(blocks []AiopsProcessBlock) []AiopsProcessBlock {
	seen := map[string]bool{}
	next := make([]AiopsProcessBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.DisplayKind == "runtime.error" {
			key := strings.TrimSpace(block.Text)
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
		}
		next = append(next, block)
	}
	return next
}

func isModelWaitingProcessText(text string) bool {
	switch strings.TrimSpace(text) {
	case "排队等待模型返回", "正在等待模型返回":
		return true
	default:
		return false
	}
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
	if existing.Kind == AiopsTransportProcessKindSearch && next.Kind == AiopsTransportProcessKindSearch {
		if len(next.Queries) == 0 || (len(next.Queries) == 1 && isSparseSearchCompletionSummary(next.Queries[0])) {
			next.Queries = append([]string(nil), existing.Queries...)
		}
		if len(next.Results) == 0 {
			next.Results = append([]AiopsSearchResult(nil), existing.Results...)
		}
		if strings.TrimSpace(next.Operation) == "" {
			next.Operation = existing.Operation
		}
		if strings.TrimSpace(next.URL) == "" {
			next.URL = existing.URL
		}
		if strings.TrimSpace(next.Adapter) == "" {
			next.Adapter = existing.Adapter
		}
		if strings.TrimSpace(next.Backend) == "" {
			next.Backend = existing.Backend
		}
		if next.SourceCount == 0 && existing.SourceCount > 0 {
			next.SourceCount = existing.SourceCount
		}
		if strings.TrimSpace(next.InputSummary) == "" || isSparseSearchCompletionSummary(next.InputSummary) {
			next.InputSummary = existing.InputSummary
		}
		if strings.TrimSpace(next.Text) == "" || strings.TrimSpace(next.Text) == strings.TrimSpace(next.OutputPreview) {
			next.Text = firstNonEmptyString(existing.Text, next.InputSummary, next.Text)
		}
	}
	return next
}

func isSparseSearchCompletionSummary(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "web_search", "browse_url", "search_web", "web_search completed", "browser.search":
		return true
	default:
		return false
	}
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
	case AiopsTransportTurnStatusFailed:
		return AiopsTransportFinalStatusFailed
	case AiopsTransportTurnStatusCanceled:
		return AiopsTransportFinalStatusCancelled
	case AiopsTransportTurnStatusCompleted:
		return AiopsTransportFinalStatusCompleted
	default:
		return AiopsTransportFinalStatusRunning
	}
}

func mapFinalStatusToTransportProcessStatus(status AiopsTransportFinalStatus) AiopsTransportProcessStatus {
	switch status {
	case AiopsTransportFinalStatusCompleted, AiopsTransportFinalStatusVerified, AiopsTransportFinalStatusPartial:
		return AiopsTransportProcessStatusCompleted
	case AiopsTransportFinalStatusFailed, AiopsTransportFinalStatusCancelled:
		return AiopsTransportProcessStatusFailed
	case AiopsTransportFinalStatusBlocked, AiopsTransportFinalStatusNeedsEvidence, AiopsTransportFinalStatusApprovalDenied, AiopsTransportFinalStatusToolUnavailable:
		return AiopsTransportProcessStatusBlocked
	default:
		return AiopsTransportProcessStatusRunning
	}
}

func mapFinalContractStatusToTransportStatus(status runtimekernel.FinalContractStatus, fallback AiopsTransportFinalStatus) AiopsTransportFinalStatus {
	switch status {
	case runtimekernel.FinalContractStatusVerified:
		return AiopsTransportFinalStatusVerified
	case runtimekernel.FinalContractStatusPartial:
		return AiopsTransportFinalStatusPartial
	case runtimekernel.FinalContractStatusBlocked:
		return AiopsTransportFinalStatusBlocked
	case runtimekernel.FinalContractStatusNeedsEvidence:
		return AiopsTransportFinalStatusNeedsEvidence
	case runtimekernel.FinalContractStatusApprovalDenied:
		return AiopsTransportFinalStatusApprovalDenied
	case runtimekernel.FinalContractStatusToolUnavailable:
		return AiopsTransportFinalStatusToolUnavailable
	case runtimekernel.FinalContractStatusCancelled:
		return AiopsTransportFinalStatusCancelled
	case runtimekernel.FinalContractStatusFailed:
		return AiopsTransportFinalStatusFailed
	case runtimekernel.FinalContractStatusUnknown:
		return AiopsTransportFinalStatusUnknown
	default:
		return fallback
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

func transportGenericAgentUIArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload, allowRCA bool) (AiopsTransportAgentUIArtifact, bool) {
	if strings.TrimSpace(tool.DisplayKind) != "rca_report" {
		return AiopsTransportAgentUIArtifact{}, false
	}
	if !allowRCA {
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
	status := firstNonEmptyString(jsonStringValueFromMap(payload, "status"), "ok")
	inlineData := asStringAnyMap(payload["inlineData"])
	if rcaReportShouldSkip(status) {
		status = "skipped"
		if inlineData == nil {
			inlineData = map[string]any{}
		}
		skipReason := firstNonEmptyString(
			jsonStringValueFromMap(payload, "skipReason"),
			jsonStringValueFromMap(payload, "reason"),
			jsonStringValueFromMap(payload, "summaryZh"),
			jsonStringValueFromMap(payload, "summary"),
			jsonStringValueFromMap(inlineData, "skipReason"),
			jsonStringValueFromMap(inlineData, "reason"),
			jsonStringValueFromMap(inlineData, "summaryZh"),
			jsonStringValueFromMap(inlineData, "summary"),
			"coroot_mcp_unavailable",
		)
		inlineData["status"] = "skipped"
		inlineData["skipReason"] = skipReason
		metadata["skipReason"] = skipReason
	}
	return AiopsTransportAgentUIArtifact{
		ID:              "agent-ui:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "artifact"),
		Type:            "rca_report",
		Title:           jsonStringValueFromMap(payload, "title"),
		TitleZh:         firstNonEmptyString(jsonStringValueFromMap(payload, "titleZh"), "根因分析"),
		Summary:         jsonStringValueFromMap(payload, "summary"),
		SummaryZh:       jsonStringValueFromMap(payload, "summaryZh"),
		Status:          status,
		Severity:        firstNonEmptyString(jsonStringValueFromMap(payload, "severity"), "info"),
		Source:          firstNonEmptyString(jsonStringValueFromMap(payload, "source"), "aiops"),
		PermissionScope: firstNonEmptyString(jsonStringValueFromMap(payload, "permissionScope"), "read"),
		RedactionStatus: firstNonEmptyString(jsonStringValueFromMap(payload, "redactionStatus"), "redacted"),
		InlineData:      inlineData,
		Metadata:        metadata,
		Actions:         asStringAnyMapList(payload["actions"]),
		CreatedAt:       firstNonEmptyString(jsonStringValueFromMap(payload, "createdAt"), now),
		UpdatedAt:       firstNonEmptyString(jsonStringValueFromMap(payload, "updatedAt"), now),
	}, true
}

func rcaReportShouldSkip(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "skipped", "unavailable", "not_configured", "timeout", "empty_data":
		return true
	default:
		return false
	}
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

func transportWorkflowEditorArtifactFromToolPayload(turnID, itemID string, tool transportToolPayload) (AiopsTransportAgentUIArtifact, bool) {
	displayKind := strings.TrimSpace(tool.DisplayKind)
	if !isWorkflowEditorCardType(displayKind) {
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
	status := firstNonEmptyString(jsonStringValueFromMap(payload, "status"), jsonStringValueFromMap(payload, "effect_status"), "ready")
	return AiopsTransportAgentUIArtifact{
		ID:              "workflow-editor:" + turnID + ":" + firstNonEmptyString(strings.TrimSpace(itemID), "artifact"),
		Type:            displayKind,
		Title:           firstNonEmptyString(jsonStringValueFromMap(payload, "title"), workflowEditorCardTitle(displayKind)),
		Summary:         firstNonEmptyString(jsonStringValueFromMap(payload, "summary"), jsonStringValueFromMap(payload, "description")),
		Status:          status,
		Severity:        firstNonEmptyString(jsonStringValueFromMap(payload, "severity"), "info"),
		Source:          "workflow_editor",
		PermissionScope: firstNonEmptyString(jsonStringValueFromMap(payload, "permissionScope"), "draft"),
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         asStringAnyMapList(payload["actions"]),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func isWorkflowEditorCardType(value string) bool {
	switch strings.TrimSpace(value) {
	case "workflow_context",
		"workflow_edit_plan",
		"workflow_patch_preview",
		"workflow_patch_apply",
		"workflow_patch_result",
		"workflow_patch_validation",
		"workflow_conflict",
		"workflow_manual_candidate",
		"workflow_tool_timeline":
		return true
	default:
		return false
	}
}

func workflowEditorCardTitle(value string) string {
	switch value {
	case "workflow_edit_plan":
		return "Workflow edit plan"
	case "workflow_patch_preview":
		return "Workflow patch preview"
	case "workflow_patch_result":
		return "Workflow patch result"
	case "workflow_manual_candidate":
		return "Workflow manual candidate"
	default:
		return "Workflow AI"
	}
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
	case strings.Contains(kind, "browser.search"), transportWebLookupToolName(name):
		return AiopsTransportProcessKindSearch
	case kind == "command", name == "exec_command":
		return AiopsTransportProcessKindCommand
	case strings.HasPrefix(kind, "file"), strings.Contains(name, "file"):
		return AiopsTransportProcessKindFile
	default:
		return AiopsTransportProcessKindTool
	}
}

func applyTransportFoldGroup(turnID string, block *AiopsProcessBlock) {
	if block == nil {
		return
	}
	switch transportFoldGroupKind(*block) {
	case "web_lookup":
		block.FoldGroupKind = "web_lookup"
		block.FoldGroupID = TransportProcessBlockStableID(turnID, "fold", "web_lookup")
	case "command":
		block.FoldGroupKind = "command"
		block.FoldGroupID = TransportProcessBlockStableID(turnID, "fold", "command")
	}
}

func transportFoldGroupKind(block AiopsProcessBlock) string {
	switch block.Kind {
	case AiopsTransportProcessKindSearch:
		return "web_lookup"
	case AiopsTransportProcessKindCommand:
		return "command"
	default:
		return ""
	}
}

func transportWebLookupToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "web_search", "browse_url", "browser.open", "browser.find", "web.open", "web.find":
		return true
	default:
		return false
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
		if query != "" && !transportToolSummaryLooksGeneric(tool.ToolName, query) {
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

type commandTerminalOutputEnvelope struct {
	Status   string `json:"status"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	ExitCode *int   `json:"exitCode"`
}

func commandTerminalOutputPreview(outputPreview string, exitCode *int, status AiopsTransportProcessStatus) string {
	outputPreview = strings.TrimSpace(outputPreview)
	if outputPreview == "" {
		return ""
	}
	envelope, ok := decodeCommandTerminalOutputEnvelope(outputPreview)
	if !ok {
		return outputPreview
	}
	failed := status == AiopsTransportProcessStatusFailed ||
		status == AiopsTransportProcessStatusRejected ||
		(exitCode != nil && *exitCode != 0) ||
		(envelope.ExitCode != nil && *envelope.ExitCode != 0) ||
		commandTerminalStatusFailed(envelope.Status)
	if failed {
		return firstNonEmptyString(envelope.Stderr, envelope.Error, envelope.Stdout, envelope.Output)
	}
	return firstNonEmptyString(envelope.Stdout, envelope.Output, envelope.Stderr, envelope.Error)
}

func decodeCommandTerminalOutputEnvelope(outputPreview string) (commandTerminalOutputEnvelope, bool) {
	var envelope commandTerminalOutputEnvelope
	if err := json.Unmarshal([]byte(outputPreview), &envelope); err == nil && commandTerminalOutputEnvelopeHasStreams(envelope) {
		return envelope, true
	}
	var nested string
	if err := json.Unmarshal([]byte(outputPreview), &nested); err == nil {
		nested = strings.TrimSpace(nested)
		if strings.HasPrefix(nested, "{") {
			return decodeCommandTerminalOutputEnvelope(nested)
		}
	}
	return commandTerminalOutputEnvelope{}, false
}

func commandTerminalOutputEnvelopeHasStreams(envelope commandTerminalOutputEnvelope) bool {
	return strings.TrimSpace(envelope.Stdout) != "" ||
		strings.TrimSpace(envelope.Stderr) != "" ||
		strings.TrimSpace(envelope.Output) != "" ||
		strings.TrimSpace(envelope.Error) != "" ||
		envelope.ExitCode != nil ||
		strings.TrimSpace(envelope.Status) != ""
}

func commandTerminalStatusFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure":
		return true
	default:
		return false
	}
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
		Operation string `json:"operation"`
		Content   string `json:"content"`
		Query     string `json:"query"`
		URL       string `json:"url"`
	}
	if json.Unmarshal([]byte(text), &payload) == nil {
		if strings.EqualFold(strings.TrimSpace(payload.Operation), "open") {
			if url := strings.TrimSpace(payload.URL); url != "" {
				return url
			}
		}
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
	if content = sanitizeUserVisibleProcessText(content); content == "" {
		return ""
	}
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

func sanitizeUserVisibleProcessText(content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	for _, marker := range []string{
		"verification completion gate",
		"block_success_final",
		"missing_verification_report",
		"execution_required,missing_verification_report",
	} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return text
}

const modelConnectionTimeoutUserVisibleText = "模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。"

func sanitizeUserVisibleRuntimeErrorText(content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	if isRawModelConnectionTimeoutText(text) {
		return modelConnectionTimeoutUserVisibleText
	}
	if isRawRuntimeStreamFailureText(text) {
		return "模型流中断，已保留已生成内容"
	}
	return sanitizeUserVisibleProcessText(text)
}

func isRawModelConnectionTimeoutText(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if !transportContainsAny(normalized, "timeout", "timed out", "超时") {
		return false
	}
	return transportContainsAny(normalized,
		"模型请求超时",
		"模型服务连接超时",
		"tls handshake timeout",
		"chat/completions",
		"llm 地址",
		"llm address",
	)
}

func isRawRuntimeStreamFailureText(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return transportContainsAny(normalized,
		"failed to receive stream chunk",
		"context deadline exceeded",
		"unexpected eof",
		"stream chunk",
		"upstream request timeout",
	)
}

func sanitizeUserVisibleFinalAnswerText(content string) string {
	text := sanitizeUserVisibleProcessText(content)
	if text == "" {
		return ""
	}
	return redactRiskyFinalAnswerOperations(text)
}

func redactRiskyFinalAnswerOperations(text string) string {
	lines := strings.Split(text, "\n")
	safe := make([]string, 0, len(lines))
	redacted := false
	for _, line := range lines {
		if isRiskyFinalAnswerOperationLine(line) {
			redacted = true
			continue
		}
		safe = append(safe, line)
	}
	if !redacted {
		return text
	}
	safeText := strings.TrimSpace(strings.Join(safe, "\n"))
	if safeText == "" {
		return ""
	}
	return safeText
}

func isRiskyFinalAnswerOperationLine(line string) bool {
	text := strings.ToLower(strings.TrimSpace(line))
	if text == "" {
		return false
	}
	if strings.Contains(text, "rm -rf") {
		return true
	}
	if isGatedOrAnalyticalFinalAnswerLine(text) {
		return false
	}
	if runtimekernel.EvaluateRiskyOperationalAdvice(line).RequiresEvidenceGate {
		return hasDirectRiskyOperationLeadIn(text) || looksLikeStandaloneRiskyOperation(text)
	}
	return transportContainsAny(text, "删除", "清理", "清空", "delete") &&
		transportContainsAny(text, "archive", "wal", "pgdata", "$pgdata", "$pg_data", "数据目录", "归档") &&
		hasDirectRiskyOperationLeadIn(text)
}

func isGatedOrAnalyticalFinalAnswerLine(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	if transportContainsAny(text,
		"结论", "根因", "原因", "机制", "路径", "表明", "说明", "可能", "假设", "推断",
		"证据", "边界", "缺失", "只读", "不做任何变更", "候选", "影响面",
		"切勿", "不要", "不能", "不可", "未验证", "无法确认",
	) && !hasDirectRiskyOperationLeadIn(text) {
		return true
	}
	if transportContainsAny(text,
		"确认根因后", "若需修复", "需要修复", "必须选定", "变更窗口", "维护窗口",
		"审批", "批准", "备份", "回滚", "验收", "权威数据源", "authoritative",
	) {
		return true
	}
	return false
}

func hasDirectRiskyOperationLeadIn(text string) bool {
	return transportContainsAny(text,
		"可以执行", "建议执行", "请执行", "直接执行", "执行以下", "运行以下",
		"执行命令", "运行命令", "run ", "execute ", "directly run", "directly execute",
		"直接清空", "直接删除", "直接清理", "清空 ", "删除 ", "清理 ", "delete ",
	)
}

func looksLikeStandaloneRiskyOperation(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return false
	}
	first := strings.Trim(fields[0], "`'\"“”‘’。，,;:()[]{}<>")
	switch first {
	case "pg_basebackup", "pg_rewind", "pg_ctl":
		return true
	default:
		return false
	}
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
	payload, ok := decodeTransportSearchEnvelope(raw)
	if !ok {
		return nil
	}
	if len(payload.Results) > 0 {
		return cleanTransportSearchResults(payload.Results)
	}
	return parseTransportSearchResultsContent(payload.Content)
}

type transportSearchEnvelope struct {
	Operation   string
	Query       string
	URL         string
	Source      string
	Content     string
	Results     []AiopsSearchResult
	Backend     string
	SourceCount int
}

func decodeTransportSearchEnvelope(raw json.RawMessage) (transportSearchEnvelope, bool) {
	raw = normalizeTransportSearchResultRaw(raw)
	if len(raw) == 0 {
		return transportSearchEnvelope{}, false
	}
	var payload struct {
		Operation string              `json:"operation"`
		Query     string              `json:"query"`
		URL       string              `json:"url"`
		Source    string              `json:"source"`
		Results   []AiopsSearchResult `json:"results"`
		Content   string              `json:"content"`
		Meta      map[string]any      `json:"meta"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return transportSearchEnvelope{}, false
	}
	results := cleanTransportSearchResults(payload.Results)
	backend := ""
	if payload.Meta != nil {
		backend = jsonStringValueFromMap(payload.Meta, "backend")
	}
	return transportSearchEnvelope{
		Operation:   strings.TrimSpace(payload.Operation),
		Query:       strings.TrimSpace(payload.Query),
		URL:         strings.TrimSpace(payload.URL),
		Source:      strings.TrimSpace(payload.Source),
		Content:     strings.TrimSpace(payload.Content),
		Results:     results,
		Backend:     backend,
		SourceCount: len(results),
	}, true
}

type transportSearchRequest struct {
	Operation string
	Query     string
	URL       string
}

func decodeTransportSearchRequest(raw json.RawMessage) transportSearchRequest {
	if len(raw) == 0 {
		return transportSearchRequest{}
	}
	var payload struct {
		Operation string `json:"operation"`
		Query     string `json:"query"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return transportSearchRequest{}
	}
	return transportSearchRequest{
		Operation: strings.TrimSpace(payload.Operation),
		Query:     strings.TrimSpace(payload.Query),
		URL:       strings.TrimSpace(payload.URL),
	}
}

func normalizeTransportSearchResultRaw(raw json.RawMessage) json.RawMessage {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err == nil {
			decoded = strings.TrimSpace(decoded)
			if strings.HasPrefix(decoded, "{") || strings.HasPrefix(decoded, "[") {
				return json.RawMessage(decoded)
			}
		}
	}
	return raw
}

func cleanTransportSearchResults(results []AiopsSearchResult) []AiopsSearchResult {
	const (
		maxVisibleSearchResults       = 5
		maxVisibleSearchResultsDomain = 2
	)
	cleaned := make([]AiopsSearchResult, 0, len(results))
	seenURL := map[string]bool{}
	seenDomainTitle := map[string]bool{}
	countByDomain := map[string]int{}
	for _, result := range results {
		item := AiopsSearchResult{
			Title:       truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.Title)), transportAgentItemSummaryByteBudget),
			URL:         truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.URL)), transportAgentItemSummaryByteBudget),
			Snippet:     truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.Snippet)), transportAgentItemSummaryByteBudget),
			Text:        truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.Text)), transportAgentItemSummaryByteBudget),
			Fetched:     result.Fetched,
			FetchError:  truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.FetchError)), transportAgentItemSummaryByteBudget),
			ContentType: truncateTransportAgentText(redactTransportAgentText(strings.TrimSpace(result.ContentType)), transportAgentItemSummaryByteBudget),
		}
		if item.Title == "" && item.URL == "" && item.Snippet == "" && item.Text == "" {
			continue
		}
		normalizedURL := normalizeSearchResultURL(item.URL)
		if normalizedURL != "" {
			if seenURL[normalizedURL] {
				continue
			}
			seenURL[normalizedURL] = true
		}
		domain := searchResultDomain(item.URL)
		if domainTitleKey := domainTitleSearchResultKey(domain, item.Title); domainTitleKey != "" {
			if seenDomainTitle[domainTitleKey] {
				continue
			}
			seenDomainTitle[domainTitleKey] = true
		}
		if domain != "" && countByDomain[domain] >= maxVisibleSearchResultsDomain {
			continue
		}
		if domain != "" {
			countByDomain[domain]++
		}
		cleaned = append(cleaned, item)
		if len(cleaned) >= maxVisibleSearchResults {
			break
		}
	}
	return cleaned
}

func normalizeSearchResultURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(raw)
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

func searchResultDomain(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
}

func domainTitleSearchResultKey(domain, title string) string {
	title = strings.ToLower(strings.Join(strings.Fields(title), " "))
	if domain == "" || title == "" {
		return ""
	}
	return domain + "\x00" + title
}

var transportSearchResultTitlePattern = regexp.MustCompile(`^\s*\d+\.\s+(.+?)\s*$`)
var transportSearchResultURLPattern = regexp.MustCompile(`^\s*URL:\s*(https?://\S+)\s*$`)
var transportSearchResultSnippetPattern = regexp.MustCompile(`^\s*Snippet:\s*(.+?)\s*$`)

func parseTransportSearchResultsContent(content string) []AiopsSearchResult {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var results []AiopsSearchResult
	var current *AiopsSearchResult
	flush := func() {
		if current == nil {
			return
		}
		cleaned := cleanTransportSearchResults([]AiopsSearchResult{*current})
		if len(cleaned) > 0 {
			results = append(results, cleaned[0])
		}
		current = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := transportSearchResultTitlePattern.FindStringSubmatch(line); len(match) == 2 {
			flush()
			current = &AiopsSearchResult{Title: strings.TrimSpace(match[1])}
			continue
		}
		if current == nil {
			continue
		}
		if match := transportSearchResultURLPattern.FindStringSubmatch(line); len(match) == 2 {
			current.URL = strings.TrimSpace(match[1])
			continue
		}
		if match := transportSearchResultSnippetPattern.FindStringSubmatch(line); len(match) == 2 {
			current.Snippet = strings.TrimSpace(match[1])
			continue
		}
		if current.Snippet != "" {
			current.Snippet = strings.TrimSpace(current.Snippet + " " + line)
		}
	}
	flush()
	if len(results) > 8 {
		return results[:8]
	}
	return results
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

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
