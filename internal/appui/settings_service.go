package appui

import (
	"context"
	"strings"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/modelrouter"
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
	payload.ReasoningEffort = normalizeReasoningEffort(payload.ReasoningEffort)
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
		next.ReasoningEffort = normalizeReasoningEffort(trimmed)
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
		Provider:         "openai",
		Model:            "gpt-5.4",
		MaxContextTokens: 200000,
		MaxOutputTokens:  20000,
		ReasoningEffort:  "medium",
		CompactModel:     "gpt-5.4-mini",
		BifrostActive:    false,
	}
	if s.repo == nil {
		return view, nil
	}
	cfg, err := s.repo.GetLLMConfig()
	if err != nil || cfg == nil {
		return view, nil
	}
	normalized := normalizeStoredLLMConfig(cfg)
	view.Provider = normalized.Provider
	view.Model = normalized.Model
	view.BaseURL = normalized.BaseURL
	view.MaxContextTokens = normalized.MaxContextTokens
	view.MaxOutputTokens = normalized.MaxOutputTokens
	view.Temperature = cloneFloat64Ptr(normalized.Temperature)
	view.TopP = cloneFloat64Ptr(normalized.TopP)
	view.ThinkingType = normalized.ThinkingType
	view.ReasoningEffort = normalized.ReasoningEffort
	view.ToolStream = normalized.ToolStream
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
		Provider:         "openai",
		Model:            "gpt-5.4",
		MaxContextTokens: 200000,
		MaxOutputTokens:  20000,
		ReasoningEffort:  "medium",
		CompactModel:     "gpt-5.4-mini",
	}
	if current != nil {
		*next = *current
	}
	if trimmed := strings.TrimSpace(payload.APIKey); trimmed != "" {
		next.APIKey = trimmed
	}
	next.FallbackProvider = strings.TrimSpace(payload.FallbackProvider)
	next.FallbackModel = strings.TrimSpace(payload.FallbackModel)
	if trimmed := strings.TrimSpace(payload.FallbackAPIKey); trimmed != "" {
		next.FallbackAPIKey = trimmed
	}
	if trimmed := strings.TrimSpace(payload.CompactModel); trimmed != "" {
		next.CompactModel = trimmed
	}
	next = normalizeLLMConfigUpdate(next, payload)
	if err := s.repo.SaveLLMConfig(next); err != nil {
		return LLMConfigUpdateResult{}, err
	}
	webDefaults := defaultWebSettingsPayload()
	webSettings, _ := s.repo.GetWebSettings()
	nextWebSettings := &store.WebSettings{
		Quota:           webDefaults.Quota,
		Model:           strings.TrimSpace(firstNonEmpty(next.Model, "gpt-5.4")),
		ReasoningEffort: next.ReasoningEffort,
		Models:          append([]store.SettingModelOption(nil), webDefaults.Models...),
	}
	if webSettings != nil {
		*nextWebSettings = *webSettings
		nextWebSettings.Models = append([]store.SettingModelOption(nil), webSettings.Models...)
		nextWebSettings.Model = strings.TrimSpace(firstNonEmpty(next.Model, nextWebSettings.Model, "gpt-5.4"))
		nextWebSettings.ReasoningEffort = next.ReasoningEffort
		if len(nextWebSettings.Models) == 0 {
			nextWebSettings.Models = append([]store.SettingModelOption(nil), webDefaults.Models...)
		}
	}
	if err := s.repo.SaveWebSettings(nextWebSettings); err != nil {
		return LLMConfigUpdateResult{}, err
	}
	s.syncAuthFromLLMConfig(ctx, next)
	return LLMConfigUpdateResult{
		OK:               true,
		Message:          "配置已保存。",
		MaxContextTokens: next.MaxContextTokens,
		MaxOutputTokens:  next.MaxOutputTokens,
	}, nil
}

func normalizeLLMContextWindow(value int) int {
	if value <= 0 {
		return 200000
	}
	if value < 10000 {
		return 10000
	}
	return value
}

func normalizeStoredLLMConfig(cfg *store.LLMConfig) store.LLMConfig {
	if cfg == nil {
		return store.LLMConfig{
			Provider:         "openai",
			Model:            "gpt-5.4",
			MaxContextTokens: 200000,
			MaxOutputTokens:  20000,
			ReasoningEffort:  "medium",
		}
	}
	cp := *cfg
	cp.Temperature = cloneFloat64Ptr(cfg.Temperature)
	cp.TopP = cloneFloat64Ptr(cfg.TopP)
	return *normalizeLLMConfigUpdate(&cp, LLMConfigUpdate{
		Provider:         cp.Provider,
		Model:            cp.Model,
		BaseURL:          cp.BaseURL,
		MaxContextTokens: cp.MaxContextTokens,
		MaxOutputTokens:  cp.MaxOutputTokens,
		Temperature:      cp.Temperature,
		TopP:             cp.TopP,
		ThinkingType:     cp.ThinkingType,
		ReasoningEffort:  cp.ReasoningEffort,
		ToolStream:       cp.ToolStream,
	})
}

func normalizeLLMConfigUpdate(base *store.LLMConfig, payload LLMConfigUpdate) *store.LLMConfig {
	next := &store.LLMConfig{}
	if base != nil {
		*next = *base
		next.Temperature = cloneFloat64Ptr(base.Temperature)
		next.TopP = cloneFloat64Ptr(base.TopP)
	}

	oldProvider := modelrouter.NormalizeProviderID(next.Provider)
	requestedProvider := strings.TrimSpace(payload.Provider)
	provider := modelrouter.NormalizeProviderID(firstNonEmpty(requestedProvider, next.Provider, "openai"))
	providerChanged := requestedProvider != "" && provider != oldProvider
	next.Provider = provider
	model := strings.TrimSpace(payload.Model)
	if model == "" && !providerChanged {
		model = strings.TrimSpace(next.Model)
	}
	if model == "" {
		model = modelrouter.DefaultModelForProvider(provider)
	}
	if model == "" {
		model = "gpt-5.4"
	}
	next.Model = model

	if baseURL := strings.TrimSpace(payload.BaseURL); baseURL != "" {
		next.BaseURL = baseURL
	} else if provider != "openai" && (providerChanged || strings.TrimSpace(next.BaseURL) == "") {
		next.BaseURL = modelrouter.DefaultBaseURLForProvider(provider)
	} else {
		next.BaseURL = strings.TrimSpace(next.BaseURL)
	}

	if payload.Temperature != nil {
		next.Temperature = cloneFloat64Ptr(payload.Temperature)
	} else if providerChanged {
		next.Temperature = nil
	}
	if payload.TopP != nil {
		next.TopP = cloneFloat64Ptr(payload.TopP)
	} else if providerChanged {
		next.TopP = nil
	}

	contextValue := payload.MaxContextTokens
	if contextValue <= 0 && !providerChanged {
		contextValue = next.MaxContextTokens
	}
	next.MaxContextTokens = normalizeLLMContextWindowForModel(provider, model, contextValue)

	outputValue := payload.MaxOutputTokens
	if outputValue <= 0 && !providerChanged {
		outputValue = next.MaxOutputTokens
	}
	next.MaxOutputTokens = normalizeLLMMaxOutputTokens(provider, model, outputValue)
	next.ThinkingType = modelrouter.NormalizeThinkingType(provider, firstNonEmpty(payload.ThinkingType, boolString(!providerChanged, next.ThinkingType)))
	next.ReasoningEffort = normalizeLLMReasoningEffort(provider, model, firstNonEmpty(payload.ReasoningEffort, boolString(!providerChanged, next.ReasoningEffort)))
	next.ToolStream = normalizeLLMToolStream(provider, payload.ToolStream)
	return next
}

func normalizeLLMContextWindowForModel(provider, model string, value int) int {
	if value > 0 {
		if preset, ok := modelrouter.ModelPresetByID(provider, model); shouldClampLLMConfigToModelPreset(provider) && ok && preset.MaxContextTokens > 0 && value > preset.MaxContextTokens {
			return preset.MaxContextTokens
		}
		return normalizeLLMContextWindow(value)
	}
	if preset, ok := modelrouter.ModelPresetByID(provider, model); ok && preset.MaxContextTokens > 0 {
		return preset.MaxContextTokens
	}
	return normalizeLLMContextWindow(value)
}

func normalizeLLMMaxOutputTokens(provider, model string, value int) int {
	preset, ok := modelrouter.ModelPresetByID(provider, model)
	cap := 0
	defaultValue := 20000
	if ok {
		if preset.MaxOutputTokens > 0 {
			cap = preset.MaxOutputTokens
		}
		if preset.DefaultMaxTokens > 0 {
			defaultValue = preset.DefaultMaxTokens
		}
	}
	if value <= 0 {
		value = defaultValue
	}
	if value < 1 {
		return 1
	}
	if shouldClampLLMConfigToModelPreset(provider) && cap > 0 && value > cap {
		return cap
	}
	return value
}

func shouldClampLLMConfigToModelPreset(provider string) bool {
	switch modelrouter.NormalizeProviderID(provider) {
	case "deepseek", "zhipu":
		return true
	default:
		return false
	}
}

func normalizeLLMReasoningEffort(provider, model string, value string) string {
	switch modelrouter.NormalizeProviderID(provider) {
	case "deepseek", "zhipu":
		return modelrouter.NormalizeReasoningEffortForProvider(provider, model, value)
	default:
		return normalizeReasoningEffort(value)
	}
}

func normalizeLLMToolStream(provider string, value bool) bool {
	preset, ok := modelrouter.ProviderPresetByID(provider)
	return ok && preset.SupportsToolStream && value
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cp := *value
	return &cp
}

func boolString(ok bool, value string) string {
	if !ok {
		return ""
	}
	return value
}

func normalizeReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "low"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "medium"
	}
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
			{ID: "glm-4.7", Name: "GLM-4.7"},
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
		base.ReasoningEffort = normalizeReasoningEffort(trimmed)
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
	case "glm-4.7":
		label = "GLM-4.7"
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
