package opsmanual

import (
	"fmt"
	"sort"
	"strings"
)

type ManualRepository interface {
	ListManuals(ListManualsRequest) ([]OpsManual, error)
	GetManual(id string) (OpsManual, error)
	SaveManual(OpsManual) error
	ListRunRecords(ListRunRecordsRequest) ([]RunRecord, error)
}

type ListManualsRequest struct {
	Status           ManualStatus
	TargetType       string
	Action           string
	Middleware       string
	ExecutionSurface string
	Limit            int
}

type ListRunRecordsRequest struct {
	ManualID   string
	WorkflowID string
	Limit      int
}

func SearchOpsManuals(repo ManualRepository, req SearchOpsManualsRequest) (SearchOpsManualsResult, error) {
	if repo == nil {
		return SearchOpsManualsResult{}, fmt.Errorf("manual repository is nil")
	}
	frame := normalizeSearchOperationFrame(req)
	result := SearchOpsManualsResult{
		OperationFrame: frame,
		SearchedFields: searchedFrameFields(frame),
	}
	manuals, err := repo.ListManuals(ListManualsRequest{Status: ManualStatusVerified})
	if err != nil {
		return SearchOpsManualsResult{}, err
	}
	hits := make([]SearchManualHit, 0, len(manuals))
	for _, manual := range manuals {
		if manual.Status != ManualStatusVerified {
			continue
		}
		hit := evaluateSearchManual(repo, manual, frame)
		if hit.UsableMode == DecisionNoMatch {
			continue
		}
		hits = append(hits, hit)
	}
	sortSearchHits(hits)
	if req.Limit > 0 && len(hits) > req.Limit {
		hits = hits[:req.Limit]
	}
	result.Manuals = hits
	if len(hits) == 0 {
		result.Decision = noHitDecision(frame)
		result.Summary = searchSummary(result.Decision, nil)
		result.NextQuestions = nextQuestionsForMissing(missingFrameForSearch(frame))
		result.RecommendedNextAction = recommendedNextAction(result.Decision)
		return result, nil
	}
	result.Decision = hits[0].UsableMode
	result.Summary = searchSummary(result.Decision, &hits[0])
	if result.Decision == DecisionNeedInfo {
		result.NextQuestions = nextQuestionsForMissing(hits[0].MissingFields)
	}
	result.RecommendedNextAction = recommendedNextAction(result.Decision)
	return result, nil
}

func RetrieveManuals(repo ManualRepository, frame OperationFrame) ([]ManualMatch, error) {
	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{OperationFrame: frame})
	if err != nil {
		return nil, err
	}
	matches := make([]ManualMatch, 0, len(result.Manuals))
	for _, hit := range result.Manuals {
		matches = append(matches, manualMatchFromSearchHit(hit, result.Summary))
	}
	return matches, nil
}

func evaluateManual(repo ManualRepository, manual OpsManual, frame OperationFrame) ManualMatch {
	match := ManualMatch{Manual: cloneManual(manual)}
	if manual.Operation.Action != "" && !equalFold(manual.Operation.Action, frame.Operation.Action) {
		match.State = DecisionReference
		match.Reasons = append(match.Reasons, "operation action differs")
		match.RunRecordSummary = summarizeRuns(repo, manual)
		return match
	}
	missing := append([]string{}, frame.Evidence.Missing...)
	gaps := []string{}
	if !listMatches(manual.Applicability.ExecutionSurface, frame.Environment.ExecutionSurface) {
		gaps = appendUnique(gaps, "execution_surface")
	}
	if !listMatches(manual.Applicability.OS, frame.Environment.OS) {
		gaps = appendUnique(gaps, "os")
	}
	if !listMatches(manual.Applicability.Platform, frame.Environment.Platform) {
		gaps = appendUnique(gaps, "platform")
	}
	for _, evidence := range manual.RequiredContext.RequiredEvidence {
		if !hasAny(frame.Evidence.Provided, evidence) {
			missing = appendUnique(missing, evidence)
		}
	}
	for _, input := range manual.RequiredContext.RequiredInputs {
		if input == "target_instance" && frame.Target.Name == "" {
			missing = appendUnique(missing, input)
		}
		if input == "backup_path" && !strings.Contains(strings.ToLower(frame.RawText), "/") && metadataString(frame.Metadata, "backup_path") == "" {
			missing = appendUnique(missing, input)
		}
	}
	for _, rule := range manual.CannotUseWhen {
		lower := strings.ToLower(rule)
		switch {
		case strings.Contains(rule, "目标实例未知") && hasAny(frame.Evidence.Missing, "target_instance"):
			missing = appendUnique(missing, "target_instance")
			match.Reasons = append(match.Reasons, "manual cannot be used while target instance is unknown")
		case strings.Contains(rule, "Kubernetes") && frame.Environment.Platform != "kubernetes":
			match.Reasons = append(match.Reasons, "manual mentions Kubernetes applicability constraint")
		case strings.Contains(rule, "无法确认数据库版本") || strings.Contains(lower, "database version"):
			if !hasAny(frame.Evidence.Provided, "version", "pg_version") {
				missing = appendUnique(missing, "version")
			}
		}
	}
	match.MissingContext = dedupe(missing)
	match.CompatibilityGaps = dedupe(gaps)
	match.RunRecordSummary = summarizeRuns(repo, manual)
	switch {
	case len(gaps) > 0:
		match.State = DecisionAdapt
		match.Reasons = append(match.Reasons, "manual requires environment adaptation")
		match.RecommendedNextActions = []string{"review_manual", "adapt_workflow"}
	case len(match.MissingContext) > 0:
		match.State = DecisionNeedMoreInfo
		match.Reasons = append(match.Reasons, "required context is missing")
		match.RecommendedNextActions = []string{"collect_context", "review_manual"}
	default:
		match.State = DecisionDirect
		match.Reasons = append(match.Reasons, "manual structural conditions are satisfied")
		match.RecommendedNextActions = []string{"fill_parameters", "run_precheck", "start_dry_run"}
	}
	return match
}

func normalizeSearchOperationFrame(req SearchOpsManualsRequest) OperationFrame {
	frame := req.OperationFrame
	if operationFrameEmpty(frame) {
		frame = BuildOperationFrame(req.Text, req.Metadata)
	} else {
		if frame.RawText == "" {
			frame.RawText = req.Text
		}
		if frame.Metadata == nil {
			frame.Metadata = cloneMap(req.Metadata)
		}
	}
	if frame.Target.Type == "" {
		frame.Target.Type = firstNonEmpty(frame.ObjectType, frame.Operation.TargetType)
	}
	if frame.ObjectType == "" {
		frame.ObjectType = frame.Target.Type
	}
	if frame.Operation.TargetType == "" {
		frame.Operation.TargetType = frame.Target.Type
	}
	if frame.Operation.Action == "" {
		frame.Operation.Action = frame.OperationType
	}
	if frame.OperationType == "" {
		frame.OperationType = frame.Operation.Action
	}
	if frame.Intent == "" {
		frame.Intent = frame.Operation.Action
	}
	if len(frame.TargetScope.Hosts) == 0 && frame.Target.Name != "" {
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, frame.Target.Name)
	}
	return frame
}

func operationFrameEmpty(frame OperationFrame) bool {
	return frame.RawText == "" &&
		frame.Target.Type == "" &&
		frame.ObjectType == "" &&
		frame.Operation.Action == "" &&
		frame.OperationType == ""
}

func evaluateSearchManual(repo ManualRepository, manual OpsManual, frame OperationFrame) SearchManualHit {
	hit := SearchManualHit{
		Manual:           cloneManual(manual),
		BoundWorkflowID:  strings.TrimSpace(manual.WorkflowRef.WorkflowID),
		RunRecordSummary: summarizeRuns(repo, manual),
	}
	manualTarget := strings.TrimSpace(firstNonEmpty(manual.Operation.TargetType, manual.Applicability.Middleware))
	manualAction := strings.TrimSpace(manual.Operation.Action)
	frameTarget := strings.TrimSpace(frame.Target.Type)
	frameAction := strings.TrimSpace(frame.Operation.Action)
	targetMatches := frameTarget != "" && manualTarget != "" && equalFold(manualTarget, frameTarget)
	actionMatches := frameAction != "" && manualAction != "" && equalFold(manualAction, frameAction)
	switch {
	case targetMatches && actionMatches:
		hit.MatchLevel = "same_object_same_operation"
		hit.MatchedFields = appendUnique(hit.MatchedFields, "object_type")
		hit.MatchedFields = appendUnique(hit.MatchedFields, "operation_type")
	case targetMatches:
		hit.MatchLevel = "same_object_different_operation"
		hit.MatchedFields = appendUnique(hit.MatchedFields, "object_type")
		hit.UsableMode = DecisionReference
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "operation_type differs")
		hit.RecommendedAction = "reference_manual"
		return hit
	case actionMatches:
		hit.MatchLevel = "different_object_same_operation"
		hit.MatchedFields = appendUnique(hit.MatchedFields, "operation_type")
		hit.UsableMode = DecisionReference
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "object_type differs")
		hit.RecommendedAction = "reference_manual"
		return hit
	default:
		hit.UsableMode = DecisionNoMatch
		return hit
	}

	missing := missingFieldsForManual(manual, frame)
	envDiffs := environmentDiffsForManual(manual, frame)
	hit.MissingFields = dedupe(missing)
	hit.EnvironmentDiffs = dedupe(envDiffs)
	if len(hit.MissingFields) == 0 {
		hit.MatchedFields = appendUnique(hit.MatchedFields, "required_context")
	}
	if len(hit.EnvironmentDiffs) == 0 {
		hit.MatchedFields = appendUnique(hit.MatchedFields, "environment")
	}
	workflowAvailable, workflowReason := workflowAvailableForSearch(manual)
	if workflowReason != "" {
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, workflowReason)
	}
	if latestRunFailed(hit.RunRecordSummary) {
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "latest run record did not pass validation")
	}
	if riskExceedsManual(frame.Risk.Level, manual.Operation.RiskLevel) {
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "requested risk level exceeds manual risk boundary")
	}

	switch {
	case len(hit.MissingFields) > 0:
		hit.UsableMode = DecisionNeedInfo
		hit.RecommendedAction = "collect_context"
	case !workflowAvailable:
		hit.UsableMode = DecisionReference
		hit.RecommendedAction = "reference_manual"
	case latestRunFailed(hit.RunRecordSummary):
		hit.UsableMode = DecisionReference
		hit.RecommendedAction = "reference_manual"
	case riskExceedsManual(frame.Risk.Level, manual.Operation.RiskLevel):
		hit.UsableMode = DecisionReference
		hit.RecommendedAction = "reference_manual"
	case len(hit.EnvironmentDiffs) > 0:
		hit.UsableMode = DecisionAdapt
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, environmentDiffReason(hit.EnvironmentDiffs))
		hit.RecommendedAction = "generate_workflow_variant"
	default:
		hit.UsableMode = DecisionDirectExecute
		hit.RecommendedAction = "run_bound_workflow"
	}
	return hit
}

func missingFieldsForManual(manual OpsManual, frame OperationFrame) []string {
	missing := relevantFrameMissing(frame)
	if len(manual.Applicability.ExecutionSurface) > 0 && frame.Environment.ExecutionSurface == "" {
		missing = appendUnique(missing, "execution_surface")
	}
	if len(manual.Applicability.OS) > 0 && frame.Environment.OS == "" {
		missing = appendUnique(missing, "os")
	}
	if len(manual.Applicability.Platform) > 0 && frame.Environment.Platform == "" {
		missing = appendUnique(missing, "platform")
	}
	for _, evidence := range manual.RequiredContext.RequiredEvidence {
		if !hasAny(frame.Evidence.Provided, evidence) {
			missing = appendUnique(missing, evidence)
		}
	}
	for _, input := range manual.RequiredContext.RequiredInputs {
		switch input {
		case "target_instance":
			if frame.Target.Name == "" {
				missing = appendUnique(missing, input)
			}
		default:
			if metadataString(frame.Metadata, input) == "" && metadataString(frame.RequiredParams, input) == "" {
				missing = appendUnique(missing, input)
			}
		}
	}
	if strings.TrimSpace(manual.Operation.RiskLevel) != "" && strings.TrimSpace(frame.Risk.Level) == "" {
		missing = appendUnique(missing, "risk_level")
	}
	return dedupe(missing)
}

func relevantFrameMissing(frame OperationFrame) []string {
	var missing []string
	for _, item := range frame.Evidence.Missing {
		switch item {
		case "target_type", "operation_type", "action", "target_instance", "execution_surface", "backup_path":
			missing = appendUnique(missing, item)
		case "environment", "symptom", "metrics":
			if frame.Operation.Action == "rca_or_repair" {
				missing = appendUnique(missing, item)
			}
		}
	}
	return missing
}

func environmentDiffsForManual(manual OpsManual, frame OperationFrame) []string {
	var diffs []string
	if frame.Environment.ExecutionSurface != "" && !listMatches(manual.Applicability.ExecutionSurface, frame.Environment.ExecutionSurface) {
		diffs = appendUnique(diffs, "execution_surface")
	}
	if frame.Environment.OS != "" && !listMatches(manual.Applicability.OS, frame.Environment.OS) {
		diffs = appendUnique(diffs, "os")
	}
	if frame.Environment.Platform != "" && !listMatches(manual.Applicability.Platform, frame.Environment.Platform) {
		diffs = appendUnique(diffs, "platform")
	}
	if len(diffs) > 0 {
		expectedPM := packageManagerForOS(firstString(manual.Applicability.OS))
		if expectedPM != "" && frame.Environment.PackageManager != "" && expectedPM != frame.Environment.PackageManager {
			diffs = appendUnique(diffs, "package_manager")
		}
	}
	return diffs
}

func workflowAvailableForSearch(manual OpsManual) (bool, string) {
	if strings.TrimSpace(manual.WorkflowRef.WorkflowID) == "" {
		return false, "manual has no bound workflow"
	}
	status := strings.ToLower(strings.TrimSpace(metadataString(manual.Metadata, "workflow_status")))
	if status == "disabled" || status == "deprecated" || status == "off" {
		return false, "bound workflow is not enabled"
	}
	if value, ok := manual.Metadata["workflow_enabled"].(bool); ok && !value {
		return false, "bound workflow is not enabled"
	}
	if value, ok := manual.Metadata["disabled"].(bool); ok && value {
		return false, "bound workflow is not enabled"
	}
	return true, ""
}

func latestRunFailed(summary RunRecordSummary) bool {
	result := strings.ToLower(strings.TrimSpace(summary.RecentResult))
	return result == "failed" || result == "error"
}

func environmentDiffReason(diffs []string) string {
	if len(diffs) == 0 {
		return ""
	}
	return "workflow environment differs: " + strings.Join(diffs, ",")
}

func noHitDecision(frame OperationFrame) DecisionState {
	if len(missingFrameForSearch(frame)) > 0 {
		return DecisionNeedInfo
	}
	return DecisionNoMatch
}

func missingFrameForSearch(frame OperationFrame) []string {
	var missing []string
	if strings.TrimSpace(frame.Target.Type) == "" {
		missing = appendUnique(missing, "object_type")
	}
	if strings.TrimSpace(frame.Operation.Action) == "" {
		missing = appendUnique(missing, "operation_type")
	}
	if strings.TrimSpace(frame.Environment.ExecutionSurface) == "" {
		missing = appendUnique(missing, "execution_surface")
	}
	if strings.TrimSpace(frame.Risk.Level) == "" {
		missing = appendUnique(missing, "risk_level")
	}
	if strings.TrimSpace(frame.Operation.Action) == "rca_or_repair" {
		missing = append(missing, relevantFrameMissing(frame)...)
	}
	return dedupe(missing)
}

func searchedFrameFields(frame OperationFrame) []string {
	fields := []string{}
	for _, item := range []struct {
		name  string
		value string
	}{
		{"object_type", frame.Target.Type},
		{"operation_type", frame.Operation.Action},
		{"target_instance", frame.Target.Name},
		{"environment", frame.Environment.Env},
		{"os", frame.Environment.OS},
		{"platform", frame.Environment.Platform},
		{"execution_surface", frame.Environment.ExecutionSurface},
	} {
		if strings.TrimSpace(item.value) != "" {
			fields = append(fields, item.name)
		}
	}
	if len(frame.Evidence.Provided) > 0 {
		fields = append(fields, "evidence")
	}
	if len(frame.RequiredParams) > 0 {
		fields = append(fields, "required_params")
	}
	if strings.TrimSpace(frame.Risk.Level) != "" {
		fields = append(fields, "risk_level")
	}
	return fields
}

func searchSummary(decision DecisionState, hit *SearchManualHit) string {
	switch decision {
	case DecisionDirectExecute:
		if hit != nil {
			return "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。"
		}
		return "找到可直接使用的运维手册。"
	case DecisionNeedInfo:
		return "信息不足，不能直接使用工作流。"
	case DecisionAdapt:
		return "找到同对象同操作手册，但当前环境存在差异，需要先生成适配工作流。"
	case DecisionReference:
		return "找到可参考的运维手册，但不能直接执行绑定工作流。"
	case DecisionNoMatch:
		return "没有找到合适的运维手册。"
	default:
		return "已完成运维手册检索。"
	}
}

func recommendedNextAction(decision DecisionState) string {
	switch decision {
	case DecisionDirectExecute:
		return "确认参数后进行 Dry Run。"
	case DecisionNeedInfo:
		return "补充缺失信息后重新检索。"
	case DecisionAdapt:
		return "生成适配工作流草稿，用户审核后 Dry Run。"
	case DecisionReference:
		return "按手册步骤参考执行，每一步都需要用户确认。"
	case DecisionNoMatch:
		return "继续普通 Agent 运维流程。"
	default:
		return ""
	}
}

func nextQuestionsForMissing(missing []string) []string {
	var questions []string
	for _, field := range dedupe(missing) {
		switch field {
		case "object_type":
			questions = append(questions, "要处理的运维对象是什么？")
		case "operation_type", "action":
			questions = append(questions, "要执行的操作类型是什么？")
		case "target_instance":
			questions = append(questions, "目标实例是哪一个？")
		case "environment":
			questions = append(questions, "这是生产、测试还是其他环境？")
		case "execution_surface":
			questions = append(questions, "执行方式是 SSH、kubectl、docker exec 还是其他方式？")
		case "symptom":
			questions = append(questions, "当前现象是什么？")
		case "metrics":
			questions = append(questions, "是否有监控指标、日志或 Coroot 证据可供参考？")
		case "backup_path":
			questions = append(questions, "备份文件要保存到哪个路径？")
		case "risk_level":
			questions = append(questions, "这个操作的风险等级是什么？是否会影响生产、写入数据或重启服务？")
		case "os":
			questions = append(questions, "目标主机的操作系统是什么？")
		case "platform":
			questions = append(questions, "部署平台是物理机、虚拟机、Docker 还是 Kubernetes？")
		default:
			questions = append(questions, "请补充 "+field+"。")
		}
	}
	return questions
}

func manualMatchFromSearchHit(hit SearchManualHit, summary string) ManualMatch {
	reasons := cloneStrings(hit.BlockedReasons)
	if len(reasons) == 0 && summary != "" {
		reasons = append(reasons, summary)
	}
	return ManualMatch{
		Manual:                 cloneManual(hit.Manual),
		State:                  hit.UsableMode,
		Reasons:                reasons,
		MissingContext:         cloneStrings(hit.MissingFields),
		CompatibilityGaps:      cloneStrings(hit.EnvironmentDiffs),
		RecommendedNextActions: legacyActionsForSearchHit(hit),
		RunRecordSummary:       hit.RunRecordSummary,
	}
}

func legacyActionsForSearchHit(hit SearchManualHit) []string {
	switch hit.UsableMode {
	case DecisionDirectExecute:
		return []string{"fill_parameters", "run_precheck", "start_dry_run"}
	case DecisionAdapt:
		return []string{"review_manual", "adapt_workflow"}
	case DecisionNeedInfo:
		return []string{"collect_context", "review_manual"}
	case DecisionReference:
		return []string{"review_manual", "step_by_step"}
	default:
		if hit.RecommendedAction != "" {
			return []string{hit.RecommendedAction}
		}
		return nil
	}
}

func sortSearchHits(hits []SearchManualHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if rankSearchHit(hits[i]) != rankSearchHit(hits[j]) {
			return rankSearchHit(hits[i]) < rankSearchHit(hits[j])
		}
		if hits[i].RunRecordSummary.SuccessCount != hits[j].RunRecordSummary.SuccessCount {
			return hits[i].RunRecordSummary.SuccessCount > hits[j].RunRecordSummary.SuccessCount
		}
		if hits[i].RunRecordSummary.FailureCount != hits[j].RunRecordSummary.FailureCount {
			return hits[i].RunRecordSummary.FailureCount < hits[j].RunRecordSummary.FailureCount
		}
		if hits[i].RunRecordSummary.LastRunAt != hits[j].RunRecordSummary.LastRunAt {
			return hits[i].RunRecordSummary.LastRunAt > hits[j].RunRecordSummary.LastRunAt
		}
		return hits[i].Manual.ID < hits[j].Manual.ID
	})
}

func rankSearchHit(hit SearchManualHit) int {
	if hit.MatchLevel == "same_object_same_operation" {
		switch hit.UsableMode {
		case DecisionDirectExecute:
			return 0
		case DecisionAdapt:
			return 1
		case DecisionNeedInfo:
			return 2
		case DecisionReference:
			return 3
		default:
			return 8
		}
	}
	switch hit.UsableMode {
	case DecisionReference:
		return 4
	case DecisionNeedInfo:
		return 5
	case DecisionAdapt:
		return 6
	case DecisionDirectExecute:
		return 7
	default:
		return 8
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func summarizeRuns(repo ManualRepository, manual OpsManual) RunRecordSummary {
	records, err := repo.ListRunRecords(ListRunRecordsRequest{ManualID: manual.ID, WorkflowID: manual.WorkflowRef.WorkflowID, Limit: 50})
	if err != nil {
		return RunRecordSummary{}
	}
	summary := RunRecordSummary{}
	for i, record := range records {
		if record.ValidationStatus == "passed" {
			summary.SuccessCount++
		}
		if record.ExecutionStatus == "failed" || record.ValidationStatus == "failed" {
			summary.FailureCount++
		}
		when := record.CompletedAt
		if when == "" {
			when = record.StartedAt
		}
		if i == 0 || when > summary.LastRunAt {
			summary.LastRunAt = when
			switch {
			case record.ValidationStatus != "":
				summary.RecentResult = record.ValidationStatus
			case record.ExecutionStatus != "":
				summary.RecentResult = record.ExecutionStatus
			}
		}
	}
	return summary
}

func sortMatches(matches []ManualMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		if rankState(matches[i].State) != rankState(matches[j].State) {
			return rankState(matches[i].State) < rankState(matches[j].State)
		}
		if matches[i].RunRecordSummary.SuccessCount != matches[j].RunRecordSummary.SuccessCount {
			return matches[i].RunRecordSummary.SuccessCount > matches[j].RunRecordSummary.SuccessCount
		}
		if matches[i].RunRecordSummary.FailureCount != matches[j].RunRecordSummary.FailureCount {
			return matches[i].RunRecordSummary.FailureCount < matches[j].RunRecordSummary.FailureCount
		}
		if matches[i].RunRecordSummary.LastRunAt != matches[j].RunRecordSummary.LastRunAt {
			return matches[i].RunRecordSummary.LastRunAt > matches[j].RunRecordSummary.LastRunAt
		}
		return matches[i].Manual.ID < matches[j].Manual.ID
	})
}

func rankState(state DecisionState) int {
	switch state {
	case DecisionDirect:
		return 0
	case DecisionAdapt:
		return 1
	case DecisionReference:
		return 2
	case DecisionNeedMoreInfo:
		return 3
	default:
		return 4
	}
}

func riskExceedsManual(requested, allowed string) bool {
	requested = strings.TrimSpace(strings.ToLower(requested))
	allowed = strings.TrimSpace(strings.ToLower(allowed))
	if requested == "" || allowed == "" {
		return false
	}
	return riskLevelRank(requested) > riskLevelRank(allowed)
}

func riskLevelRank(level string) int {
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

func listMatches(allowed []string, value string) bool {
	if len(allowed) == 0 || value == "" {
		return true
	}
	for _, item := range allowed {
		if equalFold(item, value) {
			return true
		}
	}
	return false
}

func missingFrameBasics(frame OperationFrame) []string {
	var missing []string
	if frame.Target.Type == "" {
		missing = append(missing, "target_type")
	}
	if frame.Operation.Action == "" {
		missing = append(missing, "action")
	}
	return missing
}
