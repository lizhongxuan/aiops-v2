package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/auth"
)

type authAPITestServices struct {
	auth appui.AuthService
}

func (s *authAPITestServices) ChatService() appui.ChatService         { return nil }
func (s *authAPITestServices) StateService() appui.StateService       { return nil }
func (s *authAPITestServices) SessionService() appui.SessionService   { return nil }
func (s *authAPITestServices) ApprovalService() appui.ApprovalService { return nil }
func (s *authAPITestServices) ChoiceService() appui.ChoiceService     { return nil }
func (s *authAPITestServices) SettingsService() appui.SettingsService { return nil }
func (s *authAPITestServices) HostService() appui.HostService         { return nil }
func (s *authAPITestServices) MCPService() appui.MCPService           { return nil }
func (s *authAPITestServices) TerminalService() appui.TerminalService { return nil }
func (s *authAPITestServices) AgentProfileService() appui.AgentProfileService {
	return nil
}
func (s *authAPITestServices) AuthService() appui.AuthService { return s.auth }

func TestAuthAPIHandlesLoginLogoutAndOAuthFlow(t *testing.T) {
	resetAuthSnapshotForServerTest()
	provider := &serverAuthProvider{
		startURL: "https://example.test/oauth",
		exchange: auth.CredentialTruth{Mode: auth.ModeChatGPT, Email: "oauth@example.com"},
	}
	manager := auth.NewManager(provider)
	httpSrv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithAuthManager(manager)))
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	loginBody := []byte(`{"mode":"apiKey","apiKey":"sk-test-secret","email":"ops@example.com"}`)
	loginResp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytesReader(loginBody))
	if err != nil {
		t.Fatalf("POST /api/v1/auth/login error = %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	var loginPayload map[string]any
	if err := json.NewDecoder(loginResp.Body).Decode(&loginPayload); err != nil {
		t.Fatalf("decode login payload error = %v", err)
	}
	if loginPayload["result"] != "success" {
		t.Fatalf("login payload = %+v, want success", loginPayload)
	}
	if _, ok := loginPayload["apiKey"]; ok {
		t.Fatalf("login payload leaked secret: %+v", loginPayload)
	}

	stateResp, err := http.Get(ts.URL + "/api/v1/state")
	if err != nil {
		t.Fatalf("GET /api/v1/state error = %v", err)
	}
	defer stateResp.Body.Close()
	var state map[string]any
	if err := json.NewDecoder(stateResp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state payload error = %v", err)
	}
	authState, ok := state["auth"].(map[string]any)
	if !ok || authState["connected"] != true || authState["email"] != "ops@example.com" {
		t.Fatalf("state.auth = %+v, want connected login summary", authState)
	}

	startResp, err := http.Get(ts.URL + "/api/v1/auth/oauth/start")
	if err != nil {
		t.Fatalf("GET /api/v1/auth/oauth/start error = %v", err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("oauth start status = %d, want 200", startResp.StatusCode)
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	callbackResp, err := noRedirectClient.Get(ts.URL + "/api/v1/auth/oauth/callback?state=" + provider.startState.Value + "&code=abc")
	if err != nil {
		t.Fatalf("GET /api/v1/auth/oauth/callback error = %v", err)
	}
	defer callbackResp.Body.Close()
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("oauth callback status = %d, want 302", callbackResp.StatusCode)
	}
	if loc := callbackResp.Header.Get("Location"); loc == "" {
		t.Fatal("oauth callback should redirect back to the app")
	}

	logoutResp, err := http.Post(ts.URL+"/api/v1/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/v1/auth/logout error = %v", err)
	}
	defer logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", logoutResp.StatusCode)
	}
}

type serverAuthProvider struct {
	startURL   string
	startState auth.OAuthState
	exchange   auth.CredentialTruth
}

func (p *serverAuthProvider) Configured() bool { return true }

func (p *serverAuthProvider) Start(_ context.Context, state auth.OAuthState) (string, error) {
	p.startState = state
	return p.startURL, nil
}

func (p *serverAuthProvider) Exchange(_ context.Context, req auth.OAuthCallbackRequest) (auth.CredentialTruth, error) {
	return p.exchange, nil
}

func resetAuthSnapshotForServerTest() {
	appui.ResetAuthSummaryForTest()
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
