package runtimekernel

import (
	"testing"

	"aiops-v2/internal/skills"
)

func TestMandatorySkillCatalogDefinitionsUseGlobalSkillIndex(t *testing.T) {
	reg := skills.NewRegistry()
	reg.Register(skills.Definition{
		Name:        "synthetic.triage",
		Description: "Use for diagnosis with logs",
		Discovery: skills.SkillDiscoveryMetadata{
			RequiredForMatch: true,
			ModelInvocable:   true,
			TaskIntents:      []string{"diagnose"},
			ResourceTypes:    []string{"log"},
		},
	})
	reg.Register(skills.Definition{
		Name:        "synthetic.optional",
		Description: "Optional helper",
		Discovery: skills.SkillDiscoveryMetadata{
			TaskIntents: []string{"diagnose"},
		},
	})

	kernel := NewRuntimeKernel(RuntimeKernelConfig{SkillRegistry: reg})
	defs := kernel.mandatorySkillDefinitionsForInput("diagnose the failing log")
	decision := EvaluateMandatorySkillActivation(defs, "diagnose the failing log", "Root cause confirmed.", SkillActivationSessionState{})

	if decision.Action != "require_skill_read" {
		t.Fatalf("Action = %q, want require_skill_read: %+v", decision.Action, decision)
	}
	if len(decision.RequiredSkills) != 1 || decision.RequiredSkills[0] != "synthetic.triage" {
		t.Fatalf("RequiredSkills = %#v, want synthetic.triage", decision.RequiredSkills)
	}
}
