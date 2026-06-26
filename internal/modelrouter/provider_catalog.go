package modelrouter

import "strings"

const (
	ProviderOpenAI    = "openai"
	ProviderDeepSeek  = "deepseek"
	ProviderZhipu     = "zhipu"
	ProviderAnthropic = "anthropic"
	ProviderOllama    = "ollama"
)

type BaseURLPreset struct {
	ID      string
	Label   string
	BaseURL string
	Custom  bool
}

type ProviderPreset struct {
	ID                  string
	Label               string
	Protocol            string
	RequiresAPIKey      bool
	DefaultBaseURL      string
	BaseURLPresets      []BaseURLPreset
	Models              []ModelPreset
	DefaultModel        string
	SupportsTemperature bool
	SupportsTopP        bool
	SupportsThinking    bool
	ThinkingOptions     []string
	DefaultThinkingType string
	ReasoningOptions    []string
	DefaultReasoning    string
	SupportsToolStream  bool
	DefaultToolStream   bool
}

type ModelPreset struct {
	ID                 string
	Label              string
	MaxContextTokens   int
	MaxOutputTokens    int
	DefaultMaxTokens   int
	DefaultTemperature float64
	DefaultTopP        float64
	SupportsTools      bool
	SupportsStreaming  bool
	SupportsThinking   bool
	ReasoningOptions   []string
}

const (
	defaultContextTokens   = 200000
	defaultMaxOutputTokens = 20000
)

var providerPresets = []ProviderPreset{
	{
		ID:                  ProviderOpenAI,
		Label:               "OpenAI",
		Protocol:            "OpenAI API",
		RequiresAPIKey:      true,
		DefaultBaseURL:      "https://api.openai.com/v1",
		BaseURLPresets:      []BaseURLPreset{{ID: "official", Label: "OpenAI 官方", BaseURL: "https://api.openai.com/v1"}, {ID: "custom", Label: "自定义", Custom: true}},
		DefaultModel:        "gpt-5.4",
		SupportsTemperature: true,
		SupportsTopP:        true,
		ReasoningOptions:    []string{"low", "medium", "high"},
		DefaultReasoning:    "medium",
		Models: []ModelPreset{
			modelPreset("gpt-5.4", "GPT-5.4", 200000, 32000, defaultMaxOutputTokens, true, true, true),
			modelPreset("gpt-5.4-mini", "GPT-5.4 Mini", 200000, 32000, defaultMaxOutputTokens, true, true, true),
			modelPreset("gpt-4o", "GPT-4o", 128000, 16000, 16000, true, true, false),
			modelPreset("gpt-4o-mini", "GPT-4o Mini", 128000, 16000, 16000, true, true, false),
			modelPreset("o3-mini", "o3 Mini", 128000, 16000, 16000, true, true, true),
		},
	},
	{
		ID:                  ProviderDeepSeek,
		Label:               "DeepSeek",
		Protocol:            "OpenAI 兼容接口",
		RequiresAPIKey:      true,
		DefaultBaseURL:      "https://api.deepseek.com",
		BaseURLPresets:      []BaseURLPreset{{ID: "official", Label: "DeepSeek 官方", BaseURL: "https://api.deepseek.com"}, {ID: "custom", Label: "自定义", Custom: true}},
		DefaultModel:        "deepseek-v4-pro",
		SupportsTemperature: true,
		SupportsTopP:        true,
		SupportsThinking:    true,
		ThinkingOptions:     []string{"enabled", "disabled"},
		DefaultThinkingType: "enabled",
		ReasoningOptions:    []string{"high", "max"},
		DefaultReasoning:    "high",
		Models: []ModelPreset{
			modelPreset("deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, 384000, defaultMaxOutputTokens, true, true, true),
			modelPreset("deepseek-v4-flash", "DeepSeek V4 Flash", 1000000, 384000, defaultMaxOutputTokens, true, true, true),
		},
	},
	{
		ID:                  ProviderZhipu,
		Label:               "智谱 GLM",
		Protocol:            "OpenAI 兼容接口",
		RequiresAPIKey:      true,
		DefaultBaseURL:      "https://open.bigmodel.cn/api/paas/v4/",
		BaseURLPresets:      []BaseURLPreset{{ID: "official", Label: "智谱平台 API", BaseURL: "https://open.bigmodel.cn/api/paas/v4/"}, {ID: "coding", Label: "GLM Coding Plan", BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4"}, {ID: "custom", Label: "自定义", Custom: true}},
		DefaultModel:        "glm-5.2",
		SupportsTemperature: true,
		SupportsTopP:        true,
		SupportsThinking:    true,
		ThinkingOptions:     []string{"enabled", "disabled"},
		DefaultThinkingType: "enabled",
		ReasoningOptions:    []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"},
		DefaultReasoning:    "max",
		SupportsToolStream:  true,
		DefaultToolStream:   false,
		Models: []ModelPreset{
			withDefaultTopP(modelPreset("glm-5.2", "GLM-5.2", 1000000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-5.1", "GLM-5.1", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-5", "GLM-5", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-5-turbo", "GLM-5 Turbo", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-4.7", "GLM-4.7", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-4.7-flashx", "GLM-4.7 FlashX", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-4.6", "GLM-4.6", 200000, 128000, defaultMaxOutputTokens, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-4.5-air", "GLM-4.5 Air", 128000, 96000, 16000, true, true, true), 0.95),
			withDefaultTopP(modelPreset("glm-4.5-airx", "GLM-4.5 AirX", 128000, 96000, 16000, true, true, true), 0.95),
		},
	},
	{
		ID:             ProviderAnthropic,
		Label:          "Anthropic",
		Protocol:       "Anthropic API",
		RequiresAPIKey: true,
		DefaultBaseURL: "https://api.anthropic.com",
		BaseURLPresets: []BaseURLPreset{{ID: "official", Label: "Anthropic 官方", BaseURL: "https://api.anthropic.com"}, {ID: "custom", Label: "自定义", Custom: true}},
		DefaultModel:   "claude-sonnet-4-20250514",
		Models: []ModelPreset{
			modelPreset("claude-sonnet-4-20250514", "Claude Sonnet 4", 200000, 8192, 8192, true, true, true),
			modelPreset("claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet", 200000, 8192, 8192, true, true, true),
			modelPreset("claude-3-haiku-20240307", "Claude 3 Haiku", 200000, 8192, 8192, true, true, false),
		},
	},
	{
		ID:             ProviderOllama,
		Label:          "Ollama",
		Protocol:       "Ollama 本地接口",
		RequiresAPIKey: false,
		DefaultBaseURL: "http://127.0.0.1:11434/v1",
		BaseURLPresets: []BaseURLPreset{{ID: "local", Label: "本地 Ollama", BaseURL: "http://127.0.0.1:11434/v1"}, {ID: "custom", Label: "自定义", Custom: true}},
		DefaultModel:   "qwen2.5:7b",
		Models: []ModelPreset{
			modelPreset("qwen2.5:7b", "Qwen2.5 7B", 32000, 4096, 4096, true, true, false),
			modelPreset("qwen2.5:14b", "Qwen2.5 14B", 32000, 4096, 4096, true, true, false),
			modelPreset("llama3.1:8b", "Llama 3.1 8B", 32000, 4096, 4096, true, true, false),
			modelPreset("deepseek-coder-v2:16b", "DeepSeek Coder V2 16B", 32000, 4096, 4096, true, true, false),
		},
	},
}

func modelPreset(id, label string, contextTokens, outputTokens, defaultOutput int, tools, streaming, thinking bool) ModelPreset {
	return ModelPreset{
		ID:                 id,
		Label:              label,
		MaxContextTokens:   contextTokens,
		MaxOutputTokens:    outputTokens,
		DefaultMaxTokens:   defaultOutput,
		DefaultTemperature: 1,
		DefaultTopP:        1,
		SupportsTools:      tools,
		SupportsStreaming:  streaming,
		SupportsThinking:   thinking,
	}
}

func withDefaultTopP(preset ModelPreset, topP float64) ModelPreset {
	preset.DefaultTopP = topP
	return preset
}

func ProviderPresetByID(provider string) (ProviderPreset, bool) {
	normalized := NormalizeProviderID(provider)
	for _, preset := range providerPresets {
		if preset.ID == normalized {
			return cloneProviderPreset(preset), true
		}
	}
	return ProviderPreset{}, false
}

func ModelPresetByID(provider string, model string) (ModelPreset, bool) {
	preset, ok := ProviderPresetByID(provider)
	if !ok {
		return ModelPreset{}, false
	}
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, modelPreset := range preset.Models {
		if strings.ToLower(strings.TrimSpace(modelPreset.ID)) == normalized {
			return modelPreset, true
		}
	}
	return ModelPreset{}, false
}

func DefaultModelForProvider(provider string) string {
	if preset, ok := ProviderPresetByID(provider); ok {
		return preset.DefaultModel
	}
	return ""
}

func DefaultBaseURLForProvider(provider string) string {
	if preset, ok := ProviderPresetByID(provider); ok {
		return preset.DefaultBaseURL
	}
	return ""
}

func NormalizeProviderID(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "":
		return ProviderOpenAI
	case ProviderOpenAI:
		return ProviderOpenAI
	case ProviderDeepSeek:
		return ProviderDeepSeek
	case ProviderZhipu, "glm", "bigmodel", "zai":
		return ProviderZhipu
	case ProviderAnthropic:
		return ProviderAnthropic
	case ProviderOllama:
		return ProviderOllama
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func NormalizeReasoningEffortForProvider(provider string, model string, effort string) string {
	normalizedProvider := NormalizeProviderID(provider)
	requested := strings.ToLower(strings.TrimSpace(effort))
	preset, ok := ProviderPresetByID(normalizedProvider)
	if !ok {
		return normalizeReasoningEffort(effort)
	}
	options := preset.ReasoningOptions
	if modelPreset, ok := ModelPresetByID(normalizedProvider, model); ok && len(modelPreset.ReasoningOptions) > 0 {
		options = modelPreset.ReasoningOptions
	}
	if requested != "" {
		for _, option := range options {
			if requested == option {
				return requested
			}
		}
	}
	if preset.DefaultReasoning != "" {
		return preset.DefaultReasoning
	}
	return normalizeReasoningEffort(effort)
}

func NormalizeThinkingType(provider string, value string) string {
	preset, ok := ProviderPresetByID(provider)
	if !ok || !preset.SupportsThinking {
		return ""
	}
	requested := strings.ToLower(strings.TrimSpace(value))
	for _, option := range preset.ThinkingOptions {
		if requested == option {
			return requested
		}
	}
	return preset.DefaultThinkingType
}

func cloneProviderPreset(preset ProviderPreset) ProviderPreset {
	preset.BaseURLPresets = append([]BaseURLPreset(nil), preset.BaseURLPresets...)
	preset.Models = append([]ModelPreset(nil), preset.Models...)
	preset.ThinkingOptions = append([]string(nil), preset.ThinkingOptions...)
	preset.ReasoningOptions = append([]string(nil), preset.ReasoningOptions...)
	return preset
}
