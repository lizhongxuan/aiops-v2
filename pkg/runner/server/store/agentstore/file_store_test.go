package agentstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStoreEncryptsTokenAtRest(t *testing.T) {
	t.Setenv("RUNNER_SERVER_SECRET", "test-secret")
	path := filepath.Join(t.TempDir(), "agents.json")
	store := NewFileStore(path)

	created, err := store.Create(context.Background(), AgentRecord{
		ID:      "agent-1",
		Name:    "agent-1",
		Address: "http://127.0.0.1:7072",
		Token:   "secret-token",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Token != "secret-token" {
		t.Fatalf("unexpected token in response: %s", created.Token)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, "secret-token") {
		t.Fatalf("token should not be persisted in plain text")
	}
	if !strings.Contains(content, "enc:") {
		t.Fatalf("expected encrypted token prefix")
	}

	got, err := store.Get(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Token != "secret-token" {
		t.Fatalf("expected decrypted token, got %q", got.Token)
	}
}
