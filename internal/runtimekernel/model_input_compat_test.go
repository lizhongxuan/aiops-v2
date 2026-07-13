package runtimekernel

import (
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

func buildModelInput(history []Message, compiled promptcompiler.CompiledPrompt) ([]promptinput.ModelInputItem, error) {
	result, err := buildPromptInput(history, compiled)
	if err != nil {
		return nil, err
	}
	return append([]promptinput.ModelInputItem(nil), result.Items...), nil
}

func buildPromptInput(history []Message, compiled promptcompiler.CompiledPrompt) (promptinput.BuildResult, error) {
	return buildPromptInputWithContextGovernance(history, compiled, nil)
}

func buildPromptInputWithContextGovernance(history []Message, compiled promptcompiler.CompiledPrompt, governance []ContextGovernanceEvent) (promptinput.BuildResult, error) {
	ctx := promptcompiler.CompileContext{SessionType: string(SessionTypeHost), Mode: string(ModeChat)}
	if compiled.EnvelopeV2.SchemaVersion == "" {
		compiled.EnvelopeV2 = promptcompiler.BuildPromptEnvelopeV2(compiled, ctx)
	}
	if len(compiled.Envelope.Sections) == 0 {
		compiled.Envelope = promptcompiler.BuildPromptEnvelope(compiled, ctx)
	}
	return buildRuntimePromptInputV2WithContextGovernance(history, compiled, governance, 0, nil)
}
