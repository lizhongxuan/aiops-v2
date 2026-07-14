package runtimekernel

import (
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptinput"
)

func latestRuntimePromptFingerprint(snapshot *TurnSnapshot) map[string]string {
	if snapshot == nil {
		return nil
	}
	for index := len(snapshot.Iterations) - 1; index >= 0; index-- {
		if fingerprint := cloneStringMap(snapshot.Iterations[index].PromptFingerprint); len(fingerprint) > 0 {
			return fingerprint
		}
	}
	return nil
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
		SessionID:                 step.Turn.SessionID,
		TurnID:                    step.Turn.TurnID,
		Iteration:                 step.Iteration,
		Metadata:                  traceReq.Metadata,
		VisibleTools:              traceReq.VisibleTools,
		PromptFingerprint:         traceReq.PromptFingerprint,
		PreviousPromptFingerprint: cloneStringMap(req.PreviousPromptFingerprint),
		TurnContext:               step.Turn,
		StepContextHash:           step.Hash,
		HarnessTurn:               harnessTurn,
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
			PromptInputTrace:   traceReq.PromptInputTrace,
			PromptInputDiff:    traceReq.PromptInputDiff,
			DiagnosticTrace:    traceReq.DiagnosticTrace,
			StepReference:      stepReference,
			PromptShadowParity: step.PromptShadowParity,
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
	Hash                   string                               `json:"hash"`
	TurnAssemblyHash       string                               `json:"turnAssemblyHash"`
	PermissionHash         string                               `json:"permissionHash"`
	CheckpointRef          string                               `json:"checkpointRef,omitempty"`
	Iteration              int                                  `json:"iteration"`
	ToolSurfaceFingerprint string                               `json:"toolSurfaceFingerprint"`
	ToolPolicyHash         string                               `json:"toolPolicyHash,omitempty"`
	ProviderRequest        modeltrace.ProviderRequestTrace      `json:"providerRequest"`
	PromptInputTrace       any                                  `json:"promptInputTrace,omitempty"`
	PromptInputDiff        any                                  `json:"promptInputDiff,omitempty"`
	DiagnosticTrace        any                                  `json:"diagnosticTrace,omitempty"`
	StepReference          *StepReference                       `json:"stepReference,omitempty"`
	PromptShadowParity     promptinput.PromptShadowParityReport `json:"promptShadowParity"`
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
