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
	// Provider identifies the OpenAI-compatible provider for request shaping.
	Provider string

	// APIKey is the OpenAI API key.
	APIKey string

	// BaseURL overrides the default OpenAI API endpoint (for proxies).
	BaseURL string

	// Model is the model name, e.g. "gpt-4o", "gpt-4o-mini".
	Model string

	// Temperature controls randomness (0.0 – 2.0).
	Temperature float64

	// TopP controls nucleus sampling.
	TopP float64

	// MaxTokens limits the response length.
	MaxTokens int

	// ReasoningEffort controls OpenAI reasoning effort: low, medium, or high.
	ReasoningEffort string

	// ExtraFields carries OpenAI-compatible provider-specific request fields.
	ExtraFields map[string]any
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
	provider := NormalizeProviderID(config.Provider)
	if provider == "" {
		provider = ProviderOpenAI
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

	if config.TopP > 0 {
		topP := float32(config.TopP)
		cfg.TopP = &topP
	}

	if config.MaxTokens > 0 {
		cfg.MaxTokens = &config.MaxTokens
	}
	if len(config.ExtraFields) > 0 {
		cfg.ExtraFields = cloneExtraFields(config.ExtraFields)
	}
	if effort := openAIReasoningEffortForModel(config.Model, config.ReasoningEffort); effort != "" {
		cfg.ReasoningEffort = openai.ReasoningEffortLevel(effort)
	}

	cm, err := openai.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("openai: create chat model: %w", err)
	}

	return &streamGenerateChatModel{
		inner:       cm,
		provider:    provider,
		extraFields: cloneExtraFields(config.ExtraFields),
	}, nil
}

func openAIReasoningEffortForModel(model, effort string) string {
	if !openAIModelSupportsReasoningEffort(strings.ToLower(strings.TrimSpace(model))) {
		return ""
	}
	return normalizeOpenAIReasoningEffort(effort)
}

func normalizeOpenAIReasoningEffort(value string) string {
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

func openAICompatibleExtraFields(provider string, config ProviderConfig) map[string]any {
	provider = NormalizeProviderID(provider)
	extra := map[string]any{}
	switch provider {
	case "deepseek":
		if thinking := strings.TrimSpace(config.ThinkingType); thinking != "" {
			extra["thinking"] = map[string]any{"type": thinking}
		}
		if effort := strings.TrimSpace(config.ReasoningEffort); effort != "" {
			extra["reasoning_effort"] = effort
		}
	case "zhipu":
		if thinking := strings.TrimSpace(config.ThinkingType); thinking != "" {
			extra["thinking"] = map[string]any{"type": thinking}
		}
		if effort := strings.TrimSpace(config.ReasoningEffort); effort != "" {
			extra["reasoning_effort"] = effort
		}
		if config.ToolStream {
			extra["tool_stream"] = true
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

type streamGenerateChatModel struct {
	inner       ChatModel
	provider    string
	extraFields map[string]any
	boundTools  []*schema.ToolInfo
}

func (m *streamGenerateChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m == nil || m.inner == nil {
		return nil, fmt.Errorf("openai: chat model is not configured")
	}
	opts = m.withProviderNativeTools(opts)
	return m.inner.Generate(ctx, input, opts...)
}

func (m *streamGenerateChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || m.inner == nil {
		return nil, fmt.Errorf("openai: chat model is not configured")
	}
	opts = m.withProviderNativeTools(opts)
	return m.inner.Stream(ctx, input, opts...)
}

func (m *streamGenerateChatModel) BindTools(tools []*schema.ToolInfo) error {
	if m == nil || m.inner == nil {
		return fmt.Errorf("openai: chat model is not configured")
	}
	m.boundTools = cloneToolInfos(tools)
	if !providerSupportsNativeWebSearch(m.provider) {
		return m.inner.BindTools(tools)
	}
	filtered := filterWebSearchToolInfos(tools)
	if len(filtered) == 0 {
		return nil
	}
	return m.inner.BindTools(filtered)
}

func (m *streamGenerateChatModel) withProviderNativeTools(opts []model.Option) []model.Option {
	if m == nil {
		return opts
	}
	common := model.GetCommonOptions(nil, opts...)
	tools := common.Tools
	if len(tools) == 0 {
		tools = m.boundTools
	}
	nativeExtra := openAICompatibleNativeWebSearchExtraFields(m.provider, tools)
	if len(nativeExtra) == 0 {
		return opts
	}
	extra := mergeExtraFields(m.extraFields, nativeExtra)
	return append(opts,
		openai.WithExtraFields(extra),
		openai.WithResponseMessageModifier(providerNativeWebSearchMessageModifier(m.provider)),
		openai.WithResponseChunkMessageModifier(providerNativeWebSearchChunkModifier(m.provider)),
	)
}

func openAICompatibleNativeWebSearchExtraFields(provider string, toolInfos []*schema.ToolInfo) map[string]any {
	if !providerSupportsNativeWebSearch(provider) || len(toolInfos) == 0 {
		return nil
	}
	hasWebSearch := false
	tools := make([]any, 0, len(toolInfos))
	for _, info := range toolInfos {
		if info == nil {
			continue
		}
		if isWebSearchToolInfo(info) {
			hasWebSearch = true
			continue
		}
		tools = append(tools, openAIFunctionToolPayload(info))
	}
	if !hasWebSearch {
		return nil
	}
	tools = append([]any{map[string]any{"type": "web_search"}}, tools...)
	return map[string]any{"tools": tools}
}

func filterWebSearchToolInfos(toolInfos []*schema.ToolInfo) []*schema.ToolInfo {
	if len(toolInfos) == 0 {
		return nil
	}
	filtered := make([]*schema.ToolInfo, 0, len(toolInfos))
	for _, info := range toolInfos {
		if info == nil || isWebSearchToolInfo(info) {
			continue
		}
		filtered = append(filtered, info)
	}
	return filtered
}

func cloneToolInfos(toolInfos []*schema.ToolInfo) []*schema.ToolInfo {
	if len(toolInfos) == 0 {
		return nil
	}
	out := make([]*schema.ToolInfo, len(toolInfos))
	copy(out, toolInfos)
	return out
}

func providerSupportsNativeWebSearch(provider string) bool {
	switch NormalizeProviderID(provider) {
	case ProviderOpenAI, ProviderZhipu:
		return true
	default:
		return false
	}
}

func isWebSearchToolInfo(info *schema.ToolInfo) bool {
	if info == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(info.Name)) {
	case "web_search", "search_web":
		return true
	default:
		return false
	}
}

func openAIFunctionToolPayload(info *schema.ToolInfo) map[string]any {
	function := map[string]any{
		"name": strings.TrimSpace(info.Name),
	}
	if desc := strings.TrimSpace(info.Desc); desc != "" {
		function["description"] = desc
	}
	if info.ParamsOneOf != nil {
		if params, err := info.ParamsOneOf.ToJSONSchema(); err == nil && params != nil {
			function["parameters"] = params
		}
	}
	return map[string]any{
		"type":     "function",
		"function": function,
	}
}

func providerNativeWebSearchMessageModifier(provider string) openai.ResponseMessageModifier {
	return func(_ context.Context, msg *schema.Message, rawBody []byte) (*schema.Message, error) {
		return attachProviderNativeWebSearchEventsToMessage(msg, ExtractProviderNativeWebSearchEvents(rawBody, provider)), nil
	}
}

func providerNativeWebSearchChunkModifier(provider string) openai.ResponseChunkMessageModifier {
	events := []ProviderNativeWebSearchEvent{}
	return func(_ context.Context, msg *schema.Message, rawBody []byte, end bool) (*schema.Message, error) {
		events = mergeProviderNativeWebSearchEvents(append(events, ExtractProviderNativeWebSearchEvents(rawBody, provider)...))
		if !end {
			return msg, nil
		}
		return attachProviderNativeWebSearchEventsToMessage(msg, events), nil
	}
}

func attachProviderNativeWebSearchEventsToMessage(msg *schema.Message, events []ProviderNativeWebSearchEvent) *schema.Message {
	if len(events) == 0 {
		return msg
	}
	if msg == nil {
		msg = &schema.Message{Role: schema.Assistant}
	}
	if msg.Extra == nil {
		msg.Extra = map[string]any{}
	}
	msg.Extra[ProviderNativeWebSearchExtraKey] = events
	return msg
}
