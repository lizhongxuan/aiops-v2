package runtimekernel

import (
	"strconv"
	"strings"

	"aiops-v2/internal/agentstate"
)

type incompleteFinalInput struct {
	ConfirmedFacts  []string
	MissingEvidence []string
	LikelyDirection string
	Confidence      string
	ReadOnlyChecks  []string
}

func isAssistantProgressContentAllowed(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if len([]rune(trimmed)) > 180 || strings.Count(trimmed, "\n") > 1 {
		return false
	}
	if containsRawToolCallMarkup(strings.ToLower(trimmed)) {
		return false
	}
	if hasUnclosedFinalDelimiter(trimmed) || hasUnclosedMarkdownFence(trimmed) {
		return false
	}
	return true
}

func buildDeterministicIncompleteFinal(input incompleteFinalInput) string {
	var builder strings.Builder
	builder.WriteString("还不能给最终结论（证据仍不足）\n\n")

	builder.WriteString("已确认：\n")
	writeBullets(&builder, input.ConfirmedFacts, "当前已有信息不足以确认根因，只能确认问题仍需要补证。")

	builder.WriteString("\n仍缺少：\n")
	writeBullets(&builder, input.MissingEvidence, "关键只读证据仍不足，无法把候选原因收敛为最终结论。")

	direction := strings.TrimSpace(input.LikelyDirection)
	if direction == "" {
		direction = "需要先补齐只读证据后再判断根因方向"
	}
	builder.WriteString("\n当前最可能方向：\n")
	builder.WriteString("- ")
	builder.WriteString(direction)
	builder.WriteString("\n")

	checks := compactUserVisibleStringList(input.ReadOnlyChecks)
	if len(checks) == 0 {
		checks = []string{
			"补充完整错误输出和最近日志，确认失败发生在哪一步。",
			"补充相关节点或服务的只读状态信息，确认版本、角色、timeline 或连接目标是否一致。",
			"补充监控或控制面看到的节点状态，确认实际状态与编排系统记录是否一致。",
		}
	}
	builder.WriteString("\n下一步只读检查：\n")
	for i, check := range checks {
		builder.WriteString(strconv.Itoa(i + 1))
		builder.WriteString(". ")
		builder.WriteString(check)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func constrainFinalMessageForEvidenceBoundary(text string, decision FinalEvidenceVerification) string {
	cleaned := sanitizeUserVisibleConfidenceLabels(text)
	if constrainedFinalShouldBecomeIncomplete(cleaned, decision) {
		return incompleteFinalFromEvidenceDecision(decision)
	}
	prefix := "证据边界：以下内容只能作为待核实判断。"
	if strings.Contains(cleaned, "证据边界") || strings.Contains(cleaned, "证据仍不足") {
		return cleaned
	}
	return strings.TrimSpace(prefix + "\n\n" + cleaned)
}

func sanitizeFinalAssistantContentForCommit(text string, decision FinalEvidenceVerification) (string, bool) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return cleaned, false
	}
	if !containsRawToolCallMarkup(strings.ToLower(cleaned)) {
		return cleaned, false
	}
	if decision.Action == "" || decision.Action == FinalEvidenceActionAllow {
		decision.Action = FinalEvidenceActionDowngrade
	}
	decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceLow)
	decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "raw_tool_call_markup_final")
	return incompleteFinalFromEvidenceDecision(decision), true
}

func recordRawToolCallMarkupFinalSanitized(snapshot *TurnSnapshot, turnID string, iteration int, _ string) {
	if snapshot == nil {
		return
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]string{}
	}
	snapshot.Metadata["rawToolCallMarkupSanitized"] = "true"
	snapshot.Metadata["finalRawToolCallMarkupConstrained"] = "true"
	appendAgentItem(snapshot, newAgentItem(
		errorItemID(turnID, iteration),
		agentstate.TurnItemTypeError,
		agentstate.ItemStatusFailed,
		"raw_tool_call_markup_final: final answer contained tool-call markup and was replaced with a safe evidence-limited response.",
		map[string]any{
			"reason": "raw_tool_call_markup_final",
		},
	))
}

func constrainedFinalShouldBecomeIncomplete(text string, decision FinalEvidenceVerification) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || containsRawToolCallMarkup(strings.ToLower(trimmed)) {
		return true
	}
	if hasUnclosedFinalDelimiter(trimmed) || hasUnclosedMarkdownFence(trimmed) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(decision.Action), FinalEvidenceActionBlock)
}

func incompleteFinalFromEvidenceDecision(decision FinalEvidenceVerification) string {
	var confirmed []string
	for _, item := range decision.State.Checked {
		if summary := sanitizeIncompleteFinalUserLine(item.Summary); summary != "" {
			confirmed = append(confirmed, summary)
			continue
		}
		if name := strings.TrimSpace(item.ToolName); name != "" {
			confirmed = append(confirmed, userVisibleToolName(name)+" 已有可用证据")
		}
	}
	var missing []string
	var checks []string
	for _, item := range decision.State.FailedTools {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			name = "某个必需工具"
		}
		displayName := userVisibleToolName(name)
		missing = append(missing, displayName+" 未成功返回证据；不能当作已检查结果。")
		checks = append(checks, "重新读取或替代核对 "+displayName+" 对应的只读证据。")
	}
	for _, item := range decision.State.NotChecked {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			name = "未加载工具"
		}
		displayName := userVisibleToolName(name)
		reason := strings.TrimSpace(item.Reason)
		if reason != "" {
			missing = append(missing, displayName+" 未检查："+humanizeUserVisibleDiagnostic(reason)+"。")
		} else {
			missing = append(missing, displayName+" 仍未完成检查。")
		}
		if query := strings.TrimSpace(item.SuggestedSearchQuery); query != "" {
			checks = append(checks, "补充检索："+query)
		} else {
			checks = append(checks, "补齐 "+displayName+" 对应的只读证据。")
		}
	}
	for _, reason := range decision.Reasons {
		missing = append(missing, incompleteFinalMessagesForEvidenceReason(reason)...)
	}
	return buildDeterministicIncompleteFinal(incompleteFinalInput{
		ConfirmedFacts:  confirmed,
		MissingEvidence: missing,
		LikelyDirection: "需要先补齐失败或未检查的只读证据，再判断根因方向。",
		ReadOnlyChecks:  checks,
	})
}

func sanitizeUserVisibleConfidenceLabels(text string) string {
	out := strings.TrimSpace(text)
	if out == "" {
		return ""
	}
	replacements := []struct {
		old string
		new string
	}{
		{"（置信度：高）", "（证据边界：受限）"},
		{"（置信度:高）", "（证据边界：受限）"},
		{"（置信度：中）", "（证据边界：受限）"},
		{"（置信度:中）", "（证据边界：受限）"},
		{"（置信度：低）", "（证据边界：受限）"},
		{"（置信度:低）", "（证据边界：受限）"},
		{"置信度：高", "证据边界：受限"},
		{"置信度:高", "证据边界：受限"},
		{"置信度：中", "证据边界：受限"},
		{"置信度:中", "证据边界：受限"},
		{"置信度：低", "证据边界：受限"},
		{"置信度:低", "证据边界：受限"},
		{"高置信度", "证据受限"},
		{"低置信度", "证据受限"},
		{"高置信", "证据受限"},
		{"低置信", "证据受限"},
		{"confidence: high", "evidence boundary: limited"},
		{"confidence：high", "evidence boundary: limited"},
		{"confidence high", "evidence limited"},
		{"confidence: low", "evidence boundary: limited"},
		{"confidence：low", "evidence boundary: limited"},
		{"confidence low", "evidence limited"},
	}
	for _, replacement := range replacements {
		out = strings.ReplaceAll(out, replacement.old, replacement.new)
	}
	return strings.TrimSpace(out)
}

// containsInternalFinalFallbackText is retained only as the shared raw
// provider-protocol safety boundary used by evidence fallback cleanup. It must
// not classify business vocabulary or ordinary JSON as an assistant phase.
func containsInternalFinalFallbackText(text string) bool {
	return containsRawToolCallMarkup(strings.ToLower(text))
}

func containsRawToolCallMarkup(lower string) bool {
	normalized := strings.ToLower(strings.TrimSpace(normalizeRawToolCallMarkup(lower)))
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(normalized)
	for _, marker := range []string{
		"tool_calls>",
		"<tool_calls",
		"<|tool_call",
		"||dsml||tool_calls",
		"||dsml||invoke",
		"||dsml||parameter",
		"<｜｜dsml｜｜tool_calls",
		"<｜｜dsml｜｜invoke",
		"<｜｜dsml｜｜parameter",
		"</｜｜dsml｜｜invoke>",
		"invoke name=\"web_search\"",
		"invoke name=\"exec_command\"",
	} {
		if strings.Contains(normalized, marker) || strings.Contains(compact, strings.ReplaceAll(marker, " ", "")) {
			return true
		}
	}
	return false
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func sanitizeIncompleteFinalUserLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if summary := structuredEvidenceSummaryForUser("", line); summary != "" {
		return summary
	}
	line = strings.ReplaceAll(line, "kinds=", "证据类型：")
	line = strings.ReplaceAll(line, "signals=", "识别信号：")
	line = strings.ReplaceAll(line, "excerpt=", "摘录：")
	return strings.TrimSpace(humanizeUserVisibleDiagnostic(line))
}

func writeBullets(builder *strings.Builder, values []string, fallback string) {
	values = compactUserVisibleStringList(values)
	if len(values) == 0 {
		values = []string{fallback}
	}
	for _, value := range values {
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
}

func compactUserVisibleStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := sanitizeIncompleteFinalUserLine(value); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return compactStringList(out)
}

func userVisibleToolName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "read_context_artifact":
		return "读取上下文证据"
	case "read_mcp_resource":
		return "读取 MCP 资源"
	case "list_mcp_resources":
		return "列出 MCP 资源"
	case "coroot.list_services", "coroot_list_services":
		return "Coroot 服务列表"
	case "coroot.incidents", "coroot_incidents":
		return "Coroot 异常事件"
	case "coroot.collect_rca_context", "coroot_collect_rca_context":
		return "Coroot 根因分析上下文"
	default:
		return strings.TrimSpace(name)
	}
}

func humanizeUserVisibleDiagnostic(text string) string {
	out := strings.TrimSpace(text)
	if out == "" {
		return ""
	}
	replacements := []struct {
		old string
		new string
	}{
		{"required evidence may be missing; do not use this failed tool as checked evidence", "证据读取失败，不能作为已检查结果"},
		{"read_context_artifact", "读取上下文证据"},
		{"read_mcp_resource", "读取 MCP 资源"},
		{"list_mcp_resources", "列出 MCP 资源"},
		{"coroot.collect_rca_context", "Coroot 根因分析上下文"},
		{"coroot_collect_rca_context", "Coroot 根因分析上下文"},
		{"coroot.list_services", "Coroot 服务列表"},
		{"coroot_list_services", "Coroot 服务列表"},
		{"coroot.incidents", "Coroot 异常事件"},
		{"coroot_incidents", "Coroot 异常事件"},
		{"tool_business_error", "工具执行失败"},
	}
	for _, replacement := range replacements {
		out = strings.ReplaceAll(out, replacement.old, replacement.new)
	}
	return strings.TrimSpace(out)
}

func structuredEvidenceSummaryForUser(toolName string, text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return ""
	}
	lower := strings.ToLower(trimmed)
	label := ""
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "coroot.list_services", "coroot_list_services":
		label = "Coroot 服务列表"
	case "coroot.incidents", "coroot_incidents":
		label = "Coroot 异常事件"
	case "coroot.collect_rca_context", "coroot_collect_rca_context":
		label = "Coroot 根因分析上下文"
	}
	switch {
	case strings.Contains(lower, `"categorycounts"`):
		label = "Coroot 服务概览"
	case strings.Contains(lower, `"incidents"`):
		label = "Coroot 异常事件"
	case strings.Contains(lower, `"abnormalservices"`) || strings.Contains(lower, `"service"`):
		if label == "" {
			label = "Coroot 服务证据"
		}
	case strings.Contains(lower, `"evidencerefs"`):
		if label == "" {
			label = "结构化证据"
		}
	default:
		return ""
	}
	return label + "已返回结构化证据。"
}

func incompleteFinalMessagesForEvidenceReason(reason string) []string {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if normalized == "" {
		return nil
	}
	var out []string
	if strings.Contains(normalized, "target_binding") ||
		strings.Contains(normalized, "target binding") ||
		strings.Contains(normalized, "no target") ||
		strings.Contains(normalized, "unbound") ||
		strings.Contains(normalized, "missing target") {
		out = append(out, "变更类操作必须先明确绑定目标资源，例如在输入中使用 @host 或 @IP，或先在界面选择目标资源。")
	}
	if strings.Contains(normalized, "approval") {
		out = append(out, "变更类操作需要明确审批；审批卡必须展示目标资源、命令摘要、风险和回滚/验证边界。")
	}
	if strings.Contains(normalized, "mutation") || strings.Contains(normalized, "change") || strings.Contains(normalized, "exec") {
		out = append(out, "当前不能提供或执行未绑定目标的变更命令；只能先给出缺失信息和只读核对方向。")
	}
	return out
}
