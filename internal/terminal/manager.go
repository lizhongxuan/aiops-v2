package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CommandFactory func(req CreateSessionRequest) (*exec.Cmd, error)

type ManagerOption func(*Manager)

type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	commandFactory  CommandFactory
	defaultHostID   string
	defaultShell    string
	defaultCwd      string
}

func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		sessions:      map[string]*Session{},
		defaultHostID: "server-local",
		defaultShell:  "/bin/zsh",
		defaultCwd:    "~",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

func WithCommandFactory(factory CommandFactory) ManagerOption {
	return func(m *Manager) {
		m.commandFactory = factory
	}
}

func WithDefaultShell(shell string) ManagerOption {
	return func(m *Manager) {
		if strings.TrimSpace(shell) != "" {
			m.defaultShell = strings.TrimSpace(shell)
		}
	}
}

func (m *Manager) CreateSession(_ context.Context, req CreateSessionRequest) (SessionMetadata, error) {
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		hostID = m.defaultHostID
	}
	shell := strings.TrimSpace(req.Shell)
	if shell == "" {
		shell = m.defaultShell
	}
	cwd := strings.TrimSpace(req.Cwd)
	if cwd == "" {
		cwd = m.defaultCwd
	}
	meta := SessionMetadata{
		SessionID:  fmt.Sprintf("term-%d", time.Now().UnixNano()),
		HostID:    hostID,
		Cwd:       cwd,
		Shell:     shell,
		Cols:      req.Cols,
		Rows:      req.Rows,
		Status:    SessionStatusStarting,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	cmd, err := m.buildCommand(req, meta)
	if err != nil {
		return SessionMetadata{}, err
	}

	stdin, stdout, stderr, err := startCommand(cmd, cwd)
	if err != nil {
		return SessionMetadata{}, err
	}
	meta.Status = SessionStatusRunning
	meta.PID = 0
	if cmd.Process != nil {
		meta.PID = cmd.Process.Pid
	}
	session := newSession(meta, cmd, stdin)
	session.startReaders(stdout, stderr)

	m.mu.Lock()
	m.sessions[meta.SessionID] = session
	m.mu.Unlock()
	return session.Metadata(), nil
}

func (m *Manager) buildCommand(req CreateSessionRequest, meta SessionMetadata) (*exec.Cmd, error) {
	if m.commandFactory != nil {
		return m.commandFactory(req)
	}
	args := shellArgs(meta.Shell)
	cmd := exec.Command(meta.Shell, args...)
	if cwd := resolveWorkingDir(meta.Cwd); cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd, nil
}

func (m *Manager) GetSession(sessionID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

func (m *Manager) ListSessions() []SessionMetadata {
	m.mu.RLock()
	items := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		items = append(items, session)
	}
	m.mu.RUnlock()

	result := make([]SessionMetadata, 0, len(items))
	for _, session := range items {
		result = append(result, session.Metadata())
	}
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].SessionID < result[j].SessionID
	})
	return result
}

func (m *Manager) Subscribe(sessionID string) (*Session, <-chan Event, func(), error) {
	session := m.GetSession(sessionID)
	if session == nil {
		return nil, nil, nil, fmt.Errorf("terminal session %q not found", sessionID)
	}
	events, release := session.Subscribe()
	return session, events, release, nil
}

func (m *Manager) ResolveSession(sessionID string) (*Session, error) {
	session := m.GetSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("terminal session %q not found", sessionID)
	}
	return session, nil
}

func shellArgs(shell string) []string {
	name := filepath.Base(strings.TrimSpace(shell))
	switch name {
	case "zsh", "bash", "sh", "fish", "ksh", "tcsh", "csh":
		return []string{"-i"}
	default:
		return nil
	}
}

func resolveWorkingDir(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == "~" {
		return ""
	}
	if filepath.IsAbs(cwd) {
		return cwd
	}
	return cwd
}
