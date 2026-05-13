package runnerembed

import (
	"path/filepath"
	"testing"
)

func TestConfigFromDataDirUsesAIOpsRunnerSubtree(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromDataDir("/var/lib/aiops")

	if cfg.Auth.Enabled {
		t.Fatal("embedded runner auth should be disabled behind ai-server")
	}
	if cfg.UI.Enabled {
		t.Fatal("embedded runner should not mount the standalone runner UI")
	}
	wantBase := filepath.Join("/var/lib/aiops", "runner")
	cases := map[string]string{
		"workflows": cfg.Stores.WorkflowsDir,
		"scripts":   cfg.Stores.ScriptsDir,
		"skills":    cfg.Stores.SkillsDir,
		"envs":      cfg.Stores.EnvironmentsDir,
		"mcp":       cfg.Stores.MCPDir,
	}
	for name, got := range cases {
		if filepath.Dir(got) != wantBase {
			t.Fatalf("%s dir = %s, want under %s", name, got, wantBase)
		}
	}
	if cfg.Stores.RunStateFile != filepath.Join(wantBase, "run-state.json") {
		t.Fatalf("run state = %s", cfg.Stores.RunStateFile)
	}
	if cfg.Stores.AgentStateFile != filepath.Join(wantBase, "agents.json") {
		t.Fatalf("agent state = %s", cfg.Stores.AgentStateFile)
	}
}
