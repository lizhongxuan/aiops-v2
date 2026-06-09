package runtimekernel

import (
	"strings"

	"aiops-v2/internal/skills"
)

func (k *EinoKernel) mandatorySkillDefinitionsForInput(input string) []skills.Definition {
	if k == nil || k.skillRegistry == nil {
		return nil
	}
	defs := k.skillRegistry.List()
	if len(defs) == 0 {
		return nil
	}
	index := skills.BuildSkillIndex(defs, skills.SkillIndexOptions{
		Query:    strings.TrimSpace(input),
		MaxChars: skills.MaxSkillIndexChars,
	})
	if len(index.Entries) == 0 {
		return nil
	}
	byName := make(map[string]skills.Definition, len(defs))
	for _, def := range defs {
		byName[strings.TrimSpace(def.Name)] = def
	}
	out := make([]skills.Definition, 0, len(index.Entries))
	for _, entry := range index.Entries {
		def, ok := byName[strings.TrimSpace(entry.Name)]
		if !ok || !def.Discovery.RequiredForMatch {
			continue
		}
		out = append(out, def)
	}
	return out
}
