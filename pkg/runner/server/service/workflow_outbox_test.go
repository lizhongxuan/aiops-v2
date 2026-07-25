package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowOutboxRetryAckAndRestartIsSeparateFromEventLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	event := workflowTestEvent(key, "event-1", "transaction-1", `{"change":"saved"}`)
	log, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	event, err = log.Append(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := NewFileWorkflowOutbox(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}

	retryAt := event.OccurredAt.Add(time.Minute)
	if err := outbox.Retry(ctx, key, event.EventID, errors.New("upstream included never-store-this"), retryAt); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewFileWorkflowOutbox(root)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restarted.Pending(ctx, key, retryAt, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Attempts != 1 || pending[0].AvailableAt != retryAt || strings.Contains(pending[0].LastError, "never-store-this") {
		t.Fatalf("pending after retry = %#v", pending)
	}
	if err := restarted.Ack(ctx, key, event.EventID, retryAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	afterAck, err := NewFileWorkflowOutbox(root)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = afterAck.Pending(ctx, key, retryAt.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("acked event remains pending: %#v", pending)
	}
	page, err := log.Replay(ctx, key, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != event.EventID {
		t.Fatalf("outbox ACK changed event log: %#v", page.Events)
	}
}

func TestWorkflowOutboxDeduplicatesEventAndTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	outbox, err := NewFileWorkflowOutbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	event := workflowTestEvent(key, "event-1", "transaction-1", `{"change":"a"}`)
	event.Sequence = 1
	first, err := outbox.Enqueue(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := outbox.Enqueue(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	if first.Event.EventID != second.Event.EventID {
		t.Fatalf("idempotent enqueue changed event: %#v", second)
	}
	conflict := event
	conflict.Payload = []byte(`{"change":"different"}`)
	if _, err := outbox.Enqueue(ctx, conflict); !errors.Is(err, ErrWorkflowOutboxConflict) {
		t.Fatalf("event dedupe conflict = %v", err)
	}
	transactionConflict := workflowTestEvent(key, "event-2", "transaction-1", `{"change":"a"}`)
	transactionConflict.Sequence = 2
	if _, err := outbox.Enqueue(ctx, transactionConflict); !errors.Is(err, ErrWorkflowOutboxConflict) {
		t.Fatalf("transaction dedupe conflict = %v", err)
	}
}

func TestWorkflowOutboxCanonicalPartitionAndSecretBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	outbox, err := NewFileWorkflowOutbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyA := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "shared"}
	keyB := edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: "shared"}
	for _, key := range []edit.WorkflowKey{keyA, keyB} {
		event := workflowTestEvent(key, "event-1", "transaction-1", `{"change":"safe"}`)
		event.Sequence = 1
		if _, err := outbox.Enqueue(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := outbox.Pending(ctx, keyA, time.Now().Add(24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Event.Key != keyA {
		t.Fatalf("tenant partition failed: %#v", pending)
	}
	unsafe := workflowTestEvent(keyA, "event-secret", "transaction-secret", `{"apiKey":"never-write"}`)
	unsafe.Sequence = 2
	if _, err := outbox.Enqueue(ctx, unsafe); !errors.Is(err, ErrWorkflowEventSecret) {
		t.Fatalf("secret boundary error = %v", err)
	}
}
