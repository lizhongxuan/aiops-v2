package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"runner/server/store/envstore"
)

func TestEnvironmentServiceCRUDAndValidation(t *testing.T) {
	t.Parallel()

	svc := NewEnvironmentService(envstore.NewFileStore(filepath.Join(t.TempDir(), "environments")))
	ctx := context.Background()

	if err := svc.Create(ctx, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for nil create, got %v", err)
	}
	if err := svc.Create(ctx, &envstore.EnvironmentRecord{Name: "../prod"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for bad name, got %v", err)
	}

	if err := svc.Create(ctx, &envstore.EnvironmentRecord{
		Name:        " staging ",
		Description: "staging env",
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if err := svc.Create(ctx, &envstore.EnvironmentRecord{Name: "staging"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	if err := svc.AddVar(ctx, "staging", envstore.EnvVar{
		Key:         "db_host",
		Value:       "db.staging.internal",
		Description: "database host",
	}); err != nil {
		t.Fatalf("add var: %v", err)
	}
	if err := svc.AddVar(ctx, "staging", envstore.EnvVar{Key: "DB_HOST", Value: "dup"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for duplicate var, got %v", err)
	}

	record, err := svc.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if len(record.Vars) != 1 || record.Vars[0].Key != "DB_HOST" {
		t.Fatalf("expected normalized variable key, got %+v", record.Vars)
	}

	if err := svc.UpdateVar(ctx, "staging", "DB_HOST", envstore.EnvVar{
		Key:   "OTHER_KEY",
		Value: "mismatch",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for key mismatch, got %v", err)
	}

	if err := svc.UpdateVar(ctx, "staging", "DB_HOST", envstore.EnvVar{
		Key:         "db_host",
		Value:       "db-new.internal",
		Description: "updated host",
		Sensitive:   true,
	}); err != nil {
		t.Fatalf("update var: %v", err)
	}

	record, err = svc.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("get environment after update: %v", err)
	}
	if got := record.Vars[0]; got.Value != "db-new.internal" || got.Key != "DB_HOST" || !got.Sensitive {
		t.Fatalf("unexpected updated var: %+v", got)
	}

	if err := svc.DeleteVar(ctx, "staging", "DB_HOST"); err != nil {
		t.Fatalf("delete var: %v", err)
	}
	if err := svc.DeleteVar(ctx, "staging", "DB_HOST"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deleting missing var, got %v", err)
	}
	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing environment, got %v", err)
	}
}
