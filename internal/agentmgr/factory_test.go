package agentmgr

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agents"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Mock implementations for testing
// ---------------------------------------------------------------------------

// mockCompiler implements promptcompiler.Compiler for testing.
type mockCompiler struct {
	compileFunc         func(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error)
	compileForEino      func(ctx promptcompiler.CompileContext) ([]*schema.Message, error)
	compileCalls        []promptcompiler.CompileContext
	compileForEinoCalls []promptcompiler.CompileContext
	lastCompileCtx      promptcompiler.CompileContext
	lastCompileForEino  promptcompiler.CompileContext
}

func (m *mockCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	m.lastCompileCtx = ctx
	m.compileCalls = append(m.compileCalls, ctx)
	if m.compileFunc != nil {
		return m.compileFunc(ctx)
	}
	return promptcompiler.CompiledPrompt{
		System:    promptcompiler.SystemPrompt{Content: "system"},
		Developer: promptcompiler.DeveloperInstructions{Content: "developer"},
		Tools:     promptcompiler.ToolPromptSet{Content: "tools"},
		Policy:    promptcompiler.RuntimePolicyPrompt{Content: "policy", Mode: ctx.Mode},
	}, nil
}

func (m *mockCompiler) CompileForEino(ctx promptcompiler.CompileContext) ([]*schema.Message, error) {
	m.lastCompileForEino = ctx
	m.compileForEinoCalls = append(m.compileForEinoCalls, ctx)
	if m.compileForEino != nil {
		return m.compileForEino(ctx)
	}
	return []*schema.Message{
		schema.SystemMessage("system"),
		schema.SystemMessage("developer"),
		schema.SystemMessage("tools"),
		schema.SystemMessage("policy:" + ctx.Mode),
	}, nil
}

// mockChatModel implements model.ChatModel for testing.
type mockChatModel struct {
	model.ChatModel
}

type mockTool struct {
	name        string
	description string
	meta        tooling.ToolMetadata
	readOnly    bool
	sessions    []string
	modes       []string
}

func (m *mockTool) Metadata() tooling.ToolMetadata {
	meta := m.meta
	if meta.Name == "" {
		meta.Name = m.name
	}
	if meta.Description == "" {
		meta.Description = m.description
		if meta.Description == "" {
			meta.Description = "mock tool: " + meta.Name
		}
	}
	return meta
}
func (m *mockTool) InputSchema() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) OutputSchema() json.RawMessage { return nil }
func (m *mockTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return m.Metadata().Description
}
func (m *mockTool) Prompt(tooling.PromptContext) string { return m.Metadata().Description }
func (m *mockTool) IsEnabled(ctx tooling.ToolContext) bool {
	return matchFactoryToolValue(m.sessions, ctx.SessionType) && matchFactoryToolValue(m.modes, ctx.Mode)
}
func (m *mockTool) IsReadOnly(json.RawMessage) bool { return m.readOnly }
func (m *mockTool) IsDestructive(json.RawMessage) bool {
	return !m.readOnly
}
func (m *mockTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (m *mockTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (m *mockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (m *mockTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func matchFactoryToolValue(expected []string, actual string) bool {
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

// ---------------------------------------------------------------------------
// Helper to create a test factory with common setup.
// ---------------------------------------------------------------------------

func newTestFactory(tb testing.TB) (*AgentFactory, *tooling.Registry) {
	tb.Helper()

	registry := tooling.NewRegistry()
	compiler := &mockCompiler{}
	mockModel := &mockChatModel{}
	providers := map[string]modelrouter.ChatModel{
		"openai": mockModel,
	}
	router := modelrouter.NewRouter("openai", providers, nil)
	policy := &policyengine.Engine{}

	factory := NewAgentFactory(registry, compiler, router, policy)
	return factory, registry
}

func registerFactoryTool(t *testing.T, registry *tooling.Registry, tool tooling.Tool) {
	t.Helper()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool %s: %v", tool.Metadata().Name, err)
	}
}

func registerTestTools(t *testing.T, registry *tooling.Registry) {
	t.Helper()

	// Register a few tools visible in host session
	tools := []tooling.Tool{
		&mockTool{
			name:     "read_file",
			readOnly: true,
			sessions: []string{"host", "workspace"},
			modes:    []string{"chat", "inspect", "plan", "execute"},
		},
		&mockTool{
			name:     "exec_command",
			readOnly: true,
			sessions: []string{"host", "workspace"},
			modes:    []string{"execute"},
		},
		&mockTool{
			name:     "workspace_dispatch",
			readOnly: true,
			meta: tooling.ToolMetadata{
				Origin: tooling.ToolOriginMeta,
			},
			sessions: []string{"workspace"},
			modes:    []string{"execute"},
		},
	}

	for _, tool := range tools {
		registerFactoryTool(t, registry, tool)
	}
}

func compiledToolCount(ctx promptcompiler.CompileContext) int {
	return len(ctx.AssembledTools)
}

func assembledToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, assembled := range tools {
		names = append(names, assembled.Metadata().Name)
	}
	return names
}

func runtimeToolNames(t *testing.T, tools []tool.BaseTool) []string {
	t.Helper()

	names := make([]string, 0, len(tools))
	for _, runtimeTool := range tools {
		info, err := runtimeTool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info error = %v", err)
		}
		names = append(names, info.Name)
	}
	return names
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewAgentFactory(t *testing.T) {
	factory, _ := newTestFactory(t)
	if factory == nil {
		t.Fatal("expected non-nil factory")
	}
	if factory.definitions == nil {
		t.Fatal("expected non-nil definitions map")
	}
}

func TestRegisterDefinition(t *testing.T) {
	factory, _ := newTestFactory(t)

	def := &AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "worker-v1",
		MaxIterations: 20,
	}

	err := factory.RegisterDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := factory.GetDefinition(AgentKindWorker)
	if got == nil {
		t.Fatal("expected registered definition")
	}
	if got.Name != "worker-v1" {
		t.Errorf("expected name worker-v1, got %s", got.Name)
	}
}

func TestRegisterDefinition_Invalid(t *testing.T) {
	factory, _ := newTestFactory(t)

	// nil definition
	err := factory.RegisterDefinition(nil)
	if err == nil {
		t.Fatal("expected error for nil definition")
	}

	// invalid kind
	err = factory.RegisterDefinition(&AgentDefinition{
		Kind: "invalid",
		Name: "test",
	})
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestCreateHostAgent(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateHostAgent(context.Background(), "host-1", "execute")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Kind != AgentKindWorker {
		t.Errorf("expected kind worker, got %s", cfg.Kind)
	}
	if cfg.HostID != "host-1" {
		t.Errorf("expected hostID host-1, got %s", cfg.HostID)
	}
	if cfg.Model == nil {
		t.Error("expected non-nil model")
	}
	if len(cfg.Instructions) == 0 {
		t.Error("expected non-empty instructions")
	}
	if len(cfg.Tools) == 0 {
		t.Error("expected non-empty tools")
	}
	if cfg.MaxIterations <= 0 {
		t.Error("expected positive max iterations")
	}
	if got := compiledToolCount(compiler.lastCompileForEino); got == 0 {
		t.Fatal("expected assembled tools in host compile context")
	}
}

func TestCreateHostAgent_EmptyHostID(t *testing.T) {
	factory, _ := newTestFactory(t)

	_, err := factory.CreateHostAgent(context.Background(), "", "chat")
	if err == nil {
		t.Fatal("expected error for empty hostID")
	}
}

func TestCreateHostChildAgentAddsBoundPromptAsset(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateHostChildAgent(context.Background(), hostops.SpawnHostChildRequest{
		MissionID:       "mission-1",
		HostID:          "host-a",
		HostDisplayName: "pg-primary",
		Task:            "prepare pg primary",
	})
	if err != nil {
		t.Fatalf("CreateHostChildAgent() error = %v", err)
	}
	if cfg.HostID != "host-a" || cfg.MissionID != "mission-1" {
		t.Fatalf("cfg = %+v, want host-a/mission-1", cfg)
	}
	if len(compiler.lastCompileForEino.SkillPromptAssets) != 1 {
		t.Fatalf("SkillPromptAssets = %#v, want one host child prompt", compiler.lastCompileForEino.SkillPromptAssets)
	}
	prompt := compiler.lastCompileForEino.SkillPromptAssets[0]
	for _, want := range []string{
		"你是 host-bound 运维子 Agent。",
		"你的绑定主机是 pg-primary，hostId=host-a。",
		"你只能对这个主机执行检查、配置、安装或诊断。",
		"如果任务需要其他主机信息，你只能向 manager 汇报需要协调，不能直接操作其他主机。",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("host child prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCreateHostAgent_UsesDefinitionMaxIterations(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	// Register a worker definition with custom max iterations
	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "custom-worker",
		MaxIterations: 42,
	})

	cfg, err := factory.CreateHostAgent(context.Background(), "host-1", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxIterations != 42 {
		t.Errorf("expected max iterations 42, got %d", cfg.MaxIterations)
	}
}

func TestCreateWorkspaceAgent(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	wsCfg, err := factory.CreateWorkspaceAgent(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wsCfg.MissionID != "mission-1" {
		t.Errorf("expected missionID mission-1, got %s", wsCfg.MissionID)
	}

	// Planner config
	if wsCfg.Planner.Kind != AgentKindPlanner {
		t.Errorf("expected planner kind, got %s", wsCfg.Planner.Kind)
	}
	if wsCfg.Planner.Model == nil {
		t.Error("expected non-nil planner model")
	}
	if len(wsCfg.Planner.Instructions) == 0 {
		t.Error("expected non-empty planner instructions")
	}
	if wsCfg.Planner.MissionID != "mission-1" {
		t.Errorf("expected planner missionID mission-1, got %s", wsCfg.Planner.MissionID)
	}

	// Executor config
	if wsCfg.Executor.Model == nil {
		t.Error("expected non-nil executor model")
	}

	// Replanner config
	if wsCfg.Replanner.Kind != AgentKindPlanner {
		t.Errorf("expected replanner kind planner, got %s", wsCfg.Replanner.Kind)
	}
	if wsCfg.Replanner.Model == nil {
		t.Error("expected non-nil replanner model")
	}
	if got := compiledToolCount(compiler.lastCompileForEino); got == 0 {
		t.Fatal("expected assembled tools in workspace compile context")
	}
}

func TestCreateHostAgent_RuntimeToolsMatchCompiledAssembledTools(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateHostAgent(context.Background(), "host-1", "execute")
	if err != nil {
		t.Fatalf("CreateHostAgent() error = %v", err)
	}

	got := runtimeToolNames(t, cfg.Tools)
	want := assembledToolNames(compiler.lastCompileForEino.AssembledTools)
	if len(got) != len(want) {
		t.Fatalf("runtime tools len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("runtime tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCreateWorkspaceAgent_RuntimeToolsMatchCompiledAssembledTools(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	wsCfg, err := factory.CreateWorkspaceAgent(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("CreateWorkspaceAgent() error = %v", err)
	}

	if len(compiler.compileForEinoCalls) != 3 {
		t.Fatalf("compileForEino calls = %d, want 3", len(compiler.compileForEinoCalls))
	}

	cases := []struct {
		name string
		cfg  AgentConfig
		ctx  promptcompiler.CompileContext
	}{
		{name: "planner", cfg: wsCfg.Planner, ctx: compiler.compileForEinoCalls[0]},
		{name: "executor", cfg: wsCfg.Executor, ctx: compiler.compileForEinoCalls[1]},
		{name: "replanner", cfg: wsCfg.Replanner, ctx: compiler.compileForEinoCalls[2]},
	}

	for _, tc := range cases {
		got := runtimeToolNames(t, tc.cfg.Tools)
		want := assembledToolNames(tc.ctx.AssembledTools)
		if len(got) != len(want) {
			t.Fatalf("%s runtime tools len = %d, want %d", tc.name, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s runtime tool[%d] = %q, want %q", tc.name, i, got[i], want[i])
			}
		}
	}
}

func TestCreateWorkspaceAgent_EmptyMissionID(t *testing.T) {
	factory, _ := newTestFactory(t)

	_, err := factory.CreateWorkspaceAgent(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty missionID")
	}
}

func TestCreateWorkerAgent(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-2", "check disk usage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Kind != AgentKindWorker {
		t.Errorf("expected kind worker, got %s", cfg.Kind)
	}
	if cfg.HostID != "host-2" {
		t.Errorf("expected hostID host-2, got %s", cfg.HostID)
	}
	if cfg.Model == nil {
		t.Error("expected non-nil model")
	}
	if len(cfg.Instructions) == 0 {
		t.Error("expected non-empty instructions")
	}
	if cfg.MaxIterations <= 0 {
		t.Error("expected positive max iterations")
	}
	if got := compiledToolCount(compiler.lastCompileForEino); got == 0 {
		t.Fatal("expected assembled tools in worker compile context")
	}
}

func TestCreateWorkerAgent_RuntimeToolsMatchCompiledAssembledTools(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-2", "check disk usage")
	if err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	got := runtimeToolNames(t, cfg.Tools)
	want := assembledToolNames(compiler.lastCompileForEino.AssembledTools)
	if len(got) != len(want) {
		t.Fatalf("runtime tools len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("runtime tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCreateWorkerAgent_EmptyHostID(t *testing.T) {
	factory, _ := newTestFactory(t)

	_, err := factory.CreateWorkerAgent(context.Background(), "", "some task")
	if err == nil {
		t.Fatal("expected error for empty hostID")
	}
}

func TestCreateWorkerAgent_FiltersByToolAllowlist(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "restricted-worker",
		MaxIterations: 10,
		Tools:         []string{"exec_command"},
	})

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-3", "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Tools) != 1 {
		t.Fatalf("worker tools len = %d, want 1", len(cfg.Tools))
	}
	info, err := cfg.Tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info error = %v", err)
	}
	if info.Name != "exec_command" {
		t.Fatalf("worker tool name = %q, want exec_command", info.Name)
	}
}

func TestCreateWorkerAgent_UsesDefinitionMaxIterations(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "custom-worker",
		MaxIterations: 30,
	})

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxIterations != 30 {
		t.Errorf("expected max iterations 30, got %d", cfg.MaxIterations)
	}
}

func TestCreateWorkspaceAgent_UsesDefinitionMaxIterations(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindPlanner,
		Name:          "custom-planner",
		MaxIterations: 8,
	})

	wsCfg, err := factory.CreateWorkspaceAgent(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wsCfg.Planner.MaxIterations != 8 {
		t.Errorf("expected planner max iterations 8, got %d", wsCfg.Planner.MaxIterations)
	}
}

func TestAgentFactory_UsesSharedDefinitionRegistryForHostCreation(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	defRegistry := agents.NewRegistry()
	if err := defRegistry.Register(agents.Definition{
		Kind:          string(AgentKindWorker),
		Name:          "registry-worker",
		MaxIterations: 37,
	}); err != nil {
		t.Fatalf("register shared worker definition: %v", err)
	}
	factory.SetDefinitionRegistry(defRegistry)

	cfg, err := factory.CreateHostAgent(context.Background(), "host-1", "execute")
	if err != nil {
		t.Fatalf("CreateHostAgent() error = %v", err)
	}
	if cfg.MaxIterations != 37 {
		t.Fatalf("MaxIterations = %d, want 37", cfg.MaxIterations)
	}
}

func TestAgentFactory_UsesSharedDefinitionRegistryToolAllowlist(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	registerFactoryTool(t, registry, &mockTool{
		name:     "service_metrics",
		readOnly: true,
		meta: tooling.ToolMetadata{
			IsMCP: true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "metrics",
				ServerName: "metrics",
				ToolName:   "service_metrics",
			},
		},
		sessions: []string{"workspace"},
		modes:    []string{"execute"},
	})

	defRegistry := agents.NewRegistry()
	if err := defRegistry.Register(agents.Definition{
		Kind:          string(AgentKindWorker),
		Name:          "registry-worker",
		MaxIterations: 12,
		Tools:         []string{"service_metrics"},
	}); err != nil {
		t.Fatalf("register shared worker definition: %v", err)
	}
	factory.SetDefinitionRegistry(defRegistry)

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task")
	if err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	if len(cfg.Tools) != 1 {
		t.Fatalf("worker tools len = %d, want 1", len(cfg.Tools))
	}
	info, err := cfg.Tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info error = %v", err)
	}
	if info.Name != "service_metrics" {
		t.Fatalf("worker tool name = %q, want service_metrics", info.Name)
	}
}

func TestCreateWorkerAgent_PreservesMetadataTraitsWithoutKindMCPTool(t *testing.T) {
	factory, registry := newTestFactory(t)
	compiler := factory.compiler.(*mockCompiler)

	registerFactoryTool(t, registry, &mockTool{
		name:        "read_file",
		description: "metadata-aware tool",
		readOnly:    true,
		meta: tooling.ToolMetadata{
			Aliases:     []string{"fs_read"},
			SearchHint:  "filesystem read",
			ShouldDefer: true,
			AlwaysLoad:  true,
			IsMCP:       true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "filesystem",
				ServerName: "filesystem",
				ToolName:   "read_file",
			},
		},
		sessions: []string{"workspace"},
		modes:    []string{"execute"},
	})

	defRegistry := agents.NewRegistry()
	if err := defRegistry.Register(agents.Definition{
		Kind:          string(AgentKindWorker),
		Name:          "registry-worker",
		MaxIterations: 12,
		Tools:         []string{"read_file"},
	}); err != nil {
		t.Fatalf("register shared worker definition: %v", err)
	}
	factory.SetDefinitionRegistry(defRegistry)

	if _, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task"); err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	if len(compiler.lastCompileForEino.AssembledTools) != 1 {
		t.Fatalf("assembled tools len = %d, want 1", len(compiler.lastCompileForEino.AssembledTools))
	}
	meta := compiler.lastCompileForEino.AssembledTools[0].Metadata()
	if !meta.HasMCPSource() {
		t.Fatalf("expected MCP traits in worker assembled tool, got %#v", meta)
	}
	if meta.MCPInfo.ServerID != "filesystem" {
		t.Fatalf("worker MCPInfo.ServerID = %q, want filesystem", meta.MCPInfo.ServerID)
	}
	if meta.SearchHint != "filesystem read" {
		t.Fatalf("worker SearchHint = %q, want filesystem read", meta.SearchHint)
	}
	if !meta.ShouldDefer || !meta.AlwaysLoad {
		t.Fatalf("expected defer/load traits to be preserved, got %#v", meta)
	}
	if len(meta.Aliases) != 1 || meta.Aliases[0] != "fs_read" {
		t.Fatalf("worker Aliases = %#v, want [fs_read]", meta.Aliases)
	}
}

func TestCreateWorkerAgent_ToolAllowlistKeepsMetadataDrivenMCPTool(t *testing.T) {
	factory, registry := newTestFactory(t)
	compiler := factory.compiler.(*mockCompiler)

	registerFactoryTool(t, registry, &mockTool{
		name:        "read_file",
		description: "metadata-aware tool",
		readOnly:    true,
		meta: tooling.ToolMetadata{
			IsMCP: true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "filesystem",
				ServerName: "filesystem",
				ToolName:   "read_file",
			},
		},
		sessions: []string{"workspace"},
		modes:    []string{"execute"},
	})

	defRegistry := agents.NewRegistry()
	if err := defRegistry.Register(agents.Definition{
		Kind:          string(AgentKindWorker),
		Name:          "registry-worker",
		MaxIterations: 12,
		Tools:         []string{"read_file"},
	}); err != nil {
		t.Fatalf("register shared worker definition: %v", err)
	}
	factory.SetDefinitionRegistry(defRegistry)

	if _, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task"); err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	if len(compiler.lastCompileForEino.AssembledTools) != 1 {
		t.Fatalf("assembled tools len = %d, want 1", len(compiler.lastCompileForEino.AssembledTools))
	}
	meta := compiler.lastCompileForEino.AssembledTools[0].Metadata()
	if !meta.HasMCPSource() {
		t.Fatalf("expected scope to keep metadata-driven MCP tool, got %#v", meta)
	}
}

func TestCreateWorkerAgent_PrefersBuiltinOverMetadataDrivenMCPConflict(t *testing.T) {
	factory, registry := newTestFactory(t)
	compiler := factory.compiler.(*mockCompiler)

	registerFactoryTool(t, registry, &mockTool{
		name:        "read_file",
		description: "builtin tool",
		readOnly:    true,
		sessions:    []string{"workspace"},
		modes:       []string{"execute"},
	})
	registerFactoryTool(t, registry, &mockTool{
		name:        "read_file",
		description: "metadata-aware tool",
		readOnly:    true,
		meta: tooling.ToolMetadata{
			IsMCP: true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "filesystem",
				ServerName: "filesystem",
				ToolName:   "read_file",
			},
		},
		sessions: []string{"workspace"},
		modes:    []string{"execute"},
	})

	defRegistry := agents.NewRegistry()
	if err := defRegistry.Register(agents.Definition{
		Kind:          string(AgentKindWorker),
		Name:          "registry-worker",
		MaxIterations: 12,
		Tools:         []string{"read_file"},
	}); err != nil {
		t.Fatalf("register shared worker definition: %v", err)
	}
	factory.SetDefinitionRegistry(defRegistry)

	if _, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task"); err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	if len(compiler.lastCompileForEino.AssembledTools) != 1 {
		t.Fatalf("assembled tools len = %d, want 1", len(compiler.lastCompileForEino.AssembledTools))
	}
	meta := compiler.lastCompileForEino.AssembledTools[0].Metadata()
	if meta.Description != "builtin tool" {
		t.Fatalf("worker tool description = %q, want builtin tool", meta.Description)
	}
	if meta.HasMCPSource() {
		t.Fatalf("expected builtin tool to win conflict, got %#v", meta)
	}
}
