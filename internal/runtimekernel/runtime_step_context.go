package runtimekernel

import (
	"fmt"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type RuntimeStepContext struct {
	Turn               RuntimeTurnContext                   `json:"turn"`
	TurnAssemblyHash   string                               `json:"turnAssemblyHash"`
	PermissionHash     string                               `json:"permissionHash"`
	CheckpointRef      string                               `json:"checkpointRef,omitempty"`
	Iteration          int                                  `json:"iteration"`
	ContextState       ContextPipelineResult                `json:"contextState"`
	Compiled           promptcompiler.CompiledPrompt        `json:"compiled"`
	ModelInput         []promptinput.ModelInputItem         `json:"modelInput"`
	ToolSurface        RuntimeToolRouterSnapshot            `json:"toolSurface"`
	ProviderRequest    modelrouter.ProviderRequestSnapshot  `json:"providerRequest"`
	PromptShadowParity promptinput.PromptShadowParityReport `json:"promptShadowParity"`
	Hash               string                               `json:"hash"`
}

func (s RuntimeStepContext) Validate() error {
	if s.Turn.SessionID == "" {
		return fmt.Errorf("turn session id is required")
	}
	if s.Turn.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if s.Iteration < 0 {
		return fmt.Errorf("iteration must be non-negative")
	}
	if s.TurnAssemblyHash == "" {
		return fmt.Errorf("turn assembly hash is required")
	}
	if s.PermissionHash == "" {
		return fmt.Errorf("permission hash is required")
	}
	if s.ToolSurface.Fingerprint == "" {
		return fmt.Errorf("tool router fingerprint is required")
	}
	if err := s.ToolSurface.Validate(); err != nil {
		return fmt.Errorf("step tool router: %w", err)
	}
	for i := range s.ModelInput {
		if err := s.ModelInput[i].Validate(); err != nil {
			return fmt.Errorf("model input[%d]: %w", i, err)
		}
	}
	if err := validateRuntimeStepProviderRequest(s); err != nil {
		return err
	}
	if promptinput.HasTypedModelInputLayers(s.ProviderRequest.Input) {
		if err := s.PromptShadowParity.Validate(); err != nil {
			return err
		}
		if !s.PromptShadowParity.Passed {
			return fmt.Errorf("prompt shadow parity gate rejected runtime step")
		}
	}
	if s.Hash == "" || s.Hash != ComputeRuntimeStepContextHash(s) {
		return fmt.Errorf("runtime step context hash mismatch")
	}
	return nil
}
