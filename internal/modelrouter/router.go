package modelrouter

import (
	"context"
	"strings"

	"aiops-v2/internal/auth"
	"github.com/cloudwego/eino/components/model"
)

// ---------------------------------------------------------------------------
// AgentKind identifies the type of agent requesting a model.
// Re-exported locally to avoid circular imports with promptcompiler.
// ---------------------------------------------------------------------------

// AgentKind identifies the agent type for model routing decisions.
type AgentKind string

const (
	AgentKindPlanner AgentKind = "planner"
	AgentKindWorker  AgentKind = "worker"
)

// ---------------------------------------------------------------------------
// ChatModel is the interface used by the router for LLM providers.
// It uses the real Eino model.ChatModel interface which supports
// Generate, Stream, and BindTools.
// ---------------------------------------------------------------------------

// ChatModel is an alias for the Eino ChatModel interface that supports
// tool calling via BindTools. This is the interface that all provider
// implementations must satisfy.
type ChatModel = model.ChatModel

// ---------------------------------------------------------------------------
// ProviderConfig holds the configuration for a specific LLM provider/model.
// ---------------------------------------------------------------------------

// ProviderConfig specifies which provider and model to use, along with
// generation parameters.
type ProviderConfig struct {
	// Provider identifies the LLM provider: "openai", "deepseek", "zhipu", "anthropic", "ollama".
	Provider string

	// Model is the specific model name, e.g. "gpt-4o", "claude-3-5-sonnet", "llama3".
	Model string

	// BaseURL overrides the provider endpoint, e.g. an OpenAI-compatible gateway.
	BaseURL string

	// Temperature controls randomness in generation (0.0 – 1.0+).
	Temperature float64

	// TopP controls nucleus sampling.
	TopP float64

	// MaxTokens limits the maximum number of tokens in the response.
	MaxTokens int

	// MaxContextTokens overrides the model context window when a gateway or
	// operator exposes a custom limit. Empty config defaults to 200K.
	MaxContextTokens int

	// RequestTimeoutMs controls model request timeout in milliseconds.
	RequestTimeoutMs int

	// ReasoningEffort controls provider-native reasoning effort where supported.
	ReasoningEffort string

	// ThinkingType controls OpenAI-compatible provider-specific thinking mode.
	ThinkingType string

	// ToolStream enables provider-specific streaming details for tool calls.
	ToolStream bool

	// ExtraFields carries provider-specific OpenAI-compatible request fields.
	ExtraFields map[string]any
}

const GenericReasoningFallbackPolicy = "Reasoning fallback policy: decompose the goal, list assumptions, gather evidence before conclusions, cover key claims with evidence, and state the blocker when progress cannot continue. Do not expose raw reasoning."

// ModelCapabilities describes the routing-visible context and generation
// capabilities for a provider/model pair. Runtime callers use this to choose
// context governance budgets without hard-coding provider-specific constants.
type ModelCapabilities struct {
	Provider                 string `json:"provider"`
	Model                    string `json:"model"`
	MaxContextTokens         int    `json:"maxContextTokens"`
	MaxOutputTokens          int    `json:"maxOutputTokens"`
	ExactTokenCount          bool   `json:"exactTokenCount"`
	CacheEdit                bool   `json:"cacheEdit"`
	SmallContextMode         bool   `json:"smallContextMode"`
	SupportsReasoning        bool   `json:"supportsReasoning,omitempty"`
	SupportsToolCalls        bool   `json:"supportsToolCalls,omitempty"`
	SupportsStreaming        bool   `json:"supportsStreaming,omitempty"`
	SupportsNativeWebTool    bool   `json:"supportsNativeWebTool,omitempty"`
	NativeReasoning          bool   `json:"nativeReasoning,omitempty"`
	ReasoningEffortRequested string `json:"reasoningEffortRequested,omitempty"`
	ReasoningEffortApplied   string `json:"reasoningEffortApplied,omitempty"`
	ReasoningFallbackPolicy  string `json:"reasoningFallbackPolicy,omitempty"`
}

// ---------------------------------------------------------------------------
// FallbackEntry defines a primary → fallback provider mapping.
// ---------------------------------------------------------------------------

// FallbackEntry maps a primary provider to its fallback. When the primary
// provider fails, the Router automatically switches to the fallback.
type FallbackEntry struct {
	// Primary is the preferred provider name.
	Primary string

	// Fallback is the backup provider name used when Primary fails.
	Fallback string
}

// ---------------------------------------------------------------------------
// AgentKindConfig defines per-AgentKind model routing preferences.
// ---------------------------------------------------------------------------

// AgentKindConfig specifies the preferred provider and model for a given
// AgentKind. Worker agents typically use cheaper/faster models while
// Planner agents use stronger reasoning models.
type AgentKindConfig struct {
	// Provider is the preferred provider for this AgentKind.
	Provider string

	// Model is the preferred model name for this AgentKind.
	Model string

	// Temperature override for this AgentKind (0 means use ProviderConfig value).
	Temperature float64

	// MaxTokens override for this AgentKind (0 means use ProviderConfig value).
	MaxTokens int
}

// ---------------------------------------------------------------------------
// Router is the lightweight provider router that selects and returns
// ChatModel instances based on AgentKind and ProviderConfig.
// ---------------------------------------------------------------------------

// Router manages LLM provider instances and implements provider selection
// with fallback support. It replaces the Bifrost middle layer.
type Router struct {
	// providers maps provider names to their ChatModel instances.
	providers map[string]ChatModel

	// providerFactories lazily construct ChatModel instances when no prebuilt
	// provider is registered.
	providerFactories map[string]ProviderFactory

	// defaultProvider is the provider used when no specific provider is configured.
	defaultProvider string

	// fallbacks defines the fallback chain for provider failover.
	fallbacks []FallbackEntry

	// resolver exposes the current credential truth for auth-backed providers.
	resolver auth.Resolver

	// configResolver exposes live provider/model settings from the product UI.
	configResolver ProviderConfigResolver

	// agentKindConfigs maps AgentKind to its preferred routing configuration.
	// When set, the AgentKind config takes precedence over the ProviderConfig
	// if ProviderConfig.Provider is empty.
	agentKindConfigs map[AgentKind]AgentKindConfig
}

// ProviderFactory constructs a provider model on demand.
type ProviderFactory func(ctx context.Context, agentKind AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error)

// ProviderConfigResolver resolves the current product-level LLM config. When a
// model gateway does not expose context metadata, MaxContextTokens should be
// set by the product-level settings and defaults to 200K.
type ProviderConfigResolver interface {
	ResolveProviderConfig(agentKind AgentKind) (ProviderConfig, bool)
}

// NewRouter creates a Router with the given default provider, registered
// providers, and fallback chain.
func NewRouter(defaultProvider string, providers map[string]ChatModel, fallbacks []FallbackEntry) *Router {
	if providers == nil {
		providers = make(map[string]ChatModel)
	}
	return &Router{
		providers:         providers,
		providerFactories: defaultProviderFactories(),
		defaultProvider:   defaultProvider,
		fallbacks:         fallbacks,
		agentKindConfigs:  make(map[AgentKind]AgentKindConfig),
	}
}

// SetCredentialResolver wires the current auth truth source into the router.
func (r *Router) SetCredentialResolver(resolver auth.Resolver) {
	r.resolver = resolver
}

// SetProviderConfigResolver wires live provider/model settings into routing.
func (r *Router) SetProviderConfigResolver(resolver ProviderConfigResolver) {
	r.configResolver = resolver
}

// SetProviderFactory registers or replaces the lazy factory for a provider.
func (r *Router) SetProviderFactory(provider string, factory ProviderFactory) {
	if r.providerFactories == nil {
		r.providerFactories = make(map[string]ProviderFactory)
	}
	r.providerFactories[strings.TrimSpace(provider)] = factory
}

// SetAgentKindConfig registers a per-AgentKind routing configuration.
// This allows Worker agents to use cheaper models and Planner agents
// to use stronger reasoning models.
func (r *Router) SetAgentKindConfig(kind AgentKind, config AgentKindConfig) {
	r.agentKindConfigs[kind] = config
}

// GetModel returns a ChatModel instance for the given AgentKind
// and ProviderConfig. Resolution order:
//  1. If ProviderConfig.Provider is set explicitly, use it.
//  2. If product-level LLM settings are configured, use them.
//  3. If AgentKindConfig is registered for the AgentKind, use its Provider.
//  4. Fall back to the Router's defaultProvider.
//  5. If the resolved provider is not prebuilt, try a lazy provider factory
//     with the current credential truth.
//  6. If the resolved candidate still cannot be satisfied, try the fallback chain.
//  7. If all options are exhausted, return ProviderNotFoundError.
func (r *Router) GetModel(agentKind AgentKind, config ProviderConfig) (ChatModel, error) {
	config = r.resolveEffectiveProviderConfig(agentKind, config)
	provider := r.resolveProvider(agentKind, config)
	candidates := r.resolveCandidates(provider)
	truth, hasTruth := r.resolveCredentialTruth()

	for _, candidate := range candidates {
		if m, ok := r.providers[candidate]; ok {
			return m, nil
		}
		if m, ok := r.constructProvider(candidate, agentKind, config, truth, hasTruth); ok {
			return m, nil
		}
	}

	return nil, &ProviderNotFoundError{Provider: provider}
}

// ResolveModelCapabilities returns the best-known capabilities for the
// effective provider/model route without constructing a ChatModel.
func (r *Router) ResolveModelCapabilities(agentKind AgentKind, config ProviderConfig) ModelCapabilities {
	config = r.resolveEffectiveProviderConfig(agentKind, config)
	provider := r.resolveProvider(agentKind, config)
	model := resolveProviderModel(provider, config)
	return capabilitiesForProviderModel(provider, model, config)
}

func (r *Router) ResolveEffectiveProviderConfig(agentKind AgentKind, config ProviderConfig) ProviderConfig {
	if r == nil {
		return config
	}
	return r.resolveEffectiveProviderConfig(agentKind, config)
}

func (r *Router) resolveEffectiveProviderConfig(agentKind AgentKind, config ProviderConfig) ProviderConfig {
	if r.configResolver == nil {
		return config
	}
	resolved, ok := r.configResolver.ResolveProviderConfig(agentKind)
	if !ok {
		return config
	}
	if strings.TrimSpace(config.Provider) != "" {
		resolved.Provider = config.Provider
	}
	if strings.TrimSpace(config.Model) != "" {
		resolved.Model = config.Model
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		resolved.BaseURL = config.BaseURL
	}
	if config.Temperature > 0 {
		resolved.Temperature = config.Temperature
	}
	if config.TopP > 0 {
		resolved.TopP = config.TopP
	}
	if config.MaxTokens > 0 {
		resolved.MaxTokens = config.MaxTokens
	}
	if config.MaxContextTokens > 0 {
		resolved.MaxContextTokens = config.MaxContextTokens
	}
	if config.RequestTimeoutMs > 0 {
		resolved.RequestTimeoutMs = config.RequestTimeoutMs
	}
	if strings.TrimSpace(config.ReasoningEffort) != "" {
		resolved.ReasoningEffort = config.ReasoningEffort
	}
	if strings.TrimSpace(config.ThinkingType) != "" {
		resolved.ThinkingType = config.ThinkingType
	}
	if config.ToolStream {
		resolved.ToolStream = true
	}
	if len(config.ExtraFields) > 0 {
		resolved.ExtraFields = cloneExtraFields(config.ExtraFields)
	}
	return resolved
}

func (r *Router) resolveCandidates(provider string) []string {
	if provider == "" {
		return nil
	}
	candidates := []string{provider}
	for _, fb := range r.fallbacks {
		if fb.Primary == provider && strings.TrimSpace(fb.Fallback) != "" {
			candidates = append(candidates, fb.Fallback)
		}
	}
	return candidates
}

func (r *Router) resolveCredentialTruth() (auth.CredentialTruth, bool) {
	if r.resolver == nil {
		return auth.CredentialTruth{}, false
	}
	return r.resolver.Resolve()
}

func (r *Router) constructProvider(provider string, agentKind AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, bool) {
	if r.providerFactories == nil {
		return nil, false
	}
	factory, ok := r.providerFactories[provider]
	if !ok || factory == nil {
		return nil, false
	}
	model, err := factory(context.Background(), agentKind, config, truth, hasTruth)
	if err != nil {
		return nil, false
	}
	return model, true
}

// resolveProvider determines the target provider name based on the resolution
// priority: explicit config > AgentKind config > default.
func (r *Router) resolveProvider(agentKind AgentKind, config ProviderConfig) string {
	// 1. Explicit provider in ProviderConfig takes highest priority.
	if config.Provider != "" {
		return config.Provider
	}

	// 2. Per-AgentKind configuration.
	if akc, ok := r.agentKindConfigs[agentKind]; ok && akc.Provider != "" {
		return akc.Provider
	}

	// 3. Default provider.
	return r.defaultProvider
}

func defaultProviderFactories() map[string]ProviderFactory {
	return map[string]ProviderFactory{
		"openai":    buildOpenAIProviderModel,
		"deepseek":  buildDeepSeekProviderModel,
		"zhipu":     buildZhipuProviderModel,
		"anthropic": buildAnthropicProviderModel,
	}
}

func buildOpenAIProviderModel(_ context.Context, _ AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
	return buildOpenAICompatibleProviderModel("openai", config, truth, hasTruth)
}

func buildDeepSeekProviderModel(_ context.Context, _ AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
	return buildOpenAICompatibleProviderModel("deepseek", config, truth, hasTruth)
}

func buildZhipuProviderModel(_ context.Context, _ AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
	return buildOpenAICompatibleProviderModel("zhipu", config, truth, hasTruth)
}

func buildOpenAICompatibleProviderModel(provider string, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
	apiKey := resolveProviderSecret(truth, hasTruth)
	if strings.TrimSpace(apiKey) == "" {
		return nil, &ProviderNotAvailableError{
			Provider: provider,
			Model:    resolveProviderModel(provider, config),
			Reason:   "no credential truth available",
		}
	}
	extraFields := openAICompatibleExtraFields(provider, config)
	extraFields = mergeExtraFields(config.ExtraFields, extraFields)
	return NewOpenAIChatModel(context.Background(), OpenAIConfig{
		Provider:        provider,
		APIKey:          apiKey,
		BaseURL:         strings.TrimSpace(config.BaseURL),
		Model:           resolveProviderModel(provider, config),
		Temperature:     resolveProviderTemperature(config),
		TopP:            config.TopP,
		MaxTokens:       resolveProviderMaxTokens(config),
		ReasoningEffort: strings.TrimSpace(config.ReasoningEffort),
		ExtraFields:     extraFields,
	})
}

func buildAnthropicProviderModel(_ context.Context, _ AgentKind, config ProviderConfig, truth auth.CredentialTruth, hasTruth bool) (ChatModel, error) {
	apiKey := resolveProviderSecret(truth, hasTruth)
	if strings.TrimSpace(apiKey) == "" {
		return nil, &ProviderNotAvailableError{
			Provider: "anthropic",
			Model:    resolveProviderModel("anthropic", config),
			Reason:   "no credential truth available",
		}
	}
	return NewAnthropicChatModel(AnthropicConfig{
		APIKey:      apiKey,
		BaseURL:     strings.TrimSpace(config.BaseURL),
		Model:       resolveProviderModel("anthropic", config),
		Temperature: resolveProviderTemperature(config),
		MaxTokens:   resolveProviderMaxTokens(config),
	}), nil
}

func buildOllamaProviderModel(_ context.Context, _ AgentKind, config ProviderConfig, _ auth.CredentialTruth, _ bool) (ChatModel, error) {
	return NewOllamaChatModel(context.Background(), OllamaConfig{
		BaseURL:     strings.TrimSpace(config.BaseURL),
		Model:       resolveProviderModel("ollama", config),
		Temperature: resolveProviderTemperature(config),
		MaxTokens:   resolveProviderMaxTokens(config),
	})
}

func resolveProviderSecret(truth auth.CredentialTruth, hasTruth bool) string {
	if !hasTruth {
		return ""
	}
	if trimmed := strings.TrimSpace(truth.APIKey); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(truth.AccessToken); trimmed != "" {
		return trimmed
	}
	return ""
}

func resolveProviderModel(provider string, config ProviderConfig) string {
	if strings.TrimSpace(config.Model) != "" {
		return strings.TrimSpace(config.Model)
	}
	switch provider {
	case "openai":
		return "gpt-4o"
	case "deepseek":
		return "deepseek-v4-pro"
	case "zhipu":
		return "glm-5.2"
	case "anthropic":
		return "claude-3-5-sonnet"
	case "ollama":
		return "llama3"
	default:
		return ""
	}
}

func resolveProviderTemperature(config ProviderConfig) float64 {
	return config.Temperature
}

func resolveProviderMaxTokens(config ProviderConfig) int {
	return config.MaxTokens
}

func capabilitiesForProviderModel(provider, model string, config ProviderConfig) ModelCapabilities {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	normalizedModel := strings.ToLower(model)
	if model == "" {
		model = resolveProviderModel(provider, config)
		normalizedModel = strings.ToLower(model)
	}

	caps := ModelCapabilities{
		Provider:          provider,
		Model:             model,
		MaxContextTokens:  200000,
		MaxOutputTokens:   16000,
		ExactTokenCount:   false,
		CacheEdit:         false,
		SupportsToolCalls: true,
		SupportsStreaming: true,
	}

	switch provider {
	case "openai":
		caps.ExactTokenCount = true
		caps.CacheEdit = true
		caps.SupportsReasoning = strings.HasPrefix(normalizedModel, "gpt-5") || strings.Contains(normalizedModel, "o") || isGLM47Model(normalizedModel)
		caps.SupportsNativeWebTool = providerSupportsNativeWebSearchForConfig(provider, config.BaseURL)
		switch {
		case isGLM47Model(normalizedModel):
			caps.MaxContextTokens = 200000
			caps.MaxOutputTokens = 128000
		case strings.Contains(normalizedModel, "gpt-5.4"), strings.Contains(normalizedModel, "gpt-5.5"):
			caps.MaxContextTokens = 200000
			caps.MaxOutputTokens = 32000
		case strings.Contains(normalizedModel, "gpt-5"), strings.Contains(normalizedModel, "gpt-4.1"):
			caps.MaxContextTokens = 128000
			caps.MaxOutputTokens = 32000
		case strings.Contains(normalizedModel, "gpt-4o"), strings.Contains(normalizedModel, "o4"), strings.Contains(normalizedModel, "o3"):
			caps.MaxContextTokens = 128000
			caps.MaxOutputTokens = 16000
		}
	case "deepseek", "zhipu":
		if preset, ok := ModelPresetByID(provider, model); ok {
			caps.MaxContextTokens = preset.MaxContextTokens
			caps.MaxOutputTokens = preset.MaxOutputTokens
			caps.SupportsToolCalls = preset.SupportsTools
			caps.SupportsStreaming = preset.SupportsStreaming
			caps.SupportsReasoning = preset.SupportsThinking
		} else if provider == "deepseek" {
			caps.MaxContextTokens = 1000000
			caps.MaxOutputTokens = 384000
			caps.SupportsReasoning = true
		} else {
			caps.MaxContextTokens = 200000
			caps.MaxOutputTokens = 128000
			caps.SupportsReasoning = strings.HasPrefix(normalizedModel, "glm-4.5") ||
				strings.HasPrefix(normalizedModel, "glm-4.6") ||
				strings.HasPrefix(normalizedModel, "glm-4.7") ||
				strings.HasPrefix(normalizedModel, "glm-5")
		}
		caps.ExactTokenCount = false
		caps.CacheEdit = false
		caps.SupportsNativeWebTool = false
	case "anthropic":
		caps.MaxContextTokens = 200000
		caps.MaxOutputTokens = 8192
		caps.SupportsReasoning = strings.Contains(normalizedModel, "sonnet") || strings.Contains(normalizedModel, "opus")
	case "ollama":
		caps.MaxContextTokens = 32000
		caps.MaxOutputTokens = 4096
		caps.ExactTokenCount = false
		caps.CacheEdit = false
	}

	if strings.Contains(normalizedModel, "20k") {
		caps.MaxContextTokens = 20000
	} else if strings.Contains(normalizedModel, "32k") {
		caps.MaxContextTokens = 32000
	}
	if config.MaxTokens > 0 {
		caps.MaxOutputTokens = config.MaxTokens
	}
	if config.MaxContextTokens > 0 {
		caps.MaxContextTokens = clampContextWindow(config.MaxContextTokens)
	}
	caps.SmallContextMode = caps.MaxContextTokens <= 32000
	applyReasoningCapabilityMetadata(&caps, provider, normalizedModel, config)
	return caps
}

func applyReasoningCapabilityMetadata(caps *ModelCapabilities, provider, normalizedModel string, config ProviderConfig) {
	if caps == nil {
		return
	}
	caps.NativeReasoning = providerModelSupportsNativeReasoningEffort(provider, normalizedModel)
	requested := normalizeCapabilityReasoningEffort(provider, normalizedModel, config.ReasoningEffort)
	if requested == "" {
		return
	}
	caps.ReasoningEffortRequested = requested
	if caps.NativeReasoning {
		caps.ReasoningEffortApplied = requested
		return
	}
	caps.ReasoningFallbackPolicy = GenericReasoningFallbackPolicy
}

func normalizeCapabilityReasoningEffort(provider, normalizedModel, effort string) string {
	if strings.TrimSpace(effort) == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderDeepSeek, ProviderZhipu:
		return NormalizeReasoningEffortForProvider(provider, normalizedModel, effort)
	default:
		return normalizeReasoningEffort(effort)
	}
}

func providerModelSupportsNativeReasoningEffort(provider, normalizedModel string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return openAIModelSupportsReasoningEffort(normalizedModel)
	case "deepseek", "zhipu":
		return true
	default:
		return false
	}
}

func cloneExtraFields(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func mergeExtraFields(base map[string]any, overrides map[string]any) map[string]any {
	out := cloneExtraFields(base)
	if len(overrides) == 0 {
		return out
	}
	if out == nil {
		out = make(map[string]any, len(overrides))
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func isGLM47Model(normalizedModel string) bool {
	normalizedModel = strings.ToLower(strings.TrimSpace(normalizedModel))
	return normalizedModel == "glm-4.7" ||
		strings.HasPrefix(normalizedModel, "glm-4.7-") ||
		strings.Contains(normalizedModel, "/glm-4.7")
}

func openAIModelSupportsReasoningEffort(normalizedModel string) bool {
	normalizedModel = strings.ToLower(strings.TrimSpace(normalizedModel))
	return strings.HasPrefix(normalizedModel, "gpt-5") ||
		strings.HasPrefix(normalizedModel, "o1") ||
		strings.HasPrefix(normalizedModel, "o3") ||
		strings.HasPrefix(normalizedModel, "o4")
}

func normalizeReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	default:
		return ""
	}
}

func clampContextWindow(value int) int {
	if value < 10000 {
		return 10000
	}
	return value
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ProviderNotFoundError is returned when no provider (including fallbacks)
// can be resolved for the requested configuration.
type ProviderNotFoundError struct {
	Provider string
}

func (e *ProviderNotFoundError) Error() string {
	return "modelrouter: provider not found: " + e.Provider
}

// ProviderNotAvailableError is returned by placeholder provider implementations
// when the actual eino-ext dependency is not yet integrated.
type ProviderNotAvailableError struct {
	Provider string
	Model    string
	Reason   string
}

func (e *ProviderNotAvailableError) Error() string {
	return "modelrouter: provider " + e.Provider + " (model: " + e.Model + ") not available: " + e.Reason
}
