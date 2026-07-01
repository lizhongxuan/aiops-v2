package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
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

func TestCorootWebhookIncidentStartChatStartsReadonlyChatOnUserAction(t *testing.T) {
	runtime := &interactionAPITestRuntime{runCh: make(chan runtimekernel.TurnRequest, 1)}
	sessions := runtimekernel.NewSessionManager()
	srv := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := []byte(`{
		"event":"alert",
		"project":"prod",
		"environment":"prod",
		"alert":{"name":"order-api SLO burn","severity":"critical"},
		"application":{"name":"order-api"},
		"incident":{"id":"coroot-inc-start-chat"}
	}`)
	resp, err := http.Post(ts.URL+"/api/v1/coroot/webhook", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST webhook error = %v", err)
	}
	defer resp.Body.Close()
	var created struct {
		Incident appui.IncidentView `json:"incident"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode webhook response: %v", err)
	}
	if created.Incident.ID == "" {
		t.Fatalf("webhook response missing incident: %#v", created)
	}

	getResp, err := http.Get(ts.URL + "/api/v1/incidents/" + created.Incident.ID)
	if err != nil {
		t.Fatalf("GET incident error = %v", err)
	}
	defer getResp.Body.Close()
	var detail struct {
		Incident appui.IncidentView `json:"incident"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode incident detail: %v", err)
	}
	if len(detail.Incident.Evidence) != 1 || detail.Incident.Evidence[0].RawRef == "" {
		t.Fatalf("incident evidence = %#v, want raw webhook evidence detail", detail.Incident.Evidence)
	}

	startResp, err := http.Post(ts.URL+"/api/v1/incidents/"+created.Incident.ID+"/start-chat", "application/json", nil)
	if err != nil {
		t.Fatalf("POST start-chat error = %v", err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("start-chat status = %d, want 200", startResp.StatusCode)
	}
	var started struct {
		SessionID          string `json:"sessionId"`
		TurnID             string `json:"turnId"`
		Prompt             string `json:"prompt"`
		StartedRuntimeTurn bool   `json:"startedRuntimeTurn"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&started); err != nil {
		t.Fatalf("decode start-chat response: %v", err)
	}
	if !started.StartedRuntimeTurn || started.SessionID == "" || started.TurnID == "" {
		t.Fatalf("start-chat response = %#v, want runtime turn started by user action", started)
	}
	req := waitForAPIRunTurn(t, runtime)
	if !strings.Contains(req.Input, "只读排查") || strings.Contains(req.Input, "@Coroot") {
		t.Fatalf("start-chat input = %q, want readonly prompt without literal @Coroot", req.Input)
	}
	if req.Metadata["aiops.coroot.explicitRCA"] == "true" || req.Metadata["aiops.coroot.rcaDisplayAllowed"] == "true" {
		t.Fatalf("metadata = %#v, start-chat must not auto-enable RCA", req.Metadata)
	}
	if req.Metadata["aiops.coroot.webhookIncidentId"] != created.Incident.ID {
		t.Fatalf("metadata = %#v, want webhook incident id", req.Metadata)
	}
}
