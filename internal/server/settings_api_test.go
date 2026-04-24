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

func TestSettingsAndLLMConfigAPI(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()

	if err := dataStore.SaveLLMConfig(&store.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-4o",
		APIKey:       "sk-test-12345678",
		CompactModel: "gpt-4o-mini",
	}); err != nil {
		t.Fatalf("SaveLLMConfig() error = %v", err)
	}

	sessionMgr := runtimekernel.NewSessionManager()
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/settings")
	if err != nil {
		t.Fatalf("GET /api/v1/settings error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/settings status = %d, want 200", resp.StatusCode)
	}
	var settings map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode settings response error = %v", err)
	}
	if settings["model"] == "" {
		t.Fatalf("settings response = %+v, want model", settings)
	}

	updateBody, _ := json.Marshal(map[string]any{
		"model":           "claude-3-opus",
		"reasoningEffort": "low",
	})
	updateResp, err := http.Post(ts.URL+"/api/v1/settings", "application/json", bytes.NewReader(updateBody))
	if err != nil {
		t.Fatalf("POST /api/v1/settings error = %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/settings status = %d, want 200", updateResp.StatusCode)
	}

	llmResp, err := http.Get(ts.URL + "/api/v1/llm-config")
	if err != nil {
		t.Fatalf("GET /api/v1/llm-config error = %v", err)
	}
	defer llmResp.Body.Close()
	var llm map[string]any
	if err := json.NewDecoder(llmResp.Body).Decode(&llm); err != nil {
		t.Fatalf("decode llm response error = %v", err)
	}
	if llm["apiKeyMasked"] == "" {
		t.Fatalf("llm response = %+v, want apiKeyMasked", llm)
	}
}
