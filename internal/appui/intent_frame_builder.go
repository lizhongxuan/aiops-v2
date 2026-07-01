package appui

import (
	"strings"

	"aiops-v2/internal/runtimecontract"
)

type AttachmentSummary struct {
	Name        string `json:"name,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

func BuildEvidenceEnvelope(input string, attachments []AttachmentSummary, metadata map[string]any) runtimecontract.EvidenceEnvelope {
	extraction := ExtractUserEvidence(input)
	envelope := runtimecontract.EvidenceEnvelope{}
	if extraction.HasEvidence {
		envelope.HasUserProvidedEvidence = true
		envelope.DataScopes = runtimecontract.AppendDataScope(envelope.DataScopes, runtimecontract.DataScopeWorkspace)
		for _, kind := range extraction.EvidenceKinds {
			envelope.EvidenceKinds = appendUniqueEvidenceString(envelope.EvidenceKinds, normalizeEvidenceKind(kind))
		}
		envelope.WeakSignals = appendEvidenceWeakSignals(envelope.WeakSignals, extraction)
	}
	if len(attachments) > 0 {
		envelope.HasUserProvidedEvidence = true
		envelope.EvidenceKinds = appendUniqueEvidenceString(envelope.EvidenceKinds, "attachment")
		envelope.DataScopes = runtimecontract.AppendDataScope(envelope.DataScopes, runtimecontract.DataScopeWorkspace)
	}
	envelope.DataScopes = metadataDataScopes(metadata, envelope.DataScopes)
	return envelope
}

func BuildIntentFrame(input string, envelope runtimecontract.EvidenceEnvelope, metadata map[string]any) runtimecontract.IntentFrame {
	lower := strings.ToLower(strings.TrimSpace(input))
	frame := runtimecontract.IntentFrame{
		Kind:       metadataIntentKind(metadata),
		DataScopes: append([]runtimecontract.DataScope(nil), envelope.DataScopes...),
		Evidence:   envelope,
		Confidence: runtimecontract.ConfidenceMedium,
		Classifier: "appui.intent_frame_builder.v1",
	}
	if frame.Kind == "" {
		frame.Kind = inferIntentKind(lower, envelope)
	}
	if frame.Kind == runtimecontract.IntentKindUnknown {
		frame.Confidence = runtimecontract.ConfidenceLow
	}
	noHostExec := evidenceConstraintNoHostExec(lower)
	if noHostExec {
		frame.Constraints = append(frame.Constraints, runtimecontract.IntentConstraint{
			Name:       "no_host_exec",
			Value:      "true",
			Confidence: runtimecontract.ConfidenceHigh,
			Source:     "user",
		})
	}
	noPublicWeb := userProhibitsPublicWeb(lower)
	if noPublicWeb {
		frame.Constraints = append(frame.Constraints, runtimecontract.IntentConstraint{
			Name:       "no_public_web",
			Value:      "true",
			Confidence: runtimecontract.ConfidenceHigh,
			Source:     "user",
		})
	}
	if !noPublicWeb && intentRequestsPublicWeb(lower, frame.Kind, metadata) {
		frame.Kind = runtimecontract.IntentKindResearch
		frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopePublicWeb)
		frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskNetwork)
		frame.Capabilities = appendCapabilityCandidate(frame.Capabilities, runtimecontract.CapabilityCandidate{
			Name:       "public_web_research",
			DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
			Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskNetwork},
			Reasons:    []string{"public information requested"},
		})
	}
	if !noPublicWeb && intentNeedsExternalOpsKnowledge(lower, frame.Kind, metadata) {
		frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopeOpsKnowledge)
		frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopePublicWeb)
		frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskNetwork)
		frame.Capabilities = appendCapabilityCandidate(frame.Capabilities, runtimecontract.CapabilityCandidate{
			Name:       "ops_reference_research",
			DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge, runtimecontract.DataScopePublicWeb},
			Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly, runtimecontract.ActionRiskNetwork},
			Reasons:    []string{"operational diagnosis needs external reference knowledge"},
		})
	}
	if intentRequestsOpsKnowledge(lower, metadata) {
		frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopeOpsKnowledge)
		frame.Capabilities = appendCapabilityCandidate(frame.Capabilities, runtimecontract.CapabilityCandidate{
			Name:       "search_ops_manuals",
			DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge},
			Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
			Reasons:    []string{"operational knowledge requested"},
		})
	}
	hostRuntimeRequested := intentRequestsHostRuntime(lower, metadata)
	if frame.Kind == runtimecontract.IntentKindUnknown && hostRuntimeRequested {
		frame.Kind = runtimecontract.IntentKindVerify
		frame.Confidence = runtimecontract.ConfidenceMedium
	}
	if hostRuntimeRequested && !noHostExec {
		frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime)
		frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec)
		frame.Capabilities = appendCapabilityCandidate(frame.Capabilities, runtimecontract.CapabilityCandidate{
			Name:       "host_runtime_inspection",
			DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
			Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
			Reasons:    []string{"local runtime inspection requested"},
		})
	}
	if frame.Kind == runtimecontract.IntentKindChange || intentMentionsMutation(lower) {
		frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskWrite)
	}
	if len(frame.RiskBudget) == 0 {
		frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskReadOnly)
	}
	frame.DataScopes = metadataDataScopes(metadata, frame.DataScopes)
	return runtimecontract.NormalizeIntentFrame(frame)
}

func evidenceEnvelopeFromUserEvidence(extraction UserEvidenceExtraction) runtimecontract.EvidenceEnvelope {
	envelope := runtimecontract.EvidenceEnvelope{}
	if !extraction.HasEvidence {
		return envelope
	}
	envelope.HasUserProvidedEvidence = true
	envelope.DataScopes = runtimecontract.AppendDataScope(envelope.DataScopes, runtimecontract.DataScopeWorkspace)
	for _, kind := range extraction.EvidenceKinds {
		envelope.EvidenceKinds = appendUniqueEvidenceString(envelope.EvidenceKinds, normalizeEvidenceKind(kind))
	}
	envelope.WeakSignals = appendEvidenceWeakSignals(envelope.WeakSignals, extraction)
	return envelope
}

func appendEvidenceWeakSignals(signals []runtimecontract.WeakSignal, extraction UserEvidenceExtraction) []runtimecontract.WeakSignal {
	for _, kind := range extraction.EvidenceKinds {
		switch normalizeEvidenceKind(kind) {
		case runtimecontract.EvidenceKindLog:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalLogLikeText, "evidence_extractor", "log-like text")
		case runtimecontract.EvidenceKindCommandOutput:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalCommandOutputLike, "evidence_extractor", "command-output-like text")
		case runtimecontract.EvidenceKindSQLResult:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalSQLResultLikeText, "evidence_extractor", "sql-result-like text")
		case runtimecontract.EvidenceKindMonitoring:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalMonitoringLikeText, "evidence_extractor", "monitoring-like text")
		case runtimecontract.EvidenceKindStackTrace:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalStackTraceLikeText, "evidence_extractor", "stack-trace-like text")
		case runtimecontract.EvidenceKindConfig:
			signals = appendWeakSignal(signals, runtimecontract.WeakSignalConfigLikeText, "evidence_extractor", "config-like text")
		}
	}
	for _, signal := range extraction.Signals {
		name := genericWeakSignalName(signal)
		if name != "" {
			signals = appendWeakSignal(signals, name, "evidence_extractor", "generic evidence pattern")
		}
	}
	return signals
}

func normalizeEvidenceKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "log":
		return runtimecontract.EvidenceKindLog
	case "command_output":
		return runtimecontract.EvidenceKindCommandOutput
	case "sql_result":
		return runtimecontract.EvidenceKindSQLResult
	case "monitoring":
		return runtimecontract.EvidenceKindMonitoring
	case "stack_trace":
		return runtimecontract.EvidenceKindStackTrace
	case "config":
		return runtimecontract.EvidenceKindConfig
	default:
		return strings.TrimSpace(kind)
	}
}

func genericWeakSignalName(signal string) string {
	signal = strings.ToLower(strings.TrimSpace(signal))
	if signal == "" {
		return ""
	}
	if strings.Contains(signal, "timeline") || strings.Contains(signal, "history") {
		return runtimecontract.WeakSignalTimelineLikeSequence
	}
	if strings.Contains(signal, "recovery") || strings.Contains(signal, "restore") || strings.Contains(signal, "config") {
		return runtimecontract.WeakSignalConfigLikeText
	}
	if strings.Contains(signal, "latency") || strings.Contains(signal, "checkpoint") {
		return runtimecontract.WeakSignalMonitoringLikeText
	}
	return ""
}

func appendWeakSignal(values []runtimecontract.WeakSignal, name, source, summary string) []runtimecontract.WeakSignal {
	name = strings.TrimSpace(name)
	if name == "" {
		return values
	}
	for _, value := range values {
		if value.Name == name {
			return values
		}
	}
	return append(values, runtimecontract.WeakSignal{
		Name:       name,
		Source:     strings.TrimSpace(source),
		Confidence: runtimecontract.ConfidenceMedium,
		Summary:    strings.TrimSpace(summary),
	})
}

func appendCapabilityCandidate(values []runtimecontract.CapabilityCandidate, next runtimecontract.CapabilityCandidate) []runtimecontract.CapabilityCandidate {
	next.Name = strings.TrimSpace(next.Name)
	if next.Name == "" {
		return values
	}
	for _, value := range values {
		if value.Name == next.Name {
			return values
		}
	}
	return append(values, next)
}

func inferIntentKind(lower string, envelope runtimecontract.EvidenceEnvelope) runtimecontract.IntentKind {
	switch {
	case containsAnyIntentMarker(lower, "解释", "说明", "原理", "explain"):
		return runtimecontract.IntentKindExplain
	case containsAnyIntentMarker(lower, "runbook", "操作手册", "排查手册"):
		return runtimecontract.IntentKindRunbookAuthoring
	case containsAnyIntentMarker(lower, "计划", "方案", "plan", "先不要执行"):
		return runtimecontract.IntentKindPlan
	case containsAnyIntentMarker(lower, "定位", "排查", "根因", "为什么", "异常", "故障", "diagnose", "rca"):
		return runtimecontract.IntentKindDiagnose
	case containsAnyIntentMarker(lower, "修复", "变更", "回滚", "重启", "执行修复", "change", "rollback", "restart"):
		return runtimecontract.IntentKindChange
	case containsAnyIntentMarker(lower, "验证", "确认", "检查", "verify", "check"):
		return runtimecontract.IntentKindVerify
	case envelope.HasUserProvidedEvidence:
		return runtimecontract.IntentKindDiagnose
	default:
		return runtimecontract.IntentKindUnknown
	}
}

func intentRequestsPublicWeb(lower string, kind runtimecontract.IntentKind, metadata map[string]any) bool {
	if kind == runtimecontract.IntentKindResearch || metadataBoolAny(metadata, "aiops.intent.publicWeb") {
		return true
	}
	return containsAnyIntentMarker(lower, "公开资料", "官方文档", "联网", "网页", "docs", "documentation", "source", "cite")
}

func intentNeedsExternalOpsKnowledge(lower string, kind runtimecontract.IntentKind, metadata map[string]any) bool {
	if metadataBoolAny(metadata, "aiops.intent.externalOpsKnowledge") {
		return true
	}
	switch kind {
	case runtimecontract.IntentKindDiagnose, runtimecontract.IntentKindExplain, runtimecontract.IntentKindPlan, runtimecontract.IntentKindVerify:
	default:
		return false
	}
	return containsAnyIntentMarker(lower,
		"kubernetes", "k8s", "pod", "container", "容器", "deployment", "ingress", "service",
		"linux", "systemd", "nginx", "apache", "dns", "network", "网络", "防火墙",
		"postgres", "postgresql", "mysql", "redis", "database", "数据库",
		"backup", "restore", "recovery", "replication", "failover", "cluster", "timeline", "wal",
		"备份", "恢复", "归档", "复制", "主从", "集群", "时间线",
		"crashloopbackoff", "oom", "connection refused", "connection timed out", "no such host",
	)
}

func userProhibitsPublicWeb(lower string) bool {
	return containsAnyIntentMarker(lower,
		"不要联网",
		"不联网",
		"不要搜索",
		"不搜索",
		"不要查网页",
		"不要查公开资料",
		"只基于本地",
		"仅基于本地",
		"只基于上下文",
		"仅基于上下文",
		"do not browse",
		"do not search",
		"without browsing",
		"without web",
	)
}

func intentRequestsOpsKnowledge(lower string, metadata map[string]any) bool {
	if metadataBoolAny(metadata, "aiops.intent.opsKnowledge") {
		return true
	}
	return containsAnyIntentMarker(lower, "runbook", "操作手册", "排查手册", "运维知识")
}

func intentRequestsHostRuntime(lower string, metadata map[string]any) bool {
	if metadataBoolAny(metadata, "aiops.intent.hostRuntime") {
		return true
	}
	if containsAnyIntentMarker(lower, "本机", "主机", "执行命令", "运行命令", "host", "runtime", "shell") {
		return true
	}
	hasReadOperation := containsAnyIntentMarker(lower,
		"查看", "检查", "读取", "获取", "看一下", "看下", "观测", "检测", "巡检",
		"check", "inspect", "read", "show", "get",
	)
	if !hasReadOperation {
		return false
	}
	return containsAnyIntentMarker(lower,
		"cpu", "处理器", "内存", "memory", "磁盘", "disk", "负载", "load",
		"进程", "process", "端口", "port", "网络", "network", "系统状态",
		"资源", "resource", "uptime", "top", "df", "free", "vm_stat", "lscpu",
	)
}

func intentMentionsMutation(lower string) bool {
	return containsAnyIntentMarker(lower, "修复", "变更", "回滚", "迁移", "删除", "扩容", "缩容", "执行修复", "write", "change", "rollback", "delete", "migrate")
}

func containsAnyIntentMarker(text string, markers ...string) bool {
	if text == "" {
		return false
	}
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker != "" && strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func metadataIntentKind(metadata map[string]any) runtimecontract.IntentKind {
	value := metadataStringAny(metadata, runtimecontract.MetadataIntentKind)
	switch runtimecontract.IntentKind(value) {
	case runtimecontract.IntentKindDiagnose,
		runtimecontract.IntentKindExplain,
		runtimecontract.IntentKindPlan,
		runtimecontract.IntentKindChange,
		runtimecontract.IntentKindVerify,
		runtimecontract.IntentKindResearch,
		runtimecontract.IntentKindConfigure,
		runtimecontract.IntentKindRunbookAuthoring:
		return runtimecontract.IntentKind(value)
	default:
		return ""
	}
}

func metadataDataScopes(metadata map[string]any, existing []runtimecontract.DataScope) []runtimecontract.DataScope {
	out := append([]runtimecontract.DataScope(nil), existing...)
	for _, value := range strings.Split(metadataStringAny(metadata, runtimecontract.MetadataIntentDataScopes), ",") {
		out = runtimecontract.AppendDataScope(out, runtimecontract.DataScope(strings.TrimSpace(value)))
	}
	return out
}

func metadataStringAny(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return strings.Join(typed, ",")
	case []runtimecontract.DataScope:
		parts := make([]string, 0, len(typed))
		for _, scope := range typed {
			parts = append(parts, string(scope))
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func metadataBoolAny(metadata map[string]any, key string) bool {
	value := strings.ToLower(metadataStringAny(metadata, key))
	return value == "true" || value == "1" || value == "yes"
}
