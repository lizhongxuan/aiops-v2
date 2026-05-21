package opsmanual

import (
	"fmt"
	"strings"
)

type ManualCandidateValidationOptions struct {
	RecentRuns       []RunRecord
	ActionSpecs      []ActionSpecSummary
	SearchSelfCheck  bool
	SearchFrame      OperationFrame
	SearchQueryText  string
	AllowNeedInfoHit bool
}

func ValidateManualCandidate(candidate ManualCandidate, opts ManualCandidateValidationOptions) ManualCandidateValidation {
	manual := candidate.ProposedManual
	report := ManualCandidateValidation{}
	addBlocking := func(code, field, message string) {
		report.Blocking = append(report.Blocking, ValidationIssue{Code: code, Field: field, Message: message})
	}
	addWarning := func(code, field, message string) {
		report.Warnings = append(report.Warnings, ValidationIssue{Code: code, Field: field, Message: message})
	}
	addPassed := func(code, field, message string) {
		report.Passed = append(report.Passed, ValidationIssue{Code: code, Field: field, Message: message})
	}

	if strings.TrimSpace(manual.WorkflowRef.WorkflowID) == "" {
		addBlocking("missing_workflow_id", "workflow_ref.workflow_id", "缺少 Workflow 绑定。")
	} else {
		addPassed("workflow_ref_present", "workflow_ref.workflow_id", "已绑定 Workflow。")
	}
	if strings.TrimSpace(manual.WorkflowRef.WorkflowDigest) == "" {
		addBlocking("missing_workflow_digest", "workflow_ref.workflow_digest", "缺少 Workflow digest，无法确认手册对应的真实定义。")
	} else {
		addPassed("workflow_digest_present", "workflow_ref.workflow_digest", "已记录 Workflow digest。")
	}
	if strings.TrimSpace(manual.Operation.TargetType) == "" {
		addBlocking("missing_target_type", "operation.target_type", "缺少运维对象类型。")
	}
	if strings.TrimSpace(manual.Operation.Action) == "" {
		addBlocking("missing_operation_action", "operation.action", "缺少操作类型。")
	}
	if len(nonEmptyStrings(manual.Validation)) == 0 {
		addBlocking("missing_validation", "validation", "缺少执行后的验证方式。")
	}
	if len(nonEmptyStrings(manual.CannotUseWhen)) == 0 {
		addBlocking("missing_cannot_use_when", "cannot_use_when", "缺少不能使用边界。")
	}
	if strings.TrimSpace(manual.DocumentMarkdown) == "" {
		addBlocking("missing_document_markdown", "document_markdown", "缺少用户可读手册正文。")
	}
	for name, rule := range manual.ParameterRules {
		if isSensitiveParameterKey(name) && !isEmptyValue(rule.DefaultValue) {
			addBlocking("sensitive_default_value", "parameter_rules."+name+".default_value", "敏感参数不能带明文默认值。")
		}
	}

	if strings.TrimSpace(manual.PreflightProbe.Action) == "" && strings.TrimSpace(manual.PreflightProbe.ID) == "" {
		addWarning("missing_preflight_probe", "preflight_probe", "缺少明确的只读前置检查。")
	}
	if !hasRecentSuccessfulRun(manual.WorkflowRef.WorkflowID, opts.RecentRuns) {
		addWarning("missing_recent_successful_run", "run_records", "缺少近期成功闭环记录，发布前建议先完成预检计划检查或一次真实成功执行。")
	}
	if len(nonEmptyStrings(manual.RetrievalProfile.NegativeKeywords)) == 0 {
		addWarning("missing_negative_keywords", "retrieval_profile.negative_keywords", "缺少负向关键词，检索时可能误匹配其他对象。")
	}
	if riskLevelRank(manual.Operation.RiskLevel) >= riskLevelRank("high") &&
		len(nonEmptyStrings(manual.RiskPolicy.ApprovalRequiredWhen)) == 0 &&
		!manual.RunnableConditions.RequiresApproval {
		addWarning("missing_high_risk_approval_policy", "risk_policy.approval_required_when", "高风险操作缺少审批策略。")
	}
	if strings.TrimSpace(manual.FallbackGuide.Mode) == "operator_review_required" {
		addWarning("template_fallback_requires_review", "fallback_guide", "回滚方案来自安全模板，发布前需要人工补齐。")
	}
	for _, spec := range opts.ActionSpecs {
		if spec.Deprecated && manualUsesAction(manual, spec.Action) {
			addWarning("deprecated_action", "workflow_action."+spec.Action, "Workflow 使用了已废弃 Action："+spec.Action)
		}
	}
	if opts.SearchSelfCheck {
		if issue, ok := validateManualCandidateSearchSelfCheck(manual, opts); ok {
			switch issue.Code {
			case "search_self_check_passed":
				report.Passed = append(report.Passed, issue)
			default:
				report.Warnings = append(report.Warnings, issue)
			}
		}
	}

	report.Status = manualCandidateValidationStatus(report)
	return report
}

func BuildManualGenerationUserSummary(candidate ManualCandidate, report ManualCandidateValidation) ManualGenerationUserSummary {
	manual := candidate.ProposedManual
	summary := ManualGenerationUserSummary{}
	summary.Understood = append(summary.Understood,
		fmt.Sprintf("已从 Workflow %s 生成运维手册草稿。", fallbackText(manual.WorkflowRef.WorkflowID, "待确认")),
		fmt.Sprintf("识别对象为 %s，操作为 %s。", fallbackText(manual.Operation.TargetType, "待补充"), fallbackText(manual.Operation.Action, "待补充")),
		fmt.Sprintf("风险级别为 %s，审批要求为 %s。", fallbackText(manual.Operation.RiskLevel, "待补充"), yesNo(manual.RunnableConditions.RequiresApproval)),
	)
	if manual.Verification.RequiredPreflightPlan {
		summary.Understood = append(summary.Understood, "该候选要求发布或执行前完成预检计划检查。")
	} else if manual.Verification.RequiredRunnerDryRun {
		summary.Understood = append(summary.Understood, "该候选包含历史 Runner Dry Run 要求，请按发布前检查语义审核。")
	}
	if strings.TrimSpace(manual.FallbackGuide.Mode) != "" {
		summary.Understood = append(summary.Understood, "已识别降级或 rollback 处理要求。")
	}
	summary.Understood = limitStrings(summary.Understood, 6)
	for _, issue := range append(cloneValidationIssues(report.Blocking), report.Warnings...) {
		summary.Missing = appendUnique(summary.Missing, issue.Message)
	}
	summary.Missing = limitStrings(summary.Missing, 8)
	if report.Status == "blocked" {
		summary.NextSteps = append(summary.NextSteps, "先补齐阻断项，再保存或发布为正式运维手册。")
	} else {
		summary.NextSteps = append(summary.NextSteps, "审核适用范围、审批策略、验证方式和回滚方案。")
	}
	if validationHasCodeLocal(report.Warnings, "missing_recent_successful_run") {
		summary.NextSteps = append(summary.NextSteps, "完成预检计划检查或一次成功闭环后再发布。")
	}
	if validationHasCodeLocal(report.Warnings, "template_fallback_requires_review") {
		summary.NextSteps = append(summary.NextSteps, "把模板回滚说明替换为真实回滚步骤。")
	}
	summary.NextSteps = limitStrings(summary.NextSteps, 6)
	return summary
}

func manualCandidateValidationStatus(report ManualCandidateValidation) string {
	switch {
	case len(report.Blocking) > 0:
		return "blocked"
	case len(report.Warnings) > 0:
		return "warning"
	default:
		return "passed"
	}
}

func hasRecentSuccessfulRun(workflowID string, records []RunRecord) bool {
	workflowID = strings.TrimSpace(workflowID)
	for _, record := range records {
		if workflowID != "" && strings.TrimSpace(record.WorkflowID) != workflowID {
			continue
		}
		if runRecordPassed(record) {
			return true
		}
	}
	return false
}

func manualUsesAction(manual OpsManual, action string) bool {
	action = strings.TrimSpace(action)
	if action == "" {
		return false
	}
	if strings.EqualFold(manual.PreflightProbe.Action, action) {
		return true
	}
	text := strings.ToLower(strings.Join([]string{manual.SearchDoc, manual.DocumentMarkdown}, " "))
	return strings.Contains(text, strings.ToLower(action))
}

func validateManualCandidateSearchSelfCheck(manual OpsManual, opts ManualCandidateValidationOptions) (ValidationIssue, bool) {
	if strings.TrimSpace(manual.ID) == "" {
		return ValidationIssue{Code: "search_self_check_failed", Field: "manual.id", Message: "检索自检失败：候选手册缺少 ID。"}, true
	}
	store := NewMemoryStore()
	verified := cloneManual(manual)
	verified.Status = ManualStatusVerified
	if err := store.SaveManual(verified); err != nil {
		return ValidationIssue{Code: "search_self_check_failed", Field: "search", Message: "检索自检失败：" + err.Error()}, true
	}
	frame := opts.SearchFrame
	if frame.Target.Type == "" {
		frame.Target.Type = manual.Operation.TargetType
	}
	if frame.Operation.TargetType == "" {
		frame.Operation.TargetType = manual.Operation.TargetType
	}
	if frame.Operation.Action == "" {
		frame.Operation.Action = manual.Operation.Action
	}
	if frame.Risk.Level == "" {
		frame.Risk.Level = manual.Operation.RiskLevel
	}
	result, err := SearchOpsManuals(store, SearchOpsManualsRequest{
		Text:           opts.SearchQueryText,
		OperationFrame: frame,
		Limit:          5,
	})
	if err != nil {
		return ValidationIssue{Code: "search_self_check_failed", Field: "search", Message: "检索自检失败：" + err.Error()}, true
	}
	if len(result.Manuals) == 0 {
		return ValidationIssue{Code: "search_self_check_failed", Field: "search", Message: "检索自检未命中生成的手册。"}, true
	}
	top := result.Manuals[0]
	if top.Manual.ID == manual.ID || sameObjectOperation(manual, top.Manual) {
		return ValidationIssue{Code: "search_self_check_passed", Field: "search", Message: "生成手册能被检索链路命中。"}, true
	}
	return ValidationIssue{Code: "search_self_check_failed", Field: "search", Message: "检索自检命中了其他手册，需调整检索字段。"}, true
}

func sameObjectOperation(left, right OpsManual) bool {
	return strings.EqualFold(strings.TrimSpace(left.Operation.TargetType), strings.TrimSpace(right.Operation.TargetType)) &&
		strings.EqualFold(strings.TrimSpace(left.Operation.Action), strings.TrimSpace(right.Operation.Action))
}

func limitStrings(values []string, max int) []string {
	values = nonEmptyStrings(values)
	if max > 0 && len(values) > max {
		return values[:max]
	}
	return values
}

func validationHasCodeLocal(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
