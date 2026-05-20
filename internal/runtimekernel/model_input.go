package runtimekernel

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type ModelInputDebugTraceRequest struct {
	SessionID        string
	TurnID           string
	Iteration        int
	Metadata         map[string]string
	Compiled         promptcompiler.CompiledPrompt
	ModelInput       []*schema.Message
	VisibleTools     []string
	PromptInputTrace promptinput.PromptInputTrace
	PromptInputDiff  *promptinput.TraceDiff
	DiagnosticTrace  diagnostics.DiagnosticTrace
}

func buildModelInput(history []Message, compiled promptcompiler.CompiledPrompt) ([]*schema.Message, error) {
	result, err := buildPromptInput(history, compiled)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func buildPromptInput(history []Message, compiled promptcompiler.CompiledPrompt) (promptinput.BuildResult, error) {
	result, err := promptinput.Builder{}.Build(promptinput.BuildRequest{
		History:  promptInputMessagesFromRuntime(history),
		Compiled: compiled,
	})
	if err != nil {
		return promptinput.BuildResult{}, err
	}
	return result, nil
}

func messagesForCurrentTurnModelInput(history []Message) []Message {
	filtered := promptinput.MessagesForCurrentTurnModelInput(promptInputMessagesFromRuntime(history))
	return runtimeMessagesFromPromptInput(filtered)
}

func promptInputMessagesFromRuntime(history []Message) []promptinput.Message {
	out := make([]promptinput.Message, 0, len(history))
	for _, msg := range history {
		content := msg.Content
		if msg.Role == "tool" {
			content = compactCorootServiceMetricsForModel(content)
		}
		toolResult := promptInputToolResultFromRuntime(msg.ToolResult)
		if toolResult != nil {
			toolResult.Content = compactCorootServiceMetricsForModel(toolResult.Content)
		}
		out = append(out, promptinput.Message{
			Role:       msg.Role,
			Content:    content,
			ToolCalls:  promptInputToolCallsFromRuntime(msg.ToolCalls),
			ToolResult: toolResult,
		})
	}
	return out
}

func compactCorootServiceMetricsForModel(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return content
	}
	if strings.TrimSpace(runtimeStringFromMap(payload, "tool")) != "coroot.service_metrics" {
		return content
	}
	chartSummary := runtimeCorootChartSummaryFromPayload(payload)
	if len(chartSummary) == 0 {
		return content
	}
	out := map[string]any{
		"schemaVersion": "aiops.coroot_chart_summary/v1",
		"tool":          "coroot.service_metrics",
		"chartSummary":  chartSummary,
	}
	for _, key := range []string{"status", "project", "service", "source"} {
		if value := runtimeStringFromMap(payload, key); value != "" {
			out[key] = value
		}
	}
	if rawRef := runtimeStringAnyMap(payload["rawRef"]); len(rawRef) > 0 {
		compactRef := map[string]any{}
		for _, key := range []string{"uri", "digest", "bytes"} {
			if value, ok := rawRef[key]; ok {
				compactRef[key] = value
			}
		}
		if len(compactRef) > 0 {
			out["rawRef"] = compactRef
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return content
	}
	return string(data)
}

func runtimeCorootChartSummaryFromPayload(payload map[string]any) map[string]any {
	summary := runtimeCloneStringAnyMap(runtimeStringAnyMap(payload["chartSummary"]))
	if len(summary) == 0 {
		summary = map[string]any{}
		if metricSummaries := runtimeCorootMetricSummaries(payload["metrics"]); len(metricSummaries) > 0 {
			summary["metricSummaries"] = metricSummaries
		}
		if reports := runtimeCorootReportSummaries(payload["chartReports"]); len(reports) > 0 {
			summary["reports"] = reports
		}
	}
	if service := runtimeStringFromMap(payload, "service"); service != "" {
		summary["service"] = service
	}
	return summary
}

func runtimeCorootMetricSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, metric := range runtimeStringAnyMapList(value) {
		name := runtimeStringFromMap(metric, "name")
		item := map[string]any{
			"name":  name,
			"topic": runtimeCorootTopicFromName(firstNonBlankRuntimeString(name, runtimeStringFromMap(metric, "chartTitle"))),
		}
		for _, key := range []string{"status", "value", "unit", "chartTitle"} {
			if text := runtimeStringFromMap(metric, key); text != "" {
				item[key] = text
			}
		}
		series := runtimeStringAnyMapList(metric["series"])
		if len(series) > 0 {
			item["seriesCount"] = len(series)
			pointCount := 0
			var seriesNames []string
			for _, seriesMap := range series {
				pointCount += len(runtimeAnyList(seriesMap["values"]))
				seriesNames = appendRuntimeUniqueString(seriesNames, runtimeStringFromMap(seriesMap, "name"), 5)
			}
			if pointCount > 0 {
				item["pointCount"] = pointCount
			}
			if len(seriesNames) > 0 {
				item["seriesNames"] = seriesNames
			}
		} else if pointCount := len(runtimeAnyList(metric["values"])); pointCount > 0 {
			item["seriesCount"] = 1
			item["pointCount"] = pointCount
		}
		out = append(out, item)
	}
	return out
}

func runtimeCorootReportSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, report := range runtimeStringAnyMapList(value) {
		name := runtimeStringFromMap(report, "name")
		item := map[string]any{
			"name":  name,
			"topic": runtimeCorootTopicFromName(name),
		}
		if status := runtimeStringFromMap(report, "status"); status != "" {
			item["status"] = status
		}
		chartCount := 0
		seriesCount := 0
		pointCount := 0
		var titles []string
		var seriesNames []string
		for _, widget := range runtimeStringAnyMapList(report["widgets"]) {
			if chart := runtimeStringAnyMap(widget["chart"]); len(chart) > 0 {
				chartCount++
				title := firstNonBlankRuntimeString(runtimeStringFromMap(widget, "title"), runtimeStringFromMap(chart, "title"))
				titles = appendRuntimeUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = runtimeCorootTopicFromName(title)
				}
				sc, pc, names := runtimeCorootSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendRuntimeUniqueString(seriesNames, name, 5)
				}
			}
			group := runtimeStringAnyMap(widget["chart_group"])
			if len(group) == 0 {
				continue
			}
			groupTitle := runtimeStringFromMap(group, "title")
			for _, chart := range runtimeStringAnyMapList(group["charts"]) {
				chartCount++
				title := firstNonBlankRuntimeString(groupTitle, runtimeStringFromMap(chart, "title"))
				titles = appendRuntimeUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = runtimeCorootTopicFromName(title)
				}
				sc, pc, names := runtimeCorootSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendRuntimeUniqueString(seriesNames, name, 5)
				}
			}
		}
		if chartCount > 0 {
			item["chartCount"] = chartCount
		}
		if seriesCount > 0 {
			item["seriesCount"] = seriesCount
		}
		if pointCount > 0 {
			item["pointCount"] = pointCount
		}
		if len(titles) > 0 {
			item["titles"] = titles
		}
		if len(seriesNames) > 0 {
			item["seriesNames"] = seriesNames
		}
		out = append(out, item)
	}
	return out
}

func runtimeCorootSeriesCounts(chart map[string]any) (int, int, []string) {
	seriesCount := 0
	pointCount := 0
	var names []string
	for _, series := range runtimeStringAnyMapList(chart["series"]) {
		seriesCount++
		pointCount += len(runtimeAnyList(series["data"]))
		names = appendRuntimeUniqueString(names, runtimeStringFromMap(series, "name"), 5)
	}
	if threshold := runtimeStringAnyMap(chart["threshold"]); len(threshold) > 0 {
		pointCount += len(runtimeAnyList(threshold["data"]))
	}
	return seriesCount, pointCount, names
}

func runtimeCorootTopicFromName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(normalized, "net"), strings.Contains(normalized, "network"), strings.Contains(normalized, "tcp"):
		return "net"
	case strings.Contains(normalized, "cpu"):
		return "cpu"
	case strings.Contains(normalized, "memory"), strings.Contains(normalized, "mem"), strings.Contains(normalized, "rss"):
		return "memory"
	case strings.Contains(normalized, "instances"), strings.Contains(normalized, "instance"):
		return "instances"
	default:
		return ""
	}
}

func runtimeStringAnyMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func runtimeStringAnyMapList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			out = append(out, record)
		}
	}
	return out
}

func runtimeAnyList(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

func runtimeCloneStringAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func runtimeStringFromMap(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstNonBlankRuntimeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendRuntimeUniqueString(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || (limit > 0 && len(values) >= limit) {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func promptInputToolCallsFromRuntime(toolCalls []ToolCall) []promptinput.ToolCall {
	out := make([]promptinput.ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, promptinput.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func promptInputToolResultFromRuntime(result *ToolResult) *promptinput.ToolResult {
	if result == nil {
		return nil
	}
	return &promptinput.ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func runtimeMessagesFromPromptInput(messages []promptinput.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  runtimeToolCallsFromPromptInput(msg.ToolCalls),
			ToolResult: runtimeToolResultFromPromptInput(msg.ToolResult),
		})
	}
	return out
}

func runtimeToolCallsFromPromptInput(toolCalls []promptinput.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func runtimeToolResultFromPromptInput(result *promptinput.ToolResult) *ToolResult {
	if result == nil {
		return nil
	}
	return &ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func writeModelInputDebugTrace(req ModelInputDebugTraceRequest) (string, error) {
	promptTrace := req.PromptInputTrace
	if len(promptTrace.VisibleOpsManualTools) == 0 {
		promptTrace.VisibleOpsManualTools = visibleOpsManualToolsFromNames(req.VisibleTools)
	}
	return modeltrace.Write(modeltrace.Request{
		Kind:              "runtime_model_input",
		SessionID:         req.SessionID,
		TurnID:            req.TurnID,
		Iteration:         req.Iteration,
		Metadata:          req.Metadata,
		VisibleTools:      req.VisibleTools,
		PromptFingerprint: promptFingerprintMap(req.Compiled.Fingerprint),
		Prompt: modeltrace.Prompt{
			StableHash: promptContentHash(req.Compiled.Stable.Content),
			Stable:     req.Compiled.Stable.Content,
			Dynamic:    req.Compiled.Dynamic.Content,
			System:     effectiveSystemPrompt(req.Compiled).Content,
			Developer:  effectiveDeveloperInstructions(req.Compiled).Content,
			Tools:      effectiveToolPromptSet(req.Compiled).Content,
			Policy:     effectiveRuntimePolicyPrompt(req.Compiled).Content,
		},
		ModelInput:       req.ModelInput,
		PromptInputTrace: promptTrace,
		PromptInputDiff:  req.PromptInputDiff,
		DiagnosticTrace:  req.DiagnosticTrace,
	})
}

func visibleOpsManualToolsFromNames(names []string) []string {
	var out []string
	for _, name := range names {
		switch strings.TrimSpace(name) {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

func promptFingerprintMap(fp promptcompiler.PromptFingerprint) map[string]string {
	out := map[string]string{}
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	add("version", fp.Version)
	add("compilerVersion", fp.CompilerVersion)
	add("stableHash", fp.StableHash)
	add("systemHash", fp.SystemHash)
	add("developerHash", fp.DeveloperHash)
	add("toolRegistryHash", fp.ToolRegistryHash)
	add("runtimePolicyHash", fp.RuntimePolicyHash)
	add("protocolStateHash", fp.ProtocolStateHash)
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveSystemPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.SystemPrompt {
	if compiled.System.Content != "" || compiled.System.Role != "" || compiled.System.Environment != "" {
		return compiled.System
	}
	return compiled.Stable.System
}

func effectiveDeveloperInstructions(compiled promptcompiler.CompiledPrompt) promptcompiler.DeveloperInstructions {
	if compiled.Developer.Content != "" || len(compiled.Developer.Constraints) > 0 {
		return compiled.Developer
	}
	return compiled.Stable.Developer
}

func effectiveToolPromptSet(compiled promptcompiler.CompiledPrompt) promptcompiler.ToolPromptSet {
	if compiled.Tools.Content != "" || len(compiled.Tools.Entries) > 0 {
		return compiled.Tools
	}
	return compiled.Stable.Tools
}

func effectiveRuntimePolicyPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.RuntimePolicyPrompt {
	if compiled.Policy.Content != "" || compiled.Policy.Mode != "" {
		return compiled.Policy
	}
	return compiled.Dynamic.Policy
}
