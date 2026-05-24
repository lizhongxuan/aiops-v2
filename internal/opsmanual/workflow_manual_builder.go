package opsmanual

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const workflowManualGenerationVersion = "p0-2026-05-20"

func BuildWorkflowManualCandidate(analysis WorkflowManualAnalysis) (ManualCandidate, error) {
	workflowID := strings.TrimSpace(analysis.WorkflowID)
	if workflowID == "" {
		return ManualCandidate{}, fmt.Errorf("workflow_id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := workflowManualTitle(analysis)
	manual := OpsManual{
		ID:             "manual-candidate-" + slug(workflowID),
		ManualFamilyID: firstNonEmpty(metadataString(analysis.XOpsManual, "manual_family_id"), slug(workflowID)),
		Title:          title,
		Status:         ManualStatusDraft,
		Version:        strings.TrimSpace(analysis.WorkflowVersion),
		Owner:          metadataString(analysis.XOpsManual, "owner"),
		WorkflowRef: WorkflowRef{
			WorkflowID:      workflowID,
			WorkflowVersion: strings.TrimSpace(analysis.WorkflowVersion),
			WorkflowDigest:  strings.TrimSpace(analysis.WorkflowDigest),
			StorageURI:      strings.TrimSpace(analysis.StorageURI),
		},
		Operation:       analysis.Operation,
		Applicability:   analysis.Applicability,
		RequiredContext: analysis.RequiredContext,
		ParameterRules:  cloneParameterRules(analysis.ParameterRules),
		Metadata:        workflowManualMetadata(analysis),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	manual.RequiredContext.RequiredInputs = nonEmptySortedUnique(manual.RequiredContext.RequiredInputs)
	manual.RequiredContext.RequiredEvidence = nonEmptySortedUnique(manual.RequiredContext.RequiredEvidence)
	manual.Preconditions = workflowManualPreconditions(analysis)
	manual.Validation = workflowManualValidation(analysis)
	manual.CannotUseWhen = workflowManualCannotUseWhen(analysis)
	manual.RiskNotes = workflowManualRiskNotes(analysis)
	manual.RetrievalProfile = workflowManualRetrievalProfile(analysis, manual)
	manual.Tags = workflowManualTags(manual.RetrievalProfile.Keywords)
	manual.RunnableConditions = workflowManualRunnableConditions(analysis, manual)
	manual.PreflightProbe = workflowManualPreflightProbe(analysis)
	manual.RiskPolicy = workflowManualRiskPolicy(analysis)
	manual.FallbackGuide = workflowManualFallbackGuide(analysis)
	manual.Verification = workflowManualVerification(analysis, manual)
	manual.SearchDoc = workflowManualSearchDoc(analysis, manual)
	manual.DocumentMarkdown = workflowManualMarkdown(manual, analysis)

	candidate := ManualCandidate{
		ID:             "candidate-" + slug(workflowID),
		SourceType:     "workflow_reverse_generated",
		SourceRefs:     []string{workflowID},
		ProposedManual: manual,
		ReviewStatus:   "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	candidate.StructuredValidationReport = ValidateManualCandidate(candidate, ManualCandidateValidationOptions{RecentRuns: analysis.RecentRuns})
	candidate.ValidationReport = manualCandidateValidationMessages(candidate.StructuredValidationReport)
	candidate.UserSummary = BuildManualGenerationUserSummary(candidate, candidate.StructuredValidationReport)
	if candidate.StructuredValidationReport.Status == "blocked" {
		candidate.ReviewStatus = "needs_fix"
	}
	return candidate, nil
}

func workflowManualTitle(analysis WorkflowManualAnalysis) string {
	return firstNonEmpty(
		metadataString(analysis.XOpsManual, "title"),
		analysis.Description,
		analysis.Name,
		analysis.WorkflowID,
	)
}

func workflowManualMetadata(analysis WorkflowManualAnalysis) map[string]any {
	meta := cloneMap(analysis.XOpsManual)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["source_type"] = "workflow_reverse_generated"
	meta["workflow_manual_generation_version"] = workflowManualGenerationVersion
	meta["workflow_id"] = strings.TrimSpace(analysis.WorkflowID)
	meta["workflow_name"] = strings.TrimSpace(analysis.Name)
	return meta
}

func workflowManualPreconditions(analysis WorkflowManualAnalysis) []string {
	out := []string{}
	for _, input := range analysis.RequiredContext.RequiredInputs {
		out = appendUnique(out, "确认参数 "+input+" 已由用户或系统可靠提供。")
	}
	if len(out) == 0 {
		out = appendUnique(out, "确认目标对象、执行窗口和影响范围已经人工核对。")
	}
	for _, stage := range workflowStageNames(analysis, "precheck") {
		out = appendUnique(out, "先完成前置检查："+stage+"。")
	}
	return out
}

func workflowManualValidation(analysis WorkflowManualAnalysis) []string {
	out := []string{}
	for _, hint := range analysis.ValidationHints {
		out = appendUnique(out, hint)
	}
	for _, stage := range workflowStageNames(analysis, "validate") {
		out = appendUnique(out, "验证阶段："+stage)
	}
	if len(out) == 0 {
		out = appendUnique(out, "执行后必须通过只读探测或业务指标确认结果。")
	}
	return out
}

func workflowManualCannotUseWhen(analysis WorkflowManualAnalysis) []string {
	out := cloneStrings(analysis.CannotUseHints)
	out = appendUnique(out, "Workflow 版本或 digest 与当前 Runner 定义不一致时不要使用。")
	out = appendUnique(out, "目标实例、影响范围、审批状态或回滚方案无法确认时不要使用。")
	if riskLevelRank(analysis.Operation.RiskLevel) >= riskLevelRank("high") {
		out = appendUnique(out, "高风险操作未完成审批、预检计划检查或变更窗口确认时不要使用。")
	}
	return out
}

func workflowManualRiskNotes(analysis WorkflowManualAnalysis) []string {
	out := []string{}
	if analysis.Operation.RiskLevel != "" {
		out = appendUnique(out, "风险级别："+analysis.Operation.RiskLevel)
	}
	for _, note := range metadataStringSlice(analysis.XOpsManual, "risk_notes") {
		out = appendUnique(out, note)
	}
	for _, risk := range analysis.ActionRisks {
		if risk.DataMutation {
			out = appendUnique(out, "包含数据变更步骤："+risk.StepName)
		}
		if risk.ServiceRestart {
			out = appendUnique(out, "包含服务启停步骤："+risk.StepName)
		}
		if risk.RequiresApproval {
			out = appendUnique(out, "高风险步骤需要审批："+risk.StepName)
		}
	}
	return out
}

func workflowManualRetrievalProfile(analysis WorkflowManualAnalysis, manual OpsManual) RetrievalProfile {
	keywords := []string{}
	addKeyword := func(values ...string) {
		for _, value := range values {
			for _, token := range keywordTokens(value) {
				keywords = appendUnique(keywords, token)
			}
		}
	}
	addKeyword(manual.Title, analysis.Name, analysis.Description)
	addKeyword(manual.Operation.TargetType, manual.Operation.Action, manual.Applicability.Middleware)
	for _, item := range manual.Applicability.Platform {
		addKeyword(item)
	}
	for _, item := range manual.Applicability.ExecutionSurface {
		addKeyword(item)
	}
	for _, item := range analysis.ValidationHints {
		addKeyword(item)
	}
	for _, step := range analysis.Steps {
		addKeyword(step.Name, step.Action)
	}
	negative := negativeKeywordsForTarget(manual.Operation.TargetType, manual.Applicability.Platform)
	aliases := map[string][]string{}
	if manual.Operation.TargetType != "" {
		aliases["target_type"] = []string{manual.Operation.TargetType}
	}
	if manual.Operation.Action != "" {
		aliases["action"] = []string{manual.Operation.Action}
	}
	return RetrievalProfile{
		Aliases:          aliases,
		Keywords:         nonEmptySortedUnique(keywords),
		NegativeKeywords: nonEmptySortedUnique(negative),
		EmbeddingText:    strings.TrimSpace(workflowManualSearchDoc(analysis, manual)),
		MinScore: ScoreThresholds{
			Candidate:     0.55,
			DirectExecute: 0.78,
		},
	}
}

func keywordTokens(value string) []string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return nil
	}
	replacer := strings.NewReplacer("\n", " ", "\t", " ", ",", " ", "，", " ", "。", " ", ":", " ", "：", " ", ";", " ", "；", " ", "(", " ", ")", " ")
	fields := strings.Fields(replacer.Replace(lower))
	out := []string{}
	for _, field := range fields {
		field = strings.Trim(field, "`\"'[]{}")
		if field == "" || len([]rune(field)) < 2 {
			continue
		}
		out = appendUnique(out, field)
	}
	return out
}

func negativeKeywordsForTarget(target string, platform []string) []string {
	registry := DefaultOpsManualCapabilityRegistry()
	all := registry.WorkflowTargetTypes()
	text := strings.ToLower(target + " " + strings.Join(platform, " "))
	out := []string{}
	for _, item := range all {
		if strings.Contains(text, item) {
			continue
		}
		matchedAlias := false
		for _, alias := range registry.ObjectAliasesFor(item) {
			if strings.TrimSpace(alias) != "" && strings.Contains(text, strings.ToLower(alias)) {
				matchedAlias = true
				break
			}
		}
		if matchedAlias {
			continue
		}
		out = appendUnique(out, item)
	}
	return out
}

func workflowManualTags(keywords []string) []string {
	out := []string{}
	for _, keyword := range keywords {
		if len(out) >= 8 {
			break
		}
		out = appendUnique(out, keyword)
	}
	return out
}

func workflowManualRunnableConditions(analysis WorkflowManualAnalysis, manual OpsManual) RunnableConditions {
	return RunnableConditions{
		RequiredParams:   cloneStrings(manual.RequiredContext.RequiredInputs),
		MaxRiskLevel:     firstNonEmpty(manual.Operation.RiskLevel, "medium"),
		RequiresApproval: workflowRequiresApproval(analysis),
	}
}

func workflowRequiresApproval(analysis WorkflowManualAnalysis) bool {
	if riskLevelRank(analysis.Operation.RiskLevel) >= riskLevelRank("high") {
		return true
	}
	for _, stage := range analysis.GraphStages {
		if stage.Stage == "approval" {
			return true
		}
	}
	for _, step := range analysis.Steps {
		if step.Stage == "approval" || strings.EqualFold(step.Action, "manual.approval") {
			return true
		}
	}
	return false
}

func workflowManualPreflightProbe(analysis WorkflowManualAnalysis) PreflightProbe {
	for _, step := range analysis.Steps {
		if step.Stage == "precheck" || step.ReadOnly {
			return PreflightProbe{
				ID:              slug(firstNonEmpty(step.Name, step.Action, "workflow-preflight")),
				Type:            "workflow_step",
				Action:          step.Action,
				ReadOnly:        step.ReadOnly || step.Stage == "precheck",
				TimeoutSeconds:  60,
				RequiredOutputs: cloneStrings(step.ExpectVars),
			}
		}
	}
	return PreflightProbe{}
}

func workflowManualRiskPolicy(analysis WorkflowManualAnalysis) RiskPolicy {
	policy := RiskPolicy{
		BlastRadius: "按 Workflow targets 和用户确认的目标范围执行。",
	}
	for _, risk := range analysis.ActionRisks {
		if risk.DataMutation {
			policy.DataMutation = true
		}
		if risk.ServiceRestart {
			policy.ServiceRestart = "required"
		}
		if risk.RequiresApproval {
			policy.ApprovalRequiredWhen = appendUnique(policy.ApprovalRequiredWhen, "执行 "+risk.StepName+" 前")
		}
	}
	if workflowRequiresApproval(analysis) {
		policy.ApprovalRequiredWhen = appendUnique(policy.ApprovalRequiredWhen, "存在 manual approval stage 或高风险操作时")
	}
	return policy
}

func workflowManualFallbackGuide(analysis WorkflowManualAnalysis) FallbackGuide {
	steps := workflowStageNames(analysis, "rollback")
	if len(steps) > 0 {
		return FallbackGuide{Mode: "workflow_rollback", Steps: steps}
	}
	return FallbackGuide{
		Mode: "operator_review_required",
		Steps: []string{
			"Workflow 未声明 rollback stage，发布前需要补充回滚负责人、回滚触发条件和恢复验证方式。",
		},
	}
}

func workflowManualVerification(analysis WorkflowManualAnalysis, manual OpsManual) VerificationProfile {
	needsPreflightPlan := riskLevelRank(manual.Operation.RiskLevel) >= riskLevelRank("high")
	if !needsPreflightPlan {
		for _, risk := range analysis.ActionRisks {
			if risk.DataMutation || risk.ServiceRestart {
				needsPreflightPlan = true
				break
			}
		}
	}
	for _, stage := range analysis.GraphStages {
		if stage.Stage == "dry_run" {
			needsPreflightPlan = true
			break
		}
	}
	for _, step := range analysis.Steps {
		if step.Stage == "dry_run" {
			needsPreflightPlan = true
			break
		}
	}
	return VerificationProfile{RequiredPreflightPlan: needsPreflightPlan}
}

func workflowManualSearchDoc(analysis WorkflowManualAnalysis, manual OpsManual) string {
	parts := []string{
		manual.Title,
		analysis.Description,
		manual.Operation.TargetType,
		manual.Operation.Action,
		manual.Applicability.Middleware,
		strings.Join(manual.RequiredContext.RequiredInputs, " "),
		strings.Join(manual.Validation, " "),
		strings.Join(manual.RetrievalProfile.Keywords, " "),
	}
	return strings.TrimSpace(strings.Join(nonEmptySortedUnique(parts), " "))
}

func workflowManualMarkdown(manual OpsManual, analysis WorkflowManualAnalysis) string {
	var b strings.Builder
	b.WriteString("# " + manual.Title + "\n\n")
	b.WriteString("## 适用范围\n")
	b.WriteString(markdownList([]string{
		"对象类型：" + fallbackText(manual.Operation.TargetType, "待补充"),
		"操作类型：" + fallbackText(manual.Operation.Action, "待补充"),
		"中间件：" + fallbackText(firstNonEmpty(manual.Applicability.Middleware, strings.Join(manual.Applicability.Platform, ", ")), "待补充"),
	}))
	b.WriteString("\n## 所需上下文\n")
	b.WriteString(markdownList(prefixList("参数：", manual.RequiredContext.RequiredInputs, "暂无必填参数，仍需确认目标对象和执行窗口。")))
	b.WriteString("\n## 前置检查\n")
	b.WriteString(markdownList(manual.Preconditions))
	b.WriteString("\n## 执行步骤\n")
	b.WriteString(markdownList(workflowManualStepDescriptions(analysis)))
	b.WriteString("\n## 验证方式\n")
	b.WriteString(markdownList(manual.Validation))
	b.WriteString("\n## 风险与审批\n")
	b.WriteString(markdownList(append([]string{
		"风险级别：" + fallbackText(manual.Operation.RiskLevel, "待补充"),
		"需要审批：" + yesNo(manual.RunnableConditions.RequiresApproval),
	}, manual.RiskNotes...)))
	b.WriteString("\n## 不能使用\n")
	b.WriteString(markdownList(manual.CannotUseWhen))
	b.WriteString("\n## 降级处理\n")
	b.WriteString(markdownList(manual.FallbackGuide.Steps))
	return b.String()
}

func workflowManualStepDescriptions(analysis WorkflowManualAnalysis) []string {
	out := []string{}
	for _, step := range analysis.Steps {
		name := firstNonEmpty(step.Name, step.Action, "workflow step")
		stage := fallbackText(step.Stage, "execute")
		out = appendUnique(out, fmt.Sprintf("%s：%s（%s）", stage, name, fallbackText(step.Action, "workflow action")))
	}
	if len(out) == 0 {
		out = appendUnique(out, "按 Runner Workflow 中已审核的步骤执行。")
	}
	return out
}

func workflowStageNames(analysis WorkflowManualAnalysis, stage string) []string {
	out := []string{}
	for _, step := range analysis.Steps {
		if step.Stage == stage {
			out = appendUnique(out, firstNonEmpty(step.Name, step.Action, stage))
		}
	}
	for _, graphStage := range analysis.GraphStages {
		if graphStage.Stage == stage {
			out = appendUnique(out, firstNonEmpty(graphStage.Label, graphStage.StepName, graphStage.ID, stage))
		}
	}
	return out
}

func markdownList(values []string) string {
	values = nonEmptyStrings(values)
	if len(values) == 0 {
		return "- 待补充。\n"
	}
	var b strings.Builder
	for _, value := range values {
		b.WriteString("- " + value + "\n")
	}
	return b.String()
}

func prefixList(prefix string, values []string, fallback string) []string {
	if len(nonEmptyStrings(values)) == 0 {
		return []string{fallback}
	}
	out := []string{}
	for _, value := range values {
		out = appendUnique(out, prefix+value)
	}
	return out
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func nonEmptySortedUnique(values []string) []string {
	out := nonEmptyStrings(values)
	sort.Strings(out)
	deduped := []string{}
	for _, value := range out {
		deduped = appendUnique(deduped, value)
	}
	return deduped
}

func manualCandidateValidationMessages(report ManualCandidateValidation) []string {
	out := []string{}
	for _, issue := range report.Blocking {
		out = appendUnique(out, issue.Message)
	}
	for _, issue := range report.Warnings {
		out = appendUnique(out, issue.Message)
	}
	return out
}
