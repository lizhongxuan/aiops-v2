package appui

import (
	"strings"
	"time"
)

func BuildAgentRunViewFromTrace(trace ChatRunTraceView) *AgentRunView {
	id := strings.TrimSpace(trace.ID)
	if id == "" {
		return nil
	}
	turnID := strings.TrimSpace(trace.TurnID)
	title := strings.TrimSpace(trace.Title)
	return &AgentRunView{
		ID:             id,
		SessionID:      strings.TrimSpace(trace.SessionID),
		RootTurnID:     turnID,
		ActiveTurnID:   turnID,
		UserGoal:       title,
		NormalizedGoal: title,
		RouteMode:      strings.TrimSpace(trace.RouteMode),
		Status:         mapAgentRunStatus(trace.Status),
		TargetSummary:  strings.TrimSpace(trace.TargetSummary),
		CurrentStep:    strings.TrimSpace(trace.CurrentStep),
		EvidenceCount:  trace.EvidenceCount,
	}
}

func BuildAgentRunViewFromTransportState(state AiopsTransportState) *AgentRunView {
	if state.OpsRun == nil {
		return nil
	}
	turnID := firstNonEmptyString(strings.TrimSpace(state.OpsRun.TurnID), strings.TrimSpace(state.CurrentTurnID))
	turn := state.Turns[turnID]
	if strings.TrimSpace(turn.ID) == "" && len(state.Turns) == 1 {
		for _, candidate := range state.Turns {
			turn = candidate
			break
		}
	}
	return buildAgentRunViewFromOpsRunAndTurn(*state.OpsRun, turn, 0)
}

func BuildAgentStepViewFromProcessBlock(block AiopsProcessBlock, runID string, turnID string, iteration int, checkpointID string) AgentStepView {
	kind := mapAgentStepKind(block)
	status := mapAgentStepStatus(block.Status)
	title := firstNonEmptyString(strings.TrimSpace(block.Text), strings.TrimSpace(block.InputSummary), strings.TrimSpace(block.Source))
	step := AgentStepView{
		ID:            strings.TrimSpace(block.ID),
		RunID:         strings.TrimSpace(runID),
		TurnID:        strings.TrimSpace(turnID),
		Iteration:     iteration,
		Kind:          kind,
		Status:        status,
		Title:         title,
		InputSummary:  strings.TrimSpace(block.InputSummary),
		OutputSummary: strings.TrimSpace(block.OutputPreview),
		ToolName:      strings.TrimSpace(block.Source),
		ToolCallID:    strings.TrimSpace(block.ToolCallID),
		ApprovalID:    strings.TrimSpace(block.ApprovalID),
		CheckpointID:  firstNonEmptyString(strings.TrimSpace(block.CheckpointID), strings.TrimSpace(checkpointID)),
		TargetRefs:    targetRefsFromSummary(block.TargetSummary),
		EvidenceRefs:  cleanTransportStringList(block.EvidenceRefs),
		StartedAt:     parseTransportTimestamp(block.UpdatedAt),
	}
	if kind == AgentStepKindFinalResponse && step.OutputSummary == "" {
		step.OutputSummary = title
	}
	if status == AgentStepStatusCompleted || status == AgentStepStatusFailed || status == AgentStepStatusCancelled || status == AgentStepStatusSkipped {
		step.CompletedAt = parseTransportTimestamp(block.UpdatedAt)
	}
	if status == AgentStepStatusFailed {
		step.Error = firstNonEmptyString(strings.TrimSpace(block.OutputPreview), title)
	}
	return step
}

func buildAgentRunViewFromOpsRunAndTurn(opsRun AiopsTransportOpsRun, turn AiopsTransportTurn, iteration int) *AgentRunView {
	id := strings.TrimSpace(opsRun.ID)
	if id == "" {
		return nil
	}
	turnID := firstNonEmptyString(strings.TrimSpace(opsRun.TurnID), strings.TrimSpace(turn.ID))
	userGoal := firstNonEmptyString(transportTurnUserText(turn), strings.TrimSpace(opsRun.Title))
	steps := make([]AgentStepView, 0, len(turn.Process))
	for _, block := range turn.Process {
		if strings.TrimSpace(block.ID) == "" && strings.TrimSpace(block.Text) == "" {
			continue
		}
		steps = append(steps, BuildAgentStepViewFromProcessBlock(block, id, turnID, iteration, opsRun.CheckpointID))
	}
	steps = appendAgentCheckpointSteps(steps, opsRun, turnID, iteration)
	currentStepID := firstNonEmptyString(strings.TrimSpace(opsRun.CurrentStepID), currentAgentStepID(steps))
	return &AgentRunView{
		ID:             id,
		SessionID:      strings.TrimSpace(opsRun.SessionID),
		RootTurnID:     turnID,
		ActiveTurnID:   turnID,
		UserGoal:       userGoal,
		NormalizedGoal: firstNonEmptyString(strings.TrimSpace(opsRun.Title), userGoal),
		RouteMode:      strings.TrimSpace(opsRun.RouteMode),
		Status:         mapAgentRunStatus(opsRun.Status),
		TargetSummary:  strings.TrimSpace(opsRun.TargetSummary),
		CurrentStep:    strings.TrimSpace(opsRun.CurrentStep),
		CurrentStepID:  currentStepID,
		CheckpointID:   strings.TrimSpace(opsRun.CheckpointID),
		EvidenceCount:  firstPositiveInt(opsRun.EvidenceCount, countProjectedEvidenceRefs(turn)),
		StartedAt:      parseTransportTimestamp(turn.StartedAt),
		UpdatedAt:      parseTransportTimestamp(turn.UpdatedAt),
		Steps:          steps,
	}
}

func appendAgentCheckpointSteps(steps []AgentStepView, opsRun AiopsTransportOpsRun, turnID string, iteration int) []AgentStepView {
	checkpointID := strings.TrimSpace(opsRun.CheckpointID)
	if checkpointID == "" {
		return steps
	}
	for _, step := range steps {
		if strings.TrimSpace(step.CheckpointID) == checkpointID {
			return steps
		}
	}
	status := AgentStepStatusRunning
	if strings.EqualFold(strings.TrimSpace(opsRun.Status), string(AiopsTransportTurnStatusBlocked)) {
		status = AgentStepStatusWaitingApproval
	}
	title := firstNonEmptyString(strings.TrimSpace(opsRun.CurrentStep), "等待外部确认")
	steps = append(steps, AgentStepView{
		ID:           "step:" + checkpointID,
		RunID:        strings.TrimSpace(opsRun.ID),
		TurnID:       strings.TrimSpace(turnID),
		Iteration:    iteration,
		Kind:         AgentStepKindCheckpoint,
		Status:       status,
		Title:        title,
		CheckpointID: checkpointID,
		TargetRefs:   targetRefsFromSummary(opsRun.TargetSummary),
	})
	return steps
}

func mapAgentRunStatus(status string) AgentRunStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "submitted", "pending", "queued":
		return AgentRunStatusPending
	case "completed", "complete", "done":
		return AgentRunStatusCompleted
	case "failed", "error":
		return AgentRunStatusFailed
	case "canceled", "cancelled", "rejected":
		return AgentRunStatusCancelled
	default:
		return AgentRunStatusRunning
	}
}

func mapAgentStepKind(block AiopsProcessBlock) AgentStepKind {
	if block.Status == AiopsTransportProcessStatusFailed && block.Kind == AiopsTransportProcessKindSystem {
		return AgentStepKindError
	}
	switch block.Kind {
	case AiopsTransportProcessKindSearch:
		return AgentStepKindToolSearch
	case AiopsTransportProcessKindCommand, AiopsTransportProcessKindFile, AiopsTransportProcessKindTool, AiopsTransportProcessKindSubagent:
		return AgentStepKindToolCall
	case AiopsTransportProcessKindApproval:
		return AgentStepKindApproval
	case AiopsTransportProcessKindMCP:
		return AgentStepKindMCPHealth
	case AiopsTransportProcessKindEvidence:
		return AgentStepKindEvidence
	case AiopsTransportProcessKindAssistant:
		return AgentStepKindFinalResponse
	case AiopsTransportProcessKindSystem:
		if strings.Contains(strings.ToLower(strings.TrimSpace(block.DisplayKind)), "checkpoint") {
			return AgentStepKindCheckpoint
		}
		return AgentStepKindReasoning
	default:
		return AgentStepKindReasoning
	}
}

func mapAgentStepStatus(status AiopsTransportProcessStatus) AgentStepStatus {
	switch status {
	case AiopsTransportProcessStatusQueued:
		return AgentStepStatusPending
	case AiopsTransportProcessStatusRunning:
		return AgentStepStatusRunning
	case AiopsTransportProcessStatusCompleted:
		return AgentStepStatusCompleted
	case AiopsTransportProcessStatusFailed:
		return AgentStepStatusFailed
	case AiopsTransportProcessStatusRejected:
		return AgentStepStatusCancelled
	case AiopsTransportProcessStatusBlocked:
		return AgentStepStatusWaitingApproval
	case AiopsTransportProcessStatusSkipped:
		return AgentStepStatusSkipped
	default:
		return AgentStepStatusPending
	}
}

func currentAgentStepID(steps []AgentStepView) string {
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i].Status {
		case AgentStepStatusRunning, AgentStepStatusWaitingApproval:
			return strings.TrimSpace(steps[i].ID)
		}
	}
	if len(steps) > 0 {
		return strings.TrimSpace(steps[len(steps)-1].ID)
	}
	return ""
}

func targetRefsFromSummary(summary string) []string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	replacer := strings.NewReplacer("；", "\n", ";", "\n", "，", "\n", ",", "\n")
	return cleanTransportStringList(strings.Fields(replacer.Replace(summary)))
}

func parseTransportTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	return time.Time{}
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
