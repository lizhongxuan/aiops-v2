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

	"aiops-v2/internal/observability"
)

type CommandFactory func(req CreateSessionRequest) (*exec.Cmd, error)

type ManagerOption func(*Manager)

type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]TerminalSession
	auditEvents    []AuditEvent
	everConnected  map[string]bool
	commandFactory CommandFactory
	remoteBackend  RemoteTerminalBackend
	defaultHostID  string
	defaultShell   string
	defaultCwd     string
}

func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		sessions:      map[string]TerminalSession{},
		everConnected: map[string]bool{},
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

func WithRemoteBackend(backend RemoteTerminalBackend) ManagerOption {
	return func(m *Manager) {
		m.remoteBackend = backend
	}
}

func (m *Manager) SetCommandFactory(factory CommandFactory) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandFactory = factory
}

func (m *Manager) SetRemoteBackend(backend RemoteTerminalBackend) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remoteBackend = backend
}

func WithDefaultShell(shell string) ManagerOption {
	return func(m *Manager) {
		if strings.TrimSpace(shell) != "" {
			m.defaultShell = strings.TrimSpace(shell)
		}
	}
}

func (m *Manager) CreateSession(ctx context.Context, req CreateSessionRequest) (SessionMetadata, error) {
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
		SessionID: fmt.Sprintf("term-%d", time.Now().UnixNano()),
		HostID:    hostID,
		Cwd:       cwd,
		Shell:     shell,
		Cols:      req.Cols,
		Rows:      req.Rows,
		Status:    SessionStatusStarting,
		Source:    "manual_terminal",
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if hostID != m.defaultHostID {
		m.mu.RLock()
		remoteBackend := m.remoteBackend
		hasCommandFactory := m.commandFactory != nil
		m.mu.RUnlock()
		if remoteBackend != nil {
			return m.createRemoteSession(ctx, meta, remoteBackend)
		}
		if !hasCommandFactory {
			observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, false)
			observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, false)
			return SessionMetadata{}, fmt.Errorf("remote terminal backend is not configured for host %s", hostID)
		}
	}

	cmd, err := m.buildCommand(req, meta)
	if err != nil {
		observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, false)
		observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, false)
		return SessionMetadata{}, err
	}

	stdin, stdout, stderr, err := startCommand(cmd, cwd)
	if err != nil {
		observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, false)
		observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, false)
		return SessionMetadata{}, err
	}
	meta.Status = SessionStatusRunning
	meta.PID = 0
	if cmd.Process != nil {
		meta.PID = cmd.Process.Pid
	}
	session := newSession(meta, cmd, stdin)
	session.startReaders(stdout, stderr)
	wrapped := m.wrapSession(session)

	m.mu.Lock()
	m.sessions[meta.SessionID] = wrapped
	m.mu.Unlock()
	m.recordAudit(AuditEventSessionCreated, meta)
	observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, true)
	observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, true)
	return wrapped.Metadata(), nil
}

func (m *Manager) createRemoteSession(ctx context.Context, meta SessionMetadata, backend RemoteTerminalBackend) (SessionMetadata, error) {
	meta.Status = SessionStatusRunning
	session := newRemoteSession(meta)
	handle, err := backend.OpenTerminal(ctx, RemoteTerminalOpenRequest{
		SessionID: meta.SessionID,
		HostID:    meta.HostID,
		Cwd:       meta.Cwd,
		Shell:     meta.Shell,
		Cols:      meta.Cols,
		Rows:      meta.Rows,
		StartedAt: meta.StartedAt,
		Emit:      session.emit,
	})
	if err != nil {
		observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, false)
		observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, false)
		return SessionMetadata{}, err
	}
	session.setHandle(handle)
	wrapped := m.wrapSession(session)
	m.mu.Lock()
	m.sessions[meta.SessionID] = wrapped
	m.mu.Unlock()
	m.recordAudit(AuditEventSessionCreated, meta)
	observability.RecordOpsMetric(observability.OpsMetricTerminalConnection, true)
	observability.RecordOpsMetric(observability.OpsMetricHumanHandoff, true)
	return wrapped.Metadata(), nil
}

func (m *Manager) buildCommand(req CreateSessionRequest, meta SessionMetadata) (*exec.Cmd, error) {
	if m.commandFactory != nil {
		cmd, err := m.commandFactory(req)
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			return cmd, nil
		}
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

func (m *Manager) GetSession(sessionID string) TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

func (m *Manager) ListSessions() []SessionMetadata {
	m.mu.RLock()
	items := make([]TerminalSession, 0, len(m.sessions))
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

func (m *Manager) Subscribe(sessionID string) (TerminalSession, <-chan Event, func(), error) {
	session := m.GetSession(sessionID)
	if session == nil {
		return nil, nil, nil, fmt.Errorf("terminal session %q not found", sessionID)
	}
	events, release := session.Subscribe()
	return session, events, release, nil
}

func (m *Manager) ListAuditEvents() []AuditEvent {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]AuditEvent(nil), m.auditEvents...)
	return out
}

func (m *Manager) wrapSession(session TerminalSession) TerminalSession {
	if session == nil {
		return nil
	}
	return &auditedSession{inner: session, manager: m}
}

func (m *Manager) recordAudit(eventType AuditEventType, meta SessionMetadata) {
	if m == nil || strings.TrimSpace(meta.SessionID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditEvents = append(m.auditEvents, AuditEvent{
		Type:      eventType,
		SessionID: meta.SessionID,
		HostID:    meta.HostID,
		Source:    meta.Source,
		CreatedAt: time.Now().UTC(),
	})
}

func (m *Manager) recordSubscribe(meta SessionMetadata) {
	if m == nil || strings.TrimSpace(meta.SessionID) == "" {
		return
	}
	m.mu.Lock()
	eventType := AuditEventSessionConnected
	if m.everConnected[meta.SessionID] {
		eventType = AuditEventSessionReconnected
	}
	m.everConnected[meta.SessionID] = true
	m.auditEvents = append(m.auditEvents, AuditEvent{
		Type:      eventType,
		SessionID: meta.SessionID,
		HostID:    meta.HostID,
		Source:    meta.Source,
		CreatedAt: time.Now().UTC(),
	})
	m.mu.Unlock()
}

type auditedSession struct {
	inner   TerminalSession
	manager *Manager
}

func (s *auditedSession) Metadata() SessionMetadata {
	if s == nil || s.inner == nil {
		return SessionMetadata{}
	}
	return s.inner.Metadata()
}

func (s *auditedSession) Subscribe() (<-chan Event, func()) {
	if s == nil || s.inner == nil {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	events, release := s.inner.Subscribe()
	meta := s.inner.Metadata()
	if s.manager != nil {
		s.manager.recordSubscribe(meta)
	}
	return events, func() {
		release()
		if s.manager != nil {
			s.manager.recordAudit(AuditEventSessionDisconnected, meta)
		}
	}
}

func (s *auditedSession) SendInput(data string) error {
	err := s.inner.SendInput(data)
	observability.RecordOpsMetric(observability.OpsMetricManualTerminalCommand, err == nil)
	return err
}

func (s *auditedSession) Resize(cols, rows int) {
	s.inner.Resize(cols, rows)
}

func (s *auditedSession) Signal(name string) error {
	return s.inner.Signal(name)
}

func (s *auditedSession) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	err := s.inner.Close()
	if s.manager != nil {
		s.manager.recordAudit(AuditEventSessionClosed, s.inner.Metadata())
	}
	return err
}

func (m *Manager) ResolveSession(sessionID string) (TerminalSession, error) {
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
