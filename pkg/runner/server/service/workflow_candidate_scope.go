package service

import (
	"context"
	"fmt"

	"runner/workflow/edit"
)

// WorkflowCandidateScope is a read-only projection used by trusted adapters to
// enforce a frozen turn selection. It intentionally excludes Graph bodies,
// grants, approvals, and mutable store internals.
type WorkflowCandidateScope struct {
	Key                  edit.WorkflowKey         `json:"workflowKey"`
	CandidateID          string                   `json:"candidateId"`
	CandidateRevision    uint64                   `json:"candidateRevision"`
	ChangeSetID          string                   `json:"changeSetId"`
	Base                 edit.Revision            `json:"base"`
	NormalizedOperations []edit.WorkflowOperation `json:"normalizedOperations"`
}

// ResolveCandidateScope reads the current immutable Candidate revision by
// change-set identity. Superseded, abandoned, expired, or cross-workflow
// revisions are rejected before their operations reach an adapter.
func (a *WorkflowEditApplication) ResolveCandidateScope(ctx context.Context, key edit.WorkflowKey, changeSetID string) (WorkflowCandidateScope, error) {
	if a == nil || a.candidates == nil || a.now == nil {
		return WorkflowCandidateScope{}, ErrWorkflowEditConfiguration
	}
	if key.Validate() != nil || !validWorkflowApprovalValue(changeSetID, false) {
		return WorkflowCandidateScope{}, fmt.Errorf("%w: invalid workflow candidate scope", ErrInvalid)
	}
	revision, err := a.candidates.ResolveChangeSet(ctx, key, changeSetID)
	if err != nil {
		return WorkflowCandidateScope{}, err
	}
	eligible, err := a.candidates.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: revision.CandidateID, CandidateRevision: revision.CandidateRevision,
		ChangeSetID: revision.ID, PreviewHash: revision.PreviewHash,
	}, a.now().UTC())
	if err != nil {
		return WorkflowCandidateScope{}, err
	}
	if eligible.Key != key || eligible.ID != changeSetID {
		return WorkflowCandidateScope{}, fmt.Errorf("%w: workflow candidate scope mismatch", ErrInvalid)
	}
	return WorkflowCandidateScope{
		Key: key, CandidateID: eligible.CandidateID, CandidateRevision: eligible.CandidateRevision,
		ChangeSetID: eligible.ID, Base: eligible.Base,
		NormalizedOperations: append([]edit.WorkflowOperation(nil), eligible.NormalizedOperations...),
	}, nil
}
