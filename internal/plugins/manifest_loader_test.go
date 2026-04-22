package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/commands"
)

func TestManifestLoaderLoadsPluginSpecFromFilesystem(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "example-plugin")

	writeTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "example-plugin",
  "commandsPath": "commands",
  "commandsPaths": ["extra-commands"],
  "agentsPath": "agents",
  "skillsPath": "skills",
  "outputStylesPath": "output-styles",
  "hooksConfig": "hooks/hooks.json",
  "mcpServers": [
    {"id":"plugin-mcp","name":"plugin-mcp","transport":"stdio","command":["plugin-mcp"]}
  ],
  "lspServers": [
    {"id":"plugin-lsp","name":"plugin-lsp","command":["plugin-lsp"],"languages":["go"],"roots":["."]}
  ],
  "settings": [
    {"name":"plugin-settings","values":{"enabled":true}}
  ],
  "strictPluginOnlyCustomization": true,
  "allowedMcpServers": ["plugin-mcp"],
  "additionalDirectories": ["/tmp/plugin-dir"]
}`)
	writeTestFile(t, filepath.Join(pluginDir, "commands", "deploy.json"), `{
  "name":"deploy",
  "description":"deploy command",
  "prompt":"Deploy carefully.",
  "source":"plugin"
}`)
	writeTestFile(t, filepath.Join(pluginDir, "extra-commands", "rollback.json"), `[
  {"name":"rollback","description":"rollback command","prompt":"Rollback carefully.","source":"plugin"}
]`)
	writeTestFile(t, filepath.Join(pluginDir, "agents", "worker.json"), `{
  "kind":"worker",
  "name":"plugin-worker",
  "source":"plugin",
  "description":"worker agent"
}`)
	writeTestFile(t, filepath.Join(pluginDir, "skills", "filesystem", "SKILL.md"), `---
name: filesystem
description: Filesystem helper
---

Use filesystem skill.`)
	writeTestFile(t, filepath.Join(pluginDir, "output-styles", "concise.json"), `{
  "name":"concise",
  "description":"Concise output",
  "prompt":"Be concise.",
  "source":"plugin"
}`)
	writeTestFile(t, filepath.Join(pluginDir, "hooks", "hooks.json"), `{"toolHooks":[],"turnHooks":[]}`)

	loader := NewManifestLoader(pluginDir)
	specs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("Load() len = %d, want 1", len(specs))
	}

	spec := specs[0]
	if spec.Name != "example-plugin" {
		t.Fatalf("spec.Name = %q", spec.Name)
	}
	if len(spec.Commands) != 2 {
		t.Fatalf("spec.Commands len = %d, want 2", len(spec.Commands))
	}
	if spec.Commands[0].Name != "deploy" || spec.Commands[1].Name != "rollback" {
		t.Fatalf("unexpected command names: %#v", []string{spec.Commands[0].Name, spec.Commands[1].Name})
	}
	for _, cmd := range spec.Commands {
		if cmd.Source != commands.SourcePlugin {
			t.Fatalf("command source = %q, want %q", cmd.Source, commands.SourcePlugin)
		}
	}
	if len(spec.Agents) != 1 || spec.Agents[0].Name != "plugin-worker" {
		t.Fatalf("unexpected agents: %#v", spec.Agents)
	}
	if len(spec.Skills) != 1 || spec.Skills[0].Name != "filesystem" {
		t.Fatalf("unexpected skills: %#v", spec.Skills)
	}
	if len(spec.OutputStyles) != 1 || spec.OutputStyles[0].Name != "concise" {
		t.Fatalf("unexpected output styles: %#v", spec.OutputStyles)
	}
	if len(spec.MCPServers) != 1 || spec.MCPServers[0].Config.ID != "plugin-mcp" {
		t.Fatalf("unexpected MCP servers: %#v", spec.MCPServers)
	}
	if len(spec.LSPServers) != 1 || spec.LSPServers[0].ID != "plugin-lsp" {
		t.Fatalf("unexpected LSP servers: %#v", spec.LSPServers)
	}
	if len(spec.Settings) != 1 || spec.Settings[0].Name != "plugin-settings" {
		t.Fatalf("unexpected settings: %#v", spec.Settings)
	}
	if spec.Manifest.Name != "example-plugin" {
		t.Fatalf("manifest name = %q", spec.Manifest.Name)
	}
	if !spec.Manifest.StrictPluginOnlyCustomization {
		t.Fatal("expected strictPluginOnlyCustomization to be preserved")
	}
	if len(spec.Manifest.AllowedMCPServers) != 1 || spec.Manifest.AllowedMCPServers[0] != "plugin-mcp" {
		t.Fatalf("unexpected allowed MCP servers: %#v", spec.Manifest.AllowedMCPServers)
	}
	if len(spec.Manifest.AdditionalDirectories) != 1 || spec.Manifest.AdditionalDirectories[0] != "/tmp/plugin-dir" {
		t.Fatalf("unexpected additional directories: %#v", spec.Manifest.AdditionalDirectories)
	}
	if spec.Manifest.HooksConfig == "" || !strings.HasSuffix(spec.Manifest.HooksConfig, filepath.Join("hooks", "hooks.json")) {
		t.Fatalf("unexpected hooksConfig path: %q", spec.Manifest.HooksConfig)
	}
	if len(spec.Manifest.CommandsPaths) != 2 {
		t.Fatalf("unexpected commands paths: %#v", spec.Manifest.CommandsPaths)
	}
}

func TestManifestLoaderIncludesManifestPathInErrors(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "broken-plugin")

	manifestPath := filepath.Join(pluginDir, ".codex-plugin", "plugin.json")
	writeTestFile(t, manifestPath, `{
  "name": "broken-plugin",
  "commandsPath": "commands"
}`)
	writeTestFile(t, filepath.Join(pluginDir, "commands", "broken.json"), `{`)

	loader := NewManifestLoader(pluginDir)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected Load() to fail")
	}
	if !strings.Contains(err.Error(), manifestPath) {
		t.Fatalf("expected manifest path in error, got %v", err)
	}
}

func TestManifestLoaderDeduplicatesCommandsAndSkillsByFileIdentity(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "example-plugin")

	writeTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "example-plugin",
  "commandsPath": "commands",
  "commandsPaths": ["commands/deploy.json"],
  "skillsPath": "skills",
  "skillsPaths": ["skills/filesystem/SKILL.md"]
}`)
	writeTestFile(t, filepath.Join(pluginDir, "commands", "deploy.json"), `{
  "name":"deploy",
  "description":"deploy command",
  "prompt":"Deploy carefully.",
  "source":"plugin"
}`)
	writeTestFile(t, filepath.Join(pluginDir, "skills", "filesystem", "SKILL.md"), `---
name: filesystem
description: Filesystem helper
---

Use filesystem skill.`)

	loader := NewManifestLoader(pluginDir)
	specs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("Load() len = %d, want 1", len(specs))
	}

	spec := specs[0]
	if len(spec.Commands) != 1 {
		t.Fatalf("expected command file to be loaded once, got %d commands", len(spec.Commands))
	}
	if len(spec.Skills) != 1 {
		t.Fatalf("expected skill file to be loaded once, got %d skills", len(spec.Skills))
	}
	if spec.Skills[0].FileID == "" {
		t.Fatal("expected file-backed skills to carry file identity metadata")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
