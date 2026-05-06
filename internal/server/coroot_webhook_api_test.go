package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestCorootWebhookAPICreatesIncidentOnly(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := []byte(`{
		"event":"alert",
		"project":"prod",
		"environment":"prod",
		"alert":{"name":"order-api SLO burn","severity":"critical"},
		"application":{"name":"order-api"},
		"incident":{"id":"coroot-inc-1"}
	}`)
	resp, err := http.Post(ts.URL+"/api/v1/coroot/webhook", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST webhook error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var payload struct {
		Incident           appui.IncidentView `json:"incident"`
		StartedRuntimeTurn bool               `json:"startedRuntimeTurn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Incident.ID == "" || len(payload.Incident.EvidenceRefs) != 1 {
		t.Fatalf("payload = %#v, want incident with evidence", payload)
	}
	if payload.StartedRuntimeTurn {
		t.Fatalf("StartedRuntimeTurn = true, webhook handler must not execute runtime work")
	}
}
