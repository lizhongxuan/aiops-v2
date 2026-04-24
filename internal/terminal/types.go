package terminal

import "time"

type SessionStatus string

const (
	SessionStatusStarting SessionStatus = "starting"
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusExited    SessionStatus = "exited"
	SessionStatusError     SessionStatus = "error"
)

type EventType string

const (
	EventTypeReady  EventType = "ready"
	EventTypeOutput EventType = "output"
	EventTypeStatus EventType = "status"
	EventTypeExit   EventType = "exit"
	EventTypeError  EventType = "error"
)

type CreateSessionRequest struct {
	HostID string
	Cwd    string
	Shell  string
	Cols   int
	Rows   int
}

type SessionMetadata struct {
	SessionID string        `json:"sessionId"`
	HostID    string        `json:"hostId,omitempty"`
	Cwd       string        `json:"cwd,omitempty"`
	Shell     string        `json:"shell,omitempty"`
	Cols      int           `json:"cols,omitempty"`
	Rows      int           `json:"rows,omitempty"`
	Status    SessionStatus `json:"status,omitempty"`
	StartedAt time.Time     `json:"startedAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	PID       int           `json:"pid,omitempty"`
	ExitCode  int           `json:"exitCode,omitempty"`
	ExitSignal string       `json:"exitSignal,omitempty"`
}

type Event struct {
	Type       EventType     `json:"type"`
	SessionID  string        `json:"sessionId,omitempty"`
	HostID     string        `json:"hostId,omitempty"`
	Cwd        string        `json:"cwd,omitempty"`
	Shell      string        `json:"shell,omitempty"`
	Cols       int           `json:"cols,omitempty"`
	Rows       int           `json:"rows,omitempty"`
	Status     SessionStatus `json:"status,omitempty"`
	Data       string        `json:"data,omitempty"`
	Message    string        `json:"message,omitempty"`
	Code       int           `json:"code,omitempty"`
	Signal     string        `json:"signal,omitempty"`
	StartedAt  time.Time     `json:"startedAt,omitempty"`
	UpdatedAt  time.Time     `json:"updatedAt,omitempty"`
}
