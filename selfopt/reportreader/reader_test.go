package reportreader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRunSummaryCollectsPromptRegressionArtifacts(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "self-opt-run")
	writeFile(t, filepath.Join(root, "latest_run.txt"), runDir+"\n")
	writeFile(t, filepath.Join(runDir, "manifest.json"), `{"run_id":"self-opt-run","agent":"mock"}`)
	writeFile(t, filepath.Join(runDir, "prompt-regression-synthetic", "eval", "report.json"), `{
  "summary": {"total": 2, "passed": 1, "failed": 1, "avgScore": 0.72},
  "cases": [
    {"caseId": "case-a", "passed": true, "score": 1.0},
    {"caseId": "case-b", "passed": false, "score": 0.44, "checks": [{"name":"mustInclude","passed":false,"missing":["approval"]}]}
  ]
}`)
	writeFile(t, filepath.Join(runDir, "prompt-regression-synthetic", "diagnosis.json"), `{
  "summary": {"total": 2, "passed": 1, "failed": 1, "avgScore": 0.72, "worse": 1},
  "cases": [
    {"caseId": "case-b", "passed": false, "score": 0.44, "likelyRootCause": "missing approval", "failedChecks": ["mustInclude"]}
  ]
}`)

	summary, err := ReadLatest(root)
	if err != nil {
		t.Fatal(err)
	}

	if summary.RunDir != runDir {
		t.Fatalf("unexpected run dir: %q", summary.RunDir)
	}
	if len(summary.Reports) != 1 {
		t.Fatalf("expected one report, got %+v", summary.Reports)
	}
	report := summary.Reports[0]
	if report.Name != "prompt-regression-synthetic" {
		t.Fatalf("unexpected report name: %q", report.Name)
	}
	if report.Total != 2 || report.Failed != 1 || report.AvgScore != 0.72 {
		t.Fatalf("unexpected report summary: %+v", report)
	}
	if len(report.FailedCases) != 1 || report.FailedCases[0].CaseID != "case-b" {
		t.Fatalf("expected failed case-b, got %+v", report.FailedCases)
	}
	if report.FailedCases[0].LikelyRootCause != "missing approval" {
		t.Fatalf("diagnosis root cause not propagated: %+v", report.FailedCases[0])
	}
}

func TestReadRunIncludesPassingWorseCases(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "self-opt-run")
	writeFile(t, filepath.Join(runDir, "prompt-regression-core", "eval", "report.json"), `{
  "summary": {"total": 1, "passed": 1, "failed": 0, "avgScore": 0.91},
  "cases": [
    {"caseId": "case-regressed", "passed": true, "score": 0.91, "checks": [{"name":"mustInclude","passed":true}]}
  ],
  "baselineComparison": {
    "summary": {"better": 0, "worse": 1, "same": 0, "new": 0, "missing": 0},
    "cases": [
      {"caseId":"case-regressed","baselineScore":1,"currentScore":0.91,"delta":-0.09,"baselinePassed":true,"currentPassed":true,"status":"worse","regressedChecks":["manualSelection"]}
    ]
  }
}`)
	writeFile(t, filepath.Join(runDir, "prompt-regression-core", "diagnosis.json"), `{
  "summary": {"total": 1, "passed": 1, "failed": 0, "avgScore": 0.91, "worse": 1},
  "cases": [
    {"caseId": "case-regressed", "passed": true, "score": 0.91, "movement": "worse", "likelyRootCause": "baseline regression", "failedChecks": ["manualSelection"]}
  ]
}`)

	summary, err := ReadRun(runDir)
	if err != nil {
		t.Fatal(err)
	}
	report := summary.Reports[0]
	if report.Worse != 1 {
		t.Fatalf("expected worse count from diagnosis/report, got %+v", report)
	}
	if len(report.FailedCases) != 1 {
		t.Fatalf("expected passing worse case to be surfaced, got %+v", report.FailedCases)
	}
	got := report.FailedCases[0]
	if got.CaseID != "case-regressed" || got.Movement != "worse" || got.LikelyRootCause != "baseline regression" {
		t.Fatalf("unexpected regression case: %+v", got)
	}
}

func TestReadRunErrorsWhenNoPromptRegressionReports(t *testing.T) {
	runDir := t.TempDir()
	_, err := ReadRun(runDir)
	if err == nil {
		t.Fatal("expected error when run dir has no prompt regression reports")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
