package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

type sessionAPITestRuntime struct{}

func (sessionAPITestRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}
func (sessionAPITestRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}
func (sessionAPITestRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestSessionAPI_ListCreateAndActivate(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/sessions")
	if err != nil {
		t.Fatalf("GET /api/v1/sessions error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/sessions status = %d, want 200", resp.StatusCode)
	}
	var listed appui.SessionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode session list: %v", err)
	}
	if len(listed.Sessions) != 0 {
		t.Fatalf("initial sessions = %+v, want empty", listed.Sessions)
	}

	body, _ := json.Marshal(map[string]string{"kind": "workspace"})
	createResp, err := http.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/sessions error = %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/sessions status = %d, want 200", createResp.StatusCode)
	}
	var created struct {
		ActiveSessionID string                 `json:"activeSessionId"`
		Sessions        []appui.SessionSummary `json:"sessions"`
		Snapshot        appui.StateSnapshot    `json:"snapshot"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created session response: %v", err)
	}
	if created.ActiveSessionID == "" {
		t.Fatal("created activeSessionId is empty")
	}
	if created.Snapshot.Kind != "workspace" {
		t.Fatalf("created snapshot kind = %q, want workspace", created.Snapshot.Kind)
	}
	if len(created.Sessions) != 1 || created.Sessions[0].ID != created.ActiveSessionID {
		t.Fatalf("created sessions = %+v, want active workspace session", created.Sessions)
	}

	hostBody, _ := json.Marshal(map[string]string{"kind": "single_host"})
	hostResp, err := http.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(hostBody))
	if err != nil {
		t.Fatalf("POST /api/v1/sessions (host) error = %v", err)
	}
	defer hostResp.Body.Close()
	var hostCreated struct {
		ActiveSessionID string                 `json:"activeSessionId"`
		Sessions        []appui.SessionSummary `json:"sessions"`
		Snapshot        appui.StateSnapshot    `json:"snapshot"`
	}
	if err := json.NewDecoder(hostResp.Body).Decode(&hostCreated); err != nil {
		t.Fatalf("decode host session response: %v", err)
	}
	if hostCreated.Snapshot.Kind != "single_host" {
		t.Fatalf("host snapshot kind = %q, want single_host", hostCreated.Snapshot.Kind)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/sessions/"+created.ActiveSessionID+"/activate", nil)
	if err != nil {
		t.Fatalf("new activate request: %v", err)
	}
	activateResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/sessions/:id/activate error = %v", err)
	}
	defer activateResp.Body.Close()
	if activateResp.StatusCode != http.StatusOK {
		t.Fatalf("activate status = %d, want 200", activateResp.StatusCode)
	}
	var activated struct {
		ActiveSessionID string                 `json:"activeSessionId"`
		Sessions        []appui.SessionSummary `json:"sessions"`
		Snapshot        appui.StateSnapshot    `json:"snapshot"`
	}
	if err := json.NewDecoder(activateResp.Body).Decode(&activated); err != nil {
		t.Fatalf("decode activated session response: %v", err)
	}
	if activated.ActiveSessionID != created.ActiveSessionID {
		t.Fatalf("activated activeSessionId = %q, want %q", activated.ActiveSessionID, created.ActiveSessionID)
	}
	if activated.Snapshot.SessionID != created.ActiveSessionID {
		t.Fatalf("activated snapshot.sessionId = %q, want %q", activated.Snapshot.SessionID, created.ActiveSessionID)
	}
	if len(activated.Sessions) < 2 || activated.Sessions[0].ID != created.ActiveSessionID {
		t.Fatalf("activated sessions ordering = %+v, want activated session first", activated.Sessions)
	}
}
