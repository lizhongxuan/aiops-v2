package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo                   ManualRepository
	discovery              ResourceDiscovery
	sessionContext         SessionOpsContextStore
	hintProvider           HintProvider
	workflowDigestResolver WorkflowDigestResolver
	workflowPlanChecker    WorkflowPlanChecker
}

type PrepareManualCandidateRequest struct {
	SourceType       string    `json:"source_type"`
	SourceRefs       []string  `json:"source_refs,omitempty"`
	Manual           OpsManual `json:"manual"`
	ValidationReport []string  `json:"validation_report,omitempty"`
}

type ConfirmManualCandidateRequest struct {
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewNote string `json:"review_note,omitempty"`
}

type ServiceOption func(*Service)

type WorkflowDigestResolver interface {
	ResolveWorkflowDigest(context.Context, string) (string, error)
}

type WorkflowDigestResolverFunc func(context.Context, string) (string, error)

func (f WorkflowDigestResolverFunc) ResolveWorkflowDigest(ctx context.Context, workflowID string) (string, error) {
	return f(ctx, workflowID)
}

func WithResourceDiscovery(discovery ResourceDiscovery) ServiceOption {
	return func(s *Service) {
		s.discovery = discovery
	}
}

func WithSessionOpsContextStore(store SessionOpsContextStore) ServiceOption {
	return func(s *Service) {
		s.sessionContext = store
	}
}

func WithHintProvider(provider HintProvider) ServiceOption {
	return func(s *Service) {
		s.hintProvider = provider
	}
}

func WithWorkflowDigestResolver(resolver WorkflowDigestResolver) ServiceOption {
	return func(s *Service) {
		s.workflowDigestResolver = resolver
	}
}

func WithWorkflowPlanChecker(checker WorkflowPlanChecker) ServiceOption {
	return func(s *Service) {
		s.workflowPlanChecker = checker
	}
}

func NewService(repo ManualRepository, opts ...ServiceOption) *Service {
	service := &Service{repo: repo}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) ListManuals(req ListManualsRequest) ([]OpsManual, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("manual repository is nil")
	}
	return s.repo.ListManuals(req)
}

func (s *Service) GetManual(id string) (OpsManual, error) {
	if s.repo == nil {
		return OpsManual{}, fmt.Errorf("manual repository is nil")
	}
	return s.repo.GetManual(strings.TrimSpace(id))
}

func (s *Service) RetrieveManuals(frame OperationFrame) ([]ManualMatch, error) {
	return RetrieveManuals(s.repo, frame)
}

func (s *Service) SearchOpsManuals(req SearchOpsManualsRequest) (SearchOpsManualsResult, error) {
	result, err := SearchOpsManualsWithHintProvider(context.Background(), s.repo, req, s.hintProvider)
	if err != nil {
		return SearchOpsManualsResult{}, err
	}
	return s.applySessionOpsManualSuppression(context.Background(), req, result)
}

func (s *Service) RecordOpsManualSuppressionFromMetadata(ctx context.Context, sessionID string, requestText string, metadata map[string]any) error {
	if s == nil || s.sessionContext == nil {
		return nil
	}
	if !metadataRequestsOpsManualSuppression(metadata) {
		return nil
	}
	sessionID = strings.TrimSpace(firstNonEmpty(sessionID, firstMetadataAnyValue(metadata, "session_id", "sessionId")))
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	frame := BuildOperationFrame(requestText, metadata)
	suppression, ok := OpsManualSuppressionFromMetadata(metadata, frame)
	if !ok {
		return nil
	}
	return s.sessionContext.UpsertFact(ctx, sessionID, NewOpsManualSuppressionFact(suppression, time.Now().UTC()))
}

func (s *Service) applySessionOpsManualSuppression(ctx context.Context, req SearchOpsManualsRequest, result SearchOpsManualsResult) (SearchOpsManualsResult, error) {
	if s == nil || s.sessionContext == nil || len(result.Manuals) == 0 {
		return result, nil
	}
	if explicitOpsManualUseRequested(req.Metadata) {
		return result, nil
	}
	sessionID := firstMetadataAnyValue(req.Metadata, "session_id", "sessionId")
	if strings.TrimSpace(sessionID) == "" {
		return result, nil
	}
	facts, err := s.sessionContext.ListFacts(ctx, sessionID, SessionOpsFactFilter{
		Keys: []string{SessionOpsFactOpsManualSuppression},
		Now:  time.Now().UTC(),
	})
	if err != nil {
		return result, err
	}
	if len(facts) == 0 {
		return result, nil
	}
	kept := make([]SearchManualHit, 0, len(result.Manuals))
	suppressed := make([]string, 0, len(result.Manuals))
	reason := ""
	for _, hit := range result.Manuals {
		candidate := OpsManualSuppressionForManual(hit.Manual, result.OperationFrame)
		if matched, matchedReason := suppressionMatchesFacts(facts, candidate); matched {
			suppressed = appendUnique(suppressed, hit.Manual.ID)
			if reason == "" {
				reason = firstNonEmpty(matchedReason, "user_opt_out")
			}
			continue
		}
		kept = append(kept, hit)
	}
	if len(suppressed) == 0 {
		return result, nil
	}
	result.Manuals = kept
	result.SuppressedManuals = suppressed
	result.SuppressionReason = firstNonEmpty(reason, "user_opt_out")
	if len(result.Manuals) == 0 {
		result.Decision = DecisionNoMatch
		result.Summary = "用户本会话已选择不使用该运维手册；本轮按普通只读排查继续。"
		result.NextQuestions = nil
		result.RecommendedNextAction = recommendedNextAction(DecisionNoMatch)
		result.OpsManualFlowID = BuildOpsManualFlowIDFromMetadata(req.Metadata, "", "", result.OperationFrame)
		return result, nil
	}
	top := result.Manuals[0]
	result.Decision = top.UsableMode
	result.Summary = searchSummary(result.Decision, &top, result.OperationFrame)
	if result.Decision == DecisionNeedInfo {
		result.NextQuestions = nextQuestionsForNeedInfo(top.Manual, result.OperationFrame, top.MissingFields)
	} else {
		result.NextQuestions = nil
	}
	result.RecommendedNextAction = recommendedNextAction(result.Decision)
	result.OpsManualFlowID = BuildOpsManualFlowIDFromMetadata(req.Metadata, top.Manual.ID, firstNonEmpty(top.BoundWorkflowID, top.Manual.WorkflowRef.WorkflowID), result.OperationFrame)
	return result, nil
}

func suppressionMatchesFacts(facts []SessionOpsFact, candidate OpsManualSuppression) (bool, string) {
	for _, fact := range facts {
		suppression, ok := OpsManualSuppressionFromFact(fact)
		if !ok || !suppression.Matches(candidate) {
			continue
		}
		return true, suppression.Reason
	}
	return false, ""
}

func metadataRequestsOpsManualSuppression(metadata map[string]any) bool {
	action := firstMetadataAnyValue(metadata, "opsManualAction", "ops_manual_action")
	if strings.EqualFold(strings.TrimSpace(action), "skip_ops_manual") {
		return true
	}
	skipped := firstMetadataAnyValue(metadata, "opsManualSkipped", "ops_manual_skipped")
	return strings.EqualFold(strings.TrimSpace(skipped), "true") || strings.TrimSpace(skipped) == "1"
}

func explicitOpsManualUseRequested(metadata map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(firstMetadataAnyValue(metadata, "opsManualAction", "ops_manual_action"))) {
	case "use_ops_manual", "reference_ops_manual", "run_ops_manual_preflight":
		return true
	default:
		return false
	}
}

func (s *Service) ListRunRecords(req ListRunRecordsRequest) ([]RunRecord, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("manual repository is nil")
	}
	return s.repo.ListRunRecords(req)
}

func (s *Service) ListCandidates() ([]ManualCandidate, error) {
	repo, ok := s.repo.(CandidateRepository)
	if !ok {
		return nil, fmt.Errorf("candidate repository is not configured")
	}
	candidates, err := repo.ListCandidates()
	if err != nil {
		return nil, err
	}
	out := make([]ManualCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, cloneCandidate(candidate))
	}
	return out, nil
}

func (s *Service) PrepareManualCandidate(req PrepareManualCandidateRequest) (ManualCandidate, error) {
	repo, ok := s.repo.(CandidateRepository)
	if !ok {
		return ManualCandidate{}, fmt.Errorf("candidate repository is not configured")
	}
	manual := cloneManual(req.Manual)
	manual.Status = ManualStatusDraft
	now := time.Now().UTC().Format(time.RFC3339)
	candidate := ManualCandidate{
		ID:               manual.ID,
		SourceType:       strings.TrimSpace(req.SourceType),
		SourceRefs:       cloneStrings(req.SourceRefs),
		ProposedManual:   manual,
		ValidationReport: cloneStrings(req.ValidationReport),
		ReviewStatus:     "pending",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if candidate.ID == "" {
		candidate.ID = "candidate-" + strings.ReplaceAll(now, ":", "")
	}
	if candidate.SourceType == "" {
		candidate.SourceType = "manual"
	}
	if err := repo.SaveCandidate(candidate); err != nil {
		return ManualCandidate{}, err
	}
	return cloneCandidate(candidate), nil
}

func (s *Service) GenerateManualCandidateFromWorkflow(ctx context.Context, req WorkflowManualGenerationRequest) (WorkflowManualGenerationResult, error) {
	repo, ok := s.repo.(CandidateRepository)
	if !ok {
		return WorkflowManualGenerationResult{}, fmt.Errorf("candidate repository is not configured")
	}
	result, err := GenerateWorkflowManualCandidate(ctx, req, nil)
	if err != nil {
		return WorkflowManualGenerationResult{}, err
	}
	candidate := result.Candidate
	if candidate.StructuredValidationReport.Status == "blocked" {
		candidate.ReviewStatus = "needs_fix"
	} else {
		candidate.ReviewStatus = "pending"
	}
	if err := repo.SaveCandidate(candidate); err != nil {
		return WorkflowManualGenerationResult{}, err
	}
	saved := cloneCandidate(candidate)
	return WorkflowManualGenerationResult{
		Candidate:        saved,
		ValidationReport: saved.StructuredValidationReport,
		UserSummary:      saved.UserSummary,
	}, nil
}

func (s *Service) ConfirmManualCandidate(id string, req ConfirmManualCandidateRequest) (OpsManual, error) {
	repo, ok := s.repo.(CandidateRepository)
	if !ok {
		return OpsManual{}, fmt.Errorf("candidate repository is not configured")
	}
	candidate, err := repo.GetCandidate(strings.TrimSpace(id))
	if err != nil {
		return OpsManual{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	candidate.ReviewStatus = "confirmed"
	candidate.Reviewer = strings.TrimSpace(req.Reviewer)
	candidate.ReviewNote = strings.TrimSpace(req.ReviewNote)
	candidate.UpdatedAt = now
	manual := cloneManual(candidate.ProposedManual)
	manual.Status = ManualStatusVerified
	if err := validateManualForVerification(manual); err != nil {
		return OpsManual{}, err
	}
	if manual.UpdatedAt == "" {
		manual.UpdatedAt = now
	}
	if manual.CreatedAt == "" {
		manual.CreatedAt = now
	}
	if s.workflowDigestResolver != nil {
		currentDigest, err := s.workflowDigestResolver.ResolveWorkflowDigest(context.Background(), manual.WorkflowRef.WorkflowID)
		if err != nil {
			return OpsManual{}, err
		}
		if strings.TrimSpace(currentDigest) != "" && strings.TrimSpace(currentDigest) != strings.TrimSpace(manual.WorkflowRef.WorkflowDigest) {
			return OpsManual{}, fmt.Errorf("workflow digest mismatch")
		}
	}
	if err := s.repo.SaveManual(manual); err != nil {
		return OpsManual{}, err
	}
	if err := repo.SaveCandidate(candidate); err != nil {
		return OpsManual{}, err
	}
	return manual, nil
}

func validateManualForVerification(manual OpsManual) error {
	if strings.TrimSpace(manual.WorkflowRef.WorkflowID) == "" {
		return fmt.Errorf("verified ops manual requires exactly one workflow binding")
	}
	if strings.TrimSpace(manual.WorkflowRef.WorkflowDigest) == "" {
		return fmt.Errorf("verified ops manual requires workflow digest")
	}
	if strings.TrimSpace(manual.Operation.TargetType) == "" || strings.TrimSpace(manual.Operation.Action) == "" {
		return fmt.Errorf("verified ops manual requires target type and action")
	}
	for _, input := range nonEmptyStrings(manual.RequiredContext.RequiredInputs) {
		if _, ok := manual.ParameterRules[input]; !ok {
			return fmt.Errorf("verified ops manual requires parameter rule for required input %q", input)
		}
	}
	for name, rule := range manual.ParameterRules {
		if isSensitiveParameterKey(name) && !isEmptyValue(rule.DefaultValue) {
			return fmt.Errorf("verified ops manual rejects sensitive default value for %q", name)
		}
	}
	if riskLevelRank(manual.Operation.RiskLevel) >= riskLevelRank("high") &&
		len(nonEmptyStrings(manual.RiskPolicy.ApprovalRequiredWhen)) == 0 {
		return fmt.Errorf("verified high-risk ops manual requires approval policy")
	}
	if len(nonEmptyStrings(manual.Validation)) == 0 {
		return fmt.Errorf("verified ops manual requires validation steps")
	}
	if len(nonEmptyStrings(manual.CannotUseWhen)) == 0 {
		return fmt.Errorf("verified ops manual requires cannot_use_when safety boundaries")
	}
	if strings.TrimSpace(manual.DocumentMarkdown) == "" {
		return fmt.Errorf("verified ops manual requires document_markdown")
	}
	return nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func (s *Service) CreateRunRecord(record RunRecord) (RunRecord, error) {
	repo, ok := s.repo.(RunRecordRepository)
	if !ok {
		return RunRecord{}, fmt.Errorf("run record repository is not configured")
	}
	cp := cloneRunRecord(record)
	if cp.ID == "" {
		cp.ID = "run-record-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	if err := repo.SaveRunRecord(cp); err != nil {
		return RunRecord{}, err
	}
	return cp, nil
}
