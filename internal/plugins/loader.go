package plugins

import (
	"fmt"
	"strings"
	"sync"

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

// Loader loads plugin specs from some source.
type Loader interface {
	Load() ([]Spec, error)
}

// StaticLoader is a test and bootstrap loader backed by in-memory specs.
type StaticLoader []Spec

// Load returns cloned specs.
func (l StaticLoader) Load() ([]Spec, error) {
	out := make([]Spec, len(l))
	for i, spec := range l {
		out[i] = cloneSpec(spec)
	}
	return out, nil
}

type appliedSpec struct {
	governanceName string
	toolNames      []string
	promptCommands []commands.PromptCommand
	localCommands  []commands.LocalCommand
	skills         []skills.Definition
	agentNames     []string
	mcpServerIDs   []string
	toolHookNames  []string
	turnHookNames  []string
	lspServerIDs   []string
	outputStyles   []string
	settingNames   []string
}

// Registrar distributes plugin specs into the backing registries.
type Registrar struct {
	mu           sync.Mutex
	applied      map[string]appliedSpec
	Tools        *tooling.Registry
	Commands     *commands.CommandRegistry
	Skills       *skills.Registry
	Agents       *agents.Registry
	MCP          *mcp.Registry
	Hooks        *hooks.Registry
	LSP          *lsp.Registry
	OutputStyles *outputstyle.Registry
	Settings     *settings.Registry
	Governance   *settings.Governance
}

// Register distributes a spec into all configured registries.
func (r *Registrar) Register(spec Spec) error {
	spec = cloneSpec(spec)
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return fmt.Errorf("plugins: spec name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.applied == nil {
		r.applied = make(map[string]appliedSpec)
	}
	if _, exists := r.applied[spec.Name]; exists {
		return fmt.Errorf("plugins: spec %q already registered", spec.Name)
	}

	var applied appliedSpec
	if err := r.registerGovernance(spec, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerTools(spec, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerCommands(spec.Commands, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerSkills(spec.Skills, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerAgents(spec.Agents, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerMCPServers(spec.MCPServers, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerHooks(spec, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerLSPServers(spec.LSPServers, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerOutputStyles(spec.OutputStyles, &applied); err != nil {
		r.rollback(applied)
		return err
	}
	if err := r.registerSettings(spec.Settings, &applied); err != nil {
		r.rollback(applied)
		return err
	}

	r.applied[spec.Name] = applied
	return nil
}

// Unregister removes a previously registered plugin spec from all registries.
func (r *Registrar) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.TrimSpace(name)
	applied, ok := r.applied[name]
	if !ok {
		return fmt.Errorf("plugins: spec %q is not registered", name)
	}
	r.rollback(applied)
	delete(r.applied, name)
	return nil
}

func (r *Registrar) registerTools(spec Spec, applied *appliedSpec) error {
	if len(spec.Tools) == 0 {
		return nil
	}
	if r.Tools == nil {
		return fmt.Errorf("plugins: tool registry is required")
	}
	for _, t := range spec.Tools {
		if t == nil {
			continue
		}
		name := t.Metadata().Name
		if _, exists := r.Tools.Get(name); exists {
			return fmt.Errorf("plugins: tool %q already registered", name)
		}
		if err := r.Tools.Register(t); err != nil {
			return err
		}
		applied.toolNames = append(applied.toolNames, name)
	}
	return nil
}

func (r *Registrar) registerCommands(cmds []commands.PromptCommand, applied *appliedSpec) error {
	if len(cmds) == 0 {
		return nil
	}
	if r.Commands == nil {
		return fmt.Errorf("plugins: command registry is required")
	}
	for _, cmd := range cmds {
		if err := r.Commands.RegisterPrompt(cmd); err != nil {
			return err
		}
		applied.promptCommands = append(applied.promptCommands, cmd)
	}
	return nil
}

func (r *Registrar) registerSkills(defs []skills.Definition, applied *appliedSpec) error {
	if len(defs) == 0 {
		return nil
	}
	if r.Skills == nil {
		return fmt.Errorf("plugins: skill registry is required")
	}
	if r.Commands == nil {
		return fmt.Errorf("plugins: command registry is required for skill command surface")
	}
	for _, def := range defs {
		if strings.TrimSpace(def.Source) == "" {
			def.Source = commands.SourcePlugin
		}
		if strings.TrimSpace(def.LoadedFrom) == "" {
			def.LoadedFrom = commands.LoadedFromPlugin
		}
		if !r.Skills.RegisterWithStatus(def) {
			continue
		}
		applied.skills = append(applied.skills, def)

		cmd := skills.PromptCommandForDefinition(def, commands.SourcePlugin)
		if err := r.Commands.RegisterPrompt(cmd); err != nil {
			return err
		}
		applied.promptCommands = append(applied.promptCommands, cmd)
	}
	return nil
}

func (r *Registrar) registerAgents(defs []agents.Definition, applied *appliedSpec) error {
	if len(defs) == 0 {
		return nil
	}
	if r.Agents == nil {
		return fmt.Errorf("plugins: agent registry is required")
	}
	for _, def := range defs {
		if existing, ok := r.Agents.Get(def.Name); ok && existing.Name == def.Name {
			return fmt.Errorf("plugins: agent %q already registered", def.Name)
		}
		if err := r.Agents.Register(def); err != nil {
			return err
		}
		applied.agentNames = append(applied.agentNames, strings.TrimSpace(def.Name))
	}
	return nil
}

func (r *Registrar) registerMCPServers(servers []MCPServerSpec, applied *appliedSpec) error {
	if len(servers) == 0 {
		return nil
	}
	if r.MCP == nil {
		return fmt.Errorf("plugins: MCP registry is required")
	}
	for _, server := range servers {
		if _, exists := r.MCP.GetServer(server.Config.ID); exists {
			return fmt.Errorf("plugins: MCP server %q already registered", server.Config.ID)
		}
		if err := r.MCP.RegisterServer(server.Config); err != nil {
			return err
		}
		if len(server.Tools) > 0 {
			if err := r.MCP.OnServerConnected(server.Config.ID, server.Tools); err != nil {
				return err
			}
		}
		applied.mcpServerIDs = append(applied.mcpServerIDs, server.Config.ID)
	}
	return nil
}

func (r *Registrar) registerHooks(spec Spec, applied *appliedSpec) error {
	if len(spec.ToolHooks) == 0 && len(spec.TurnHooks) == 0 {
		return nil
	}
	if r.Hooks == nil {
		return fmt.Errorf("plugins: hook registry is required")
	}
	for _, reg := range spec.ToolHooks {
		if err := r.Hooks.RegisterTool(reg); err != nil {
			return err
		}
		applied.toolHookNames = append(applied.toolHookNames, strings.TrimSpace(reg.Name))
	}
	for _, reg := range spec.TurnHooks {
		if err := r.Hooks.RegisterTurn(reg); err != nil {
			return err
		}
		applied.turnHookNames = append(applied.turnHookNames, strings.TrimSpace(reg.Name))
	}
	return nil
}

func (r *Registrar) registerLSPServers(servers []lsp.ServerConfig, applied *appliedSpec) error {
	if len(servers) == 0 {
		return nil
	}
	if r.LSP == nil {
		return fmt.Errorf("plugins: LSP registry is required")
	}
	for _, server := range servers {
		if _, exists := r.LSP.GetServer(server.ID); exists {
			return fmt.Errorf("plugins: LSP server %q already registered", server.ID)
		}
		if err := r.LSP.RegisterServer(server); err != nil {
			return err
		}
		applied.lspServerIDs = append(applied.lspServerIDs, strings.TrimSpace(server.ID))
	}
	return nil
}

func (r *Registrar) registerOutputStyles(defs []outputstyle.Definition, applied *appliedSpec) error {
	if len(defs) == 0 {
		return nil
	}
	if r.OutputStyles == nil {
		return fmt.Errorf("plugins: output style registry is required")
	}
	for _, def := range defs {
		if _, exists := r.OutputStyles.Get(def.Name); exists {
			return fmt.Errorf("plugins: output style %q already registered", def.Name)
		}
		if err := r.OutputStyles.Register(def); err != nil {
			return err
		}
		applied.outputStyles = append(applied.outputStyles, strings.TrimSpace(def.Name))
	}
	return nil
}

func (r *Registrar) registerSettings(entries []settings.Entry, applied *appliedSpec) error {
	if len(entries) == 0 {
		return nil
	}
	if r.Settings == nil {
		return fmt.Errorf("plugins: settings registry is required")
	}
	for _, entry := range entries {
		if _, exists := r.Settings.Get(entry.Name); exists {
			return fmt.Errorf("plugins: settings entry %q already registered", entry.Name)
		}
		if err := r.Settings.Register(entry); err != nil {
			return err
		}
		applied.settingNames = append(applied.settingNames, strings.TrimSpace(entry.Name))
	}
	return nil
}

func (r *Registrar) registerGovernance(spec Spec, applied *appliedSpec) error {
	if r.Governance == nil {
		return nil
	}

	contribution := settings.GovernanceContribution{
		AllowedMCPServers:     spec.Manifest.AllowedMCPServers,
		AdditionalDirectories: spec.Manifest.AdditionalDirectories,
	}
	if spec.Manifest.StrictPluginOnlyCustomization {
		contribution.RestrictToPluginOnly = settings.AllCustomizationSurfaces()
	}
	if len(contribution.RestrictToPluginOnly) == 0 &&
		len(contribution.AllowedMCPServers) == 0 &&
		len(contribution.AdditionalDirectories) == 0 {
		return nil
	}

	snapshot, err := r.Governance.Project(spec.Name, contribution)
	if err != nil {
		return err
	}
	if err := r.validateGovernanceCompatibility(snapshot); err != nil {
		return err
	}
	if err := r.Governance.Register(spec.Name, contribution); err != nil {
		return err
	}
	applied.governanceName = spec.Name
	return nil
}

func (r *Registrar) validateGovernanceCompatibility(snapshot settings.GovernanceSnapshot) error {
	if r.Commands != nil {
		for _, cmd := range r.Commands.ListSkillLikePromptCommands() {
			if snapshot.AllowsSource(settings.SurfaceSkills, cmd.Source) {
				continue
			}
			return fmt.Errorf("plugins: strictPluginOnlyCustomization conflicts with existing skill command %q from %q", cmd.Name, cmd.Source)
		}
	}
	if r.Agents != nil {
		for _, def := range r.Agents.List() {
			if snapshot.AllowsSource(settings.SurfaceAgents, def.Source) {
				continue
			}
			return fmt.Errorf("plugins: strictPluginOnlyCustomization conflicts with existing agent %q from %q", def.Name, def.Source)
		}
	}
	if r.Hooks != nil {
		for _, reg := range r.Hooks.ListTool() {
			if snapshot.AllowsSource(settings.SurfaceHooks, reg.Source) {
				continue
			}
			return fmt.Errorf("plugins: strictPluginOnlyCustomization conflicts with existing tool hook %q from %q", reg.Name, reg.Source)
		}
		for _, reg := range r.Hooks.ListTurn() {
			if snapshot.AllowsSource(settings.SurfaceHooks, reg.Source) {
				continue
			}
			return fmt.Errorf("plugins: strictPluginOnlyCustomization conflicts with existing turn hook %q from %q", reg.Name, reg.Source)
		}
	}
	if r.MCP != nil {
		for _, cfg := range r.MCP.ListServers() {
			if !snapshot.AllowsSource(settings.SurfaceMCP, cfg.Source) {
				return fmt.Errorf("plugins: strictPluginOnlyCustomization conflicts with existing MCP server %q from %q", cfg.ID, cfg.Source)
			}
			if !snapshot.AllowsMCPServer(cfg.Source, cfg.ID) {
				return fmt.Errorf("plugins: allowedMcpServers conflicts with existing MCP server %q from %q", cfg.ID, cfg.Source)
			}
		}
	}
	return nil
}

func (r *Registrar) rollback(applied appliedSpec) {
	if applied.governanceName != "" && r.Governance != nil {
		r.Governance.Unregister(applied.governanceName)
	}
	for i := len(applied.settingNames) - 1; i >= 0; i-- {
		if r.Settings != nil {
			r.Settings.Unregister(applied.settingNames[i])
		}
	}
	for i := len(applied.outputStyles) - 1; i >= 0; i-- {
		if r.OutputStyles != nil {
			r.OutputStyles.Unregister(applied.outputStyles[i])
		}
	}
	for i := len(applied.lspServerIDs) - 1; i >= 0; i-- {
		if r.LSP != nil {
			r.LSP.UnregisterServer(applied.lspServerIDs[i])
		}
	}
	for i := len(applied.turnHookNames) - 1; i >= 0; i-- {
		if r.Hooks != nil {
			r.Hooks.UnregisterTurn(applied.turnHookNames[i])
		}
	}
	for i := len(applied.toolHookNames) - 1; i >= 0; i-- {
		if r.Hooks != nil {
			r.Hooks.UnregisterTool(applied.toolHookNames[i])
		}
	}
	for i := len(applied.mcpServerIDs) - 1; i >= 0; i-- {
		if r.MCP != nil {
			r.MCP.UnregisterServer(applied.mcpServerIDs[i])
		}
	}
	for i := len(applied.agentNames) - 1; i >= 0; i-- {
		if r.Agents != nil {
			r.Agents.Unregister(applied.agentNames[i])
		}
	}
	for i := len(applied.skills) - 1; i >= 0; i-- {
		if r.Skills != nil {
			r.Skills.UnregisterDefinition(applied.skills[i])
		}
	}
	for i := len(applied.promptCommands) - 1; i >= 0; i-- {
		if r.Commands != nil {
			r.Commands.UnregisterPromptRecord(applied.promptCommands[i])
		}
	}
	for i := len(applied.localCommands) - 1; i >= 0; i-- {
		if r.Commands != nil {
			r.Commands.UnregisterLocalRecord(applied.localCommands[i])
		}
	}
	for i := len(applied.toolNames) - 1; i >= 0; i-- {
		if r.Tools != nil {
			r.Tools.Unregister(applied.toolNames[i])
		}
	}
}

func cloneSpec(spec Spec) Spec {
	spec.Manifest = cloneManifest(spec.Manifest)
	spec.Tools = append([]tooling.Tool(nil), spec.Tools...)
	spec.Commands = append([]commands.PromptCommand(nil), spec.Commands...)
	spec.Skills = append([]skills.Definition(nil), spec.Skills...)
	spec.Agents = append([]agents.Definition(nil), spec.Agents...)
	spec.ToolHooks = append([]hooks.ToolRegistration(nil), spec.ToolHooks...)
	spec.TurnHooks = append([]hooks.TurnRegistration(nil), spec.TurnHooks...)
	spec.LSPServers = append([]lsp.ServerConfig(nil), spec.LSPServers...)
	spec.OutputStyles = append([]outputstyle.Definition(nil), spec.OutputStyles...)
	spec.Settings = append([]settings.Entry(nil), spec.Settings...)

	if len(spec.MCPServers) > 0 {
		servers := make([]MCPServerSpec, len(spec.MCPServers))
		for i, server := range spec.MCPServers {
			servers[i] = MCPServerSpec{
				Config: cloneMCPServerConfig(server.Config),
				Tools:  append([]tooling.Tool(nil), server.Tools...),
			}
		}
		spec.MCPServers = servers
	}
	for i := range spec.LSPServers {
		spec.LSPServers[i].Command = append([]string(nil), spec.LSPServers[i].Command...)
		spec.LSPServers[i].Languages = append([]string(nil), spec.LSPServers[i].Languages...)
		spec.LSPServers[i].Roots = append([]string(nil), spec.LSPServers[i].Roots...)
	}
	return spec
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.CommandsPaths = append([]string(nil), manifest.CommandsPaths...)
	manifest.AgentsPaths = append([]string(nil), manifest.AgentsPaths...)
	manifest.SkillsPaths = append([]string(nil), manifest.SkillsPaths...)
	manifest.OutputStylesPaths = append([]string(nil), manifest.OutputStylesPaths...)
	manifest.MCPServers = cloneMCPServerConfigs(manifest.MCPServers)
	manifest.LSPServers = cloneLSPServers(manifest.LSPServers)
	manifest.Settings = cloneSettingsEntries(manifest.Settings)
	manifest.AIOps = cloneAIOpsManifest(manifest.AIOps)
	manifest.AllowedMCPServers = append([]string(nil), manifest.AllowedMCPServers...)
	manifest.AdditionalDirectories = append([]string(nil), manifest.AdditionalDirectories...)
	return manifest
}
