package modelrouter

import (
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestBuildProviderRequestSnapshotSeparatesStableCacheKeyFromDynamicIds(t *testing.T) {
	items := []promptinput.ModelInputItem{{
		ID:           "user-1",
		ProviderRole: promptinput.ProviderRoleUser,
		Content:      "check nginx",
		CacheGroup:   "turn-user",
	}}
	req := ProviderRequestSnapshot{
		Model:    "gpt-4.1",
		Provider: "openai",
		Input:    items,
		Tools:    []ProviderToolSpec{{Name: "exec_command", Hash: "tool-hash"}},
		ClientMetadata: map[string]string{
			"turnId":  "turn-1",
			"traceId": "trace-1",
		},
		ReasoningEffort: "high",
	}
	req.ComputeHashes()
	firstCacheKey := req.PromptCacheKey
	req.ClientMetadata["turnId"] = "turn-2"
	req.ClientMetadata["traceId"] = "trace-2"
	req.ComputeHashes()
	if req.PromptCacheKey != firstCacheKey {
		t.Fatalf("PromptCacheKey changed after dynamic id mutation: %q != %q", req.PromptCacheKey, firstCacheKey)
	}
	if req.ModelInputHash == "" || req.RequestPropertiesHash == "" {
		t.Fatalf("hashes missing: %#v", req)
	}
}
