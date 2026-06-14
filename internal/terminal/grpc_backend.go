package terminal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type RemoteTerminalBackend interface {
	OpenTerminal(context.Context, RemoteTerminalOpenRequest) (RemoteTerminalHandle, error)
}

type RemoteTerminalOpenRequest struct {
	SessionID string
	HostID    string
	Cwd       string
	Shell     string
	Cols      int
	Rows      int
	StartedAt time.Time
	Emit      func(Event)
}

type RemoteTerminalHandle interface {
	SendInput(data string) error
	Resize(cols, rows int) error
	Signal(name string) error
	Close() error
}

type remoteSession struct {
	mu          sync.RWMutex
	meta        SessionMetadata
	handle      RemoteTerminalHandle
	subscribers map[int]chan Event
	nextSubID   int
	history     []Event
	closed      bool
	closeOnce   sync.Once
}

func newRemoteSession(meta SessionMetadata) *remoteSession {
	return &remoteSession{
		meta:        meta,
		subscribers: map[int]chan Event{},
	}
}

func (s *remoteSession) setHandle(handle RemoteTerminalHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handle = handle
}

func (s *remoteSession) Metadata() SessionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

func (s *remoteSession) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = ch
	ready := Event{
		Type:      EventTypeReady,
		SessionID: s.meta.SessionID,
		HostID:    s.meta.HostID,
		Cwd:       s.meta.Cwd,
		Shell:     s.meta.Shell,
		Cols:      s.meta.Cols,
		Rows:      s.meta.Rows,
		Status:    s.meta.Status,
		StartedAt: s.meta.StartedAt,
		UpdatedAt: s.meta.UpdatedAt,
	}
	history := append([]Event(nil), s.history...)
	s.mu.Unlock()

	ch <- ready
	for _, event := range history {
		ch <- event
	}

	release := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if existing, ok := s.subscribers[id]; ok {
			delete(s.subscribers, id)
			close(existing)
		}
	}
	return ch, release
}

func (s *remoteSession) SendInput(data string) error {
	s.mu.RLock()
	if s.closed || s.handle == nil {
		s.mu.RUnlock()
		return fmt.Errorf("terminal session is closed")
	}
	handle := s.handle
	s.mu.RUnlock()
	if err := handle.SendInput(data); err != nil {
		return err
	}
	s.touch()
	return nil
}

func (s *remoteSession) Resize(cols, rows int) {
	s.mu.RLock()
	handle := s.handle
	closed := s.closed
	s.mu.RUnlock()
	if handle != nil && !closed {
		_ = handle.Resize(cols, rows)
	}
	s.mu.Lock()
	s.meta.Cols = cols
	s.meta.Rows = rows
	s.meta.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *remoteSession) Signal(name string) error {
	name = strings.TrimSpace(name)
	s.mu.RLock()
	if s.closed || s.handle == nil {
		s.mu.RUnlock()
		return fmt.Errorf("terminal session is closed")
	}
	handle := s.handle
	s.mu.RUnlock()
	return handle.Signal(name)
}

func (s *remoteSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.RLock()
		handle := s.handle
		s.mu.RUnlock()
		if handle != nil {
			err = handle.Close()
		}
		s.markExited(0, "")
	})
	return err
}

func (s *remoteSession) emit(event Event) {
	if event.SessionID == "" {
		event.SessionID = s.Metadata().SessionID
	}
	if event.HostID == "" {
		event.HostID = s.Metadata().HostID
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	if event.Status == "" {
		event.Status = s.Metadata().Status
	}

	s.mu.Lock()
	switch event.Type {
	case EventTypeStatus:
		if event.Status != "" {
			s.meta.Status = event.Status
		}
	case EventTypeExit:
		s.meta.Status = SessionStatusExited
		s.meta.ExitCode = event.Code
		s.meta.ExitSignal = event.Signal
		s.closed = true
	case EventTypeError:
		s.meta.Status = SessionStatusError
		s.closed = true
	}
	s.meta.UpdatedAt = event.UpdatedAt
	if event.Type == EventTypeOutput || event.Type == EventTypeStatus || event.Type == EventTypeExit || event.Type == EventTypeError {
		s.history = append(s.history, event)
		if len(s.history) > 64 {
			s.history = append([]Event(nil), s.history[len(s.history)-64:]...)
		}
	}
	clients := make([]chan Event, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		clients = append(clients, ch)
	}
	shouldClose := event.Type == EventTypeExit || event.Type == EventTypeError
	s.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- event:
		default:
		}
	}
	if shouldClose {
		s.closeSubscribers()
	}
}

func (s *remoteSession) touch() {
	s.mu.Lock()
	s.meta.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *remoteSession) markExited(exitCode int, exitSignal string) {
	s.emit(Event{
		Type:      EventTypeExit,
		SessionID: s.Metadata().SessionID,
		HostID:    s.Metadata().HostID,
		Status:    SessionStatusExited,
		Code:      exitCode,
		Signal:    exitSignal,
		UpdatedAt: time.Now().UTC(),
	})
}

func (s *remoteSession) closeSubscribers() {
	s.mu.Lock()
	clients := make([]chan Event, 0, len(s.subscribers))
	for id, ch := range s.subscribers {
		delete(s.subscribers, id)
		clients = append(clients, ch)
	}
	s.mu.Unlock()
	for _, ch := range clients {
		close(ch)
	}
}
