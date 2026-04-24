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
		if payload["type"] == "turn_event" {
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

func TestAppWebSocket_PushesTurnEventEnvelopeWhenToolProjectionArrives(t *testing.T) {
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
		if payload["type"] != "turn_event" {
			continue
		}
		eventMap, ok := payload["event"].(map[string]any)
		if !ok {
			t.Fatalf("turn_event payload = %+v, want event object", payload)
		}
		if eventMap["type"] != string(appui.TurnEventToolCallStart) {
			t.Fatalf("event.type = %v, want %s", eventMap["type"], appui.TurnEventToolCallStart)
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
