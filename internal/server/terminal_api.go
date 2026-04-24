package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/appui"
	"golang.org/x/net/websocket"
)

type terminalServiceProvider interface {
	appui.HTTPServices
	TerminalService() appui.TerminalService
}

func terminalServiceFromHTTP(ui appui.HTTPServices) (appui.TerminalService, bool) {
	if provider, ok := ui.(terminalServiceProvider); ok {
		return provider.TerminalService(), true
	}
	return nil, false
}

func (s *HTTPServer) handleTerminalSessions(w http.ResponseWriter, r *http.Request) {
	terminalSvc, _ := terminalServiceFromHTTP(s.ui)
	if terminalSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal service is not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := terminalSvc.ListSessions(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req appui.TerminalCreateSessionCommand
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		meta, err := terminalSvc.CreateSession(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleTerminalWebSocket() websocket.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if s.terminalManager == nil {
			return
		}
		req := conn.Request()
		sessionID := ""
		if req != nil {
			sessionID = strings.TrimSpace(req.URL.Query().Get("sessionId"))
		}
		session, events, release, err := s.terminalManager.Subscribe(sessionID)
		if err != nil {
			_ = websocket.JSON.Send(conn, map[string]any{
				"type":    "error",
				"message": err.Error(),
			})
			return
		}
		defer release()

		writeMu := &sync.Mutex{}
		sendJSON := func(payload any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return websocket.JSON.Send(conn, payload)
		}

		ctx := conn.Request().Context()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				var msg map[string]any
				if err := websocket.JSON.Receive(conn, &msg); err != nil {
					return
				}
				switch strings.TrimSpace(asString(msg["type"])) {
				case "input":
					_ = session.SendInput(asString(msg["data"]))
				case "resize":
					session.Resize(asInt(msg["cols"]), asInt(msg["rows"]))
				case "signal":
					_ = session.Signal(asString(msg["signal"]))
				case "close":
					_ = session.Close()
				case "ping":
					_ = sendJSON(map[string]any{"type": "heartbeat", "time": time.Now().UTC().Format(time.RFC3339Nano)})
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				_ = sendJSON(evt)
			}
		}
	})
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
