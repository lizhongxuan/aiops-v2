package config

import "testing"

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
