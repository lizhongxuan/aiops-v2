package modelrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

const (
	ReasoningSummaryTextDeltaMethod = "item/reasoning/summaryTextDelta"
	ReasoningSummaryPartAddedMethod = "item/reasoning/summaryPartAdded"
	ReasoningTextDeltaMethod        = "item/reasoning/textDelta"
)

type ReasoningStreamEvent struct {
	Method       string
	ThreadID     string
	TurnID       string
	ItemID       string
	SummaryIndex int
	ContentIndex int
	Delta        string
	Summary      string
	PartAdded    bool
	Raw          bool
}

type openAIReasoningEnvelope struct {
	Method string `json:"method"`
	Params struct {
		ThreadID     string `json:"threadId"`
		TurnID       string `json:"turnId"`
		ItemID       string `json:"itemId"`
		SummaryIndex int    `json:"summaryIndex"`
		ContentIndex int    `json:"contentIndex"`
		Delta        string `json:"delta"`
	} `json:"params"`
}

func ParseOpenAIReasoningEvent(raw []byte, showRawReasoning bool) (*ReasoningStreamEvent, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var envelope openAIReasoningEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	method := strings.TrimSpace(envelope.Method)
	switch method {
	case ReasoningSummaryTextDeltaMethod:
		return &ReasoningStreamEvent{
			Method:       method,
			ThreadID:     envelope.Params.ThreadID,
			TurnID:       envelope.Params.TurnID,
			ItemID:       envelope.Params.ItemID,
			SummaryIndex: envelope.Params.SummaryIndex,
			Delta:        envelope.Params.Delta,
		}, nil
	case ReasoningSummaryPartAddedMethod:
		return &ReasoningStreamEvent{
			Method:       method,
			ThreadID:     envelope.Params.ThreadID,
			TurnID:       envelope.Params.TurnID,
			ItemID:       envelope.Params.ItemID,
			SummaryIndex: envelope.Params.SummaryIndex,
			PartAdded:    true,
		}, nil
	case ReasoningTextDeltaMethod:
		if !showRawReasoning {
			return nil, nil
		}
		return &ReasoningStreamEvent{
			Method:       method,
			ThreadID:     envelope.Params.ThreadID,
			TurnID:       envelope.Params.TurnID,
			ItemID:       envelope.Params.ItemID,
			ContentIndex: envelope.Params.ContentIndex,
			Delta:        envelope.Params.Delta,
			Raw:          true,
		}, nil
	default:
		return nil, nil
	}
}

func ParseOpenAIReasoningExtra(extra map[string]any, showRawReasoning bool) (*ReasoningStreamEvent, error) {
	if len(extra) == 0 {
		return nil, nil
	}
	if method, _ := extra["method"].(string); strings.HasPrefix(method, "item/reasoning/") {
		raw, err := json.Marshal(extra)
		if err != nil {
			return nil, err
		}
		return ParseOpenAIReasoningEvent(raw, showRawReasoning)
	}
	for _, key := range []string{"openai_event", "event", "raw_event"} {
		value, ok := extra[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return ParseOpenAIReasoningEvent([]byte(typed), showRawReasoning)
		case map[string]any:
			raw, err := json.Marshal(typed)
			if err != nil {
				return nil, err
			}
			return ParseOpenAIReasoningEvent(raw, showRawReasoning)
		}
	}
	return nil, nil
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
