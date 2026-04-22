package agents

import (
	"strings"
	"testing"

	"aiops-v2/internal/settings"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	def := Definition{
		Kind:          "worker",
		Name:          "worker-default",
		Source:        string(SourceBuiltin),
		Description:   "default worker agent",
		Prompt:        "You are a worker agent.",
		Tools:         []string{"read_file", "exec_command"},
		Model:         "gpt-4o-mini",
		Hooks:         []string{"pre_tool_use"},
		MCPServers:    []string{"filesystem"},
		MaxIterations: 12,
	}

	if err := r.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get("worker-default")
	if !ok {
		t.Fatal("Get() returned false for registered definition")
	}
	if got.Kind != def.Kind || got.Name != def.Name || got.Source != string(SourceBuiltin) || got.Prompt != def.Prompt {
		t.Fatalf("Get() = %+v, want %+v", got, def)
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	list[0].Tools[0] = "mutated"

	again, ok := r.Get("worker-default")
	if !ok {
		t.Fatal("expected definition to remain registered after List mutation")
	}
	if again.Tools[0] != "read_file" {
		t.Fatalf("registry mutated through List() copy: got Tools[0] = %q", again.Tools[0])
	}
}

func TestDefinitionToAgentToolPreservesOrchestrationFields(t *testing.T) {
	def := Definition{
		Kind:          "worker",
		Name:          "worker-default",
		Source:        string(SourceBuiltin),
		Description:   "default worker agent",
		Prompt:        "You are a worker agent.",
		Tools:         []string{"read_file", "exec_command"},
		Model:         "gpt-4o-mini",
		Hooks:         []string{"pre_tool_use"},
		MCPServers:    []string{"filesystem"},
		MaxIterations: 12,
	}

	got := def.ToAgentTool()

	if got.Kind != def.Kind {
		t.Fatalf("AgentTool.Kind = %q, want %q", got.Kind, def.Kind)
	}
	if got.Name != def.Name {
		t.Fatalf("AgentTool.Name = %q, want %q", got.Name, def.Name)
	}
	if got.Description != def.Description {
		t.Fatalf("AgentTool.Description = %q, want %q", got.Description, def.Description)
	}
	if got.Prompt != def.Prompt {
		t.Fatalf("AgentTool.Prompt = %q, want %q", got.Prompt, def.Prompt)
	}
	if got.Model != def.Model {
		t.Fatalf("AgentTool.Model = %q, want %q", got.Model, def.Model)
	}
	if got.MaxIterations != def.MaxIterations {
		t.Fatalf("AgentTool.MaxIterations = %d, want %d", got.MaxIterations, def.MaxIterations)
	}
	if len(got.Tools) != len(def.Tools) || got.Tools[0] != def.Tools[0] {
		t.Fatalf("AgentTool.Tools = %#v, want %#v", got.Tools, def.Tools)
	}
	if len(got.Hooks) != len(def.Hooks) || got.Hooks[0] != def.Hooks[0] {
		t.Fatalf("AgentTool.Hooks = %#v, want %#v", got.Hooks, def.Hooks)
	}
	if len(got.MCPServers) != len(def.MCPServers) || got.MCPServers[0] != def.MCPServers[0] {
		t.Fatalf("AgentTool.MCPServers = %#v, want %#v", got.MCPServers, def.MCPServers)
	}

	got.Tools[0] = "mutated"
	got.Hooks[0] = "mutated"
	got.MCPServers[0] = "mutated"
	if def.Tools[0] != "read_file" || def.Hooks[0] != "pre_tool_use" || def.MCPServers[0] != "filesystem" {
		t.Fatalf("ToAgentTool must deep copy slices, got definition %#v", def)
	}
}

func TestRegistrySourcePrecedence(t *testing.T) {
	r := NewRegistry()

	defs := []Definition{
		{Kind: "worker", Name: "worker-policy", Source: string(SourcePolicySettings)},
		{Kind: "worker", Name: "worker-built-in", Source: string(SourceBuiltin)},
		{Kind: "worker", Name: "worker-plugin", Source: string(SourcePlugin)},
		{Kind: "worker", Name: "worker-user", Source: string(SourceUserSettings)},
		{Kind: "worker", Name: "worker-project", Source: string(SourceProjectSettings)},
		{Kind: "worker", Name: "worker-flag", Source: string(SourceFlagSettings)},
	}

	for _, def := range defs {
		if err := r.Register(def); err != nil {
			t.Fatalf("Register(%q) error = %v", def.Name, err)
		}
	}

	got, ok := r.Get("worker")
	if !ok {
		t.Fatal("expected kind lookup to resolve to an active definition")
	}
	if got.Name != "worker-policy" || got.Source != string(SourcePolicySettings) {
		t.Fatalf("Get(worker) = %+v, want policy settings definition", got)
	}

	for _, def := range defs {
		got, ok := r.Get(def.Name)
		if !ok {
			t.Fatalf("Get(%q) returned false", def.Name)
		}
		if got.Source != def.Source {
			t.Fatalf("Get(%q).Source = %q, want %q", def.Name, got.Source, def.Source)
		}
	}

	list := r.List()
	if len(list) != len(defs) {
		t.Fatalf("List() len = %d, want %d", len(list), len(defs))
	}
}

func TestRegistryRejectDuplicateNameAcrossSources(t *testing.T) {
	r := NewRegistry()

	first := Definition{Kind: "worker", Name: "shared", Source: string(SourceBuiltin)}
	second := Definition{Kind: "planner", Name: "shared", Source: string(SourcePlugin)}

	if err := r.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := r.Register(second); err == nil {
		t.Fatal("Register(second) should fail for duplicate name")
	}

	got, ok := r.Get("shared")
	if !ok {
		t.Fatal("expected first definition to remain registered")
	}
	if got.Kind != "worker" || got.Source != string(SourceBuiltin) {
		t.Fatalf("Get(shared) = %+v, want builtin worker", got)
	}
}

func TestRegistryRejectDuplicateKindWithinSource(t *testing.T) {
	r := NewRegistry()

	first := Definition{Kind: "worker", Name: "worker-a", Source: string(SourceBuiltin)}
	second := Definition{Kind: "worker", Name: "worker-b", Source: string(SourceBuiltin)}

	if err := r.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := r.Register(second); err == nil {
		t.Fatal("Register(second) should fail for duplicate kind within the same source")
	}

	got, ok := r.Get("worker")
	if !ok {
		t.Fatal("expected first definition to remain addressable by kind")
	}
	if got.Name != "worker-a" || got.Source != string(SourceBuiltin) {
		t.Fatalf("Get(worker) = %+v, want builtin worker-a", got)
	}
}

func TestRegistryRegisterBatchAtomicity(t *testing.T) {
	r := NewRegistry()

	entries := []Definition{
		{Kind: "worker", Name: "worker-a", Source: string(SourceBuiltin)},
		{Kind: "planner", Name: "worker-a", Source: string(SourcePlugin)},
	}

	if err := r.RegisterBatch(entries); err == nil {
		t.Fatal("RegisterBatch() should fail for duplicate name")
	}

	if list := r.List(); len(list) != 0 {
		t.Fatalf("List() len = %d, want 0 after failed batch", len(list))
	}
}

func TestRegistryListReturnsAllDefinitions(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(Definition{Kind: "worker", Name: "worker-base", Source: string(SourceBuiltin)}); err != nil {
		t.Fatalf("Register(base) error = %v", err)
	}
	if err := r.Register(Definition{Kind: "worker", Name: "worker-plugin", Source: string(SourcePlugin)}); err != nil {
		t.Fatalf("Register(plugin) error = %v", err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
	if list[0].Name == list[1].Name {
		t.Fatalf("List() returned duplicate entries: %+v", list)
	}
}

func TestRegistryRejectsCustomAgentsWhenStrictPluginOnlyEnabled(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		RestrictToPluginOnly: []settings.CustomizationSurface{settings.SurfaceAgents},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	err := r.Register(Definition{
		Kind:   "worker",
		Name:   "custom-worker",
		Source: string(SourceUserSettings),
	})
	if err == nil {
		t.Fatal("expected strict plugin-only policy to reject userSettings agent definition")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
}
