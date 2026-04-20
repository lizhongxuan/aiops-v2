package agents

import (
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	def := Definition{
		Kind:          "worker",
		Name:          "worker-default",
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
	if got.Kind != def.Kind || got.Name != def.Name || got.Prompt != def.Prompt {
		t.Fatalf("Get() = %+v, want %+v", got, def)
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
}

func TestRegistryRejectDuplicateName(t *testing.T) {
	r := NewRegistry()

	first := Definition{Kind: "worker", Name: "shared"}
	second := Definition{Kind: "planner", Name: "shared"}

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
	if got.Kind != "worker" {
		t.Fatalf("Get(shared).Kind = %q, want worker", got.Kind)
	}
}

func TestRegistryRejectDuplicateKind(t *testing.T) {
	r := NewRegistry()

	first := Definition{Kind: "worker", Name: "worker-a"}
	second := Definition{Kind: "worker", Name: "worker-b"}

	if err := r.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := r.Register(second); err == nil {
		t.Fatal("Register(second) should fail for duplicate kind")
	}

	got, ok := r.Get("worker")
	if !ok {
		t.Fatal("expected first definition to remain addressable by kind")
	}
	if got.Name != "worker-a" {
		t.Fatalf("Get(worker).Name = %q, want worker-a", got.Name)
	}
}

func TestRegistryRegisterBatchAtomicity(t *testing.T) {
	r := NewRegistry()

	entries := []Definition{
		{Kind: "worker", Name: "worker-a"},
		{Kind: "worker", Name: "worker-b"},
	}

	if err := r.RegisterBatch(entries); err == nil {
		t.Fatal("RegisterBatch() should fail for duplicate kind")
	}

	if list := r.List(); len(list) != 0 {
		t.Fatalf("List() len = %d, want 0 after failed batch", len(list))
	}
}
