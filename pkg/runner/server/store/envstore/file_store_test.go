package envstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileStoreCRUD(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "environments"))
	ctx := context.Background()

	created, err := store.Create(ctx, EnvironmentRecord{
		Name:        "staging",
		Description: "staging env",
	})
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if created.Name != "staging" {
		t.Fatalf("unexpected environment: %+v", created)
	}

	_, err = store.AddVar(ctx, "staging", EnvVar{
		Key:         "DB_HOST",
		Value:       "db.internal",
		Description: "database host",
		Sensitive:   false,
	})
	if err != nil {
		t.Fatalf("add var: %v", err)
	}

	got, err := store.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if len(got.Vars) != 1 || got.Vars[0].Key != "DB_HOST" {
		t.Fatalf("unexpected vars: %+v", got.Vars)
	}

	_, err = store.UpdateVar(ctx, "staging", "DB_HOST", EnvVar{
		Key:         "DB_HOST",
		Value:       "db.staging.internal",
		Description: "database host",
		Sensitive:   true,
	})
	if err != nil {
		t.Fatalf("update var: %v", err)
	}

	got, err = store.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("get environment after update: %v", err)
	}
	if got.Vars[0].Value != "db.staging.internal" || !got.Vars[0].Sensitive {
		t.Fatalf("unexpected updated var: %+v", got.Vars[0])
	}

	_, err = store.DeleteVar(ctx, "staging", "DB_HOST")
	if err != nil {
		t.Fatalf("delete var: %v", err)
	}
	got, err = store.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("get environment after delete: %v", err)
	}
	if len(got.Vars) != 0 {
		t.Fatalf("expected no vars, got %+v", got.Vars)
	}
}
