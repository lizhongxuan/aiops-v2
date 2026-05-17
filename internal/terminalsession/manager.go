package terminalsession

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusKilled    Status = "killed"
)

type Options struct {
	MaxSessions int
	WorkingDir  string
	IdleTimeout time.Duration
}

type StartRequest struct {
	Command    string
	Args       []string
	WorkingDir string
	YieldTime  time.Duration
}

type Session struct {
	ID        string
	Command   string
	Args      []string
	Status    Status
	StartedAt time.Time
}

type Snapshot struct {
	ID       string
	Status   Status
	Stdout   string
	Stderr   string
	ExitCode int
	Error    string
}

type Manager struct {
	mu       sync.RWMutex
	nextID   atomic.Uint64
	opts     Options
	sessions map[string]*sessionState
}

type sessionState struct {
	mu      sync.RWMutex
	id      string
	command string
	args    []string
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	status  Status
	exit    int
	errText string
	started time.Time
}

func NewManager(opts Options) *Manager {
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 4
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 10 * time.Minute
	}
	return &Manager{opts: opts, sessions: map[string]*sessionState{}}
}

func (m *Manager) Start(ctx context.Context, req StartRequest) (*Session, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("terminal session: command is required")
	}
	m.mu.Lock()
	if len(m.sessions) >= m.opts.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("terminal session: max sessions reached")
	}
	id := fmt.Sprintf("term-%d", m.nextID.Add(1))
	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, command, req.Args...)
	cmd.Dir = firstNonEmpty(req.WorkingDir, m.opts.WorkingDir, ".")
	cmd.Env = os.Environ()
	state := &sessionState{
		id:      id,
		command: command,
		args:    append([]string(nil), req.Args...),
		cancel:  cancel,
		cmd:     cmd,
		status:  StatusRunning,
		exit:    -1,
		started: time.Now(),
	}
	m.sessions[id] = state
	m.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.removeFailed(id, cancel)
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.removeFailed(id, cancel)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		m.removeFailed(id, cancel)
		return nil, err
	}
	go state.copyToBuffer(stdout, &state.stdout)
	go state.copyToBuffer(stderr, &state.stderr)
	go state.wait(runCtx)
	if req.YieldTime > 0 {
		time.Sleep(req.YieldTime)
	}
	return &Session{
		ID:        id,
		Command:   command,
		Args:      append([]string(nil), req.Args...),
		Status:    StatusRunning,
		StartedAt: state.started,
	}, nil
}

func (m *Manager) Read(id string, maxBytes int) Snapshot {
	m.mu.RLock()
	state := m.sessions[id]
	m.mu.RUnlock()
	if state == nil {
		return Snapshot{ID: id, Status: StatusFailed, Error: "terminal session not found", ExitCode: -1}
	}
	return state.snapshot(maxBytes)
}

func (m *Manager) Kill(id string) error {
	m.mu.RLock()
	state := m.sessions[id]
	m.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("terminal session %q not found", id)
	}
	state.mu.Lock()
	if state.status != StatusRunning {
		state.mu.Unlock()
		return nil
	}
	state.status = StatusKilled
	state.errText = "killed"
	state.mu.Unlock()
	state.cancel()
	return nil
}

func (m *Manager) removeFailed(id string, cancel context.CancelFunc) {
	cancel()
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (s *sessionState) copyToBuffer(reader io.Reader, dst *bytes.Buffer) {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			s.mu.Lock()
			_, _ = dst.Write(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *sessionState) wait(ctx context.Context) {
	err := s.cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusKilled {
		return
	}
	if err == nil {
		s.status = StatusCompleted
		s.exit = 0
		return
	}
	s.status = StatusFailed
	s.errText = err.Error()
	s.exit = -1
	if exitErr, ok := err.(*exec.ExitError); ok {
		s.exit = exitErr.ExitCode()
	}
	if ctx.Err() != nil {
		s.errText = ctx.Err().Error()
	}
}

func (s *sessionState) snapshot(maxBytes int) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		ID:       s.id,
		Status:   s.status,
		Stdout:   trimBufferString(s.stdout.String(), maxBytes),
		Stderr:   trimBufferString(s.stderr.String(), maxBytes),
		ExitCode: s.exit,
		Error:    s.errText,
	}
}

func trimBufferString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[len(value)-maxBytes:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if abs, err := filepath.Abs(trimmed); err == nil {
				return abs
			}
			return trimmed
		}
	}
	return "."
}
