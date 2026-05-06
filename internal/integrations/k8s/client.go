package k8s

import "time"

type Options struct {
	ActionTokenSecret []byte
	Now               func() time.Time
}

type Workload struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int    `json:"replicas"`
	Image     string `json:"image,omitempty"`
	Status    string `json:"status"`
}

type Event struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type LogLine struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type RolloutStatus struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Revision  string `json:"revision,omitempty"`
}
