package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCasesAcceptsRootCauseCategory(t *testing.T) {
	dir := t.TempDir()
	writeRootCauseCase(t, filepath.Join(dir, "case.json"), "prompt")

	cases, err := LoadCases(dir)
	if err != nil {
		t.Fatalf("LoadCases failed: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	if cases[0].RootCauseCategory != "prompt" {
		t.Fatalf("root cause category = %q, want prompt", cases[0].RootCauseCategory)
	}
}

func TestLoadCasesRejectsInvalidRootCauseCategory(t *testing.T) {
	dir := t.TempDir()
	writeRootCauseCase(t, filepath.Join(dir, "case.json"), "network")

	_, err := LoadCases(dir)
	if err == nil {
		t.Fatal("LoadCases succeeded with invalid rootCauseCategory")
	}
	if !strings.Contains(err.Error(), "invalid rootCauseCategory") {
		t.Fatalf("error = %v, want invalid rootCauseCategory", err)
	}
}

func TestScoreCaseCarriesRootCauseCategory(t *testing.T) {
	score := ScoreCase(Case{
		ID:                "case-1",
		Category:          "debug",
		RootCauseCategory: "tool",
		Expected: Expected{
			MustInclude: []string{"ok"},
		},
	}, RunOutput{Answer: "ok 验证方式 go test ./internal/eval"})

	if score.RootCauseCategory != "tool" {
		t.Fatalf("score root cause category = %q, want tool", score.RootCauseCategory)
	}
}

func writeRootCauseCase(t *testing.T, path string, rootCause string) {
	t.Helper()
	data := `{
  "id": "case-1",
  "category": "debug",
  "rootCauseCategory": "` + rootCause + `",
  "input": "最小失败复现",
  "expected": {
    "mustInclude": ["ok"]
  }
}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write case: %v", err)
	}
}
