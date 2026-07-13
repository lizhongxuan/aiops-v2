package runtimekernel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/agentstate"
)

const (
	FinalEvidenceConfidenceHigh   = "high"
	FinalEvidenceConfidenceMedium = "medium"
	FinalEvidenceConfidenceLow    = "low"

	FinalEvidenceActionAllow     = "allow"
	FinalEvidenceActionDowngrade = "downgrade"
	FinalEvidenceActionBlock     = "block"
)

type FinalEvidenceState struct {
	Checked            []CheckedEvidence  `json:"checked,omitempty"`
	NotChecked         []NotCheckedItem   `json:"notChecked,omitempty"`
	FailedTools        []FailedToolImpact `json:"failedTools,omitempty"`
	ApprovedActions    []string           `json:"approvedActions,omitempty"`
	PerformedActions   []string           `json:"performedActions,omitempty"`
	PostChecks         []string           `json:"postChecks,omitempty"`
	RequiredPostChecks []string           `json:"requiredPostChecks,omitempty"`
	Confidence         string             `json:"confidence"`
	ExecCommandAllowed bool               `json:"execCommandAllowed"`
	TargetBound        bool               `json:"targetBound"`
	// MutationIntentWithoutTarget is true when the current user request asks for
	// a mutating operation but the runtime did not bind a concrete target.
	MutationIntentWithoutTarget bool `json:"mutationIntentWithoutTarget"`
}

type CheckedEvidence struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type NotCheckedItem struct {
	ToolName             string `json:"toolName,omitempty"`
	ToolCallID           string `json:"toolCallId,omitempty"`
	Reason               string `json:"reason,omitempty"`
	RequiredAction       string `json:"requiredAction,omitempty"`
	SuggestedSearchQuery string `json:"suggestedSearchQuery,omitempty"`
}

type FailedToolImpact struct {
	ToolName     string `json:"toolName,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	FailureClass string `json:"failureClass,omitempty"`
	Impact       string `json:"impact,omitempty"`
}

type FinalEvidenceVerification struct {
	Action     string             `json:"action"`
	Confidence string             `json:"confidence"`
	Reasons    []string           `json:"reasons,omitempty"`
	State      FinalEvidenceState `json:"state"`
}

func BuildFinalEvidenceState(snapshot *TurnSnapshot, session *SessionState) FinalEvidenceState {
	state := FinalEvidenceState{
		Checked:            checkedEvidenceFromSnapshot(snapshot),
		FailedTools:        failedToolImpactsFromSnapshot(snapshot),
		ApprovedActions:    approvedActionsFromSnapshot(snapshot),
		PerformedActions:   performedActionsFromSnapshot(snapshot),
		RequiredPostChecks: requiredMutationPostChecksFromSnapshot(snapshot),
	}
	state.Checked = append(userProvidedEvidenceFromSnapshot(snapshot), state.Checked...)
	if session != nil {
		state.NotChecked = notCheckedItemsFromDiscovery(session.ToolDiscovery)
	}
	state.ExecCommandAllowed = finalEvidenceExecCommandAllowed(snapshot, session)
	state.TargetBound = finalEvidenceTargetBound(snapshot, session)
	state.MutationIntentWithoutTarget = finalEvidenceMutationIntentWithoutTarget(snapshot, session, state)
	state.Confidence = inferFinalEvidenceConfidence(state)
	return state
}

func approvedActionsFromSnapshot(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var actions []string
	for _, item := range snapshot.AgentItems {
		if item.Type != agentstate.TurnItemTypeApprovalDecided || item.Status != agentstate.ItemStatusCompleted {
			continue
		}
		var data approvalAgentItemData
		if json.Unmarshal(item.Payload.Data, &data) != nil || data.Status != "approved" {
			continue
		}
		if ref := finalActionRef(data.ToolName, data.ToolCallID); ref != "" {
			actions = append(actions, ref)
		}
	}
	return uniqueSortedHarnessStrings(actions)
}

func performedActionsFromSnapshot(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var actions []string
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if !invocation.Mutating || invocation.Status != ToolInvocationCompleted {
				continue
			}
			if ref := finalActionRef(invocation.ToolName, invocation.ToolCallID); ref != "" {
				actions = append(actions, ref)
			}
		}
	}
	return uniqueSortedHarnessStrings(actions)
}

func requiredMutationPostChecksFromSnapshot(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var refs []string
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Mutating {
				refs = append(refs, invocation.RequiredPostCheckRefs...)
			}
		}
	}
	return uniqueSortedHarnessStrings(refs)
}

func finalActionRef(toolName, toolCallID string) string {
	toolName = strings.TrimSpace(toolName)
	toolCallID = strings.TrimSpace(toolCallID)
	if toolName == "" {
		return toolCallID
	}
	if toolCallID == "" {
		return toolName
	}
	return toolName + "#" + toolCallID
}

// VerifyFinalEvidence is retained as a source-compatible adapter. Display text
// is deliberately ignored so wording cannot change the runtime decision.
func VerifyFinalEvidence(_ string, state FinalEvidenceState) FinalEvidenceVerification {
	return VerifyFinalEvidenceFacts(state)
}

func VerifyFinalEvidenceFacts(state FinalEvidenceState) FinalEvidenceVerification {
	state.Confidence = normalizeFinalEvidenceConfidence(state.Confidence)
	decision := FinalEvidenceVerification{
		Action:     FinalEvidenceActionAllow,
		Confidence: state.Confidence,
		State:      state,
	}
	if len(state.FailedTools) > 0 {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceMedium)
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "failed_tool_requires_lower_confidence")
	}
	if len(state.NotChecked) > 0 {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceLow)
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "not_checked_item_requires_lower_confidence")
	}
	if len(outstandingRequiredPostChecks(state)) > 0 {
		decision.Action = FinalEvidenceActionDowngrade
		decision.Confidence = minFinalEvidenceConfidence(decision.Confidence, FinalEvidenceConfidenceMedium)
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "required_postcheck_incomplete")
	}
	if state.MutationIntentWithoutTarget {
		decision.Action = FinalEvidenceActionBlock
		decision.Confidence = FinalEvidenceConfidenceLow
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "mutation_intent_requires_explicit_target_binding")
		decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "no_explicit_target_binding")
		if !state.ExecCommandAllowed {
			decision.Reasons = appendFinalEvidenceReason(decision.Reasons, "exec_command_not_allowed")
		}
	}
	return decision
}

func finalEvidenceExecCommandAllowed(snapshot *TurnSnapshot, session *SessionState) bool {
	if snapshot != nil {
		metadata := snapshot.Metadata
		if metadataBool(metadata["aiops.tool.execCommandAllowed"]) || metadataBool(metadata["aiops.route.allowsExecCommand"]) {
			return true
		}
		if snapshot.SessionType == SessionTypeHost && snapshot.Mode == ModeExecute && strings.TrimSpace(sessionHostID(session)) != "" {
			return true
		}
	}
	return false
}

func finalEvidenceTargetBound(snapshot *TurnSnapshot, session *SessionState) bool {
	if snapshot != nil {
		metadata := snapshot.Metadata
		binding := strings.ToLower(strings.TrimSpace(metadata["aiops.target.binding"]))
		if binding == "host" || binding == "multi_host" || binding == "resource" {
			return true
		}
		if strings.TrimSpace(metadata["aiops.target.hostId"]) != "" || strings.TrimSpace(metadata["aiops.target.refs"]) != "" {
			return true
		}
		if snapshot.SessionType == SessionTypeHost && strings.TrimSpace(sessionHostID(session)) != "" {
			return true
		}
	}
	return strings.TrimSpace(sessionHostID(session)) != ""
}

func sessionHostID(session *SessionState) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.HostID)
}

func finalEvidenceMutationIntentWithoutTarget(snapshot *TurnSnapshot, session *SessionState, state FinalEvidenceState) bool {
	if state.TargetBound || state.ExecCommandAllowed {
		return false
	}
	if finalEvidenceAnalysisOnlyOrUserEvidenceRCA(snapshot) {
		return false
	}
	if !finalEvidenceNoTargetBindingGuardApplies(snapshot) {
		return false
	}
	return finalEvidenceLatestUserMutationIntent(session)
}

func finalEvidenceAnalysisOnlyOrUserEvidenceRCA(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	metadata := snapshot.Metadata
	if metadataBool(metadata["taskDepth.analysisOnly"]) ||
		metadataBool(metadata["taskDepth.executionProhibited"]) ||
		metadataBool(metadata["aiops.execution.prohibited"]) ||
		metadataBool(metadata["aiops.route.userProhibitedHostExec"]) {
		return true
	}
	mode := strings.ToLower(strings.TrimSpace(metadata["aiops.route.mode"]))
	if mode == "evidence_rca" && metadataBool(metadata["aiops.userEvidence.present"]) {
		return true
	}
	return false
}

func finalEvidenceNoTargetBindingGuardApplies(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	metadata := snapshot.Metadata
	if strings.EqualFold(strings.TrimSpace(metadata["aiops.target.binding"]), "none") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(metadata["aiops.route.mode"])) {
	case "chat_advisory", "advisory":
		return true
	default:
		return false
	}
}

func finalEvidenceLatestUserMutationIntent(session *SessionState) bool {
	if session == nil {
		return false
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		msg := session.Messages[i]
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			return containsOperationalMutationIntent(msg.Content)
		}
	}
	return false
}

func containsOperationalMutationIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	if hasAnyRiskMarker(lower, []string{
		"只做原理分析",
		"只做分析",
		"不要连接或执行",
		"不连接或执行",
		"不要执行任何",
		"不执行任何",
		"不要重启",
		"不要 重启",
		"不重启",
		"不要执行",
		"不要变更",
		"do not restart",
		"don't restart",
		"no restart",
	}) {
		return false
	}
	if looksLikeOperationalAnalysisQuestion(lower) {
		return false
	}
	if containsMutationCommandAdvice(lower) {
		return true
	}
	return hasAnyRiskMarker(lower, []string{
		"重启",
		"启动",
		"停止",
		"重载",
		"发布",
		"部署",
		"安装",
		"升级",
		"更新",
		"清理",
		"删除",
		"修复",
		"回滚",
		"扩容",
		"缩容",
		"切换",
		"执行",
		"restart",
		" start ",
		" stop ",
		" reload",
		"deploy",
		"install",
		"upgrade",
		"update",
		"delete",
		"remove",
		"repair",
		"fix",
		"rollback",
		"rollout restart",
		" scale ",
		" apply ",
		" patch ",
	})
}

func looksLikeOperationalAnalysisQuestion(lower string) bool {
	if !hasAnyRiskMarker(lower, []string{
		"为什么",
		"什么原因",
		"有哪些原因",
		"原因会导致",
		"导致",
		"分析",
		"解释",
		"why",
		"cause",
		"reason",
		"explain",
	}) {
		return false
	}
	if hasAnyRiskMarker(lower, []string{
		"帮我执行",
		"请执行",
		"执行一下",
		"现在执行",
		"立即执行",
		"帮我重启",
		"请重启",
		"帮我清理",
		"请清理",
		"帮我删除",
		"请删除",
		"run this",
		"execute this",
		"please run",
		"please execute",
	}) {
		return false
	}
	return true
}

func finalAnswerAsksExplicitTargetBinding(answer string) bool {
	lower := strings.ToLower(strings.TrimSpace(answer))
	if lower == "" {
		return false
	}
	return hasAnyRiskMarker(lower, []string{
		"@host",
		"@ip",
		"明确绑定",
		"绑定目标",
		"选择目标",
		"指定目标",
		"指定主机",
		"目标主机",
		"选择主机",
		"select a target",
		"bind a target",
		"explicit target",
		"target binding",
	})
}

func checkedEvidenceFromSnapshot(snapshot *TurnSnapshot) []CheckedEvidence {
	if snapshot == nil {
		return nil
	}
	var out []CheckedEvidence
	seen := map[string]bool{}
	for _, iter := range snapshot.Iterations {
		toolNames := map[string]string{}
		for _, call := range iter.ToolCalls {
			toolNames[call.ID] = call.Name
		}
		for _, result := range iter.ToolResults {
			if strings.TrimSpace(result.Error) != "" {
				continue
			}
			if isCoveredReadReuseResult(result) {
				continue
			}
			toolName := strings.TrimSpace(toolNames[result.ToolCallID])
			summary := checkedEvidenceSummaryForToolResult(toolName, result)
			if summary == "" {
				continue
			}
			item := CheckedEvidence{
				ToolCallID: strings.TrimSpace(result.ToolCallID),
				ToolName:   toolName,
				Summary:    truncateRunes(summary, 180),
			}
			key := item.ToolCallID + "\x00" + item.ToolName + "\x00" + item.Summary
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

func checkedEvidenceSummaryForToolResult(toolName string, result ToolResult) string {
	normalizedTool := strings.ToLower(strings.TrimSpace(toolName))
	if normalizedTool == "tool_search" {
		return ""
	}
	if summary, ok := publicWebCheckedEvidenceSummary(normalizedTool, result); ok {
		return summary
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = firstNonEmptyLine(result.Content)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	if looksLikeToolDiscoveryPayload(summary) || looksLikeToolDiscoveryPayload(result.Content) {
		return ""
	}
	if structured := structuredEvidenceSummaryForUser(toolName, summary); structured != "" {
		return structured
	}
	if structured := structuredEvidenceSummaryForUser(toolName, result.Content); structured != "" {
		return structured
	}
	if containsInternalFinalFallbackText(summary) {
		return ""
	}
	summary = sanitizeIncompleteFinalUserLine(summary)
	if summary == "" || containsInternalFinalFallbackText(summary) {
		return ""
	}
	return summary
}

func publicWebCheckedEvidenceSummary(toolName string, result ToolResult) (string, bool) {
	if toolName != "web_search" && toolName != "browse_url" {
		return "", false
	}
	raw := strings.TrimSpace(result.Content)
	if raw == "" {
		raw = strings.TrimSpace(result.Summary)
	}
	type publicWebEnvelope struct {
		Operation string `json:"operation"`
		Query     string `json:"query"`
		URL       string `json:"url"`
		Source    string `json:"source"`
		Results   []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
		Meta struct {
			Backend  string `json:"backend"`
			FinalURL string `json:"finalUrl"`
		} `json:"meta"`
	}
	var env publicWebEnvelope
	if raw != "" && json.Unmarshal([]byte(raw), &env) == nil && (env.Operation != "" || env.Source != "" || len(env.Results) > 0) {
		sourceCount := len(env.Results)
		switch strings.ToLower(strings.TrimSpace(env.Operation)) {
		case "open":
			url := firstNonEmpty(env.URL, env.Meta.FinalURL)
			if url != "" {
				return "公开网页读取已返回来源：" + url, true
			}
			return "公开网页读取已完成。", true
		default:
			query := strings.TrimSpace(env.Query)
			if query != "" {
				return fmt.Sprintf("公开网页搜索已返回 %d 个来源：%s", sourceCount, truncateRunes(query, 80)), true
			}
			return fmt.Sprintf("公开网页搜索已返回 %d 个来源。", sourceCount), true
		}
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = firstNonEmptyLine(result.Content)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" || looksLikeToolDiscoveryPayload(summary) || containsInternalFinalFallbackText(summary) {
		return "", true
	}
	return sanitizeIncompleteFinalUserLine(summary), true
}

func looksLikeToolDiscoveryPayload(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") {
		return false
	}
	var payload map[string]any
	if json.Unmarshal([]byte(text), &payload) != nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(anyString(payload["mode"])))
	if mode == "search" || mode == "select" || mode == "describe" {
		return true
	}
	errorType := strings.ToLower(strings.TrimSpace(anyString(payload["errorType"])))
	if strings.HasPrefix(errorType, "tool_") || errorType == "mcp_unavailable" || errorType == "dedicated_tool_preferred" {
		return true
	}
	for _, key := range []string{"selection", "descriptions", "loadedTools", "loadedPacks", "suggestedSearchQuery", "requiredAction"} {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func snapshotHasUserProvidedEvidence(snapshot *TurnSnapshot) bool {
	if snapshot == nil || len(snapshot.Metadata) == 0 {
		return false
	}
	if !metadataBool(snapshot.Metadata["aiops.userEvidence.present"]) {
		return false
	}
	return strings.TrimSpace(snapshot.Metadata["aiops.userEvidence.rawExcerpt"]) != "" ||
		strings.TrimSpace(snapshot.Metadata["aiops.userEvidence.kinds"]) != "" ||
		strings.TrimSpace(snapshot.Metadata["aiops.userEvidence.signals"]) != ""
}

func userProvidedEvidenceFromSnapshot(snapshot *TurnSnapshot) []CheckedEvidence {
	if !snapshotHasUserProvidedEvidence(snapshot) {
		return nil
	}
	metadata := snapshot.Metadata
	parts := []string{}
	if kinds := strings.TrimSpace(metadata["aiops.userEvidence.kinds"]); kinds != "" {
		parts = append(parts, "用户提供了"+strings.ReplaceAll(kinds, ",", "、")+"证据")
	}
	if signals := strings.TrimSpace(metadata["aiops.userEvidence.signals"]); signals != "" {
		parts = append(parts, "识别到信号："+strings.ReplaceAll(signals, ",", "、"))
	}
	if excerpt := strings.TrimSpace(metadata["aiops.userEvidence.rawExcerpt"]); excerpt != "" {
		parts = append(parts, "摘录："+excerpt)
	}
	summary := strings.Join(parts, "; ")
	if summary == "" {
		summary = "user-provided evidence present"
	}
	return []CheckedEvidence{{
		ToolName: "user_provided_evidence",
		Summary:  truncateRunes(summary, 180),
	}}
}

func failedToolImpactsFromSnapshot(snapshot *TurnSnapshot) []FailedToolImpact {
	summaries := failedToolSummariesFromSnapshot(snapshot)
	if len(summaries) == 0 {
		return nil
	}
	out := make([]FailedToolImpact, 0, len(summaries))
	seen := map[string]bool{}
	for _, summary := range summaries {
		toolName := strings.TrimSpace(summary.Tool)
		failureClass := strings.TrimSpace(summary.FailureClass)
		key := toolName + "\x00" + failureClass
		if seen[key] {
			continue
		}
		seen[key] = true
		impact := "证据读取失败，不能作为已检查结果"
		if failureClass == "partial_result" {
			impact = "部分子任务未完成，聚合结果不完整"
		}
		out = append(out, FailedToolImpact{
			ToolName:     toolName,
			FailureClass: failureClass,
			Impact:       impact,
		})
	}
	return out
}

func notCheckedItemsFromDiscovery(discovery ToolDiscoverySessionState) []NotCheckedItem {
	if len(discovery.RejectedCalls) == 0 {
		return nil
	}
	out := make([]NotCheckedItem, 0, len(discovery.RejectedCalls))
	for _, call := range discovery.RejectedCalls {
		switch strings.TrimSpace(call.ErrorType) {
		case "tool_unloaded", "tool_hidden_by_policy", "tool_not_found", "dedicated_tool_preferred", "mcp_unavailable":
		default:
			continue
		}
		out = append(out, NotCheckedItem{
			ToolName:             strings.TrimSpace(call.ToolName),
			ToolCallID:           strings.TrimSpace(call.ToolCallID),
			Reason:               strings.TrimSpace(call.ErrorType),
			RequiredAction:       strings.TrimSpace(call.RequiredAction),
			SuggestedSearchQuery: strings.TrimSpace(call.SuggestedSearchQuery),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ToolName == out[j].ToolName {
			return out[i].Reason < out[j].Reason
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out
}

func inferFinalEvidenceConfidence(state FinalEvidenceState) string {
	if len(state.Checked) == 0 {
		return FinalEvidenceConfidenceLow
	}
	if len(state.FailedTools) > 0 || len(state.NotChecked) > 0 || len(outstandingRequiredPostChecks(state)) > 0 {
		return FinalEvidenceConfidenceMedium
	}
	return FinalEvidenceConfidenceHigh
}

func normalizeFinalEvidenceConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case FinalEvidenceConfidenceHigh:
		return FinalEvidenceConfidenceHigh
	case FinalEvidenceConfidenceMedium:
		return FinalEvidenceConfidenceMedium
	case FinalEvidenceConfidenceLow:
		return FinalEvidenceConfidenceLow
	default:
		return FinalEvidenceConfidenceLow
	}
}

func minFinalEvidenceConfidence(current, cap string) string {
	current = normalizeFinalEvidenceConfidence(current)
	cap = normalizeFinalEvidenceConfidence(cap)
	if finalEvidenceConfidenceRank(current) <= finalEvidenceConfidenceRank(cap) {
		return current
	}
	return cap
}

func finalEvidenceConfidenceRank(value string) int {
	switch normalizeFinalEvidenceConfidence(value) {
	case FinalEvidenceConfidenceHigh:
		return 3
	case FinalEvidenceConfidenceMedium:
		return 2
	default:
		return 1
	}
}

func finalAnswerClaimsChecked(answer string) bool {
	text := strings.ToLower(answer)
	for _, marker := range []string{
		"已检查", "已确认", "确认全部", "全部检查", "checked", "verified", "confirmed", "inspected",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func finalAnswerClaimsHighConfidence(answer string) bool {
	text := strings.ToLower(answer)
	for _, marker := range []string{"高置信", "confirmed", "definitely", "no issue", "normal"} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	compact := compactFinalEvidenceText(text)
	for _, marker := range []string{"置信度:高", "置信度：高", "置信度高", "confidence:high", "confidence：high"} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	for _, marker := range []string{"确定", "明确", "正常"} {
		if containsAffirmedChineseMarker(text, marker) {
			return true
		}
	}
	return false
}

func finalAnswerClaimsMissingEvidence(answer string) bool {
	text := strings.ToLower(answer)
	for _, marker := range []string{
		"缺失证据", "缺乏", "无法收集", "无法获取", "无法完成", "无法确定", "未配置", "not_configured", "not configured", "missing_evidence", "missing evidence",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func compactFinalEvidenceText(text string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "")
	return replacer.Replace(text)
}

func containsAffirmedChineseMarker(text, marker string) bool {
	for offset := 0; ; {
		index := strings.Index(text[offset:], marker)
		if index < 0 {
			return false
		}
		absolute := offset + index
		if !hasChineseNegationPrefix(text[:absolute]) {
			return true
		}
		offset = absolute + len(marker)
	}
}

func hasChineseNegationPrefix(prefix string) bool {
	runes := []rune(prefix)
	if len(runes) > 6 {
		runes = runes[len(runes)-6:]
	}
	tail := string(runes)
	for _, marker := range []string{
		"无法", "不能", "未能", "不可", "不", "并未", "没有", "缺乏",
	} {
		if strings.Contains(tail, marker) {
			return true
		}
	}
	return false
}

func appendFinalEvidenceReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func finalEvidenceBlockedFallback(decision FinalEvidenceVerification) string {
	if finalEvidenceNeedsRiskReviewFallback(decision) {
		return "安全结论：该建议包含高风险或破坏性运维动作，当前只能作为风险审查，不能作为直接执行步骤。\n\n必须先完成：只读证据确认、目标与角色确认、备份或快照、维护/停服窗口、人工审批、回滚方案和执行后验收。涉及数据目录、归档/WAL、主从切换、重建副本或权威数据源变更的动作，只能作为审批后的候选动作；在这些前置条件缺失时，应先补齐证据，并把结论保持在受限证据边界内。"
	}
	if finalEvidenceHasReason(decision, riskyAdviceCategoryUngatedMutationCommandAdvice) ||
		finalEvidenceHasReason(decision, "exec_command_not_allowed") ||
		finalEvidenceHasReason(decision, "no_explicit_target_binding") ||
		finalEvidenceHasReason(decision, "mutation_intent_requires_explicit_target_binding") {
		return "不能继续执行或提供变更命令：当前请求没有明确绑定可执行目标，或当前模式不允许变更执行。请在输入中使用 @host、@IP，或先在界面选择目标资源后重新发起；绑定后系统会展示审批卡，批准后再执行并做只读验收。"
	}
	return "当前答案缺少必须的证据或安全字段，不能作为最终结论。请补充只读证据、影响范围、回滚和验收信息后再继续。"
}

func finalEvidenceNeedsRiskReviewFallback(decision FinalEvidenceVerification) bool {
	for _, reason := range []string{
		"destructive_archive_or_data_deletion",
		"high_risk_replication_repair",
		"unsupported_timeline_authority_inference",
	} {
		if finalEvidenceHasReason(decision, reason) {
			return true
		}
	}
	return false
}

func finalEvidenceHasReason(decision FinalEvidenceVerification, reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
	}
	for _, item := range decision.Reasons {
		if strings.TrimSpace(item) == reason {
			return true
		}
	}
	return false
}
