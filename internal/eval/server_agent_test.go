package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/agentui"
)

func TestServerAgentRunPostsMessagePollsStateAndExtractsOutput(t *testing.T) {
	var postSeen bool
	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/chat/message":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if req["message"] != "inspect payment-api" {
				t.Fatalf("message = %#v", req["message"])
			}
			if req["sessionId"] != "eval-server-run-server-case" {
				t.Fatalf("sessionId = %#v, want per-case eval session", req["sessionId"])
			}
			meta, ok := req["metadata"].(map[string]any)
			if !ok || meta["eval.caseId"] != "server-case" || meta["eval.rootCauseCategory"] != "tool" {
				t.Fatalf("metadata = %#v, want eval case metadata", req["metadata"])
			}
			postSeen = true
			writeTestJSON(t, w, map[string]any{
				"accepted":  true,
				"sessionId": "sess-server",
				"turnId":    "turn-server",
				"status":    "accepted",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state":
			polls++
			active := polls == 1
			writeTestJSON(t, w, serverStatePayload(active))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	agent := ServerAgent{
		Config: ServerAgentConfig{
			BaseURL:      server.URL,
			RunID:        "server-run",
			PollTimeout:  2 * time.Second,
			PollInterval: time.Millisecond,
		},
	}
	output, err := agent.Run(context.Background(), Case{
		ID:                "server-case",
		Category:          "server",
		RootCauseCategory: "tool",
		Input:             "inspect payment-api",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !postSeen {
		t.Fatal("server did not receive chat message POST")
	}
	if polls < 2 {
		t.Fatalf("polls = %d, want at least 2", polls)
	}
	if output.Answer != "payment-api is healthy" {
		t.Fatalf("answer = %q", output.Answer)
	}
	if len(output.Events) != 2 || output.Events[1].Kind != agentui.AgentEventTool {
		t.Fatalf("events = %#v", output.Events)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Name != "read_file" || !strings.Contains(string(output.ToolCalls[0].Arguments), "app.log") {
		t.Fatalf("tool calls = %#v", output.ToolCalls)
	}
	if len(output.TurnItems) == 0 {
		t.Fatal("expected turn items derived from agent events")
	}
	if output.TurnItems[0].Type != agentstate.TurnItemTypeToolCall {
		t.Fatalf("first turn item type = %q, want tool_call", output.TurnItems[0].Type)
	}
}

func TestServerAgentRunIgnoresStaleStateForPreviousTurn(t *testing.T) {
	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/chat/message":
			writeTestJSON(t, w, map[string]any{
				"accepted":        true,
				"sessionId":       "eval-current-run-current-case",
				"turnId":          "turn-current",
				"clientTurnId":    "eval-current-run-current-case",
				"clientMessageId": "eval-current-run-current-case-message",
				"status":          "accepted",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state":
			polls++
			if polls == 1 {
				writeTestJSON(t, w, map[string]any{
					"sessionId": "sess-stale",
					"cards": []map[string]any{
						{"role": "assistant", "text": "stale answer", "clientTurnId": "eval-old-case"},
					},
					"runtime": map[string]any{
						"turn": map[string]any{
							"active":       false,
							"phase":        "failed",
							"clientTurnId": "eval-old-case",
						},
					},
				})
				return
			}
			writeTestJSON(t, w, map[string]any{
				"sessionId": "eval-current-run-current-case",
				"cards": []map[string]any{
					{"role": "assistant", "text": "fresh answer", "clientTurnId": "eval-current-run-current-case"},
				},
				"runtime": map[string]any{
					"turn": map[string]any{
						"active":          false,
						"phase":           "completed",
						"clientTurnId":    "eval-current-run-current-case",
						"clientMessageId": "eval-current-run-current-case-message",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	agent := ServerAgent{
		Config: ServerAgentConfig{
			BaseURL:      server.URL,
			RunID:        "current-run",
			PollTimeout:  time.Second,
			PollInterval: time.Millisecond,
		},
	}
	output, err := agent.Run(context.Background(), Case{
		ID:       "current-case",
		Category: "server",
		Input:    "hello",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if polls < 2 {
		t.Fatalf("polls = %d, want stale state ignored and fresh state polled", polls)
	}
	if output.Answer != "fresh answer" {
		t.Fatalf("answer = %q, want fresh answer", output.Answer)
	}
}

func TestServerAgentRunReturnsClearErrorForFailedTurn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/chat/message":
			writeTestJSON(t, w, map[string]any{
				"accepted":  true,
				"sessionId": "sess-failed",
				"turnId":    "turn-failed",
				"status":    "accepted",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state":
			writeTestJSON(t, w, map[string]any{
				"sessionId": "sess-failed",
				"runtime": map[string]any{
					"turn": map[string]any{"active": false, "phase": "failed"},
				},
				"config": map[string]any{
					"agentItemEvents": []agentui.AgentEvent{{
						EventID:    "turn-item:turn-failed:error",
						Seq:        1,
						SessionID:  "sess-failed",
						TurnID:     "turn-failed",
						Kind:       agentui.AgentEventTurn,
						Phase:      agentui.AgentEventPhaseFailed,
						Status:     agentui.AgentEventStatusFailed,
						Visibility: agentui.AgentEventVisibilityPrimary,
						Source:     agentui.AgentEventSourceProjection,
						Payload:    json.RawMessage(`{"error":"model config missing"}`),
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	agent := ServerAgent{Config: ServerAgentConfig{
		BaseURL:      server.URL,
		PollTimeout:  time.Second,
		PollInterval: time.Millisecond,
	}}
	output, err := agent.Run(context.Background(), Case{ID: "failed-case", Category: "server", Input: "hello"})
	if err == nil || !strings.Contains(err.Error(), "model config missing") {
		t.Fatalf("error = %v, want model config detail", err)
	}
	if len(output.Events) != 1 || len(output.TurnItems) != 1 {
		t.Fatalf("output should retain failed-turn artifacts: %#v", output)
	}
}

func TestServerAgentRunTimesOutBlockedChatPost(t *testing.T) {
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

func TestServerRunErrorIgnoresRecoveredToolFailureWithFinalAnswer(t *testing.T) {
	state := serverStateSnapshot{
		Runtime: serverRuntimeSnapshot{Turn: serverRuntimeTurnSnapshot{Active: false, Phase: "completed"}},
	}
	output := RunOutput{
		Answer: "工具失败已按 FailurePolicy 处理，并给出验证方式。",
		Events: []agentui.AgentEvent{{
			EventID: "tool-result-failed",
			Kind:    agentui.AgentEventTool,
			Phase:   agentui.AgentEventPhaseFailed,
			Status:  agentui.AgentEventStatusFailed,
			Payload: json.RawMessage(`{"toolName":"exec_command","outputSummary":"exec_command failed"}`),
		}},
	}

	if errMsg := serverRunError(state, serverChatResponse{}, output); errMsg != "" {
		t.Fatalf("serverRunError() = %q, want no run error for recovered failed tool", errMsg)
	}
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

func serverStatePayload(active bool) map[string]any {
	events := []agentui.AgentEvent{
		{
			EventID:    "turn-item:turn-server:tool-call-1",
			Seq:        1,
			SessionID:  "sess-server",
			TurnID:     "turn-server",
			Kind:       agentui.AgentEventTool,
			Phase:      agentui.AgentEventPhaseStarted,
			Status:     agentui.AgentEventStatusCompleted,
			Visibility: agentui.AgentEventVisibilityPrimary,
			Source:     agentui.AgentEventSourceProjection,
			Payload:    json.RawMessage(`{"toolCallId":"call-1","toolName":"read_file","inputSummary":"app.log"}`),
		},
		{
			EventID:    "turn-item:turn-server:tool-result-1",
			Seq:        2,
			SessionID:  "sess-server",
			TurnID:     "turn-server",
			Kind:       agentui.AgentEventTool,
			Phase:      agentui.AgentEventPhaseCompleted,
			Status:     agentui.AgentEventStatusCompleted,
			Visibility: agentui.AgentEventVisibilityPrimary,
			Source:     agentui.AgentEventSourceProjection,
			Payload:    json.RawMessage(`{"toolCallId":"call-1","toolName":"read_file","outputSummary":"ok"}`),
		},
	}
	return map[string]any{
		"sessionId": "sess-server",
		"cards": []map[string]any{
			{"role": "user", "text": "inspect payment-api", "clientTurnId": "eval-server-run-server-case"},
			{"role": "assistant", "text": "payment-api is healthy", "clientTurnId": "eval-server-run-server-case"},
		},
		"toolInvocations": []map[string]any{
			{"id": "call-1", "name": "read_file", "inputJson": `{"path":"app.log"}`, "status": "completed"},
		},
		"runtime": map[string]any{
			"turn": map[string]any{
				"active":       active,
				"phase":        "completed",
				"clientTurnId": "eval-server-run-server-case",
			},
		},
		"config": map[string]any{
			"agentItemEvents": events,
		},
		"agentEventProjection": map[string]any{
			"sessionId":     "sess-server",
			"currentTurnId": "turn-server",
			"finalMessages": map[string]any{
				"turn-server": map[string]any{
					"turnId": "turn-server",
					"text":   "payment-api is healthy",
					"status": "completed",
				},
			},
		},
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
