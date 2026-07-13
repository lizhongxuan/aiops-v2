package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
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
	control RuntimeStepControlFacts,
	thresholds ContextBudgetThresholds,
	modelName string,
	assemblies ...*agentassembly.TurnAssembly,
) (RuntimeStepContext, promptinput.BuildResult, error) {
	if session == nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, fmt.Errorf("session is required")
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
		return RuntimeStepContext{}, promptinput.BuildResult{}, err
	}
	if len(assemblies) > 0 && assemblies[0] != nil {
		assembly := assemblies[0]
		if err := assembly.Validate(); err != nil {
			return RuntimeStepContext{}, promptinput.BuildResult{}, fmt.Errorf("turn assembly: %w", err)
		}
		if assembly.Hash != control.TurnAssemblyHash {
			return RuntimeStepContext{}, promptinput.BuildResult{}, fmt.Errorf("turn assembly hash drift")
		}
		turnCtx.AdmissionFacts = assembly.AdmissionFacts
		turnCtx.AdmissionError = ""
		turnCtx.Profile = assembly.AdmissionFacts.Profile
		turnCtx.Route.Profile = assembly.AdmissionFacts.Profile
	}
	promptBuild, err := buildRuntimePromptInputV2WithContextGovernance(
		contextMessages,
		compiled,
		append([]ContextGovernanceEvent(nil), session.ContextGovernanceEvents...),
		iteration,
		currentPendingStepCause(session),
	)
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, err
	}
	audit, err := modelrouter.ProviderMessageAuditFromModelInputItems(promptBuild.Items)
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, err
	}
	providerReq := modelrouter.ProviderRequestSnapshot{
		Provider:          firstNonBlankRuntimeString(modelCaps.Provider, string(agentKind)),
		Model:             firstNonBlankRuntimeString(modelCaps.Model, strings.TrimSpace(modelName)),
		Input:             promptBuild.Items,
		PromptFingerprint: compiled.Fingerprint,
		Tools:             providerToolSpecsFromRuntimeToolSurface(toolSurface),
		ReasoningEffort:   firstMetadataValue(turnReq.Metadata, "reasoningEffort", "reasoning_effort"),
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
	compiled.Fingerprint = providerReq.PromptFingerprint
	promptShadowParity, err := buildRuntimePromptShadowParity(contextMessages, compiled, providerReq.Input, iteration, currentPendingStepCause(session), toolSurface, providerReq.Tools)
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, err
	}
	step := RuntimeStepContext{
		Turn:               turnCtx,
		TurnAssemblyHash:   control.TurnAssemblyHash,
		PermissionHash:     control.PermissionHash,
		CheckpointRef:      control.CheckpointRef,
		Iteration:          iteration,
		ContextState:       contextState,
		Compiled:           compiled,
		ModelInput:         promptBuild.Items,
		ToolSurface:        toolSurface,
		ProviderRequest:    providerReq,
		PromptShadowParity: promptShadowParity,
	}
	step, err = FreezeRuntimeStepContext(step)
	if err != nil {
		return RuntimeStepContext{}, promptinput.BuildResult{}, fmt.Errorf("runtime step context: %w", err)
	}
	return step, promptBuild, nil
}

func currentPendingStepCause(session *SessionState) *StepRevisionCause {
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.PendingStepCause == nil {
		return nil
	}
	cause := *session.CurrentTurn.PendingStepCause
	return &cause
}

func runtimeToolRouterSnapshotFromCompile(tools []promptcompiler.Tool, visibleToolNames []string, fingerprint string, policy tooling.ToolSurfacePolicySnapshot) RuntimeToolRouterSnapshot {
	return RuntimeToolRouterSnapshotFromPolicy(toolNames(tools), policy, visibleToolNames, append([]string{}, visibleToolNames...), fingerprint)
}

func providerToolSpecsFromStepToolRouter(surface StepToolRouter) []modelrouter.ProviderToolSpec {
	names := surface.ModelVisibleTools
	out := make([]modelrouter.ProviderToolSpec, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, modelrouter.ProviderToolSpec{Name: tooling.ProviderSafeToolName(name), Hash: surface.Fingerprint})
	}
	return out
}

func providerToolSpecsFromRuntimeToolSurface(surface RuntimeToolRouterSnapshot) []modelrouter.ProviderToolSpec {
	return providerToolSpecsFromStepToolRouter(surface)
}

func writeRuntimeStepTrace(traceConfig modeltrace.Config, step RuntimeStepContext, req RuntimeTraceDebugRequest, references ...*StepReference) (string, error) {
	if !traceConfig.Enabled {
		return "", nil
	}
	req.SessionID = step.Turn.SessionID
	req.TurnID = step.Turn.TurnID
	req.Iteration = step.Iteration
	req.Compiled = step.Compiled
	req.ModelInput = append([]promptinput.ModelInputItem(nil), step.ProviderRequest.Input...)
	if len(req.VisibleTools) == 0 {
		req.VisibleTools = append([]string(nil), step.ToolSurface.ModelVisibleTools...)
	}
	traceReq := buildModelInputTraceRequest(req)
	finalEvidenceState, _ := traceReq.FinalEvidenceState.(FinalEvidenceState)
	var stepReference *StepReference
	if len(references) > 0 && references[0] != nil {
		cloned := cloneStepReference(*references[0])
		stepReference = &cloned
	}
	harnessTurn := BuildHarnessTurnTrace(nil, step, FinalEvidenceVerification{
		Action:     FinalEvidenceActionAllow,
		Confidence: finalEvidenceState.Confidence,
		State:      finalEvidenceState,
	})
	root := modeltrace.TraceDocumentV2Directory(traceConfig.RootDir, step.Turn.SessionID, step.Turn.TurnID)
	rawRef, err := modeltrace.WriteRawPayloadRef(root, "provider-request", "provider_request_hashes", runtimeProviderRequestTraceDocumentV2{
		Provider:              step.ProviderRequest.Provider,
		Model:                 step.ProviderRequest.Model,
		ModelInputHash:        step.ProviderRequest.ModelInputHash,
		ProviderMessagesHash:  step.ProviderRequest.ProviderMessagesHash,
		RequestPropertiesHash: step.ProviderRequest.RequestPropertiesHash,
		PromptCacheKey:        step.ProviderRequest.PromptCacheKey,
		ToolCount:             len(step.ProviderRequest.Tools),
	})
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
		StepContextHash:   step.Hash,
		HarnessTurn:       harnessTurn,
		StepContext: runtimeStepTraceDocumentV2{
			Hash:                   step.Hash,
			TurnAssemblyHash:       step.TurnAssemblyHash,
			PermissionHash:         step.PermissionHash,
			CheckpointRef:          step.CheckpointRef,
			Iteration:              step.Iteration,
			ToolSurfaceFingerprint: step.ToolSurface.Fingerprint,
			ToolPolicyHash:         step.ToolSurface.PolicyHash,
			ProviderRequest: modeltrace.ProviderRequestTrace{
				ModelInputHash:        step.ProviderRequest.ModelInputHash,
				ProviderMessagesHash:  step.ProviderRequest.ProviderMessagesHash,
				RequestPropertiesHash: step.ProviderRequest.RequestPropertiesHash,
				PromptCacheKey:        step.ProviderRequest.PromptCacheKey,
			},
			PromptInputTrace: traceReq.PromptInputTrace,
			PromptInputDiff:  traceReq.PromptInputDiff,
			DiagnosticTrace:  traceReq.DiagnosticTrace,
			StepReference:    stepReference,
		},
		ProviderRequest: modeltrace.ProviderRequestTrace{
			ModelInputHash:        step.ProviderRequest.ModelInputHash,
			ProviderMessagesHash:  step.ProviderRequest.ProviderMessagesHash,
			RequestPropertiesHash: step.ProviderRequest.RequestPropertiesHash,
			PromptCacheKey:        step.ProviderRequest.PromptCacheKey,
		},
		ToolSurface:                 step.ToolSurface,
		TurnAssembly:                req.TurnAssembly,
		LegacyAgentAssemblySnapshot: req.LegacyAgentAssemblySnapshot,
		TurnAssemblyShadow:          req.TurnAssemblyShadow,
		Prompt:                      traceReq.Prompt,
		ModelInput:                  traceReq.ModelInput,
		SpecialInputWorldState:      traceReq.SpecialInputWorldState,
		PromptInputTrace:            traceReq.PromptInputTrace,
		PromptInputDiff:             traceReq.PromptInputDiff,
		DiagnosticTrace:             traceReq.DiagnosticTrace,
		FinalEvidenceState:          traceReq.FinalEvidenceState,
		RawPayloadRefs:              []modeltrace.RawPayloadRef{rawRef},
	})
}

type runtimeStepTraceDocumentV2 struct {
	Hash                   string                          `json:"hash"`
	TurnAssemblyHash       string                          `json:"turnAssemblyHash"`
	PermissionHash         string                          `json:"permissionHash"`
	CheckpointRef          string                          `json:"checkpointRef,omitempty"`
	Iteration              int                             `json:"iteration"`
	ToolSurfaceFingerprint string                          `json:"toolSurfaceFingerprint"`
	ToolPolicyHash         string                          `json:"toolPolicyHash,omitempty"`
	ProviderRequest        modeltrace.ProviderRequestTrace `json:"providerRequest"`
	PromptInputTrace       any                             `json:"promptInputTrace,omitempty"`
	PromptInputDiff        any                             `json:"promptInputDiff,omitempty"`
	DiagnosticTrace        any                             `json:"diagnosticTrace,omitempty"`
	StepReference          *StepReference                  `json:"stepReference,omitempty"`
}

type runtimeProviderRequestTraceDocumentV2 struct {
	Provider              string `json:"provider,omitempty"`
	Model                 string `json:"model,omitempty"`
	ModelInputHash        string `json:"modelInputHash"`
	ProviderMessagesHash  string `json:"providerMessagesHash"`
	RequestPropertiesHash string `json:"requestPropertiesHash"`
	PromptCacheKey        string `json:"promptCacheKey"`
	ToolCount             int    `json:"toolCount"`
}
