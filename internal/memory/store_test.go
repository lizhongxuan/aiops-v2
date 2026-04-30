package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONStoreDisabledByDefaultDoesNotPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewJSONStore(Config{Path: path})

	_, err := store.Put(context.Background(), Item{Scope: ScopeProject, Text: "remember this"})
	if err != ErrDisabled {
		t.Fatalf("Put() error = %v, want ErrDisabled", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("disabled store should not create file, stat err=%v", statErr)
	}
}

func TestJSONStoreRedactsSecretsAndTracksUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewJSONStore(Config{Path: path, Enabled: true})

	item, err := store.Put(context.Background(), Item{
		Scope:     ScopeSession,
		SessionID: "sess-1",
		Text:      "database token=super-secret-value should be hidden",
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if strings.Contains(item.Text, "super-secret-value") || !strings.Contains(item.Text, "[REDACTED]") {
		t.Fatalf("stored text was not redacted: %q", item.Text)
	}

	results, err := store.Search(context.Background(), Query{Scope: ScopeSession, SessionID: "sess-1", Text: "database", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].UsageCount != 1 || results[0].LastUsedAt.IsZero() {
		t.Fatalf("search results = %#v, want one used memory", results)
	}
}

func TestJSONStoreSearchFiltersStaleAndLimitsResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewJSONStore(Config{Path: path, Enabled: true})
	now := time.Now().UTC()
	for _, item := range []Item{
		{Scope: ScopeProject, ProjectID: "proj", Text: "redis latency dashboard", CreatedAt: now.Add(-3 * time.Hour)},
		{Scope: ScopeProject, ProjectID: "proj", Text: "redis error budget", CreatedAt: now.Add(-2 * time.Hour)},
		{Scope: ScopeProject, ProjectID: "proj", Text: "redis stale runbook", Stale: true, CreatedAt: now.Add(-1 * time.Hour)},
	} {
		if _, err := store.Put(context.Background(), item); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}

	results, err := store.Search(context.Background(), Query{Scope: ScopeProject, ProjectID: "proj", Text: "redis", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want limit 1", len(results))
	}
	if strings.Contains(results[0].Text, "stale") {
		t.Fatalf("stale memory should not be returned: %#v", results)
	}
}
