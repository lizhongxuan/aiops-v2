package promptcompiler

import (
	"strings"
	"testing"
)

func TestHostOpsManagerPromptIncludesMandatoryRoutingRules(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		AgentKind:           AgentKindPlanner,
		HostOpsManager:      true,
		HostOpsPlanRequired: true,
		Profile:             PromptProfileHostManager,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	profile := compiledPromptSectionForTest(t, compiled, "profile.host_manager").Content

	for _, want := range []string{
		"Create a compact plan for complex host work before delegation.",
		"Do not run host commands directly.",
		"Delegate clear sub-tasks to host-bound child agents",
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("host manager profile missing %q:\n%s", want, profile)
		}
	}
}

func TestHostOpsManagerPromptOmittedForNormalChat(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	if strings.Contains(modelInput, "## Host Operations Manager") {
		t.Fatalf("normal chat should not include host ops manager section:\n%s", modelInput)
	}
}
