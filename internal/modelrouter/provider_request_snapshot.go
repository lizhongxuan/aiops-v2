package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"aiops-v2/internal/promptinput"
)

type ProviderToolSpec struct {
	Name string `json:"name"`
	Hash string `json:"hash,omitempty"`
}

type ProviderRequestSnapshot struct {
	Provider              string                       `json:"provider"`
	Model                 string                       `json:"model"`
	Input                 []promptinput.ModelInputItem `json:"input"`
	Tools                 []ProviderToolSpec           `json:"tools,omitempty"`
	ReasoningEffort       string                       `json:"reasoningEffort,omitempty"`
	Temperature           float64                      `json:"temperature,omitempty"`
	TopP                  float64                      `json:"topP,omitempty"`
	MaxTokens             int                          `json:"maxTokens,omitempty"`
	ParallelToolCalls     bool                         `json:"parallelToolCalls,omitempty"`
	ClientMetadata        map[string]string            `json:"clientMetadata,omitempty"`
	ModelInputHash        string                       `json:"modelInputHash,omitempty"`
	ProviderMessagesHash  string                       `json:"providerMessagesHash,omitempty"`
	RequestPropertiesHash string                       `json:"requestPropertiesHash,omitempty"`
	PromptCacheKey        string                       `json:"promptCacheKey,omitempty"`
	MessageAudit          *ProviderMessageAudit        `json:"messageAudit,omitempty"`
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
	r.PromptCacheKey = stableHash(map[string]any{
		"provider":        r.Provider,
		"model":           r.Model,
		"tools":           r.Tools,
		"reasoningEffort": r.ReasoningEffort,
		"cacheGroups":     cacheGroupsForProviderInput(r.Input),
	})
}

func cacheGroupsForProviderInput(items []promptinput.ModelInputItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.CacheGroup != "" {
			out = append(out, item.CacheGroup)
		}
	}
	return out
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
