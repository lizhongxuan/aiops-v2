package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestPromptAssemblyBoundarySourcesAreTraceOnly(t *testing.T) {
	req := buildModelInputTraceRequest(RuntimeTraceDebugRequest{
		Compiled: promptcompiler.CompiledPrompt{
			Envelope: promptcompiler.PromptEnvelope{Sections: []promptcompiler.PromptCompiledSection{{
				ID:        "runtime.state",
				Layer:     promptcompiler.PromptSectionKindDynamic,
				Role:      "developer",
				Content:   "runtime state",
				Stability: promptcompiler.PromptSectionKindDynamic,
				Source:    "runtimekernel",
			}}},
		},
		AssemblySource:                "runtimekernel.test",
		PromptCompilerSource:          "promptcompiler.Compiler",
		ToolSurfaceSource:             "runtimekernel.toolSurface",
		AdapterName:                   "eino",
		ToolSurfaceFingerprint:        "surface-boundary-test",
		ToolSurfacePolicySnapshotHash: "policy-boundary-test",
	})

	trace := req.PromptInputTrace
	if trace.AssemblySource != "runtimekernel.test" ||
		trace.PromptCompilerSource != "promptcompiler.Compiler" ||
		trace.ToolSurfaceSource != "runtimekernel.toolSurface" ||
		trace.AdapterName != "eino" {
		t.Fatalf("boundary trace sources = %#v", trace)
	}
	for _, item := range req.ModelInput {
		if strings.Contains(item.Content, "runtimekernel.test") ||
			strings.Contains(item.Content, "runtimekernel.toolSurface") {
			t.Fatalf("boundary trace source leaked into model input item: %#v", item)
		}
	}
}

func TestPromptSectionTraceHasStableBoundaryIdentifiers(t *testing.T) {
	compiled := promptcompiler.CompiledPrompt{
		Envelope: promptcompiler.PromptEnvelope{Sections: []promptcompiler.PromptCompiledSection{{
			ID:        "base.contract",
			Layer:     promptcompiler.PromptSectionKindStable,
			Role:      "system",
			Content:   "base contract",
			Stability: promptcompiler.PromptSectionKindStable,
			Source:    "promptcompiler.base",
		}, {
			ID:        "tool.surface",
			Layer:     promptcompiler.PromptSectionKindStable,
			Role:      "developer",
			Content:   "tool surface",
			Stability: promptcompiler.PromptSectionKindStable,
			Source:    "tooling.assembled",
		}, {
			ID:        "runtime.state",
			Layer:     promptcompiler.PromptSectionKindDynamic,
			Role:      "developer",
			Content:   "runtime state",
			Stability: promptcompiler.PromptSectionKindDynamic,
			Source:    "runtimekernel.state",
		}}},
	}

	sections := promptcompiler.BuildPromptSectionTrace(compiled)
	if len(sections) != 3 {
		t.Fatalf("len(sections) = %d, want 3", len(sections))
	}
	for _, section := range sections {
		if strings.TrimSpace(section.ID) == "" ||
			strings.TrimSpace(section.Source) == "" ||
			!strings.HasPrefix(section.Hash, "sha256:") {
			t.Fatalf("section trace missing stable boundary fields: %#v", section)
		}
	}
}
