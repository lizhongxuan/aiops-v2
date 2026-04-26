package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"aiops-v2/internal/appui"
	"golang.org/x/net/websocket"
)

func (s *HTTPServer) appWebSocketHandler() websocket.Handler {
	heartbeatEvery := s.appWSHeartbeatTick
	if heartbeatEvery <= 0 {
		heartbeatEvery = 15 * time.Second
	}
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()

		ctx := context.Background()
		if req := conn.Request(); req != nil {
			ctx = req.Context()
		}

		writeMu := &sync.Mutex{}
		sendJSON := func(payload any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return websocket.JSON.Send(conn, payload)
		}

		var updates <-chan appui.StateSnapshot
		unsubscribe := func() {}
		var agentEvents <-chan appui.AgentEvent
		unsubscribeAgentEvents := func() {}
		if s.appSnapshots != nil {
			updates, unsubscribe = s.appSnapshots.Subscribe()
			defer unsubscribe()
		}

		snapshot, err := s.ui.StateService().GetState(ctx)
		if err != nil {
			_ = sendJSON(map[string]any{"type": "heartbeat"})
			return
		}
		snapshot = s.withAgentEventProjection(ctx, snapshot)
		if s.agentEvents != nil {
			var afterSeq int64
			if snapshot.AgentEventProjection != nil {
				afterSeq = snapshot.AgentEventProjection.LastSeq
			}
			agentEvents, unsubscribeAgentEvents = s.agentEvents.Subscribe(ctx, snapshot.SessionID, afterSeq)
			defer unsubscribeAgentEvents()
		}
		if err := sendJSON(snapshot); err != nil {
			return
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				var message map[string]any
				if err := websocket.JSON.Receive(conn, &message); err != nil {
					return
				}
				if message["type"] == "ping" {
					_ = sendJSON(map[string]any{
						"type": "heartbeat",
						"time": time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			}
		}()

		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case snapshot, ok := <-updates:
				if !ok {
					updates = nil
					continue
				}
				if err := sendJSON(snapshot); err != nil {
					return
				}
			case event, ok := <-agentEvents:
				if !ok {
					agentEvents = nil
					continue
				}
				if err := sendJSON(map[string]any{
					"type":  "agent_event",
					"event": event,
				}); err != nil {
					return
				}
			case <-ticker.C:
				if err := sendJSON(map[string]any{
					"type": "heartbeat",
					"time": time.Now().UTC().Format(time.RFC3339Nano),
				}); err != nil {
					return
				}
			}
		}
	})
}

func encodeWebSocketMessage(payload any) json.RawMessage {
	data, _ := json.Marshal(payload)
	return data
}
