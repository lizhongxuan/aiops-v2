package service

import (
	"context"
	"time"

	"runner/workflow/edit"
)

// GetCurrent exposes the application-owned Candidate read boundary to trusted
// in-process adapters. Callers never receive the underlying store and cannot
// use this method to create, revise, abandon, or apply a Candidate.
func (a *WorkflowEditApplication) GetCurrent(ctx context.Context, key edit.WorkflowKey, candidateID string) (edit.WorkflowCandidate, error) {
	if a == nil || a.candidates == nil {
		return edit.WorkflowCandidate{}, ErrWorkflowEditConfiguration
	}
	return a.candidates.GetCurrent(ctx, key, candidateID)
}

// GetRevision returns one immutable Candidate revision through the same
// application-owned persistence boundary used by Preview and Apply.
func (a *WorkflowEditApplication) GetRevision(ctx context.Context, key edit.WorkflowKey, candidateID string, revision uint64) (WorkflowCandidateRevision, error) {
	if a == nil || a.candidates == nil {
		return WorkflowCandidateRevision{}, ErrWorkflowEditConfiguration
	}
	return a.candidates.GetRevision(ctx, key, candidateID, revision)
}

// CheckApplyEligibility revalidates the exact persisted Candidate binding at
// the approval-request boundary. The later Apply path repeats this check under
// its transaction lock; this read is not an execution grant.
func (a *WorkflowEditApplication) CheckApplyEligibility(ctx context.Context, binding WorkflowCandidateApplyBinding, now time.Time) (WorkflowCandidateRevision, error) {
	if a == nil || a.candidates == nil {
		return WorkflowCandidateRevision{}, ErrWorkflowEditConfiguration
	}
	return a.candidates.CheckApplyEligibility(ctx, binding, now)
}
