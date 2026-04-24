package events

import (
	"sync"
)

type Hub struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[string]map[uint64]chan Event
}

func NewHub() *Hub {
	return &Hub{
		subscribers: map[string]map[uint64]chan Event{},
	}
}

func (h *Hub) Publish(runID string, evt Event) {
	h.mu.RLock()
	targets := h.subscribers[runID]
	if len(targets) == 0 {
		h.mu.RUnlock()
		return
	}
	copied := make([]chan Event, 0, len(targets))
	for _, ch := range targets {
		copied = append(copied, ch)
	}
	h.mu.RUnlock()

	for _, ch := range copied {
		select {
		case ch <- evt:
		default:
			// Drop when the subscriber is too slow.
		}
	}
}

func (h *Hub) Subscribe(runID string) (Subscriber, bool) {
	if h == nil || runID == "" {
		return Subscriber{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan Event, 128)
	group := h.subscribers[runID]
	if group == nil {
		group = map[uint64]chan Event{}
		h.subscribers[runID] = group
	}
	group[id] = ch
	return Subscriber{
		ID:    id,
		RunID: runID,
		C:     ch,
	}, true
}

func (h *Hub) Unsubscribe(sub Subscriber) {
	if h == nil || sub.ID == 0 || sub.RunID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	group := h.subscribers[sub.RunID]
	if group == nil {
		return
	}
	ch := group[sub.ID]
	if ch != nil {
		close(ch)
	}
	delete(group, sub.ID)
	if len(group) == 0 {
		delete(h.subscribers, sub.RunID)
	}
}
