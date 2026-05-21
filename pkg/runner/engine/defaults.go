package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"runner/modules"
	"runner/modules/builtin"
	runnercmd "runner/modules/cmd"
	runnerhttp "runner/modules/http"
	"runner/modules/script"
	runnershell "runner/modules/shell"
	"runner/modules/wait"
)

// DefaultRegistry returns a registry populated with built-in modules.
func DefaultRegistry() *modules.Registry {
	reg := modules.NewRegistry()
	for _, action := range runnerCoreModuleActions() {
		switch action.Module {
		case "script/shell":
			reg.Register(action.ID, script.New("shell"))
		case "script/python":
			reg.Register(action.ID, script.New("python"))
		case "http/request":
			reg.Register(action.ID, runnerhttp.New())
		case "builtin/tcp_ping":
			reg.Register(action.ID, builtin.NewTCPPing())
		case "builtin/http_check":
			reg.Register(action.ID, builtin.NewHTTPCheck())
		case "builtin/ssl_expiry_check":
			reg.Register(action.ID, builtin.NewSSLExpiryCheck())
		case "builtin/dns_resolve":
			reg.Register(action.ID, builtin.NewDNSResolve())
		case "wait/event":
			reg.Register(action.ID, wait.NewEvent())
		}
	}
	reg.Register("cmd.run", runnercmd.New())
	reg.Register("shell.run", runnershell.New())
	return reg
}

type runnerCorePluginManifest struct {
	AIOps struct {
		RunnerActions []struct {
			ID     string `json:"id"`
			Module string `json:"module"`
		} `json:"runner_actions"`
	} `json:"aiops"`
}

type runnerCoreModuleAction struct {
	ID     string
	Module string
}

func runnerCoreModuleActions() []runnerCoreModuleAction {
	data, err := os.ReadFile(runnerCoreManifestPath())
	if err != nil {
		return nil
	}
	var manifest runnerCorePluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	actions := make([]runnerCoreModuleAction, 0, len(manifest.AIOps.RunnerActions))
	for _, item := range manifest.AIOps.RunnerActions {
		id := strings.TrimSpace(item.ID)
		module := strings.TrimSpace(item.Module)
		if id != "" && module != "" {
			actions = append(actions, runnerCoreModuleAction{ID: id, Module: module})
		}
	}
	return actions
}

func runnerCoreManifestPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("plugins", "builtin", "runner-core", ".codex-plugin", "plugin.json")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "plugins", "builtin", "runner-core", ".codex-plugin", "plugin.json"))
}
