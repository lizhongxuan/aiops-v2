package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func TestActiveTurnMigrateCLIJsonDryRunAndApply(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	seed, err := store.NewJSONFileStore(dataDir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	session := &runtimekernel.SessionState{
		ID:        "session-1",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeExecute,
		CreatedAt: now,
		UpdatedAt: now,
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:          "turn-bad",
			SessionID:   "session-1",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeExecute,
			Lifecycle:   runtimekernel.TurnLifecycleRunning,
			ResumeState: runtimekernel.TurnResumeStateNone,
			StartedAt:   now,
			UpdatedAt:   now,
		},
	}
	if err := seed.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := seed.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var dryRunOut bytes.Buffer
	if err := runCLI([]string{"--data-dir", dataDir, "--store-driver", "json", "--dry-run", "--output", "json"}, &dryRunOut, &bytes.Buffer{}, func(string) string { return "" }); err != nil {
		t.Fatalf("dry-run runCLI() error = %v", err)
	}
	if !strings.Contains(dryRunOut.String(), `"dryRun": true`) {
		t.Fatalf("dry-run output = %s, want dryRun true", dryRunOut.String())
	}
	reopened, err := store.NewJSONFileStore(dataDir, time.Hour)
	if err != nil {
		t.Fatalf("reopen after dry-run error = %v", err)
	}
	unchanged, err := reopened.GetSession("session-1")
	if err != nil {
		t.Fatalf("GetSession() after dry-run error = %v", err)
	}
	if unchanged.CurrentTurn.Lifecycle != runtimekernel.TurnLifecycleRunning {
		t.Fatalf("dry-run lifecycle = %q, want running", unchanged.CurrentTurn.Lifecycle)
	}
	reopened.Close()

	var applyOut bytes.Buffer
	if err := runCLI([]string{"--data-dir", dataDir, "--store-driver", "json", "--apply", "--output", "text"}, &applyOut, &bytes.Buffer{}, func(string) string { return "" }); err != nil {
		t.Fatalf("apply runCLI() error = %v", err)
	}
	if !strings.Contains(applyOut.String(), "changed=1") {
		t.Fatalf("apply output = %s, want changed=1", applyOut.String())
	}
	appliedStore, err := store.NewJSONFileStore(dataDir, time.Hour)
	if err != nil {
		t.Fatalf("reopen after apply error = %v", err)
	}
	applied, err := appliedStore.GetSession("session-1")
	if err != nil {
		t.Fatalf("GetSession() after apply error = %v", err)
	}
	if applied.CurrentTurn.Lifecycle != runtimekernel.TurnLifecycleFailed {
		t.Fatalf("apply lifecycle = %q, want failed", applied.CurrentTurn.Lifecycle)
	}
	appliedStore.Close()
}

func TestActiveTurnMigrateCLIRejectsApplyAndDryRunTogether(t *testing.T) {
	err := runCLI([]string{"--data-dir", t.TempDir(), "--apply", "--dry-run"}, &bytes.Buffer{}, &bytes.Buffer{}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("runCLI() error = %v, want mutual exclusion error", err)
	}
}
