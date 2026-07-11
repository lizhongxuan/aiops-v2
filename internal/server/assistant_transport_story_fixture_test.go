package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

type assistantTransportStoryResult struct {
	State           appui.AiopsTransportState
	NormalizedState map[string]any
	Snapshot        *runtimekernel.TurnSnapshot
	TraceRef        string
	RawTransport    string
}

const (
	assistantTransportStoryRequestTimeout = 5 * time.Second
	assistantTransportStoryClientTimeout  = 6 * time.Second
)

func loadAssistantTransportStories(t *testing.T) []assistantTransportStory {
	t.Helper()
	dir := filepath.Join("testdata", "assistant_transport_story")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read assistant transport stories from %s: %v", dir, err)
	}
	stories := make([]assistantTransportStory, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read assistant transport story %s: %v", path, err)
		}
		var story assistantTransportStory
		if err := json.Unmarshal(raw, &story); err != nil {
			t.Fatalf("decode assistant transport story %s: %v", path, err)
		}
		if strings.TrimSpace(story.Name) == "" || strings.TrimSpace(fmt.Sprint(story.Command["type"])) == "" {
			t.Fatalf("assistant transport story %s must define name and command.type", path)
		}
		if len(story.ProviderResponses) == 0 {
			t.Fatalf("assistant transport story %s must define providerResponses", path)
		}
		stories = append(stories, story)
	}
	sort.Slice(stories, func(i, j int) bool { return stories[i].Name < stories[j].Name })
	if len(stories) == 0 {
		t.Fatalf("no assistant transport stories found in %s", dir)
	}
	return stories
}

func runAssistantTransportStory(t *testing.T, story assistantTransportStory) assistantTransportStoryResult {
	t.Helper()

	registry := tooling.NewRegistry()
	for _, outcome := range story.ToolOutcomes {
		outcome := outcome
		if strings.TrimSpace(outcome.Name) == "" {
			t.Fatalf("story %q has tool outcome without name", story.Name)
		}
		schemaData := outcome.InputSchema
		if len(schemaData) == 0 {
			schemaData = json.RawMessage(`{"type":"object","additionalProperties":true}`)
		}
		toolDef := &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        outcome.Name,
				Description: firstStoryValue(outcome.Description, "deterministic story tool "+outcome.Name),
				Origin:      tooling.ToolOriginBuiltin,
				Layer:       tooling.ToolLayerCore,
				AlwaysLoad:  true,
				RiskLevel:   tooling.ToolRiskLevel(firstStoryValue(outcome.Risk, string(tooling.ToolRiskLow))),
				Mutating:    outcome.Mutating,
			},
			InputSchemaData: schemaData,
			ReadOnlyFunc:    func(json.RawMessage) bool { return !outcome.Mutating },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				if outcome.Error != "" {
					return tooling.ToolResult{Error: outcome.Error}, errors.New(outcome.Error)
				}
				return tooling.ToolResult{Content: outcome.Content}, nil
			},
		}
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("register story tool %q: %v", outcome.Name, err)
		}
	}

	provider := newStoryProvider(t, story.ProviderResponses)
	router := modelrouter.NewRouter("story", map[string]modelrouter.ChatModel{"story": provider}, nil)
	router.SetProviderConfigResolver(storyProviderConfigResolver{})
	sessions := runtimekernel.NewSessionManager()
	traceRoot := t.TempDir()
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{
		ToolSource:  storyToolSource{registry: registry},
		Compiler:    promptcompiler.NewCompiler(),
		Policy:      &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies(), CompletionPolicy: &policyengine.DefaultCompletionEvaluator{}},
		Projector:   storyEventEmitter{},
		ModelRouter: router,
		Sessions:    sessions,
		DebugConfig: func(context.Context) runtimekernel.RuntimeDebugConfig {
			return runtimekernel.RuntimeDebugConfig{ModelInputTrace: true, ModelInputTraceRoot: traceRoot}
		},
	})

	sessionID := "story-session-" + storySlug(story.Name)
	threadID := "story-thread-" + storySlug(story.Name)
	initial := appui.NewAiopsTransportState(sessionID, threadID)
	initial.UpdatedAt = "2000-01-01T00:00:00Z"
	payload, err := json.Marshal(map[string]any{
		"state":    initial,
		"threadId": threadID,
		"commands": []map[string]any{story.Command},
	})
	if err != nil {
		t.Fatalf("story %q marshal request: %v", story.Name, err)
	}

	httpServer := httptest.NewServer(NewHTTPServer(appui.NewServices(kernel, sessions)).Handler())
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), assistantTransportStoryRequestTimeout)
	defer cancel()
	result, err := executeAssistantTransportStoryHTTP(
		ctx,
		&http.Client{Timeout: assistantTransportStoryClientTimeout},
		httpServer.URL,
		initial,
		payload,
		sessions,
	)
	if err != nil {
		failAssistantTransportStory(t, story, result, "AssistantTransport request failed: %v", err)
		return result
	}
	if err := provider.assertExhausted(); err != nil {
		failAssistantTransportStory(t, story, result, "deterministic provider script mismatch: %v", err)
		return result
	}
	return result
}

func assertAssistantTransportStory(t *testing.T, story assistantTransportStory, result assistantTransportStoryResult) {
	t.Helper()
	if result.Snapshot == nil {
		failAssistantTransportStory(t, story, result, "runtime turn snapshot is missing")
	}
	turn, ok := result.State.Turns[result.State.CurrentTurnID]
	if !ok {
		failAssistantTransportStory(t, story, result, "current transport turn %q is missing", result.State.CurrentTurnID)
	}
	got := storyTransportAssert{
		TurnStatus:  string(turn.Status),
		Messages:    projectStoryMessages(turn),
		Tools:       projectStoryTools(turn),
		Approvals:   projectStoryApprovals(result.State, turn),
		Evidence:    projectStoryEvidence(turn),
		TraceHashes: projectStoryTraceHashes(result.Snapshot),
	}
	normalizeStoryAssert(&got)
	want := story.Want
	normalizeStoryAssert(&want)
	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if !bytes.Equal(gotJSON, wantJSON) {
		failAssistantTransportStory(t, story, result, "assertion mismatch\nwant=%s\n got=%s", wantJSON, gotJSON)
	}
	if result.TraceRef == "" {
		failAssistantTransportStory(t, story, result, "model input trace ref is missing")
	}
}

func projectStoryMessages(turn appui.AiopsTransportTurn) []storyMessage {
	messages := make([]storyMessage, 0, len(turn.Process)+1)
	for _, block := range turn.Process {
		if block.Kind == appui.AiopsTransportProcessKindAssistant && strings.TrimSpace(block.Text) != "" {
			messages = append(messages, storyMessage{Phase: firstStoryValue(block.Phase, "commentary"), Text: block.Text})
		}
	}
	if turn.Final != nil && strings.TrimSpace(turn.Final.Text) != "" {
		messages = append(messages, storyMessage{Phase: "final_answer", Text: turn.Final.Text})
	}
	return messages
}

func projectStoryTools(turn appui.AiopsTransportTurn) []storyToolAssert {
	tools := make([]storyToolAssert, 0)
	for _, block := range turn.Process {
		if block.ToolCallID == "" {
			continue
		}
		tools = append(tools, storyToolAssert{ID: block.ToolCallID, Name: block.Source, Status: string(block.Status)})
	}
	return tools
}

func projectStoryApprovals(state appui.AiopsTransportState, turn appui.AiopsTransportTurn) []string {
	seen := map[string]bool{}
	for id := range state.PendingApprovals {
		if id != "" {
			seen[id] = true
		}
	}
	for _, block := range turn.Process {
		if block.ApprovalID != "" {
			seen[block.ApprovalID] = true
		}
	}
	return sortedStoryKeys(seen)
}

func projectStoryEvidence(turn appui.AiopsTransportTurn) []string {
	seen := map[string]bool{}
	for _, block := range turn.Process {
		for _, ref := range block.EvidenceRefs {
			if ref != "" {
				seen[ref] = true
			}
		}
		if block.Kind == appui.AiopsTransportProcessKindEvidence && block.RawRef != "" {
			seen[block.RawRef] = true
		}
	}
	if turn.Final != nil {
		for _, ref := range turn.Final.CheckedEvidenceRefs {
			if ref != "" {
				seen[ref] = true
			}
		}
	}
	return sortedStoryKeys(seen)
}

func projectStoryTraceHashes(snapshot *runtimekernel.TurnSnapshot) map[string]string {
	if snapshot == nil || len(snapshot.Iterations) == 0 {
		return map[string]string{}
	}
	iteration := snapshot.Iterations[len(snapshot.Iterations)-1]
	out := make(map[string]string, len(iteration.PromptFingerprint))
	for key, value := range iteration.PromptFingerprint {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	return out
}

func normalizeStoryAssert(assertion *storyTransportAssert) {
	if assertion.Messages == nil {
		assertion.Messages = []storyMessage{}
	}
	if assertion.Tools == nil {
		assertion.Tools = []storyToolAssert{}
	}
	if assertion.Approvals == nil {
		assertion.Approvals = []string{}
	}
	if assertion.Evidence == nil {
		assertion.Evidence = []string{}
	}
	if assertion.TraceHashes == nil {
		assertion.TraceHashes = map[string]string{}
	}
	sort.Strings(assertion.Approvals)
	sort.Strings(assertion.Evidence)
}

func executeAssistantTransportStoryHTTP(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	initial appui.AiopsTransportState,
	payload []byte,
	sessions *runtimekernel.SessionManager,
) (assistantTransportStoryResult, error) {
	stateValue, err := assistantTransportStoryStateValue(initial)
	if err != nil {
		return assistantTransportStoryResult{}, fmt.Errorf("encode initial transport state: %w", err)
	}
	var raw bytes.Buffer
	capture := func(cause error) (assistantTransportStoryResult, error) {
		result, captureErr := captureAssistantTransportStoryResult(initial, stateValue, sessions, raw.String())
		if captureErr != nil {
			return result, fmt.Errorf("%w (capture partial story result: %v)", cause, captureErr)
		}
		return result, cause
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/assistant/transport", bytes.NewReader(payload))
	if err != nil {
		return capture(fmt.Errorf("build AssistantTransport request: %w", err))
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/plain")
	response, err := client.Do(request)
	if err != nil {
		return capture(fmt.Errorf("POST /api/v1/assistant/transport: %w", err))
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			raw.WriteString(line)
			if strings.HasPrefix(strings.TrimSpace(line), "aui-state:") {
				next, applyErr := applyAssistantTransportStoryFrame(stateValue, line)
				if applyErr != nil {
					return capture(fmt.Errorf("apply AssistantTransport state frame: %w", applyErr))
				}
				stateValue = next
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return capture(fmt.Errorf("read AssistantTransport response: %w", readErr))
		}
	}
	result, captureErr := captureAssistantTransportStoryResult(initial, stateValue, sessions, raw.String())
	if captureErr != nil {
		return result, fmt.Errorf("capture AssistantTransport result: %w", captureErr)
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("AssistantTransport status=%d body=%s", response.StatusCode, raw.String())
	}
	return result, nil
}

func assistantTransportStoryStateValue(state appui.AiopsTransportState) (any, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func applyAssistantTransportStoryFrame(state any, line string) (any, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "aui-state:") {
		return state, nil
	}
	var ops []struct {
		Type  string          `json:"type"`
		Path  json.RawMessage `json:"path"`
		Value any             `json:"value"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "aui-state:")), &ops); err != nil {
		return state, fmt.Errorf("decode state ops: %w", err)
	}
	next := state
	for index, op := range ops {
		if len(op.Path) == 0 || bytes.Equal(bytes.TrimSpace(op.Path), []byte("null")) {
			return state, fmt.Errorf("op[%d]: path is required and must be an array", index)
		}
		var path []any
		if err := json.Unmarshal(op.Path, &path); err != nil {
			return state, fmt.Errorf("op[%d]: decode path: %w", index, err)
		}
		updated, err := applyAssistantTransportStoryOpValue(next, op.Type, path, op.Value)
		if err != nil {
			return state, fmt.Errorf("op[%d]: %w", index, err)
		}
		next = updated
	}
	return next, nil
}

func applyAssistantTransportStoryOpValue(state any, opType string, path []any, value any) (any, error) {
	var updater func(any) (any, error)
	switch opType {
	case assistantTransportStreamOpSet:
		updater = func(any) (any, error) { return value, nil }
	case assistantTransportStreamOpAppendText:
		appendValue, ok := value.(string)
		if !ok {
			return state, fmt.Errorf("append-text value must be string, got %T", value)
		}
		updater = func(current any) (any, error) {
			text, ok := current.(string)
			if !ok {
				return nil, fmt.Errorf("expected string at path %v, got %T", path, current)
			}
			return text + appendValue, nil
		}
	default:
		return state, fmt.Errorf("invalid operation type %q", opType)
	}
	return updateAssistantTransportStoryPath(state, path, updater)
}

func updateAssistantTransportStoryPath(state any, path []any, updater func(any) (any, error)) (any, error) {
	if len(path) == 0 {
		return updater(state)
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
		child, err := updateAssistantTransportStoryPath(current[key], path[1:], updater)
		if err != nil {
			return state, err
		}
		next := make(map[string]any, len(current)+1)
		for existingKey, existingValue := range current {
			next[existingKey] = existingValue
		}
		next[key] = child
		return next, nil
	case []any:
		index, err := assistantTransportStoryArrayIndex(path[0])
		if err != nil {
			return state, fmt.Errorf("path %v: %w", path, err)
		}
		if index < 0 || index > len(current) {
			return state, fmt.Errorf("array index %d out of bounds for length %d", index, len(current))
		}
		next := append([]any(nil), current...)
		if index == len(next) {
			next = append(next, nil)
		}
		child, err := updateAssistantTransportStoryPath(next[index], path[1:], updater)
		if err != nil {
			return state, err
		}
		next[index] = child
		return next, nil
	default:
		return state, fmt.Errorf("invalid intermediate %T at path %v", state, path)
	}
}

func assistantTransportStoryArrayIndex(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > float64(math.MaxInt) || typed < float64(math.MinInt) {
			return 0, fmt.Errorf("expected integer array index, got %v", typed)
		}
		return int(typed), nil
	case string:
		index, err := strconv.Atoi(typed)
		if err != nil {
			return 0, fmt.Errorf("expected array index, got %q", typed)
		}
		return index, nil
	default:
		return 0, fmt.Errorf("expected array index, got %T", value)
	}
}

func captureAssistantTransportStoryResult(initial appui.AiopsTransportState, stateValue any, sessions *runtimekernel.SessionManager, raw string) (assistantTransportStoryResult, error) {
	encoded, err := json.Marshal(stateValue)
	if err != nil {
		return assistantTransportStoryResult{RawTransport: raw}, err
	}
	var state appui.AiopsTransportState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return assistantTransportStoryResult{RawTransport: raw}, err
	}
	if state.SessionID == "" {
		state.SessionID = initial.SessionID
	}
	snapshot := latestAssistantTransportStorySnapshot(sessions, state.SessionID)
	traceRef := ""
	if snapshot != nil && len(snapshot.Iterations) > 0 {
		traceRef = snapshot.Iterations[len(snapshot.Iterations)-1].ModelInputTraceFile
	}
	normalized, err := normalizedAssistantTransportStoryState(state)
	if err != nil {
		return assistantTransportStoryResult{State: state, Snapshot: snapshot, TraceRef: traceRef, RawTransport: raw}, err
	}
	return assistantTransportStoryResult{
		State:           state,
		NormalizedState: normalized,
		Snapshot:        snapshot,
		TraceRef:        traceRef,
		RawTransport:    raw,
	}, nil
}

func latestAssistantTransportStorySnapshot(sessions *runtimekernel.SessionManager, sessionID string) *runtimekernel.TurnSnapshot {
	if sessions == nil {
		return nil
	}
	session := sessions.Get(sessionID)
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil {
		return session.CurrentTurn
	}
	if len(session.TurnHistory) > 0 {
		return &session.TurnHistory[len(session.TurnHistory)-1]
	}
	return nil
}

func normalizedAssistantTransportStoryState(state appui.AiopsTransportState) (map[string]any, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	normalizeAssistantTransportStoryJSON(normalized, state.CurrentTurnID)
	return normalized, nil
}

func normalizeAssistantTransportStoryJSON(state map[string]any, turnID string) {
	if state == nil {
		return
	}
	if state["updatedAt"] != nil && state["updatedAt"] != "" {
		state["updatedAt"] = "<timestamp>"
	}
	if state["currentTurnId"] == turnID && turnID != "" {
		state["currentTurnId"] = "<turn-id>"
	}
	if order, ok := state["turnOrder"].([]any); ok {
		for index, value := range order {
			if value == turnID && turnID != "" {
				order[index] = "<turn-id>"
			}
		}
	}
	turns, ok := state["turns"].(map[string]any)
	if !ok || turnID == "" {
		return
	}
	turn, exists := turns[turnID]
	if !exists {
		return
	}
	normalizeAssistantTransportStoryTurnJSON(turn, turnID)
	delete(turns, turnID)
	turns["<turn-id>"] = turn
}

func normalizeAssistantTransportStoryTurnJSON(value any, turnID string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isProtectedAssistantTransportStoryFact(key) {
				continue
			}
			if isStoryTimestampKey(key) && child != "" {
				typed[key] = "<timestamp>"
				continue
			}
			if (key == "id" || key == "turnId") && turnID != "" {
				if text, ok := child.(string); ok {
					typed[key] = strings.ReplaceAll(text, turnID, "<turn-id>")
				}
			}
			normalizeAssistantTransportStoryTurnJSON(typed[key], turnID)
		}
	case []any:
		for index := range typed {
			normalizeAssistantTransportStoryTurnJSON(typed[index], turnID)
		}
	}
}

func isProtectedAssistantTransportStoryFact(key string) bool {
	switch key {
	case "toolCallId", "toolCallIds", "approvalId", "evidenceRefs", "checkedEvidenceRefs", "payload", "metadata", "inlineData", "externalReferences", "rawRef":
		return true
	default:
		return false
	}
}

func isStoryTimestampKey(key string) bool {
	switch key {
	case "createdAt", "updatedAt", "startedAt", "completedAt", "requestedAt", "resolvedAt", "timestamp":
		return true
	default:
		return false
	}
}

func failAssistantTransportStory(t *testing.T, story assistantTransportStory, result assistantTransportStoryResult, format string, args ...any) {
	t.Helper()
	cause := fmt.Errorf(format, args...)
	t.Fatal(formatAssistantTransportStoryFailure(story, result, cause))
}

func formatAssistantTransportStoryFailure(story assistantTransportStory, result assistantTransportStoryResult, cause error) string {
	command, _ := json.MarshalIndent(story.Command, "", "  ")
	state, _ := json.MarshalIndent(result.NormalizedState, "", "  ")
	snapshot, _ := json.MarshalIndent(result.Snapshot, "", "  ")
	return fmt.Sprintf("%v\ncommand=%s\nlatest transport state=%s\nturn snapshot=%s\ntrace ref=%s\nraw transport=%s", cause, command, state, snapshot, result.TraceRef, result.RawTransport)
}

type storyProvider struct {
	mu        sync.Mutex
	responses []*schema.Message
}

func newStoryProvider(t *testing.T, fixtures []storyProviderResponse) *storyProvider {
	t.Helper()
	responses := make([]*schema.Message, 0, len(fixtures))
	for _, fixture := range fixtures {
		if !strings.EqualFold(strings.TrimSpace(fixture.Role), "assistant") {
			t.Fatalf("story provider response role=%q, want assistant", fixture.Role)
		}
		calls := make([]schema.ToolCall, 0, len(fixture.ToolCalls))
		for _, call := range fixture.ToolCalls {
			calls = append(calls, schema.ToolCall{ID: call.ID, Type: "function", Function: schema.FunctionCall{Name: call.Name, Arguments: string(call.Arguments)}})
		}
		responses = append(responses, schema.AssistantMessage(fixture.Content, calls))
	}
	return &storyProvider{responses: responses}
}

func (p *storyProvider) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return p.next()
}

func (p *storyProvider) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := p.next()
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (p *storyProvider) BindTools([]*schema.ToolInfo) error { return nil }

func (p *storyProvider) next() (*schema.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) == 0 {
		return nil, errors.New("assistant transport story provider has no response remaining")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *storyProvider) assertExhausted() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) != 0 {
		return fmt.Errorf("%d scripted provider response(s) were not consumed", len(p.responses))
	}
	return nil
}

type storyProviderConfigResolver struct{}

func (storyProviderConfigResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	return modelrouter.ProviderConfig{Provider: "story", Model: "story", MaxContextTokens: runtimekernel.DefaultMaxTokens}, true
}

type storyToolSource struct{ registry *tooling.Registry }

func (s storyToolSource) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{SessionType: string(session), Mode: string(mode), AssembledTools: s.registry.AssembleTools(string(session), string(mode))}
}

func (s storyToolSource) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []tool.BaseTool {
	return s.registry.AssembleToolPool(string(session), string(mode))
}

func (s storyToolSource) CompileContextWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.registry.CompileContextWithMetadata(string(session), string(mode), metadata)
}

func (s storyToolSource) AssembleToolPoolWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []tool.BaseTool {
	return s.registry.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

type storyEventEmitter struct{}

func (storyEventEmitter) Emit(runtimekernel.LifecycleEvent) {}

func sortedStoryKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstStoryValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func storySlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(value)
	return value
}
