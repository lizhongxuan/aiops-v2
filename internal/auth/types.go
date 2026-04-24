package auth

import (
	"context"
	"net/http"
	"time"
)

// Mode identifies the supported login path.
type Mode string

const (
	ModeChatGPT           Mode = "chatgpt"
	ModeChatGPTAuthTokens Mode = "chatgptAuthTokens"
	ModeAPIKey            Mode = "apiKey"
)

const DefaultCookieName = "aiops_auth_session"

// CredentialTruth is the authoritative, server-side-only auth record.
type CredentialTruth struct {
	Mode        Mode   `json:"mode"`
	Email       string `json:"email,omitempty"`
	PlanType    string `json:"planType,omitempty"`
	AccountID   string `json:"accountId,omitempty"`
	AccessToken string `json:"-"`
	APIKey      string `json:"-"`
}

// UIAuthSummary is the frontend-safe projection of the current auth state.
type UIAuthSummary struct {
	Connected       bool   `json:"connected"`
	Pending         bool   `json:"pending"`
	Mode            string `json:"mode,omitempty"`
	PlanType        string `json:"planType,omitempty"`
	Email           string `json:"email,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	OAuthConfigured bool   `json:"oauthConfigured,omitempty"`
}

// OAuthState keeps the OAuth CSRF token and lightweight bookkeeping.
type OAuthState struct {
	Value     string
	CreatedAt time.Time
	Mode      Mode
	ReturnTo  string
}

// CookieSession is the persisted browser session bound to a cookie value.
type CookieSession struct {
	Value     string
	CreatedAt time.Time
	ExpiresAt time.Time
	Summary   UIAuthSummary
}

// LoginRequest is the public input accepted by the auth domain.
type LoginRequest struct {
	Mode             Mode   `json:"mode"`
	AccessToken      string `json:"accessToken,omitempty"`
	APIKey           string `json:"apiKey,omitempty"`
	ChatGPTAccountID string `json:"chatgptAccountId,omitempty"`
	ChatGPTPlanType  string `json:"chatgptPlanType,omitempty"`
	PlanType         string `json:"planType,omitempty"`
	Email            string `json:"email,omitempty"`
}

// OAuthCallbackRequest captures the browser callback query parameters.
type OAuthCallbackRequest struct {
	State string
	Code  string
	Error string
}

// AuthResult is the transport-safe response shared by auth login/logout/start/callback.
type AuthResult struct {
	Result  string        `json:"result"`
	AuthURL string        `json:"authUrl,omitempty"`
	Summary UIAuthSummary `json:"summary"`
	State   string        `json:"state,omitempty"`
	Cookie  *http.Cookie  `json:"-"`
}

// OAuthProvider resolves OAuth URLs and exchanges codes for credentials.
type OAuthProvider interface {
	Configured() bool
	Start(ctx context.Context, state OAuthState) (string, error)
	Exchange(ctx context.Context, req OAuthCallbackRequest) (CredentialTruth, error)
}
