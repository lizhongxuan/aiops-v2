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
	if err := assistantTransportRequireSessionSnapshotSource(source); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
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
	session, resolveErr := assistantTransportResolveResumeSession(source, initial)
	if resolveErr != nil {
		current := assistantTransportFailedResumeState(initial, resolveErr)
		_ = encoder.WriteStateOps(assistantTransportFullStateOps(current))
		_ = encoder.WriteError(current.LastError)
		return
	}
	current, err := projectAssistantTransportSessionState(s, assistantTransportCloneState(initial), session, projector, source)
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
	if streamed, err := s.streamAssistantTransportState(r.Context(), encoder, source, projector, s.ui.ChatService(), current); err != nil && !errors.Is(err, context.Canceled) {
		failed := assistantTransportFailedResumeState(streamed, err)
		_ = encoder.WriteStateOps(assistantTransportDiffStateOps(streamed, failed))
		_ = encoder.WriteError(failed.LastError)
		return
	}
}

func assistantTransportResolveResumeSession(source appui.SessionSource, state appui.AiopsTransportState) (*runtimekernel.SessionState, error) {
	if source == nil {
		return nil, errors.New("assistant transport session source is not configured")
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		session, err := assistantTransportGetSessionSnapshot(source, sessionID)
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}
	if threadID := strings.TrimSpace(state.ThreadID); threadID != "" {
		session, err := assistantTransportGetSessionSnapshot(source, threadID)
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		sessions, err := assistantTransportListSessionSnapshots(source)
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			if session == nil || session.CurrentTurn == nil {
				continue
			}
			if strings.TrimSpace(session.CurrentTurn.ID) == turnID {
				return session, nil
			}
		}
	}
	return nil, nil
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
