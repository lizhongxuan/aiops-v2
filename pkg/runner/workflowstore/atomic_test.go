package workflowstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowNamespaceRejectsTraversalAndInvalidKey(t *testing.T) {
	t.Parallel()

	if _, err := NewWorkflowNamespace(t.TempDir(), edit.WorkflowKey{
		TenantID:   "tenant-a",
		WorkflowID: "../workflow-a",
	}); err == nil {
		t.Fatal("NewWorkflowNamespace() accepted an invalid workflow key")
	}

	namespace, err := NewWorkflowNamespace(t.TempDir(), edit.WorkflowKey{
		TenantID:   "tenant-a",
		WorkflowID: "workflow-a",
	})
	if err != nil {
		t.Fatalf("NewWorkflowNamespace() error = %v", err)
	}

	for _, relative := range []string{
		"", ".", "..", "../outside", "nested/../../outside", "/absolute",
		`C:/outside`, `nested\outside`, "nested//file", "nested/./file", "nested/file name",
	} {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			if _, err := namespace.Path(relative); err == nil {
				t.Fatalf("Path(%q) unexpectedly succeeded", relative)
			}
		})
	}

	path, err := namespace.Path("transactions/current.json")
	if err != nil {
		t.Fatalf("Path(valid) error = %v", err)
	}
	if relative, err := filepath.Rel(namespace.Dir(), path); err != nil || relative != filepath.Join("transactions", "current.json") {
		t.Fatalf("Path(valid) = %q, relative = %q, err = %v", path, relative, err)
	}
}

func TestWorkflowNamespaceSeparatesTenantAndDelimiterCollisions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keys := []edit.WorkflowKey{
		{TenantID: "tenant-a", WorkflowID: "shared-workflow"},
		{TenantID: "tenant-b", WorkflowID: "shared-workflow"},
		{TenantID: "a-b", WorkflowID: "c"},
		{TenantID: "a", WorkflowID: "b-c"},
	}
	dirs := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		namespace, err := NewWorkflowNamespace(root, key)
		if err != nil {
			t.Fatalf("NewWorkflowNamespace(%+v) error = %v", key, err)
		}
		if _, exists := dirs[namespace.Dir()]; exists {
			t.Fatalf("namespace collision for %+v: %q", key, namespace.Dir())
		}
		dirs[namespace.Dir()] = struct{}{}
		relative, err := filepath.Rel(root, namespace.Dir())
		if err != nil {
			t.Fatalf("Rel() error = %v", err)
		}
		for _, segment := range strings.Split(relative, string(filepath.Separator)) {
			if segment == key.TenantID || segment == key.WorkflowID {
				t.Fatalf("namespace path exposes an enumerable identifier for %+v: %q", key, namespace.Dir())
			}
		}
	}
}

func TestWorkflowNamespaceAtomicWriteReplacesCompleteFile(t *testing.T) {
	t.Parallel()

	namespace, err := NewWorkflowNamespace(t.TempDir(), edit.WorkflowKey{
		TenantID:   "tenant-a",
		WorkflowID: "workflow-a",
	})
	if err != nil {
		t.Fatalf("NewWorkflowNamespace() error = %v", err)
	}

	first := bytes.Repeat([]byte("first-generation\n"), 1024)
	second := bytes.Repeat([]byte("second-generation\n"), 2048)
	for _, contents := range [][]byte{first, second} {
		if err := namespace.WithLock(context.Background(), func(locked *WorkflowNamespace) error {
			return locked.AtomicWriteFile("transactions/current.json", contents, 0o640)
		}); err != nil {
			t.Fatalf("WithLock(AtomicWriteFile) error = %v", err)
		}
	}

	target, err := namespace.Path("transactions/current.json")
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("atomic result has %d bytes, want complete %d-byte generation", len(got), len(second))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o640 {
			t.Fatalf("file mode = %o, want 640", gotMode)
		}
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".atomic-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", temps)
	}
}

func TestWorkflowNamespaceAtomicWriteRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires additional privileges on Windows")
	}

	namespace, err := NewWorkflowNamespace(t.TempDir(), edit.WorkflowKey{
		TenantID:   "tenant-a",
		WorkflowID: "workflow-a",
	})
	if err != nil {
		t.Fatalf("NewWorkflowNamespace() error = %v", err)
	}
	if err := os.MkdirAll(namespace.Dir(), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(namespace.Dir(), "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err = namespace.AtomicWriteFile("escape/current.json", []byte("must not escape"), 0o600)
	if err == nil {
		t.Fatal("AtomicWriteFile() followed a namespace-internal symlink outside the namespace")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "current.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside file exists or Stat failed unexpectedly: %v", statErr)
	}
}

func TestWorkflowNamespaceCrossProcessLock(t *testing.T) {
	if os.Getenv("WORKFLOWSTORE_LOCK_HELPER") == "1" {
		runWorkflowNamespaceLockHelper(t)
		return
	}

	root := t.TempDir()
	ready := filepath.Join(root, "helper.ready")
	release := filepath.Join(root, "helper.release")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkflowNamespaceCrossProcessLock$")
	command.Env = append(os.Environ(),
		"WORKFLOWSTORE_LOCK_HELPER=1",
		"WORKFLOWSTORE_LOCK_ROOT="+root,
		"WORKFLOWSTORE_LOCK_READY="+ready,
		"WORKFLOWSTORE_LOCK_RELEASE="+release,
	)
	var helperOutput bytes.Buffer
	command.Stdout = &helperOutput
	command.Stderr = &helperOutput
	if err := command.Start(); err != nil {
		t.Fatalf("helper Start() error = %v", err)
	}
	defer func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	}()
	waitForFile(t, ready, 5*time.Second)

	namespace, err := NewWorkflowNamespace(root, testWorkflowLockKey())
	if err != nil {
		t.Fatalf("NewWorkflowNamespace() error = %v", err)
	}
	lockContext, lockCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer lockCancel()
	lock, err := namespace.Lock(lockContext)
	if lock != nil {
		_ = lock.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second process Lock() error = %v, want context deadline exceeded", err)
	}

	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatalf("release helper: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("helper Wait() error = %v\n%s", err, helperOutput.String())
	}

	lock, err = namespace.Lock(context.Background())
	if err != nil {
		t.Fatalf("Lock() after helper release error = %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("released lock Close() error = %v", err)
	}
}

func runWorkflowNamespaceLockHelper(t *testing.T) {
	namespace, err := NewWorkflowNamespace(os.Getenv("WORKFLOWSTORE_LOCK_ROOT"), testWorkflowLockKey())
	if err != nil {
		t.Fatalf("helper NewWorkflowNamespace() error = %v", err)
	}
	lock, err := namespace.Lock(context.Background())
	if err != nil {
		t.Fatalf("helper Lock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()
	if err := os.WriteFile(os.Getenv("WORKFLOWSTORE_LOCK_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatalf("helper ready: %v", err)
	}
	waitForFile(t, os.Getenv("WORKFLOWSTORE_LOCK_RELEASE"), 8*time.Second)
}

func testWorkflowLockKey() edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-lock-test"}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", path)
}
