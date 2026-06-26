package appui

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

type defaultSessionService struct {
	sessions SessionSource
	writer   SessionStore
	builder  *SnapshotBuilder
}

func NewSessionService(sessions SessionSource, writer SessionStore, builder *SnapshotBuilder) SessionService {
	return &defaultSessionService{
		sessions: sessions,
		writer:   writer,
		builder:  builder,
	}
}

func (s *defaultSessionService) ListSessions(context.Context) (SessionListResponse, error) {
	if s.sessions == nil {
		return SessionListResponse{Sessions: []SessionSummary{}}, nil
	}
	all := s.builder.SortSessions(userVisibleSessions(s.sessions.List()))
	items := make([]SessionSummary, 0, len(all))
	for _, session := range all {
		items = append(items, s.builder.BuildSessionSummary(session))
	}
	activeSessionID := ""
	if latest := latestUserVisibleSession(s.sessions); latest != nil {
		activeSessionID = latest.ID
	}
	return SessionListResponse{
		ActiveSessionID: activeSessionID,
		Sessions:        items,
	}, nil
}

func latestUserVisibleSession(source SessionSource) *runtimekernel.SessionState {
	if source == nil {
		return nil
	}
	sessions := userVisibleSessions(source.List())
	if len(sessions) == 0 {
		return nil
	}
	return NewSnapshotBuilder().SortSessions(sessions)[0]
}

func userVisibleSessions(sessions []*runtimekernel.SessionState) []*runtimekernel.SessionState {
	out := make([]*runtimekernel.SessionState, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || isInternalHostChildSessionID(session.ID) {
			continue
		}
		out = append(out, session)
	}
	return out
}

func isInternalHostChildSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "host-child:")
}

func (s *defaultSessionService) CreateSession(_ context.Context, kind string, hostID ...string) (SessionMutationResponse, error) {
	if s.writer == nil {
		return SessionMutationResponse{}, fmt.Errorf("session store is not configured")
	}
	sessionType, mode, normalizedKind, err := mapCreateKind(kind)
	if err != nil {
		return SessionMutationResponse{}, err
	}
	session := s.writer.GetOrCreate("", sessionType, mode)
	if normalizedKind == "single_host" {
		session.HostID = resolveCreateSessionHostID(hostID)
	}
	s.writer.Update(session)
	return s.buildMutationResponse(session), nil
}

func (s *defaultSessionService) ActivateSession(_ context.Context, sessionID string) (SessionMutationResponse, error) {
	if s.writer == nil {
		return SessionMutationResponse{}, fmt.Errorf("session store is not configured")
	}
	session := s.writer.Get(sessionID)
	if session == nil {
		return SessionMutationResponse{}, fmt.Errorf("session %q not found", sessionID)
	}
	s.writer.Update(session)
	return s.buildMutationResponse(session), nil
}

func (s *defaultSessionService) buildMutationResponse(active *runtimekernel.SessionState) SessionMutationResponse {
	list, _ := s.ListSessions(context.Background())
	return SessionMutationResponse{
		ActiveSessionID: active.ID,
		Sessions:        list.Sessions,
		Snapshot:        s.builder.BuildStateSnapshot(active),
	}
}

func resolveCreateSessionHostID(requested []string) string {
	for _, candidate := range requested {
		if hostID := strings.TrimSpace(candidate); hostID != "" {
			return hostID
		}
	}
	return ""
}

func mapCreateKind(kind string) (runtimekernel.SessionType, runtimekernel.Mode, string, error) {
	switch kind {
	case "", "single_host":
		return runtimekernel.SessionTypeHost, runtimekernel.ModeExecute, "single_host", nil
	case "workspace":
		return runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute, "workspace", nil
	default:
		return "", "", "", fmt.Errorf("unsupported session kind %q", kind)
	}
}
