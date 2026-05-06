package incidents

import "time"

type EvidenceRef struct {
	ID         string    `json:"id"`
	Source     string    `json:"source,omitempty"`
	RawRef     string    `json:"rawRef,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Confidence string    `json:"confidence,omitempty"`
	EntityID   string    `json:"entityId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
