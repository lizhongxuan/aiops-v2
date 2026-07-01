import {
  AlertCircle,
  CheckCircle2,
  Circle,
  Download,
  ExternalLink,
  Loader2,
  Plus,
  Save,
  Search,
  Trash2,
  XCircle,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { buildHostTerminalEntry } from "@/components/hosts/hostProfileViewModels";
import {
  ConfirmButton,
  EmptyState,
  Field,
  LoadingState,
  SelectField,
  SettingsPageFrame,
  StatusAlert,
  ToneBadge,
} from "@/pages/settingsComponents";
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
  connectionMode: string;
  agentUrl: string;
  agentServerUrl: string;
  labelsText: string;
};

type InstallDialogState = {
  open: boolean;
  host: HostRecord | null;
  status: "running" | "success" | "error";
  currentStep: string;
  runId: string;
  error: string;
};

type HostErrorDialogState = {
  open: boolean;
  hostLabel: string;
  heartbeatLabel: string;
  detailLabel: string;
  error: string;
};

type HostOperationResultState = {
  open: boolean;
  type: "success" | "error" | "info";
  text: string;
};

const blankDraft: HostDraft = {
  id: "",
  name: "",
  address: "",
  sshUser: "root",
  sshPort: "22",
  sshPassword: "",
  agentVersion: "v0.1.0",
  connectionMode: "aiops_pull",
  agentUrl: "",
  agentServerUrl: "",
  labelsText: "",
};

const hostAgentInstallSteps = [
  { id: "validate-inputs", label: "校验输入" },
  { id: "validate-agent-server-url", label: "校验安装地址" },
  { id: "connect-ssh", label: "连接 SSH" },
  { id: "detect-platform", label: "识别平台" },
  { id: "ssh-preflight", label: "SSH 预检" },
  { id: "build-artifact", label: "构建 Node" },
  { id: "upload-artifact", label: "上传 Node" },
  { id: "write-config", label: "写入配置" },
  { id: "install-files", label: "安装文件" },
  { id: "install-service", label: "安装服务" },
  { id: "start-service", label: "启动服务" },
  { id: "verify-local-health", label: "验证本机健康检查" },
  { id: "finalize-host", label: "完成安装" },
];

export function HostsPage() {
  const navigate = useNavigate();
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [sessions, setSessions] = useState<unknown[]>([]);
  const [terminalSessions, setTerminalSessions] = useState<unknown[]>([]);
  const [query, setQuery] = useState("");
  const [heartbeat, setHeartbeat] = useState("all");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [pageError, setPageError] = useState("");
  const [operationResult, setOperationResult] =
    useState<HostOperationResultState | null>(null);
  const [dialogMessage, setDialogMessage] = useState<{
    type: "success" | "error" | "info";
    text: string;
  } | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingHost, setEditingHost] = useState<HostRecord | null>(null);
  const [draft, setDraft] = useState<HostDraft>(blankDraft);
  const [installDialog, setInstallDialog] = useState<InstallDialogState>({
    open: false,
    host: null,
    status: "running",
    currentStep: "validate-inputs",
    runId: "",
    error: "",
  });
  const [hostErrorDialog, setHostErrorDialog] = useState<HostErrorDialogState>({
    open: false,
    hostLabel: "",
    heartbeatLabel: "",
    detailLabel: "",
    error: "",
  });

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
      setTerminalSessions(
        terminalPayload.items || terminalPayload.sessions || [],
      );
      setPageError("");
    } catch (error) {
      setPageError(error instanceof Error ? error.message : "加载主机列表失败");
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
      connectionMode: normalizeHostConnectionMode(host.connectionMode),
      agentUrl: editableAgentURL(host),
      agentServerUrl: editableAgentServerURL(host.agentServerUrl),
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
        connectionMode: normalizeHostConnectionMode(draft.connectionMode),
        agentUrl: draft.agentUrl.trim(),
        agentServerUrl: draft.agentServerUrl.trim(),
        labels: parseLabels(draft.labelsText),
      };
      if (editingHost?.id) {
        payload.id = draft.id;
        await updateHost(editingHost.id, payload);
      } else {
        await createHost(payload);
      }
      setDialogOpen(false);
      showOperationResult("success", "主机信息已保存");
      await load();
    } catch (error) {
      setDialogMessage({
        type: "error",
        text: error instanceof Error ? error.message : "保存主机失败",
      });
    } finally {
      setSaving(false);
    }
  }

  async function removeHost(hostId: string) {
    try {
      await deleteHost(hostId);
      showOperationResult("success", "主机已删除");
      await load();
    } catch (error) {
      showOperationResult(
        "error",
        error instanceof Error ? error.message : "删除主机失败",
      );
    }
  }

  async function installAgent(host: HostRecord) {
    const connectionMode = normalizeHostConnectionMode(host.connectionMode);
    setInstallDialog({
      open: true,
      host,
      status: "running",
      currentStep: "validate-inputs",
      runId: host.installRunId || "",
      error: "",
    });
    try {
      const response = (await retryHostInstall(host.id, {
        agentVersion: host.agentVersion || "v0.1.0",
        connectionMode,
        agentServerUrl:
          connectionMode === "node_push_grpc"
            ? resolveInstallCallbackURL(host)
            : "",
        sshCredentialRef: host.sshCredentialRef || "",
        force: false,
      })) as {
        host?: HostRecord;
        items?: HostRecord[];
        installRunId?: string;
        installWorkflowId?: string;
      };
      const responseHost =
        response.items?.find((item) => item.id === host.id) ||
        response.host ||
        host;
      const nextStep = responseHost.installStep || "finalize-host";
      const nextStatus =
        responseHost.installState === "installed" ||
        nextStep === "finalize-host"
          ? "success"
          : "running";
      setInstallDialog({
        open: true,
        host: responseHost,
        status: nextStatus,
        currentStep: nextStep,
        runId:
          response.installRunId ||
          responseHost.installRunId ||
          host.installRunId ||
          "",
        error: responseHost.lastError || "",
      });
      showOperationResult("success", "已提交 Node 安装任务");
      await load();
    } catch (error) {
      setInstallDialog((current) => ({
        ...current,
        open: true,
        host,
        status: "error",
        currentStep:
          current.currentStep || host.installStep || "validate-inputs",
        error: error instanceof Error ? error.message : "安装 Node 失败",
      }));
    }
  }

  function openHostError(row: any) {
    setHostErrorDialog({
      open: true,
      hostLabel: row.title || row.id || "目标主机",
      heartbeatLabel: row.heartbeatLabel || "心跳异常",
      detailLabel: row.heartbeatDetailLabel || "",
      error: row.lastError || "暂无错误详情",
    });
  }

  function showOperationResult(
    type: HostOperationResultState["type"],
    text: string,
  ) {
    setOperationResult({ open: true, type, text });
  }

  useEffect(() => {
    void load();
  }, []);

  const draftConnectionMode = normalizeHostConnectionMode(draft.connectionMode);

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
      {pageError ? (
        <StatusAlert type="error" title="加载失败" message={pageError} />
      ) : null}
      {loading ? (
        <LoadingState label="加载主机列表" />
      ) : (
        <Card className="rounded-lg bg-white">
          <CardContent className="flex flex-col gap-3 pt-0">
            <div className="flex flex-col gap-2 md:flex-row md:items-center">
              <label className="relative min-w-0 flex-1">
                <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
                <Input
                  className="pl-8"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="搜索 IP、用户名、来源"
                />
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
                      const terminalEntry = buildHostTerminalEntry(
                        row.raw || row,
                      );
                      return (
                        <tr key={row.id}>
                          <td className="py-3 pr-3">
                            <div className="font-medium text-slate-900">
                              {row.title}
                            </div>
                          </td>
                          <td className="py-3 pr-3">
                            <ToneBadge
                              tone={toneFromHeartbeat(row.heartbeatTone)}
                            >
                              {row.heartbeatLabel}
                            </ToneBadge>
                            {row.heartbeatDetailLabel ? (
                              <div className="mt-1 max-w-[160px] break-words text-xs leading-4 text-slate-500">
                                {row.heartbeatDetailLabel}
                              </div>
                            ) : null}
                            {row.lastError ? (
                              <Button
                                variant="destructive"
                                size="xs"
                                className="mt-2"
                                aria-label={`查看 ${row.title} 错误详情`}
                                onClick={() => openHostError(row)}
                              >
                                <AlertCircle />
                                查看错误
                              </Button>
                            ) : null}
                          </td>
                          <td className="py-3 pr-3">
                            <div
                              className="max-w-[220px] truncate text-slate-900"
                              title={row.systemLabel}
                            >
                              {row.systemLabel}
                            </div>
                            <div
                              className="max-w-[220px] truncate text-xs text-slate-500"
                              title={row.kernelLabel}
                            >
                              Kernel {row.kernelLabel}
                            </div>
                            <div
                              className="max-w-[220px] truncate text-xs text-slate-500"
                              title={row.resourceLabel}
                            >
                              {row.resourceLabel}
                            </div>
                          </td>
                          <td className="py-3 pr-3">
                            {row.labels?.length ? (
                              <div className="flex max-w-xs flex-wrap gap-1">
                                {row.labels.map(
                                  (label: {
                                    key: string;
                                    value: string;
                                    label: string;
                                  }) => (
                                    <ToneBadge key={label.label}>
                                      {label.label}
                                    </ToneBadge>
                                  ),
                                )}
                              </div>
                            ) : (
                              <span className="text-xs text-slate-400">
                                未打标签
                              </span>
                            )}
                          </td>
                          <td className="py-3 pr-3">
                            <div>{row.sourceLabel}</div>
                            <div className="text-xs text-slate-500">
                              {row.sshLabel}
                            </div>
                          </td>
                          <td className="py-3">
                            <div className="flex flex-wrap justify-end gap-2">
                              {row.canOpenInstallRun ? (
                                <Button variant="outline" size="sm" asChild>
                                  <Link
                                    to={`/runner-studio/runs/${encodeURIComponent(row.installRunId)}`}
                                  >
                                    <ExternalLink />
                                    Run
                                  </Link>
                                </Button>
                              ) : null}
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => void installAgent(row.raw)}
                                disabled={row.heartbeat === "installing"}
                              >
                                <Download />
                                安装 Node
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => navigate(`/terminal/${row.id}`)}
                                disabled={!terminalEntry.canOpenTerminal}
                                title={
                                  terminalEntry.disabledReason ||
                                  "打开独立主机终端"
                                }
                              >
                                终端
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => openEdit(row.raw)}
                              >
                                编辑
                              </Button>
                              <ConfirmButton
                                variant="destructive"
                                size="icon-sm"
                                aria-label={`删除主机 ${row.id}`}
                                confirm={`确认删除主机 ${row.id}？`}
                                onConfirm={() => void removeHost(row.id)}
                              >
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
              <EmptyState
                title="暂无主机"
                description="没有符合条件的主机，调整筛选或接入新主机。"
              />
            )}
          </CardContent>
        </Card>
      )}

      <Dialog open={dialogOpen} onOpenChange={changeDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editingHost ? "编辑主机" : "接入主机"}</DialogTitle>
            <DialogDescription>
              填写主机接入信息并保存配置。SSH 用户必须是 root 或具备 sudo 权限。
            </DialogDescription>
          </DialogHeader>
          {dialogMessage ? (
            <StatusAlert
              type={dialogMessage.type}
              title={dialogMessage.type === "error" ? "操作失败" : "操作完成"}
              message={dialogMessage.text}
            />
          ) : null}
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="名称（可选）">
              <Input
                value={draft.name}
                onChange={(event) =>
                  setDraft((prev) => ({ ...prev, name: event.target.value }))
                }
              />
            </Field>
            <Field label="地址">
              <Input
                value={draft.address}
                onChange={(event) =>
                  setDraft((prev) => {
                    const address = event.target.value;
                    const shouldFollowAddress =
                      draftConnectionMode === "aiops_pull" &&
                      (!prev.agentUrl.trim() ||
                        prev.agentUrl.trim() === defaultAgentURL(prev.address));
                    return {
                      ...prev,
                      address,
                      agentUrl: shouldFollowAddress
                        ? defaultAgentURL(address)
                        : prev.agentUrl,
                    };
                  })
                }
              />
            </Field>
            <Field label="SSH 用户">
              <Input
                value={draft.sshUser}
                onChange={(event) =>
                  setDraft((prev) => ({ ...prev, sshUser: event.target.value }))
                }
              />
            </Field>
            <Field label="SSH 端口">
              <Input
                value={draft.sshPort}
                onChange={(event) =>
                  setDraft((prev) => ({ ...prev, sshPort: event.target.value }))
                }
              />
            </Field>
            <Field label="SSH 密码" hint="留空保留已保存密码">
              <Input
                type="password"
                value={draft.sshPassword}
                onChange={(event) =>
                  setDraft((prev) => ({
                    ...prev,
                    sshPassword: event.target.value,
                  }))
                }
                placeholder="输入 SSH 用户密码"
                autoComplete="new-password"
              />
            </Field>
            <Field label="Node 版本">
              <Input
                value={draft.agentVersion}
                onChange={(event) =>
                  setDraft((prev) => ({
                    ...prev,
                    agentVersion: event.target.value,
                  }))
                }
                placeholder="v0.1.0"
              />
            </Field>
            <Field label="连接方式">
              <SelectField
                value={draftConnectionMode}
                onChange={(connectionMode) =>
                  setDraft((prev) => ({
                    ...prev,
                    connectionMode,
                    agentUrl:
                      connectionMode === "aiops_pull" && !prev.agentUrl.trim()
                        ? defaultAgentURL(prev.address)
                        : prev.agentUrl,
                  }))
                }
                options={[
                  {
                    label: "aiops-v2 主动连接 Node（默认）",
                    value: "aiops_pull",
                  },
                  {
                    label: "Node 主动回连 aiops-v2（gRPC）",
                    value: "node_push_grpc",
                  },
                ]}
              />
            </Field>
            <div className="md:col-span-2">
              {draftConnectionMode === "node_push_grpc" ? (
                <Field
                  label="AI Server 回调地址"
                  hint="Node 主动回连 aiops-v2 时使用，必须是目标主机可访问的 ai-server HTTP 地址"
                >
                  <Input
                    value={draft.agentServerUrl}
                    onChange={(event) =>
                      setDraft((prev) => ({
                        ...prev,
                        agentServerUrl: event.target.value,
                      }))
                    }
                    placeholder="http://ai-server.example:18080"
                  />
                </Field>
              ) : (
                <Field
                  label="Node 连接地址"
                  hint="aiops-v2 主动连接 Node 时使用，通常是 http://主机IP:7072"
                >
                  <Input
                    value={draft.agentUrl}
                    onChange={(event) =>
                      setDraft((prev) => ({
                        ...prev,
                        agentUrl: event.target.value,
                      }))
                    }
                    placeholder={defaultAgentURL(draft.address)}
                  />
                </Field>
              )}
            </div>
            <div className="md:col-span-2">
              <Field
                label="标签"
                hint="用逗号或换行分隔，例如 env=prod, role=web, cluster=ops-k8s"
              >
                <Input
                  value={draft.labelsText}
                  onChange={(event) =>
                    setDraft((prev) => ({
                      ...prev,
                      labelsText: event.target.value,
                    }))
                  }
                  placeholder="env=prod, role=web"
                />
              </Field>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button
              onClick={() => void saveHost()}
              disabled={saving || !draft.address || !draft.sshUser}
            >
              <Save />
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <HostAgentInstallDialog
        state={installDialog}
        onOpenChange={(open) =>
          setInstallDialog((current) => ({ ...current, open }))
        }
      />
      <HostErrorDialog
        state={hostErrorDialog}
        onOpenChange={(open) =>
          setHostErrorDialog((current) => ({ ...current, open }))
        }
      />
      <HostOperationResultDialog
        state={operationResult}
        onOpenChange={(open) => {
          if (!open) setOperationResult(null);
        }}
      />
    </SettingsPageFrame>
  );
}

function HostOperationResultDialog({
  state,
  onOpenChange,
}: {
  state: HostOperationResultState | null;
  onOpenChange: (open: boolean) => void;
}) {
  const Icon = state?.type === "error" ? AlertCircle : CheckCircle2;
  const title = state?.type === "error" ? "操作失败" : "操作完成";
  const iconClass =
    state?.type === "error" ? "text-red-600" : "text-emerald-600";
  return (
    <Dialog open={Boolean(state?.open)} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="host-operation-result-dialog"
        className="sm:max-w-sm"
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Icon className={`h-5 w-5 ${iconClass}`} />
            {title}
          </DialogTitle>
          <DialogDescription className="whitespace-pre-wrap break-words text-base text-slate-700">
            {state?.text || ""}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function HostErrorDialog({
  state,
  onOpenChange,
}: {
  state: HostErrorDialogState;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={state.open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="host-error-dialog"
        className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-2xl"
      >
        <DialogHeader>
          <DialogTitle>主机错误详情</DialogTitle>
          <DialogDescription>
            {state.hostLabel} · {state.heartbeatLabel}
            {state.detailLabel ? ` · ${state.detailLabel}` : ""}
          </DialogDescription>
        </DialogHeader>
        <div
          data-testid="host-error-dialog-scroll"
          className="min-h-0 overflow-y-auto rounded-lg border bg-slate-950 p-3 text-xs leading-5 text-slate-100"
        >
          <pre className="whitespace-pre-wrap break-words font-mono">
            {state.error}
          </pre>
        </div>
        <DialogFooter className="shrink-0">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function HostAgentInstallDialog({
  state,
  onOpenChange,
}: {
  state: InstallDialogState;
  onOpenChange: (open: boolean) => void;
}) {
  const hostLabel =
    state.host?.name || state.host?.address || state.host?.id || "目标主机";
  return (
    <Dialog open={state.open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="host-agent-install-dialog"
        className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>Node 安装步骤</DialogTitle>
          <DialogDescription>
            {hostLabel} ·{" "}
            {state.status === "success"
              ? "安装流程已完成"
              : state.status === "error"
                ? "安装流程失败"
                : "正在执行安装流程"}
          </DialogDescription>
        </DialogHeader>
        <div
          data-testid="host-agent-install-dialog-scroll"
          className="min-h-0 space-y-3 overflow-y-auto pr-1"
        >
          <div className="grid gap-2">
            {hostAgentInstallSteps.map((step) => {
              const status = installStepStatus(step.id, state);
              return (
                <div
                  key={step.id}
                  className="flex items-center gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2 text-sm"
                >
                  {status === "done" ? (
                    <CheckCircle2 className="h-4 w-4 text-emerald-600" />
                  ) : null}
                  {status === "running" ? (
                    <Loader2 className="h-4 w-4 animate-spin text-blue-600" />
                  ) : null}
                  {status === "failed" ? (
                    <XCircle className="h-4 w-4 text-red-600" />
                  ) : null}
                  {status === "pending" ? (
                    <Circle className="h-4 w-4 text-slate-300" />
                  ) : null}
                  <span
                    className={
                      status === "pending"
                        ? "text-slate-500"
                        : "font-medium text-slate-900"
                    }
                  >
                    {step.label}
                  </span>
                </div>
              );
            })}
          </div>
          {state.runId ? (
            <div className="rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-600">
              Run ID: {state.runId}
            </div>
          ) : null}
          {state.error ? (
            <StatusAlert type="error" title="安装失败" message={state.error} />
          ) : null}
        </div>
        <DialogFooter className="shrink-0">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function installStepStatus(stepId: string, state: InstallDialogState) {
  const currentIndex = Math.max(
    0,
    hostAgentInstallSteps.findIndex((step) => step.id === state.currentStep),
  );
  const stepIndex = hostAgentInstallSteps.findIndex(
    (step) => step.id === stepId,
  );
  if (state.status === "success") return "done";
  if (state.status === "error" && stepIndex === currentIndex) return "failed";
  if (stepIndex < currentIndex) return "done";
  if (stepIndex === currentIndex) return "running";
  return "pending";
}

function formatLabels(labels?: Record<string, string>) {
  return Object.entries(labels || {})
    .map(([key, value]) => [key.trim(), String(value || "").trim()])
    .filter(([key, value]) => key && value)
    .sort(([leftKey, leftValue], [rightKey, rightValue]) =>
      `${leftKey}=${leftValue}`.localeCompare(`${rightKey}=${rightValue}`),
    )
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

function editableAgentServerURL(value?: string) {
  const trimmed = String(value || "").trim();
  if (!trimmed) return "";
  if (looksLikeNodeEndpointURL(trimmed)) return browserOrigin();
  return trimmed;
}

function editableAgentURL(host: HostRecord) {
  const saved = String(host.agentUrl || "").trim();
  if (saved) return saved;
  return defaultAgentURL(host.address || host.name || host.id || "");
}

function defaultAgentURL(address?: string) {
  const trimmed = String(address || "").trim();
  if (!trimmed) return "http://<host>:7072";
  if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/+$/, "");
  return `http://${trimmed}:7072`;
}

function normalizeHostConnectionMode(value?: string) {
  const mode = String(value || "").trim();
  if (mode === "node_push_grpc" || mode === "grpc_reverse") {
    return "node_push_grpc";
  }
  return "aiops_pull";
}

function resolveInstallCallbackURL(host: HostRecord) {
  const saved = String(host.agentServerUrl || "").trim();
  if (saved && !looksLikeNodeEndpointURL(saved)) return saved;
  return browserOrigin();
}

function looksLikeNodeEndpointURL(value: string) {
  try {
    const parsed = new URL(value);
    const path = parsed.pathname.replace(/\/+$/, "");
    return (
      parsed.port === "7072" ||
      path === "/exec" ||
      path === "/run" ||
      path === "/terminal" ||
      path.startsWith("/api/v1/host-agents")
    );
  } catch {
    return false;
  }
}

function browserOrigin() {
  if (typeof window === "undefined") return "";
  return window.location.origin === "null" ? "" : window.location.origin;
}
