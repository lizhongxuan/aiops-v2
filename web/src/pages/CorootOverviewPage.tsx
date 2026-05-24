import { CheckCircle2, Save, Settings, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";

type CorootConfig = {
  configured?: boolean;
  baseUrl?: string;
  proxyBaseUrl?: string;
  iframeUrl?: string;
  iframeMode?: boolean;
  project?: string;
  timeout?: string;
  tokenConfigured?: boolean;
  lastSuccessAt?: string;
};

type CorootConfigDraft = {
  baseUrl: string;
  project: string;
  token: string;
  iframeUrl: string;
  timeout: string;
};

type McpServer = {
  name?: string;
  status?: string;
  toolCount?: number;
  resourceCount?: number;
  error?: string;
};

type EvidenceItem = {
  id?: string;
  evidence_ref?: string;
  evidenceRef?: string;
  title?: string;
  summary?: string;
  case_id?: string;
  caseId?: string;
  created_at?: string;
  createdAt?: string;
};

type ArtifactItem = {
  id?: string;
  artifactId?: string;
  type?: string;
  title?: string;
  summary?: string;
  case_id?: string;
  caseId?: string;
  created_at?: string;
  createdAt?: string;
};

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), {
    credentials: "include",
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

function itemsFrom<T>(payload: unknown): T[] {
  if (Array.isArray(payload)) return payload as T[];
  if (payload && typeof payload === "object" && Array.isArray((payload as { items?: unknown[] }).items)) {
    return (payload as { items: T[] }).items;
  }
  return [];
}

function text(value: unknown, fallback = "-") {
  const normalized = typeof value === "string" ? value.trim() : String(value || "").trim();
  return normalized || fallback;
}

function caseIdOf(item: EvidenceItem | ArtifactItem) {
  return text(item.caseId || item.case_id, "");
}

export function CorootOverviewPage() {
  const [config, setConfig] = useState<CorootConfig>({});
  const [draft, setDraft] = useState<CorootConfigDraft>(() => draftFromConfig({}));
  const [mcpServers, setMcpServers] = useState<McpServer[]>([]);
  const [evidence, setEvidence] = useState<EvidenceItem[]>([]);
  const [artifacts, setArtifacts] = useState<ArtifactItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const corootMcpServers = useMemo(
    () => mcpServers.filter((server) => text(server.name, "").toLowerCase().includes("coroot")),
    [mcpServers],
  );
  const connectedMcpCount = corootMcpServers.filter((server) => text(server.status, "").toLowerCase() === "connected").length;
  const rcaEnabled = connectedMcpCount > 0 && corootMcpServers.some((server) => Number(server.toolCount || 0) > 0);

  async function load() {
    setLoading(true);
    try {
      const [nextConfig, mcpPayload, evidencePayload, artifactPayload] = await Promise.all([
        requestJson<CorootConfig>("/api/v1/coroot/config").catch(() => ({ configured: false })),
        requestJson<{ items?: McpServer[] }>("/api/v1/mcp/servers").catch(() => ({ items: [] })),
        requestJson<{ items?: EvidenceItem[] }>("/api/v1/coroot/evidence").catch(() => ({ items: [] })),
        requestJson<{ items?: ArtifactItem[] }>("/api/v1/agent-ui-artifacts?source=coroot").catch(() => ({ items: [] })),
      ]);
      setConfig(nextConfig);
      setDraft(draftFromConfig(nextConfig));
      setMcpServers(itemsFrom<McpServer>(mcpPayload));
      setEvidence(itemsFrom<EvidenceItem>(evidencePayload));
      setArtifacts(itemsFrom<ArtifactItem>(artifactPayload));
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 Coroot 信息失败" });
    } finally {
      setLoading(false);
    }
  }

  async function testConnection() {
    try {
      await requestJson("/api/v1/coroot/test-connection", { method: "POST" });
      await load();
      setMessage({ type: "success", text: "连接正常" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "测试连接失败" });
    }
  }

  async function saveConfig() {
    try {
      const nextConfig = await requestJson<CorootConfig>("/api/v1/coroot/config", {
        method: "PUT",
        body: JSON.stringify(draft),
      });
      setConfig(nextConfig);
      setDraft(draftFromConfig(nextConfig));
      setMessage({ type: "success", text: "配置已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存 Coroot 配置失败" });
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="Coroot 观测"
      description="只展示 Coroot 配置、MCP 状态、RCA Skills、Evidence 和发送到 AI Chat 的图表。"
      actions={
        <Button onClick={() => void testConnection()}><CheckCircle2 />测试连接</Button>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "success" ? "操作完成" : "操作失败"} message={message.text} /> : null}

      {config.configured === false ? (
        <Card data-testid="coroot-not-configured" className="rounded-lg border-amber-200 bg-amber-50">
          <CardHeader><CardTitle>Coroot 未配置</CardTitle><CardDescription>请先配置 Coroot upstream。</CardDescription></CardHeader>
        </Card>
      ) : null}

      <div className="grid gap-3 md:grid-cols-3">
        <Card className="ops-statistic rounded-lg bg-white">
          <CardHeader><CardDescription>Coroot MCP</CardDescription><CardTitle>{connectedMcpCount ? "已连接" : "未连接"}</CardTitle></CardHeader>
        </Card>
        <Card className="ops-statistic rounded-lg bg-white">
          <CardHeader><CardDescription>RCA Skills</CardDescription><CardTitle>{rcaEnabled ? "已启用" : "未启用"}</CardTitle></CardHeader>
        </Card>
        <Card className="ops-statistic rounded-lg bg-white">
          <CardHeader><CardDescription>最近 Evidence</CardDescription><CardTitle>{evidence.length}</CardTitle></CardHeader>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="grid gap-4">
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Settings className="size-4" />Coroot 配置</CardTitle>
              <CardDescription>连接信息从这里保存并持久化，服务启动参数不再作为 Coroot 配置来源。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 text-sm text-slate-700">
              <div className="grid gap-2">
                <InfoRow label="配置状态" value={config.configured === false ? "未配置" : "已配置"} />
                <InfoRow label="Base URL" value={text(config.baseUrl, "未设置")} />
                <InfoRow label="Project ID" value={text(config.project, "default")} />
                <InfoRow label="API Key" value={config.tokenConfigured ? "已保存" : "未设置"} />
                <InfoRow label="最近成功连接" value={text(config.lastSuccessAt, "暂无")} />
              </div>
              <form
                className="grid gap-3 rounded-lg border bg-slate-50 p-3"
                onSubmit={(event) => {
                  event.preventDefault();
                  void saveConfig();
                }}
              >
                <ConfigInput
                  label="Base URL"
                  name="baseUrl"
                  value={draft.baseUrl}
                  placeholder="例如 http://172.18.13.11:8000"
                  required
                  onChange={(value) => setDraft((current) => ({ ...current, baseUrl: value }))}
                />
                <div className="grid gap-3 md:grid-cols-2">
                  <ConfigInput
                    label="Project ID"
                    name="project"
                    value={draft.project}
                    placeholder="default"
                    onChange={(value) => setDraft((current) => ({ ...current, project: value }))}
                  />
                  <ConfigInput
                    label="API Key"
                    name="token"
                    type="password"
                    value={draft.token}
                    placeholder={config.tokenConfigured ? "已保存，留空表示不修改" : "粘贴 Coroot API Key"}
                    onChange={(value) => setDraft((current) => ({ ...current, token: value }))}
                  />
                </div>
                <div className="grid gap-3 md:grid-cols-2">
                  <ConfigInput
                    label="Iframe URL"
                    name="iframeUrl"
                    value={draft.iframeUrl}
                    placeholder="可选，默认使用内置代理"
                    onChange={(value) => setDraft((current) => ({ ...current, iframeUrl: value }))}
                  />
                  <ConfigInput
                    label="Timeout"
                    name="timeout"
                    value={draft.timeout}
                    placeholder="30s"
                    onChange={(value) => setDraft((current) => ({ ...current, timeout: value }))}
                  />
                </div>
                <div className="flex justify-end">
                  <Button type="submit"><Save />保存配置</Button>
                </div>
              </form>
            </CardContent>
          </Card>

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>MCP 状态</CardTitle>
              <CardDescription>只展示 Coroot MCP 服务和可用资源。</CardDescription>
            </CardHeader>
            <CardContent>
              {loading ? <p className="text-sm text-slate-500">加载中...</p> : <RecordList items={corootMcpServers} empty="暂无 Coroot MCP 服务" renderItem={(server, index) => (
                <div key={server.name || index} className="rounded-lg border bg-slate-50 p-3 text-sm">
                  <div className="flex items-center justify-between gap-3">
                    <strong>{text(server.name, `coroot-mcp-${index + 1}`)}</strong>
                    <ToneBadge tone={text(server.status, "").toLowerCase() === "connected" ? "success" : "warning"}>{text(server.status, "unknown")}</ToneBadge>
                  </div>
                  <div className="mt-1 text-xs text-slate-500">{Number(server.toolCount || 0)} tools · {Number(server.resourceCount || 0)} resources</div>
                  {server.error ? <div className="mt-1 text-xs text-red-600">{server.error}</div> : null}
                </div>
              )} />}
            </CardContent>
          </Card>

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>最近 Evidence</CardTitle>
              <CardDescription>Coroot 证据可以进入 Case，也可以被 Prompt Trace 追溯。</CardDescription>
            </CardHeader>
            <CardContent>
              <RecordList items={evidence} empty="暂无 Coroot Evidence" renderItem={(item, index) => (
                <EvidenceCard key={text(item.evidenceRef || item.evidence_ref || item.id, String(index))} item={item} />
              )} />
            </CardContent>
          </Card>
        </div>

        <aside className="grid content-start gap-4">
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4" />RCA Skills</CardTitle>
              <CardDescription>由 Coroot MCP 工具和资源决定是否可用。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2 text-sm">
              <ToneBadge tone={rcaEnabled ? "success" : "warning"}>{rcaEnabled ? "Coroot RCA 已启用" : "Coroot RCA 未启用"}</ToneBadge>
              <p className="text-slate-600">可用于慢请求、服务依赖、中间件异常的根因证据采集。</p>
            </CardContent>
          </Card>

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>最近发送到 AI Chat 的图表</CardTitle>
              <CardDescription>Agent-to-UI Coroot 图表 artifact。</CardDescription>
            </CardHeader>
            <CardContent>
              <RecordList items={artifacts} empty="暂无图表 artifact" renderItem={(item, index) => (
                <ArtifactCard key={text(item.id || item.artifactId, String(index))} item={item} />
              )} />
            </CardContent>
          </Card>
        </aside>
      </div>
    </SettingsPageFrame>
  );
}

function draftFromConfig(config: CorootConfig): CorootConfigDraft {
  return {
    baseUrl: text(config.baseUrl, ""),
    project: text(config.project, "default"),
    token: "",
    iframeUrl: text(config.iframeUrl, ""),
    timeout: text(config.timeout, "30s"),
  };
}

function ConfigInput({
  label,
  name,
  value,
  placeholder,
  type = "text",
  required = false,
  onChange,
}: {
  label: string;
  name: string;
  value: string;
  placeholder?: string;
  type?: string;
  required?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid gap-1 text-xs font-medium text-slate-600">
      <span>{label}</span>
      <input
        className="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none transition focus:border-slate-400"
        name={name}
        type={type}
        value={value}
        placeholder={placeholder}
        required={required}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 border-b border-slate-100 pb-2 last:border-0">
      <span className="text-slate-500">{label}</span>
      <span className="break-all text-right font-medium text-slate-900">{value}</span>
    </div>
  );
}

function RecordList<T>({ items, empty, renderItem }: { items: T[]; empty: string; renderItem: (item: T, index: number) => ReactNode }) {
  if (!items.length) return <p className="text-sm text-slate-500">{empty}</p>;
  return <div className="grid gap-2">{items.map(renderItem)}</div>;
}

function EvidenceCard({ item }: { item: EvidenceItem }) {
  const evidenceRef = text(item.evidenceRef || item.evidence_ref || item.id, "unknown-evidence");
  const caseId = caseIdOf(item);
  return (
    <div className="rounded-lg border bg-slate-50 p-3 text-sm">
      <div className="font-medium">{text(item.title, evidenceRef)}</div>
      <div className="mt-1 text-xs text-slate-500">{text(item.summary)}</div>
      <div className="mt-1 break-all text-xs text-slate-400">{evidenceRef}</div>
      {caseId ? <Link className="mt-2 inline-block text-xs font-medium text-slate-900 underline-offset-4 hover:underline" to={`/incidents/${encodeURIComponent(caseId)}`}>查看 Case</Link> : null}
    </div>
  );
}

function ArtifactCard({ item }: { item: ArtifactItem }) {
  const artifactId = text(item.id || item.artifactId, "unknown-artifact");
  const caseId = caseIdOf(item);
  return (
    <div className="rounded-lg border bg-slate-50 p-3 text-sm">
      <div className="font-medium">{text(item.title, artifactId)}</div>
      <div className="mt-1 text-xs text-slate-500">{text(item.summary || item.type)}</div>
      <div className="mt-1 break-all text-xs text-slate-400">{artifactId}</div>
      <div className="mt-2 flex flex-wrap gap-2 text-xs">
        {caseId ? <Link className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/incidents/${encodeURIComponent(caseId)}`}>查看 Case</Link> : null}
        <Link className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/debug/prompts?artifact_id=${encodeURIComponent(artifactId)}`}>查看 Prompt Trace</Link>
      </div>
    </div>
  );
}
