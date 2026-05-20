package runtimekernel

import (
	"encoding/json"
	"strings"
)

func applyIntentToolPacks(metadata map[string]string, input string) map[string]string {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return metadata
	}
	if containsAnyIntent(text, []string{"rca", "root cause", "根因", "异常", "warning", "告警", "延迟升高", "error rate", "slo", "topology", "依赖", "cpu", "memory", "内存", "net", "网络", "指标", "图表", "趋势", "时序", "metric", "metrics", "chart", "charts", "timeseries"}) {
		metadata = enableIntentToolPack(metadata, "coroot_rca")
	}
	if containsAnyIntent(text, []string{"incident", "alert", "告警", "事件", "timeline"}) {
		metadata = enableIntentToolPack(metadata, "coroot_incident")
	}
	if containsAnyIntent(text, []string{"业务影响", "影响哪些业务", "业务能力", "租户", "依赖关系", "服务图谱", "runbook 关联", "business impact", "tenant", "dependency graph"}) {
		metadata = enableIntentToolPack(metadata, "opsgraph")
	}
	if containsAnyIntent(text, []string{"mcp resource", "mcp_resource", "resource uri", "mcp://"}) || (strings.Contains(text, "resource") && strings.Contains(text, "://")) {
		metadata = enableIntentToolPack(metadata, "mcp_resource")
	}
	return metadata
}

func enableIntentToolPack(metadata map[string]string, pack string) map[string]string {
	metadata = ensureTurnMetadata(metadata)
	metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], pack)
	return metadata
}

func containsAnyIntent(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func updateToolSearchPackTurnMetadata(metadata map[string]string, toolName string, result ToolResult) map[string]string {
	if toolName != "tool_search" && (result.Display == nil || result.Display.Type != "tool_search") {
		return metadata
	}
	data := []byte(result.Content)
	if result.Display != nil && len(result.Display.Data) > 0 {
		data = result.Display.Data
	}
	var payload struct {
		Matches []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
			Pack string `json:"pack"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return metadata
	}
	for _, match := range payload.Matches {
		if !strings.EqualFold(strings.TrimSpace(match.Kind), "pack") {
			continue
		}
		pack := strings.TrimSpace(match.Pack)
		if pack == "" {
			pack = strings.TrimSpace(match.Name)
		}
		if pack != "" {
			metadata = enableIntentToolPack(metadata, pack)
		}
	}
	return metadata
}
