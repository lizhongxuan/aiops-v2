package state

import "time"

type ResourceState struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Desired   map[string]any `json:"desired"`
	Current   map[string]any `json:"current"`
	Diff      map[string]any `json:"diff"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type HostResult struct {
	Host       string         `json:"host"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
	Message    string         `json:"message,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
}

type StepState struct {
	Name       string                `json:"name"`
	Status     string                `json:"status"`
	StartedAt  time.Time             `json:"started_at,omitempty"`
	FinishedAt time.Time             `json:"finished_at,omitempty"`
	Message    string                `json:"message,omitempty"`
	Hosts      map[string]HostResult `json:"hosts,omitempty"`
}

type RunState struct {
	RunID             string                   `json:"run_id"`
	WorkflowName      string                   `json:"workflow_name"`
	WorkflowVersion   string                   `json:"workflow_version,omitempty"`
	Status            string                   `json:"status"`
	Message           string                   `json:"message,omitempty"`
	LastError         string                   `json:"last_error,omitempty"`
	InterruptedReason string                   `json:"interrupted_reason,omitempty"`
	LastNotifyError   string                   `json:"last_notify_error,omitempty"`
	Version           int64                    `json:"version"`
	StartedAt         time.Time                `json:"started_at,omitempty"`
	FinishedAt        time.Time                `json:"finished_at,omitempty"`
	UpdatedAt         time.Time                `json:"updated_at,omitempty"`
	Args              map[string]any           `json:"args,omitempty"`
	Steps             []StepState              `json:"steps,omitempty"`
	Resources         map[string]ResourceState `json:"resources,omitempty"`
}
