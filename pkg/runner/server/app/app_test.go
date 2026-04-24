package app

import (
	"os"
	"path/filepath"
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
