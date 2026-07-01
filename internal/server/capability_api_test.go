package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

func TestCapabilityAPIListCapabilities(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{{ID: "ops-triage", Name: "Ops Triage", DefaultEnabled: true}},
		mcpCatalog:   []store.AgentMCPCatalogEntry{{ID: "warehouse", Name: "Warehouse", Type: "http", DefaultEnabled: true, Permission: "readonly"}},
	}
	services := appui.NewServices(websocketAPITestRuntime{}, nil,
		appui.WithSkillCatalogRepository(repo),
		appui.WithAgentMCPCatalogRepository(repo),
	)
	server := NewHTTPServer(services, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/capabilities")
	if err != nil {
		t.Fatalf("GET /api/v1/capabilities error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/capabilities status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Items []appui.CapabilityRecord `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if got := capabilityAPIRecordByID(payload.Items, "warehouse"); got == nil || got.Kind != "capability" || got.Category != "data" || got.Facets.Connection == nil {
		t.Fatalf("warehouse record = %+v, want data capability with connection facet", got)
	}
	if got := capabilityAPIRecordByID(payload.Items, "ops-triage"); got == nil || got.Category != "skill" {
		t.Fatalf("ops-triage record = %+v, want skill capability", got)
	}
}

func capabilityAPIRecordByID(items []appui.CapabilityRecord, id string) *appui.CapabilityRecord {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
