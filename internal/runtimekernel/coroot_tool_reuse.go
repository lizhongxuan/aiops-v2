package runtimekernel

import (
	"encoding/json"
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

const corootReuseSkipReason = "covered_by_prior_broad_query"

type coveredReadBatchReuse struct {
	index      int
	priorIndex int
}

func maybeReuseCoveredReadResult(snapshot *TurnSnapshot, tools []promptcompiler.Tool, tc ToolCall) (DispatchResult, bool) {
	toolName := canonicalRuntimeToolName(tools, tc)
	switch toolName {
	case "coroot.list_services":
		return maybeReuseCorootListServicesResult(snapshot, tools, tc)
	case "coroot.incidents":
		return maybeReuseCorootIncidentsResult(snapshot, tools, tc)
	default:
		return DispatchResult{}, false
	}
}

func broadenCoveredReadBatch(tools []promptcompiler.Tool, batch []ToolCall) []ToolCall {
	if len(batch) < 2 {
		return batch
	}
	out := append([]ToolCall(nil), batch...)
	listServiceStatusByProject := map[string][]int{}
	hasBroadListServices := map[string]bool{}
	for i, call := range out {
		if canonicalRuntimeToolName(tools, call) != "coroot.list_services" {
			continue
		}
		args := decodeToolCallObjectArgs(call.Arguments)
		projectKey := strings.ToLower(argString(args, "project"))
		if argsOnlyContainMeaningfulFields(args, "project") {
			hasBroadListServices[projectKey] = true
			continue
		}
		if argString(args, "status") != "" && argsOnlyContainMeaningfulFields(args, "project", "status") {
			listServiceStatusByProject[projectKey] = append(listServiceStatusByProject[projectKey], i)
		}
	}
	for projectKey, indexes := range listServiceStatusByProject {
		if hasBroadListServices[projectKey] || len(indexes) < 2 {
			continue
		}
		first := indexes[0]
		args := decodeToolCallObjectArgs(out[first].Arguments)
		out[first].Arguments = projectOnlyArguments(args)
	}
	return out
}

func maybeReuseCorootListServicesResult(snapshot *TurnSnapshot, tools []promptcompiler.Tool, tc ToolCall) (DispatchResult, bool) {
	current := decodeToolCallObjectArgs(tc.Arguments)
	requestedStatus := strings.TrimSpace(argString(current, "status"))
	if requestedStatus == "" || !argsOnlyContainMeaningfulFields(current, "project", "status") {
		return DispatchResult{}, false
	}
	priorCall, priorResult, ok := findPriorCorootResult(snapshot, tools, tc, "coroot.list_services", func(call ToolCall) bool {
		args := decodeToolCallObjectArgs(call.Arguments)
		if !sameOptionalProject(current, args) {
			return false
		}
		return argsOnlyContainMeaningfulFields(args, "project")
	})
	if !ok {
		return DispatchResult{}, false
	}
	return corootReusedDispatchResult(tools, tc, priorCall, priorResult, map[string]any{
		"requestedStatus": requestedStatus,
		"modelGuidance":   "A prior broad coroot.list_services call in this turn already returned statusCounts and problemServices. Use that prior result; do not call coroot.list_services again just to split warning or critical services.",
	}), true
}

func maybeReuseCorootIncidentsResult(snapshot *TurnSnapshot, tools []promptcompiler.Tool, tc ToolCall) (DispatchResult, bool) {
	current := decodeToolCallObjectArgs(tc.Arguments)
	if !corootIncidentsArgsCoveredByBroad(current) {
		return DispatchResult{}, false
	}
	priorCall, priorResult, ok := findPriorCorootResult(snapshot, tools, tc, "coroot.incidents", func(call ToolCall) bool {
		args := decodeToolCallObjectArgs(call.Arguments)
		if !sameOptionalProject(current, args) {
			return false
		}
		return argsOnlyContainMeaningfulFields(args, "project", "limit")
	})
	if !ok {
		return DispatchResult{}, false
	}
	return corootReusedDispatchResult(tools, tc, priorCall, priorResult, map[string]any{
		"modelGuidance": "A prior broad coroot.incidents call in this turn already returned the recent incidents list. Use that prior result; do not call coroot.incidents again only to change limit or showResolved while answering the same broad anomaly question.",
	}), true
}

func coveredReadReusePriorIndex(tools []promptcompiler.Tool, prior []ToolCall, current ToolCall) (int, bool) {
	if len(prior) == 0 {
		return -1, false
	}
	toolName := canonicalRuntimeToolName(tools, current)
	currentArgs := decodeToolCallObjectArgs(current.Arguments)
	for i := len(prior) - 1; i >= 0; i-- {
		priorCall := prior[i]
		if canonicalRuntimeToolName(tools, priorCall) != toolName {
			continue
		}
		priorArgs := decodeToolCallObjectArgs(priorCall.Arguments)
		if !sameOptionalProject(currentArgs, priorArgs) {
			continue
		}
		switch toolName {
		case "coroot.list_services":
			if argString(currentArgs, "status") != "" &&
				argsOnlyContainMeaningfulFields(currentArgs, "project", "status") &&
				argsOnlyContainMeaningfulFields(priorArgs, "project") {
				return i, true
			}
		case "coroot.incidents":
			if corootIncidentsArgsCoveredByBroad(currentArgs) &&
				argsOnlyContainMeaningfulFields(priorArgs, "project", "limit") {
				return i, true
			}
		}
	}
	return -1, false
}

func coveredReadReuseFromBatchResult(tools []promptcompiler.Tool, batch []ToolCall, results []DispatchResult, reuse coveredReadBatchReuse) (DispatchResult, bool) {
	if reuse.index < 0 || reuse.index >= len(batch) || reuse.index >= len(results) ||
		reuse.priorIndex < 0 || reuse.priorIndex >= len(batch) || reuse.priorIndex >= len(results) {
		return DispatchResult{}, false
	}
	current := batch[reuse.index]
	priorCall := batch[reuse.priorIndex]
	priorDispatch := results[reuse.priorIndex]
	priorResult := priorDispatch.Result
	if strings.TrimSpace(priorDispatch.Error) != "" || strings.TrimSpace(priorResult.Error) != "" {
		return DispatchResult{}, false
	}
	content := strings.TrimSpace(priorResult.Content)
	if content == "" || !corootResultLooksUsable(content, canonicalRuntimeToolName(tools, current)) {
		return DispatchResult{}, false
	}
	toolResult := ToolResult{
		ToolCallID: firstNonBlankRuntimeString(priorResult.ToolCallID, priorCall.ID),
		Content:    content,
		Error:      priorResult.Error,
	}
	return corootReusedDispatchResult(tools, current, priorCall, toolResult, map[string]any{
		"modelGuidance": "A prior broad read call in this same tool batch already returned the data needed for this narrower filter. Use that prior result; do not call the same read tool again only to split or narrow data already represented there.",
	}), true
}

func findPriorCorootResult(
	snapshot *TurnSnapshot,
	tools []promptcompiler.Tool,
	current ToolCall,
	canonicalName string,
	callMatches func(ToolCall) bool,
) (ToolCall, ToolResult, bool) {
	if snapshot == nil || callMatches == nil {
		return ToolCall{}, ToolResult{}, false
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		iteration := snapshot.Iterations[i]
		callsByID := make(map[string]ToolCall, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			callsByID[strings.TrimSpace(call.ID)] = call
		}
		for j := len(iteration.ToolResults) - 1; j >= 0; j-- {
			result := iteration.ToolResults[j]
			if strings.TrimSpace(result.ToolCallID) == "" || strings.TrimSpace(result.ToolCallID) == strings.TrimSpace(current.ID) {
				continue
			}
			if strings.TrimSpace(result.Error) != "" || !corootResultLooksUsable(result.Content, canonicalName) {
				continue
			}
			call, ok := callsByID[strings.TrimSpace(result.ToolCallID)]
			if !ok || canonicalRuntimeToolName(tools, call) != canonicalName || !callMatches(call) {
				continue
			}
			return call, result, true
		}
	}
	return ToolCall{}, ToolResult{}, false
}

func corootReusedDispatchResult(tools []promptcompiler.Tool, tc ToolCall, priorCall ToolCall, priorResult ToolResult, fields map[string]any) DispatchResult {
	toolName := canonicalRuntimeToolName(tools, tc)
	if toolName == "" {
		toolName = strings.TrimSpace(tc.Name)
	}
	payload := map[string]any{
		"schemaVersion":        "aiops.coroot/v1",
		"tool":                 toolName,
		"status":               "reused",
		"skipReason":           corootReuseSkipReason,
		"reusedFromToolCallId": strings.TrimSpace(priorResult.ToolCallID),
		"reusedFromToolName":   canonicalRuntimeToolName(tools, priorCall),
		"source":               "coroot",
		"evidenceRefs":         []string{},
	}
	for key, value := range fields {
		payload[key] = value
	}
	content, _ := json.Marshal(payload)
	result := tooling.ToolResult{
		ToolCallID: tc.ID,
		Content:    string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "coroot.reuse",
			Title: toolName,
			Data:  append(json.RawMessage(nil), content...),
		},
	}
	return DispatchResult{
		ToolCallID: tc.ID,
		Content:    result.Content,
		Metadata:   toolMetadataForToolCall(tools, tc),
		Result:     result,
		Outcome:    "tool_reused",
		Source:     "runtime",
	}
}

func corootResultLooksUsable(content string, canonicalName string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	if corootJSONResultLooksUsable(content, canonicalName) {
		return true
	}
	if summary := materializedResultSummary(content); summary != "" && corootJSONResultLooksUsable(summary, canonicalName) {
		return true
	}
	lower := strings.ToLower(content)
	if strings.Contains(lower, `"type":"tool_error"`) || strings.Contains(lower, "tool_error") {
		return false
	}
	return strings.Contains(lower, "external ref:") && strings.Contains(lower, "store://tool-spills/")
}

func corootJSONResultLooksUsable(content string, canonicalName string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &payload); err != nil {
		return false
	}
	if strings.TrimSpace(anyString(payload["type"])) == "tool_error" {
		return false
	}
	if tool := strings.TrimSpace(anyString(payload["tool"])); tool != "" && tool != canonicalName {
		return false
	}
	status := strings.TrimSpace(anyString(payload["status"]))
	if status == "" {
		return len(payload) > 0
	}
	return status == "ok" || status == "healthy" || status == "partial"
}

func materializedResultSummary(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "Summary:") {
		return ""
	}
	content = strings.TrimSpace(strings.TrimPrefix(content, "Summary:"))
	if idx := strings.Index(content, "\nExternal ref:"); idx >= 0 {
		content = content[:idx]
	}
	return strings.TrimSpace(content)
}

func isCoveredReadReuseResult(result ToolResult) bool {
	if result.Display != nil && strings.EqualFold(strings.TrimSpace(result.Display.Type), "coroot.reuse") {
		return true
	}
	return strings.Contains(result.Content, corootReuseSkipReason)
}

func canonicalRuntimeToolName(tools []promptcompiler.Tool, tc ToolCall) string {
	if toolDef := toolForToolCall(tools, tc); toolDef != nil {
		return strings.TrimSpace(toolDef.Metadata().Name)
	}
	name := strings.TrimSpace(tc.Name)
	switch tooling.ProviderSafeToolName(name) {
	case "coroot_list_services":
		return "coroot.list_services"
	case "coroot_incidents":
		return "coroot.incidents"
	default:
		return name
	}
}

func decodeToolCallObjectArgs(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return map[string]json.RawMessage{}
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return map[string]json.RawMessage{}
	}
	return args
}

func argsOnlyContainMeaningfulFields(args map[string]json.RawMessage, allowed ...string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key, value := range args {
		if !argValueMeaningful(value) {
			continue
		}
		if _, ok := allowedSet[key]; !ok {
			return false
		}
	}
	return true
}

func argValueMeaningful(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == `""` || trimmed == "0" {
		return false
	}
	if trimmed == "[]" || trimmed == "{}" {
		return false
	}
	return true
}

func argString(args map[string]json.RawMessage, key string) string {
	raw, ok := args[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func argInt(args map[string]json.RawMessage, key string) int {
	raw, ok := args[key]
	if !ok {
		return 0
	}
	var value int
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return 0
}

func projectOnlyArguments(args map[string]json.RawMessage) json.RawMessage {
	projectRaw, ok := args["project"]
	if !ok || !argValueMeaningful(projectRaw) {
		return json.RawMessage(`{}`)
	}
	payload := map[string]json.RawMessage{"project": projectRaw}
	data, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(data)
}

func sameOptionalProject(a, b map[string]json.RawMessage) bool {
	left := argString(a, "project")
	right := argString(b, "project")
	return left == "" || right == "" || strings.EqualFold(left, right)
}

func corootIncidentsArgsCoveredByBroad(args map[string]json.RawMessage) bool {
	if !argsOnlyContainMeaningfulFields(args, "project", "limit", "showResolved", "status") {
		return false
	}
	if limit := argInt(args, "limit"); limit > 50 {
		return false
	}
	status := strings.ToLower(argString(args, "status"))
	return status == "" || status == "open"
}
