package eval

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var verificationHints = []string{
	"验证", "测试", "go test", "npm test", "pytest", "smoke", "复现", "检查命令", "确认方式",
}

var vaguePhrases = []string{
	"需要更多信息", "无法判断", "可能是", "可能需要", "建议检查", "进一步分析", "大概", "不确定",
}

var concreteTokenPattern = regexp.MustCompile("`[^`]+`|[[:alnum:]_./-]+\\.(go|js|ts|vue|json|md|yaml|yml|py)|go test|npm test|pytest")

// ScoreCase evaluates one run output against deterministic case expectations.
func ScoreCase(c Case, output RunOutput) CaseScore {
	checks := []CheckResult{
		scoreMustInclude(output.Answer, c.Expected.MustInclude),
		scoreMustNotInclude(output.Answer, c.Expected.MustNotInclude),
		scoreExpectedToolCalls(output.ToolCalls, c.Expected.ExpectedToolCalls),
		scoreMustMentionFiles(output.Answer, c.Expected.MustMentionFiles),
		scoreNotVague(output.Answer),
		scoreHasVerification(output.Answer),
	}

	passed := 0
	for _, check := range checks {
		if check.Passed {
			passed++
		}
	}
	score := 0.0
	if len(checks) > 0 {
		score = float64(passed) / float64(len(checks))
	}
	return CaseScore{
		CaseID:       c.ID,
		Category:     c.Category,
		Passed:       passed == len(checks),
		Score:        score,
		PassedChecks: passed,
		TotalChecks:  len(checks),
		Checks:       checks,
	}
}

func scoreMustInclude(answer string, expected []string) CheckResult {
	matched, missing := matchAll(answer, expected)
	return CheckResult{
		Name:    "mustInclude",
		Passed:  len(missing) == 0,
		Detail:  fmt.Sprintf("%d/%d required snippets matched", len(matched), len(expected)),
		Matched: matched,
		Missing: missing,
	}
}

func scoreMustNotInclude(answer string, forbidden []string) CheckResult {
	var present []string
	for _, item := range forbidden {
		if containsFold(answer, item) {
			present = append(present, item)
		}
	}
	return CheckResult{
		Name:    "mustNotInclude",
		Passed:  len(present) == 0,
		Detail:  fmt.Sprintf("%d forbidden snippets found", len(present)),
		Matched: present,
	}
}

func scoreExpectedToolCalls(calls []ToolCall, expected []string) CheckResult {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	var matched, missing []string
	for _, name := range expected {
		if stringSliceContainsFold(names, name) {
			matched = append(matched, name)
		} else {
			missing = append(missing, name)
		}
	}
	var unexpected []string
	for _, name := range names {
		if !stringSliceContainsFold(expected, name) {
			unexpected = append(unexpected, name)
		}
	}
	return CheckResult{
		Name:       "expectedToolCalls",
		Passed:     len(missing) == 0 && len(unexpected) == 0,
		Detail:     fmt.Sprintf("%d/%d expected tools called, %d unexpected tools", len(matched), len(expected), len(unexpected)),
		Matched:    matched,
		Missing:    missing,
		Unexpected: unexpected,
	}
}

func scoreMustMentionFiles(answer string, files []string) CheckResult {
	matched, missing := matchAll(answer, files)
	return CheckResult{
		Name:    "mustMentionFiles",
		Passed:  len(missing) == 0,
		Detail:  fmt.Sprintf("%d/%d expected files mentioned", len(matched), len(files)),
		Matched: matched,
		Missing: missing,
	}
}

func scoreNotVague(answer string) CheckResult {
	vague := IsVagueAnswer(answer)
	return CheckResult{
		Name:   "notVague",
		Passed: !vague,
		Detail: map[bool]string{
			true:  "answer has enough concrete detail",
			false: "answer is empty, too short, or only generic guidance",
		}[!vague],
	}
}

func scoreHasVerification(answer string) CheckResult {
	for _, hint := range verificationHints {
		if containsFold(answer, hint) {
			return CheckResult{Name: "hasVerification", Passed: true, Detail: "verification method found", Matched: []string{hint}}
		}
	}
	return CheckResult{Name: "hasVerification", Passed: false, Detail: "missing an explicit verification method"}
}

// IsVagueAnswer detects empty or low-information answers with a conservative heuristic.
func IsVagueAnswer(answer string) bool {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return true
	}
	if utf8.RuneCountInString(trimmed) < 40 {
		return true
	}
	containsVaguePhrase := false
	for _, phrase := range vaguePhrases {
		if containsFold(trimmed, phrase) {
			containsVaguePhrase = true
			break
		}
	}
	if containsVaguePhrase && !concreteTokenPattern.MatchString(trimmed) {
		return true
	}
	return false
}

func matchAll(answer string, expected []string) ([]string, []string) {
	var matched, missing []string
	for _, item := range expected {
		if containsFold(answer, item) {
			matched = append(matched, item)
		} else {
			missing = append(missing, item)
		}
	}
	return matched, missing
}

func containsFold(haystack, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func stringSliceContainsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}
