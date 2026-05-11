package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"runner/server/config"
)

func TestReadinessCheckerRequiresUIDist(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := config.Default()
	cfg.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(base, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(base, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(base, "envs")
	cfg.Stores.MCPDir = filepath.Join(base, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(base, "data", "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(base, "data", "agents.json")
	cfg.UI.Enabled = true
	cfg.UI.DistDir = filepath.Join(base, "dist-missing")

	checker := readinessChecker{cfg: cfg}
	if err := checker.Ready(nil); err == nil {
		t.Fatal("expected missing dist to fail readiness")
	}

	cfg.UI.DistDir = filepath.Join(base, "dist")
	if err := os.MkdirAll(cfg.UI.DistDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.UI.DistDir, "index.html"), []byte("<html>runner-web</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	checker = readinessChecker{cfg: cfg}
	if err := checker.Ready(nil); err != nil {
		t.Fatalf("expected dist readiness success, got %v", err)
	}
}

func TestNewRuntimeBuildsHandlerWithoutListening(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.UI.Enabled = false
	cfg.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(base, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(base, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(base, "envs")
	cfg.Stores.MCPDir = filepath.Join(base, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(base, "data", "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(base, "data", "agents.json")

	runtime, err := NewRuntime(t.Context(), RuntimeOptions{Config: cfg})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	defer func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/actions/catalog", nil)
	rec := httptest.NewRecorder()
	runtime.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body == "" || !strings.Contains(body, "cmd.run") {
		t.Fatalf("action catalog body = %q, want cmd.run", body)
	}
}
