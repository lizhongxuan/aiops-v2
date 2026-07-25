package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"runner/workflow/edit"
	"runner/workflowstore"
)

func TestWorkflowCommitRecoveryCurrentPointerRetainsImportRollbackMetaAndHistory(t *testing.T) {
	requireWorkflowCommitFailpoints(t)
	root := t.TempDir()
	key := workflowCommitTestKey("tenant-artifacts")
	failing, err := NewWorkflowCommitStore(root, withWorkflowCommitFailpointForTest(failOnceAt(workflowCommitAfterCurrentPointer, errors.New("crash after durable current pointer"))))
	if err != nil {
		t.Fatal(err)
	}
	input := workflowCommitInputFixture(key, "", "rv-artifacts", "transaction-artifacts", "idem-artifacts", "digest-artifacts")
	artifact := func(kind, marker string) workflowstore.TypedEnvelope {
		payload, marshalErr := json.Marshal(map[string]string{"marker": marker})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return workflowstore.TypedEnvelope{Kind: kind, SchemaVersion: edit.SchemaVersion, Payload: payload}
	}
	input.Meta = artifact("workflow.meta", "meta")
	input.History = artifact("workflow.history", "history")
	input.ImportResult = artifact("workflow.import", "import")
	input.RollbackResult = artifact("workflow.rollback", "rollback")
	if _, err := failing.Commit(context.Background(), input); err == nil {
		t.Fatal("Commit() at current-pointer failpoint unexpectedly succeeded")
	}

	assertSnapshot := func(label string, snapshot workflowstore.TransactionSnapshot, snapshotErr error) {
		t.Helper()
		if snapshotErr != nil || snapshot.ResourceVersion != "rv-artifacts" || snapshot.TransactionID != "transaction-artifacts" ||
			!reflect.DeepEqual(snapshot.GraphPayload, input.GraphPayload) || !reflect.DeepEqual(snapshot.Meta, input.Meta) ||
			!reflect.DeepEqual(snapshot.History, input.History) || !reflect.DeepEqual(snapshot.ImportResult, input.ImportResult) ||
			!reflect.DeepEqual(snapshot.RollbackResult, input.RollbackResult) {
			t.Fatalf("%s snapshot lost transaction artifacts: %#v err=%v", label, snapshot, snapshotErr)
		}
	}
	visible, visibleErr := failing.Snapshot(context.Background(), key)
	assertSnapshot("before recovery", visible, visibleErr)
	restarted, err := NewWorkflowCommitStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	recovered, recoveredErr := restarted.Snapshot(context.Background(), key)
	assertSnapshot("after recovery", recovered, recoveredErr)
}
