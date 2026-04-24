package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func TestHostAPI_CRUDSessionsAndSelect(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()

	sessionMgr := runtimekernel.NewSessionManager(dataStore)
	session := sessionMgr.GetOrCreate("sess-host", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.Messages = []runtimekernel.Message{
		{ID: "u-1", Role: "user", Content: "检查服务", Timestamp: time.Now().UTC()},
		{ID: "a-1", Role: "assistant", Content: "服务正常", Timestamp: time.Now().UTC()},
	}
	sessionMgr.Update(session)

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
	var hostSessions struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(hostSessionsResp.Body).Decode(&hostSessions); err != nil {
		t.Fatalf("decode host sessions response error = %v", err)
	}
	if len(hostSessions.Items) != 1 {
		t.Fatalf("host sessions = %+v, want one session", hostSessions.Items)
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
