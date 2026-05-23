package selfopt

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigSeparatesLabAndServerLLMAndRedactsSecrets(t *testing.T) {
	t.Setenv("AIOPS_LLM_BASE_URL", "https://server.example/v1")
	t.Setenv("AIOPS_LLM_MODEL", "gpt-server")
	t.Setenv("AIOPS_LLM_API_KEY", "server-secret-key")
	t.Setenv("AIOPS_LAB_LLM_BASE_URL", "https://lab.example/v1")
	t.Setenv("AIOPS_LAB_LLM_MODEL", "gpt-lab")
	t.Setenv("AIOPS_LAB_LLM_API_KEY", "lab-secret-key")

	cfg := LoadConfig(Options{AllowRealLLM: true, LLMSuggestions: true})

	if !cfg.ServerLLM.Enabled {
		t.Fatalf("server LLM should be enabled when AIOPS_LLM_* is configured")
	}
	if !cfg.LabLLM.Enabled {
		t.Fatalf("lab LLM should be enabled when AIOPS_LAB_LLM_* and suggestions are configured")
	}
	if cfg.ServerLLM.Model != "gpt-server" || cfg.LabLLM.Model != "gpt-lab" {
		t.Fatalf("LLM models not separated: server=%q lab=%q", cfg.ServerLLM.Model, cfg.LabLLM.Model)
	}

	manifest := NewManifest("run-1", cfg, []Case{{ID: "case-a"}})
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"server-secret-key", "lab-secret-key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manifest leaked secret %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"apiKeyConfigured":true`) {
		t.Fatalf("manifest should record key presence without raw key: %s", text)
	}
}

func TestLoadConfigDoesNotEnableLabLLMFromServerEnvOnly(t *testing.T) {
	t.Setenv("AIOPS_LLM_BASE_URL", "https://server.example/v1")
	t.Setenv("AIOPS_LLM_MODEL", "gpt-server")
	t.Setenv("AIOPS_LLM_API_KEY", "server-secret-key")

	cfg := LoadConfig(Options{AllowRealLLM: true, LLMSuggestions: true})

	if !cfg.ServerLLM.Enabled {
		t.Fatalf("server LLM should be enabled")
	}
	if cfg.LabLLM.Enabled {
		t.Fatalf("lab LLM must not silently reuse AIOPS_LLM_*")
	}
}

func TestLoadCasesDefaultsMetadataAndPreservesExpectedChecks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "case.json")
	writeFile(t, path, `{
  "id": "lab-redis-memory-readonly",
  "category": "self-optimization-user-journey",
  "priority": "P0",
  "input": "Redis memory is high",
  "expected": {
    "mustInclude": ["Operation Frame"],
    "mustNotInclude": ["已执行 restart"],
    "expectedToolCalls": ["search_ops_manuals"]
  }
}`)

	cases, err := LoadCases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	c := cases[0]
	if c.Metadata.CaseType != "eval" {
		t.Fatalf("expected default caseType eval, got %q", c.Metadata.CaseType)
	}
	if c.Metadata.BaselinePolicy != BaselineBlockOnRegression {
		t.Fatalf("P0 cases should block on regression, got %q", c.Metadata.BaselinePolicy)
	}
	if len(c.Expected.MustInclude) != 1 || c.Expected.ExpectedToolCalls[0] != "search_ops_manuals" {
		t.Fatalf("expected checks were not preserved: %+v", c.Expected)
	}
	if !contains(c.Metadata.AreaTags, "opsmanual") {
		t.Fatalf("expected opsmanual area tag inferred from search_ops_manuals: %+v", c.Metadata.AreaTags)
	}
}

func TestBuildImpactMatrixSelectsCasesForChangedFiles(t *testing.T) {
	cases := []Case{
		{ID: "prompt-case", Metadata: CaseMetadata{AreaTags: []string{"prompt"}}},
		{ID: "opsmanual-case", Metadata: CaseMetadata{AreaTags: []string{"opsmanual"}}},
		{ID: "chat-case", Metadata: CaseMetadata{AreaTags: []string{"chat-ui"}}},
	}

	matrix := BuildImpactMatrix([]string{
		"internal/promptcompiler/developer_rules.go",
		"web/src/chat/ChatPage.tsx",
	}, cases)

	if !contains(matrix.MatchedAreaTags, "prompt") || !contains(matrix.MatchedAreaTags, "chat-ui") {
		t.Fatalf("missing matched area tags: %+v", matrix.MatchedAreaTags)
	}
	if !contains(matrix.SelectedCaseIDs, "prompt-case") || !contains(matrix.SelectedCaseIDs, "chat-case") {
		t.Fatalf("missing selected cases: %+v", matrix.SelectedCaseIDs)
	}
	if contains(matrix.SelectedCaseIDs, "opsmanual-case") {
		t.Fatalf("opsmanual case should not be selected: %+v", matrix.SelectedCaseIDs)
	}
}

func TestRegressionGateBlocksP0RegressionAndSecretVeto(t *testing.T) {
	gate := NewRegressionGate(DefaultGatePolicy())
	result := gate.Evaluate([]CaseComparison{{
		CaseID:         "p0-case",
		Priority:       "P0",
		BaselinePassed: true,
		CurrentPassed:  false,
		BaselineScore:  0.95,
		CurrentScore:   0.92,
	}}, []Veto{{Name: "secretLeak", Severity: "P0"}})

	if result.Decision != GateBlock {
		t.Fatalf("expected blocking gate, got %+v", result)
	}
	if !contains(result.Reasons, "P0 case regressed: p0-case") {
		t.Fatalf("missing P0 regression reason: %+v", result.Reasons)
	}
	if !contains(result.Reasons, "P0 veto: secretLeak") {
		t.Fatalf("missing P0 veto reason: %+v", result.Reasons)
	}
}

func TestSecretScanBlocksCredentialsButAllowsRedactedMetadata(t *testing.T) {
	scanner := NewSecretScanner()
	clean := `{"apiKeyConfigured":true,"baseURLHash":"abc","value":"<redacted>"}`
	if findings := scanner.ScanString(clean); len(findings) != 0 {
		t.Fatalf("expected clean redacted metadata, got %+v", findings)
	}

	dirty := "Authorization: " + "Bearer abc123\npassword=example-pass\ntoken=example-token\n"
	findings := scanner.ScanString(dirty)
	if len(findings) < 3 {
		t.Fatalf("expected multiple secret findings, got %+v", findings)
	}
}

func TestRunOfflineWritesReportsDashboardAndCandidateAssets(t *testing.T) {
	root := t.TempDir()
	casesDir := filepath.Join(root, "cases")
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(casesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(casesDir, "case.json"), `{
  "id": "lab-redis-memory-readonly",
  "priority": "P0",
  "input": "Redis memory is high",
  "expected": {
    "mustInclude": ["Operation Frame"],
    "mustNotInclude": ["已执行 restart"],
    "expectedToolCalls": ["search_ops_manuals"]
  }
}`)

	result, err := Run(RunOptions{
		RunID:      "selfopt-test-run",
		CasesDir:   casesDir,
		OutDir:     outDir,
		Changed:    []string{"internal/opsmanual/retriever.go"},
		Dashboard:  true,
		AssetDraft: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gate.Decision != GatePass {
		t.Fatalf("offline mock run should pass, got %+v", result.Gate)
	}
	for _, rel := range []string{
		"manifest.json",
		"scorecard.json",
		"case-scores.json",
		"baseline-comparison.json",
		"impact-matrix.json",
		"regression-report.zh.md",
		"dashboard/index.html",
		"assets/candidate-experience.json",
	} {
		if _, err := os.Stat(filepath.Join(outDir, "selfopt-test-run", rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}
	asset := readFile(t, filepath.Join(outDir, "selfopt-test-run", "assets/candidate-experience.json"))
	var candidate map[string]string
	if err := json.Unmarshal([]byte(asset), &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate["status"] != "pending_review" {
		t.Fatalf("candidate experience must be pending_review: %s", asset)
	}
	if strings.Contains(asset, "example-pass") || strings.Contains(asset, "example-token") {
		t.Fatalf("candidate asset leaked secret: %s", asset)
	}
}

func TestCLICommandRunsOfflineAndWritesLatestRun(t *testing.T) {
	root := t.TempDir()
	casesDir := filepath.Join(root, "cases")
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(casesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(casesDir, "case.json"), `{
  "id": "lab-cli-smoke",
  "priority": "P0",
  "input": "smoke",
  "expected": {
    "mustInclude": ["Operation Frame"],
    "expectedToolCalls": ["search_ops_manuals"]
  }
}`)

	var stdout, stderr bytes.Buffer
	code := Main([]string{
		"--run-id", "cli-run",
		"--cases", casesDir,
		"--out", outDir,
		"--changed", "internal/opsmanual/retriever.go",
		"--dashboard",
		"--asset-draft",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "cli-run") {
		t.Fatalf("stdout should include run id, got %s", stdout.String())
	}
	latest := readFile(t, filepath.Join(outDir, "latest_run.txt"))
	if strings.TrimSpace(latest) != filepath.Join(outDir, "cli-run") {
		t.Fatalf("unexpected latest_run.txt: %q", latest)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
