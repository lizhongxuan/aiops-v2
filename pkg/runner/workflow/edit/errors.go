package edit

import "encoding/json"

type ErrorCode string

const (
	ErrorInvalidWorkflowKey    ErrorCode = "INVALID_WORKFLOW_KEY"
	ErrorStaleRevision         ErrorCode = "STALE_REVISION"
	ErrorStaleChangeSet        ErrorCode = "STALE_CHANGESET"
	ErrorPreviewMismatch       ErrorCode = "PREVIEW_MISMATCH"
	ErrorInvalidPatch          ErrorCode = "INVALID_PATCH"
	ErrorUnknownAction         ErrorCode = "UNKNOWN_ACTION"
	ErrorSchemaViolation       ErrorCode = "SCHEMA_VIOLATION"
	ErrorInvalidEdge           ErrorCode = "INVALID_EDGE"
	ErrorWouldCreateCycle      ErrorCode = "WOULD_CREATE_CYCLE"
	ErrorReferenceLocked       ErrorCode = "REFERENCE_LOCKED"
	ErrorApprovalRequired      ErrorCode = "APPROVAL_REQUIRED"
	ErrorApprovalScopeExceeded ErrorCode = "APPROVAL_SCOPE_EXCEEDED"
	ErrorDuplicateTransaction  ErrorCode = "DUPLICATE_TRANSACTION"
	ErrorIdempotencyKeyReused  ErrorCode = "IDEMPOTENCY_KEY_REUSED"
	ErrorNoEffect              ErrorCode = "NO_EFFECT"
	ErrorMissingConfiguration  ErrorCode = "MISSING_CONFIGURATION"
	ErrorPublishGateFailed     ErrorCode = "PUBLISH_GATE_FAILED"
	ErrorValidationBasisStale  ErrorCode = "VALIDATION_BASIS_STALE"
)

type DomainError struct {
	Kind             string          `json:"kind" jsonschema:"enum=error"`
	Code             ErrorCode       `json:"errorCode" jsonschema:"enum=INVALID_WORKFLOW_KEY,enum=STALE_REVISION,enum=STALE_CHANGESET,enum=PREVIEW_MISMATCH,enum=INVALID_PATCH,enum=UNKNOWN_ACTION,enum=SCHEMA_VIOLATION,enum=INVALID_EDGE,enum=WOULD_CREATE_CYCLE,enum=REFERENCE_LOCKED,enum=APPROVAL_REQUIRED,enum=APPROVAL_SCOPE_EXCEEDED,enum=DUPLICATE_TRANSACTION,enum=IDEMPOTENCY_KEY_REUSED,enum=NO_EFFECT,enum=MISSING_CONFIGURATION,enum=PUBLISH_GATE_FAILED,enum=VALIDATION_BASIS_STALE"`
	Message          string          `json:"message"`
	Key              *WorkflowKey    `json:"workflowKey,omitempty"`
	CurrentRevision  string          `json:"currentRevision,omitempty"`
	BaseRevision     string          `json:"baseRevision,omitempty"`
	ConflictingPaths []string        `json:"conflictingPaths,omitempty"`
	ChangedBy        ActorKind       `json:"changedBy,omitempty"`
	RetryHint        string          `json:"retryHint,omitempty"`
	Details          json.RawMessage `json:"details,omitempty"`
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Code) + ": " + e.Message
}
