package evidence

import (
	"context"
	"sync"
)

// Store persists evidence records and incident links.
type Store interface {
	Put(context.Context, Record) error
	Get(context.Context, string) (Record, bool, error)
	LinkIncident(context.Context, IncidentLink) error
	ListIncident(context.Context, string) ([]Record, error)
}

// InMemoryStore is a process-local evidence store for tests and MVP runtime.
type InMemoryStore struct {
	mu      sync.RWMutex
	records map[string]Record
	links   map[string][]IncidentLink
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		records: make(map[string]Record),
		links:   make(map[string][]IncidentLink),
	}
}

func (s *InMemoryStore) Put(_ context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.Ref] = cloneRecord(rec)
	return nil
}

func (s *InMemoryStore) Get(_ context.Context, ref string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[ref]
	if !ok {
		return Record{}, false, nil
	}
	return cloneRecord(rec), true, nil
}

func (s *InMemoryStore) LinkIncident(_ context.Context, link IncidentLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.links[link.IncidentID] {
		if existing.Ref == link.Ref && existing.Relation == link.Relation {
			return nil
		}
	}
	s.links[link.IncidentID] = append(s.links[link.IncidentID], link)
	return nil
}

func (s *InMemoryStore) ListIncident(_ context.Context, incidentID string) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	links := append([]IncidentLink(nil), s.links[incidentID]...)
	out := make([]Record, 0, len(links))
	for _, link := range links {
		if rec, ok := s.records[link.Ref]; ok {
			out = append(out, cloneRecord(rec))
		}
	}
	return out, nil
}

func cloneRecord(rec Record) Record {
	if rec.Data != nil {
		rec.Data = cloneMap(rec.Data)
	}
	return rec
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
