package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestExperiencePackAPIListCandidatesDoesNot404(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/experience-packs/candidates?limit=100")
	if err != nil {
		t.Fatalf("get candidates: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Items == nil {
		t.Fatal("items should be an empty array when no persisted packs exist")
	}
}

func TestExperiencePackAPIListReuseRecordsDoesNot404(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/experience-packs/pack-pg/reuse-records?limit=20")
	if err != nil {
		t.Fatalf("get reuse records: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Items == nil {
		t.Fatal("items should be an empty array when no reuse records exist")
	}
}
