package appui

import (
	"context"
	"fmt"
	"sync"
)

type AgentEventRepository interface {
	AppendAgentEvent(sessionID string, event AgentEvent) error
	ListAgentEvents(sessionID string, afterSeq int64) ([]AgentEvent, error)
	SaveAgentEventProjection(sessionID string, projection AgentEventProjection) error
	LoadAgentEventProjection(sessionID string) (AgentEventProjection, bool, error)
}

type agentEventSubscriber struct {
	sessionID string
	ch        chan AgentEvent
}

type agentEventService struct {
	mu                  sync.RWMutex
	repo                AgentEventRepository
	projector           *AgentEventProjector
	eventsBySession     map[string][]AgentEvent
	seenBySession       map[string]map[string]AgentEvent
	projectionBySession map[string]AgentEventProjection
	subscribers         map[int]agentEventSubscriber
	nextSubscriberID    int
}

func NewAgentEventService(repo AgentEventRepository) AgentEventService {
	return &agentEventService{
		repo:                repo,
		projector:           NewAgentEventProjector(),
		eventsBySession:     map[string][]AgentEvent{},
		seenBySession:       map[string]map[string]AgentEvent{},
		projectionBySession: map[string]AgentEventProjection{},
		subscribers:         map[int]agentEventSubscriber{},
	}
}

func (s *agentEventService) Append(ctx context.Context, event AgentEvent) (AgentEvent, error) {
	if err := ctx.Err(); err != nil {
		return AgentEvent{}, err
	}
	if err := event.Validate(); err != nil {
		return AgentEvent{}, err
	}

	s.mu.Lock()
	if s.seenBySession[event.SessionID] == nil {
		s.seenBySession[event.SessionID] = map[string]AgentEvent{}
	}
	if existing, ok := s.seenBySession[event.SessionID][event.EventID]; ok {
		s.mu.Unlock()
		return existing, nil
	}

	events := s.eventsBySession[event.SessionID]
	event.Seq = int64(len(events) + 1)
	proj := s.projectionBySession[event.SessionID]
	if proj.SessionID == "" {
		proj = AgentEventProjection{SessionID: event.SessionID, Status: "idle"}
	}
	nextProjection, err := s.projector.Apply(proj, event)
	if err != nil {
		s.mu.Unlock()
		return AgentEvent{}, err
	}
	s.eventsBySession[event.SessionID] = append(events, event)
	s.seenBySession[event.SessionID][event.EventID] = event
	s.projectionBySession[event.SessionID] = nextProjection

	subscribers := make([]chan AgentEvent, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		if sub.sessionID == event.SessionID {
			subscribers = append(subscribers, sub.ch)
		}
	}
	repo := s.repo
	s.mu.Unlock()

	if repo != nil {
		if err := repo.AppendAgentEvent(event.SessionID, event); err != nil {
			return AgentEvent{}, fmt.Errorf("append agent event repository: %w", err)
		}
		if err := repo.SaveAgentEventProjection(event.SessionID, nextProjection); err != nil {
			return AgentEvent{}, fmt.Errorf("save agent event projection: %w", err)
		}
	}

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
	return event, nil
}

func (s *agentEventService) Subscribe(ctx context.Context, sessionID string, afterSeq int64) (<-chan AgentEvent, func()) {
	ch := make(chan AgentEvent, 32)

	s.mu.Lock()
	id := s.nextSubscriberID
	s.nextSubscriberID++
	s.subscribers[id] = agentEventSubscriber{sessionID: sessionID, ch: ch}
	var backlog []AgentEvent
	for _, event := range s.eventsBySession[sessionID] {
		if event.Seq > afterSeq {
			backlog = append(backlog, event)
		}
	}
	s.mu.Unlock()

	for _, event := range backlog {
		select {
		case ch <- event:
		default:
			break
		}
	}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			s.mu.Lock()
			if sub, ok := s.subscribers[id]; ok {
				delete(s.subscribers, id)
				close(sub.ch)
			}
			s.mu.Unlock()
		})
	}
	go func() {
		<-ctx.Done()
		unsubscribe()
	}()
	return ch, unsubscribe
}

func (s *agentEventService) Projection(ctx context.Context, sessionID string) (AgentEventProjection, error) {
	if err := ctx.Err(); err != nil {
		return AgentEventProjection{}, err
	}
	s.mu.RLock()
	proj, ok := s.projectionBySession[sessionID]
	s.mu.RUnlock()
	if ok {
		return proj, nil
	}
	if s.repo != nil {
		loaded, found, err := s.repo.LoadAgentEventProjection(sessionID)
		if err != nil {
			return AgentEventProjection{}, err
		}
		if found {
			return loaded, nil
		}
	}
	return ensureAgentEventProjection(AgentEventProjection{SessionID: sessionID, Status: "idle"}), nil
}

func (s *agentEventService) Replay(ctx context.Context, sessionID string, afterSeq int64) ([]AgentEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	events := append([]AgentEvent(nil), s.eventsBySession[sessionID]...)
	s.mu.RUnlock()
	if len(events) == 0 && s.repo != nil {
		return s.repo.ListAgentEvents(sessionID, afterSeq)
	}
	out := make([]AgentEvent, 0, len(events))
	for _, event := range events {
		if event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	return out, nil
}
