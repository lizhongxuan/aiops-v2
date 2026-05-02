package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRunCLIFiltersCasesByPriority(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLIRawCase(t, filepath.Join(casesDir, "p0.json"), `{"id":"p0-case","category":"CLI","priority":"P0","input":"Run P0","expected":{"mustInclude":["MockAgent"]}}`)
	writeCLIRawCase(t, filepath.Join(casesDir, "p1.json"), `{"id":"p1-case","category":"CLI","priority":"P1","input":"Run P1","expected":{"mustInclude":["MockAgent"]}}`)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-priority", "P0",
		"-out", outDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "- p0-case") || strings.Contains(stdout.String(), "p1-case") {
		t.Fatalf("stdout did not show P0-only run:\n%s", stdout.String())
	}
	report, err := eval.LoadReport(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if len(report.Cases) != 1 || report.Cases[0].CaseID != "p0-case" || report.Cases[0].Priority != "P0" {
		t.Fatalf("report cases = %#v, want only P0 case", report.Cases)
	}
}

func writeCLIRawCase(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create case dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data+"\n"), 0o644); err != nil {
		t.Fatalf("write raw case: %v", err)
	}
}

func TestRunCLIExecutesServerAgent(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLICase(t, casesDir, "server-basic")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/chat/message":
			writeCLIJSONResponse(t, w, map[string]any{
				"accepted":  true,
				"sessionId": "sess-cli-server",
				"turnId":    "turn-cli-server",
				"status":    "accepted",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state":
			writeCLIJSONResponse(t, w, map[string]any{
				"sessionId": "sess-cli-server",
				"cards": []map[string]any{
					{"role": "assistant", "text": "server adapter completed the local turn through /api/v1/state, with verification: go test ./internal/eval"},
				},
				"runtime": map[string]any{
					"turn": map[string]any{"active": false, "phase": "completed"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "server",
		"-server-url", server.URL,
		"-poll-timeout", "1s",
		"-poll-interval", "1ms",
		"-cases", casesDir,
		"-out", outDir,
		"-run-id", "server-run",
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "eval run: server-run") || !strings.Contains(stdout.String(), "summary: 1/1 passed") {
		t.Fatalf("stdout missing server eval summary:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "server-basic", "answer.txt")); err != nil {
		t.Fatalf("expected server answer artifact: %v", err)
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

func writeCLIJSONResponse(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response json: %v", err)
	}
}
