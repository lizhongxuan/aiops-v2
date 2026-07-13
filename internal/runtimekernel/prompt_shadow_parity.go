package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func buildRuntimePromptShadowParity(history []Message, compiled promptcompiler.CompiledPrompt, v2Items []promptinput.ModelInputItem, iteration int, cause *StepRevisionCause, toolSurface RuntimeToolRouterSnapshot, providerTools []modelrouter.ProviderToolSpec) (promptinput.PromptShadowParityReport, error) {
	promptHistory, _ := promptInputMessagesFromRuntimeWithContextDedupe(history)
	promptHistory = promptHistoryWithEffectiveUsers(promptHistory)
	kind, currentUser, _, err := runtimePromptCurrentInput(promptHistory, iteration, cause)
	if err != nil {
		return promptinput.PromptShadowParityReport{}, err
	}
	legacyTools := make([]string, 0, len(toolSurface.ModelVisibleTools))
	for _, name := range toolSurface.ModelVisibleTools {
		if name = strings.TrimSpace(name); name != "" {
			legacyTools = append(legacyTools, tooling.ProviderSafeToolName(name))
		}
	}
	v2Tools := make([]string, 0, len(providerTools))
	for _, tool := range providerTools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			v2Tools = append(v2Tools, name)
		}
	}
	report, err := promptinput.BuildPromptShadowParity(promptinput.PromptShadowParityInput{
		LegacyEnvelope:   compiled.Envelope,
		LegacyHistory:    promptHistory,
		CurrentInputKind: kind,
		CurrentUserInput: currentUser,
		ContinuationKind: runtimePromptShadowContinuationKind(kind, cause),
		LegacyToolNames:  legacyTools,
		V2ToolNames:      v2Tools,
		LegacyPolicyHash: toolSurface.PolicyHash,
		V2PolicyHash:     toolSurface.PolicyHash,
		V2Items:          v2Items,
	})
	if err != nil {
		return promptinput.PromptShadowParityReport{}, err
	}
	if err := report.Validate(); err != nil {
		return promptinput.PromptShadowParityReport{}, err
	}
	if !report.Passed {
		return promptinput.PromptShadowParityReport{}, fmt.Errorf("prompt shadow parity gate: %s", strings.Join(report.GateViolations, ","))
	}
	return report, nil
}

func runtimePromptShadowContinuationKind(kind promptinput.CurrentInputKind, cause *StepRevisionCause) string {
	if kind != promptinput.CurrentInputKindContinuation {
		return string(kind)
	}
	if cause == nil {
		return "after_tool"
	}
	switch cause.Kind {
	case StepRevisionKindApprovalResumed:
		return "after_approval"
	case StepRevisionKindModelRetryResumed:
		return "after_retry"
	default:
		return firstNonBlankRuntimeString(strings.TrimSpace(cause.Kind), "after_tool")
	}
}
