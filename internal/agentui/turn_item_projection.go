package agentui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
)

// ProjectTurnItemsToAgentEvents converts shadow protocol items into AgentEvent
// records. It is intentionally side-effect free so callers can compare this
// projection with the existing event stream before switching UI paths.
func ProjectTurnItemsToAgentEvents(sessionID, turnID string, items []agentstate.TurnItem, startSeq int64) []AgentEvent {
	events := make([]AgentEvent, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		event, ok := agentEventFromTurnItem(sessionID, turnID, item)
		if !ok {
			continue
		}
		if seen[event.EventID] {
			continue
		}
		seen[event.EventID] = true
		event.Seq = startSeq + int64(len(events)) + 1
		events = append(events, event)
	}
	return events
}

func agentEventFromTurnItem(sessionID, turnID string, item agentstate.TurnItem) (AgentEvent, bool) {
	if strings.TrimSpace(item.ID) == "" || !item.Type.IsValid() || !item.Status.IsValid() {
		return AgentEvent{}, false
	}
	eventID := fmt.Sprintf("turn-item:%s:%s", turnID, item.ID)
	if item.Type == agentstate.TurnItemTypeFinalAnswer {
		eventID = fmt.Sprintf("turn-item:%s:final_answer", turnID)
	}
	event := AgentEvent{
		EventID:    eventID,
		SessionID:  sessionID,
		TurnID:     turnID,
		Kind:       eventKindForTurnItem(item.Type),
		Phase:      eventPhaseForTurnItem(item.Type, item.Status),
		Status:     eventStatusForTurnItem(item.Status),
		Visibility: eventVisibilityForTurnItem(item.Type),
		Source:     AgentEventSourceProjection,
		CreatedAt:  timestampString(item.CreatedAt),
	}
	if item.Status == agentstate.ItemStatusRunning {
		event.StartedAt = event.CreatedAt
	}
	if item.Status == agentstate.ItemStatusCompleted || item.Status == agentstate.ItemStatusFailed || item.Status == agentstate.ItemStatusCancelled {
		event.CompletedAt = timestampString(firstNonZeroTime(item.UpdatedAt, item.CreatedAt))
	}
	event.Payload = payloadForTurnItem(item)
	return event, true
}

func eventKindForTurnItem(typ agentstate.TurnItemType) AgentEventKind {
	switch typ {
	case agentstate.TurnItemTypeUserMessage:
		return AgentEventTurn
	case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
		return AgentEventTool
	case agentstate.TurnItemTypePlan:
		return AgentEventPlan
	case agentstate.TurnItemTypeApproval:
		return AgentEventApproval
	case agentstate.TurnItemTypeEvidence:
		return AgentEventEvidence
	case agentstate.TurnItemTypeFinalAnswer:
		return AgentEventAssistant
	case agentstate.TurnItemTypeError:
		return AgentEventTurn
	default:
		return AgentEventSystem
	}
}

func eventPhaseForTurnItem(typ agentstate.TurnItemType, status agentstate.ItemStatus) AgentEventPhase {
	switch status {
	case agentstate.ItemStatusBlocked:
		return AgentEventPhaseBlocked
	case agentstate.ItemStatusFailed:
		return AgentEventPhaseFailed
	case agentstate.ItemStatusCancelled:
		return AgentEventPhaseCanceled
	}
	switch typ {
	case agentstate.TurnItemTypeToolCall:
		return AgentEventPhaseStarted
	case agentstate.TurnItemTypeToolResult, agentstate.TurnItemTypeFinalAnswer:
		return AgentEventPhaseCompleted
	case agentstate.TurnItemTypePlan:
		return AgentEventPhaseUpdated
	case agentstate.TurnItemTypeApproval:
		return AgentEventPhaseRequested
	case agentstate.TurnItemTypeEvidence:
		return AgentEventPhaseCompleted
	default:
		if status == agentstate.ItemStatusRunning {
			return AgentEventPhaseStarted
		}
		return AgentEventPhaseCompleted
	}
}

func eventStatusForTurnItem(status agentstate.ItemStatus) AgentEventStatus {
	switch status {
	case agentstate.ItemStatusPending:
		return AgentEventStatusQueued
	case agentstate.ItemStatusRunning:
		return AgentEventStatusRunning
	case agentstate.ItemStatusCompleted:
		return AgentEventStatusCompleted
	case agentstate.ItemStatusBlocked:
		return AgentEventStatusBlocked
	case agentstate.ItemStatusFailed:
		return AgentEventStatusFailed
	case agentstate.ItemStatusCancelled:
		return AgentEventStatusCanceled
	default:
		return AgentEventStatusSkipped
	}
}

func eventVisibilityForTurnItem(typ agentstate.TurnItemType) AgentEventVisibility {
	switch typ {
	case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult, agentstate.TurnItemTypePlan, agentstate.TurnItemTypeApproval, agentstate.TurnItemTypeFinalAnswer:
		return AgentEventVisibilityPrimary
	default:
		return AgentEventVisibilityDebug
	}
}

func payloadForTurnItem(item agentstate.TurnItem) json.RawMessage {
	summary := strings.TrimSpace(item.Payload.Summary)
	var payload any
	switch item.Type {
	case agentstate.TurnItemTypeUserMessage:
		payload = TurnPayload{Prompt: summary, Summary: summary}
	case agentstate.TurnItemTypeToolCall:
		tool := decodeToolPayloadData(item.Payload.Data)
		if tool.ToolName == "" {
			tool.ToolName = summary
		}
		tool.InputSummary = summary
		payload = tool
	case agentstate.TurnItemTypeToolResult:
		tool := decodeToolPayloadData(item.Payload.Data)
		tool.OutputSummary = summary
		payload = tool
	case agentstate.TurnItemTypeFinalAnswer:
		payload = AssistantPayload{Text: summary, Channel: "final"}
	case agentstate.TurnItemTypePlan:
		payload = PlanPayload{Title: summary}
	case agentstate.TurnItemTypeApproval:
		payload = ApprovalPayload{Title: summary, Reason: summary}
	case agentstate.TurnItemTypeEvidence:
		payload = EvidencePayload{Summary: summary}
	case agentstate.TurnItemTypeError:
		payload = TurnPayload{Summary: summary, Error: summary}
	default:
		payload = SystemPayload{Title: string(item.Type), Summary: summary}
	}
	data, _ := json.Marshal(payload)
	return data
}

func decodeToolPayloadData(data json.RawMessage) ToolPayload {
	if len(data) == 0 {
		return ToolPayload{}
	}
	var raw struct {
		ID         string          `json:"id"`
		Name       string          `json:"name"`
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		Arguments  json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ToolPayload{}
	}
	toolCallID := strings.TrimSpace(raw.ToolCallID)
	if toolCallID == "" {
		toolCallID = strings.TrimSpace(raw.ID)
	}
	toolName := strings.TrimSpace(raw.ToolName)
	if toolName == "" {
		toolName = strings.TrimSpace(raw.Name)
	}
	return ToolPayload{
		ToolCallID:   toolCallID,
		ToolName:     toolName,
		InputPreview: raw.Arguments,
	}
}

func timestampString(ts time.Time) string {
	if ts.IsZero() {
		ts = time.Unix(0, 0).UTC()
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
