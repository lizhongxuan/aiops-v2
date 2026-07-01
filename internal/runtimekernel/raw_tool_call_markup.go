package runtimekernel

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	rawDSMLInvokePattern = regexp.MustCompile(`(?s)<｜｜DSML｜｜invoke\s+name="([^"]+)">(.*?)</｜｜DSML｜｜invoke>`)
	rawDSMLParamPattern  = regexp.MustCompile(`(?s)<｜｜DSML｜｜parameter\s+name="([^"]+)"(?:\s+string="([^"]+)")?>(.*?)</｜｜DSML｜｜parameter>`)
)

func rawToolCallsFromAssistantText(text, turnID string, iteration int) []ToolCall {
	if !containsRawToolCallMarkup(strings.ToLower(text)) {
		return nil
	}
	matches := rawDSMLInvokePattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(matches))
	for idx, match := range matches {
		if len(match) < 3 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		args := rawDSMLParamsToMap(match[2])
		rawArgs, err := json.Marshal(args)
		if err != nil {
			continue
		}
		calls = append(calls, ToolCall{
			ID:        fmt.Sprintf("%s-raw-dsml-%d-%d", strings.TrimSpace(turnID), iteration, idx),
			Name:      name,
			Arguments: rawArgs,
		})
	}
	return calls
}

func rawDSMLParamsToMap(body string) map[string]any {
	out := map[string]any{}
	for _, match := range rawDSMLParamPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 4 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		stringAttr := strings.ToLower(strings.TrimSpace(match[2]))
		value := strings.TrimSpace(match[3])
		if stringAttr == "false" {
			out[name] = parseRawDSMLScalar(value)
			continue
		}
		out[name] = value
	}
	return out
}

func parseRawDSMLScalar(value string) any {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return value
}
