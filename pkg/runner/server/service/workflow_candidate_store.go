package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
)

const (
	WorkflowCandidateRevisionReady      edit.CandidateState = "ready"
	WorkflowCandidateRevisionSuperseded edit.CandidateState = "superseded"
	WorkflowCandidateStateActive                            = "active"
	WorkflowCandidateStateAbandoned                         = "abandoned"
)

type WorkflowCandidateRevision struct {
	edit.WorkflowChangeSet
	ParentRevision       uint64                   `json:"parentCandidateRevision,omitempty"`
	NormalizedOperations []edit.WorkflowOperation `json:"normalizedOperations"`
	Risk                 json.RawMessage          `json:"risk"`
	Diff                 json.RawMessage          `json:"diff"`
	ActorID              string                   `json:"actorId"`
	CanvasSessionID      string                   `json:"canvasSessionId"`
	AgentSessionID       string                   `json:"agentSessionId,omitempty"`
	TurnID               string                   `json:"turnId,omitempty"`
	ToolCallID           string                   `json:"toolCallId,omitempty"`
}

type WorkflowCandidateApplyBinding struct {
	Key               edit.WorkflowKey
	CandidateID       string
	CandidateRevision uint64
	ChangeSetID       string
	PreviewHash       string
}

type WorkflowCandidateStore interface {
	Create(context.Context, WorkflowCandidateRevision) (edit.WorkflowCandidate, error)
	Revise(context.Context, edit.WorkflowKey, string, uint64, WorkflowCandidateRevision) (edit.WorkflowCandidate, error)
	GetCurrent(context.Context, edit.WorkflowKey, string) (edit.WorkflowCandidate, error)
	GetRevision(context.Context, edit.WorkflowKey, string, uint64) (WorkflowCandidateRevision, error)
	ListRevisions(context.Context, edit.WorkflowKey, string) ([]WorkflowCandidateRevision, error)
	ResolveChangeSet(context.Context, edit.WorkflowKey, string) (WorkflowCandidateRevision, error)
	Abandon(context.Context, edit.WorkflowKey, string, uint64, time.Time) (edit.WorkflowCandidate, error)
	CheckApplyEligibility(context.Context, WorkflowCandidateApplyBinding, time.Time) (WorkflowCandidateRevision, error)
}

func validateWorkflowCandidateRevision(revision WorkflowCandidateRevision) error {
	if revision.SchemaVersion != edit.SchemaVersion {
		return fmt.Errorf("%w: candidate schemaVersion must be %s", ErrInvalid, edit.SchemaVersion)
	}
	if err := revision.Key.Validate(); err != nil {
		return candidateDomainError(edit.ErrorInvalidWorkflowKey, revision.Key, "", "", "invalid workflow key", "use the authenticated workflow scope")
	}
	if err := validateWorkflowCandidateID(revision.CandidateID); err != nil {
		return err
	}
	if revision.CandidateRevision == 0 || (revision.CandidateRevision == 1 && revision.ParentRevision != 0) || (revision.CandidateRevision > 1 && revision.ParentRevision+1 != revision.CandidateRevision) {
		return fmt.Errorf("%w: candidate revision/parent is invalid", ErrInvalid)
	}
	for name, value := range map[string]string{
		"changeSetId": revision.ID, "base.resourceVersion": revision.Base.ResourceVersion,
		"base.semanticHash": revision.Base.SemanticHash, "base.layoutHash": revision.Base.LayoutHash,
		"previewHash": revision.PreviewHash, "intent": revision.Intent, "actor": string(revision.Actor),
		"actorId": revision.ActorID, "canvasSessionId": revision.CanvasSessionID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalid, name)
		}
	}
	if revision.State != WorkflowCandidateRevisionReady {
		return fmt.Errorf("%w: new candidate revision must be ready", ErrInvalid)
	}
	if revision.CreatedAt.IsZero() || !revision.ExpiresAt.After(revision.CreatedAt) {
		return fmt.Errorf("%w: candidate expiry must be after creation", ErrInvalid)
	}
	if revision.Validation.Kind != "validation" {
		return fmt.Errorf("%w: validation kind is required", ErrInvalid)
	}
	if err := revision.Validation.Basis.Validate(); err != nil {
		return fmt.Errorf("%w: invalid validation basis", ErrInvalid)
	}
	if len(revision.Operations) == 0 || len(revision.NormalizedOperations) == 0 {
		return fmt.Errorf("%w: original and normalized operations are required", ErrInvalid)
	}
	for _, operations := range [][]edit.WorkflowOperation{revision.Operations, revision.NormalizedOperations} {
		for _, operation := range operations {
			if err := operation.Validate(); err != nil {
				return fmt.Errorf("%w: invalid candidate operation", ErrInvalid)
			}
		}
	}
	if len(revision.Risk) == 0 || !json.Valid(revision.Risk) || len(revision.Diff) == 0 || !json.Valid(revision.Diff) {
		return fmt.Errorf("%w: risk and diff must be valid JSON", ErrInvalid)
	}
	if err := validateWorkflowCandidateSecretRefs(revision); err != nil {
		return candidateDomainError(edit.ErrorSchemaViolation, revision.Key, "", "", "candidate contains an inline credential; use secretRef", "replace credential values with a server-managed secretRef")
	}
	return nil
}

func validateWorkflowCandidateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || id != trimmed || trimmed == "." || trimmed == ".." || strings.ContainsAny(trimmed, "/\\\x00") {
		return fmt.Errorf("%w: invalid candidate id", ErrInvalid)
	}
	return nil
}

func validateWorkflowCandidateSecretRefs(revision WorkflowCandidateRevision) error {
	raw, err := json.Marshal(revision)
	if err != nil {
		return fmt.Errorf("candidate serialization failed")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("candidate serialization failed")
	}
	return walkWorkflowCandidateSecrets(value)
}

func walkWorkflowCandidateSecrets(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if err := walkWorkflowCandidateSecrets(item); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range typed {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if normalized == "secretref" {
				if ref, ok := item.(string); !ok || strings.TrimSpace(ref) == "" {
					return fmt.Errorf("invalid secretRef")
				}
				continue
			}
			if isWorkflowCandidateSecretField(normalized) {
				return fmt.Errorf("inline credential")
			}
			if normalized == "credential" || normalized == "credentials" {
				object, ok := item.(map[string]any)
				if !ok || !workflowCandidateHasSecretRef(object) {
					return fmt.Errorf("credential must use secretRef")
				}
			}
			if err := walkWorkflowCandidateSecrets(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func workflowCandidateHasSecretRef(value map[string]any) bool {
	for key, item := range value {
		if strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key)) == "secretref" {
			ref, ok := item.(string)
			return ok && strings.TrimSpace(ref) != ""
		}
	}
	return false
}

func isWorkflowCandidateSecretField(field string) bool {
	switch field {
	case "password", "passwd", "secret", "token", "apikey", "accesskey", "privatekey", "clientsecret", "authorization", "bearertoken", "refreshtoken", "accesstoken":
		return true
	default:
		return false
	}
}

func candidateDomainError(code edit.ErrorCode, key edit.WorkflowKey, current, base, message, retry string) error {
	keyCopy := key
	return &edit.DomainError{Kind: "error", Code: code, Key: &keyCopy, CurrentRevision: current, BaseRevision: base, Message: message, RetryHint: retry}
}
