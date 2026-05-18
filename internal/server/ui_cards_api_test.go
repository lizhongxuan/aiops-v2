package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestUICardsAPIListCRUDAndSubresources(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/ui-cards")
	if err != nil {
		t.Fatalf("GET /api/v1/ui-cards error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ui-cards status = %d, want 200", resp.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(asSlice(list["items"])) == 0 || len(asSlice(list["cards"])) == 0 || int(list["total"].(float64)) == 0 {
		t.Fatalf("list payload = %#v, want items/cards/total", list)
	}
	items := asSlice(list["items"])
	first := items[0].(map[string]any)
	builtInID := first["id"].(string)

	createBody, _ := json.Marshal(map[string]any{
		"id":              "custom-api-card",
		"name":            "Custom API Card",
		"kind":            "timeline",
		"renderer":        "agent-ui/timeline",
		"rendererVersion": "0.1.0",
		"schemaVersion":   "2026-05-16",
		"payloadSchema":   map[string]any{"type": "object"},
		"actionPolicy":    map[string]any{"allowed": []string{"inspect"}},
		"displayPolicy":   map[string]any{"density": "compact"},
		"redactionPolicy": map[string]any{"mode": "default"},
		"samplePayloads":  []map[string]any{{"title": "sample"}},
	})
	createResp, err := http.Post(ts.URL+"/api/v1/ui-cards", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/ui-cards error = %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/ui-cards status = %d, want 201", createResp.StatusCode)
	}

	statusBody, _ := json.Marshal(map[string]string{"status": "disabled"})
	statusResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodPut, ts.URL+"/api/v1/ui-cards/custom-api-card/status", statusBody))
	if err != nil {
		t.Fatalf("PUT status error = %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status status = %d, want 200", statusResp.StatusCode)
	}
	var statusPayload map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&statusPayload); err != nil {
		t.Fatal(err)
	}
	if statusPayload["status"] != "disabled" {
		t.Fatalf("PUT status payload = %#v, want disabled", statusPayload)
	}

	versionResp, err := http.Post(ts.URL+"/api/v1/ui-cards/custom-api-card/versions", "application/json", bytes.NewReader([]byte(`{"reason":"schema update"}`)))
	if err != nil {
		t.Fatalf("POST versions error = %v", err)
	}
	defer versionResp.Body.Close()
	if versionResp.StatusCode != http.StatusOK {
		t.Fatalf("POST versions status = %d, want 200", versionResp.StatusCode)
	}
	var versionPayload map[string]any
	if err := json.NewDecoder(versionResp.Body).Decode(&versionPayload); err != nil {
		t.Fatal(err)
	}
	if versionPayload["version"].(float64) < 2 {
		t.Fatalf("POST versions payload = %#v, want incremented version", versionPayload)
	}

	validateResp, err := http.Post(ts.URL+"/api/v1/ui-cards/custom-api-card/validate", "application/json", bytes.NewReader([]byte(`{"payload":{"title":"ok"}}`)))
	if err != nil {
		t.Fatalf("POST validate error = %v", err)
	}
	defer validateResp.Body.Close()
	if validateResp.StatusCode != http.StatusOK {
		t.Fatalf("validate status = %d, want 200", validateResp.StatusCode)
	}
	previewResp, err := http.Post(ts.URL+"/api/v1/ui-cards/custom-api-card/preview", "application/json", bytes.NewReader([]byte(`{"payload":{"title":"ok"}}`)))
	if err != nil {
		t.Fatalf("POST preview error = %v", err)
	}
	defer previewResp.Body.Close()
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", previewResp.StatusCode)
	}

	deleteBuiltIn, err := http.DefaultClient.Do(mustRequest(t, http.MethodDelete, ts.URL+"/api/v1/ui-cards/"+builtInID, nil))
	if err != nil {
		t.Fatalf("DELETE built-in error = %v", err)
	}
	defer deleteBuiltIn.Body.Close()
	if deleteBuiltIn.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE built-in status = %d, want 409", deleteBuiltIn.StatusCode)
	}
}
