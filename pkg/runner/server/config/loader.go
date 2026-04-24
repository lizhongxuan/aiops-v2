package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	cfg := Default()
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		applyEnvOverrides(&cfg)
		cfg.UI.BasePath = normalizeBasePath(cfg.UI.BasePath)
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(trimmed)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	applyEnvOverrides(&cfg)
	cfg.UI.BasePath = normalizeBasePath(cfg.UI.BasePath)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if token := strings.TrimSpace(os.Getenv("RUNNER_TOKEN")); token != "" {
		cfg.Auth.Token = token
	}
	if token := strings.TrimSpace(os.Getenv("RUNNER_AGENT_TOKEN")); token != "" {
		cfg.Agent.DispatchToken = token
	}
	if addr := strings.TrimSpace(os.Getenv("RUNNER_SERVER_ADDR")); addr != "" {
		cfg.Server.Addr = addr
	}
	if distDir := strings.TrimSpace(os.Getenv("RUNNER_UI_DIST_DIR")); distDir != "" {
		cfg.UI.DistDir = distDir
	}
	if basePath := strings.TrimSpace(os.Getenv("RUNNER_UI_BASE_PATH")); basePath != "" {
		cfg.UI.BasePath = basePath
	}
}

func normalizeBasePath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value == "/" {
		return "/"
	}
	return path.Clean("/"+value) + "/"
}

func (c Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.Server.Addr) == "" {
		errs = append(errs, fmt.Errorf("server.addr is required"))
	}
	if c.Server.ReadTimeoutSec <= 0 {
		errs = append(errs, fmt.Errorf("server.read_timeout_sec must be > 0"))
	}
	if c.Server.WriteTimeoutSec <= 0 {
		errs = append(errs, fmt.Errorf("server.write_timeout_sec must be > 0"))
	}
	if c.Server.ShutdownTimeoutSec <= 0 {
		errs = append(errs, fmt.Errorf("server.shutdown_timeout_sec must be > 0"))
	}

	if c.Auth.Enabled && strings.TrimSpace(c.Auth.Token) == "" {
		errs = append(errs, fmt.Errorf("auth.token is required when auth.enabled=true"))
	}

	if strings.TrimSpace(c.Stores.WorkflowsDir) == "" {
		errs = append(errs, fmt.Errorf("stores.workflows_dir is required"))
	}
	if strings.TrimSpace(c.Stores.ScriptsDir) == "" {
		errs = append(errs, fmt.Errorf("stores.scripts_dir is required"))
	}
	if strings.TrimSpace(c.Stores.SkillsDir) == "" {
		errs = append(errs, fmt.Errorf("stores.skills_dir is required"))
	}
	if strings.TrimSpace(c.Stores.EnvironmentsDir) == "" {
		errs = append(errs, fmt.Errorf("stores.environments_dir is required"))
	}
	if strings.TrimSpace(c.Stores.MCPDir) == "" {
		errs = append(errs, fmt.Errorf("stores.mcp_dir is required"))
	}
	if strings.TrimSpace(c.Stores.RunStateFile) == "" {
		errs = append(errs, fmt.Errorf("stores.run_state_file is required"))
	}
	if strings.TrimSpace(c.Stores.AgentStateFile) == "" {
		errs = append(errs, fmt.Errorf("stores.agent_state_file is required"))
	}

	if c.Execution.MaxConcurrentRuns <= 0 {
		errs = append(errs, fmt.Errorf("execution.max_concurrent_runs must be > 0"))
	}
	if c.Execution.QueueSize <= 0 {
		errs = append(errs, fmt.Errorf("execution.queue_size must be > 0"))
	}
	if c.Execution.PollIntervalSec <= 0 {
		errs = append(errs, fmt.Errorf("execution.poll_interval_sec must be > 0"))
	}
	if c.Execution.AsyncTimeoutSec <= 0 {
		errs = append(errs, fmt.Errorf("execution.async_timeout_sec must be > 0"))
	}
	if c.Execution.MaxOutputBytes <= 0 {
		errs = append(errs, fmt.Errorf("execution.max_output_bytes must be > 0"))
	}

	if c.Agent.RegistrationTTLSec <= 0 {
		errs = append(errs, fmt.Errorf("agent.registration_ttl_sec must be > 0"))
	}
	if c.Agent.OfflineGraceSec <= 0 {
		errs = append(errs, fmt.Errorf("agent.offline_grace_sec must be > 0"))
	}
	if c.Agent.HealthCheckIntervalSec <= 0 {
		errs = append(errs, fmt.Errorf("agent.health_check_interval_sec must be > 0"))
	}

	if strings.TrimSpace(c.Logging.Level) == "" {
		errs = append(errs, fmt.Errorf("logging.level is required"))
	}
	if strings.TrimSpace(c.Logging.Format) == "" {
		errs = append(errs, fmt.Errorf("logging.format is required"))
	}
	if c.UI.Enabled && strings.TrimSpace(c.UI.DistDir) == "" {
		errs = append(errs, fmt.Errorf("ui.dist_dir is required when ui.enabled=true"))
	}

	return errors.Join(errs...)
}
