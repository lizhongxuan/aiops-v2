package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func TestHostAPI_CRUDRemovedEndpointsAndSelect(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()

	sessionMgr := runtimekernel.NewSessionManager(dataStore)

	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody, _ := json.Marshal(map[string]any{
		"id":            "host-a",
		"name":          "web-01",
		"address":       "10.0.0.11",
		"sshUser":       "ubuntu",
		"sshPort":       22,
		"labels":        map[string]string{"env": "prod"},
		"installViaSsh": true,
	})
	createResp, err := http.Post(ts.URL+"/api/v1/hosts", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/hosts error = %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/hosts status = %d, want 200", createResp.StatusCode)
	}

	hostSessionsResp, err := http.Get(ts.URL + "/api/v1/hosts/host-a/sessions?limit=8")
	if err != nil {
		t.Fatalf("GET /api/v1/hosts/:id/sessions error = %v", err)
	}
	defer hostSessionsResp.Body.Close()
	if hostSessionsResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/v1/hosts/:id/sessions status = %d, want 404", hostSessionsResp.StatusCode)
	}

	tagsResp, err := http.Post(ts.URL+"/api/v1/hosts/tags", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST /api/v1/hosts/tags error = %v", err)
	}
	defer tagsResp.Body.Close()
	if tagsResp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /api/v1/hosts/tags status = %d, want 404", tagsResp.StatusCode)
	}

	selectBody, _ := json.Marshal(map[string]string{"hostId": "host-a"})
	selectResp, err := http.Post(ts.URL+"/api/v1/host/select", "application/json", bytes.NewReader(selectBody))
	if err != nil {
		t.Fatalf("POST /api/v1/host/select error = %v", err)
	}
	defer selectResp.Body.Close()
	if selectResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/host/select status = %d, want 200", selectResp.StatusCode)
	}
	var payload struct {
		Snapshot appui.StateSnapshot `json:"snapshot"`
	}
	if err := json.NewDecoder(selectResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode select host response error = %v", err)
	}
	if payload.Snapshot.SelectedHostID != "host-a" {
		t.Fatalf("snapshot = %+v, want selected host-a", payload.Snapshot)
	}
}
