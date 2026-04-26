package appui

import (
	"context"
	"strings"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/store"
)

type defaultSettingsService struct {
	repo        SettingsRepository
	authManager *auth.Manager
}

func NewSettingsService(repo SettingsRepository, managers ...*auth.Manager) SettingsService {
	var manager *auth.Manager
	if len(managers) > 0 {
		manager = managers[0]
	}
	service := &defaultSettingsService{repo: repo, authManager: manager}
	if repo != nil && manager != nil {
		if cfg, err := repo.GetLLMConfig(); err == nil {
			service.syncAuthFromLLMConfig(context.Background(), cfg)
		}
	}
	return service
}

func (s *defaultSettingsService) GetSettings(context.Context) (WebSettingsPayload, error) {
	payload := defaultWebSettingsPayload()
	if s.repo == nil {
		return payload, nil
	}
	if current, err := s.repo.GetWebSettings(); err == nil && current != nil {
		payload = mergeWebSettings(payload, *current)
	}
	if llm, err := s.repo.GetLLMConfig(); err == nil && llm != nil {
		if trimmed := strings.TrimSpace(llm.Model); trimmed != "" {
			payload.Model = trimmed
		}
	}
	if len(payload.Models) == 0 {
		payload.Models = append([]store.SettingModelOption(nil), defaultWebSettingsPayload().Models...)
	}
	if strings.TrimSpace(payload.Model) == "" {
		payload.Model = "gpt-5.4"
	}
	payload.Models = ensureSettingModelOption(payload.Models, payload.Model)
	if strings.TrimSpace(payload.ReasoningEffort) == "" {
		payload.ReasoningEffort = "medium"
	}
	return payload, nil
}

func (s *defaultSettingsService) UpdateSettings(ctx context.Context, payload WebSettingsPayload) (WebSettingsPayload, error) {
	current, err := s.GetSettings(ctx)
	if err != nil {
		return WebSettingsPayload{}, err
	}
	next := current
	if trimmed := strings.TrimSpace(payload.Quota); trimmed != "" {
		next.Quota = trimmed
	}
	if trimmed := strings.TrimSpace(payload.Model); trimmed != "" {
		next.Model = trimmed
	}
	if trimmed := strings.TrimSpace(payload.ReasoningEffort); trimmed != "" {
		next.ReasoningEffort = trimmed
	}
	if len(payload.Models) > 0 {
		next.Models = append([]store.SettingModelOption(nil), payload.Models...)
	}
	if s.repo == nil {
		return next, nil
	}
	if err := s.repo.SaveWebSettings(&store.WebSettings{
		Quota:           next.Quota,
		Model:           next.Model,
		ReasoningEffort: next.ReasoningEffort,
		Models:          append([]store.SettingModelOption(nil), next.Models...),
	}); err != nil {
		return WebSettingsPayload{}, err
	}
	return s.GetSettings(ctx)
}

func (s *defaultSettingsService) GetLLMConfig(context.Context) (LLMConfigView, error) {
	view := LLMConfigView{
		Provider:      "openai",
		Model:         "gpt-5.4",
		CompactModel:  "gpt-5.4-mini",
		BifrostActive: false,
	}
	if s.repo == nil {
		return view, nil
	}
	cfg, err := s.repo.GetLLMConfig()
	if err != nil || cfg == nil {
		return view, nil
	}
	view.Provider = strings.TrimSpace(firstNonEmpty(cfg.Provider, view.Provider))
	view.Model = strings.TrimSpace(firstNonEmpty(cfg.Model, view.Model))
	view.BaseURL = strings.TrimSpace(cfg.BaseURL)
	view.FallbackProvider = strings.TrimSpace(cfg.FallbackProvider)
	view.FallbackModel = strings.TrimSpace(cfg.FallbackModel)
	view.CompactModel = strings.TrimSpace(firstNonEmpty(cfg.CompactModel, view.CompactModel))
	view.APIKeySet = strings.TrimSpace(cfg.APIKey) != ""
	view.APIKeyMasked = maskSecret(cfg.APIKey)
	view.BifrostActive = strings.TrimSpace(view.Provider) != "" && strings.TrimSpace(view.Model) != ""
	return view, nil
}

func (s *defaultSettingsService) UpdateLLMConfig(ctx context.Context, payload LLMConfigUpdate) (LLMConfigUpdateResult, error) {
	if s.repo == nil {
		return LLMConfigUpdateResult{OK: false, Error: "settings repository is not configured"}, nil
	}
	current, _ := s.repo.GetLLMConfig()
	next := &store.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-5.4",
		CompactModel: "gpt-5.4-mini",
	}
	if current != nil {
		*next = *current
	}
	if trimmed := strings.TrimSpace(payload.Provider); trimmed != "" {
		next.Provider = trimmed
	}
	if trimmed := strings.TrimSpace(payload.Model); trimmed != "" {
		next.Model = trimmed
	}
	if trimmed := strings.TrimSpace(payload.APIKey); trimmed != "" {
		next.APIKey = trimmed
	}
	next.BaseURL = strings.TrimSpace(payload.BaseURL)
	next.FallbackProvider = strings.TrimSpace(payload.FallbackProvider)
	next.FallbackModel = strings.TrimSpace(payload.FallbackModel)
	if trimmed := strings.TrimSpace(payload.FallbackAPIKey); trimmed != "" {
		next.FallbackAPIKey = trimmed
	}
	if trimmed := strings.TrimSpace(payload.CompactModel); trimmed != "" {
		next.CompactModel = trimmed
	}
	if err := s.repo.SaveLLMConfig(next); err != nil {
		return LLMConfigUpdateResult{}, err
	}
	webDefaults := defaultWebSettingsPayload()
	webSettings, _ := s.repo.GetWebSettings()
	nextWebSettings := &store.WebSettings{
		Quota:           webDefaults.Quota,
		Model:           strings.TrimSpace(firstNonEmpty(next.Model, "gpt-5.4")),
		ReasoningEffort: webDefaults.ReasoningEffort,
		Models:          append([]store.SettingModelOption(nil), webDefaults.Models...),
	}
	if webSettings != nil {
		*nextWebSettings = *webSettings
		nextWebSettings.Models = append([]store.SettingModelOption(nil), webSettings.Models...)
		nextWebSettings.Model = strings.TrimSpace(firstNonEmpty(next.Model, nextWebSettings.Model, "gpt-5.4"))
		if len(nextWebSettings.Models) == 0 {
			nextWebSettings.Models = append([]store.SettingModelOption(nil), webDefaults.Models...)
		}
	}
	if err := s.repo.SaveWebSettings(nextWebSettings); err != nil {
		return LLMConfigUpdateResult{}, err
	}
	s.syncAuthFromLLMConfig(ctx, next)
	return LLMConfigUpdateResult{
		OK:      true,
		Message: "配置已保存。",
	}, nil
}

func (s *defaultSettingsService) syncAuthFromLLMConfig(ctx context.Context, cfg *store.LLMConfig) {
	if s == nil || s.authManager == nil || cfg == nil {
		return
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return
	}
	resp := s.authManager.Login(ctx, auth.LoginRequest{
		Mode:     auth.ModeAPIKey,
		APIKey:   apiKey,
		PlanType: strings.TrimSpace(firstNonEmpty(cfg.Provider, "configured")),
	})
	setAuthSummary(resp.Summary)
}

func defaultWebSettingsPayload() WebSettingsPayload {
	return WebSettingsPayload{
		Quota:           "Unlimited",
		Model:           "gpt-5.4",
		ReasoningEffort: "medium",
		Models: []store.SettingModelOption{
			{ID: "gpt-5.4", Name: "GPT-5.4"},
			{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
			{ID: "claude-sonnet-4", Name: "Claude Sonnet 4"},
		},
	}
}

func mergeWebSettings(base WebSettingsPayload, incoming store.WebSettings) WebSettingsPayload {
	if trimmed := strings.TrimSpace(incoming.Quota); trimmed != "" {
		base.Quota = trimmed
	}
	if trimmed := strings.TrimSpace(incoming.Model); trimmed != "" {
		base.Model = trimmed
	}
	if trimmed := strings.TrimSpace(incoming.ReasoningEffort); trimmed != "" {
		base.ReasoningEffort = trimmed
	}
	if len(incoming.Models) > 0 {
		base.Models = append([]store.SettingModelOption(nil), incoming.Models...)
	}
	return base
}

func ensureSettingModelOption(models []store.SettingModelOption, model string) []store.SettingModelOption {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return models
	}
	for _, option := range models {
		if strings.TrimSpace(option.ID) == trimmed {
			return models
		}
	}
	label := trimmed
	switch trimmed {
	case "gpt-5.4":
		label = "GPT-5.4"
	case "gpt-5.4-mini":
		label = "GPT-5.4 Mini"
	case "claude-sonnet-4":
		label = "Claude Sonnet 4"
	}
	return append(append([]store.SettingModelOption(nil), models...), store.SettingModelOption{
		ID:   trimmed,
		Name: label,
	})
}

func maskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 8 {
		return "****"
	}
	return trimmed[:4] + "****" + trimmed[len(trimmed)-4:]
}
