import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { fetchCorootConfig, saveCorootConfig, testCorootConnection, type CorootAuthMode } from "@/api/coroot";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field, SelectField, SettingsPageFrame, StatusAlert } from "@/pages/settingsComponents";

const authModeOptions = [
  { label: "anonymous_readonly", value: "anonymous_readonly" },
  { label: "embed_trust", value: "embed_trust" },
  { label: "session_passthrough", value: "session_passthrough" },
];

export function CorootConfigPage() {
  const navigate = useNavigate();
  const [baseUrl, setBaseUrl] = useState("");
  const [project, setProject] = useState("");
  const [authMode, setAuthMode] = useState<CorootAuthMode>("anonymous_readonly");
  const [token, setToken] = useState("");
  const [timeout, setTimeoutValue] = useState("30s");
  const [status, setStatus] = useState<{ type: "success" | "error" | "info"; title: string; message: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const needsSharedSecret = authMode === "embed_trust";

  useEffect(() => {
    let cancelled = false;
    fetchCorootConfig()
      .then((config) => {
        if (cancelled) return;
        setBaseUrl(config.baseUrl || "");
        setProject(config.project || "");
        setAuthMode(config.authMode || "anonymous_readonly");
      })
      .catch(() => {
        if (!cancelled) setStatus({ type: "info", title: "等待配置", message: "填写 Coroot 网关地址和项目 ID 后保存。" });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function saveConfig({ enter }: { enter: boolean }) {
    setSaving(true);
    setStatus(null);
    try {
      const config = await saveCorootConfig(buildCorootConfigInput({
        baseUrl,
        project,
        authMode,
        secret: token,
        timeout,
      }));
      setStatus({ type: "success", title: "已保存", message: "Coroot 嵌入配置已更新。" });
      if (enter) navigate(config.entryPath || `/coroot/p/${config.project || project || "default"}/applications`);
    } catch (error) {
      setStatus({ type: "error", title: "保存失败", message: error instanceof Error ? error.message : String(error) });
    } finally {
      setSaving(false);
    }
  }

  async function testConnection() {
    setStatus(null);
    try {
      const result = await testCorootConnection(buildCorootConfigInput({ baseUrl, project, authMode, secret: token, timeout }));
      setStatus({
        type: result.ok ? "success" : "error",
        title: result.ok ? "连接可用" : "连接失败",
        message: result.message || result.error || (result.ok ? "Coroot 网关已响应。" : "Coroot 网关未返回可用状态。"),
      });
    } catch (error) {
      setStatus({ type: "error", title: "连接失败", message: error instanceof Error ? error.message : String(error) });
    }
  }

  return (
    <SettingsPageFrame title="Coroot 配置" description="连接 Coroot 嵌入式监控视图" contentClassName="max-w-4xl">
      {status ? <StatusAlert type={status.type} title={status.title} message={status.message} /> : null}
      <Card className="rounded-lg bg-white">
        <CardHeader>
          <CardTitle>嵌入配置</CardTitle>
          <CardDescription>保存后将通过 AIOps 路由打开 Coroot 工作区。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <Field label="Base URL">
            <Input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="http://172.18.13.11:8000" />
          </Field>
          <Field label="Project ID">
            <Input value={project} onChange={(event) => setProject(event.target.value)} placeholder="5hxbfx6p" />
          </Field>
          <Field label="Auth mode">
            <SelectField value={authMode} onChange={(value) => setAuthMode(value as CorootAuthMode)} options={authModeOptions} />
          </Field>
          {needsSharedSecret ? (
            <Field label="Embed trust shared secret" hint="留空表示保留已保存密钥。">
              <Input type="password" value={token} onChange={(event) => setToken(event.target.value)} />
            </Field>
          ) : null}
          <Field label="Timeout">
            <Input value={timeout} onChange={(event) => setTimeoutValue(event.target.value)} placeholder="30s" />
          </Field>
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={testConnection}>
              测试连接
            </Button>
            <Button type="button" variant="outline" disabled={saving} onClick={() => saveConfig({ enter: false })}>
              保存
            </Button>
            <Button type="button" disabled={saving} onClick={() => saveConfig({ enter: true })}>
              保存并进入 Coroot
            </Button>
          </div>
        </CardContent>
      </Card>
    </SettingsPageFrame>
  );
}

function buildCorootConfigInput({
  baseUrl,
  project,
  authMode,
  secret,
  timeout,
}: {
  baseUrl: string;
  project: string;
  authMode: CorootAuthMode;
  secret: string;
  timeout: string;
}) {
  const trimmedSecret = secret.trim();
  return {
    baseUrl,
    project,
    authMode,
    embedMode: authMode === "embed_trust" ? "full" : "readonly",
    uiGatewayEnabled: true,
    clearToken: false,
    embedTrustSecret: authMode === "embed_trust" && trimmedSecret ? trimmedSecret : undefined,
    timeout,
  };
}
