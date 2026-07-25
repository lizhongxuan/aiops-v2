package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowApprovalStorePersistsAndConsumesOnlyAtCommit(t *testing.T) {
	root := t.TempDir()
	original, facts, now := newWorkflowApprovalFixture(t, root, "approval-1")
	path, err := original.statePath(facts.Key)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("approval state permissions=%v", info.Mode().Perm())
	}
	oldBinding, err := original.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	restarted := NewWorkflowApprovalFileStore(root)
	if err := restarted.ConsumeWithCommit(context.Background(), oldBinding, now.Add(time.Minute), "txn-forged", func(WorkflowApprovalConsumption) error { return nil }); err == nil {
		t.Fatal("process-local binding survived restart")
	}
	binding, err := restarted.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify after restart: %v", err)
	}
	if binding.ApprovalID() != facts.ApprovalID || binding.WorkflowKey() != facts.Key {
		t.Fatalf("binding lost server-owned scope: %#v", binding)
	}
	before, err := restarted.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || before.State != WorkflowApprovalActive || before.Approval.ConsumedAt != nil {
		t.Fatalf("verify must not consume approval: record=%#v err=%v", before, err)
	}

	var committed WorkflowApprovalConsumption
	err = restarted.ConsumeWithCommit(context.Background(), binding, now.Add(2*time.Minute), "txn-1", func(value WorkflowApprovalConsumption) error {
		committed = value
		return nil
	})
	if err != nil {
		t.Fatalf("consume with commit: %v", err)
	}
	if committed.TransactionID != "txn-1" || committed.ApprovalID != facts.ApprovalID || committed.PreviewHash != facts.PreviewHash {
		t.Fatalf("commit callback received incomplete binding: %#v", committed)
	}

	afterRestart := NewWorkflowApprovalFileStore(root)
	after, err := afterRestart.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || after.State != WorkflowApprovalConsumed || after.Approval.ConsumedAt == nil || after.ConsumptionID != "txn-1" {
		t.Fatalf("consumption was not durable: record=%#v err=%v", after, err)
	}
}

func TestWorkflowApprovalStoreCommitFailureLeavesApprovalActive(t *testing.T) {
	store, facts, now := newWorkflowApprovalFixture(t, t.TempDir(), "approval-retry")
	binding, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("wal unavailable")
	if err := store.ConsumeWithCommit(context.Background(), binding, now.Add(2*time.Minute), "txn-failed", func(WorkflowApprovalConsumption) error { return want }); !errors.Is(err, want) {
		t.Fatalf("consume error=%v, want callback error", err)
	}
	record, err := store.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || record.State != WorkflowApprovalActive || record.Approval.ConsumedAt != nil {
		t.Fatalf("failed commit consumed approval: record=%#v err=%v", record, err)
	}
	if err := store.ConsumeWithCommit(context.Background(), binding, now.Add(3*time.Minute), "txn-retry", func(WorkflowApprovalConsumption) error { return nil }); err != nil {
		t.Fatalf("approval was not retryable: %v", err)
	}
}

func TestWorkflowApprovalCommitLedgerRecoversConsumptionAfterPointerCrash(t *testing.T) {
	t.Setenv("AIOPS_WORKFLOW_FAILPOINT_TEST", "1")
	approvalRoot, commitRoot := t.TempDir(), t.TempDir()
	_, facts, now := newWorkflowApprovalFixture(t, approvalRoot, "approval-ledger")
	seed, err := NewWorkflowCommitStore(commitRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Commit(context.Background(), workflowCommitInputFixture(facts.Key, "", facts.ExpectedRevision, "transaction-seed", "idem-seed", "digest-seed")); err != nil {
		t.Fatal(err)
	}
	failing, err := NewWorkflowCommitStore(commitRoot, withWorkflowCommitFailpointForTest(failOnceAt(workflowCommitAfterCurrentPointer, errors.New("crash after pointer"))))
	if err != nil {
		t.Fatal(err)
	}
	store := NewWorkflowApprovalFileStore(approvalRoot, failing)
	binding, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(facts.Key, facts.ExpectedRevision, "rv-8", "transaction-apply", "idem-apply", "digest-apply")
	input.Receipt.ChangeSetID = facts.ChangeSetID
	if _, err := store.ConsumeWithWorkflowCommit(context.Background(), binding, now.Add(2*time.Minute), input); err == nil {
		t.Fatal("pointer crash unexpectedly reported success")
	}
	plain := NewWorkflowApprovalFileStore(approvalRoot)
	before, err := plain.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || before.State != WorkflowApprovalActive {
		t.Fatalf("sidecar should still be active before recovery: %#v, %v", before, err)
	}
	restarted, err := NewWorkflowCommitStore(commitRoot)
	if err != nil || restarted.Recover(context.Background(), facts.Key) != nil {
		t.Fatalf("recover commit ledger: %v", err)
	}
	reconciled := NewWorkflowApprovalFileStore(approvalRoot, restarted)
	after, err := reconciled.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || after.State != WorkflowApprovalConsumed || after.ConsumptionID != input.Receipt.TransactionID {
		t.Fatalf("authoritative ledger was not projected: %#v, %v", after, err)
	}
	if _, err := reconciled.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(3*time.Minute)); workflowCommitErrorCode(err) != edit.ErrorDuplicateTransaction {
		t.Fatalf("consumed approval verified again: %v", err)
	}
	snapshot, err := restarted.Snapshot(context.Background(), facts.Key)
	if err != nil || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("approval ledger = %#v, %v", snapshot.ApprovalConsumptions, err)
	}
}

func TestWorkflowApprovalConfiguredStoresSerializeRevokeAndAtomicApply(t *testing.T) {
	approvalRoot, commitRoot := t.TempDir(), t.TempDir()
	_, facts, now := newWorkflowApprovalFixture(t, approvalRoot, "approval-cross-process-race")
	firstCommit, err := NewWorkflowCommitStore(commitRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondCommit, err := NewWorkflowCommitStore(commitRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCommit.Commit(context.Background(), workflowCommitInputFixture(facts.Key, "", facts.ExpectedRevision, "transaction-seed", "idem-seed", "digest-seed")); err != nil {
		t.Fatal(err)
	}
	applyStore := NewWorkflowApprovalFileStore(approvalRoot, firstCommit)
	revokeStore := NewWorkflowApprovalFileStore(approvalRoot, secondCommit)
	binding, err := applyStore.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(facts.Key, facts.ExpectedRevision, "rv-8", "transaction-apply", "idem-apply", "digest-apply")
	input.Receipt.ChangeSetID = facts.ChangeSetID

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := applyStore.ConsumeWithWorkflowCommit(context.Background(), binding, now.Add(2*time.Minute), input)
		results <- err
	}()
	go func() {
		<-start
		results <- revokeStore.RevokeApplyApproval(context.Background(), facts.Key, facts.ApprovalID, now.Add(2*time.Minute))
	}()
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("atomic apply/revoke winners: first=%v second=%v", first, second)
	}
	record, err := applyStore.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || (record.State != WorkflowApprovalConsumed && record.State != WorkflowApprovalRevoked) {
		t.Fatalf("atomic apply/revoke state = %#v, %v", record, err)
	}
	snapshot, err := firstCommit.Snapshot(context.Background(), facts.Key)
	if err != nil {
		t.Fatal(err)
	}
	if record.State == WorkflowApprovalConsumed && (snapshot.ResourceVersion != "rv-8" || len(snapshot.ApprovalConsumptions) != 1) {
		t.Fatalf("consumed approval is not bound to atomic commit: %#v", snapshot)
	}
	if record.State == WorkflowApprovalRevoked && (snapshot.ResourceVersion != facts.ExpectedRevision || len(snapshot.ApprovalConsumptions) != 0) {
		t.Fatalf("revoked approval allowed graph commit: %#v", snapshot)
	}
}

func TestWorkflowApprovalStoreConcurrentDoubleConsumeHasOneWinner(t *testing.T) {
	store, facts, now := newWorkflowApprovalFixture(t, t.TempDir(), "approval-race")
	binding, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var successes, callbacks atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			err := store.ConsumeWithCommit(context.Background(), binding, now.Add(2*time.Minute), "txn-"+string(rune('a'+index)), func(WorkflowApprovalConsumption) error {
				callbacks.Add(1)
				return nil
			})
			if err == nil {
				successes.Add(1)
				return
			}
			assertWorkflowApprovalCode(t, err, edit.ErrorDuplicateTransaction)
		}(index)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 || callbacks.Load() != 1 {
		t.Fatalf("successes=%d callbacks=%d, want exactly one", successes.Load(), callbacks.Load())
	}
}

func TestWorkflowApprovalStoreConcurrentRevokeOrConsumeHasOneTerminalWinner(t *testing.T) {
	store, facts, now := newWorkflowApprovalFixture(t, t.TempDir(), "approval-revoke-race")
	binding, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- store.RevokeApplyApproval(context.Background(), facts.Key, facts.ApprovalID, now.Add(2*time.Minute))
	}()
	go func() {
		<-start
		results <- store.ConsumeWithCommit(context.Background(), binding, now.Add(2*time.Minute), "txn-race", func(WorkflowApprovalConsumption) error { return nil })
	}()
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("revoke/consume winners: first=%v second=%v", first, second)
	}
	record, err := store.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || (record.State != WorkflowApprovalRevoked && record.State != WorkflowApprovalConsumed) {
		t.Fatalf("unexpected terminal state: record=%#v err=%v", record, err)
	}
}

func TestWorkflowApprovalStoreRevalidatesEveryTrustedBindingField(t *testing.T) {
	store, facts, now := newWorkflowApprovalFixture(t, t.TempDir(), "approval-scope")
	tests := []struct {
		name string
		edit func(*WorkflowTrustedApplyFacts)
		code edit.ErrorCode
	}{
		{"tenant", func(f *WorkflowTrustedApplyFacts) { f.Key.TenantID = "other" }, edit.ErrorApprovalRequired},
		{"workflow", func(f *WorkflowTrustedApplyFacts) { f.Key.WorkflowID = "other" }, edit.ErrorApprovalRequired},
		{"actor", func(f *WorkflowTrustedApplyFacts) { f.ActorID = "other" }, edit.ErrorApprovalRequired},
		{"session", func(f *WorkflowTrustedApplyFacts) { f.CanvasSessionID = "other" }, edit.ErrorApprovalRequired},
		{"plan", func(f *WorkflowTrustedApplyFacts) { f.PlanHash = workflowApprovalHash("other-plan") }, edit.ErrorApprovalRequired},
		{"base semantic", func(f *WorkflowTrustedApplyFacts) { f.BaseSemanticHash = workflowApprovalHash("other-base") }, edit.ErrorApprovalRequired},
		{"candidate", func(f *WorkflowTrustedApplyFacts) { f.CandidateID = "other" }, edit.ErrorStaleChangeSet},
		{"change set", func(f *WorkflowTrustedApplyFacts) { f.ChangeSetID = "other" }, edit.ErrorStaleChangeSet},
		{"preview", func(f *WorkflowTrustedApplyFacts) { f.PreviewHash = workflowApprovalHash("other-preview") }, edit.ErrorPreviewMismatch},
		{"revision", func(f *WorkflowTrustedApplyFacts) { f.ExpectedRevision = "rv-other" }, edit.ErrorStaleRevision},
		{"operation", func(f *WorkflowTrustedApplyFacts) { f.Operations = append(f.Operations, edit.OperationDeleteNode) }, edit.ErrorApprovalScopeExceeded},
		{"operation budget", func(f *WorkflowTrustedApplyFacts) {
			f.Operations = []edit.OperationKind{edit.OperationAddNode, edit.OperationUpdateNode, edit.OperationAddEdge, edit.OperationUpdateEdge}
		}, edit.ErrorApprovalScopeExceeded},
		{"added node budget", func(f *WorkflowTrustedApplyFacts) { f.AddedNodeCount = 3 }, edit.ErrorApprovalScopeExceeded},
		{"delete", func(f *WorkflowTrustedApplyFacts) { f.DeletesExisting = true }, edit.ErrorApprovalScopeExceeded},
		{"risk", func(f *WorkflowTrustedApplyFacts) { f.Risk = WorkflowRiskHigh }, edit.ErrorApprovalScopeExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := facts
			changed.Operations = append([]edit.OperationKind(nil), facts.Operations...)
			test.edit(&changed)
			_, err := store.VerifyTrustedApplyFacts(context.Background(), changed, now.Add(time.Minute))
			assertWorkflowApprovalCode(t, err, test.code)
		})
	}
}

func TestWorkflowApprovalOperationKindsAreDeduplicatedWithoutRelaxingActualCount(t *testing.T) {
	store, facts, now := newWorkflowApprovalFixture(t, t.TempDir(), "approval-operation-count")
	facts.Operations = []edit.OperationKind{edit.OperationUpdateNode}
	facts.OperationCount = 4
	if _, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute)); err == nil {
		t.Fatal("deduplicated operation kinds relaxed the grant's actual operation budget")
	} else {
		assertWorkflowApprovalCode(t, err, edit.ErrorApprovalScopeExceeded)
	}

	facts.OperationCount = 3
	binding, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify exact operation budget: %v", err)
	}
	consumption := workflowApprovalConsumption(binding.payload.Facts, "transaction-operation-count", now.Add(2*time.Minute))
	if consumption.OperationCount != 3 || len(consumption.Operations) != 1 {
		t.Fatalf("consumption operation accounting = %#v", consumption)
	}
	envelope, err := workflowApprovalConsumptionEnvelope(consumption)
	if err != nil {
		t.Fatalf("encode counted consumption: %v", err)
	}
	decoded, err := validateWorkflowApprovalConsumptionEnvelope(envelope, facts.Key, consumption.TransactionID)
	if err != nil || decoded.OperationCount != 3 {
		t.Fatalf("decode counted consumption = %#v, %v", decoded, err)
	}

	legacy := consumption
	legacy.TransactionID = "transaction-legacy-count"
	legacy.OperationCount = 0
	legacyEnvelope, err := workflowApprovalConsumptionEnvelope(legacy)
	if err != nil {
		t.Fatalf("encode legacy consumption: %v", err)
	}
	decodedLegacy, err := validateWorkflowApprovalConsumptionEnvelope(legacyEnvelope, facts.Key, legacy.TransactionID)
	if err != nil || decodedLegacy.OperationCount != len(legacy.Operations) {
		t.Fatalf("decode legacy consumption = %#v, %v", decodedLegacy, err)
	}
}

func TestWorkflowApprovalStorePersistsExpiryAndRejectsRevokedGrant(t *testing.T) {
	root := t.TempDir()
	store, facts, now := newWorkflowApprovalFixture(t, root, "approval-expire")
	if _, err := store.VerifyTrustedApplyFacts(context.Background(), facts, now.Add(31*time.Minute)); err == nil {
		t.Fatal("expired approval was accepted")
	}
	restarted := NewWorkflowApprovalFileStore(root)
	record, err := restarted.GetApplyApproval(context.Background(), facts.Key, facts.ApprovalID)
	if err != nil || record.State != WorkflowApprovalExpired {
		t.Fatalf("expiry not persisted: record=%#v err=%v", record, err)
	}

	store2, facts2, now2 := newWorkflowApprovalFixture(t, t.TempDir(), "approval-revoked-grant")
	if err := store2.RevokeGrant(context.Background(), facts2.Key, facts2.GrantID, now2.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store2.VerifyTrustedApplyFacts(context.Background(), facts2, now2.Add(2*time.Minute)); err == nil {
		t.Fatal("approval backed by a revoked grant was accepted")
	}
}

func TestWorkflowApprovalStoreDoesNotPersistOrEchoRejectedCredentialMaterial(t *testing.T) {
	root := t.TempDir()
	store := NewWorkflowApprovalFileStore(root)
	secret := "credential-sentinel-never-persist"
	now := time.Now().UTC()
	record := validWorkflowGrantRecord(now)
	record.Grant.PlanHash = secret + "\n"
	err := store.IssueGrant(context.Background(), record)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe validation error: %v", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected grant created persistence entries: %v", entries)
	}
}

func newWorkflowApprovalFixture(t *testing.T, root, approvalID string) (*WorkflowApprovalFileStore, WorkflowTrustedApplyFacts, time.Time) {
	t.Helper()
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	store := NewWorkflowApprovalFileStore(root)
	grant := validWorkflowGrantRecord(now)
	if err := store.IssueGrant(context.Background(), grant); err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	approval := WorkflowApplyApprovalRecord{
		Approval: edit.WorkflowApplyApproval{ID: approvalID, ActorID: grant.Grant.ActorID, Key: grant.Grant.Key, CandidateID: "candidate-1", ChangeSetID: "change-1", PreviewHash: workflowApprovalHash("preview"), ExpectedRevision: "rv-7", ExpiresAt: now.Add(30 * time.Minute)},
		GrantID:  grant.Grant.ID, CanvasSessionID: grant.CanvasSessionID, PlanHash: grant.Grant.PlanHash, State: WorkflowApprovalActive, IssuedAt: now,
	}
	if err := store.IssueApplyApproval(context.Background(), approval); err != nil {
		t.Fatalf("issue approval: %v", err)
	}
	facts := WorkflowTrustedApplyFacts{
		Key: grant.Grant.Key, ActorID: grant.Grant.ActorID, CanvasSessionID: grant.CanvasSessionID,
		GrantID: grant.Grant.ID, ApprovalID: approvalID, PlanHash: grant.Grant.PlanHash, BaseSemanticHash: grant.Grant.BaseSemanticHash,
		CandidateID: approval.Approval.CandidateID, ChangeSetID: approval.Approval.ChangeSetID, PreviewHash: approval.Approval.PreviewHash,
		ExpectedRevision: approval.Approval.ExpectedRevision, Operations: []edit.OperationKind{edit.OperationAddNode, edit.OperationUpdateNode}, AddedNodeCount: 1, Risk: WorkflowRiskMedium,
	}
	return store, facts, now
}

func validWorkflowGrantRecord(now time.Time) WorkflowEditGrantRecord {
	return WorkflowEditGrantRecord{
		Grant:           edit.WorkflowEditGrant{ID: "grant-1", Key: edit.WorkflowKey{TenantID: "tenant-1", WorkflowID: "workflow-1"}, ActorID: "actor-1", PlanHash: workflowApprovalHash("plan"), BaseSemanticHash: workflowApprovalHash("base"), AllowedOps: []edit.OperationKind{edit.OperationAddNode, edit.OperationUpdateNode, edit.OperationAddEdge, edit.OperationUpdateEdge}, MaxOperations: 3, MaxAddedNodes: 2, ExpiresAt: now.Add(time.Hour)},
		CanvasSessionID: "canvas-session-1", RiskScope: WorkflowGrantRiskScope{MaxRisk: WorkflowRiskMedium}, State: WorkflowApprovalActive, IssuedAt: now,
	}
}

func workflowApprovalHash(seed string) string { return "sha256:" + strings.Repeat(seed[:1], 64) }

func assertWorkflowApprovalCode(t *testing.T, err error, want edit.ErrorCode) {
	t.Helper()
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != want {
		t.Errorf("error=%v, want domain code %s", err, want)
	}
}
