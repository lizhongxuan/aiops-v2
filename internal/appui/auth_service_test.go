package appui

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/auth"
)

func TestAuthServiceLoginAndLogoutUpdateSnapshotAuthState(t *testing.T) {
	resetAuthSnapshotForTest()
	svc := NewAuthService(auth.NewManager(nil))

	login, err := svc.Login(context.Background(), auth.LoginRequest{
		Mode:     auth.ModeAPIKey,
		APIKey:   "sk-test-secret",
		Email:    "ops@example.com",
		PlanType: "team",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.Result != "success" {
		t.Fatalf("Login result = %q, want success", login.Result)
	}
	if !login.Summary.Connected || login.Summary.Email != "ops@example.com" {
		t.Fatalf("login summary = %+v", login.Summary)
	}

	state := StateSnapshot{}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	authJSON, ok := decoded["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth payload missing: %+v", decoded)
	}
	if got := authJSON["email"]; got != "ops@example.com" {
		t.Fatalf("auth.email = %v, want ops@example.com", got)
	}
	if got := authJSON["connected"]; got != true {
		t.Fatalf("auth.connected = %v, want true", got)
	}
	if _, ok := authJSON["apiKey"]; ok {
		t.Fatalf("auth payload leaked secret field: %+v", authJSON)
	}

	logout, err := svc.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if logout.Summary.Connected {
		t.Fatalf("logout summary = %+v, want disconnected", logout.Summary)
	}
}

func TestAuthServiceOAuthStartAndCallback(t *testing.T) {
	resetAuthSnapshotForTest()
	provider := &authTestProvider{
		startURL: "https://example.test/oauth",
		exchange: auth.CredentialTruth{
			Mode:     auth.ModeChatGPT,
			Email:    "oauth@example.com",
			PlanType: "plus",
		},
	}
	svc := NewAuthService(auth.NewManager(provider))

	start, err := svc.OAuthStart(context.Background())
	if err != nil {
		t.Fatalf("OAuthStart() error = %v", err)
	}
	if start.AuthURL != "https://example.test/oauth" {
		t.Fatalf("authUrl = %q, want oauth URL", start.AuthURL)
	}
	if !start.Summary.Pending {
		t.Fatalf("summary = %+v, want pending", start.Summary)
	}

	callback, err := svc.OAuthCallback(context.Background(), auth.OAuthCallbackRequest{
		State: provider.startState.Value,
		Code:  "code-123",
	})
	if err != nil {
		t.Fatalf("OAuthCallback() error = %v", err)
	}
	if !callback.Summary.Connected {
		t.Fatalf("callback summary = %+v, want connected", callback.Summary)
	}
}

type authTestProvider struct {
	startURL   string
	startState auth.OAuthState
	exchange   auth.CredentialTruth
}

func (p *authTestProvider) Configured() bool { return true }

func (p *authTestProvider) Start(_ context.Context, state auth.OAuthState) (string, error) {
	p.startState = state
	return p.startURL, nil
}

func (p *authTestProvider) Exchange(_ context.Context, req auth.OAuthCallbackRequest) (auth.CredentialTruth, error) {
	return p.exchange, nil
}

func resetAuthSnapshotForTest() {
	setAuthSummary(auth.UIAuthSummary{})
}
