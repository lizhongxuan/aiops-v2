package terminal

import "time"

type SessionStatus string

const (
	SessionStatusStarting SessionStatus = "starting"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusExited   SessionStatus = "exited"
	SessionStatusError    SessionStatus = "error"
)

type EventType string

const (
	EventTypeReady  EventType = "ready"
	EventTypeOutput EventType = "output"
	EventTypeStatus EventType = "status"
	EventTypeExit   EventType = "exit"
	EventTypeError  EventType = "error"
)

type AuditEventType string

const (
	AuditEventSessionCreated      AuditEventType = "session_created"
	AuditEventSessionConnected    AuditEventType = "session_connected"
	AuditEventSessionReconnected  AuditEventType = "session_reconnected"
	AuditEventSessionDisconnected AuditEventType = "session_disconnected"
	AuditEventSessionClosed       AuditEventType = "session_closed"
)

type CreateSessionRequest struct {
	HostID string
	Cwd    string
	Shell  string
	Cols   int
	Rows   int
}

type SessionMetadata struct {
	SessionID  string        `json:"sessionId"`
	HostID     string        `json:"hostId,omitempty"`
	Cwd        string        `json:"cwd,omitempty"`
	Shell      string        `json:"shell,omitempty"`
	Cols       int           `json:"cols,omitempty"`
	Rows       int           `json:"rows,omitempty"`
	Status     SessionStatus `json:"status,omitempty"`
	Source     string        `json:"source,omitempty"`
	StartedAt  time.Time     `json:"startedAt"`
	UpdatedAt  time.Time     `json:"updatedAt"`
	PID        int           `json:"pid,omitempty"`
	ExitCode   int           `json:"exitCode,omitempty"`
	ExitSignal string        `json:"exitSignal,omitempty"`
}

type AuditEvent struct {
	Type      AuditEventType `json:"type"`
	SessionID string         `json:"sessionId"`
	HostID    string         `json:"hostId,omitempty"`
	Source    string         `json:"source,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type Event struct {
	Type      EventType     `json:"type"`
	SessionID string        `json:"sessionId,omitempty"`
	HostID    string        `json:"hostId,omitempty"`
	Cwd       string        `json:"cwd,omitempty"`
	Shell     string        `json:"shell,omitempty"`
	Cols      int           `json:"cols,omitempty"`
	Rows      int           `json:"rows,omitempty"`
	Status    SessionStatus `json:"status,omitempty"`
	Data      string        `json:"data,omitempty"`
	Message   string        `json:"message,omitempty"`
	Code      int           `json:"code,omitempty"`
	Signal    string        `json:"signal,omitempty"`
	StartedAt time.Time     `json:"startedAt,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt,omitempty"`
}

type TerminalSession interface {
	Metadata() SessionMetadata
	Subscribe() (<-chan Event, func())
	SendInput(data string) error
	Resize(cols, rows int)
	Signal(name string) error
	Close() error
}
