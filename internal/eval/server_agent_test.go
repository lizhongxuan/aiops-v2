package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
		clientTurnID := message.Metadata["clientTurnId"]
		if clientTurnID == "" {
			t.Fatalf("metadata missing server-recognized clientTurnId: %#v", message.Metadata)
		}
		writer := newServerTransportTestWriter(w)
		finalBlock := canonicalServerTransportFinalBlock("final-1", "payment-api is healthy", "completed")
		finalBlock["text"] = "payment-api "
		writeServerTransportTestOps(t, writer,
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-server"),
			serverTransportTestOp("set", []any{"status"}, "working"),
			serverTransportTestOp("set", []any{"runtimeLiveness", "activeTurns", "turn-server"}, true),
			serverTransportTestOp("set", []any{"turns", "turn-server"}, map[string]any{
				"id": "turn-server", "clientTurnId": clientTurnID, "clientMessageId": message.ID,
				"status": "working", "user": map[string]any{"id": message.ID, "text": "inspect payment-api"},
				"agentItems": []map[string]any{
					transportAgentItem("model-1", "model_call", "completed", "model response", map[string]any{
						"promptFingerprint": map[string]any{"stableHash": "stable-hash", "developerHash": "developer-hash"},
					}),
					transportAgentItem("tool-call-1", "tool_call", "completed", "read app.log", map[string]any{
						"id": "call-1", "toolName": "read_file", "arguments": map[string]any{"path": "app.log"},
					}),
					transportAgentItem("approval-1", "approval", "completed", "approved", map[string]any{"approvalId": "approval-1", "decision": "approved"}),
					transportAgentItem("evidence-1", "evidence", "completed", "log evidence", map[string]any{"evidenceId": "evidence-1", "source": "app.log"}),
				},
				"blockOrder": []string{"display-only", "final-1"},
				"blocksById": map[string]any{
					"display-only": canonicalServerTransportProcessBlock("display-only", "file", "completed", "presentation only", map[string]any{
						"source": "must_not_become_tool", "toolCallId": "process-call",
					}),
					"final-1": finalBlock,
				},
			}),
		)
		writeServerTransportTestOps(t, writer,
			serverTransportTestOp("append-text", []any{"turns", "turn-server", "blocksById", "final-1", "text"}, "is healthy"),
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
	if string(output.ToolCalls[0].Arguments) != `{"path":"app.log"}` {
		t.Fatalf("tool arguments = %s", output.ToolCalls[0].Arguments)
	}
	if len(output.TurnItems) != 4 {
		t.Fatalf("turn items = %#v, want canonical transport items", output.TurnItems)
	}
	score := ScoreCase(Case{ID: "server-case", Category: "server"}, output)
	if len(score.PromptFingerprints) != 1 || score.PromptFingerprints[0]["developerHash"] != "developer-hash" {
		t.Fatalf("prompt fingerprints = %#v", score.PromptFingerprints)
	}
	if output.TurnItems[2].Type != agentstate.TurnItemTypeApproval || output.TurnItems[3].Type != agentstate.TurnItemTypeEvidence {
		t.Fatalf("approval/evidence items not preserved: %#v", output.TurnItems)
	}
	wantEvents := agentui.ProjectTurnItemsToAgentEvents("eval-server-run-server-case", "turn-server", output.TurnItems, 0)
	if !reflect.DeepEqual(output.Events, wantEvents) {
		t.Fatalf("events are not canonical projection:\n got %#v\nwant %#v", output.Events, wantEvents)
	}
}

func TestServerAgentAssistantTransportAccumulatorFailsClosed(t *testing.T) {
	tests := []struct{ name, line, wantErr string }{
		{name: "unknown operation", line: `aui-state:[{"type":"merge","path":["status"],"value":"idle"}]`, wantErr: "unsupported operation"},
		{name: "missing path", line: `aui-state:[{"type":"set","value":"idle"}]`, wantErr: "path is required"},
		{name: "unknown typed path", line: `aui-state:[{"type":"set","path":["legacySnapshot"],"value":{}}]`, wantErr: "unknown field"},
		{name: "legacy process field", line: `aui-state:[{"type":"set","path":["turns","turn-1"],"value":{"id":"turn-1","status":"working","blockOrder":[],"blocksById":{},"process":[]}}]`, wantErr: "unknown field"},
		{name: "legacy final field", line: `aui-state:[{"type":"set","path":["turns","turn-1"],"value":{"id":"turn-1","status":"completed","blockOrder":[],"blocksById":{},"final":{"id":"final-1","status":"completed"}}}]`, wantErr: "unknown field"},
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

func TestServerAgentAssistantTransportCanonicalBlocksFailClosed(t *testing.T) {
	finalBlock := appui.AiopsTransportBlock{
		Type: appui.AiopsTransportBlockTypeFinalAnswer,
		AiopsProcessBlock: appui.AiopsProcessBlock{
			ID: "final-1", Kind: appui.AiopsTransportProcessKindAssistant, Status: appui.AiopsTransportProcessStatusCompleted,
		},
		FinalContract: &appui.AiopsTransportFinal{ID: "final-1", Status: appui.AiopsTransportFinalStatusCompleted},
	}
	tests := []struct {
		name string
		turn appui.AiopsTransportTurn
		want string
	}{
		{name: "missing ordered block", turn: appui.AiopsTransportTurn{ID: "turn-1", BlockOrder: []string{"final-1"}, BlocksByID: map[string]appui.AiopsTransportBlock{}}, want: "missing block"},
		{name: "duplicate order entry", turn: appui.AiopsTransportTurn{ID: "turn-1", BlockOrder: []string{"final-1", "final-1"}, BlocksByID: map[string]appui.AiopsTransportBlock{"final-1": finalBlock}}, want: "duplicate"},
		{name: "unordered block", turn: appui.AiopsTransportTurn{ID: "turn-1", BlocksByID: map[string]appui.AiopsTransportBlock{"final-1": finalBlock}}, want: "unordered blocks"},
		{name: "block id mismatch", turn: appui.AiopsTransportTurn{ID: "turn-1", BlockOrder: []string{"other"}, BlocksByID: map[string]appui.AiopsTransportBlock{"other": finalBlock}}, want: "has id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := serverTransportCanonicalBlocks(test.turn); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("serverTransportCanonicalBlocks() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestServerAgentAssistantTransportUsesTypedStatusNotFinalMarkdown(t *testing.T) {
	answer := "# FAILED\nPending approval mentioned only in prose."
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return completedServerTransportOps("turn-typed", clientTurnID, messageID, answer)
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
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return []map[string]any{
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-failed"),
			serverTransportTestOp("set", []any{"turns", "turn-failed"}, map[string]any{
				"id": "turn-failed", "clientTurnId": clientTurnID, "clientMessageId": messageID,
				"status": "failed", "user": map[string]any{"id": messageID, "text": "hello"},
				"agentItems": []map[string]any{transportAgentItem("failure-1", "error", "failed", "request failed", nil)},
				"blockOrder": []string{"failure-1", "final-failed"},
				"blocksById": map[string]any{
					"failure-1":    canonicalServerTransportProcessBlock("failure-1", "system", "failed", "request failed", nil),
					"final-failed": canonicalServerTransportFinalBlock("final-failed", "failure summary", "failed"),
				},
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
	if len(output.Events) != 1 || len(output.TurnItems) != 1 {
		t.Fatalf("output should retain typed failed-turn facts: %#v", output)
	}
}

func TestServerAgentAssistantTransportReturnsTypedFinalContractFailure(t *testing.T) {
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return []map[string]any{
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-final-failed"),
			serverTransportTestOp("set", []any{"turns", "turn-final-failed"}, map[string]any{
				"id": "turn-final-failed", "clientTurnId": clientTurnID, "clientMessageId": messageID,
				"status": "completed", "blockOrder": []string{"final-failed"},
				"blocksById": map[string]any{"final-failed": canonicalServerTransportFinalBlock("final-failed", "typed failure", "failed")},
			}),
			serverTransportTestOp("set", []any{"status"}, "idle"),
		}
	})
	defer server.Close()
	_, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "typed-final-failure", Category: "server", Input: "hello"},
	)
	if err == nil || !strings.Contains(err.Error(), "server turn failed") {
		t.Fatalf("error = %v, want typed finalContract status failure", err)
	}
}

func TestServerAgentAssistantTransportRejectsTerminalTurnWithActiveToolFacts(t *testing.T) {
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return append(completedServerTransportOps("turn-active", clientTurnID, messageID, "typed final"),
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

func TestServerAgentAssistantTransportRejectsCurrentTurnFallback(t *testing.T) {
	state := appui.NewAiopsTransportState("session-1", "thread-1")
	state.CurrentTurnID = "unrelated-turn"
	state.Turns["unrelated-turn"] = appui.AiopsTransportTurn{
		ID: "unrelated-turn", Status: appui.AiopsTransportTurnStatusCompleted,
		User: &appui.AiopsTransportMessage{ID: "different-message", Text: "stale"},
	}
	if turnID, _, err := serverTransportTargetTurn(state, "target-client-turn", "target-message"); err == nil {
		t.Fatalf("target correlation accepted CurrentTurnID fallback %q", turnID)
	}
}

func TestServerAgentAssistantTransportRejectsAmbiguousMessageCorrelation(t *testing.T) {
	state := appui.NewAiopsTransportState("session-1", "thread-1")
	for _, turnID := range []string{"turn-a", "turn-b"} {
		state.Turns[turnID] = appui.AiopsTransportTurn{
			ID: turnID, ClientTurnID: "same-client-turn", ClientMessageID: "same-message", Status: appui.AiopsTransportTurnStatusCompleted,
			User: &appui.AiopsTransportMessage{ID: "same-message", Text: "duplicate"},
		}
	}
	if turnID, _, err := serverTransportTargetTurn(state, "same-client-turn", "same-message"); err == nil {
		t.Fatalf("target correlation accepted ambiguous turn %q", turnID)
	}
}

func TestServerAgentAssistantTransportCompletedWithoutFinalTextIsNotRunError(t *testing.T) {
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return []map[string]any{
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-empty-final"),
			serverTransportTestOp("set", []any{"turns", "turn-empty-final"}, map[string]any{
				"id": "turn-empty-final", "clientTurnId": clientTurnID, "clientMessageId": messageID, "status": "completed",
				"user":       map[string]any{"id": messageID, "text": "hello"},
				"blockOrder": []string{"final-empty"},
				"blocksById": map[string]any{"final-empty": canonicalServerTransportFinalBlock("final-empty", "", "completed")},
			}),
			serverTransportTestOp("set", []any{"status"}, "idle"),
		}
	})
	defer server.Close()
	output, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "empty-final", Category: "server", Input: "hello"},
	)
	if err != nil {
		t.Fatalf("typed completed turn returned text-derived run error: %v", err)
	}
	if output.Answer != "" {
		t.Fatalf("answer = %q, want empty output fact", output.Answer)
	}
}

func TestServerAgentAssistantTransportRejectsFinalAnswerWithoutTypedContract(t *testing.T) {
	server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
		return []map[string]any{
			serverTransportTestOp("set", []any{"currentTurnId"}, "turn-untyped-final"),
			serverTransportTestOp("set", []any{"turns", "turn-untyped-final"}, map[string]any{
				"id": "turn-untyped-final", "clientTurnId": clientTurnID, "clientMessageId": messageID, "status": "completed",
				"blockOrder": []string{"final-1"},
				"blocksById": map[string]any{
					"final-1": canonicalServerTransportProcessBlock("final-1", "assistant", "completed", "untyped final prose", map[string]any{
						"type": "final_answer", "phase": "final_answer", "streamState": "complete",
					}),
				},
			}),
			serverTransportTestOp("set", []any{"status"}, "idle"),
		}
	})
	defer server.Close()
	_, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
		context.Background(), Case{ID: "untyped-final", Category: "server", Input: "hello"},
	)
	if err == nil || !strings.Contains(err.Error(), "missing finalContract") {
		t.Fatalf("error = %v, want fail-closed typed finalContract requirement", err)
	}
}

func TestServerAgentAssistantTransportRejectsTruncatedCanonicalFacts(t *testing.T) {
	tests := []struct {
		name       string
		turnFields map[string]any
	}{
		{name: "turn envelope", turnFields: map[string]any{
			"agentItemsTruncated": true, "agentItemsOriginalCount": 12, "agentItemsHash": "sha256:turn",
		}},
		{name: "agent item", turnFields: map[string]any{
			"agentItems": []map[string]any{{
				"schemaVersion": appui.AiopsTransportAgentItemSchemaVersion,
				"id":            "tool-1", "type": "tool_call", "status": "completed", "truncated": true,
				"contentHash": "sha256:item", "payload": map[string]any{"summary": "bounded"},
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newServerTransportStateServer(t, func(clientTurnID, messageID string) []map[string]any {
				turn := map[string]any{
					"id": "turn-truncated", "clientTurnId": clientTurnID, "clientMessageId": messageID,
					"status": "completed", "blockOrder": []string{"final-1"},
					"blocksById": map[string]any{"final-1": canonicalServerTransportFinalBlock("final-1", "", "completed")},
				}
				for key, value := range tt.turnFields {
					turn[key] = value
				}
				return []map[string]any{
					serverTransportTestOp("set", []any{"currentTurnId"}, "turn-truncated"),
					serverTransportTestOp("set", []any{"turns", "turn-truncated"}, turn),
					serverTransportTestOp("set", []any{"status"}, "idle"),
				}
			})
			defer server.Close()
			_, err := (ServerAgent{Config: ServerAgentConfig{BaseURL: server.URL, PollTimeout: time.Second}}).Run(
				context.Background(), Case{ID: "truncated", Category: "server", Input: "hello"},
			)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "truncated") {
				t.Fatalf("error = %v, want fail-closed truncation error", err)
			}
		})
	}
}

func TestServerAgentAssistantTransportCompletionReadsPendingAndLivenessFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*appui.AiopsTransportState, *appui.AiopsTransportTurn)
	}{
		{name: "pending approval", mutate: func(state *appui.AiopsTransportState, _ *appui.AiopsTransportTurn) {
			state.PendingApprovals["approval-1"] = appui.AiopsTransportApproval{ID: "approval-1", TurnID: "turn-1"}
		}},
		{name: "runtime pending approval", mutate: func(state *appui.AiopsTransportState, _ *appui.AiopsTransportTurn) {
			state.RuntimeLiveness.PendingApprovals["approval-1"] = true
		}},
		{name: "active turn", mutate: func(state *appui.AiopsTransportState, _ *appui.AiopsTransportTurn) {
			state.RuntimeLiveness.ActiveTurns["turn-1"] = true
		}},
		{name: "active command stream", mutate: func(state *appui.AiopsTransportState, _ *appui.AiopsTransportTurn) {
			state.RuntimeLiveness.ActiveCommandStreams["call-1"] = true
		}},
		{name: "running typed tool", mutate: func(_ *appui.AiopsTransportState, turn *appui.AiopsTransportTurn) {
			turn.BlockOrder = append([]string{"call-1"}, turn.BlockOrder...)
			turn.BlocksByID["call-1"] = appui.AiopsTransportBlock{
				Type: appui.AiopsTransportBlockType(appui.AiopsTransportProcessKindTool),
				AiopsProcessBlock: appui.AiopsProcessBlock{
					ID: "call-1", Kind: appui.AiopsTransportProcessKindTool, Status: appui.AiopsTransportProcessStatusRunning,
				},
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := appui.NewAiopsTransportState("session-1", "thread-1")
			state.Status = appui.AiopsTransportStatusIdle
			turn := appui.AiopsTransportTurn{
				ID: "turn-1", Status: appui.AiopsTransportTurnStatusCompleted,
				BlockOrder: []string{"final-1"},
				BlocksByID: map[string]appui.AiopsTransportBlock{
					"final-1": {
						Type: appui.AiopsTransportBlockTypeFinalAnswer,
						AiopsProcessBlock: appui.AiopsProcessBlock{
							ID: "final-1", Kind: appui.AiopsTransportProcessKindAssistant, Status: appui.AiopsTransportProcessStatusCompleted,
						},
						FinalContract: &appui.AiopsTransportFinal{ID: "final-1", Status: appui.AiopsTransportFinalStatusCompleted},
					},
				},
			}
			tt.mutate(&state, &turn)
			if serverTransportSettled(state, "turn-1", turn) {
				t.Fatal("completion ignored typed pending/liveness fact")
			}
		})
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

func newServerTransportStateServer(t *testing.T, ops func(string, string) []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != serverAssistantTransportPath {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		request := decodeServerTransportTestRequest(t, r)
		message := request.Commands[0].Message
		writer := newServerTransportTestWriter(w)
		writeServerTransportTestOps(t, writer, ops(message.Metadata["clientTurnId"], message.ID)...)
		flushServerTransportTestWriter(t, writer)
	}))
}

func completedServerTransportOps(turnID, clientTurnID, messageID, answer string) []map[string]any {
	return []map[string]any{
		serverTransportTestOp("set", []any{"currentTurnId"}, turnID),
		serverTransportTestOp("set", []any{"turns", turnID}, map[string]any{
			"id": turnID, "clientTurnId": clientTurnID, "clientMessageId": messageID,
			"status": "completed", "user": map[string]any{"id": messageID, "text": "hello"},
			"agentItems": []map[string]any{},
			"blockOrder": []string{turnID + ":final"},
			"blocksById": map[string]any{turnID + ":final": canonicalServerTransportFinalBlock(turnID+":final", answer, "completed")},
		}),
		serverTransportTestOp("set", []any{"status"}, "idle"),
	}
}

func canonicalServerTransportProcessBlock(id, kind, status, text string, fields map[string]any) map[string]any {
	block := map[string]any{
		"type": kind, "id": id, "kind": kind, "status": status, "text": text,
	}
	for key, value := range fields {
		block[key] = value
	}
	return block
}

func canonicalServerTransportFinalBlock(id, answer, status string) map[string]any {
	processStatus := status
	streamState := "complete"
	if status == "running" {
		processStatus = "running"
		streamState = "streaming"
	}
	return map[string]any{
		"type": "final_answer", "id": id, "kind": "assistant", "displayKind": "assistant.message",
		"phase": "final_answer", "streamState": streamState, "status": processStatus, "text": answer,
		"finalContract": map[string]any{"id": id, "text": answer, "answerText": answer, "status": status},
	}
}

func transportAgentItem(id, itemType, status, summary string, data map[string]any) map[string]any {
	payload := map[string]any{"summary": summary}
	if data != nil {
		payload["data"] = data
	}
	return map[string]any{
		"schemaVersion": appui.AiopsTransportAgentItemSchemaVersion,
		"id":            id, "type": itemType, "status": status, "payload": payload,
		"createdAt": "2026-07-12T01:02:03Z", "updatedAt": "2026-07-12T01:02:04Z",
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
