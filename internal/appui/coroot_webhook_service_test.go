package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/incidents"
)

func TestCorootWebhookServiceCreatesIncidentWithoutRuntimeExecution(t *testing.T) {
	incidentService := NewIncidentService(incidents.NewService(incidents.NewInMemoryStore(), nil))
	service := NewCorootWebhookService(incidentService)

	result, err := service.Handle(context.Background(), CorootWebhookCommand{
		Payload: []byte(`{
			"event":"incident",
			"project":"prod",
			"environment":"prod",
			"incident":{"id":"coroot-inc-1","title":"order-api latency spike"},
			"application":{"name":"order-api"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Incident.ID == "" || len(result.Incident.EvidenceRefs) != 1 {
		t.Fatalf("result = %#v, want incident with raw evidence", result)
	}
	if result.StartedRuntimeTurn {
		t.Fatalf("StartedRuntimeTurn = true, webhook must only create incident/evidence")
	}
}
