package events

import "time"

type Event struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id,omitempty"`
	Workflow  string         `json:"workflow,omitempty"`
	Step      string         `json:"step,omitempty"`
	Host      string         `json:"host,omitempty"`
	Status    string         `json:"status,omitempty"`
	Message   string         `json:"message,omitempty"`
	Output    map[string]any `json:"output,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}
