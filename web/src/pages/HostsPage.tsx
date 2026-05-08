import { Plus, RefreshCw, Save, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
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

export function HostsPage() {
  const navigate = useNavigate();
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [sessions, setSessions] = useState<unknown[]>([]);
  const [terminalSessions, setTerminalSessions] = useState<unknown[]>([]);
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
      const [hostPayload, sessionPayload, terminalPayload] = await Promise.all([fetchHosts(), fetchSessions(), fetchTerminalSessions()]);
      setHosts(hostPayload.items || []);
      setSessions(sessionPayload.items || sessionPayload.sessions || []);
      setTerminalSessions(terminalPayload.items || terminalPayload.sessions || []);
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
      title="Hosts"
      description="主机清单、会话计数和接入状态。React 页面复用现有 hostListViewModel 与 hosts API。"
      actions={
        <>
          <Button variant="outline" onClick={() => void load()}>
            <RefreshCw />
            刷新
          </Button>
          <Button onClick={openCreate}>
            <Plus />
            接入主机
          </Button>
        </>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {loading ? (
        <LoadingState label="加载主机列表" />
      ) : (
        <>
          <div className="grid grid-cols-3 gap-3">
            {model.stats.map((item: { label: string; value: number }) => (
              <Card key={item.label} size="sm" className="rounded-lg bg-white">
                <CardContent className="py-3">
                  <div className="whitespace-nowrap text-xs font-medium uppercase tracking-normal text-slate-500">{item.label}</div>
                  <div className="mt-1 text-lg font-semibold text-slate-950">{item.value}</div>
                </CardContent>
              </Card>
            ))}
          </div>
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

function formatLabels(labels?: Record<string, string>) {
  return Object.entries(labels || {})
    .map(([key, value]) => [key.trim(), String(value || "").trim()])
    .filter(([key, value]) => key && value)
    .sort(([leftKey, leftValue], [rightKey, rightValue]) => `${leftKey}=${leftValue}`.localeCompare(`${rightKey}=${rightValue}`))
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
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
