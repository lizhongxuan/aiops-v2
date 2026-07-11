package runtimekernel

import (
	"encoding/json"
	"fmt"
	"sort"
)

// GetSnapshot returns the latest immutable session snapshot published by
// GetOrCreate, repository hydration, or Update. Callers must treat the returned
// value as read-only; later publications replace it instead of mutating it.
func (sm *SessionManager) GetSnapshot(sessionID string) (*SessionState, error) {
	if sm == nil || sessionID == "" {
		return nil, nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.snapshots[sessionID], sm.snapshotErrors[sessionID]
}

// ListSnapshots returns the immutable published sessions ordered like List.
func (sm *SessionManager) ListSnapshots() ([]*SessionState, error) {
	if sm == nil {
		return nil, nil
	}
	sm.mu.RLock()
	if len(sm.snapshotErrors) > 0 {
		ids := make([]string, 0, len(sm.snapshotErrors))
		for id := range sm.snapshotErrors {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		err := sm.snapshotErrors[ids[0]]
		sm.mu.RUnlock()
		return nil, err
	}
	result := make([]*SessionState, 0, len(sm.snapshots))
	for _, snapshot := range sm.snapshots {
		result = append(result, snapshot)
	}
	sm.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (sm *SessionManager) publishSnapshotLocked(sessionID string, snapshot *SessionState, err error) {
	if err != nil {
		delete(sm.snapshots, sessionID)
		sm.snapshotErrors[sessionID] = fmt.Errorf("publish session snapshot %q: %w", sessionID, err)
		return
	}
	sm.snapshots[sessionID] = snapshot
	delete(sm.snapshotErrors, sessionID)
}

func cloneSessionSnapshot(src *SessionState) (*SessionState, error) {
	if src == nil {
		return nil, nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst SessionState
	if err := json.Unmarshal(raw, &dst); err != nil {
		return nil, err
	}
	return &dst, nil
}
