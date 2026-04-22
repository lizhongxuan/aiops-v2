package plugins

import (
	"aiops-v2/internal/agents"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/settings"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/tooling"
)

// Manifest captures the plugin manifest metadata and resolved component paths.
type Manifest struct {
	Name                          string
	ManifestPath                  string
	Root                          string
	CommandsPaths                 []string
	AgentsPaths                   []string
	SkillsPaths                   []string
	OutputStylesPaths             []string
	HooksConfig                   string
	MCPServers                    []mcp.ServerConfig
	LSPServers                    []lsp.ServerConfig
	Settings                      []settings.Entry
	StrictPluginOnlyCustomization bool
	AllowedMCPServers             []string
	AdditionalDirectories         []string
}

// MCPServerSpec packages server config plus eagerly connected tools for assembly.
type MCPServerSpec struct {
	Config mcp.ServerConfig
	Tools  []tooling.Tool
}

// Spec is the plugin assembly payload distributed into registries.
type Spec struct {
	Name         string
	Manifest     Manifest
	Tools        []tooling.Tool
	Commands     []commands.PromptCommand
	Skills       []skills.Definition
	Agents       []agents.Definition
	MCPServers   []MCPServerSpec
	ToolHooks    []hooks.ToolRegistration
	TurnHooks    []hooks.TurnRegistration
	LSPServers   []lsp.ServerConfig
	OutputStyles []outputstyle.Definition
	Settings     []settings.Entry
}
