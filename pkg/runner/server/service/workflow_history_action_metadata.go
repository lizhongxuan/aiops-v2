package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	"runner/workflow/edit"
	"runner/workflowstore"
)

const workflowHistoryActionMetadataDirectory = "history-actions"

// workflowHistoryActionMetadata is a server-owned bridge between an
// Undo/Redo Preview and its eventual Apply. It is immutable and bound to the
// exact persisted candidate identity; the committed History envelope remains
// the final source of truth.
type workflowHistoryActionMetadata struct {
	SchemaVersion        string                    `json:"schemaVersion"`
	ActionKind           WorkflowHistoryActionKind `json:"actionKind"`
	WorkflowKey          edit.WorkflowKey          `json:"workflowKey"`
	CandidateID          string                    `json:"candidateId"`
	CandidateRevision    uint64                    `json:"candidateRevision"`
	ChangeSetID          string                    `json:"changeSetId"`
	PreviewHash          string                    `json:"previewHash"`
	BaseRevision         edit.Revision             `json:"baseRevision"`
	TargetTransactionIDs []string                  `json:"targetTransactionIds"`
}

func newWorkflowHistoryActionMetadata(result WorkflowEditPreviewResult, action WorkflowHistoryActionKind, targets []string) workflowHistoryActionMetadata {
	return workflowHistoryActionMetadata{
		SchemaVersion: edit.SchemaVersion, ActionKind: action,
		WorkflowKey: result.ChangeSet.Key, CandidateID: result.Candidate.ID,
		CandidateRevision: result.CandidateRevision, ChangeSetID: result.ChangeSet.ID,
		PreviewHash: result.PreviewHash, BaseRevision: result.ChangeSet.Base,
		TargetTransactionIDs: append([]string(nil), targets...),
	}
}

func (a *WorkflowEditApplication) persistWorkflowHistoryActionMetadata(ctx context.Context, metadata workflowHistoryActionMetadata) error {
	if a == nil || a.commits == nil {
		return ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowHistoryActionMetadata(metadata); err != nil {
		return err
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return a.commits.withWorkflowCommitLock(ctx, metadata.WorkflowKey, func(namespace *workflowstore.WorkflowNamespace) error {
		path := workflowHistoryActionMetadataPath(metadata.ChangeSetID)
		absolute, err := namespace.Path(path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(absolute)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("invalid workflow history action metadata")
			}
			storedRaw, readErr := os.ReadFile(absolute)
			if readErr != nil {
				return readErr
			}
			var stored workflowHistoryActionMetadata
			if edit.DecodeStrict(storedRaw, &stored) != nil || validateWorkflowHistoryActionMetadata(stored) != nil {
				return fmt.Errorf("invalid workflow history action metadata")
			}
			if !reflect.DeepEqual(stored, metadata) {
				return fmt.Errorf("%w: workflow history action metadata", ErrConflict)
			}
			return nil
		case !errors.Is(err, os.ErrNotExist):
			return err
		default:
			return namespace.AtomicWriteFile(path, raw, 0o600)
		}
	})
}

func (a *WorkflowEditApplication) loadWorkflowHistoryActionMetadata(ctx context.Context, key edit.WorkflowKey, changeSetID string) (workflowHistoryActionMetadata, bool, error) {
	if a == nil || a.commits == nil {
		return workflowHistoryActionMetadata{}, false, ErrWorkflowEditConfiguration
	}
	if key.Validate() != nil || !validWorkflowApprovalValue(changeSetID, true) {
		return workflowHistoryActionMetadata{}, false, fmt.Errorf("%w: invalid workflow history action lookup", ErrInvalid)
	}
	var result workflowHistoryActionMetadata
	found := false
	err := a.commits.withWorkflowCommitLock(ctx, key, func(namespace *workflowstore.WorkflowNamespace) error {
		absolute, err := namespace.Path(workflowHistoryActionMetadataPath(changeSetID))
		if err != nil {
			return err
		}
		info, err := os.Lstat(absolute)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid workflow history action metadata")
		}
		raw, err := os.ReadFile(absolute)
		if err != nil {
			return err
		}
		if edit.DecodeStrict(raw, &result) != nil || validateWorkflowHistoryActionMetadata(result) != nil || result.WorkflowKey != key || result.ChangeSetID != changeSetID {
			return fmt.Errorf("invalid workflow history action metadata")
		}
		found = true
		return nil
	})
	return result, found, err
}

func (a *WorkflowEditApplication) resolveWorkflowHistoryActionMetadata(ctx context.Context, candidate WorkflowCandidateRevision) (workflowHistoryActionMetadata, error) {
	metadata, found, err := a.loadWorkflowHistoryActionMetadata(ctx, candidate.Key, candidate.ID)
	if err != nil {
		return workflowHistoryActionMetadata{}, err
	}
	if !found {
		// Generic Preview has no privileged sidecar. It is explicitly committed
		// as a normal action; text in candidate.Intent is never consulted.
		return workflowHistoryActionMetadata{
			SchemaVersion: edit.SchemaVersion, ActionKind: WorkflowHistoryActionNormal,
			WorkflowKey: candidate.Key, CandidateID: candidate.CandidateID,
			CandidateRevision: candidate.CandidateRevision, ChangeSetID: candidate.ID,
			PreviewHash: candidate.PreviewHash, BaseRevision: candidate.Base,
		}, nil
	}
	if metadata.WorkflowKey != candidate.Key || metadata.CandidateID != candidate.CandidateID ||
		metadata.CandidateRevision != candidate.CandidateRevision || metadata.ChangeSetID != candidate.ID ||
		metadata.PreviewHash != candidate.PreviewHash || metadata.BaseRevision != candidate.Base {
		return workflowHistoryActionMetadata{}, workflowPreviewDomainError(edit.ErrorPreviewMismatch, candidate.Key, candidate.PreviewHash, metadata.PreviewHash, "workflow history action does not match the persisted candidate", "create and review a new history candidate")
	}
	records, err := a.workflowEditHistoryRecords(ctx, candidate.Key)
	if err != nil {
		return workflowHistoryActionMetadata{}, err
	}
	byTransaction := make(map[string]workflowEditHistoryRecord, len(records))
	for _, record := range records {
		byTransaction[record.entry.TransactionID] = record
	}
	expected := make([]edit.WorkflowOperation, 0)
	for _, transactionID := range metadata.TargetTransactionIDs {
		target, exists := byTransaction[transactionID]
		if !exists || !target.entry.Reversible {
			return workflowHistoryActionMetadata{}, workflowPreviewDomainError(edit.ErrorStaleChangeSet, candidate.Key, candidate.Base.ResourceVersion, "", "workflow history action target is no longer authoritative", "reload durable history and create a new candidate")
		}
		if metadata.ActionKind == WorkflowHistoryActionUndo && target.actionValid && target.entry.ActionKind == WorkflowHistoryActionUndo {
			return workflowHistoryActionMetadata{}, workflowPreviewDomainError(edit.ErrorInvalidPatch, candidate.Key, candidate.Base.ResourceVersion, "", "an undo transaction must be replayed with redo", "choose redo for the durable undo transaction")
		}
		if metadata.ActionKind == WorkflowHistoryActionRedo && (!target.actionValid || target.entry.ActionKind != WorkflowHistoryActionUndo) {
			return workflowHistoryActionMetadata{}, workflowPreviewDomainError(edit.ErrorInvalidPatch, candidate.Key, candidate.Base.ResourceVersion, "", "redo requires a durable undo transaction", "reload durable history and choose an authoritative undo transaction")
		}
		expected = append(expected, cloneWorkflowOperations(target.inverse.Operations)...)
	}
	normalized, err := normalizeWorkflowPreviewOperations(expected)
	if err != nil || !workflowHistoryOperationsEqual(normalized, candidate.NormalizedOperations) {
		return workflowHistoryActionMetadata{}, workflowPreviewDomainError(edit.ErrorPreviewMismatch, candidate.Key, candidate.PreviewHash, "", "workflow history action operations do not match its durable targets", "create and review a new history candidate")
	}
	return metadata, nil
}

func validateWorkflowHistoryActionMetadata(metadata workflowHistoryActionMetadata) error {
	if metadata.SchemaVersion != edit.SchemaVersion || metadata.WorkflowKey.Validate() != nil ||
		(metadata.ActionKind != WorkflowHistoryActionUndo && metadata.ActionKind != WorkflowHistoryActionRedo) ||
		metadata.CandidateRevision == 0 || metadata.BaseRevision.ResourceVersion == "" || len(metadata.TargetTransactionIDs) == 0 {
		return fmt.Errorf("%w: invalid workflow history action metadata", ErrInvalid)
	}
	for _, value := range []string{metadata.CandidateID, metadata.ChangeSetID, metadata.PreviewHash} {
		if !validWorkflowApprovalValue(value, true) {
			return fmt.Errorf("%w: invalid workflow history action metadata", ErrInvalid)
		}
	}
	seen := make(map[string]struct{}, len(metadata.TargetTransactionIDs))
	for _, transactionID := range metadata.TargetTransactionIDs {
		if !validWorkflowApprovalValue(transactionID, true) {
			return fmt.Errorf("%w: invalid workflow history action target", ErrInvalid)
		}
		if _, exists := seen[transactionID]; exists {
			return fmt.Errorf("%w: duplicate workflow history action target", ErrInvalid)
		}
		seen[transactionID] = struct{}{}
	}
	return nil
}

func workflowHistoryActionMetadataPath(changeSetID string) string {
	digest := sha256.Sum256([]byte(changeSetID))
	return workflowHistoryActionMetadataDirectory + "/" + hex.EncodeToString(digest[:]) + ".json"
}
