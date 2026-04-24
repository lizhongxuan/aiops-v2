package appui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"aiops-v2/internal/auth"
)

// AuthLoginRequest reuses the formal auth-domain login request.
type AuthLoginRequest = auth.LoginRequest

// AuthCallbackRequest reuses the formal auth-domain callback request.
type AuthCallbackRequest = auth.OAuthCallbackRequest

// AuthActionResponse is the transport response shared by auth handlers.
type AuthActionResponse struct {
	Result  string             `json:"result"`
	AuthURL string             `json:"authUrl,omitempty"`
	Summary auth.UIAuthSummary `json:"summary"`
	Cookie  *http.Cookie       `json:"-"`
}

// AuthService exposes the first-party auth domain to HTTP handlers.
type AuthService interface {
	Login(context.Context, AuthLoginRequest) (AuthActionResponse, error)
	Logout(context.Context) (AuthActionResponse, error)
	OAuthStart(context.Context) (AuthActionResponse, error)
	OAuthCallback(context.Context, AuthCallbackRequest) (AuthActionResponse, error)
	Summary() auth.UIAuthSummary
	Resolve() (auth.CredentialTruth, bool)
}

type defaultAuthService struct {
	manager *auth.Manager
}

var authSnapshotState = struct {
	mu      sync.RWMutex
	summary auth.UIAuthSummary
}{
	summary: auth.UIAuthSummary{},
}

func NewAuthService(manager *auth.Manager) AuthService {
	if manager == nil {
		manager = auth.NewManager(nil)
	}
	setAuthSummary(manager.Summary())
	return &defaultAuthService{manager: manager}
}

func (s *defaultAuthService) Login(ctx context.Context, req AuthLoginRequest) (AuthActionResponse, error) {
	resp := s.manager.Login(ctx, req)
	setAuthSummary(resp.Summary)
	return AuthActionResponse{Result: resp.Result, AuthURL: resp.AuthURL, Summary: resp.Summary, Cookie: resp.Cookie}, nil
}

func (s *defaultAuthService) Logout(ctx context.Context) (AuthActionResponse, error) {
	resp := s.manager.Logout(ctx)
	setAuthSummary(resp.Summary)
	return AuthActionResponse{Result: resp.Result, AuthURL: resp.AuthURL, Summary: resp.Summary, Cookie: resp.Cookie}, nil
}

func (s *defaultAuthService) OAuthStart(ctx context.Context) (AuthActionResponse, error) {
	resp := s.manager.StartOAuth(ctx)
	setAuthSummary(resp.Summary)
	return AuthActionResponse{Result: resp.Result, AuthURL: resp.AuthURL, Summary: resp.Summary, Cookie: resp.Cookie}, nil
}

func (s *defaultAuthService) OAuthCallback(ctx context.Context, req AuthCallbackRequest) (AuthActionResponse, error) {
	resp := s.manager.OAuthCallback(ctx, req)
	setAuthSummary(resp.Summary)
	return AuthActionResponse{Result: resp.Result, AuthURL: resp.AuthURL, Summary: resp.Summary, Cookie: resp.Cookie}, nil
}

func (s *defaultAuthService) Summary() auth.UIAuthSummary {
	if s == nil || s.manager == nil {
		return snapshotAuthSummary()
	}
	return s.manager.Summary()
}

func (s *defaultAuthService) Resolve() (auth.CredentialTruth, bool) {
	if s == nil || s.manager == nil {
		return auth.CredentialTruth{}, false
	}
	return s.manager.Resolve()
}

func setAuthSummary(summary auth.UIAuthSummary) {
	authSnapshotState.mu.Lock()
	authSnapshotState.summary = summary
	authSnapshotState.mu.Unlock()
}

func snapshotAuthSummary() auth.UIAuthSummary {
	authSnapshotState.mu.RLock()
	defer authSnapshotState.mu.RUnlock()
	return authSnapshotState.summary
}

// ResetAuthSummaryForTest clears the package-level auth snapshot state.
func ResetAuthSummaryForTest() {
	setAuthSummary(auth.UIAuthSummary{})
}

func (s StateSnapshot) MarshalJSON() ([]byte, error) {
	type stateAlias StateSnapshot
	summary := snapshotAuthSummary()
	if summary.OAuthConfigured == false {
		summary.OAuthConfigured = s.Auth.Connected || summary.Connected || summary.Pending || strings.TrimSpace(summary.Mode) != ""
	}
	if !summary.Connected && s.Auth.Connected {
		summary.Connected = true
	}
	payload := struct {
		stateAlias
		Auth   auth.UIAuthSummary `json:"auth"`
		Config map[string]any     `json:"config"`
	}{
		stateAlias: stateAlias(s),
		Auth:       summary,
		Config:     cloneConfigWithOAuth(s.Config, summary.OAuthConfigured),
	}
	return json.Marshal(payload)
}

func cloneConfigWithOAuth(config map[string]any, oauthConfigured bool) map[string]any {
	out := map[string]any{}
	for key, value := range config {
		out[key] = value
	}
	if oauthConfigured {
		out["oauthConfigured"] = true
	} else if _, ok := out["oauthConfigured"]; !ok {
		out["oauthConfigured"] = false
	}
	return out
}
