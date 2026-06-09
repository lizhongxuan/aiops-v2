package promptcompiler

import "strings"

// BuildPromptSectionTrace returns redaction-safe prompt section metadata.
func BuildPromptSectionTrace(compiled CompiledPrompt) []PromptSectionTrace {
	sections := []PromptSectionTrace{
		promptSectionTrace("system.role", PromptSectionKindStable, "system", compiled.Stable.System.Content, PromptSectionCacheMiss),
		promptSectionTrace("developer.core_rules", PromptSectionKindStable, "developer", compiled.Stable.Developer.Content, PromptSectionCacheMiss),
		promptSectionTrace("tools.index", PromptSectionKindStable, "tool-registry", compiled.Stable.Tools.Content, PromptSectionCacheMiss),
		promptSectionTrace("runtime.policy", PromptSectionKindDynamic, "runtime-policy", compiled.Dynamic.Policy.Content, PromptSectionCacheMiss),
		promptSectionTrace("protocol.state", PromptSectionKindDynamic, "protocol-state", renderProtocolPromptState(compiled.Dynamic.ProtocolState), PromptSectionCacheMiss),
		promptSectionTrace("context.dynamic_assets", PromptSectionKindDynamic, "dynamic-assets", dynamicAssetsFingerprintText(compiled.Dynamic), PromptSectionCacheMiss),
	}
	return sections
}

// DiffPromptSections reports redaction-safe section hash changes.
func DiffPromptSections(previous, current []PromptSectionTrace) []ChangedPromptSection {
	prevByID := map[string]PromptSectionTrace{}
	for _, section := range previous {
		prevByID[section.ID] = section
	}
	var changes []ChangedPromptSection
	seen := map[string]bool{}
	for _, section := range current {
		seen[section.ID] = true
		prev, ok := prevByID[section.ID]
		switch {
		case !ok:
			changes = append(changes, ChangedPromptSection{
				ID:          section.ID,
				Reason:      PromptSectionChangeSectionAdded,
				CurrentHash: section.Hash,
			})
		case prev.Hash != section.Hash:
			changes = append(changes, ChangedPromptSection{
				ID:           section.ID,
				Reason:       promptSectionChangeReason(section.ID),
				PreviousHash: prev.Hash,
				CurrentHash:  section.Hash,
			})
		}
	}
	for _, section := range previous {
		if seen[section.ID] {
			continue
		}
		changes = append(changes, ChangedPromptSection{
			ID:           section.ID,
			Reason:       PromptSectionChangeSectionRemoved,
			PreviousHash: section.Hash,
		})
	}
	return changes
}

// ChangedPromptSections is the stable public name used by prompt runtime
// callers that compare section traces between model calls.
func ChangedPromptSections(previous, current []PromptSectionTrace) []ChangedPromptSection {
	return DiffPromptSections(previous, current)
}

// ApplyPromptSectionCache annotates current sections with cache status relative
// to the previous model input trace. It records reuse decisions without storing
// raw prompt section text.
func ApplyPromptSectionCache(previous, current []PromptSectionTrace) []PromptSectionTrace {
	if len(current) == 0 {
		return nil
	}
	prevByID := map[string]PromptSectionTrace{}
	for _, section := range previous {
		prevByID[section.ID] = section
	}
	out := append([]PromptSectionTrace(nil), current...)
	for i := range out {
		prev, ok := prevByID[out[i].ID]
		switch {
		case !ok:
			out[i].Cache = PromptSectionCacheMiss
		case prev.Hash == out[i].Hash:
			out[i].Cache = PromptSectionCacheHit
		default:
			out[i].Cache = PromptSectionCacheInvalidated
		}
	}
	return out
}

func promptSectionTrace(id, kind, source, content, cache string) PromptSectionTrace {
	trimmed := strings.TrimSpace(content)
	return PromptSectionTrace{
		ID:             id,
		Kind:           kind,
		Source:         source,
		Hash:           "sha256:" + hashPromptText(trimmed),
		Bytes:          len([]byte(trimmed)),
		TokensEstimate: promptSectionEstimateTokens(trimmed),
		Cache:          cache,
	}
}

func dynamicAssetsFingerprintText(dynamic DynamicPromptDelta) string {
	var parts []string
	parts = append(parts, dynamic.SkillPromptAssets...)
	parts = append(parts, dynamic.EvidenceReminders...)
	for _, section := range dynamic.ExtraSections {
		parts = append(parts, section.Title, section.Content)
	}
	if dynamic.ToolDelta.Content != "" {
		parts = append(parts, dynamic.ToolDelta.Content)
	}
	return strings.Join(parts, "\n")
}

func promptSectionEstimateTokens(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	tokens := len(value) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func promptSectionChangeReason(id string) string {
	switch id {
	case "system.role":
		return PromptSectionChangeSystemRoleChanged
	case "developer.core_rules":
		return PromptSectionChangeDeveloperRulesChanged
	case "tools.index":
		return PromptSectionChangeToolsIndexChanged
	case "runtime.policy":
		return PromptSectionChangeRuntimePolicyChanged
	case "protocol.state":
		return PromptSectionChangeProtocolStateChanged
	case "context.dynamic_assets":
		return PromptSectionChangeDynamicAssetsChanged
	default:
		return PromptSectionChangeSectionContentChanged
	}
}
