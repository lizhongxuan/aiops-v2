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

type GraphRunState struct {
	GraphVersion string               `json:"graph_version,omitempty"`
	Nodes        map[string]NodeState `json:"nodes,omitempty"`
	Edges        map[string]EdgeState `json:"edges,omitempty"`
	UpdatedAt    time.Time            `json:"updated_at,omitempty"`
}

type NodeState struct {
	ID         string                `json:"id"`
	Name       string                `json:"name,omitempty"`
	Type       string                `json:"type,omitempty"`
	ParentID   string                `json:"parent_id,omitempty"`
	Status     string                `json:"status"`
	Attempt    int                   `json:"attempt,omitempty"`
	Iterations []NodeIterationState  `json:"iterations,omitempty"`
	StartedAt  time.Time             `json:"started_at,omitempty"`
	FinishedAt time.Time             `json:"finished_at,omitempty"`
	Message    string                `json:"message,omitempty"`
	Hosts      map[string]HostResult `json:"hosts,omitempty"`
	Output     map[string]any        `json:"output,omitempty"`
}

type NodeIterationState struct {
	Index      int                  `json:"index"`
	Status     string               `json:"status"`
	Item       any                  `json:"item,omitempty"`
	Nodes      map[string]NodeState `json:"nodes,omitempty"`
	StartedAt  time.Time            `json:"started_at,omitempty"`
	FinishedAt time.Time            `json:"finished_at,omitempty"`
	Message    string               `json:"message,omitempty"`
}

type EdgeState struct {
	ID         string    `json:"id"`
	Source     string    `json:"source,omitempty"`
	Target     string    `json:"target,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Status     string    `json:"status"`
	SelectedAt time.Time `json:"selected_at,omitempty"`
	Message    string    `json:"message,omitempty"`
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
	Graph             *GraphRunState           `json:"graph,omitempty"`
	Resources         map[string]ResourceState `json:"resources,omitempty"`
}
