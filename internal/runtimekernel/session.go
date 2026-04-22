package runtimekernel

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// SessionManager manages Host and Workspace sessions.
//
// Host Session: single ChatModelAgent
// Workspace Session: PlanExecuteAgent
// ---------------------------------------------------------------------------

// SessionManager manages the lifecycle of Host and Workspace sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
	repo     SessionRepository
}

// SessionRepository is the persistence abstraction used by SessionManager.
// It intentionally matches the store.Store session methods so the store package
// can be used as a backing repository without creating an import cycle.
type SessionRepository interface {
	GetSession(id string) (*SessionState, error)
	SaveSession(session *SessionState) error
	ListSessions() ([]*SessionState, error)
	DeleteSession(id string) error
}

// NewSessionManager creates a new SessionManager.
// The optional repository allows the manager to hydrate and persist sessions.
func NewSessionManager(repo ...SessionRepository) *SessionManager {
	var backing SessionRepository
	if len(repo) > 0 {
		backing = repo[0]
	}
	return &SessionManager{
		sessions: make(map[string]*SessionState),
		repo:     backing,
	}
}

// SetRepository attaches or replaces the persistence backend.
func (sm *SessionManager) SetRepository(repo SessionRepository) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.repo = repo
}

// GetOrCreate retrieves an existing session by ID, or creates a new one
// if the ID is empty or not found.
func (sm *SessionManager) GetOrCreate(sessionID string, sessionType SessionType, mode Mode) *SessionState {
	if sessionID != "" {
		sm.mu.RLock()
		if s, ok := sm.sessions[sessionID]; ok {
			sm.mu.RUnlock()
			return s
		}
		repo := sm.repo
		sm.mu.RUnlock()

		if repo != nil {
			if persisted, err := repo.GetSession(sessionID); err == nil && persisted != nil {
				sm.mu.Lock()
				sm.sessions[persisted.ID] = persisted
				sm.mu.Unlock()
				return persisted
			}
		}
	}

	id := sessionID
	if id == "" {
		id = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	now := time.Now()
	session := &SessionState{
		ID:   id,
		Type: sessionType,
		Mode: mode,
		Context: ContextWindow{
			MaxTokens: DefaultMaxTokens,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	sm.mu.Lock()
	sm.sessions[id] = session
	repo := sm.repo
	sm.mu.Unlock()

	if repo != nil {
		_ = repo.SaveSession(session)
	}
	return session
}

// Get retrieves a session by ID. Returns nil if not found.
func (sm *SessionManager) Get(sessionID string) *SessionState {
	sm.mu.RLock()
	session, ok := sm.sessions[sessionID]
	repo := sm.repo
	sm.mu.RUnlock()
	if ok {
		return session
	}
	if repo != nil && sessionID != "" {
		if persisted, err := repo.GetSession(sessionID); err == nil && persisted != nil {
			sm.mu.Lock()
			sm.sessions[persisted.ID] = persisted
			sm.mu.Unlock()
			return persisted
		}
	}
	return nil
}

// Update persists changes to a session.
func (sm *SessionManager) Update(session *SessionState) {
	if session == nil {
		return
	}
	sm.mu.RLock()
	repo := sm.repo
	sm.mu.RUnlock()
	session.UpdatedAt = time.Now()
	sm.mu.Lock()
	sm.sessions[session.ID] = session
	sm.mu.Unlock()
	if repo != nil {
		_ = repo.SaveSession(session)
	}
}

// Delete removes a session.
func (sm *SessionManager) Delete(sessionID string) {
	sm.mu.RLock()
	repo := sm.repo
	sm.mu.RUnlock()
	sm.mu.Lock()
	delete(sm.sessions, sessionID)
	sm.mu.Unlock()
	if repo != nil {
		_ = repo.DeleteSession(sessionID)
	}
}

// List returns all active sessions.
func (sm *SessionManager) List() []*SessionState {
	sm.mu.RLock()
	repo := sm.repo
	sm.mu.RUnlock()
	if repo != nil {
		if persisted, err := repo.ListSessions(); err == nil {
			sm.mu.Lock()
			for _, sess := range persisted {
				if sess == nil || sess.ID == "" {
					continue
				}
				sm.sessions[sess.ID] = sess
			}
			sm.mu.Unlock()
		}
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*SessionState, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// ListByType returns sessions filtered by session type.
func (sm *SessionManager) ListByType(sessionType SessionType) []*SessionState {
	sessions := sm.List()
	var result []*SessionState
	for _, s := range sessions {
		if s.Type == sessionType {
			result = append(result, s)
		}
	}
	return result
}

// GetLatest returns the most recently updated session across all session types.
func (sm *SessionManager) GetLatest() *SessionState {
	sessions := sm.List()
	if len(sessions) == 0 {
		return nil
	}
	return sessions[0]
}

// GetLatestByType returns the most recently updated session for the given type.
func (sm *SessionManager) GetLatestByType(sessionType SessionType) *SessionState {
	sessions := sm.ListByType(sessionType)
	if len(sessions) == 0 {
		return nil
	}
	return sessions[0]
}
