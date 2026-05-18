package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

func (s *HTTPServer) handleAssistantTransportResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source := s.assistantTransportSessionSource()
	if source == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant transport session source is not configured"})
		return
	}

	req, err := decodeAssistantTransportRequest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	encoder := newAssistantTransportStreamEncoder(w)
	projector := appui.NewTransportProjector()
	initial := assistantTransportInitialState(req)
	session := assistantTransportResolveResumeSession(source, initial)
	current, err := projectAssistantTransportSessionState(s, assistantTransportCloneState(initial), session, projector)
	if err != nil {
		current = assistantTransportFailedResumeState(initial, err)
		_ = encoder.WriteStateOps(assistantTransportFullStateOps(current))
		_ = encoder.WriteError(current.LastError)
		return
	}
	if err := encoder.WriteStateOps(assistantTransportFullStateOps(current)); err != nil {
		return
	}
	if session == nil || assistantTransportSessionTurnIsTerminal(session) || session.CurrentTurn == nil {
		return
	}
	if _, err := s.streamAssistantTransportState(r.Context(), encoder, source, projector, s.ui.ChatService(), current); err != nil && !errors.Is(err, context.Canceled) {
		return
	}
}

func assistantTransportResolveResumeSession(source appui.SessionSource, state appui.AiopsTransportState) *runtimekernel.SessionState {
	if source == nil {
		return nil
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		if session := source.Get(sessionID); session != nil {
			return session
		}
	}
	if threadID := strings.TrimSpace(state.ThreadID); threadID != "" {
		if session := source.Get(threadID); session != nil {
			return session
		}
	}
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		for _, session := range source.List() {
			if session == nil || session.CurrentTurn == nil {
				continue
			}
			if strings.TrimSpace(session.CurrentTurn.ID) == turnID {
				return session
			}
		}
	}
	return nil
}

func assistantTransportFullStateOps(state appui.AiopsTransportState) []assistantTransportStreamStateOp {
	return []assistantTransportStreamStateOp{
		{
			Type:  assistantTransportStreamOpSet,
			Path:  []any{},
			Value: state,
		},
	}
}

func assistantTransportFailedResumeState(state appui.AiopsTransportState, err error) appui.AiopsTransportState {
	next := assistantTransportCloneState(state)
	next.Status = appui.AiopsTransportStatusFailed
	next.LastError = strings.TrimSpace(err.Error())
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return next
}
