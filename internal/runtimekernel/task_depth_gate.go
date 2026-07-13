package runtimekernel

import (
	"encoding/json"
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/taskdepth"
)

const prematureFinalGuardMetadataKey = "taskDepth.prematureFinalGuardTriggered"
const completeFollowupMetadataKey = "aiops.answer.requireCompleteFollowup"
const smalltalkOnlyMetadataKey = "aiops.answer.smalltalkOnly"

type PlanRequirementDecision struct {
	Required      bool     `json:"required"`
	Reason        string   `json:"reason,omitempty"`
	ReminderLevel string   `json:"reminderLevel,omitempty"`
	Missing       []string `json:"missing,omitempty"`
}

func depthProfileFromTurnRequest(req TurnRequest) taskdepth.Profile {
	if frame, ok := intentFrameFromTurnMetadata(req.Metadata); ok {
		return taskdepth.ClassifyFromIntentFrame(frame, taskdepth.Options{
			Input:    req.Input,
			Mode:     string(req.Mode),
			Metadata: req.Metadata,
		})
	}
	return taskdepth.Classify(taskdepth.Options{
		Input:    req.Input,
		Mode:     string(req.Mode),
		Metadata: req.Metadata,
	})
}

func intentFrameFromTurnMetadata(metadata map[string]string) (runtimecontract.IntentFrame, bool) {
	if len(metadata) == 0 {
		return runtimecontract.IntentFrame{}, false
	}
	if raw := strings.TrimSpace(metadata[runtimecontract.MetadataIntentFrame]); raw != "" {
		var frame runtimecontract.IntentFrame
		if err := json.Unmarshal([]byte(raw), &frame); err == nil {
			return runtimecontract.NormalizeIntentFrame(frame), true
		}
	}
	frame := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKind(strings.TrimSpace(metadata[runtimecontract.MetadataIntentKind])),
		DataScopes: metadataDataScopes(metadata[runtimecontract.MetadataIntentDataScopes]),
		RiskBudget: metadataActionRisks(metadata[runtimecontract.MetadataIntentRiskBudget]),
		Confidence: strings.TrimSpace(metadata[runtimecontract.MetadataIntentConfidence]),
	}
	if frame.Kind == "" && len(frame.DataScopes) == 0 && len(frame.RiskBudget) == 0 {
		return runtimecontract.IntentFrame{}, false
	}
	return runtimecontract.NormalizeIntentFrame(frame), true
}

func metadataDataScopes(raw string) []runtimecontract.DataScope {
	fields := splitRuntimeMetadataList(raw)
	out := make([]runtimecontract.DataScope, 0, len(fields))
	for _, field := range fields {
		out = append(out, runtimecontract.DataScope(field))
	}
	return out
}

func metadataActionRisks(raw string) []runtimecontract.ActionRisk {
	fields := splitRuntimeMetadataList(raw)
	out := make([]runtimecontract.ActionRisk, 0, len(fields))
	for _, field := range fields {
		out = append(out, runtimecontract.ActionRisk(field))
	}
	return out
}

func splitRuntimeMetadataList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func applyDepthProfileToCompileContext(ctx promptcompiler.CompileContext, profile taskdepth.Profile, reasoningEffort string) promptcompiler.CompileContext {
	ctx.TaskDepth = profile
	ctx.ReasoningEffort = strings.TrimSpace(reasoningEffort)
	return ctx
}

func applyTurnPromptProfileMetadata(ctx promptcompiler.CompileContext, metadata map[string]string) promptcompiler.CompileContext {
	if effort := firstMetadataValue(metadata, "reasoningEffort", "reasoning_effort"); effort != "" {
		ctx.ReasoningEffort = effort
	}
	if style := firstMetadataValue(metadata, "answerStyle", "answer_style"); style != "" {
		ctx.AnswerStyle = style
	}
	if metadataFlag(metadata, smalltalkOnlyMetadataKey) {
		ctx.SkillPromptAssets = append(ctx.SkillPromptAssets, smalltalkOnlyPromptAsset())
	}
	if metadataFlag(metadata, completeFollowupMetadataKey) {
		ctx.SkillPromptAssets = append(ctx.SkillPromptAssets, completeFollowupPromptAsset())
	}
	return ctx
}

func metadataFlag(metadata map[string]string, key string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata[key]), "true")
}

func completeFollowupPromptAsset() string {
	return strings.Join([]string{
		"## Complete follow-up answer",
		"The user is asking a follow-up that requests missing details or complete coverage. Answer as a complete standalone answer, not as a terse delta against the previous message.",
		"Cover every subtopic the user explicitly named. For programming-language or runtime questions, include the relevant internal representation, implementation details, thread-safety or concurrency guarantees, and the mechanism that causes blocking, waiting, panic, or data races where applicable.",
		"If the previous answer was incomplete, replace it with a complete answer instead of only saying what was missing.",
	}, "\n")
}

func smalltalkOnlyPromptAsset() string {
	return strings.Join([]string{
		"## Small-talk turn",
		"The current user input is conversational small talk, greeting, acknowledgement, or thanks.",
		"Do not call tools or inspect monitoring data. Do not continue a previous Coroot or RCA investigation unless the current user input explicitly asks for it.",
		"Reply briefly in the user's language. Do not proactively list Coroot, monitoring, terminal, or incident-analysis options.",
	}, "\n")
}

func applyRuntimeStateMetadata(ctx promptcompiler.CompileContext, metadata map[string]string, session *SessionState, snapshot *TurnSnapshot) promptcompiler.CompileContext {
	ctx.WebState = runtimeStateMentionState(metadata,
		"aiops.weblearn.enabled",
	)
	if metadataListContains(metadata["enableToolPack"], "public_web") {
		ctx.WebState = "requested"
	}
	ctx.OpsGraphState = runtimeStateMentionState(metadata,
		"aiops.opsGraph.explicitMention",
		"aiops.ops_graph.explicitMention",
	)
	ctx.CorootState = runtimeStateMentionState(metadata,
		"aiops.coroot.explicitMention",
		"aiops.coroot.explicitRCA",
		"aiops.tool.corootRCAAllowed",
	)
	ctx.OpsManusState = runtimeStateMentionState(metadata,
		"aiops.opsManuals.explicitMention",
	)
	if session != nil {
		ctx.PendingApprovals = len(session.PendingApprovals)
		ctx.PendingEvidence = len(session.PendingEvidence)
	}
	if constraints := runtimeStateUserConstraints(metadata); len(constraints) > 0 {
		ctx.UserConstraints = constraints
	}
	if snapshot != nil && snapshot.ResumeState != "" && snapshot.ResumeState != TurnResumeStateNone {
		ctx.TimeoutRecoveryState = string(snapshot.ResumeState)
	}
	return ctx
}

func runtimeStateMentionState(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if metadataBool(metadata[key]) {
			return "requested"
		}
	}
	return "not_requested"
}

func runtimeStateUserConstraints(metadata map[string]string) []string {
	raw := firstMetadataValue(metadata, "userConstraints", "user_constraints", "aiops.userConstraints")
	if raw == "" {
		return nil
	}
	values := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func shouldGuardPrematureFinal(profile taskdepth.Profile, snapshot *TurnSnapshot, iteration int, assistantContent string) bool {
	finalAttempt := strings.TrimSpace(assistantContent) != ""
	decision := EvaluatePlanRequirement(profile, snapshot, finalAttempt)
	if !decision.Required {
		return false
	}
	if snapshot == nil || iteration > 0 {
		return false
	}
	if strings.TrimSpace(snapshot.Metadata[prematureFinalGuardMetadataKey]) == "true" {
		return false
	}
	if !finalAttempt {
		return false
	}
	return true
}

func EvaluatePlanRequirement(profile taskdepth.Profile, snapshot *TurnSnapshot, finalAttempt bool) PlanRequirementDecision {
	if profile.AnalysisOnly {
		return PlanRequirementDecision{ReminderLevel: "none"}
	}
	if !profile.RequiresPlan && !taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiStep) {
		return PlanRequirementDecision{ReminderLevel: "none"}
	}
	missing := []string{}
	if !turnHasPlan(snapshot) {
		missing = append(missing, "plan")
	}
	if (profile.RequiresEvidence || taskdepth.AtLeast(profile.Level, taskdepth.LevelInvestigation)) && !turnHasEvidence(snapshot) {
		missing = append(missing, "evidence")
	}
	if profile.RequiresValidation || taskdepth.AtLeast(profile.Level, taskdepth.LevelOperations) {
		if snapshot == nil || strings.TrimSpace(snapshot.Metadata["validation.completed"]) != "true" {
			missing = append(missing, "validation")
		}
	}
	if taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiAgent) {
		if snapshot == nil || strings.TrimSpace(snapshot.Metadata["task.claimed"]) != "true" {
			missing = append(missing, "task_claim")
		}
	}
	if len(missing) == 0 {
		return PlanRequirementDecision{ReminderLevel: "none"}
	}
	level := "soft"
	if finalAttempt {
		level = "hard"
	}
	return PlanRequirementDecision{
		Required:      true,
		Reason:        "task_depth_requires_plan",
		ReminderLevel: level,
		Missing:       missing,
	}
}

func planRequirementDecisionTrace(decision PlanRequirementDecision) *promptinput.PlanRequirementDecisionTrace {
	if !decision.Required && decision.ReminderLevel == "" {
		return nil
	}
	trace := &promptinput.PlanRequirementDecisionTrace{
		Required: decision.Required,
		Decision: decision.ReminderLevel,
		Reason:   decision.Reason,
		Signals:  append([]string(nil), decision.Missing...),
	}
	if trace.Decision == "" {
		trace.Decision = "none"
	}
	return trace
}

func prematureFinalGuardPrompt(profile taskdepth.Profile) string {
	return "## Premature final answer guard\nThis request is classified as " + string(profile.Level) + ". You produced a final answer without a plan or direct evidence. Continue the task: create or update a plan if available, gather the minimum direct evidence, or ask the smallest missing question. Do not finalize yet."
}

func markPrematureFinalGuard(snapshot *TurnSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]string{}
	}
	snapshot.Metadata[prematureFinalGuardMetadataKey] = "true"
}

func turnHasPlan(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, item := range snapshot.AgentItems {
		if string(item.Type) == "plan" {
			return true
		}
	}
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			if result.Display != nil && result.Display.Type == "plan" {
				return true
			}
		}
	}
	return false
}

func turnHasEvidence(snapshot *TurnSnapshot) bool {
	if snapshotHasUserProvidedEvidence(snapshot) {
		return true
	}
	return countActualToolDispatches(snapshot) > 0
}

func missingEvidenceFinalBlocker(profile taskdepth.Profile, snapshot *TurnSnapshot, assistantContent string) (string, bool) {
	if strings.TrimSpace(assistantContent) == "" || finalLooksLikeBlocker(assistantContent) {
		return "", false
	}
	if profile.AnalysisOnly {
		return "", false
	}
	if !profile.RequiresEvidence && !taskdepth.AtLeast(profile.Level, taskdepth.LevelInvestigation) {
		return "", false
	}
	if turnHasEvidence(snapshot) {
		return "", false
	}
	if selectedHostInventoryAnswer(snapshot, assistantContent) {
		return "", false
	}
	if snapshot == nil || strings.TrimSpace(snapshot.Metadata[prematureFinalGuardMetadataKey]) != "true" {
		return "", false
	}
	return strings.Join([]string{
		"缺少直接工具证据，不能给出已检查或已完成结论。当前这轮没有成功执行任何可验证的工具调用。",
		"",
		"建议：",
		"- 确认目标主机、服务、时间窗口或图谱节点是否已选中。",
		"- 选择可用工具进行只读证据采集，例如主机信息、OpsGraph 关系、观测指标或日志查询。",
		"- 如果需要检查远程主机，请先确认主机接入、SSH 连通性和权限，然后重试。",
		"- 如果暂时没有工具权限，我只能给出下一步排查建议，不能声称已完成检查。",
	}, "\n"), true
}

func selectedHostInventoryAnswer(snapshot *TurnSnapshot, assistantContent string) bool {
	if snapshot == nil || !metadataBool(snapshot.Metadata["aiops.host.metadataAvailable"]) {
		return false
	}
	content := strings.ToLower(strings.TrimSpace(assistantContent))
	if content == "" || containsOperationalConclusion(content) {
		return false
	}
	matches := 0
	for _, key := range []string{
		"aiops.host.id",
		"aiops.host.label",
		"aiops.host.address",
		"aiops.host.sshUser",
		"aiops.host.sshPort",
	} {
		value := strings.ToLower(strings.TrimSpace(snapshot.Metadata[key]))
		if value != "" && strings.Contains(content, value) {
			matches++
		}
	}
	if matches < 2 {
		return false
	}
	fieldLabels := 0
	for _, marker := range []string{"主机", "地址", "ssh", "端口", "host", "address", "user", "port"} {
		if strings.Contains(content, marker) {
			fieldLabels++
		}
	}
	return fieldLabels >= 2
}

func containsOperationalConclusion(content string) bool {
	for _, marker := range []string{
		"已检查",
		"检查完成",
		"已验证",
		"验证完成",
		"已执行",
		"排查完成",
		"修复完成",
		"运行正常",
		"状态正常",
		"未发现异常",
		"故障",
		"异常",
		"根因",
		"指标",
		"日志",
	} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func finalLooksLikeBlocker(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if safeTerminal := EvaluateSafeTerminalFinal(text); len(safeTerminal.TerminalStates) > 0 {
		return safeTerminal.Valid
	}
	for _, marker := range []string{"缺少", "需要你", "无法继续", "权限", "blocked", "approval", "请提供", "未执行"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
