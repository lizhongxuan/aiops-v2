package auth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// DefaultOAuthProvider is a small local implementation that builds OAuth URLs
// from a configurable authorize endpoint template.
type DefaultOAuthProvider struct {
	AuthorizeURL string
	ExchangeFunc func(context.Context, OAuthCallbackRequest) (CredentialTruth, error)
}

func (p *DefaultOAuthProvider) Configured() bool {
	return strings.TrimSpace(p.AuthorizeURL) != ""
}

func (p *DefaultOAuthProvider) Start(_ context.Context, state OAuthState) (string, error) {
	if !p.Configured() {
		return "", fmt.Errorf("oauth not configured")
	}
	parsed, err := url.Parse(strings.TrimSpace(p.AuthorizeURL))
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("state", state.Value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (p *DefaultOAuthProvider) Exchange(ctx context.Context, req OAuthCallbackRequest) (CredentialTruth, error) {
	if p.ExchangeFunc == nil {
		return CredentialTruth{}, fmt.Errorf("oauth exchange function is not configured")
	}
	return p.ExchangeFunc(ctx, req)
}
