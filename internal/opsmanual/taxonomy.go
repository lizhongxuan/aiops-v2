package opsmanual

import "strings"

type taxonomyRule struct {
	Value   string
	Needles []string
}

var objectTypeRules = []taxonomyRule{
	{Value: "kafka", Needles: []string{"kafka", "consumer group", "consumer_group", "broker", "partition", "rebalance", "lag"}},
	{Value: "postgresql", Needles: []string{"postgresql", "postgres", "pg_dump", "pg_basebackup", " pg ", " pg-", "pg-", "pg "}},
	{Value: "mysql", Needles: []string{"mysql", "mysqldump"}},
	{Value: "redis", Needles: []string{"redis", "used_memory_rss"}},
	{Value: "kubernetes_pod", Needles: []string{"crashloopbackoff", "oomkilled", " pod ", " pod-", "pod ", "容器组"}},
	{Value: "kubernetes_workload", Needles: []string{"deployment", "statefulset", "daemonset", "workload", "k8s", "kubernetes", "kubectl", "工作负载"}},
	{Value: "host", Needles: []string{"主机", "虚拟机", "vm", "systemd", "systemctl"}},
	{Value: "network", Needles: []string{"network", "网络", "latency", "timeout", "丢包", "dns"}},
	{Value: "tool", Needles: []string{"工具", "tool", "install package"}},
}

var operationTypeRules = []taxonomyRule{
	{Value: "backup", Needles: []string{"备份", "backup", "back up", "back-up", "dump"}},
	{Value: "restore", Needles: []string{"数据恢复", "restore", "rollback data"}},
	{Value: "restart", Needles: []string{"重启", "restart", "systemctl restart"}},
	{Value: "scale", Needles: []string{"扩容", "缩容", "scale"}},
	{Value: "deploy", Needles: []string{"部署", "主从", "install", "搭建"}},
	{Value: "migration", Needles: []string{"迁移", "migration", "migrate"}},
	{Value: "status_check", Needles: []string{"status check", "health check", "健康检查", "巡检", "状态检查", "检查状态", "运行状态", "健康状态"}},
	{Value: "rca_or_repair", Needles: []string{"排查", "故障", "诊断", "恢复", "rca", "triage", "troubleshoot", "troubleshooting", "diagnose", "diagnosis", "repair", "checkout", "lag", "rebalance", "broker", "partition", "consumer group", "crashloopbackoff", "oomkilled", "频繁重启"}},
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
	normalized := normalizeText(text)
	for _, rule := range objectTypeRules {
		if containsAnyTaxonomyNeedle(normalized, rule.Needles) {
			return rule.Value
		}
	}
	return ""
}

func detectOperationType(text string) string {
	normalized := normalizeText(text)
	if strings.Contains(normalized, "恢复") && strings.Contains(normalized, "数据") {
		return "restore"
	}
	if strings.Contains(normalized, "crashloopbackoff") || strings.Contains(normalized, "oomkilled") ||
		strings.Contains(normalized, "频繁重启") || strings.Contains(normalized, "反复重启") {
		return "rca_or_repair"
	}
	if looksLikeStatusCheck(normalized) && !looksLikeTroubleshooting(normalized) {
		return "status_check"
	}
	for _, rule := range operationTypeRules {
		if rule.Value == "restart" && !hasPositiveRestartIntent(normalized) {
			continue
		}
		if containsAnyTaxonomyNeedle(normalized, rule.Needles) {
			return rule.Value
		}
	}
	return ""
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
	for _, rule := range operationTypeRules {
		if rule.Value == value {
			return append([]string{value}, rule.Needles...)
		}
	}
	return []string{value}
}

func objectAliases(value string) []string {
	for _, rule := range objectTypeRules {
		if rule.Value == value {
			return append([]string{value}, rule.Needles...)
		}
	}
	return []string{value}
}
