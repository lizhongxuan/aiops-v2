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
			Provider:         "openai",
			Model:            "gpt-5.4",
			APIKey:           "sk-test-12345678",
			MaxContextTokens: 131072,
			CompactModel:     "gpt-5.4-mini",
			ReasoningEffort:  "HIGH",
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
	if llmView.MaxContextTokens != 131072 {
		t.Fatalf("llmView.MaxContextTokens = %d, want 131072", llmView.MaxContextTokens)
	}
	if llmView.ReasoningEffort != "high" {
		t.Fatalf("llmView.ReasoningEffort = %q, want high", llmView.ReasoningEffort)
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
		Provider:         "anthropic",
		Model:            "claude-sonnet-4",
		MaxContextTokens: 9000,
		ReasoningEffort:  "invalid",
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK || repo.llm.Provider != "anthropic" || repo.llm.APIKey != "sk-test-12345678" {
		t.Fatalf("result = %+v repo.llm = %+v, want merged llm config", result, repo.llm)
	}
	if result.MaxContextTokens != 10000 || repo.llm.MaxContextTokens != 10000 {
		t.Fatalf("result = %+v repo.llm = %+v, want min context 10000", result, repo.llm)
	}
	if repo.llm.ReasoningEffort != "medium" || repo.web.ReasoningEffort != "medium" {
		t.Fatalf("repo.llm = %+v repo.web = %+v, want invalid reasoning effort normalized to medium", repo.llm, repo.web)
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
	if llmView.Model != "gpt-5.4" || llmView.CompactModel != "gpt-5.4-mini" || llmView.MaxContextTokens != 200000 {
		t.Fatalf("llmView = %+v, want gpt-5.4 / gpt-5.4-mini / 200000 defaults", llmView)
	}
}

func TestSettingsServiceNormalizesDeepSeekLLMConfig(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider:        "deepseek",
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("UpdateLLMConfig() = %+v, want ok", result)
	}
	if repo.llm.Provider != "deepseek" || repo.llm.Model != "deepseek-v4-pro" {
		t.Fatalf("saved provider/model = %s/%s, want deepseek/deepseek-v4-pro", repo.llm.Provider, repo.llm.Model)
	}
	if repo.llm.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("saved baseURL = %q, want DeepSeek official URL", repo.llm.BaseURL)
	}
	if repo.llm.MaxContextTokens != 1000000 || repo.llm.MaxOutputTokens != 20000 {
		t.Fatalf("saved context/output = %d/%d, want 1000000/20000", repo.llm.MaxContextTokens, repo.llm.MaxOutputTokens)
	}
	if repo.llm.ThinkingType != "enabled" || repo.llm.ReasoningEffort != "high" {
		t.Fatalf("saved thinking/reasoning = %q/%q, want enabled/high", repo.llm.ThinkingType, repo.llm.ReasoningEffort)
	}
}

func TestSettingsServiceNormalizesZhipuLLMConfig(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider:        "zhipu",
		ReasoningEffort: "xhigh",
		ThinkingType:    "disabled",
		ToolStream:      true,
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("UpdateLLMConfig() = %+v, want ok", result)
	}
	if repo.llm.Provider != "zhipu" || repo.llm.Model != "glm-5.2" {
		t.Fatalf("saved provider/model = %s/%s, want zhipu/glm-5.2", repo.llm.Provider, repo.llm.Model)
	}
	if repo.llm.BaseURL != "https://open.bigmodel.cn/api/paas/v4/" {
		t.Fatalf("saved baseURL = %q, want Zhipu official URL", repo.llm.BaseURL)
	}
	if repo.llm.MaxContextTokens != 1000000 || repo.llm.MaxOutputTokens != 20000 {
		t.Fatalf("saved context/output = %d/%d, want 1000000/20000", repo.llm.MaxContextTokens, repo.llm.MaxOutputTokens)
	}
	if repo.llm.ThinkingType != "disabled" || repo.llm.ReasoningEffort != "xhigh" || !repo.llm.ToolStream {
		t.Fatalf("saved thinking/reasoning/toolStream = %q/%q/%v, want disabled/xhigh/true", repo.llm.ThinkingType, repo.llm.ReasoningEffort, repo.llm.ToolStream)
	}

	result, err = svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider:   "zhipu",
		Model:      "glm-5.2",
		ToolStream: false,
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() disabling tool stream error = %v", err)
	}
	if !result.OK || repo.llm.ToolStream {
		t.Fatalf("saved toolStream after disabling = %+v/%v, want false", result, repo.llm.ToolStream)
	}
}

func TestSettingsServiceAllowsCustomModelMaxOutputOverride(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider:        "deepseek",
		Model:           "custom-deepseek-compatible",
		MaxOutputTokens: 64000,
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("UpdateLLMConfig() = %+v, want ok", result)
	}
	if repo.llm.MaxOutputTokens != 64000 {
		t.Fatalf("custom model max output = %d, want 64000", repo.llm.MaxOutputTokens)
	}
}

func TestSettingsServicePreservesOpenAIConfigWithoutProviderSpecificDefaults(t *testing.T) {
	repo := &settingsRepoStub{
		llm: &store.LLMConfig{
			Provider:         "openai",
			Model:            "gpt-5.4",
			APIKey:           "sk-test-12345678",
			BaseURL:          "https://api.openai.com/v1",
			MaxContextTokens: 200000,
			ReasoningEffort:  "high",
		},
	}
	svc := NewSettingsService(repo)

	result, err := svc.UpdateLLMConfig(context.Background(), LLMConfigUpdate{
		Provider: "openai",
		Model:    "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("UpdateLLMConfig() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("UpdateLLMConfig() = %+v, want ok", result)
	}
	if repo.llm.APIKey != "sk-test-12345678" {
		t.Fatalf("saved api key = %q, want existing key preserved", repo.llm.APIKey)
	}
	if repo.llm.ThinkingType != "" || repo.llm.ToolStream {
		t.Fatalf("openai saved provider-specific fields thinking=%q toolStream=%v, want empty/false", repo.llm.ThinkingType, repo.llm.ToolStream)
	}
}

func TestSettingsServiceDefaultModelOptionsIncludeGLM47(t *testing.T) {
	svc := NewSettingsService(&settingsRepoStub{})

	settings, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	for _, option := range settings.Models {
		if option.ID == "glm-4.7" && option.Name == "GLM-4.7" {
			return
		}
	}
	t.Fatalf("default model options = %+v, want GLM-4.7 option", settings.Models)
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
