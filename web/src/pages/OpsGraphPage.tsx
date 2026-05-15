import { useState } from "react";
import { Link } from "react-router-dom";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import "./erpSrePages.css";

type Entity = {
  id?: string;
  name?: string;
  type?: string;
  status?: string;
  relation?: string;
  impact?: string;
  health?: string;
  hostProfile?: Record<string, unknown>;
  host_profile?: Record<string, unknown>;
  hostLease?: Record<string, unknown>;
  host_lease?: Record<string, unknown>;
  members?: Entity[];
  relatedExperiencePacks?: Entity[];
  related_experience_packs?: Entity[];
};

type GraphContext = {
  entity: Entity;
  hostProfile: Record<string, unknown>;
  hostLease: Record<string, unknown>;
  middlewareMembers: Entity[];
  relatedExperiencePacks: Entity[];
};

const SEARCH_TYPES = [
  ["case", "Case"],
  ["service", "服务"],
  ["endpoint", "接口"],
  ["host", "主机"],
  ["middleware", "中间件"],
];

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
  const [context, setContext] = useState<GraphContext>({ entity: {}, hostProfile: {}, hostLease: {}, middlewareMembers: [], relatedExperiencePacks: [] });
  const [neighbors, setNeighbors] = useState<Entity[]>([]);
  const [impact, setImpact] = useState<{ capabilities?: Entity[]; tenants?: Entity[] }>({});

  async function searchEntity() {
    const text = query.trim();
    if (!text) return;
    const lookup = await requestJson<{ matches?: Entity[]; items?: Entity[] }>("/api/v1/opsgraph/lookup", { method: "POST", body: JSON.stringify({ query: text, entity_types: SEARCH_TYPES.map(([type]) => type) }) }).catch(() => ({ matches: [{ id: text, name: text, type: "service" }] }));
    const matches = lookup.matches || lookup.items || [];
    const entity = matches[0] || { id: text, name: text };
    const entityId = entity.id || entity.name || text;
    const [neighborhood, businessImpact] = await Promise.all([
      requestJson<{ entity?: Entity; neighbors?: Entity[] }>(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/neighborhood?depth=2`).catch(() => ({ entity, neighbors: [] })),
      requestJson<{ capabilities?: Entity[]; tenants?: Entity[] }>(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/business-impact`).catch(() => ({ capabilities: [], tenants: [] })),
    ]);
    const resolvedEntity = neighborhood.entity || entity;
    const resolvedNeighbors = neighborhood.neighbors || [];
    setLookupRows(matches);
    setSelectedEntity(resolvedEntity);
    setContext(buildGraphContext(resolvedEntity, resolvedNeighbors));
    setNeighbors(resolvedNeighbors);
    setImpact(businessImpact);
  }

  return (
    <SettingsPageFrame
      title="OpsGraph"
      description="用 Case、服务、接口、主机和中间件的最小关系辅助根因定位，不提供资产编辑入口。"
    >
          <Card className="erp-sre-panel rounded-lg bg-white">
            <CardHeader><CardTitle>图谱查询</CardTitle></CardHeader>
            <CardContent className="grid gap-3">
              <div className="flex flex-wrap gap-2">
                {SEARCH_TYPES.map(([type, label]) => <ToneBadge key={type}>{label}</ToneBadge>)}
              </div>
              <div className="opsgraph-search flex gap-2">
                <input className="min-h-9 flex-1 rounded-lg border px-3" value={query} onChange={(event) => setQuery(event.target.value)} data-testid="opsgraph-search-input" placeholder="搜索 Case、服务、接口、主机、中间件" />
                <button className="rounded-lg border border-emerald-700 bg-emerald-50 px-4 font-semibold text-emerald-900" type="button" data-testid="opsgraph-search-button" onClick={() => void searchEntity()}>搜索</button>
              </div>
              <div className="opsgraph-lookup mt-3 flex flex-wrap gap-2">{lookupRows.map((item) => <span key={item.id || item.name} className="rounded-full bg-slate-100 px-2 py-1 text-xs">{item.name || item.id}</span>)}</div>
            </CardContent>
          </Card>

          <section className="erp-sre-layout grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
            <Card className="erp-sre-panel rounded-lg bg-white">
              <CardHeader><CardTitle>邻域概览</CardTitle></CardHeader>
              <CardContent className="grid gap-3">
                {!selectedEntity.id ? (
                  <section data-testid="opsgraph-empty-guide" className="rounded-lg border border-dashed bg-white p-4 text-sm text-slate-600">
                    <div className="font-medium text-slate-950">如何生成图谱</div>
                    <p className="mt-1 leading-6">搜索 Case、服务、主机或中间件后会展示邻域。关系来自 Coroot MCP RCA 结果、Case 证据、主机上报画像、HostLease 和运维手册绑定。</p>
                  </section>
                ) : null}
                <article className="rounded-lg border bg-slate-50 p-4">
                  <div className="text-lg font-semibold">{selectedEntity.name || selectedEntity.id || "未选择实体"}</div>
                  <div className="mt-2 flex flex-wrap gap-2"><ToneBadge>{entityTypeLabel(selectedEntity.type)}</ToneBadge><ToneBadge>{statusLabel(selectedEntity.status)}</ToneBadge>{selectedEntity.health ? <ToneBadge>{selectedEntity.health}</ToneBadge> : null}</div>
                  {selectedEntity.id ? (
                    <div className="mt-3 flex flex-wrap gap-2 text-sm">
                      <Link className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/incidents?entity_id=${encodeURIComponent(selectedEntity.id)}`}>查看关联 Case</Link>
                      <Link className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/settings/ops-manuals?entity_id=${encodeURIComponent(selectedEntity.id)}`}>查看运维手册</Link>
                    </div>
                  ) : null}
                </article>
                <div className="grid gap-2 md:grid-cols-2">
                  {neighbors.map((neighbor) => (
                    <div key={neighbor.id || neighbor.name} className="rounded-lg border p-3">
                      <div className="font-medium">{neighbor.name || neighbor.id}</div>
                      <div className="mt-1 flex flex-wrap gap-2"><ToneBadge>{entityTypeLabel(neighbor.type)}</ToneBadge><ToneBadge>{neighbor.relation || "关联"}</ToneBadge></div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
            <aside className="grid min-w-0 gap-4">
              <OpsGraphContextPanel selectedEntity={selectedEntity} context={context} />
              <Card className="erp-sre-panel rounded-lg bg-white"><CardHeader><CardTitle>邻域列表</CardTitle></CardHeader><CardContent className="grid gap-2">{neighbors.map((neighbor) => <div key={neighbor.id || neighbor.name} className="flex flex-wrap items-center justify-between gap-2"><span>{neighbor.name || neighbor.id}</span>{neighbor.type === "host" && (neighbor.id || neighbor.name) ? <Link className="text-xs font-medium text-slate-900 underline-offset-4 hover:underline" to={`/settings/hosts?host_id=${encodeURIComponent(String(neighbor.id || neighbor.name))}`}>主机与租约</Link> : null}</div>)}</CardContent></Card>
              <Card className="erp-sre-panel rounded-lg bg-white"><CardHeader><CardTitle>业务影响</CardTitle></CardHeader><CardContent className="grid gap-3"><div>{(impact.capabilities || []).map((item) => <p key={item.name || item.id}>{item.name || item.id}: {item.impact}</p>)}</div><div>{(impact.tenants || []).map((item) => <p key={item.name || item.id}>{item.name || item.id}: {item.impact}</p>)}</div></CardContent></Card>
            </aside>
          </section>
    </SettingsPageFrame>
  );
}

function OpsGraphContextPanel({ selectedEntity, context }: { selectedEntity: Entity; context: GraphContext }) {
  const hostProfile = context.hostProfile;
  const hostLease = context.hostLease;
  const members = context.middlewareMembers;
  const packs = context.relatedExperiencePacks;
  const selectedId = String(selectedEntity.id || selectedEntity.name || "");

  return (
    <Card className="erp-sre-panel rounded-lg bg-white">
      <CardHeader><CardTitle>定位上下文</CardTitle></CardHeader>
      <CardContent className="grid gap-4">
        {Object.keys(hostProfile).length ? (
          <section className="rounded-lg border bg-slate-50 p-3">
            <div className="font-medium text-slate-950">HostProfile 摘要</div>
            <dl className="mt-2 grid gap-1 text-sm text-slate-600">
              <div><dt className="inline font-medium">主机：</dt><dd className="inline">{readText(hostProfile, "display_name", "displayName", "host_id", "hostId")}</dd></div>
              <div><dt className="inline font-medium">系统：</dt><dd className="inline">{readText(hostProfile, "os")} / {readText(hostProfile, "arch")}</dd></div>
              <div><dt className="inline font-medium">Agent：</dt><dd className="inline">{readText(hostProfile, "agent_version", "agentVersion") || "未知"}</dd></div>
              <div><dt className="inline font-medium">标签：</dt><dd className="inline">{formatLabels(hostProfile.labels)}</dd></div>
            </dl>
            {readText(hostProfile, "host_id", "hostId") ? <Link className="mt-2 inline-flex text-sm font-medium text-slate-900 underline-offset-4 hover:underline" to={`/settings/hosts?host_id=${encodeURIComponent(readText(hostProfile, "host_id", "hostId"))}`}>主机与租约</Link> : null}
          </section>
        ) : null}

        {Object.keys(hostLease).length ? (
          <section className="rounded-lg border bg-slate-50 p-3">
            <div className="font-medium text-slate-950">当前 HostLease</div>
            <dl className="mt-2 grid gap-1 text-sm text-slate-600">
              <div><dt className="inline font-medium">租约：</dt><dd className="inline">{readText(hostLease, "lease_id", "leaseId", "id")}</dd></div>
              <div><dt className="inline font-medium">状态：</dt><dd className="inline">{statusLabel(readText(hostLease, "status", "state"))}</dd></div>
              <div><dt className="inline font-medium">Case：</dt><dd className="inline">{readText(hostLease, "mission_id", "caseId", "case_id") || "-"}</dd></div>
              <div><dt className="inline font-medium">过期：</dt><dd className="inline">{readText(hostLease, "expires_at", "expiresAt") || "-"}</dd></div>
            </dl>
          </section>
        ) : null}

        {members.length ? (
          <section className="rounded-lg border bg-slate-50 p-3">
            <div className="font-medium text-slate-950">中间件成员</div>
            <div className="mt-2 grid gap-2">
              {members.map((member) => (
                <div key={member.id || member.name} className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span>{member.name || member.id}</span>
                  <span className="text-slate-500">{member.role || "member"} · {statusLabel(member.status)}</span>
                </div>
              ))}
            </div>
          </section>
        ) : null}

        {packs.length ? (
          <section className="rounded-lg border bg-slate-50 p-3">
            <div className="font-medium text-slate-950">关联运维手册</div>
            <div className="mt-2 grid gap-2 text-sm">
              {packs.map((pack) => <Link key={pack.id || pack.name} className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/settings/ops-manuals?entity_id=${encodeURIComponent(selectedId)}`}>{pack.name || pack.id}</Link>)}
            </div>
          </section>
        ) : null}

        {!Object.keys(hostProfile).length && !Object.keys(hostLease).length && !members.length && !packs.length ? <p className="text-sm text-slate-500">选择主机或中间件节点后展示 HostProfile、HostLease、成员和运维手册上下文。</p> : null}
      </CardContent>
    </Card>
  );
}

function buildGraphContext(entity: Entity, neighbors: Entity[]): GraphContext {
  const selectedProfile = recordValue(entity.hostProfile || entity.host_profile);
  const selectedLease = recordValue(entity.hostLease || entity.host_lease);
  const firstHostNeighbor = neighbors.find((neighbor) => neighbor.type === "host" && (neighbor.hostProfile || neighbor.host_profile || neighbor.hostLease || neighbor.host_lease)) || {};
  return {
    entity,
    hostProfile: Object.keys(selectedProfile).length ? selectedProfile : recordValue(firstHostNeighbor.hostProfile || firstHostNeighbor.host_profile),
    hostLease: Object.keys(selectedLease).length ? selectedLease : recordValue(firstHostNeighbor.hostLease || firstHostNeighbor.host_lease),
    middlewareMembers: arrayValue(entity.members),
    relatedExperiencePacks: arrayValue(entity.relatedExperiencePacks || entity.related_experience_packs),
  };
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function arrayValue(value: unknown): Entity[] {
  return Array.isArray(value) ? value as Entity[] : [];
}

function readText(source: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = source[key];
    if (value !== undefined && value !== null && value !== "") return String(value);
  }
  return "";
}

function formatLabels(value: unknown) {
  const labels = recordValue(value);
  const text = Object.entries(labels).map(([key, item]) => `${key}=${item}`).join("，");
  return text || "-";
}

function entityTypeLabel(value = "") {
  const labels: Record<string, string> = {
    case: "Case",
    service: "服务",
    endpoint: "接口",
    host: "主机",
    middleware: "中间件",
    database: "数据库",
  };
  return labels[value] || value || "实体";
}

function statusLabel(value = "") {
  const labels: Record<string, string> = {
    healthy: "健康",
    online: "在线",
    warning: "告警",
    degraded: "降级",
    conflict: "冲突",
    active: "活跃",
    unknown: "未知",
  };
  return labels[value] || value || "未知";
}
