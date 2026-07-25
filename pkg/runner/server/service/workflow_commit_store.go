package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflowstore"
)

type WorkflowCommitInput struct {
	Key                                                    edit.WorkflowKey
	ExpectedResourceVersion, IdempotencyKey, RequestDigest string
	GraphPayload                                           []byte
	Meta, History, ImportResult, RollbackResult            workflowstore.TypedEnvelope
	Receipt                                                edit.WorkflowMutationReceipt
	ValidationBasis                                        edit.ValidationBasis
	Checkpoint, Inverse, ApprovalConsume, Audit            workflowstore.TypedEnvelope
	approvalConsumption                                    *workflowstore.TypedEnvelope
	Events, Outbox                                         []workflowstore.TypedEnvelope
}
type WorkflowCommitStore struct {
	root      string
	now       func() time.Time
	recovery  WorkflowCommitRecoveryCallbacks
	failpoint workflowCommitFailpointHook
}

type WorkflowReceiptLifecycleUpdate struct {
	Key           edit.WorkflowKey
	TransactionID string
	ExpectedBasis edit.ValidationBasis
	Status        string
	Validation    edit.ValidationResult
	Artifact      workflowstore.TypedEnvelope
}

func NewWorkflowCommitStore(root string, options ...WorkflowCommitStoreOption) (*WorkflowCommitStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: workflow commit root is required", ErrInvalid)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workflow commit root: %w", err)
	}
	store := &WorkflowCommitStore{root: absolute, now: time.Now}
	for _, option := range options {
		if err := option(store); err != nil {
			return nil, err
		}
	}
	return store, nil
}
func (s *WorkflowCommitStore) Commit(ctx context.Context, input WorkflowCommitInput) (edit.WorkflowMutationReceipt, error) {
	if ctx == nil {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: commit context is required", ErrInvalid)
	}
	if err := validateWorkflowCommitInput(input); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	var result edit.WorkflowMutationReceipt
	err := s.withWorkflowCommitLock(ctx, input.Key, func(locked *workflowstore.WorkflowNamespace) error {
		var err error
		result, err = s.commitWorkflowLocked(ctx, locked, input)
		return err
	})
	return result, err
}

func (s *WorkflowCommitStore) withWorkflowCommitLock(ctx context.Context, key edit.WorkflowKey, action func(*workflowstore.WorkflowNamespace) error) error {
	if action == nil {
		return fmt.Errorf("%w: workflow commit action is required", ErrInvalid)
	}
	if err := validateWorkflowCommitRead(ctx, key); err != nil {
		return err
	}
	namespace, err := workflowstore.NewWorkflowNamespace(s.root, key)
	if err != nil {
		return err
	}
	return namespace.WithLock(ctx, func(locked *workflowstore.WorkflowNamespace) error {
		if err := s.recoverWorkflowCommitNamespace(ctx, locked, key); err != nil {
			return err
		}
		return action(locked)
	})
}

func (s *WorkflowCommitStore) commitWorkflowLocked(ctx context.Context, locked *workflowstore.WorkflowNamespace, input WorkflowCommitInput) (edit.WorkflowMutationReceipt, error) {
	current, found, err := workflowstore.LoadCurrentTransactionSnapshot(locked)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	idempotencyHash := workflowCommitIdempotencyHash(input.IdempotencyKey)
	if found {
		for _, persisted := range current.Receipts {
			if persisted.IdempotencyKeyHash == idempotencyHash {
				if persisted.RequestDigest != input.RequestDigest {
					return edit.WorkflowMutationReceipt{}, workflowCommitDomainError(edit.ErrorIdempotencyKeyReused, input.Key, current.ResourceVersion, input.ExpectedResourceVersion, "idempotency key was already used for another request", "use the original request or a new idempotency key")
				}
				return persisted.Receipt, nil
			}
		}
	}
	currentVersion, generation := "", uint64(0)
	receipts := make([]workflowstore.PersistedReceipt, 0, 1)
	approvals, lifecycle := []workflowstore.TypedEnvelope{}, []workflowstore.TypedEnvelope{}
	if found {
		if current.Key != input.Key {
			return edit.WorkflowMutationReceipt{}, fmt.Errorf("workflow commit snapshot key binding mismatch")
		}
		currentVersion, generation = current.ResourceVersion, current.Generation
		receipts = append(receipts, current.Receipts...)
		approvals = append(approvals, current.ApprovalConsumptions...)
		lifecycle = append(lifecycle, current.LifecycleArtifacts...)
	}
	if currentVersion != input.ExpectedResourceVersion {
		return edit.WorkflowMutationReceipt{}, workflowCommitDomainError(edit.ErrorStaleRevision, input.Key, currentVersion, input.ExpectedResourceVersion, "workflow resource version changed", "reload the current workflow and rebase the change")
	}
	receipts = append(receipts, workflowstore.PersistedReceipt{IdempotencyKeyHash: idempotencyHash, RequestDigest: input.RequestDigest, Receipt: input.Receipt})
	if input.approvalConsumption != nil {
		approvals = append(approvals, *input.approvalConsumption)
	}
	next := workflowstore.TransactionSnapshot{
		FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Key: input.Key, Generation: generation + 1,
		ResourceVersion: input.Receipt.RevisionAfter.ResourceVersion, PreviousResourceVersion: currentVersion,
		TransactionID: input.Receipt.TransactionID, GraphPayload: append([]byte(nil), input.GraphPayload...),
		Meta: input.Meta, History: input.History, ImportResult: input.ImportResult, RollbackResult: input.RollbackResult,
		Receipt: input.Receipt, ValidationBasis: input.ValidationBasis, Checkpoint: input.Checkpoint, Inverse: input.Inverse,
		ApprovalConsume: input.ApprovalConsume, ApprovalConsumptions: approvals, LifecycleArtifacts: lifecycle, Audit: input.Audit,
		Events: append([]workflowstore.TypedEnvelope(nil), input.Events...), Outbox: append([]workflowstore.TypedEnvelope(nil), input.Outbox...),
		Receipts: receipts, CommittedAt: s.now().UTC(),
	}
	if err := s.publishWorkflowCommitSnapshot(ctx, locked, next); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	return input.Receipt, nil
}

// UpdateReceiptLifecycle publishes a new immutable generation for a committed
// graph without changing its resource version. The transaction snapshot is the
// authoritative lifecycle ledger; callers cannot complete a different basis.
func (s *WorkflowCommitStore) UpdateReceiptLifecycle(ctx context.Context, update WorkflowReceiptLifecycleUpdate) (edit.WorkflowMutationReceipt, error) {
	if err := validateWorkflowReceiptLifecycleUpdate(update); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	var result edit.WorkflowMutationReceipt
	err := s.withWorkflowCommitLock(ctx, update.Key, func(locked *workflowstore.WorkflowNamespace) error {
		current, found, err := workflowstore.LoadCurrentTransactionSnapshot(locked)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: workflow commit snapshot", ErrNotFound)
		}
		if current.Key != update.Key || current.TransactionID != update.TransactionID || current.ValidationBasis != update.ExpectedBasis {
			return workflowCommitDomainError(edit.ErrorStaleRevision, update.Key, current.ResourceVersion, update.ExpectedBasis.ResourceVersion, "workflow validation basis changed", "reload the committed workflow and repeat the post-check")
		}
		if current.Receipt.Status != "verifying" {
			if workflowReceiptLifecycleReplay(current, update) {
				result = current.Receipt
				return nil
			}
			return workflowCommitDomainError(edit.ErrorDuplicateTransaction, update.Key, current.ResourceVersion, update.ExpectedBasis.ResourceVersion, "workflow receipt lifecycle is already settled", "use the persisted transaction receipt")
		}

		next := current
		next.Generation++
		next.PreviousResourceVersion = current.ResourceVersion
		next.Receipt.Status, next.Receipt.Validation = update.Status, update.Validation
		next.LifecycleArtifacts = append(append([]workflowstore.TypedEnvelope(nil), current.LifecycleArtifacts...), update.Artifact)
		next.CommittedAt = s.now().UTC()
		foundReceipt := false
		next.Receipts = append([]workflowstore.PersistedReceipt(nil), current.Receipts...)
		for index := range next.Receipts {
			if next.Receipts[index].Receipt.TransactionID == update.TransactionID {
				next.Receipts[index].Receipt = next.Receipt
				foundReceipt = true
			}
		}
		if !foundReceipt {
			return fmt.Errorf("workflow transaction receipt ledger is incomplete")
		}
		if err := s.publishWorkflowCommitSnapshot(ctx, locked, next); err != nil {
			return err
		}
		result = next.Receipt
		return nil
	})
	return result, err
}

func validateWorkflowReceiptLifecycleUpdate(update WorkflowReceiptLifecycleUpdate) error {
	if err := update.Key.Validate(); err != nil {
		return workflowCommitDomainError(edit.ErrorInvalidWorkflowKey, update.Key, "", update.ExpectedBasis.ResourceVersion, "invalid workflow key", "use the authenticated workflow scope")
	}
	if !validWorkflowApprovalValue(update.TransactionID, true) || update.ExpectedBasis.Validate() != nil || update.Validation.Kind != "validation" || update.Validation.Basis != update.ExpectedBasis || !validWorkflowCommitEnvelope(update.Artifact) {
		return fmt.Errorf("%w: invalid workflow receipt lifecycle update", ErrInvalid)
	}
	switch update.Status {
	case "completed", "completed_with_warnings":
		if !update.Validation.Valid {
			return fmt.Errorf("%w: completed workflow receipt requires valid post-checks", ErrInvalid)
		}
	case "configuration_required", "saved_with_issues":
	default:
		return fmt.Errorf("%w: invalid workflow receipt lifecycle status", ErrInvalid)
	}
	return nil
}

func workflowReceiptLifecycleReplay(current workflowstore.TransactionSnapshot, update WorkflowReceiptLifecycleUpdate) bool {
	if current.Receipt.Status != update.Status || !reflect.DeepEqual(current.Receipt.Validation, update.Validation) {
		return false
	}
	for _, artifact := range current.LifecycleArtifacts {
		if reflect.DeepEqual(artifact, update.Artifact) {
			return true
		}
	}
	return false
}

func (s *WorkflowCommitStore) Snapshot(ctx context.Context, key edit.WorkflowKey) (workflowstore.TransactionSnapshot, error) {
	snapshot, found, err := s.currentWorkflowCommitSnapshot(ctx, key)
	if err != nil {
		return workflowstore.TransactionSnapshot{}, err
	}
	if !found {
		return workflowstore.TransactionSnapshot{}, fmt.Errorf("%w: workflow commit snapshot", ErrNotFound)
	}
	return snapshot, nil
}
func (s *WorkflowCommitStore) LookupReceipt(ctx context.Context, key edit.WorkflowKey, idempotencyKey string) (edit.WorkflowMutationReceipt, bool, error) {
	if strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(idempotencyKey) != idempotencyKey {
		return edit.WorkflowMutationReceipt{}, false, fmt.Errorf("%w: invalid idempotency key", ErrInvalid)
	}
	snapshot, found, err := s.currentWorkflowCommitSnapshot(ctx, key)
	if err != nil || !found {
		return edit.WorkflowMutationReceipt{}, false, err
	}
	hash := workflowCommitIdempotencyHash(idempotencyKey)
	for _, persisted := range snapshot.Receipts {
		if persisted.IdempotencyKeyHash == hash {
			return persisted.Receipt, true, nil
		}
	}
	return edit.WorkflowMutationReceipt{}, false, nil
}
func (s *WorkflowCommitStore) currentWorkflowCommitSnapshot(ctx context.Context, key edit.WorkflowKey) (workflowstore.TransactionSnapshot, bool, error) {
	if err := validateWorkflowCommitRead(ctx, key); err != nil {
		return workflowstore.TransactionSnapshot{}, false, err
	}
	namespace, err := workflowstore.NewWorkflowNamespace(s.root, key)
	if err != nil {
		return workflowstore.TransactionSnapshot{}, false, err
	}
	snapshot, found, err := workflowstore.LoadCurrentTransactionSnapshot(namespace)
	if err == nil && found && snapshot.Key != key {
		err = fmt.Errorf("workflow commit snapshot key binding mismatch")
	}
	return snapshot, found, err
}
func validateWorkflowCommitInput(input WorkflowCommitInput) error {
	if err := input.Key.Validate(); err != nil {
		return workflowCommitDomainError(edit.ErrorInvalidWorkflowKey, input.Key, "", input.ExpectedResourceVersion, "invalid workflow key", "use the authenticated workflow scope")
	}
	if strings.TrimSpace(input.ExpectedResourceVersion) != input.ExpectedResourceVersion || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || strings.TrimSpace(input.RequestDigest) == "" || strings.TrimSpace(input.RequestDigest) != input.RequestDigest || len(input.GraphPayload) == 0 {
		return fmt.Errorf("%w: invalid workflow commit identity, idempotency, or graph", ErrInvalid)
	}
	if input.Receipt.SchemaVersion != edit.SchemaVersion || input.Receipt.Kind != "receipt" || input.Receipt.Key != input.Key || strings.TrimSpace(input.Receipt.TransactionID) == "" || input.Receipt.RevisionAfter == nil || strings.TrimSpace(input.Receipt.RevisionAfter.ResourceVersion) == "" || input.Receipt.RevisionBefore.ResourceVersion != input.ExpectedResourceVersion || input.Receipt.RevisionAfter.ResourceVersion == input.ExpectedResourceVersion || input.Receipt.Validation.Basis != input.ValidationBasis {
		return fmt.Errorf("%w: invalid workflow commit receipt binding", ErrInvalid)
	}
	if err := input.ValidationBasis.Validate(); err != nil {
		return fmt.Errorf("%w: invalid workflow commit validation basis: %v", ErrInvalid, err)
	}
	envelopes := make([]workflowstore.TypedEnvelope, 0, 8+len(input.Events)+len(input.Outbox))
	envelopes = append(envelopes, input.Meta, input.History, input.ImportResult, input.RollbackResult, input.Checkpoint, input.Inverse, input.ApprovalConsume, input.Audit)
	envelopes = append(envelopes, input.Events...)
	envelopes = append(envelopes, input.Outbox...)
	for _, envelope := range envelopes {
		if !validWorkflowCommitEnvelope(envelope) {
			return fmt.Errorf("%w: invalid workflow commit typed envelope", ErrInvalid)
		}
	}
	if input.approvalConsumption != nil {
		if _, err := validateWorkflowApprovalConsumptionEnvelope(*input.approvalConsumption, input.Key, input.Receipt.TransactionID); err != nil {
			return err
		}
	}
	return nil
}

func validWorkflowCommitEnvelope(envelope workflowstore.TypedEnvelope) bool {
	return strings.TrimSpace(envelope.Kind) != "" && strings.TrimSpace(envelope.SchemaVersion) != "" && len(envelope.Payload) > 0 && json.Valid(envelope.Payload)
}
func validateWorkflowCommitRead(ctx context.Context, key edit.WorkflowKey) error {
	if ctx == nil {
		return fmt.Errorf("%w: workflow commit context is required", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return workflowCommitDomainError(edit.ErrorInvalidWorkflowKey, key, "", "", "invalid workflow key", "use the authenticated workflow scope")
	}
	return nil
}
func workflowCommitIdempotencyHash(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}
func workflowCommitDomainError(code edit.ErrorCode, key edit.WorkflowKey, current, base, message, retry string) error {
	return &edit.DomainError{Kind: "error", Code: code, Message: message, Key: &key, CurrentRevision: current, BaseRevision: base, RetryHint: retry}
}
