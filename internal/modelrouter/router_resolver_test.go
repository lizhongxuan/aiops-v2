package modelrouter

import (
	"context"
	"testing"

	"aiops-v2/internal/auth"
)

type resolverStub struct {
	truth auth.CredentialTruth
	ok    bool
}

func (r *resolverStub) Resolve() (auth.CredentialTruth, bool) {
	return r.truth, r.ok
}

type providerConfigResolverStub struct {
	config ProviderConfig
	ok     bool
}

func (r *providerConfigResolverStub) ResolveProviderConfig(AgentKind) (ProviderConfig, bool) {
	return r.config, r.ok
}

func TestGetModel_ResolverBackedProviderUsesCurrentTruth(t *testing.T) {
	r := NewRouter("openai", nil, nil)
	resolver := &resolverStub{
		truth: auth.CredentialTruth{Mode: auth.ModeAPIKey, APIKey: "sk-first"},
		ok:    true,
	}
	r.SetCredentialResolver(resolver)
	r.SetProviderFactory("openai", func(_ context.Context, _ AgentKind, _ ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
		if !hasTruth {
			t.Fatal("expected resolver truth to be available")
		}
		secret := truth.APIKey
		if secret == "" {
			secret = truth.AccessToken
		}
		return &mockModel{name: secret}, nil
	})

	first, err := r.GetModel(AgentKindWorker, ProviderConfig{})
	if err != nil {
		t.Fatalf("first GetModel() error = %v", err)
	}
	if got := first.(*mockModel).name; got != "sk-first" {
		t.Fatalf("first model name = %q, want %q", got, "sk-first")
	}

	resolver.truth = auth.CredentialTruth{Mode: auth.ModeChatGPTAuthTokens, AccessToken: "tok-second"}

	second, err := r.GetModel(AgentKindWorker, ProviderConfig{})
	if err != nil {
		t.Fatalf("second GetModel() error = %v", err)
	}
	if got := second.(*mockModel).name; got != "tok-second" {
		t.Fatalf("second model name = %q, want %q", got, "tok-second")
	}
	if first == second {
		t.Fatal("expected lazy construction to return a fresh model when truth changes")
	}
}

func TestGetModel_ResolverBackedProviderFallsBackWhenTruthUnavailable(t *testing.T) {
	ollama := &mockModel{name: "ollama"}
	r := NewRouter("openai", map[string]ChatModel{"ollama": ollama}, []FallbackEntry{
		{Primary: "openai", Fallback: "ollama"},
	})
	r.SetCredentialResolver(&resolverStub{ok: false})

	m, err := r.GetModel(AgentKindPlanner, ProviderConfig{})
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if m != ollama {
		t.Fatalf("expected fallback ollama model, got %T", m)
	}
}

func TestGetModel_ProviderConfigResolverOverridesDefaultRouting(t *testing.T) {
	r := NewRouter("openai", nil, nil)
	r.SetAgentKindConfig(AgentKindWorker, AgentKindConfig{
		Provider: "openai",
		Model:    "gpt-4o-mini",
	})
	r.SetProviderConfigResolver(&providerConfigResolverStub{
		config: ProviderConfig{
			Provider:         "openai",
			Model:            "gpt-5.4",
			BaseURL:          "http://127.0.0.1:8317/v1",
			MaxContextTokens: 64000,
		},
		ok: true,
	})
	r.SetProviderFactory("openai", func(_ context.Context, _ AgentKind, config ProviderConfig, _ auth.CredentialTruth, _ bool) (ChatModel, error) {
		return &mockModel{name: config.Provider + "|" + config.Model + "|" + config.BaseURL}, nil
	})

	m, err := r.GetModel(AgentKindWorker, ProviderConfig{})
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if got := m.(*mockModel).name; got != "openai|gpt-5.4|http://127.0.0.1:8317/v1" {
		t.Fatalf("model config = %q, want saved provider/model/baseURL", got)
	}

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{})
	if caps.MaxContextTokens != 64000 {
		t.Fatalf("capabilities max context = %d, want resolver override 64000", caps.MaxContextTokens)
	}
}
