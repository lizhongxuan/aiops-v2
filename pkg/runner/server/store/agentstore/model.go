package agentstore

import "time"

const (
	StatusOnline   = "online"
	StatusOffline  = "offline"
	StatusDegraded = "degraded"
)

type AgentRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Address      string    `json:"address"`
	Token        string    `json:"token"`
	Tags         []string  `json:"tags,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Status       string    `json:"status"`
	LastBeatAt   time.Time `json:"last_beat_at,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	LastLoad     float64   `json:"last_load,omitempty"`
	RunningTasks int       `json:"running_tasks,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Heartbeat struct {
	Status       string  `json:"status"`
	Load         float64 `json:"load"`
	RunningTasks int     `json:"running_tasks"`
	Error        string  `json:"error,omitempty"`
}

type Filter struct {
	Status string
	Tag    string
	Limit  int
}
