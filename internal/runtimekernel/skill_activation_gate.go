package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/skills"
)

type MandatorySkillDecision struct {
	Action         string   `json:"action"` // allow | require_skill_read | warn
	RequiredSkills []string `json:"requiredSkills,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

func EvaluateMandatorySkillActivation(defs []skills.Definition, input, answer string, state SkillActivationSessionState) MandatorySkillDecision {
	required := map[string][]string{}
	for _, def := range defs {
		defName := strings.TrimSpace(def.Name)
		if defName == "" || !def.Discovery.RequiredForMatch {
			continue
		}
		if _, loaded := state.LoadedSkills[defName]; loaded {
			continue
		}
		if reasons := mandatorySkillMatchReasons(def, input); len(reasons) > 0 {
			required[defName] = reasons
		}
	}
	for _, match := range state.LastSearchResults {
		name := strings.TrimSpace(match.Name)
		if name == "" || !match.RequiredForMatch {
			continue
		}
		if _, loaded := state.LoadedSkills[name]; loaded {
			continue
		}
		if reasons := mandatorySearchMatchReasons(match, input); len(reasons) > 0 {
			required[name] = reasons
		}
	}
	if len(required) == 0 {
		return MandatorySkillDecision{Action: "allow"}
	}
	names := make([]string, 0, len(required))
	reasonSet := map[string]bool{}
	for name, reasons := range required {
		names = append(names, name)
		for _, reason := range reasons {
			reasonSet[reason] = true
		}
	}
	sort.Strings(names)
	reasons := make([]string, 0, len(reasonSet))
	for reason := range reasonSet {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	if answerClaimsFinalCertainty(answer) {
		return MandatorySkillDecision{Action: "require_skill_read", RequiredSkills: names, Reasons: reasons}
	}
	return MandatorySkillDecision{Action: "warn", RequiredSkills: names, Reasons: reasons}
}

func mandatorySkillMatchReasons(def skills.Definition, input string) []string {
	text := strings.ToLower(input)
	var reasons []string
	for _, intent := range def.Discovery.TaskIntents {
		if containsFoldToken(text, intent) {
			reasons = append(reasons, "task_intent_match")
			break
		}
	}
	for _, resourceType := range def.Discovery.ResourceTypes {
		if containsFoldToken(text, resourceType) {
			reasons = append(reasons, "resource_type_match")
			break
		}
	}
	for _, path := range def.Discovery.Paths {
		needle := strings.Trim(strings.ToLower(path), "*")
		if needle != "" && strings.Contains(text, needle) {
			reasons = append(reasons, "path_match")
			break
		}
	}
	if matchesSkillText(def.Discovery.WhenToUse, text) || matchesSkillText(def.Description, text) {
		reasons = append(reasons, "when_to_use_match")
	}
	return uniqueSortedReasons(reasons)
}

func mandatorySearchMatchReasons(match SkillSearchMatchSnapshot, input string) []string {
	text := strings.ToLower(input)
	var reasons []string
	for _, intent := range match.TaskIntents {
		if containsFoldToken(text, intent) {
			reasons = append(reasons, "task_intent_match")
			break
		}
	}
	for _, resourceType := range match.ResourceTypes {
		if containsFoldToken(text, resourceType) {
			reasons = append(reasons, "resource_type_match")
			break
		}
	}
	if matchesSkillText(match.WhenToUse, text) || matchesSkillText(match.Description, text) {
		reasons = append(reasons, "when_to_use_match")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "search_result_match")
	}
	return uniqueSortedReasons(reasons)
}

func answerClaimsFinalCertainty(answer string) bool {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return false
	}
	for _, marker := range []string{
		"root cause", "definitely", "confirmed", "final answer", "结论", "根因", "确定", "确认",
	} {
		if strings.Contains(answer, marker) {
			return true
		}
	}
	return true
}

func mandatorySkillRetryPrompt(decision MandatorySkillDecision) string {
	if decision.Action != "require_skill_read" || len(decision.RequiredSkills) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"## Mandatory skill activation retry\nBefore finalizing, call skill_search if needed, then skill_read for required skill(s): %s. Reasons: %s. Do not provide a high-confidence final answer until the required skill body is loaded or explain why it cannot be loaded.",
		strings.Join(decision.RequiredSkills, ", "),
		strings.Join(decision.Reasons, ", "),
	)
}

func containsFoldToken(text, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	return needle != "" && strings.Contains(text, needle)
}

func matchesSkillText(value, inputLower string) bool {
	terms := strings.Fields(strings.ToLower(value))
	matches := 0
	for _, term := range terms {
		term = strings.Trim(term, ".,;:!?()[]{}\"'")
		if len([]rune(term)) < 3 {
			continue
		}
		if strings.Contains(inputLower, term) {
			matches++
		}
		if matches >= 2 {
			return true
		}
	}
	return false
}

func uniqueSortedReasons(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
