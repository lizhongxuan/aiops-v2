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
		Provider:         "openai",
		Model:            "gpt-4o",
		APIKey:           "sk-test-12345678",
		MaxContextTokens: 131072,
		CompactModel:     "gpt-4o-mini",
		ReasoningEffort:  "high",
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
	if llm["maxContextTokens"] != float64(131072) {
		t.Fatalf("llm response = %+v, want maxContextTokens 131072", llm)
	}
	if llm["reasoningEffort"] != "high" {
		t.Fatalf("llm response = %+v, want reasoningEffort high", llm)
	}

	updateLLMBody, _ := json.Marshal(map[string]any{
		"provider":         "openai",
		"model":            "gpt-5.4",
		"maxContextTokens": 9000,
		"reasoningEffort":  "low",
	})
	updateLLMReq, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/llm-config", bytes.NewReader(updateLLMBody))
	if err != nil {
		t.Fatalf("new llm update request: %v", err)
	}
	updateLLMReq.Header.Set("Content-Type", "application/json")
	updateLLMResp, err := http.DefaultClient.Do(updateLLMReq)
	if err != nil {
		t.Fatalf("PUT /api/v1/llm-config error = %v", err)
	}
	defer updateLLMResp.Body.Close()
	if updateLLMResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/llm-config status = %d, want 200", updateLLMResp.StatusCode)
	}
	var updateLLM map[string]any
	if err := json.NewDecoder(updateLLMResp.Body).Decode(&updateLLM); err != nil {
		t.Fatalf("decode llm update response error = %v", err)
	}
	if updateLLM["maxContextTokens"] != float64(10000) {
		t.Fatalf("llm update response = %+v, want maxContextTokens 10000", updateLLM)
	}
	storedLLM, err := dataStore.GetLLMConfig()
	if err != nil {
		t.Fatalf("GetLLMConfig() after update error = %v", err)
	}
	if storedLLM.ReasoningEffort != "low" {
		t.Fatalf("stored LLM = %+v, want reasoningEffort low", storedLLM)
	}

	deepSeekBody, _ := json.Marshal(map[string]any{
		"provider":        "deepseek",
		"apiKey":          "sk-deepseek-test",
		"reasoningEffort": "max",
		"thinkingType":    "enabled",
	})
	deepSeekReq, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/llm-config", bytes.NewReader(deepSeekBody))
	if err != nil {
		t.Fatalf("new deepseek update request: %v", err)
	}
	deepSeekReq.Header.Set("Content-Type", "application/json")
	deepSeekResp, err := http.DefaultClient.Do(deepSeekReq)
	if err != nil {
		t.Fatalf("PUT deepseek /api/v1/llm-config error = %v", err)
	}
	defer deepSeekResp.Body.Close()
	if deepSeekResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT deepseek status = %d, want 200", deepSeekResp.StatusCode)
	}
	var deepSeekResult map[string]any
	if err := json.NewDecoder(deepSeekResp.Body).Decode(&deepSeekResult); err != nil {
		t.Fatalf("decode deepseek update response error = %v", err)
	}
	if deepSeekResult["maxContextTokens"] != float64(1000000) || deepSeekResult["maxOutputTokens"] != float64(20000) {
		t.Fatalf("deepseek result = %+v, want context/output 1000000/20000", deepSeekResult)
	}
	storedLLM, err = dataStore.GetLLMConfig()
	if err != nil {
		t.Fatalf("GetLLMConfig() after deepseek update error = %v", err)
	}
	if storedLLM.Provider != "deepseek" || storedLLM.Model != "deepseek-v4-pro" || storedLLM.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("stored deepseek LLM = %+v, want official defaults", storedLLM)
	}
	if storedLLM.ReasoningEffort != "max" || storedLLM.ThinkingType != "enabled" {
		t.Fatalf("stored deepseek reasoning/thinking = %q/%q, want max/enabled", storedLLM.ReasoningEffort, storedLLM.ThinkingType)
	}

	zhipuBody, _ := json.Marshal(map[string]any{
		"provider":        "zhipu",
		"apiKey":          "zai-test-key",
		"reasoningEffort": "xhigh",
		"thinkingType":    "disabled",
		"toolStream":      true,
	})
	zhipuReq, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/llm-config", bytes.NewReader(zhipuBody))
	if err != nil {
		t.Fatalf("new zhipu update request: %v", err)
	}
	zhipuReq.Header.Set("Content-Type", "application/json")
	zhipuResp, err := http.DefaultClient.Do(zhipuReq)
	if err != nil {
		t.Fatalf("PUT zhipu /api/v1/llm-config error = %v", err)
	}
	defer zhipuResp.Body.Close()
	if zhipuResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT zhipu status = %d, want 200", zhipuResp.StatusCode)
	}
	storedLLM, err = dataStore.GetLLMConfig()
	if err != nil {
		t.Fatalf("GetLLMConfig() after zhipu update error = %v", err)
	}
	if storedLLM.Provider != "zhipu" || storedLLM.Model != "glm-5.2" || storedLLM.BaseURL != "https://open.bigmodel.cn/api/paas/v4/" {
		t.Fatalf("stored zhipu LLM = %+v, want official defaults", storedLLM)
	}
	if storedLLM.ReasoningEffort != "xhigh" || storedLLM.ThinkingType != "disabled" || !storedLLM.ToolStream {
		t.Fatalf("stored zhipu reasoning/thinking/toolStream = %q/%q/%v, want xhigh/disabled/true", storedLLM.ReasoningEffort, storedLLM.ThinkingType, storedLLM.ToolStream)
	}
}

func TestRuntimeSettingsAPI(t *testing.T) {
	dataStore, err := store.NewJSONFileStore(t.TempDir(), 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()

	sessionMgr := runtimekernel.NewSessionManager()
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/runtime-settings")
	if err != nil {
		t.Fatalf("GET /api/v1/runtime-settings error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/runtime-settings status = %d, want 200", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode runtime settings response: %v", err)
	}
	settings, ok := payload["settings"].(map[string]any)
	if !ok {
		t.Fatalf("runtime settings response = %+v, want settings object", payload)
	}
	if payload["defaults"] == nil {
		t.Fatalf("runtime settings response = %+v, want defaults", payload)
	}
	agentRuntime, _ := settings["agentRuntime"].(map[string]any)
	if agentRuntime["intentFrameRouting"] != "trace_only" {
		t.Fatalf("agentRuntime = %+v, want trace_only", agentRuntime)
	}

	updateBody, _ := json.Marshal(map[string]any{
		"workflow": map[string]any{
			"validationProvider": "docker",
			"validationImage":    "python:3.12-bookworm",
		},
		"debug": map[string]any{
			"modelInputTrace": false,
		},
	})
	req, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/runtime-settings", bytes.NewReader(updateBody))
	if err != nil {
		t.Fatalf("new PATCH runtime settings request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /api/v1/runtime-settings error = %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/v1/runtime-settings status = %d, want 200", updateResp.StatusCode)
	}
	var updated map[string]any
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode runtime settings update response: %v", err)
	}
	updatedSettings := updated["settings"].(map[string]any)
	workflow := updatedSettings["workflow"].(map[string]any)
	if workflow["validationProvider"] != "docker" || workflow["validationImage"] != "python:3.12-bookworm" {
		t.Fatalf("workflow = %+v, want docker/bookworm", workflow)
	}
	debug := updatedSettings["debug"].(map[string]any)
	if debug["modelInputTrace"] != false {
		t.Fatalf("debug = %+v, want modelInputTrace false", debug)
	}
	if updated["updatedAt"] == "" {
		t.Fatalf("updated payload = %+v, want updatedAt", updated)
	}

	badReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/runtime-settings", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatalf("new bad PATCH request: %v", err)
	}
	badReq.Header.Set("Content-Type", "application/json")
	badResp, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatalf("bad PATCH /api/v1/runtime-settings error = %v", err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad PATCH status = %d, want 400", badResp.StatusCode)
	}

	postResp, err := http.Post(ts.URL+"/api/v1/runtime-settings", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST /api/v1/runtime-settings error = %v", err)
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/runtime-settings status = %d, want 405", postResp.StatusCode)
	}
}
