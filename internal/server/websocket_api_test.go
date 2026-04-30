package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
	"golang.org/x/net/websocket"
)

type websocketAPITestRuntime struct{}

func (websocketAPITestRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}
func (websocketAPITestRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}
func (websocketAPITestRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestAppWebSocket_StreamsInitialSnapshotAndHeartbeat(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.Messages = []runtimekernel.Message{{ID: "msg-1", Role: "assistant", Content: "hello ws", Timestamp: time.Now().UTC()}}
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, sessionMgr),
		WithAppWebSocketHeartbeat(15*time.Millisecond),
	)
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	origin := "http://example.test/"
	conn, err := websocket.Dial(wsURL, "", origin)
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var snapshot appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &snapshot); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}
	if snapshot.SessionID != session.ID {
		t.Fatalf("snapshot.sessionId = %q, want %q", snapshot.SessionID, session.ID)
	}
	if snapshot.Kind != "workspace" {
		t.Fatalf("snapshot.kind = %q, want workspace", snapshot.Kind)
	}

	var heartbeat map[string]any
	if err := websocket.JSON.Receive(conn, &heartbeat); err != nil {
		t.Fatalf("receive heartbeat: %v", err)
	}
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("heartbeat.type = %v, want heartbeat", heartbeat["type"])
	}
}

func TestAppWebSocket_InitialSnapshotMatchesStateAPI(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.Messages = []runtimekernel.Message{{ID: "msg-1", Role: "user", Content: "same snapshot", Timestamp: time.Now().UTC()}}
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, sessionMgr), WithAppWebSocketHeartbeat(20*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	stateResp, err := ts.Client().Get(ts.URL + "/api/v1/state")
	if err != nil {
		t.Fatalf("GET /api/v1/state error = %v", err)
	}
	defer stateResp.Body.Close()
	var stateSnapshot appui.StateSnapshot
	if err := json.NewDecoder(stateResp.Body).Decode(&stateSnapshot); err != nil {
		t.Fatalf("decode state snapshot: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var wsSnapshot appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &wsSnapshot); err != nil {
		t.Fatalf("receive ws snapshot: %v", err)
	}
	if wsSnapshot.SessionID != stateSnapshot.SessionID || wsSnapshot.Kind != stateSnapshot.Kind || wsSnapshot.SelectedHostID != stateSnapshot.SelectedHostID {
		t.Fatalf("ws snapshot = %+v, want same session/kind/host as state %+v", wsSnapshot, stateSnapshot)
	}
}

func TestAppWebSocket_StateAPIAndInitialSnapshotIncludeAgentEventProjection(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.HostID = "host-a"
	sessionMgr.Update(session)
	services := appui.NewServices(websocketAPITestRuntime{}, sessionMgr)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:    "evt-agent-projection-baseline",
		SessionID:  session.ID,
		TurnID:     "turn-agent-projection-baseline",
		Kind:       appui.AgentEventTurn,
		Phase:      appui.AgentEventPhaseRequested,
		Status:     appui.AgentEventStatusQueued,
		Visibility: appui.AgentEventVisibilityPrimary,
		Source:     appui.AgentEventSourceUI,
		CreatedAt:  now,
		Payload:    json.RawMessage(`{"title":"修复数据库复制异常","prompt":"修复数据库复制异常"}`),
	}); err != nil {
		t.Fatalf("append agent event: %v", err)
	}

	httpServer := NewHTTPServer(services, WithAppWebSocketHeartbeat(20*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	stateResp, err := ts.Client().Get(ts.URL + "/api/v1/state")
	if err != nil {
		t.Fatalf("GET /api/v1/state error = %v", err)
	}
	defer stateResp.Body.Close()
	var stateSnapshot appui.StateSnapshot
	if err := json.NewDecoder(stateResp.Body).Decode(&stateSnapshot); err != nil {
		t.Fatalf("decode state snapshot: %v", err)
	}
	if stateSnapshot.AgentEventProjection == nil {
		t.Fatal("state snapshot missing agentEventProjection")
	}
	if stateSnapshot.AgentEventProjection.SessionID != session.ID || stateSnapshot.AgentEventProjection.Status != "working" {
		t.Fatalf("state agentEventProjection = %+v, want session %q working", stateSnapshot.AgentEventProjection, session.ID)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var wsSnapshot appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &wsSnapshot); err != nil {
		t.Fatalf("receive ws snapshot: %v", err)
	}
	if wsSnapshot.AgentEventProjection == nil {
		t.Fatal("websocket initial snapshot missing agentEventProjection")
	}
	if wsSnapshot.AgentEventProjection.SessionID != stateSnapshot.AgentEventProjection.SessionID || wsSnapshot.AgentEventProjection.LastSeq != stateSnapshot.AgentEventProjection.LastSeq {
		t.Fatalf("ws agentEventProjection = %+v, want same baseline as state %+v", wsSnapshot.AgentEventProjection, stateSnapshot.AgentEventProjection)
	}
}

func TestAppWebSocket_DoesNotReplayEventsAlreadyCoveredByInitialProjection(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessionMgr.Update(session)
	services := appui.NewServices(websocketAPITestRuntime{}, sessionMgr)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:    "evt-existing-delta",
		SessionID:  session.ID,
		TurnID:     "turn-existing",
		Kind:       appui.AgentEventAssistant,
		Phase:      appui.AgentEventPhaseDelta,
		Status:     appui.AgentEventStatusRunning,
		Visibility: appui.AgentEventVisibilityPrimary,
		Source:     appui.AgentEventSourceRuntime,
		CreatedAt:  now,
		Payload:    json.RawMessage(`{"channel":"final","delta":"already in snapshot"}`),
	}); err != nil {
		t.Fatalf("append agent event: %v", err)
	}

	httpServer := NewHTTPServer(services, WithAppWebSocketHeartbeat(time.Hour))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(250 * time.Millisecond))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}
	if initial.AgentEventProjection == nil || initial.AgentEventProjection.LastSeq != 1 {
		t.Fatalf("initial projection = %+v, want lastSeq 1", initial.AgentEventProjection)
	}

	var payload map[string]any
	if err := websocket.JSON.Receive(conn, &payload); err == nil {
		t.Fatalf("received replay payload after initial snapshot = %+v, want no already-covered event", payload)
	}
}

func TestAppWebSocket_PushesUpdatedSnapshotWhenProjectionEventArrives(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.Messages = []runtimekernel.Message{{ID: "msg-1", Role: "assistant", Content: "before update", Timestamp: time.Now().UTC()}}
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, sessionMgr), WithAppWebSocketHeartbeat(50*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}
	if len(initial.Cards) != 1 || initial.Cards[0].Text != "before update" {
		t.Fatalf("initial cards = %+v, want before update", initial.Cards)
	}

	session.Messages = append(session.Messages, runtimekernel.Message{
		ID:        "msg-2",
		Role:      "assistant",
		Content:   "after update",
		Timestamp: time.Now().UTC(),
	})
	session.UpdatedAt = time.Now().UTC().Add(time.Millisecond)
	sessionMgr.Update(session)

	httpServer.ProjectionSubscriber().OnSnapshot(projection.Snapshot{
		SessionID: session.ID,
		TurnID:    "turn-1",
		Timestamp: time.Now().UTC(),
	})

	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive payload: %v", err)
		}
		if payload["type"] == "heartbeat" {
			continue
		}
		if payload["type"] == "agent_event" || payload["type"] == "turn"+"_"+"event" {
			continue
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		var snapshot appui.StateSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Fatalf("decode snapshot: %v", err)
		}
		if len(snapshot.Cards) < 2 {
			t.Fatalf("updated cards = %+v, want appended card", snapshot.Cards)
		}
		if got := snapshot.Cards[len(snapshot.Cards)-1].Text; got != "after update" {
			t.Fatalf("latest card text = %q, want after update", got)
		}
		break
	}
}

func TestAppWebSocket_DoesNotEmitLegacyTurnEventEnvelope(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, sessionMgr), WithAppWebSocketHeartbeat(50*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}

	httpServer.ProjectionSubscriber().OnToolInvocation(projection.ToolInvocation{
		ID:        "tool-1",
		SessionID: session.ID,
		TurnID:    "turn-1",
		ToolName:  "web_search",
		Status:    projection.ToolInvocationStarted,
		StartedAt: time.Now().UTC(),
	})

	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive payload: %v", err)
		}
		messageType, _ := payload["type"].(string)
		switch messageType {
		case "":
			if payload["sessionId"] == "" {
				t.Fatalf("unexpected untyped websocket payload: %+v", payload)
			}
			continue
		case "heartbeat":
			continue
		case "snapshot", "agent_event":
			return
		case "turn" + "_" + "event":
			t.Fatalf("legacy turn event envelope emitted on /ws: %+v", payload)
		default:
			t.Fatalf("unexpected websocket message type %v, payload %+v", payload["type"], payload)
		}
	}
}

func TestAppWebSocket_PushesAgentEventEnvelopeWhenToolProjectionArrives(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, sessionMgr), WithAppWebSocketHeartbeat(50*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}

	httpServer.ProjectionSubscriber().OnToolInvocation(projection.ToolInvocation{
		ID:        "tool-1",
		SessionID: session.ID,
		TurnID:    "turn-1",
		ToolName:  "web_search",
		Status:    projection.ToolInvocationStarted,
		StartedAt: time.Now().UTC(),
	})

	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive payload: %v", err)
		}
		if payload["type"] == "heartbeat" {
			continue
		}
		if payload["type"] != "agent_event" {
			continue
		}
		eventMap, ok := payload["event"].(map[string]any)
		if !ok {
			t.Fatalf("agent_event payload = %+v, want event object", payload)
		}
		if eventMap["kind"] != string(appui.AgentEventTool) || eventMap["phase"] != string(appui.AgentEventPhaseStarted) {
			t.Fatalf("event kind/phase = %v/%v, want tool/started", eventMap["kind"], eventMap["phase"])
		}
		if eventMap["sessionId"] != session.ID || eventMap["turnId"] != "turn-1" {
			t.Fatalf("event ids = %+v, want session %q turn-1", eventMap, session.ID)
		}
		if eventMap["eventId"] == "" || eventMap["seq"] == nil || eventMap["createdAt"] == "" {
			t.Fatalf("event envelope missing stable fields: %+v", eventMap)
		}
		break
	}
}

func TestAppWebSocket_PushesAgentEventEnvelopeWhenActivityProjectionArrives(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessionMgr.Update(session)

	httpServer := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, sessionMgr), WithAppWebSocketHeartbeat(50*time.Millisecond))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}

	httpServer.ProjectionSubscriber().OnActivity(projection.ActivityStats{
		SessionID: session.ID,
		TurnID:    "turn-activity-1",
		Iteration: 0,
		Stage:     "call_model",
	})

	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive payload: %v", err)
		}
		if payload["type"] == "heartbeat" {
			continue
		}
		if payload["type"] != "agent_event" {
			continue
		}
		eventMap, ok := payload["event"].(map[string]any)
		if !ok {
			t.Fatalf("agent_event payload = %+v, want event object", payload)
		}
		if eventMap["kind"] != string(appui.AgentEventSystem) || eventMap["phase"] != string(appui.AgentEventPhaseUpdated) {
			t.Fatalf("event kind/phase = %v/%v, want system/updated", eventMap["kind"], eventMap["phase"])
		}
		eventPayload, _ := eventMap["payload"].(map[string]any)
		if eventPayload["displayKind"] != "runtime.activity" || eventPayload["stage"] != "call_model" {
			t.Fatalf("event payload = %+v, want runtime.activity call_model", eventPayload)
		}
		break
	}
}

func TestAppWebSocket_PushesTerminalSnapshotAfterTerminalAgentEvent(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	session := sessionMgr.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:              "turn-terminal-1",
		ClientTurnID:    "client-turn-terminal-1",
		ClientMessageID: "client-msg-terminal-1",
		SessionID:       session.ID,
		SessionType:     runtimekernel.SessionTypeHost,
		Mode:            runtimekernel.ModeChat,
		Lifecycle:       runtimekernel.TurnLifecycleRunning,
		ResumeState:     runtimekernel.TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
	}
	sessionMgr.Update(session)
	services := appui.NewServices(websocketAPITestRuntime{}, sessionMgr)

	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:      "turn-terminal-1:turn.started",
		SessionID:    session.ID,
		TurnID:       "turn-terminal-1",
		ClientTurnID: "client-turn-terminal-1",
		Kind:         appui.AgentEventTurn,
		Phase:        appui.AgentEventPhaseStarted,
		Status:       appui.AgentEventStatusRunning,
		Visibility:   appui.AgentEventVisibilityPrimary,
		Source:       appui.AgentEventSourceRuntime,
		CreatedAt:    now.Format(time.RFC3339Nano),
		Payload:      json.RawMessage(`{"title":"will fail","clientMessageId":"client-msg-terminal-1"}`),
	}); err != nil {
		t.Fatalf("append started event: %v", err)
	}

	httpServer := NewHTTPServer(services, WithAppWebSocketHeartbeat(time.Hour))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}
	if initial.AgentEventProjection == nil || initial.AgentEventProjection.Status != "working" {
		t.Fatalf("initial projection = %+v, want working", initial.AgentEventProjection)
	}

	completedAt := now.Add(time.Second)
	session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleFailed
	session.CurrentTurn.Error = "model provider returned 429"
	session.CurrentTurn.UpdatedAt = completedAt
	session.CurrentTurn.CompletedAt = &completedAt
	sessionMgr.Update(session)

	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:      "turn-terminal-1:turn.failed.async",
		SessionID:    session.ID,
		TurnID:       "turn-terminal-1",
		ClientTurnID: "client-turn-terminal-1",
		Kind:         appui.AgentEventTurn,
		Phase:        appui.AgentEventPhaseFailed,
		Status:       appui.AgentEventStatusFailed,
		Visibility:   appui.AgentEventVisibilityPrimary,
		Source:       appui.AgentEventSourceSystem,
		CreatedAt:    completedAt.Format(time.RFC3339Nano),
		Payload:      json.RawMessage(`{"summary":"model provider returned 429","error":"model provider returned 429"}`),
	}); err != nil {
		t.Fatalf("append terminal event: %v", err)
	}

	var sawTerminalEvent bool
	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive terminal stream payload: %v", err)
		}
		if payload["type"] == "heartbeat" {
			continue
		}
		if payload["type"] == "agent_event" {
			eventMap, _ := payload["event"].(map[string]any)
			if eventMap["eventId"] == "turn-terminal-1:turn.failed.async" {
				sawTerminalEvent = true
			}
			continue
		}
		raw, _ := json.Marshal(payload)
		var snapshot appui.StateSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatalf("decode terminal snapshot: %v, payload %+v", err, payload)
		}
		if !sawTerminalEvent {
			t.Fatalf("received snapshot before terminal event: %+v", payload)
		}
		if snapshot.Runtime.Turn.Active || snapshot.Runtime.Turn.Phase != "failed" {
			t.Fatalf("terminal snapshot runtime.turn = %+v, want inactive failed", snapshot.Runtime.Turn)
		}
		if snapshot.AgentEventProjection == nil || snapshot.AgentEventProjection.Status != "failed" {
			t.Fatalf("terminal snapshot projection = %+v, want failed", snapshot.AgentEventProjection)
		}
		break
	}
}

func TestAppWebSocket_ResubscribesAgentEventsWhenSnapshotSessionChanges(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	services := appui.NewServices(websocketAPITestRuntime{}, sessionMgr)
	httpServer := NewHTTPServer(services, WithAppWebSocketHeartbeat(time.Hour))
	ts := httptest.NewServer(httpServer.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var initial appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &initial); err != nil {
		t.Fatalf("receive initial snapshot: %v", err)
	}

	session := sessionMgr.GetOrCreate("session-after-snapshot", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:              "turn-after-snapshot",
		ClientTurnID:    "client-turn-after-snapshot",
		ClientMessageID: "client-msg-after-snapshot",
		SessionID:       session.ID,
		SessionType:     runtimekernel.SessionTypeHost,
		Mode:            runtimekernel.ModeChat,
		Lifecycle:       runtimekernel.TurnLifecycleRunning,
		ResumeState:     runtimekernel.TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
	}
	sessionMgr.Update(session)

	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:      "turn-after-snapshot:turn.started",
		SessionID:    session.ID,
		TurnID:       "turn-after-snapshot",
		ClientTurnID: "client-turn-after-snapshot",
		Kind:         appui.AgentEventTurn,
		Phase:        appui.AgentEventPhaseStarted,
		Status:       appui.AgentEventStatusRunning,
		Visibility:   appui.AgentEventVisibilityPrimary,
		Source:       appui.AgentEventSourceRuntime,
		CreatedAt:    now.Format(time.RFC3339Nano),
		Payload:      json.RawMessage(`{"title":"new session","clientMessageId":"client-msg-after-snapshot"}`),
	}); err != nil {
		t.Fatalf("append started event: %v", err)
	}
	proj, err := services.AgentEventService().Projection(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	httpServer.appSnapshots.Broadcast(appui.StateSnapshot{
		SessionID:      session.ID,
		Kind:           "single_host",
		SelectedHostID: "server-local",
		Runtime: appui.RuntimeSnapshot{
			Turn: appui.RuntimeTurnSnapshot{
				Active:          true,
				Phase:           "executing",
				HostID:          "server-local",
				ClientTurnID:    "client-turn-after-snapshot",
				ClientMessageID: "client-msg-after-snapshot",
			},
			Codex:    map[string]any{"status": "connected"},
			Activity: map[string]any{},
		},
		AgentEventProjection: &proj,
	})

	var sessionSnapshot appui.StateSnapshot
	if err := websocket.JSON.Receive(conn, &sessionSnapshot); err != nil {
		t.Fatalf("receive session snapshot: %v", err)
	}
	if sessionSnapshot.SessionID != session.ID || sessionSnapshot.AgentEventProjection == nil || sessionSnapshot.AgentEventProjection.LastSeq != 1 {
		t.Fatalf("session snapshot = %+v, want session %q projection seq 1", sessionSnapshot, session.ID)
	}

	completedAt := now.Add(time.Second)
	session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleFailed
	session.CurrentTurn.Error = "model provider returned 429"
	session.CurrentTurn.UpdatedAt = completedAt
	session.CurrentTurn.CompletedAt = &completedAt
	sessionMgr.Update(session)

	if _, err := services.AgentEventService().Append(context.Background(), appui.AgentEvent{
		EventID:      "turn-after-snapshot:turn.failed.async",
		SessionID:    session.ID,
		TurnID:       "turn-after-snapshot",
		ClientTurnID: "client-turn-after-snapshot",
		Kind:         appui.AgentEventTurn,
		Phase:        appui.AgentEventPhaseFailed,
		Status:       appui.AgentEventStatusFailed,
		Visibility:   appui.AgentEventVisibilityPrimary,
		Source:       appui.AgentEventSourceSystem,
		CreatedAt:    completedAt.Format(time.RFC3339Nano),
		Payload:      json.RawMessage(`{"summary":"model provider returned 429","error":"model provider returned 429"}`),
	}); err != nil {
		t.Fatalf("append terminal event: %v", err)
	}

	var sawTerminalEvent bool
	for {
		var payload map[string]any
		if err := websocket.JSON.Receive(conn, &payload); err != nil {
			t.Fatalf("receive changed-session terminal payload: %v", err)
		}
		if payload["type"] == "agent_event" {
			eventMap, _ := payload["event"].(map[string]any)
			if eventMap["eventId"] == "turn-after-snapshot:turn.failed.async" {
				sawTerminalEvent = true
			}
			continue
		}
		raw, _ := json.Marshal(payload)
		var snapshot appui.StateSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatalf("decode changed-session snapshot: %v, payload %+v", err, payload)
		}
		if !sawTerminalEvent {
			t.Fatalf("received terminal snapshot before terminal event: %+v", payload)
		}
		if snapshot.SessionID != session.ID || snapshot.Runtime.Turn.Active || snapshot.Runtime.Turn.Phase != "failed" {
			t.Fatalf("terminal snapshot = %+v, want failed snapshot for %q", snapshot, session.ID)
		}
		break
	}
}
