package runbooks

import (
	"sort"
	"strings"
	"sync"
)

type InstanceStore interface {
	Put(instance RunbookInstance)
	Get(id string) (RunbookInstance, bool)
	List(status string) []RunbookInstance
}

type InMemoryInstanceStore struct {
	mu        sync.RWMutex
	instances map[string]RunbookInstance
}

func NewInMemoryInstanceStore() *InMemoryInstanceStore {
	return &InMemoryInstanceStore{instances: map[string]RunbookInstance{}}
}

func (s *InMemoryInstanceStore) Put(instance RunbookInstance) {
	if s == nil || strings.TrimSpace(instance.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[instance.ID] = cloneInstance(instance)
}

func (s *InMemoryInstanceStore) Get(id string) (RunbookInstance, bool) {
	if s == nil {
		return RunbookInstance{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	instance, ok := s.instances[strings.TrimSpace(id)]
	if !ok {
		return RunbookInstance{}, false
	}
	return cloneInstance(instance), true
}

func (s *InMemoryInstanceStore) List(status string) []RunbookInstance {
	if s == nil {
		return nil
	}
	status = strings.TrimSpace(status)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RunbookInstance, 0, len(s.instances))
	for _, instance := range s.instances {
		if status != "" && instance.Status != status {
			continue
		}
		out = append(out, cloneInstance(instance))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAtUnix == out[j].UpdatedAtUnix {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAtUnix > out[j].UpdatedAtUnix
	})
	return out
}
