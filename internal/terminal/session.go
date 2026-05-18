package terminal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Session struct {
	mu          sync.RWMutex
	meta        SessionMetadata
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	subscribers map[int]chan Event
	nextSubID   int
	history     []Event
	closed      bool
	closeOnce   sync.Once
	doneCh      chan struct{}
}

func newSession(meta SessionMetadata, cmd *exec.Cmd, stdin io.WriteCloser) *Session {
	return &Session{
		meta:        meta,
		cmd:         cmd,
		stdin:       stdin,
		subscribers: map[int]chan Event{},
		doneCh:      make(chan struct{}),
	}
}

func (s *Session) Metadata() SessionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

func (s *Session) Subscribe() (<-chan Event, func()) {
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

func (s *Session) SendInput(data string) error {
	s.mu.RLock()
	if s.closed || s.stdin == nil {
		s.mu.RUnlock()
		return fmt.Errorf("terminal session is closed")
	}
	stdin := s.stdin
	s.mu.RUnlock()

	if _, err := io.WriteString(stdin, data); err != nil {
		return err
	}
	s.touch()
	return nil
}

func (s *Session) Resize(cols, rows int) {
	s.mu.Lock()
	s.meta.Cols = cols
	s.meta.Rows = rows
	s.meta.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Session) Signal(name string) error {
	s.mu.RLock()
	cmd := s.cmd
	s.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("terminal session is not running")
	}
	sig, err := parseSignal(name)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return cmd.Process.Signal(sig)
	}
	return cmd.Process.Signal(sig)
}

func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		stdin := s.stdin
		cmd := s.cmd
		s.mu.Unlock()
		if stdin != nil {
			_ = stdin.Close()
		}
		if cmd != nil && cmd.Process != nil {
			if runtime.GOOS == "windows" {
				err = cmd.Process.Kill()
			} else {
				err = cmd.Process.Signal(syscall.SIGTERM)
			}
		}
		s.touch()
	})
	return err
}

func (s *Session) touch() {
	s.mu.Lock()
	s.meta.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Session) broadcast(event Event) {
	s.mu.Lock()
	switch event.Type {
	case EventTypeOutput, EventTypeStatus, EventTypeExit, EventTypeError:
		s.history = append(s.history, event)
		if len(s.history) > 64 {
			s.history = append([]Event(nil), s.history[len(s.history)-64:]...)
		}
	}
	clients := make([]chan Event, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		clients = append(clients, ch)
	}
	s.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Session) markExited(exitCode int, exitSignal string) {
	s.mu.Lock()
	s.meta.Status = SessionStatusExited
	s.meta.ExitCode = exitCode
	s.meta.ExitSignal = exitSignal
	s.meta.UpdatedAt = time.Now().UTC()
	s.closed = true
	cmd := s.cmd
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	s.broadcast(Event{
		Type:      EventTypeExit,
		SessionID: s.meta.SessionID,
		HostID:    s.meta.HostID,
		Status:    SessionStatusExited,
		Code:      exitCode,
		Signal:    exitSignal,
		UpdatedAt: time.Now().UTC(),
	})
	s.closeSubscribers()
}

func (s *Session) markError(message string) {
	s.mu.Lock()
	s.meta.Status = SessionStatusError
	s.meta.UpdatedAt = time.Now().UTC()
	s.closed = true
	s.mu.Unlock()
	s.broadcast(Event{
		Type:      EventTypeError,
		SessionID: s.meta.SessionID,
		HostID:    s.meta.HostID,
		Status:    SessionStatusError,
		Message:   message,
		UpdatedAt: time.Now().UTC(),
	})
	s.closeSubscribers()
}

func (s *Session) closeSubscribers() {
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

func (s *Session) startReaders(stdout io.Reader, stderr io.Reader) {
	var wg sync.WaitGroup
	pump := func(reader io.Reader) {
		defer wg.Done()
		if reader == nil {
			return
		}
		bufReader := bufio.NewReader(reader)
		buf := make([]byte, 4096)
		for {
			n, err := bufReader.Read(buf)
			if n > 0 {
				s.broadcast(Event{
					Type:      EventTypeOutput,
					SessionID: s.meta.SessionID,
					HostID:    s.meta.HostID,
					Data:      string(buf[:n]),
					Status:    s.meta.Status,
					UpdatedAt: time.Now().UTC(),
				})
				s.touch()
			}
			if err != nil {
				if err != io.EOF {
					s.broadcast(Event{
						Type:      EventTypeError,
						SessionID: s.meta.SessionID,
						HostID:    s.meta.HostID,
						Message:   err.Error(),
						Status:    SessionStatusError,
						UpdatedAt: time.Now().UTC(),
					})
				}
				return
			}
		}
	}

	wg.Add(2)
	go pump(stdout)
	go pump(stderr)

	go func() {
		wg.Wait()
		s.mu.RLock()
		cmd := s.cmd
		s.mu.RUnlock()
		exitCode, exitSignal := waitCommand(cmd)
		s.markExited(exitCode, exitSignal)
	}()
}

func parseSignal(name string) (syscall.Signal, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGKILL":
		return syscall.SIGKILL, nil
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGQUIT":
		return syscall.SIGQUIT, nil
	default:
		return 0, fmt.Errorf("unsupported signal %q", name)
	}
}

func startCommand(cmd *exec.Cmd, cwd string) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	if cmd.Dir == "" {
		if resolved := resolveWorkingDir(cwd); resolved != "" {
			cmd.Dir = resolved
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		runCommandCleanup(cmd)
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		runCommandCleanup(cmd)
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		runCommandCleanup(cmd)
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		runCommandCleanup(cmd)
		return nil, nil, nil, err
	}
	return stdin, stdout, stderr, nil
}

func parseExitError(err error) (int, string) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1, ""
	}
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		if status.Signaled() {
			return 128 + int(status.Signal()), status.Signal().String()
		}
		return status.ExitStatus(), ""
	}
	if exitErr.ProcessState != nil {
		return exitErr.ProcessState.ExitCode(), ""
	}
	return 1, ""
}
