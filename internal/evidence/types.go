package evidence

import (
	"time"

	"aiops-v2/internal/resourcebinding"
)

// Kind classifies evidence so downstream policy/UI can distinguish signals.
type Kind string

const (
	KindMetric Kind = "metric"
	KindEvent  Kind = "event"
	KindLog    Kind = "log"
	KindChange Kind = "change"
	KindManual Kind = "manual"
	KindOther  Kind = "other"
)

// Relation describes how evidence is connected to an incident.
type Relation string

const (
	RelationSupports Relation = "supports"
	RelationRefutes  Relation = "refutes"
	RelationContext  Relation = "context"
)

// Record is a durable evidence item addressable by Ref.
type Record struct {
	Ref         string                       `json:"ref"`
	IncidentID  string                       `json:"incidentId,omitempty"`
	SourceTool  string                       `json:"sourceTool,omitempty"`
	Source      string                       `json:"source,omitempty"`
	Kind        Kind                         `json:"kind,omitempty"`
	Service     string                       `json:"service,omitempty"`
	Environment string                       `json:"environment,omitempty"`
	ResourceRef *resourcebinding.ResourceRef `json:"resourceRef,omitempty"`
	TimeRange   string                       `json:"timeRange,omitempty"`
	Summary     string                       `json:"summary"`
	Data        map[string]any               `json:"data,omitempty"`
	SessionID   string                       `json:"sessionId,omitempty"`
	TurnID      string                       `json:"turnId,omitempty"`
	ToolCallID  string                       `json:"toolCallId,omitempty"`
	CreatedAt   time.Time                    `json:"createdAt"`
}

// RecordRequest is the input contract for creating evidence.
type RecordRequest struct {
	IncidentID  string                       `json:"incidentId,omitempty"`
	SourceTool  string                       `json:"sourceTool,omitempty"`
	Source      string                       `json:"source,omitempty"`
	Kind        Kind                         `json:"kind,omitempty"`
	Service     string                       `json:"service,omitempty"`
	Environment string                       `json:"environment,omitempty"`
	ResourceRef *resourcebinding.ResourceRef `json:"resourceRef,omitempty"`
	TimeRange   string                       `json:"timeRange,omitempty"`
	Summary     string                       `json:"summary"`
	Data        map[string]any               `json:"data,omitempty"`
	SessionID   string                       `json:"sessionId,omitempty"`
	TurnID      string                       `json:"turnId,omitempty"`
	ToolCallID  string                       `json:"toolCallId,omitempty"`
}

// IncidentLink records an evidence-to-incident relation.
type IncidentLink struct {
	IncidentID string    `json:"incidentId"`
	Ref        string    `json:"ref"`
	Relation   Relation  `json:"relation"`
	CreatedAt  time.Time `json:"createdAt"`
}
