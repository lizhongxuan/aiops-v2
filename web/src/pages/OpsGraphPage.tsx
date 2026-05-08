import { useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import "./erpSrePages.css";

type Entity = { id?: string; name?: string; type?: string; status?: string; relation?: string; impact?: string };

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function OpsGraphPage() {
  const [query, setQuery] = useState("");
  const [lookupRows, setLookupRows] = useState<Entity[]>([]);
  const [selectedEntity, setSelectedEntity] = useState<Entity>({});
  const [neighbors, setNeighbors] = useState<Entity[]>([]);
  const [impact, setImpact] = useState<{ capabilities?: Entity[]; tenants?: Entity[] }>({});

  async function searchEntity() {
    const text = query.trim();
    if (!text) return;
    const lookup = await requestJson<{ matches?: Entity[]; items?: Entity[] }>("/api/v1/opsgraph/lookup", { method: "POST", body: JSON.stringify({ query: text }) }).catch(() => ({ matches: [{ id: text, name: text, type: "service" }] }));
    const matches = lookup.matches || lookup.items || [];
    const entity = matches[0] || { id: text, name: text };
    const entityId = entity.id || entity.name || text;
    const [neighborhood, businessImpact] = await Promise.all([
      requestJson<{ entity?: Entity; neighbors?: Entity[] }>(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/neighborhood?depth=2`).catch(() => ({ entity, neighbors: [] })),
      requestJson<{ capabilities?: Entity[]; tenants?: Entity[] }>(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/business-impact`).catch(() => ({ capabilities: [], tenants: [] })),
    ]);
    setLookupRows(matches);
    setSelectedEntity(neighborhood.entity || entity);
    setNeighbors(neighborhood.neighbors || []);
    setImpact(businessImpact);
  }

  return (
    <section className="erp-sre-page">
      <div className="erp-sre-shell">
        <SettingsPageFrame
          title="ERP 图谱"
          description="以实体搜索、邻域和业务影响为主，不做复杂全屏图谱。"
          actions={<><Button asChild variant="outline"><Link to="/erp">ERP 健康</Link></Button><Button asChild><Link to="/incidents">事故工作台</Link></Button></>}
        >
          <Card className="erp-sre-panel rounded-lg bg-white">
            <CardHeader><CardTitle>实体搜索</CardTitle></CardHeader>
            <CardContent>
              <div className="opsgraph-search flex gap-2">
                <input className="min-h-9 flex-1 rounded-lg border px-3" value={query} onChange={(event) => setQuery(event.target.value)} data-testid="opsgraph-search-input" placeholder="service / tenant / db / job" />
                <button className="rounded-lg border border-emerald-700 bg-emerald-50 px-4 font-semibold text-emerald-900" type="button" data-testid="opsgraph-search-button" onClick={() => void searchEntity()}>搜索</button>
              </div>
              <div className="opsgraph-lookup mt-3 flex flex-wrap gap-2">{lookupRows.map((item) => <span key={item.id || item.name} className="rounded-full bg-slate-100 px-2 py-1 text-xs">{item.name || item.id}</span>)}</div>
            </CardContent>
          </Card>

          <section className="erp-sre-layout grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
            <Card className="erp-sre-panel rounded-lg bg-white">
              <CardHeader><CardTitle>邻域概览</CardTitle></CardHeader>
              <CardContent className="grid gap-3">
                <article className="rounded-lg border bg-slate-50 p-4">
                  <div className="text-lg font-semibold">{selectedEntity.name || selectedEntity.id || "未选择实体"}</div>
                  <div className="mt-2 flex gap-2"><ToneBadge>{selectedEntity.type || "entity"}</ToneBadge><ToneBadge>{selectedEntity.status || "unknown"}</ToneBadge></div>
                </article>
                <div className="grid gap-2 md:grid-cols-2">
                  {neighbors.map((neighbor) => <div key={neighbor.id || neighbor.name} className="rounded-lg border p-3">{neighbor.name || neighbor.id}<div className="text-xs text-slate-500">{neighbor.relation}</div></div>)}
                </div>
              </CardContent>
            </Card>
            <aside className="erp-sre-grid grid gap-4">
              <Card className="erp-sre-panel rounded-lg bg-white"><CardHeader><CardTitle>邻域列表</CardTitle></CardHeader><CardContent className="grid gap-2">{neighbors.map((neighbor) => <div key={neighbor.id || neighbor.name}>{neighbor.name || neighbor.id}</div>)}</CardContent></Card>
              <Card className="erp-sre-panel rounded-lg bg-white"><CardHeader><CardTitle>业务影响</CardTitle></CardHeader><CardContent className="grid gap-3"><div>{(impact.capabilities || []).map((item) => <p key={item.name || item.id}>{item.name || item.id}: {item.impact}</p>)}</div><div>{(impact.tenants || []).map((item) => <p key={item.name || item.id}>{item.name || item.id}: {item.impact}</p>)}</div></CardContent></Card>
            </aside>
          </section>
        </SettingsPageFrame>
      </div>
    </section>
  );
}
