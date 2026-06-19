package opssemantic

import "strings"

func ClassifyRisk(text string) OpsRiskLevel {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if containsAny(normalized, []string{
		"删除", "移除", "清空", "销毁", "格式化", "覆盖", "drop", "delete", "remove", "destroy", "format", "overwrite",
	}) {
		return RiskDestructive
	}
	if hasReadOnlyInspectionIntent(normalized) && !hasMutationIntent(normalized) {
		return RiskReadOnly
	}
	if containsAny(normalized, []string{
		"系统服务", "防火墙", "网络", "权限", "关键配置", "内核", "路由", "账号", "证书",
		"system service", "firewall", "network", "permission", "privilege", "kernel", "route", "account", "certificate",
	}) {
		return RiskHighWrite
	}
	if containsAny(normalized, []string{
		"安装", "升级", "修改配置", "启动服务", "停止服务", "重启服务", "依赖", "写入配置",
		"install", "upgrade", "modify config", "start service", "stop service", "restart service", "dependency",
	}) {
		return RiskMediumWrite
	}
	if containsAny(normalized, []string{
		"临时文件", "准备目录", "创建文件", "touch", "mkdir", "temporary file", "temp file", "staging",
	}) {
		return RiskLowWrite
	}
	return RiskReadOnly
}

func hasReadOnlyInspectionIntent(text string) bool {
	return containsAny(text, []string{
		"查看", "检查", "查询", "诊断", "分析", "汇总", "总结", "只读", "只读执行", "回传证据",
		"inspect", "check", "query", "diagnose", "analyze", "summarize", "read-only", "readonly",
	})
}

func hasMutationIntent(text string) bool {
	return containsAny(text, []string{
		"删除", "移除", "清空", "销毁", "格式化", "覆盖",
		"安装", "升级", "修改", "调整", "设置", "配置", "启动", "停止", "重启", "写入", "创建",
		"touch", "mkdir", "drop", "delete", "remove", "destroy", "format", "overwrite",
		"install", "upgrade", "modify", "change", "set", "configure", "start", "stop", "restart", "write", "create",
	})
}

func RiskRequiresApproval(risk OpsRiskLevel) bool {
	switch risk {
	case RiskMediumWrite, RiskHighWrite, RiskDestructive:
		return true
	default:
		return false
	}
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
