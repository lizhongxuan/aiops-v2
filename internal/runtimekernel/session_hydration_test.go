package runtimekernel

import (
	"context"
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

func TestRunTurnHydratesPersistedSessionFromRepository(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	repo := newHydrationBarrierRepository(&SessionState{
		ID:        "persisted-session",
		Type:      SessionTypeHost,
		Mode:      ModeChat,
		Messages:  []Message{{ID: "prior-message", Role: "user", Content: "prior context"}},
		CreatedAt: createdAt,
	})
	close(repo.release)

	kernel := newLoopKernel(t, &reasoningStreamModel{finalContent: "completed"}, nil, nil, nil)
	kernel.debugConfig = func(context.Context) RuntimeDebugConfig { return RuntimeDebugConfig{} }
	kernel.sessions = NewSessionManager(repo)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "persisted-session",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-hydrated-session",
		Input:       "continue the existing conversation",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("RunTurn() status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("persisted-session")
	if session == nil {
		t.Fatal("RunTurn() did not retain the hydrated session")
	}
	if !session.CreatedAt.Equal(createdAt) {
		t.Fatalf("hydrated session CreatedAt = %s, want %s", session.CreatedAt, createdAt)
	}
	if len(session.Messages) == 0 || session.Messages[0].ID != "prior-message" {
		t.Fatalf("RunTurn() did not preserve persisted conversation history: %#v", session.Messages)
	}
}
