package service

import (
	"context"
	"fmt"
	"strings"

	"runner/workflow/edit"
)

// WorkflowCandidateAbandonRequest is the trusted boundary used to discard the
// exact Candidate revision currently shown by the editor. Identity fields are
// supplied by the authenticated HTTP scope, never by browser JSON.
type WorkflowCandidateAbandonRequest struct {
	SchemaVersion     string
	WorkflowKey       edit.WorkflowKey
	CandidateID       string
	CandidateRevision uint64
	ChangeSetID       string
	PreviewHash       string
	Expected          edit.Revision
	IdempotencyKey    string
	ActorID           string
	CanvasSessionID   string
	SessionID         string
}

type WorkflowCandidateAbandonResult struct {
	Kind          string                 `json:"kind"`
	SchemaVersion string                 `json:"schemaVersion"`
	Candidate     edit.WorkflowCandidate `json:"candidate"`
}

// AbandonCandidate marks the exact current Candidate as abandoned without
// mutating the committed graph. Replaying the same exact binding is safe.
func (a *WorkflowEditApplication) AbandonCandidate(ctx context.Context, request WorkflowCandidateAbandonRequest) (WorkflowCandidateAbandonResult, error) {
	if a == nil || a.backend == nil || a.candidates == nil {
		return WorkflowCandidateAbandonResult{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowCandidateAbandonRequest(request); err != nil {
		return WorkflowCandidateAbandonResult{}, err
	}
	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowCandidateAbandonResult{}, err
	}
	if snapshot.Revision != request.Expected {
		return WorkflowCandidateAbandonResult{}, workflowPreviewConflict(request.WorkflowKey, request.Expected, snapshot.Revision, []string{"/resourceVersion"}, nil)
	}
	candidate, err := a.candidates.GetCurrent(ctx, request.WorkflowKey, request.CandidateID)
	if err != nil {
		return WorkflowCandidateAbandonResult{}, err
	}
	if candidate.CurrentRevision != request.CandidateRevision || candidate.CurrentChangeSetID != request.ChangeSetID || candidate.CurrentPreviewHash != request.PreviewHash {
		return WorkflowCandidateAbandonResult{}, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, candidate.CurrentChangeSetID, request.ChangeSetID, "candidate revision changed", "reload the current candidate")
	}
	revision, err := a.candidates.GetRevision(ctx, request.WorkflowKey, request.CandidateID, request.CandidateRevision)
	if err != nil {
		return WorkflowCandidateAbandonResult{}, err
	}
	if revision.ActorID != request.ActorID || revision.CanvasSessionID != request.CanvasSessionID || !workflowPreviewActorCanRevise(revision.Actor, edit.ActorKind("user")) {
		return WorkflowCandidateAbandonResult{}, workflowPreviewDomainError(edit.ErrorApprovalRequired, request.WorkflowKey, "", "", "candidate belongs to another editing scope", "reload the workflow or create a new candidate")
	}
	if candidate.State == WorkflowCandidateStateAbandoned {
		return WorkflowCandidateAbandonResult{Kind: "candidate_discarded", SchemaVersion: edit.SchemaVersion, Candidate: candidate}, nil
	}
	if candidate.State != WorkflowCandidateStateActive {
		return WorkflowCandidateAbandonResult{}, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, candidate.CurrentChangeSetID, request.ChangeSetID, "candidate is no longer active", "reload the current candidate")
	}
	candidate, err = a.candidates.Abandon(ctx, request.WorkflowKey, request.CandidateID, request.CandidateRevision, a.now().UTC())
	if err != nil {
		return WorkflowCandidateAbandonResult{}, err
	}
	return WorkflowCandidateAbandonResult{Kind: "candidate_discarded", SchemaVersion: edit.SchemaVersion, Candidate: candidate}, nil
}

func validateWorkflowCandidateAbandonRequest(request WorkflowCandidateAbandonRequest) error {
	valid := func(value string) bool {
		return value != "" && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00')
	}
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || validateWorkflowCandidateID(request.CandidateID) != nil || request.CandidateRevision == 0 ||
		!valid(request.ChangeSetID) || !valid(request.PreviewHash) || !valid(request.Expected.ResourceVersion) || !valid(request.Expected.SemanticHash) || !valid(request.Expected.LayoutHash) ||
		!validWorkflowApprovalValue(request.IdempotencyKey, true) || !validWorkflowApprovalValue(request.ActorID, true) || !validWorkflowApprovalValue(request.CanvasSessionID, true) || !validWorkflowApprovalValue(request.SessionID, true) {
		return fmt.Errorf("%w: invalid trusted candidate abandon request", ErrInvalid)
	}
	return nil
}
