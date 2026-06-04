package workflowgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("workflow generation session not found")

type SessionStore interface {
	Create(ctx context.Context, session WorkflowGenerationSession) (*WorkflowGenerationSession, error)
	Get(ctx context.Context, id string) (*WorkflowGenerationSession, error)
	Update(ctx context.Context, id string, update func(*WorkflowGenerationSession) error) (*WorkflowGenerationSession, error)
	AppendEvent(ctx context.Context, id string, event WorkflowGenerationEvent) (*WorkflowGenerationEvent, error)
	ListEvents(ctx context.Context, id string) ([]WorkflowGenerationEvent, error)
}

type MemorySessionStore struct {
	mu       sync.Mutex
	now      func() time.Time
	nextID   int64
	nextSeq  map[string]int64
	sessions map[string]WorkflowGenerationSession
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		now:      func() time.Time { return time.Now().UTC() },
		nextSeq:  map[string]int64{},
		sessions: map[string]WorkflowGenerationSession{},
	}
}

func (s *MemorySessionStore) Create(_ context.Context, session WorkflowGenerationSession) (*WorkflowGenerationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.nextID++
	if session.ID == "" {
		session.ID = fmt.Sprintf("wfgen-%d", s.nextID)
	}
	if session.Status == "" {
		session.Status = SessionStatusPlanStarted
	}
	if session.Plan != nil {
		session.PlanVersion = session.Plan.Version
		session.Slots = append([]RequiredSlot(nil), session.Plan.RequiredSlots...)
	}
	if session.PlanVersion == 0 {
		session.PlanVersion = 1
	}
	if session.ValidationProvider == "" {
		session.ValidationProvider = ValidationProviderNone
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now
	session.Events = cloneEvents(session.Events)
	s.sessions[session.ID] = cloneSession(session)
	return cloneSessionPtr(session), nil
}

func (s *MemorySessionStore) Get(_ context.Context, id string) (*WorkflowGenerationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneSessionPtr(session), nil
}

func (s *MemorySessionStore) Update(_ context.Context, id string, update func(*WorkflowGenerationSession) error) (*WorkflowGenerationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if update != nil {
		if err := update(&session); err != nil {
			return nil, err
		}
	}
	if session.Plan != nil {
		session.PlanVersion = session.Plan.Version
		session.Slots = append([]RequiredSlot(nil), session.Plan.RequiredSlots...)
	}
	session.UpdatedAt = s.now()
	s.sessions[id] = cloneSession(session)
	return cloneSessionPtr(session), nil
}

func (s *MemorySessionStore) AppendEvent(_ context.Context, id string, event WorkflowGenerationEvent) (*WorkflowGenerationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	now := s.now()
	s.nextSeq[id]++
	event.Sequence = s.nextSeq[id]
	event.SessionID = id
	if event.ID == "" {
		event.ID = fmt.Sprintf("%s-ev-%06d", id, event.Sequence)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	session.Events = append(session.Events, event)
	session.UpdatedAt = now
	s.sessions[id] = cloneSession(session)
	cloned := cloneEvent(event)
	return &cloned, nil
}

func (s *MemorySessionStore) ListEvents(_ context.Context, id string) ([]WorkflowGenerationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneEvents(session.Events), nil
}

type FileSessionStore struct {
	*MemorySessionStore
	dir string
}

func NewFileSessionStore(dir string) (*FileSessionStore, error) {
	if dir == "" {
		dir = filepath.Join("data", "workflow-generation", "sessions")
	}
	store := &FileSessionStore{
		MemorySessionStore: NewMemorySessionStore(),
		dir:                dir,
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileSessionStore) Create(ctx context.Context, session WorkflowGenerationSession) (*WorkflowGenerationSession, error) {
	created, err := s.MemorySessionStore.Create(ctx, session)
	if err != nil {
		return nil, err
	}
	return created, s.persist(created.ID)
}

func (s *FileSessionStore) Update(ctx context.Context, id string, update func(*WorkflowGenerationSession) error) (*WorkflowGenerationSession, error) {
	updated, err := s.MemorySessionStore.Update(ctx, id, update)
	if err != nil {
		return nil, err
	}
	return updated, s.persist(id)
}

func (s *FileSessionStore) AppendEvent(ctx context.Context, id string, event WorkflowGenerationEvent) (*WorkflowGenerationEvent, error) {
	appended, err := s.MemorySessionStore.AppendEvent(ctx, id, event)
	if err != nil {
		return nil, err
	}
	return appended, s.persist(id)
}

func (s *FileSessionStore) persist(id string) error {
	session, err := s.MemorySessionStore.Get(context.Background(), id)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, id+".tmp")
	dst := filepath.Join(s.dir, id+".json")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func (s *FileSessionStore) load() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return err
		}
		var session WorkflowGenerationSession
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}
		s.MemorySessionStore.sessions[session.ID] = cloneSession(session)
		for _, event := range session.Events {
			if event.Sequence > s.MemorySessionStore.nextSeq[session.ID] {
				s.MemorySessionStore.nextSeq[session.ID] = event.Sequence
			}
		}
	}
	return nil
}

func cloneSessionPtr(input WorkflowGenerationSession) *WorkflowGenerationSession {
	cloned := cloneSession(input)
	return &cloned
}

func cloneSession(input WorkflowGenerationSession) WorkflowGenerationSession {
	data, err := json.Marshal(input)
	if err != nil {
		return input
	}
	var output WorkflowGenerationSession
	if err := json.Unmarshal(data, &output); err != nil {
		return input
	}
	return output
}

func cloneEvents(input []WorkflowGenerationEvent) []WorkflowGenerationEvent {
	if len(input) == 0 {
		return nil
	}
	output := make([]WorkflowGenerationEvent, len(input))
	for i, event := range input {
		output[i] = cloneEvent(event)
	}
	return output
}

func cloneEvent(input WorkflowGenerationEvent) WorkflowGenerationEvent {
	data, err := json.Marshal(input)
	if err != nil {
		return input
	}
	var output WorkflowGenerationEvent
	if err := json.Unmarshal(data, &output); err != nil {
		return input
	}
	return output
}
