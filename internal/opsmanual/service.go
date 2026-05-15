package opsmanual

import (
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo ManualRepository
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

func NewService(repo ManualRepository) *Service {
	return &Service{repo: repo}
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
	return SearchOpsManuals(s.repo, req)
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
	if strings.TrimSpace(manual.Operation.TargetType) == "" || strings.TrimSpace(manual.Operation.Action) == "" {
		return fmt.Errorf("verified ops manual requires target type and action")
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
