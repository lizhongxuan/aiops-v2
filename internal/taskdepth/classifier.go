package taskdepth

import (
	"strconv"
	"strings"
)

func Classify(opts Options) Profile {
	input := strings.ToLower(strings.TrimSpace(opts.Input))
	level := LevelTrivial
	reasons := []string{}

	if override := metadataValue(opts.Metadata, "taskDepth", "task_depth", "depth"); override != "" {
		level = NormalizeLevel(override)
		reasons = append(reasons, "metadata override: "+string(level))
		return profileFor(level, reasons)
	}

	if count, _ := strconv.Atoi(metadataValue(opts.Metadata, "hostMentionCount", "host_mention_count")); count >= 2 {
		level = maxLevel(level, LevelMultiAgent)
		reasons = append(reasons, "multiple host mentions")
	}

	if containsAny(input, []string{"@主机", "跨主机", "多个主机", "多主机", "多个目标主机", "child agent", "子 agent"}) {
		level = maxLevel(level, LevelMultiAgent)
		reasons = append(reasons, "multi-agent or multi-host wording")
	}

	mutationIntent := containsAny(input, []string{"恢复", "修复", "重启", "回滚", "迁移", "备份", "扩容", "缩容", "删除", "变更", "执行"})
	if mutationIntent || (strings.EqualFold(strings.TrimSpace(opts.Mode), "execute") && !isReadOnlyInspectionIntent(input)) {
		level = maxLevel(level, LevelOperations)
		reasons = append(reasons, "operation or mutation intent")
	}

	if containsAny(input, []string{"排查", "故障", "异常", "根因", "rca", "为什么", "错误", "不可用", "超时", "慢", "延迟", "报警", "告警", "incident", "健康检查", "状态检查", "关键指标"}) {
		level = maxLevel(level, LevelInvestigation)
		reasons = append(reasons, "investigation or RCA wording")
	}

	if containsAny(input, []string{"分析", "设计", "计划", "分步骤", "多步", "详细"}) {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "multi-step wording")
	}

	if level == LevelTrivial && containsAny(input, []string{"查看", "查询", "状态", "当前", "列表"}) {
		level = LevelSimpleRead
		reasons = append(reasons, "simple read wording")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "simple conversational request")
	}
	return profileFor(level, reasons)
}

func isReadOnlyInspectionIntent(input string) bool {
	if input == "" {
		return false
	}
	if containsAny(input, []string{"恢复", "修复", "重启", "回滚", "迁移", "备份", "扩容", "缩容", "删除", "变更", "执行", "restart", "rollback", "delete", "remove", "scale", "migrate", "backup", "restore", "change"}) {
		return false
	}
	return containsAny(input, []string{
		"查看", "查询", "看下", "看一下", "获取", "显示", "列出", "列表", "当前", "状态", "资源", "使用率", "指标", "信息",
		"show", "view", "check", "inspect", "read", "get", "list", "status", "usage", "resource", "resources", "info",
	})
}

func profileFor(level Level, reasons []string) Profile {
	level = NormalizeLevel(string(level))
	return Profile{
		Level:                level,
		Reasons:              append([]string(nil), reasons...),
		RequiresPlan:         AtLeast(level, LevelMultiStep),
		RequiresEvidence:     AtLeast(level, LevelInvestigation),
		RequiresValidation:   AtLeast(level, LevelOperations),
		AllowsFirstTurnFinal: !AtLeast(level, LevelMultiStep),
	}
}

func maxLevel(current, candidate Level) Level {
	if Rank(candidate) > Rank(current) {
		return candidate
	}
	return current
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func metadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}
