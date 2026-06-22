package operatorruntime

import (
	"context"
	"testing"
)

func TestSeedDefaultCatalogCreatesPGGuardDefaults(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := SeedDefaultCatalog(ctx, store); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	templates, _ := store.ListInspectionTemplates(ctx)
	problems, _ := store.ListProblemTypes(ctx)
	actions, _ := store.ListActions(ctx)
	bindings, _ := store.ListWorkflowBindings(ctx)
	if len(templates) != 1 {
		t.Fatalf("expected one default inspection template, got %#v", templates)
	}
	if len(problems) != 2 {
		t.Fatalf("expected two default problem types, got %#v", problems)
	}
	if len(actions) != 1 || actions[0].ID != "postgres.replication.reconnect_replica.v1" {
		t.Fatalf("expected default reconnect action, got %#v", actions)
	}
	if len(bindings) != 1 || bindings[0].WorkflowRef != "builtin.postgres.replication_reconnect_replica.v1" {
		t.Fatalf("expected default workflow binding, got %#v", bindings)
	}
}
