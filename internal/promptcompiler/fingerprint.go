package promptcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const promptFingerprintVersion = "prompt-fingerprint-v2"

const promptLayerHashSchemaVersion = "aiops.prompt-layer-hash.v1"

type PromptLayerFingerprintEntry struct {
	LogicalLayer PromptLogicalLayer `json:"logicalLayer"`
	SectionID    string             `json:"sectionId,omitempty"`
	ProviderRole string             `json:"providerRole,omitempty"`
	SemanticRole string             `json:"semanticRole,omitempty"`
	Source       string             `json:"source,omitempty"`
	BundleRef    string             `json:"bundleRef,omitempty"`
	ContentHash  string             `json:"contentHash"`
	PayloadHash  string             `json:"payloadHash,omitempty"`
	CausalHash   string             `json:"causalHash,omitempty"`
}

// BuildPromptFingerprintForAdapter fingerprints a CompiledPrompt assembled by
// thin adapters that already provide a section-first envelope.
func BuildPromptFingerprintForAdapter(compiled CompiledPrompt) PromptFingerprint {
	return buildPromptFingerprint(compiled)
}

func buildPromptFingerprint(compiled CompiledPrompt) PromptFingerprint {
	fingerprint := PromptFingerprint{
		Version:           promptFingerprintVersion,
		CompilerVersion:   promptFingerprintVersion,
		StableHash:        hashPromptText(CompiledPromptStableText(compiled)),
		SystemHash:        hashPromptText(CompiledPromptBaseContractText(compiled)),
		DeveloperHash:     hashPromptText(CompiledPromptProfileText(compiled)),
		ToolRegistryHash:  hashPromptText(CompiledPromptToolSurfaceText(compiled)),
		RuntimePolicyHash: hashPromptText(CompiledPromptRuntimeStateText(compiled)),
		ProtocolStateHash: hashPromptJSON(compiled.Dynamic.ProtocolState),
	}
	if strings.TrimSpace(compiled.EnvelopeV2.SchemaVersion) != "" {
		if layered, err := BuildPromptFingerprintFromEnvelopeV2(compiled.EnvelopeV2); err == nil {
			fingerprint.AbsoluteSystemHash = layered.AbsoluteSystemHash
			fingerprint.RoleProfileHash = layered.RoleProfileHash
			fingerprint.StableRuntimeContractHash = layered.StableRuntimeContractHash
			fingerprint.StablePrefixHash = layered.StablePrefixHash
			fingerprint.TurnStableHash = layered.TurnStableHash
			fingerprint.TurnPrefixHash = layered.TurnPrefixHash
			fingerprint.DynamicContextHash = layered.DynamicContextHash
		}
	}
	return fingerprint
}

func BuildPromptFingerprintFromEnvelopeV2(envelope PromptEnvelopeV2) (PromptFingerprint, error) {
	if err := envelope.Validate(); err != nil {
		return PromptFingerprint{}, err
	}
	entries := make([]PromptLayerFingerprintEntry, 0, len(envelope.Sections))
	for _, section := range envelope.Sections {
		entries = append(entries, PromptLayerFingerprintEntry{
			LogicalLayer: section.LogicalLayer,
			SectionID:    strings.TrimSpace(section.ID),
			ProviderRole: strings.TrimSpace(section.Role),
			SemanticRole: string(section.LogicalLayer),
			Source:       strings.TrimSpace(section.Source),
			BundleRef:    strings.TrimSpace(section.BundleRef),
			ContentHash:  HashPromptLayerContent(section.Content),
		})
	}
	return BuildPromptFingerprintFromLayerEntries(entries, ""), nil
}

func BuildPromptFingerprintFromLayerEntries(entries []PromptLayerFingerprintEntry, modelInputHash string) PromptFingerprint {
	layerHash := func(layer PromptLogicalLayer) string {
		layerEntries := make([]PromptLayerFingerprintEntry, 0)
		for _, entry := range entries {
			if entry.LogicalLayer == layer {
				layerEntries = append(layerEntries, entry)
			}
		}
		if len(layerEntries) == 0 {
			return ""
		}
		return hashPromptJSON(struct {
			SchemaVersion string                        `json:"schemaVersion"`
			Kind          string                        `json:"kind"`
			Layer         PromptLogicalLayer            `json:"layer"`
			Entries       []PromptLayerFingerprintEntry `json:"entries"`
		}{promptLayerHashSchemaVersion, "logical-layer", layer, layerEntries})
	}
	fingerprint := PromptFingerprint{
		Version:                   promptFingerprintVersion,
		CompilerVersion:           promptFingerprintVersion,
		AbsoluteSystemHash:        layerHash(LayerAbsoluteSystemCore),
		RoleProfileHash:           layerHash(LayerRoleProfileCore),
		StableRuntimeContractHash: layerHash(LayerStableRuntimeContract),
		TurnStableHash:            layerHash(LayerTurnStableFacts),
		ConversationHistoryHash:   layerHash(LayerConversationHistory),
		DynamicContextHash:        layerHash(LayerStepDynamicContext),
		CurrentUserInputHash:      layerHash(LayerCurrentUserInput),
		ModelInputHash:            strings.TrimSpace(modelInputHash),
	}
	if fingerprint.AbsoluteSystemHash != "" && fingerprint.RoleProfileHash != "" && fingerprint.StableRuntimeContractHash != "" {
		fingerprint.StablePrefixHash = hashPromptJSON(struct {
			SchemaVersion string `json:"schemaVersion"`
			Kind          string `json:"kind"`
			L0            string `json:"l0"`
			L1            string `json:"l1"`
			L2            string `json:"l2"`
		}{promptLayerHashSchemaVersion, "stable-prefix", fingerprint.AbsoluteSystemHash, fingerprint.RoleProfileHash, fingerprint.StableRuntimeContractHash})
	}
	if fingerprint.StablePrefixHash != "" && fingerprint.TurnStableHash != "" {
		fingerprint.TurnPrefixHash = hashPromptJSON(struct {
			SchemaVersion string `json:"schemaVersion"`
			Kind          string `json:"kind"`
			StablePrefix  string `json:"stablePrefix"`
			L3            string `json:"l3"`
		}{promptLayerHashSchemaVersion, "turn-prefix", fingerprint.StablePrefixHash, fingerprint.TurnStableHash})
	}
	return fingerprint
}

func HashPromptLayerContent(content string) string {
	return hashPromptJSON(struct {
		SchemaVersion string `json:"schemaVersion"`
		Kind          string `json:"kind"`
		Content       string `json:"content"`
	}{promptLayerHashSchemaVersion, "entry-content", content})
}

func HashPromptLayerPayload(value any) string {
	return hashPromptJSON(struct {
		SchemaVersion string `json:"schemaVersion"`
		Kind          string `json:"kind"`
		Value         any    `json:"value"`
	}{promptLayerHashSchemaVersion, "entry-payload", value})
}

func HashPromptLayerCausal(value any) string {
	return hashPromptJSON(struct {
		SchemaVersion string `json:"schemaVersion"`
		Kind          string `json:"kind"`
		Value         any    `json:"value"`
	}{promptLayerHashSchemaVersion, "entry-causal", value})
}

func hashPromptText(value string) string {
	return hashPromptBytes([]byte(strings.TrimSpace(value)))
}

func hashPromptJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return hashPromptText("")
	}
	return hashPromptBytes(data)
}

func hashPromptBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
