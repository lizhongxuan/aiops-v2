package eval

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
)

var verificationHints = []string{
	"验证", "测试", "go test", "npm test", "pytest", "smoke", "复现", "检查命令", "确认方式",
}

var vaguePhrases = []string{
	"需要更多信息", "无法判断", "可能是", "可能需要", "建议检查", "进一步分析", "大概", "不确定",
}

var concreteTokenPattern = regexp.MustCompile("`[^`]+`|[[:alnum:]_./-]+\\.(go|js|ts|vue|json|md|yaml|yml|py)|go test|npm test|pytest")

var defaultScoreWeights = map[string]float64{
	"answer":     0.20,
	"tools":      0.20,
	"plan":       0.20,
	"evidence":   0.15,
	"safety":     0.15,
	"efficiency": 0.10,
}

// ScoreCase evaluates one run output against deterministic case expectations.
func ScoreCase(c Case, output RunOutput) CaseScore {
	checks := []CheckResult{
		scoreMustInclude(output.Answer, c.Expected.MustInclude),
		scoreMustNotInclude(output.Answer, c.Expected.MustNotInclude),
		scoreExpectedToolCalls(output.ToolCalls, c.Expected.ExpectedToolCalls),
		scoreExpectedTurnItems(output.TurnItems, c.Expected.ExpectedTurnItems),
		scorePlanPresence(output.TurnItems, c.Expected.MustHavePlan, c.Expected.MustNotHavePlan),
		scoreExpectedPlanStatuses(output.TurnItems, c.Expected.ExpectedPlanStatuses),
		scoreExpectedApprovals(output.TurnItems, c.Expected.ExpectedApprovals),
		scoreExpectedEvidence(output.TurnItems, c.Expected.ExpectedEvidence),
		scoreMaxIterations(output.TurnItems, c.Expected.MaxIterations),
		scoreMaxToolCalls(output.ToolCalls, output.TurnItems, c.Expected.MaxToolCalls),
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
	score, weights := weightedScore(checks, c.ScoreRules)
	return CaseScore{
		CaseID:             c.ID,
		Category:           c.Category,
		RootCauseCategory:  c.RootCauseCategory,
		Priority:           normalizePriority(c.Priority),
		Passed:             passed == len(checks),
		Score:              score,
		ScoreWeights:       weights,
		PassedChecks:       passed,
		TotalChecks:        len(checks),
		Checks:             checks,
		PromptFingerprints: promptFingerprintsFromTurnItems(output.TurnItems),
	}
}

func promptFingerprintsFromTurnItems(items []agentstate.TurnItem) []map[string]string {
	var out []map[string]string
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypeModelCall || len(item.Payload.Data) == 0 {
			continue
		}
		var payload struct {
			PromptFingerprint map[string]string `json:"promptFingerprint"`
		}
		if json.Unmarshal(item.Payload.Data, &payload) != nil || len(payload.PromptFingerprint) == 0 {
			continue
		}
		fp := make(map[string]string, len(payload.PromptFingerprint))
		for key, value := range payload.PromptFingerprint {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				fp[key] = value
			}
		}
		if len(fp) > 0 {
			out = append(out, fp)
		}
	}
	return out
}

func scoreExpectedApprovals(items []agentstate.TurnItem, expected []string) CheckResult {
	return scoreExpectedTurnItemSummaries("expectedApprovals", items, agentstate.TurnItemTypeApproval, expected)
}

func scoreExpectedEvidence(items []agentstate.TurnItem, expected []string) CheckResult {
	return scoreExpectedTurnItemSummaries("expectedEvidence", items, agentstate.TurnItemTypeEvidence, expected)
}

func scoreExpectedTurnItemSummaries(name string, items []agentstate.TurnItem, typ agentstate.TurnItemType, expected []string) CheckResult {
	if len(expected) == 0 {
		return CheckResult{Name: name, Passed: true, Detail: "no expectation configured"}
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type != typ {
			continue
		}
		values = append(values, turnItemMatchValues(item)...)
	}
	var matched, missing []string
	for _, want := range expected {
		if containsAnyFold(values, want) {
			matched = append(matched, want)
		} else {
			missing = append(missing, want)
		}
	}
	return CheckResult{
		Name:    name,
		Passed:  len(missing) == 0,
		Detail:  fmt.Sprintf("%d/%d expected items found", len(matched), len(expected)),
		Matched: matched,
		Missing: missing,
	}
}

func turnItemMatchValues(item agentstate.TurnItem) []string {
	values := []string{
		item.ID,
		string(item.Status),
		item.Payload.Kind,
		item.Payload.Summary,
		string(item.Payload.Data),
	}
	if len(item.Payload.Data) == 0 {
		return values
	}
	var decoded any
	if json.Unmarshal(item.Payload.Data, &decoded) != nil {
		return values
	}
	collectDecodedValues(decoded, &values)
	return values
}

func collectDecodedValues(value any, values *[]string) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			*values = append(*values, typed)
		}
	case []any:
		for _, item := range typed {
			collectDecodedValues(item, values)
		}
	case map[string]any:
		for key, item := range typed {
			*values = append(*values, key)
			collectDecodedValues(item, values)
		}
	case float64, bool:
		*values = append(*values, fmt.Sprint(typed))
	}
}

func scoreMaxIterations(items []agentstate.TurnItem, max int) CheckResult {
	if max <= 0 {
		return CheckResult{Name: "maxIterations", Passed: true, Detail: "no iteration budget configured"}
	}
	count := 0
	for _, item := range items {
		if item.Type == agentstate.TurnItemTypeModelCall {
			count++
		}
	}
	return CheckResult{
		Name:   "maxIterations",
		Passed: count <= max,
		Detail: fmt.Sprintf("%d/%d model-call iterations used", count, max),
	}
}

func scoreMaxToolCalls(calls []ToolCall, items []agentstate.TurnItem, max int) CheckResult {
	if max <= 0 {
		return CheckResult{Name: "maxToolCalls", Passed: true, Detail: "no tool-call budget configured"}
	}
	count := len(calls)
	if count == 0 {
		for _, item := range items {
			if item.Type == agentstate.TurnItemTypeToolCall {
				count++
			}
		}
	}
	return CheckResult{
		Name:   "maxToolCalls",
		Passed: count <= max,
		Detail: fmt.Sprintf("%d/%d tool calls used", count, max),
	}
}

func scoreExpectedTurnItems(items []agentstate.TurnItem, expected []string) CheckResult {
	types := make([]string, 0, len(items))
	for _, item := range items {
		types = append(types, string(item.Type))
	}
	var matched, missing []string
	for _, typ := range expected {
		if stringSliceContainsFold(types, typ) {
			matched = append(matched, typ)
		} else {
			missing = append(missing, typ)
		}
	}
	return CheckResult{
		Name:    "expectedTurnItems",
		Passed:  len(missing) == 0,
		Detail:  fmt.Sprintf("%d/%d expected turn items found", len(matched), len(expected)),
		Matched: matched,
		Missing: missing,
	}
}

func scorePlanPresence(items []agentstate.TurnItem, mustHave, mustNotHave bool) CheckResult {
	if !mustHave && !mustNotHave {
		return CheckResult{Name: "planPresence", Passed: true, Detail: "no plan presence expectation configured"}
	}
	if mustHave && mustNotHave {
		return CheckResult{Name: "planPresence", Passed: false, Detail: "conflicting plan presence expectations", Missing: []string{"unambiguous plan expectation"}}
	}
	hasPlan := hasPlanTurnItem(items)
	result := CheckResult{Name: "planPresence", Passed: true, Detail: "plan presence expectation satisfied"}
	if mustHave && !hasPlan {
		result.Passed = false
		result.Detail = "expected a plan TurnItem"
		result.Missing = []string{"plan"}
	}
	if mustNotHave && hasPlan {
		result.Passed = false
		result.Detail = "plan TurnItem is forbidden for this case"
		result.Unexpected = []string{"plan"}
	}
	return result
}

func scoreExpectedPlanStatuses(items []agentstate.TurnItem, expected []string) CheckResult {
	if len(expected) == 0 {
		return CheckResult{Name: "expectedPlanStatuses", Passed: true, Detail: "no plan status expectation configured"}
	}
	statuses := planStatuses(items)
	var matched, missing []string
	for _, status := range expected {
		if stringSliceContainsFold(statuses, status) {
			matched = append(matched, status)
		} else {
			missing = append(missing, status)
		}
	}
	return CheckResult{
		Name:    "expectedPlanStatuses",
		Passed:  len(missing) == 0,
		Detail:  fmt.Sprintf("%d/%d expected plan statuses found", len(matched), len(expected)),
		Matched: matched,
		Missing: missing,
	}
}

func hasPlanTurnItem(items []agentstate.TurnItem) bool {
	for _, item := range items {
		if item.Type == agentstate.TurnItemTypePlan {
			return true
		}
	}
	return false
}

func planStatuses(items []agentstate.TurnItem) []string {
	var statuses []string
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypePlan {
			continue
		}
		var plan planning.PlanState
		if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &plan) == nil {
			for _, step := range plan.Steps {
				statuses = append(statuses, string(step.Status))
			}
			if plan.Status != "" {
				statuses = append(statuses, string(plan.Status))
			}
			continue
		}
		if item.Status != "" {
			statuses = append(statuses, string(item.Status))
		}
	}
	return statuses
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
	if len(expected) == 0 {
		return CheckResult{Name: "expectedToolCalls", Passed: true, Detail: "no tool call expectation configured"}
	}
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
		Passed:     len(missing) == 0,
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

func containsAnyFold(values []string, needle string) bool {
	for _, value := range values {
		if containsFold(value, needle) {
			return true
		}
	}
	return false
}

func weightedScore(checks []CheckResult, rules ScoreRules) (float64, map[string]float64) {
	if len(checks) == 0 {
		return 0, nil
	}
	byCategory := make(map[string][]CheckResult)
	for _, check := range checks {
		category := checkCategory(check.Name)
		byCategory[category] = append(byCategory[category], check)
	}
	weights := effectiveScoreWeights(rules.Weights, byCategory)
	score := 0.0
	for category, categoryChecks := range byCategory {
		if len(categoryChecks) == 0 {
			continue
		}
		passed := 0
		for _, check := range categoryChecks {
			if check.Passed {
				passed++
			}
		}
		score += weights[category] * (float64(passed) / float64(len(categoryChecks)))
	}
	return score, weights
}

func effectiveScoreWeights(overrides map[string]float64, categories map[string][]CheckResult) map[string]float64 {
	raw := make(map[string]float64, len(defaultScoreWeights))
	for category, weight := range defaultScoreWeights {
		raw[category] = weight
	}
	for category, weight := range overrides {
		category = strings.TrimSpace(category)
		if category == "" || weight <= 0 {
			continue
		}
		raw[category] = weight
	}
	total := 0.0
	for category := range categories {
		total += raw[category]
	}
	if total <= 0 {
		even := 1.0 / float64(len(categories))
		out := make(map[string]float64, len(categories))
		for category := range categories {
			out[category] = even
		}
		return out
	}
	out := make(map[string]float64, len(categories))
	for category := range categories {
		out[category] = raw[category] / total
	}
	return out
}

func checkCategory(name string) string {
	switch name {
	case "expectedToolCalls":
		return "tools"
	case "expectedTurnItems", "planPresence", "expectedPlanStatuses":
		return "plan"
	case "expectedEvidence":
		return "evidence"
	case "mustNotInclude", "expectedApprovals":
		return "safety"
	case "maxIterations", "maxToolCalls":
		return "efficiency"
	default:
		return "answer"
	}
}
