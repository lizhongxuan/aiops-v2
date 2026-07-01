package opsmanual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const RedactedValue = "[REDACTED]"

type WorkflowResult struct {
	ID                  string             `json:"id,omitempty"`
	SessionID           string             `json:"session_id,omitempty"`
	OpsManualFlowID     string             `json:"ops_manual_flow_id,omitempty"`
	ManualID            string             `json:"manual_id,omitempty"`
	WorkflowID          string             `json:"workflow_id"`
	WorkflowVersion     string             `json:"workflow_version,omitempty"`
	WorkflowDigest      string             `json:"workflow_digest,omitempty"`
	WorkflowYAML        string             `json:"workflow_yaml,omitempty"`
	OperationFrame      OperationFrame     `json:"operation_frame"`
	EnvironmentSnapshot EnvironmentProfile `json:"environment_snapshot"`
	Parameters          map[string]any     `json:"parameters,omitempty"`
	ApprovalRef         string             `json:"approval_ref,omitempty"`
	PreflightStatus     string             `json:"preflight_status,omitempty"`
	DryRunStatus        string             `json:"dry_run_status,omitempty"`
	ExecutionStatus     string             `json:"execution_status,omitempty"`
	ValidationStatus    string             `json:"validation_status,omitempty"`
	RollbackStatus      string             `json:"rollback_status,omitempty"`
	FailureReason       string             `json:"failure_reason,omitempty"`
	UserFeedback        string             `json:"user_feedback,omitempty"`
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
		SessionID:           strings.TrimSpace(result.SessionID),
		OpsManualFlowID:     strings.TrimSpace(result.OpsManualFlowID),
		ManualID:            strings.TrimSpace(result.ManualID),
		WorkflowID:          workflowID,
		WorkflowVersion:     strings.TrimSpace(result.WorkflowVersion),
		WorkflowDigest:      digest,
		OperationFrame:      result.OperationFrame,
		EnvironmentSnapshot: result.EnvironmentSnapshot,
		RedactedParameters:  RedactParameters(result.Parameters),
		ApprovalRef:         strings.TrimSpace(result.ApprovalRef),
		PreflightStatus:     strings.TrimSpace(result.PreflightStatus),
		DryRunStatus:        strings.TrimSpace(result.DryRunStatus),
		ExecutionStatus:     strings.TrimSpace(result.ExecutionStatus),
		ValidationStatus:    strings.TrimSpace(result.ValidationStatus),
		RollbackStatus:      strings.TrimSpace(result.RollbackStatus),
		FailureReason:       strings.TrimSpace(result.FailureReason),
		UserFeedback:        strings.TrimSpace(result.UserFeedback),
		Operator:            strings.TrimSpace(result.Operator),
		StartedAt:           strings.TrimSpace(result.StartedAt),
		CompletedAt:         strings.TrimSpace(result.CompletedAt),
	}, nil
}

func SummarizeRunRecords(records []RunRecord) RunRecordSummary {
	summary := RunRecordSummary{}
	for _, record := range records {
		if runRecordSkipped(record) {
			summary.SkippedCount++
		}
		if runRecordPassed(record) {
			summary.SuccessCount++
		}
		if runRecordFailed(record) {
			summary.FailureCount++
		}
		when := runRecordTime(record)
		if when > summary.LastRunAt {
			summary.LastRunAt = when
			summary.LatestStatus = recentRunResult(record)
			summary.RecentResult = summary.LatestStatus
		}
	}
	summary.UsedCount = summary.SuccessCount + summary.FailureCount
	sorted := append([]RunRecord{}, records...)
	sortRunRecordsByTime(sorted)
	for _, record := range sorted {
		switch {
		case runRecordFailed(record):
			summary.ConsecutiveFailures++
		case runRecordPassed(record):
			goto done
		default:
			goto done
		}
	}
done:
	if summary.LatestStatus == "" {
		summary.LatestStatus = summary.RecentResult
	}
	if latestRunFailed(summary) {
		summary.Suppressed = true
		summary.SuppressedReason = "latest run record did not pass validation"
	}
	if summary.ConsecutiveFailures >= 2 {
		summary.Suppressed = true
		summary.SuppressedReason = fmt.Sprintf("consecutive failures: %d", summary.ConsecutiveFailures)
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
		return normalizeRunRecordStatus(record.ValidationStatus)
	case record.ExecutionStatus != "":
		return normalizeRunRecordStatus(record.ExecutionStatus)
	case record.DryRunStatus != "":
		return normalizeRunRecordStatus(record.DryRunStatus)
	default:
		return ""
	}
}

func runRecordPassed(record RunRecord) bool {
	return normalizeRunRecordStatus(record.ValidationStatus) == "passed" ||
		(record.ValidationStatus == "" && normalizeRunRecordStatus(record.ExecutionStatus) == "passed")
}

func runRecordFailed(record RunRecord) bool {
	return normalizeRunRecordStatus(record.ValidationStatus) == "failed" ||
		normalizeRunRecordStatus(record.ExecutionStatus) == "failed"
}

func runRecordSkipped(record RunRecord) bool {
	return normalizeRunRecordStatus(record.ValidationStatus) == "skipped" ||
		normalizeRunRecordStatus(record.ExecutionStatus) == "skipped" ||
		normalizeRunRecordStatus(record.DryRunStatus) == "skipped"
}

func normalizeRunRecordStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "passed", "pass", "success", "succeeded", "ok":
		return "passed"
	case "failed", "fail", "error", "errored":
		return "failed"
	case "skipped", "skip", "declined", "not_used", "not-used", "user_skipped", "user-skipped":
		return "skipped"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func sortRunRecordsByTime(records []RunRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left := runRecordTime(records[i])
		right := runRecordTime(records[j])
		if left == right {
			return records[i].ID < records[j].ID
		}
		return left > right
	})
}
