package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// ServerAgent drives the normal aiops HTTP API and converts the state snapshot
// back into the existing eval RunOutput contract.
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

type serverChatResponse struct {
	Accepted        bool   `json:"accepted"`
	SessionID       string `json:"sessionId"`
	TurnID          string `json:"turnId"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	Status          string `json:"status"`
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
}

func postServerChatMessage(ctx context.Context, client *http.Client, cfg ServerAgentConfig, c Case, clientTurnID, clientMessageID string) (serverChatResponse, error) {
	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		sessionID = defaultServerEvalID(cfg, c)
	}
	metadata := map[string]string{
		"eval.caseId":   c.ID,
		"eval.category": c.Category,
	}
	if priority := strings.TrimSpace(c.Priority); priority != "" {
		metadata["eval.priority"] = normalizePriority(priority)
	}
	if rootCause := strings.TrimSpace(c.RootCauseCategory); rootCause != "" {
		metadata["eval.rootCauseCategory"] = rootCause
	}
	body := map[string]any{
		"sessionId":       sessionID,
		"sessionType":     strings.TrimSpace(cfg.SessionType),
		"mode":            strings.TrimSpace(cfg.Mode),
		"hostId":          strings.TrimSpace(cfg.HostID),
		"message":         c.Input,
		"role":            "user",
		"clientTurnId":    clientTurnID,
		"clientMessageId": clientMessageID,
		"metadata":        metadata,
	}
	var resp serverChatResponse
	if err := doJSON(ctx, client, http.MethodPost, cfg.BaseURL+"/api/v1/chat/message", body, &resp); err != nil {
		return serverChatResponse{}, err
	}
	if strings.TrimSpace(resp.Error) != "" {
		return resp, fmt.Errorf("server accepted turn with error: %s", resp.Error)
	}
	if strings.TrimSpace(resp.SessionID) == "" || strings.TrimSpace(resp.TurnID) == "" {
		return resp, fmt.Errorf("server response missing sessionId or turnId")
	}
	if resp.ClientTurnID == "" {
		resp.ClientTurnID = clientTurnID
	}
	if resp.ClientMessageID == "" {
		resp.ClientMessageID = clientMessageID
	}
	return resp, nil
}

func defaultServerEvalID(cfg ServerAgentConfig, c Case) string {
	parts := []string{"eval"}
	if runID := sanitizePathComponent(cfg.RunID); runID != "" {
		parts = append(parts, runID)
	}
	parts = append(parts, sanitizePathComponent(c.ID))
	return strings.Join(parts, "-")
}

func pollServerState(ctx context.Context, client *http.Client, cfg ServerAgentConfig, chat serverChatResponse) (serverStateSnapshot, error) {
	deadline := time.Now().Add(cfg.PollTimeout)
	for {
		var state serverStateSnapshot
		if err := doJSON(ctx, client, http.MethodGet, cfg.BaseURL+"/api/v1/state", nil, &state); err != nil {
			return serverStateSnapshot{}, err
		}
		if serverTurnFinished(state, chat) {
			return state, nil
		}
		if time.Now().After(deadline) {
			return serverStateSnapshot{}, fmt.Errorf("poll /api/v1/state timed out after %s for session %s turn %s", cfg.PollTimeout, chat.SessionID, chat.TurnID)
		}
		timer := time.NewTimer(cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return serverStateSnapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func doJSON(ctx context.Context, client *http.Client, method, url string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: status %d: %s", method, url, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, url, err)
	}
	return nil
}

type serverStateSnapshot struct {
	SessionID            string                        `json:"sessionId,omitempty"`
	Cards                []serverCard                  `json:"cards,omitempty"`
	ToolInvocations      []serverToolInvocation        `json:"toolInvocations,omitempty"`
	Runtime              serverRuntimeSnapshot         `json:"runtime"`
	Config               map[string]json.RawMessage    `json:"config,omitempty"`
	AgentEventProjection *agentui.AgentEventProjection `json:"agentEventProjection,omitempty"`
}

type serverCard struct {
	ClientTurnID string `json:"clientTurnId,omitempty"`
	Role         string `json:"role,omitempty"`
	Text         string `json:"text,omitempty"`
	Message      string `json:"message,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

type serverToolInvocation struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	InputJSON string `json:"inputJson,omitempty"`
	Status    string `json:"status,omitempty"`
}

type serverRuntimeSnapshot struct {
	Turn serverRuntimeTurnSnapshot `json:"turn"`
}

type serverRuntimeTurnSnapshot struct {
	Active          bool   `json:"active"`
	Phase           string `json:"phase,omitempty"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
}

func serverTurnFinished(state serverStateSnapshot, chat serverChatResponse) bool {
	if !serverStateMatchesChat(state, chat) {
		return false
	}
	if state.Runtime.Turn.Active {
		return false
	}
	if state.AgentEventProjection != nil {
		if final, ok := state.AgentEventProjection.FinalMessages[chat.TurnID]; ok && strings.TrimSpace(final.Text) != "" {
			return true
		}
	}
	if strings.TrimSpace(chat.Output) != "" || strings.TrimSpace(chat.Error) != "" {
		return true
	}
	for _, card := range state.Cards {
		if strings.EqualFold(strings.TrimSpace(card.Role), "assistant") && strings.TrimSpace(firstNonEmpty(card.Text, card.Message, card.Summary)) != "" {
			return true
		}
	}
	switch strings.ToLower(strings.TrimSpace(state.Runtime.Turn.Phase)) {
	case "completed", "failed", "error", "cancelled", "canceled", "aborted":
		return true
	default:
		return false
	}
}

func serverStateMatchesChat(state serverStateSnapshot, chat serverChatResponse) bool {
	if strings.TrimSpace(chat.SessionID) != "" && strings.TrimSpace(state.SessionID) != "" && state.SessionID != chat.SessionID {
		return false
	}
	if state.AgentEventProjection != nil && strings.TrimSpace(state.AgentEventProjection.CurrentTurnID) != "" && strings.TrimSpace(chat.TurnID) != "" && state.AgentEventProjection.CurrentTurnID != chat.TurnID {
		return false
	}
	if strings.TrimSpace(state.Runtime.Turn.ClientTurnID) != "" && strings.TrimSpace(chat.ClientTurnID) != "" && state.Runtime.Turn.ClientTurnID != chat.ClientTurnID {
		return false
	}
	if strings.TrimSpace(state.Runtime.Turn.ClientMessageID) != "" && strings.TrimSpace(chat.ClientMessageID) != "" && state.Runtime.Turn.ClientMessageID != chat.ClientMessageID {
		return false
	}
	return true
}

func runOutputFromServerState(state serverStateSnapshot, chat serverChatResponse) RunOutput {
	events := serverAgentEvents(state, chat.TurnID)
	return RunOutput{
		Answer:    serverAnswer(state, chat),
		Events:    events,
		ToolCalls: serverToolCalls(state, events),
		TurnItems: serverTurnItems(events),
	}
}

func serverAnswer(state serverStateSnapshot, chat serverChatResponse) string {
	if state.AgentEventProjection != nil && state.AgentEventProjection.FinalMessages != nil {
		if final, ok := state.AgentEventProjection.FinalMessages[chat.TurnID]; ok && strings.TrimSpace(final.Text) != "" {
			return strings.TrimSpace(final.Text)
		}
		for _, final := range state.AgentEventProjection.FinalMessages {
			if strings.TrimSpace(final.Text) != "" {
				return strings.TrimSpace(final.Text)
			}
		}
	}
	for i := len(state.Cards) - 1; i >= 0; i-- {
		card := state.Cards[i]
		if !strings.EqualFold(strings.TrimSpace(card.Role), "assistant") {
			continue
		}
		if strings.TrimSpace(card.ClientTurnID) != "" && strings.TrimSpace(chat.ClientTurnID) != "" && card.ClientTurnID != chat.ClientTurnID {
			continue
		}
		if text := strings.TrimSpace(firstNonEmpty(card.Text, card.Message, card.Summary)); text != "" {
			return text
		}
	}
	return strings.TrimSpace(chat.Output)
}

func serverAgentEvents(state serverStateSnapshot, turnID string) []agentui.AgentEvent {
	if len(state.Config) == 0 {
		return nil
	}
	raw := state.Config["agentItemEvents"]
	if len(raw) == 0 {
		return nil
	}
	var events []agentui.AgentEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil
	}
	if strings.TrimSpace(turnID) == "" {
		return events
	}
	filtered := events[:0]
	for _, event := range events {
		if strings.TrimSpace(event.TurnID) == "" || event.TurnID == turnID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func serverToolCalls(state serverStateSnapshot, events []agentui.AgentEvent) []ToolCall {
	calls := make([]ToolCall, 0, len(state.ToolInvocations))
	seen := map[string]bool{}
	for _, invocation := range state.ToolInvocations {
		name := strings.TrimSpace(invocation.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(invocation.ID)
		if id == "" {
			id = name
		}
		seen[id] = true
		var args json.RawMessage
		if input := strings.TrimSpace(invocation.InputJSON); input != "" {
			args = json.RawMessage(input)
		}
		calls = append(calls, ToolCall{ID: id, Name: name, Arguments: args})
	}
	for _, event := range events {
		if event.Kind != agentui.AgentEventTool {
			continue
		}
		var payload agentui.ToolPayload
		_ = json.Unmarshal(event.Payload, &payload)
		id := firstNonEmpty(payload.ToolCallID, event.EventID)
		name := strings.TrimSpace(payload.ToolName)
		if name == "" || seen[id] {
			continue
		}
		calls = append(calls, ToolCall{ID: id, Name: name})
		seen[id] = true
	}
	return calls
}

func serverRunError(state serverStateSnapshot, chat serverChatResponse, output RunOutput) string {
	if strings.TrimSpace(chat.Error) != "" {
		return strings.TrimSpace(chat.Error)
	}
	if state.AgentEventProjection != nil && strings.EqualFold(strings.TrimSpace(state.AgentEventProjection.Status), "failed") {
		return "server turn failed"
	}
	for _, event := range output.Events {
		if event.Status != agentui.AgentEventStatusFailed || event.Kind == agentui.AgentEventTool {
			continue
		}
		if summary := serverEventSummary(event); summary != "" && summary != string(event.Kind) {
			return summary
		}
		return fmt.Sprintf("server turn failed at %s", event.Kind)
	}
	phase := strings.ToLower(strings.TrimSpace(state.Runtime.Turn.Phase))
	if phase == "failed" || phase == "error" {
		return "server turn failed: " + phase
	}
	if strings.TrimSpace(output.Answer) == "" {
		return "server turn completed without a final answer"
	}
	return ""
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
