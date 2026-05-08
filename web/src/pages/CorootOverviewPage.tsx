import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type Service = { id: string; name?: string; status?: string };
type CorootConfig = { configured?: boolean; baseUrl?: string; iframeMode?: boolean };

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function CorootOverviewPage() {
  const [config, setConfig] = useState<CorootConfig>({});
  const [services, setServices] = useState<Service[]>([]);
  const [tab, setTab] = useState("services");
  const [embedUrl, setEmbedUrl] = useState("/api/v1/coroot/");
  const [drawerOpen, setDrawerOpen] = useState(false);

  useEffect(() => {
    async function load() {
      const nextConfig = await requestJson<CorootConfig>("/api/v1/coroot/config").catch(() => ({ configured: false }));
      setConfig(nextConfig);
      if (nextConfig.configured !== false) {
        const payload = await requestJson<Service[] | { items?: Service[] }>("/api/v1/coroot/api/v1/services").catch(() => []);
        setServices(Array.isArray(payload) ? payload : payload.items || []);
      }
    }
    void load().catch(() => undefined);
  }, []);

  const stats = useMemo(() => ({
    ok: services.filter((item) => item.status === "ok" || item.status === "healthy").length,
    warning: services.filter((item) => item.status === "warning").length,
    critical: services.filter((item) => item.status === "critical" || item.status === "error").length,
  }), [services]);

  function openService(service: Service) {
    setEmbedUrl(`/api/v1/coroot/api/v1/services/${encodeURIComponent(service.id)}/overview`);
    setTab("services");
  }

  function openTopology() {
    setEmbedUrl("/api/v1/coroot/api/v1/topology");
    setTab("topology");
  }

  async function sendQuickAction(label: string) {
    await requestJson("/api/v1/chat/message", {
      method: "POST",
      body: JSON.stringify({
        message: label,
        monitorContext: { source: "coroot", tab, services },
      }),
    }).catch(() => undefined);
  }

  return (
    <SettingsPageFrame
      title="Coroot 监控总览"
      description="展示 Coroot 服务列表、Dashboard iframe、拓扑入口和 AI monitor context。"
      actions={<Button onClick={() => setDrawerOpen(true)}>AI 助手</Button>}
    >
      {config.configured === false ? (
        <Card data-testid="coroot-not-configured" className="rounded-lg border-amber-200 bg-amber-50">
          <CardHeader><CardTitle>Coroot 未配置</CardTitle><CardDescription>请先配置 Coroot upstream。</CardDescription></CardHeader>
        </Card>
      ) : null}

      <div className="grid gap-3 md:grid-cols-3">
        <Card className="ops-statistic rounded-lg bg-white"><CardHeader><CardDescription>健康</CardDescription><CardTitle>{stats.ok}</CardTitle></CardHeader></Card>
        <Card className="ops-statistic rounded-lg bg-white"><CardHeader><CardDescription>告警</CardDescription><CardTitle>{stats.warning}</CardTitle></CardHeader></Card>
        <Card className="ops-statistic rounded-lg bg-white"><CardHeader><CardDescription>异常</CardDescription><CardTitle>{stats.critical}</CardTitle></CardHeader></Card>
      </div>

      <div className="flex flex-wrap gap-2" data-testid="coroot-tab-bar">
        {[
          ["services", "服务总览"],
          ["dashboard", "Dashboard"],
          ["topology", "拓扑视图"],
        ].map(([key, label]) => (
          <button key={key} className={`ops-tabs-tab rounded-lg border px-3 py-2 text-sm ${tab === key ? "active bg-slate-900 text-white" : "bg-white"}`} type="button" onClick={() => { setTab(key); if (key === "dashboard") setEmbedUrl(config.baseUrl || "/api/v1/coroot/"); if (key === "topology") openTopology(); }}>{label}</button>
        ))}
      </div>

      {tab === "services" ? (
        <Card data-testid="tab-content-services" className="rounded-lg bg-white">
          <CardHeader><CardTitle>服务列表</CardTitle></CardHeader>
          <CardContent>
            <div className="ops-data-table-table overflow-auto">
              <table className="data-table w-full border-collapse text-sm">
                <tbody>
                  {services.map((service) => (
                    <tr key={service.id}>
                      <td className="border-b p-2 font-medium">{service.name || service.id}</td>
                      <td className="border-b p-2">{service.status}</td>
                      <td className="border-b p-2"><Button size="sm" variant="outline" onClick={() => openService(service)}>详情</Button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {config.configured === false ? null : (
              <section className="embed-panel mt-4 rounded-lg border bg-slate-50 p-3">
                <iframe title="Coroot service overview" className="h-72 w-full rounded border bg-white" src={embedUrl} />
              </section>
            )}
          </CardContent>
        </Card>
      ) : null}

      {tab === "dashboard" ? (
        <Card data-testid="tab-content-dashboard" className="rounded-lg bg-white">
          <CardHeader><CardTitle>Dashboard</CardTitle></CardHeader>
          <CardContent><iframe title="Coroot Dashboard" data-testid="dashboard-iframe" className="h-[520px] w-full rounded-lg border bg-white" src={config.baseUrl || "/api/v1/coroot/"} /></CardContent>
        </Card>
      ) : null}

      {tab === "topology" ? (
        <Card data-testid="tab-content-topology" className="rounded-lg bg-white">
          <CardHeader><CardTitle>服务拓扑</CardTitle><CardDescription>Coroot topology iframe</CardDescription></CardHeader>
          <CardContent><iframe title="Coroot topology" className="h-[520px] w-full rounded-lg border bg-white" src={embedUrl} /></CardContent>
        </Card>
      ) : null}

      {drawerOpen ? (
        <aside className="monitor-ai-drawer fixed right-4 top-20 z-50 grid w-[360px] gap-4 rounded-xl border bg-white p-4 shadow-xl">
          <header className="flex items-center justify-between"><strong>AI 助手</strong><Button size="sm" variant="outline" onClick={() => setDrawerOpen(false)}>关闭</Button></header>
          <div className="quick-actions grid gap-2">
            {["解释当前面板", "定位异常原因", "生成排查步骤", "总结服务风险"].map((label) => (
              <button key={label} type="button" className="action-btn rounded-lg border bg-white p-2 text-left text-sm hover:bg-slate-50" onClick={() => void sendQuickAction(label)}>{label}</button>
            ))}
          </div>
          <pre className="rounded-lg bg-slate-950 p-3 text-xs text-white">{JSON.stringify({ source: "coroot", tab, count: services.length }, null, 2)}</pre>
        </aside>
      ) : null}
    </SettingsPageFrame>
  );
}
