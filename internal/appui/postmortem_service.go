package appui

import (
	"context"
	"fmt"
)

type PostmortemService interface {
	Draft(ctx context.Context, incidentID string) (PostmortemDraftView, error)
}

type defaultPostmortemService struct {
	incidents IncidentService
}

func NewPostmortemService(incidents IncidentService) PostmortemService {
	return &defaultPostmortemService{incidents: incidents}
}

func (s *defaultPostmortemService) Draft(ctx context.Context, incidentID string) (PostmortemDraftView, error) {
	incident, ok := s.incidents.Get(ctx, incidentID)
	if !ok {
		return PostmortemDraftView{}, fmt.Errorf("incident %q not found", incidentID)
	}
	if incident.Postmortem == nil {
		return PostmortemDraftView{Impact: incident.BusinessCapability, RootCause: "", Mitigation: ""}, nil
	}
	return *incident.Postmortem, nil
}
