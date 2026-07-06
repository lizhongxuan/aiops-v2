package promptcompiler

import "strings"

// BuildPromptSectionTrace returns redaction-safe prompt section metadata.
func BuildPromptSectionTrace(compiled CompiledPrompt) []PromptSectionTrace {
	sections := make([]PromptSectionTrace, 0, len(compiled.Envelope.Sections))
	for _, section := range compiled.Envelope.Sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		kind := strings.TrimSpace(section.Stability)
		if kind == "" {
			kind = strings.TrimSpace(section.Layer)
		}
		if kind == "" {
			kind = PromptSectionKindDynamic
		}
		source := strings.TrimSpace(section.Source)
		if source == "" {
			source = section.ID
		}
		sections = append(sections, promptSectionTrace(section.ID, kind, source, content, PromptSectionCacheMiss))
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
	contract := LookupPromptSectionContract(id)
	tokensEstimate := promptSectionEstimateTokens(trimmed)
	return PromptSectionTrace{
		ID:             id,
		Kind:           kind,
		Source:         source,
		Hash:           "sha256:" + hashPromptText(trimmed),
		Bytes:          len([]byte(trimmed)),
		TokensEstimate: tokensEstimate,
		TokenEstimate:  tokensEstimate,
		Cache:          cache,
		RetentionRank:  contract.RetentionRank,
		RetentionClass: contract.RetentionClass,
		CompactAction:  CompactActionKeptOriginal,
		Action:         "kept",
		CompactSchema:  contract.CompactSchema,
		Redaction:      contract.RedactionPolicy,
		Purpose:        contract.Purpose,
	}
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
	case "base.contract":
		return PromptSectionChangeSystemRoleChanged
	case "tool.surface":
		return PromptSectionChangeToolsIndexChanged
	case "runtime.state":
		return PromptSectionChangeRuntimePolicyChanged
	case "dynamic.context":
		return PromptSectionChangeDynamicAssetsChanged
	default:
		return PromptSectionChangeSectionContentChanged
	}
}
