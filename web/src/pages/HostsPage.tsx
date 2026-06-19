import { Download, ExternalLink, Plus, Save, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { buildHostTerminalEntry } from "@/components/hosts/hostProfileViewModels";
import { ConfirmButton, EmptyState, Field, LoadingState, SelectField, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";
import {
  buildHostsViewModel,
  createHost,
  deleteHost,
  fetchHosts,
  fetchSessions,
  fetchTerminalSessions,
  retryHostInstall,
  type HostRecord,
  updateHost,
} from "@/pages/settingsApi";

type HostDraft = {
  id: string;
  name: string;
  address: string;
  sshUser: string;
  sshPort: string;
  sshPassword: string;
  agentVersion: string;
  labelsText: string;
};

const blankDraft: HostDraft = {
  id: "",
  name: "",
  address: "",
  sshUser: "root",
  sshPort: "22",
  sshPassword: "",
  agentVersion: "v0.1.0",
  labelsText: "",
};

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
  const [dialogMessage, setDialogMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);
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
      const [hostPayload, sessionPayload, terminalPayload] = await Promise.all([
        fetchHosts(),
        fetchSessions(),
        fetchTerminalSessions(),
      ]);
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
    setDialogMessage(null);
    setDialogOpen(true);
  }

  function openEdit(host: HostRecord) {
    setEditingHost(host);
    setDialogMessage(null);
    setDraft({
      id: host.id || "",
      name: host.name || "",
      address: host.address || host.name || host.id || "",
      sshUser: host.sshUser || "root",
      sshPort: String(host.sshPort || "22"),
      sshPassword: "",
      agentVersion: host.agentVersion || "v0.1.0",
      labelsText: formatLabels(host.labels),
    });
    setDialogOpen(true);
  }

  function changeDialogOpen(open: boolean) {
    setDialogOpen(open);
    if (!open) setDialogMessage(null);
  }

  async function saveHost() {
    setSaving(true);
    setDialogMessage(null);
    try {
      const payload: Record<string, unknown> = {
        name: draft.name.trim(),
        address: draft.address,
        sshUser: draft.sshUser,
        sshPort: Number(draft.sshPort) || 22,
        transport: "manual",
        installViaSsh: false,
        sshPassword: draft.sshPassword,
        agentVersion: draft.agentVersion || "v0.1.0",
        labels: parseLabels(draft.labelsText),
      };
      if (editingHost?.id) {
        payload.id = draft.id;
        await updateHost(editingHost.id, payload);
      } else {
        await createHost(payload);
      }
      setDialogOpen(false);
      setMessage({ type: "success", text: "主机信息已保存" });
      await load();
    } catch (error) {
      setDialogMessage({ type: "error", text: error instanceof Error ? error.message : "保存主机失败" });
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

  async function installAgent(host: HostRecord) {
    try {
      await retryHostInstall(host.id, {
        agentVersion: host.agentVersion || "v0.1.0",
        sshCredentialRef: host.sshCredentialRef || "",
        force: false,
      });
      setMessage({ type: "success", text: "已提交 Agent 安装任务" });
      await load();
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "安装 Agent 失败" });
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="主机列表"
      description="展示已接入主机、心跳状态、系统基础信息和操作入口。"
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
                <div className="hosts-table-shell overflow-x-auto">
                  <table className="w-full table-fixed text-left text-sm">
                    <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                      <tr>
                        <th className="w-[18%] py-2 pr-3">主机 IP / 用户名</th>
                        <th className="w-[8%] py-2 pr-3">心跳</th>
                        <th className="w-[22%] py-2 pr-3">基础信息</th>
                        <th className="w-[17%] py-2 pr-3">标签</th>
                        <th className="w-[10%] py-2 pr-3">来源 / SSH</th>
                        <th className="w-[25%] py-2 text-right">操作</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {model.pageRows.map((row: any) => {
                        const terminalEntry = buildHostTerminalEntry(row.raw || row);
                        return (
                          <tr key={row.id}>
                          <td className="py-3 pr-3">
                            <div className="font-medium text-slate-900">{row.title}</div>
                            <div className="text-xs text-slate-500">{row.subtitle}</div>
                          </td>
                          <td className="py-3 pr-3">
                            <ToneBadge tone={toneFromHeartbeat(row.heartbeatTone)}>{row.heartbeatLabel}</ToneBadge>
                            {row.installDetailLabel ? <div className="mt-1 text-xs text-slate-500">{row.installDetailLabel}</div> : null}
                            {row.lastError ? <div className="mt-1 max-w-xs break-words text-xs text-red-700">{row.lastError}</div> : null}
                          </td>
                          <td className="py-3 pr-3">
                            <div className="max-w-[220px] truncate text-slate-900" title={row.systemLabel}>{row.systemLabel}</div>
                            <div className="max-w-[220px] truncate text-xs text-slate-500" title={row.kernelLabel}>Kernel {row.kernelLabel}</div>
                            <div className="max-w-[220px] truncate text-xs text-slate-500" title={row.resourceLabel}>{row.resourceLabel}</div>
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
                          <td className="py-3 pr-3">
                            <div>{row.sourceLabel}</div>
                            <div className="text-xs text-slate-500">{row.sshLabel}</div>
                          </td>
                          <td className="py-3">
                            <div className="flex flex-wrap justify-end gap-2">
                              {row.canOpenInstallRun ? (
                                <Button variant="outline" size="sm" asChild>
                                  <Link to={`/runner-studio/runs/${encodeURIComponent(row.installRunId)}`}>
                                    <ExternalLink />
                                    Run
                                  </Link>
                                </Button>
                              ) : null}
                              <Button variant="outline" size="sm" onClick={() => void installAgent(row.raw)} disabled={row.heartbeat === "installing"}>
                                <Download />
                                安装 Agent
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => navigate(`/terminal/${row.id}`)}
                                disabled={!terminalEntry.canOpenTerminal}
                                title={terminalEntry.disabledReason || "打开独立主机终端"}
                              >
                                终端
                              </Button>
                              <Button variant="outline" size="sm" onClick={() => openEdit(row.raw)}>
                                编辑
                              </Button>
                              <ConfirmButton variant="destructive" size="icon-sm" confirm={`确认删除主机 ${row.id}？`} onConfirm={() => void removeHost(row.id)}>
                                <Trash2 />
                              </ConfirmButton>
                            </div>
                          </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : (
                <EmptyState title="暂无主机" description="没有符合条件的主机，调整筛选或接入新主机。" />
              )}
            </CardContent>
          </Card>
      )}

      <Dialog open={dialogOpen} onOpenChange={changeDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editingHost ? "编辑主机" : "接入主机"}</DialogTitle>
            <DialogDescription>填写主机接入信息并保存配置。SSH 用户必须是 root 或具备 sudo 权限。</DialogDescription>
          </DialogHeader>
          {dialogMessage ? <StatusAlert type={dialogMessage.type} title={dialogMessage.type === "error" ? "操作失败" : "操作完成"} message={dialogMessage.text} /> : null}
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="名称（可选）">
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
            <Field label="SSH 密码" hint="可选。保存后仅由服务端内部 secret 使用；编辑时留空表示保留已保存密码或使用默认 SSH 认证。">
              <Input
                type="password"
                value={draft.sshPassword}
                onChange={(event) => setDraft((prev) => ({ ...prev, sshPassword: event.target.value }))}
                placeholder="输入 SSH 用户密码"
                autoComplete="new-password"
              />
            </Field>
            <Field label="Agent 版本">
              <Input value={draft.agentVersion} onChange={(event) => setDraft((prev) => ({ ...prev, agentVersion: event.target.value }))} placeholder="v0.1.0" />
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
            <Button onClick={() => void saveHost()} disabled={saving || !draft.address || !draft.sshUser}>
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

function toneFromHeartbeat(tone: string) {
  if (tone === "success") return "success";
  if (tone === "warning") return "warning";
  if (tone === "error" || tone === "danger") return "danger";
  return "default";
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
