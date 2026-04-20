package runtimekernel

import (
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// SessionManager manages Host and Workspace sessions.
//
// Host Session: single ChatModelAgent, visible tool/skill/mcp_tool/ui_surface
// Workspace Session: PlanExecuteAgent, additionally visible workspace:*
// ---------------------------------------------------------------------------

// SessionManager manages the lifecycle of Host and Workspace sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*SessionState),
	}
}

// GetOrCreate retrieves an existing session by ID, or creates a new one
// if the ID is empty or not found.
func (sm *SessionManager) GetOrCreate(sessionID string, sessionType SessionType, mode Mode) *SessionState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sessionID != "" {
		if s, ok := sm.sessions[sessionID]; ok {
			return s
		}
	}

	// Create new session
	id := sessionID
	if id == "" {
		id = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	session := &SessionState{
		ID:   id,
		Type: sessionType,
		Mode: mode,
		Context: ContextWindow{
			MaxTokens: DefaultMaxTokens,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.sessions[id] = session
	return session
}

// Get retrieves a session by ID. Returns nil if not found.
func (sm *SessionManager) Get(sessionID string) *SessionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionID]
}

// Update persists changes to a session.
func (sm *SessionManager) Update(session *SessionState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	session.UpdatedAt = time.Now()
	sm.sessions[session.ID] = session
}

// Delete removes a session.
func (sm *SessionManager) Delete(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// List returns all active sessions.
func (sm *SessionManager) List() []*SessionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*SessionState, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		result = append(result, s)
	}
	return result
}

// ListByType returns sessions filtered by session type.
func (sm *SessionManager) ListByType(sessionType SessionType) []*SessionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var result []*SessionState
	for _, s := range sm.sessions {
		if s.Type == sessionType {
			result = append(result, s)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Session visibility helpers
// ---------------------------------------------------------------------------

// HostSessionVisibleKinds returns the capability kinds visible in a Host session.
// Host Session: tool, skill, mcp_tool, ui_surface
func HostSessionVisibleKinds() []string {
	return []string{"tool", "skill", "mcp_tool", "ui_surface"}
}

// WorkspaceSessionVisibleKinds returns the capability kinds visible in a Workspace session.
// Workspace Session: tool, skill, mcp_tool, ui_surface, workspace
func WorkspaceSessionVisibleKinds() []string {
	return []string{"tool", "skill", "mcp_tool", "ui_surface", "workspace"}
}

// IsKindVisibleInSession checks if a capability kind is visible in the given session type.
func IsKindVisibleInSession(kind string, sessionType SessionType) bool {
	switch sessionType {
	case SessionTypeHost:
		for _, k := range HostSessionVisibleKinds() {
			if k == kind {
				return true
			}
		}
		return false
	case SessionTypeWorkspace:
		for _, k := range WorkspaceSessionVisibleKinds() {
			if k == kind {
				return true
			}
		}
		return false
	default:
		return false
	}
}
