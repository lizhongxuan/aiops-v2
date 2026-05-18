package opsmanual

import (
	"fmt"
	"strings"
)

type LegacyExperiencePack struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Summary         string            `json:"summary,omitempty"`
	Status          string            `json:"status,omitempty"`
	WorkflowID      string            `json:"workflow_id,omitempty"`
	WorkflowVersion string            `json:"workflow_version,omitempty"`
	WorkflowDigest  string            `json:"workflow_digest,omitempty"`
	WorkflowYAML    string            `json:"workflow_yaml,omitempty"`
	TargetType      string            `json:"target_type,omitempty"`
	Action          string            `json:"action,omitempty"`
	RequiredInputs  []string          `json:"required_inputs,omitempty"`
	Validation      []string          `json:"validation,omitempty"`
	CannotUseWhen   []string          `json:"cannot_use_when,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	RunRecords      []LegacyRunRecord `json:"run_records,omitempty"`
}

type LegacyRunRecord struct {
	ManualID         string         `json:"manual_id,omitempty"`
	WorkflowID       string         `json:"workflow_id,omitempty"`
	WorkflowVersion  string         `json:"workflow_version,omitempty"`
	WorkflowDigest   string         `json:"workflow_digest,omitempty"`
	Parameters       map[string]any `json:"parameters,omitempty"`
	DryRunStatus     string         `json:"dry_run_status,omitempty"`
	ExecutionStatus  string         `json:"execution_status,omitempty"`
	ValidationStatus string         `json:"validation_status,omitempty"`
	RollbackStatus   string         `json:"rollback_status,omitempty"`
	FailureReason    string         `json:"failure_reason,omitempty"`
	StartedAt        string         `json:"started_at,omitempty"`
	CompletedAt      string         `json:"completed_at,omitempty"`
}

type LegacyMigrationResult struct {
	Manual           OpsManual        `json:"manual"`
	RunRecords       []RunRecord      `json:"run_records,omitempty"`
	RunRecordSummary RunRecordSummary `json:"run_record_summary"`
}

func MigrateLegacyExperiencePack(pack LegacyExperiencePack) (LegacyMigrationResult, error) {
	id := strings.TrimSpace(pack.ID)
	if id == "" {
		return LegacyMigrationResult{}, fmt.Errorf("legacy experience pack id is required")
	}
	workflowID := strings.TrimSpace(pack.WorkflowID)
	if workflowID == "" {
		return LegacyMigrationResult{}, fmt.Errorf("legacy experience pack workflow_id is required")
	}
	digest := firstNonEmpty(pack.WorkflowDigest, digestIfRaw(pack.WorkflowYAML))
	manualID := "manual-" + slug(id)
	manual := OpsManual{
		ID:             manualID,
		ManualFamilyID: slug(id),
		Title:          firstNonEmpty(pack.Name, id),
		Status:         legacyManualStatus(pack.Status),
		Version:        pack.WorkflowVersion,
		WorkflowRef: WorkflowRef{
			WorkflowID:      workflowID,
			WorkflowVersion: strings.TrimSpace(pack.WorkflowVersion),
			WorkflowDigest:  digest,
		},
		Operation: OperationProfile{
			TargetType: strings.TrimSpace(pack.TargetType),
			Action:     strings.TrimSpace(pack.Action),
		},
		RequiredContext:  RequiredContext{RequiredInputs: cloneStrings(pack.RequiredInputs)},
		Validation:       cloneStrings(pack.Validation),
		CannotUseWhen:    cloneStrings(pack.CannotUseWhen),
		DocumentMarkdown: legacyManualMarkdown(pack),
		SearchDoc:        strings.TrimSpace(pack.Name + " " + pack.Summary),
		Metadata:         cloneMap(pack.Metadata),
	}
	records := make([]RunRecord, 0, len(pack.RunRecords))
	for _, legacy := range pack.RunRecords {
		record, err := BuildRunRecordFromWorkflowResult(WorkflowResult{
			ManualID:         firstNonEmpty(legacy.ManualID, manualID),
			WorkflowID:       firstNonEmpty(legacy.WorkflowID, workflowID),
			WorkflowVersion:  firstNonEmpty(legacy.WorkflowVersion, pack.WorkflowVersion),
			WorkflowDigest:   firstNonEmpty(legacy.WorkflowDigest, digest),
			Parameters:       legacy.Parameters,
			DryRunStatus:     legacy.DryRunStatus,
			ExecutionStatus:  legacy.ExecutionStatus,
			ValidationStatus: legacy.ValidationStatus,
			RollbackStatus:   legacy.RollbackStatus,
			FailureReason:    legacy.FailureReason,
			StartedAt:        legacy.StartedAt,
			CompletedAt:      legacy.CompletedAt,
		})
		if err != nil {
			return LegacyMigrationResult{}, err
		}
		records = append(records, record)
	}
	return LegacyMigrationResult{
		Manual:           manual,
		RunRecords:       records,
		RunRecordSummary: SummarizeRunRecords(records),
	}, nil
}

func legacyManualStatus(status string) ManualStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "verified", "published", "active":
		return ManualStatusVerified
	case "deprecated", "disabled":
		return ManualStatusDeprecated
	default:
		return ManualStatusDraft
	}
}

func legacyManualMarkdown(pack LegacyExperiencePack) string {
	title := firstNonEmpty(pack.Name, pack.ID)
	if strings.TrimSpace(pack.Summary) == "" {
		return "# " + title
	}
	return "# " + title + "\n\n" + strings.TrimSpace(pack.Summary)
}
