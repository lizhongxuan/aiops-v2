package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

type assistantTransportStory struct {
	Name              string                  `json:"name"`
	Command           map[string]any          `json:"command"`
	ProviderResponses []storyProviderResponse `json:"providerResponses"`
	ToolOutcomes      []storyToolOutcome      `json:"toolOutcomes"`
	Want              storyTransportAssert    `json:"want"`
}

type storyProviderResponse struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []storyToolCall `json:"toolCalls,omitempty"`
}

type storyToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type storyToolOutcome struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Content     string          `json:"content,omitempty"`
	Error       string          `json:"error,omitempty"`
	Risk        string          `json:"risk,omitempty"`
	Mutating    bool            `json:"mutating,omitempty"`
}

type storyTransportAssert struct {
	TurnStatus  string            `json:"turnStatus"`
	Messages    []storyMessage    `json:"messages"`
	Tools       []storyToolAssert `json:"tools"`
	Approvals   []string          `json:"approvals"`
	Evidence    []string          `json:"evidence"`
	TraceHashes map[string]string `json:"traceHashes"`
}

type storyMessage struct {
	Phase string `json:"phase"`
	Text  string `json:"text"`
}

type storyToolAssert struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func TestAssistantTransportStories(t *testing.T) {
	for _, story := range loadAssistantTransportStories(t) {
		story := story
		t.Run(story.Name, func(t *testing.T) {
			result := runAssistantTransportStory(t, story)
			assertAssistantTransportStory(t, story, result)
		})
	}
}

func TestAssistantTransportStoryAccumulatorSupportsRootObjectAndArrayPaths(t *testing.T) {
	var state any = map[string]any{"stale": true}
	var err error
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, nil, map[string]any{"items": []any{}})
	if err != nil {
		t.Fatalf("root set: %v", err)
	}
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, []any{"items", 0}, map[string]any{})
	if err != nil {
		t.Fatalf("array insert: %v", err)
	}
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, []any{"items", "0", "name"}, "first")
	if err != nil {
		t.Fatalf("nested array set: %v", err)
	}
	want := map[string]any{"items": []any{map[string]any{"name": "first"}}}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("state = %#v, want %#v", state, want)
	}
}

func TestAssistantTransportStoryAccumulatorRejectsInvalidPathsAndAppendTargets(t *testing.T) {
	tests := []struct {
		name  string
		state any
		type_ string
		path  []any
		value any
	}{
		{name: "primitive intermediate", state: map[string]any{"turns": "invalid"}, type_: assistantTransportStreamOpSet, path: []any{"turns", "turn-1"}, value: map[string]any{}},
		{name: "array index type", state: map[string]any{"items": []any{}}, type_: assistantTransportStreamOpSet, path: []any{"items", "nope"}, value: "x"},
		{name: "array index out of bounds", state: map[string]any{"items": []any{}}, type_: assistantTransportStreamOpSet, path: []any{"items", 2}, value: "x"},
		{name: "append target not string", state: map[string]any{"message": 7}, type_: assistantTransportStreamOpAppendText, path: []any{"message"}, value: "x"},
		{name: "append value not string", state: map[string]any{"message": "ok"}, type_: assistantTransportStreamOpAppendText, path: []any{"message"}, value: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, _ := json.Marshal(tt.state)
			got, err := applyAssistantTransportStoryOpValue(tt.state, tt.type_, tt.path, tt.value)
			if err == nil {
				t.Fatalf("apply op returned nil error for state=%#v path=%#v", tt.state, tt.path)
			}
			after, _ := json.Marshal(got)
			if string(after) != string(before) {
				t.Fatalf("failed op mutated state: before=%s after=%s", before, after)
			}
		})
	}
}

func TestAssistantTransportStoryAccumulatorRejectsMissingOrNullPath(t *testing.T) {
	for _, frame := range []string{
		`aui-state:[{"type":"set","value":{"status":"idle"}}]`,
		`aui-state:[{"type":"set","path":null,"value":{"status":"idle"}}]`,
	} {
		if _, err := applyAssistantTransportStoryFrame(map[string]any{"status": "working"}, frame); err == nil {
			t.Fatalf("apply frame error = nil, want malformed path rejection: %s", frame)
		}
	}
}

func TestNormalizeAssistantTransportStoryJSONPreservesFactIdentifiersAndPayloads(t *testing.T) {
	turnID := "turn-runtime-123"
	state := map[string]any{
		"currentTurnId": turnID,
		"turnOrder":     []any{turnID},
		"updatedAt":     "2026-07-12T00:00:00Z",
		"turns": map[string]any{
			turnID: map[string]any{
				"id":        turnID,
				"updatedAt": "2026-07-12T00:00:01Z",
				"process": []any{map[string]any{
					"id":           "block-" + turnID,
					"toolCallId":   "call-fact-" + turnID,
					"approvalId":   "approval-fact-" + turnID,
					"evidenceRefs": []any{"evidence-fact-" + turnID},
					"payload":      map[string]any{"source": "payload-fact-" + turnID},
					"metadata":     map[string]any{"source": "metadata-fact-" + turnID},
				}},
			},
		},
		"artifacts": map[string]any{
			"artifact-fact-" + turnID: map[string]any{"preview": "artifact-payload-" + turnID},
		},
	}
	normalizeAssistantTransportStoryJSON(state, turnID)
	turns := state["turns"].(map[string]any)
	turn := turns["<turn-id>"].(map[string]any)
	block := turn["process"].([]any)[0].(map[string]any)
	if state["currentTurnId"] != "<turn-id>" || state["updatedAt"] != "<timestamp>" || turn["id"] != "<turn-id>" || turn["updatedAt"] != "<timestamp>" || block["id"] != "block-<turn-id>" {
		t.Fatalf("runtime identity/time normalization incomplete: %#v", state)
	}
	if block["toolCallId"] != "call-fact-"+turnID || block["approvalId"] != "approval-fact-"+turnID || block["evidenceRefs"].([]any)[0] != "evidence-fact-"+turnID {
		t.Fatalf("fact identifiers were normalized: %#v", block)
	}
	if block["payload"].(map[string]any)["source"] != "payload-fact-"+turnID || block["metadata"].(map[string]any)["source"] != "metadata-fact-"+turnID {
		t.Fatalf("payload or metadata facts were normalized: %#v", block)
	}
	artifacts := state["artifacts"].(map[string]any)
	if artifacts["artifact-fact-"+turnID].(map[string]any)["preview"] != "artifact-payload-"+turnID {
		t.Fatalf("artifact facts were normalized: %#v", artifacts)
	}
}

func TestAssistantTransportStoryProviderRejectsUnusedResponses(t *testing.T) {
	provider := newStoryProvider(t, []storyProviderResponse{
		{Role: "assistant", Content: "first"},
		{Role: "assistant", Content: "unused"},
	})
	if _, err := provider.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := provider.assertExhausted(); err == nil {
		t.Fatal("assertExhausted() error = nil, want unused scripted response error")
	}
}

func TestAssistantTransportStoryHTTPTimeoutPreservesPartialDiagnostics(t *testing.T) {
	initial := appui.NewAiopsTransportState("story-session-timeout", "story-thread-timeout")
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate(initial.SessionID, runtimekernel.SessionTypeWorkspace, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:        "turn-timeout",
		SessionID: initial.SessionID,
		Iterations: []runtimekernel.IterationState{{
			ModelInputTraceFile: "trace-timeout.json",
		}},
	}
	sessions.Update(session)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`aui-state:[{"type":"set","path":["status"],"value":"working"}]` + "\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result, err := executeAssistantTransportStoryHTTP(ctx, &http.Client{Timeout: time.Second}, server.URL, initial, []byte(`{}`), sessions)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("execute error = %v, want context deadline exceeded", err)
	}
	if result.State.Status != appui.AiopsTransportStatusWorking || result.Snapshot == nil || result.TraceRef != "trace-timeout.json" || !strings.Contains(result.RawTransport, "aui-state:") {
		t.Fatalf("partial result = %#v, want accumulated state/snapshot/trace/raw transport", result)
	}
	details := formatAssistantTransportStoryFailure(assistantTransportStory{Name: "timeout", Command: map[string]any{"type": "add-message"}}, result, err)
	for _, want := range []string{"command=", "latest transport state=", "turn snapshot=", "trace ref=trace-timeout.json"} {
		if !strings.Contains(details, want) {
			t.Fatalf("diagnostic missing %q:\n%s", want, details)
		}
	}
}

func TestAssistantTransportStoryHTTPFailuresPreserveUnifiedDiagnostics(t *testing.T) {
	stateLine := `aui-state:[{"type":"set","path":["status"],"value":"working"}]` + "\n"
	readBoom := errors.New("story body read failed")
	tests := []struct {
		name       string
		client     *http.Client
		wantStatus appui.AiopsTransportStatus
	}{
		{
			name: "request",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("story request failed")
			})},
			wantStatus: appui.AiopsTransportStatusIdle,
		},
		{
			name: "read",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       &assistantTransportStoryReadErrorBody{data: []byte(stateLine), err: readBoom},
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})},
			wantStatus: appui.AiopsTransportStatusWorking,
		},
		{
			name: "status",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader(stateLine)),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})},
			wantStatus: appui.AiopsTransportStatusWorking,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initial := appui.NewAiopsTransportState("story-session-"+tt.name, "story-thread-"+tt.name)
			sessions := runtimekernel.NewSessionManager()
			session := sessions.GetOrCreate(initial.SessionID, runtimekernel.SessionTypeWorkspace, runtimekernel.ModeChat)
			session.CurrentTurn = &runtimekernel.TurnSnapshot{ID: "turn-" + tt.name, SessionID: initial.SessionID, Iterations: []runtimekernel.IterationState{{ModelInputTraceFile: "trace-" + tt.name + ".json"}}}
			sessions.Update(session)

			result, err := executeAssistantTransportStoryHTTP(context.Background(), tt.client, "http://story.invalid", initial, []byte(`{}`), sessions)
			if err == nil {
				t.Fatal("execute error = nil, want failure")
			}
			if result.State.Status != tt.wantStatus || result.Snapshot == nil || result.TraceRef != "trace-"+tt.name+".json" {
				t.Fatalf("partial result = %#v, want status/snapshot/trace", result)
			}
			details := formatAssistantTransportStoryFailure(assistantTransportStory{Name: tt.name, Command: map[string]any{"type": "add-message"}}, result, err)
			for _, want := range []string{"command=", "latest transport state=", "turn snapshot=", "trace ref=trace-" + tt.name + ".json"} {
				if !strings.Contains(details, want) {
					t.Fatalf("diagnostic missing %q:\n%s", want, details)
				}
			}
		})
	}
}

type assistantTransportStoryRoundTripper func(*http.Request) (*http.Response, error)

func (f assistantTransportStoryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type assistantTransportStoryReadErrorBody struct {
	data []byte
	err  error
}

func (b *assistantTransportStoryReadErrorBody) Read(target []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, b.err
	}
	count := copy(target, b.data)
	b.data = b.data[count:]
	return count, nil
}

func (*assistantTransportStoryReadErrorBody) Close() error { return nil }
