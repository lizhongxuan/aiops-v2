package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

type agentProfileRepoStub struct {
	skillCatalog []store.SkillCatalogEntry
	mcpCatalog   []store.AgentMCPCatalogEntry
	profiles     []store.AgentProfileRecord
}

func (r *agentProfileRepoStub) GetSkillCatalog() ([]store.SkillCatalogEntry, error) {
	return append([]store.SkillCatalogEntry(nil), r.skillCatalog...), nil
}
func (r *agentProfileRepoStub) SaveSkillCatalog(items []store.SkillCatalogEntry) error {
	r.skillCatalog = append([]store.SkillCatalogEntry(nil), items...)
	return nil
}
func (r *agentProfileRepoStub) GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error) {
	return append([]store.AgentMCPCatalogEntry(nil), r.mcpCatalog...), nil
}
func (r *agentProfileRepoStub) SaveAgentMCPCatalog(items []store.AgentMCPCatalogEntry) error {
	r.mcpCatalog = append([]store.AgentMCPCatalogEntry(nil), items...)
	return nil
}
func (r *agentProfileRepoStub) GetAgentProfiles() ([]store.AgentProfileRecord, error) {
	return append([]store.AgentProfileRecord(nil), r.profiles...), nil
}
func (r *agentProfileRepoStub) SaveAgentProfiles(items []store.AgentProfileRecord) error {
	r.profiles = append([]store.AgentProfileRecord(nil), items...)
	return nil
}

func agentProfileRecordForServerTest() store.AgentProfileRecord {
	return store.AgentProfileRecord{
		"id":   "main-agent",
		"name": "Primary Agent",
		"type": "main-agent",
		"runtime": map[string]any{
			"model":           "gpt-5.4",
			"reasoningEffort": "medium",
			"approvalPolicy":  "untrusted",
			"sandboxMode":     "workspace-write",
		},
		"systemPrompt": map[string]any{
			"content": "line one\nline two",
			"preview": "line one",
		},
		"commandPermissions": map[string]any{
			"categoryPolicies": map[string]any{"system_inspection": "allow"},
		},
		"capabilityPermissions": map[string]any{"commandExecution": "enabled"},
		"skills":                []any{map[string]any{"id": "ops-triage", "name": "Ops Triage", "enabled": true, "activationMode": "default_enabled"}},
		"mcps":                  []any{map[string]any{"id": "filesystem", "name": "Filesystem MCP", "enabled": true, "permission": "readonly"}},
	}
}

type agentProfileAPITestServices struct {
	svc appui.AgentProfileService
}

func (s *agentProfileAPITestServices) ChatService() appui.ChatService         { return nil }
func (s *agentProfileAPITestServices) StateService() appui.StateService       { return nil }
func (s *agentProfileAPITestServices) SessionService() appui.SessionService   { return nil }
func (s *agentProfileAPITestServices) ApprovalService() appui.ApprovalService { return nil }
func (s *agentProfileAPITestServices) ChoiceService() appui.ChoiceService     { return nil }
func (s *agentProfileAPITestServices) SettingsService() appui.SettingsService { return nil }
func (s *agentProfileAPITestServices) HostService() appui.HostService         { return nil }
func (s *agentProfileAPITestServices) MCPService() appui.MCPService           { return nil }
func (s *agentProfileAPITestServices) AuthService() appui.AuthService         { return nil }
func (s *agentProfileAPITestServices) TerminalService() appui.TerminalService { return nil }
func (s *agentProfileAPITestServices) AgentProfileService() appui.AgentProfileService {
	return s.svc
}

func TestAgentProfileAPI(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{{ID: "ops-triage", Name: "Ops Triage", DefaultEnabled: true}},
		mcpCatalog:   []store.AgentMCPCatalogEntry{{ID: "filesystem", Name: "Filesystem MCP", Type: "stdio", DefaultEnabled: true, Permission: "readonly"}},
		profiles: []store.AgentProfileRecord{
			agentProfileRecordForServerTest(),
		},
	}
	svc := appui.NewAgentProfileService(repo)
	server := NewHTTPServer(&agentProfileAPITestServices{svc: svc}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/agent-skills")
	if err != nil {
		t.Fatalf("GET /api/v1/agent-skills error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-skills status = %d, want 200", resp.StatusCode)
	}
	var skillsPayload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&skillsPayload); err != nil {
		t.Fatalf("decode skills payload error = %v", err)
	}
	if len(asSlice(skillsPayload["items"])) < 1 {
		t.Fatalf("skills payload = %+v, want at least one item", skillsPayload)
	}
	if !payloadItemsContainID(asSlice(skillsPayload["items"]), "ops-triage") {
		t.Fatalf("skills payload = %+v, want ops-triage item", skillsPayload)
	}

	upsertBody, _ := json.Marshal(map[string]any{"id": "incident-summary", "name": "Incident Summary", "enabled": true})
	upsertResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodPut, ts.URL+"/api/v1/agent-skills/incident-summary", upsertBody))
	if err != nil {
		t.Fatalf("PUT /api/v1/agent-skills/:id error = %v", err)
	}
	defer upsertResp.Body.Close()
	if upsertResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/agent-skills/:id status = %d, want 200", upsertResp.StatusCode)
	}

	deleteResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodDelete, ts.URL+"/api/v1/agent-skills/incident-summary", nil))
	if err != nil {
		t.Fatalf("DELETE /api/v1/agent-skills/:id error = %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/agent-skills/:id status = %d, want 200", deleteResp.StatusCode)
	}

	profilesResp, err := http.Get(ts.URL + "/api/v1/agent-profiles")
	if err != nil {
		t.Fatalf("GET /api/v1/agent-profiles error = %v", err)
	}
	defer profilesResp.Body.Close()
	if profilesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-profiles status = %d, want 200", profilesResp.StatusCode)
	}
	var profilesPayload map[string]any
	if err := json.NewDecoder(profilesResp.Body).Decode(&profilesPayload); err != nil {
		t.Fatalf("decode profiles payload error = %v", err)
	}
	if len(asSlice(profilesPayload["items"])) != 1 || len(asSlice(profilesPayload["skillCatalog"])) < 1 {
		t.Fatalf("profiles payload = %+v, want catalog data", profilesPayload)
	}

	previewResp, err := http.Get(ts.URL + "/api/v1/agent-profile/preview?profileId=main-agent")
	if err != nil {
		t.Fatalf("GET /api/v1/agent-profile/preview error = %v", err)
	}
	defer previewResp.Body.Close()
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", previewResp.StatusCode)
	}
	var previewPayload map[string]any
	if err := json.NewDecoder(previewResp.Body).Decode(&previewPayload); err != nil {
		t.Fatalf("decode preview payload error = %v", err)
	}
	if previewPayload["profileId"] != "main-agent" {
		t.Fatalf("preview payload = %+v, want profileId", previewPayload)
	}

	saveBody, _ := json.Marshal(map[string]any{"id": "main-agent", "name": "Primary Agent", "description": "updated"})
	saveResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodPut, ts.URL+"/api/v1/agent-profile", saveBody))
	if err != nil {
		t.Fatalf("PUT /api/v1/agent-profile error = %v", err)
	}
	defer saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/agent-profile status = %d, want 200", saveResp.StatusCode)
	}

	resetResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodPost, ts.URL+"/api/v1/agent-profile/reset", nil))
	if err != nil {
		t.Fatalf("POST /api/v1/agent-profile/reset error = %v", err)
	}
	defer resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/agent-profile/reset status = %d, want 200", resetResp.StatusCode)
	}

	exportResp, err := http.Get(ts.URL + "/api/v1/agent-profiles/export")
	if err != nil {
		t.Fatalf("GET /api/v1/agent-profiles/export error = %v", err)
	}
	defer exportResp.Body.Close()
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d, want 200", exportResp.StatusCode)
	}

	importBody, _ := json.Marshal(map[string]any{"profiles": []any{agentProfileRecordForServerTest()}})
	importResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodPost, ts.URL+"/api/v1/agent-profiles/import", importBody))
	if err != nil {
		t.Fatalf("POST /api/v1/agent-profiles/import error = %v", err)
	}
	defer importResp.Body.Close()
	if importResp.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d, want 200", importResp.StatusCode)
	}
}

func mustRequest(t *testing.T, method, url string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func payloadItemsContainID(items []any, id string) bool {
	for _, item := range items {
		record, ok := item.(map[string]any)
		if ok && record["id"] == id {
			return true
		}
	}
	return false
}
