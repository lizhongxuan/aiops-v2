import { RefreshCw, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field, LoadingState, SelectField, SettingsPageFrame, StatGrid, StatusAlert } from "@/pages/settingsComponents";
import { fetchLlmConfig, type LlmConfigView, updateLlmConfig } from "@/pages/settingsApi";

const providers = [
  { label: "OpenAI", value: "openai" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Ollama", value: "ollama" },
];

const modelPresets: Record<string, string[]> = {
  openai: ["gpt-5.4", "gpt-5.4-mini", "gpt-4o", "gpt-4o-mini", "o3-mini"],
  anthropic: ["claude-sonnet-4-20250514", "claude-3-5-sonnet-20241022", "claude-3-haiku-20240307"],
  ollama: ["qwen2.5:7b", "qwen2.5:14b", "llama3.1:8b", "deepseek-coder-v2:16b"],
};

function defaultBaseURL(provider: string) {
  if (provider === "openai") return "https://api.openai.com/v1";
  if (provider === "anthropic") return "https://api.anthropic.com";
  if (provider === "ollama") return "http://127.0.0.1:11434/v1";
  return "";
}

export function LLMConfigPage() {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [config, setConfig] = useState<LlmConfigView | null>(null);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);
  const [form, setForm] = useState({ provider: "openai", model: "gpt-5.4", apiKey: "", baseURL: "" });

  const modelOptions = useMemo(() => (modelPresets[form.provider] || []).map((model) => ({ label: model, value: model })), [form.provider]);
  const needsApiKey = form.provider !== "ollama";
  const connected = Boolean(config?.bifrostActive && (!needsApiKey || config.apiKeySet));

  async function load() {
    setLoading(true);
    try {
      const next = await fetchLlmConfig();
      setConfig(next);
      setForm({
        provider: next.provider || "openai",
        model: next.model || "gpt-5.4",
        apiKey: "",
        baseURL: next.baseURL || "",
      });
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
      const payload = { ...form };
      if (!payload.apiKey) delete (payload as { apiKey?: string }).apiKey;
      const result = await updateLlmConfig(payload);
      setMessage({ type: result.ok === false ? "info" : "success", text: result.message || result.error || "配置已保存" });
      await load();
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="LLM 配置"
      description="配置主模型 Provider、模型名和兼容 OpenAI 格式的 Base URL。API Key 留空时保持现有密钥。"
      actions={
        <>
          <Button variant="outline" onClick={() => void load()} disabled={loading || saving}>
            <RefreshCw />
            刷新
          </Button>
          <Button data-testid="llm-save-button" onClick={() => void save()} disabled={loading || saving}>
            <Save />
            保存并重启 Runtime
          </Button>
        </>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {loading && !config ? (
        <LoadingState label="加载 LLM 配置" />
      ) : (
        <>
          <StatGrid
            items={[
              { label: "Provider", value: config?.provider || form.provider },
              { label: "Model", value: config?.model || form.model },
              { label: "API Key", value: config?.apiKeySet ? config.apiKeyMasked || "已设置" : "未设置", tone: config?.apiKeySet ? "ok" : "warn" },
              { label: "模型状态", value: connected ? "已配置" : "未连接", tone: connected ? "ok" : "bad" },
            ]}
          />

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>主 LLM 配置</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-2">
              <Field label="Provider">
                <SelectField
                  aria-label="Provider"
                  value={form.provider}
                  options={providers}
                  onChange={(provider) =>
                    setForm({
                      provider,
                      model: modelPresets[provider]?.[0] || form.model,
                      apiKey: "",
                      baseURL: provider === "ollama" ? defaultBaseURL(provider) : "",
                    })
                  }
                />
              </Field>
              <Field label="Model">
                <Input list="llm-model-presets" value={form.model} onChange={(event) => setForm((prev) => ({ ...prev, model: event.target.value }))} />
                <datalist id="llm-model-presets">
                  {modelOptions.map((option) => (
                    <option key={option.value} value={option.value} />
                  ))}
                </datalist>
              </Field>
              {needsApiKey ? (
                <Field label="API Key" hint={config?.apiKeySet ? "已设置时留空会保持原密钥。" : "Provider 需要 API Key。"}>
                  <Input type="password" value={form.apiKey} onChange={(event) => setForm((prev) => ({ ...prev, apiKey: event.target.value }))} />
                </Field>
              ) : null}
              <Field label="Base URL" hint={`默认：${defaultBaseURL(form.provider) || "官方地址"}`}>
                <Input value={form.baseURL} onChange={(event) => setForm((prev) => ({ ...prev, baseURL: event.target.value }))} />
              </Field>
            </CardContent>
          </Card>
        </>
      )}
    </SettingsPageFrame>
  );
}
