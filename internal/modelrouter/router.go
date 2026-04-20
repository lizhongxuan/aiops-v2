package modelrouter

import (
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
	// Provider identifies the LLM provider: "openai", "anthropic", "ollama".
	Provider string

	// Model is the specific model name, e.g. "gpt-4o", "claude-3-5-sonnet", "llama3".
	Model string

	// Temperature controls randomness in generation (0.0 – 1.0+).
	Temperature float64

	// MaxTokens limits the maximum number of tokens in the response.
	MaxTokens int
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

	// defaultProvider is the provider used when no specific provider is configured.
	defaultProvider string

	// fallbacks defines the fallback chain for provider failover.
	fallbacks []FallbackEntry

	// agentKindConfigs maps AgentKind to its preferred routing configuration.
	// When set, the AgentKind config takes precedence over the ProviderConfig
	// if ProviderConfig.Provider is empty.
	agentKindConfigs map[AgentKind]AgentKindConfig
}

// NewRouter creates a Router with the given default provider, registered
// providers, and fallback chain.
func NewRouter(defaultProvider string, providers map[string]ChatModel, fallbacks []FallbackEntry) *Router {
	return &Router{
		providers:        providers,
		defaultProvider:  defaultProvider,
		fallbacks:        fallbacks,
		agentKindConfigs: make(map[AgentKind]AgentKindConfig),
	}
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
//  2. If AgentKindConfig is registered for the AgentKind, use its Provider.
//  3. Fall back to the Router's defaultProvider.
//  4. If the resolved provider is not available, try the fallback chain.
//  5. If all options are exhausted, return ProviderNotFoundError.
func (r *Router) GetModel(agentKind AgentKind, config ProviderConfig) (ChatModel, error) {
	provider := r.resolveProvider(agentKind, config)

	// Try primary provider.
	if m, ok := r.providers[provider]; ok {
		return m, nil
	}

	// Try fallback chain.
	for _, fb := range r.fallbacks {
		if fb.Primary == provider {
			if m, ok := r.providers[fb.Fallback]; ok {
				return m, nil
			}
		}
	}

	return nil, &ProviderNotFoundError{Provider: provider}
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


