package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type sessionRecord struct {
	truth   CredentialTruth
	summary UIAuthSummary
}

// Manager owns credential truth, oauth state, and cookie sessions.
type Manager struct {
	mu             sync.RWMutex
	provider       OAuthProvider
	sessions       map[string]sessionRecord
	oauthStates    map[string]OAuthState
	currentSession string
	summary        UIAuthSummary
}

func NewManager(provider OAuthProvider) *Manager {
	mgr := &Manager{
		provider:    provider,
		sessions:    map[string]sessionRecord{},
		oauthStates: map[string]OAuthState{},
	}
	mgr.summary.OAuthConfigured = provider != nil && provider.Configured()
	return mgr
}

func (m *Manager) Login(ctx context.Context, req LoginRequest) AuthResult {
	mode := req.Mode
	if mode == "" {
		mode = ModeChatGPT
	}
	switch mode {
	case ModeAPIKey:
		apiKey := strings.TrimSpace(req.APIKey)
		if apiKey == "" {
			return m.fail("missing_secret", false, string(mode), req, nil)
		}
		return m.persistTruth(CredentialTruth{
			Mode:     ModeAPIKey,
			Email:    strings.TrimSpace(req.Email),
			PlanType: firstNonEmpty(req.PlanType, req.ChatGPTPlanType),
			APIKey:   apiKey,
		}, "success")
	case ModeChatGPTAuthTokens:
		token := strings.TrimSpace(req.AccessToken)
		if token == "" {
			return m.fail("missing_secret", false, string(mode), req, nil)
		}
		return m.persistTruth(CredentialTruth{
			Mode:        ModeChatGPTAuthTokens,
			Email:       strings.TrimSpace(req.Email),
			PlanType:    firstNonEmpty(req.ChatGPTPlanType, req.PlanType),
			AccountID:   strings.TrimSpace(req.ChatGPTAccountID),
			AccessToken: token,
		}, "success")
	case ModeChatGPT:
		return m.startOAuth(ctx, mode)
	default:
		return m.fail("unsupported_mode", false, string(mode), req, nil)
	}
}

func (m *Manager) StartOAuth(ctx context.Context) AuthResult {
	return m.startOAuth(ctx, ModeChatGPT)
}

func (m *Manager) OAuthCallback(ctx context.Context, req OAuthCallbackRequest) AuthResult {
	m.mu.RLock()
	state, ok := m.oauthStates[strings.TrimSpace(req.State)]
	configured := m.summary.OAuthConfigured
	m.mu.RUnlock()

	if !configured {
		return m.fail("oauth_not_configured", true, string(ModeChatGPT), LoginRequest{Mode: ModeChatGPT}, nil)
	}
	if strings.TrimSpace(req.State) == "" || !ok {
		return m.fail("invalid_state", true, string(state.Mode), LoginRequest{Mode: state.Mode}, nil)
	}
	if strings.TrimSpace(req.Code) == "" {
		return m.fail("missing_code", true, string(state.Mode), LoginRequest{Mode: state.Mode}, nil)
	}
	if m.provider == nil {
		return m.fail("oauth_not_configured", true, string(state.Mode), LoginRequest{Mode: state.Mode}, nil)
	}
	truth, err := m.provider.Exchange(ctx, req)
	if err != nil {
		return m.fail("exchange_failed", true, string(state.Mode), LoginRequest{Mode: state.Mode}, err)
	}
	if truth.Mode == "" {
		truth.Mode = state.Mode
	}
	if truth.Email == "" {
		truth.Email = m.summary.Email
	}
	return m.persistTruth(truth, "success")
}

func (m *Manager) Logout(context.Context) AuthResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentSession != "" {
		delete(m.sessions, m.currentSession)
	}
	m.currentSession = ""
	m.summary = UIAuthSummary{OAuthConfigured: m.provider != nil && m.provider.Configured()}
	return AuthResult{
		Result:  "success",
		Summary: m.summary,
		Cookie:  ClearingSessionCookie(),
	}
}

func (m *Manager) Summary() UIAuthSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.summary
}

func (m *Manager) SummaryForCookie(value string) (UIAuthSummary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.sessions[strings.TrimSpace(value)]
	if !ok {
		return UIAuthSummary{OAuthConfigured: m.summary.OAuthConfigured}, false
	}
	summary := record.summary
	summary.OAuthConfigured = m.summary.OAuthConfigured
	return summary, true
}

func (m *Manager) Resolve() (CredentialTruth, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentSession == "" {
		return CredentialTruth{}, false
	}
	record, ok := m.sessions[m.currentSession]
	if !ok {
		return CredentialTruth{}, false
	}
	return record.truth, true
}

func (m *Manager) persistTruth(truth CredentialTruth, result string) AuthResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := newSessionToken()
	summary := UIAuthSummary{
		Connected:       true,
		Pending:         false,
		Mode:            string(truth.Mode),
		PlanType:        strings.TrimSpace(truth.PlanType),
		Email:           strings.TrimSpace(truth.Email),
		LastError:       "",
		OAuthConfigured: m.summary.OAuthConfigured,
	}
	if summary.Mode == "" {
		summary.Mode = string(ModeAPIKey)
	}
	m.sessions[session] = sessionRecord{truth: truth, summary: summary}
	m.currentSession = session
	m.summary = summary
	return AuthResult{
		Result:  result,
		Summary: summary,
		Cookie:  SessionCookie(session),
	}
}

func (m *Manager) startOAuth(ctx context.Context, mode Mode) AuthResult {
	m.mu.Lock()
	configured := m.provider != nil && m.provider.Configured()
	m.summary.OAuthConfigured = configured
	m.mu.Unlock()

	if !configured {
		return m.fail("oauth_not_configured", true, string(mode), LoginRequest{Mode: mode}, nil)
	}
	state := OAuthState{
		Value:     newOAuthStateToken(),
		CreatedAt: time.Now().UTC(),
		Mode:      mode,
	}
	authURL, err := m.provider.Start(ctx, state)
	if err != nil {
		return m.fail("oauth_start_failed", true, string(mode), LoginRequest{Mode: mode}, err)
	}
	m.mu.Lock()
	m.oauthStates[state.Value] = state
	m.summary = UIAuthSummary{
		Connected:       false,
		Pending:         true,
		Mode:            string(mode),
		OAuthConfigured: true,
	}
	m.currentSession = ""
	m.mu.Unlock()
	return AuthResult{
		Result:  "oauth_started",
		AuthURL: authURL,
		State:   state.Value,
		Summary: m.Summary(),
	}
}

func (m *Manager) fail(result string, pending bool, mode string, req LoginRequest, err error) AuthResult {
	summary := UIAuthSummary{
		Connected:       false,
		Pending:         pending,
		Mode:            strings.TrimSpace(mode),
		PlanType:        strings.TrimSpace(firstNonEmpty(req.ChatGPTPlanType, req.PlanType)),
		Email:           strings.TrimSpace(req.Email),
		LastError:       result,
		OAuthConfigured: m.provider != nil && m.provider.Configured(),
	}
	m.mu.Lock()
	m.summary = summary
	m.mu.Unlock()
	_ = err
	return AuthResult{
		Result:  result,
		Summary: summary,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (m *Manager) touchOAuthState(state string) (OAuthState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.oauthStates[strings.TrimSpace(state)]
	return value, ok
}

func (m *Manager) CookieForSession(value string) (*sessionRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.sessions[strings.TrimSpace(value)]
	if !ok {
		return nil, false
	}
	copy := record
	return &copy, true
}

func (m *Manager) String() string {
	return fmt.Sprintf("auth.Manager{summary:%+v}", m.Summary())
}
