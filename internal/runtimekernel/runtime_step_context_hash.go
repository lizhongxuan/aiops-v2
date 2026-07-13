package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

type RuntimeStepControlFacts struct {
	TurnAssemblyHash string
	PermissionHash   string
	CheckpointRef    string
}

func runtimeStepPermissionHash(assembly *agentassembly.TurnAssembly, policy tooling.ToolSurfacePolicySnapshot) string {
	if assembly == nil {
		return ""
	}
	return agentassembly.StableHash("runtime-step.permission", map[string]any{
		"turnAssemblyHash":      assembly.Hash,
		"permissionProfile":     assembly.PermissionProfile,
		"approvalPolicy":        policy.ApprovalPolicy,
		"permissionPolicyHash":  policy.PermissionHash,
		"toolSurfacePolicyHash": policy.Hash,
	})
}

func FreezeRuntimeStepContext(input RuntimeStepContext) (RuntimeStepContext, error) {
	input.Hash = ""
	frozen, err := cloneRuntimeStepContext(input)
	if err != nil {
		return RuntimeStepContext{}, err
	}
	modelInput, err := cloneRuntimeStepModelInput(frozen.ModelInput)
	if err != nil {
		return RuntimeStepContext{}, err
	}
	frozen.ProviderRequest.Input = modelInput
	audit, err := modelrouter.ProviderMessageAuditFromModelInputItems(modelInput)
	if err != nil {
		return RuntimeStepContext{}, fmt.Errorf("provider message audit: %w", err)
	}
	frozen.ProviderRequest.MessageAudit = &audit
	frozen.ProviderRequest.ProviderMessagesHash = audit.ProviderMessagesHash
	frozen.ProviderRequest.ComputeHashes()
	frozen.Hash = ComputeRuntimeStepContextHash(frozen)
	if err := frozen.Validate(); err != nil {
		return RuntimeStepContext{}, err
	}
	return frozen, nil
}

func ComputeRuntimeStepContextHash(step RuntimeStepContext) string {
	return agentassembly.StableHash("runtime-step-context", map[string]any{
		"turnAssemblyHash": step.TurnAssemblyHash,
		"permissionHash":   step.PermissionHash,
		"checkpointRef":    step.CheckpointRef,
		"iteration":        step.Iteration,
		"turn":             step.Turn,
		"contextState":     step.ContextState,
		"compiled":         step.Compiled,
		"modelInputHash":   step.ProviderRequest.ModelInputHash,
		"toolRouter":       step.ToolSurface,
		"provider": map[string]any{
			"providerMessagesHash":  step.ProviderRequest.ProviderMessagesHash,
			"requestPropertiesHash": step.ProviderRequest.RequestPropertiesHash,
			"promptCacheKey":        step.ProviderRequest.PromptCacheKey,
		},
	})
}

func (s RuntimeStepContext) ValidatedProviderRequest() (modelrouter.ProviderRequestSnapshot, error) {
	if err := s.Validate(); err != nil {
		return modelrouter.ProviderRequestSnapshot{}, err
	}
	frozen, err := cloneRuntimeStepContext(s)
	if err != nil {
		return modelrouter.ProviderRequestSnapshot{}, err
	}
	return frozen.ProviderRequest, nil
}

func validateRuntimeStepProviderRequest(step RuntimeStepContext) error {
	if strings.TrimSpace(step.ProviderRequest.ModelInputHash) == "" ||
		strings.TrimSpace(step.ProviderRequest.ProviderMessagesHash) == "" ||
		strings.TrimSpace(step.ProviderRequest.RequestPropertiesHash) == "" ||
		strings.TrimSpace(step.ProviderRequest.PromptCacheKey) == "" {
		return fmt.Errorf("provider request hashes are required")
	}
	fromStep := modelrouter.ProviderRequestSnapshot{Input: step.ModelInput}
	fromStep.ComputeHashes()
	if fromStep.ModelInputHash != step.ProviderRequest.ModelInputHash {
		return fmt.Errorf("provider request model input hash does not match step model input")
	}
	recomputed := step.ProviderRequest
	recomputed.ComputeHashes()
	if recomputed.ModelInputHash != step.ProviderRequest.ModelInputHash ||
		recomputed.RequestPropertiesHash != step.ProviderRequest.RequestPropertiesHash ||
		recomputed.PromptCacheKey != step.ProviderRequest.PromptCacheKey {
		return fmt.Errorf("provider request hash mismatch")
	}
	audit, err := modelrouter.ProviderMessageAuditFromModelInputItems(step.ProviderRequest.Input)
	if err != nil {
		return fmt.Errorf("provider message audit: %w", err)
	}
	if step.ProviderRequest.MessageAudit == nil ||
		audit.ProviderMessagesHash != step.ProviderRequest.ProviderMessagesHash ||
		audit.ProviderMessagesHash != step.ProviderRequest.MessageAudit.ProviderMessagesHash ||
		agentassembly.StableHash("provider-message-audit", audit.Items) != agentassembly.StableHash("provider-message-audit", step.ProviderRequest.MessageAudit.Items) {
		return fmt.Errorf("provider message audit mismatch")
	}
	return nil
}

func cloneRuntimeStepContext(input RuntimeStepContext) (RuntimeStepContext, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return RuntimeStepContext{}, fmt.Errorf("marshal runtime step context: %w", err)
	}
	var out RuntimeStepContext
	if err := json.Unmarshal(data, &out); err != nil {
		return RuntimeStepContext{}, fmt.Errorf("unmarshal runtime step context: %w", err)
	}
	return out, nil
}

func cloneRuntimeStepModelInput(input []promptinput.ModelInputItem) ([]promptinput.ModelInputItem, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime step model input: %w", err)
	}
	var out []promptinput.ModelInputItem
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal runtime step model input: %w", err)
	}
	return out, nil
}
