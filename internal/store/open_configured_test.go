package store

import (
	"strings"
	"testing"
	"time"
)

func TestOpenConfiguredStore(t *testing.T) {
	t.Run("json returns json file store", func(t *testing.T) {
		got, err := OpenConfiguredStore(OpenConfig{
			DataDir:    t.TempDir(),
			Driver:     "json",
			FlushEvery: time.Hour,
		})
		if err != nil {
			t.Fatalf("OpenConfiguredStore() error = %v", err)
		}
		defer got.Close()
		if _, ok := got.(*JSONFileStore); !ok {
			t.Fatalf("OpenConfiguredStore() type = %T, want *JSONFileStore", got)
		}
	})

	t.Run("postgres requires dsn", func(t *testing.T) {
		_, err := OpenConfiguredStore(OpenConfig{DataDir: t.TempDir(), Driver: "postgres"})
		if err == nil || !strings.Contains(err.Error(), "AIOPS_POSTGRES_DSN") {
			t.Fatalf("OpenConfiguredStore() error = %v, want postgres dsn error", err)
		}
	})

	t.Run("mysql requires dsn", func(t *testing.T) {
		_, err := OpenConfiguredStore(OpenConfig{DataDir: t.TempDir(), Driver: "mysql"})
		if err == nil || !strings.Contains(err.Error(), "AIOPS_MYSQL_DSN") {
			t.Fatalf("OpenConfiguredStore() error = %v, want mysql dsn error", err)
		}
	})

	t.Run("unknown driver rejected", func(t *testing.T) {
		_, err := OpenConfiguredStore(OpenConfig{DataDir: t.TempDir(), Driver: "sqlite"})
		if err == nil || !strings.Contains(err.Error(), "unsupported store driver") {
			t.Fatalf("OpenConfiguredStore() error = %v, want unsupported driver error", err)
		}
	})
}
