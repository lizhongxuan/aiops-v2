package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowOutboxDispatcherPreservesOrderRetriesAndAcks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-dispatch"}
	log, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := NewFileWorkflowOutbox(root)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 2; index++ {
		event, appendErr := log.Append(ctx, workflowTestEvent(key, workflowEventTestID("event", index), workflowEventTestID("transaction", index), `{}`))
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		if _, enqueueErr := outbox.Enqueue(ctx, event); enqueueErr != nil {
			t.Fatal(enqueueErr)
		}
	}
	now := time.Now().UTC().Add(time.Hour)
	delivered := make([]uint64, 0, 2)
	failFirst := true
	dispatcher := NewWorkflowOutboxDispatcher(outbox, func(_ context.Context, event edit.WorkflowDomainEvent) error {
		if failFirst {
			failFirst = false
			return errors.New("transient downstream failure")
		}
		delivered = append(delivered, event.Sequence)
		return nil
	}, time.Second)
	result, err := dispatcher.Dispatch(ctx, key, now, 10)
	if err != nil || result.Delivered != 0 || result.Retried != 1 || len(delivered) != 0 {
		t.Fatalf("first dispatch = %#v delivered=%v err=%v", result, delivered, err)
	}
	result, err = dispatcher.Dispatch(ctx, key, now.Add(2*time.Second), 10)
	if err != nil || result.Delivered != 2 || result.Retried != 0 || len(delivered) != 2 || delivered[0] != 1 || delivered[1] != 2 {
		t.Fatalf("retry dispatch = %#v delivered=%v err=%v", result, delivered, err)
	}
	replay, err := log.Replay(ctx, key, 0, 10)
	if err != nil || len(replay.Events) != 2 {
		t.Fatalf("event log was changed by delivery = %#v, %v", replay, err)
	}
}
