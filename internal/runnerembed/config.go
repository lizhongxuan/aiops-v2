package runnerembed

import (
	"path/filepath"

	"runner/server/config"
)

func ConfigFromDataDir(dataDir string) config.Config {
	base := filepath.Join(dataDir, "runner")
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.Auth.Token = ""
	cfg.UI.Enabled = false
	cfg.UI.DistDir = ""
	cfg.UI.BasePath = "/"
	cfg.UI.CORSOrigins = nil
	cfg.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(base, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(base, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(base, "environments")
	cfg.Stores.MCPDir = filepath.Join(base, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(base, "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(base, "agents.json")
	return cfg
}
