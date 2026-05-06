package coroot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/incidents"
)

type MappedWebhookIncident struct {
	Incident incidents.CreateRequest `json:"incident"`
	Evidence []incidents.EvidenceRef `json:"evidence"`
}

func DecodeWebhook(payload json.RawMessage) (WebhookEvent, error) {
	if len(payload) == 0 {
		return WebhookEvent{}, fmt.Errorf("webhook payload is required")
	}
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return WebhookEvent{}, err
	}
	event.Raw = append(json.RawMessage(nil), payload...)
	if event.Event == "" {
		event.Event = event.Type
	}
	return event, nil
}

func MapWebhookToIncident(event WebhookEvent, raw json.RawMessage) (MappedWebhookIncident, error) {
	service := firstWebhookText(event.Application.Name, event.Service, event.Deployment.Service)
	title := firstWebhookText(event.Incident.Title, event.Alert.Name, service, event.Event)
	if title == "" {
		return MappedWebhookIncident{}, fmt.Errorf("unable to derive incident title from Coroot webhook")
	}
	environment := firstWebhookText(event.Environment, event.Project)
	severity := firstWebhookText(event.Incident.Severity, event.Alert.Severity)
	rawRef := corootWebhookRawRef(raw)
	summary := webhookSummary(event, title, service)
	return MappedWebhookIncident{
		Incident: incidents.CreateRequest{
			ExternalID:       event.Incident.ID,
			Title:            title,
			Severity:         severity,
			Source:           "coroot",
			Environment:      environment,
			AffectedServices: []string{service},
		},
		Evidence: []incidents.EvidenceRef{{
			Source:     "coroot",
			RawRef:     rawRef,
			Summary:    summary,
			Confidence: "high",
			EntityID:   firstWebhookText(event.Application.ID, service),
		}},
	}, nil
}

func webhookSummary(event WebhookEvent, title, service string) string {
	parts := []string{"Coroot " + firstWebhookText(event.Event, "webhook")}
	if title != "" {
		parts = append(parts, title)
	}
	if service != "" {
		parts = append(parts, "service="+service)
	}
	if event.Deployment.Version != "" {
		parts = append(parts, "deployment="+event.Deployment.Version)
	}
	return strings.Join(parts, " · ")
}

func corootWebhookRawRef(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return "coroot:webhook:" + hex.EncodeToString(sum[:8])
}

func firstWebhookText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
