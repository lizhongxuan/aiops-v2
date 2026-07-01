package promptcompiler

import (
	"strings"
	"testing"
)

func TestBuildBaseRuntimeContractContainsOnlyThinBaseRules(t *testing.T) {
	content := buildBaseRuntimeContract("mode: execute")

	for _, want := range []string{
		"Do not fabricate tool results, system state, evidence, timelines, or external facts.",
		"Separate verified facts, inference, and unknowns.",
		"Only call current model-visible tools.",
		"Tool failure, empty output, permission denial, or timeout is not health proof.",
		"Simple tasks get concise answers; complex tasks advance by evidence.",
		"Mutations must obey runtime approval, resource scope, and post-check.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("base runtime contract missing %q:\n%s", want, content)
		}
	}

	for _, forbidden := range []string{
		"Runtime Policy",
		"single thread/turn loop",
		"recovery state",
		"compaction state",
		"Task Triage",
		"Planning",
		"Responsiveness",
		"Evidence Contract",
		"Tool Use Contract",
		"Approval Contract",
		"Final Answer Contract",
		"mode: execute",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("base runtime contract leaked old always-on content %q:\n%s", forbidden, content)
		}
	}
}

func TestCompiledEnvelopeUsesThinBaseContract(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	base := compiledPromptSectionForTest(t, compiled, "base.contract")
	if base.Content != buildBaseRuntimeContract("") {
		t.Fatalf("base.contract content mismatch:\n%s", base.Content)
	}
	for _, forbidden := range []string{
		"# Role",
		"# Behavior",
		"# Environment",
		"You are an AIOps assistant",
		"brief progress updates",
		"use tools silently",
	} {
		if strings.Contains(base.Content, forbidden) {
			t.Fatalf("base.contract leaked legacy system content %q:\n%s", forbidden, base.Content)
		}
	}
}

func TestDeveloperConstraintsAreDerivedFromThinSections(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	constraints := strings.Join(compiled.Developer.Constraints, "\n")
	for _, want := range []string{
		"Use the current runtime state, model-visible tools, user input, and registered evidence as the only sources of operational truth.",
		"Only call tools visible in the current runtime tool surface; hidden tools are unavailable.",
		"chat: default to direct answers and read-only tools; mutating operations are forbidden.",
	} {
		if !strings.Contains(constraints, want) {
			t.Fatalf("developer constraints missing thin section line %q:\n%s", want, constraints)
		}
	}

	for _, forbidden := range []string{
		"Responsiveness: before tool calls",
		"Responsiveness: after prior tool work",
		"When public web or documentation sources support analysis",
		"When public current data needs higher precision",
		"Do not attempt any mutation operations.",
	} {
		if strings.Contains(constraints, forbidden) {
			t.Fatalf("developer constraints leaked legacy standalone rule %q:\n%s", forbidden, constraints)
		}
	}
}
