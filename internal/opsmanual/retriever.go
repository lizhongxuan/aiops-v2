package opsmanual

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
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
	OpsManualFlowID string
	ManualID        string
	WorkflowID      string
	Limit           int
}

func SearchOpsManuals(repo ManualRepository, req SearchOpsManualsRequest) (SearchOpsManualsResult, error) {
	return SearchOpsManualsWithHintProvider(context.Background(), repo, req, nil)
}

func SearchOpsManualsWithHintProvider(ctx context.Context, repo ManualRepository, req SearchOpsManualsRequest, provider HintProvider) (SearchOpsManualsResult, error) {
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
	hits := generateCandidates(repo, manuals, frame)
	if len(hits) == 0 {
		hits = append(hits, genericStatefulClusterRepairFallbackHits(repo, frame)...)
	}
	sortSearchHits(hits)
	hits = applyManualHintsToSearchHits(ctx, hits, frame, req, provider)
	if req.Limit > 0 && len(hits) > req.Limit {
		hits = hits[:req.Limit]
	}
	result.Manuals = hits
	if len(hits) > 0 {
		result.OpsManualFlowID = BuildOpsManualFlowIDFromMetadata(req.Metadata, hits[0].Manual.ID, firstNonEmpty(hits[0].BoundWorkflowID, hits[0].Manual.WorkflowRef.WorkflowID), frame)
	} else {
		result.OpsManualFlowID = BuildOpsManualFlowIDFromMetadata(req.Metadata, "", "", frame)
	}
	if len(hits) == 0 {
		result.Decision = noHitDecision(frame)
		result.Summary = searchSummary(result.Decision, nil, frame)
		result.NextQuestions = nextQuestionsForMissing(missingFrameForSearch(frame))
		result.RecommendedNextAction = recommendedNextAction(result.Decision)
		return result, nil
	}
	result.Decision = hits[0].UsableMode
	result.Summary = searchSummary(result.Decision, &hits[0], frame)
	if result.Decision == DecisionNeedInfo {
		result.NextQuestions = nextQuestionsForNeedInfo(hits[0].Manual, frame, hits[0].MissingFields)
	}
	result.RecommendedNextAction = recommendedNextAction(result.Decision)
	return result, nil
}

func genericStatefulClusterRepairFallbackHits(repo ManualRepository, frame OperationFrame) []SearchManualHit {
	if !shouldOfferGenericStatefulClusterRepair(frame) {
		return nil
	}
	manual := genericStatefulClusterRepairManual(frame)
	hit := evaluateSearchManual(repo, manual, frame)
	if hit.UsableMode == DecisionNoMatch {
		return nil
	}
	hit.MatchLevel = firstNonEmpty(hit.MatchLevel, "generic_stateful_cluster_repair")
	hit.HintSources = appendUnique(hit.HintSources, "generic_capability_fallback")
	if hit.UsableMode == DecisionDirectExecute || hit.UsableMode == DecisionAdapt {
		hit.UsableMode = DecisionReference
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "generic fallback is not a high-confidence manual match")
	}
	hit.RecommendedAction = "reference_manual"
	hit = EnrichSearchManualHitRecommendation(hit)
	return []SearchManualHit{hit}
}

func shouldOfferGenericStatefulClusterRepair(frame OperationFrame) bool {
	targetType := strings.TrimSpace(frame.Target.Type)
	if targetType == "" {
		return false
	}
	if !operationFrameHasResourceScope(frame) {
		return false
	}
	action := strings.TrimSpace(firstNonEmpty(frame.Operation.Action, frame.OperationType, frame.Intent))
	switch action {
	case "restore", "rca_or_repair", "repair", "recover":
	default:
		lower := strings.ToLower(frame.RawText)
		if !strings.Contains(lower, "恢复") &&
			!strings.Contains(lower, "修复") &&
			!strings.Contains(lower, "异常") &&
			!strings.Contains(lower, "recover") &&
			!strings.Contains(lower, "restore") &&
			!strings.Contains(lower, "repair") &&
			!strings.Contains(lower, "fix") {
			return false
		}
	}
	if frame.Operation.Stateful || DefaultOpsManualCapabilityRegistry().IsStatefulTargetType(targetType) {
		return true
	}
	for _, role := range frame.Roles {
		if role.Kind == ResourceRoleDataNode || role.Kind == ResourceRoleMonitor {
			return true
		}
	}
	return false
}

func genericStatefulClusterRepairManual(frame OperationFrame) OpsManual {
	targetType := strings.TrimSpace(frame.Target.Type)
	action := strings.TrimSpace(firstNonEmpty(frame.Operation.Action, frame.OperationType, frame.Intent, "rca_or_repair"))
	return OpsManual{
		ID:      "manual-generic-stateful-cluster-repair",
		Title:   "通用有状态集群恢复运维手册",
		Status:  ManualStatusVerified,
		Version: "v1",
		WorkflowRef: WorkflowRef{
			WorkflowID: "workflow-generic-stateful-cluster-repair",
		},
		Operation: OperationProfile{
			TargetType: targetType,
			Action:     action,
			RiskLevel:  "high",
			Stateful:   true,
		},
		RetrievalProfile: RetrievalProfile{
			MinScore: ScoreThresholds{Candidate: 0.1, DirectExecute: 0.5},
		},
		RunnableConditions: RunnableConditions{
			RequiresApproval: true,
		},
		PreflightProbe: PreflightProbe{
			ID:       "generic_stateful_cluster_readonly_preflight",
			Type:     "generic",
			Action:   "collect_readonly_cluster_evidence",
			ReadOnly: true,
			RequiredOutputs: []string{
				"resource_roles",
				"member_health",
				"storage_health",
				"sync_status",
				"observer_health",
			},
		},
		RiskPolicy: RiskPolicy{
			BlastRadius:  "cluster",
			DataMutation: true,
			ApprovalRequiredWhen: []string{
				"repair_execution",
				"data_loss_acceptable",
				"role_rebuild",
			},
		},
		Validation: []string{
			"验证 member_health 正常",
			"验证 sync_status 正常",
			"验证 observer_health 正常",
		},
		SearchDoc:        "generic stateful middleware cluster repair recovery read_only_evidence_first approval_before_mutation member_health storage_health sync_status observer_health",
		DocumentMarkdown: "通用有状态集群恢复手册：先收集只读证据，再给方案；执行前必须审批；恢复后必须独立验证成员健康、同步状态和观察点健康。",
		Metadata: map[string]any{
			"preflight_discovers_context": true,
		},
	}
}

func applyManualHintsToSearchHits(ctx context.Context, hits []SearchManualHit, frame OperationFrame, req SearchOpsManualsRequest, provider HintProvider) []SearchManualHit {
	if len(hits) == 0 || provider == nil {
		return hits
	}
	hints, err := provider.ManualHints(ctx, HintQuery{
		Text:           firstNonEmpty(strings.TrimSpace(req.Text), strings.TrimSpace(frame.RawText)),
		OperationFrame: frame,
		SessionID:      firstMetadataAnyValue(req.Metadata, "session_id", "sessionId"),
		ProjectID:      firstMetadataAnyValue(req.Metadata, "project_id", "projectId"),
		Now:            time.Now().UTC(),
		Limit:          8,
	})
	if err != nil || len(hints) == 0 {
		return hits
	}
	original := make(map[string]int, len(hits))
	for idx, hit := range hits {
		original[hit.Manual.ID] = idx
	}
	hintSources := map[string][]string{}
	for _, hint := range hints {
		if !manualHintUsableForFrame(hint, frame) {
			continue
		}
		manualID := strings.TrimSpace(hint.ManualID)
		if manualID == "" {
			continue
		}
		source := firstNonEmpty(strings.TrimSpace(hint.Source), "memory_hint")
		if source != "memory_hint" && source != "letta_hint" {
			source = "memory_hint"
		}
		hintSources[manualID] = appendUnique(hintSources[manualID], source)
	}
	if len(hintSources) == 0 {
		return hits
	}
	for idx := range hits {
		if sources := hintSources[hits[idx].Manual.ID]; len(sources) > 0 {
			for _, source := range sources {
				hits[idx].HintSources = appendUnique(hits[idx].HintSources, source)
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if manualHintNearTie(hits[i], hits[j], frame) {
			leftHinted := len(hits[i].HintSources) > 0
			rightHinted := len(hits[j].HintSources) > 0
			if leftHinted != rightHinted {
				return leftHinted
			}
		}
		return original[hits[i].Manual.ID] < original[hits[j].Manual.ID]
	})
	return hits
}

func manualHintUsableForFrame(hint ManualHint, frame OperationFrame) bool {
	if !hint.Redacted {
		return false
	}
	now := time.Now().UTC()
	if !hint.ExpiresAt.IsZero() && !hint.ExpiresAt.After(now) {
		return false
	}
	return hintScopeMatches(hint.ObjectType, frame.Target.Type) && hintActionMatches(hint.Action, frame.Operation.Action)
}

func manualHintNearTie(left SearchManualHit, right SearchManualHit, frame OperationFrame) bool {
	if !sameObjectActionSearchHit(left, frame) || !sameObjectActionSearchHit(right, frame) {
		return false
	}
	if rankSearchHit(left) != rankSearchHit(right) {
		return false
	}
	return math.Abs(left.ScoreBreakdown.FinalScore-right.ScoreBreakdown.FinalScore) <= 0.03
}

func sameObjectActionSearchHit(hit SearchManualHit, frame OperationFrame) bool {
	manualTarget := firstNonEmpty(hit.Manual.Operation.TargetType, hit.Manual.Applicability.Middleware)
	return strings.TrimSpace(manualTarget) != "" &&
		strings.TrimSpace(frame.Target.Type) != "" &&
		equalFold(manualTarget, frame.Target.Type) &&
		operationsCompatibleForSearch(hit.Manual.Operation.Action, frame.Operation.Action)
}

func generateCandidates(repo ManualRepository, manuals []OpsManual, frame OperationFrame) []SearchManualHit {
	hits := make([]SearchManualHit, 0, len(manuals))
	for _, manual := range manuals {
		hit := evaluateSearchManual(repo, manual, frame)
		if hit.UsableMode == DecisionNoMatch {
			continue
		}
		hits = append(hits, hit)
	}
	return hits
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
		case DefaultOpsManualCapabilityRegistry().ManualApplicabilityConstraintReason(rule, frame.Environment.Platform) != "":
			match.Reasons = append(match.Reasons, DefaultOpsManualCapabilityRegistry().ManualApplicabilityConstraintReason(rule, frame.Environment.Platform))
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
		match.RecommendedNextActions = []string{"fill_parameters", "run_precheck", "confirm_execution"}
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
		frame.Metadata = mergeFrameMetadata(frame.Metadata, req.Metadata)
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
	applyExplicitContextMetadata(&frame, frame.Metadata, DefaultOpsManualCapabilityRegistry())
	if len(frame.TargetScope.Hosts) == 0 && frame.Target.Name != "" {
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, frame.Target.Name)
	}
	return frame
}

func operationFrameEmpty(frame OperationFrame) bool {
	return frame.RawText == "" &&
		frame.Target.Type == "" &&
		frame.Target.Name == "" &&
		frame.ObjectType == "" &&
		frame.Operation.Action == "" &&
		frame.OperationType == "" &&
		len(frame.TargetScope.Hosts) == 0 &&
		len(frame.RequiredParams) == 0 &&
		len(frame.Metadata) == 0
}

func mergeFrameMetadata(primary, fallback map[string]any) map[string]any {
	if len(primary) == 0 {
		return cloneMap(fallback)
	}
	if len(fallback) == 0 {
		return primary
	}
	merged := cloneMap(fallback)
	for key, value := range primary {
		merged[key] = value
	}
	return merged
}

func evaluateSearchManual(repo ManualRepository, manual OpsManual, frame OperationFrame) SearchManualHit {
	summary := summarizeRuns(repo, manual)
	hit := SearchManualHit{
		Manual:           cloneManual(manual),
		BoundWorkflowID:  strings.TrimSpace(manual.WorkflowRef.WorkflowID),
		RunRecordSummary: summary,
		PreflightStatus:  PreflightStatusNotRun,
	}
	filter := hardFilterCandidate(manual, frame, summary)
	if !filter.Allowed {
		hit.UsableMode = DecisionNoMatch
		hit.BlockedReasons = append(hit.BlockedReasons, filter.Reasons...)
		return EnrichSearchManualHitRecommendation(hit)
	}
	hit.ScoreBreakdown = calculateScoreBreakdown(manual, frame, summary, nil)
	manualTarget := strings.TrimSpace(firstNonEmpty(manual.Operation.TargetType, manual.Applicability.Middleware))
	manualAction := strings.TrimSpace(manual.Operation.Action)
	frameTarget := strings.TrimSpace(frame.Target.Type)
	frameAction := strings.TrimSpace(frame.Operation.Action)
	targetMatches := frameTarget != "" && manualTarget != "" && equalFold(manualTarget, frameTarget)
	actionMatches := operationsCompatibleForSearch(manualAction, frameAction)
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
		return EnrichSearchManualHitRecommendation(hit)
	case actionMatches:
		hit.MatchLevel = "different_object_same_operation"
		hit.MatchedFields = appendUnique(hit.MatchedFields, "operation_type")
		hit.UsableMode = DecisionReference
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, "object_type differs")
		hit.RecommendedAction = "reference_manual"
		return EnrichSearchManualHitRecommendation(hit)
	default:
		hit.UsableMode = DecisionNoMatch
		return EnrichSearchManualHitRecommendation(hit)
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
	for _, reason := range filter.Reasons {
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, reason)
	}

	switch {
	case len(hit.MissingFields) > 0:
		hit.UsableMode = DecisionNeedInfo
		hit.RecommendedAction = "collect_context"
	case filter.MaxDecision == DecisionReference:
		hit.UsableMode = DecisionReference
		hit.RecommendedAction = "reference_manual"
	case len(hit.EnvironmentDiffs) > 0:
		hit.UsableMode = DecisionAdapt
		hit.BlockedReasons = appendUnique(hit.BlockedReasons, environmentDiffReason(hit.EnvironmentDiffs))
		hit.RecommendedAction = "generate_workflow_variant"
	default:
		hit.UsableMode = DecisionDirectExecute
		hit.PreflightStatus = PreflightStatusNotRun
		hit.RecommendedAction = "run_preflight_probe"
		if hit.RunRecordSummary.SuccessCount == 0 {
			hit.UsableMode = DecisionReference
			hit.RecommendedAction = "reference_manual"
			hit.BlockedReasons = appendUnique(hit.BlockedReasons, "no successful run record for execution recommendation")
		} else if hit.ScoreBreakdown.FinalScore < directThreshold(manual) {
			hit.UsableMode = DecisionReference
			hit.RecommendedAction = "reference_manual"
			hit.BlockedReasons = appendUnique(hit.BlockedReasons, "manual score is below direct execution threshold")
		}
	}
	hit.UsableMode = capDecision(hit.UsableMode, filter.MaxDecision)
	return EnrichSearchManualHitRecommendation(hit)
}

func operationsCompatibleForSearch(manualAction, frameAction string) bool {
	manualAction = strings.TrimSpace(manualAction)
	frameAction = strings.TrimSpace(frameAction)
	if manualAction == "" || frameAction == "" {
		return false
	}
	if equalFold(manualAction, frameAction) {
		return true
	}
	return equalFold(manualAction, "rca_or_repair") && equalFold(frameAction, "status_check")
}

func candidateThreshold(manual OpsManual) float64 {
	if threshold := effectiveRetrievalProfile(manual).MinScore.Candidate; threshold > 0 {
		return threshold
	}
	return candidateMinScore
}

func directThreshold(manual OpsManual) float64 {
	if threshold := effectiveRetrievalProfile(manual).MinScore.DirectExecute; threshold > 0 {
		return threshold
	}
	return directExecuteMinScore
}

func missingFieldsForManual(manual OpsManual, frame OperationFrame) []string {
	missing := relevantFrameMissingForManual(manual, frame)
	if len(manual.Applicability.ExecutionSurface) > 0 && frame.Environment.ExecutionSurface == "" {
		if !manualPreflightDiscoversContext(manual) {
			missing = appendUnique(missing, "execution_surface")
		}
	}
	if len(manual.Applicability.OS) > 0 && frame.Environment.OS == "" {
		missing = appendUnique(missing, "os")
	}
	if len(manual.Applicability.Platform) > 0 && frame.Environment.Platform == "" {
		missing = appendUnique(missing, "platform")
	}
	for _, evidence := range manual.RequiredContext.RequiredEvidence {
		if frame.Operation.Action == "status_check" && (evidence == "symptom" || evidence == "metrics") {
			continue
		}
		if manualPreflightDiscoversContext(manual) && manualReadOnlyPreflightCanCollect(manual, evidence) {
			continue
		}
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

func relevantFrameMissingForManual(manual OpsManual, frame OperationFrame) []string {
	var missing []string
	for _, item := range relevantFrameMissing(frame) {
		switch item {
		case "target_instance":
			if !manualRequiresInput(manual, item) && (operationFrameHasResourceScope(frame) || manualPreflightDiscoversContext(manual)) {
				continue
			}
		case "execution_surface":
			if !manualRequiresInput(manual, item) && manualPreflightDiscoversContext(manual) {
				continue
			}
		case "environment", "symptom", "metrics":
			if manualPreflightDiscoversContext(manual) {
				continue
			}
		}
		missing = appendUnique(missing, item)
	}
	return missing
}

func manualRequiresInput(manual OpsManual, input string) bool {
	for _, required := range manual.RequiredContext.RequiredInputs {
		if strings.EqualFold(strings.TrimSpace(required), strings.TrimSpace(input)) {
			return true
		}
	}
	if rule, ok := manual.ParameterRules[input]; ok && rule.Required {
		return true
	}
	return false
}

func operationFrameHasResourceScope(frame OperationFrame) bool {
	if len(frame.Roles) > 0 || len(frame.TargetScope.Hosts) > 0 {
		return true
	}
	return strings.TrimSpace(frame.TargetScope.Service) != "" || strings.TrimSpace(frame.TargetScope.Cluster) != ""
}

func manualReadOnlyPreflightCanCollect(manual OpsManual, field string) bool {
	if !manual.PreflightProbe.ReadOnly || strings.TrimSpace(field) == "" {
		return false
	}
	return hasAny(manual.PreflightProbe.RequiredOutputs, field)
}

func manualPreflightDiscoversContext(manual OpsManual) bool {
	value, ok := manual.Metadata["preflight_discovers_context"]
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled && manual.PreflightProbe.ReadOnly
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
	if summary.Suppressed {
		return true
	}
	result := strings.ToLower(strings.TrimSpace(firstNonEmpty(summary.LatestStatus, summary.RecentResult)))
	return result == "failed" || result == "error"
}

func environmentDiffReason(diffs []string) string {
	if len(diffs) == 0 {
		return ""
	}
	return "workflow environment differs: " + strings.Join(diffs, ",")
}

func noHitDecision(frame OperationFrame) DecisionState {
	if strings.TrimSpace(frame.Target.Type) == "" || strings.TrimSpace(frame.Operation.Action) == "" {
		return DecisionNeedInfo
	}
	if strings.TrimSpace(frame.Operation.Action) != "rca_or_repair" && len(missingFrameForSearch(frame)) > 0 {
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

func searchSummary(decision DecisionState, hit *SearchManualHit, frame OperationFrame) string {
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
		return "没有可直接运行的 Workflow，可继续只读自动化排查。"
	case DecisionNoMatch:
		if strings.TrimSpace(frame.Target.Type) != "" {
			return fmt.Sprintf("没有找到适用于 %s 的可用运维手册。", displayObjectType(frame.Target.Type))
		}
		return "没有找到合适的运维手册。"
	default:
		return "已完成运维手册检索。"
	}
}

func recommendedNextAction(decision DecisionState) string {
	switch decision {
	case DecisionDirectExecute:
		return "运行 Node 0 预检，通过后确认或审批执行。"
	case DecisionNeedInfo:
		return "补充缺失信息后重新检索。"
	case DecisionAdapt:
		return "生成适配工作流草稿，用户审核并完成发布前检查。"
	case DecisionReference:
		return "没有可直接运行的 Workflow；继续只读自动化排查，若缺目标、时间范围、权限或观测数据会说明阻塞原因。"
	case DecisionNoMatch:
		return "AI 会继续自动尝试只读排查；如果缺少目标、时间范围、权限或观测数据，会先让你补齐必要信息。"
	default:
		return ""
	}
}

func displayObjectType(value string) string {
	return DefaultOpsManualCapabilityRegistry().DisplayObjectType(value)
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
			questions = append(questions, "当前能直接描述的指标或日志现象是什么？")
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

func nextQuestionsForNeedInfo(manual OpsManual, frame OperationFrame, missing []string) []string {
	if frame.Operation.Action == "rca_or_repair" || manual.Operation.Action == "rca_or_repair" {
		prioritized := []string{}
		for _, field := range []string{"target_instance", "environment", "execution_surface", "symptom", "metrics"} {
			if hasAny(missing, field) {
				prioritized = appendUnique(prioritized, field)
			}
		}
		for _, field := range missing {
			if field == "risk_level" || field == "os" || field == "platform" {
				continue
			}
			prioritized = appendUnique(prioritized, field)
		}
		return limitQuestions(nextQuestionsForMissing(prioritized), 4)
	}
	return limitQuestions(nextQuestionsForMissing(missing), 4)
}

func limitQuestions(questions []string, limit int) []string {
	if limit <= 0 || len(questions) <= limit {
		return questions
	}
	return questions[:limit]
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
		return []string{"fill_parameters", "run_preflight_probe", "confirm_execution"}
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
		if hits[i].ScoreBreakdown.FinalScore != hits[j].ScoreBreakdown.FinalScore {
			return hits[i].ScoreBreakdown.FinalScore > hits[j].ScoreBreakdown.FinalScore
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
	return SummarizeRunRecords(records)
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
