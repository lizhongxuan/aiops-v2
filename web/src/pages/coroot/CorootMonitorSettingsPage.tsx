import { RefreshCw, Save, TestTube2 } from "lucide-react";
import { useEffect, useState } from "react";

import { fetchCorootConfig, saveCorootConfig, testCorootConnection, type CorootAuthMode } from "@/api/coroot";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { fetchMcpRuntimeHealth, refreshMcpRuntimeHealth, type McpHealthRecord } from "@/pages/complexPagesApi";
import { Field, LoadingState, SelectField, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";

const authModeOptions = [
  { label: "anonymous_readonly", value: "anonymous_readonly" },
  { label: "embed_trust", value: "embed_trust" },
  { label: "session_passthrough", value: "session_passthrough" },
];

export function CorootMonitorSettingsPage() {
  const [baseUrl, setBaseUrl] = useState("");
  const [project, setProject] = useState("");
  const [authMode, setAuthMode] = useState<CorootAuthMode>("anonymous_readonly");
  const [apiToken, setApiToken] = useState("");
  const [apiTokenConfigured, setApiTokenConfigured] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [passwordConfigured, setPasswordConfigured] = useState(false);
  const [embedSecret, setEmbedSecret] = useState("");
  const [timeout, setTimeoutValue] = useState("30s");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [mcpRefreshing, setMcpRefreshing] = useState(false);
  const [mcpHealth, setMcpHealth] = useState<McpHealthRecord | null>(null);
  const [status, setStatus] = useState<{ type: "success" | "error" | "info"; title: string; message: string } | null>(null);
  const [mcpError, setMcpError] = useState("");
  const needsSharedSecret = authMode === "embed_trust";

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setMcpError("");
      try {
        const [config, health] = await Promise.all([fetchCorootConfig(), fetchMcpRuntimeHealth()]);
        if (cancelled) return;
        setBaseUrl(config.baseUrl || "");
        setProject(config.project || "");
        setAuthMode(config.authMode || "anonymous_readonly");
        setApiTokenConfigured(Boolean(config.tokenConfigured));
        setUsername(config.username || "");
        setPasswordConfigured(Boolean(config.passwordConfigured));
        setMcpHealth(resolveCorootHealth(health.items));
        setStatus(config.configured ? null : { type: "info", title: "等待配置", message: "填写 Coroot 地址和项目 ID 后保存。" });
      } catch (error) {
        if (cancelled) return;
        setStatus({ type: "error", title: "加载失败", message: error instanceof Error ? error.message : String(error) });
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  async function saveConfig() {
    setSaving(true);
    setStatus(null);
    try {
      const config = await saveCorootConfig(buildCorootConfigInput({ baseUrl, project, authMode, apiToken, username, password, embedSecret, timeout }));
      setBaseUrl(config.baseUrl || baseUrl);
      setProject(config.project || project);
      setAuthMode(config.authMode || authMode);
      setApiTokenConfigured(Boolean(config.tokenConfigured || apiToken.trim()));
      setUsername(config.username || username);
      setPasswordConfigured(Boolean(config.passwordConfigured || password.trim()));
      setApiToken("");
      setPassword("");
      setEmbedSecret("");
      setStatus({ type: "success", title: "已保存", message: "Coroot 监控配置已更新，AI Chat 下一次 @Coroot 请求会使用当前配置。" });
    } catch (error) {
      setStatus({ type: "error", title: "保存失败", message: error instanceof Error ? error.message : String(error) });
    } finally {
      setSaving(false);
    }
  }

  async function testConnection() {
    setStatus(null);
    try {
      const result = await testCorootConnection(buildCorootConfigInput({ baseUrl, project, authMode, apiToken, username, password, embedSecret, timeout }));
      setStatus({
        type: result.ok ? "success" : "error",
        title: result.ok ? "连接可用" : "连接失败",
        message: result.message || result.error || (result.ok ? "Coroot 网关已响应。" : "Coroot 网关未返回可用状态。"),
      });
    } catch (error) {
      setStatus({ type: "error", title: "连接失败", message: error instanceof Error ? error.message : String(error) });
    }
  }

  async function refreshMcpHealth() {
    setMcpRefreshing(true);
    setMcpError("");
    try {
      const next = await refreshMcpRuntimeHealth("coroot");
      setMcpHealth(next);
    } catch (error) {
      setMcpError(error instanceof Error ? error.message : String(error));
    } finally {
      setMcpRefreshing(false);
    }
  }

  return (
    <SettingsPageFrame title="Coroot 监控配置" description="配置 Coroot 嵌入地址、凭证，以及 AI Chat 可见的 Coroot MCP 状态。">
      {status ? <StatusAlert type={status.type} title={status.title} message={status.message} /> : null}
      {loading ? (
        <LoadingState label="加载 Coroot 监控配置" />
      ) : (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
          <Card id="connection" className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>连接配置</CardTitle>
              <CardDescription>保存后用于 Coroot 嵌入页面；AI Chat 在服务端读取证据，不会自动拿 iframe cookie。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-2">
              <Field label="Base URL">
                <Input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="http://172.18.13.11:8000" />
              </Field>
              <Field label="Project ID">
                <Input value={project} onChange={(event) => setProject(event.target.value)} placeholder="5hxbfx6p" />
              </Field>
              <Field label="Auth mode">
                <SelectField value={authMode} onChange={(value) => setAuthMode(value as CorootAuthMode)} options={authModeOptions} />
              </Field>
              <Field label="Timeout">
                <Input value={timeout} onChange={(event) => setTimeoutValue(event.target.value)} placeholder="30s" />
              </Field>
              <Field
                label="AI Chat Web Session / API 凭证"
                hint={
                  apiTokenConfigured
                    ? "已保存凭证；留空表示保留现有值。可粘贴 coroot_session=... 或纯 session 值。"
                    : "Coroot 页面数据接口需要 Web session 或 Embed Trust；项目 API Key 通常只用于 collector / Prometheus 接口。"
                }
              >
                <Input type="password" value={apiToken} onChange={(event) => setApiToken(event.target.value)} placeholder={apiTokenConfigured ? "已配置，留空不变" : "粘贴 coroot_session 或兼容 API 凭证"} />
              </Field>
              <Field label="Coroot Web 用户名" hint="用于 AI Chat 后端登录 Coroot Web API；留空则只使用上方凭证或 Embed Trust。">
                <Input value={username} onChange={(event) => setUsername(event.target.value)} placeholder="admin" />
              </Field>
              <Field
                label="Coroot Web 密码"
                hint={passwordConfigured ? "已保存密码；留空表示保留现有密码。" : "用于 AI Chat 后端登录获取 coroot_session。"}
              >
                <Input type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder={passwordConfigured ? "已配置，留空不变" : "输入 Coroot Web 密码"} />
              </Field>
              {needsSharedSecret ? (
                <Field label="Embed trust shared secret" hint="留空表示保留已保存密钥。">
                  <Input type="password" value={embedSecret} onChange={(event) => setEmbedSecret(event.target.value)} />
                </Field>
              ) : null}
              <div className="flex flex-wrap items-end gap-2 md:col-span-2">
                <Button type="button" variant="outline" onClick={() => void testConnection()}>
                  <TestTube2 className="h-4 w-4" />
                  测试连接
                </Button>
                <Button type="button" disabled={saving} onClick={() => void saveConfig()}>
                  <Save className="h-4 w-4" />
                  {saving ? "保存中" : "保存配置"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card id="mcp-status" className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>AI Chat 可见的 Coroot MCP</CardTitle>
              <CardDescription>@Coroot 根因定位会通过这个 MCP 读取 Coroot 证据。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="text-slate-500">状态</span>
                <ToneBadge tone={mcpHealth?.status === "healthy" ? "success" : "warning"}>{mcpHealth?.status || "missing"}</ToneBadge>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-slate-500">Server ID</span>
                <span className="font-mono text-slate-900">{mcpHealth?.serverId || "coroot"}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-slate-500">可用工具</span>
                <span className="font-medium text-slate-900">{mcpHealth?.availableToolCount ?? 0}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-slate-500">API Token</span>
                <ToneBadge tone={apiTokenConfigured ? "success" : "warning"}>{apiTokenConfigured ? "configured" : "missing"}</ToneBadge>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-slate-500">Web 登录</span>
                <ToneBadge tone={passwordConfigured || username ? "success" : "warning"}>{passwordConfigured || username ? "configured" : "missing"}</ToneBadge>
              </div>
              {!apiTokenConfigured && !passwordConfigured && authMode !== "embed_trust" ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
                  嵌入 UI 可以复用浏览器登录态；AI Chat MCP 在后端读取 Coroot 证据，不会自动拿 iframe cookie。请配置 coroot_session、Coroot Web 用户名/密码，或启用 Embed Trust。
                </div>
              ) : null}
              {mcpHealth?.lastError || mcpError ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">{mcpError || mcpHealth?.lastError}</div>
              ) : null}
              <Button type="button" variant="outline" onClick={() => void refreshMcpHealth()} disabled={mcpRefreshing}>
                <RefreshCw className="h-4 w-4" />
                {mcpRefreshing ? "刷新中" : "刷新 MCP 状态"}
              </Button>
            </CardContent>
          </Card>
        </div>
      )}
    </SettingsPageFrame>
  );
}

function resolveCorootHealth(items?: McpHealthRecord[]) {
  return items?.find((item) => item.serverId === "coroot") || null;
}

function buildCorootConfigInput({
  baseUrl,
  project,
  authMode,
  apiToken,
  username,
  password,
  embedSecret,
  timeout,
}: {
  baseUrl: string;
  project: string;
  authMode: CorootAuthMode;
  apiToken: string;
  username: string;
  password: string;
  embedSecret: string;
  timeout: string;
}) {
  const trimmedToken = apiToken.trim();
  const trimmedUsername = username.trim();
  const trimmedPassword = password.trim();
  const trimmedEmbedSecret = embedSecret.trim();
  return {
    baseUrl,
    project,
    authMode,
    embedMode: authMode === "embed_trust" ? ("full" as const) : ("readonly" as const),
    uiGatewayEnabled: true,
    clearToken: false,
    token: trimmedToken || undefined,
    username: trimmedUsername || undefined,
    password: trimmedPassword || undefined,
    clearPassword: false,
    embedTrustSecret: authMode === "embed_trust" && trimmedEmbedSecret ? trimmedEmbedSecret : undefined,
    timeout,
  };
}
