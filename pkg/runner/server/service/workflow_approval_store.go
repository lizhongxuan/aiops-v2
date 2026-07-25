package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"runner/workflow/edit"
	"runner/workflowstore"
)

type WorkflowApprovalState string

const (
	WorkflowApprovalActive   WorkflowApprovalState = "active"
	WorkflowApprovalRevoked  WorkflowApprovalState = "revoked"
	WorkflowApprovalExpired  WorkflowApprovalState = "expired"
	WorkflowApprovalConsumed WorkflowApprovalState = "consumed"
)

type WorkflowRiskLevel string

const (
	WorkflowRiskLow      WorkflowRiskLevel = "low"
	WorkflowRiskMedium   WorkflowRiskLevel = "medium"
	WorkflowRiskHigh     WorkflowRiskLevel = "high"
	WorkflowRiskCritical WorkflowRiskLevel = "critical"
)

type WorkflowGrantRiskScope struct {
	MaxRisk WorkflowRiskLevel `json:"maxRisk"`
}

type WorkflowEditGrantRecord struct {
	Grant           edit.WorkflowEditGrant `json:"grant"`
	CanvasSessionID string                 `json:"canvasSessionId"`
	RiskScope       WorkflowGrantRiskScope `json:"riskScope"`
	State           WorkflowApprovalState  `json:"state"`
	IssuedAt        time.Time              `json:"issuedAt"`
	RevokedAt       *time.Time             `json:"revokedAt,omitempty"`
}

type WorkflowApplyApprovalRecord struct {
	Approval        edit.WorkflowApplyApproval `json:"approval"`
	GrantID         string                     `json:"grantId"`
	CanvasSessionID string                     `json:"canvasSessionId"`
	PlanHash        string                     `json:"planHash"`
	State           WorkflowApprovalState      `json:"state"`
	IssuedAt        time.Time                  `json:"issuedAt"`
	RevokedAt       *time.Time                 `json:"revokedAt,omitempty"`
	ConsumptionID   string                     `json:"consumptionId,omitempty"`
}

type WorkflowTrustedApplyFacts struct {
	Key                                                     edit.WorkflowKey
	ActorID, CanvasSessionID, GrantID, ApprovalID           string
	PlanHash, BaseSemanticHash                              string
	CandidateID, ChangeSetID, PreviewHash, ExpectedRevision string
	Operations                                              []edit.OperationKind
	OperationCount                                          int
	AddedNodeCount                                          int
	DeletesExisting                                         bool
	Risk                                                    WorkflowRiskLevel
}

type workflowVerifiedPayload struct {
	Facts    WorkflowTrustedApplyFacts
	IssuedAt time.Time
}

type VerifiedWorkflowApplyBinding struct {
	payload workflowVerifiedPayload
	mac     [sha256.Size]byte
}

func (b VerifiedWorkflowApplyBinding) ApprovalID() string            { return b.payload.Facts.ApprovalID }
func (b VerifiedWorkflowApplyBinding) WorkflowKey() edit.WorkflowKey { return b.payload.Facts.Key }

type WorkflowApprovalConsumption struct {
	SchemaVersion    string               `json:"schemaVersion"`
	TransactionID    string               `json:"transactionId"`
	ApprovalID       string               `json:"approvalId"`
	GrantID          string               `json:"grantId"`
	Key              edit.WorkflowKey     `json:"workflowKey"`
	ActorID          string               `json:"actorId"`
	CanvasSessionID  string               `json:"canvasSessionId"`
	PlanHash         string               `json:"planHash"`
	BaseSemanticHash string               `json:"baseSemanticHash"`
	CandidateID      string               `json:"candidateId"`
	ChangeSetID      string               `json:"changeSetId"`
	PreviewHash      string               `json:"previewHash"`
	ExpectedRevision string               `json:"expectedRevision"`
	Operations       []edit.OperationKind `json:"operations"`
	OperationCount   int                  `json:"operationCount,omitempty"`
	Risk             WorkflowRiskLevel    `json:"risk"`
	ConsumedAt       time.Time            `json:"consumedAt"`
}

type WorkflowApprovalFileStore struct {
	Root         string
	transactions *WorkflowCommitStore
	sealKey      [sha256.Size]byte
	sealReady    bool
}

type workflowApprovalFile struct {
	FormatVersion int                                    `json:"formatVersion"`
	SchemaVersion string                                 `json:"schemaVersion"`
	Key           edit.WorkflowKey                       `json:"workflowKey"`
	Grants        map[string]WorkflowEditGrantRecord     `json:"grants"`
	Approvals     map[string]WorkflowApplyApprovalRecord `json:"approvals"`
	UpdatedAt     time.Time                              `json:"updatedAt"`
}

var workflowApprovalPathLocks sync.Map

const workflowApprovalConsumptionEnvelopeKind = "workflow.approval_consumed"

func NewWorkflowApprovalFileStore(root string, transactions ...*WorkflowCommitStore) *WorkflowApprovalFileStore {
	store := &WorkflowApprovalFileStore{Root: root}
	if len(transactions) > 0 {
		store.transactions = transactions[0]
	}
	_, err := rand.Read(store.sealKey[:])
	store.sealReady = err == nil
	return store
}

func (s *WorkflowApprovalFileStore) IssueGrant(ctx context.Context, record WorkflowEditGrantRecord) error {
	if err := validateWorkflowGrantRecord(record); err != nil {
		return err
	}
	return s.withState(ctx, record.Grant.Key, true, func(state *workflowApprovalFile) (bool, error) {
		if _, exists := state.Grants[record.Grant.ID]; exists {
			return false, fmt.Errorf("%w: workflow edit grant", ErrAlreadyExists)
		}
		record.IssuedAt, record.Grant.ExpiresAt = record.IssuedAt.UTC(), record.Grant.ExpiresAt.UTC()
		state.Grants[record.Grant.ID], state.UpdatedAt = record, record.IssuedAt
		return true, nil
	})
}

func (s *WorkflowApprovalFileStore) IssueApplyApproval(ctx context.Context, record WorkflowApplyApprovalRecord) error {
	if err := validateWorkflowApplyApprovalRecord(record); err != nil {
		return err
	}
	return s.withState(ctx, record.Approval.Key, false, func(state *workflowApprovalFile) (bool, error) {
		dirty := expireWorkflowApprovals(state, record.IssuedAt) > 0
		grant, found := state.Grants[record.GrantID]
		if !found || grant.State != WorkflowApprovalActive || grant.Grant.Key != record.Approval.Key || grant.Grant.ActorID != record.Approval.ActorID || grant.CanvasSessionID != record.CanvasSessionID || grant.Grant.PlanHash != record.PlanHash || record.Approval.ExpiresAt.After(grant.Grant.ExpiresAt) {
			return dirty, workflowApprovalError(edit.ErrorApprovalRequired, record.Approval.Key, "an active matching workflow edit grant is required", "request a new grant")
		}
		if _, exists := state.Approvals[record.Approval.ID]; exists {
			return dirty, fmt.Errorf("%w: workflow apply approval", ErrAlreadyExists)
		}
		record.IssuedAt, record.Approval.ExpiresAt = record.IssuedAt.UTC(), record.Approval.ExpiresAt.UTC()
		state.Approvals[record.Approval.ID], state.UpdatedAt = record, record.IssuedAt
		return true, nil
	})
}

func (s *WorkflowApprovalFileStore) GetGrant(ctx context.Context, key edit.WorkflowKey, id string) (WorkflowEditGrantRecord, error) {
	var result WorkflowEditGrantRecord
	err := s.withState(ctx, key, false, func(state *workflowApprovalFile) (bool, error) {
		var found bool
		result, found = state.Grants[id]
		if !found {
			return false, fmt.Errorf("%w: workflow edit grant", ErrNotFound)
		}
		return false, nil
	})
	return result, err
}

func (s *WorkflowApprovalFileStore) GetApplyApproval(ctx context.Context, key edit.WorkflowKey, id string) (WorkflowApplyApprovalRecord, error) {
	var result WorkflowApplyApprovalRecord
	err := s.withState(ctx, key, false, func(state *workflowApprovalFile) (bool, error) {
		var found bool
		result, found = state.Approvals[id]
		if !found {
			return false, fmt.Errorf("%w: workflow apply approval", ErrNotFound)
		}
		return false, nil
	})
	return result, err
}

func (s *WorkflowApprovalFileStore) RevokeGrant(ctx context.Context, key edit.WorkflowKey, id string, at time.Time) error {
	return s.withState(ctx, key, false, func(state *workflowApprovalFile) (bool, error) {
		dirty := expireWorkflowApprovals(state, at) > 0
		record, found := state.Grants[id]
		if !found || record.State != WorkflowApprovalActive {
			return dirty, workflowApprovalError(edit.ErrorApprovalRequired, key, "workflow edit grant is not active", "request a new grant")
		}
		when := at.UTC()
		record.State, record.RevokedAt = WorkflowApprovalRevoked, &when
		state.Grants[id], state.UpdatedAt = record, when
		return true, nil
	})
}

func (s *WorkflowApprovalFileStore) RevokeApplyApproval(ctx context.Context, key edit.WorkflowKey, id string, at time.Time) error {
	return s.withState(ctx, key, false, func(state *workflowApprovalFile) (bool, error) {
		dirty := expireWorkflowApprovals(state, at) > 0
		record, found := state.Approvals[id]
		if !found || record.State != WorkflowApprovalActive {
			return dirty, workflowApprovalError(edit.ErrorApprovalRequired, key, "workflow apply approval is not active", "request a new approval")
		}
		when := at.UTC()
		record.State, record.RevokedAt = WorkflowApprovalRevoked, &when
		state.Approvals[id], state.UpdatedAt = record, when
		return true, nil
	})
}

func (s *WorkflowApprovalFileStore) Expire(ctx context.Context, key edit.WorkflowKey, at time.Time) (int, error) {
	count := 0
	err := s.withState(ctx, key, false, func(state *workflowApprovalFile) (bool, error) {
		count = expireWorkflowApprovals(state, at)
		return count > 0, nil
	})
	return count, err
}

func (s *WorkflowApprovalFileStore) VerifyTrustedApplyFacts(ctx context.Context, facts WorkflowTrustedApplyFacts, at time.Time) (VerifiedWorkflowApplyBinding, error) {
	var binding VerifiedWorkflowApplyBinding
	err := s.withState(ctx, facts.Key, false, func(state *workflowApprovalFile) (bool, error) {
		dirty := expireWorkflowApprovals(state, at) > 0
		record, err := verifyWorkflowApplyFacts(state, facts)
		if err != nil {
			return dirty, err
		}
		if !s.sealReady {
			return dirty, fmt.Errorf("%w: trusted binding unavailable", ErrUnavailable)
		}
		binding.payload = workflowVerifiedPayload{Facts: facts, IssuedAt: record.IssuedAt}
		binding.mac = s.signBinding(binding.payload)
		return dirty, nil
	})
	if errors.Is(err, ErrNotFound) {
		err = workflowApprovalError(edit.ErrorApprovalRequired, facts.Key, "workflow approval scope was not found", "request a new approval")
	}
	return binding, err
}

func (s *WorkflowApprovalFileStore) ConsumeWithCommit(ctx context.Context, binding VerifiedWorkflowApplyBinding, at time.Time, transactionID string, commit func(WorkflowApprovalConsumption) error) error {
	if commit == nil || !validWorkflowApprovalValue(transactionID, true) || !s.validBinding(binding) {
		return workflowApprovalError(edit.ErrorApprovalRequired, binding.payload.Facts.Key, "verified workflow approval binding is required", "verify the current apply facts")
	}
	if s.transactions != nil {
		return fmt.Errorf("%w: configured approval stores require atomic workflow commit", ErrInvalid)
	}
	facts := binding.payload.Facts
	return s.withState(ctx, facts.Key, false, func(state *workflowApprovalFile) (bool, error) {
		dirty := expireWorkflowApprovals(state, at) > 0
		record, err := verifyWorkflowApplyFacts(state, facts)
		if err != nil {
			return dirty, err
		}
		if !record.IssuedAt.Equal(binding.payload.IssuedAt) {
			return dirty, workflowApprovalError(edit.ErrorApprovalRequired, facts.Key, "workflow approval binding is stale", "verify the current apply facts")
		}
		when := at.UTC()
		consumption := workflowApprovalConsumption(facts, transactionID, when)
		if err := commit(consumption); err != nil {
			return dirty, err
		}
		record.State, record.ConsumptionID = WorkflowApprovalConsumed, transactionID
		record.Approval.ConsumedAt, state.UpdatedAt = &when, when
		state.Approvals[facts.ApprovalID] = record
		return true, nil
	})
}

// ConsumeWithWorkflowCommit serializes approval revalidation, commit pointer
// publication, and approval-sidecar projection under the WorkflowKey lock. The
// cumulative consumption envelope in the transaction snapshot is authoritative
// if the process stops after publishing the pointer but before saving sidecar.
func (s *WorkflowApprovalFileStore) ConsumeWithWorkflowCommit(ctx context.Context, binding VerifiedWorkflowApplyBinding, at time.Time, input WorkflowCommitInput) (edit.WorkflowMutationReceipt, error) {
	if s.transactions == nil || !s.validBinding(binding) || at.IsZero() {
		return edit.WorkflowMutationReceipt{}, workflowApprovalError(edit.ErrorApprovalRequired, binding.payload.Facts.Key, "verified workflow approval binding and transaction store are required", "verify the current apply facts")
	}
	facts := binding.payload.Facts
	if input.Key != facts.Key || input.ExpectedResourceVersion != facts.ExpectedRevision {
		return edit.WorkflowMutationReceipt{}, workflowApprovalError(edit.ErrorStaleRevision, facts.Key, "workflow commit revision does not match the approval", "reload and rebase")
	}
	if input.Receipt.ChangeSetID != facts.ChangeSetID {
		return edit.WorkflowMutationReceipt{}, workflowApprovalError(edit.ErrorStaleChangeSet, facts.Key, "workflow commit change set does not match the approval", "review the current candidate")
	}
	when := at.UTC()
	consumption := workflowApprovalConsumption(facts, input.Receipt.TransactionID, when)
	envelope, err := workflowApprovalConsumptionEnvelope(consumption)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	input.ApprovalConsume, input.approvalConsumption = envelope, &envelope
	if err := validateWorkflowCommitInput(input); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}

	var result edit.WorkflowMutationReceipt
	err = s.transactions.withWorkflowCommitLock(ctx, facts.Key, func(locked *workflowstore.WorkflowNamespace) error {
		return s.withStateAndSnapshot(ctx, facts.Key, false, locked, func(state *workflowApprovalFile) (bool, error) {
			dirty := expireWorkflowApprovals(state, when) > 0
			if receipt, found, replayErr := workflowApprovalCommitReplay(locked, input, facts); replayErr != nil || found {
				result = receipt
				return dirty, replayErr
			}
			record, verifyErr := verifyWorkflowApplyFacts(state, facts)
			if verifyErr != nil {
				return dirty, verifyErr
			}
			if !record.IssuedAt.Equal(binding.payload.IssuedAt) {
				return dirty, workflowApprovalError(edit.ErrorApprovalRequired, facts.Key, "workflow approval binding is stale", "verify the current apply facts")
			}
			receipt, commitErr := s.transactions.commitWorkflowLocked(ctx, locked, input)
			if commitErr != nil {
				return dirty, commitErr
			}
			record.State, record.ConsumptionID = WorkflowApprovalConsumed, input.Receipt.TransactionID
			record.Approval.ConsumedAt, state.UpdatedAt = &when, when
			state.Approvals[facts.ApprovalID], result = record, receipt
			return true, nil
		})
	})
	return result, err
}

func workflowApprovalCommitReplay(locked *workflowstore.WorkflowNamespace, input WorkflowCommitInput, facts WorkflowTrustedApplyFacts) (edit.WorkflowMutationReceipt, bool, error) {
	snapshot, found, err := workflowstore.LoadCurrentTransactionSnapshot(locked)
	if err != nil || !found {
		return edit.WorkflowMutationReceipt{}, false, err
	}
	hash := workflowCommitIdempotencyHash(input.IdempotencyKey)
	for _, persisted := range snapshot.Receipts {
		if persisted.IdempotencyKeyHash != hash {
			continue
		}
		if persisted.RequestDigest != input.RequestDigest {
			return edit.WorkflowMutationReceipt{}, false, workflowCommitDomainError(edit.ErrorIdempotencyKeyReused, input.Key, snapshot.ResourceVersion, input.ExpectedResourceVersion, "idempotency key was already used for another request", "use the original request or a new idempotency key")
		}
		if persisted.Receipt.TransactionID != input.Receipt.TransactionID {
			return edit.WorkflowMutationReceipt{}, false, fmt.Errorf("workflow approval receipt transaction binding mismatch")
		}
		for _, envelope := range snapshot.ApprovalConsumptions {
			consumption, decodeErr := validateWorkflowApprovalConsumptionEnvelope(envelope, facts.Key, persisted.Receipt.TransactionID)
			if decodeErr == nil && consumption.ApprovalID == facts.ApprovalID && consumption.GrantID == facts.GrantID && consumption.PreviewHash == facts.PreviewHash {
				return persisted.Receipt, true, nil
			}
		}
		return edit.WorkflowMutationReceipt{}, false, fmt.Errorf("workflow approval receipt is missing its authoritative consumption")
	}
	return edit.WorkflowMutationReceipt{}, false, nil
}

func verifyWorkflowApplyFacts(state *workflowApprovalFile, facts WorkflowTrustedApplyFacts) (WorkflowApplyApprovalRecord, error) {
	grant, grantFound := state.Grants[facts.GrantID]
	approval, approvalFound := state.Approvals[facts.ApprovalID]
	if !approvalFound || !grantFound || grant.State != WorkflowApprovalActive || approval.State == WorkflowApprovalRevoked || approval.State == WorkflowApprovalExpired {
		return approval, workflowApprovalError(edit.ErrorApprovalRequired, facts.Key, "active workflow grant and approval are required", "request a new approval")
	}
	if approval.State == WorkflowApprovalConsumed {
		return approval, workflowApprovalError(edit.ErrorDuplicateTransaction, facts.Key, "workflow approval was already consumed", "use the existing transaction receipt")
	}
	if state.Key != facts.Key || grant.Grant.Key != facts.Key || approval.Approval.Key != facts.Key || grant.Grant.ActorID != facts.ActorID || approval.Approval.ActorID != facts.ActorID || grant.CanvasSessionID != facts.CanvasSessionID || approval.CanvasSessionID != facts.CanvasSessionID || grant.Grant.PlanHash != facts.PlanHash || approval.PlanHash != facts.PlanHash || grant.Grant.BaseSemanticHash != facts.BaseSemanticHash || approval.GrantID != facts.GrantID {
		return approval, workflowApprovalError(edit.ErrorApprovalRequired, facts.Key, "workflow approval identity scope does not match", "request an approval for the current workflow scope")
	}
	if approval.Approval.CandidateID != facts.CandidateID || approval.Approval.ChangeSetID != facts.ChangeSetID {
		return approval, workflowApprovalError(edit.ErrorStaleChangeSet, facts.Key, "workflow candidate or change set changed", "review the current candidate")
	}
	if approval.Approval.PreviewHash != facts.PreviewHash {
		return approval, workflowApprovalError(edit.ErrorPreviewMismatch, facts.Key, "workflow preview changed after approval", "review the current preview")
	}
	if approval.Approval.ExpectedRevision != facts.ExpectedRevision {
		return approval, workflowApprovalError(edit.ErrorStaleRevision, facts.Key, "workflow revision changed after approval", "reload and rebase")
	}
	allowed := make(map[edit.OperationKind]struct{}, len(grant.Grant.AllowedOps))
	for _, operation := range grant.Grant.AllowedOps {
		allowed[operation] = struct{}{}
	}
	for _, operation := range facts.Operations {
		if _, ok := allowed[operation]; !ok {
			return approval, workflowApprovalError(edit.ErrorApprovalScopeExceeded, facts.Key, "workflow operation is outside the grant", "request a broader grant")
		}
	}
	operationCount := facts.OperationCount
	if operationCount == 0 {
		operationCount = len(facts.Operations)
	}
	if operationCount < len(facts.Operations) || operationCount > grant.Grant.MaxOperations || facts.AddedNodeCount < 0 || facts.AddedNodeCount > grant.Grant.MaxAddedNodes || (facts.DeletesExisting && !grant.Grant.AllowDeleteExisting) || workflowRiskRank(facts.Risk) == 0 || workflowRiskRank(facts.Risk) > workflowRiskRank(grant.RiskScope.MaxRisk) {
		return approval, workflowApprovalError(edit.ErrorApprovalScopeExceeded, facts.Key, "workflow change exceeds the approved risk budget", "reduce the change or request a broader grant")
	}
	return approval, nil
}

func validateWorkflowGrantRecord(record WorkflowEditGrantRecord) error {
	if record.Grant.Key.Validate() != nil {
		return workflowApprovalError(edit.ErrorInvalidWorkflowKey, record.Grant.Key, "invalid workflow key", "use the authenticated workflow scope")
	}
	if record.State != WorkflowApprovalActive || record.IssuedAt.IsZero() || !record.Grant.ExpiresAt.After(record.IssuedAt) || !validWorkflowApprovalValue(record.Grant.ID, true) || !validWorkflowApprovalValue(record.Grant.ActorID, true) || !validWorkflowApprovalValue(record.CanvasSessionID, true) || !validWorkflowApprovalValue(record.Grant.PlanHash, false) || !validWorkflowApprovalValue(record.Grant.BaseSemanticHash, false) || len(record.Grant.AllowedOps) == 0 || record.Grant.MaxOperations < 1 || record.Grant.MaxAddedNodes < 0 || workflowRiskRank(record.RiskScope.MaxRisk) == 0 {
		return fmt.Errorf("%w: invalid workflow edit grant", ErrInvalid)
	}
	for _, operation := range record.Grant.AllowedOps {
		if !validWorkflowOperationKind(operation) {
			return fmt.Errorf("%w: invalid workflow edit grant", ErrInvalid)
		}
	}
	return nil
}

func validateWorkflowApplyApprovalRecord(record WorkflowApplyApprovalRecord) error {
	if record.Approval.Key.Validate() != nil {
		return workflowApprovalError(edit.ErrorInvalidWorkflowKey, record.Approval.Key, "invalid workflow key", "use the authenticated workflow scope")
	}
	values := []string{record.Approval.ID, record.Approval.ActorID, record.GrantID, record.CanvasSessionID, record.PlanHash, record.Approval.CandidateID, record.Approval.ChangeSetID, record.Approval.PreviewHash, record.Approval.ExpectedRevision}
	if record.State != WorkflowApprovalActive || record.Approval.ConsumedAt != nil || record.IssuedAt.IsZero() || !record.Approval.ExpiresAt.After(record.IssuedAt) {
		return fmt.Errorf("%w: invalid workflow apply approval", ErrInvalid)
	}
	for _, value := range values {
		if !validWorkflowApprovalValue(value, false) {
			return fmt.Errorf("%w: invalid workflow apply approval", ErrInvalid)
		}
	}
	return nil
}

func (s *WorkflowApprovalFileStore) withState(ctx context.Context, key edit.WorkflowKey, create bool, action func(*workflowApprovalFile) (bool, error)) error {
	if s.transactions == nil {
		return s.withStateLocal(ctx, key, create, action)
	}
	return s.transactions.withWorkflowCommitLock(ctx, key, func(locked *workflowstore.WorkflowNamespace) error {
		return s.withStateAndSnapshot(ctx, key, create, locked, action)
	})
}

func (s *WorkflowApprovalFileStore) withStateAndSnapshot(ctx context.Context, key edit.WorkflowKey, create bool, locked *workflowstore.WorkflowNamespace, action func(*workflowApprovalFile) (bool, error)) error {
	snapshot, found, err := workflowstore.LoadCurrentTransactionSnapshot(locked)
	if err != nil {
		return err
	}
	return s.withStateLocal(ctx, key, create, func(state *workflowApprovalFile) (bool, error) {
		dirty := false
		if found {
			var reconcileErr error
			dirty, reconcileErr = reconcileWorkflowApprovalConsumptions(state, snapshot)
			if reconcileErr != nil {
				return false, reconcileErr
			}
		}
		actionDirty, actionErr := action(state)
		return dirty || actionDirty, actionErr
	})
}

func (s *WorkflowApprovalFileStore) withStateLocal(ctx context.Context, key edit.WorkflowKey, create bool, action func(*workflowApprovalFile) (bool, error)) error {
	path, err := s.statePath(key)
	if err != nil {
		return err
	}
	lockValue, _ := workflowApprovalPathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	state, found, err := loadWorkflowApprovalFile(path)
	if err != nil {
		return err
	}
	if !found {
		if !create {
			return fmt.Errorf("%w: workflow approval state", ErrNotFound)
		}
		state = workflowApprovalFile{FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Key: key, Grants: map[string]WorkflowEditGrantRecord{}, Approvals: map[string]WorkflowApplyApprovalRecord{}}
	}
	dirty, actionErr := action(&state)
	if dirty {
		if err := saveWorkflowApprovalFile(path, state); err != nil {
			return err
		}
	}
	return actionErr
}

func reconcileWorkflowApprovalConsumptions(state *workflowApprovalFile, snapshot workflowstore.TransactionSnapshot) (bool, error) {
	if snapshot.Key != state.Key {
		return false, fmt.Errorf("workflow approval consumption key binding mismatch")
	}
	dirty := false
	for _, envelope := range snapshot.ApprovalConsumptions {
		value, err := validateWorkflowApprovalConsumptionEnvelope(envelope, state.Key, "")
		if err != nil {
			return false, err
		}
		approval, approvalFound := state.Approvals[value.ApprovalID]
		grant, grantFound := state.Grants[value.GrantID]
		if !approvalFound || !grantFound || approval.GrantID != value.GrantID || approval.CanvasSessionID != value.CanvasSessionID || approval.PlanHash != value.PlanHash || approval.Approval.ActorID != value.ActorID || approval.Approval.Key != value.Key || approval.Approval.CandidateID != value.CandidateID || approval.Approval.ChangeSetID != value.ChangeSetID || approval.Approval.PreviewHash != value.PreviewHash || approval.Approval.ExpectedRevision != value.ExpectedRevision || grant.Grant.Key != value.Key || grant.Grant.ActorID != value.ActorID || grant.CanvasSessionID != value.CanvasSessionID || grant.Grant.PlanHash != value.PlanHash || grant.Grant.BaseSemanticHash != value.BaseSemanticHash || value.ConsumedAt.Before(approval.IssuedAt) || !value.ConsumedAt.Before(approval.Approval.ExpiresAt) || !value.ConsumedAt.Before(grant.Grant.ExpiresAt) {
			return false, fmt.Errorf("workflow approval consumption scope binding mismatch")
		}
		allowed := make(map[edit.OperationKind]struct{}, len(grant.Grant.AllowedOps))
		for _, operation := range grant.Grant.AllowedOps {
			allowed[operation] = struct{}{}
		}
		for _, operation := range value.Operations {
			if _, ok := allowed[operation]; !ok {
				return false, fmt.Errorf("workflow approval consumption operation scope mismatch")
			}
		}
		if workflowRiskRank(value.Risk) > workflowRiskRank(grant.RiskScope.MaxRisk) {
			return false, fmt.Errorf("workflow approval consumption risk scope mismatch")
		}
		if approval.State == WorkflowApprovalConsumed {
			if approval.ConsumptionID != value.TransactionID || approval.Approval.ConsumedAt == nil || !approval.Approval.ConsumedAt.Equal(value.ConsumedAt) {
				return false, fmt.Errorf("workflow approval was consumed by a different transaction")
			}
			continue
		}
		when := value.ConsumedAt.UTC()
		approval.State, approval.ConsumptionID, approval.RevokedAt = WorkflowApprovalConsumed, value.TransactionID, nil
		approval.Approval.ConsumedAt = &when
		state.Approvals[value.ApprovalID] = approval
		if state.UpdatedAt.Before(when) {
			state.UpdatedAt = when
		}
		dirty = true
	}
	return dirty, nil
}

func (s *WorkflowApprovalFileStore) statePath(key edit.WorkflowKey) (string, error) {
	if strings.TrimSpace(s.Root) == "" {
		return "", fmt.Errorf("%w: workflow approval root is required", ErrInvalid)
	}
	if err := key.Validate(); err != nil {
		return "", workflowApprovalError(edit.ErrorInvalidWorkflowKey, key, "invalid workflow key", "use the authenticated workflow scope")
	}
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	return filepath.Join(s.Root, encode(key.TenantID), encode(key.WorkflowID), "approval-state.json"), nil
}

func loadWorkflowApprovalFile(path string) (workflowApprovalFile, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return workflowApprovalFile{}, false, nil
	}
	if err != nil {
		return workflowApprovalFile{}, false, err
	}
	var state workflowApprovalFile
	if edit.DecodeStrict(raw, &state) != nil || state.FormatVersion != 1 || state.SchemaVersion != edit.SchemaVersion || state.Key.Validate() != nil || state.Grants == nil || state.Approvals == nil {
		return workflowApprovalFile{}, false, fmt.Errorf("invalid persisted workflow approval state")
	}
	for id, record := range state.Grants {
		if id != record.Grant.ID || record.Grant.Key != state.Key || !validWorkflowApprovalState(record.State, false) {
			return workflowApprovalFile{}, false, fmt.Errorf("invalid persisted workflow approval state")
		}
	}
	for id, record := range state.Approvals {
		if id != record.Approval.ID || record.Approval.Key != state.Key || !validWorkflowApprovalState(record.State, true) || (record.State == WorkflowApprovalConsumed) != (record.Approval.ConsumedAt != nil) {
			return workflowApprovalFile{}, false, fmt.Errorf("invalid persisted workflow approval state")
		}
	}
	return state, true, nil
}

func saveWorkflowApprovalFile(path string, state workflowApprovalFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func expireWorkflowApprovals(state *workflowApprovalFile, at time.Time) int {
	count, when := 0, at.UTC()
	for id, record := range state.Grants {
		if record.State == WorkflowApprovalActive && !at.Before(record.Grant.ExpiresAt) {
			record.State = WorkflowApprovalExpired
			state.Grants[id], count = record, count+1
		}
	}
	for id, record := range state.Approvals {
		if record.State == WorkflowApprovalActive && !at.Before(record.Approval.ExpiresAt) {
			record.State = WorkflowApprovalExpired
			state.Approvals[id], count = record, count+1
		}
	}
	if count > 0 {
		state.UpdatedAt = when
	}
	return count
}

func (s *WorkflowApprovalFileStore) signBinding(payload workflowVerifiedPayload) [sha256.Size]byte {
	raw, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, s.sealKey[:])
	_, _ = mac.Write(raw)
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func (s *WorkflowApprovalFileStore) validBinding(binding VerifiedWorkflowApplyBinding) bool {
	expected := s.signBinding(binding.payload)
	return s.sealReady && hmac.Equal(binding.mac[:], expected[:])
}

func workflowApprovalConsumption(f WorkflowTrustedApplyFacts, transactionID string, at time.Time) WorkflowApprovalConsumption {
	operationCount := f.OperationCount
	if operationCount == 0 {
		operationCount = len(f.Operations)
	}
	return WorkflowApprovalConsumption{SchemaVersion: edit.SchemaVersion, TransactionID: transactionID, ApprovalID: f.ApprovalID, GrantID: f.GrantID, Key: f.Key, ActorID: f.ActorID, CanvasSessionID: f.CanvasSessionID, PlanHash: f.PlanHash, BaseSemanticHash: f.BaseSemanticHash, CandidateID: f.CandidateID, ChangeSetID: f.ChangeSetID, PreviewHash: f.PreviewHash, ExpectedRevision: f.ExpectedRevision, Operations: append([]edit.OperationKind(nil), f.Operations...), OperationCount: operationCount, Risk: f.Risk, ConsumedAt: at}
}

func workflowApprovalConsumptionEnvelope(value WorkflowApprovalConsumption) (workflowstore.TypedEnvelope, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return workflowstore.TypedEnvelope{}, err
	}
	envelope := workflowstore.TypedEnvelope{Kind: workflowApprovalConsumptionEnvelopeKind, SchemaVersion: edit.SchemaVersion, Payload: payload}
	if _, err := validateWorkflowApprovalConsumptionEnvelope(envelope, value.Key, value.TransactionID); err != nil {
		return workflowstore.TypedEnvelope{}, err
	}
	return envelope, nil
}

func validateWorkflowApprovalConsumptionEnvelope(envelope workflowstore.TypedEnvelope, key edit.WorkflowKey, transactionID string) (WorkflowApprovalConsumption, error) {
	var value WorkflowApprovalConsumption
	if envelope.Kind != workflowApprovalConsumptionEnvelopeKind || envelope.SchemaVersion != edit.SchemaVersion || edit.DecodeStrict(envelope.Payload, &value) != nil || value.SchemaVersion != edit.SchemaVersion || value.Key != key || value.Key.Validate() != nil || value.ConsumedAt.IsZero() || workflowRiskRank(value.Risk) == 0 || len(value.Operations) == 0 {
		return value, fmt.Errorf("%w: invalid workflow approval consumption envelope", ErrInvalid)
	}
	if transactionID != "" && value.TransactionID != transactionID {
		return value, fmt.Errorf("%w: workflow approval consumption transaction binding mismatch", ErrInvalid)
	}
	values := []struct {
		value    string
		pathSafe bool
	}{
		{value.TransactionID, true}, {value.ApprovalID, true}, {value.GrantID, true}, {value.ActorID, true}, {value.CanvasSessionID, true},
		{value.PlanHash, false}, {value.BaseSemanticHash, false}, {value.CandidateID, false}, {value.ChangeSetID, false}, {value.PreviewHash, false}, {value.ExpectedRevision, false},
	}
	for _, item := range values {
		if !validWorkflowApprovalValue(item.value, item.pathSafe) {
			return value, fmt.Errorf("%w: invalid workflow approval consumption envelope", ErrInvalid)
		}
	}
	seen := make(map[edit.OperationKind]struct{}, len(value.Operations))
	for _, operation := range value.Operations {
		if !validWorkflowOperationKind(operation) {
			return value, fmt.Errorf("%w: invalid workflow approval consumption envelope", ErrInvalid)
		}
		if _, duplicate := seen[operation]; duplicate {
			return value, fmt.Errorf("%w: duplicate workflow approval consumption operation", ErrInvalid)
		}
		seen[operation] = struct{}{}
	}
	if value.OperationCount == 0 {
		// Envelopes written before operationCount existed necessarily contained
		// one entry per operation because duplicate kinds were rejected.
		value.OperationCount = len(value.Operations)
	}
	if value.OperationCount < len(value.Operations) {
		return value, fmt.Errorf("%w: invalid workflow approval consumption operation count", ErrInvalid)
	}
	return value, nil
}

func workflowApprovalError(code edit.ErrorCode, key edit.WorkflowKey, message, retry string) error {
	copyKey := key
	return &edit.DomainError{Kind: "error", Code: code, Key: &copyKey, Message: message, RetryHint: retry}
}

func validWorkflowApprovalValue(value string, pathSafe bool) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	return !pathSafe || !strings.ContainsAny(value, "/\\")
}

func validWorkflowApprovalState(state WorkflowApprovalState, consumable bool) bool {
	return state == WorkflowApprovalActive || state == WorkflowApprovalRevoked || state == WorkflowApprovalExpired || (consumable && state == WorkflowApprovalConsumed)
}

func validWorkflowOperationKind(kind edit.OperationKind) bool {
	switch kind {
	case edit.OperationAddNode, edit.OperationUpdateNode, edit.OperationDeleteNode, edit.OperationAddEdge, edit.OperationUpdateEdge, edit.OperationDeleteEdge, edit.OperationUpdateWorkflow, edit.OperationUpdateLayout, edit.OperationAttachManualBinding:
		return true
	default:
		return false
	}
}

func workflowRiskRank(level WorkflowRiskLevel) int {
	switch level {
	case WorkflowRiskLow:
		return 1
	case WorkflowRiskMedium:
		return 2
	case WorkflowRiskHigh:
		return 3
	case WorkflowRiskCritical:
		return 4
	default:
		return 0
	}
}
