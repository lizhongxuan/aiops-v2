package terminalsession

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTerminalSessionLifecycle(t *testing.T) {
	mgr := NewManager(Options{MaxSessions: 2})
	sess, err := mgr.Start(context.Background(), StartRequest{
		Command:   "sh",
		Args:      []string{"-c", "printf ready; sleep 0.05; printf done"},
		YieldTime: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	first := mgr.Read(sess.ID, 1024)
	if !strings.Contains(first.Stdout, "ready") {
		t.Fatalf("first output = %#v", first)
	}
	final := waitTerminalCompleted(t, mgr, sess.ID)
	if final.Status != StatusCompleted {
		t.Fatalf("final status = %q, want completed; output=%#v", final.Status, final)
	}
	if !strings.Contains(final.Stdout, "done") {
		t.Fatalf("final output = %#v", final)
	}
}

func TestTerminalSessionKill(t *testing.T) {
	mgr := NewManager(Options{MaxSessions: 1})
	sess, err := mgr.Start(context.Background(), StartRequest{
		Command:   "sh",
		Args:      []string{"-c", "sleep 5"},
		YieldTime: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Kill(sess.ID); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	state := waitTerminalStopped(t, mgr, sess.ID)
	if state.Status != StatusKilled {
		t.Fatalf("status = %q, want killed", state.Status)
	}
}

func waitTerminalCompleted(t *testing.T, mgr *Manager, id string) Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := mgr.Read(id, 4096)
		if snap.Status == StatusCompleted || snap.Status == StatusFailed {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	return mgr.Read(id, 4096)
}

func waitTerminalStopped(t *testing.T, mgr *Manager, id string) Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := mgr.Read(id, 4096)
		if snap.Status != StatusRunning {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	return mgr.Read(id, 4096)
}
