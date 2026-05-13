package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentmgr"
	"aiops-v2/internal/agents"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/observability"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/settings"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
)

type registryAdapterMockTool struct {
	name     string
	meta     tooling.ToolMetadata
	sessions []string
	modes    []string
}

func TestBuildRuntimeObserverDisabledReturnsNoop(t *testing.T) {
	observer, provider := buildRuntimeObserver(context.Background(), func(string) string { return "" })
	defer provider.Shutdown(context.Background())
	if !isNoopRuntimeObserver(observer) {
		t.Fatalf("observer type = %T, want runtimekernel.NoopObserver", observer)
	}
	if provider.Enabled() {
		t.Fatal("provider should be disabled")
	}
}

func TestBuildRuntimeObserverEnabledReturnsOTelObserver(t *testing.T) {
	env := map[string]string{
		"AIOPS_OTEL_ENABLED":      "1",
		"AIOPS_OTEL_ENDPOINT":     "http://127.0.0.1:9/v1/traces",
		"AIOPS_OTEL_SERVICE_NAME": "aiops-v2-agent-test",
	}
	observer, provider := buildRuntimeObserver(context.Background(), func(key string) string { return env[key] })
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	defer provider.Shutdown(shutdownCtx)
	if _, ok := observer.(observability.RuntimeObserver); !ok {
		t.Fatalf("observer type = %T, want observability.RuntimeObserver", observer)
	}
	if !provider.Enabled() {
		t.Fatal("provider should be enabled")
	}
}

func TestRunnerStudioUpstreamFromEnv(t *testing.T) {
	t.Run("prefers runner studio specific env", func(t *testing.T) {
		env := map[string]string{
			"AIOPS_RUNNER_STUDIO_UPSTREAM_URL": " http://runner-studio.internal ",
			"RUNNER_STUDIO_UPSTREAM_URL":       "http://runner-fallback.internal",
			"AIOPS_RUNNER_API_BASE_URL":        "http://runner-api.internal",
		}
		got := runnerStudioUpstreamFromEnv(func(key string) string { return env[key] })
		if got != "http://runner-studio.internal" {
			t.Fatalf("upstream = %q, want runner studio specific env", got)
		}
	})

	t.Run("falls back to runner api base url", func(t *testing.T) {
		env := map[string]string{
			"AIOPS_RUNNER_API_BASE_URL": "http://runner-api.internal",
		}
		got := runnerStudioUpstreamFromEnv(func(key string) string { return env[key] })
		if got != "http://runner-api.internal" {
			t.Fatalf("upstream = %q, want runner API base URL", got)
		}
	})

	t.Run("returns empty when unset", func(t *testing.T) {
		got := runnerStudioUpstreamFromEnv(func(string) string { return "" })
		if got != "" {
			t.Fatalf("upstream = %q, want empty", got)
		}
	})
}

func TestOpenConfiguredStoreDefaultsToJSONFileStore(t *testing.T) {
	dataDir := t.TempDir()
	got, err := openConfiguredStore(dataDir, func(string) string { return "" })
	if err != nil {
		t.Fatalf("openConfiguredStore() error = %v", err)
	}
	defer got.Close()
	if _, ok := got.(*store.JSONFileStore); !ok {
		t.Fatalf("openConfiguredStore() type = %T, want *store.JSONFileStore", got)
	}
}

func TestOpenConfiguredStoreRequiresMySQLDSN(t *testing.T) {
	_, err := openConfiguredStore(t.TempDir(), func(key string) string {
		if key == "AIOPS_STORE_DRIVER" {
			return "mysql"
		}
		return ""
	})
	if err == nil {
		t.Fatal("openConfiguredStore() succeeded without mysql dsn")
	}
	if !strings.Contains(err.Error(), "AIOPS_MYSQL_DSN") {
		t.Fatalf("error = %q, want AIOPS_MYSQL_DSN", err.Error())
	}
}

func TestOpenConfiguredStoreRejectsUnknownDriver(t *testing.T) {
	_, err := openConfiguredStore(t.TempDir(), func(key string) string {
		if key == "AIOPS_STORE_DRIVER" {
			return "postgres"
		}
		return ""
	})
	if err == nil {
		t.Fatal("openConfiguredStore() succeeded with unknown driver")
	}
	if !strings.Contains(err.Error(), "unsupported store driver") {
		t.Fatalf("error = %q, want unsupported store driver", err.Error())
	}
}

func TestCorootEndpointFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "prefers explicit endpoint",
			env: map[string]string{
				"AIOPS_COROOT_ENDPOINT": " http://coroot-endpoint.internal ",
				"AIOPS_COROOT_BASE_URL": "http://coroot-base.internal",
				"COROOT_BASE_URL":       "http://coroot-fallback.internal",
			},
			want: "http://coroot-endpoint.internal",
		},
		{
			name: "falls back to aiops base url",
			env: map[string]string{
				"AIOPS_COROOT_BASE_URL": " http://127.0.0.1:18180 ",
				"COROOT_BASE_URL":       "http://coroot-fallback.internal",
			},
			want: "http://127.0.0.1:18180",
		},
		{
			name: "falls back to coroot base url",
			env: map[string]string{
				"COROOT_BASE_URL": " http://coroot.local ",
			},
			want: "http://coroot.local",
		},
		{
			name: "returns empty when unset",
			env:  map[string]string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := corootEndpointFromEnv(func(key string) string { return tt.env[key] })
			if got != tt.want {
				t.Fatalf("corootEndpointFromEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func isNoopRuntimeObserver(observer runtimekernel.Observer) bool {
	_, ok := observer.(runtimekernel.NoopObserver)
	return ok
}

func (m *registryAdapterMockTool) Metadata() tooling.ToolMetadata {
	meta := m.meta
	if meta.Name == "" {
		meta.Name = m.name
	}
	if meta.Description == "" {
		meta.Description = m.name
	}
	return meta
}

func (m *registryAdapterMockTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (m *registryAdapterMockTool) OutputSchema() json.RawMessage { return nil }
func (m *registryAdapterMockTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return m.Metadata().Description
}
func (m *registryAdapterMockTool) Prompt(tooling.PromptContext) string {
	return m.Metadata().Description
}
func (m *registryAdapterMockTool) IsEnabled(ctx tooling.ToolContext) bool {
	return matchRegistryAdapterValue(m.sessions, ctx.SessionType) && matchRegistryAdapterValue(m.modes, ctx.Mode)
}
func (m *registryAdapterMockTool) IsReadOnly(json.RawMessage) bool        { return true }
func (m *registryAdapterMockTool) IsDestructive(json.RawMessage) bool     { return false }
func (m *registryAdapterMockTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (m *registryAdapterMockTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (m *registryAdapterMockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (m *registryAdapterMockTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func matchRegistryAdapterValue(expected []string, actual string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		if candidate == actual {
			return true
		}
	}
	return false
}

func registerRegistryAdapterMockTool(t *testing.T, registry *tooling.Registry, tool *registryAdapterMockTool) {
	t.Helper()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register %q: %v", tool.Metadata().Name, err)
	}
}

func TestRegistryAdapterSkillPromptAssetsPreferSkillRegistryOverCommandSurface(t *testing.T) {
	registry := tooling.NewRegistry()

	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "filesystem",
		Prompt: "filesystem prompt asset",
	})

	commandRegistry := commands.NewRegistry()
	if err := commandRegistry.RegisterPrompt(commands.PromptCommand{
		Name:       "filesystem",
		Prompt:     "command-surface filesystem prompt asset",
		Source:     "repo",
		LoadedFrom: "skills/filesystem/SKILL.md",
	}); err != nil {
		t.Fatalf("register prompt command: %v", err)
	}

	adapter := newRegistryAdapter(registry, commandRegistry, featureflag.Default())
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 1 {
		t.Fatalf("SkillPromptAssets len = %d, want 1", len(ctx.SkillPromptAssets))
	}
	if ctx.SkillPromptAssets[0] != "command-surface filesystem prompt asset" {
		t.Fatalf("SkillPromptAssets[0] = %q, want command-surface filesystem prompt asset", ctx.SkillPromptAssets[0])
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Prompt != "command-surface filesystem prompt asset" {
		t.Fatalf("skillPromptCommands[0].Prompt = %q, want command-surface filesystem prompt asset", cmds[0].Prompt)
	}
	if !strings.HasSuffix(filepath.ToSlash(cmds[0].LoadedFrom), "skills/filesystem/SKILL.md") {
		t.Fatalf("skillPromptCommands[0].LoadedFrom = %q, want suffix %q", cmds[0].LoadedFrom, "skills/filesystem/SKILL.md")
	}
}

func TestRegistryAdapterSkillPromptAssetsPreferSkillRegistry(t *testing.T) {
	adapter := newRegistryAdapter(tooling.NewRegistry(), nil, featureflag.Default())
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 0 {
		t.Fatalf("SkillPromptAssets len = %d, want 0", len(ctx.SkillPromptAssets))
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 0 {
		t.Fatalf("skillPromptCommands len = %d, want 0", len(cmds))
	}
}

func TestRegistryAdapterSkillPromptAssetsDoNotFallbackWithoutCommandSurface(t *testing.T) {
	adapter := newRegistryAdapter(tooling.NewRegistry(), nil, featureflag.Default())
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 0 {
		t.Fatalf("SkillPromptAssets len = %d, want 0", len(ctx.SkillPromptAssets))
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 0 {
		t.Fatalf("skillPromptCommands len = %d, want 0", len(cmds))
	}
}

func TestRegistryAdapterSkillPromptCommandsUseOnlyCommandSurface(t *testing.T) {
	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "filesystem",
		Prompt: "filesystem prompt asset",
	})

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry, featureflag.Default())
	cmds := adapter.skillPromptCommands()

	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Name != "filesystem" {
		t.Fatalf("skillPromptCommands[0].Name = %q, want filesystem", cmds[0].Name)
	}
}

func TestRegistryAdapterSkillPromptCommandsProjectSkillRegistrySources(t *testing.T) {
	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "plugin-skill",
		Prompt: "plugin prompt asset",
		Source: "/Users/me/.codex/plugins/cache/example/plugin-skill/SKILL.md",
	})
	skillRegistry.Register(skills.Definition{
		Name:   "bundled-skill",
		Prompt: "bundled prompt asset",
		Source: "/Users/me/.codex/skills/.system/bundled-skill/SKILL.md",
	})
	skillRegistry.Register(skills.Definition{
		Name:   "project-skill",
		Prompt: "project prompt asset",
		Source: "/repo/skills/project-skill/SKILL.md",
	})

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry, featureflag.Default())
	cmds := adapter.skillPromptCommands()

	if len(cmds) != 3 {
		t.Fatalf("skillPromptCommands len = %d, want 3", len(cmds))
	}
	if cmds[0].Source != commands.SourcePlugin {
		t.Fatalf("cmds[0].Source = %q, want %q", cmds[0].Source, commands.SourcePlugin)
	}
	if cmds[1].Source != commands.SourceBundled {
		t.Fatalf("cmds[1].Source = %q, want %q", cmds[1].Source, commands.SourceBundled)
	}
	if cmds[2].Source != commands.SourceProjectSettings {
		t.Fatalf("cmds[2].Source = %q, want %q", cmds[2].Source, commands.SourceProjectSettings)
	}
}

func TestLoadSkillRegistryFromEnvInjectsPromptAssets(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "filesystem")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: filesystem
description: Filesystem helper
---
Use filesystem skill prompt asset.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("AIOPS_SKILLS_DIRS", root)
	skillRegistry, err := loadSkillRegistryFromEnv()
	if err != nil {
		t.Fatalf("loadSkillRegistryFromEnv() error = %v", err)
	}

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry, featureflag.Default())
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 1 {
		t.Fatalf("SkillPromptAssets len = %d, want 1", len(ctx.SkillPromptAssets))
	}
	if ctx.SkillPromptAssets[0] != "Use filesystem skill prompt asset." {
		t.Fatalf("SkillPromptAssets[0] = %q, want %q", ctx.SkillPromptAssets[0], "Use filesystem skill prompt asset.")
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Name != "filesystem" {
		t.Fatalf("skillPromptCommands[0].Name = %q, want filesystem", cmds[0].Name)
	}
}

func TestRegisterPluginsFromEnvRegistersManifestComponents(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "example-plugin")

	writeMainTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "example-plugin",
  "commandsPath": "commands",
  "agentsPath": "agents",
  "skillsPath": "skills",
  "outputStylesPath": "output-styles",
  "mcpServers": [
    {"id":"plugin-mcp","name":"plugin-mcp","transport":"stdio","command":["plugin-mcp"]}
  ],
  "lspServers": [
    {"id":"plugin-lsp","name":"plugin-lsp","command":["plugin-lsp"],"languages":["go"],"roots":["."]}
  ],
  "settings": [
    {"name":"plugin-settings","values":{"enabled":true}}
  ]
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "commands", "deploy.json"), `{
  "name":"deploy",
  "description":"deploy command",
  "prompt":"Deploy carefully.",
  "source":"plugin"
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "agents", "worker.json"), `{
  "kind":"worker",
  "name":"plugin-worker",
  "source":"plugin",
  "description":"worker agent"
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "skills", "filesystem", "SKILL.md"), `---
name: filesystem
description: Filesystem helper
---

Use filesystem skill.`)
	writeMainTestFile(t, filepath.Join(pluginDir, "output-styles", "concise.json"), `{
  "name":"concise",
  "description":"Concise output",
  "prompt":"Be concise.",
  "source":"plugin"
}`)

	t.Setenv("AIOPS_PLUGIN_DIRS", pluginDir)

	commandRegistry := commands.NewRegistry()
	skillRegistry := skills.NewRegistry()
	agentRegistry := agents.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	lspRegistry := lsp.NewRegistry()
	outputStyleRegistry := outputstyle.NewRegistry()
	settingsRegistry := settings.NewRegistry()

	registrar := &plugins.Registrar{
		Commands:     commandRegistry,
		Skills:       skillRegistry,
		Agents:       agentRegistry,
		MCP:          mcpRegistry,
		LSP:          lspRegistry,
		OutputStyles: outputStyleRegistry,
		Settings:     settingsRegistry,
	}

	if err := registerPluginsFromEnv(registrar); err != nil {
		t.Fatalf("registerPluginsFromEnv() error = %v", err)
	}

	if _, ok := commandRegistry.GetPrompt("deploy"); !ok {
		t.Fatal("expected plugin command to be registered")
	}
	if _, ok := skillRegistry.Get("filesystem"); !ok {
		t.Fatal("expected plugin skill to be registered")
	}
	skillCmds := commandRegistry.ListSkillLikePromptCommands()
	if len(skillCmds) < 1 {
		t.Fatal("expected at least one skill-like command")
	}
	var sawFilesystem bool
	for _, cmd := range skillCmds {
		if cmd.Name == "filesystem" {
			sawFilesystem = true
			break
		}
	}
	if !sawFilesystem {
		t.Fatalf("skill-like commands = %#v, want filesystem to be present", skillCmds)
	}
	if _, ok := agentRegistry.Get("plugin-worker"); !ok {
		t.Fatal("expected plugin agent to be registered")
	}
	if _, ok := mcpRegistry.GetServer("plugin-mcp"); !ok {
		t.Fatal("expected plugin MCP server to be registered")
	}
	if _, ok := lspRegistry.GetServer("plugin-lsp"); !ok {
		t.Fatal("expected plugin LSP server to be registered")
	}
	if _, ok := outputStyleRegistry.Get("concise"); !ok {
		t.Fatal("expected plugin output style to be registered")
	}
	if _, ok := settingsRegistry.Get("plugin-settings"); !ok {
		t.Fatal("expected plugin settings to be registered")
	}
}

func TestRegistryAdapterUsesSameFlaggedAssemblyForPromptAndRuntimePools(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "write_file", sessions: []string{"host"}, modes: []string{"chat"}})

	flags := featureflag.Flags{
		DisabledTools: []string{"write_file"},
		DeferredTools: []string{"read_file"},
	}
	adapter := newRegistryAdapter(registry, nil, flags)

	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	if len(ctx.AssembledTools) != 1 {
		t.Fatalf("CompileContext AssembledTools len = %d, want 1", len(ctx.AssembledTools))
	}
	if ctx.AssembledTools[0].Metadata().Name != "read_file" {
		t.Fatalf("CompileContext AssembledTools[0].Name = %q, want read_file", ctx.AssembledTools[0].Metadata().Name)
	}
	if !ctx.AssembledTools[0].Metadata().ShouldDefer {
		t.Fatalf("expected deferred metadata in CompileContext, got %#v", ctx.AssembledTools[0].Metadata())
	}

	pool := adapter.AssembleToolPool(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPool() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "read_file" {
		t.Fatalf("tool pool Info().Name = %q, want read_file", info.Name)
	}
}

func TestRegistryAdapterToolPromptSetMatchesRuntimeToolPool(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "write_file", sessions: []string{"host"}, modes: []string{"chat"}})

	flags := featureflag.Flags{
		DisabledTools: []string{"write_file"},
	}
	adapter := newRegistryAdapter(registry, nil, flags)
	compiler := promptcompiler.NewCompiler()

	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	compiled, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if len(compiled.Tools.Entries) != 1 {
		t.Fatalf("compiled tool entries len = %d, want 1", len(compiled.Tools.Entries))
	}
	if !strings.Contains(compiled.Tools.Content, "read_file") {
		t.Fatalf("tool prompt content = %q, want read_file entry", compiled.Tools.Content)
	}
	if strings.Contains(compiled.Tools.Content, "write_file") {
		t.Fatalf("tool prompt content should not include filtered tool: %q", compiled.Tools.Content)
	}

	pool := adapter.AssembleToolPool(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPool() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if !strings.Contains(compiled.Tools.Content, info.Name) {
		t.Fatalf("tool prompt content %q should include runtime tool %q", compiled.Tools.Content, info.Name)
	}
}

func TestRegistryAdapterCompileContextDoesNotLeakLegacyMCPPromptAssetsForFilteredTools(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{
		name:     "coroot.query",
		sessions: []string{"host"},
		modes:    []string{"inspect"},
		meta: tooling.ToolMetadata{
			IsMCP: true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "coroot",
				ServerName: "coroot",
				ToolName:   "coroot.query",
			},
		},
	})

	adapter := newRegistryAdapter(registry, nil, featureflag.Flags{
		DisabledTools: []string{"coroot.query"},
	})
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("inspect"))

	if len(ctx.AssembledTools) != 0 {
		t.Fatalf("CompileContext AssembledTools len = %d, want 0", len(ctx.AssembledTools))
	}
	if len(ctx.MCPPromptAssets) != 0 {
		t.Fatalf("MCPPromptAssets len = %d, want 0", len(ctx.MCPPromptAssets))
	}
}

func TestRegisterBuiltinAgentDefinitionsWorkerUsesToolScopeForMCPTraits(t *testing.T) {
	agentRegistry := agents.NewRegistry()
	agentFactory := agentmgr.NewAgentFactory(tooling.NewRegistry(), nil, nil, nil)

	if err := registerBuiltinAgentDefinitions(agentRegistry, agentFactory); err != nil {
		t.Fatalf("registerBuiltinAgentDefinitions() error = %v", err)
	}

	worker, ok := agentRegistry.Get("worker")
	if !ok {
		t.Fatal("expected builtin worker definition to be registered")
	}

	if len(worker.Tools) != 0 {
		t.Fatalf("worker.Tools = %#v, want empty allowlist", worker.Tools)
	}
}

func TestRegistryAdapterDefaultFlagsMatchUnflaggedRegistryAssembly(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "exec_command", sessions: []string{"host"}, modes: []string{"chat"}})

	adapter := newRegistryAdapter(registry, nil, featureflag.Default())
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	wantTools := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{})

	if len(ctx.AssembledTools) != len(wantTools) {
		t.Fatalf("CompileContext AssembledTools len = %d, want %d", len(ctx.AssembledTools), len(wantTools))
	}
	for i := range wantTools {
		gotMeta := ctx.AssembledTools[i].Metadata()
		wantMeta := wantTools[i].Metadata()
		if gotMeta.Name != wantMeta.Name {
			t.Fatalf("CompileContext tool[%d].Name = %q, want %q", i, gotMeta.Name, wantMeta.Name)
		}
		if gotMeta.ShouldDefer != wantMeta.ShouldDefer || gotMeta.HasMCPSource() != wantMeta.HasMCPSource() {
			t.Fatalf("CompileContext tool[%d] metadata = %#v, want %#v", i, gotMeta, wantMeta)
		}
	}

	pool := adapter.AssembleToolPool(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	wantPool := tooling.AssembleEinoToolPool(wantTools)
	if len(pool) != len(wantPool) {
		t.Fatalf("AssembleToolPool() len = %d, want %d", len(pool), len(wantPool))
	}
	for i := range wantPool {
		gotInfo, err := pool[i].Info(context.Background())
		if err != nil {
			t.Fatalf("pool[%d].Info() error = %v", i, err)
		}
		wantInfo, err := wantPool[i].Info(context.Background())
		if err != nil {
			t.Fatalf("wantPool[%d].Info() error = %v", i, err)
		}
		if gotInfo.Name != wantInfo.Name {
			t.Fatalf("pool[%d].Info().Name = %q, want %q", i, gotInfo.Name, wantInfo.Name)
		}
	}
}

func writeMainTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(fmt.Sprintf("%s", content), "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
