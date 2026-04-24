package skillstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileStoreCRUD(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "skills"))
	ctx := context.Background()

	created, err := store.Create(ctx, SkillRecord{
		Name:        "ops-checklist",
		Description: "ops checklist",
		Triggers:    []string{"ops", "check"},
		Content:     "# Ops Checklist",
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if created.Name != "ops-checklist" {
		t.Fatalf("unexpected created skill: %+v", created)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(items))
	}

	got, err := store.Get(ctx, "ops-checklist")
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Content != "# Ops Checklist" {
		t.Fatalf("unexpected content: %s", got.Content)
	}

	updated, err := store.Update(ctx, "ops-checklist", SkillRecord{
		Description: "updated description",
		Triggers:    []string{"ops"},
		Content:     "# Updated",
	})
	if err != nil {
		t.Fatalf("update skill: %v", err)
	}
	if updated.Description != "updated description" || updated.Content != "# Updated" {
		t.Fatalf("unexpected updated skill: %+v", updated)
	}

	if err := store.Delete(ctx, "ops-checklist"); err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if _, err := store.Get(ctx, "ops-checklist"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
