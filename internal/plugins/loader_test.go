package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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

type stubTool struct {
	meta tooling.ToolMetadata
}

func (t stubTool) Metadata() tooling.ToolMetadata { return t.meta }
func (t stubTool) InputSchema() json.RawMessage   { return nil }
func (t stubTool) OutputSchema() json.RawMessage  { return nil }
func (t stubTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return t.meta.Description
}
func (t stubTool) Prompt(tooling.PromptContext) string { return t.meta.Description }
func (t stubTool) IsEnabled(tooling.ToolContext) bool  { return true }
func (t stubTool) IsReadOnly(json.RawMessage) bool     { return true }
func (t stubTool) IsDestructive(json.RawMessage) bool  { return false }
func (t stubTool) IsConcurrencySafe(json.RawMessage) bool {
	return true
}
func (t stubTool) ValidateInput(context.Context, json.RawMessage) error { return nil }
func (t stubTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (t stubTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func TestStaticLoaderReturnsClonedSpecs(t *testing.T) {
	loader := StaticLoader{
		{
			Name: "plugin-a",
			Manifest: Manifest{
				Name:              "plugin-a",
				AllowedMCPServers: []string{"plugin-mcp"},
			},
			Commands: []commands.PromptCommand{
				{Name: "command-a", Prompt: "prompt-a", Source: commands.SourcePlugin},
			},
		},
	}

	specs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("Load() len = %d, want 1", len(specs))
	}

	specs[0].Commands[0].Name = "mutated"
	specs[0].Manifest.AllowedMCPServers[0] = "mutated-mcp"
	again, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() second call error = %v", err)
	}
	if again[0].Commands[0].Name != "command-a" {
		t.Fatalf("StaticLoader should return cloned specs, got %#v", again[0].Commands[0])
	}
	if again[0].Manifest.AllowedMCPServers[0] != "plugin-mcp" {
		t.Fatalf("StaticLoader should clone manifest metadata, got %#v", again[0].Manifest.AllowedMCPServers)
	}
}

func TestRegistrarRegistersSpecAcrossRegistries(t *testing.T) {
	commandRegistry := commands.NewRegistry()
	skillRegistry := skills.NewRegistry()
	agentRegistry := agents.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	hookRegistry := hooks.NewRegistry()
	lspRegistry := lsp.NewRegistry()
	outputStyleRegistry := outputstyle.NewRegistry()
	settingsRegistry := settings.NewRegistry()

	registrar := &Registrar{
		Commands:     commandRegistry,
		Skills:       skillRegistry,
		Agents:       agentRegistry,
		MCP:          mcpRegistry,
		Hooks:        hookRegistry,
		LSP:          lspRegistry,
		OutputStyles: outputStyleRegistry,
		Settings:     settingsRegistry,
	}

	spec := Spec{
		Name: "plugin-a",
		Commands: []commands.PromptCommand{
			{Name: "direct-command", Prompt: "direct prompt", Source: commands.SourcePlugin},
		},
		Skills: []skills.Definition{
			{Name: "plugin-skill", Description: "skill desc", Prompt: "skill prompt"},
		},
		Agents: []agents.Definition{
			{
				Kind:   "planner",
				Name:   "plugin-planner",
				Source: string(agents.SourcePlugin),
			},
		},
		MCPServers: []MCPServerSpec{
			{
				Config: mcp.ServerConfig{ID: "plugin-mcp", Transport: "stdio", Command: []string{"plugin-mcp"}},
				Tools: []tooling.Tool{
					stubTool{meta: tooling.ToolMetadata{Name: "plugin.query", Description: "query", IsMCP: true}},
				},
			},
		},
		ToolHooks: []hooks.ToolRegistration{
			{
				Name:  "plugin-pre-tool",
				Stage: hooks.StagePreToolUse,
				Hook:  func(context.Context, *hooks.ToolEvent) error { return nil },
			},
		},
		LSPServers: []lsp.ServerConfig{
			{
				ID:        "plugin-lsp",
				Name:      "plugin-lsp",
				Command:   []string{"plugin-lsp"},
				Languages: []string{"go"},
			},
		},
		OutputStyles: []outputstyle.Definition{
			{
				Name:   "plugin-style",
				Prompt: "Use plugin formatting",
				Source: "plugin",
			},
		},
		Settings: []settings.Entry{
			{Name: "plugin-settings", Values: map[string]any{"enabled": true}},
		},
	}

	if err := registrar.Register(spec); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, ok := commandRegistry.GetPrompt("direct-command"); !ok {
		t.Fatal("expected direct command to be registered")
	}
	if _, ok := skillRegistry.Get("plugin-skill"); !ok {
		t.Fatal("expected plugin skill to be registered")
	}
	skillCmd, ok := commandRegistry.GetPrompt("plugin-skill")
	if !ok {
		t.Fatal("expected plugin skill to be exposed via command surface")
	}
	if skillCmd.Source != commands.SourcePlugin {
		t.Fatalf("plugin skill command source = %q, want %q", skillCmd.Source, commands.SourcePlugin)
	}
	if _, ok := agentRegistry.Get("plugin-planner"); !ok {
		t.Fatal("expected plugin agent to be registered")
	}
	if _, ok := mcpRegistry.GetServer("plugin-mcp"); !ok {
		t.Fatal("expected plugin MCP server config to be registered")
	}
	if len(mcpRegistry.ListServerTools("plugin-mcp")) != 1 {
		t.Fatalf("expected plugin MCP tools to be connected")
	}
	if _, ok := lspRegistry.GetServer("plugin-lsp"); !ok {
		t.Fatal("expected plugin LSP server to be registered")
	}
	if _, ok := outputStyleRegistry.Get("plugin-style"); !ok {
		t.Fatal("expected plugin output style to be registered")
	}
	if _, ok := settingsRegistry.Get("plugin-settings"); !ok {
		t.Fatal("expected plugin settings entry to be registered")
	}

	if err := registrar.Unregister("plugin-a"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	if _, ok := commandRegistry.GetPrompt("direct-command"); ok {
		t.Fatal("expected direct command to be removed on unregister")
	}
	if _, ok := skillRegistry.Get("plugin-skill"); ok {
		t.Fatal("expected plugin skill to be removed on unregister")
	}
	if _, ok := lspRegistry.GetServer("plugin-lsp"); ok {
		t.Fatal("expected plugin LSP server to be removed on unregister")
	}
	if _, ok := outputStyleRegistry.Get("plugin-style"); ok {
		t.Fatal("expected plugin output style to be removed on unregister")
	}
	if _, ok := settingsRegistry.Get("plugin-settings"); ok {
		t.Fatal("expected plugin settings to be removed on unregister")
	}
}

func TestRegistrarRollsBackOnLaterFailure(t *testing.T) {
	commandRegistry := commands.NewRegistry()
	skillRegistry := skills.NewRegistry()
	agentRegistry := agents.NewRegistry()
	lspRegistry := lsp.NewRegistry()
	outputStyleRegistry := outputstyle.NewRegistry()

	registrar := &Registrar{
		Commands:     commandRegistry,
		Skills:       skillRegistry,
		Agents:       agentRegistry,
		LSP:          lspRegistry,
		OutputStyles: outputStyleRegistry,
	}

	err := registrar.Register(Spec{
		Name: "plugin-b",
		Skills: []skills.Definition{
			{Name: "plugin-skill", Description: "skill desc", Prompt: "skill prompt"},
		},
		Agents: []agents.Definition{
			{
				Kind:   "worker",
				Name:   "plugin-worker",
				Source: string(agents.SourcePlugin),
			},
		},
		LSPServers: []lsp.ServerConfig{
			{
				ID:        "plugin-lsp",
				Name:      "plugin-lsp",
				Command:   []string{"plugin-lsp"},
				Languages: []string{"go"},
			},
		},
		OutputStyles: []outputstyle.Definition{
			{
				Name:   "plugin-style",
				Prompt: "Use plugin formatting",
				Source: "plugin",
			},
		},
		Settings: []settings.Entry{
			{Name: "plugin-settings", Values: map[string]any{"enabled": true}},
		},
	})
	if err == nil {
		t.Fatal("expected Register() to fail without settings registry")
	}

	if _, ok := skillRegistry.Get("plugin-skill"); ok {
		t.Fatal("expected skill registration to be rolled back")
	}
	if _, ok := commandRegistry.GetPrompt("plugin-skill"); ok {
		t.Fatal("expected derived skill command registration to be rolled back")
	}
	if _, ok := agentRegistry.Get("plugin-worker"); ok {
		t.Fatal("expected plugin agent registration to be rolled back")
	}
	if _, ok := lspRegistry.GetServer("plugin-lsp"); ok {
		t.Fatal("expected plugin LSP registration to be rolled back")
	}
	if _, ok := outputStyleRegistry.Get("plugin-style"); ok {
		t.Fatal("expected plugin output style registration to be rolled back")
	}
}

func TestRegistrarAllowsSameCommandAndSkillNamesAcrossSources(t *testing.T) {
	commandRegistry := commands.NewRegistry()
	skillRegistry := skills.NewRegistry()

	projectSkill := skills.Definition{
		Name:        "deploy",
		Description: "project skill",
		Prompt:      "Use the project deploy skill.",
		Source:      commands.SourceProjectSettings,
		LoadedFrom:  "/repo/.codex/skills/deploy/SKILL.md",
		FileID:      "project:deploy",
	}
	skillRegistry.Register(projectSkill)
	if err := commandRegistry.RegisterPrompt(skills.PromptCommandForDefinition(projectSkill, commands.SourceProjectSettings)); err != nil {
		t.Fatalf("RegisterPrompt(project skill) error = %v", err)
	}

	projectCommand := commands.PromptCommand{
		Name:        "sync",
		Description: "project command",
		Prompt:      "Use the project sync command.",
		Source:      commands.SourceProjectSettings,
		LoadedFrom:  "/repo/.codex/commands/sync.json",
	}
	if err := commandRegistry.RegisterPrompt(projectCommand); err != nil {
		t.Fatalf("RegisterPrompt(project command) error = %v", err)
	}

	registrar := &Registrar{
		Commands: commandRegistry,
		Skills:   skillRegistry,
	}

	err := registrar.Register(Spec{
		Name: "plugin-a",
		Commands: []commands.PromptCommand{
			{
				Name:        "sync",
				Description: "plugin command",
				Prompt:      "Use the plugin sync command.",
				Source:      commands.SourcePlugin,
				LoadedFrom:  "/plugin/commands/sync.json",
			},
		},
		Skills: []skills.Definition{
			{
				Name:        "deploy",
				Description: "plugin skill",
				Prompt:      "Use the plugin deploy skill.",
				Source:      commands.SourcePlugin,
				LoadedFrom:  "/plugin/skills/deploy/SKILL.md",
				FileID:      "plugin:deploy",
			},
		},
	})
	if err != nil {
		t.Fatalf("Register(plugin-a) error = %v", err)
	}

	gotSkill, ok := skillRegistry.Get("deploy")
	if !ok {
		t.Fatal("expected deploy skill to remain addressable")
	}
	if gotSkill.Description != "plugin skill" {
		t.Fatalf("expected plugin skill to win active view, got %q", gotSkill.Description)
	}
	gotCommand, ok := commandRegistry.GetPrompt("sync")
	if !ok {
		t.Fatal("expected sync command to remain addressable")
	}
	if gotCommand.Description != "plugin command" {
		t.Fatalf("expected plugin command to win active view, got %q", gotCommand.Description)
	}

	if err := registrar.Unregister("plugin-a"); err != nil {
		t.Fatalf("Unregister(plugin-a) error = %v", err)
	}

	gotSkill, ok = skillRegistry.Get("deploy")
	if !ok {
		t.Fatal("expected project deploy skill to become active again")
	}
	if gotSkill.Description != "project skill" {
		t.Fatalf("expected project skill fallback after unregister, got %q", gotSkill.Description)
	}
	gotCommand, ok = commandRegistry.GetPrompt("sync")
	if !ok {
		t.Fatal("expected project sync command to become active again")
	}
	if gotCommand.Description != "project command" {
		t.Fatalf("expected project command fallback after unregister, got %q", gotCommand.Description)
	}
}

func TestRegistrarRejectsStrictPluginOnlyPolicyWhenCustomSourcesAlreadyExist(t *testing.T) {
	governance := settings.NewGovernance()
	commandRegistry := commands.NewRegistry()
	commandRegistry.SetGovernance(governance)

	if err := commandRegistry.RegisterPrompt(commands.PromptCommand{
		Name:       "custom-skill",
		Prompt:     "custom prompt",
		Source:     commands.SourceUserSettings,
		LoadedFrom: commands.LoadedFromSkills,
	}); err != nil {
		t.Fatalf("RegisterPrompt(custom-skill) error = %v", err)
	}

	registrar := &Registrar{
		Commands:   commandRegistry,
		Governance: governance,
	}

	err := registrar.Register(Spec{
		Name: "plugin-strict",
		Manifest: Manifest{
			StrictPluginOnlyCustomization: true,
		},
	})
	if err == nil {
		t.Fatal("expected strict plugin-only manifest to conflict with existing custom sources")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
	if governance.Snapshot().IsRestrictedToPluginOnly(settings.SurfaceSkills) {
		t.Fatal("failed registration should not leave strict plugin-only policy behind")
	}
}
