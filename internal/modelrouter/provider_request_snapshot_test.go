package modelrouter

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
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
	req.Input[0].ID = "user-2"
	req.ComputeHashes()
	if req.PromptCacheKey != firstCacheKey {
		t.Fatalf("PromptCacheKey changed after dynamic id mutation: %q != %q", req.PromptCacheKey, firstCacheKey)
	}
	if req.ModelInputHash == "" || req.RequestPropertiesHash == "" {
		t.Fatalf("hashes missing: %#v", req)
	}
}

func TestBuildProviderRequestSnapshotLegacyCacheFallbackUsesStableGroupOnly(t *testing.T) {
	base := ProviderRequestSnapshot{Provider: "legacy", Model: "legacy-model", Input: []promptinput.ModelInputItem{
		{ID: "stable", ProviderRole: promptinput.ProviderRoleSystem, Content: "stable contract", CacheGroup: promptcompiler.PromptSectionKindStable},
		{ID: "dynamic", ProviderRole: promptinput.ProviderRoleUser, Content: "current user", CacheGroup: promptcompiler.PromptSectionKindDynamic},
	}}
	base.ComputeHashes()
	dynamic := base
	dynamic.Input = append([]promptinput.ModelInputItem(nil), base.Input...)
	dynamic.Input[1].Content = "different current user"
	dynamic.ComputeHashes()
	if base.PromptCacheKey != dynamic.PromptCacheKey || base.ModelInputHash == dynamic.ModelInputHash {
		t.Fatal("legacy dynamic group changed cache prefix or failed to change full model hash")
	}
	stable := base
	stable.Input = append([]promptinput.ModelInputItem(nil), base.Input...)
	stable.Input[0].Content = "changed stable contract"
	stable.ComputeHashes()
	if base.PromptCacheKey == stable.PromptCacheKey {
		t.Fatal("legacy stable group change did not change cache key")
	}
}

func TestBuildProviderRequestSnapshotPromptCacheKeyIgnoresDynamicLogicalLayers(t *testing.T) {
	base := ProviderRequestSnapshot{
		Model:           "glm-5.1",
		Provider:        "zai",
		Input:           canonicalProviderRequestItems(),
		Tools:           []ProviderToolSpec{{Name: "exec_command", Hash: "tool-hash"}},
		ReasoningEffort: "high",
	}
	base.ComputeHashes()
	if base.PromptFingerprint.StablePrefixHash == "" || base.PromptFingerprint.TurnPrefixHash == "" || base.PromptFingerprint.ModelInputHash != base.ModelInputHash {
		t.Fatalf("canonical fingerprint missing or inconsistent: %#v", base)
	}
	for name, index := range map[string]int{"history L4": 4, "dynamic L5": 5, "current user L6": 6} {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Input = append([]promptinput.ModelInputItem(nil), base.Input...)
			changed.Input[index].Content += " changed"
			changed.ComputeHashes()
			if base.PromptCacheKey != changed.PromptCacheKey {
				t.Fatalf("dynamic layer changed PromptCacheKey: %q != %q", base.PromptCacheKey, changed.PromptCacheKey)
			}
			if base.ModelInputHash == changed.ModelInputHash {
				t.Fatal("dynamic layer did not change ModelInputHash")
			}
		})
	}
}

func TestBuildProviderRequestSnapshotPromptCacheKeyChangesWithTurnPrefixAndRequestProperties(t *testing.T) {
	base := ProviderRequestSnapshot{
		Model: "glm-5.1", Provider: "zai", Input: canonicalProviderRequestItems(),
		Tools: []ProviderToolSpec{{Name: "exec_command", Hash: "tool-hash"}}, ReasoningEffort: "high",
	}
	base.ComputeHashes()
	for name, index := range map[string]int{"L0": 0, "L1": 1, "L2": 2, "L3": 3} {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Input = append([]promptinput.ModelInputItem(nil), base.Input...)
			changed.Input[index].Content += " changed"
			changed.ComputeHashes()
			if base.PromptCacheKey == changed.PromptCacheKey {
				t.Fatalf("%s change did not update PromptCacheKey", name)
			}
		})
	}
	propertyChanges := map[string]func(*ProviderRequestSnapshot){
		"provider": func(req *ProviderRequestSnapshot) { req.Provider = "other-provider" },
		"model":    func(req *ProviderRequestSnapshot) { req.Model = "other-model" },
		"tools": func(req *ProviderRequestSnapshot) {
			req.Tools = []ProviderToolSpec{{Name: "exec_command", Hash: "different-tool-hash"}}
		},
		"reasoning":     func(req *ProviderRequestSnapshot) { req.ReasoningEffort = "low" },
		"temperature":   func(req *ProviderRequestSnapshot) { req.Temperature = 0.4 },
		"topP":          func(req *ProviderRequestSnapshot) { req.TopP = 0.8 },
		"maxTokens":     func(req *ProviderRequestSnapshot) { req.MaxTokens = 2048 },
		"parallelTools": func(req *ProviderRequestSnapshot) { req.ParallelToolCalls = true },
	}
	for name, mutate := range propertyChanges {
		t.Run("request property "+name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			changed.ComputeHashes()
			if base.PromptCacheKey == changed.PromptCacheKey || base.RequestPropertiesHash == changed.RequestPropertiesHash {
				t.Fatalf("%s change did not update request properties/cache key", name)
			}
		})
	}
	legacyAlias := base
	legacyAlias.PromptFingerprint.StableHash = "legacy-stable"
	legacyAlias.PromptFingerprint.DeveloperHash = "legacy-developer"
	legacyAlias.ComputeHashes()
	if legacyAlias.PromptFingerprint.StableHash != "legacy-stable" || legacyAlias.PromptFingerprint.DeveloperHash != "legacy-developer" {
		t.Fatalf("canonical fingerprint overwrote legacy aliases: %#v", legacyAlias.PromptFingerprint)
	}
	invalid := base
	invalid.Input = append([]promptinput.ModelInputItem(nil), base.Input[1:]...)
	invalid.ComputeHashes()
	if invalid.PromptFingerprint.StablePrefixHash != "" || invalid.PromptFingerprint.ModelInputHash != "" || invalid.PromptCacheKey != "" {
		t.Fatalf("invalid typed input retained stale canonical fingerprint/cache key: %#v", invalid)
	}
	mixed := base
	mixed.Input = append([]promptinput.ModelInputItem(nil), base.Input...)
	mixed.Input = append(mixed.Input[:len(mixed.Input)-1], promptinput.ModelInputItem{ID: "untyped", ProviderRole: promptinput.ProviderRoleSystem, Content: "invalid mixed item"}, mixed.Input[len(mixed.Input)-1])
	mixed.ComputeHashes()
	if mixed.PromptCacheKey != "" {
		t.Fatalf("mixed typed/untyped input received cache key: %#v", mixed)
	}
}

func canonicalProviderRequestItems() []promptinput.ModelInputItem {
	item := func(id string, role promptinput.ProviderRole, layer promptcompiler.PromptLogicalLayer, content string) promptinput.ModelInputItem {
		cacheGroup := promptcompiler.PromptSectionKindDynamic
		if layer == promptcompiler.LayerAbsoluteSystemCore || layer == promptcompiler.LayerRoleProfileCore || layer == promptcompiler.LayerStableRuntimeContract || layer == promptcompiler.LayerTurnStableFacts {
			cacheGroup = promptcompiler.PromptSectionKindStable
		}
		return promptinput.ModelInputItem{
			ID: id, ProviderRole: role, SemanticRole: string(layer), Content: content,
			Source: promptinput.ModelInputSource{Layer: string(layer)}, CacheGroup: cacheGroup,
		}
	}
	return []promptinput.ModelInputItem{
		item("l0", promptinput.ProviderRoleSystem, promptcompiler.LayerAbsoluteSystemCore, "system"),
		item("l1", promptinput.ProviderRoleSystem, promptcompiler.LayerRoleProfileCore, "role"),
		item("l2", promptinput.ProviderRoleSystem, promptcompiler.LayerStableRuntimeContract, "contract"),
		item("l3", promptinput.ProviderRoleSystem, promptcompiler.LayerTurnStableFacts, "turn"),
		item("l4", promptinput.ProviderRoleUser, promptcompiler.LayerConversationHistory, "history"),
		item("l5", promptinput.ProviderRoleSystem, promptcompiler.LayerStepDynamicContext, "dynamic"),
		item("l6", promptinput.ProviderRoleUser, promptcompiler.LayerCurrentUserInput, "current"),
	}
}
