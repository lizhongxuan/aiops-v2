package appui

import (
	"context"
	"fmt"
	"testing"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/store"
)

type settingsRepoStub struct {
	web            *store.WebSettings
	llm            *store.LLMConfig
	runtime        *store.RuntimeSettings
	saveRuntimeErr error
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

func (s *settingsRepoStub) GetRuntimeSettings() (*store.RuntimeSettings, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("runtime settings not found")
	}
	cp := store.NormalizeRuntimeSettings(*s.runtime)
	return &cp, nil
}

func (s *settingsRepoStub) SaveRuntimeSettings(settings *store.RuntimeSettings) error {
	if s.saveRuntimeErr != nil {
		return s.saveRuntimeErr
	}
	cp := store.NormalizeRuntimeSettings(*settings)
	s.runtime = &cp
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

func TestSettingsServiceRuntimeSettingsDefaultsAndPartialUpdate(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)

	initial, err := svc.GetRuntimeSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeSettings() error = %v", err)
	}
	if initial.Settings.AgentRuntime.IntentFrameRouting != "trace_only" || !initial.Settings.Debug.ModelInputTrace {
		t.Fatalf("initial settings = %+v, want defaults", initial.Settings)
	}

	enabled := true
	provider := "docker"
	image := "python:3.12-bookworm"
	updated, err := svc.UpdateRuntimeSettings(context.Background(), RuntimeSettingsUpdate{
		Tooling: &RuntimeToolingSettingsUpdate{
			ReadOnlyRetryEnabled: &enabled,
		},
		Workflow: &RuntimeWorkflowSettingsUpdate{
			ValidationProvider: &provider,
			ValidationImage:    &image,
		},
	})
	if err != nil {
		t.Fatalf("UpdateRuntimeSettings() error = %v", err)
	}
	if !updated.Settings.Tooling.ReadOnlyRetryEnabled || updated.Settings.Tooling.ReadOnlyRetryMaxPerTurn != 3 {
		t.Fatalf("updated tooling = %+v, want partial update preserving defaults", updated.Settings.Tooling)
	}
	if updated.Settings.Workflow.ValidationProvider != "docker" || updated.Settings.Workflow.ValidationImage != "python:3.12-bookworm" {
		t.Fatalf("updated workflow = %+v, want docker/bookworm", updated.Settings.Workflow)
	}
	if repo.runtime == nil || repo.runtime.Workflow.ValidationProvider != "docker" {
		t.Fatalf("repo.runtime = %+v, want saved runtime settings", repo.runtime)
	}
	if snapshotter, ok := svc.(RuntimeSettingsProvider); !ok {
		t.Fatalf("settings service does not implement RuntimeSettingsProvider")
	} else if snapshotter.Snapshot(context.Background()).Workflow.ValidationProvider != "docker" {
		t.Fatalf("provider snapshot = %+v, want saved settings", snapshotter.Snapshot(context.Background()))
	}
}

func TestSettingsServiceRuntimeSettingsNormalizesInvalidPartialUpdate(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)

	route := "bad"
	retries := 99
	maxBackoff := -2
	updated, err := svc.UpdateRuntimeSettings(context.Background(), RuntimeSettingsUpdate{
		AgentRuntime: &RuntimeAgentSettingsUpdate{
			IntentFrameRouting: &route,
		},
		Tooling: &RuntimeToolingSettingsUpdate{
			ReadOnlyRetryMaxPerTurn:   &retries,
			ReadOnlyRetryBackoffMaxMs: &maxBackoff,
		},
	})
	if err != nil {
		t.Fatalf("UpdateRuntimeSettings() error = %v", err)
	}
	if updated.Settings.AgentRuntime.IntentFrameRouting != "trace_only" {
		t.Fatalf("IntentFrameRouting = %q, want trace_only", updated.Settings.AgentRuntime.IntentFrameRouting)
	}
	if updated.Settings.Tooling.ReadOnlyRetryMaxPerTurn != 10 || updated.Settings.Tooling.ReadOnlyRetryBackoffMaxMs != 2000 {
		t.Fatalf("tooling = %+v, want normalized retry values", updated.Settings.Tooling)
	}
}

func TestSettingsServiceRuntimeSettingsSaveFailureDoesNotUpdateSnapshot(t *testing.T) {
	initial := store.DefaultRuntimeSettings()
	repo := &settingsRepoStub{runtime: &initial, saveRuntimeErr: fmt.Errorf("write failed")}
	svc := NewSettingsService(repo)
	before := svc.(RuntimeSettingsProvider).Snapshot(context.Background())

	route := "active"
	_, err := svc.UpdateRuntimeSettings(context.Background(), RuntimeSettingsUpdate{
		AgentRuntime: &RuntimeAgentSettingsUpdate{IntentFrameRouting: &route},
	})
	if err == nil {
		t.Fatal("UpdateRuntimeSettings() error = nil, want save failure")
	}
	after := svc.(RuntimeSettingsProvider).Snapshot(context.Background())
	if before.AgentRuntime.IntentFrameRouting != after.AgentRuntime.IntentFrameRouting {
		t.Fatalf("snapshot changed after save failure: before=%+v after=%+v", before, after)
	}
}

func TestSettingsServiceRuntimeSettingsNotifiesListenersAfterSave(t *testing.T) {
	repo := &settingsRepoStub{}
	svc := NewSettingsService(repo)
	registrar, ok := svc.(RuntimeSettingsListenerRegistrar)
	if !ok {
		t.Fatal("settings service does not implement RuntimeSettingsListenerRegistrar")
	}
	var observed []store.RuntimeSettings
	registrar.AddRuntimeSettingsListener(func(settings store.RuntimeSettings) {
		observed = append(observed, settings)
	})

	mode := "warning"
	if _, err := svc.UpdateRuntimeSettings(context.Background(), RuntimeSettingsUpdate{
		Workflow: &RuntimeWorkflowSettingsUpdate{ReferenceGuardMode: &mode},
	}); err != nil {
		t.Fatalf("UpdateRuntimeSettings() error = %v", err)
	}
	if len(observed) != 1 || observed[0].Workflow.ReferenceGuardMode != "warning" {
		t.Fatalf("observed = %+v, want one warning snapshot", observed)
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
