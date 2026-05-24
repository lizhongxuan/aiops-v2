package opsmanual

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionOpsContextStore interface {
	UpsertFact(ctx context.Context, sessionID string, fact SessionOpsFact) error
	ListFacts(ctx context.Context, sessionID string, filter SessionOpsFactFilter) ([]SessionOpsFact, error)
	PruneExpired(ctx context.Context, now time.Time) error
	ClearSession(ctx context.Context, sessionID string) error
}

type MemorySessionOpsContextStore struct {
	mu       sync.RWMutex
	contexts map[string]SessionOpsContext
}

var _ SessionOpsContextStore = (*MemorySessionOpsContextStore)(nil)

func NewMemorySessionOpsContextStore() *MemorySessionOpsContextStore {
	return &MemorySessionOpsContextStore{contexts: map[string]SessionOpsContext{}}
}

func (s *MemorySessionOpsContextStore) UpsertFact(_ context.Context, sessionID string, fact SessionOpsFact) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	fact.Key = strings.TrimSpace(fact.Key)
	if fact.Key == "" {
		return fmt.Errorf("session ops fact key is required")
	}
	now := time.Now().UTC()
	if fact.CreatedAt.IsZero() {
		fact.CreatedAt = now
	}
	if fact.UpdatedAt.IsZero() {
		fact.UpdatedAt = now
	}
	fact = sanitizeSessionOpsFact(fact)
	identity := sessionFactIdentity(fact)

	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := s.contexts[sessionID]
	ctx.SessionID = sessionID
	replaced := false
	for i, existing := range ctx.Facts {
		if sessionFactIdentity(existing) != identity {
			continue
		}
		fact.CreatedAt = existing.CreatedAt
		ctx.Facts[i] = fact
		replaced = true
		break
	}
	if !replaced {
		ctx.Facts = append(ctx.Facts, fact)
	}
	ctx.UpdatedAt = fact.UpdatedAt
	s.contexts[sessionID] = ctx
	return nil
}

func (s *MemorySessionOpsContextStore) ListFacts(_ context.Context, sessionID string, filter SessionOpsFactFilter) ([]SessionOpsFact, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	s.mu.RLock()
	ctx := s.contexts[sessionID]
	s.mu.RUnlock()
	out := make([]SessionOpsFact, 0, len(ctx.Facts))
	for _, fact := range ctx.Facts {
		if !factMatchesFilter(fact, filter) {
			continue
		}
		out = append(out, sanitizeSessionOpsFact(fact))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ConfirmedByUser != out[j].ConfirmedByUser {
			return out[i].ConfirmedByUser
		}
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *MemorySessionOpsContextStore) PruneExpired(_ context.Context, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sessionID, ctx := range s.contexts {
		kept := make([]SessionOpsFact, 0, len(ctx.Facts))
		for _, fact := range ctx.Facts {
			if !factExpired(fact, now) {
				kept = append(kept, fact)
			}
		}
		ctx.Facts = kept
		ctx.UpdatedAt = now
		if len(ctx.Facts) == 0 {
			delete(s.contexts, sessionID)
			continue
		}
		s.contexts[sessionID] = ctx
	}
	return nil
}

func (s *MemorySessionOpsContextStore) ClearSession(_ context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contexts, sessionID)
	return nil
}
