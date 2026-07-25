package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"runner/workflow/edit"
	"runner/workflowstore"
)

func TestWorkflowCommitStorePersistsCompleteSnapshotAndReceiptAcrossRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatalf("NewWorkflowCommitStore() error = %v", err)
	}
	input := workflowCommitInputFixture(workflowCommitTestKey("tenant-a"), "", "rv-1", "transaction-1", "idem-1", "digest-1")
	receipt, err := store.Commit(context.Background(), input)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !reflect.DeepEqual(receipt, input.Receipt) {
		t.Fatalf("Commit() receipt = %#v, want %#v", receipt, input.Receipt)
	}

	restarted, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatalf("restart NewWorkflowCommitStore() error = %v", err)
	}
	snapshot, err := restarted.Snapshot(context.Background(), input.Key)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !reflect.DeepEqual(snapshot.GraphPayload, input.GraphPayload) || !reflect.DeepEqual(snapshot.Meta, input.Meta) || !reflect.DeepEqual(snapshot.History, input.History) || !reflect.DeepEqual(snapshot.ImportResult, input.ImportResult) || !reflect.DeepEqual(snapshot.RollbackResult, input.RollbackResult) || !reflect.DeepEqual(snapshot.Checkpoint, input.Checkpoint) || !reflect.DeepEqual(snapshot.Inverse, input.Inverse) || !reflect.DeepEqual(snapshot.ApprovalConsume, input.ApprovalConsume) || !reflect.DeepEqual(snapshot.Audit, input.Audit) || !reflect.DeepEqual(snapshot.Events, input.Events) || !reflect.DeepEqual(snapshot.Outbox, input.Outbox) {
		t.Fatalf("Snapshot() did not round-trip the complete commit envelope: %#v", snapshot)
	}
	if snapshot.Key != input.Key || snapshot.ResourceVersion != "rv-1" || !reflect.DeepEqual(snapshot.Receipt, input.Receipt) || !reflect.DeepEqual(snapshot.ValidationBasis, input.ValidationBasis) {
		t.Fatalf("Snapshot() identity/receipt/basis mismatch: %#v", snapshot)
	}
	lookedUp, found, err := restarted.LookupReceipt(context.Background(), input.Key, input.IdempotencyKey)
	if err != nil || !found || !reflect.DeepEqual(lookedUp, receipt) {
		t.Fatalf("LookupReceipt() = %#v, %v, %v; want persisted receipt", lookedUp, found, err)
	}
}

func TestWorkflowCommitStoreIdempotentReplayPrecedesCAS(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(workflowCommitTestKey("tenant-a"), "", "rv-1", "transaction-1", "idem-1", "digest-1")
	first, err := store.Commit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Commit(context.Background(), input)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("exact replay = %#v, %v; want original receipt %#v", replayed, err, first)
	}

	reused := input
	reused.RequestDigest = "digest-for-different-payload"
	if _, err := restarted.Commit(context.Background(), reused); workflowCommitErrorCode(err) != edit.ErrorIdempotencyKeyReused {
		t.Fatalf("reused idempotency key error = %T %v, want %s", err, err, edit.ErrorIdempotencyKeyReused)
	}
	afterRejected, err := restarted.Snapshot(context.Background(), input.Key)
	if err != nil {
		t.Fatal(err)
	}
	if afterRejected.Generation != 1 || afterRejected.ResourceVersion != "rv-1" {
		t.Fatalf("rejected idempotency reuse changed snapshot to generation/version %d/%q", afterRejected.Generation, afterRejected.ResourceVersion)
	}
}

func TestWorkflowCommitStoreReceiptLifecycleCASIsAuthoritativeAndIdempotent(t *testing.T) {
	t.Parallel()
	store, err := NewWorkflowCommitStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(workflowCommitTestKey("tenant-a"), "", "rv-1", "transaction-1", "idem-1", "digest-1")
	input.Receipt.Status = "verifying"
	if _, err := store.Commit(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	validation := input.Receipt.Validation
	artifact := workflowstore.TypedEnvelope{Kind: "workflow.postcheck", SchemaVersion: edit.SchemaVersion, Payload: json.RawMessage(`{"status":"completed"}`)}
	update := WorkflowReceiptLifecycleUpdate{Key: input.Key, TransactionID: input.Receipt.TransactionID, ExpectedBasis: input.ValidationBasis, Status: "completed", Validation: validation, Artifact: artifact}
	receipt, err := store.UpdateReceiptLifecycle(context.Background(), update)
	if err != nil || receipt.Status != "completed" {
		t.Fatalf("UpdateReceiptLifecycle() = %#v, %v", receipt, err)
	}
	snapshot, err := store.Snapshot(context.Background(), input.Key)
	if err != nil || snapshot.Generation != 2 || snapshot.ResourceVersion != "rv-1" || len(snapshot.LifecycleArtifacts) != 1 || snapshot.Receipt.Status != "completed" {
		t.Fatalf("lifecycle snapshot = %#v, %v", snapshot, err)
	}
	if replay, err := store.UpdateReceiptLifecycle(context.Background(), update); err != nil || !reflect.DeepEqual(replay, receipt) {
		t.Fatalf("lifecycle replay = %#v, %v", replay, err)
	}
	afterReplay, _ := store.Snapshot(context.Background(), input.Key)
	if afterReplay.Generation != 2 {
		t.Fatalf("idempotent lifecycle replay advanced generation to %d", afterReplay.Generation)
	}
	stale := update
	stale.ExpectedBasis.CatalogVersion = "changed"
	stale.Validation.Basis = stale.ExpectedBasis
	if _, err := store.UpdateReceiptLifecycle(context.Background(), stale); workflowCommitErrorCode(err) != edit.ErrorStaleRevision {
		t.Fatalf("stale lifecycle basis error = %v", err)
	}
}

func TestWorkflowCommitStoreTwoInstancesCASAllowsOneWinner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstStore, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	key := workflowCommitTestKey("tenant-a")
	if _, err := firstStore.Commit(context.Background(), workflowCommitInputFixture(key, "", "rv-1", "transaction-1", "idem-1", "digest-1")); err != nil {
		t.Fatal(err)
	}
	inputs := []WorkflowCommitInput{
		workflowCommitInputFixture(key, "rv-1", "rv-2-a", "transaction-2-a", "idem-2-a", "digest-2-a"),
		workflowCommitInputFixture(key, "rv-1", "rv-2-b", "transaction-2-b", "idem-2-b", "digest-2-b"),
	}
	stores := []*WorkflowCommitStore{firstStore, secondStore}
	start := make(chan struct{})
	results := make(chan struct {
		receipt edit.WorkflowMutationReceipt
		err     error
	}, len(stores))
	var workers sync.WaitGroup
	for index := range stores {
		index := index
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			receipt, err := stores[index].Commit(context.Background(), inputs[index])
			results <- struct {
				receipt edit.WorkflowMutationReceipt
				err     error
			}{receipt: receipt, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	winners, stale := 0, 0
	var winningVersion string
	var winningTransactionID string
	for result := range results {
		switch {
		case result.err == nil:
			winners++
			winningVersion = result.receipt.RevisionAfter.ResourceVersion
			winningTransactionID = result.receipt.TransactionID
		case workflowCommitErrorCode(result.err) == edit.ErrorStaleRevision:
			stale++
		default:
			t.Fatalf("unexpected concurrent Commit() error = %T %v", result.err, result.err)
		}
	}
	if winners != 1 || stale != 1 {
		t.Fatalf("concurrent results: winners=%d stale=%d, want 1/1", winners, stale)
	}
	snapshot, err := firstStore.Snapshot(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != winningVersion || snapshot.Generation != 2 {
		t.Fatalf("current snapshot version/generation = %q/%d, want %q/2", snapshot.ResourceVersion, snapshot.Generation, winningVersion)
	}
	for _, input := range inputs {
		_, found, err := firstStore.LookupReceipt(context.Background(), key, input.IdempotencyKey)
		if err != nil {
			t.Fatal(err)
		}
		wantFound := input.Receipt.TransactionID == winningTransactionID
		if found != wantFound {
			t.Fatalf("receipt for %s found=%v, want %v; stale commit wrote partial state", input.Receipt.TransactionID, found, wantFound)
		}
	}
}

func TestWorkflowCommitStoreIsolatesSameWorkflowIDAcrossTenants(t *testing.T) {
	t.Parallel()

	store, err := NewWorkflowCommitStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		input := workflowCommitInputFixture(workflowCommitTestKey(tenant), "", "rv-1", "transaction-1", "shared-idempotency", "shared-digest")
		input.GraphPayload = []byte("tenant: " + tenant + "\n")
		if _, err := store.Commit(context.Background(), input); err != nil {
			t.Fatalf("Commit(%s) error = %v", tenant, err)
		}
	}
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		snapshot, err := store.Snapshot(context.Background(), workflowCommitTestKey(tenant))
		if err != nil {
			t.Fatalf("Snapshot(%s) error = %v", tenant, err)
		}
		if string(snapshot.GraphPayload) != "tenant: "+tenant+"\n" || snapshot.Key.TenantID != tenant {
			t.Fatalf("Snapshot(%s) crossed tenant boundary: %#v", tenant, snapshot)
		}
		receipt, found, err := store.LookupReceipt(context.Background(), workflowCommitTestKey(tenant), "shared-idempotency")
		if err != nil || !found || receipt.Key.TenantID != tenant {
			t.Fatalf("LookupReceipt(%s) crossed tenant boundary: %#v, %v, %v", tenant, receipt, found, err)
		}
	}
}

func workflowCommitInputFixture(key edit.WorkflowKey, expected, next, transactionID, idempotencyKey, digest string) WorkflowCommitInput {
	basis := edit.ValidationBasis{ResourceVersion: "basis-" + next, SemanticHash: "semantic-basis", CatalogVersion: "catalog-1", ActionSchemaDigest: "actions-1", ReferenceSnapshotHash: "references-1", EnvironmentBindingVersion: "environment-1"}
	revisionAfter := edit.Revision{ResourceVersion: next, SemanticHash: "semantic-" + next, LayoutHash: "layout-" + next}
	receipt := edit.WorkflowMutationReceipt{SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: transactionID, ChangeSetID: "change-set-" + transactionID, Key: key, Status: "committed", RevisionBefore: edit.Revision{ResourceVersion: expected}, RevisionAfter: &revisionAfter, Validation: edit.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}}, AuditID: "audit-" + transactionID}
	envelope := func(kind string) workflowstore.TypedEnvelope {
		return workflowstore.TypedEnvelope{Kind: kind, SchemaVersion: "test.v1", Payload: json.RawMessage(`{"ok":true}`)}
	}
	return WorkflowCommitInput{
		Key: key, ExpectedResourceVersion: expected, IdempotencyKey: idempotencyKey, RequestDigest: digest,
		GraphPayload: []byte("name: " + transactionID + "\n"), Meta: envelope("meta"), History: envelope("history"),
		ImportResult: envelope("import"), RollbackResult: envelope("rollback"), Receipt: receipt, ValidationBasis: basis,
		Checkpoint: envelope("checkpoint"), Inverse: envelope("inverse"), ApprovalConsume: envelope("approval-consume"), Audit: envelope("audit"),
		Events: []workflowstore.TypedEnvelope{envelope("event")}, Outbox: []workflowstore.TypedEnvelope{envelope("outbox")},
	}
}

func workflowCommitTestKey(tenant string) edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: tenant, WorkflowID: "shared-workflow"}
}

func workflowCommitErrorCode(err error) edit.ErrorCode {
	var domainError *edit.DomainError
	if errors.As(err, &domainError) {
		return domainError.Code
	}
	return ""
}
