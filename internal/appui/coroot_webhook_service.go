package appui

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/integrations/coroot"
)

type CorootWebhookCommand struct {
	Payload json.RawMessage `json:"payload"`
}

type CorootWebhookResult struct {
	Incident           IncidentView      `json:"incident"`
	Evidence           []EvidenceRefView `json:"evidence,omitempty"`
	StartedRuntimeTurn bool              `json:"startedRuntimeTurn"`
}

type CorootWebhookService interface {
	Handle(ctx context.Context, cmd CorootWebhookCommand) (CorootWebhookResult, error)
}

type defaultCorootWebhookService struct {
	incidents IncidentService
}

func NewCorootWebhookService(incidents IncidentService) CorootWebhookService {
	return &defaultCorootWebhookService{incidents: incidents}
}

func (s *defaultCorootWebhookService) Handle(ctx context.Context, cmd CorootWebhookCommand) (CorootWebhookResult, error) {
	if len(cmd.Payload) == 0 {
		return CorootWebhookResult{}, fmt.Errorf("payload is required")
	}
	event, err := coroot.DecodeWebhook(cmd.Payload)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	mapped, err := coroot.MapWebhookToIncident(event, cmd.Payload)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	incident, err := s.incidents.Create(ctx, mapped.Incident)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	evidence := make([]EvidenceRefView, 0, len(mapped.Evidence))
	for _, item := range mapped.Evidence {
		created, err := s.incidents.AddEvidence(ctx, incident.ID, item)
		if err != nil {
			return CorootWebhookResult{}, err
		}
		evidence = append(evidence, created)
	}
	incident, _ = s.incidents.Get(ctx, incident.ID)
	return CorootWebhookResult{Incident: incident, Evidence: evidence}, nil
}
