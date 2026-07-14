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
	"time"

	"aiops-v2/internal/agentstate"
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
	turnID, turn, err := serverTransportTargetTurn(state, clientTurnID, messageID)
	if err != nil {
		return RunOutput{}, err
	}
	output, err := serverTransportRunOutput(state.SessionID, turnID, turn)
	if err != nil {
		return RunOutput{}, err
	}
	if !serverTransportSettled(state, turnID, turn) {
		return output, fmt.Errorf("AssistantTransport stream ended before target turn %s reached a typed terminal state", turnID)
	}
	if detail := serverTransportRunError(state, turn); detail != "" {
		return output, errors.New(detail)
	}
	return output, nil
}

func serverTransportRequestForCase(initial appui.AiopsTransportState, cfg ServerAgentConfig, c Case, clientTurnID, messageID string) serverTransportRequest {
	metadata := map[string]string{"eval.caseId": c.ID, "eval.category": c.Category, "clientTurnId": clientTurnID}
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

func serverTransportTargetTurn(state appui.AiopsTransportState, clientTurnID, messageID string) (string, appui.AiopsTransportTurn, error) {
	clientTurnID = strings.TrimSpace(clientTurnID)
	messageID = strings.TrimSpace(messageID)
	if clientTurnID == "" || messageID == "" {
		return "", appui.AiopsTransportTurn{}, errors.New("AssistantTransport target correlation requires clientTurnId and clientMessageId")
	}
	matches := make([]string, 0, 1)
	for id, turn := range state.Turns {
		if strings.TrimSpace(turn.ClientTurnID) == clientTurnID && strings.TrimSpace(turn.ClientMessageID) == messageID {
			matches = append(matches, id)
		}
	}
	if len(matches) == 1 {
		id := matches[0]
		return id, state.Turns[id], nil
	}
	if len(matches) > 1 {
		return "", appui.AiopsTransportTurn{}, fmt.Errorf("AssistantTransport target correlation is ambiguous for clientTurnId %q and clientMessageId %q", clientTurnID, messageID)
	}
	return "", appui.AiopsTransportTurn{}, fmt.Errorf("AssistantTransport response missing exact target correlation for clientTurnId %q and clientMessageId %q", clientTurnID, messageID)
}

func serverTransportSettled(state appui.AiopsTransportState, turnID string, turn appui.AiopsTransportTurn) bool {
	finalContract, err := serverTransportFinalContract(turn)
	if err != nil || finalContract == nil || !serverTransportFinalStatusIsTerminal(finalContract.Status) {
		return false
	}
	if state.RuntimeLiveness.ActiveTurns[turnID] || anyServerTransportFact(state.RuntimeLiveness.ActiveAgents) || anyServerTransportFact(state.RuntimeLiveness.ActiveCommandStreams) || serverTransportHasActiveTool(turn) {
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

func serverTransportFinalStatusIsTerminal(status appui.AiopsTransportFinalStatus) bool {
	switch status {
	case appui.AiopsTransportFinalStatusCompleted,
		appui.AiopsTransportFinalStatusFailed,
		appui.AiopsTransportFinalStatusVerified,
		appui.AiopsTransportFinalStatusPartial,
		appui.AiopsTransportFinalStatusBlocked,
		appui.AiopsTransportFinalStatusNeedsEvidence,
		appui.AiopsTransportFinalStatusApprovalDenied,
		appui.AiopsTransportFinalStatusToolUnavailable,
		appui.AiopsTransportFinalStatusCancelled:
		return true
	default:
		return false
	}
}

func anyServerTransportFact(facts map[string]bool) bool {
	for _, active := range facts {
		if active {
			return true
		}
	}
	return false
}

func serverTransportHasActiveTool(turn appui.AiopsTransportTurn) bool {
	blocks, err := serverTransportCanonicalBlocks(turn)
	if err != nil {
		// An invalid transcript cannot prove that the turn has settled.
		return true
	}
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

func serverTransportRunOutput(sessionID, turnID string, turn appui.AiopsTransportTurn) (RunOutput, error) {
	items, err := serverTransportTurnItems(turn)
	if err != nil {
		return RunOutput{}, err
	}
	calls := make([]ToolCall, 0)
	seenCallIDs := map[string]bool{}
	for index, item := range items {
		if item.Type != agentstate.TurnItemTypeToolCall {
			continue
		}
		call, err := serverTransportToolCall(item)
		if err != nil {
			return RunOutput{}, fmt.Errorf("agentItems[%d]: %w", index, err)
		}
		if seenCallIDs[call.ID] {
			return RunOutput{}, fmt.Errorf("agentItems[%d]: duplicate tool call id %q", index, call.ID)
		}
		seenCallIDs[call.ID] = true
		calls = append(calls, call)
	}
	finalContract, err := serverTransportFinalContract(turn)
	if err != nil {
		return RunOutput{}, err
	}
	answer := ""
	if finalContract != nil {
		answer = firstNonEmpty(finalContract.AnswerText, finalContract.Text)
	}
	events := agentui.ProjectTurnItemsToAgentEvents(sessionID, turnID, items, 0)
	return RunOutput{Answer: answer, Events: events, ToolCalls: calls, TurnItems: items}, nil
}

func serverTransportFinalContract(turn appui.AiopsTransportTurn) (*appui.AiopsTransportFinal, error) {
	blocks, err := serverTransportCanonicalBlocks(turn)
	if err != nil {
		return nil, err
	}
	var final *appui.AiopsTransportFinal
	for _, block := range blocks {
		if block.Type != appui.AiopsTransportBlockTypeFinalAnswer {
			if block.FinalContract != nil {
				return nil, fmt.Errorf("AssistantTransport block %q has finalContract but type %q", block.ID, block.Type)
			}
			continue
		}
		if block.FinalContract == nil {
			return nil, fmt.Errorf("AssistantTransport final_answer block %q is missing finalContract", block.ID)
		}
		if final != nil {
			return nil, fmt.Errorf("AssistantTransport turn %q has multiple final_answer blocks", turn.ID)
		}
		if strings.TrimSpace(block.FinalContract.ID) != strings.TrimSpace(block.ID) {
			return nil, fmt.Errorf("AssistantTransport final_answer block %q finalContract id = %q", block.ID, block.FinalContract.ID)
		}
		contract := *block.FinalContract
		final = &contract
	}
	return final, nil
}

func serverTransportCanonicalBlocks(turn appui.AiopsTransportTurn) ([]appui.AiopsTransportBlock, error) {
	blocks := make([]appui.AiopsTransportBlock, 0, len(turn.BlockOrder))
	seen := make(map[string]bool, len(turn.BlockOrder))
	for index, id := range turn.BlockOrder {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("AssistantTransport turn %q blockOrder[%d] is empty", turn.ID, index)
		}
		if seen[id] {
			return nil, fmt.Errorf("AssistantTransport turn %q blockOrder contains duplicate %q", turn.ID, id)
		}
		block, ok := turn.BlocksByID[id]
		if !ok {
			return nil, fmt.Errorf("AssistantTransport turn %q blockOrder references missing block %q", turn.ID, id)
		}
		if strings.TrimSpace(block.ID) != id {
			return nil, fmt.Errorf("AssistantTransport turn %q block %q has id %q", turn.ID, id, block.ID)
		}
		seen[id] = true
		blocks = append(blocks, block)
	}
	if len(seen) != len(turn.BlocksByID) {
		return nil, fmt.Errorf("AssistantTransport turn %q blocksById contains unordered blocks", turn.ID)
	}
	return blocks, nil
}

func serverTransportTurnItems(turn appui.AiopsTransportTurn) ([]agentstate.TurnItem, error) {
	if turn.AgentItemsTruncated {
		return nil, fmt.Errorf("AssistantTransport canonical agent items truncated: originalCount=%d originalBytes=%d hash=%q ref=%q", turn.AgentItemsOriginalCount, turn.AgentItemsOriginalBytes, turn.AgentItemsHash, turn.AgentItemsRef)
	}
	items := make([]agentstate.TurnItem, 0, len(turn.AgentItems))
	seen := map[string]bool{}
	for index, wire := range turn.AgentItems {
		if wire.SchemaVersion != appui.AiopsTransportAgentItemSchemaVersion {
			return nil, fmt.Errorf("agentItems[%d] schemaVersion = %q, want %q", index, wire.SchemaVersion, appui.AiopsTransportAgentItemSchemaVersion)
		}
		if wire.Truncated {
			return nil, fmt.Errorf("agentItems[%d] %q is truncated: originalBytes=%d hash=%q ref=%q", index, wire.ID, wire.OriginalBytes, wire.ContentHash, wire.Ref)
		}
		createdAt, err := serverTransportTime(wire.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("agentItems[%d] createdAt: %w", index, err)
		}
		updatedAt, err := serverTransportTime(wire.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("agentItems[%d] updatedAt: %w", index, err)
		}
		item := agentstate.TurnItem{
			ID: wire.ID, Type: agentstate.TurnItemType(wire.Type), Status: agentstate.ItemStatus(wire.Status),
			Payload:   agentstate.PayloadEnvelope{Kind: wire.Payload.Kind, Summary: wire.Payload.Summary, Data: append(json.RawMessage(nil), wire.Payload.Data...)},
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("agentItems[%d]: %w", index, err)
		}
		if seen[item.ID] {
			return nil, fmt.Errorf("agentItems[%d]: duplicate item id %q", index, item.ID)
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	return items, nil
}

func serverTransportTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func serverTransportToolCall(item agentstate.TurnItem) (ToolCall, error) {
	var payload struct {
		ID         string          `json:"id"`
		ToolCallID string          `json:"toolCallId"`
		Name       string          `json:"name"`
		ToolName   string          `json:"toolName"`
		Arguments  json.RawMessage `json:"arguments"`
	}
	if len(item.Payload.Data) == 0 {
		return ToolCall{}, errors.New("tool_call payload.data is required")
	}
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		return ToolCall{}, fmt.Errorf("decode tool_call payload.data: %w", err)
	}
	name := firstNonEmpty(payload.ToolName, payload.Name)
	if strings.TrimSpace(name) == "" {
		return ToolCall{}, errors.New("tool_call payload requires toolName or name")
	}
	return ToolCall{
		ID: firstNonEmpty(payload.ID, payload.ToolCallID, item.ID), Name: name,
		Arguments: append(json.RawMessage(nil), payload.Arguments...),
	}, nil
}

func serverTransportRunError(state appui.AiopsTransportState, turn appui.AiopsTransportTurn) string {
	finalContract, _ := serverTransportFinalContract(turn)
	finalStatus := appui.AiopsTransportFinalStatusUnknown
	if finalContract != nil {
		finalStatus = finalContract.Status
	}
	if state.Status == appui.AiopsTransportStatusFailed || turn.Status == appui.AiopsTransportTurnStatusFailed || finalStatus == appui.AiopsTransportFinalStatusFailed {
		return firstNonEmpty(state.LastError, "server turn failed")
	}
	if state.Status == appui.AiopsTransportStatusCanceled || turn.Status == appui.AiopsTransportTurnStatusCanceled || finalStatus == appui.AiopsTransportFinalStatusCancelled {
		return "server turn canceled"
	}
	if state.Status == appui.AiopsTransportStatusBlocked || turn.Status == appui.AiopsTransportTurnStatusBlocked || finalStatus == appui.AiopsTransportFinalStatusBlocked {
		return "server turn blocked on a typed pending approval or user input"
	}
	return ""
}
