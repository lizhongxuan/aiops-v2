package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
	"github.com/cloudwego/eino/schema"
)

func (k *RuntimeKernel) buildRuntimeStepContext(
	req TurnRequest,
	session *SessionState,
	agentKind modelrouter.AgentKind,
	iteration int,
	contextState ContextPipelineResult,
	contextMessages []Message,
	compiled promptcompiler.CompiledPrompt,
	toolSurface RuntimeToolRouterSnapshot,
	thresholds ContextBudgetThresholds,
	modelName string,
) (RuntimeStepContext, promptinput.BuildResult, []*schema.Message, error) {
	if session == nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, nil, fmt.Errorf("session is required")
	}
	turnReq := req
	turnReq.SessionID = firstNonBlankRuntimeString(turnReq.SessionID, session.ID)
	turnReq.TurnID = firstNonBlankRuntimeString(turnReq.TurnID, session.ActiveTurn.TurnID)
	if turnReq.TurnID == "" && session.CurrentTurn != nil {
		turnReq.TurnID = session.CurrentTurn.ID
	}
	turnReq.HostID = firstNonBlankRuntimeString(turnReq.HostID, session.HostID)

	modelCaps := modelrouter.ModelCapabilities{
		Provider:         string(agentKind),
		Model:            strings.TrimSpace(modelName),
		MaxContextTokens: thresholds.MaxContextTokens,
		MaxOutputTokens:  thresholds.ReservedOutputTokens,
	}
	if k != nil && k.modelRouter != nil {
		modelCaps = k.modelRouter.ResolveModelCapabilities(agentKind, modelrouter.ProviderConfig{})
		if modelCaps.Provider == "" {
			modelCaps.Provider = string(agentKind)
		}
		if modelCaps.Model == "" {
			modelCaps.Model = strings.TrimSpace(modelName)
		}
	}
	turnCtx, err := BuildRuntimeTurnContext(turnReq, session, RuntimeTurnContextOptions{
		Model: modelCaps,
		ContextBudget: RuntimeContextBudgetSnapshot{
			MaxTokens:    thresholds.MaxContextTokens,
			TargetTokens: thresholds.EffectiveContextWindow,
		},
		ToolPolicyHash: toolSurface.PolicyHash,
		Lineage: RuntimeLineageSnapshot{
			AgentKind: string(agentKind),
		},
	})
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, nil, err
	}
	promptBuild, err := buildPromptInputWithContextGovernance(contextMessages, compiled, append([]ContextGovernanceEvent(nil), session.ContextGovernanceEvents...))
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, nil, err
	}
	modelInput, audit, err := modelrouter.ModelInputItemsToEinoMessages(promptBuild.Items)
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, nil, err
	}
	providerReq := modelrouter.ProviderRequestSnapshot{
		Provider:        firstNonBlankRuntimeString(modelCaps.Provider, string(agentKind)),
		Model:           firstNonBlankRuntimeString(modelCaps.Model, strings.TrimSpace(modelName)),
		Input:           promptBuild.Items,
		Tools:           providerToolSpecsFromRuntimeToolSurface(toolSurface),
		ReasoningEffort: firstMetadataValue(turnReq.Metadata, "reasoningEffort", "reasoning_effort"),
		ClientMetadata: map[string]string{
			"sessionId":       turnCtx.SessionID,
			"turnId":          turnCtx.TurnID,
			"clientTurnId":    turnCtx.ClientTurnID,
			"clientMessageId": turnCtx.ClientMessageID,
		},
		ProviderMessagesHash: audit.ProviderMessagesHash,
		MessageAudit:         &audit,
	}
	providerReq.ComputeHashes()
	step := RuntimeStepContext{
		Turn:            turnCtx,
		Iteration:       iteration,
		ContextState:    contextState,
		Compiled:        compiled,
		ModelInput:      promptBuild.Items,
		ToolSurface:     toolSurface,
		ProviderRequest: providerReq,
	}
	if err := step.Validate(); err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, nil, fmt.Errorf("runtime step context: %w", err)
	}
	return step, promptBuild, modelInput, nil
}

func runtimeToolRouterSnapshotFromCompile(tools []promptcompiler.Tool, visibleToolNames []string, fingerprint string, policy tooling.ToolSurfacePolicySnapshot) RuntimeToolRouterSnapshot {
	return RuntimeToolRouterSnapshotFromPolicy(toolNames(tools), policy, visibleToolNames, nil, fingerprint)
}

func providerToolSpecsFromRuntimeToolSurface(surface RuntimeToolRouterSnapshot) []modelrouter.ProviderToolSpec {
	names := surface.ModelVisibleTools
	if len(names) == 0 {
		names = surface.DispatchableTools
	}
	out := make([]modelrouter.ProviderToolSpec, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, modelrouter.ProviderToolSpec{Name: name, Hash: surface.Fingerprint})
	}
	return out
}

func writeRuntimeStepTrace(step RuntimeStepContext, req ModelInputDebugTraceRequest) (string, error) {
	if !modeltrace.Enabled() {
		return "", nil
	}
	req.SessionID = step.Turn.SessionID
	req.TurnID = step.Turn.TurnID
	req.Iteration = step.Iteration
	req.Compiled = step.Compiled
	if len(req.VisibleTools) == 0 {
		req.VisibleTools = append([]string(nil), step.ToolSurface.ModelVisibleTools...)
	}
	traceReq := buildModelInputTraceRequest(req)
	root := modeltrace.TraceDocumentV2Directory("", step.Turn.SessionID, step.Turn.TurnID)
	rawRef, err := modeltrace.WriteRawPayloadRef(root, "provider-request", "provider_request", step.ProviderRequest)
	if err != nil {
		return "", err
	}
	return modeltrace.WriteTraceDocumentV2(root, modeltrace.TraceDocumentV2{
		SessionID:         step.Turn.SessionID,
		TurnID:            step.Turn.TurnID,
		Iteration:         step.Iteration,
		Metadata:          traceReq.Metadata,
		VisibleTools:      traceReq.VisibleTools,
		PromptFingerprint: traceReq.PromptFingerprint,
		TurnContext:       step.Turn,
		StepContext: runtimeStepTraceDocumentV2{
			RuntimeStepContext: step,
			PromptInputTrace:   traceReq.PromptInputTrace,
			PromptInputDiff:    traceReq.PromptInputDiff,
			DiagnosticTrace:    traceReq.DiagnosticTrace,
			ProviderMessages:   traceReq.ModelInput,
		},
		ProviderRequest: modeltrace.ProviderRequestTrace{
			ModelInputHash:        step.ProviderRequest.ModelInputHash,
			ProviderMessagesHash:  step.ProviderRequest.ProviderMessagesHash,
			RequestPropertiesHash: step.ProviderRequest.RequestPropertiesHash,
			PromptCacheKey:        step.ProviderRequest.PromptCacheKey,
		},
		ToolSurface:        step.ToolSurface,
		Prompt:             traceReq.Prompt,
		ModelInput:         traceReq.ModelInput,
		PromptInputTrace:   traceReq.PromptInputTrace,
		PromptInputDiff:    traceReq.PromptInputDiff,
		DiagnosticTrace:    traceReq.DiagnosticTrace,
		FinalEvidenceState: traceReq.FinalEvidenceState,
		RawPayloadRefs:     []modeltrace.RawPayloadRef{rawRef},
	})
}

type runtimeStepTraceDocumentV2 struct {
	RuntimeStepContext
	PromptInputTrace any               `json:"promptInputTrace,omitempty"`
	PromptInputDiff  any               `json:"promptInputDiff,omitempty"`
	DiagnosticTrace  any               `json:"diagnosticTrace,omitempty"`
	ProviderMessages []*schema.Message `json:"providerMessages,omitempty"`
}
