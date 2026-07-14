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

func TestRunCLIWritesDiagnosisReports(t *testing.T) {
	workspace := t.TempDir()
	reportPath := filepath.Join(workspace, "report.json")
	outDir := filepath.Join(workspace, "out")
	writeCLIJSON(t, reportPath, eval.Report{
		RunID:   "cli-diagnose",
		Summary: eval.ReportSummary{Total: 1, Passed: 1, AvgScore: 1},
		Cases:   []eval.CaseScore{{CaseID: "case-1", Passed: true, Score: 1}},
	})

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-report", reportPath,
		"-cases", filepath.Join(workspace, "missing-cases"),
		"-trace-dir", filepath.Join(workspace, "missing-traces"),
		"-out", outDir,
	}, &stdout, &stderr, func() time.Time {
		return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	})

	if code != 0 {
		t.Fatalf("runCLI exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "prompt diagnosis: "+outDir) {
		t.Fatalf("stdout = %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "diagnosis.zh.md")); err != nil {
		t.Fatalf("expected diagnosis.zh.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "run-metadata.json")); err != nil {
		t.Fatalf("expected run-metadata.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "trend.zh.md")); err != nil {
		t.Fatalf("expected trend.zh.md: %v", err)
	}
}

func TestRunCLIWritesDraftCasesWhenRequested(t *testing.T) {
	workspace := t.TempDir()
	casesDir := filepath.Join(workspace, "cases")
	reportPath := filepath.Join(workspace, "report.json")
	outDir := filepath.Join(workspace, "out")
	draftDir := filepath.Join(workspace, "drafts")
	writeCLIJSON(t, filepath.Join(casesDir, "case-1.json"), eval.Case{
		ID:       "case-1",
		Category: "prompt",
		Input:    "input",
	})
	writeCLIJSON(t, reportPath, eval.Report{
		RunID:   "cli-diagnose",
		Summary: eval.ReportSummary{Total: 1, Failed: 1},
		Cases: []eval.CaseScore{{
			CaseID: "case-1",
			Passed: false,
			Score:  0,
			Checks: []eval.CheckResult{{Name: "mustInclude", Passed: false}},
		}},
	})

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-report", reportPath,
		"-cases", casesDir,
		"-trace-dir", filepath.Join(workspace, "missing-traces"),
		"-out", outDir,
		"-draft-cases-out", draftDir,
	}, &stdout, &stderr, func() time.Time {
		return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	})

	if code != 0 {
		t.Fatalf("runCLI exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "draft cases: 1 written") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(draftDir, "case-1.json")); err != nil {
		t.Fatalf("expected draft case: %v", err)
	}
}

func TestRunCLIReadsLLMAPIKeyFromEnvironment(t *testing.T) {
	workspace := t.TempDir()
	reportPath := filepath.Join(workspace, "report.json")
	outDir := filepath.Join(workspace, "out")
	writeCLIJSON(t, reportPath, eval.Report{
		RunID:   "cli-diagnose-llm",
		Summary: eval.ReportSummary{Total: 1, Failed: 1},
		Cases: []eval.CaseScore{{
			CaseID: "case-llm",
			Passed: false,
			Score:  0,
			Checks: []eval.CheckResult{{Name: "mustInclude", Passed: false}},
		}},
	})

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"修改 tool surface policy，不要改全局 prompt。"}}]}`))
	}))
	defer server.Close()
	t.Setenv("AIOPS_LAB_LLM_API_KEY", "env-secret-key")

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-report", reportPath,
		"-cases", filepath.Join(workspace, "missing-cases"),
		"-trace-dir", filepath.Join(workspace, "missing-traces"),
		"-out", outDir,
		"-llm-suggestions",
		"-llm-base-url", server.URL + "/v1",
		"-llm-model", "glm-5.1",
	}, &stdout, &stderr, func() time.Time {
		return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	})

	if code != 0 {
		t.Fatalf("runCLI exit=%d stderr=%s", code, stderr.String())
	}
	if authHeader != "Bearer env-secret-key" {
		t.Fatalf("Authorization header = %q, want env key", authHeader)
	}
	if strings.Contains(stdout.String()+stderr.String(), "env-secret-key") {
		t.Fatalf("CLI output leaked LLM API key")
	}
}

func TestRunCLIRequiresReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), nil, &stdout, &stderr, time.Now)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-report is required") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func writeCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
