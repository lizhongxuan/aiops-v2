import { Check, ChevronDown, Ellipsis, Eraser, History, LaptopMinimal, LoaderCircle, Plus, Server, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { useRegisterAppShellHeaderActions } from "@/app/AppShellChromeContext";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  activateSession,
  createSession,
  fetchHosts,
  fetchLlmConfig,
  fetchSessions,
  selectHost,
  type HostRecord,
  type LlmConfigView,
  type SessionKind,
  type SessionSummary,
} from "@/pages/settingsApi";
import { resolveUiFixtureSessions, resolveUiFixtureState } from "@/lib/uiFixtureRuntime";
import { fetchAssistantTransportResumeState } from "@/transport/assistantTransportControl";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { SessionTargetContext, type SessionTargetContextValue } from "./SessionTargetContext";
import { SessionWorkspaceContext } from "./SessionWorkspaceContext";

type TargetOption = {
  value: string;
  label: string;
  description: string;
  kind: SessionTargetContextValue["targetKind"];
  metadata: Record<string, string>;
  hostId?: string;
};

type SessionContextBarProps = {
  kind: SessionKind;
  title: string;
  newSessionLabel: string;
  description: string;
  activeThreadId: string;
  skipInitialLoad?: boolean;
  terminalHref?: string;
  onThreadChange: (threadId: string, initialState?: AiopsTransportState, autoResume?: boolean) => void;
  children: ReactNode;
};

export const SESSION_CONTEXT_TIMEOUT_MS = 8000;

export function withSessionContextTimeout<T>(
  promise: Promise<T>,
  timeoutMs = SESSION_CONTEXT_TIMEOUT_MS,
  label = "session context request",
): Promise<T> {
  let timeoutId: ReturnType<typeof window.setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timeoutId = window.setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
  });
  return Promise.race([promise, timeout]).finally(() => {
    if (timeoutId !== undefined) {
      window.clearTimeout(timeoutId);
    }
  });
}

export function formatLlmLabel(config: Pick<LlmConfigView, "model" | "provider"> | null | undefined) {
  const model = firstText(config?.model);
  return model || "LLM 未配置";
}

export function SessionContextBar({
  kind,
  title,
  newSessionLabel,
  description,
  activeThreadId,
  skipInitialLoad = false,
  terminalHref,
  onThreadChange,
  children,
}: SessionContextBarProps) {
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [activeSessionId, setActiveSessionId] = useState(() => (skipInitialLoad ? activeThreadId : ""));
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [llm, setLlm] = useState<LlmConfigView | null>(null);
  const [target, setTarget] = useState(fallbackTargetValue(kind));
  const [busy, setBusy] = useState(false);
  const [activeAction, setActiveAction] = useState<"create" | "refresh" | null>(null);
  const [createFeedback, setCreateFeedback] = useState<"idle" | "success" | "error">("idle");
  const [sessionInitError, setSessionInitError] = useState("");
  const [composerFocusNonce, setComposerFocusNonce] = useState(0);
  const createFeedbackTimer = useRef<number | null>(null);

  const scopedSessions = useMemo(() => sessions.filter((session) => normalizeKind(session.kind) === kind), [kind, sessions]);
  const activeSession =
    scopedSessions.find((session) => session.id === activeSessionId) ||
    scopedSessions.find((session) => session.id === activeThreadId) ||
    null;
  const targetOptions = useMemo(() => buildTargetOptions(hosts, kind), [hosts, kind]);
  const activeTarget = targetOptions.find((item) => item.value === target) || targetOptions[0];
  const targetContext = useMemo<SessionTargetContextValue>(
    () => ({
      targetValue: activeTarget?.value || fallbackTargetValue(kind),
      targetKind: activeTarget?.kind || fallbackTargetKind(kind),
      targetLabel: activeTarget?.label || fallbackTargetLabel(kind),
      targetDescription: activeTarget?.description || fallbackTargetDescription(kind),
      hostId: activeTarget?.hostId,
      metadata: activeTarget?.metadata || fallbackTargetMetadata(kind),
    }),
    [activeTarget, kind],
  );
  const llmLabel = formatLlmLabel(llm);
  const llmConfigured = Boolean(llm?.provider && llm?.model);
  const hasUsableSession = Boolean(activeSession || activeSessionId);
  const composerDisabledReason = resolveComposerDisabledReason({
    activeAction,
    hasActiveSession: hasUsableSession,
    llmConfigured: llmConfigured || skipInitialLoad,
    sessionInitError,
  });

  function setTransientCreateFeedback(state: "success" | "error") {
    if (createFeedbackTimer.current) {
      window.clearTimeout(createFeedbackTimer.current);
    }
    setCreateFeedback(state);
    createFeedbackTimer.current = window.setTimeout(() => {
      setCreateFeedback("idle");
      createFeedbackTimer.current = null;
    }, 1600);
  }

  async function load() {
    const fixtureSessions = resolveUiFixtureSessions();
    const fixtureState = resolveUiFixtureState();
    setSessionInitError("");
    if (fixtureSessions) {
      const nextSessions = fixtureSessions.sessions || fixtureSessions.items || [];
      const nextHosts = Array.isArray(fixtureState?.hosts) ? fixtureState.hosts : hosts;
      setSessions(nextSessions);
      setHosts(nextHosts);
      setLlm({ provider: "openai", model: "gpt-5.4", apiKeySet: true });
      applySession(fixtureSessions.activeSessionId || nextSessions[0]?.id || activeThreadId, nextSessions, false, nextHosts);
      return;
    }
    setActiveAction("refresh");
    setBusy(true);
    try {
      const [sessionResult, hostResult, llmResult] = await Promise.allSettled([
        withSessionContextTimeout(fetchSessions(), SESSION_CONTEXT_TIMEOUT_MS, "加载会话"),
        withSessionContextTimeout(fetchHosts(), SESSION_CONTEXT_TIMEOUT_MS, "加载主机"),
        withSessionContextTimeout(fetchLlmConfig(), SESSION_CONTEXT_TIMEOUT_MS, "加载 LLM 配置"),
      ]);
      const nextSessions =
        sessionResult.status === "fulfilled" ? sessionResult.value.sessions || sessionResult.value.items || [] : sessions;
      const nextHosts = hostResult.status === "fulfilled" ? hostResult.value.items || [] : hosts;
      if (sessionResult.status === "rejected") {
        setSessionInitError("会话初始化失败，请刷新重试");
      }

      setSessions(nextSessions);
      setHosts(nextHosts);
      setLlm(llmResult.status === "fulfilled" ? llmResult.value : null);
      const nextActive =
        firstText(
          sessionResult.status === "fulfilled"
            ? nextSessions.find(
                (session) => session.id === sessionResult.value.activeSessionId && normalizeKind(session.kind) === kind,
              )?.id
            : "",
          nextSessions.find((session) => normalizeKind(session.kind) === kind)?.id,
        ) || "";
      if (!nextActive && sessionResult.status === "fulfilled") {
        try {
          const hostIdToBind = resolveNewSessionHostTargetId(kind, buildTargetOptions(nextHosts, kind), target, nextHosts);
          const payload = await withSessionContextTimeout(createSession(kind, hostIdToBind), SESSION_CONTEXT_TIMEOUT_MS, "创建会话");
          const createdSessions = payload.sessions || payload.items || [];
          const createdActive = payload.activeSessionId || createdSessions.find((session) => normalizeKind(session.kind) === kind)?.id || "";
          setSessions(createdSessions);
          if (createdActive) {
            applySessionWithOverride(createdActive, createdSessions, true, nextHosts, hostIdToBind);
          } else {
            setSessionInitError("会话初始化失败，请刷新重试");
          }
        } catch (error) {
          console.error(error);
          setSessionInitError("会话初始化失败，请刷新重试");
        }
        return;
      }
      const hydratedState = await hydrateTerminalSessionState(nextActive, nextSessions);
      applySession(nextActive, nextSessions, true, nextHosts, hydratedState);
    } finally {
      setBusy(false);
      setActiveAction(null);
    }
  }

  async function createAndActivateSession() {
    setActiveAction("create");
    setBusy(true);
    try {
      const nextHosts = hosts;
      const hostIdToBind = resolveNewSessionHostTargetId(kind, targetOptions, target, nextHosts);
      const payload = await createSession(kind, hostIdToBind);
      const nextSessions = payload.sessions || payload.items || [];
      const nextActive = payload.activeSessionId || nextSessions[0]?.id || "";
      setSessions(nextSessions);
      applySessionWithOverride(nextActive, nextSessions, true, nextHosts, hostIdToBind);

      if (hostIdToBind) {
        await selectHost(hostIdToBind);
        setSessions((items) =>
          items.map((item) => (item.id === nextActive ? { ...item, selectedHostId: hostIdToBind } : item)),
        );
      }

      setComposerFocusNonce((value) => value + 1);
      setCreateFeedback("success");
      setTransientCreateFeedback("success");
    } catch (error) {
      console.error(error);
      setTransientCreateFeedback("error");
    } finally {
      setBusy(false);
      setActiveAction(null);
    }
  }

  async function handleActivateSession(sessionId: string) {
    if (!sessionId) return;
    setBusy(true);
    try {
      const payload = await activateSession(sessionId);
      const nextSessions = payload.sessions || payload.items || sessions;
      setSessions(nextSessions);
      const nextActive = payload.activeSessionId || sessionId;
      const hydratedState = await hydrateTerminalSessionState(nextActive, nextSessions);
      applySession(nextActive, nextSessions, true, hosts, hydratedState);
      setComposerFocusNonce((value) => value + 1);
    } catch (error) {
      console.error(error);
    } finally {
      setBusy(false);
    }
  }

  async function handleTargetChange(value: string) {
    setTarget(value);
    const option = targetOptions.find((item) => item.value === value);
    if (!option?.hostId) {
      return;
    }
    setBusy(true);
    try {
      await selectHost(option.hostId);
      setSessions((items) =>
        items.map((item) => (item.id === activeSessionId ? { ...item, selectedHostId: option.hostId } : item)),
      );
    } catch (error) {
      console.error(error);
    } finally {
      setBusy(false);
    }
  }

  async function handleClearContext() {
    await createAndActivateSession();
  }

  function applySession(
    sessionId: string,
    sourceSessions = sessions,
    force = false,
    sourceHosts = hosts,
    hydratedInitialState?: AiopsTransportState | null,
  ) {
    return applySessionWithOverride(sessionId, sourceSessions, force, sourceHosts, undefined, hydratedInitialState);
  }

  function applySessionWithOverride(
    sessionId: string,
    sourceSessions = sessions,
    force = false,
    sourceHosts = hosts,
    hostIdOverride?: string,
    hydratedInitialState?: AiopsTransportState | null,
  ) {
    if (!sessionId) return;
    const session = sourceSessions.find((item) => item.id === sessionId);
    const nextTarget = targetValueFromSession(session, sourceHosts, kind, hostIdOverride);
    setActiveSessionId(sessionId);
    setTarget(nextTarget);
    const shouldSwitchThread = sessionId !== activeThreadId;
    const shouldApplyHydratedState = Boolean(hydratedInitialState);
    if (shouldSwitchThread || shouldApplyHydratedState) {
      const initialState = hydratedInitialState || createInitialAiopsTransportState(sessionId);
      initialState.sessionId = sessionId;
      initialState.threadId = sessionId;
      onThreadChange(sessionId, initialState, hydratedInitialState ? false : shouldAutoResumeSession(session));
    }
  }

  async function hydrateTerminalSessionState(sessionId: string, sourceSessions = sessions) {
    const session = sourceSessions.find((item) => item.id === sessionId);
    if (!shouldHydrateTerminalSession(session)) {
      return null;
    }
    try {
      return await withSessionContextTimeout(
        fetchAssistantTransportResumeState(sessionId),
        SESSION_CONTEXT_TIMEOUT_MS,
        "恢复会话",
      );
    } catch (error) {
      console.error(error);
      return null;
    }
  }

  useEffect(() => {
    if (skipInitialLoad) {
      const fixtureSessions = resolveUiFixtureSessions();
      const fixtureState = resolveUiFixtureState();
      if (fixtureSessions) {
        const nextSessions = fixtureSessions.sessions || fixtureSessions.items || [];
        setSessions(nextSessions);
        if (Array.isArray(fixtureState?.hosts)) {
          setHosts(fixtureState.hosts);
        }
        setLlm({ provider: "openai", model: "gpt-5.4", apiKeySet: true });
      }
      setActiveSessionId(activeThreadId);
      return;
    }
    void load();
  }, [activeThreadId, skipInitialLoad]);

  useEffect(() => {
    return () => {
      if (createFeedbackTimer.current) {
        window.clearTimeout(createFeedbackTimer.current);
      }
    };
  }, []);

  const createButtonLabel =
    activeAction === "create"
      ? kind === "workspace"
        ? "创建中"
        : "创建中"
      : createFeedback === "success"
        ? kind === "workspace"
          ? "已创建"
          : "已创建"
        : createFeedback === "error"
          ? "创建失败"
          : newSessionLabel;
  const headerActions = useMemo(
    () => (
      <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
        <Button onClick={() => void createAndActivateSession()} disabled={busy} className="rounded-full">
          {activeAction === "create" ? (
            <LoaderCircle className="animate-spin" />
          ) : createFeedback === "success" ? (
            <Check />
          ) : createFeedback === "error" ? (
            <X />
          ) : (
            <Plus />
          )}
          {createButtonLabel}
        </Button>
        {kind === "workspace" ? (
          <SessionMenu
            label="工作目标"
            icon={<Server className="h-3.5 w-3.5" />}
            currentLabel={formatTargetButtonLabel(kind, activeTarget?.label)}
            disabled={!targetOptions.length || busy}
            items={targetOptions.map((option) => ({
              key: option.value,
              label: option.label,
              description: option.description,
              onSelect: () => void handleTargetChange(option.value),
            }))}
          />
        ) : null}
        <MoreActionsMenu
          busy={busy}
          currentSessionLabel={activeSession ? sessionLabel(activeSession, kind) : "未创建会话"}
          terminalHref={terminalHref}
          canClear={Boolean(activeSession)}
          onClearContext={() => void handleClearContext()}
          sessionItems={scopedSessions.map((session) => ({
            key: session.id,
            label: sessionLabel(session, kind),
            onSelect: () => void handleActivateSession(session.id),
          }))}
        />
      </div>
    ),
    [activeAction, activeSession, activeTarget?.label, busy, createButtonLabel, createFeedback, kind, scopedSessions, targetOptions, terminalHref],
  );

  useRegisterAppShellHeaderActions(headerActions);

  return (
    <SessionTargetContext.Provider value={targetContext}>
      <SessionWorkspaceContext.Provider
        value={{
          kind,
          title,
          activeSessionId,
          activeSessionLabel: activeSession ? sessionLabel(activeSession, kind) : activeSessionId || "未创建",
          llmLabel,
          llmConfigured,
          busy,
          composerDisabledReason,
          composerFocusNonce,
          createSession: () => {
            void createAndActivateSession();
          },
          clearContext: () => {
            void handleClearContext();
          },
          refreshContext: () => {
            void load();
          },
        }}
      >
        <section className="flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-white">
          <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
        </section>
      </SessionWorkspaceContext.Provider>
    </SessionTargetContext.Provider>
  );
}

function SessionMenu({
  label,
  icon,
  currentLabel,
  items,
  disabled,
}: {
  label: string;
  icon: ReactNode;
  currentLabel: string;
  items: Array<{ key: string; label: string; description?: string; onSelect: () => void }>;
  disabled?: boolean;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button variant="outline" className="max-w-full gap-2 rounded-full">
          <span className="flex items-center gap-1.5">
            {icon}
            {label}
          </span>
          <span className="max-w-48 truncate text-slate-500">{currentLabel}</span>
          <ChevronDown className="h-3.5 w-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <DropdownMenuLabel>{label}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {items.map((item) => (
          <DropdownMenuItem key={item.key} onSelect={item.onSelect} className="flex flex-col items-start gap-0.5">
            <span className="font-medium text-slate-900">{item.label}</span>
            {item.description ? <span className="text-xs text-slate-500">{item.description}</span> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function MoreActionsMenu({
  busy,
  currentSessionLabel,
  terminalHref,
  canClear,
  onClearContext,
  sessionItems,
}: {
  busy: boolean;
  currentSessionLabel: string;
  terminalHref?: string;
  canClear: boolean;
  onClearContext: () => void;
  sessionItems: Array<{ key: string; label: string; onSelect: () => void }>;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={busy}>
        <Button variant="outline" size="icon" aria-label="更多操作" className="rounded-full">
          <Ellipsis className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        <DropdownMenuLabel>更多操作</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {terminalHref ? (
          <DropdownMenuItem asChild>
            <a href={terminalHref} className="flex items-center gap-2">
              <LaptopMinimal className="h-4 w-4" />
              终端
            </a>
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem disabled={!canClear} onSelect={onClearContext}>
          <Eraser className="h-4 w-4" />
          清空上下文
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>历史会话</DropdownMenuLabel>
        {sessionItems.length ? (
          sessionItems.map((item) => (
            <DropdownMenuItem key={item.key} onSelect={item.onSelect}>
              <History className="h-4 w-4" />
              <span className="truncate">{item.label}</span>
            </DropdownMenuItem>
          ))
        ) : (
          <DropdownMenuItem disabled>
            <History className="h-4 w-4" />
            {currentSessionLabel}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function buildTargetOptionsForTest(hosts: HostRecord[], kind: SessionKind): TargetOption[] {
  return buildTargetOptions(hosts, kind);
}

export function resolveHostTargetIdForTest(kind: SessionKind, options: TargetOption[], targetValue: string, hosts: HostRecord[]) {
  return resolveHostTargetId(kind, options, targetValue, hosts);
}

function buildTargetOptions(hosts: HostRecord[], kind: SessionKind): TargetOption[] {
  const normalizedHosts = hostOptions(hosts);
  if (kind === "single_host") {
    return [
      {
        value: "none",
        label: "未选择执行目标",
        description: "默认咨询模式；输入 @local 或 @主机名 选择执行目标",
        kind: "all",
        metadata: {},
      },
      ...normalizedHosts,
    ];
  }

  const options: TargetOption[] = [
    {
      value: "all",
      label: "全部主机",
      description: "主 Agent 可基于全部主机上下文规划",
      kind: "all",
      metadata: { "aiops.target.kind": "all", "aiops.target.label": "全部主机" },
    },
    ...normalizedHosts,
  ];
  for (const [key, value, count] of labelGroups(hosts)) {
    options.push({
      value: `label:${key}=${value}`,
      label: `标签组 · ${key}=${value}`,
      description: `${count} 台主机`,
      kind: "label",
      metadata: {
        "aiops.target.kind": "label",
        "aiops.target.labelKey": key,
        "aiops.target.labelValue": value,
        "aiops.target.label": `${key}=${value}`,
        ...(isEnvironmentLabel(key)
          ? {
              "aiops.environment": value,
              "aiops.target.environment": value,
              "aiops.coroot.project": value,
            }
          : {}),
      },
    });
  }
  for (const [cluster, count] of k8sGroups(hosts)) {
    options.push({
      value: `k8s:${cluster}`,
      label: `K8s · ${cluster}`,
      description: `${count} 台主机/节点`,
      kind: "k8s",
      metadata: { "aiops.target.kind": "k8s", "aiops.target.cluster": cluster, "aiops.target.label": cluster },
    });
  }
  return options;
}

function hostOptions(hosts: HostRecord[]) {
  const normalized = ensureLocalHost(hosts);
  return normalized.map((host) => {
    const label = host.name || host.address || host.id;
    return {
      value: `host:${host.id}`,
      label,
      description: firstText(host.address, host.id),
      kind: "host" as const,
      hostId: host.id,
      metadata: hostTargetMetadata(host, label),
    };
  });
}

function ensureLocalHost(hosts: HostRecord[]) {
  const deduped = hosts.filter((host) => host?.id);
  const localHost = deduped.find((host) => host.id === "server-local");
  if (localHost) {
    return [localHost, ...deduped.filter((host) => host.id !== "server-local")];
  }
  return [
    {
      id: "server-local",
      name: "server-local",
      address: "local",
      status: "online",
      terminalCapable: true,
      labels: {},
    },
    ...deduped,
  ];
}

function labelGroups(hosts: HostRecord[]) {
  const groups = new Map<string, { key: string; value: string; count: number }>();
  for (const host of hosts) {
    for (const [key, value] of Object.entries(host.labels || {})) {
      if (!key || !value) continue;
      const id = `${key}\u0000${value}`;
      const existing = groups.get(id) || { key, value, count: 0 };
      existing.count += 1;
      groups.set(id, existing);
    }
  }
  return Array.from(groups.values()).map((item) => [item.key, item.value, item.count] as const);
}

function k8sGroups(hosts: HostRecord[]) {
  const groups = new Map<string, number>();
  for (const host of hosts) {
    const labels = host.labels || {};
    const cluster = firstText(labels.k8s, labels.cluster, labels.clusterName, labels["k8s.cluster"]);
    if (cluster) groups.set(cluster, (groups.get(cluster) || 0) + 1);
  }
  return Array.from(groups.entries());
}

function hostTargetMetadata(host: HostRecord, label: string) {
  const labels = host.labels || {};
  const metadata: Record<string, string> = {
    "aiops.target.kind": "host",
    "aiops.target.hostId": host.id,
    "aiops.target.label": label,
  };
  const environment = resolveEnvironmentLabel(labels);
  if (environment) {
    metadata["aiops.environment"] = environment;
    metadata["aiops.target.environment"] = environment;
  }
  const corootProject = resolveCorootProjectLabel(labels, environment);
  if (corootProject) {
    metadata["aiops.coroot.project"] = corootProject;
  }
  const cluster = firstText(labels.k8s, labels.cluster, labels.clusterName, labels["k8s.cluster"]);
  if (cluster) {
    metadata["aiops.target.cluster"] = cluster;
  }
  return metadata;
}

function resolveEnvironmentLabel(labels: Record<string, string>) {
  return firstText(labels["aiops.environment"], labels.environment, labels.env, labels.stage, labels["app.kubernetes.io/environment"]);
}

function resolveCorootProjectLabel(labels: Record<string, string>, fallbackEnvironment?: string) {
  return firstText(
    labels["aiops.coroot.project"],
    labels["coroot.project"],
    labels.corootProject,
    labels.coroot_project,
    fallbackEnvironment,
  );
}

function isEnvironmentLabel(key: string) {
  return ["env", "environment", "stage", "aiops.environment", "app.kubernetes.io/environment"].includes(key.trim().toLowerCase());
}

function normalizeKind(kind?: string) {
  return kind === "workspace" ? "workspace" : "single_host";
}

function sessionLabel(session: SessionSummary, kind: SessionKind = "single_host") {
  const title = firstText(session.title, session.preview, session.id);
  if (kind === "single_host") {
    return title;
  }
  const host = firstText(session.selectedHostId, "未选择执行目标");
  return `${title} · ${host}`;
}

function shouldAutoResumeSession(session?: SessionSummary | null) {
  if (!session) {
    return false;
  }
  if ((session.messageCount || 0) > 0) {
    return true;
  }
  return firstText(session.status).toLowerCase() !== "empty";
}

function shouldHydrateTerminalSession(session?: SessionSummary | null) {
  if (!session || (session.messageCount || 0) <= 0) {
    return false;
  }
  return ["completed", "failed", "canceled"].includes(firstText(session.status).toLowerCase());
}

function targetValueFromSession(
  session?: SessionSummary | null,
  hosts: HostRecord[] = [],
  kind: SessionKind = "single_host",
  hostIdOverride?: string,
) {
  const selectedHostId = firstText(hostIdOverride, session?.selectedHostId);
  if (kind === "single_host") {
    return "none";
  }
  if (selectedHostId === "server-local") {
    return "all";
  }
  const hostExists = hosts.some((host) => host.id === selectedHostId);
  return hostExists ? `host:${selectedHostId}` : "all";
}

function resolveNewSessionHostTargetId(kind: SessionKind, options: TargetOption[], targetValue: string, hosts: HostRecord[]) {
  if (kind === "single_host") {
    return undefined;
  }
  return resolveHostTargetId(kind, options, targetValue, hosts);
}

export function formatTargetButtonLabel(kind: SessionKind, label?: string) {
  if (kind === "single_host") {
    return label || "未选择执行目标";
  }
  return label || "全部主机";
}

export function resolveComposerDisabledReason({
  activeAction,
  hasActiveSession,
  llmConfigured,
  sessionInitError,
}: {
  activeAction?: "create" | "refresh" | null;
  hasActiveSession: boolean;
  llmConfigured: boolean;
  sessionInitError?: string;
}) {
  if (activeAction === "create") {
    return "正在创建会话";
  }
  if (!llmConfigured) {
    return "请先在设置中配置 LLM";
  }
  if (!hasActiveSession) {
    return sessionInitError || "正在初始化会话";
  }
  return "";
}

function fallbackTargetValue(kind: SessionKind) {
  return kind === "single_host" ? "none" : "all";
}

function fallbackTargetKind(kind: SessionKind): SessionTargetContextValue["targetKind"] {
  return "all";
}

function fallbackTargetLabel(kind: SessionKind) {
  return kind === "single_host" ? "未选择执行目标" : "全部主机";
}

function fallbackTargetDescription(kind: SessionKind) {
  return kind === "single_host" ? "默认咨询模式；输入 @local 或 @主机名 选择执行目标" : "全部主机上下文";
}

function fallbackTargetMetadata(kind: SessionKind) {
  if (kind === "single_host") {
    return {};
  }
  return {
    "aiops.target.kind": "all",
    "aiops.target.label": "全部主机",
  };
}

function resolveHostTargetId(kind: SessionKind, options: TargetOption[], targetValue: string, hosts: HostRecord[]) {
  if (kind !== "single_host") {
    return undefined;
  }
  if (!targetValue || targetValue === "none") {
    return undefined;
  }
  const explicit = options.find((item) => item.value === targetValue && item.hostId)?.hostId;
  if (explicit) {
    return explicit;
  }
  return undefined;
}

function firstText(...values: unknown[]) {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : String(value || "").trim();
    if (text) return text;
  }
  return "";
}
