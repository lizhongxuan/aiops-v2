package memory

import (
	"errors"
	"time"
)

var ErrDisabled = errors.New("memory store is disabled")

type Scope string
type Kind string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"

	KindOpsManualManualHint Kind = "ops_manual_manual_hint"
	KindOpsManualParamHint  Kind = "ops_manual_param_hint"
)

type Config struct {
	Path    string
	Enabled bool
}

type Item struct {
	ID              string    `json:"id"`
	Scope           Scope     `json:"scope"`
	SessionID       string    `json:"sessionId,omitempty"`
	ProjectID       string    `json:"projectId,omitempty"`
	Kind            Kind      `json:"kind,omitempty"`
	ObjectType      string    `json:"objectType,omitempty"`
	OperationAction string    `json:"operationAction,omitempty"`
	TargetAlias     string    `json:"targetAlias,omitempty"`
	ManualID        string    `json:"manualId,omitempty"`
	WorkflowID      string    `json:"workflowId,omitempty"`
	ParamID         string    `json:"paramId,omitempty"`
	ParamValue      string    `json:"paramValue,omitempty"`
	ParamLabel      string    `json:"paramLabel,omitempty"`
	Source          string    `json:"source,omitempty"`
	Redacted        bool      `json:"redacted,omitempty"`
	Text            string    `json:"text"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
	LastUsedAt      time.Time `json:"lastUsedAt,omitempty"`
	UsageCount      int       `json:"usageCount,omitempty"`
	Stale           bool      `json:"stale,omitempty"`
}

type Query struct {
	Scope     Scope
	SessionID string
	ProjectID string
	Text      string
	Limit     int
}
