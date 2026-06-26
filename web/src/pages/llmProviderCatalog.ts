export type LlmProviderId = "openai" | "deepseek" | "zhipu" | "anthropic" | "ollama";

export type LlmBaseURLPreset = {
  id: string;
  label: string;
  baseURL: string;
  custom?: boolean;
};

export type LlmModelPreset = {
  id: string;
  label: string;
  maxContextTokens: number;
  maxOutputTokens: number;
  defaultMaxOutputTokens: number;
  defaultTemperature: number;
  defaultTopP: number;
  supportsTools: boolean;
  supportsStreaming: boolean;
  supportsThinking: boolean;
  reasoningOptions?: string[];
};

export type LlmProviderPreset = {
  id: LlmProviderId;
  label: string;
  protocol: string;
  requiresAPIKey: boolean;
  defaultBaseURL: string;
  baseURLPresets: LlmBaseURLPreset[];
  defaultModel: string;
  models: LlmModelPreset[];
  supportsTemperature: boolean;
  supportsTopP: boolean;
  supportsThinking: boolean;
  thinkingOptions: string[];
  defaultThinkingType: string;
  reasoningOptions: string[];
  defaultReasoning: string;
  supportsToolStream: boolean;
  defaultToolStream: boolean;
};

export type LlmConfigFormDefaults = {
  provider: LlmProviderId;
  modelMode: "preset" | "custom";
  model: string;
  customModel: string;
  baseURLMode: "preset" | "custom";
  baseURL: string;
  customBaseURL: string;
  maxContextTokens: string;
  maxOutputTokens: string;
  reasoningEffort: string;
  thinkingType: string;
  temperature: string;
  topP: string;
  toolStream: boolean;
};

export const CUSTOM_MODEL_VALUE = "__custom_model__";
export const CUSTOM_BASE_URL_VALUE = "__custom_base_url__";
export const DEFAULT_CUSTOM_CONTEXT_TOKENS = 200000;
export const DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS = 20000;

function modelPreset(
  id: string,
  label: string,
  maxContextTokens: number,
  maxOutputTokens: number,
  defaultMaxOutputTokens: number,
  supportsTools = true,
  supportsStreaming = true,
  supportsThinking = true,
  defaultTopP = 1,
): LlmModelPreset {
  return {
    id,
    label,
    maxContextTokens,
    maxOutputTokens,
    defaultMaxOutputTokens,
    defaultTemperature: 1,
    defaultTopP,
    supportsTools,
    supportsStreaming,
    supportsThinking,
  };
}

export const LLM_PROVIDER_PRESETS: LlmProviderPreset[] = [
  {
    id: "openai",
    label: "OpenAI",
    protocol: "OpenAI API",
    requiresAPIKey: true,
    defaultBaseURL: "https://api.openai.com/v1",
    baseURLPresets: [
      { id: "official", label: "OpenAI 官方", baseURL: "https://api.openai.com/v1" },
      { id: "custom", label: "自定义", baseURL: "", custom: true },
    ],
    defaultModel: "gpt-5.4",
    models: [
      modelPreset("gpt-5.4", "GPT-5.4", 200000, 32000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS),
      modelPreset("gpt-5.4-mini", "GPT-5.4 Mini", 200000, 32000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS),
      modelPreset("gpt-4o", "GPT-4o", 128000, 16000, 16000, true, true, false),
      modelPreset("gpt-4o-mini", "GPT-4o Mini", 128000, 16000, 16000, true, true, false),
      modelPreset("o3-mini", "o3 Mini", 128000, 16000, 16000),
    ],
    supportsTemperature: true,
    supportsTopP: true,
    supportsThinking: false,
    thinkingOptions: [],
    defaultThinkingType: "",
    reasoningOptions: ["low", "medium", "high"],
    defaultReasoning: "medium",
    supportsToolStream: false,
    defaultToolStream: false,
  },
  {
    id: "deepseek",
    label: "DeepSeek",
    protocol: "OpenAI 兼容接口",
    requiresAPIKey: true,
    defaultBaseURL: "https://api.deepseek.com",
    baseURLPresets: [
      { id: "official", label: "DeepSeek 官方", baseURL: "https://api.deepseek.com" },
      { id: "custom", label: "自定义", baseURL: "", custom: true },
    ],
    defaultModel: "deepseek-v4-pro",
    models: [
      modelPreset("deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, 384000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS),
      modelPreset("deepseek-v4-flash", "DeepSeek V4 Flash", 1000000, 384000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS),
    ],
    supportsTemperature: true,
    supportsTopP: true,
    supportsThinking: true,
    thinkingOptions: ["enabled", "disabled"],
    defaultThinkingType: "enabled",
    reasoningOptions: ["high", "max"],
    defaultReasoning: "high",
    supportsToolStream: false,
    defaultToolStream: false,
  },
  {
    id: "zhipu",
    label: "智谱 GLM",
    protocol: "OpenAI 兼容接口",
    requiresAPIKey: true,
    defaultBaseURL: "https://open.bigmodel.cn/api/paas/v4/",
    baseURLPresets: [
      { id: "official", label: "智谱平台 API", baseURL: "https://open.bigmodel.cn/api/paas/v4/" },
      { id: "coding", label: "GLM Coding Plan", baseURL: "https://open.bigmodel.cn/api/coding/paas/v4" },
      { id: "custom", label: "自定义", baseURL: "", custom: true },
    ],
    defaultModel: "glm-5.2",
    models: [
      modelPreset("glm-5.2", "GLM-5.2", 1000000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-5.1", "GLM-5.1", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-5", "GLM-5", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-5-turbo", "GLM-5 Turbo", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-4.7", "GLM-4.7", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-4.7-flashx", "GLM-4.7 FlashX", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-4.6", "GLM-4.6", 200000, 128000, DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS, true, true, true, 0.95),
      modelPreset("glm-4.5-air", "GLM-4.5 Air", 128000, 96000, 16000, true, true, true, 0.95),
      modelPreset("glm-4.5-airx", "GLM-4.5 AirX", 128000, 96000, 16000, true, true, true, 0.95),
    ],
    supportsTemperature: true,
    supportsTopP: true,
    supportsThinking: true,
    thinkingOptions: ["enabled", "disabled"],
    defaultThinkingType: "enabled",
    reasoningOptions: ["max", "xhigh", "high", "medium", "low", "minimal", "none"],
    defaultReasoning: "max",
    supportsToolStream: true,
    defaultToolStream: false,
  },
  {
    id: "anthropic",
    label: "Anthropic",
    protocol: "Anthropic API",
    requiresAPIKey: true,
    defaultBaseURL: "https://api.anthropic.com",
    baseURLPresets: [
      { id: "official", label: "Anthropic 官方", baseURL: "https://api.anthropic.com" },
      { id: "custom", label: "自定义", baseURL: "", custom: true },
    ],
    defaultModel: "claude-sonnet-4-20250514",
    models: [
      modelPreset("claude-sonnet-4-20250514", "Claude Sonnet 4", 200000, 8192, 8192),
      modelPreset("claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet", 200000, 8192, 8192),
      modelPreset("claude-3-haiku-20240307", "Claude 3 Haiku", 200000, 8192, 8192, true, true, false),
    ],
    supportsTemperature: false,
    supportsTopP: false,
    supportsThinking: false,
    thinkingOptions: [],
    defaultThinkingType: "",
    reasoningOptions: [],
    defaultReasoning: "",
    supportsToolStream: false,
    defaultToolStream: false,
  },
  {
    id: "ollama",
    label: "Ollama",
    protocol: "Ollama 本地接口",
    requiresAPIKey: false,
    defaultBaseURL: "http://127.0.0.1:11434/v1",
    baseURLPresets: [
      { id: "local", label: "本地 Ollama", baseURL: "http://127.0.0.1:11434/v1" },
      { id: "custom", label: "自定义", baseURL: "", custom: true },
    ],
    defaultModel: "qwen2.5:7b",
    models: [
      modelPreset("qwen2.5:7b", "Qwen2.5 7B", 32000, 4096, 4096, true, true, false),
      modelPreset("qwen2.5:14b", "Qwen2.5 14B", 32000, 4096, 4096, true, true, false),
      modelPreset("llama3.1:8b", "Llama 3.1 8B", 32000, 4096, 4096, true, true, false),
      modelPreset("deepseek-coder-v2:16b", "DeepSeek Coder V2 16B", 32000, 4096, 4096, true, true, false),
    ],
    supportsTemperature: false,
    supportsTopP: false,
    supportsThinking: false,
    thinkingOptions: [],
    defaultThinkingType: "",
    reasoningOptions: [],
    defaultReasoning: "",
    supportsToolStream: false,
    defaultToolStream: false,
  },
];

export function normalizeProviderId(provider: string | null | undefined): LlmProviderId {
  const normalized = String(provider || "").trim().toLowerCase();
  if (normalized === "deepseek") return "deepseek";
  if (normalized === "zhipu" || normalized === "glm" || normalized === "bigmodel" || normalized === "zai") return "zhipu";
  if (normalized === "anthropic") return "anthropic";
  if (normalized === "ollama") return "ollama";
  return "openai";
}

export function getProviderPreset(provider: string | null | undefined): LlmProviderPreset {
  const normalized = normalizeProviderId(provider);
  return LLM_PROVIDER_PRESETS.find((item) => item.id === normalized) || LLM_PROVIDER_PRESETS[0];
}

export function getModelPreset(provider: string | null | undefined, model: string | null | undefined): LlmModelPreset | undefined {
  const normalized = String(model || "").trim().toLowerCase();
  if (!normalized) return undefined;
  return getProviderPreset(provider).models.find((item) => item.id.toLowerCase() === normalized);
}

export function getReasoningOptions(provider: string | null | undefined, model?: string | null): string[] {
  const modelPreset = getModelPreset(provider, model);
  if (modelPreset?.reasoningOptions?.length) return [...modelPreset.reasoningOptions];
  return [...getProviderPreset(provider).reasoningOptions];
}

export function getModelOptions(provider: string | null | undefined) {
  return [
    ...getProviderPreset(provider).models.map((model) => ({ label: model.label, value: model.id })),
    { label: "自定义", value: CUSTOM_MODEL_VALUE },
  ];
}

export function getBaseURLOptions(provider: string | null | undefined) {
  return getProviderPreset(provider).baseURLPresets.map((item) => ({ label: item.label, value: item.custom ? CUSTOM_BASE_URL_VALUE : item.baseURL }));
}

export function defaultFormForProvider(provider: string | null | undefined): LlmConfigFormDefaults {
  const preset = getProviderPreset(provider);
  const model = getModelPreset(preset.id, preset.defaultModel);
  return {
    provider: preset.id,
    modelMode: "preset",
    model: preset.defaultModel,
    customModel: "",
    baseURLMode: "preset",
    baseURL: preset.defaultBaseURL,
    customBaseURL: "",
    ...defaultsForProviderModel(preset, model),
  };
}

export function defaultFormForModel(provider: string | null | undefined, model: string | null | undefined): Partial<LlmConfigFormDefaults> {
  const preset = getProviderPreset(provider);
  return defaultsForProviderModel(preset, getModelPreset(preset.id, model));
}

function defaultsForProviderModel(preset: LlmProviderPreset, model: LlmModelPreset | undefined): Pick<
  LlmConfigFormDefaults,
  "maxContextTokens" | "maxOutputTokens" | "reasoningEffort" | "thinkingType" | "temperature" | "topP" | "toolStream"
> {
  return {
    maxContextTokens: String(model?.maxContextTokens || DEFAULT_CUSTOM_CONTEXT_TOKENS),
    maxOutputTokens: String(model?.defaultMaxOutputTokens || DEFAULT_CUSTOM_MAX_OUTPUT_TOKENS),
    reasoningEffort: preset.defaultReasoning,
    thinkingType: preset.supportsThinking ? preset.defaultThinkingType : "",
    temperature: String(model?.defaultTemperature ?? 1),
    topP: String(model?.defaultTopP ?? 1),
    toolStream: preset.supportsToolStream ? preset.defaultToolStream : false,
  };
}
