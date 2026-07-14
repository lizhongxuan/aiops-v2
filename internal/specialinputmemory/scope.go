package specialinputmemory

import "strings"

type ScopeKey struct {
	SessionID      string `json:"sessionId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	Scope          string `json:"scope,omitempty"`
	EnvironmentKey string `json:"environmentKey,omitempty"`
	ClusterKey     string `json:"clusterKey,omitempty"`
}

func NewScopeKey(sessionID, taskID, scope, environmentKey, clusterKey string) ScopeKey {
	return ScopeKey{
		SessionID:      strings.TrimSpace(sessionID),
		TaskID:         strings.TrimSpace(taskID),
		Scope:          normalizedScope(scope),
		EnvironmentKey: compactToken(environmentKey),
		ClusterKey:     compactToken(clusterKey),
	}
}

func (s ScopeKey) Matches(other ScopeKey) bool {
	left := NewScopeKey(s.SessionID, s.TaskID, s.Scope, s.EnvironmentKey, s.ClusterKey)
	right := NewScopeKey(other.SessionID, other.TaskID, other.Scope, other.EnvironmentKey, other.ClusterKey)
	if left.Scope != right.Scope {
		return false
	}
	if left.Scope == ScopeCurrentTask && left.TaskID != "" && right.TaskID != "" && left.TaskID != right.TaskID {
		return false
	}
	if left.EnvironmentKey != "" && right.EnvironmentKey != "" && left.EnvironmentKey != right.EnvironmentKey {
		return false
	}
	if left.ClusterKey != "" && right.ClusterKey != "" && left.ClusterKey != right.ClusterKey {
		return false
	}
	return true
}
