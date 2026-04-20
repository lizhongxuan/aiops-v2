package agentmgr

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/capability"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
)

// ---------------------------------------------------------------------------
// Mock implementations for testing
// ---------------------------------------------------------------------------

// mockCompiler implements promptcompiler.Compiler for testing.
type mockCompiler struct {
	compileFunc        func(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error)
	compileForEino     func(ctx promptcompiler.CompileContext) ([]*schema.Message, error)
	lastCompileCtx     promptcompiler.CompileContext
	lastCompileForEino promptcompiler.CompileContext
}

func (m *mockCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	m.lastCompileCtx = ctx
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

// mockToolRuntime implements capability.ToolRuntime for testing.
type mockToolRuntime struct {
	name string
}

func (m *mockToolRuntime) Description() string                        { return "mock tool: " + m.name }
func (m *mockToolRuntime) CheckPermissions(ctx context.Context) error { return nil }
func (m *mockToolRuntime) IsReadOnly() bool                           { return true }
func (m *mockToolRuntime) IsDestructive() bool                        { return false }
func (m *mockToolRuntime) IsConcurrencySafe() bool                    { return true }
func (m *mockToolRuntime) Display() capability.ToolDisplayPayload {
	return capability.ToolDisplayPayload{Type: "text"}
}
func (m *mockToolRuntime) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockToolRuntime) Execute(ctx context.Context, args json.RawMessage) (capability.ToolResult, error) {
	return capability.ToolResult{Content: "ok"}, nil
}

// ---------------------------------------------------------------------------
// Helper to create a test factory with common setup.
// ---------------------------------------------------------------------------

func newTestFactory(t *testing.T) (*AgentFactory, *capability.Registry) {
	t.Helper()

	registry := capability.NewRegistry()
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

func registerTestTools(t *testing.T, registry *capability.Registry) {
	t.Helper()

	// Register a few tools visible in host session
	tools := []capability.Entry{
		{
			ID:   "tool-read-file",
			Name: "read_file",
			Kind: capability.KindTool,
			Tool: &mockToolRuntime{name: "read_file"},
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"chat", "inspect", "plan", "execute"},
			},
		},
		{
			ID:   "tool-exec-cmd",
			Name: "exec_command",
			Kind: capability.KindTool,
			Tool: &mockToolRuntime{name: "exec_command"},
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"execute"},
			},
		},
		{
			ID:   "tool-workspace-dispatch",
			Name: "workspace_dispatch",
			Kind: capability.KindWorkspace,
			Tool: &mockToolRuntime{name: "workspace_dispatch"},
			Visibility: capability.Visibility{
				SessionTypes: []string{"workspace"},
				Modes:        []string{"execute"},
			},
		},
	}

	for _, e := range tools {
		if err := registry.Register(e); err != nil {
			t.Fatalf("register tool %s: %v", e.Name, err)
		}
	}
}

func compiledToolCount(ctx promptcompiler.CompileContext) int {
	return len(ctx.AssembledTools)
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
		CapabilityScope: CapabilityScope{
			Kinds: []capability.Kind{capability.KindTool, capability.KindMCPTool},
		},
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

func TestCreateWorkerAgent_EmptyHostID(t *testing.T) {
	factory, _ := newTestFactory(t)

	_, err := factory.CreateWorkerAgent(context.Background(), "", "some task")
	if err == nil {
		t.Fatal("expected error for empty hostID")
	}
}

func TestCreateWorkerAgent_FiltersByCapabilityScope(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)

	// Register worker definition that only allows KindTool (not KindWorkspace)
	factory.RegisterDefinition(&AgentDefinition{
		Kind:          AgentKindWorker,
		Name:          "restricted-worker",
		MaxIterations: 10,
		CapabilityScope: CapabilityScope{
			Kinds: []capability.Kind{capability.KindTool},
		},
	})

	cfg, err := factory.CreateWorkerAgent(context.Background(), "host-3", "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Worker should only have KindTool tools, not workspace tools
	for _, t2 := range cfg.Tools {
		info, _ := t2.Info(context.Background())
		if info != nil && info.Name == "workspace_dispatch" {
			t.Error("worker agent should not have workspace_dispatch tool")
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
