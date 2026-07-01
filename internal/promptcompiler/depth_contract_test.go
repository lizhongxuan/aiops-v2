package promptcompiler

import (
	"strings"
	"testing"

	"aiops-v2/internal/taskdepth"
)

func TestTaskDepthContractSectionPlacementAndContent(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		TaskDepth: taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	wantOrder := []string{"base.contract", "runtime.state", "profile.advisor", "tool.surface"}
	for i, want := range wantOrder {
		if i >= len(compiled.Envelope.Sections) || compiled.Envelope.Sections[i].ID != want {
			t.Fatalf("section[%d] = %#v, want %q", i, compiled.Envelope.Sections, want)
		}
	}
	state := compiledPromptSectionForTest(t, compiled, "runtime.state").Content
	for _, want := range []string{
		"task_depth: investigation",
		"requires_plan: true",
		"requires_evidence: true",
	} {
		if !strings.Contains(state, want) {
			t.Fatalf("runtime.state missing %q:\n%s", want, state)
		}
	}
	base := compiledPromptSectionForTest(t, compiled, "base.contract").Content
	if !strings.Contains(base, "Simple tasks get concise answers; complex tasks advance by evidence.") {
		t.Fatalf("base contract missing task-depth invariant:\n%s", base)
	}
}

func TestCompletionGateLongRulesDoNotEnterEnvelope(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	for _, want := range []string{
		"base.contract",
		"runtime.state",
		"tool.surface",
	} {
		if !strings.Contains(strings.Join(compiledSectionIDsForTest(compiled.Envelope.Sections), "\n"), want) {
			t.Fatalf("compiled envelope missing %q: %#v", want, compiled.Envelope.Sections)
		}
	}
	for _, forbidden := range []string{"## Completion Gate", "verification status is PARTIAL", "expected vs actual", "budget-limited synthesis"} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("model input leaked old completion rule %q:\n%s", forbidden, modelInput)
		}
	}
}

func TestResourceInspectionLongRuleDoesNotEnterEnvelope(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	for _, forbidden := range []string{
		"resource inspection",
		"concrete requested values",
		"not only health or normality",
	} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("model input leaked old resource inspection rule %q:\n%s", forbidden, modelInput)
		}
	}
}

func TestCurrentEnvironmentLongRuleDoesNotEnterEnvelope(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	for _, forbidden := range []string{
		"current host, current environment, local runtime, or selected resource",
		"do not use web_search or browse_url",
		"environment-bound tools",
	} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("model input leaked old current-environment rule %q:\n%s", forbidden, modelInput)
		}
	}
}

func TestReasoningEffortFallbackPolicy(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{ReasoningEffort: "high"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	state := compiledPromptSectionForTest(t, compiled, "runtime.state").Content
	if !strings.Contains(state, "reasoning_effort: high") {
		t.Fatalf("runtime.state missing reasoning effort:\n%s", state)
	}
	if strings.Contains(strings.ToLower(compiledEnvelopeTextForTest(compiled)), "show raw reasoning") {
		t.Fatalf("model input must not expose raw reasoning:\n%s", compiledEnvelopeTextForTest(compiled))
	}
}

func TestReasoningEffortFallbackPolicyVisibleWhenProviderUnsupported(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{ReasoningEffort: "high"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	for _, forbidden := range []string{
		"decompose the goal",
		"list assumptions",
	} {
		if strings.Contains(strings.ToLower(modelInput), forbidden) {
			t.Fatalf("model input contains old reasoning fallback/domain term %q:\n%s", forbidden, modelInput)
		}
	}
}

func extractReasoningFallbackPolicyLine(developer string) string {
	for _, line := range strings.Split(developer, "\n") {
		if strings.Contains(strings.ToLower(line), "reasoning fallback policy") {
			return line
		}
	}
	return ""
}

func compiledEnvelopeTextForTest(compiled CompiledPrompt) string {
	parts := make([]string, 0, len(compiled.Envelope.Sections)*2)
	for _, section := range compiled.Envelope.Sections {
		parts = append(parts, section.ID, section.Content)
	}
	return strings.Join(parts, "\n")
}
