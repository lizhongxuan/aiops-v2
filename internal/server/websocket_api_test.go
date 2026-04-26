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
