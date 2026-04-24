package service

import (
	"context"
	"testing"
	"time"
)

func TestFileRunRecordStoreCRUD(t *testing.T) {
	t.Parallel()

	store := NewFileRunRecordStore(t.TempDir() + "/run-records.json")
	ctx := context.Background()
	now := time.Now().UTC()

	meta := RunMeta{
		RunID:          "run-12345678",
		WorkflowName:   "wf-a",
		WorkflowYAML:   "name: wf-a",
		TriggeredBy:    "tester",
		IdempotencyKey: "same-key",
		Status:         "queued",
		Summary:        "queued",
		CreatedAt:      now,
		QueuedAt:       now,
	}
	if err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.Get(ctx, meta.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IdempotencyKey != "same-key" {
		t.Fatalf("unexpected record: %+v", got)
	}

	meta.Status = "success"
	meta.Summary = "done"
	if err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("update: %v", err)
	}

	items, err := store.List(ctx, RunFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].Status != "success" {
		t.Fatalf("unexpected items: %+v", items)
	}

	if err := store.Delete(ctx, meta.RunID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	items, err = store.List(ctx, RunFilter{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %+v", items)
	}
}
