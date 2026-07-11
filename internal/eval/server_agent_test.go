package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/agentui"
	"aiops-v2/internal/appui"
)

func TestServerAgentRunTimesOutBlockedAssistantTransportPost(t *testing.T) {
	started := time.Now()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	agent := ServerAgent{Config: ServerAgentConfig{
		BaseURL:      "http://eval.local",
		PollTimeout:  30 * time.Millisecond,
		PollInterval: time.Millisecond,
		HTTPClient:   client,
	}}
	_, err := agent.Run(context.Background(), Case{ID: "blocked-post", Category: "server", Input: "hello"})
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Run elapsed %s, want bounded by poll timeout", elapsed)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestServerTurnItemsPreserveFailedToolResult(t *testing.T) {
	events := []agentui.AgentEvent{{
		EventID:    "turn-item:turn-1:tool-result-failed",
		Seq:        1,
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		Kind:       agentui.AgentEventTool,
		Phase:      agentui.AgentEventPhaseFailed,
		Status:     agentui.AgentEventStatusFailed,
		Visibility: agentui.AgentEventVisibilityPrimary,
		Source:     agentui.AgentEventSourceProjection,
		Payload:    json.RawMessage(`{"toolCallId":"call-1","toolName":"exec_command","outputSummary":"exec_command failed","error":"exit status 1"}`),
	}}

	items := serverTurnItems(events)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Type != agentstate.TurnItemTypeToolResult {
		t.Fatalf("item type = %q, want tool_result", items[0].Type)
	}
	if items[0].Status != agentstate.ItemStatusFailed {
		t.Fatalf("item status = %q, want failed", items[0].Status)
	}
}

func TestServerTurnItemsPreserveModelCallPromptFingerprint(t *testing.T) {
	events := []agentui.AgentEvent{{
		EventID:    "turn-item:turn-1:model-0",
		Seq:        1,
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		Kind:       agentui.AgentEventSystem,
		Phase:      agentui.AgentEventPhaseCompleted,
		Status:     agentui.AgentEventStatusCompleted,
		Visibility: agentui.AgentEventVisibilityDebug,
		Source:     agentui.AgentEventSourceProjection,
		Payload:    json.RawMessage(`{"title":"model_call","summary":"model response received","promptFingerprint":{"stableHash":"stable-hash","developerHash":"developer-hash"}}`),
	}}

	items := serverTurnItems(events)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Type != agentstate.TurnItemTypeModelCall {
		t.Fatalf("item type = %q, want model_call", items[0].Type)
	}
	score := ScoreCase(Case{ID: "case-1", Category: "server"}, RunOutput{
		Answer:    "ok 验证方式 go test ./internal/eval",
		TurnItems: items,
	})
	if len(score.PromptFingerprints) != 1 || score.PromptFingerprints[0]["developerHash"] != "developer-hash" {
		t.Fatalf("prompt fingerprints = %#v", score.PromptFingerprints)
	}
}

func TestServerAgentRunUsesAssistantTransportAndRejectsLegacyStateDependency(t *testing.T) {
	legacyPaths := map[string]bool{
		"/api/v1/" + "chat/message": true,
		"/api/v1/" + "state":        true,
	}
	var transportRequests int
	var legacyRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if legacyPaths[r.URL.Path] {
			legacyRequests = append(legacyRequests, r.URL.Path)
			http.Error(w, "legacy route is gone", http.StatusGone)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != serverAssistantTransportPath {
			http.NotFound(w, r)
			return
		}
		transportRequests++
		request := decodeServerTransportTestRequest(t, r)
		if request.State.SchemaVersion != appui.AiopsTransportSchemaVersion || request.State.SessionID != "eval-server-run-server-case" {
			t.Fatalf("initial transport state = %#v", request.State)
		}
		if len(request.Commands) != 1 || request.Commands[0].Type != "add-message" {
			t.Fatalf("commands = %#v, want one add-message", request.Commands)
		}
		message := request.Commands[0].Message
		if len(message.Parts) != 1 || message.Parts[0].Text != "inspect payment-api" {
			t.Fatalf("message = %#v", message)
		}
		if message.Metadata["eval.caseId"] != "server-case" || message.Metadata["eval.rootCauseCategory"] != "tool" {
			t.Fatalf("metadata = %#v", message.Metadata)
		}
		writer := newServerTransportTestWriter(w)
		writeServerTransportTestOps(t, writer,
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-server"),
			serverTransportTestOp("set", []any{"status"}, "working"),
			serverTransportTestOp("set", []any{"runtimeLiveness", "activeTurns", "turn-server"}, true),
			serverTransportTestOp("set", []any{"turns", "turn-server"}, map[string]any{
				"id":     "turn-server",
				"status": "working",
				"user":   map[string]any{"id": message.ID, "text": "inspect payment-api"},
				"process": []map[string]any{{
					"id": "tool-call-1", "kind": "file", "status": "completed", "text": "read app.log",
					"source": "read_file", "toolCallId": "call-1", "inputSummary": "app.log",
				}},
				"final": map[string]any{"id": "final-1", "text": "payment-api ", "answerText": "payment-api is healthy", "status": "completed"},
			}),
		)
		writeServerTransportTestOps(t, writer,
			serverTransportTestOp("append-text", []any{"turns", "turn-server", "final", "text"}, "is healthy"),
			serverTransportTestOp("set", []any{"turns", "turn-server", "status"}, "completed"),
			serverTransportTestOp("set", []any{"runtimeLiveness", "activeTurns", "turn-server"}, false),
			serverTransportTestOp("set", []any{"status"}, "idle"),
		)
		flushServerTransportTestWriter(t, writer)
	}))
	defer server.Close()

	output, err := (ServerAgent{Config: ServerAgentConfig{
		BaseURL: server.URL, RunID: "server-run", PollTimeout: 2 * time.Second,
	}}).Run(context.Background(), Case{
		ID: "server-case", Category: "server", RootCauseCategory: "tool", Input: "inspect payment-api",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if transportRequests != 1 || len(legacyRequests) != 0 {
		t.Fatalf("transport requests = %d, legacy requests = %#v", transportRequests, legacyRequests)
	}
	if output.Answer != "payment-api is healthy" {
		t.Fatalf("answer = %q", output.Answer)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Name != "read_file" || output.ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool calls = %#v", output.ToolCalls)
	}
}

func TestServerAgentAssistantTransportAccumulatorFailsClosed(t *testing.T) {
	tests := []struct{ name, line, wantErr string }{
		{name: "unknown operation", line: `aui-state:[{"type":"merge","path":["status"],"value":"idle"}]`, wantErr: "unsupported operation"},
		{name: "missing path", line: `aui-state:[{"type":"set","value":"idle"}]`, wantErr: "path is required"},
		{name: "unknown typed path", line: `aui-state:[{"type":"set","path":["legacySnapshot"],"value":{}}]`, wantErr: "unknown field"},
		{name: "invalid object path", line: `aui-state:[{"type":"set","path":[0],"value":"idle"}]`, wantErr: "object key"},
		{name: "append target", line: `aui-state:[{"type":"append-text","path":["seq"],"value":"x"}]`, wantErr: "append-text target"},
		{name: "append value", line: `aui-state:[{"type":"append-text","path":["status"],"value":7}]`, wantErr: "append-text value"},
		{name: "unknown frame", line: `data:{"status":"idle"}`, wantErr: "unsupported stream frame"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accumulator, err := newServerTransportAccumulator(appui.NewAiopsTransportState("session-1", "thread-1"))
			if err != nil {
				t.Fatalf("new accumulator: %v", err)
			}
			if err := accumulator.ApplyFrame(tt.line); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ApplyFrame() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestServerAgentAssistantTransportUsesTypedStatusNotFinalMarkdown(t *testing.T) {
	answer := "# FAILED\nPending approval mentioned only in prose."
	server := newServerTransportStateServer(t, func(messageID string) []map[string]any {
		return completedServerTransportOps("turn-typed", messageID, answer)
	})
	defer server.Close()
	output, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "typed-status", Category: "server", Input: "hello"},
	)
	if err != nil || output.Answer != answer {
		t.Fatalf("Run output = %#v, error = %v; final prose must not determine status", output, err)
	}
}

func TestServerAgentAssistantTransportReturnsTypedFailureDetail(t *testing.T) {
	server := newServerTransportStateServer(t, func(messageID string) []map[string]any {
		return []map[string]any{
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-failed"),
			serverTransportTestOp("set", []any{"turns", "turn-failed"}, map[string]any{
				"id": "turn-failed", "status": "failed", "user": map[string]any{"id": messageID, "text": "hello"},
				"process": []map[string]any{{"id": "failure-1", "kind": "system", "status": "failed", "text": "request failed"}},
				"final":   map[string]any{"id": "final-failed", "text": "failure summary", "status": "failed"},
			}),
			serverTransportTestOp("set", []any{"lastError"}, "model config missing"),
			serverTransportTestOp("set", []any{"status"}, "failed"),
		}
	})
	defer server.Close()
	output, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "typed-failure", Category: "server", Input: "hello"},
	)
	if err == nil || !strings.Contains(err.Error(), "model config missing") {
		t.Fatalf("error = %v, want typed lastError", err)
	}
	if len(output.Events) != 2 || len(output.TurnItems) != 1 {
		t.Fatalf("output should retain typed failed-turn facts: %#v", output)
	}
}

func TestServerAgentAssistantTransportRejectsTerminalTurnWithActiveToolFacts(t *testing.T) {
	server := newServerTransportStateServer(t, func(messageID string) []map[string]any {
		return append(completedServerTransportOps("turn-active", messageID, "typed final"),
			serverTransportTestOp("set", []any{"runtimeLiveness", "activeCommandStreams", "call-active"}, true))
	})
	defer server.Close()
	_, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "active-tool", Category: "server", Input: "hello"},
	)
	if err == nil || !strings.Contains(err.Error(), "typed terminal state") {
		t.Fatalf("error = %v, want active tool facts to prevent completion", err)
	}
}

func decodeServerTransportTestRequest(t *testing.T, r *http.Request) serverTransportRequest {
	t.Helper()
	var request serverTransportRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatalf("decode transport request: %v", err)
	}
	return request
}

func newServerTransportStateServer(t *testing.T, ops func(string) []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != serverAssistantTransportPath {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		request := decodeServerTransportTestRequest(t, r)
		writer := newServerTransportTestWriter(w)
		writeServerTransportTestOps(t, writer, ops(request.Commands[0].Message.ID)...)
		flushServerTransportTestWriter(t, writer)
	}))
}

func completedServerTransportOps(turnID, messageID, answer string) []map[string]any {
	return []map[string]any{
		serverTransportTestOp("set", []any{"currentTurnId"}, turnID),
		serverTransportTestOp("set", []any{"turns", turnID}, map[string]any{
			"id": turnID, "status": "completed", "user": map[string]any{"id": messageID, "text": "hello"},
			"final": map[string]any{"id": turnID + ":final", "text": answer, "answerText": answer, "status": "completed"},
		}),
		serverTransportTestOp("set", []any{"status"}, "idle"),
	}
}

func newServerTransportTestWriter(w http.ResponseWriter) *bufio.Writer {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return bufio.NewWriter(w)
}

func serverTransportTestOp(opType string, path []any, value any) map[string]any {
	return map[string]any{"type": opType, "path": path, "value": value}
}

func writeServerTransportTestOps(t *testing.T, writer *bufio.Writer, ops ...map[string]any) {
	t.Helper()
	payload, err := json.Marshal(ops)
	if err != nil {
		t.Fatalf("marshal transport ops: %v", err)
	}
	if _, err := fmt.Fprintf(writer, "aui-state:%s\n", payload); err != nil {
		t.Fatalf("write transport ops: %v", err)
	}
}

func flushServerTransportTestWriter(t *testing.T, writer *bufio.Writer) {
	t.Helper()
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush transport response: %v", err)
	}
}
