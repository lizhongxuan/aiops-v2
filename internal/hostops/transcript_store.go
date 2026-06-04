package hostops

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

var ErrTranscriptNotFound = errors.New("host child transcript not found")

type TranscriptStore interface {
	Append(ctx context.Context, childAgentID string, item TranscriptItem) error
	List(ctx context.Context, childAgentID string) ([]TranscriptItem, error)
}

type InMemoryTranscriptStore struct {
	mu          sync.RWMutex
	transcript  map[string][]TranscriptItem
	nextCounter int64
}

func NewInMemoryTranscriptStore() *InMemoryTranscriptStore {
	return &InMemoryTranscriptStore{
		transcript: map[string][]TranscriptItem{},
	}
}

func (s *InMemoryTranscriptStore) Append(_ context.Context, childAgentID string, item TranscriptItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" {
		s.nextCounter++
		item.ID = "transcript-item-" + strconv.FormatInt(s.nextCounter, 10)
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	s.transcript[childAgentID] = append(s.transcript[childAgentID], cloneTranscriptItem(item))
	return nil
}

func (s *InMemoryTranscriptStore) List(_ context.Context, childAgentID string) ([]TranscriptItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, ok := s.transcript[childAgentID]
	if !ok {
		return []TranscriptItem{}, nil
	}
	return cloneTranscriptItems(items), nil
}

func cloneTranscriptItems(items []TranscriptItem) []TranscriptItem {
	result := make([]TranscriptItem, len(items))
	for i, item := range items {
		result[i] = cloneTranscriptItem(item)
	}
	return result
}

func cloneTranscriptItem(item TranscriptItem) TranscriptItem {
	if item.Payload != nil {
		item.Payload = cloneAnyMap(item.Payload)
	}
	return item
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
