package coroot

import "encoding/json"

type WebhookEvent struct {
	Event       string            `json:"event,omitempty"`
	Type        string            `json:"type,omitempty"`
	Project     string            `json:"project,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Alert       WebhookAlert      `json:"alert,omitempty"`
	Incident    WebhookIncident   `json:"incident,omitempty"`
	Application WebhookApp        `json:"application,omitempty"`
	Service     string            `json:"service,omitempty"`
	Deployment  WebhookDeployment `json:"deployment,omitempty"`
	Raw         json.RawMessage   `json:"-"`
}

type WebhookAlert struct {
	Name     string `json:"name,omitempty"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status,omitempty"`
}

type WebhookIncident struct {
	ID       string `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type WebhookApp struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type WebhookDeployment struct {
	ID      string `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
	Service string `json:"service,omitempty"`
}
