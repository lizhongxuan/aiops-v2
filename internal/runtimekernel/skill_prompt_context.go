package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

func appendSkillActivationContext(compileCtx promptcompiler.CompileContext, session *SessionState) promptcompiler.CompileContext {
	if session == nil || len(session.SkillActivation.LoadedSkills) == 0 {
		return compileCtx
	}
	names := make([]string, 0, len(session.SkillActivation.LoadedSkills))
	for name := range session.SkillActivation.LoadedSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ref := session.SkillActivation.LoadedSkills[name]
		if strings.TrimSpace(ref.Name) == "" {
			continue
		}
		compileCtx.LoadedSkillRefs = append(compileCtx.LoadedSkillRefs, promptcompiler.LoadedSkillPromptRef{
			Name:   ref.Name,
			Source: ref.Source,
			Reason: ref.Reason,
			Range:  fmt.Sprintf("%d:%d", ref.Range.Offset, ref.Range.Limit),
			Hash:   ref.Hash,
		})
	}
	return compileCtx
}

func activeSkillToolPolicies(session *SessionState) []tooling.SkillToolPolicy {
	if session == nil || len(session.SkillActivation.LoadedSkills) == 0 {
		return nil
	}
	names := session.SkillActivation.EnabledSkills()
	out := make([]tooling.SkillToolPolicy, 0, len(names))
	for _, name := range names {
		ref := session.SkillActivation.LoadedSkills[name]
		if len(ref.AllowedTools) == 0 && len(ref.DeniedTools) == 0 && strings.TrimSpace(ref.RiskCeiling) == "" {
			continue
		}
		out = append(out, tooling.SkillToolPolicy{
			SkillName:    ref.Name,
			AllowedTools: append([]string(nil), ref.AllowedTools...),
			DeniedTools:  append([]string(nil), ref.DeniedTools...),
			RiskCeiling:  ref.RiskCeiling,
		})
	}
	return out
}
