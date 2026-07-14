package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

const promptCacheKeySchemaVersion = "aiops.prompt-cache-key.v2"

type ProviderToolSpec struct {
	Name string `json:"name"`
	Hash string `json:"hash,omitempty"`
}

type ProviderRequestSnapshot struct {
	Provider              string                           `json:"provider"`
	Model                 string                           `json:"model"`
	Input                 []promptinput.ModelInputItem     `json:"input"`
	Tools                 []ProviderToolSpec               `json:"tools,omitempty"`
	ReasoningEffort       string                           `json:"reasoningEffort,omitempty"`
	Temperature           float64                          `json:"temperature,omitempty"`
	TopP                  float64                          `json:"topP,omitempty"`
	MaxTokens             int                              `json:"maxTokens,omitempty"`
	ParallelToolCalls     bool                             `json:"parallelToolCalls,omitempty"`
	ClientMetadata        map[string]string                `json:"clientMetadata,omitempty"`
	ModelInputHash        string                           `json:"modelInputHash,omitempty"`
	ProviderMessagesHash  string                           `json:"providerMessagesHash,omitempty"`
	RequestPropertiesHash string                           `json:"requestPropertiesHash,omitempty"`
	PromptCacheKey        string                           `json:"promptCacheKey,omitempty"`
	PromptFingerprint     promptcompiler.PromptFingerprint `json:"promptFingerprint,omitempty"`
	MessageAudit          *ProviderMessageAudit            `json:"messageAudit,omitempty"`
}

func (r *ProviderRequestSnapshot) ComputeHashes() {
	r.ModelInputHash = stableHash(r.Input)
	r.RequestPropertiesHash = stableHash(map[string]any{
		"provider":          r.Provider,
		"model":             r.Model,
		"tools":             r.Tools,
		"reasoningEffort":   r.ReasoningEffort,
		"temperature":       r.Temperature,
		"topP":              r.TopP,
		"maxTokens":         r.MaxTokens,
		"parallelToolCalls": r.ParallelToolCalls,
	})
	r.PromptFingerprint = clearCanonicalPromptFingerprint(r.PromptFingerprint)
	canonical, fingerprintErr := promptinput.BuildModelInputPromptFingerprint(r.Input)
	if fingerprintErr == nil && canonical.ModelInputHash == r.ModelInputHash {
		r.PromptFingerprint = mergeCanonicalPromptFingerprint(r.PromptFingerprint, canonical)
	}
	if promptinput.HasTypedModelInputLayers(r.Input) && (fingerprintErr != nil || r.PromptFingerprint.TurnPrefixHash == "" || r.PromptFingerprint.ModelInputHash != r.ModelInputHash) {
		r.PromptCacheKey = ""
		return
	}
	stableInputHash := r.PromptFingerprint.TurnPrefixHash
	if stableInputHash == "" {
		stableInputHash = promptCacheInputHash(stableProviderInput(r.Input))
	}
	r.PromptCacheKey = stableHash(map[string]any{
		"schemaVersion":         promptCacheKeySchemaVersion,
		"requestPropertiesHash": r.RequestPropertiesHash,
		"turnPrefixHash":        stableInputHash,
	})
}

func clearCanonicalPromptFingerprint(fingerprint promptcompiler.PromptFingerprint) promptcompiler.PromptFingerprint {
	fingerprint.AbsoluteSystemHash = ""
	fingerprint.RoleProfileHash = ""
	fingerprint.StableRuntimeContractHash = ""
	fingerprint.StablePrefixHash = ""
	fingerprint.TurnStableHash = ""
	fingerprint.TurnPrefixHash = ""
	fingerprint.ConversationHistoryHash = ""
	fingerprint.DynamicContextHash = ""
	fingerprint.CurrentUserInputHash = ""
	fingerprint.ModelInputHash = ""
	return fingerprint
}

func stableProviderInput(items []promptinput.ModelInputItem) []promptinput.ModelInputItem {
	stable := make([]promptinput.ModelInputItem, 0, len(items))
	for _, item := range items {
		if item.CacheGroup == promptcompiler.PromptSectionKindStable {
			stable = append(stable, item)
		}
	}
	return stable
}

func mergeCanonicalPromptFingerprint(base, canonical promptcompiler.PromptFingerprint) promptcompiler.PromptFingerprint {
	if base.Version == "" {
		base.Version = canonical.Version
	}
	if base.CompilerVersion == "" {
		base.CompilerVersion = canonical.CompilerVersion
	}
	base.AbsoluteSystemHash = canonical.AbsoluteSystemHash
	base.RoleProfileHash = canonical.RoleProfileHash
	base.StableRuntimeContractHash = canonical.StableRuntimeContractHash
	base.StablePrefixHash = canonical.StablePrefixHash
	base.TurnStableHash = canonical.TurnStableHash
	base.TurnPrefixHash = canonical.TurnPrefixHash
	base.ConversationHistoryHash = canonical.ConversationHistoryHash
	base.DynamicContextHash = canonical.DynamicContextHash
	base.CurrentUserInputHash = canonical.CurrentUserInputHash
	base.ModelInputHash = canonical.ModelInputHash
	return base
}

func promptCacheInputHash(items []promptinput.ModelInputItem) string {
	type cacheToolCall struct {
		Name      string          `json:"name,omitempty"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
	}
	type cacheToolResult struct {
		Content string `json:"content,omitempty"`
	}
	type cacheSource struct {
		Layer     string `json:"layer,omitempty"`
		SectionID string `json:"sectionId,omitempty"`
		Origin    string `json:"origin,omitempty"`
	}
	type cacheInputItem struct {
		ProviderRole promptinput.ProviderRole            `json:"providerRole"`
		SemanticRole string                              `json:"semanticRole,omitempty"`
		Content      string                              `json:"content,omitempty"`
		ContentParts []promptinput.ModelInputContentPart `json:"contentParts,omitempty"`
		Name         string                              `json:"name,omitempty"`
		ToolCalls    []cacheToolCall                     `json:"toolCalls,omitempty"`
		ToolResult   *cacheToolResult                    `json:"toolResult,omitempty"`
		Source       cacheSource                         `json:"source,omitempty"`
		Phase        string                              `json:"phase,omitempty"`
		CacheGroup   string                              `json:"cacheGroup,omitempty"`
	}

	cacheItems := make([]cacheInputItem, 0, len(items))
	for _, item := range items {
		cacheCalls := make([]cacheToolCall, 0, len(item.ToolCalls))
		for _, call := range item.ToolCalls {
			cacheCalls = append(cacheCalls, cacheToolCall{
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Arguments...),
			})
		}
		var toolResult *cacheToolResult
		if item.ToolResult != nil {
			toolResult = &cacheToolResult{Content: item.ToolResult.Content}
		}
		cacheItems = append(cacheItems, cacheInputItem{
			ProviderRole: item.ProviderRole,
			SemanticRole: item.SemanticRole,
			Content:      item.Content,
			ContentParts: append([]promptinput.ModelInputContentPart(nil), item.ContentParts...),
			Name:         item.Name,
			ToolCalls:    cacheCalls,
			ToolResult:   toolResult,
			Source: cacheSource{
				Layer:     item.Source.Layer,
				SectionID: item.Source.SectionID,
				Origin:    item.Source.Origin,
			},
			Phase:      item.Phase,
			CacheGroup: item.CacheGroup,
		})
	}
	return stableHash(cacheItems)
}

func stableHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type ProviderMessageAudit struct {
	ProviderMessagesHash string                     `json:"providerMessagesHash"`
	Items                []ProviderMessageAuditItem `json:"items"`
}

type ProviderMessageAuditItem struct {
	ItemID              string `json:"itemId"`
	ProviderRole        string `json:"providerRole"`
	ToolCallID          string `json:"toolCallId,omitempty"`
	ItemHash            string `json:"itemHash"`
	ProviderMessageHash string `json:"providerMessageHash"`
}
