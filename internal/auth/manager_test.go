package auth

import (
	"context"
	"testing"
)

type oauthProviderStub struct {
	configured  bool
	startURL    string
	exchange    CredentialTruth
	startErr    error
	exchangeErr error
	startState  OAuthState
	callbackReq OAuthCallbackRequest
}

func (s *oauthProviderStub) Configured() bool { return s.configured }

func (s *oauthProviderStub) Start(_ context.Context, state OAuthState) (string, error) {
	s.startState = state
	return s.startURL, s.startErr
}

func (s *oauthProviderStub) Exchange(_ context.Context, req OAuthCallbackRequest) (CredentialTruth, error) {
	s.callbackReq = req
	return s.exchange, s.exchangeErr
}

func TestManagerLoginApiKeyCreatesCookieSessionAndSummary(t *testing.T) {
	mgr := NewManager(nil)

	result := mgr.Login(context.Background(), LoginRequest{
		Mode:     ModeAPIKey,
		APIKey:   "sk-test-secret",
		Email:    "ops@example.com",
		PlanType: "pro",
	})

	if result.Result != "success" {
		t.Fatalf("result = %q, want success", result.Result)
	}
	if result.Summary.Mode != string(ModeAPIKey) {
		t.Fatalf("summary.mode = %q, want %q", result.Summary.Mode, ModeAPIKey)
	}
	if !result.Summary.Connected || result.Summary.Pending {
		t.Fatalf("summary = %+v, want connected and not pending", result.Summary)
	}
	if result.Summary.Email != "ops@example.com" {
		t.Fatalf("summary.email = %q, want ops@example.com", result.Summary.Email)
	}
	if result.Summary.PlanType != "pro" {
		t.Fatalf("summary.planType = %q, want pro", result.Summary.PlanType)
	}
	if result.Cookie == nil || result.Cookie.Name != DefaultCookieName || result.Cookie.Value == "" {
		t.Fatalf("cookie = %+v, want auth session cookie", result.Cookie)
	}
	if got, _ := mgr.SummaryForCookie(result.Cookie.Value); !got.Connected {
		t.Fatalf("SummaryForCookie() = %+v, want connected", got)
	}
}

func TestManagerLoginChatGPTReturnsOAuthURLWhenConfigured(t *testing.T) {
	provider := &oauthProviderStub{configured: true, startURL: "https://auth.example/start"}
	mgr := NewManager(provider)

	result := mgr.Login(context.Background(), LoginRequest{Mode: ModeChatGPT})

	if result.Result != "oauth_started" {
		t.Fatalf("result = %q, want oauth_started", result.Result)
	}
	if !result.Summary.Pending || result.Summary.Connected {
		t.Fatalf("summary = %+v, want pending and disconnected", result.Summary)
	}
	if result.AuthURL != "https://auth.example/start" {
		t.Fatalf("authUrl = %q, want https://auth.example/start", result.AuthURL)
	}
	if provider.startState.Value == "" {
		t.Fatal("start state should be generated")
	}
}

func TestManagerOAuthCallbackValidatesStateAndExchangesCode(t *testing.T) {
	provider := &oauthProviderStub{
		configured: true,
		exchange: CredentialTruth{
			Mode:     ModeChatGPT,
			Email:    "oauth@example.com",
			PlanType: "plus",
		},
	}
	mgr := NewManager(provider)
	start := mgr.StartOAuth(context.Background())

	result := mgr.OAuthCallback(context.Background(), OAuthCallbackRequest{
		State: start.State,
		Code:  "code-123",
	})

	if result.Result != "success" {
		t.Fatalf("result = %q, want success", result.Result)
	}
	if !result.Summary.Connected || result.Summary.Pending {
		t.Fatalf("summary = %+v, want connected and not pending", result.Summary)
	}
	if result.Summary.Email != "oauth@example.com" {
		t.Fatalf("summary.email = %q, want oauth@example.com", result.Summary.Email)
	}
	if provider.callbackReq.Code != "code-123" || provider.callbackReq.State != start.State {
		t.Fatalf("callback request = %+v, want state/code from callback", provider.callbackReq)
	}
	if result.Cookie == nil || result.Cookie.Value == "" {
		t.Fatal("oauth callback should create session cookie")
	}
}

func TestManagerLogoutClearsSummary(t *testing.T) {
	mgr := NewManager(nil)
	login := mgr.Login(context.Background(), LoginRequest{Mode: ModeAPIKey, APIKey: "sk-test-secret"})
	if login.Cookie == nil {
		t.Fatal("expected login cookie")
	}

	result := mgr.Logout(context.Background())

	if result.Result != "success" {
		t.Fatalf("result = %q, want success", result.Result)
	}
	if result.Summary.Connected || result.Summary.Pending {
		t.Fatalf("summary = %+v, want disconnected", result.Summary)
	}
	if got, _ := mgr.SummaryForCookie(login.Cookie.Value); got.Connected {
		t.Fatalf("SummaryForCookie() = %+v, want disconnected after logout", got)
	}
}

func TestManagerLoginRejectsMissingSecrets(t *testing.T) {
	mgr := NewManager(nil)

	apiKey := mgr.Login(context.Background(), LoginRequest{Mode: ModeAPIKey})
	if apiKey.Result != "missing_secret" {
		t.Fatalf("api key result = %q, want missing_secret", apiKey.Result)
	}

	token := mgr.Login(context.Background(), LoginRequest{Mode: ModeChatGPTAuthTokens})
	if token.Result != "missing_secret" {
		t.Fatalf("token result = %q, want missing_secret", token.Result)
	}
}

func TestManagerOAuthStartWithoutProviderReturnsConfiguredError(t *testing.T) {
	mgr := NewManager(nil)

	result := mgr.StartOAuth(context.Background())
	if result.Result != "oauth_not_configured" {
		t.Fatalf("result = %q, want oauth_not_configured", result.Result)
	}
	if result.AuthURL != "" {
		t.Fatalf("authUrl = %q, want empty", result.AuthURL)
	}
}

func TestManagerCallbackRejectsInvalidStateAndMissingCode(t *testing.T) {
	provider := &oauthProviderStub{configured: true, exchange: CredentialTruth{Mode: ModeChatGPT}}
	mgr := NewManager(provider)
	start := mgr.StartOAuth(context.Background())

	invalid := mgr.OAuthCallback(context.Background(), OAuthCallbackRequest{State: "wrong", Code: "code"})
	if invalid.Result != "invalid_state" {
		t.Fatalf("invalid result = %q, want invalid_state", invalid.Result)
	}

	missingCode := mgr.OAuthCallback(context.Background(), OAuthCallbackRequest{State: start.State})
	if missingCode.Result != "missing_code" {
		t.Fatalf("missing code result = %q, want missing_code", missingCode.Result)
	}
}

func TestManagerCookieSessionLookup(t *testing.T) {
	mgr := NewManager(nil)
	result := mgr.Login(context.Background(), LoginRequest{Mode: ModeAPIKey, APIKey: "sk-test-secret"})
	if result.Cookie == nil {
		t.Fatal("expected cookie")
	}

	summary, ok := mgr.SummaryForCookie(result.Cookie.Value)
	if !ok {
		t.Fatal("expected cookie session to be found")
	}
	if !summary.Connected {
		t.Fatalf("summary = %+v, want connected", summary)
	}
	if cookie := result.Cookie; cookie.Name != DefaultCookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, DefaultCookieName)
	}
	if cookie := SessionCookie(result.Cookie.Value); cookie.Name != DefaultCookieName {
		t.Fatalf("session cookie name = %q, want %q", cookie.Name, DefaultCookieName)
	}
}
