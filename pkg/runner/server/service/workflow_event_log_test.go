package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowEventLogAppendReplayGapAndRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	log, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}

	first, err := log.Append(ctx, workflowTestEvent(key, "event-1", "transaction-1", `{"step":1}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := log.Append(ctx, workflowTestEvent(key, "event-2", "transaction-2", `{"step":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = %d, %d; want 1, 2", first.Sequence, second.Sequence)
	}

	page, err := log.Replay(ctx, key, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != "event-1" || !page.HasMore || page.LatestSequence != 2 {
		t.Fatalf("unexpected first page: %#v", page)
	}
	gaps, err := log.QueryGaps(ctx, key, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].FromSequence != 3 || gaps[0].ToSequence != 3 {
		t.Fatalf("gaps = %#v; want [3,3]", gaps)
	}

	restarted, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	page, err = restarted.Replay(ctx, key, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != "event-2" {
		t.Fatalf("restart replay = %#v", page.Events)
	}
}

func TestWorkflowEventLogCanonicalPartitionAndIdempotency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	log, err := NewFileWorkflowEventLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyA := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "shared"}
	keyB := edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: "shared"}
	eventA := workflowTestEvent(keyA, "event-1", "transaction-1", `{"change":"a"}`)

	stored, err := log.Append(ctx, eventA)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := log.Append(ctx, eventA)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Sequence != stored.Sequence {
		t.Fatalf("idempotent append sequence = %d; want %d", replayed.Sequence, stored.Sequence)
	}
	other, err := log.Append(ctx, workflowTestEvent(keyB, "event-1", "transaction-1", `{"change":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	if other.Sequence != 1 {
		t.Fatalf("tenant-b sequence = %d; want 1", other.Sequence)
	}
	page, err := log.Replay(ctx, keyA, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || !strings.Contains(string(page.Events[0].Payload), `"a"`) {
		t.Fatalf("tenant-a replay leaked or changed: %#v", page.Events)
	}

	conflict := eventA
	conflict.Payload = []byte(`{"change":"different"}`)
	if _, err := log.Append(ctx, conflict); !errors.Is(err, ErrWorkflowEventConflict) {
		t.Fatalf("duplicate event conflict error = %v", err)
	}
	transactionConflict := workflowTestEvent(keyA, "event-2", "transaction-1", `{"change":"a"}`)
	if _, err := log.Append(ctx, transactionConflict); !errors.Is(err, ErrWorkflowEventConflict) {
		t.Fatalf("duplicate transaction conflict error = %v", err)
	}
}

func TestWorkflowEventLogRejectsSequenceGapAndSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	log, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}

	gapped := workflowTestEvent(key, "event-gap", "transaction-gap", `{}`)
	gapped.Sequence = 2
	if _, err := log.Append(ctx, gapped); !errors.Is(err, ErrWorkflowEventSequence) {
		t.Fatalf("gap error = %v", err)
	}
	secret := "never-write-this-value"
	unsafe := workflowTestEvent(key, "event-secret", "transaction-secret", `{"password":"`+secret+`"}`)
	if _, err := log.Append(ctx, unsafe); !errors.Is(err, ErrWorkflowEventSecret) || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret error = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "events", "tenant-a", "workflow-1.json")); err == nil && strings.Contains(string(data), secret) {
		t.Fatal("secret was persisted")
	}
	safe := workflowTestEvent(key, "event-safe", "transaction-safe", `{"credentials":{"secretRef":"vault://workflow/key"}}`)
	if _, err := log.Append(ctx, safe); err != nil {
		t.Fatalf("secretRef event rejected: %v", err)
	}
}

func TestWorkflowKeyEventSequencerConcurrentManualMCPAndUndo(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	logs := make([]*FileWorkflowEventLog, 2)
	for index := range logs {
		var err error
		logs[index], err = NewFileWorkflowEventLog(root)
		if err != nil {
			t.Fatal(err)
		}
	}
	key := edit.WorkflowKey{TenantID: "tenant-sequence", WorkflowID: "workflow-shared"}
	const producers = 18
	start := make(chan struct{})
	errorsByProducer := make(chan error, producers)
	var wait sync.WaitGroup
	for index := 0; index < producers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			event := workflowTestEvent(key, fmt.Sprintf("event-%02d", index), fmt.Sprintf("transaction-%02d", index), fmt.Sprintf(`{"producer":%d}`, index))
			event.Actor = []edit.ActorKind{"manual", "mcp", "undo"}[index%3]
			sequencer, err := logs[index%len(logs)].Sequencer(key)
			if err == nil {
				_, err = sequencer.Append(ctx, event)
			}
			errorsByProducer <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsByProducer)
	for err := range errorsByProducer {
		if err != nil {
			t.Fatal(err)
		}
	}
	replay, err := logs[0].Replay(ctx, key, 0, producers)
	if err != nil || len(replay.Events) != producers {
		t.Fatalf("concurrent replay count=%d err=%v", len(replay.Events), err)
	}
	for index, event := range replay.Events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event[%d].Sequence=%d", index, event.Sequence)
		}
	}
	otherKey := edit.WorkflowKey{TenantID: "tenant-other", WorkflowID: "workflow-shared"}
	other, err := logs[1].Append(ctx, workflowTestEvent(otherKey, "event-other", "transaction-other", `{}`))
	if err != nil || other.Sequence != 1 {
		t.Fatalf("other WorkflowKey sequence=%d err=%v", other.Sequence, err)
	}
}

func workflowTestEvent(key edit.WorkflowKey, eventID, transactionID, payload string) edit.WorkflowDomainEvent {
	return edit.WorkflowDomainEvent{
		SchemaVersion: edit.SchemaVersion,
		EventID:       eventID,
		Type:          "transaction.applied",
		Key:           key,
		Actor:         edit.ActorKind("manual"),
		TransactionID: transactionID,
		Payload:       []byte(payload),
		OccurredAt:    time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
	}
}
