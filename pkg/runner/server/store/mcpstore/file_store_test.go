package mcpstore

import (
	"context"
	"testing"
)

func TestFileStoreCRUD(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	created, err := store.Create(ctx, ServerRecord{
		ID:      "mcp-http",
		Name:    "HTTP Server",
		Type:    TypeHTTP,
		URL:     "http://127.0.0.1:8080/mcp",
		EnvVars: map[string]string{"TOKEN": "secret"},
		Tools: []ToolRecord{
			{Name: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != StatusStopped {
		t.Fatalf("unexpected status: %s", created.Status)
	}

	got, err := store.Get(ctx, "mcp-http")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.URL != "http://127.0.0.1:8080/mcp" {
		t.Fatalf("unexpected url: %s", got.URL)
	}
	if got.EnvVars["TOKEN"] != "secret" {
		t.Fatalf("unexpected env: %#v", got.EnvVars)
	}

	updated, err := store.Update(ctx, "mcp-http", ServerRecord{
		Name:      "HTTP Server v2",
		Type:      TypeHTTP,
		URL:       "http://127.0.0.1:9090/mcp",
		Status:    StatusRunning,
		EnvVars:   map[string]string{},
		Tools:     []ToolRecord{{Name: "status"}},
		LastError: "",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "HTTP Server v2" || updated.Status != StatusRunning {
		t.Fatalf("unexpected update: %#v", updated)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected items: %d", len(items))
	}

	if err := store.Delete(ctx, "mcp-http"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, "mcp-http"); err == nil {
		t.Fatal("expected not found after delete")
	}
}
