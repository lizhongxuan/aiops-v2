package opsmanual

import "strings"

type taxonomyRule struct {
	Value   string
	Needles []string
}

func normalizeText(text string) string {
	replacer := strings.NewReplacer("，", " ", "。", " ", "；", " ", ",", " ", ";", " ", "：", " ", ":", " ")
	normalized := " " + strings.ToLower(replacer.Replace(text)) + " "
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	return normalized
}

func detectObjectType(text string) string {
	return DefaultOpsManualCapabilityRegistry().DetectObjectType(text)
}

func detectOperationType(text string) string {
	return DefaultOpsManualCapabilityRegistry().DetectOperationType(text)
}

func looksLikeStatusCheck(normalized string) bool {
	if containsAnyTaxonomyNeedle(normalized, []string{"status check", "health check", "健康检查", "巡检", "状态检查", "检查状态", "运行状态", "健康状态"}) {
		return true
	}
	return strings.Contains(normalized, "检查") && strings.Contains(normalized, "状态")
}

func looksLikeTroubleshooting(normalized string) bool {
	return containsAnyTaxonomyNeedle(normalized, []string{
		"排查", "故障", "诊断", "异常", "报错", "错误", "失败", "恢复",
		"rca", "triage", "troubleshoot", "troubleshooting", "diagnose", "diagnosis", "repair", "lag", "rebalance", "crashloopbackoff",
		"oomkilled", "频繁重启", "timeout", "latency", "慢查询",
	})
}

func containsAnyTaxonomyNeedle(normalized string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(normalized, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func operationAliases(value string) []string {
	return DefaultOpsManualCapabilityRegistry().OperationAliasesFor(value)
}

func objectAliases(value string) []string {
	return DefaultOpsManualCapabilityRegistry().ObjectAliasesFor(value)
}
