package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"

	"runner/workflowstore"
)

func TestWorkflowCommitRecoveryAfterPreparedKeepsOldSnapshot(t *testing.T) {
	requireWorkflowCommitFailpoints(t)
	root := t.TempDir()
	key := workflowCommitTestKey("tenant-a")
	seedStore, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedStore.Commit(context.Background(), workflowCommitInputFixture(key, "", "rv-1", "transaction-1", "idem-1", "digest-1")); err != nil {
		t.Fatal(err)
	}

	failing, err := NewWorkflowCommitStore(root, withWorkflowCommitFailpointForTest(failOnceAt(workflowCommitAfterPrepared, errors.New("crash after prepared"))))
	if err != nil {
		t.Fatal(err)
	}
	second := workflowCommitInputFixture(key, "rv-1", "rv-2", "transaction-2", "idem-2", "digest-2")
	if _, err := failing.Commit(context.Background(), second); err == nil {
		t.Fatal("Commit() at after-prepared failpoint unexpectedly succeeded")
	}

	restarted, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecovery, err := restarted.Snapshot(context.Background(), key)
	if err != nil || beforeRecovery.ResourceVersion != "rv-1" {
		t.Fatalf("reader observed partial prepared snapshot: version=%q err=%v", beforeRecovery.ResourceVersion, err)
	}
	if err := restarted.Recover(context.Background(), key); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	afterRecovery, err := restarted.Snapshot(context.Background(), key)
	if err != nil || afterRecovery.ResourceVersion != "rv-1" || afterRecovery.Generation != 1 {
		t.Fatalf("discarding prepared changed current snapshot: %#v, %v", afterRecovery, err)
	}
	if _, found, err := restarted.LookupReceipt(context.Background(), key, "idem-2"); err != nil || found {
		t.Fatalf("prepared transaction leaked receipt: found=%v err=%v", found, err)
	}
	if _, err := restarted.Commit(context.Background(), second); err != nil {
		t.Fatalf("Commit() after prepared recovery error = %v", err)
	}
}

func TestWorkflowCommitRecoveryAfterCurrentPointerCompletesCallbacksOnce(t *testing.T) {
	requireWorkflowCommitFailpoints(t)
	root := t.TempDir()
	key := workflowCommitTestKey("tenant-a")
	failing, err := NewWorkflowCommitStore(root, withWorkflowCommitFailpointForTest(failOnceAt(workflowCommitAfterCurrentPointer, errors.New("crash after current pointer"))))
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(key, "", "rv-1", "transaction-1", "idem-1", "digest-1")
	if _, err := failing.Commit(context.Background(), input); err == nil {
		t.Fatal("Commit() at current-pointer failpoint unexpectedly succeeded")
	}
	visible, err := failing.Snapshot(context.Background(), key)
	if err != nil || visible.ResourceVersion != "rv-1" || visible.TransactionID != "transaction-1" {
		t.Fatalf("reader did not observe complete new snapshot: %#v, %v", visible, err)
	}

	recorder := &workflowCommitCallbackRecorder{}
	restarted, err := NewWorkflowCommitStore(root, WithWorkflowCommitRecoveryCallbacks(recorder.callbacks()))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(context.Background(), key); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if err := restarted.Recover(context.Background(), key); err != nil {
		t.Fatalf("second Recover() error = %v", err)
	}
	if receipt, err := restarted.Commit(context.Background(), input); err != nil || receipt.TransactionID != "transaction-1" {
		t.Fatalf("idempotent replay after recovery = %#v, %v", receipt, err)
	}
	recorder.assertCounts(t, 1, 1, 1)
}

func TestWorkflowCommitRecoveryResumesAfterEachAcknowledgement(t *testing.T) {
	requireWorkflowCommitFailpoints(t)
	tests := []struct {
		name          string
		point         workflowCommitFailpoint
		beforeRecover [3]int
	}{
		{name: "receipt", point: workflowCommitAfterReceiptAck, beforeRecover: [3]int{1, 0, 0}},
		{name: "event", point: workflowCommitAfterEventAck, beforeRecover: [3]int{1, 1, 0}},
		{name: "outbox", point: workflowCommitAfterOutboxAck, beforeRecover: [3]int{1, 1, 1}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			recorder := &workflowCommitCallbackRecorder{}
			store, err := NewWorkflowCommitStore(
				t.TempDir(),
				WithWorkflowCommitRecoveryCallbacks(recorder.callbacks()),
				withWorkflowCommitFailpointForTest(failOnceAt(test.point, errors.New("injected acknowledgement crash"))),
			)
			if err != nil {
				t.Fatal(err)
			}
			key := workflowCommitTestKey("tenant-a")
			input := workflowCommitInputFixture(key, "", "rv-1", "transaction-1", "idem-1", "digest-1")
			if _, err := store.Commit(context.Background(), input); err == nil {
				t.Fatal("Commit() at acknowledgement failpoint unexpectedly succeeded")
			}
			recorder.assertCounts(t, test.beforeRecover[0], test.beforeRecover[1], test.beforeRecover[2])
			if err := store.Recover(context.Background(), key); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			if err := store.Recover(context.Background(), key); err != nil {
				t.Fatalf("second Recover() error = %v", err)
			}
			recorder.assertCounts(t, 1, 1, 1)
		})
	}
}

func TestWorkflowCommitFailpointClassifiesStorageFailuresAndPreservesVisibility(t *testing.T) {
	requireWorkflowCommitFailpoints(t)
	tests := []struct {
		name        string
		point       workflowCommitFailpoint
		injected    error
		wantClass   WorkflowCommitFailureClass
		wantVisible bool
	}{
		{name: "disk full before WAL", point: workflowCommitBeforePreparedWrite, injected: syscall.ENOSPC, wantClass: WorkflowCommitFailureDiskFull},
		{name: "rename before snapshot", point: workflowCommitBeforeSnapshotWrite, injected: errors.New("rename failed"), wantClass: WorkflowCommitFailureRename},
		{name: "fsync before committed", point: workflowCommitBeforeCommittedWrite, injected: errors.New("fsync failed"), wantClass: WorkflowCommitFailureFsync, wantVisible: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := NewWorkflowCommitStore(root, withWorkflowCommitFailpointForTest(failOnceAt(test.point, test.injected)))
			if err != nil {
				t.Fatal(err)
			}
			key := workflowCommitTestKey("tenant-a")
			_, err = store.Commit(context.Background(), workflowCommitInputFixture(key, "", "rv-1", "transaction-1", "idem-1", "digest-1"))
			var failure *WorkflowCommitFailure
			if !errors.As(err, &failure) || failure.Class != test.wantClass {
				t.Fatalf("Commit() error = %T %v, want class %s", err, err, test.wantClass)
			}
			_, snapshotErr := store.Snapshot(context.Background(), key)
			visible := snapshotErr == nil
			if visible != test.wantVisible {
				t.Fatalf("snapshot visibility=%v err=%v, want %v", visible, snapshotErr, test.wantVisible)
			}
			restarted, err := NewWorkflowCommitStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.Recover(context.Background(), key); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
		})
	}
}

type workflowCommitCallbackRecorder struct {
	mu                       sync.Mutex
	receipts, events, outbox int
}

func (r *workflowCommitCallbackRecorder) callbacks() WorkflowCommitRecoveryCallbacks {
	return WorkflowCommitRecoveryCallbacks{
		ConfirmReceipt: func(context.Context, workflowstore.TransactionSnapshot) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.receipts++
			return nil
		},
		ConfirmEvents: func(context.Context, workflowstore.TransactionSnapshot) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.events++
			return nil
		},
		ConfirmOutbox: func(context.Context, workflowstore.TransactionSnapshot) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.outbox++
			return nil
		},
	}
}

func (r *workflowCommitCallbackRecorder) assertCounts(t *testing.T, receipts, events, outbox int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.receipts != receipts || r.events != events || r.outbox != outbox {
		t.Fatalf("callback counts = receipt:%d event:%d outbox:%d, want %d/%d/%d", r.receipts, r.events, r.outbox, receipts, events, outbox)
	}
}

func failOnceAt(target workflowCommitFailpoint, injected error) workflowCommitFailpointHook {
	var mu sync.Mutex
	fired := false
	return func(point workflowCommitFailpoint) error {
		mu.Lock()
		defer mu.Unlock()
		if point == target && !fired {
			fired = true
			return injected
		}
		return nil
	}
}

func requireWorkflowCommitFailpoints(t *testing.T) {
	t.Helper()
	if os.Getenv("AIOPS_WORKFLOW_FAILPOINT_TEST") != "1" {
		t.Skip("set AIOPS_WORKFLOW_FAILPOINT_TEST=1 to enable crash injection")
	}
}
