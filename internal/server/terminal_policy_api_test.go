package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/terminalpolicy"
)

func TestTerminalPolicyAPIReadsAndWritesConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal-command-policies.json")
	service := appui.NewTerminalPolicyService(path)
	server := NewHTTPServer(&terminalPolicyAPITestServices{policy: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/terminal-policies")
	if err != nil {
		t.Fatalf("GET /api/v1/terminal-policies error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	var initial terminalpolicy.Config
	if err := json.NewDecoder(resp.Body).Decode(&initial); err != nil {
		t.Fatalf("decode initial response: %v", err)
	}
	if initial.SchemaVersion != "aiops.terminal_policy/v1" {
		t.Fatalf("initial schema = %q, want terminal policy schema", initial.SchemaVersion)
	}

	update := terminalpolicy.Config{
		SchemaVersion: "aiops.terminal_policy/v1",
		Rules: []terminalpolicy.Rule{
			{ID: "allow-ss-listen", Effect: terminalpolicy.RuleEffectAllow, Command: "ss", ArgsPrefix: []string{"-ltnp"}, Reason: "bounded socket inspection"},
		},
	}
	body, _ := json.Marshal(update)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/terminal-policies", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/v1/terminal-policies error = %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", putResp.StatusCode)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("policy file was not written: %v", err)
	}
	if !bytes.Contains(raw, []byte("allow-ss-listen")) {
		t.Fatalf("policy file = %s, want saved rule", raw)
	}
	if decision := service.Evaluate(terminalpolicy.CommandRequest{Command: "ss", Args: []string{"-ltnp"}}); decision.Action != terminalpolicy.PolicyActionAllow || decision.RuleID != "allow-ss-listen" {
		t.Fatalf("service decision = %#v, want updated policy allow", decision)
	}
}

type terminalPolicyAPITestServices struct {
	policy appui.TerminalPolicyService
}

func (s *terminalPolicyAPITestServices) ChatService() appui.ChatService         { return nil }
func (s *terminalPolicyAPITestServices) StateService() appui.StateService       { return nil }
func (s *terminalPolicyAPITestServices) SessionService() appui.SessionService   { return nil }
func (s *terminalPolicyAPITestServices) ApprovalService() appui.ApprovalService { return nil }
func (s *terminalPolicyAPITestServices) ChoiceService() appui.ChoiceService     { return nil }
func (s *terminalPolicyAPITestServices) SettingsService() appui.SettingsService { return nil }
func (s *terminalPolicyAPITestServices) HostService() appui.HostService         { return nil }
func (s *terminalPolicyAPITestServices) MCPService() appui.MCPService           { return nil }
func (s *terminalPolicyAPITestServices) AgentProfileService() appui.AgentProfileService {
	return nil
}
func (s *terminalPolicyAPITestServices) AuthService() appui.AuthService         { return nil }
func (s *terminalPolicyAPITestServices) TerminalService() appui.TerminalService { return nil }
func (s *terminalPolicyAPITestServices) TerminalPolicyService() appui.TerminalPolicyService {
	return s.policy
}
