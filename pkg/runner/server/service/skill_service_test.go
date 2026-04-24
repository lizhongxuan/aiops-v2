package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"runner/server/store/skillstore"
)

func TestSkillServiceCRUDAndErrorMapping(t *testing.T) {
	t.Parallel()

	svc := NewSkillService(skillstore.NewFileStore(filepath.Join(t.TempDir(), "skills")))
	ctx := context.Background()

	if err := svc.Create(ctx, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for nil create, got %v", err)
	}
	if err := svc.Create(ctx, &skillstore.SkillRecord{Name: " "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for blank name, got %v", err)
	}

	if err := svc.Create(ctx, &skillstore.SkillRecord{
		Name:        " ops-checklist ",
		Description: "ops checklist",
		Triggers:    []string{"ops", "check"},
		Content:     "# Ops Checklist",
	}); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := svc.Create(ctx, &skillstore.SkillRecord{Name: "ops-checklist"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	items, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(items) != 1 || items[0].Name != "ops-checklist" {
		t.Fatalf("unexpected skills: %+v", items)
	}

	got, err := svc.Get(ctx, "ops-checklist")
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Content != "# Ops Checklist" {
		t.Fatalf("unexpected skill content: %q", got.Content)
	}

	if err := svc.Update(ctx, "ops-checklist", &skillstore.SkillRecord{
		Description: "updated description",
		Triggers:    []string{"ops"},
		Content:     "# Updated",
	}); err != nil {
		t.Fatalf("update skill: %v", err)
	}

	got, err = svc.Get(ctx, "ops-checklist")
	if err != nil {
		t.Fatalf("get updated skill: %v", err)
	}
	if got.Description != "updated description" || got.Content != "# Updated" || len(got.Triggers) != 1 {
		t.Fatalf("unexpected updated skill: %+v", got)
	}

	if err := svc.Delete(ctx, "ops-checklist"); err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if _, err := svc.Get(ctx, "ops-checklist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := svc.Update(ctx, "missing", &skillstore.SkillRecord{Content: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing update, got %v", err)
	}
	if err := svc.Delete(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing delete, got %v", err)
	}
}
