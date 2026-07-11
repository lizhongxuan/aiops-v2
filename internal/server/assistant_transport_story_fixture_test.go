package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

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
	resp, err := http.Post(httpServer.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("story %q POST /api/v1/assistant/transport: %v", story.Name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("story %q read transport response: %v", story.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("story %q transport status=%d body=%s", story.Name, resp.StatusCode, raw)
	}

	state := replayAssistantTransportStoryState(t, initial, raw)
	session := sessions.Get(state.SessionID)
	if session == nil {
		t.Fatalf("story %q runtime session %q not found; transport=%s", story.Name, state.SessionID, raw)
	}
	snapshot := session.CurrentTurn
	if snapshot == nil && len(session.TurnHistory) > 0 {
		snapshot = &session.TurnHistory[len(session.TurnHistory)-1]
	}
	traceRef := ""
	if snapshot != nil && len(snapshot.Iterations) > 0 {
		traceRef = snapshot.Iterations[len(snapshot.Iterations)-1].ModelInputTraceFile
	}
	return assistantTransportStoryResult{
		State:           state,
		NormalizedState: normalizeAssistantTransportStoryState(t, state),
		Snapshot:        snapshot,
		TraceRef:        traceRef,
		RawTransport:    string(raw),
	}
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

func replayAssistantTransportStoryState(t *testing.T, initial appui.AiopsTransportState, raw []byte) appui.AiopsTransportState {
	t.Helper()
	base, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("marshal initial transport state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(base, &state); err != nil {
		t.Fatalf("decode initial transport state: %v", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "aui-state:") {
			continue
		}
		var ops []struct {
			Type  string `json:"type"`
			Path  []any  `json:"path"`
			Value any    `json:"value"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "aui-state:")), &ops); err != nil {
			t.Fatalf("decode AssistantTransport state ops: %v\nline=%s", err, line)
		}
		for _, op := range ops {
			applyAssistantTransportStoryOp(t, state, op.Type, op.Path, op.Value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan AssistantTransport response: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal replayed transport state: %v", err)
	}
	var result appui.AiopsTransportState
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode replayed transport state: %v\nstate=%s", err, encoded)
	}
	return result
}

func applyAssistantTransportStoryOp(t *testing.T, root map[string]any, opType string, path []any, value any) {
	t.Helper()
	if len(path) == 0 {
		t.Fatalf("AssistantTransport story op has empty path")
	}
	current := root
	for _, rawPart := range path[:len(path)-1] {
		part := fmt.Sprint(rawPart)
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	last := fmt.Sprint(path[len(path)-1])
	switch opType {
	case assistantTransportStreamOpSet:
		current[last] = value
	case assistantTransportStreamOpAppendText:
		prefix, _ := current[last].(string)
		current[last] = prefix + fmt.Sprint(value)
	default:
		t.Fatalf("unsupported AssistantTransport story op %q", opType)
	}
}

func normalizeAssistantTransportStoryState(t *testing.T, state appui.AiopsTransportState) map[string]any {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal story state for normalization: %v", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		t.Fatalf("decode story state for normalization: %v", err)
	}
	turnID := state.CurrentTurnID
	normalizeStoryJSON(normalized, turnID)
	if turns, ok := normalized["turns"].(map[string]any); ok && turnID != "" {
		if turn, exists := turns[turnID]; exists {
			delete(turns, turnID)
			turns["<turn-id>"] = turn
		}
	}
	return normalized
}

func normalizeStoryJSON(value any, turnID string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isStoryTimestampKey(key) && child != "" {
				typed[key] = "<timestamp>"
				continue
			}
			if text, ok := child.(string); ok && turnID != "" && key != "toolCallId" && key != "approvalId" {
				typed[key] = strings.ReplaceAll(text, turnID, "<turn-id>")
			}
			normalizeStoryJSON(typed[key], turnID)
		}
	case []any:
		for index := range typed {
			if text, ok := typed[index].(string); ok && turnID != "" {
				typed[index] = strings.ReplaceAll(text, turnID, "<turn-id>")
			}
			normalizeStoryJSON(typed[index], turnID)
		}
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
	command, _ := json.MarshalIndent(story.Command, "", "  ")
	state, _ := json.MarshalIndent(result.NormalizedState, "", "  ")
	snapshot, _ := json.MarshalIndent(result.Snapshot, "", "  ")
	t.Fatalf(format+"\ncommand=%s\nlatest transport state=%s\nturn snapshot=%s\ntrace ref=%s\nraw transport=%s", append(args, command, state, snapshot, result.TraceRef, result.RawTransport)...)
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
