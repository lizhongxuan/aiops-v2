import { Plus, Save, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { listHostLeases, listHostProfiles, listHostReportHistory } from "@/api/hostProfiles";
import {
  buildHostProfileDetail,
  buildHostExecutionRisks,
  buildHostProfileRows,
  normalizeHostLease,
  type HostExecutionRiskView,
  type HostLeaseRowView,
  type HostProfileRowView,
} from "@/components/hosts/hostProfileViewModels";
import { ConfirmButton, EmptyState, Field, LoadingState, SelectField, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";
import {
  buildHostsViewModel,
  createHost,
  deleteHost,
  fetchHosts,
  fetchSessions,
  fetchTerminalSessions,
  type HostRecord,
  updateHost,
} from "@/pages/settingsApi";

type HostDraft = {
  id: string;
  name: string;
  address: string;
  sshUser: string;
  sshPort: string;
  transport: string;
  labelsText: string;
};

const blankDraft: HostDraft = { id: "", name: "", address: "", sshUser: "root", sshPort: "22", transport: "ssh_bootstrap", labelsText: "" };
type HostTab = "profiles" | "leases" | "reports" | "access";
type HostReportRow = { reportId: string; hostId: string; status: string; reportedAt: string; summary: string };

export function HostsPage() {
  const navigate = useNavigate();
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [sessions, setSessions] = useState<unknown[]>([]);
  const [terminalSessions, setTerminalSessions] = useState<unknown[]>([]);
  const [hostProfiles, setHostProfiles] = useState<HostProfileRowView[]>([]);
  const [hostLeases, setHostLeases] = useState<HostLeaseRowView[]>([]);
  const [hostReports, setHostReports] = useState<HostReportRow[]>([]);
  const [hostRisks, setHostRisks] = useState<HostExecutionRiskView[]>([]);
  const [tab, setTab] = useState<HostTab>("profiles");
  const [query, setQuery] = useState("");
  const [heartbeat, setHeartbeat] = useState("all");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingHost, setEditingHost] = useState<HostRecord | null>(null);
  const [draft, setDraft] = useState<HostDraft>(blankDraft);

  const model = useMemo(
    () =>
      buildHostsViewModel({
        hosts,
        sessions,
        terminalSessions,
        query,
        filters: { heartbeat, source: "all", ssh: "all" },
        pageSize: 100,
      }),
    [heartbeat, hosts, query, sessions, terminalSessions],
  );

  async function load() {
    setLoading(true);
    try {
      const [hostPayload, sessionPayload, terminalPayload, profilePayload, leasePayload] = await Promise.all([
        fetchHosts(),
        fetchSessions(),
        fetchTerminalSessions(),
        listHostProfiles({ limit: 100 }).catch(() => ({ items: [] })),
        listHostLeases({ limit: 100 }).catch(() => ({ items: [] })),
      ]);
      const rawProfiles = itemsFrom(profilePayload);
      const rawLeases = itemsFrom(leasePayload);
      const firstHostId = hostIdOf(rawProfiles[0]) || hostPayload.items?.[0]?.id || "";
      const reportPayload = firstHostId ? await listHostReportHistory(firstHostId).catch(() => ({ items: [] })) : { items: [] };
      setHosts(hostPayload.items || []);
      setSessions(sessionPayload.items || sessionPayload.sessions || []);
      setTerminalSessions(terminalPayload.items || terminalPayload.sessions || []);
      setHostLeases(rawLeases.map(normalizeHostLease));
      setHostProfiles(buildHostProfileRows({ profiles: rawProfiles, leases: rawLeases }));
      setHostRisks(buildHostExecutionRisks({ profiles: rawProfiles, leases: rawLeases }));
      setHostReports(itemsFrom(reportPayload).map(normalizeHostReport));
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载主机列表失败" });
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    setEditingHost(null);
    setDraft(blankDraft);
    setDialogOpen(true);
  }

  function openEdit(host: HostRecord) {
    setEditingHost(host);
    setDraft({
      id: host.id || "",
      name: host.name || "",
      address: host.address || host.name || host.id || "",
      sshUser: host.sshUser || "root",
      sshPort: String(host.sshPort || "22"),
      transport: host.transport || "ssh_bootstrap",
      labelsText: formatLabels(host.labels),
    });
    setDialogOpen(true);
  }

  async function saveHost() {
    setSaving(true);
    try {
      const payload = {
        id: draft.id,
        name: draft.name || draft.address || draft.id,
        address: draft.address,
        sshUser: draft.sshUser,
        sshPort: Number(draft.sshPort) || 22,
        transport: draft.transport,
        labels: parseLabels(draft.labelsText),
      };
      if (editingHost?.id) {
        await updateHost(editingHost.id, payload);
      } else {
        await createHost(payload);
      }
      setDialogOpen(false);
      setMessage({ type: "success", text: "主机信息已保存" });
      await load();
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存主机失败" });
    } finally {
      setSaving(false);
    }
  }

  async function removeHost(hostId: string) {
    try {
      await deleteHost(hostId);
      setMessage({ type: "success", text: "主机已删除" });
      await load();
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "删除主机失败" });
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="主机与租约"
      description="展示主机客户端上报画像、HostLease 锁状态、上报历史和接入配置。"
      actions={
        <Button onClick={openCreate}>
          <Plus />
          接入主机
        </Button>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {loading ? (
        <LoadingState label="加载主机列表" />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <StatCard label="主机画像" value={hostProfiles.length} />
            <StatCard label="主机租约" value={hostLeases.length} />
            <StatCard label="执行风险" value={hostRisks.length} />
            <StatCard label="上报历史" value={hostReports.length} />
          </div>

          <div className="flex flex-wrap gap-2">
            {[
              ["profiles", "主机画像"],
              ["leases", "主机租约"],
              ["reports", "上报历史"],
              ["access", "接入配置"],
            ].map(([key, label]) => (
              <button
                key={key}
                type="button"
                className={`rounded-lg border px-3 py-2 text-sm ${tab === key ? "bg-slate-900 text-white" : "bg-white text-slate-700"}`}
                onClick={() => setTab(key as HostTab)}
              >
                {label}
              </button>
            ))}
          </div>

          {tab === "profiles" ? (
            <HostProfilesPanel rows={hostProfiles} leases={hostLeases} reports={hostReports} risks={hostRisks} />
          ) : null}

          {tab === "leases" ? (
            <HostLeasesPanel rows={hostLeases} />
          ) : null}

          {tab === "reports" ? (
            <HostReportsPanel rows={hostReports} />
          ) : null}

          {tab === "access" ? (
          <Card className="rounded-lg bg-white">
            <CardContent className="flex flex-col gap-3 pt-0">
              <div className="flex flex-col gap-2 md:flex-row md:items-center">
                <label className="relative min-w-0 flex-1">
                  <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
                  <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 IP、用户名、来源" />
                </label>
                <div className="w-full md:w-44">
                  <SelectField
                    aria-label="heartbeat filter"
                    value={heartbeat}
                    onChange={setHeartbeat}
                    options={[
                      { label: "全部心跳", value: "all" },
                      { label: "在线", value: "online" },
                      { label: "待安装", value: "installing" },
                      { label: "离线", value: "offline" },
                      { label: "超时", value: "stale" },
                    ]}
                  />
                </div>
              </div>

              {model.pageRows.length ? (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[900px] text-left text-sm">
                    <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                      <tr>
                        <th className="py-2 pr-3">主机 IP / 用户名</th>
                        <th className="py-2 pr-3">心跳</th>
                        <th className="py-2 pr-3">标签</th>
                        <th className="py-2 pr-3">会话</th>
                        <th className="py-2 pr-3">来源 / SSH</th>
                        <th className="py-2 text-right">操作</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {model.pageRows.map((row: any) => (
                        <tr key={row.id}>
                          <td className="py-3 pr-3">
                            <div className="font-medium text-slate-900">{row.title}</div>
                            <div className="text-xs text-slate-500">{row.subtitle}</div>
                          </td>
                          <td className="py-3 pr-3">
                            <ToneBadge tone={row.heartbeat === "online" ? "success" : row.heartbeat === "offline" ? "danger" : "warning"}>{row.heartbeatLabel}</ToneBadge>
                          </td>
                          <td className="py-3 pr-3">
                            {row.labels?.length ? (
                              <div className="flex max-w-xs flex-wrap gap-1">
                                {row.labels.map((label: { key: string; value: string; label: string }) => (
                                  <ToneBadge key={label.label}>{label.label}</ToneBadge>
                                ))}
                              </div>
                            ) : (
                              <span className="text-xs text-slate-400">未打标签</span>
                            )}
                          </td>
                          <td className="py-3 pr-3">{row.sessionCount}</td>
                          <td className="py-3 pr-3">
                            <div>{row.sourceLabel}</div>
                            <div className="text-xs text-slate-500">{row.sshLabel}</div>
                          </td>
                          <td className="py-3">
                            <div className="flex justify-end gap-2">
                              <Button variant="outline" onClick={() => navigate(`/terminal/${row.id}`)} disabled={!row.canOpenSsh}>
                                终端
                              </Button>
                              <Button variant="outline" onClick={() => openEdit(row.raw)}>
                                编辑
                              </Button>
                              <ConfirmButton variant="destructive" confirm={`确认删除主机 ${row.id}？`} onConfirm={() => void removeHost(row.id)}>
                                <Trash2 />
                              </ConfirmButton>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <EmptyState title="暂无主机" description="没有符合条件的主机，调整筛选或接入新主机。" />
              )}
            </CardContent>
          </Card>
          ) : null}
        </>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editingHost ? "编辑主机" : "接入主机"}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="Host ID">
              <Input value={draft.id} onChange={(event) => setDraft((prev) => ({ ...prev, id: event.target.value }))} disabled={Boolean(editingHost)} />
            </Field>
            <Field label="名称">
              <Input value={draft.name} onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))} />
            </Field>
            <Field label="地址">
              <Input value={draft.address} onChange={(event) => setDraft((prev) => ({ ...prev, address: event.target.value }))} />
            </Field>
            <Field label="SSH 用户">
              <Input value={draft.sshUser} onChange={(event) => setDraft((prev) => ({ ...prev, sshUser: event.target.value }))} />
            </Field>
            <Field label="SSH 端口">
              <Input value={draft.sshPort} onChange={(event) => setDraft((prev) => ({ ...prev, sshPort: event.target.value }))} />
            </Field>
            <Field label="Transport">
              <SelectField
                value={draft.transport}
                onChange={(transport) => setDraft((prev) => ({ ...prev, transport }))}
                options={[
                  { label: "ssh_bootstrap", value: "ssh_bootstrap" },
                  { label: "grpc_reverse", value: "grpc_reverse" },
                  { label: "local", value: "local" },
                ]}
              />
            </Field>
            <div className="md:col-span-2">
              <Field label="标签" hint="用逗号或换行分隔，例如 env=prod, role=web, cluster=ops-k8s">
                <Input value={draft.labelsText} onChange={(event) => setDraft((prev) => ({ ...prev, labelsText: event.target.value }))} placeholder="env=prod, role=web" />
              </Field>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={() => void saveHost()} disabled={saving || !draft.id}>
              <Save />
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsPageFrame>
  );
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <Card size="sm" className="rounded-lg bg-white">
      <CardContent className="py-3">
        <div className="whitespace-nowrap text-xs font-medium uppercase tracking-normal text-slate-500">{label}</div>
        <div className="mt-1 text-lg font-semibold text-slate-950">{value}</div>
      </CardContent>
    </Card>
  );
}

function HostProfilesPanel({
  rows,
  leases,
  reports,
  risks,
}: {
  rows: HostProfileRowView[];
  leases: HostLeaseRowView[];
  reports: HostReportRow[];
  risks: HostExecutionRiskView[];
}) {
  const [selectedHostId, setSelectedHostId] = useState("");
  const selectedProfile = rows.find((row) => row.hostId === selectedHostId) || rows[0];
  const detail = selectedProfile ? buildHostProfileDetail({ profile: selectedProfile, leases, reports }) : null;

  return (
    <Card className="rounded-lg bg-white">
      <CardContent className="grid gap-4 pt-0">
        {risks.length ? (
          <div className="grid gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3">
            <div className="text-sm font-medium text-amber-900">执行风险提示</div>
            <div className="grid gap-2">
              {risks.map((risk, index) => (
                <div key={`${risk.key}-${risk.hostId}-${index}`} className="text-sm text-amber-900">
                  <strong>{risk.label}</strong>：{risk.message}
                </div>
              ))}
            </div>
          </div>
        ) : null}
        {rows.length ? (
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[900px] text-left text-sm">
                <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                  <tr>
                    <th className="py-2 pr-3">主机</th>
                    <th className="py-2 pr-3">状态</th>
                    <th className="py-2 pr-3">OS / 架构</th>
                    <th className="py-2 pr-3">标签</th>
                    <th className="py-2 pr-3">最近上报</th>
                    <th className="py-2 pr-3">允许执行</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {rows.map((row) => (
                    <tr
                      key={`${row.hostId}-${row.displayName}`}
                      className={selectedProfile?.hostId === row.hostId ? "bg-slate-50" : ""}
                      onClick={() => setSelectedHostId(row.hostId)}
                    >
                      <td className="py-3 pr-3">
                        <button type="button" className="text-left" onClick={() => setSelectedHostId(row.hostId)}>
                          <div className="font-medium text-slate-900">{row.displayName}</div>
                          <div className="text-xs text-slate-500">{row.hostId}</div>
                        </button>
                      </td>
                      <td className="py-3 pr-3"><ToneBadge tone={badgeTone(row.statusTone)}>{row.statusLabel}</ToneBadge></td>
                      <td className="py-3 pr-3">{row.osLabel} / {row.archLabel}</td>
                      <td className="py-3 pr-3">{row.labelsText || "未标注"}</td>
                      <td className="py-3 pr-3">{row.lastHeartbeatAt || "-"}</td>
                      <td className="py-3 pr-3">{row.riskCount ? `否，${row.riskCount} 个风险` : "是"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {detail ? (
              <aside className="grid content-start gap-3 rounded-lg border bg-slate-50 p-3 text-sm">
                <div>
                  <div className="font-medium text-slate-950">{detail.displayName}</div>
                  <div className="text-xs text-slate-500">{detail.hostId}</div>
                </div>
                {detail.sections.map((section) => (
                  <section key={section.title} className="grid gap-1 border-t pt-3">
                    <h3 className="text-xs font-semibold uppercase tracking-normal text-slate-500">{section.title}</h3>
                    {section.items.map((item) => (
                      <div key={`${section.title}-${item.label}`} className="grid grid-cols-[96px_minmax(0,1fr)] gap-2">
                        <span className="text-slate-500">{item.label}</span>
                        <span className="break-words text-slate-900">{item.value}</span>
                      </div>
                    ))}
                  </section>
                ))}
              </aside>
            ) : null}
          </div>
        ) : <EmptyState title="暂无主机画像" description="主机客户端上报环境信息后会显示在这里。" />}
      </CardContent>
    </Card>
  );
}

function HostLeasesPanel({ rows }: { rows: HostLeaseRowView[] }) {
  return (
    <Card className="rounded-lg bg-white">
      <CardContent className="pt-0">
        {rows.length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[820px] text-left text-sm">
              <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                <tr>
                  <th className="py-2 pr-3">Lease</th>
                  <th className="py-2 pr-3">主机</th>
                  <th className="py-2 pr-3">状态</th>
                  <th className="py-2 pr-3">持有 Case</th>
                  <th className="py-2 pr-3">会话</th>
                  <th className="py-2 pr-3">过期时间</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {rows.map((row) => (
                  <tr key={row.leaseId}>
                    <td className="py-3 pr-3 font-medium text-slate-900">{row.leaseId}</td>
                    <td className="py-3 pr-3">{row.hostId}</td>
                    <td className="py-3 pr-3"><ToneBadge tone={badgeTone(row.statusTone)}>{row.statusLabel}</ToneBadge></td>
                    <td className="py-3 pr-3">{row.missionLabel}</td>
                    <td className="py-3 pr-3">{row.ownerSessionLabel}</td>
                    <td className="py-3 pr-3">{row.expiresAt || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <EmptyState title="暂无主机租约" description="聊天或 Workflow 锁定主机后会显示租约。" />}
      </CardContent>
    </Card>
  );
}

function HostReportsPanel({ rows }: { rows: HostReportRow[] }) {
  return (
    <Card className="rounded-lg bg-white">
      <CardContent className="grid gap-2 pt-0">
        {rows.length ? rows.map((row) => (
          <div key={row.reportId} className="rounded-lg border bg-slate-50 p-3 text-sm">
            <div className="font-medium text-slate-900">{row.reportId}</div>
            <div className="mt-1 text-xs text-slate-500">{row.hostId} · {row.status} · {row.reportedAt}</div>
            <div className="mt-1 text-slate-700">{row.summary}</div>
          </div>
        )) : <EmptyState title="暂无上报历史" description="主机客户端上报后会保留最近历史。" />}
      </CardContent>
    </Card>
  );
}

function formatLabels(labels?: Record<string, string>) {
  return Object.entries(labels || {})
    .map(([key, value]) => [key.trim(), String(value || "").trim()])
    .filter(([key, value]) => key && value)
    .sort(([leftKey, leftValue], [rightKey, rightValue]) => `${leftKey}=${leftValue}`.localeCompare(`${rightKey}=${rightValue}`))
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
}

function badgeTone(tone: "success" | "warning" | "danger" | "neutral") {
  return tone === "neutral" ? "default" : tone;
}

function parseLabels(input: string) {
  const labels: Record<string, string> = {};
  for (const entry of input.split(/[\n,]+/)) {
    const [rawKey, ...rawValue] = entry.split("=");
    const key = rawKey.trim();
    const value = rawValue.join("=").trim();
    if (key && value) labels[key] = value;
  }
  return labels;
}

function itemsFrom(payload: unknown): unknown[] {
  if (Array.isArray(payload)) return payload;
  if (payload && typeof payload === "object" && Array.isArray((payload as { items?: unknown[] }).items)) {
    return (payload as { items: unknown[] }).items;
  }
  return [];
}

function readText(source: unknown, ...keys: string[]) {
  if (!source || typeof source !== "object" || Array.isArray(source)) return "";
  const record = source as Record<string, unknown>;
  for (const key of keys) {
    const value = record[key];
    if (value !== undefined && value !== null && value !== "") return String(value).trim();
  }
  return "";
}

function hostIdOf(source: unknown) {
  return readText(source, "hostId", "host_id", "id");
}

function normalizeHostReport(source: unknown): HostReportRow {
  return {
    reportId: readText(source, "reportId", "report_id", "id") || "unknown-report",
    hostId: readText(source, "hostId", "host_id"),
    status: readText(source, "status", "state") || "unknown",
    reportedAt: readText(source, "reportedAt", "reported_at", "createdAt", "created_at"),
    summary: readText(source, "summary", "description", "detail"),
  };
}
