package runtimekernel

import (
	"sync"
	"testing"
	"time"
)

type hydrationBarrierRepository struct {
	mu      sync.Mutex
	stored  *SessionState
	calls   int
	saves   int
	ready   chan struct{}
	release chan struct{}
}

func newHydrationBarrierRepository(stored *SessionState) *hydrationBarrierRepository {
	return &hydrationBarrierRepository{
		stored:  stored,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *hydrationBarrierRepository) GetSession(string) (*SessionState, error) {
	r.mu.Lock()
	r.calls++
	if r.calls == 2 {
		close(r.ready)
	}
	r.mu.Unlock()
	<-r.release

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stored == nil {
		return nil, nil
	}
	copy := *r.stored
	return &copy, nil
}

func (r *hydrationBarrierRepository) SaveSession(session *SessionState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves++
	copy := *session
	r.stored = &copy
	return nil
}

func (r *hydrationBarrierRepository) ListSessions() ([]*SessionState, error) { return nil, nil }
func (r *hydrationBarrierRepository) DeleteSession(string) error             { return nil }

func TestSessionManagerConcurrentGetOrCreateSharesHydratedSession(t *testing.T) {
	repo := newHydrationBarrierRepository(&SessionState{
		ID:        "shared-session",
		CreatedAt: time.Now(),
	})
	manager := NewSessionManager(repo)

	first, second := concurrentSessionCalls(t, repo, func() *SessionState {
		return manager.GetOrCreate("shared-session", SessionTypeHost, ModeChat)
	})
	if first != second {
		t.Fatalf("concurrent GetOrCreate returned different session pointers: %p and %p", first, second)
	}
}

func TestSessionManagerConcurrentGetOrCreateSharesNewSession(t *testing.T) {
	repo := newHydrationBarrierRepository(nil)
	manager := NewSessionManager(repo)

	first, second := concurrentSessionCalls(t, repo, func() *SessionState {
		return manager.GetOrCreate("new-session", SessionTypeHost, ModeChat)
	})
	if first != second {
		t.Fatalf("concurrent GetOrCreate returned different session pointers: %p and %p", first, second)
	}
	if repo.saves != 1 {
		t.Fatalf("SaveSession calls = %d, want 1", repo.saves)
	}
}

func TestSessionManagerConcurrentGetSharesHydratedSession(t *testing.T) {
	repo := newHydrationBarrierRepository(&SessionState{
		ID:        "shared-session",
		CreatedAt: time.Now(),
	})
	manager := NewSessionManager(repo)

	first, second := concurrentSessionCalls(t, repo, func() *SessionState {
		return manager.Get("shared-session")
	})
	if first != second {
		t.Fatalf("concurrent Get returned different session pointers: %p and %p", first, second)
	}
}

func concurrentSessionCalls(t *testing.T, repo *hydrationBarrierRepository, call func() *SessionState) (*SessionState, *SessionState) {
	t.Helper()
	results := make(chan *SessionState, 2)
	for range 2 {
		go func() { results <- call() }()
	}
	<-repo.ready
	close(repo.release)
	return <-results, <-results
}
