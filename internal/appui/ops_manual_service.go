package appui

import (
	"fmt"
	"strings"

	"aiops-v2/internal/opsmanual"
)

type OpsManualView = opsmanual.OpsManual
type OpsManualCandidateView = opsmanual.ManualCandidate

type OpsManualListRequest struct {
	Status           opsmanual.ManualStatus `json:"status,omitempty"`
	TargetType       string                 `json:"target_type,omitempty"`
	Action           string                 `json:"action,omitempty"`
	Middleware       string                 `json:"middleware,omitempty"`
	ExecutionSurface string                 `json:"execution_surface,omitempty"`
	Limit            int                    `json:"limit,omitempty"`
}

type OpsManualListResult struct {
	Items []OpsManualView `json:"items"`
	Total int             `json:"total"`
}

type OpsManualRetrieveRequest struct {
	Text           string                   `json:"text,omitempty"`
	OperationFrame opsmanual.OperationFrame `json:"operation_frame,omitempty"`
	Metadata       map[string]any           `json:"metadata,omitempty"`
}

type OpsManualMatchList struct {
	OperationFrame opsmanual.OperationFrame `json:"operation_frame"`
	Matches        []opsmanual.ManualMatch  `json:"matches"`
}

type OpsManualCandidateListResult struct {
	Items []OpsManualCandidateView `json:"items"`
	Total int                      `json:"total"`
}

type OpsManualRunRecordsRequest struct {
	ManualID   string `json:"manual_id,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type OpsManualRunRecordList struct {
	Items []opsmanual.RunRecord `json:"items"`
	Total int                   `json:"total"`
}

type OpsManualPrepareCandidateRequest struct {
	SourceType       string              `json:"source_type"`
	SourceRefs       []string            `json:"source_refs,omitempty"`
	Manual           opsmanual.OpsManual `json:"manual"`
	ValidationReport []string            `json:"validation_report,omitempty"`
}

type OpsManualReviewRequest struct {
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewNote string `json:"review_note,omitempty"`
}

type OpsManualService interface {
	ListManuals(OpsManualListRequest) (OpsManualListResult, error)
	GetManual(id string) (OpsManualView, error)
	SearchOpsManuals(opsmanual.SearchOpsManualsRequest) (opsmanual.SearchOpsManualsResult, error)
	RetrieveManuals(OpsManualRetrieveRequest) (OpsManualMatchList, error)
	ListCandidates() (OpsManualCandidateListResult, error)
	ListRunRecords(OpsManualRunRecordsRequest) (OpsManualRunRecordList, error)
	PrepareManualCandidate(OpsManualPrepareCandidateRequest) (OpsManualCandidateView, error)
	ConfirmManualCandidate(id string, req OpsManualReviewRequest) (OpsManualView, error)
}

type OpsManualDomainProvider interface {
	OpsManualDomainService() *opsmanual.Service
}

type defaultOpsManualService struct {
	domain *opsmanual.Service
}

func NewOpsManualService(domain *opsmanual.Service) OpsManualService {
	return &defaultOpsManualService{domain: domain}
}

func (s *defaultOpsManualService) ListManuals(req OpsManualListRequest) (OpsManualListResult, error) {
	if s.domain == nil {
		return OpsManualListResult{}, fmt.Errorf("ops manual service is not configured")
	}
	items, err := s.domain.ListManuals(opsmanual.ListManualsRequest{
		Status:           req.Status,
		TargetType:       strings.TrimSpace(req.TargetType),
		Action:           strings.TrimSpace(req.Action),
		Middleware:       strings.TrimSpace(req.Middleware),
		ExecutionSurface: strings.TrimSpace(req.ExecutionSurface),
		Limit:            req.Limit,
	})
	if err != nil {
		return OpsManualListResult{}, err
	}
	return OpsManualListResult{Items: items, Total: len(items)}, nil
}

func (s *defaultOpsManualService) GetManual(id string) (OpsManualView, error) {
	if s.domain == nil {
		return OpsManualView{}, fmt.Errorf("ops manual service is not configured")
	}
	return s.domain.GetManual(id)
}

func (s *defaultOpsManualService) SearchOpsManuals(req opsmanual.SearchOpsManualsRequest) (opsmanual.SearchOpsManualsResult, error) {
	if s.domain == nil {
		return opsmanual.SearchOpsManualsResult{}, fmt.Errorf("ops manual service is not configured")
	}
	return s.domain.SearchOpsManuals(req)
}

func (s *defaultOpsManualService) RetrieveManuals(req OpsManualRetrieveRequest) (OpsManualMatchList, error) {
	if s.domain == nil {
		return OpsManualMatchList{}, fmt.Errorf("ops manual service is not configured")
	}
	frame := req.OperationFrame
	if frame.Target.Type == "" && frame.Operation.Action == "" {
		frame = opsmanual.BuildOperationFrame(req.Text, req.Metadata)
	}
	result, err := s.domain.SearchOpsManuals(opsmanual.SearchOpsManualsRequest{
		Text:           req.Text,
		OperationFrame: frame,
		Metadata:       req.Metadata,
	})
	if err != nil {
		return OpsManualMatchList{}, err
	}
	matches := make([]opsmanual.ManualMatch, 0, len(result.Manuals))
	for _, hit := range result.Manuals {
		actions := []string{}
		if hit.RecommendedAction != "" {
			actions = append(actions, hit.RecommendedAction)
		}
		matches = append(matches, opsmanual.ManualMatch{
			Manual:                 hit.Manual,
			State:                  hit.UsableMode,
			Reasons:                hit.BlockedReasons,
			MissingContext:         hit.MissingFields,
			CompatibilityGaps:      hit.EnvironmentDiffs,
			RecommendedNextActions: legacyOpsManualActions(hit.UsableMode, actions),
			RunRecordSummary:       hit.RunRecordSummary,
		})
	}
	return OpsManualMatchList{OperationFrame: result.OperationFrame, Matches: matches}, nil
}

func legacyOpsManualActions(state opsmanual.DecisionState, fallback []string) []string {
	switch state {
	case opsmanual.DecisionDirectExecute:
		return []string{"fill_parameters", "run_precheck", "start_dry_run"}
	case opsmanual.DecisionAdapt:
		return []string{"review_manual", "adapt_workflow"}
	case opsmanual.DecisionNeedInfo:
		return []string{"collect_context", "review_manual"}
	case opsmanual.DecisionReference:
		return []string{"review_manual", "step_by_step"}
	default:
		return fallback
	}
}

func (s *defaultOpsManualService) ListCandidates() (OpsManualCandidateListResult, error) {
	if s.domain == nil {
		return OpsManualCandidateListResult{}, fmt.Errorf("ops manual service is not configured")
	}
	items, err := s.domain.ListCandidates()
	if err != nil {
		return OpsManualCandidateListResult{}, err
	}
	return OpsManualCandidateListResult{Items: items, Total: len(items)}, nil
}

func (s *defaultOpsManualService) ListRunRecords(req OpsManualRunRecordsRequest) (OpsManualRunRecordList, error) {
	if s.domain == nil {
		return OpsManualRunRecordList{}, fmt.Errorf("ops manual service is not configured")
	}
	records, err := s.domain.ListRunRecords(opsmanual.ListRunRecordsRequest{
		ManualID:   strings.TrimSpace(req.ManualID),
		WorkflowID: strings.TrimSpace(req.WorkflowID),
		Limit:      req.Limit,
	})
	if err != nil {
		return OpsManualRunRecordList{}, err
	}
	return OpsManualRunRecordList{Items: records, Total: len(records)}, nil
}

func (s *defaultOpsManualService) PrepareManualCandidate(req OpsManualPrepareCandidateRequest) (OpsManualCandidateView, error) {
	if s.domain == nil {
		return OpsManualCandidateView{}, fmt.Errorf("ops manual service is not configured")
	}
	return s.domain.PrepareManualCandidate(opsmanual.PrepareManualCandidateRequest{
		SourceType:       req.SourceType,
		SourceRefs:       req.SourceRefs,
		Manual:           req.Manual,
		ValidationReport: req.ValidationReport,
	})
}

func (s *defaultOpsManualService) ConfirmManualCandidate(id string, req OpsManualReviewRequest) (OpsManualView, error) {
	if s.domain == nil {
		return OpsManualView{}, fmt.Errorf("ops manual service is not configured")
	}
	return s.domain.ConfirmManualCandidate(id, opsmanual.ConfirmManualCandidateRequest{
		Reviewer:   req.Reviewer,
		ReviewNote: req.ReviewNote,
	})
}

func (s *defaultOpsManualService) OpsManualDomainService() *opsmanual.Service {
	if s == nil {
		return nil
	}
	return s.domain
}
