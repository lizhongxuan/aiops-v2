package agentassembly

import (
	"sort"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

type PromptSectionSnapshot struct {
	Lifecycle Lifecycle           `json:"lifecycle,omitempty"`
	Sections  []PromptSectionItem `json:"sections,omitempty"`
	Hash      string              `json:"hash,omitempty"`
}

type PromptSectionItem struct {
	ID             string `json:"id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Source         string `json:"source,omitempty"`
	Hash           string `json:"hash,omitempty"`
	Bytes          int    `json:"bytes,omitempty"`
	TokensEstimate int    `json:"tokensEstimate,omitempty"`
}

func PromptSectionSnapshotFromTrace(sections []promptcompiler.PromptSectionTrace) PromptSectionSnapshot {
	out := PromptSectionSnapshot{Lifecycle: LifecycleRequestScope}
	for _, section := range sections {
		id := strings.TrimSpace(section.ID)
		if id == "" {
			continue
		}
		out.Sections = append(out.Sections, PromptSectionItem{
			ID:             id,
			Kind:           strings.TrimSpace(section.Kind),
			Source:         strings.TrimSpace(section.Source),
			Hash:           strings.TrimSpace(section.Hash),
			Bytes:          section.Bytes,
			TokensEstimate: section.TokensEstimate,
		})
	}
	sort.Slice(out.Sections, func(i, j int) bool {
		return out.Sections[i].ID < out.Sections[j].ID
	})
	out.Hash = StableHash("prompt-sections.snapshot", out.Sections)
	return out
}
