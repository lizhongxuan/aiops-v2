package config

import "testing"

func TestDefaultUIDistDirPointsAtRunnerEmbeddedDist(t *testing.T) {
	cfg := Default()

	if cfg.UI.DistDir != "./server/ui/dist" {
		t.Fatalf("expected default ui dist to point at runner embedded dist, got %q", cfg.UI.DistDir)
	}
}

func TestLoadAppliesUIBasePathEnvOverride(t *testing.T) {
	t.Setenv("RUNNER_UI_BASE_PATH", "/runner-abc")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.UI.BasePath != "/runner-abc/" {
		t.Fatalf("expected normalized base path, got %q", cfg.UI.BasePath)
	}
}

func TestLoadAppliesAgentDispatchTokenEnvOverride(t *testing.T) {
	t.Setenv("RUNNER_AGENT_TOKEN", "agent-dispatch-token")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Agent.DispatchToken != "agent-dispatch-token" {
		t.Fatalf("expected agent dispatch token override, got %q", cfg.Agent.DispatchToken)
	}
}
