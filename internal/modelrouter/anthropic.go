package modelrouter

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Anthropic ChatModel placeholder implementation.
// eino-ext does not yet have an anthropic package, so we provide a placeholder
// that satisfies model.ChatModel but returns an error on use.
// ---------------------------------------------------------------------------

// AnthropicConfig holds configuration for creating an Anthropic ChatModel.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key.
	APIKey string

	// BaseURL overrides the default Anthropic API endpoint.
	BaseURL string

	// Model is the model name, e.g. "claude-3-5-sonnet", "claude-3-haiku".
	Model string

	// Temperature controls randomness (0.0 – 1.0).
	Temperature float64

	// MaxTokens limits the response length.
	MaxTokens int
}

// AnthropicChatModel is a placeholder implementation of model.ChatModel
// for the Anthropic provider. It will be replaced by the actual eino-ext
// Anthropic ChatModel once the dependency is available.
type AnthropicChatModel struct {
	config AnthropicConfig
}

// NewAnthropicChatModel creates a placeholder Anthropic ChatModel instance.
func NewAnthropicChatModel(config AnthropicConfig) *AnthropicChatModel {
	return &AnthropicChatModel{config: config}
}

// Generate is a placeholder that returns an error indicating the real
// eino-ext Anthropic integration is not yet available.
func (m *AnthropicChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, &ProviderNotAvailableError{
		Provider: "anthropic",
		Model:    m.config.Model,
		Reason:   "eino-ext/components/model/anthropic dependency not yet integrated",
	}
}

// Stream is a placeholder that returns an error.
func (m *AnthropicChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, &ProviderNotAvailableError{
		Provider: "anthropic",
		Model:    m.config.Model,
		Reason:   "eino-ext/components/model/anthropic dependency not yet integrated",
	}
}

// BindTools is a placeholder that returns an error.
func (m *AnthropicChatModel) BindTools(_ []*schema.ToolInfo) error {
	return &ProviderNotAvailableError{
		Provider: "anthropic",
		Model:    m.config.Model,
		Reason:   "eino-ext/components/model/anthropic dependency not yet integrated",
	}
}

// ProviderName returns the provider identifier.
func (m *AnthropicChatModel) ProviderName() string {
	return "anthropic"
}

// ModelName returns the configured model name.
func (m *AnthropicChatModel) ModelName() string {
	return m.config.Model
}

// Compile-time check that AnthropicChatModel satisfies model.ChatModel.
var _ model.ChatModel = (*AnthropicChatModel)(nil)
