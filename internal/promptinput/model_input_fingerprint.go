package promptinput

import (
	"strings"

	"aiops-v2/internal/promptcompiler"
)

func BuildModelInputPromptFingerprint(items []ModelInputItem) (promptcompiler.PromptFingerprint, error) {
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return promptcompiler.PromptFingerprint{}, err
		}
	}
	modelInputHash := StableModelInputHash(items)
	if !HasTypedModelInputLayers(items) {
		return promptcompiler.BuildPromptFingerprintFromLayerEntries(nil, modelInputHash), nil
	}
	if err := ValidateModelInputCausalOrder(items); err != nil {
		return promptcompiler.PromptFingerprint{}, err
	}
	if err := ValidateModelInputLogicalOrder(items, true); err != nil {
		return promptcompiler.PromptFingerprint{}, err
	}
	entries := make([]promptcompiler.PromptLayerFingerprintEntry, 0, len(items))
	for _, item := range items {
		layer := promptcompiler.PromptLogicalLayer(strings.TrimSpace(item.Source.Layer))
		payloadHash := ""
		if item.ReasoningContent != "" || len(item.ContentParts) > 0 || item.Name != "" {
			payloadHash = promptcompiler.HashPromptLayerPayload(map[string]any{
				"reasoningContent": item.ReasoningContent,
				"contentParts":     item.ContentParts,
				"name":             item.Name,
			})
		}
		causalHash := ""
		if len(item.ToolCalls) > 0 || strings.TrimSpace(item.ToolCallID) != "" || item.ToolResult != nil {
			causalHash = promptcompiler.HashPromptLayerCausal(map[string]any{
				"toolCalls": item.ToolCalls, "toolCallId": item.ToolCallID, "toolResult": item.ToolResult,
			})
		}
		entries = append(entries, promptcompiler.PromptLayerFingerprintEntry{
			LogicalLayer: layer,
			SectionID:    firstNonBlankPromptInputString(item.Source.SectionID, item.ID),
			ProviderRole: string(item.ProviderRole),
			SemanticRole: strings.TrimSpace(item.SemanticRole),
			Source:       strings.TrimSpace(item.Source.Origin),
			BundleRef:    strings.TrimSpace(item.Metadata["bundle_ref"]),
			ContentHash:  promptcompiler.HashPromptLayerContent(item.Content),
			PayloadHash:  payloadHash,
			CausalHash:   causalHash,
		})
	}
	return promptcompiler.BuildPromptFingerprintFromLayerEntries(entries, modelInputHash), nil
}
