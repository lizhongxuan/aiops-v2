package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestAgentUIArtifactsAPIListGetValidate(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/agent-ui-artifacts?caseId=case-demo&type=ops_manual_search_result&limit=1")
	if err != nil {
		t.Fatalf("GET /api/v1/agent-ui-artifacts error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	items := asSlice(list["items"])
	if len(items) != 1 || int(list["total"].(float64)) == 0 {
		t.Fatalf("list payload = %#v, want one filtered item", list)
	}
	id := items[0].(map[string]any)["id"].(string)

	getResp, err := http.Get(ts.URL + "/api/v1/agent-ui-artifacts/" + id)
	if err != nil {
		t.Fatalf("GET artifact error = %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}

	validateBody, _ := json.Marshal(map[string]any{"artifactId": id})
	validateResp, err := http.Post(ts.URL+"/api/v1/agent-ui-artifacts/validate", "application/json", bytes.NewReader(validateBody))
	if err != nil {
		t.Fatalf("POST validate error = %v", err)
	}
	defer validateResp.Body.Close()
	if validateResp.StatusCode != http.StatusOK {
		t.Fatalf("validate status = %d, want 200", validateResp.StatusCode)
	}
	var validation map[string]any
	if err := json.NewDecoder(validateResp.Body).Decode(&validation); err != nil {
		t.Fatal(err)
	}
	if validation["valid"] != true {
		t.Fatalf("validation = %#v, want valid", validation)
	}
}
