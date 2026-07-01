import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import "./erpSrePages.css";

type Capability = { id?: string; name?: string; status?: string; summary?: string };
type Metric = { id?: string; name?: string; value?: string; trend?: number[] };
type Tenant = { id?: string; name?: string; impact?: string; severity?: string };

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function ERPHealthPage() {
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);

  useEffect(() => {
    async function load() {
      const [health, metricPayload, tenantPayload] = await Promise.all([
        requestJson<{ capabilities?: Capability[] }>("/api/v1/erp/health?environment=prod"),
        requestJson<{ metrics?: Metric[]; items?: Metric[] }>("/api/v1/erp/business-metrics"),
        requestJson<{ tenants?: Tenant[]; items?: Tenant[] }>("/api/v1/erp/tenant-impact"),
      ]);
      setCapabilities(health.capabilities || []);
      setMetrics(metricPayload.metrics || metricPayload.items || []);
      setTenants(tenantPayload.tenants || tenantPayload.items || []);
    }
    void load().catch(() => undefined);
  }, []);

  async function createIncidentFromHealth() {
    const degraded = capabilities.find((item) => item.status && item.status !== "healthy");
    await requestJson("/api/v1/incidents", {
      method: "POST",
      body: JSON.stringify({
        source: "erp-health",
        title: "ERP 健康异常",
        severity: "SEV2",
        environment: "prod",
        businessCapability: degraded?.name || "ERP",
      }),
    }).catch(() => undefined);
  }

  return (
    <section className="erp-sre-page">
      <div className="erp-sre-shell">
        <SettingsPageFrame
          title="ERP 健康"
          description="按业务能力、关键指标和租户影响查看 ERP 生产状态。"
          actions={
            <>
              <Button data-testid="erp-create-incident" onClick={() => void createIncidentFromHealth()}>创建事故</Button>
              <Button asChild variant="outline"><Link to="/incidents">事故工作台</Link></Button>
              <Button asChild variant="outline"><Link to="/opsgraph">ERP 图谱</Link></Button>
            </>
          }
        >
          <Card className="erp-sre-panel rounded-lg bg-white">
            <CardHeader><CardTitle>业务能力健康矩阵</CardTitle></CardHeader>
            <CardContent className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
              {capabilities.map((item) => (
                <article key={item.id || item.name} className="rounded-lg border bg-slate-50 p-3">
                  <div className="font-medium text-slate-900">{item.name || item.id}</div>
                  <div className="mt-2"><ToneBadge tone={item.status === "healthy" ? "success" : "warning"}>{item.status || "unknown"}</ToneBadge></div>
                  <p className="mt-2 text-sm text-slate-600">{item.summary || "暂无摘要"}</p>
                </article>
              ))}
            </CardContent>
          </Card>

          <section className="erp-sre-grid two grid gap-4 lg:grid-cols-2">
            <Card className="erp-sre-panel rounded-lg bg-white">
              <CardHeader><CardTitle>关键业务指标</CardTitle></CardHeader>
              <CardContent className="grid gap-2">
                {metrics.map((metric) => (
                  <div key={metric.id || metric.name} className="rounded-lg border p-3">
                    <div className="font-medium">{metric.name}</div>
                    <div className="text-lg font-semibold">{metric.value}</div>
                    <div className="mt-2 flex h-8 items-end gap-1">{(metric.trend || []).map((value, index) => <span key={index} className="w-4 rounded bg-slate-300" style={{ height: `${Math.max(10, Math.min(100, value))}%` }} />)}</div>
                  </div>
                ))}
              </CardContent>
            </Card>
            <Card className="erp-sre-panel rounded-lg bg-white">
              <CardHeader><CardTitle>受影响租户</CardTitle></CardHeader>
              <CardContent className="grid gap-2">
                {tenants.map((tenant) => (
                  <div key={tenant.id || tenant.name} className="rounded-lg border p-3">
                    <div className="font-medium">{tenant.name || tenant.id}</div>
                    <p className="text-sm text-slate-600">{tenant.impact}</p>
                  </div>
                ))}
              </CardContent>
            </Card>
          </section>
        </SettingsPageFrame>
      </div>
    </section>
  );
}
