package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflowstore"
)

// WorkflowEditApplyRequest is model-visible input. Every identity field is
// compared with WorkflowEditTrustedApplyContext; missing trusted facts are
// never inferred from this request.
type WorkflowEditApplyRequest struct {
	edit.ApplyPatchRequest
	CandidateID       string `json:"candidateId"`
	CandidateRevision uint64 `json:"candidateRevision"`
	ActorID           string `json:"actorId"`
	CanvasSessionID   string `json:"canvasSessionId"`
}

// WorkflowEditTrustedApplyContext is created by the authenticated tool/runtime
// boundary and is deliberately not a JSON transport contract.
type WorkflowEditTrustedApplyContext struct {
	Key                                             edit.WorkflowKey
	ActorID, CanvasSessionID, CandidateID           string
	ChangeSetID, PreviewHash, ExpectedRevision      string
	GrantID, ApprovalID, PlanHash, BaseSemanticHash string
	ReferenceSnapshotHash                           string
	CandidateRevision                               uint64
}

type WorkflowTrustedApplySelector struct {
	Key             edit.WorkflowKey
	ActorID         string
	CanvasSessionID string
	ChangeSetID     string
	ApprovalID      string
}

func NewWorkflowEditMutationApplication(backend WorkflowEditReadBackend, access WorkflowEditActionAccessResolver, references WorkflowEditReferenceValidator, candidates WorkflowCandidateStore, approvals *WorkflowApprovalFileStore, commits *WorkflowCommitStore) (*WorkflowEditApplication, error) {
	if candidates == nil || approvals == nil || commits == nil || approvals.transactions != commits {
		return nil, ErrWorkflowEditConfiguration
	}
	application, err := NewWorkflowEditApplication(backend, access, references, candidates)
	if err != nil {
		return nil, err
	}
	application.approvals, application.commits = approvals, commits
	return application, nil
}

// ResolveTrustedApplyContext projects authoritative apply facts from the
// authenticated principal/session plus persisted Candidate/Grant/Approval.
// Tool metadata cannot provide or override any returned field.
func (a *WorkflowEditApplication) ResolveTrustedApplyContext(ctx context.Context, selector WorkflowTrustedApplySelector) (WorkflowEditTrustedApplyContext, error) {
	if a == nil || a.candidates == nil || a.approvals == nil || a.now == nil {
		return WorkflowEditTrustedApplyContext{}, ErrWorkflowEditConfiguration
	}
	if selector.Key.Validate() != nil {
		return WorkflowEditTrustedApplyContext{}, workflowApprovalError(edit.ErrorInvalidWorkflowKey, selector.Key, "invalid workflow key", "use the authenticated workflow scope")
	}
	for _, value := range []string{selector.ActorID, selector.CanvasSessionID, selector.ChangeSetID, selector.ApprovalID} {
		if !validWorkflowApprovalValue(value, false) {
			return WorkflowEditTrustedApplyContext{}, fmt.Errorf("%w: invalid trusted workflow apply selector", ErrInvalid)
		}
	}
	candidate, err := a.candidates.ResolveChangeSet(ctx, selector.Key, selector.ChangeSetID)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	approval, err := a.approvals.GetApplyApproval(ctx, selector.Key, selector.ApprovalID)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	grant, err := a.approvals.GetGrant(ctx, selector.Key, approval.GrantID)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	trusted := WorkflowEditTrustedApplyContext{Key: selector.Key, ActorID: selector.ActorID, CanvasSessionID: selector.CanvasSessionID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion, GrantID: approval.GrantID, ApprovalID: approval.Approval.ID, PlanHash: grant.Grant.PlanHash, BaseSemanticHash: candidate.Base.SemanticHash, ReferenceSnapshotHash: candidate.Validation.Basis.ReferenceSnapshotHash}
	if approval.Approval.ID != selector.ApprovalID || approval.Approval.Key != selector.Key || approval.Approval.ActorID != selector.ActorID || approval.CanvasSessionID != selector.CanvasSessionID || approval.Approval.CandidateID != candidate.CandidateID || approval.Approval.ChangeSetID != candidate.ID || approval.Approval.PreviewHash != candidate.PreviewHash || approval.Approval.ExpectedRevision != candidate.Base.ResourceVersion || approval.PlanHash != grant.Grant.PlanHash || grant.Grant.Key != selector.Key || grant.Grant.ActorID != selector.ActorID || grant.CanvasSessionID != selector.CanvasSessionID || grant.Grant.BaseSemanticHash != candidate.Base.SemanticHash || candidate.ActorID != selector.ActorID || candidate.CanvasSessionID != selector.CanvasSessionID {
		return WorkflowEditTrustedApplyContext{}, workflowApprovalError(edit.ErrorApprovalRequired, selector.Key, "persisted workflow approval scope does not match authenticated execution context", "review and approve the current candidate")
	}
	// A consumed approval may only authorize the exact idempotent replay that
	// Apply checks before any mutation gate. Returning its persisted facts here
	// does not make a new commit possible: a non-matching request continues to
	// the locked approval verification and is rejected as consumed.
	if approval.State == WorkflowApprovalConsumed {
		if approval.ConsumptionID == "" || approval.Approval.ConsumedAt == nil {
			return WorkflowEditTrustedApplyContext{}, workflowApprovalError(edit.ErrorApprovalRequired, selector.Key, "consumed workflow approval is missing its transaction binding", "read the persisted transaction receipt")
		}
		return trusted, nil
	}
	if _, err := a.candidates.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{Key: selector.Key, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash}, a.now().UTC()); err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	facts, err := workflowEditApplyFacts(candidate, trusted)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	if _, err := a.approvals.VerifyTrustedApplyFacts(ctx, facts, a.now().UTC()); err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	return trusted, nil
}

func (a *WorkflowEditApplication) Apply(ctx context.Context, trusted WorkflowEditTrustedApplyContext, request WorkflowEditApplyRequest) (result edit.WorkflowMutationReceipt, resultErr error) {
	startedAt := time.Now().UTC()
	receiptReplay := false
	var correlation WorkflowCandidateRevision
	defer func() {
		a.observeWorkflowEdit(ctx, WorkflowEditObservation{
			Operation: WorkflowEditObserveApply, Key: request.WorkflowKey, ActorID: trusted.ActorID,
			SessionID: correlation.AgentSessionID, TurnID: correlation.TurnID, ToolCallID: correlation.ToolCallID,
			CandidateID: request.CandidateID, CandidateRevision: request.CandidateRevision, ChangeSetID: request.ChangeSetID,
			TransactionID: result.TransactionID, ApprovalID: request.ApplyApprovalID, ReceiptReplay: receiptReplay,
			StartedAt: startedAt, Duration: time.Since(startedAt),
		}, resultErr)
	}()
	if a == nil || a.backend == nil || a.candidates == nil || a.approvals == nil || a.commits == nil || a.now == nil {
		return edit.WorkflowMutationReceipt{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowEditApplyIdentity(trusted, request); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	requestDigest, err := workflowQueryDigest([]any{edit.SchemaVersion, trusted, request})
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if receipt, found, err := a.workflowEditApplyReplay(ctx, request.WorkflowKey, request.IdempotencyKey, requestDigest); err != nil || found {
		receiptReplay = found
		return receipt, err
	}

	at := a.now().UTC()
	binding := WorkflowCandidateApplyBinding{Key: request.WorkflowKey, CandidateID: request.CandidateID, CandidateRevision: request.CandidateRevision, ChangeSetID: request.ChangeSetID, PreviewHash: request.PreviewHash}
	candidate, err := a.candidates.CheckApplyEligibility(ctx, binding, at)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	correlation = candidate
	if err := validateWorkflowEditApplyCandidate(candidate, trusted); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	expectedPreviewHash, err := workflowPreviewHash(candidate.Key, candidate.CandidateRevision, candidate.ParentChangeSetID, candidate.NormalizedOperations, candidate.Validation.Basis)
	if err != nil || expectedPreviewHash != candidate.PreviewHash {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorPreviewMismatch, candidate.Key, expectedPreviewHash, candidate.PreviewHash, "candidate preview binding is invalid", "review a newly generated preview")
	}

	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if snapshot.Revision != candidate.Base {
		return edit.WorkflowMutationReceipt{}, workflowPreviewConflict(request.WorkflowKey, candidate.Base, snapshot.Revision, []string{"/resourceVersion"}, nil)
	}
	applied, err := applyWorkflowPreviewOperations(snapshot.Graph, candidate.NormalizedOperations)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	applied.Graph.UI = withoutGraphResourceVersion(applied.Graph.UI)
	gate, err := a.validateSnapshot(ctx, snapshot, applied.Graph, request.ExpectedRevision)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	basis, err := a.workflowPreviewValidationBasis(ctx, snapshot, applied.Graph)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if basis != candidate.Validation.Basis {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorValidationBasisStale, request.WorkflowKey, basis.ResourceVersion, candidate.Validation.Basis.ResourceVersion, "workflow validation basis changed after preview", "refresh context and review a new preview")
	}
	if !gate.Validation.Valid || !gate.Compiled {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorPublishGateFailed, request.WorkflowKey, snapshot.Revision.ResourceVersion, request.ExpectedRevision, "workflow failed the pre-commit validation or compile gate", "repair the candidate and preview again")
	}
	compiled, err := a.backend.CompileWorkflow(ctx, request.WorkflowKey, applied.Graph)
	if err != nil || compiled == nil || strings.TrimSpace(compiled.YAML) == "" {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorPublishGateFailed, request.WorkflowKey, snapshot.Revision.ResourceVersion, request.ExpectedRevision, "workflow failed the pre-commit compile gate", "repair the candidate and preview again")
	}
	compiledHashes, err := workflowYAMLGraphHashes([]byte(compiled.YAML))
	if err != nil || compiledHashes.SemanticHash != gate.Revision.SemanticHash || compiledHashes.LayoutHash != gate.Revision.LayoutHash {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorPublishGateFailed, request.WorkflowKey, snapshot.Revision.ResourceVersion, request.ExpectedRevision, "compiled workflow does not match the validated graph", "refresh action schemas and preview again")
	}
	current, err := a.commits.Snapshot(ctx, request.WorkflowKey)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if current.ResourceVersion != snapshot.Revision.ResourceVersion {
		return edit.WorkflowMutationReceipt{}, workflowCommitDomainError(edit.ErrorStaleRevision, request.WorkflowKey, current.ResourceVersion, snapshot.Revision.ResourceVersion, "workflow changed before commit", "reload and rebase")
	}
	currentHashes, err := workflowYAMLGraphHashes(current.GraphPayload)
	if err != nil || currentHashes.SemanticHash != snapshot.Revision.SemanticHash || currentHashes.LayoutHash != snapshot.Revision.LayoutHash {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: read backend and commit snapshot disagree", ErrWorkflowEditSnapshot)
	}
	historyAction, err := a.resolveWorkflowHistoryActionMetadata(ctx, candidate)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}

	commitInput, err := workflowEditApplyCommitInput(request, trusted, candidate, snapshot, current, applied, gate, basis, historyAction, compiled.YAML, requestDigest, at)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	commitAt := a.now().UTC()
	if _, err := a.candidates.CheckApplyEligibility(ctx, binding, commitAt); err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if receipt, found, replayErr := a.workflowEditApplyReplay(ctx, request.WorkflowKey, request.IdempotencyKey, requestDigest); replayErr != nil || found {
		receiptReplay = found
		return receipt, replayErr
	}
	facts, err := workflowEditApplyFacts(candidate, trusted)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	approvalBinding, err := a.approvals.VerifyTrustedApplyFacts(ctx, facts, commitAt)
	if err != nil {
		var domain *edit.DomainError
		if errors.As(err, &domain) && domain.Code == edit.ErrorDuplicateTransaction {
			if receipt, found, replayErr := a.workflowEditApplyReplay(ctx, request.WorkflowKey, request.IdempotencyKey, requestDigest); replayErr != nil || found {
				return receipt, replayErr
			}
		}
		return edit.WorkflowMutationReceipt{}, err
	}
	receipt, err := a.approvals.ConsumeWithWorkflowCommit(ctx, approvalBinding, commitAt, commitInput)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	// A layout-only transaction may retain a lifecycle gate only through a new
	// server artifact. Reissue is deliberately post-commit: failure cannot turn
	// an already durable commit into an apparent Apply failure or trigger a
	// duplicate retry. Ineligible, stale, or unproven transactions remain in
	// verifying state and must run validation/dry-run again.
	if workflowLayoutOnlyTouchedPaths(applied.Diff.TouchedPaths) {
		if reissued, reissueErr := a.ReissueLayoutOnlyLifecycleGate(ctx, request.WorkflowKey, receipt.TransactionID); reissueErr == nil {
			return reissued, nil
		}
	}
	return receipt, nil
}

func validateWorkflowEditApplyIdentity(trusted WorkflowEditTrustedApplyContext, request WorkflowEditApplyRequest) error {
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || trusted.Key.Validate() != nil || request.WorkflowKey != trusted.Key || request.CandidateRevision == 0 || request.CandidateRevision != trusted.CandidateRevision || strings.TrimSpace(request.Reason) == "" {
		return fmt.Errorf("%w: invalid workflow apply identity", ErrInvalid)
	}
	pairs := [][2]string{{request.ActorID, trusted.ActorID}, {request.CanvasSessionID, trusted.CanvasSessionID}, {request.CandidateID, trusted.CandidateID}, {request.ChangeSetID, trusted.ChangeSetID}, {request.PreviewHash, trusted.PreviewHash}, {request.ExpectedRevision, trusted.ExpectedRevision}, {request.CandidateGrantID, trusted.GrantID}, {request.ApplyApprovalID, trusted.ApprovalID}}
	for _, pair := range pairs {
		if !validWorkflowApprovalValue(pair[0], false) || pair[0] != pair[1] {
			return workflowApprovalError(edit.ErrorApprovalRequired, request.WorkflowKey, "workflow apply identity does not match trusted execution context", "review and approve the current candidate")
		}
	}
	for _, value := range []string{request.IdempotencyKey, trusted.PlanHash, trusted.BaseSemanticHash, trusted.ReferenceSnapshotHash} {
		if !validWorkflowApprovalValue(value, false) {
			return fmt.Errorf("%w: invalid workflow apply binding", ErrInvalid)
		}
	}
	return nil
}

func validateWorkflowEditApplyCandidate(candidate WorkflowCandidateRevision, trusted WorkflowEditTrustedApplyContext) error {
	if candidate.Key != trusted.Key || candidate.CandidateID != trusted.CandidateID || candidate.CandidateRevision != trusted.CandidateRevision || candidate.ID != trusted.ChangeSetID || candidate.PreviewHash != trusted.PreviewHash || candidate.Base.ResourceVersion != trusted.ExpectedRevision || candidate.Base.SemanticHash != trusted.BaseSemanticHash || candidate.ActorID != trusted.ActorID || candidate.CanvasSessionID != trusted.CanvasSessionID || candidate.Validation.Basis.ReferenceSnapshotHash != trusted.ReferenceSnapshotHash {
		return workflowApprovalError(edit.ErrorApprovalRequired, trusted.Key, "candidate does not match trusted apply scope", "review and approve the current candidate")
	}
	if !candidate.Validation.Valid {
		return workflowPreviewDomainError(edit.ErrorPublishGateFailed, trusted.Key, candidate.Base.ResourceVersion, trusted.ExpectedRevision, "candidate preview is not valid", "repair the candidate before applying")
	}
	return nil
}

func (a *WorkflowEditApplication) workflowEditApplyReplay(ctx context.Context, key edit.WorkflowKey, idempotencyKey, requestDigest string) (edit.WorkflowMutationReceipt, bool, error) {
	snapshot, err := a.commits.Snapshot(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return edit.WorkflowMutationReceipt{}, false, nil
	}
	if err != nil {
		return edit.WorkflowMutationReceipt{}, false, err
	}
	hash := workflowCommitIdempotencyHash(idempotencyKey)
	for _, persisted := range snapshot.Receipts {
		if persisted.IdempotencyKeyHash != hash {
			continue
		}
		if persisted.RequestDigest != requestDigest {
			return edit.WorkflowMutationReceipt{}, false, workflowCommitDomainError(edit.ErrorIdempotencyKeyReused, key, snapshot.ResourceVersion, "", "idempotency key was already used for another request", "use the original request or a new idempotency key")
		}
		return persisted.Receipt, true, nil
	}
	return edit.WorkflowMutationReceipt{}, false, nil
}

func workflowEditApplyFacts(candidate WorkflowCandidateRevision, trusted WorkflowEditTrustedApplyContext) (WorkflowTrustedApplyFacts, error) {
	var risk WorkflowEditPreviewRisk
	if edit.DecodeStrict(candidate.Risk, &risk) != nil || risk.Level == "" {
		return WorkflowTrustedApplyFacts{}, fmt.Errorf("invalid persisted workflow candidate risk")
	}
	operationSet := make(map[edit.OperationKind]struct{}, len(candidate.NormalizedOperations))
	for _, operation := range candidate.NormalizedOperations {
		operationSet[operation.Value.Kind()] = struct{}{}
	}
	operations := make([]edit.OperationKind, 0, len(operationSet))
	for operation := range operationSet {
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i] < operations[j] })
	return WorkflowTrustedApplyFacts{Key: trusted.Key, ActorID: trusted.ActorID, CanvasSessionID: trusted.CanvasSessionID, GrantID: trusted.GrantID, ApprovalID: trusted.ApprovalID, PlanHash: trusted.PlanHash, BaseSemanticHash: trusted.BaseSemanticHash, CandidateID: trusted.CandidateID, ChangeSetID: trusted.ChangeSetID, PreviewHash: trusted.PreviewHash, ExpectedRevision: trusted.ExpectedRevision, Operations: operations, OperationCount: len(candidate.NormalizedOperations), AddedNodeCount: risk.AddedNodeCount, DeletesExisting: risk.DeletesExisting, Risk: risk.Level}, nil
}

func workflowEditApplyCommitInput(request WorkflowEditApplyRequest, trusted WorkflowEditTrustedApplyContext, candidate WorkflowCandidateRevision, snapshot WorkflowEditSnapshot, current workflowstore.TransactionSnapshot, applied edit.ApplyResult, gate WorkflowEditValidationResult, basis edit.ValidationBasis, historyAction workflowHistoryActionMetadata, yaml, requestDigest string, at time.Time) (WorkflowCommitInput, error) {
	nextVersion, err := workflowQueryDigest([]any{snapshot.Revision.ResourceVersion, requestDigest})
	if err != nil {
		return WorkflowCommitInput{}, err
	}
	token := strings.TrimPrefix(requestDigest, "sha256:")
	committedBasis := basis
	committedBasis.ResourceVersion = nextVersion
	validation := workflowPreviewValidation(gate, committedBasis)
	receipt := edit.WorkflowMutationReceipt{SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: "txn-" + token[:32], ChangeSetID: candidate.ID, Key: request.WorkflowKey, Status: "verifying", RevisionBefore: snapshot.Revision, RevisionAfter: &edit.Revision{ResourceVersion: nextVersion, SemanticHash: gate.Revision.SemanticHash, LayoutHash: gate.Revision.LayoutHash}, Delta: applied.Diff.Delta, Validation: validation, CheckpointID: "checkpoint-" + token[:24], InverseTransactionID: "inverse-" + token[:24], AuditID: "audit-" + token[:24]}
	risk := decodeWorkflowPreviewRisk(candidate.Risk)
	channel, sessionID := "mcp", candidate.AgentSessionID
	if strings.TrimSpace(candidate.ToolCallID) == "" {
		channel, sessionID = "manual", candidate.CanvasSessionID
	}
	audit, err := BuildWorkflowEditAuditEnvelope(WorkflowEditAuditInput{
		AuditID: receipt.AuditID, Key: request.WorkflowKey, Actor: candidate.Actor, Channel: channel, Operation: "apply",
		UserID: trusted.ActorID, SessionID: sessionID, TurnID: candidate.TurnID, ToolCallID: candidate.ToolCallID,
		ChangeSetID: candidate.ID, TransactionID: receipt.TransactionID, BeforeRevision: snapshot.Revision,
		AfterRevision: *receipt.RevisionAfter, Details: workflowEditAuditDetails(risk, trusted.ApprovalID, request.Reason, receipt.Status), OccurredAt: at,
	})
	if err != nil {
		return WorkflowCommitInput{}, err
	}
	payloads := map[string]any{
		"history": workflowEditHistoryArtifact{
			TransactionID: receipt.TransactionID, ChangeSetID: candidate.ID,
			TouchedPaths: applied.Diff.TouchedPaths, ActionKind: historyAction.ActionKind,
			TargetTransactionIDs: append([]string(nil), historyAction.TargetTransactionIDs...),
		},
		"import_result": map[string]any{"applied": false}, "rollback_result": map[string]any{"applied": false},
		"checkpoint": map[string]any{"id": receipt.CheckpointID, "resourceVersion": snapshot.Revision.ResourceVersion, "graph": string(current.GraphPayload)},
		"inverse":    applied.Inverse, "approval_consume": map[string]any{"approvalId": trusted.ApprovalID, "pending": true},
		"event":  map[string]any{"type": "workflow.transaction.applied", "transactionId": receipt.TransactionID, "resourceVersion": nextVersion, "occurredAt": at},
		"outbox": map[string]any{"transactionId": receipt.TransactionID, "status": "pending"},
	}
	envelopes := make(map[string]workflowstore.TypedEnvelope, len(payloads))
	for kind, payload := range payloads {
		envelopes[kind], err = visualWriteEnvelope(kind, payload)
		if err != nil {
			return WorkflowCommitInput{}, err
		}
	}
	envelopes["audit"] = audit
	return WorkflowCommitInput{Key: request.WorkflowKey, ExpectedResourceVersion: snapshot.Revision.ResourceVersion, IdempotencyKey: request.IdempotencyKey, RequestDigest: requestDigest, GraphPayload: []byte(yaml), Meta: current.Meta, History: envelopes["history"], ImportResult: envelopes["import_result"], RollbackResult: envelopes["rollback_result"], Receipt: receipt, ValidationBasis: committedBasis, Checkpoint: envelopes["checkpoint"], Inverse: envelopes["inverse"], ApprovalConsume: envelopes["approval_consume"], Audit: envelopes["audit"], Events: []workflowstore.TypedEnvelope{envelopes["event"]}, Outbox: []workflowstore.TypedEnvelope{envelopes["outbox"]}}, nil
}
