package config

import "time"

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Stores    StoresConfig    `yaml:"stores"`
	Execution ExecutionConfig `yaml:"execution"`
	Agent     AgentConfig     `yaml:"agent"`
	Security  SecurityConfig  `yaml:"security"`
	Logging   LoggingConfig   `yaml:"logging"`
	UI        UIConfig        `yaml:"ui"`
}

type ServerConfig struct {
	Addr               string `yaml:"addr"`
	ReadTimeoutSec     int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec    int    `yaml:"write_timeout_sec"`
	ShutdownTimeoutSec int    `yaml:"shutdown_timeout_sec"`
}

type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

type StoresConfig struct {
	WorkflowsDir    string `yaml:"workflows_dir"`
	ScriptsDir      string `yaml:"scripts_dir"`
	SkillsDir       string `yaml:"skills_dir"`
	EnvironmentsDir string `yaml:"environments_dir"`
	MCPDir          string `yaml:"mcp_dir"`
	RunStateFile    string `yaml:"run_state_file"`
	AgentStateFile  string `yaml:"agent_state_file"`
}

type ExecutionConfig struct {
	MaxConcurrentRuns int  `yaml:"max_concurrent_runs"`
	QueueSize         int  `yaml:"queue_size"`
	DefaultAsync      bool `yaml:"default_async"`
	PollIntervalSec   int  `yaml:"poll_interval_sec"`
	AsyncTimeoutSec   int  `yaml:"async_timeout_sec"`
	MaxOutputBytes    int  `yaml:"max_output_bytes"`
}

type AgentConfig struct {
	RegistrationTTLSec     int    `yaml:"registration_ttl_sec"`
	OfflineGraceSec        int    `yaml:"offline_grace_sec"`
	HealthCheckIntervalSec int    `yaml:"health_check_interval_sec"`
	PreferHeartbeatStatus  bool   `yaml:"prefer_heartbeat_status"`
	DispatchToken          string `yaml:"dispatch_token"`
}

type SecurityConfig struct {
	AllowedActions []string `yaml:"allowed_actions"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type UIConfig struct {
	Enabled     bool     `yaml:"enabled"`
	DistDir     string   `yaml:"dist_dir"`
	BasePath    string   `yaml:"base_path"`
	DocsURL     string   `yaml:"docs_url"`
	RepoURL     string   `yaml:"repo_url"`
	CORSOrigins []string `yaml:"cors_origins"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Addr:               ":8090",
			ReadTimeoutSec:     15,
			WriteTimeoutSec:    60,
			ShutdownTimeoutSec: 20,
		},
		Auth: AuthConfig{
			Enabled: true,
			Token:   "runner-server-token",
		},
		Stores: StoresConfig{
			WorkflowsDir:    "./data/workflows",
			ScriptsDir:      "./data/scripts",
			SkillsDir:       "./data/skills",
			EnvironmentsDir: "./data/environments",
			MCPDir:          "./data/mcp",
			RunStateFile:    "./data/run-state.json",
			AgentStateFile:  "./data/agents.json",
		},
		Execution: ExecutionConfig{
			MaxConcurrentRuns: 10,
			QueueSize:         200,
			DefaultAsync:      true,
			PollIntervalSec:   2,
			AsyncTimeoutSec:   int((10 * time.Hour).Seconds()),
			MaxOutputBytes:    65536,
		},
		Agent: AgentConfig{
			RegistrationTTLSec:     30,
			OfflineGraceSec:        90,
			HealthCheckIntervalSec: 15,
			PreferHeartbeatStatus:  true,
			DispatchToken:          "",
		},
		Security: SecurityConfig{
			AllowedActions: []string{
				"cmd.run",
				"shell.run",
				"script.shell",
				"script.python",
				"wait.event",
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		UI: UIConfig{
			Enabled:     false,
			DistDir:     "./runner-web/frontend/dist",
			BasePath:    "/",
			DocsURL:     "https://kdcloud.io/kme/docs",
			RepoURL:     "https://github.com/kdcloud-io/kme",
			CORSOrigins: []string{"http://127.0.0.1:5174", "http://localhost:5174"},
		},
	}
}

func (c Config) ReadTimeout() time.Duration {
	return time.Duration(c.Server.ReadTimeoutSec) * time.Second
}

func (c Config) WriteTimeout() time.Duration {
	return time.Duration(c.Server.WriteTimeoutSec) * time.Second
}

func (c Config) ShutdownTimeout() time.Duration {
	return time.Duration(c.Server.ShutdownTimeoutSec) * time.Second
}
