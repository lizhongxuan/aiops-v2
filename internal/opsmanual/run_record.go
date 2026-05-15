package opsmanual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const RedactedValue = "[REDACTED]"

type WorkflowResult struct {
	ID                  string             `json:"id,omitempty"`
	ManualID            string             `json:"manual_id,omitempty"`
	WorkflowID          string             `json:"workflow_id"`
	WorkflowVersion     string             `json:"workflow_version,omitempty"`
	WorkflowDigest      string             `json:"workflow_digest,omitempty"`
	WorkflowYAML        string             `json:"workflow_yaml,omitempty"`
	OperationFrame      OperationFrame     `json:"operation_frame"`
	EnvironmentSnapshot EnvironmentProfile `json:"environment_snapshot"`
	Parameters          map[string]any     `json:"parameters,omitempty"`
	ApprovalRef         string             `json:"approval_ref,omitempty"`
	DryRunStatus        string             `json:"dry_run_status,omitempty"`
	ExecutionStatus     string             `json:"execution_status,omitempty"`
	ValidationStatus    string             `json:"validation_status,omitempty"`
	RollbackStatus      string             `json:"rollback_status,omitempty"`
	FailureReason       string             `json:"failure_reason,omitempty"`
	Operator            string             `json:"operator,omitempty"`
	StartedAt           string             `json:"started_at,omitempty"`
	CompletedAt         string             `json:"completed_at,omitempty"`
}

func BuildRunRecordFromWorkflowResult(result WorkflowResult) (RunRecord, error) {
	workflowID := strings.TrimSpace(result.WorkflowID)
	if workflowID == "" {
		return RunRecord{}, fmt.Errorf("workflow_id is required")
	}
	digest := strings.TrimSpace(result.WorkflowDigest)
	if digest == "" && result.WorkflowYAML != "" {
		digest = DigestWorkflowYAML(result.WorkflowYAML)
	}
	id := strings.TrimSpace(result.ID)
	if id == "" {
		id = "run-record-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return RunRecord{
		ID:                  id,
		ManualID:            strings.TrimSpace(result.ManualID),
		WorkflowID:          workflowID,
		WorkflowVersion:     strings.TrimSpace(result.WorkflowVersion),
		WorkflowDigest:      digest,
		OperationFrame:      result.OperationFrame,
		EnvironmentSnapshot: result.EnvironmentSnapshot,
		RedactedParameters:  RedactParameters(result.Parameters),
		ApprovalRef:         strings.TrimSpace(result.ApprovalRef),
		DryRunStatus:        strings.TrimSpace(result.DryRunStatus),
		ExecutionStatus:     strings.TrimSpace(result.ExecutionStatus),
		ValidationStatus:    strings.TrimSpace(result.ValidationStatus),
		RollbackStatus:      strings.TrimSpace(result.RollbackStatus),
		FailureReason:       strings.TrimSpace(result.FailureReason),
		Operator:            strings.TrimSpace(result.Operator),
		StartedAt:           strings.TrimSpace(result.StartedAt),
		CompletedAt:         strings.TrimSpace(result.CompletedAt),
	}, nil
}

func SummarizeRunRecords(records []RunRecord) RunRecordSummary {
	summary := RunRecordSummary{}
	for _, record := range records {
		if record.ValidationStatus == "passed" {
			summary.SuccessCount++
		}
		if record.ExecutionStatus == "failed" || record.ValidationStatus == "failed" {
			summary.FailureCount++
		}
		when := runRecordTime(record)
		if when > summary.LastRunAt {
			summary.LastRunAt = when
			summary.RecentResult = recentRunResult(record)
		}
	}
	return summary
}

func DigestWorkflowYAML(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func RedactParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	out := make(map[string]any, len(parameters))
	for key, value := range parameters {
		if isSensitiveParameterKey(key) {
			out[key] = RedactedValue
			continue
		}
		out[key] = redactValue(value)
	}
	return out
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return RedactParameters(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if isSensitiveParameterKey(key) {
				out[key] = RedactedValue
			} else {
				out[key] = value
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactValue(item)
		}
		return out
	default:
		return value
	}
}

func isSensitiveParameterKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"password", "passwd", "token", "secret", "key", "credential"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func runRecordTime(record RunRecord) string {
	if record.CompletedAt != "" {
		return record.CompletedAt
	}
	return record.StartedAt
}

func recentRunResult(record RunRecord) string {
	switch {
	case record.ValidationStatus != "":
		return record.ValidationStatus
	case record.ExecutionStatus != "":
		return record.ExecutionStatus
	case record.DryRunStatus != "":
		return record.DryRunStatus
	default:
		return ""
	}
}
