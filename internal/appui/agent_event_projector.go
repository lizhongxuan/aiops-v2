package appui

import (
	"encoding/json"
	"fmt"
	"strings"
)

type AgentEventProjector struct{}

func NewAgentEventProjector() *AgentEventProjector {
	return &AgentEventProjector{}
}

func (p *AgentEventProjector) Apply(proj AgentEventProjection, event AgentEvent) (AgentEventProjection, error) {
	if err := event.Validate(); err != nil {
		return proj, err
	}
	if proj.SessionID == "" {
		proj.SessionID = event.SessionID
	}
	if proj.SessionID != event.SessionID {
		return proj, fmt.Errorf("event session %q does not match projection session %q", event.SessionID, proj.SessionID)
	}
	if proj.ThreadID == "" {
		proj.ThreadID = event.ThreadID
	}
	if shouldAdoptCurrentTurn(proj, event) {
		proj.CurrentTurnID = event.TurnID
	}
	if event.Seq > proj.LastSeq {
		proj.LastSeq = event.Seq
	}
	proj = ensureAgentEventProjection(proj)

	switch event.Kind {
	case AgentEventTurn:
		proj = applyTurnAgentEventToProjection(proj, event)
	case AgentEventAgent:
		proj = applyAgentEventToProjection(proj, event)
	case AgentEventTool:
		proj = applyToolEventToProjection(proj, event)
	case AgentEventApproval:
		proj = applyApprovalEventToProjection(proj, event)
	case AgentEventAssistant:
		proj = applyAssistantEventToProjection(proj, event)
	case AgentEventPlan:
		proj = applyPlanEventToProjection(proj, event)
	case AgentEventEvidence:
		proj = applyEvidenceEventToProjection(proj, event)
	case AgentEventReasoning:
		proj = applyReasoningEventToProjection(proj, event)
	case AgentEventSystem:
		proj = applySystemEventToProjection(proj, event)
	case AgentEventDiff:
		proj = applyDiffEventToProjection(proj, event)
	case AgentEventArtifact:
		proj = applyArtifactEventToProjection(proj, event)
	}

	proj.Status = deriveProjectionStatus(proj.RuntimeLiveness, proj.Diff, proj.LastTerminalFailed)
	return proj, nil
}

func shouldAdoptCurrentTurn(proj AgentEventProjection, event AgentEvent) bool {
	if event.TurnID == "" || event.Kind != AgentEventTurn {
		return false
	}
	switch event.Phase {
	case AgentEventPhaseRequested, AgentEventPhaseStarted, AgentEventPhaseUpdated, AgentEventPhaseDelta, AgentEventPhaseBlocked:
		return true
	default:
		return proj.CurrentTurnID == ""
	}
}

func (p *AgentEventProjector) Replay(sessionID string, events []AgentEvent) (AgentEventProjection, error) {
	proj := ensureAgentEventProjection(AgentEventProjection{
		SessionID: sessionID,
		Status:    "idle",
	})
	for _, event := range events {
		next, err := p.Apply(proj, event)
		if err != nil {
			return proj, err
		}
		proj = next
	}
	proj.Status = deriveProjectionStatus(proj.RuntimeLiveness, proj.Diff, proj.LastTerminalFailed)
	return proj, nil
}

func deriveProjectionStatus(l RuntimeLiveness, diff *DiffProjection, lastTerminalFailed bool) string {
	if hasAny(l.PendingApprovals) || hasAny(l.PendingUserInputs) {
		return "blocked"
	}
	if hasAny(l.ActiveTurns) || hasAny(l.ActiveAgents) || hasAny(l.ActiveCommandStreams) {
		return "working"
	}
	if lastTerminalFailed {
		return "failed"
	}
	if diff != nil {
		return "reviewing"
	}
	return "idle"
}

func ensureAgentEventProjection(proj AgentEventProjection) AgentEventProjection {
	proj.RuntimeLiveness = ensureRuntimeLiveness(proj.RuntimeLiveness)
	proj.FinalMessages = cloneAssistantFinalMap(proj.FinalMessages)
	proj.ProcessGroups = cloneProcessGroups(proj.ProcessGroups)
	if proj.Status == "" {
		proj.Status = "idle"
	}
	return proj
}

func ensureRuntimeLiveness(l RuntimeLiveness) RuntimeLiveness {
	return RuntimeLiveness{
		ActiveTurns:          cloneBoolMap(l.ActiveTurns),
		ActiveAgents:         cloneBoolMap(l.ActiveAgents),
		PendingApprovals:     cloneBoolMap(l.PendingApprovals),
		PendingUserInputs:    cloneBoolMap(l.PendingUserInputs),
		ActiveCommandStreams: cloneBoolMap(l.ActiveCommandStreams),
	}
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneAssistantFinalMap(values map[string]AssistantFinal) map[string]AssistantFinal {
	out := make(map[string]AssistantFinal, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneProcessGroups(values map[string][]TimelineEntry) map[string][]TimelineEntry {
	out := make(map[string][]TimelineEntry, len(values))
	for key, value := range values {
		out[key] = append([]TimelineEntry(nil), value...)
	}
	return out
}

func applyTurnAgentEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload TurnPayload
	decodeAgentEventPayload(event.Payload, &payload)
	rowID := resolveTurnTimelineRowID(proj, event, payload)
	title := payload.Prompt
	if title == "" && event.Phase == AgentEventPhaseRequested {
		title = payload.Title
	}
	summary := payload.Summary
	if summary == "" {
		summary = payload.Error
	}
	switch event.Phase {
	case AgentEventPhaseRequested, AgentEventPhaseStarted:
		if event.TurnID != "" {
			proj.RuntimeLiveness.ActiveTurns[event.TurnID] = true
		}
		proj.LastTerminalFailed = false
		if summary == "" {
			if event.Phase == AgentEventPhaseStarted && payload.Title != "" {
				summary = payload.Title
			} else if event.Phase == AgentEventPhaseRequested {
				summary = "正在发送请求"
			} else {
				summary = "正在等待 Agent 启动"
			}
		}
	case AgentEventPhaseCompleted, AgentEventPhaseCanceled:
		delete(proj.RuntimeLiveness.ActiveTurns, event.TurnID)
		proj.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
		proj = completeFinalMessageForTurn(proj, event)
		proj.LastTerminalFailed = false
		if summary == "" {
			if event.Phase == AgentEventPhaseCanceled {
				summary = "已停止生成"
			} else {
				summary = "已完成"
			}
		}
	case AgentEventPhaseFailed:
		delete(proj.RuntimeLiveness.ActiveTurns, event.TurnID)
		proj.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
		proj = completeFinalMessageForTurn(proj, event)
		proj.LastTerminalFailed = true
		if summary == "" {
			summary = "请求失败"
		}
	}
	if rowID != "" {
		proj.Timeline = upsertTimelineEntry(proj.Timeline, TimelineEntry{
			ID:         rowID,
			Kind:       event.Kind,
			TurnID:     event.TurnID,
			AgentID:    event.AgentID,
			Phase:      event.Phase,
			Status:     event.Status,
			Visibility: event.Visibility,
			Title:      title,
			Summary:    summary,
			UpdatedAt:  event.CreatedAt,
			Seq:        event.Seq,
		})
	}
	return proj
}

func completeFinalMessageForTurn(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	if event.TurnID == "" || proj.FinalMessages == nil {
		return proj
	}
	final, ok := proj.FinalMessages[event.TurnID]
	if !ok {
		return proj
	}
	final.Status = event.Status
	final.UpdatedAt = event.CreatedAt
	proj.FinalMessages[event.TurnID] = final
	return proj
}

func resolveTurnTimelineRowID(proj AgentEventProjection, event AgentEvent, payload TurnPayload) string {
	if payload.ClientMessageID != "" {
		return payload.ClientMessageID
	}
	if event.TurnID != "" {
		for _, row := range proj.Timeline {
			if row.Kind == AgentEventTurn && row.TurnID == event.TurnID && row.ID != "" {
				return row.ID
			}
		}
	}
	if event.ClientTurnID != "" {
		return event.ClientTurnID
	}
	if payload.ClientTurnID != "" {
		return payload.ClientTurnID
	}
	return event.TurnID
}

func applyAgentEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload AgentPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := event.AgentID
	if id == "" {
		id = event.EventID
	}
	row := AgentProjection{
		ID:          id,
		Handle:      payload.Handle,
		Name:        payload.Name,
		Role:        payload.Role,
		Status:      event.Status,
		LastAction:  payload.LastAction,
		LastSummary: payload.LastSummary,
		Stats:       payload.Stats,
		StartedAt:   event.StartedAt,
		CompletedAt: event.CompletedAt,
	}
	proj.Agents = upsertAgentProjection(proj.Agents, row)
	switch event.Phase {
	case AgentEventPhaseStarted, AgentEventPhaseUpdated, AgentEventPhaseDelta:
		proj.RuntimeLiveness.ActiveAgents[id] = true
	case AgentEventPhaseCompleted, AgentEventPhaseFailed, AgentEventPhaseCanceled:
		delete(proj.RuntimeLiveness.ActiveAgents, id)
	}
	return proj
}

func applyToolEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload ToolPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ToolCallID
	if id == "" {
		id = event.EventID
	}
	title := payload.DisplayName
	if payload.Title != "" {
		title = payload.Title
	} else if title == "" {
		title = payload.ToolName
	}
	summary := toolProjectionSummary(payload)
	row := TimelineEntry{
		ID:           id,
		Kind:         event.Kind,
		TurnID:       event.TurnID,
		AgentID:      event.AgentID,
		ToolCallID:   id,
		DisplayKind:  payload.DisplayKind,
		Phase:        event.Phase,
		Status:       event.Status,
		Visibility:   event.Visibility,
		Title:        title,
		Summary:      summary,
		Risk:         payload.Risk,
		RawRef:       payload.RawRef,
		Foldable:     payload.Foldable,
		AutoCollapse: payload.AutoCollapse,
		Collapsed:    payload.AutoCollapse && event.Status == AgentEventStatusCompleted,
		DurationMs:   payload.DurationMs,
		UpdatedAt:    event.CreatedAt,
		Seq:          event.Seq,
	}
	proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
	if event.TurnID != "" {
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
	}
	switch event.Phase {
	case AgentEventPhaseStarted, AgentEventPhaseUpdated, AgentEventPhaseDelta:
		proj.RuntimeLiveness.ActiveCommandStreams[id] = true
	case AgentEventPhaseCompleted, AgentEventPhaseFailed, AgentEventPhaseCanceled:
		delete(proj.RuntimeLiveness.ActiveCommandStreams, id)
	}
	return proj
}

func applyPlanEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload PlanPayload
	decodeAgentEventPayload(event.Payload, &payload)
	steps := normalizePlanStepsForProjection(payload.Steps)
	id := event.EventID
	if event.TurnID != "" {
		id = event.TurnID + ":plan"
	}
	summary := ""
	for _, step := range steps {
		if step.Status == "running" {
			summary = step.Text
			break
		}
	}
	if summary == "" && len(steps) > 0 {
		summary = steps[len(steps)-1].Text
	}
	row := TimelineEntry{
		ID:           id,
		Kind:         event.Kind,
		TurnID:       event.TurnID,
		AgentID:      event.AgentID,
		DisplayKind:  "plan",
		Phase:        event.Phase,
		Status:       event.Status,
		Visibility:   event.Visibility,
		Title:        payload.Title,
		Summary:      summary,
		Steps:        steps,
		Foldable:     true,
		AutoCollapse: event.Status == AgentEventStatusCompleted,
		Collapsed:    event.Status == AgentEventStatusCompleted,
		UpdatedAt:    event.CreatedAt,
		Seq:          event.Seq,
	}
	proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
	if event.TurnID != "" {
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
	}
	return proj
}

func normalizePlanStepsForProjection(steps []PlanStep) []PlanStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]PlanStep, 0, len(steps))
	runningSeen := false
	for _, step := range steps {
		next := step
		status := strings.ToLower(strings.TrimSpace(next.Status))
		switch status {
		case "in_progress":
			status = "running"
		case "":
			status = "pending"
		}
		if status == "running" {
			if runningSeen {
				status = "pending"
			} else {
				runningSeen = true
			}
		}
		next.Status = status
		out = append(out, next)
	}
	return out
}

func applyEvidenceEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload EvidencePayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ID
	if id == "" {
		id = event.EventID
	}
	title := payload.Title
	if title == "" {
		title = payload.Kind
	}
	row := TimelineEntry{
		ID:          id,
		Kind:        event.Kind,
		TurnID:      event.TurnID,
		AgentID:     event.AgentID,
		DisplayKind: "evidence." + payload.Kind,
		Phase:       event.Phase,
		Status:      event.Status,
		Visibility:  event.Visibility,
		Title:       title,
		Summary:     payload.Summary,
		RawRef:      payload.RawRef,
		UpdatedAt:   event.CreatedAt,
		Seq:         event.Seq,
	}
	proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
	if event.TurnID != "" {
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
	}
	return proj
}

func applyReasoningEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload ReasoningPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ItemID
	if id == "" {
		id = event.EventID
	}
	title := "正在思考"
	if event.Phase == AgentEventPhaseCompleted || event.Status == AgentEventStatusCompleted {
		title = "思考摘要"
	}
	summary := payload.Summary
	if summary == "" {
		summary = payload.Delta
	}
	foldable := payload.Foldable || event.Phase == AgentEventPhaseCompleted || event.Status == AgentEventStatusCompleted
	autoCollapse := payload.AutoCollapse || event.Phase == AgentEventPhaseCompleted || event.Status == AgentEventStatusCompleted
	row := TimelineEntry{
		ID:           id,
		Kind:         event.Kind,
		TurnID:       event.TurnID,
		AgentID:      event.AgentID,
		DisplayKind:  "reasoning.summary",
		Phase:        event.Phase,
		Status:       event.Status,
		Visibility:   event.Visibility,
		Title:        title,
		Summary:      summary,
		Foldable:     foldable,
		AutoCollapse: autoCollapse,
		Collapsed:    autoCollapse && event.Status == AgentEventStatusCompleted,
		UpdatedAt:    event.CreatedAt,
		Seq:          event.Seq,
	}
	proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
	if event.TurnID != "" {
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
	}
	return proj
}

func applySystemEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload SystemPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ID
	if id == "" {
		id = event.EventID
	}
	row := TimelineEntry{
		ID:          id,
		Kind:        event.Kind,
		TurnID:      event.TurnID,
		AgentID:     event.AgentID,
		DisplayKind: payload.DisplayKind,
		Phase:       event.Phase,
		Status:      event.Status,
		Visibility:  event.Visibility,
		Title:       payload.Title,
		Summary:     payload.Summary,
		Detail:      payload.Detail,
		UpdatedAt:   event.CreatedAt,
		Seq:         event.Seq,
	}
	proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
	if event.TurnID != "" {
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
	}
	return proj
}

func toolProjectionSummary(payload ToolPayload) string {
	if payload.Error != "" {
		if payload.InputSummary != "" {
			return payload.InputSummary + ": " + payload.Error
		}
		return payload.Error
	}
	if payload.OutputSummary != "" {
		return payload.OutputSummary
	}
	return payload.InputSummary
}

func applyApprovalEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload ApprovalPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ApprovalID
	if id == "" {
		id = event.EventID
	}
	row := ApprovalProjection{
		ID:           id,
		ApprovalType: payload.ApprovalType,
		Title:        payload.Title,
		Reason:       payload.Reason,
		Risk:         payload.Risk,
		Decision:     payload.Decision,
		Targets:      append([]string(nil), payload.Targets...),
		Status:       event.Status,
		UpdatedAt:    event.CreatedAt,
	}
	proj.Approvals = upsertApprovalProjection(proj.Approvals, row)
	switch event.Phase {
	case AgentEventPhaseRequested, AgentEventPhaseBlocked:
		proj.RuntimeLiveness.PendingApprovals[id] = true
		if payload.ApprovalType == "user_input" || payload.ApprovalType == "ask_user" {
			proj.RuntimeLiveness.PendingUserInputs[id] = true
		}
	case AgentEventPhaseResolved, AgentEventPhaseCompleted, AgentEventPhaseCanceled:
		delete(proj.RuntimeLiveness.PendingApprovals, id)
		delete(proj.RuntimeLiveness.PendingUserInputs, id)
	}
	return proj
}

func applyAssistantEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload AssistantPayload
	decodeAgentEventPayload(event.Payload, &payload)
	if payload.Channel == "intent" || payload.Channel == "summary" {
		text := payload.Text
		if text == "" {
			text = payload.Delta
		}
		if text == "" || event.TurnID == "" {
			return proj
		}
		id := payload.MessageID
		if id == "" {
			id = event.EventID
		}
		row := TimelineEntry{
			ID:         id,
			Kind:       event.Kind,
			TurnID:     event.TurnID,
			AgentID:    event.AgentID,
			Phase:      event.Phase,
			Status:     event.Status,
			Visibility: event.Visibility,
			Title:      payload.Channel,
			Summary:    text,
			UpdatedAt:  event.CreatedAt,
			Seq:        event.Seq,
		}
		proj.Timeline = upsertTimelineEntry(proj.Timeline, row)
		proj.ProcessGroups[event.TurnID] = upsertTimelineEntry(proj.ProcessGroups[event.TurnID], row)
		return proj
	}
	if payload.Channel != "" && payload.Channel != "final" {
		return proj
	}
	text := payload.Delta
	if text == "" {
		text = payload.Text
	}
	if text == "" || event.TurnID == "" {
		return proj
	}
	final := proj.FinalMessages[event.TurnID]
	final.TurnID = event.TurnID
	final.Text += text
	final.Status = event.Status
	final.UpdatedAt = event.CreatedAt
	proj.FinalMessages[event.TurnID] = final
	return proj
}

func applyDiffEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload DiffPayload
	decodeAgentEventPayload(event.Payload, &payload)
	proj.Diff = &DiffProjection{
		Scope:        payload.Scope,
		Files:        append([]DiffFile(nil), payload.Files...),
		FilesCount:   payload.FilesCount,
		AddedLines:   payload.AddedLines,
		RemovedLines: payload.RemovedLines,
		Summary:      payload.Summary,
		UpdatedAt:    event.CreatedAt,
	}
	return proj
}

func applyArtifactEventToProjection(proj AgentEventProjection, event AgentEvent) AgentEventProjection {
	var payload ArtifactPayload
	decodeAgentEventPayload(event.Payload, &payload)
	id := payload.ArtifactID
	if id == "" {
		id = event.EventID
	}
	row := ArtifactProjection{
		ID:          id,
		Kind:        payload.Kind,
		Title:       payload.Title,
		Summary:     payload.Summary,
		URI:         payload.URI,
		ContentType: payload.ContentType,
		Status:      event.Status,
		UpdatedAt:   event.CreatedAt,
	}
	proj.Artifacts = upsertArtifactProjection(proj.Artifacts, row)
	return proj
}

func hasAny(values map[string]bool) bool {
	for _, active := range values {
		if active {
			return true
		}
	}
	return false
}

func decodeAgentEventPayload(raw json.RawMessage, target any) {
	if len(raw) == 0 || target == nil {
		return
	}
	_ = json.Unmarshal(raw, target)
}

func upsertTimelineEntry(list []TimelineEntry, row TimelineEntry) []TimelineEntry {
	for i := range list {
		if list[i].ID == row.ID && list[i].Kind == row.Kind {
			next := append([]TimelineEntry(nil), list...)
			next[i] = mergeTimelineEntry(next[i], row)
			return next
		}
	}
	return append(append([]TimelineEntry(nil), list...), row)
}

func mergeTimelineEntry(existing, row TimelineEntry) TimelineEntry {
	if row.Title == "" {
		row.Title = existing.Title
	}
	if row.Summary == "" {
		row.Summary = existing.Summary
	}
	if row.Detail == "" {
		row.Detail = existing.Detail
	}
	return row
}

func upsertAgentProjection(list []AgentProjection, row AgentProjection) []AgentProjection {
	for i := range list {
		if list[i].ID == row.ID {
			next := append([]AgentProjection(nil), list...)
			if row.Handle == "" {
				row.Handle = next[i].Handle
			}
			if row.Name == "" {
				row.Name = next[i].Name
			}
			if row.Role == "" {
				row.Role = next[i].Role
			}
			if row.LastAction == "" {
				row.LastAction = next[i].LastAction
			}
			if row.LastSummary == "" {
				row.LastSummary = next[i].LastSummary
			}
			if row.Stats == (AgentStats{}) {
				row.Stats = next[i].Stats
			}
			if row.StartedAt == "" {
				row.StartedAt = next[i].StartedAt
			}
			next[i] = row
			return next
		}
	}
	return append(append([]AgentProjection(nil), list...), row)
}

func upsertApprovalProjection(list []ApprovalProjection, row ApprovalProjection) []ApprovalProjection {
	for i := range list {
		if list[i].ID == row.ID {
			next := append([]ApprovalProjection(nil), list...)
			if row.Title == "" {
				row.Title = next[i].Title
			}
			if row.Reason == "" {
				row.Reason = next[i].Reason
			}
			if row.ApprovalType == "" {
				row.ApprovalType = next[i].ApprovalType
			}
			next[i] = row
			return next
		}
	}
	return append(append([]ApprovalProjection(nil), list...), row)
}

func upsertArtifactProjection(list []ArtifactProjection, row ArtifactProjection) []ArtifactProjection {
	for i := range list {
		if list[i].ID == row.ID {
			next := append([]ArtifactProjection(nil), list...)
			next[i] = row
			return next
		}
	}
	return append(append([]ArtifactProjection(nil), list...), row)
}
