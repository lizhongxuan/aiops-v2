package taskdepth

import (
	"strconv"
	"strings"

	"aiops-v2/internal/runtimecontract"
)

func Classify(opts Options) Profile {
	return classifyLegacy(opts)
}

func ClassifyFromIntentFrame(frame runtimecontract.IntentFrame, opts Options) Profile {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	if isEmptyIntentFrame(frame) {
		profile := classifyLegacy(opts)
		profile.Reasons = append(profile.Reasons, "legacy fallback: empty intent frame")
		return profile
	}

	level := LevelTrivial
	reasons := []string{}
	advisoryAnalysisOnly := advisoryAnalysisOnlyFromMetadata(opts.Metadata)
	executionProhibited := executionProhibitedByIntentFrame(frame) || advisoryAnalysisOnly
	analysisOnly := advisoryAnalysisOnly || (riskBudgetIsReadOnly(frame.RiskBudget) && executionProhibitedByIntentFrame(frame))
	requiresApproval := false

	switch frame.Kind {
	case runtimecontract.IntentKindExplain:
		level = maxLevel(level, LevelSimpleRead)
		reasons = append(reasons, "intent kind explain")
	case runtimecontract.IntentKindPlan, runtimecontract.IntentKindRunbookAuthoring:
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "intent kind "+string(frame.Kind))
	case runtimecontract.IntentKindResearch:
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "intent kind research")
	case runtimecontract.IntentKindDiagnose, runtimecontract.IntentKindVerify:
		level = maxLevel(level, LevelInvestigation)
		reasons = append(reasons, "intent kind "+string(frame.Kind))
	case runtimecontract.IntentKindChange:
		level = maxLevel(level, LevelOperations)
		reasons = append(reasons, "intent kind change")
	case runtimecontract.IntentKindConfigure:
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "intent kind configure")
	}

	if frame.Evidence.HasUserProvidedEvidence {
		if frame.Kind == runtimecontract.IntentKindDiagnose || frame.Kind == runtimecontract.IntentKindVerify || frame.Kind == runtimecontract.IntentKindUnknown {
			level = maxLevel(level, LevelInvestigation)
		} else {
			level = maxLevel(level, LevelMultiStep)
		}
		reasons = append(reasons, "user-provided evidence")
	}

	if hasDataScope(frame, runtimecontract.DataScopePublicWeb) || hasDataScope(frame, runtimecontract.DataScopeExternalMCP) {
		if frame.Kind == runtimecontract.IntentKindResearch || frame.Kind == runtimecontract.IntentKindDiagnose || frame.Kind == runtimecontract.IntentKindVerify {
			level = maxLevel(level, LevelInvestigation)
			reasons = append(reasons, "external data scope")
		}
	}

	for _, risk := range frame.RiskBudget {
		switch risk {
		case runtimecontract.ActionRiskWrite, runtimecontract.ActionRiskHostExec, runtimecontract.ActionRiskDestruct:
			if advisoryAnalysisOnly {
				reasons = append(reasons, "route prohibits execution for advisory analysis")
				continue
			}
			level = maxLevel(level, LevelOperations)
			requiresApproval = true
			reasons = append(reasons, "risk budget includes "+string(risk))
		case runtimecontract.ActionRiskNetwork:
			reasons = append(reasons, "risk budget includes "+string(risk))
		}
	}

	for _, capability := range frame.Capabilities {
		for _, risk := range capability.Risks {
			switch risk {
			case runtimecontract.ActionRiskWrite, runtimecontract.ActionRiskHostExec, runtimecontract.ActionRiskDestruct:
				if advisoryAnalysisOnly {
					reasons = append(reasons, "route prohibits capability execution for advisory analysis")
					continue
				}
				level = maxLevel(level, LevelOperations)
				requiresApproval = true
				reasons = append(reasons, "capability risk includes "+string(risk))
			}
		}
		for _, scope := range capability.DataScopes {
			if scope == runtimecontract.DataScopePublicWeb || scope == runtimecontract.DataScopeExternalMCP {
				level = maxLevel(level, LevelInvestigation)
				reasons = append(reasons, "capability uses external data scope")
			}
		}
	}

	level, reasons = applyStructuralSignals(level, reasons, opts)
	if len(reasons) == 0 {
		reasons = append(reasons, "structured intent conversational request")
	}

	profile := profileFor(level, reasons, analysisOnly, executionProhibited)
	profile.RequiresApproval = requiresApproval && !analysisOnly
	if profile.RequiresApproval {
		profile.RequiresValidation = true
	}
	return profile
}

func classifyLegacy(opts Options) Profile {
	input := strings.ToLower(strings.TrimSpace(opts.Input))
	level := LevelTrivial
	reasons := []string{}
	advisoryAnalysisOnly := advisoryAnalysisOnlyFromMetadata(opts.Metadata)
	executionProhibited := advisoryAnalysisOnly || executionProhibitedByInput(input) || metadataBool(opts.Metadata, "aiops.route.userProhibitedHostExec", "aiops.execution.prohibited")
	analysisOnly := advisoryAnalysisOnly || (executionProhibited && (analysisOnlyIntent(input) || metadataBool(opts.Metadata, "aiops.userEvidence.present")))

	if override := metadataValue(opts.Metadata, "taskDepth", "task_depth", "depth"); override != "" {
		level = NormalizeLevel(override)
		reasons = append(reasons, "metadata override: "+string(level))
		return profileFor(level, reasons, analysisOnly, executionProhibited)
	}

	if count, _ := strconv.Atoi(metadataValue(opts.Metadata, "hostMentionCount", "host_mention_count")); count >= 2 {
		level = maxLevel(level, LevelMultiAgent)
		reasons = append(reasons, "multiple host mentions")
	}

	if legacyContainsAny(input, []string{"@主机", "跨主机", "多个主机", "多主机", "多个目标主机", "child agent", "子 agent"}) {
		level = maxLevel(level, LevelMultiAgent)
		reasons = append(reasons, "multi-agent or multi-host wording")
	}

	mutationIntent := legacyContainsAny(input, []string{"恢复", "修复", "重启", "回滚", "迁移", "备份", "扩容", "缩容", "删除", "变更", "执行"})
	if (mutationIntent && !analysisOnly) || (strings.EqualFold(strings.TrimSpace(opts.Mode), "execute") && !isReadOnlyInspectionIntent(input) && !analysisOnly) {
		level = maxLevel(level, LevelOperations)
		reasons = append(reasons, "operation or mutation intent")
	}
	if analysisOnly {
		reasons = append(reasons, "analysis-only no-execution intent")
	}

	if legacyContainsAny(input, []string{"排查", "故障", "异常", "根因", "rca", "为什么", "错误", "不可用", "超时", "慢", "延迟", "报警", "告警", "incident", "健康检查", "状态检查", "关键指标"}) {
		level = maxLevel(level, LevelInvestigation)
		reasons = append(reasons, "investigation or RCA wording")
	}

	if legacyContainsAny(input, []string{"分析", "设计", "计划", "分步骤", "多步", "详细"}) {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "multi-step wording")
	}

	if level == LevelTrivial && legacyContainsAny(input, []string{"查看", "查询", "状态", "当前", "列表"}) {
		level = LevelSimpleRead
		reasons = append(reasons, "simple read wording")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "simple conversational request")
	}
	return profileFor(level, reasons, analysisOnly, executionProhibited)
}

func isEmptyIntentFrame(frame runtimecontract.IntentFrame) bool {
	return frame.Kind == runtimecontract.IntentKindUnknown &&
		len(frame.DataScopes) == 0 &&
		len(frame.RiskBudget) == 0 &&
		len(frame.Capabilities) == 0 &&
		!frame.Evidence.HasUserProvidedEvidence &&
		len(frame.Evidence.EvidenceKinds) == 0 &&
		len(frame.Evidence.DataScopes) == 0 &&
		len(frame.Evidence.WeakSignals) == 0
}

func riskBudgetIsReadOnly(risks []runtimecontract.ActionRisk) bool {
	if len(risks) == 0 {
		return false
	}
	for _, risk := range risks {
		if risk != runtimecontract.ActionRiskReadOnly {
			return false
		}
	}
	return true
}

func executionProhibitedByIntentFrame(frame runtimecontract.IntentFrame) bool {
	for _, constraint := range frame.Constraints {
		name := strings.ToLower(strings.TrimSpace(constraint.Name))
		value := strings.ToLower(strings.TrimSpace(constraint.Value))
		if name == "no_host_exec" || name == "no_execution" || name == "analysis_only" {
			return value == "" || value == "true" || value == "1" || value == "yes"
		}
	}
	return false
}

func hasDataScope(frame runtimecontract.IntentFrame, want runtimecontract.DataScope) bool {
	if runtimecontract.ContainsDataScope(frame.DataScopes, want) || runtimecontract.ContainsDataScope(frame.Evidence.DataScopes, want) {
		return true
	}
	for _, capability := range frame.Capabilities {
		if runtimecontract.ContainsDataScope(capability.DataScopes, want) {
			return true
		}
	}
	return false
}

func applyStructuralSignals(level Level, reasons []string, opts Options) (Level, []string) {
	input := strings.TrimSpace(opts.Input)
	if len(input) >= 2400 {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "long input")
	}
	if strings.Count(input, "```") >= 2 {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "fenced code block")
	}
	if count, _ := strconv.Atoi(metadataValue(opts.Metadata, "attachmentCount", "attachment_count", "attachments")); count > 0 {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "attachments present")
		if count >= 3 {
			level = maxLevel(level, LevelInvestigation)
			reasons = append(reasons, "multiple attachments")
		}
	}
	if count, _ := strconv.Atoi(metadataValue(opts.Metadata, "conversationTurnCount", "conversation_turn_count")); count >= 6 {
		level = maxLevel(level, LevelMultiStep)
		reasons = append(reasons, "multi-turn conversation")
	}
	return level, reasons
}

func isReadOnlyInspectionIntent(input string) bool {
	if input == "" {
		return false
	}
	if legacyContainsAny(input, []string{"恢复", "修复", "重启", "回滚", "迁移", "备份", "扩容", "缩容", "删除", "变更", "执行", "restart", "rollback", "delete", "remove", "scale", "migrate", "backup", "restore", "change"}) {
		return false
	}
	return legacyContainsAny(input, []string{
		"查看", "查询", "看下", "看一下", "获取", "显示", "列出", "列表", "当前", "状态", "资源", "使用率", "指标", "信息",
		"show", "view", "check", "inspect", "read", "get", "list", "status", "usage", "resource", "resources", "info",
	})
}

func profileFor(level Level, reasons []string, analysisOnly, executionProhibited bool) Profile {
	level = NormalizeLevel(string(level))
	if analysisOnly && AtLeast(level, LevelOperations) {
		level = LevelInvestigation
	}
	return Profile{
		Level:                level,
		Reasons:              append([]string(nil), reasons...),
		RequiresPlan:         AtLeast(level, LevelMultiStep),
		RequiresEvidence:     AtLeast(level, LevelInvestigation),
		RequiresValidation:   AtLeast(level, LevelOperations) && !analysisOnly,
		AllowsFirstTurnFinal: !AtLeast(level, LevelMultiStep),
		AnalysisOnly:         analysisOnly,
		ExecutionProhibited:  executionProhibited,
	}
}

func executionProhibitedByInput(input string) bool {
	return legacyContainsAny(input, []string{
		"不要执行", "不要连接", "不要采集", "不要运行", "不要操作",
		"不执行", "不连接", "不运行",
		"只基于", "仅基于",
		"do not execute", "do not connect", "don't execute", "don't connect",
		"without running", "without executing", "without connecting",
	})
}

func analysisOnlyIntent(input string) bool {
	return legacyContainsAny(input, []string{
		"只做原理分析", "原理分析", "证据清单", "证据分析", "基于下面证据", "基于证据",
		"只基于", "仅基于", "只分析", "仅分析", "只做分析",
		"可能原因", "什么原因", "为什么", "会导致",
		"principle analysis", "root-cause theory", "evidence checklist", "possible causes",
	})
}

func advisoryAnalysisOnlyFromMetadata(metadata map[string]string) bool {
	mode := strings.ToLower(strings.TrimSpace(metadataValue(metadata, "aiops.route.mode", "routeMode", "route_mode")))
	switch mode {
	case "chat_advisory", "advisory", "evidence_rca":
	default:
		return false
	}
	return metadataExplicitFalse(metadata, "aiops.tool.execCommandAllowed", "aiops.tool.hostMutationAllowed")
}

func metadataExplicitFalse(metadata map[string]string, keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(metadata[key])) {
		case "false", "0", "no", "n", "off":
			continue
		default:
			return false
		}
	}
	return true
}

func metadataBool(metadata map[string]string, keys ...string) bool {
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(metadata[key])) {
		case "true", "1", "yes", "y", "on":
			return true
		}
	}
	return false
}

func maxLevel(current, candidate Level) Level {
	if Rank(candidate) > Rank(current) {
		return candidate
	}
	return current
}

func legacyContainsAny(text string, needles []string) bool {
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
