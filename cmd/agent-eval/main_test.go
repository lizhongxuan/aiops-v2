package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/eval"
)

func TestRunCLIExecutesMockEvalSavesBaselineAndPrintsSummary(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	baselinePath := filepath.Join(t.TempDir(), "baseline", "report.json")
	writeCLICase(t, casesDir, "cli-basic")

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-out", outDir,
		"-run-id", "cli-run",
		"-save-baseline", baselinePath,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"eval run: cli-run",
		"output: " + outDir,
		"summary: 1/1 passed, avg score 1.00",
		"- cli-basic [PASS] 1.00",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
	for _, path := range []string{
		filepath.Join(outDir, "report.json"),
		filepath.Join(outDir, "report.md"),
		filepath.Join(outDir, "cli-basic", "answer.txt"),
		baselinePath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected CLI artifact %s: %v", path, err)
		}
	}
}

func TestRunCLIPrintsBaselineComparison(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	writeCLICase(t, casesDir, "cli-basic")
	writeCLIJSON(t, baselinePath, eval.Report{Cases: []eval.CaseScore{
		{CaseID: "cli-basic", Score: 0.5, Passed: false},
		{CaseID: "missing-case", Score: 1, Passed: true},
	}})

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-out", outDir,
		"-baseline", baselinePath,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"baseline: better=1 worse=0 same=0 new=0 missing=1",
		"- cli-basic: better (0.50 -> 1.00, delta 0.50)",
		"- missing-case: missing (1.00 -> 0.00, delta 0.00)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestRunCLIUsesClockForDefaultOutputDir(t *testing.T) {
	casesDir := t.TempDir()
	workspace := t.TempDir()
	writeCLICase(t, casesDir, "cli-basic")
	t.Chdir(workspace)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	wantOut := filepath.Join(".data", "eval_runs", "20260428T010203Z")
	if !strings.Contains(stdout.String(), "output: "+wantOut) {
		t.Fatalf("stdout missing default output dir %q:\n%s", wantOut, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, wantOut, "report.json")); err != nil {
		t.Fatalf("expected default report path: %v", err)
	}
}

func TestRunCLIReturnsErrorForUnsupportedAgent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{"-agent", "real"}, &stdout, &stderr, fixedEvalNow)

	if code != 1 {
		t.Fatalf("runCLI exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unsupported agent "real"`) {
		t.Fatalf("stderr missing unsupported-agent detail:\n%s", stderr.String())
	}
}

func fixedEvalNow() time.Time {
	return time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)
}

func writeCLICase(t *testing.T, dir, id string) {
	t.Helper()
	writeCLIJSON(t, filepath.Join(dir, id+".json"), eval.Case{
		ID:       id,
		Category: "CLI",
		Input:    "Run the local eval CLI",
	})
}

func writeCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write json %s: %v", path, err)
	}
}
