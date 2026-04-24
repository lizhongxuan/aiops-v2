package modelrouter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// OpenAI ChatModel factory using real eino-ext/components/model/openai.
// ---------------------------------------------------------------------------

// OpenAIConfig holds configuration for creating an OpenAI ChatModel.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	APIKey string

	// BaseURL overrides the default OpenAI API endpoint (for proxies).
	BaseURL string

	// Model is the model name, e.g. "gpt-4o", "gpt-4o-mini".
	Model string

	// Temperature controls randomness (0.0 – 2.0).
	Temperature float64

	// MaxTokens limits the response length.
	MaxTokens int
}

// NewOpenAIChatModel creates a real OpenAI ChatModel instance using eino-ext.
// Returns a model.ChatModel that can be registered with the Router.
func NewOpenAIChatModel(ctx context.Context, config OpenAIConfig) (ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}

	cfg := &openai.ChatModelConfig{
		APIKey:  config.APIKey,
		Model:   config.Model,
		BaseURL: config.BaseURL,
	}

	if config.Temperature > 0 {
		temp := float32(config.Temperature)
		cfg.Temperature = &temp
	}

	if config.MaxTokens > 0 {
		cfg.MaxTokens = &config.MaxTokens
	}

	cm, err := openai.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("openai: create chat model: %w", err)
	}

	return &streamGenerateChatModel{inner: cm}, nil
}

type streamGenerateChatModel struct {
	inner ChatModel
}

func (m *streamGenerateChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m == nil || m.inner == nil {
		return nil, fmt.Errorf("openai: chat model is not configured")
	}
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.ConcatMessageStream(stream)
}

func (m *streamGenerateChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || m.inner == nil {
		return nil, fmt.Errorf("openai: chat model is not configured")
	}
	return m.inner.Stream(ctx, input, opts...)
}

func (m *streamGenerateChatModel) BindTools(tools []*schema.ToolInfo) error {
	if m == nil || m.inner == nil {
		return fmt.Errorf("openai: chat model is not configured")
	}
	return m.inner.BindTools(tools)
}
