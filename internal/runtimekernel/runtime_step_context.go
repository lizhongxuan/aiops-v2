package runtimekernel

import (
	"fmt"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type RuntimeStepContext struct {
	Turn            RuntimeTurnContext                  `json:"turn"`
	Iteration       int                                 `json:"iteration"`
	ContextState    ContextPipelineResult               `json:"contextState"`
	Compiled        promptcompiler.CompiledPrompt       `json:"compiled"`
	ModelInput      []promptinput.ModelInputItem        `json:"modelInput"`
	ToolSurface     RuntimeToolRouterSnapshot           `json:"toolSurface"`
	ProviderRequest modelrouter.ProviderRequestSnapshot `json:"providerRequest"`
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
	for i := range s.ModelInput {
		if err := s.ModelInput[i].Validate(); err != nil {
			return fmt.Errorf("model input[%d]: %w", i, err)
		}
	}
	return nil
}
