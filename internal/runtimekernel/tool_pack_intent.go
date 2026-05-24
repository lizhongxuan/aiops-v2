package runtimekernel

import (
	"encoding/json"
	"slices"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

func applyIntentToolPacks(metadata map[string]string, input string) map[string]string {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return metadata
	}
	if containsAnyIntent(text, []string{"rca", "root cause", "根因", "异常", "warning", "告警", "延迟升高", "error rate", "排查", "诊断", "故障", "问题", "diagnose", "diagnosis", "outage", "incident analysis"}) {
		metadata = enableIntentToolPack(metadata, "coroot_rca")
	}
	if containsAnyIntent(text, []string{"图表", "chart", "charts", "指标明细", "metric detail", "metrics detail", "时序", "timeseries", "趋势", "slo 详情", "slo status", "service metrics", "latency chart", "cpu chart", "memory chart"}) {
		metadata = enableIntentToolPack(metadata, "coroot_metrics")
	}
	if containsAnyIntent(text, []string{"拓扑图", "topology", "service topology", "依赖图", "dependency graph", "dependencies", "服务拓扑"}) {
		metadata = enableIntentToolPack(metadata, "coroot_topology")
	}
	if containsAnyIntent(text, []string{"coroot rca report", "native rca", "rca reference", "root cause report", "coroot 根因报告"}) {
		metadata = enableIntentToolPack(metadata, "coroot_rca_reference")
	}
	if containsAnyIntent(text, []string{"incident", "alert", "告警", "事件", "timeline"}) {
		metadata = enableIntentToolPack(metadata, "coroot_incident")
	}
	if containsAnyIntent(text, []string{"log", "logs", "logging", "日志", "错误日志"}) {
		metadata = enableIntentToolPack(metadata, "coroot_logs")
	}
	if containsAnyIntent(text, []string{"trace", "traces", "tracing", "span", "spans", "链路", "调用链", "trace id"}) {
		metadata = enableIntentToolPack(metadata, "coroot_traces")
	}
	if containsAnyIntent(text, []string{"profile", "profiling", "flamegraph", "cpu profile", "memory profile", "pprof", "火焰图", "性能剖析"}) {
		metadata = enableIntentToolPack(metadata, "coroot_profiling")
	}
	if containsAnyIntent(text, []string{"deployment", "deployments", "deploy", "release", "rollout", "rollback", "发布", "部署", "变更"}) {
		metadata = enableIntentToolPack(metadata, "coroot_deployments")
	}
	if containsAnyIntent(text, []string{"node", "nodes", "host", "hosts", "infrastructure", "infra", "主机", "节点", "机器", "基础设施"}) {
		metadata = enableIntentToolPack(metadata, "coroot_nodes")
	}
	if containsAnyIntent(text, []string{"risk", "risks", "风险", "隐患"}) {
		metadata = enableIntentToolPack(metadata, "coroot_risks")
	}
	if containsAnyIntent(text, []string{"dashboard", "dashboards", "panel", "coroot panel", "仪表盘", "看板", "面板"}) {
		metadata = enableIntentToolPack(metadata, "coroot_dashboard")
	}
	if containsAnyIntent(text, []string{"integration", "integrations", "inspection", "inspections", "configuration", "config", "category", "custom application", "集成", "巡检", "配置", "应用分类", "自定义应用"}) {
		metadata = enableIntentToolPack(metadata, "coroot_config_read")
	}
	if containsAnyIntent(text, []string{"coroot health", "health check", "project status", "projects", "项目", "项目状态", "agent status", "prometheus status"}) {
		metadata = enableIntentToolPack(metadata, "coroot_admin_read")
	}
	if containsAnyIntent(text, []string{"业务影响", "影响哪些业务", "业务能力", "租户", "依赖关系", "服务图谱", "runbook 关联", "business impact", "tenant", "dependency graph"}) {
		metadata = enableIntentToolPack(metadata, "opsgraph")
	}
	if opsmanual.ShouldSearchForOpsManuals(text) {
		metadata = enableIntentToolPack(metadata, "ops_manual_flow")
	}
	if containsAnyIntent(text, []string{"mcp resource", "mcp_resource", "resource uri", "mcp://"}) || (strings.Contains(text, "resource") && strings.Contains(text, "://")) {
		metadata = enableIntentToolPack(metadata, "mcp_resource")
	}
	return metadata
}

func applyContinuationToolPacks(metadata map[string]string, input string, session *SessionState) map[string]string {
	if !isContinuationOnlyInput(input) || session == nil {
		return metadata
	}
	for _, toolName := range recentSessionToolNames(session, 4) {
		switch {
		case toolNameMatches(toolName, "coroot.collect_rca_context"):
			metadata = enableIntentToolPack(metadata, "coroot_rca")
		case toolNameMatches(toolName, "coroot.service_metrics"),
			toolNameMatches(toolName, "coroot.slo_status"):
			metadata = enableIntentToolPack(metadata, "coroot_metrics")
		case toolNameMatches(toolName, "coroot.rca_report"):
			metadata = enableIntentToolPack(metadata, "coroot_rca_reference")
		case toolNameMatches(toolName, "coroot.service_topology"):
			metadata = enableIntentToolPack(metadata, "coroot_topology")
		case toolNameMatches(toolName, "coroot.nodes_overview"),
			toolNameMatches(toolName, "coroot.get_node"):
			metadata = enableIntentToolPack(metadata, "coroot_nodes")
		case toolNameMatches(toolName, "coroot.traces_overview"),
			toolNameMatches(toolName, "coroot.application_traces"):
			metadata = enableIntentToolPack(metadata, "coroot_traces")
		case toolNameMatches(toolName, "coroot.deployments_overview"):
			metadata = enableIntentToolPack(metadata, "coroot_deployments")
		case toolNameMatches(toolName, "coroot.risks_overview"):
			metadata = enableIntentToolPack(metadata, "coroot_risks")
		case toolNameMatches(toolName, "coroot.application_logs"):
			metadata = enableIntentToolPack(metadata, "coroot_logs")
		case toolNameMatches(toolName, "coroot.application_profiling"):
			metadata = enableIntentToolPack(metadata, "coroot_profiling")
		case toolNameMatches(toolName, "coroot.incidents"),
			toolNameMatches(toolName, "coroot.alert_rules"),
			toolNameMatches(toolName, "coroot.incident_timeline"):
			metadata = enableIntentToolPack(metadata, "coroot_incident")
		case toolNameMatches(toolName, "coroot.list_dashboards"),
			toolNameMatches(toolName, "coroot.get_dashboard"),
			toolNameMatches(toolName, "coroot.get_panel_data"):
			metadata = enableIntentToolPack(metadata, "coroot_dashboard")
		case toolNameMatches(toolName, "coroot.list_integrations"),
			toolNameMatches(toolName, "coroot.get_integration"),
			toolNameMatches(toolName, "coroot.list_inspections"),
			toolNameMatches(toolName, "coroot.get_inspection_config"),
			toolNameMatches(toolName, "coroot.get_application_categories"),
			toolNameMatches(toolName, "coroot.get_custom_applications"):
			metadata = enableIntentToolPack(metadata, "coroot_config_read")
		case toolNameMatches(toolName, "coroot.health_check"),
			toolNameMatches(toolName, "coroot.list_projects"),
			toolNameMatches(toolName, "coroot.get_project_status"):
			metadata = enableIntentToolPack(metadata, "coroot_admin_read")
		case toolNameMatches(toolName, "opsgraph.business_impact"):
			metadata = enableIntentToolPack(metadata, "opsgraph")
		case toolNameMatches(toolName, "list_mcp_resources"),
			toolNameMatches(toolName, "read_mcp_resource"),
			toolNameMatches(toolName, "mcp.list_resources"),
			toolNameMatches(toolName, "mcp.read_resource"):
			metadata = enableIntentToolPack(metadata, "mcp_resource")
		}
	}
	return metadata
}

func isContinuationOnlyInput(input string) bool {
	text := strings.ToLower(strings.TrimSpace(input))
	text = strings.Trim(text, " \t\r\n。.!！?？,，;；:：")
	if text == "" {
		return false
	}
	switch text {
	case "继续", "继续看", "继续查", "继续排查", "继续分析", "继续下钻", "下一步", "下一步查", "下钻", "往下查", "接着看", "接着查", "接着排查", "continue", "go on", "next", "proceed":
		return true
	}
	if strings.HasPrefix(text, "继续") && len([]rune(text)) <= 8 {
		return true
	}
	if strings.HasPrefix(text, "接着") && len([]rune(text)) <= 8 {
		return true
	}
	return false
}

func recentSessionToolNames(session *SessionState, maxTurns int) []string {
	if session == nil || maxTurns <= 0 {
		return nil
	}
	var names []string
	turns := 0
	for i := len(session.TurnHistory) - 1; i >= 0 && turns < maxTurns; i-- {
		turns++
		names = append(names, toolNamesFromSnapshot(session.TurnHistory[i])...)
	}
	for i := len(session.Messages) - 1; i >= 0 && len(names) < 64; i-- {
		for _, toolCall := range session.Messages[i].ToolCalls {
			names = append(names, toolCall.Name)
		}
	}
	return names
}

func toolNamesFromSnapshot(snapshot TurnSnapshot) []string {
	names := make([]string, 0, len(snapshot.AgentItems)+len(snapshot.Iterations))
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		switch item.Type {
		case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
			if name := toolNameFromAgentItem(item); name != "" {
				names = append(names, name)
			}
		default:
			continue
		}
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		iteration := snapshot.Iterations[i]
		names = append(names, iteration.VisibleTools...)
		for _, toolCall := range iteration.ToolCalls {
			names = append(names, toolCall.Name)
		}
	}
	return names
}

func toolNameFromAgentItem(item agentstate.TurnItem) string {
	if len(item.Payload.Data) > 0 {
		var payload struct {
			Name     string `json:"name"`
			ToolName string `json:"toolName"`
			Tool     string `json:"tool"`
		}
		if err := json.Unmarshal(item.Payload.Data, &payload); err == nil {
			for _, value := range []string{payload.ToolName, payload.Name, payload.Tool} {
				if strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return strings.TrimSpace(item.Payload.Summary)
}

func toolNameMatches(name string, internalName string) bool {
	name = strings.TrimSpace(name)
	internalName = strings.TrimSpace(internalName)
	if name == "" || internalName == "" {
		return false
	}
	return strings.EqualFold(name, internalName) ||
		strings.EqualFold(name, tooling.ProviderSafeToolName(internalName))
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
			break
		}
	}
	return metadata
}

func visibleToolsForContextMode(tools []string, thresholds ContextBudgetThresholds) []string {
	if !thresholds.SmallContextMode {
		return append([]string(nil), tools...)
	}
	allowed := make([]string, 0, len(tools))
	for _, name := range tools {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if smallContextCoreTool(trimmed) || !smallContextHighVolumeTool(trimmed) {
			allowed = append(allowed, trimmed)
		}
	}
	return allowed
}

func filterToolsForContextMode(tools []tooling.Tool, thresholds ContextBudgetThresholds) []tooling.Tool {
	if !thresholds.SmallContextMode {
		return tools
	}
	filtered := make([]tooling.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if smallContextCoreTool(meta.Name) || tooling.ToolAllowedInSmallContext(meta) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func smallContextCoreTool(name string) bool {
	return slices.Contains([]string{
		"tool_search",
		"mcp.list_resources",
		"mcp.read_resource",
		"list_mcp_resources",
		"read_mcp_resource",
	}, name)
}

func smallContextHighVolumeTool(name string) bool {
	normalized := strings.ToLower(name)
	return strings.Contains(normalized, "debug.dump") ||
		strings.Contains(normalized, "dump_all") ||
		strings.Contains(normalized, "all_metrics") ||
		strings.Contains(normalized, "raw_metrics")
}
