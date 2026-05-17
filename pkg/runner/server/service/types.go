package service

import (
	"context"
	"time"

	"runner/state"
)

const (
	WorkflowStatusDraft        = "draft"
	WorkflowStatusValidated    = "validated"
	WorkflowStatusDryRunPassed = "dry_run_passed"
	WorkflowStatusPublished    = "published"
)

type WorkflowRecord struct {
	Name                string            `json:"name"`
	Description         string            `json:"description,omitempty"`
	Version             string            `json:"version,omitempty"`
	RawYAML             []byte            `json:"-"`
	Labels              map[string]string `json:"labels,omitempty"`
	SaveNote            string            `json:"save_note,omitempty"`
	SaveNoteSet         bool              `json:"-"`
	Status              string            `json:"status,omitempty"`
	ValidatedGraphHash  string            `json:"validated_graph_hash,omitempty"`
	ValidatedLayoutHash string            `json:"validated_layout_hash,omitempty"`
	ValidatedAt         time.Time         `json:"validated_at,omitempty"`
	ValidatedBy         string            `json:"validated_by,omitempty"`
	DryRunGraphHash     string            `json:"dry_run_graph_hash,omitempty"`
	DryRunLayoutHash    string            `json:"dry_run_layout_hash,omitempty"`
	DryRunAt            time.Time         `json:"dry_run_at,omitempty"`
	DryRunBy            string            `json:"dry_run_by,omitempty"`
	PublishedGraphHash  string            `json:"published_graph_hash,omitempty"`
	PublishedAt         time.Time         `json:"published_at,omitempty"`
	CreatedAt           time.Time         `json:"created_at,omitempty"`
	UpdatedAt           time.Time         `json:"updated_at,omitempty"`
}

type WorkflowReferenceGuardMode string

const (
	WorkflowReferenceGuardModeEnforce WorkflowReferenceGuardMode = "enforce"
	WorkflowReferenceGuardModeWarn    WorkflowReferenceGuardMode = "warn"
)

type WorkflowGuardWarning struct {
	Code       string              `json:"code"`
	Message    string              `json:"message"`
	References []WorkflowReference `json:"references,omitempty"`
}

type WorkflowValidateOptions struct {
	Actor string
}

type WorkflowDryRunOptions struct {
	Actor             string
	ExpectedGraphHash string
}

type WorkflowPublishOptions struct {
	SaveNote            string
	SaveNoteSet         bool
	RiskAcknowledged    bool
	WarningAcknowledged bool
	ValidatedGraphHash  string
	DryRunGraphHash     string
}

type WorkflowRollbackOptions struct {
	SaveNote string
}

type WorkflowImportOptions struct {
	Overwrite bool
	SaveNote  string
}

type WorkflowBundle struct {
	BundleVersion string                  `json:"bundle_version"`
	ExportedAt    time.Time               `json:"exported_at"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description,omitempty"`
	Version       string                  `json:"version,omitempty"`
	YAML          string                  `json:"yaml"`
	Labels        map[string]string       `json:"labels,omitempty"`
	SaveNote      string                  `json:"save_note,omitempty"`
	Status        string                  `json:"status,omitempty"`
	PublishedAt   time.Time               `json:"published_at,omitempty"`
	Versions      []WorkflowBundleVersion `json:"versions,omitempty"`
}

type WorkflowBundleVersion struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version,omitempty"`
	Status      string    `json:"status,omitempty"`
	SaveNote    string    `json:"save_note,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Checksum    string    `json:"checksum,omitempty"`
	YAML        string    `json:"yaml"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

type WorkflowVersionRecord struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version,omitempty"`
	Status      string    `json:"status,omitempty"`
	SaveNote    string    `json:"save_note,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Checksum    string    `json:"checksum,omitempty"`
	RawYAML     []byte    `json:"-"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
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
	RunID           string            `json:"run_id"`
	WorkflowName    string            `json:"workflow_name,omitempty"`
	WorkflowYAML    string            `json:"workflow_yaml,omitempty"`
	ManualID        string            `json:"manual_id,omitempty"`
	WorkflowID      string            `json:"workflow_id,omitempty"`
	WorkflowVersion string            `json:"workflow_version,omitempty"`
	WorkflowDigest  string            `json:"workflow_digest,omitempty"`
	Vars            map[string]any    `json:"vars,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	TriggeredBy     string            `json:"triggered_by,omitempty"`
	IdempotencyKey  string            `json:"idempotency_key,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	QueuedAt        time.Time         `json:"queued_at"`
	StartedAt       time.Time         `json:"started_at,omitempty"`
	FinishedAt      time.Time         `json:"finished_at,omitempty"`
	Status          string            `json:"status"`
	Message         string            `json:"message,omitempty"`
	Summary         string            `json:"summary,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type OpsManualRunRecord struct {
	RunID           string         `json:"run_id"`
	ManualID        string         `json:"manual_id,omitempty"`
	WorkflowID      string         `json:"workflow_id,omitempty"`
	WorkflowName    string         `json:"workflow_name,omitempty"`
	WorkflowVersion string         `json:"workflow_version,omitempty"`
	WorkflowDigest  string         `json:"workflow_digest,omitempty"`
	Status          string         `json:"status"`
	ErrorCode       string         `json:"error_code,omitempty"`
	Message         string         `json:"message,omitempty"`
	TriggeredBy     string         `json:"triggered_by,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       time.Time      `json:"started_at,omitempty"`
	FinishedAt      time.Time      `json:"finished_at,omitempty"`
}

type OpsManualRunRecordSink interface {
	RecordRun(ctx context.Context, record OpsManualRunRecord) error
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
	Graph             *state.GraphRunState           `json:"graph,omitempty"`
	Resources         map[string]state.ResourceState `json:"resources,omitempty"`
}

type RunRequest struct {
	WorkflowName         string         `json:"workflow_name"`
	WorkflowYAML         string         `json:"workflow_yaml"`
	ManualID             string         `json:"manual_id,omitempty"`
	WorkflowID           string         `json:"workflow_id,omitempty"`
	WorkflowVersion      string         `json:"workflow_version,omitempty"`
	WorkflowDigest       string         `json:"workflow_digest,omitempty"`
	PreflightStatus      string         `json:"preflight_status,omitempty"`
	PreflightEvidenceRef string         `json:"preflight_evidence_ref,omitempty"`
	Vars                 map[string]any `json:"vars"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	TriggeredBy          string         `json:"triggered_by"`
	IdempotencyKey       string         `json:"idempotency_key"`
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
