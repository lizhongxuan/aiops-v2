package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"runner/workflow/edit"
)

type WorkflowEditObserveOperation string

const (
	WorkflowEditObservePreview   WorkflowEditObserveOperation = "preview"
	WorkflowEditObserveApply     WorkflowEditObserveOperation = "apply"
	WorkflowEditObserveValidate  WorkflowEditObserveOperation = "validate"
	WorkflowEditObservePostCheck WorkflowEditObserveOperation = "post_check"
	WorkflowEditObserveRecovery  WorkflowEditObserveOperation = "wal_recovery"
	WorkflowEditObserveEvent     WorkflowEditObserveOperation = "event_delivery"
	WorkflowEditObserveGap       WorkflowEditObserveOperation = "event_gap"
	WorkflowEditObserveAbandon   WorkflowEditObserveOperation = "candidate_abandonment"
)

type WorkflowEditObserveOutcome string

const (
	WorkflowEditOutcomeSuccess             WorkflowEditObserveOutcome = "success"
	WorkflowEditOutcomeFailure             WorkflowEditObserveOutcome = "failure"
	WorkflowEditOutcomeConflict            WorkflowEditObserveOutcome = "conflict"
	WorkflowEditOutcomeAuthorizationDenied WorkflowEditObserveOutcome = "authorization_denied"
	WorkflowEditOutcomeDuplicate           WorkflowEditObserveOutcome = "duplicate_transaction"
	WorkflowEditOutcomeInvalidCompleted    WorkflowEditObserveOutcome = "invalid_completed"
	WorkflowEditOutcomeGapUnrecoverable    WorkflowEditObserveOutcome = "event_gap_unrecoverable"
)

// WorkflowEditObservation contains bounded correlation facts only. Graphs,
// prompts, tool arguments, credentials, and other raw user input have no field
// in this contract and therefore cannot accidentally enter telemetry.
type WorkflowEditObservation struct {
	Operation         WorkflowEditObserveOperation `json:"operation"`
	Outcome           WorkflowEditObserveOutcome   `json:"outcome"`
	Key               edit.WorkflowKey             `json:"workflowKey"`
	ActorID           string                       `json:"actorId,omitempty"`
	SessionID         string                       `json:"sessionId,omitempty"`
	TurnID            string                       `json:"turnId,omitempty"`
	ToolCallID        string                       `json:"toolCallId,omitempty"`
	CandidateID       string                       `json:"candidateId,omitempty"`
	CandidateRevision uint64                       `json:"candidateRevision,omitempty"`
	ChangeSetID       string                       `json:"changeSetId,omitempty"`
	TransactionID     string                       `json:"transactionId,omitempty"`
	EventID           string                       `json:"eventId,omitempty"`
	RunID             string                       `json:"runId,omitempty"`
	ApprovalID        string                       `json:"approvalId,omitempty"`
	ErrorCode         edit.ErrorCode               `json:"errorCode,omitempty"`
	ReceiptReplay     bool                         `json:"receiptReplay,omitempty"`
	GapSize           uint64                       `json:"gapSize,omitempty"`
	EventLag          time.Duration                `json:"eventLag,omitempty"`
	StartedAt         time.Time                    `json:"startedAt"`
	Duration          time.Duration                `json:"duration"`
}

type WorkflowEditObserver interface {
	ObserveWorkflowEdit(context.Context, WorkflowEditObservation) error
}

func (a *WorkflowEditApplication) SetObserver(observer WorkflowEditObserver) {
	if a != nil {
		a.observer = observer
		if observed, ok := a.candidates.(interface{ SetObserver(WorkflowEditObserver) }); ok {
			observed.SetObserver(observer)
		}
	}
}

func (a *WorkflowEditApplication) observeWorkflowEdit(ctx context.Context, observation WorkflowEditObservation, operationErr error) {
	if a == nil || a.observer == nil {
		return
	}
	observation.Outcome, observation.ErrorCode = workflowEditObservationOutcome(operationErr, observation.Outcome)
	if observation.StartedAt.IsZero() {
		observation.StartedAt = time.Now().UTC()
	}
	if observation.Duration < 0 {
		observation.Duration = 0
	}
	defer func() { _ = recover() }()
	_ = a.observer.ObserveWorkflowEdit(ctx, observation)
}

func workflowEditObservationOutcome(err error, preferred WorkflowEditObserveOutcome) (WorkflowEditObserveOutcome, edit.ErrorCode) {
	if err == nil {
		if preferred != "" {
			return preferred, ""
		}
		return WorkflowEditOutcomeSuccess, ""
	}
	var domain *edit.DomainError
	if !errors.As(err, &domain) {
		if preferred != "" {
			return preferred, ""
		}
		return WorkflowEditOutcomeFailure, ""
	}
	if preferred != "" {
		return preferred, domain.Code
	}
	switch domain.Code {
	case edit.ErrorStaleRevision, edit.ErrorStaleChangeSet, edit.ErrorPreviewMismatch, edit.ErrorIdempotencyKeyReused, edit.ErrorReferenceLocked:
		return WorkflowEditOutcomeConflict, domain.Code
	case edit.ErrorApprovalRequired, edit.ErrorApprovalScopeExceeded:
		return WorkflowEditOutcomeAuthorizationDenied, domain.Code
	case edit.ErrorDuplicateTransaction:
		return WorkflowEditOutcomeDuplicate, domain.Code
	default:
		return WorkflowEditOutcomeFailure, domain.Code
	}
}

type WorkflowEditAuditQuery struct {
	Key           edit.WorkflowKey `json:"workflowKey"`
	TransactionID string           `json:"transactionId,omitempty"`
	ActorID       string           `json:"actorId,omitempty"`
	From          time.Time        `json:"from,omitempty"`
	To            time.Time        `json:"to,omitempty"`
}

type WorkflowEditAuditQueryResult struct {
	Kind          string                    `json:"kind"`
	SchemaVersion string                    `json:"schemaVersion"`
	Records       []WorkflowEditAuditRecord `json:"records"`
}

// QueryAudit reads the durable CommitStore ledger. It does not create a second
// audit database or trust caller-supplied Graph/tool payloads.
func (a *WorkflowEditApplication) QueryAudit(ctx context.Context, query WorkflowEditAuditQuery) (WorkflowEditAuditQueryResult, error) {
	if query.Key.Validate() != nil || (query.TransactionID != strings.TrimSpace(query.TransactionID)) || (query.ActorID != strings.TrimSpace(query.ActorID)) || (!query.From.IsZero() && !query.To.IsZero() && query.To.Before(query.From)) {
		return WorkflowEditAuditQueryResult{}, fmt.Errorf("%w: invalid workflow audit query", ErrInvalid)
	}
	records, err := a.workflowEditHistoryRecords(ctx, query.Key)
	if err != nil {
		return WorkflowEditAuditQueryResult{}, err
	}
	result := WorkflowEditAuditQueryResult{Kind: "workflow_audit", SchemaVersion: edit.SchemaVersion, Records: []WorkflowEditAuditRecord{}}
	for _, history := range records {
		var record WorkflowEditAuditRecord
		if history.snapshot.Audit.Kind != workflowEditAuditKind || edit.DecodeStrict(history.snapshot.Audit.Payload, &record) != nil || record.Key != query.Key || record.TransactionID != history.entry.TransactionID {
			continue
		}
		if query.TransactionID != "" && record.TransactionID != query.TransactionID {
			continue
		}
		if query.ActorID != "" && record.UserID != query.ActorID {
			continue
		}
		if !query.From.IsZero() && record.OccurredAt.Before(query.From) || !query.To.IsZero() && record.OccurredAt.After(query.To) {
			continue
		}
		result.Records = append(result.Records, record)
	}
	sort.Slice(result.Records, func(i, j int) bool { return result.Records[i].OccurredAt.After(result.Records[j].OccurredAt) })
	return result, nil
}

func workflowEditAuditDetails(risk WorkflowEditPreviewRisk, approvalID, reason, result string) map[string]any {
	details, err := redactWorkflowAuditDetails(map[string]any{
		"risk": risk.Level, "approvalId": approvalID, "reason": reason, "result": result,
	})
	if err != nil {
		return nil
	}
	return details
}

func decodeWorkflowPreviewRisk(raw json.RawMessage) WorkflowEditPreviewRisk {
	var risk WorkflowEditPreviewRisk
	_ = edit.DecodeStrict(raw, &risk)
	return risk
}
