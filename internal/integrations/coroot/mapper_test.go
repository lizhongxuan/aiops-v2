package coroot

import (
	"encoding/json"
	"testing"
)

func TestDecodeWebhookMapsAlertIncidentDeployment(t *testing.T) {
	payload := json.RawMessage(`{
		"event":"alert",
		"project":"prod",
		"environment":"prod",
		"alert":{"name":"order-api SLO burn","severity":"critical"},
		"application":{"name":"order-api"},
		"incident":{"id":"coroot-inc-1","url":"https://coroot.example/incidents/coroot-inc-1"},
		"deployment":{"version":"2026.05.04-1"}
	}`)

	event, err := DecodeWebhook(payload)
	if err != nil {
		t.Fatalf("DecodeWebhook() error = %v", err)
	}
	mapped, err := MapWebhookToIncident(event, payload)
	if err != nil {
		t.Fatalf("MapWebhookToIncident() error = %v", err)
	}
	if mapped.Incident.Title != "order-api SLO burn" {
		t.Fatalf("title = %q", mapped.Incident.Title)
	}
	if mapped.Incident.Source != "coroot" || mapped.Incident.Environment != "prod" {
		t.Fatalf("incident = %#v", mapped.Incident)
	}
	if len(mapped.Evidence) != 1 || mapped.Evidence[0].RawRef == "" || mapped.Evidence[0].Summary == "" {
		t.Fatalf("evidence = %#v, want raw webhook evidence", mapped.Evidence)
	}
	if mapped.Incident.ExternalID != "coroot-inc-1" {
		t.Fatalf("external id = %q", mapped.Incident.ExternalID)
	}
}
