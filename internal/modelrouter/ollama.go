package modelrouter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ollama"
)

// ---------------------------------------------------------------------------
// Ollama ChatModel factory using real eino-ext/components/model/ollama.
// ---------------------------------------------------------------------------

// OllamaConfig holds configuration for creating an Ollama ChatModel.
type OllamaConfig struct {
	// BaseURL is the Ollama server endpoint, e.g. "http://localhost:11434".
	BaseURL string

	// Model is the model name, e.g. "llama3", "mistral", "codellama".
	Model string

	// Temperature controls randomness (0.0 – 1.0).
	Temperature float64

	// MaxTokens limits the response length (num_predict in Ollama).
	MaxTokens int
}

// NewOllamaChatModel creates a real Ollama ChatModel instance using eino-ext.
// Returns a model.ChatModel that can be registered with the Router.
func NewOllamaChatModel(ctx context.Context, config OllamaConfig) (ChatModel, error) {
	cfg := &ollama.ChatModelConfig{
		BaseURL: config.BaseURL,
		Model:   config.Model,
	}

	cm, err := ollama.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ollama: create chat model: %w", err)
	}

	return cm, nil
}
