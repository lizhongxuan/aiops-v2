package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCasesDefaultsPriorityToP1(t *testing.T) {
	dir := t.TempDir()
	writePriorityCase(t, filepath.Join(dir, "case.json"), "")
	cases, err := LoadCases(dir)
	if err != nil {
		t.Fatalf("LoadCases failed: %v", err)
	}
	if cases[0].Priority != "P1" {
		t.Fatalf("priority = %q, want P1", cases[0].Priority)
	}
}

func TestLoadCasesRejectsInvalidPriority(t *testing.T) {
	dir := t.TempDir()
	writePriorityCase(t, filepath.Join(dir, "case.json"), "P9")
	_, err := LoadCases(dir)
	if err == nil {
		t.Fatal("LoadCases succeeded with invalid priority")
	}
}

func writePriorityCase(t *testing.T, path string, priority string) {
	t.Helper()
	priorityField := ""
	if priority != "" {
		priorityField = `, "priority": "` + priority + `"`
	}
	data := `{"id":"case-1","category":"prompt"` + priorityField + `,"input":"hello","expected":{"mustInclude":["ok"]}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write case: %v", err)
	}
}
