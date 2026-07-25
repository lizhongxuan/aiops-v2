package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflowstore"
)

const (
	workflowEditAuditKind = "workflow.edit.audit"
	workflowAuditRedacted = "[REDACTED]"
	workflowAuditOmitted  = "[OMITTED]"
)

type WorkflowEditAuditInput struct {
	AuditID, Channel, Operation, UserID, SessionID, TurnID, ToolCallID, ChangeSetID, TransactionID string
	Key                                                                                            edit.WorkflowKey
	Actor                                                                                          edit.ActorKind
	BeforeRevision                                                                                 edit.Revision
	AfterRevision                                                                                  edit.Revision
	Details                                                                                        map[string]any
	OccurredAt                                                                                     time.Time
}

type WorkflowEditAuditRecord struct {
	Kind           string           `json:"kind"`
	SchemaVersion  string           `json:"schemaVersion"`
	AuditID        string           `json:"auditId"`
	Key            edit.WorkflowKey `json:"workflowKey"`
	Actor          edit.ActorKind   `json:"actor"`
	Channel        string           `json:"channel"`
	Operation      string           `json:"operation"`
	UserID         string           `json:"userId"`
	SessionID      string           `json:"sessionId"`
	TurnID         string           `json:"turnId,omitempty"`
	ToolCallID     string           `json:"toolCallId,omitempty"`
	ChangeSetID    string           `json:"changeSetId"`
	TransactionID  string           `json:"transactionId"`
	BeforeRevision edit.Revision    `json:"beforeRevision"`
	AfterRevision  edit.Revision    `json:"afterRevision"`
	Details        map[string]any   `json:"details,omitempty"`
	OccurredAt     time.Time        `json:"occurredAt"`
}

func BuildWorkflowEditAudit(input WorkflowEditAuditInput) (WorkflowEditAuditRecord, error) {
	if input.Key.Validate() != nil || input.OccurredAt.IsZero() ||
		!workflowAuditRequired(string(input.Actor), input.Channel, input.Operation, input.UserID, input.SessionID, input.ChangeSetID, input.TransactionID,
			input.BeforeRevision.ResourceVersion, input.AfterRevision.ResourceVersion) {
		return WorkflowEditAuditRecord{}, fmt.Errorf("%w: invalid workflow edit audit correlation", ErrInvalid)
	}
	auditID := strings.TrimSpace(input.AuditID)
	if auditID == "" {
		digest, err := workflowQueryDigest([]any{input.Key, input.TransactionID, input.ChangeSetID, input.AfterRevision})
		if err != nil {
			return WorkflowEditAuditRecord{}, err
		}
		auditID = "audit-" + strings.TrimPrefix(digest, "sha256:")[:32]
	}
	if !workflowAuditRequired(auditID) {
		return WorkflowEditAuditRecord{}, fmt.Errorf("%w: invalid workflow edit audit id", ErrInvalid)
	}
	details, err := redactWorkflowAuditDetails(input.Details)
	if err != nil {
		return WorkflowEditAuditRecord{}, fmt.Errorf("%w: invalid workflow edit audit details", ErrInvalid)
	}
	return WorkflowEditAuditRecord{
		Kind: workflowEditAuditKind, SchemaVersion: edit.SchemaVersion, AuditID: auditID, Key: input.Key,
		Actor: input.Actor, Channel: input.Channel, Operation: input.Operation, UserID: input.UserID, SessionID: input.SessionID,
		TurnID: input.TurnID, ToolCallID: input.ToolCallID, ChangeSetID: input.ChangeSetID, TransactionID: input.TransactionID,
		BeforeRevision: input.BeforeRevision, AfterRevision: input.AfterRevision, Details: details, OccurredAt: input.OccurredAt.UTC(),
	}, nil
}

func BuildWorkflowEditAuditEnvelope(input WorkflowEditAuditInput) (workflowstore.TypedEnvelope, error) {
	record, err := BuildWorkflowEditAudit(input)
	if err != nil {
		return workflowstore.TypedEnvelope{}, err
	}
	payload, err := json.Marshal(record)
	return workflowstore.TypedEnvelope{Kind: workflowEditAuditKind, SchemaVersion: edit.SchemaVersion, Payload: payload}, err
}

func workflowAuditRequired(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func redactWorkflowAuditDetails(details map[string]any) (map[string]any, error) {
	if len(details) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return redactWorkflowAuditMap(cloned), nil
}

func redactWorkflowAuditMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
		switch {
		case workflowAuditRawInputField(normalized):
			result[key] = workflowAuditOmitted
		case workflowAuditSensitiveField(normalized):
			result[key] = workflowAuditRedacted
		default:
			result[key] = redactWorkflowAuditValue(item)
		}
	}
	return result
}

func workflowAuditSensitiveField(field string) bool {
	if field == "secretref" {
		return false
	}
	for _, marker := range []string{"secret", "credential", "token", "password", "passwd", "apikey", "accesskey", "privatekey", "authorization"} {
		if strings.Contains(field, marker) {
			return true
		}
	}
	return false
}

func redactWorkflowAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactWorkflowAuditMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactWorkflowAuditValue(item)
		}
		return result
	case string:
		lower := strings.ToLower(typed)
		if strings.Contains(lower, "bearer ") || strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "credential=") {
			return workflowAuditRedacted
		}
	}
	return value
}

func workflowAuditRawInputField(field string) bool {
	switch field {
	case "input", "request", "body", "content", "rawinput", "originalinput", "rawrequest", "requestbody", "prompt", "messages", "fullinput", "toolinput":
		return true
	default:
		return false
	}
}
