package service

import (
	"context"
	"fmt"
	"strings"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

// WorkflowLegacyPatchRequest is the narrow compatibility boundary for the
// retired workflow-ai HTTP transport. Identity is supplied by trusted server
// scope, while Operations have already been converted to the canonical v2
// contract. It deliberately has no raw graph or arbitrary map field.
type WorkflowLegacyPatchRequest struct {
	SchemaVersion    string                   `json:"schemaVersion"`
	WorkflowKey      edit.WorkflowKey         `json:"workflowKey"`
	ExpectedRevision string                   `json:"expectedRevision"`
	IdempotencyKey   string                   `json:"idempotencyKey"`
	ActorID          string                   `json:"actorId"`
	CanvasSessionID  string                   `json:"canvasSessionId"`
	SessionID        string                   `json:"sessionId"`
	Reason           string                   `json:"reason"`
	Confirmed        bool                     `json:"confirmed,omitempty"`
	Operations       []edit.WorkflowOperation `json:"operations"`
}

type WorkflowLegacySnapshotResult struct {
	Snapshot   WorkflowEditSnapshot         `json:"snapshot"`
	Validation WorkflowEditValidationResult `json:"validation"`
}

type WorkflowLegacyPatchValidationResult struct {
	Graph      visual.Graph                 `json:"graph"`
	Validation WorkflowEditValidationResult `json:"validation"`
}

func (a *WorkflowEditApplication) ReadLegacySnapshot(ctx context.Context, key edit.WorkflowKey) (WorkflowLegacySnapshotResult, error) {
	if a == nil || a.backend == nil {
		return WorkflowLegacySnapshotResult{}, ErrWorkflowEditConfiguration
	}
	snapshot, err := a.readSnapshot(ctx, key)
	if err != nil {
		return WorkflowLegacySnapshotResult{}, err
	}
	validation, err := a.validateSnapshot(ctx, snapshot, snapshot.Graph, snapshot.Revision.ResourceVersion)
	if err != nil {
		return WorkflowLegacySnapshotResult{}, err
	}
	return WorkflowLegacySnapshotResult{Snapshot: snapshot, Validation: validation}, nil
}

func (a *WorkflowEditApplication) ValidateLegacyPatch(ctx context.Context, request WorkflowLegacyPatchRequest) (WorkflowLegacyPatchValidationResult, error) {
	batch := legacyWorkflowCommandBatch(request)
	if err := validateWorkflowCommandBatch(batch); err != nil {
		return WorkflowLegacyPatchValidationResult{}, err
	}
	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowLegacyPatchValidationResult{}, err
	}
	if snapshot.Revision.ResourceVersion != request.ExpectedRevision {
		return WorkflowLegacyPatchValidationResult{}, workflowPreviewConflict(request.WorkflowKey, edit.Revision{ResourceVersion: request.ExpectedRevision}, snapshot.Revision, []string{"/resourceVersion"}, nil)
	}
	applied, err := applyWorkflowPreviewOperations(snapshot.Graph, request.Operations)
	if err != nil {
		return WorkflowLegacyPatchValidationResult{}, workflowPreviewDomainError(edit.ErrorInvalidPatch, request.WorkflowKey, snapshot.Revision.ResourceVersion, request.ExpectedRevision, "legacy workflow patch is invalid", "repair the typed patch operations")
	}
	validation, err := a.validateSnapshot(ctx, snapshot, applied.Graph, request.ExpectedRevision)
	if err != nil {
		return WorkflowLegacyPatchValidationResult{}, err
	}
	return WorkflowLegacyPatchValidationResult{Graph: applied.Graph, Validation: validation}, nil
}

func (a *WorkflowEditApplication) PreviewLegacyPatch(ctx context.Context, request WorkflowLegacyPatchRequest) (WorkflowEditPreviewResult, error) {
	batch := legacyWorkflowCommandBatch(request)
	if err := validateWorkflowCommandBatch(batch); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	candidateID, err := workflowCommandBatchCandidateID(batch)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	return a.resolveOrCreateManualPreview(ctx, batch, candidateID)
}

func (a *WorkflowEditApplication) ApplyLegacyPatch(ctx context.Context, request WorkflowLegacyPatchRequest) (WorkflowManualCommandResult, error) {
	return a.executeWorkflowCommandBatch(ctx, legacyWorkflowCommandBatch(request))
}

func legacyWorkflowCommandBatch(request WorkflowLegacyPatchRequest) workflowCommandBatchRequest {
	return workflowCommandBatchRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: request.WorkflowKey, ExpectedRevision: request.ExpectedRevision,
		IdempotencyKey: request.IdempotencyKey, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID,
		SessionID: request.SessionID, Reason: request.Reason, Confirmed: request.Confirmed,
		Intent: "legacy workflow patch", Operations: request.Operations,
	}
}

func workflowCommandBatchCandidateID(request workflowCommandBatchRequest) (string, error) {
	identityHash, err := workflowQueryDigest([]any{edit.SchemaVersion, "manual", request.WorkflowKey, request.ActorID, request.CanvasSessionID, request.IdempotencyKey})
	if err != nil {
		return "", err
	}
	token := strings.TrimPrefix(identityHash, "sha256:")
	if len(token) < 32 {
		return "", fmt.Errorf("%w: invalid workflow command identity digest", ErrInvalid)
	}
	return "manual-" + token[:32], nil
}
