package promptcompiler

import "strings"

func BuildPromptEnvelope(compiled CompiledPrompt, ctx CompileContext) PromptEnvelope {
	sections := make([]PromptCompiledSection, 0, 5)
	if base := strings.TrimSpace(buildBaseRuntimeContract("")); base != "" {
		sections = append(sections, PromptCompiledSection{
			ID:        "base.contract",
			Layer:     PromptSectionKindStable,
			Role:      "system",
			Content:   base,
			Stability: PromptSectionKindStable,
			Source:    "base",
			Required:  true,
		})
	}
	if runtimeState := strings.TrimSpace(compiled.Dynamic.Policy.Content); runtimeState != "" {
		sections = append(sections, PromptCompiledSection{
			ID:        "runtime.state",
			Layer:     PromptSectionKindDynamic,
			Role:      "system",
			Content:   runtimeState,
			Stability: PromptSectionKindDynamic,
			Source:    "runtime",
			Required:  true,
		})
	}
	profile := resolvePromptEnvelopeProfile(ctx)
	if profileContent := buildProfileFragment(profile, ctx.HostContext); strings.TrimSpace(profileContent) != "" {
		sections = append(sections, PromptCompiledSection{
			ID:        "profile." + profile,
			Layer:     PromptSectionKindStable,
			Role:      "system",
			Content:   profileContent,
			Stability: PromptSectionKindStable,
			Source:    "profile",
			Required:  true,
		})
	}
	if tools := strings.TrimSpace(compiled.Stable.Tools.Content); tools != "" {
		sections = append(sections, PromptCompiledSection{
			ID:        "tool.surface",
			Layer:     PromptSectionKindStable,
			Role:      "system",
			Content:   tools,
			Stability: PromptSectionKindStable,
			Source:    "tools",
			Required:  true,
		})
	}
	if dynamicContext := strings.TrimSpace(compiled.effectiveDynamicContextContent()); dynamicContext != "" {
		sections = append(sections, PromptCompiledSection{
			ID:        "dynamic.context",
			Layer:     PromptSectionKindDynamic,
			Role:      "system",
			Content:   dynamicContext,
			Stability: PromptSectionKindDynamic,
			Source:    "dynamic",
		})
	}
	return PromptEnvelope{Sections: sections}
}

func resolvePromptEnvelopeProfile(ctx CompileContext) string {
	if profile := normalizePromptProfile(ctx.Profile); profile != "" {
		return profile
	}
	if ctx.HostOpsManager {
		return PromptProfileHostManager
	}
	if strings.EqualFold(strings.TrimSpace(ctx.SessionType), "host") && strings.EqualFold(strings.TrimSpace(ctx.Mode), "execute") {
		return PromptProfileHostWorker
	}
	if strings.EqualFold(strings.TrimSpace(ctx.Mode), "inspect") {
		return PromptProfileEvidenceRCA
	}
	return PromptProfileAdvisor
}

// CompiledPromptSectionContent returns the prompt text for a section id from
// the section-first envelope. It falls back to compatibility fields only for
// callers that construct legacy CompiledPrompt values in tests or adapters.
func CompiledPromptSectionContent(compiled CompiledPrompt, sectionID string) string {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID != "" {
		for _, section := range compiled.Envelope.Sections {
			if section.ID == sectionID {
				return strings.TrimSpace(section.Content)
			}
		}
	}
	switch sectionID {
	case "base.contract":
		if content := strings.TrimSpace(compiled.System.Content); content != "" {
			return content
		}
		return strings.TrimSpace(compiled.Stable.System.Content)
	case "profile.advisor", "profile.evidence_rca", "profile.host_worker", "profile.host_manager":
		if content := strings.TrimSpace(compiled.Developer.Content); content != "" {
			return content
		}
		return strings.TrimSpace(compiled.Stable.Developer.Content)
	case "tool.surface":
		if content := strings.TrimSpace(compiled.Tools.Content); content != "" {
			return content
		}
		return strings.TrimSpace(compiled.Stable.Tools.Content)
	case "runtime.state":
		if content := strings.TrimSpace(compiled.Policy.Content); content != "" {
			return content
		}
		return strings.TrimSpace(compiled.Dynamic.Policy.Content)
	case "dynamic.context":
		return strings.TrimSpace(compiled.effectiveDynamicContextContent())
	default:
		return ""
	}
}

func CompiledPromptStableText(compiled CompiledPrompt) string {
	if len(compiled.Envelope.Sections) > 0 {
		parts := make([]string, 0, len(compiled.Envelope.Sections))
		for _, section := range compiled.Envelope.Sections {
			if section.Stability != PromptSectionKindStable && section.Layer != PromptSectionKindStable {
				continue
			}
			if content := strings.TrimSpace(section.Content); content != "" {
				parts = append(parts, content)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return strings.TrimSpace(compiled.Stable.Content)
}

func CompiledPromptDynamicText(compiled CompiledPrompt) string {
	if len(compiled.Envelope.Sections) > 0 {
		parts := make([]string, 0, len(compiled.Envelope.Sections))
		for _, section := range compiled.Envelope.Sections {
			if section.Stability != PromptSectionKindDynamic && section.Layer != PromptSectionKindDynamic {
				continue
			}
			if content := strings.TrimSpace(section.Content); content != "" {
				parts = append(parts, content)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return strings.TrimSpace(compiled.Dynamic.Content)
}

func (c CompiledPrompt) effectiveDynamicContextContent() string {
	content := strings.TrimSpace(c.Dynamic.Content)
	if content == "" {
		return ""
	}
	policyContent := strings.TrimSpace(c.Policy.Content)
	if policyContent == "" {
		policyContent = strings.TrimSpace(c.Dynamic.Policy.Content)
	}
	if policyContent != "" && strings.HasSuffix(content, policyContent) {
		content = strings.TrimSpace(strings.TrimSuffix(content, policyContent))
	}
	return content
}

func CompiledPromptToolSurfaceText(compiled CompiledPrompt) string {
	return CompiledPromptSectionContent(compiled, "tool.surface")
}

func CompiledPromptRuntimeStateText(compiled CompiledPrompt) string {
	return CompiledPromptSectionContent(compiled, "runtime.state")
}

func CompiledPromptBaseContractText(compiled CompiledPrompt) string {
	return CompiledPromptSectionContent(compiled, "base.contract")
}

func CompiledPromptProfileText(compiled CompiledPrompt) string {
	if len(compiled.Envelope.Sections) > 0 {
		for _, section := range compiled.Envelope.Sections {
			if strings.HasPrefix(section.ID, "profile.") {
				return strings.TrimSpace(section.Content)
			}
		}
	}
	if content := strings.TrimSpace(compiled.Developer.Content); content != "" {
		return content
	}
	return strings.TrimSpace(compiled.Stable.Developer.Content)
}
