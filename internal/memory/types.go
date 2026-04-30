package memory

import (
	"errors"
	"time"
)

var ErrDisabled = errors.New("memory store is disabled")

type Scope string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
)

type Config struct {
	Path    string
	Enabled bool
}

type Item struct {
	ID         string    `json:"id"`
	Scope      Scope     `json:"scope"`
	SessionID  string    `json:"sessionId,omitempty"`
	ProjectID  string    `json:"projectId,omitempty"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	UsageCount int       `json:"usageCount,omitempty"`
	Stale      bool      `json:"stale,omitempty"`
}

type Query struct {
	Scope     Scope
	SessionID string
	ProjectID string
	Text      string
	Limit     int
}
