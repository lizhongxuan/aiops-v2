package operatorruntime

import (
	"context"
	"testing"
)

func TestMemoryStoreCRUD(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.SavePGCluster(ctx, validPGCluster()); err != nil {
		t.Fatalf("save cluster: %v", err)
	}
	items, err := store.ListPGClusters(ctx)
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(items) != 1 || items[0].ID != "pg-order" {
		t.Fatalf("unexpected clusters: %#v", items)
	}
	got, ok, err := store.GetPGCluster(ctx, "pg-order")
	if err != nil || !ok || got.Name != "order-postgres" {
		t.Fatalf("get cluster = %#v, %v, %v", got, ok, err)
	}
}

func TestMemoryStoreRejectsInvalidGuardRule(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	rule := validGuardRule()
	if err := store.SaveGuardRule(ctx, rule); err == nil {
		t.Fatalf("expected guard rule save to fail without dependent objects")
	}
}

func TestGuardRunEventsAreAppendOnly(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	run := GuardRun{ID: "run-1", GuardRuleRef: "guard.pg-order.replication", State: GuardRunPendingInspection}
	if err := store.CreateGuardRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := store.AppendGuardRunEvent(ctx, "run-1", GuardRunEvent{Type: "inspected", Message: "first"}); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := store.AppendGuardRunEvent(ctx, "run-1", GuardRunEvent{Type: "matched", Message: "second"}); err != nil {
		t.Fatalf("append second: %v", err)
	}
	got, ok, err := store.GetGuardRun(ctx, "run-1")
	if err != nil || !ok {
		t.Fatalf("get run: %v %v", ok, err)
	}
	if len(got.Events) != 2 || got.Events[0].Message != "first" || got.Events[1].Message != "second" {
		t.Fatalf("events were not append-only: %#v", got.Events)
	}
}
