import { Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  CUSTOM_BASE_URL_VALUE,
  CUSTOM_MODEL_VALUE,
  LLM_PROVIDER_PRESETS,
  defaultFormForModel,
  defaultFormForProvider,
  getBaseURLOptions,
  getModelOptions,
  getModelPreset,
  getProviderPreset,
  getReasoningOptions,
  normalizeProviderId,
  type LlmConfigFormDefaults,
} from "@/pages/llmProviderCatalog";
import {
  DEFAULT_LLM_CONTEXT_TOKENS,
  DEFAULT_LLM_MAX_OUTPUT_TOKENS,
  fetchLlmConfig,
  type LlmConfigUpdate,
  type LlmConfigView,
  normalizeLlmContextTokens,
  normalizeLlmMaxOutputTokens,
  updateLlmConfig,
} from "@/pages/settingsApi";
import { Field, LoadingState, SelectField, SettingsPageFrame, StatGrid, StatusAlert } from "@/pages/settingsComponents";

type LlmConfigForm = LlmConfigFormDefaults & {
  apiKey: string;
};

const providerOptions = LLM_PROVIDER_PRESETS.map((provider) => ({ label: provider.label, value: provider.id }));

function providerLabel(provider: string) {
  return getProviderPreset(provider).label || provider || "未选择";
}

function providerProtocolLabel(provider: string) {
  return getProviderPreset(provider).protocol || "自定义接口";
}

function inferProvider(config: Pick<LlmConfigView, "provider" | "model" | "baseURL"> | null | undefined) {
  const provider = normalizeProviderId(config?.provider);
  if (provider === "openai" && (isDeepSeekBaseURL(config?.baseURL) || isDeepSeekModel(config?.model))) return "deepseek";
  if (provider === "openai" && (isGLMModel(config?.model) || isZhipuBaseURL(config?.baseURL))) return "zhipu";
  return provider;
}

function isDeepSeekModel(model: unknown) {
  return String(model || "").trim().toLowerCase().startsWith("deepseek-");
}

function isGLMModel(model: unknown) {
  return String(model || "").trim().toLowerCase().startsWith("glm-");
}

function isDeepSeekBaseURL(baseURL: unknown) {
  return String(baseURL || "").trim().toLowerCase().includes("api.deepseek.com");
}

function isZhipuBaseURL(baseURL: unknown) {
  const normalized = String(baseURL || "").trim().toLowerCase();
  return normalized.includes("api.z.ai") || normalized.includes("open.bigmodel.cn");
}

function formWithApiKey(defaults: LlmConfigFormDefaults): LlmConfigForm {
  return { ...defaults, apiKey: "" };
}

function formFromConfig(config: LlmConfigView): LlmConfigForm {
  const provider = inferProvider(config);
  const defaults = defaultFormForProvider(provider);
  const preset = getProviderPreset(provider);
  const savedModel = String(config.model || "").trim();
  const knownModel = getModelPreset(provider, savedModel);
  const savedBaseURL = String(config.baseURL || "").trim();
  const knownBaseURL = preset.baseURLPresets.find((item) => !item.custom && item.baseURL === savedBaseURL);
  const modelDefaults = defaultFormForModel(provider, knownModel?.id || savedModel || defaults.model);
  return {
    ...defaults,
    ...modelDefaults,
    provider,
    modelMode: knownModel || !savedModel ? "preset" : "custom",
    model: knownModel?.id || defaults.model,
    customModel: knownModel || !savedModel ? "" : savedModel,
    baseURLMode: knownBaseURL || !savedBaseURL ? "preset" : "custom",
    baseURL: knownBaseURL?.baseURL || defaults.baseURL,
    customBaseURL: knownBaseURL || !savedBaseURL ? "" : savedBaseURL,
    apiKey: "",
    maxContextTokens: String(normalizeLlmContextTokens(config.maxContextTokens || modelDefaults.maxContextTokens || DEFAULT_LLM_CONTEXT_TOKENS)),
    maxOutputTokens: String(
      normalizeLlmMaxOutputTokens(config.maxOutputTokens || modelDefaults.maxOutputTokens || DEFAULT_LLM_MAX_OUTPUT_TOKENS, knownModel?.maxOutputTokens),
    ),
    temperature: config.temperature === undefined || config.temperature === null ? String(modelDefaults.temperature || "1") : String(config.temperature),
    topP: config.topP === undefined || config.topP === null ? String(modelDefaults.topP || "1") : String(config.topP),
    thinkingType: config.thinkingType || String(modelDefaults.thinkingType || ""),
    reasoningEffort: config.reasoningEffort || String(modelDefaults.reasoningEffort || ""),
    toolStream: Boolean(config.toolStream ?? modelDefaults.toolStream),
  };
}

export function LLMConfigPage() {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [config, setConfig] = useState<LlmConfigView | null>(null);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);
  const [form, setForm] = useState<LlmConfigForm>(() => formWithApiKey(defaultFormForProvider("openai")));

  const providerPreset = useMemo(() => getProviderPreset(form.provider), [form.provider]);
  const modelOptions = useMemo(() => getModelOptions(form.provider), [form.provider]);
  const baseURLOptions = useMemo(() => getBaseURLOptions(form.provider), [form.provider]);
  const reasoningOptions = useMemo(() => getReasoningOptions(form.provider, form.modelMode === "custom" ? form.customModel : form.model), [form.provider, form.model, form.modelMode, form.customModel]);
  const effectiveModel = form.modelMode === "custom" ? form.customModel.trim() : form.model;
  const effectiveBaseURL = form.baseURLMode === "custom" ? form.customBaseURL.trim() : form.baseURL;
  const selectedModelPreset = getModelPreset(form.provider, effectiveModel);
  const needsApiKey = providerPreset.requiresAPIKey;
  const connected = Boolean(config?.bifrostActive && (!needsApiKey || config.apiKeySet));
  const showSamplingNoEffectHint = form.provider === "deepseek" && form.thinkingType === "enabled";

  async function load() {
    setLoading(true);
    try {
      const next = await fetchLlmConfig();
      setConfig(next);
      setForm(formFromConfig(next));
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载配置失败" });
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    setSaving(true);
    try {
      const outputCap = selectedModelPreset?.maxOutputTokens;
      const normalizedContext = normalizeLlmContextTokens(form.maxContextTokens);
      const normalizedOutput = normalizeLlmMaxOutputTokens(form.maxOutputTokens, outputCap);
      const payload: LlmConfigUpdate = {
        provider: form.provider,
        model: effectiveModel || providerPreset.defaultModel,
        maxContextTokens: normalizedContext,
        maxOutputTokens: normalizedOutput,
        reasoningEffort: reasoningOptions.length ? form.reasoningEffort : undefined,
      };
      if (form.apiKey.trim()) payload.apiKey = form.apiKey.trim();
      if (!(form.provider === "openai" && form.baseURLMode === "preset" && effectiveBaseURL === providerPreset.defaultBaseURL)) {
        payload.baseURL = effectiveBaseURL;
      }
      if (providerPreset.supportsTemperature) payload.temperature = form.temperature;
      if (providerPreset.supportsTopP) payload.topP = form.topP;
      if (providerPreset.supportsThinking) payload.thinkingType = form.thinkingType;
      if (providerPreset.supportsToolStream) payload.toolStream = form.toolStream;
      setForm((prev) => ({ ...prev, maxContextTokens: String(normalizedContext), maxOutputTokens: String(normalizedOutput) }));
      const result = await updateLlmConfig(payload);
      await load();
      setMessage({ type: result.ok === false ? "info" : "success", text: result.ok === false ? result.message || result.error || "配置已保存" : "配置已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存失败" });
    } finally {
      setSaving(false);
    }
  }

  function selectProvider(provider: string) {
    setForm(formWithApiKey(defaultFormForProvider(provider)));
  }

  function selectModel(value: string) {
    if (value === CUSTOM_MODEL_VALUE) {
      setForm((prev) => ({
        ...prev,
        modelMode: "custom",
        customModel: "",
        ...defaultFormForModel(prev.provider, ""),
      }));
      return;
    }
    setForm((prev) => ({
      ...prev,
      modelMode: "preset",
      model: value,
      customModel: "",
      ...defaultFormForModel(prev.provider, value),
    }));
  }

  function selectBaseURL(value: string) {
    if (value === CUSTOM_BASE_URL_VALUE) {
      setForm((prev) => ({ ...prev, baseURLMode: "custom", customBaseURL: "" }));
      return;
    }
    setForm((prev) => ({ ...prev, baseURLMode: "preset", baseURL: value, customBaseURL: "" }));
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="LLM 配置"
      description=""
      actions={
        <Button data-testid="llm-save-button" onClick={() => void save()} disabled={loading || saving}>
          <Save />
          {saving ? "保存中" : "保存配置"}
        </Button>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "保存失败" : "保存成功"} message={message.text} /> : null}
      {loading && !config ? (
        <LoadingState label="加载 LLM 配置" />
      ) : (
        <>
          <StatGrid
            items={[
              { label: "模型接入", value: providerLabel(config ? inferProvider(config) : form.provider) },
              { label: "接口协议", value: providerProtocolLabel(config ? inferProvider(config) : form.provider) },
              { label: "Model", value: config?.model || effectiveModel },
              { label: "Context", value: normalizeLlmContextTokens(config?.maxContextTokens || form.maxContextTokens).toLocaleString() },
              { label: "API Key", value: config?.apiKeySet ? config.apiKeyMasked || "已设置" : "未设置", tone: config?.apiKeySet ? "ok" : "warn" },
              { label: "模型状态", value: connected ? "已配置" : "未连接", tone: connected ? "ok" : "bad" },
            ]}
          />

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>主 LLM 配置</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-2">
              <Field label="模型接入">
                <SelectField data-testid="llm-provider-select" aria-label="Provider" value={form.provider} options={providerOptions} onChange={selectProvider} />
              </Field>
              <Field label="Model">
                <SelectField
                  data-testid="llm-model-select"
                  aria-label="Model"
                  value={form.modelMode === "custom" ? CUSTOM_MODEL_VALUE : form.model}
                  options={modelOptions}
                  onChange={selectModel}
                />
              </Field>
              {form.modelMode === "custom" ? (
                <Field label="自定义 Model">
                  <Input data-testid="llm-custom-model-input" value={form.customModel} onChange={(event) => setForm((prev) => ({ ...prev, customModel: event.target.value }))} />
                </Field>
              ) : null}
              <Field label="Base URL">
                <SelectField
                  data-testid="llm-base-url-select"
                  aria-label="Base URL"
                  value={form.baseURLMode === "custom" ? CUSTOM_BASE_URL_VALUE : form.baseURL}
                  options={baseURLOptions}
                  onChange={selectBaseURL}
                />
              </Field>
              {form.baseURLMode === "custom" ? (
                <Field label="自定义 Base URL">
                  <Input data-testid="llm-custom-base-url-input" value={form.customBaseURL} onChange={(event) => setForm((prev) => ({ ...prev, customBaseURL: event.target.value }))} />
                </Field>
              ) : null}
              <Field label="上下文大小">
                <Input
                  data-testid="llm-context-tokens-input"
                  type="number"
                  min={10000}
                  step={1000}
                  value={form.maxContextTokens}
                  onChange={(event) => setForm((prev) => ({ ...prev, maxContextTokens: event.target.value }))}
                />
              </Field>
              <Field label="最大输出 Tokens">
                <Input
                  data-testid="llm-max-output-tokens-input"
                  type="number"
                  min={1}
                  step={1000}
                  value={form.maxOutputTokens}
                  onChange={(event) => setForm((prev) => ({ ...prev, maxOutputTokens: event.target.value }))}
                />
              </Field>
              {reasoningOptions.length ? (
                <Field label="Reasoning">
                  <SelectField
                    data-testid="llm-reasoning-effort-select"
                    aria-label="Reasoning"
                    value={form.reasoningEffort}
                    options={reasoningOptions.map((option) => ({ label: option, value: option }))}
                    onChange={(reasoningEffort) => setForm((prev) => ({ ...prev, reasoningEffort }))}
                  />
                </Field>
              ) : null}
              {providerPreset.supportsThinking ? (
                <Field label="Thinking">
                  <SelectField
                    data-testid="llm-thinking-type-select"
                    aria-label="Thinking"
                    value={form.thinkingType}
                    options={providerPreset.thinkingOptions.map((option) => ({ label: option, value: option }))}
                    onChange={(thinkingType) => setForm((prev) => ({ ...prev, thinkingType }))}
                  />
                </Field>
              ) : null}
              {needsApiKey ? (
                <Field label="API Key">
                  <Input data-testid="llm-api-key-input" type="password" value={form.apiKey} onChange={(event) => setForm((prev) => ({ ...prev, apiKey: event.target.value }))} />
                </Field>
              ) : null}
              {providerPreset.supportsTemperature ? (
                <Field label="Temperature" hint={showSamplingNoEffectHint ? "DeepSeek thinking enabled 时采样参数不会生效。" : undefined}>
                  <Input data-testid="llm-temperature-input" type="number" step="0.01" value={form.temperature} onChange={(event) => setForm((prev) => ({ ...prev, temperature: event.target.value }))} />
                </Field>
              ) : null}
              {providerPreset.supportsTopP ? (
                <Field label="Top P" hint={showSamplingNoEffectHint ? "DeepSeek thinking enabled 时 top_p 不会生效。" : undefined}>
                  <Input data-testid="llm-top-p-input" type="number" step="0.01" value={form.topP} onChange={(event) => setForm((prev) => ({ ...prev, topP: event.target.value }))} />
                </Field>
              ) : null}
              {providerPreset.supportsToolStream ? (
                <Field label="Tool Stream">
                  <label className="flex h-8 items-center gap-2 text-sm text-slate-700">
                    <input data-testid="llm-tool-stream-checkbox" type="checkbox" checked={form.toolStream} onChange={(event) => setForm((prev) => ({ ...prev, toolStream: event.target.checked }))} />
                    启用工具调用过程流式输出
                  </label>
                </Field>
              ) : null}
            </CardContent>
          </Card>
        </>
      )}
    </SettingsPageFrame>
  );
}
