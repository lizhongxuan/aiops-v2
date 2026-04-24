package eventstore

import (
	"context"
	"testing"
	"time"

	"runner/server/events"
)

func TestFileStoreAppendAndList(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	first := events.Event{
		Type:      "run_queued",
		RunID:     "run-12345678",
		Workflow:  "wf-a",
		Status:    "queued",
		Timestamp: time.Now().UTC(),
	}
	second := events.Event{
		Type:      "run_finish",
		RunID:     "run-12345678",
		Workflow:  "wf-a",
		Status:    "success",
		Timestamp: time.Now().UTC(),
	}
	if err := store.Append(ctx, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := store.Append(ctx, second); err != nil {
		t.Fatalf("append second: %v", err)
	}

	items, err := store.List(ctx, "run-12345678")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].Type != "run_queued" || items[1].Type != "run_finish" {
		t.Fatalf("unexpected items: %+v", items)
	}
}
