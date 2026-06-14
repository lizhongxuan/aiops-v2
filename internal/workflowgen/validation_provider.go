package workflowgen

import (
	"context"
	"time"

	"runner/workflow/visual"
)

type ValidationRequest struct {
	SessionID     string            `json:"session_id,omitempty"`
	Graph         visual.Graph      `json:"graph"`
	Scenario      string            `json:"scenario,omitempty"`
	MockInputs    map[string]any    `json:"mock_inputs,omitempty"`
	AllowedImages []string          `json:"allowed_images,omitempty"`
	EnvAllowlist  []string          `json:"env_allowlist,omitempty"`
	Timeout       time.Duration     `json:"timeout,omitempty"`
	NetworkPolicy string            `json:"network_policy,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type ValidationResult struct {
	ID            string                  `json:"id"`
	Provider      ValidationProvider      `json:"provider"`
	Status        string                  `json:"status"`
	Scenario      string                  `json:"scenario,omitempty"`
	Summary       string                  `json:"summary,omitempty"`
	FailureNodeID string                  `json:"failure_node_id,omitempty"`
	Image         string                  `json:"image,omitempty"`
	ExitCode      int                     `json:"exit_code,omitempty"`
	StdoutSummary string                  `json:"stdout_summary,omitempty"`
	StderrSummary string                  `json:"stderr_summary,omitempty"`
	NodeResults   []NodeValidationSummary `json:"node_results,omitempty"`
	StartedAt     time.Time               `json:"started_at,omitempty"`
	EndedAt       time.Time               `json:"ended_at,omitempty"`
	DurationMs    int64                   `json:"duration_ms,omitempty"`
	SkippedReason string                  `json:"skipped_reason,omitempty"`
}

type NodeValidationSummary struct {
	NodeID        string `json:"node_id"`
	Action        string `json:"action,omitempty"`
	Status        string `json:"status"`
	Summary       string `json:"summary,omitempty"`
	ExitCode      int    `json:"exit_code,omitempty"`
	StdoutSummary string `json:"stdout_summary,omitempty"`
	StderrSummary string `json:"stderr_summary,omitempty"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

type WorkflowValidationProvider interface {
	Name() ValidationProvider
	Validate(ctx context.Context, req ValidationRequest) (*ValidationResult, error)
}

type StaticValidationProvider struct{}

func (p StaticValidationProvider) Name() ValidationProvider {
	return ValidationProviderNone
}

func (p StaticValidationProvider) Validate(_ context.Context, req ValidationRequest) (*ValidationResult, error) {
	started := time.Now().UTC()
	if err := visual.ValidateGraph(req.Graph); err != nil {
		ended := time.Now().UTC()
		return &ValidationResult{
			ID:            "static-" + started.Format("20060102150405.000000000"),
			Provider:      ValidationProviderNone,
			Status:        "failed",
			Scenario:      req.Scenario,
			Summary:       err.Error(),
			StartedAt:     started,
			EndedAt:       ended,
			DurationMs:    ended.Sub(started).Milliseconds(),
			SkippedReason: "",
		}, nil
	}
	ended := time.Now().UTC()
	return &ValidationResult{
		ID:         "static-" + started.Format("20060102150405.000000000"),
		Provider:   ValidationProviderNone,
		Status:     "passed",
		Scenario:   req.Scenario,
		Summary:    "Runner graph static validation passed.",
		StartedAt:  started,
		EndedAt:    ended,
		DurationMs: ended.Sub(started).Milliseconds(),
	}, nil
}
