package opsmanual

import "strings"

// ShouldSearchForOpsManuals gates progressive disclosure for operations manuals.
// Diagnosis-only prompts should stay on evidence collection tools; manuals are for
// explicit remediation/change/manual-guided operation requests.
func ShouldSearchForOpsManuals(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if containsOpsManualIntent(normalized) {
		return true
	}
	if isOpsManualAdvisoryOnly(normalized) {
		return false
	}
	return containsOpsManualResolutionIntent(normalized)
}

func containsOpsManualIntent(text string) bool {
	return containsAnyOpsManualIntent(text, []string{
		"运维手册",
		"操作手册",
		"应急预案",
		"故障预案",
		"runbook",
		"playbook",
		"ops manual",
		"operation manual",
		"search_ops_manuals",
		"workflow 预检",
		"运行预检",
		"run preflight",
		"按手册",
	})
}

func containsOpsManualResolutionIntent(text string) bool {
	if containsExplicitMutationRequest(text) {
		return true
	}
	return containsAnyOpsManualIntent(text, []string{
		"修复",
		"恢复服务",
		"恢复数据库",
		"恢复访问",
		"恢复业务",
		"恢复集群",
		"恢复可用",
		"服务恢复",
		"数据库恢复",
		"故障处置",
		"应急处理",
		"重启",
		"重载",
		"回滚",
		"扩容",
		"缩容",
		"切换",
		"切流",
		"迁移",
		"备份",
		"清理",
		"删除",
		"变更",
		"改配置",
		"配置变更",
		"restart",
		"reload",
		"rollback",
		"recover",
		"restore",
		"repair",
		"fix",
		"failover",
		"scale",
		"migrate",
		"migration",
		"backup",
		"change",
	})
}

func isOpsManualAdvisoryOnly(text string) bool {
	if containsExplicitMutationRequest(text) {
		return false
	}
	return containsAnyOpsManualIntent(text, []string{
		"解决方案",
		"处理方案",
		"处置方案",
		"恢复方案",
		"重启方案",
		"备份方案",
		"迁移方案",
		"怎么",
		"如何",
		"为什么",
		"原因",
		"思路",
		"分析",
		"排查",
		"定位",
		"执行环境",
		"执行方式",
		"how to",
		"why",
		"root cause",
		"analyze",
		"analyse",
		"troubleshoot",
		"investigate",
	})
}

func containsExplicitMutationRequest(text string) bool {
	return containsAnyOpsManualIntent(text, []string{
		"帮我修复",
		"帮我解决",
		"帮我恢复",
		"帮我重启",
		"帮我回滚",
		"帮我扩容",
		"帮我缩容",
		"帮我切换",
		"帮我切流",
		"帮我迁移",
		"帮我备份",
		"帮我清理",
		"帮我删除",
		"请修复",
		"请解决",
		"请恢复",
		"请重启",
		"请回滚",
		"请扩容",
		"请缩容",
		"请切换",
		"请切流",
		"请迁移",
		"请备份",
		"请清理",
		"请删除",
		"执行修复",
		"执行解决",
		"执行恢复",
		"执行重启",
		"执行回滚",
		"执行扩容",
		"执行缩容",
		"执行切换",
		"执行切流",
		"执行迁移",
		"执行备份",
		"执行变更",
		"执行清理",
		"执行删除",
		"开始修复",
		"开始恢复",
		"直接修复",
		"直接解决",
		"直接恢复",
		"立刻修复",
		"立刻恢复",
		"马上修复",
		"马上恢复",
		"给我修复",
		"给我解决",
		"修复掉",
		"解决掉",
	})
}

func containsAnyOpsManualIntent(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}
