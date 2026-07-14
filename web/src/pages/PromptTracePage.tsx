import { useEffect, useMemo, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { ArrowLeftIcon, CopyIcon, FileTextIcon } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import { parsePromptTrace, redactSensitiveText, shortHash } from "@/utils/promptTraceViewModel";

type TraceItem = {
  id: string;
  sessionId?: string;
  turnId?: string;
  caseId?: string;
  iteration?: number;
  messageCount?: number;
  visibleTools?: string[];
  createdAt?: string;
  modifiedAt?: string;
  llmRequestCount?: number;
  usage?: TraceUsage;
  averageDurationMs?: number;
  userPromptPreview?: string;
  relativePath?: string;
  jsonPath?: string;
  markdownPath?: string;
  diffPath?: string;
  promptFingerprint?: Record<string, string>;
};

type TraceUsage = {
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
};

type TraceListPayload = { traces?: TraceItem[]; rootDir?: string; selectedId?: string; controlChain?: ControlChainPayload };
type FilePayload = { content?: string };
type ControlChainPayload = Record<string, unknown> & { available?: boolean; unavailableReason?: string };
type AgentUiSourceUserRequest = {
  turnId?: string;
  content?: string;
  preview?: string;
};
type AgentUiSources = {
  userRequests?: AgentUiSourceUserRequest[];
};

const TRACE_LIST_CARD_CLASS = "h-28 min-h-28 max-h-28 shrink-0 overflow-hidden rounded-lg border p-3 text-left text-sm";
const TRACE_LLM_CARD_CLASS = "h-20 min-h-20 max-h-20 shrink-0 overflow-hidden rounded-lg border p-3 text-left text-sm";
const TRACE_LIST_CARD_ACTIVE_CLASS = "border-slate-900 bg-slate-50";
const TRACE_LIST_CARD_IDLE_CLASS = "border-slate-200 bg-white";
const TWO_LINE_CLAMP_STYLE: CSSProperties = {
  display: "-webkit-box",
  WebkitBoxOrient: "vertical",
  WebkitLineClamp: 2,
};

async function requestJson<T>(path: string): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include" });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

function displayTime(value = "") {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function compactTraceLabel(value = "", maxLength = 30) {
  const text = String(value || "");
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength - 3)}...`;
}

function sessionCardTitle(group: TraceSessionGroup) {
  return [
    group.topic || group.label,
    `会话 ${group.label}`,
    group.latestAt ? `最近 ${displayTime(group.latestAt)}` : "",
    `用户请求 ${group.turns.length}`,
    `LLM 请求 ${group.traces.length}`,
    group.hostAgentCount ? `主机 Agent ${group.hostAgentCount}` : "",
    group.caseIds.length ? `Case ${group.caseIds.join("，")}` : "",
  ].filter(Boolean).join("\n");
}

function turnCardTitle(turn: TraceTurnGroup, preview: string) {
  return [
    preview,
    turn.label,
    turn.latestAt ? displayTime(turn.latestAt) : "",
  ].filter(Boolean).join("\n");
}

function llmCardTitle(trace: TraceItem) {
  return [
    trace.relativePath || trace.id,
    trace.createdAt ? displayTime(trace.createdAt) : "",
    trace.usage?.totalTokens ? `Token ${formatNumber(trace.usage.totalTokens)}` : "",
    trace.averageDurationMs ? `平均响应 ${formatDurationMs(trace.averageDurationMs)}` : "",
    trace.promptFingerprint?.stableHash ? `stable ${trace.promptFingerprint.stableHash}` : "",
    trace.visibleTools?.length ? `工具 ${trace.visibleTools.join("，")}` : "",
  ].filter(Boolean).join("\n");
}

function formatNumber(value = 0) {
  return (Number(value) || 0).toLocaleString();
}

function formatDurationMs(value = 0) {
  const ms = Number(value) || 0;
  if (ms <= 0) return "";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

function formatTraceMs(value = 0) {
  const ms = Number(value) || 0;
  return ms > 0 ? `${formatNumber(ms)} ms` : "-";
}

function traceTurnStats(turn: TraceTurnGroup) {
  const usage = turn.traces.reduce<TraceUsage>((sum, trace) => ({
    promptTokens: (sum.promptTokens || 0) + (trace.usage?.promptTokens || 0),
    completionTokens: (sum.completionTokens || 0) + (trace.usage?.completionTokens || 0),
    totalTokens: (sum.totalTokens || 0) + (trace.usage?.totalTokens || 0),
  }), {});
  const durations = turn.traces.map((trace) => Number(trace.averageDurationMs) || 0).filter((value) => value > 0);
  return {
    usage,
    averageDurationMs: durations.length ? Math.round(durations.reduce((sum, value) => sum + value, 0) / durations.length) : 0,
  };
}

export function PromptTracePage() {
  const initialQuery = useMemo(() => readPromptTraceQuery(), []);
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState<TraceItem[]>([]);
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [activeView, setActiveView] = useState(initialQuery.view || "overview");
  const [activeRaw, setActiveRaw] = useState(initialQuery.raw || "markdown");
  const [fileCache, setFileCache] = useState<Record<string, string>>({});
  const [controlChainCache, setControlChainCache] = useState<Record<string, ControlChainPayload>>({});
  const [error, setError] = useState("");
  const [detailOpen, setDetailOpen] = useState(Boolean(initialQuery.path));
  const directPath = initialQuery.path;

  async function loadTraces() {
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson<TraceListPayload>("/api/v1/debug/model-input-traces?limit=2000");
      const nextTraces = payload.traces || [];
      setTraces(nextTraces);
      setSelectedId((current) => {
        const directTrace = directPath ? findTraceByPath(nextTraces, directPath) : null;
        return directTrace?.id || payload.selectedId || current || nextTraces[0]?.id || "";
      });
      if (directPath) {
        setDetailOpen(true);
      }
    } catch (cause) {
      setError((cause as Error).message || "Prompt trace 加载失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadTraces();
  }, []);

  const filteredTraces = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return traces;
    return traces.filter((item) => [item.sessionId, item.caseId, item.turnId, item.relativePath, item.promptFingerprint?.stableHash].filter(Boolean).some((value) => String(value).toLowerCase().includes(needle)));
  }, [query, traces]);

  const sessionGroups = useMemo(() => buildTraceSessionGroups(filteredTraces), [filteredTraces]);
  const selectedTrace = filteredTraces.find((item) => item.id === selectedId) || sessionGroups[0]?.traces[0] || null;
  const selectedSessionGroup = selectedTrace ? sessionGroups.find((group) => group.traces.some((trace) => trace.id === selectedTrace.id)) || null : null;
  const selectedTurnGroup = selectedTrace && selectedSessionGroup ? selectedSessionGroup.turns.find((turn) => turn.traces.some((trace) => trace.id === selectedTrace.id)) || null : null;
  const controlChainKey = selectedTrace?.sessionId && selectedTrace?.turnId ? `${selectedTrace.sessionId}:${selectedTrace.turnId}` : "";
  const selectedControlChain = controlChainKey ? controlChainCache[controlChainKey] : undefined;
  const rawPath = selectedTrace ? (activeRaw === "markdown" ? selectedTrace.markdownPath : selectedTrace.jsonPath) || "" : "";
  const activePath = activeView === "raw"
    ? directPath || rawPath
    : activeView === "diff"
      ? selectedTrace?.diffPath || ""
      : selectedTrace?.jsonPath || "";

  useEffect(() => {
    async function loadFile() {
      if (!activePath || fileCache[activePath]) return;
      try {
        const payload = await requestJson<FilePayload>(`/api/v1/debug/model-input-traces/file?path=${encodeURIComponent(activePath)}`);
        setFileCache((current) => ({ ...current, [activePath]: payload.content || "" }));
      } catch (cause) {
        setError((cause as Error).message || "Prompt trace 文件读取失败");
      }
    }
    void loadFile();
  }, [activePath, fileCache]);

  useEffect(() => {
    if (!selectedTrace?.sessionId || !selectedTrace.turnId || !controlChainKey || selectedControlChain) return;
    let cancelled = false;
    const params = new URLSearchParams({
      includeControlChain: "true",
      sessionId: selectedTrace.sessionId,
      turnId: selectedTrace.turnId,
    });
    void requestJson<TraceListPayload>(`/api/v1/debug/model-input-traces?${params.toString()}`)
      .then((payload) => {
        if (!cancelled) setControlChainCache((current) => ({ ...current, [controlChainKey]: payload.controlChain || { available: false, unavailableReason: "控制链未返回" } }));
      })
      .catch(() => {
        if (!cancelled) setControlChainCache((current) => ({ ...current, [controlChainKey]: { available: false, unavailableReason: "控制链读取失败" } }));
      });
    return () => { cancelled = true; };
  }, [controlChainKey, selectedControlChain, selectedTrace?.sessionId, selectedTrace?.turnId]);

  const jsonContent = selectedTrace?.jsonPath ? fileCache[selectedTrace.jsonPath] || "" : "";
  const rawContentPath = activeView === "raw" ? activePath : rawPath;
  const rawContent = rawContentPath ? fileCache[rawContentPath] || "" : "";
  const diffContent = selectedTrace?.diffPath ? fileCache[selectedTrace.diffPath] || "" : "";
  const traceViewModel = useMemo(() => jsonContent ? parsePromptTrace(jsonContent, selectedControlChain) : null, [jsonContent, selectedControlChain]);
  const sourceUserRequests = ((traceViewModel?.agentUiSources as AgentUiSources | undefined)?.userRequests || []);
  const directJsonPath = isPromptMarkdownDirectView(directPath, activeView, activeRaw) ? selectedTrace?.jsonPath || "" : "";

  useEffect(() => {
    async function loadDirectJsonMetadata() {
      if (!directJsonPath || fileCache[directJsonPath]) return;
      try {
        const payload = await requestJson<FilePayload>(`/api/v1/debug/model-input-traces/file?path=${encodeURIComponent(directJsonPath)}`);
        setFileCache((current) => ({ ...current, [directJsonPath]: payload.content || "" }));
      } catch {
        // The Prompt MD remains useful even if optional JSON metadata is missing.
      }
    }
    void loadDirectJsonMetadata();
  }, [directJsonPath, fileCache]);

  if (isPromptMarkdownDirectView(directPath, activeView, activeRaw)) {
    return (
      <PromptMarkdownFilePage
        path={directPath}
        content={rawContent}
        loading={Boolean(activePath && !rawContent && !error)}
        error={error}
        selectedTrace={selectedTrace}
        traceViewModel={traceViewModel}
      />
    );
  }

  const views = [
    ["overview", "概览"],
    ["control_chain", "控制链"],
    ["layers", "提示层"],
    ["messages", "消息"],
    ["tools", "工具"],
    ["diff", "差异"],
    ["raw", "原始"],
  ];

  return (
    <SettingsPageFrame
      title="Prompt Trace"
      description="按会话、用户请求和 LLM 请求查看本次对话的 Prompt、消息、工具、Diff 和 Raw。"
    >
      {error ? <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div> : null}
      <div
        data-testid="prompt-trace-scroll"
        className="h-[calc(100vh-8rem)] min-h-[560px] overflow-x-auto overflow-y-hidden pb-2"
      >
        <div
          data-testid="prompt-trace-layout"
          className="grid h-full min-w-[720px] grid-cols-[minmax(180px,240px)_minmax(220px,300px)_minmax(260px,1fr)] gap-3 overflow-hidden"
        >
        <Card className="flex min-h-0 min-w-0 flex-col rounded-lg bg-white">
          <CardHeader>
            <CardTitle>历史会话</CardTitle>
          </CardHeader>
          <CardContent className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-3">
            <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索会话 / Turn / Hash / Case" />
            <div className="flex min-h-0 flex-col gap-2 overflow-auto">
              {loading ? <p className="py-6 text-center text-sm text-slate-500">加载 Prompt Trace...</p> : null}
              {!loading && sessionGroups.length ? sessionGroups.map((group) => (
                <button
                  key={group.id}
                  type="button"
                  data-testid="prompt-trace-session-card"
                  title={sessionCardTitle(group)}
                  className={`${TRACE_LIST_CARD_CLASS} ${group.id === selectedSessionGroup?.id ? TRACE_LIST_CARD_ACTIVE_CLASS : TRACE_LIST_CARD_IDLE_CLASS}`}
                  onClick={() => { setSelectedId(group.traces[0]?.id || ""); setActiveView("overview"); }}
                >
                  <span data-testid="prompt-trace-session-title" className="block line-clamp-2 break-words font-medium leading-5" style={TWO_LINE_CLAMP_STYLE}>{group.topic || group.label}</span>
                  <span className="mt-1 block truncate text-xs text-slate-500">最近 {displayTime(group.latestAt)}</span>
                  <span className="mt-2 flex flex-wrap gap-2">
                    <ToneBadge>用户请求 {group.turns.length}</ToneBadge>
                    <ToneBadge>LLM 请求 {group.traces.length}</ToneBadge>
                    {group.hostAgentCount ? <ToneBadge>主机 Agent {group.hostAgentCount}</ToneBadge> : null}
                  </span>
                </button>
              )) : null}
              {!loading && !sessionGroups.length ? <p className="py-6 text-center text-sm text-slate-500">暂无 Prompt Trace。</p> : null}
            </div>
          </CardContent>
        </Card>

        <Card className="flex min-h-0 min-w-0 flex-col rounded-lg bg-white">
          <CardHeader>
            <CardTitle>用户请求列表</CardTitle>
            <CardDescription>选择某次用户发出的对话请求。</CardDescription>
          </CardHeader>
          <CardContent className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto">
            {selectedSessionGroup?.turns.length ? selectedSessionGroup.turns.map((turn) => {
              const sourceRequest = sourceUserRequests.find((request) => request.turnId === turn.id) || null;
              const preview = turn.preview || sourceRequest?.content || sourceRequest?.preview || turn.label || "未记录用户消息";
              const stats = traceTurnStats(turn);
              return (
                <button
                  key={turn.id}
                  type="button"
                  data-testid="prompt-trace-turn-card"
                  title={turnCardTitle(turn, preview)}
                  className={`${TRACE_LIST_CARD_CLASS} ${turn.id === selectedTurnGroup?.id ? TRACE_LIST_CARD_ACTIVE_CLASS : TRACE_LIST_CARD_IDLE_CLASS}`}
                  onClick={() => { setSelectedId(turn.traces[0]?.id || ""); setActiveView("overview"); }}
                >
                  <div data-testid="prompt-trace-turn-preview" className="line-clamp-2 overflow-hidden text-sm font-medium leading-5 text-slate-950" style={TWO_LINE_CLAMP_STYLE}>{preview}</div>
                  <div className="mt-2 flex min-w-0 flex-wrap gap-2 overflow-hidden">
                    <ToneBadge>{turn.role === "host" ? "主机 Agent" : "管理 Agent"}</ToneBadge>
                    <ToneBadge><span className="block max-w-[180px] truncate">{compactTraceLabel(turn.label)}</span></ToneBadge>
                    {stats.usage.totalTokens ? <ToneBadge>Token {formatNumber(stats.usage.totalTokens)}</ToneBadge> : null}
                    {stats.averageDurationMs ? <ToneBadge>平均 {formatDurationMs(stats.averageDurationMs)}</ToneBadge> : null}
                  </div>
                  <div className="mt-1 truncate text-xs text-slate-500">{displayTime(turn.latestAt)}</div>
                </button>
              );
            }) : <p className="py-6 text-center text-sm text-slate-500">先选择一个会话。</p>}
          </CardContent>
        </Card>

        <Card className="flex min-h-0 min-w-0 flex-col rounded-lg bg-white">
          <CardHeader>
            <CardTitle>LLM 请求列表</CardTitle>
          </CardHeader>
          <CardContent className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden">
            <div className="flex min-h-0 min-w-0 flex-col gap-2 overflow-auto" data-testid="prompt-trace-llm-list">
              {(selectedTurnGroup?.traces || []).map((trace) => (
                <button
                  key={trace.id}
                  type="button"
                  data-testid="prompt-trace-llm-card"
                  title={llmCardTitle(trace)}
                  className={`${TRACE_LLM_CARD_CLASS} min-w-0 ${trace.id === selectedTrace?.id ? TRACE_LIST_CARD_ACTIVE_CLASS : TRACE_LIST_CARD_IDLE_CLASS}`}
                  onClick={() => { setSelectedId(trace.id); setActiveView("overview"); setDetailOpen(true); }}
                >
                  <span data-testid="prompt-trace-llm-path" className="block max-w-full truncate font-mono text-xs text-slate-500" title={trace.relativePath || trace.id}>{trace.relativePath || trace.id}</span>
                  <span className="mt-2 flex min-w-0 flex-wrap gap-2 overflow-hidden">
                    {trace.usage?.totalTokens ? <ToneBadge>Token {formatNumber(trace.usage.totalTokens)}</ToneBadge> : null}
                    {trace.averageDurationMs ? <ToneBadge>{trace.llmRequestCount && trace.llmRequestCount > 1 ? "平均 " : ""}{formatDurationMs(trace.averageDurationMs)}</ToneBadge> : null}
                    {trace.promptFingerprint?.stableHash ? <ToneBadge>stable {shortHash(trace.promptFingerprint.stableHash)}</ToneBadge> : null}
                    {trace.visibleTools?.length ? <ToneBadge>工具 {trace.visibleTools.length}</ToneBadge> : null}
                  </span>
                </button>
              ))}
            </div>
            {selectedTurnGroup?.traces.length ? null : <p className="py-6 text-center text-sm text-slate-500">先选择一次用户请求。</p>}
          </CardContent>
        </Card>
        </div>
      </div>
      <PromptTraceDetailDialog
        open={detailOpen}
        onOpenChange={setDetailOpen}
        selectedTrace={selectedTrace}
        activePath={activePath}
        views={views}
        activeView={activeView}
        setActiveView={setActiveView}
        activeRaw={activeRaw}
        setActiveRaw={setActiveRaw}
        traceViewModel={traceViewModel}
        rawContent={rawContent}
        diffContent={diffContent}
        selectedTraceMessageCount={selectedTrace?.messageCount}
        selectedTraceVisibleTools={selectedTrace?.visibleTools}
      />
    </SettingsPageFrame>
  );
}

function readPromptTraceQuery() {
  if (typeof window === "undefined") {
    return { path: "", view: "", raw: "" };
  }
  const params = new URLSearchParams(window.location.search);
  const view = params.get("view") || "";
  const raw = params.get("raw") || "";
  return {
    path: normalizeModelInputTracePath(params.get("path") || ""),
    view: ["overview", "control_chain", "layers", "messages", "tools", "diff", "raw"].includes(view) ? view : "",
    raw: ["markdown", "json"].includes(raw) ? raw : "",
  };
}

function normalizeModelInputTracePath(value: string) {
  const text = value.trim();
  const marker = "model-input-traces/";
  const index = text.indexOf(marker);
  if (index >= 0) {
    return text.slice(index + marker.length).replace(/^\/+/, "");
  }
  return text.replace(/^\/+/, "");
}

function findTraceByPath(traces: TraceItem[], path: string) {
  const normalized = normalizeModelInputTracePath(path);
  return traces.find((trace) => (
    normalizeModelInputTracePath(trace.id || "") === normalized ||
    normalizeModelInputTracePath(trace.relativePath || "") === normalized ||
    normalizeModelInputTracePath(trace.jsonPath || "") === normalized ||
    normalizeModelInputTracePath(trace.markdownPath || "") === normalized ||
    normalizeModelInputTracePath(trace.diffPath || "") === normalized
  )) || null;
}

function isPromptMarkdownDirectView(path: string, view: string, raw: string) {
  return Boolean(path && (path.endsWith(".md") || (view === "raw" && raw === "markdown")));
}

function PromptMarkdownFilePage({
  path,
  content,
  loading,
  error,
  selectedTrace,
  traceViewModel,
}: {
  path: string;
  content: string;
  loading: boolean;
  error: string;
  selectedTrace: TraceItem | null;
  traceViewModel: ReturnType<typeof parsePromptTrace> | null;
}) {
  const [copied, setCopied] = useState<"content" | "path" | "">("");
  const safePath = normalizeModelInputTracePath(path);
  const modelCallTitle = promptMarkdownModelCallTitle(selectedTrace, safePath);
  const fileName = safePath.split("/").filter(Boolean).at(-1) || safePath || "Prompt MD";
  const toolCount = selectedTrace?.visibleTools?.length || traceViewModel?.summary.visibleToolCount || 0;

  function returnToAgent() {
    window.history.back();
  }

  async function copyText(value: string, kind: "content" | "path") {
    if (!value) return;
    try {
      await navigator.clipboard?.writeText(value);
      setCopied(kind);
      window.setTimeout(() => setCopied(""), 1500);
    } catch {
      setCopied("");
    }
  }

  return (
    <SettingsPageFrame
      title="Prompt MD"
      description="查看主机 Agent 发给 LLM 的完整模型输入。"
      actions={(
        <button
          type="button"
          data-testid="prompt-md-header-return"
          className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50"
          onClick={returnToAgent}
        >
          <ArrowLeftIcon className="size-4" aria-hidden="true" />
          返回主机 Agent
        </button>
      )}
    >
      <section className="grid min-h-[calc(100vh-8rem)] grid-rows-[auto_minmax(0,1fr)] gap-4">
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex min-w-0 items-center gap-2">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-slate-200 bg-slate-50 text-slate-600">
                  <FileTextIcon className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <h2 className="truncate text-base font-semibold text-slate-950">{modelCallTitle}</h2>
                  <p className="mt-1 truncate font-mono text-xs text-slate-500" title={safePath}>{safePath}</p>
                </div>
              </div>
              <div className="mt-3 flex min-w-0 flex-wrap gap-2">
                {selectedTrace?.createdAt ? <ToneBadge>{displayTime(selectedTrace.createdAt)}</ToneBadge> : null}
                {selectedTrace?.usage?.totalTokens ? <ToneBadge>Token {formatNumber(selectedTrace.usage.totalTokens)}</ToneBadge> : null}
                <ToneBadge>工具 {toolCount}</ToneBadge>
                <ToneBadge>{fileName}</ToneBadge>
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
                onClick={() => copyText(content, "content")}
                disabled={!content}
              >
                <CopyIcon className="size-4" aria-hidden="true" />
                {copied === "content" ? "已复制" : "复制内容"}
              </button>
              <button
                type="button"
                className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
                onClick={() => copyText(safePath, "path")}
              >
                <CopyIcon className="size-4" aria-hidden="true" />
                {copied === "path" ? "已复制" : "复制路径"}
              </button>
              <button
                type="button"
                className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
                onClick={returnToAgent}
              >
                <ArrowLeftIcon className="size-4" aria-hidden="true" />
                返回 Agent
              </button>
            </div>
          </div>
        </div>

        {error ? <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div> : null}
        <article className="min-h-0 overflow-hidden rounded-lg border border-slate-200 bg-white">
          <pre
            data-testid="prompt-md-content"
            className="h-full min-h-[520px] overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-xs leading-5 text-slate-900"
          >
            {loading ? "Loading Prompt MD..." : content || "暂无 Prompt MD 内容"}
          </pre>
        </article>
      </section>
    </SettingsPageFrame>
  );
}

function promptMarkdownModelCallTitle(trace: TraceItem | null, path: string) {
  if (typeof trace?.iteration === "number") {
    return `第 ${trace.iteration} 轮调用 LLM`;
  }
  const match = path.match(/iteration-(\d+)/);
  if (match) {
    return `第 ${Number(match[1])} 轮调用 LLM`;
  }
  return "Prompt MD";
}

function PromptTraceDetailDialog({
  open,
  onOpenChange,
  selectedTrace,
  activePath,
  views,
  activeView,
  setActiveView,
  activeRaw,
  setActiveRaw,
  traceViewModel,
  rawContent,
  diffContent,
  selectedTraceMessageCount,
  selectedTraceVisibleTools,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedTrace: TraceItem | null;
  activePath: string;
  views: string[][];
  activeView: string;
  setActiveView: (view: string) => void;
  activeRaw: string;
  setActiveRaw: (raw: string) => void;
  traceViewModel: ReturnType<typeof parsePromptTrace> | null;
  rawContent: string;
  diffContent: string;
  selectedTraceMessageCount?: number;
  selectedTraceVisibleTools?: string[];
}) {
  const llmRequests = (traceViewModel?.agentUiSources?.userRequests || []).flatMap((request) => request.llmRequests || []);
  const contextGovernance = traceViewModel?.contextGovernance;
  const contextGovernanceEmptyText = contextGovernance?.emptyText || "暂无上下文治理事件";
  const primaryMetrics = llmRequests[0]?.detail?.metrics || {};
  const primaryFinishReason = llmRequests[0]?.detail?.finishReason || "";
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        data-testid="prompt-trace-detail-dialog"
        className="!flex max-h-[calc(100vh-3rem)] w-[min(1120px,calc(100vw-2rem))] flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <DialogHeader className="shrink-0 border-b border-slate-100 px-5 py-4">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
              <DialogTitle>模型请求详情</DialogTitle>
              <DialogDescription className="mt-2 grid gap-2">
                <span className="block max-w-full truncate font-mono" title={selectedTrace?.relativePath || activePath || ""}>{selectedTrace?.relativePath || activePath || "未选择 Prompt Trace"}</span>
              </DialogDescription>
            </div>
            <DialogClose asChild>
              <button type="button" className="shrink-0 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">
                关闭
              </button>
            </DialogClose>
          </div>
        </DialogHeader>
        <div className="shrink-0 border-b border-slate-100 px-5 py-3">
          <div className="flex flex-wrap gap-2">
            {views.map(([key, label]) => (
              <button key={key} type="button" className={`rounded-lg border px-3 py-2 text-sm ${activeView === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveView(key)}>{label}</button>
            ))}
          </div>
        </div>

        <div data-testid="prompt-trace-detail-scroll" className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <div className="grid gap-4">
            {activeView === "overview" ? (
              <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
                <CompactMetricCard label="消息数" value={traceViewModel?.summary.messageCount ?? selectedTraceMessageCount ?? 0} />
                <CompactMetricCard label="工具数" value={traceViewModel?.summary.visibleToolCount ?? selectedTraceVisibleTools?.length ?? 0} />
                <CompactMetricCard label="输入字符" value={formatNumber(traceViewModel?.summary.promptCharCount ?? 0)} />
                <CompactMetricCard label="工具表字符" value={formatNumber(traceViewModel?.summary.toolRegistryCharCount ?? 0)} />
                <CompactMetricCard label="总词元" value={selectedTrace?.usage?.totalTokens ? formatNumber(selectedTrace.usage.totalTokens) : "-"} />
                <CompactMetricCard label="平均响应" value={selectedTrace?.averageDurationMs ? formatDurationMs(selectedTrace.averageDurationMs) : "-"} />
                <CompactMetricCard label="首词元" value={formatTraceMs(primaryMetrics.firstDeltaMs)} />
                <CompactMetricCard label="流式耗时" value={formatTraceMs(primaryMetrics.streamMs)} />
                <CompactMetricCard label="流式片段" value={primaryMetrics.deltaCount ? formatNumber(primaryMetrics.deltaCount) : "-"} />
                <CompactMetricCard label="输出字符" value={primaryMetrics.outputChars ? formatNumber(primaryMetrics.outputChars) : "-"} />
                <CompactMetricCard label="结束原因" value={finishReasonLabel(primaryFinishReason)} />
                {llmRequests.length ? (
                  <section className="rounded-lg border border-slate-200 bg-white p-4 sm:col-span-2 lg:col-span-6">
                    <h3 className="font-medium text-slate-950">模型返回内容</h3>
                    <div className="mt-3 grid gap-3">
                      {llmRequests.map((request) => (
                        <div key={request.id} data-testid="prompt-trace-llm-response-card" className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                          {request.detail?.slowCauses?.length ? (
                            <div className="flex min-w-0 flex-wrap gap-2 text-xs text-slate-500">
                              {request.detail.slowCauses.map((cause) => <ToneBadge key={cause.id}>{cause.label}</ToneBadge>)}
                            </div>
                          ) : null}
                          <div className="mt-3 grid gap-3">
                            <div>
                              <pre
                                data-testid="prompt-trace-llm-output"
                                className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-slate-950 p-3 text-xs text-white"
                              >
                                {llmTextOutput(request)}
                              </pre>
                            </div>
                            {request.toolCalls?.length ? (
                              <div>
                                <div className="text-xs font-medium text-slate-600">模型工具调用</div>
                                <pre
                                  data-testid="prompt-trace-llm-tool-calls"
                                  className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-slate-950 p-3 text-xs text-white"
                                >
                                  {formatToolCallsForDisplay(request.toolCalls)}
                                </pre>
                              </div>
                            ) : null}
                          </div>
                          {request.detail?.error && request.detail.error !== "暂无错误" ? <pre className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap rounded-lg bg-red-950 p-3 text-xs text-white">{request.detail.error}</pre> : null}
                        </div>
                      ))}
                    </div>
                  </section>
                ) : null}
                <SpecialInputTracePanel specialInput={traceViewModel?.specialInput} />
                <ContextGovernancePanels
                  contextGovernance={contextGovernance}
                  emptyText={contextGovernanceEmptyText}
                />
                <ContextPanel title="Raw Payload Refs">
                  {traceViewModel?.rawPayloadRefs?.length ? (
                    <div className="grid gap-2">
                      {traceViewModel.rawPayloadRefs.map((ref) => (
                        <div key={ref.id || ref.path} className="rounded-lg border border-slate-100 bg-white p-2 text-xs">
                          <ToneBadge>{ref.kind || "raw_payload"}</ToneBadge>
                          <span className="ml-2 break-all font-mono text-slate-600">{ref.path}</span>
                          {ref.sha256 ? <span className="ml-2 font-mono text-slate-400">{shortHash(ref.sha256)}</span> : null}
                        </div>
                      ))}
                    </div>
                  ) : <EmptyGovernanceState text="暂无 raw payload refs" />}
                </ContextPanel>
              </section>
            ) : null}

            {activeView === "control_chain" ? <ControlChainPanel trace={traceViewModel} /> : null}
            {activeView === "layers" ? <section className="grid gap-3">{(traceViewModel?.layers || []).map((layer) => <Card key={layer.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{layer.title}</CardTitle><CardDescription>{layer.providerRole} / {layer.promptLayer}</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{layer.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "messages" ? <section className="grid gap-3">{(traceViewModel?.messages || []).map((message) => <Card key={message.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{message.providerRole || message.semanticRole || "message"}</CardTitle><CardDescription>{message.charCount} chars</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{message.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "tools" ? (
              <section className="grid gap-3">
                <ToolSurfacePanels toolSurface={traceViewModel?.toolSurface} />
                <ContextPanel title="Visible Tools">
                  <div className="flex flex-wrap gap-2">{(traceViewModel?.tools.visible || selectedTraceVisibleTools || []).map((tool) => <ToneBadge key={tool}>{tool}</ToneBadge>)}</div>
                </ContextPanel>
                <pre className="max-h-[520px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{traceViewModel?.tools.registryText || ""}</pre>
              </section>
            ) : null}
            {activeView === "diff" ? <pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{redactSensitiveText(diffContent) || "暂无 diff 文件"}</pre> : null}
            {activeView === "raw" ? <section className="grid gap-3"><div className="flex gap-2">{["markdown", "json"].map((key) => <button key={key} type="button" className={`rounded-lg border px-3 py-1 text-sm ${activeRaw === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveRaw(key)}>{key}</button>)}</div><pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{redactSensitiveText(rawContent) || "Loading..."}</pre></section> : null}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

type ContextGovernanceViewModel = NonNullable<ReturnType<typeof parsePromptTrace>["contextGovernance"]>;
type ToolSurfaceViewModel = NonNullable<ReturnType<typeof parsePromptTrace>["toolSurface"]>;
type SpecialInputViewModel = NonNullable<ReturnType<typeof parsePromptTrace>["specialInput"]>;
type PromptTraceViewModel = ReturnType<typeof parsePromptTrace>;

function ControlChainPanel({ trace }: { trace: PromptTraceViewModel | null }) {
  const chain = trace?.controlChain;
  const facts = trace?.controlFacts;
  const divergence = chain?.firstDivergence;
  const approval = trace?.approvalControl;
  const tools = trace?.toolControl;
  if (!trace) return <EmptyGovernanceState text="正在读取控制链…" />;
  return (
    <section data-testid="prompt-trace-control-chain" className="grid gap-4">
      <div className={`rounded-lg border p-4 ${divergence ? "border-amber-300 bg-amber-50" : "border-slate-200 bg-white"}`}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500">First divergence</p>
            <h3 className="mt-1 text-lg font-semibold text-slate-950">{divergence ? `#${divergence.sequence} ${controlKindLabel(divergence.kind)}` : "未记录权威分歧"}</h3>
          </div>
          <div data-testid="prompt-trace-first-divergence-owner" className="rounded-md bg-slate-950 px-3 py-2 font-mono text-sm text-white">
            {divergence?.ownerModule || "owner: -"}
          </div>
        </div>
        <p className="mt-2 text-xs text-slate-600">
          {chain?.available === false ? chain.unavailableReason || "Canonical rollout 不可用" : divergence?.mismatchFields?.length ? `不一致字段：${divergence.mismatchFields.join("、")}` : "只依据 typed divergence / mismatch/hash 对比，不从错误文案推断。"}
        </p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <ControlFact label="TurnAssembly" value={facts?.turnAssemblyHash} />
        <ControlFact label="StepContext" value={facts?.stepContextHash} />
        <ControlFact label="Step Revision" value={facts?.stepRevisionKind} hash={false} />
      </div>

      <ContextPanel title="Canonical sequence">
        {chain?.events?.length ? (
          <div data-testid="prompt-trace-control-strip" className="flex gap-2 overflow-x-auto pb-2">
            {chain.events.map((event) => (
              <div key={event.eventId || `${event.sequence}-${event.kind}`} className={`min-w-40 rounded-lg border p-3 ${event.isDivergence ? "border-amber-400 bg-amber-50" : "border-slate-200 bg-slate-50"}`}>
                <div className="flex items-center justify-between gap-2 text-xs"><span className="font-mono text-slate-500">#{event.sequence}</span><ToneBadge>{event.ownerModule}</ToneBadge></div>
                <div className="mt-2 text-sm font-medium text-slate-950">{controlKindLabel(event.kind)}</div>
                <div className="mt-2 truncate font-mono text-[11px] text-slate-500" title={event.hash}>{shortHash(event.hash) || "hash -"}</div>
                {controlEventRef(event) ? <div className="mt-1 truncate font-mono text-[11px] text-slate-400" title={controlEventRef(event)}>{controlEventRef(event)}</div> : null}
              </div>
            ))}
          </div>
        ) : <EmptyGovernanceState text={chain?.unavailableReason || "暂无 canonical rollout events"} />}
      </ContextPanel>

      <div data-testid="prompt-trace-control-prompt-hashes" className="grid gap-3 lg:grid-cols-2">
        <PromptHashGroup title="稳定前缀 · L0-L3" items={trace.promptHashes.stable} />
        <PromptHashGroup title="动态后缀 · L4-L6" items={trace.promptHashes.dynamic} />
      </div>

      <ContextPanel title="Prompt cache">
        <div data-testid="prompt-trace-cache-sections" className="grid gap-2">
          {trace.promptCache.sections.length ? trace.promptCache.sections.map((section) => (
            <div key={section.id} className="grid gap-2 rounded-md bg-slate-50 px-3 py-2 text-xs sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center">
              <div className="min-w-0"><span className="font-medium text-slate-700">{section.id}</span><span className="ml-2 font-mono text-slate-400">{section.kind}</span></div>
              <ToneBadge>{section.cache}</ToneBadge>
              <span className="truncate font-mono text-slate-500" title={section.missReason}>{section.cache === "hit" ? shortHash(section.hash) || "hit" : section.missReason}</span>
            </div>
          )) : <EmptyGovernanceState text="暂无 prompt cache trace" />}
        </div>
      </ContextPanel>

      <div data-testid="prompt-trace-control-bindings" className="grid gap-3 lg:grid-cols-2">
        <ContextPanel title="Tool surface / policy">
          <div className="grid gap-3 text-xs sm:grid-cols-3">
            <ToolRefList label="Model-visible" values={tools?.modelVisible || []} />
            <ToolRefList label="Dispatchable" values={tools?.dispatchable || []} />
            <ToolRefList label="Hidden" values={(tools?.hidden || []).map((item) => `${item.tool} · ${item.reason}`)} />
          </div>
          <div className="mt-3 font-mono text-xs text-slate-500">policy {shortHash(tools?.policyHash) || "-"} · surface {shortHash(tools?.fingerprint) || "-"}</div>
          {tools?.diff.visibleNotDispatchable.length || tools?.diff.dispatchableNotVisible.length ? <p className="mt-2 text-xs text-amber-700">差异：visible-only [{tools.diff.visibleNotDispatchable.join(", ") || "-"}] · dispatch-only [{tools.diff.dispatchableNotVisible.join(", ") || "-"}]</p> : null}
        </ContextPanel>
        <ContextPanel title="Approval binding">
          <div className="grid gap-2 text-xs">
            <ControlRefLine label="Approval" value={approval?.approvalId} />
            <ControlRefLine label="Token" value={approval?.actionTokenHash} />
            <ControlRefLine label="Mismatch" value={approval?.mismatchFields?.join(", ")} hash={false} />
            <ControlRefLine label="Checkpoint" value={approval?.checkpointRef || facts?.checkpointRef} hash={false} />
            <ControlRefLine label="Rollout" value={approval?.rolloutRef || facts?.rolloutRef} hash={false} />
          </div>
        </ContextPanel>
      </div>
    </section>
  );
}

function ControlFact({ label, value, hash = true }: { label: string; value?: string; hash?: boolean }) {
  return <div className="rounded-lg border border-slate-200 bg-slate-50 p-3"><div className="text-xs text-slate-500">{label}</div><div className="mt-1 truncate font-mono text-sm text-slate-950" title={value || ""}>{hash ? shortHash(value) : value || "-"}</div></div>;
}

function PromptHashGroup({ title, items }: { title: string; items: PromptTraceViewModel["promptHashes"]["all"] }) {
  return <ContextPanel title={title}><div className="grid gap-2">{items.map((item) => <div key={item.key} data-testid={`prompt-trace-hash-${item.key}`} className="flex items-center gap-2 rounded-md bg-slate-50 px-3 py-2 text-xs"><span className="inline-flex min-w-12 shrink-0 justify-center rounded-md bg-slate-900 px-2 py-1 font-mono text-[11px] font-semibold text-white">{item.layer}</span><span className="min-w-32 text-slate-600">{item.label}</span><span className="min-w-0 flex-1 truncate font-mono" title={item.value}>{shortHash(item.value) || "-"}</span><ToneBadge>{controlChangeLabel(item.change)}</ToneBadge></div>)}</div></ContextPanel>;
}

function ToolRefList({ label, values }: { label: string; values: string[] }) {
  return <div><div className="mb-2 font-medium text-slate-600">{label}</div><div className="grid gap-1">{values.length ? values.map((value) => <span key={value} className="truncate rounded bg-slate-100 px-2 py-1 font-mono" title={value}>{value}</span>) : <span className="text-slate-400">-</span>}</div></div>;
}

function ControlRefLine({ label, value, hash = true }: { label: string; value?: unknown; hash?: boolean }) {
  const display = displayControlRef(value, hash);
  return <div data-testid={`prompt-trace-control-ref-${label.toLowerCase()}`} title={display} className="flex min-w-0 gap-3"><span className="w-20 shrink-0 text-slate-500">{label}</span><span className="truncate font-mono text-slate-800">{display || "未记录"}</span></div>;
}

function controlChangeLabel(change: string) { return change === "changed" ? "changed" : change === "unchanged" ? "unchanged" : "unknown"; }
function controlKindLabel(kind = "") { return kind ? kind.replaceAll("_", " ") : "unknown"; }
function controlEventRef(event: PromptTraceViewModel["controlChain"]["events"][number]) { return event.sourceRefs[0] || (event.payloadRefs[0] ? `${event.payloadRefs[0].key}:${shortHash(event.payloadRefs[0].ref)}` : ""); }
function displayControlRef(value: unknown, hash: boolean) {
  if (value && typeof value === "object") {
    const ref = value as { sequence?: number | null; eventId?: string; hash?: string; ref?: string };
    return [ref.sequence == null ? "" : `#${ref.sequence}`, ref.eventId, shortHash(ref.hash || ref.ref)].filter(Boolean).join(" · ");
  }
  const text = typeof value === "string" ? value : "";
  return text && hash ? shortHash(text) : text;
}

function CompactMetricCard({ label, value }: { label: string; value: ReactNode }) {
  const title = `${label}: ${String(value ?? "")}`;
  return (
    <div
      className="flex h-14 min-h-14 max-h-14 flex-col justify-center overflow-hidden rounded-lg border border-slate-200 bg-slate-50 px-3 py-2"
      title={title}
    >
      <div className="truncate text-xs leading-5 text-slate-500">{label}</div>
      <div className="truncate text-sm font-semibold leading-5 text-slate-950">{value}</div>
    </div>
  );
}

function finishReasonLabel(value?: string) {
  const text = (value || "").trim();
  if (!text) return "-";
  const labels: Record<string, string> = {
    stop: "正常结束",
    length: "达到长度上限",
    tool_calls: "等待工具调用",
    content_filter: "内容过滤",
  };
  return labels[text] || text;
}

function llmTextOutput(request: { detail?: { output?: string; hasOutput?: boolean }; toolCalls?: unknown[] }) {
  const output = request.detail?.output || "暂无输出";
  if (request.detail?.hasOutput === false && request.toolCalls?.length) {
    return "本轮没有自然语言文本，模型返回工具调用。";
  }
  return output;
}

function formatToolCallsForDisplay(toolCalls: Array<{ id?: string; type?: string; name?: string; arguments?: string }>) {
  return JSON.stringify(
    toolCalls.map((call) => ({
      id: call.id || "",
      type: call.type || "function",
      name: call.name || "",
      arguments: call.arguments || "",
    })),
    null,
    2,
  );
}

function ToolSurfacePanels({ toolSurface }: { toolSurface?: ToolSurfaceViewModel }) {
  const summary = toolSurface?.summary;
  return (
    <section className="grid gap-3">
      <section className="grid gap-3 md:grid-cols-4">
        <ToolSurfaceMetric label="Initial Tools" value={summary?.initialToolCount ?? 0} />
        <ToolSurfaceMetric label="Deferred Families" value={summary?.deferredFamilyCount ?? 0} />
        <ToolSurfaceMetric label="MCP Health" value={summary?.mcpHealthCount ?? 0} />
        <ToolSurfaceMetric label="Filtered Tools" value={summary?.filteredToolCount ?? 0} />
      </section>
      <section className="grid gap-3 md:grid-cols-2">
        <ContextPanel title="Initial Tool Surface">
          {toolSurface?.initialTools.length ? (
            <div className="flex flex-wrap gap-2">{toolSurface.initialTools.map((tool) => <ToneBadge key={tool}>{tool}</ToneBadge>)}</div>
          ) : <EmptyGovernanceState text="暂无 initial tools" />}
        </ContextPanel>
        <ContextPanel title="Loaded Tool Packs">
          {toolSurface?.loadedTools.length || toolSurface?.loadedPacks.length ? (
            <div className="grid gap-2">
              <ReferenceLine label="Tools" values={toolSurface.loadedTools} />
              <ReferenceLine label="Packs" values={toolSurface.loadedPacks} />
            </div>
          ) : <EmptyGovernanceState text="暂无 loaded tools" />}
        </ContextPanel>
        <ContextPanel title="Deferred Families">
          {toolSurface?.deferredFamilies.length ? (
            <div className="grid gap-2">
              {toolSurface.deferredFamilies.map((family, index) => (
                <div key={`${family.pack}-${family.capability}-${index}`} className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                  <div className="flex min-w-0 flex-wrap gap-2">
                    {family.pack ? <ToneBadge>{family.pack}</ToneBadge> : null}
                    {family.capability ? <ToneBadge>{family.capability}</ToneBadge> : null}
                    {family.source ? <ToneBadge>{family.source}</ToneBadge> : null}
                    {family.mcpServerId ? <ToneBadge>{family.mcpServerId}</ToneBadge> : null}
                    {family.healthStatus ? <ToneBadge>{family.healthStatus}</ToneBadge> : null}
                    {family.toolCount ? <ToneBadge>tools {family.toolCount}</ToneBadge> : null}
                  </div>
                  {family.unavailableReason ? <p className="mt-2 break-words text-slate-600">{family.unavailableReason}</p> : null}
                </div>
              ))}
            </div>
          ) : <EmptyGovernanceState text="暂无 deferred families" />}
        </ContextPanel>
        <ContextPanel title="MCP Health">
          {toolSurface?.mcpHealth.length ? (
            <div className="grid gap-2">
              {toolSurface.mcpHealth.map((item) => (
                <div key={item.serverId} className="flex min-w-0 flex-wrap gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                  <ToneBadge>{item.serverId}</ToneBadge>
                  <span className="min-w-0 flex-1 break-words text-slate-700">{item.status || "-"}</span>
                </div>
              ))}
            </div>
          ) : <EmptyGovernanceState text="暂无 MCP health" />}
        </ContextPanel>
        <ContextPanel title="Tool Search Events">
          {toolSurface?.toolSearchEvents.length ? (
            <div className="grid gap-2">
              {toolSurface.toolSearchEvents.map((event) => (
                <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                  <div className="flex min-w-0 flex-wrap gap-2">
                    {event.mode ? <ToneBadge>{event.mode}</ToneBadge> : null}
                    {event.ranker ? <ToneBadge>{event.ranker}</ToneBadge> : null}
                    {event.intent ? <ToneBadge>{event.intent}</ToneBadge> : null}
                    {event.targetCompatibility ? <ToneBadge>{event.targetCompatibility}</ToneBadge> : null}
                    {event.riskDecision ? <ToneBadge>{event.riskDecision}</ToneBadge> : null}
                    {event.riskLevel ? <ToneBadge>risk {event.riskLevel}</ToneBadge> : null}
                    {event.matchCount ? <ToneBadge>matches {formatNumber(event.matchCount)}</ToneBadge> : null}
                    {event.rejectedCount ? <ToneBadge>rejected {formatNumber(event.rejectedCount)}</ToneBadge> : null}
                  </div>
                  {event.query ? <p className="mt-2 break-words text-slate-700">{event.query}</p> : null}
                  <div className="mt-2 grid gap-2">
                    <ReferenceLine label="Targets" values={event.targetRefs} />
                    <ReferenceLine label="Required Caps" values={event.requiredCaps} />
                    <ReferenceLine label="Forbidden Caps" values={event.forbiddenCaps} />
                    <ReferenceLine label="Match Reasons" values={event.matchReasons} />
                    <ReferenceLine label="Environment" values={event.environmentFacts} />
                    <ReferenceLine label="Matches" values={event.matches} />
                    {event.mcpHealth.length ? (
                      <div className="grid gap-1">
                        {event.mcpHealth.map((item) => (
                          <div key={`${event.id}-${item.serverId}`} className="flex min-w-0 flex-wrap gap-2">
                            <ToneBadge>{item.serverId}</ToneBadge>
                            <span className="min-w-0 flex-1 break-words text-slate-600">{item.status || "-"}</span>
                          </div>
                        ))}
                      </div>
                    ) : null}
                    {event.rejectedReasons.length ? (
                      <div className="grid gap-1">
                        {event.rejectedReasons.map((reason, index) => (
                          <div key={`${event.id}-${reason.toolName}-${index}`} className="rounded-md border border-slate-100 bg-white p-2">
                            <div className="flex flex-wrap gap-2">
                              {reason.toolName ? <ToneBadge>{reason.toolName}</ToneBadge> : null}
                              {reason.reason ? <ToneBadge>{reason.reason}</ToneBadge> : null}
                              {reason.mcpServerId ? <ToneBadge>{reason.mcpServerId}</ToneBadge> : null}
                              {reason.healthStatus ? <ToneBadge>{reason.healthStatus}</ToneBadge> : null}
                            </div>
                            {reason.filteredReason ? <p className="mt-1 break-words text-slate-600">{reason.filteredReason}</p> : null}
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          ) : <EmptyGovernanceState text="暂无 tool search events" />}
        </ContextPanel>
        <ContextPanel title="Filtered Tools">
          {toolSurface?.filteredTools.length ? (
            <div className="grid gap-2">
              {toolSurface.filteredTools.map((item) => (
                <div key={item.toolName} className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                  <ToneBadge>{item.toolName}</ToneBadge>
                  {item.reason ? <p className="mt-2 break-words text-slate-600">{item.reason}</p> : null}
                </div>
              ))}
            </div>
          ) : <EmptyGovernanceState text="暂无 filtered tools" />}
        </ContextPanel>
        <ContextPanel title="Rejected Tool Reasons">
          {toolSurface?.rejectedToolReasons.length ? (
            <div className="grid gap-2">
              {toolSurface.rejectedToolReasons.map((item, index) => (
                <div key={`${item.toolName}-${index}`} className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                  <div className="flex flex-wrap gap-2">
                    {item.toolName ? <ToneBadge>{item.toolName}</ToneBadge> : null}
                    {item.errorType ? <ToneBadge>{item.errorType}</ToneBadge> : null}
                    {item.requiredAction ? <ToneBadge>{item.requiredAction}</ToneBadge> : null}
                  </div>
                  {item.reason ? <p className="mt-2 break-words text-slate-600">{item.reason}</p> : null}
                </div>
              ))}
            </div>
          ) : <EmptyGovernanceState text="暂无 rejected tools" />}
        </ContextPanel>
      </section>
    </section>
  );
}

function SpecialInputTracePanel({ specialInput }: { specialInput?: SpecialInputViewModel | null }) {
  if (!specialInput) {
    return (
      <section className="rounded-lg border border-slate-200 bg-white p-4 sm:col-span-2 lg:col-span-6">
        <h3 className="font-medium text-slate-950">特殊输入短期记忆</h3>
        <div className="mt-3">
          <EmptyGovernanceState text="暂无特殊输入短期记忆 trace" />
        </div>
      </section>
    );
  }
  const active = specialInput.activeGrant;
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-4 sm:col-span-2 lg:col-span-6" data-testid="prompt-trace-special-input">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <h3 className="font-medium text-slate-950">特殊输入短期记忆</h3>
        {active ? <ToneBadge>{active.resourceKind || "resource"}:{active.resourceId || active.display || active.id}</ToneBadge> : <ToneBadge>无 active grant</ToneBadge>}
        {specialInput.summary.pendingConfirmationCount ? <ToneBadge>待确认 {specialInput.summary.pendingConfirmationCount}</ToneBadge> : null}
        {specialInput.summary.conflictCount ? <ToneBadge>冲突 {specialInput.summary.conflictCount}</ToneBadge> : null}
      </div>
      {specialInput.modelSummary ? <p className="mt-2 break-words text-sm text-slate-600">{specialInput.modelSummary}</p> : null}
      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        <div className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
          <div className="font-medium text-slate-700">Active Grant</div>
          {active ? (
            <div className="mt-2 grid gap-1 text-slate-600">
              <div>grant: <span className="font-mono">{active.id || "-"}</span></div>
              <div>resource: <span className="font-mono">{active.resourceKind || "-"}/{active.resourceId || active.display || "-"}</span></div>
              <div>actions: <span className="font-mono">{active.allowedActions?.join(", ") || "-"}</span></div>
              <div>validation: <span className="font-mono">{active.validationHash || "-"}</span></div>
            </div>
          ) : <p className="mt-2 text-slate-500">没有 active execution scope。</p>}
        </div>
        <div className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
          <div className="font-medium text-slate-700">Role Bindings</div>
          {specialInput.roleBindings?.length ? (
            <div className="mt-2 grid gap-2">
              {specialInput.roleBindings.map((binding) => (
                <div key={binding.id || binding.bindingHash || `${binding.roleKey}-${binding.resourceId}`} className="rounded-md bg-white px-2 py-1">
                  <span className="font-mono">{[binding.environmentKey, binding.clusterKey, binding.roleKey].filter(Boolean).join("/") || binding.roleKey || "-"}</span>
                  <span className="mx-2 text-slate-400">-&gt;</span>
                  <span className="font-mono">{binding.resourceKind || "resource"}:{binding.resourceId || "-"}</span>
                  {binding.runtimeName ? <span className="ml-2 text-slate-500">{binding.runtimeName}</span> : null}
                </div>
              ))}
            </div>
          ) : <p className="mt-2 text-slate-500">没有 role binding。</p>}
        </div>
        {specialInput.pendingConfirmations?.length ? (
          <div className="rounded-lg border border-amber-100 bg-amber-50 p-3 text-xs">
            <div className="font-medium text-amber-900">Pending Confirmation</div>
            <div className="mt-2 grid gap-1 text-amber-900">
              {specialInput.pendingConfirmations.map((pending) => (
                <div key={pending.id || pending.reason}>{pending.kind || "target"}: {pending.reason || pending.id}</div>
              ))}
            </div>
          </div>
        ) : null}
        {specialInput.conflicts?.length ? (
          <div className="rounded-lg border border-red-100 bg-red-50 p-3 text-xs">
            <div className="font-medium text-red-900">Conflicts</div>
            <div className="mt-2 grid gap-1 text-red-900">
              {specialInput.conflicts.map((conflict) => (
                <div key={conflict.id || conflict.roleKey}>{[conflict.environmentKey, conflict.clusterKey, conflict.roleKey].filter(Boolean).join("/")}: {conflict.reasons?.join(", ") || conflict.id}</div>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </section>
  );
}

function ToolSurfaceMetric({ label, value }: { label: string; value: number }) {
  return (
    <Card className="rounded-lg bg-slate-50">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle>{formatNumber(value)}</CardTitle>
      </CardHeader>
    </Card>
  );
}

function ContextGovernancePanels({
  contextGovernance,
  emptyText: _emptyText,
}: {
  contextGovernance?: ContextGovernanceViewModel;
  emptyText: string;
}) {
  const hasBudget = Boolean(contextGovernance?.budgetEvents.length);
  const hasCompaction = Boolean(contextGovernance?.compactionEvents.length);
  const hasMaterialization = Boolean(contextGovernance?.materializationEvents.length);
  const hasExternalReferences = Boolean(contextGovernance?.externalReferences.length);
  const hasEnvironmentContext = Boolean(contextGovernance?.environmentContext);
  const hasExternalKnowledge = Boolean(contextGovernance?.externalKnowledgeEvidence.length);

  const materializationEvents = contextGovernance?.materializationEvents || [];
  const detailedMaterializationEvents = materializationEvents.filter(hasMaterializationDetail);
  const genericMaterializationCount = materializationEvents.length - detailedMaterializationEvents.length;

  if (!hasBudget && !hasCompaction && !hasMaterialization && !hasExternalReferences && !hasEnvironmentContext && !hasExternalKnowledge) {
    return null;
  }

  return (
    <section className="md:col-span-6 grid gap-3 md:grid-cols-2">
      {hasBudget ? (
        <ContextPanel title="上下文预算">
          <div className="grid gap-3">
            {contextGovernance?.budgetEvents.map((event) => (
              <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <EventHeader event={event} />
                <div className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
                  {event.budgetItems.map((item) => (
                    <div key={item.key} className="rounded-md border border-slate-200 bg-white p-2">
                      <div className="truncate text-slate-500" title={item.key}>{governanceBudgetLabel(item.label)}</div>
                      <div className="mt-1 font-mono font-medium text-slate-950">{formatGovernanceValue(item.value)}</div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </ContextPanel>
      ) : null}

      {hasCompaction ? (
        <ContextPanel title="历史上下文压缩">
          <div className="grid gap-3">
            {contextGovernance?.compactionEvents.map((event) => (
              <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <EventHeader event={event} />
                {event.compactedIds.length ? <ReferenceLine label="已压缩" values={event.compactedIds} /> : null}
                {event.droppedGroupIds.length ? <ReferenceLine label="已移除" values={event.droppedGroupIds} /> : null}
              </div>
            ))}
          </div>
        </ContextPanel>
      ) : null}

      {hasMaterialization ? (
        <ContextPanel title="工具结果整理">
          <p className="mb-3 text-sm text-slate-600">
            工具返回内容较长时，系统会按上下文预算整理为摘要或引用，避免把原始大文本全部塞进提示词。
          </p>
          <div className="grid gap-3">
            {genericMaterializationCount > 0 ? <GenericMaterializationSummary count={genericMaterializationCount} /> : null}
            {detailedMaterializationEvents.map((event, index) => (
              <MaterializationEventCard key={event.id} event={event} index={index} />
            ))}
          </div>
        </ContextPanel>
      ) : null}

      {hasExternalReferences ? (
        <ContextPanel title="外部引用">
          <div className="grid gap-2">
            {contextGovernance?.externalReferences.map((reference) => (
              <div key={reference.id} className="flex min-w-0 flex-wrap items-center gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                <ToneBadge>{reference.layer || "上下文"}</ToneBadge>
                <span className="min-w-0 flex-1 truncate font-mono text-slate-950" title={reference.referenceId}>{reference.referenceId}</span>
                <span className="truncate text-slate-500" title={reference.kind}>{governanceKindLabel(reference.kind)}</span>
              </div>
            ))}
          </div>
        </ContextPanel>
      ) : null}

      {hasEnvironmentContext ? (
        <ContextPanel title="环境上下文">
          <div className="grid gap-2 text-xs">
            <div className="flex min-w-0 flex-wrap gap-2">
              {contextGovernance?.environmentContext?.hasConflict ? <ToneBadge>目标冲突</ToneBadge> : null}
              {contextGovernance?.environmentContext?.readOnlyReason ? <ToneBadge>{contextGovernance.environmentContext.readOnlyReason}</ToneBadge> : null}
            </div>
            <ReferenceLine label="目标" values={contextGovernance?.environmentContext?.targetRefs || []} />
            {contextGovernance?.environmentContext?.compactContext ? (
              <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-slate-950 p-3 text-white">{contextGovernance.environmentContext.compactContext}</pre>
            ) : null}
          </div>
        </ContextPanel>
      ) : null}

      {hasExternalKnowledge ? (
        <ContextPanel title="外部知识证据">
          <div className="grid gap-2">
            {contextGovernance?.externalKnowledgeEvidence.map((item) => (
              <div key={item.id} className="grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                <div className="flex min-w-0 flex-wrap gap-2">
                  {item.kind ? <ToneBadge>{knowledgeLabel(item.kind)}</ToneBadge> : null}
                  {item.sourceKind ? <ToneBadge>{knowledgeLabel(item.sourceKind)}</ToneBadge> : null}
                  {item.product ? <ToneBadge>{item.product}</ToneBadge> : null}
                  {item.version ? <ToneBadge>{item.version}</ToneBadge> : null}
                  {item.confidence ? <ToneBadge>{knowledgeLabel(item.confidence)}</ToneBadge> : null}
                </div>
                {item.query ? <p className="break-words text-slate-700">{item.query}</p> : null}
                {item.sourceTitle || item.sourceURL ? (
                  <p className="break-words font-mono text-slate-600">{item.sourceTitle || item.sourceURL}</p>
                ) : null}
                {item.applicability ? <p className="break-words text-slate-600">{item.applicability}</p> : null}
                {item.relevantExcerpt ? <p className="break-words text-slate-700">{item.relevantExcerpt}</p> : null}
              </div>
            ))}
          </div>
        </ContextPanel>
      ) : null}
    </section>
  );
}

function ContextPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-4">
      <h3 className="font-medium text-slate-950">{title}</h3>
      <div className="mt-3">{children}</div>
    </section>
  );
}

function EventHeader({ event }: { event: ContextGovernanceViewModel["events"][number] }) {
  return (
    <div className="grid gap-2">
      <div className="flex min-w-0 flex-wrap gap-2 text-xs text-slate-500">
        <ToneBadge>{governanceKindLabel(event.kind)}</ToneBadge>
        {event.retryLabel ? <ToneBadge>重试 {event.retryLabel}</ToneBadge> : null}
        {event.timeout ? <ToneBadge>已超时</ToneBadge> : null}
      </div>
      <p className="text-sm text-slate-700">{governanceMessage(event)}</p>
    </div>
  );
}

function MaterializationEventCard({
  event,
  index,
}: {
  event: ContextGovernanceViewModel["events"][number];
  index: number;
}) {
  const toolLabel = event.toolName || event.toolCallId || `第 ${index + 1} 次整理`;
  const tier = materializationTierLabel(event.materializationTier);
  return (
    <div className="rounded-lg border border-slate-100 bg-slate-50 p-3">
      <div className="flex min-w-0 flex-wrap gap-2 text-xs text-slate-500">
        <ToneBadge>{toolLabel}</ToneBadge>
        {tier ? <ToneBadge>{tier}</ToneBadge> : null}
        {event.referenceIds.length ? <ToneBadge>{`引用 ${event.referenceIds.length} 个`}</ToneBadge> : null}
      </div>
      <div className="mt-2 grid gap-1 text-sm text-slate-700">
        {event.toolName ? <p>工具：{event.toolName}</p> : null}
        {event.toolCallId ? <p>调用 ID：{event.toolCallId}</p> : null}
        {tier ? <p>结果级别：{tier}</p> : null}
        {event.originalBytes ? <p>原始大小：{formatBytes(event.originalBytes)}</p> : null}
        {event.inlineBytes ? <p>放入提示词：{formatBytes(event.inlineBytes)}</p> : null}
        <p>{materializationDetailMessage(event)}</p>
      </div>
      {event.referenceIds.length ? <ReferenceLine label="引用" values={event.referenceIds} /> : null}
    </div>
  );
}

function GenericMaterializationSummary({ count }: { count: number }) {
  return (
    <div className="rounded-lg border border-slate-100 bg-slate-50 p-3 text-sm text-slate-700">
      <div className="flex flex-wrap gap-2 text-xs">
        <ToneBadge>{`${count} 次整理`}</ToneBadge>
        <ToneBadge>旧格式记录</ToneBadge>
      </div>
      <p className="mt-2">
        这些记录只说明工具结果经过上下文预算处理；当前 trace 未记录工具名、级别和大小，所以合并展示，避免重复刷屏。
      </p>
    </div>
  );
}

function hasMaterializationDetail(event: ContextGovernanceViewModel["events"][number]) {
  return Boolean(
    event.toolCallId ||
      event.toolName ||
      event.materializationTier ||
      event.originalBytes ||
      event.inlineBytes ||
      event.referenceIds.length,
  );
}

function materializationTierLabel(tier = "") {
  const labels: Record<string, string> = {
    small: "小结果",
    medium: "中等结果",
    large: "大结果",
  };
  return labels[tier] || tier;
}

function materializationDetailMessage(event: ContextGovernanceViewModel["events"][number]) {
  const tier = (event.materializationTier || "").toLowerCase();
  if (tier === "small") {
    return "结果较小，内容直接放入提示词。";
  }
  if (tier === "medium") {
    return "结果中等，提示词内保留摘要、预览和外部引用。";
  }
  if (tier === "large") {
    return "结果较大，提示词内只保留摘要和外部引用，完整内容保存在引用中。";
  }
  return governanceMessage(event);
}

function formatBytes(value = 0) {
  const bytes = Number(value) || 0;
  if (bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB"];
  let amount = bytes;
  let unitIndex = 0;
  while (amount >= 1024 && unitIndex < units.length - 1) {
    amount /= 1024;
    unitIndex += 1;
  }
  const formatted = amount >= 10 || Number.isInteger(amount) ? Math.round(amount).toString() : amount.toFixed(1);
  return `${formatted} ${units[unitIndex]}`;
}

function governanceKindLabel(kind = "") {
  const normalized = kind.toLowerCase();
  if (/material|spill|externalize|tool[._-]?result/.test(normalized)) return "工具结果整理";
  if (normalized.includes("compact")) return "历史上下文压缩";
  if (normalized.includes("budget")) return "上下文预算";
  if (normalized.includes("reference")) return "外部引用";
  return "上下文治理";
}

function governanceMessage(event: ContextGovernanceViewModel["events"][number]) {
  const kind = event.kind.toLowerCase();
  if (/material|spill|externalize|tool[._-]?result/.test(kind)) {
    return "工具输出已按上下文预算整理，只把后续回答需要的摘要或引用放入 Prompt。";
  }
  if (kind.includes("compact") || event.compactedIds.length || event.droppedGroupIds.length) {
    return "历史对话内容较长，已压缩为摘要继续参与后续请求。";
  }
  if (event.budgetItems.length) {
    return "本次请求记录了上下文窗口、压缩阈值等预算参数。";
  }
  return event.message || "已记录一条上下文治理事件。";
}

function governanceBudgetLabel(label = "") {
  const labels: Record<string, string> = {
    "Max Context": "最大上下文",
    "Reserved Output": "预留输出",
    "Effective Window": "可用窗口",
    Warning: "预警阈值",
    "Auto Compact": "自动压缩阈值",
    "Blocking Limit": "阻断上限",
    "Small Context": "小上下文模式",
  };
  return labels[label] || label;
}

function knowledgeLabel(value = "") {
  const labels: Record<string, string> = {
    external_knowledge: "外部知识",
    official_docs: "官方文档",
    high: "高可信度",
    medium: "中可信度",
    low: "低可信度",
  };
  return labels[value] || value;
}

function ReferenceLine({ label, values }: { label: string; values: string[] }) {
  return (
    <div className="mt-3 flex min-w-0 flex-wrap gap-2 text-xs">
      <span className="text-slate-500">{label}</span>
      {values.map((value) => <ToneBadge key={value}><span className="block max-w-[220px] truncate" title={value}>{value}</span></ToneBadge>)}
    </div>
  );
}

function EmptyGovernanceState({ text }: { text: string }) {
  return <p className="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-4 text-sm text-slate-500">{text}</p>;
}

function formatGovernanceValue(value: string | number) {
  return typeof value === "number" ? formatNumber(value) : value || "-";
}

type TraceTurnGroup = {
  id: string;
  label: string;
  role: "manager" | "host";
  preview: string;
  traces: TraceItem[];
  latestAt: string;
};

type TraceSessionGroup = {
  id: string;
  label: string;
  topic: string;
  traces: TraceItem[];
  turns: TraceTurnGroup[];
  caseIds: string[];
  hostAgentCount: number;
  latestAt: string;
};

function buildTraceSessionGroups(items: TraceItem[]): TraceSessionGroup[] {
  const hostParentTurnIds = new Set(items.map(hostParentTurnId).filter(Boolean) as string[]);
  const groups = new Map<string, TraceSessionGroup>();
  for (const trace of items) {
    const sessionId = traceSessionKey(trace, hostParentTurnIds);
    const group = groups.get(sessionId) || {
      id: sessionId,
      label: sessionLabelForTrace(trace, sessionId),
      topic: "",
      traces: [],
      turns: [],
      caseIds: [],
      hostAgentCount: 0,
      latestAt: "",
    };
    group.traces.push(trace);
    if (isHostAgentTrace(trace)) {
      group.hostAgentCount = uniqueHostAgentCount(group.traces);
    }
    if (trace.caseId && !group.caseIds.includes(trace.caseId)) {
      group.caseIds.push(trace.caseId);
    }
    group.latestAt = latestTime(group.latestAt, trace.createdAt || trace.modifiedAt || "");
    groups.set(sessionId, group);
  }

  for (const group of groups.values()) {
    const turns = new Map<string, TraceTurnGroup>();
    for (const trace of group.traces) {
      const turnId = traceTurnKey(trace);
      const turn = turns.get(turnId) || {
        id: turnId,
        label: turnLabelForTrace(trace),
        role: isHostAgentTrace(trace) ? "host" : "manager",
        preview: "",
        traces: [],
        latestAt: "",
      };
      if (!turn.preview && trace.userPromptPreview) {
        turn.preview = trace.userPromptPreview;
      }
      turn.traces.push(trace);
      turn.latestAt = latestTime(turn.latestAt, trace.createdAt || trace.modifiedAt || "");
      turns.set(turnId, turn);
    }
    for (const turn of turns.values()) {
      turn.traces.sort(compareTraceRequestOrder);
    }
    group.turns = Array.from(turns.values()).sort(compareLatestDesc);
    group.topic = group.turns.find((turn) => turn.role === "manager" && turn.preview)?.preview
      || group.turns.find((turn) => turn.preview)?.preview
      || group.label;
    group.traces.sort(compareTraceDesc);
  }

  return Array.from(groups.values()).sort(compareLatestDesc);
}

function traceSessionKey(trace: TraceItem, hostParentTurnIds = new Set<string>()) {
  const parentTurnId = hostParentTurnId(trace);
  if (parentTurnId) {
    return `hostops:${parentTurnId}`;
  }
  if (trace.turnId && hostParentTurnIds.has(trace.turnId)) {
    return `hostops:${trace.turnId}`;
  }
  return trace.sessionId || "unknown-session";
}

function traceTurnKey(trace: TraceItem) {
  if (isHostAgentTrace(trace)) {
    return `host:${hostAgentKey(trace)}:${trace.turnId || "unknown-turn"}`;
  }
  return trace.turnId || "unknown-turn";
}

function isHostAgentTrace(trace: TraceItem) {
  return Boolean(
    trace.sessionId?.startsWith("host-child:") ||
    trace.relativePath?.startsWith("host-child-") ||
    trace.id?.startsWith("host-child-"),
  );
}

function hostParentTurnId(trace: TraceItem) {
  const fromSession = trace.sessionId?.match(/^host-child:hostops:(turn-[^:]+):/)?.[1];
  if (fromSession) return fromSession;
  const path = trace.relativePath || trace.id || "";
  return path.match(/^host-child-hostops-(turn-[^-\/]+(?:-[^-\/]+)*)-/)?.[1] || "";
}

function hostAgentKey(trace: TraceItem) {
  return hostAgentDisplay(trace) || trace.sessionId || trace.relativePath || trace.id || "host-agent";
}

function hostAgentDisplay(trace: TraceItem) {
  const sessionHost = trace.sessionId?.match(/^host-child:hostops:turn-[^:]+:(.+)$/)?.[1] || "";
  const path = trace.relativePath || trace.id || "";
  const pathHost = path.match(/^host-child-hostops-turn-[^-\/]+(?:-[^-\/]+)*-(.+?)\//)?.[1] || "";
  return formatHostAgentName(sessionHost || pathHost);
}

function formatHostAgentName(value: string) {
  const text = value.trim();
  if (!text) return "";
  const normalized = text.startsWith("remote-") ? text.slice("remote-".length) : text;
  const octets = normalized.match(/^(\d+)-(\d+)-(\d+)-(\d+)$/);
  if (octets) {
    return `@${octets.slice(1).join(".")}`;
  }
  return text.startsWith("@") ? text : `@${text}`;
}

function sessionLabelForTrace(trace: TraceItem, sessionId: string) {
  const parentTurnId = hostParentTurnId(trace);
  if (parentTurnId || sessionId.startsWith("hostops:")) {
    return parentTurnId || sessionId.replace(/^hostops:/, "");
  }
  return trace.sessionId || "未知会话";
}

function turnLabelForTrace(trace: TraceItem) {
  if (isHostAgentTrace(trace)) {
    return hostAgentDisplay(trace) || trace.turnId || "主机 Agent";
  }
  return trace.turnId || "未知 Turn";
}

function uniqueHostAgentCount(traces: TraceItem[]) {
  const keys = new Set(traces.filter(isHostAgentTrace).map(hostAgentKey));
  return keys.size;
}

function latestTime(left = "", right = "") {
  if (!left) return right;
  if (!right) return left;
  return new Date(right).getTime() > new Date(left).getTime() ? right : left;
}

function compareLatestDesc(left: { latestAt: string }, right: { latestAt: string }) {
  return new Date(right.latestAt || 0).getTime() - new Date(left.latestAt || 0).getTime();
}

function compareTraceDesc(left: TraceItem, right: TraceItem) {
  return new Date(right.createdAt || right.modifiedAt || 0).getTime() - new Date(left.createdAt || left.modifiedAt || 0).getTime();
}

function compareTraceRequestOrder(left: TraceItem, right: TraceItem) {
  const leftIteration = typeof left.iteration === "number" ? left.iteration : Number.MAX_SAFE_INTEGER;
  const rightIteration = typeof right.iteration === "number" ? right.iteration : Number.MAX_SAFE_INTEGER;
  if (leftIteration !== rightIteration) return leftIteration - rightIteration;
  const leftTime = new Date(left.createdAt || left.modifiedAt || 0).getTime();
  const rightTime = new Date(right.createdAt || right.modifiedAt || 0).getTime();
  if (leftTime !== rightTime) return leftTime - rightTime;
  return String(left.relativePath || left.id).localeCompare(String(right.relativePath || right.id));
}
