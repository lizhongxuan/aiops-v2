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

func containsFactoryToolName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
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
		MissionID:            "mission-1",
		HostID:               "host-a",
		HostDisplayName:      "host-a",
		Task:                 "inspect assigned host readiness",
		PlanStepID:           "step-1",
		RiskLevel:            "read_only",
		EvidenceRequirements: []string{"command_result"},
	})
	if err != nil {
		t.Fatalf("CreateHostChildAgent() error = %v", err)
	}
	if cfg.HostID != "host-a" || cfg.MissionID != "mission-1" {
		t.Fatalf("cfg = %+v, want host-a/mission-1", cfg)
	}
	if len(compiler.lastCompileForEino.SkillPromptAssets) != 0 {
		t.Fatalf("SkillPromptAssets = %#v, want host child prompt separated from skill context", compiler.lastCompileForEino.SkillPromptAssets)
	}
	if len(compiler.lastCompileForEino.HostTaskPromptAssets) != 1 {
		t.Fatalf("HostTaskPromptAssets = %#v, want one host child prompt asset", compiler.lastCompileForEino.HostTaskPromptAssets)
	}
	if len(compiler.lastCompileForEino.ExtraSections) != 0 {
		t.Fatalf("ExtraSections = %#v, want no host task context in generic extra sections", compiler.lastCompileForEino.ExtraSections)
	}
	prompt := compiler.lastCompileForEino.HostTaskPromptAssets[0]
	for _, want := range []string{
		"prompt_section_id: host_agent.binding.v1",
		"prompt_section_id: host_agent.assigned_subtask.v1",
		"prompt_section_id: host_agent.execution_protocol.v1",
		"prompt_section_id: host_agent.report_contract.v1",
		"prompt_section_id: host_agent.stop_block_conditions.v1",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("host child prompt missing %q:\n%s", want, prompt)
		}
	}
	if cfg.Metadata["hostTaskPromptAssetSource"] == "" {
		t.Fatalf("metadata = %#v, want hostTaskPromptAssetSource", cfg.Metadata)
	}
	if cfg.Metadata["runtimeBase"] != "host_agent" {
		t.Fatalf("metadata = %#v, want host agent runtime base", cfg.Metadata)
	}
	if cfg.Metadata["agentRole"] != "host_child_task" {
		t.Fatalf("metadata = %#v, want host child task role", cfg.Metadata)
	}
}

func TestHostChildPromptAssetRendersStableSectionIDs(t *testing.T) {
	prompt := hostChildPromptAsset(hostops.SpawnHostChildRequest{
		MissionID:            "mission-prompt",
		ChildAgentID:         "child-prompt",
		HostID:               "host-a",
		HostDisplayName:      "generic host",
		Task:                 "inspect assigned service state",
		PlanStepID:           "step-a",
		RiskLevel:            "read_only",
		EvidenceRequirements: []string{"command_result"},
	})
	for _, want := range []string{
		"prompt_section_id: host_agent.binding.v1",
		"prompt_section_id: host_agent.assigned_subtask.v1",
		"prompt_section_id: host_agent.execution_protocol.v1",
		"prompt_section_id: host_agent.report_contract.v1",
		"prompt_section_id: host_agent.stop_block_conditions.v1",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing stable section id %q:\n%s", want, prompt)
		}
	}
}

func TestHostChildPromptAssetIncludesBindingSubtaskProtocolReportAndStopSections(t *testing.T) {
	prompt := hostChildPromptAsset(hostops.SpawnHostChildRequest{
		MissionID:            "mission-prompt",
		ChildAgentID:         "child-prompt",
		HostID:               "host-a",
		Task:                 "inspect assigned host",
		PlanStepID:           "step-a",
		RiskLevel:            "read_only",
		EvidenceRequirements: []string{"artifact_ref"},
	})
	for _, want := range []string{
		"bound_host_id: host-a",
		"mission_id: mission-prompt",
		"plan_step_id: step-a",
		"goal: inspect assigned host",
		"Use HostCommandTool for automated host commands.",
		"HostTaskReport",
		"needs_manager_coordination",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestHostChildPromptAssetRedactsSecrets(t *testing.T) {
	prompt := hostChildPromptAsset(hostops.SpawnHostChildRequest{
		MissionID:    "mission-prompt",
		ChildAgentID: "child-prompt",
		HostID:       "host-a",
		Task:         "inspect token=plain-secret password=plain-secret",
		Constraints:  []string{"Bearer plain-secret", "cookie: session=plain-secret"},
		PlanStepID:   "step-a",
		RiskLevel:    "read_only",
	})
	if strings.Contains(prompt, "plain-secret") || strings.Contains(prompt, "Bearer plain-secret") {
		t.Fatalf("prompt contains unredacted secret:\n%s", prompt)
	}
	if !strings.Contains(prompt, "[REDACTED") {
		t.Fatalf("prompt missing redaction marker:\n%s", prompt)
	}
}

func TestHostChildPromptAssetUsesSafeDefaultsForEmptyFields(t *testing.T) {
	prompt := hostChildPromptAsset(hostops.SpawnHostChildRequest{HostID: "host-a"})
	for _, want := range []string{
		"role: host_child",
		"goal: operate on the assigned host and report evidence to the manager",
		"plan_step_id: unspecified",
		"risk: unknown",
		"evidence_requirements: command_result",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing safe default %q:\n%s", want, prompt)
		}
	}
}

func TestHostChildPromptAssetContainsNoDomainHardcoding(t *testing.T) {
	prompt := hostChildPromptAsset(hostops.SpawnHostChildRequest{
		MissionID:    "mission-prompt",
		ChildAgentID: "child-prompt",
		HostID:       "host-a",
		Task:         "inspect assigned generic service and process state",
		PlanStepID:   "step-a",
		RiskLevel:    "read_only",
	})
	for _, forbidden := range []string{
		"nginx",
		"redis",
		"mysql",
		"kubernetes",
		"example.com",
		"120.77",
		"真实主机",
		"固定中间件",
		"固定网站",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("prompt contains domain hardcoding %q:\n%s", forbidden, prompt)
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

func TestCreateWorkerAgent_ReusesHostAgentRuntimeProfile(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	compiler := factory.compiler.(*mockCompiler)

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-2", "check disk usage")
	if err != nil {
		t.Fatalf("CreateWorkerAgent() error = %v", err)
	}

	if compiler.lastCompileForEino.SessionType != "host" {
		t.Fatalf("SessionType = %q, want host", compiler.lastCompileForEino.SessionType)
	}
	if compiler.lastCompileForEino.Mode != "execute" {
		t.Fatalf("Mode = %q, want execute", compiler.lastCompileForEino.Mode)
	}
	if compiler.lastCompileForEino.HostContext != "host-2" {
		t.Fatalf("HostContext = %q, want host-2", compiler.lastCompileForEino.HostContext)
	}
	if cfg.Input != "check disk usage" {
		t.Fatalf("Input = %q, want task input", cfg.Input)
	}
	if cfg.Metadata["runtimeProfile"] != "host_agent_full_runtime" {
		t.Fatalf("metadata = %#v, want host agent runtime profile", cfg.Metadata)
	}
	if cfg.Metadata["compatibilityEntry"] != "CreateWorkerAgent" {
		t.Fatalf("metadata = %#v, want compatibility entry marker", cfg.Metadata)
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

func TestCreateWorkerAgent_AllowlistPreservesMandatoryInitialTools(t *testing.T) {
	factory, registry := newTestFactory(t)

	for _, name := range []string{
		"read_file",
		"exec_command",
		"web_search",
		"grep",
		"list_mcp_resources",
		"read_mcp_resource",
		"skill_search",
		"skill_read",
	} {
		registerFactoryTool(t, registry, &mockTool{
			name:     name,
			readOnly: true,
			sessions: []string{"host", "workspace"},
			modes:    []string{"chat", "inspect", "plan", "execute"},
		})
	}

	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "restricted-worker",
		MaxIterations: 10,
		Tools:         []string{"read_file"},
	})

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-3", "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := runtimeToolNames(t, cfg.Tools)
	for _, want := range []string{
		"read_file",
		"exec_command",
		"web_search",
		"grep",
		"list_mcp_resources",
		"read_mcp_resource",
		"skill_search",
		"skill_read",
	} {
		if !containsFactoryToolName(names, want) {
			t.Fatalf("worker tools = %v, missing %s", names, want)
		}
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
		sessions: []string{"host"},
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

	names := runtimeToolNames(t, cfg.Tools)
	for _, want := range []string{"service_metrics", "exec_command"} {
		if !containsFactoryToolName(names, want) {
			t.Fatalf("worker tools = %v, missing %s", names, want)
		}
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
		sessions: []string{"host"},
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
		sessions: []string{"host"},
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
		sessions:    []string{"host"},
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
		sessions: []string{"host"},
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
