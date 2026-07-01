package appui

import (
	"context"

	"aiops-v2/internal/incidents"
)

type IncidentView = incidents.IncidentCase
type EvidenceRefView = incidents.EvidenceRef
type HypothesisView = incidents.Hypothesis
type PostmortemDraftView = incidents.PostmortemDraft

type IncidentCreateCommand = incidents.CreateRequest
type IncidentUpdateCommand = incidents.UpdateRequest
type IncidentCloseCommand = incidents.CloseRequest

type IncidentService interface {
	Create(ctx context.Context, cmd IncidentCreateCommand) (IncidentView, error)
	Get(ctx context.Context, id string) (IncidentView, bool)
	List(ctx context.Context) ([]IncidentView, error)
	Update(ctx context.Context, id string, cmd IncidentUpdateCommand) (IncidentView, error)
	AddEvidence(ctx context.Context, incidentID string, evidence EvidenceRefView) (EvidenceRefView, error)
	RankHypotheses(ctx context.Context, incidentID string, hypotheses []HypothesisView) ([]HypothesisView, error)
	Close(ctx context.Context, id string, cmd IncidentCloseCommand) (IncidentView, error)
}

type defaultIncidentService struct {
	domain *incidents.Service
}

func NewIncidentService(domain *incidents.Service) IncidentService {
	if domain == nil {
		domain = incidents.NewService(nil, nil)
	}
	return &defaultIncidentService{domain: domain}
}

func (s *defaultIncidentService) Create(_ context.Context, cmd IncidentCreateCommand) (IncidentView, error) {
	return s.domain.Create(cmd)
}

func (s *defaultIncidentService) Get(_ context.Context, id string) (IncidentView, bool) {
	incident, ok := s.domain.Get(id)
	if !ok {
		return IncidentView{}, false
	}
	incident.Evidence = s.domain.Evidence(id)
	return incident, true
}

func (s *defaultIncidentService) List(context.Context) ([]IncidentView, error) {
	return s.domain.List(), nil
}

func (s *defaultIncidentService) Update(_ context.Context, id string, cmd IncidentUpdateCommand) (IncidentView, error) {
	return s.domain.Update(id, cmd)
}

func (s *defaultIncidentService) AddEvidence(_ context.Context, incidentID string, evidence EvidenceRefView) (EvidenceRefView, error) {
	return s.domain.AddEvidence(incidentID, evidence)
}

func (s *defaultIncidentService) RankHypotheses(_ context.Context, incidentID string, hypotheses []HypothesisView) ([]HypothesisView, error) {
	return s.domain.RankHypotheses(incidentID, hypotheses)
}

func (s *defaultIncidentService) Close(_ context.Context, id string, cmd IncidentCloseCommand) (IncidentView, error) {
	return s.domain.Close(id, cmd)
}
