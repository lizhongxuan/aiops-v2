package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/agentui"
)

// ServerAgentConfig configures the eval adapter that drives a running aiops server.
type ServerAgentConfig struct {
	BaseURL      string
	SessionID    string
	RunID        string
	SessionType  string
	Mode         string
	HostID       string
	PollTimeout  time.Duration
	PollInterval time.Duration
	HTTPClient   *http.Client
}

// ServerAgent drives the public AssistantTransport endpoint and maps its typed facts
// into the existing eval RunOutput contract.
type ServerAgent struct {
	Config ServerAgentConfig
}

func (a ServerAgent) Run(ctx context.Context, c Case) (RunOutput, error) {
	cfg := normalizeServerAgentConfig(a.Config)
	if cfg.BaseURL == "" {
		return RunOutput{}, fmt.Errorf("server url is required")
	}
	if strings.TrimSpace(cfg.RunID) == "" {
		cfg.RunID = time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.PollTimeout)
	defer cancel()
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	turnID := defaultServerEvalID(cfg, c)
	messageID := turnID + "-message"
	return runServerAssistantTransportAgent(runCtx, client, cfg, c, turnID, messageID)
}

func normalizeServerAgentConfig(cfg ServerAgentConfig) ServerAgentConfig {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if strings.TrimSpace(cfg.SessionType) == "" {
		cfg.SessionType = "host"
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "execute"
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 2 * time.Minute
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	return cfg
}

func defaultServerEvalID(cfg ServerAgentConfig, c Case) string {
	parts := []string{"eval"}
	if runID := sanitizePathComponent(cfg.RunID); runID != "" {
		parts = append(parts, runID)
	}
	parts = append(parts, sanitizePathComponent(c.ID))
	return strings.Join(parts, "-")
}

func serverTurnItems(events []agentui.AgentEvent) []agentstate.TurnItem {
	items := make([]agentstate.TurnItem, 0, len(events))
	for _, event := range events {
		typ, ok := serverTurnItemType(event)
		if !ok {
			continue
		}
		data := event.Payload
		if typ == agentstate.TurnItemTypeAssistantMessage {
			data = serverAssistantMessagePayloadData(event)
		}
		items = append(items, agentstate.TurnItem{
			ID:      firstNonEmpty(event.EventID, fmt.Sprintf("%s-%d", event.Kind, event.Seq)),
			Type:    typ,
			Status:  serverItemStatus(event.Status),
			Payload: agentstate.PayloadEnvelope{Kind: string(event.Kind), Summary: serverEventSummary(event), Data: data},
		})
	}
	return items
}

func serverTurnItemType(event agentui.AgentEvent) (agentstate.TurnItemType, bool) {
	switch event.Kind {
	case agentui.AgentEventTool:
		if serverToolEventIsResult(event) {
			return agentstate.TurnItemTypeToolResult, true
		}
		return agentstate.TurnItemTypeToolCall, true
	case agentui.AgentEventPlan:
		return agentstate.TurnItemTypePlan, true
	case agentui.AgentEventApproval:
		return agentstate.TurnItemTypeApproval, true
	case agentui.AgentEventEvidence:
		return agentstate.TurnItemTypeEvidence, true
	case agentui.AgentEventAssistant:
		return agentstate.TurnItemTypeAssistantMessage, true
	case agentui.AgentEventSystem:
		if isModelCallSystemEvent(event) {
			return agentstate.TurnItemTypeModelCall, true
		}
		return "", false
	case agentui.AgentEventTurn:
		if event.Phase == agentui.AgentEventPhaseFailed {
			return agentstate.TurnItemTypeError, true
		}
		return agentstate.TurnItemTypeUserMessage, true
	default:
		return "", false
	}
}

func serverAssistantMessagePayloadData(event agentui.AgentEvent) json.RawMessage {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	if strings.TrimSpace(serverPayloadStringValue(payload["displayKind"])) == "" {
		payload["displayKind"] = "assistant.message"
	}
	if strings.TrimSpace(serverPayloadStringValue(payload["phase"])) == "" {
		payload["phase"] = "final_answer"
	}
	if strings.TrimSpace(serverPayloadStringValue(payload["streamState"])) == "" {
		switch event.Phase {
		case agentui.AgentEventPhaseCompleted:
			payload["streamState"] = "complete"
		case agentui.AgentEventPhaseFailed, agentui.AgentEventPhaseCanceled, agentui.AgentEventPhaseBlocked:
			payload["streamState"] = "incomplete"
		default:
			payload["streamState"] = "streaming"
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)
	}
	return data
}

func serverPayloadStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func serverToolEventIsResult(event agentui.AgentEvent) bool {
	if event.Phase == agentui.AgentEventPhaseCompleted {
		return true
	}
	if event.Phase != agentui.AgentEventPhaseFailed && event.Phase != agentui.AgentEventPhaseCanceled {
		return false
	}
	var payload struct {
		OutputSummary string          `json:"outputSummary"`
		OutputPreview json.RawMessage `json:"outputPreview"`
		Error         string          `json:"error"`
		ArtifactID    string          `json:"artifactId"`
		ExitCode      *int            `json:"exitCode"`
	}
	if len(event.Payload) == 0 || json.Unmarshal(event.Payload, &payload) != nil {
		return false
	}
	return strings.TrimSpace(payload.OutputSummary) != "" ||
		len(payload.OutputPreview) > 0 ||
		strings.TrimSpace(payload.Error) != "" ||
		strings.TrimSpace(payload.ArtifactID) != "" ||
		payload.ExitCode != nil
}

func isModelCallSystemEvent(event agentui.AgentEvent) bool {
	if len(event.Payload) == 0 {
		return false
	}
	var payload struct {
		Title       string `json:"title"`
		DisplayKind string `json:"displayKind"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(payload.Title), "model_call") ||
		strings.EqualFold(strings.TrimSpace(payload.DisplayKind), "model_call")
}

func serverItemStatus(status agentui.AgentEventStatus) agentstate.ItemStatus {
	switch status {
	case agentui.AgentEventStatusQueued:
		return agentstate.ItemStatusPending
	case agentui.AgentEventStatusRunning, agentui.AgentEventStatusWaiting:
		return agentstate.ItemStatusRunning
	case agentui.AgentEventStatusCompleted:
		return agentstate.ItemStatusCompleted
	case agentui.AgentEventStatusBlocked:
		return agentstate.ItemStatusBlocked
	case agentui.AgentEventStatusFailed:
		return agentstate.ItemStatusFailed
	case agentui.AgentEventStatusCanceled:
		return agentstate.ItemStatusCancelled
	default:
		return agentstate.ItemStatusPending
	}
}

func serverEventSummary(event agentui.AgentEvent) string {
	if len(event.Payload) == 0 {
		return string(event.Kind)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return string(event.Kind)
	}
	for _, key := range []string{"summary", "title", "text", "toolName", "outputSummary", "inputSummary", "error", "reason"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return string(event.Kind)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
