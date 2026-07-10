package runtimekernel

import (
	"testing"

	"aiops-v2/internal/skills"
	"aiops-v2/internal/tooling"
)

func TestMandatorySkillActivationBlocksPrematureFinal(t *testing.T) {
	decision := EvaluateMandatorySkillActivation([]skills.Definition{{
		Name:        "synthetic.triage",
		Description: "Generic triage",
		Discovery: skills.SkillDiscoveryMetadata{
			WhenToUse:        "Use for log diagnosis",
			ResourceTypes:    []string{"log"},
			TaskIntents:      []string{"diagnose"},
			RequiredForMatch: true,
			ModelInvocable:   true,
		},
	}}, "diagnose log failure", "The root cause is definitely known.", SkillActivationSessionState{})

	if decision.Action != "require_skill_read" {
		t.Fatalf("Action = %q, want require_skill_read: %+v", decision.Action, decision)
	}
	if len(decision.RequiredSkills) != 1 || decision.RequiredSkills[0] != "synthetic.triage" {
		t.Fatalf("RequiredSkills = %v", decision.RequiredSkills)
	}
}

func TestMandatorySkillActivationAllowsLoadedSkill(t *testing.T) {
	state := SkillActivationSessionState{LoadedSkills: map[string]LoadedSkillRef{
		"synthetic.triage": {Name: "synthetic.triage", Source: "skill_read"},
	}}
	decision := EvaluateMandatorySkillActivation([]skills.Definition{{
		Name: "synthetic.triage",
		Discovery: skills.SkillDiscoveryMetadata{
			TaskIntents:      []string{"diagnose"},
			RequiredForMatch: true,
			ModelInvocable:   true,
		},
	}}, "diagnose issue", "The conclusion is confirmed.", state)

	if decision.Action != "allow" {
		t.Fatalf("Action = %q, want allow: %+v", decision.Action, decision)
	}
}

func TestMandatorySkillActivationUsesSearchSnapshots(t *testing.T) {
	state := SkillActivationSessionState{LastSearchResults: []SkillSearchMatchSnapshot{{
		Name:             "synthetic.triage",
		TaskIntents:      []string{"diagnose"},
		RequiredForMatch: true,
	}}}
	decision := EvaluateMandatorySkillActivation(nil, "diagnose issue", "final answer", state)
	if decision.Action != "require_skill_read" {
		t.Fatalf("Action = %q, want require_skill_read: %+v", decision.Action, decision)
	}
}

func TestMandatorySkillActivationSatisfiedByCorootEvidenceToolSurface(t *testing.T) {
	decision := MandatorySkillDecision{
		Action:         "require_skill_read",
		RequiredSkills: []string{"coroot-evidence"},
	}
	tools := []tooling.Tool{&tooling.StaticTool{Meta: tooling.ToolMetadata{
		Name:   "coroot.incidents",
		Domain: "coroot",
		Pack:   "coroot_incident",
	}}}

	if !mandatorySkillActivationSatisfiedByToolSurface(decision, tools) {
		t.Fatalf("mandatory skill decision should be satisfied by visible Coroot evidence tools")
	}
}

func TestMandatorySkillActivationDoesNotSatisfyMixedSkillsByCorootToolSurface(t *testing.T) {
	decision := MandatorySkillDecision{
		Action:         "require_skill_read",
		RequiredSkills: []string{"coroot-evidence", "ops-triage"},
	}
	tools := []tooling.Tool{&tooling.StaticTool{Meta: tooling.ToolMetadata{
		Name:   "coroot.incidents",
		Domain: "coroot",
		Pack:   "coroot_incident",
	}}}

	if mandatorySkillActivationSatisfiedByToolSurface(decision, tools) {
		t.Fatalf("mixed mandatory skills should not be satisfied only by Coroot tools")
	}
}
