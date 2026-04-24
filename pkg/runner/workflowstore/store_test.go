package workflowstore

import (
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	if _, err := sanitizeName(""); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := sanitizeName("bad!name"); err == nil {
		t.Fatal("expected error for invalid name")
	}
	if got, err := sanitizeName("good-name_1"); err != nil || got != "good-name_1" {
		t.Fatalf("expected sanitized name, got %q err=%v", got, err)
	}
}

func TestWorkflowNameFromFile(t *testing.T) {
	if name, ok := workflowNameFromFile("demo.yaml"); !ok || name != "demo" {
		t.Fatalf("expected demo.yaml -> demo, got %q ok=%v", name, ok)
	}
	if name, ok := workflowNameFromFile("demo.yml"); !ok || name != "demo" {
		t.Fatalf("expected demo.yml -> demo, got %q ok=%v", name, ok)
	}
	if _, ok := workflowNameFromFile("demo.txt"); ok {
		t.Fatalf("expected demo.txt to be ignored")
	}
}

func TestStorePutNameMismatch(t *testing.T) {
	store := New(t.TempDir())
	raw := []byte(`version: v0.1
name: demo
steps:
  - name: ok
    action: cmd.run
    args:
      cmd: "echo hi"
`)
	if _, err := store.Put("other", raw); err == nil || !strings.Contains(err.Error(), "workflow name mismatch") {
		t.Fatalf("expected name mismatch error, got %v", err)
	}
}

func TestStorePutMissingName(t *testing.T) {
	store := New(t.TempDir())
	raw := []byte(`version: v0.1
steps:
  - name: ok
    action: cmd.run
    args:
      cmd: "echo hi"
`)
	if _, err := store.Put("", raw); err == nil || !strings.Contains(err.Error(), "workflow name is required") {
		t.Fatalf("expected missing name error, got %v", err)
	}
}

func TestStorePutInvalidName(t *testing.T) {
	store := New(t.TempDir())
	raw := []byte(`version: v0.1
name: bad*name
steps:
  - name: ok
    action: cmd.run
    args:
      cmd: "echo hi"
`)
	if _, err := store.Put("bad*name", raw); err == nil || !strings.Contains(err.Error(), "invalid workflow name") {
		t.Fatalf("expected invalid name error, got %v", err)
	}

	rawGood := []byte(`version: v0.1
name: good-name
steps:
  - name: ok
    action: cmd.run
    args:
      cmd: "echo hi"
`)
	if _, err := store.Put("good-name", rawGood); err != nil {
		t.Fatalf("expected valid name to succeed, got %v", err)
	}
}
