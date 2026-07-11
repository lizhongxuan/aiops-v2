package eval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"aiops-v2/internal/agentui"
	"aiops-v2/internal/appui"
)

const serverAssistantTransportPath = "/api/v1/assistant/transport"

type serverTransportRequest struct {
	State    appui.AiopsTransportState `json:"state"`
	Commands []serverTransportCommand  `json:"commands"`
	ThreadID string                    `json:"threadId"`
	Config   map[string]string         `json:"config,omitempty"`
}

type serverTransportCommand struct {
	Type    string                 `json:"type"`
	Message serverTransportMessage `json:"message"`
}

type serverTransportMessage struct {
	ID       string            `json:"id"`
	Role     string            `json:"role"`
	HostID   string            `json:"hostId,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Parts    []serverTextPart  `json:"parts"`
}

type serverTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func runServerAssistantTransportAgent(ctx context.Context, client *http.Client, cfg ServerAgentConfig, c Case, clientTurnID, messageID string) (RunOutput, error) {
	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		sessionID = defaultServerEvalID(cfg, c)
	}
	initial := appui.NewAiopsTransportState(sessionID, sessionID)
	state, err := requestServerAssistantTransport(ctx, client, cfg.BaseURL, initial, serverTransportRequestForCase(initial, cfg, c, clientTurnID, messageID))
	if err != nil {
		return RunOutput{}, err
	}
	turnID, turn, err := serverTransportTargetTurn(state, messageID)
	if err != nil {
		return RunOutput{}, err
	}
	output := serverTransportRunOutput(turnID, turn)
	if !serverTransportSettled(state, turnID, turn) {
		return output, fmt.Errorf("AssistantTransport stream ended before target turn %s reached a typed terminal state", turnID)
	}
	if detail := serverTransportRunError(state, turn); detail != "" {
		return output, errors.New(detail)
	}
	return output, nil
}

func serverTransportRequestForCase(initial appui.AiopsTransportState, cfg ServerAgentConfig, c Case, clientTurnID, messageID string) serverTransportRequest {
	metadata := map[string]string{"eval.caseId": c.ID, "eval.category": c.Category, "eval.clientTurnId": clientTurnID}
	if value := strings.TrimSpace(c.Priority); value != "" {
		metadata["eval.priority"] = normalizePriority(value)
	}
	if value := strings.TrimSpace(c.RootCauseCategory); value != "" {
		metadata["eval.rootCauseCategory"] = value
	}
	return serverTransportRequest{
		State: initial, ThreadID: initial.ThreadID,
		Config: map[string]string{"sessionType": strings.TrimSpace(cfg.SessionType), "mode": strings.TrimSpace(cfg.Mode)},
		Commands: []serverTransportCommand{{Type: "add-message", Message: serverTransportMessage{
			ID: messageID, Role: "user", HostID: strings.TrimSpace(cfg.HostID), Metadata: metadata,
			Parts: []serverTextPart{{Type: "text", Text: c.Input}},
		}}},
	}
}

func requestServerAssistantTransport(ctx context.Context, client *http.Client, baseURL string, initial appui.AiopsTransportState, body serverTransportRequest) (appui.AiopsTransportState, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return appui.AiopsTransportState{}, fmt.Errorf("marshal AssistantTransport request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+serverAssistantTransportPath, bytes.NewReader(payload))
	if err != nil {
		return appui.AiopsTransportState{}, fmt.Errorf("build AssistantTransport request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/plain")
	response, err := client.Do(request)
	if err != nil {
		return appui.AiopsTransportState{}, fmt.Errorf("POST %s: %w", serverAssistantTransportPath, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return appui.AiopsTransportState{}, fmt.Errorf("POST %s: status %d: %s", serverAssistantTransportPath, response.StatusCode, strings.TrimSpace(string(data)))
	}
	accumulator, err := newServerTransportAccumulator(initial)
	if err != nil {
		return appui.AiopsTransportState{}, err
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			if err := accumulator.ApplyFrame(line); err != nil {
				return appui.AiopsTransportState{}, fmt.Errorf("apply AssistantTransport frame: %w", err)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return appui.AiopsTransportState{}, fmt.Errorf("read AssistantTransport stream: %w", readErr)
		}
	}
	return accumulator.state, nil
}

type serverTransportAccumulator struct {
	value any
	state appui.AiopsTransportState
}

func newServerTransportAccumulator(initial appui.AiopsTransportState) (*serverTransportAccumulator, error) {
	raw, err := json.Marshal(initial)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	state, err := decodeServerTransportState(value)
	if err != nil {
		return nil, err
	}
	return &serverTransportAccumulator{value: value, state: state}, nil
}

func (a *serverTransportAccumulator) ApplyFrame(line string) error {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "3:") {
		var message string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "3:")), &message); err != nil {
			return fmt.Errorf("decode error frame: %w", err)
		}
		return fmt.Errorf("AssistantTransport error: %s", strings.TrimSpace(message))
	}
	if !strings.HasPrefix(line, "aui-state:") {
		return fmt.Errorf("unsupported stream frame %q", line)
	}
	var ops []struct {
		Type  string          `json:"type"`
		Path  json.RawMessage `json:"path"`
		Value any             `json:"value"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "aui-state:")), &ops); err != nil {
		return fmt.Errorf("decode state operations: %w", err)
	}
	next := a.value
	for index, op := range ops {
		if len(op.Path) == 0 || bytes.Equal(bytes.TrimSpace(op.Path), []byte("null")) {
			return fmt.Errorf("operation %d path is required and must be an array", index)
		}
		var path []any
		if err := json.Unmarshal(op.Path, &path); err != nil {
			return fmt.Errorf("operation %d decode path: %w", index, err)
		}
		updated, err := applyServerTransportOperation(next, op.Type, path, op.Value)
		if err != nil {
			return fmt.Errorf("operation %d: %w", index, err)
		}
		next = updated
	}
	state, err := decodeServerTransportState(next)
	if err != nil {
		return fmt.Errorf("typed state validation: %w", err)
	}
	a.value, a.state = next, state
	return nil
}

func decodeServerTransportState(value any) (appui.AiopsTransportState, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return appui.AiopsTransportState{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var state appui.AiopsTransportState
	if err := decoder.Decode(&state); err != nil {
		return state, err
	}
	if state.SchemaVersion != appui.AiopsTransportSchemaVersion {
		return state, fmt.Errorf("schemaVersion = %q, want %q", state.SchemaVersion, appui.AiopsTransportSchemaVersion)
	}
	return state, nil
}

func applyServerTransportOperation(state any, opType string, path []any, value any) (any, error) {
	switch opType {
	case "set":
		return updateServerTransportPath(state, path, func(any) (any, error) { return value, nil })
	case "append-text":
		appendValue, ok := value.(string)
		if !ok {
			return state, fmt.Errorf("append-text value must be string, got %T", value)
		}
		return updateServerTransportPath(state, path, func(current any) (any, error) {
			text, ok := current.(string)
			if !ok {
				return nil, fmt.Errorf("append-text target at path %v must be string, got %T", path, current)
			}
			return text + appendValue, nil
		})
	default:
		return state, fmt.Errorf("unsupported operation %q", opType)
	}
}

func updateServerTransportPath(state any, path []any, update func(any) (any, error)) (any, error) {
	if len(path) == 0 {
		return update(state)
	}
	if state == nil {
		state = map[string]any{}
	}
	switch current := state.(type) {
	case map[string]any:
		key, ok := path[0].(string)
		if !ok {
			return state, fmt.Errorf("expected object key at path %v, got %T", path, path[0])
		}
		child, err := updateServerTransportPath(current[key], path[1:], update)
		if err != nil {
			return state, err
		}
		next := make(map[string]any, len(current)+1)
		for k, v := range current {
			next[k] = v
		}
		next[key] = child
		return next, nil
	case []any:
		index, err := serverTransportArrayIndex(path[0])
		if err != nil || index < 0 || index > len(current) {
			return state, fmt.Errorf("invalid array path %v: index=%d: %w", path, index, err)
		}
		next := append([]any(nil), current...)
		if index == len(next) {
			next = append(next, nil)
		}
		child, err := updateServerTransportPath(next[index], path[1:], update)
		if err != nil {
			return state, err
		}
		next[index] = child
		return next, nil
	default:
		return state, fmt.Errorf("cannot traverse %T at path %v", state, path)
	}
}

func serverTransportArrayIndex(value any) (int, error) {
	if number, ok := value.(float64); ok && number == float64(int(number)) {
		return int(number), nil
	}
	if text, ok := value.(string); ok {
		return strconv.Atoi(text)
	}
	return 0, fmt.Errorf("array index must be integer, got %T", value)
}

func serverTransportTargetTurn(state appui.AiopsTransportState, messageID string) (string, appui.AiopsTransportTurn, error) {
	for id, turn := range state.Turns {
		if turn.User != nil && strings.TrimSpace(turn.User.ID) == strings.TrimSpace(messageID) {
			return id, turn, nil
		}
	}
	if id := strings.TrimSpace(state.CurrentTurnID); id != "" {
		if turn, ok := state.Turns[id]; ok {
			return id, turn, nil
		}
	}
	return "", appui.AiopsTransportTurn{}, errors.New("AssistantTransport response missing target turn facts")
}

func serverTransportSettled(state appui.AiopsTransportState, turnID string, turn appui.AiopsTransportTurn) bool {
	if state.RuntimeLiveness.ActiveTurns[turnID] || anyServerTransportFact(state.RuntimeLiveness.ActiveAgents) || anyServerTransportFact(state.RuntimeLiveness.ActiveCommandStreams) || serverTransportHasActiveTool(turn.Process) {
		return false
	}
	pending := len(state.PendingApprovals) > 0 || anyServerTransportFact(state.RuntimeLiveness.PendingApprovals) || anyServerTransportFact(state.RuntimeLiveness.PendingUserInputs)
	switch turn.Status {
	case appui.AiopsTransportTurnStatusCompleted:
		return state.Status == appui.AiopsTransportStatusIdle && !pending
	case appui.AiopsTransportTurnStatusFailed:
		return (state.Status == appui.AiopsTransportStatusFailed || state.Status == appui.AiopsTransportStatusIdle) && !pending
	case appui.AiopsTransportTurnStatusCanceled:
		return (state.Status == appui.AiopsTransportStatusCanceled || state.Status == appui.AiopsTransportStatusIdle) && !pending
	case appui.AiopsTransportTurnStatusBlocked:
		return state.Status == appui.AiopsTransportStatusBlocked && pending
	}
	return false
}

func anyServerTransportFact(facts map[string]bool) bool {
	for _, active := range facts {
		if active {
			return true
		}
	}
	return false
}

func serverTransportHasActiveTool(blocks []appui.AiopsProcessBlock) bool {
	for _, block := range blocks {
		if block.Status == appui.AiopsTransportProcessStatusRunning || block.Status == appui.AiopsTransportProcessStatusQueued {
			switch block.Kind {
			case appui.AiopsTransportProcessKindSearch, appui.AiopsTransportProcessKindCommand, appui.AiopsTransportProcessKindFile, appui.AiopsTransportProcessKindTool, appui.AiopsTransportProcessKindMCP, appui.AiopsTransportProcessKindSubagent:
				return true
			}
		}
	}
	return false
}

func serverTransportRunOutput(turnID string, turn appui.AiopsTransportTurn) RunOutput {
	events := make([]agentui.AgentEvent, 0, len(turn.Process)+1)
	calls, seen := []ToolCall{}, map[string]bool{}
	for index, block := range turn.Process {
		payload, _ := json.Marshal(map[string]any{"displayKind": block.DisplayKind, "text": block.Text, "toolCallId": block.ToolCallID, "toolName": block.Source, "inputSummary": block.InputSummary, "outputSummary": block.OutputPreview})
		events = append(events, agentui.AgentEvent{EventID: firstNonEmpty(block.ID, fmt.Sprintf("transport-%d", index)), Seq: int64(index + 1), TurnID: turnID, Kind: serverTransportEventKind(block.Kind), Phase: serverTransportEventPhase(block.Status), Status: serverTransportEventStatus(block.Status), Visibility: agentui.AgentEventVisibilityPrimary, Source: agentui.AgentEventSourceProjection, Payload: payload})
		if name := strings.TrimSpace(block.Source); name != "" {
			id := firstNonEmpty(block.ToolCallID, block.ID, name)
			if !seen[id] {
				seen[id], calls = true, append(calls, ToolCall{ID: id, Name: name})
			}
		}
	}
	answer := ""
	if turn.Final != nil {
		answer = firstNonEmpty(turn.Final.AnswerText, turn.Final.Text)
		payload, _ := json.Marshal(map[string]any{"displayKind": "assistant.message", "phase": "final_answer", "streamState": "complete", "text": turn.Final.Text, "answerText": turn.Final.AnswerText, "status": turn.Final.Status})
		events = append(events, agentui.AgentEvent{EventID: firstNonEmpty(turn.Final.ID, turnID+":final"), Seq: int64(len(events) + 1), TurnID: turnID, Kind: agentui.AgentEventAssistant, Phase: serverTransportTurnPhase(turn.Status), Status: serverTransportTurnStatus(turn.Status), Visibility: agentui.AgentEventVisibilityPrimary, Source: agentui.AgentEventSourceProjection, Payload: payload})
	}
	return RunOutput{Answer: answer, Events: events, ToolCalls: calls, TurnItems: serverTurnItems(events)}
}

func serverTransportEventKind(kind appui.AiopsTransportProcessKind) agentui.AgentEventKind {
	switch kind {
	case appui.AiopsTransportProcessKindPlan:
		return agentui.AgentEventPlan
	case appui.AiopsTransportProcessKindApproval:
		return agentui.AgentEventApproval
	case appui.AiopsTransportProcessKindEvidence:
		return agentui.AgentEventEvidence
	case appui.AiopsTransportProcessKindAssistant, appui.AiopsTransportProcessKindReasoning:
		return agentui.AgentEventAssistant
	case appui.AiopsTransportProcessKindSystem:
		return agentui.AgentEventSystem
	}
	return agentui.AgentEventTool
}

func serverTransportEventStatus(status appui.AiopsTransportProcessStatus) agentui.AgentEventStatus {
	switch status {
	case appui.AiopsTransportProcessStatusQueued:
		return agentui.AgentEventStatusQueued
	case appui.AiopsTransportProcessStatusRunning:
		return agentui.AgentEventStatusRunning
	case appui.AiopsTransportProcessStatusFailed:
		return agentui.AgentEventStatusFailed
	case appui.AiopsTransportProcessStatusBlocked:
		return agentui.AgentEventStatusBlocked
	case appui.AiopsTransportProcessStatusRejected:
		return agentui.AgentEventStatusCanceled
	}
	return agentui.AgentEventStatusCompleted
}

func serverTransportEventPhase(status appui.AiopsTransportProcessStatus) agentui.AgentEventPhase {
	switch status {
	case appui.AiopsTransportProcessStatusQueued, appui.AiopsTransportProcessStatusRunning:
		return agentui.AgentEventPhaseStarted
	case appui.AiopsTransportProcessStatusFailed:
		return agentui.AgentEventPhaseFailed
	case appui.AiopsTransportProcessStatusBlocked:
		return agentui.AgentEventPhaseBlocked
	case appui.AiopsTransportProcessStatusRejected:
		return agentui.AgentEventPhaseCanceled
	}
	return agentui.AgentEventPhaseCompleted
}

func serverTransportTurnStatus(status appui.AiopsTransportTurnStatus) agentui.AgentEventStatus {
	switch status {
	case appui.AiopsTransportTurnStatusFailed:
		return agentui.AgentEventStatusFailed
	case appui.AiopsTransportTurnStatusCanceled:
		return agentui.AgentEventStatusCanceled
	case appui.AiopsTransportTurnStatusBlocked:
		return agentui.AgentEventStatusBlocked
	}
	return agentui.AgentEventStatusCompleted
}

func serverTransportTurnPhase(status appui.AiopsTransportTurnStatus) agentui.AgentEventPhase {
	switch status {
	case appui.AiopsTransportTurnStatusFailed:
		return agentui.AgentEventPhaseFailed
	case appui.AiopsTransportTurnStatusCanceled:
		return agentui.AgentEventPhaseCanceled
	case appui.AiopsTransportTurnStatusBlocked:
		return agentui.AgentEventPhaseBlocked
	}
	return agentui.AgentEventPhaseCompleted
}

func serverTransportRunError(state appui.AiopsTransportState, turn appui.AiopsTransportTurn) string {
	if state.Status == appui.AiopsTransportStatusFailed || turn.Status == appui.AiopsTransportTurnStatusFailed {
		return firstNonEmpty(state.LastError, serverTransportFailedProcess(turn.Process), "server turn failed")
	}
	if state.Status == appui.AiopsTransportStatusCanceled || turn.Status == appui.AiopsTransportTurnStatusCanceled {
		return "server turn canceled"
	}
	if state.Status == appui.AiopsTransportStatusBlocked || turn.Status == appui.AiopsTransportTurnStatusBlocked {
		return "server turn blocked on a typed pending approval or user input"
	}
	if turn.Final == nil || strings.TrimSpace(firstNonEmpty(turn.Final.AnswerText, turn.Final.Text)) == "" {
		return "server turn completed without typed final facts"
	}
	return ""
}

func serverTransportFailedProcess(blocks []appui.AiopsProcessBlock) string {
	for _, block := range blocks {
		if block.Status == appui.AiopsTransportProcessStatusFailed {
			return firstNonEmpty(block.OutputPreview, block.Text, block.Source)
		}
	}
	return ""
}
