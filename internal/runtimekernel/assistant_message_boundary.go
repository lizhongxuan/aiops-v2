package runtimekernel

import (
	"strconv"
	"strings"
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
	runeCount := len([]rune(trimmed))
	if runeCount > 180 {
		return false
	}
	if strings.Count(trimmed, "\n") > 1 {
		return false
	}
	lower := strings.ToLower(trimmed)
	if runeCount > 100 && containsAnyAssistantFinalMarker(lower) {
		return false
	}
	return true
}

func containsAnyAssistantFinalMarker(lower string) bool {
	for _, marker := range []string{
		"根因", "结论", "证据", "影响面", "下一步", "机制链路",
		"root cause", "conclusion", "evidence", "impact", "next step",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func finalMessageHasProcessIntent(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if len([]rune(trimmed)) > 260 || containsAnyAssistantFinalMarker(lower) {
		return false
	}
	for _, marker := range []string{
		"let me ", "i will ", "i'll ", "try browsing", "browse ", "search ", "look up ",
		"我先", "让我", "我会继续", "我将继续", "现在我会", "接下来我", "继续查阅", "继续搜索", "继续查看",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
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

	checks := compactStringList(input.ReadOnlyChecks)
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
	prefix := "证据边界：当前证据仍受限，以下内容只能作为待核实判断。"
	if strings.Contains(cleaned, "证据边界") || strings.Contains(cleaned, "证据仍不足") {
		return cleaned
	}
	return strings.TrimSpace(prefix + "\n\n" + cleaned)
}

func constrainedFinalShouldBecomeIncomplete(text string, decision FinalEvidenceVerification) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || finalMessageHasProcessIntent(trimmed) || containsInternalFinalFallbackText(trimmed) {
		return true
	}
	if decision.Action == FinalEvidenceActionAllow {
		return false
	}
	if !containsConcreteFinalContent(trimmed) {
		return true
	}
	return false
}

func containsConcreteFinalContent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	matches := 0
	for _, marker := range []string{
		"根因", "原因", "证据：", "可能原因", "下一步", "timeline", "pgbackrest", "postgres", "wal",
		"recovery_target", "restore_command", "$pgdata", "pg_auto", "primary_conninfo", ".history",
		"root cause", "evidence:", "next step",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			matches++
		}
	}
	return matches >= 1
}

func incompleteFinalFromEvidenceDecision(decision FinalEvidenceVerification) string {
	var confirmed []string
	for _, item := range decision.State.Checked {
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			confirmed = append(confirmed, summary)
			continue
		}
		if name := strings.TrimSpace(item.ToolName); name != "" {
			confirmed = append(confirmed, name+" 已有可用证据")
		}
	}
	var missing []string
	var checks []string
	for _, item := range decision.State.FailedTools {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			name = "某个必需工具"
		}
		missing = append(missing, name+" 未成功返回证据；不能当作已检查结果。")
		checks = append(checks, "重新读取或替代核对 "+name+" 对应的只读证据。")
	}
	for _, item := range decision.State.NotChecked {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			name = "未加载工具"
		}
		reason := strings.TrimSpace(item.Reason)
		if reason != "" {
			missing = append(missing, name+" 未检查："+reason+"。")
		} else {
			missing = append(missing, name+" 仍未完成检查。")
		}
		if query := strings.TrimSpace(item.SuggestedSearchQuery); query != "" {
			checks = append(checks, "补充检索："+query)
		} else {
			checks = append(checks, "补齐 "+name+" 对应的只读证据。")
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

func containsInternalFinalFallbackText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "final contract") ||
		strings.Contains(lower, "non_substantive_final_answer") ||
		strings.Contains(lower, "official-domain fallback results") ||
		strings.Contains(lower, `{"content"`) ||
		strings.Contains(lower, "kinds=") ||
		strings.Contains(lower, "signals=")
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
	if containsInternalFinalFallbackText(line) {
		return "已存在内部证据或检索结果，但本轮没有形成可直接展示的结构化最终结论。"
	}
	line = strings.ReplaceAll(line, "kinds=", "证据类型：")
	line = strings.ReplaceAll(line, "signals=", "识别信号：")
	line = strings.ReplaceAll(line, "excerpt=", "摘录：")
	return strings.TrimSpace(line)
}

func writeBullets(builder *strings.Builder, values []string, fallback string) {
	values = compactStringList(values)
	if len(values) == 0 {
		values = []string{fallback}
	}
	for _, value := range values {
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
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
