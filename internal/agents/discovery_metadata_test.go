package agents

import "testing"

func TestAgentDefinitionParsesDiscoveryMetadata(t *testing.T) {
	r := NewRegistry()
	def := Definition{
		Kind:        "explorer",
		Name:        "synthetic-explorer",
		Source:      string(SourceBuiltin),
		Description: "collects independent evidence",
		Prompt:      "full prompt body must stay out of catalog listings",
		Discovery: AgentDiscoveryMetadata{
			WhenToUse:       "Use when independent read-only evidence is needed.",
			CapabilityKinds: []string{"evidence", "analysis"},
			ResourceTypes:   []string{"logs", "metrics"},
			OperationKinds:  []string{"read", "inspect"},
			Modes:           []string{"explore"},
			ModelInvocable:  true,
		},
		Budget: AgentBudgetMetadata{
			MaxConcurrent: 2,
			CostClass:     "low",
		},
	}

	if err := r.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get("synthetic-explorer")
	if !ok {
		t.Fatal("Get() returned false for registered definition")
	}
	if got.Discovery.WhenToUse != def.Discovery.WhenToUse {
		t.Fatalf("Discovery.WhenToUse = %q, want %q", got.Discovery.WhenToUse, def.Discovery.WhenToUse)
	}
	if got.Discovery.CapabilityKinds[0] != "evidence" {
		t.Fatalf("Discovery.CapabilityKinds = %#v, want evidence first", got.Discovery.CapabilityKinds)
	}
	if got.Discovery.ModelInvocable != true {
		t.Fatal("Discovery.ModelInvocable = false, want true")
	}
	if got.Budget.MaxConcurrent != 2 || got.Budget.CostClass != "low" {
		t.Fatalf("Budget = %#v, want maxConcurrent=2 costClass=low", got.Budget)
	}

	got.Discovery.CapabilityKinds[0] = "mutated"
	again, ok := r.Get("synthetic-explorer")
	if !ok {
		t.Fatal("expected definition to remain registered after mutation")
	}
	if again.Discovery.CapabilityKinds[0] != "evidence" {
		t.Fatalf("registry mutated through Discovery copy: got %#v", again.Discovery.CapabilityKinds)
	}
}

func TestAgentDefinitionRejectsUnknownCostClass(t *testing.T) {
	def := Definition{
		Kind:   "worker",
		Name:   "synthetic-worker",
		Source: string(SourceBuiltin),
		Budget: AgentBudgetMetadata{CostClass: "expensive"},
	}

	if err := def.Validate(); err == nil {
		t.Fatal("Validate() should reject unknown cost class")
	}
}
