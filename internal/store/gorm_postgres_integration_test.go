package store

import (
	"os"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

func TestPostgresStoreRoundTripWhenDSNConfigured(t *testing.T) {
	dsn := os.Getenv("AIOPS_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("AIOPS_POSTGRES_TEST_DSN is not set")
	}

	repo, err := NewPostgresStore(dsn)
	if err != nil {
		t.Fatalf("NewPostgresStore() error = %v", err)
	}
	defer repo.Close()

	now := time.Now().UTC()
	session := &runtimekernel.SessionState{
		ID:        "sess-postgres-integration",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeChat,
		HostID:    "server-local",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	restored, err := repo.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if restored.ID != session.ID || restored.HostID != "server-local" {
		t.Fatalf("restored session = %#v", restored)
	}
}
