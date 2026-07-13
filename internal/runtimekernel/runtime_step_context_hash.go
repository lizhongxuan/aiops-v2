package runtimekernel

import (
	"bytes"
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
	if agentassembly.StableHash("runtime-step.model-input", input.ModelInput) != agentassembly.StableHash("runtime-step.model-input", input.ProviderRequest.Input) {
		return RuntimeStepContext{}, fmt.Errorf("step and provider model input conflict")
	}
	input.Hash = ""
	frozen, err := cloneRuntimeStepContext(input)
	if err != nil {
		return RuntimeStepContext{}, err
	}
	providerInput, err := cloneRuntimeStepModelInput(frozen.ProviderRequest.Input)
	if err != nil {
		return RuntimeStepContext{}, err
	}
	modelInputMirror, err := cloneRuntimeStepModelInput(providerInput)
	if err != nil {
		return RuntimeStepContext{}, err
	}
	frozen.ProviderRequest.Input = providerInput
	frozen.ModelInput = modelInputMirror
	audit, err := modelrouter.ProviderMessageAuditFromModelInputItems(providerInput)
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
		"turnAssemblyHash":   step.TurnAssemblyHash,
		"permissionHash":     step.PermissionHash,
		"checkpointRef":      step.CheckpointRef,
		"iteration":          step.Iteration,
		"turn":               step.Turn,
		"contextState":       runtimeStepControlContext(step.ContextState),
		"compiled":           step.Compiled,
		"promptShadowParity": step.PromptShadowParity,
		"modelInputHash":     step.ProviderRequest.ModelInputHash,
		"toolRouter":         step.ToolSurface,
		"provider": map[string]any{
			"providerMessagesHash":  step.ProviderRequest.ProviderMessagesHash,
			"requestPropertiesHash": step.ProviderRequest.RequestPropertiesHash,
			"promptCacheKey":        step.ProviderRequest.PromptCacheKey,
			"promptFingerprint":     step.ProviderRequest.PromptFingerprint,
			"clientMetadata":        step.ProviderRequest.ClientMetadata,
		},
	})
}

// runtimeStepControlContext removes wall-clock observations before deriving a
// control hash. Time remains available in the typed context for diagnostics,
// but replaying identical facts at a later instant must not create a new Step.
func runtimeStepControlContext(input ContextPipelineResult) any {
	input.Messages = append([]Message(nil), input.Messages...)
	for index := range input.Messages {
		input.Messages[index].ID = fmt.Sprintf("<message:%d>", index+1)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return input
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return input
	}
	return normalizeRuntimeStepControlValue("", value)
}

func normalizeRuntimeStepControlValue(key string, value any) any {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "createdat", "updatedat", "startedat", "finishedat", "completedat", "requestedat", "resolvedat", "timestamp", "expiresat":
		if value != nil && fmt.Sprint(value) != "" {
			return "<wall-clock>"
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, child := range typed {
			out[childKey] = normalizeRuntimeStepControlValue(childKey, child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = normalizeRuntimeStepControlValue(key, child)
		}
		return out
	default:
		return typed
	}
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
	if promptinput.HasTypedModelInputLayers(step.ProviderRequest.Input) && step.Compiled.Fingerprint != step.ProviderRequest.PromptFingerprint {
		return fmt.Errorf("compiled and provider prompt fingerprints do not match")
	}
	recomputed := step.ProviderRequest
	recomputed.ComputeHashes()
	if recomputed.ModelInputHash != step.ProviderRequest.ModelInputHash ||
		recomputed.RequestPropertiesHash != step.ProviderRequest.RequestPropertiesHash ||
		recomputed.PromptCacheKey != step.ProviderRequest.PromptCacheKey ||
		recomputed.PromptFingerprint != step.ProviderRequest.PromptFingerprint {
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
