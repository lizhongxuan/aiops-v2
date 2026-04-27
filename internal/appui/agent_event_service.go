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
	loadedBySession     map[string]bool
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
		loadedBySession:     map[string]bool{},
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
	if err := s.ensureSessionLoaded(ctx, event.SessionID); err != nil {
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
	event.Seq = nextAgentEventSeq(events)
	proj := s.projectionBySession[event.SessionID]
	if proj.SessionID == "" {
		proj = AgentEventProjection{SessionID: event.SessionID, Status: "idle"}
	}
	nextProjection, err := s.projector.Apply(proj, event)
	if err != nil {
		s.mu.Unlock()
		return AgentEvent{}, err
	}
	repo := s.repo
	if repo != nil {
		if err := repo.AppendAgentEvent(event.SessionID, event); err != nil {
			s.mu.Unlock()
			return AgentEvent{}, fmt.Errorf("append agent event repository: %w", err)
		}
		if err := repo.SaveAgentEventProjection(event.SessionID, nextProjection); err != nil {
			s.mu.Unlock()
			return AgentEvent{}, fmt.Errorf("save agent event projection: %w", err)
		}
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
	s.mu.Unlock()

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
	if err := s.ensureSessionLoaded(ctx, sessionID); err != nil {
		close(ch)
		return ch, func() {}
	}

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
	if err := s.ensureSessionLoaded(ctx, sessionID); err != nil {
		return AgentEventProjection{}, err
	}
	s.mu.RLock()
	proj, ok := s.projectionBySession[sessionID]
	s.mu.RUnlock()
	if ok {
		return ensureAgentEventProjection(proj), nil
	}
	return ensureAgentEventProjection(AgentEventProjection{SessionID: sessionID, Status: "idle"}), nil
}

func (s *agentEventService) Replay(ctx context.Context, sessionID string, afterSeq int64) ([]AgentEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ensureSessionLoaded(ctx, sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	events := append([]AgentEvent(nil), s.eventsBySession[sessionID]...)
	s.mu.RUnlock()
	out := make([]AgentEvent, 0, len(events))
	for _, event := range events {
		if event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s *agentEventService) ensureSessionLoaded(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.repo == nil || sessionID == "" {
		return nil
	}
	s.mu.RLock()
	loaded := s.loadedBySession[sessionID]
	s.mu.RUnlock()
	if loaded {
		return nil
	}

	events, err := s.repo.ListAgentEvents(sessionID, 0)
	if err != nil {
		return fmt.Errorf("list agent events repository: %w", err)
	}
	proj := AgentEventProjection{SessionID: sessionID, Status: "idle"}
	if len(events) > 0 {
		proj, err = s.projector.Replay(sessionID, events)
		if err != nil {
			return fmt.Errorf("replay agent event repository: %w", err)
		}
	} else if loadedProjection, found, err := s.repo.LoadAgentEventProjection(sessionID); err != nil {
		return fmt.Errorf("load agent event projection repository: %w", err)
	} else if found {
		proj = loadedProjection
	}

	seen := map[string]AgentEvent{}
	for _, event := range events {
		seen[event.EventID] = event
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadedBySession[sessionID] {
		return nil
	}
	s.eventsBySession[sessionID] = append([]AgentEvent(nil), events...)
	s.seenBySession[sessionID] = seen
	s.projectionBySession[sessionID] = ensureAgentEventProjection(proj)
	s.loadedBySession[sessionID] = true
	return nil
}

func nextAgentEventSeq(events []AgentEvent) int64 {
	var maxSeq int64
	for _, event := range events {
		if event.Seq > maxSeq {
			maxSeq = event.Seq
		}
	}
	return maxSeq + 1
}
