package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"runner/workflow/edit"
)

type WorkflowCandidateFileStore struct {
	Root     string
	observer WorkflowEditObserver
}

type workflowCandidateManifest struct {
	FormatVersion int                    `json:"formatVersion"`
	SchemaVersion string                 `json:"schemaVersion"`
	Candidate     edit.WorkflowCandidate `json:"candidate"`
	SupersededBy  map[string]uint64      `json:"supersededBy"`
	AbandonedAt   *time.Time             `json:"abandonedAt,omitempty"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

// workflowCandidateTransaction is the immutable candidate source of truth.
// The revision and its domain event are written as one atomic fact; current.json
// and the durable event stream are recoverable projections of these facts.
type workflowCandidateTransaction struct {
	FormatVersion int                       `json:"formatVersion"`
	SchemaVersion string                    `json:"schemaVersion"`
	Revision      WorkflowCandidateRevision `json:"revision"`
	Event         edit.WorkflowDomainEvent  `json:"event"`
}

var workflowCandidatePathLocks sync.Map

func NewWorkflowCandidateFileStore(root string) *WorkflowCandidateFileStore {
	return &WorkflowCandidateFileStore{Root: root}
}

func (s *WorkflowCandidateFileStore) SetObserver(observer WorkflowEditObserver) {
	if s != nil {
		s.observer = observer
	}
}

func (s *WorkflowCandidateFileStore) Create(ctx context.Context, revision WorkflowCandidateRevision) (edit.WorkflowCandidate, error) {
	if err := ctx.Err(); err != nil {
		return edit.WorkflowCandidate{}, err
	}
	if err := validateWorkflowCandidateRevision(revision); err != nil {
		return edit.WorkflowCandidate{}, err
	}
	if revision.CandidateRevision != 1 {
		return edit.WorkflowCandidate{}, fmt.Errorf("%w: first candidate revision must be 1", ErrInvalid)
	}
	dir, err := s.candidateDir(revision.Key, revision.CandidateID)
	if err != nil {
		return edit.WorkflowCandidate{}, err
	}
	lock := workflowCandidatePathLock(dir)
	lock.Lock()
	defer lock.Unlock()
	if manifest, found, err := s.loadManifest(dir); err != nil {
		return edit.WorkflowCandidate{}, err
	} else if found {
		stored, readErr := s.readRevision(dir, 1)
		if readErr == nil && sameWorkflowCandidateRevision(stored, revision) {
			return manifest.Candidate, nil
		}
		return edit.WorkflowCandidate{}, fmt.Errorf("%w: workflow candidate", ErrAlreadyExists)
	}
	if err := s.writeRevisionImmutable(ctx, dir, revision); err != nil {
		return edit.WorkflowCandidate{}, err
	}
	candidate := edit.WorkflowCandidate{ID: revision.CandidateID, Key: revision.Key, CurrentChangeSetID: revision.ID, CurrentRevision: 1, CurrentPreviewHash: revision.PreviewHash, State: WorkflowCandidateStateActive, ExpiresAt: revision.ExpiresAt}
	manifest := workflowCandidateManifest{FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Candidate: candidate, SupersededBy: map[string]uint64{}, UpdatedAt: revision.CreatedAt}
	if err := s.saveManifest(dir, manifest); err != nil {
		return edit.WorkflowCandidate{}, err
	}
	return candidate, nil
}

func (s *WorkflowCandidateFileStore) Revise(ctx context.Context, key edit.WorkflowKey, candidateID string, expected uint64, next WorkflowCandidateRevision) (edit.WorkflowCandidate, error) {
	if err := validateWorkflowCandidateRevision(next); err != nil {
		return edit.WorkflowCandidate{}, err
	}
	if next.Key != key || next.CandidateID != candidateID || next.ParentRevision != expected || next.CandidateRevision != expected+1 {
		return edit.WorkflowCandidate{}, fmt.Errorf("%w: revision binding does not match CAS request", ErrInvalid)
	}
	var result edit.WorkflowCandidate
	err := s.withCandidate(ctx, key, candidateID, func(dir string, manifest *workflowCandidateManifest) error {
		if manifest.Candidate.CurrentRevision == next.CandidateRevision {
			stored, err := s.readRevision(dir, next.CandidateRevision)
			if err == nil && sameWorkflowCandidateRevision(stored, next) {
				result = manifest.Candidate
				return nil
			}
		}
		if manifest.Candidate.State != WorkflowCandidateStateActive || manifest.Candidate.CurrentRevision != expected {
			return workflowCandidateCASConflict(key, expected, manifest.Candidate.CurrentRevision)
		}
		if err := s.writeRevisionImmutable(ctx, dir, next); err != nil {
			return err
		}
		manifest.SupersededBy[strconv.FormatUint(expected, 10)] = next.CandidateRevision
		manifest.Candidate.CurrentRevision, manifest.Candidate.CurrentChangeSetID = next.CandidateRevision, next.ID
		manifest.Candidate.CurrentPreviewHash, manifest.Candidate.ExpiresAt = next.PreviewHash, next.ExpiresAt
		manifest.UpdatedAt, result = next.CreatedAt, manifest.Candidate
		return s.saveManifest(dir, *manifest)
	})
	return result, err
}

func (s *WorkflowCandidateFileStore) GetCurrent(ctx context.Context, key edit.WorkflowKey, candidateID string) (edit.WorkflowCandidate, error) {
	var result edit.WorkflowCandidate
	err := s.withCandidate(ctx, key, candidateID, func(_ string, manifest *workflowCandidateManifest) error {
		result = manifest.Candidate
		return nil
	})
	return result, err
}

func (s *WorkflowCandidateFileStore) GetRevision(ctx context.Context, key edit.WorkflowKey, candidateID string, revision uint64) (WorkflowCandidateRevision, error) {
	var result WorkflowCandidateRevision
	err := s.withCandidate(ctx, key, candidateID, func(dir string, manifest *workflowCandidateManifest) (err error) {
		result, err = s.readRevisionView(dir, *manifest, revision)
		return err
	})
	return result, err
}

func (s *WorkflowCandidateFileStore) ListRevisions(ctx context.Context, key edit.WorkflowKey, candidateID string) ([]WorkflowCandidateRevision, error) {
	var items []WorkflowCandidateRevision
	err := s.withCandidate(ctx, key, candidateID, func(dir string, manifest *workflowCandidateManifest) error {
		items = make([]WorkflowCandidateRevision, 0, manifest.Candidate.CurrentRevision)
		for revision := uint64(1); revision <= manifest.Candidate.CurrentRevision; revision++ {
			item, err := s.readRevisionView(dir, *manifest, revision)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

func (s *WorkflowCandidateFileStore) ResolveChangeSet(ctx context.Context, key edit.WorkflowKey, changeSetID string) (WorkflowCandidateRevision, error) {
	if strings.TrimSpace(s.Root) == "" || key.Validate() != nil || strings.TrimSpace(changeSetID) == "" || strings.TrimSpace(changeSetID) != changeSetID || strings.ContainsRune(changeSetID, '\x00') {
		return WorkflowCandidateRevision{}, fmt.Errorf("%w: invalid workflow candidate changeSetId", ErrInvalid)
	}
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	paths, err := filepath.Glob(filepath.Join(s.Root, encode(key.TenantID), encode(key.WorkflowID), "*", "revisions", "*.json"))
	if err != nil {
		return WorkflowCandidateRevision{}, err
	}
	var result WorkflowCandidateRevision
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return WorkflowCandidateRevision{}, err
		}
		transaction, err := readWorkflowCandidateTransaction(path)
		if err != nil {
			return WorkflowCandidateRevision{}, err
		}
		if transaction.Revision.Key != key {
			return WorkflowCandidateRevision{}, fmt.Errorf("workflow candidate transaction key mismatch")
		}
		if transaction.Revision.ID == changeSetID {
			if result.ID != "" {
				return WorkflowCandidateRevision{}, fmt.Errorf("%w: duplicate workflow candidate changeSetId", ErrConflict)
			}
			result = transaction.Revision
		}
	}
	if result.ID == "" {
		return WorkflowCandidateRevision{}, fmt.Errorf("%w: workflow candidate changeSetId", ErrNotFound)
	}
	return result, nil
}

func (s *WorkflowCandidateFileStore) Abandon(ctx context.Context, key edit.WorkflowKey, candidateID string, expected uint64, at time.Time) (edit.WorkflowCandidate, error) {
	startedAt := time.Now().UTC()
	var result edit.WorkflowCandidate
	err := s.withCandidate(ctx, key, candidateID, func(dir string, manifest *workflowCandidateManifest) error {
		if manifest.Candidate.State != WorkflowCandidateStateActive || manifest.Candidate.CurrentRevision != expected {
			return workflowCandidateCASConflict(key, expected, manifest.Candidate.CurrentRevision)
		}
		when := at.UTC()
		manifest.Candidate.State, manifest.AbandonedAt, manifest.UpdatedAt = WorkflowCandidateStateAbandoned, &when, when
		result = manifest.Candidate
		return s.saveManifest(dir, *manifest)
	})
	if s != nil && s.observer != nil {
		observation := WorkflowEditObservation{Operation: WorkflowEditObserveAbandon, Key: key, CandidateID: candidateID, CandidateRevision: expected, StartedAt: startedAt, Duration: time.Since(startedAt)}
		outcome, code := workflowEditObservationOutcome(err, "")
		observation.Outcome, observation.ErrorCode = outcome, code
		func() {
			defer func() { _ = recover() }()
			_ = s.observer.ObserveWorkflowEdit(ctx, observation)
		}()
	}
	return result, err
}

func (s *WorkflowCandidateFileStore) CheckApplyEligibility(ctx context.Context, binding WorkflowCandidateApplyBinding, at time.Time) (WorkflowCandidateRevision, error) {
	var result WorkflowCandidateRevision
	err := s.withCandidate(ctx, binding.Key, binding.CandidateID, func(dir string, manifest *workflowCandidateManifest) error {
		if manifest.Candidate.State != WorkflowCandidateStateActive || manifest.Candidate.CurrentRevision != binding.CandidateRevision {
			return workflowCandidateCASConflict(binding.Key, binding.CandidateRevision, manifest.Candidate.CurrentRevision)
		}
		revision, err := s.readRevisionView(dir, *manifest, binding.CandidateRevision)
		if err != nil {
			return err
		}
		if !at.Before(revision.ExpiresAt) {
			return candidateDomainError(edit.ErrorStaleChangeSet, binding.Key, strconv.FormatUint(revision.CandidateRevision, 10), strconv.FormatUint(binding.CandidateRevision, 10), "candidate has expired", "create and review a new candidate")
		}
		if revision.ID != binding.ChangeSetID {
			return candidateDomainError(edit.ErrorStaleChangeSet, binding.Key, revision.ID, binding.ChangeSetID, "change set is not current", "reload the current candidate")
		}
		if revision.PreviewHash != binding.PreviewHash {
			return candidateDomainError(edit.ErrorPreviewMismatch, binding.Key, revision.PreviewHash, binding.PreviewHash, "preview does not match the reviewed candidate", "review the current preview before applying")
		}
		result = revision
		return nil
	})
	return result, err
}

func (s *WorkflowCandidateFileStore) readRevisionView(dir string, manifest workflowCandidateManifest, revision uint64) (WorkflowCandidateRevision, error) {
	item, err := s.readRevision(dir, revision)
	if err != nil {
		return WorkflowCandidateRevision{}, err
	}
	if _, ok := manifest.SupersededBy[strconv.FormatUint(revision, 10)]; ok {
		item.State = WorkflowCandidateRevisionSuperseded
	} else if manifest.Candidate.State == WorkflowCandidateStateAbandoned && revision == manifest.Candidate.CurrentRevision {
		item.State = edit.CandidateState(WorkflowCandidateStateAbandoned)
	}
	return item, nil
}

func (s *WorkflowCandidateFileStore) candidateDir(key edit.WorkflowKey, candidateID string) (string, error) {
	if strings.TrimSpace(s.Root) == "" {
		return "", fmt.Errorf("%w: candidate root is required", ErrInvalid)
	}
	if err := key.Validate(); err != nil {
		return "", candidateDomainError(edit.ErrorInvalidWorkflowKey, key, "", "", "invalid workflow key", "use the authenticated workflow scope")
	}
	if err := validateWorkflowCandidateID(candidateID); err != nil {
		return "", err
	}
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	return filepath.Join(s.Root, encode(key.TenantID), encode(key.WorkflowID), encode(candidateID)), nil
}

func (s *WorkflowCandidateFileStore) withCandidate(ctx context.Context, key edit.WorkflowKey, candidateID string, action func(string, *workflowCandidateManifest) error) error {
	dir, err := s.candidateDir(key, candidateID)
	if err != nil {
		return err
	}
	lock := workflowCandidatePathLock(dir)
	lock.Lock()
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, found, err := s.loadManifest(dir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: workflow candidate", ErrNotFound)
	}
	if manifest.Candidate.Key != key || manifest.Candidate.ID != candidateID {
		return fmt.Errorf("workflow candidate manifest binding mismatch")
	}
	return action(dir, &manifest)
}

func (s *WorkflowCandidateFileStore) loadManifest(dir string) (workflowCandidateManifest, bool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s.recoverManifest(dir, workflowCandidateManifest{}, false)
	}
	if err != nil {
		return workflowCandidateManifest{}, false, err
	}
	var manifest workflowCandidateManifest
	if err := edit.DecodeStrict(raw, &manifest); err != nil {
		return workflowCandidateManifest{}, false, fmt.Errorf("decode workflow candidate manifest: %w", err)
	}
	if manifest.FormatVersion != 1 || manifest.SchemaVersion != edit.SchemaVersion || manifest.Candidate.CurrentRevision == 0 || manifest.Candidate.Key.Validate() != nil {
		return workflowCandidateManifest{}, false, fmt.Errorf("invalid workflow candidate manifest")
	}
	if manifest.SupersededBy == nil {
		manifest.SupersededBy = map[string]uint64{}
	}
	return s.recoverManifest(dir, manifest, true)
}

func (s *WorkflowCandidateFileStore) readRevision(dir string, revision uint64) (WorkflowCandidateRevision, error) {
	transaction, err := readWorkflowCandidateTransaction(workflowCandidateRevisionPath(dir, revision))
	if errors.Is(err, os.ErrNotExist) {
		return WorkflowCandidateRevision{}, fmt.Errorf("%w: candidate revision", ErrNotFound)
	}
	if err != nil {
		return WorkflowCandidateRevision{}, err
	}
	return transaction.Revision, nil
}

func (s *WorkflowCandidateFileStore) writeRevisionImmutable(ctx context.Context, dir string, revision WorkflowCandidateRevision) error {
	path := workflowCandidateRevisionPath(dir, revision.CandidateRevision)
	transaction, err := newWorkflowCandidateTransaction(revision)
	if err != nil {
		return err
	}
	log, err := NewFileWorkflowEventLog(s.Root)
	if err != nil {
		return err
	}
	_, err = log.persistCandidateTransaction(ctx, path, transaction)
	return err
}

func (s *WorkflowCandidateFileStore) saveManifest(dir string, manifest workflowCandidateManifest) error {
	return writeWorkflowJSON(filepath.Join(dir, "current.json"), manifest)
}

func (s *WorkflowCandidateFileStore) recoverManifest(dir string, manifest workflowCandidateManifest, found bool) (workflowCandidateManifest, bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "revisions", "*.json"))
	if err != nil || len(paths) == 0 {
		return manifest, found, err
	}
	sort.Strings(paths)
	revisions := make([]WorkflowCandidateRevision, 0, len(paths))
	for _, path := range paths {
		transaction, readErr := readWorkflowCandidateTransaction(path)
		if readErr != nil {
			return workflowCandidateManifest{}, false, readErr
		}
		revisions = append(revisions, transaction.Revision)
	}
	first, latest := revisions[0], revisions[len(revisions)-1]
	for index, revision := range revisions {
		if revision.CandidateRevision != uint64(index+1) || revision.Key != first.Key || revision.CandidateID != first.CandidateID {
			return workflowCandidateManifest{}, false, fmt.Errorf("invalid workflow candidate transaction chain")
		}
	}
	if found && (manifest.Candidate.Key != first.Key || manifest.Candidate.ID != first.CandidateID || manifest.Candidate.CurrentRevision > latest.CandidateRevision) {
		return workflowCandidateManifest{}, false, fmt.Errorf("workflow candidate manifest binding mismatch")
	}
	if found && manifest.Candidate.CurrentRevision == latest.CandidateRevision {
		return manifest, true, nil
	}
	superseded := make(map[string]uint64, len(revisions)-1)
	for index := 1; index < len(revisions); index++ {
		superseded[strconv.Itoa(index)] = uint64(index + 1)
	}
	candidate := edit.WorkflowCandidate{ID: latest.CandidateID, Key: latest.Key, CurrentChangeSetID: latest.ID, CurrentRevision: latest.CandidateRevision, CurrentPreviewHash: latest.PreviewHash, State: WorkflowCandidateStateActive, ExpiresAt: latest.ExpiresAt}
	recovered := workflowCandidateManifest{FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Candidate: candidate, SupersededBy: superseded, UpdatedAt: latest.CreatedAt}
	if err := s.saveManifest(dir, recovered); err != nil {
		return workflowCandidateManifest{}, false, err
	}
	return recovered, true, nil
}

func newWorkflowCandidateTransaction(revision WorkflowCandidateRevision) (workflowCandidateTransaction, error) {
	digest, err := workflowQueryDigest([]any{revision.Key, revision.CandidateID, revision.CandidateRevision, revision.ID})
	if err != nil {
		return workflowCandidateTransaction{}, err
	}
	token := strings.TrimPrefix(digest, "sha256:")
	payload, err := json.Marshal(map[string]any{"kind": "candidate_transaction", "candidateId": revision.CandidateID, "candidateRevision": revision.CandidateRevision, "parentCandidateRevision": revision.ParentRevision, "previewHash": revision.PreviewHash})
	if err != nil {
		return workflowCandidateTransaction{}, err
	}
	eventType := "workflow.candidate.revised"
	if revision.CandidateRevision == 1 {
		eventType = "workflow.candidate.created"
	}
	event := edit.WorkflowDomainEvent{SchemaVersion: edit.SchemaVersion, EventID: "candidate-event-" + token[:32], Type: eventType, Key: revision.Key, Revision: revision.Base, Actor: revision.Actor, TransactionID: "candidate-txn-" + token[:32], ChangeSetID: revision.ID, CandidateRevision: revision.CandidateRevision, Payload: payload, OccurredAt: revision.CreatedAt}
	return workflowCandidateTransaction{FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Revision: revision, Event: event}, nil
}

func readWorkflowCandidateTransaction(path string) (workflowCandidateTransaction, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return workflowCandidateTransaction{}, err
	}
	var transaction workflowCandidateTransaction
	if edit.DecodeStrict(raw, &transaction) == nil && transaction.FormatVersion == 1 && transaction.SchemaVersion == edit.SchemaVersion {
		if err := validateWorkflowCandidateTransaction(transaction); err != nil {
			return workflowCandidateTransaction{}, err
		}
		return transaction, nil
	}
	var legacy WorkflowCandidateRevision
	if err := edit.DecodeStrict(raw, &legacy); err != nil {
		return workflowCandidateTransaction{}, fmt.Errorf("decode workflow candidate transaction: %w", err)
	}
	return newWorkflowCandidateTransaction(legacy)
}

func validateWorkflowCandidateTransaction(transaction workflowCandidateTransaction) error {
	if err := validateWorkflowCandidateRevision(transaction.Revision); err != nil {
		return fmt.Errorf("invalid persisted candidate revision: %w", err)
	}
	expected, err := newWorkflowCandidateTransaction(transaction.Revision)
	if err != nil || !sameWorkflowEvent(expected.Event, transaction.Event) {
		return fmt.Errorf("invalid persisted candidate event")
	}
	return nil
}

func sameWorkflowCandidateRevision(left, right WorkflowCandidateRevision) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func listWorkflowCandidateTransactions(root string, key edit.WorkflowKey) ([]workflowCandidateTransaction, error) {
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	paths, err := filepath.Glob(filepath.Join(root, encode(key.TenantID), encode(key.WorkflowID), "*", "revisions", "*.json"))
	if err != nil {
		return nil, err
	}
	transactions := make([]workflowCandidateTransaction, 0, len(paths))
	for _, path := range paths {
		transaction, readErr := readWorkflowCandidateTransaction(path)
		if readErr != nil {
			return nil, readErr
		}
		if transaction.Revision.Key != key {
			return nil, fmt.Errorf("workflow candidate transaction key mismatch")
		}
		transactions = append(transactions, transaction)
	}
	sort.Slice(transactions, func(i, j int) bool {
		if transactions[i].Event.OccurredAt.Equal(transactions[j].Event.OccurredAt) {
			return transactions[i].Event.TransactionID < transactions[j].Event.TransactionID
		}
		return transactions[i].Event.OccurredAt.Before(transactions[j].Event.OccurredAt)
	})
	return transactions, nil
}

func workflowCandidatePathLock(path string) *sync.Mutex {
	value, _ := workflowCandidatePathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return value.(*sync.Mutex)
}

func workflowCandidateRevisionPath(dir string, revision uint64) string {
	return filepath.Join(dir, "revisions", fmt.Sprintf("%020d.json", revision))
}

func workflowCandidateCASConflict(key edit.WorkflowKey, expected, current uint64) error {
	return candidateDomainError(edit.ErrorStaleChangeSet, key, strconv.FormatUint(current, 10), strconv.FormatUint(expected, 10), "candidate revision changed", "reload and rebase onto the current candidate")
}
