package service

import (
	"time"

	"runner/state"
)

type WorkflowRecord struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	RawYAML     []byte            `json:"-"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

type ScriptRecord struct {
	Name        string            `json:"name"`
	Language    string            `json:"language"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Content     string            `json:"content"`
	Version     int64             `json:"version"`
	Checksum    string            `json:"checksum"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type RunMeta struct {
	RunID          string            `json:"run_id"`
	WorkflowName   string            `json:"workflow_name,omitempty"`
	WorkflowYAML   string            `json:"workflow_yaml,omitempty"`
	Vars           map[string]any    `json:"vars,omitempty"`
	TriggeredBy    string            `json:"triggered_by,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	QueuedAt       time.Time         `json:"queued_at"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	FinishedAt     time.Time         `json:"finished_at,omitempty"`
	Status         string            `json:"status"`
	Message        string            `json:"message,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

type RunDetail struct {
	RunMeta
	WorkflowVersion   string                         `json:"workflow_version,omitempty"`
	LastError         string                         `json:"last_error,omitempty"`
	InterruptedReason string                         `json:"interrupted_reason,omitempty"`
	LastNotifyError   string                         `json:"last_notify_error,omitempty"`
	Version           int64                          `json:"version"`
	UpdatedAt         time.Time                      `json:"updated_at,omitempty"`
	Args              map[string]any                 `json:"args,omitempty"`
	Steps             []state.StepState              `json:"steps,omitempty"`
	Resources         map[string]state.ResourceState `json:"resources,omitempty"`
}

type RunRequest struct {
	WorkflowName   string         `json:"workflow_name"`
	WorkflowYAML   string         `json:"workflow_yaml"`
	Vars           map[string]any `json:"vars"`
	TriggeredBy    string         `json:"triggered_by"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type RunResponse struct {
	RunID        string    `json:"run_id"`
	Status       string    `json:"status"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type RunFilter struct {
	Status   string
	Workflow string
	Limit    int
}

type ScriptFilter struct {
	Language string
	Tag      string
	Limit    int
}

type AgentFilter struct {
	Status string
	Tag    string
	Limit  int
}
