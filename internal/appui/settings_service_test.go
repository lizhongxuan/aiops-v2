package appui

import (
	"context"
	"fmt"
	"testing"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/store"
)

type settingsRepoStub struct {
	web *store.WebSettings
	llm *store.LLMConfig
}

func (s *settingsRepoStub) GetWebSettings() (*store.WebSettings, error) {
	if s.web == nil {
		return nil, fmt.Errorf("web settings not found")
	}
	cp := *s.web
	cp.Models = append([]store.SettingModelOption(nil), cp.Models...)
	return &cp, nil
}

func (s *settingsRepoStub) SaveWebSettings(settings *store.WebSettings) error {
	cp := *settings
	cp.Models = append([]store.SettingModelOption(nil), cp.Models...)
	s.web = &cp
	return nil
}

func (s *settingsRepoStub) GetLLMConfig() (*store.LLMConfig, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("llm config not found")
	}
	cp := *s.llm
	return &cp, nil
}

func (s *settingsRepoStub) SaveLLMConfig(config *store.LLMConfig) error {
	cp := *config
	s.llm = &cp
	return nil
}

func TestSettingsServiceLoadsAndUpdatesSettingsAndLLMConfig(t *testing.T) {
	ResetAuthSummaryForTest()
	repo := &settingsRepoStub{
		web: &store.WebSettings{
			Model:           "gpt-5.4",
			ReasoningEffort: "high",
		},
		llm: &store.LLMConfig{
			Provider:     "openai",
			Model:        "gpt-5.4",
			APIKey:       "sk-test-12345678",
			CompactModel: "gpt-5.4-mini",
		},
	}
	svc := NewSettingsService(repo)

	settings, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if settings.Model != "gpt-5.4" || settings.ReasoningEffort != "high" {
		t.Fatalf("settings = %+v, want stored settings", settings)
	}

	llmView, err := svc.GetLLMConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLLMConfig() error = %v", err)
	}
	if !llmView.APIKeySet || llmView.APIKeyMasked == "" {
		t.Fatalf("llmView = %+v, want masked api key", llmView)
	}

	updated, err := svc.UpdateSettings(context.Background(), WebSettingsPayload{
		Model:           "claude-3-opus",
		ReasoningEffort: "low",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if updated.Model != "gpt-5.4" || repo.web.Model != "claude-3-opus" {
		t.Fatalf("updated = %+v repo.web = %+v, want llm model surfaced while web settings persist local selection", updated, repo.web)
	}

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider: "anthropic",
		Model:    "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK || repo.llm.Provider != "anthropic" || repo.llm.APIKey != "sk-test-12345678" {
		t.Fatalf("result = %+v repo.llm = %+v, want merged llm config", result, repo.llm)
	}
}

func TestSettingsServiceDefaultsToGPT54WhenRepoIsEmpty(t *testing.T) {
	svc := NewSettingsService(&settingsRepoStub{})

	settings, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if settings.Model != "gpt-5.4" {
		t.Fatalf("settings.Model = %q, want gpt-5.4", settings.Model)
	}

	llmView, err := svc.GetLLMConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLLMConfig() error = %v", err)
	}
	if llmView.Model != "gpt-5.4" || llmView.CompactModel != "gpt-5.4-mini" {
		t.Fatalf("llmView = %+v, want gpt-5.4 / gpt-5.4-mini defaults", llmView)
	}
}

func TestSettingsServiceSyncsLLMConfigToAuthSummary(t *testing.T) {
	ResetAuthSummaryForTest()
	manager := auth.NewManager(nil)
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo, manager)

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider: "openai",
		Model:    "gpt-5.4",
		APIKey:   "sk-live-12345678",
		BaseURL:  "http://127.0.0.1:8317/v1",
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("UpdateLLMConfig() = %+v, want ok", result)
	}

	summary := snapshotAuthSummary()
	if !summary.Connected || summary.Mode != string(auth.ModeAPIKey) {
		t.Fatalf("auth summary = %+v, want connected apiKey", summary)
	}
	truth, ok := manager.Resolve()
	if !ok || truth.APIKey != "sk-live-12345678" {
		t.Fatalf("resolved truth ok=%v truth=%+v, want saved api key", ok, truth)
	}
}
