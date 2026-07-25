package service

import (
	"context"
	"testing"
	"time"
)

func TestWorkflowEventAppliedIsPublishedAfterCommitAndReplaysAfterAck(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	events, err := NewWorkflowDomainEventService(fixture.app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Shutdown(context.Background()) })
	candidate := workflowTestEvent(fixture.request.WorkflowKey, "candidate-event-1", "candidate-transaction-1", `{}`)
	candidate.Type, candidate.ChangeSetID, candidate.CandidateRevision, candidate.Actor = "workflow.candidate.revised", fixture.request.ChangeSetID, fixture.request.CandidateRevision, "mcp"
	if _, err := events.log.Append(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}

	before, err := events.Replay(context.Background(), fixture.request.WorkflowKey, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Events) != 1 || before.Events[0].Type != WorkflowEventCandidateRevised {
		t.Fatalf("candidate replay before commit = %#v", before.Events)
	}

	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := events.Replay(context.Background(), receipt.Key, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Events) != 2 || after.Events[1].Type != WorkflowEventTransactionApplied || after.Events[1].TransactionID != receipt.TransactionID || after.Events[1].Revision != *receipt.RevisionAfter || after.Events[1].CandidateRevision != fixture.request.CandidateRevision || after.Events[1].Actor != "mcp" {
		t.Fatalf("committed replay = %#v", after.Events)
	}

	if _, err := events.Dispatch(context.Background(), receipt.Key, time.Now().UTC().Add(time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	pending, err := events.outbox.Pending(context.Background(), receipt.Key, time.Now().UTC().Add(2*time.Hour), 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after dispatch = %#v, %v", pending, err)
	}
	afterAck, err := events.Replay(context.Background(), receipt.Key, 0, 100)
	if err != nil || len(afterAck.Events) != 2 || afterAck.Events[1].EventID != after.Events[1].EventID {
		t.Fatalf("retained replay after outbox ACK = %#v, %v", afterAck.Events, err)
	}
}

func TestWorkflowEventSubscriptionRecoversSlowConsumerFromRetainedLog(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	events, err := NewWorkflowDomainEventService(fixture.app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Shutdown(context.Background()) })
	key := fixture.request.WorkflowKey
	stream, cancel, err := events.Subscribe(context.Background(), key, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	const count = 40
	for index := 0; index < count; index++ {
		event := workflowTestEvent(key, workflowEventTestID("event", index), workflowEventTestID("transaction", index), `{}`)
		event, err = events.log.Append(context.Background(), event)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events.outbox.Enqueue(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := events.Dispatch(context.Background(), key, time.Now().UTC().Add(time.Hour), count); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for sequence := uint64(1); sequence <= count; sequence++ {
		select {
		case event := <-stream:
			if event.Sequence != sequence {
				t.Fatalf("event sequence = %d, want %d", event.Sequence, sequence)
			}
		case <-deadline:
			t.Fatalf("timed out after sequence %d", sequence-1)
		}
	}
}

func TestWorkflowEventRecoveryReplaysCommittedWALHistoryAfterRestart(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	events, err := NewWorkflowDomainEventService(fixture.app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Shutdown(context.Background()) })
	if err := events.Recover(context.Background(), receipt.Key); err != nil {
		t.Fatal(err)
	}
	replay, err := events.Replay(context.Background(), receipt.Key, 0, 100)
	if err != nil || len(replay.Events) != 2 || replay.Events[0].Sequence != 1 || replay.Events[1].Sequence != 2 || replay.Events[1].TransactionID != receipt.TransactionID {
		t.Fatalf("recovered WAL replay = %#v, %v", replay.Events, err)
	}
}

func workflowEventTestID(prefix string, index int) string {
	return prefix + "-" + time.Date(2026, 7, 21, 0, 0, index, 0, time.UTC).Format("150405")
}
