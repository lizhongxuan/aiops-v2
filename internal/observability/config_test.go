package observability

import "testing"

func TestConfigFromEnvDefaultDisabled(t *testing.T) {
	cfg := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled {
		t.Fatal("default config should be disabled")
	}
	if cfg.Endpoint != "http://localhost:6006/v1/traces" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.ServiceName != "aiops-v2-agent" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
	if cfg.IncludePrompt {
		t.Fatal("IncludePrompt should be false by default")
	}
}

func TestConfigFromEnvEnabled(t *testing.T) {
	env := map[string]string{
		"AIOPS_OTEL_ENABLED":        "true",
		"AIOPS_OTEL_ENDPOINT":       "http://127.0.0.1:6006/v1/traces",
		"AIOPS_OTEL_SERVICE_NAME":   "local-aiops",
		"AIOPS_OTEL_PROJECT":        "agent-debug",
		"AIOPS_OTEL_INCLUDE_PROMPT": "1",
	}
	cfg := ConfigFromEnv(func(key string) string { return env[key] })
	if !cfg.Enabled {
		t.Fatal("enabled config was not enabled")
	}
	if cfg.Endpoint != "http://127.0.0.1:6006/v1/traces" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.ServiceName != "local-aiops" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
	if cfg.Project != "agent-debug" {
		t.Fatalf("project = %q", cfg.Project)
	}
	if !cfg.IncludePrompt {
		t.Fatal("IncludePrompt should be true")
	}
}

func TestConfigFromEnvTrimsValues(t *testing.T) {
	env := map[string]string{
		"AIOPS_OTEL_ENABLED":      " yes ",
		"AIOPS_OTEL_ENDPOINT":     " http://localhost:6006/v1/traces ",
		"AIOPS_OTEL_SERVICE_NAME": " local-aiops ",
		"AIOPS_OTEL_PROJECT":      " local ",
	}
	cfg := ConfigFromEnv(func(key string) string { return env[key] })
	if !cfg.Enabled {
		t.Fatal("enabled config was not enabled")
	}
	if cfg.Endpoint != "http://localhost:6006/v1/traces" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.ServiceName != "local-aiops" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
	if cfg.Project != "local" {
		t.Fatalf("project = %q", cfg.Project)
	}
}
