package mcpstore

import (
	"errors"
	"time"
)

const (
	TypeStdio = "stdio"
	TypeHTTP  = "http"

	StatusRunning = "running"
	StatusStopped = "stopped"
)

var (
	ErrNotFound = errors.New("mcp server not found")
	ErrExists   = errors.New("mcp server already exists")
)

type ToolRecord struct {
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	ParametersSchema map[string]any `json:"parameters_schema,omitempty"`
}

type ServerRecord struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
	Status    string            `json:"status"`
	Tools     []ToolRecord      `json:"tools,omitempty"`
	LastError string            `json:"last_error,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
}
