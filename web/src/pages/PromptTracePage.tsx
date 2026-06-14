import { useEffect, useMemo, useState } from "react";
import type { CSSProperties, ReactNode } from "react";

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

type TraceListPayload = { traces?: TraceItem[]; rootDir?: string; selectedId?: string };
type FilePayload = { content?: string };
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
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState<TraceItem[]>([]);
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [activeView, setActiveView] = useState("overview");
  const [activeRaw, setActiveRaw] = useState("markdown");
  const [fileCache, setFileCache] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [detailOpen, setDetailOpen] = useState(false);

  async function loadTraces() {
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson<TraceListPayload>("/api/v1/debug/model-input-traces?limit=2000");
      setTraces(payload.traces || []);
      setSelectedId((current) => payload.selectedId || current || payload.traces?.[0]?.id || "");
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
  const selectedSessionGroup = selectedTrace ? sessionGroups.find((group) => group.id === traceSessionKey(selectedTrace)) || null : null;
  const selectedTurnGroup = selectedTrace && selectedSessionGroup ? selectedSessionGroup.turns.find((turn) => turn.id === traceTurnKey(selectedTrace)) || null : null;
  const rawPath = selectedTrace ? (activeRaw === "markdown" ? selectedTrace.markdownPath : selectedTrace.jsonPath) || "" : "";
  const activePath = activeView === "raw" ? rawPath : activeView === "diff" ? selectedTrace?.diffPath || "" : selectedTrace?.jsonPath || "";

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

  const jsonContent = selectedTrace?.jsonPath ? fileCache[selectedTrace.jsonPath] || "" : "";
  const rawContent = rawPath ? fileCache[rawPath] || "" : "";
  const diffContent = selectedTrace?.diffPath ? fileCache[selectedTrace.diffPath] || "" : "";
  const traceViewModel = useMemo(() => jsonContent ? parsePromptTrace(jsonContent) : null, [jsonContent]);
  const sourceUserRequests = ((traceViewModel?.agentUiSources as AgentUiSources | undefined)?.userRequests || []);

  const views = [
    ["overview", "概览"],
    ["layers", "Prompt 层"],
    ["messages", "消息"],
    ["tools", "工具"],
    ["diff", "Diff"],
    ["raw", "Raw"],
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
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="max-h-[88vh] overflow-hidden sm:max-w-5xl">
        <DialogHeader>
          <div className="flex flex-wrap items-start justify-between gap-3 pr-2">
            <div className="min-w-0">
              <DialogTitle>LLM 请求详情</DialogTitle>
              <DialogDescription className="mt-2 grid gap-2">
                <span className="truncate" title={selectedTrace?.relativePath || activePath || ""}>{selectedTrace?.relativePath || activePath || "未选择 Prompt Trace"}</span>
                <span className="flex flex-wrap gap-2">
                  {selectedTrace?.promptFingerprint?.stableHash ? <ToneBadge>stable {shortHash(selectedTrace.promptFingerprint.stableHash)}</ToneBadge> : null}
                  {selectedTrace?.promptFingerprint?.developerHash ? <ToneBadge>developer {shortHash(selectedTrace.promptFingerprint.developerHash)}</ToneBadge> : null}
                  {selectedTrace?.promptFingerprint?.toolRegistryHash ? <ToneBadge>tools {shortHash(selectedTrace.promptFingerprint.toolRegistryHash)}</ToneBadge> : null}
                </span>
              </DialogDescription>
            </div>
            <DialogClose asChild>
              <button type="button" className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">
                关闭
              </button>
            </DialogClose>
          </div>
        </DialogHeader>
        <div className="max-h-[calc(88vh-120px)] overflow-auto pr-1">
          <div className="flex flex-wrap gap-2">
            {views.map(([key, label]) => (
              <button key={key} type="button" className={`rounded-lg border px-3 py-2 text-sm ${activeView === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveView(key)}>{label}</button>
            ))}
          </div>

          <div className="mt-4 grid gap-4">
            {activeView === "overview" ? (
              <section className="grid gap-3 md:grid-cols-5">
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Messages</CardDescription><CardTitle>{traceViewModel?.summary.messageCount ?? selectedTraceMessageCount ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Tools</CardDescription><CardTitle>{traceViewModel?.summary.visibleToolCount ?? selectedTraceVisibleTools?.length ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Prompt chars</CardDescription><CardTitle>{traceViewModel?.summary.promptCharCount ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Total tokens</CardDescription><CardTitle>{selectedTrace?.usage?.totalTokens ? formatNumber(selectedTrace.usage.totalTokens) : "-"}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Avg response</CardDescription><CardTitle>{selectedTrace?.averageDurationMs ? formatDurationMs(selectedTrace.averageDurationMs) : "-"}</CardTitle></CardHeader></Card>
                {llmRequests.length ? (
                  <section className="md:col-span-5 rounded-lg border border-slate-200 bg-white p-4">
                    <h3 className="font-medium text-slate-950">LLM 返回内容</h3>
                    <div className="mt-3 grid gap-3">
                      {llmRequests.map((request) => (
                        <div key={request.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                          <div className="flex min-w-0 flex-wrap gap-2 text-xs text-slate-500">
                            <ToneBadge>{request.id}</ToneBadge>
                            <ToneBadge>{request.detail?.tokens || "暂无 token 信息"}</ToneBadge>
                            <ToneBadge>{request.detail?.duration || "暂无耗时"}</ToneBadge>
                          </div>
                          <pre className="mt-3 max-h-64 overflow-auto whitespace-pre-wrap rounded-lg bg-slate-950 p-3 text-xs text-white">{request.detail?.output || "暂无输出"}</pre>
                          {request.detail?.error && request.detail.error !== "暂无错误" ? <pre className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap rounded-lg bg-red-950 p-3 text-xs text-white">{request.detail.error}</pre> : null}
                        </div>
                      ))}
                    </div>
                  </section>
                ) : null}
                <ContextGovernancePanels
                  contextGovernance={contextGovernance}
                  emptyText={contextGovernanceEmptyText}
                />
                <pre className="md:col-span-5 overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{JSON.stringify({ summary: traceViewModel?.summary, warnings: traceViewModel?.warnings }, null, 2)}</pre>
              </section>
            ) : null}

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
  emptyText,
}: {
  contextGovernance?: ContextGovernanceViewModel;
  emptyText: string;
}) {
  return (
    <section className="md:col-span-5 grid gap-3 md:grid-cols-2">
      <ContextPanel title="Context Budget">
        {contextGovernance?.budgetEvents.length ? (
          <div className="grid gap-3">
            {contextGovernance.budgetEvents.map((event) => (
              <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <EventHeader event={event} />
                <div className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
                  {event.budgetItems.map((item) => (
                    <div key={item.key} className="rounded-md border border-slate-200 bg-white p-2">
                      <div className="truncate text-slate-500" title={item.key}>{item.label}</div>
                      <div className="mt-1 font-mono font-medium text-slate-950">{formatGovernanceValue(item.value)}</div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        ) : <EmptyGovernanceState text={emptyText} />}
      </ContextPanel>

      <ContextPanel title="Compaction Events">
        {contextGovernance?.compactionEvents.length ? (
          <div className="grid gap-3">
            {contextGovernance.compactionEvents.map((event) => (
              <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <EventHeader event={event} />
                {event.compactedIds.length ? <ReferenceLine label="Compacted" values={event.compactedIds} /> : null}
                {event.droppedGroupIds.length ? <ReferenceLine label="Dropped" values={event.droppedGroupIds} /> : null}
              </div>
            ))}
          </div>
        ) : <EmptyGovernanceState text={emptyText} />}
      </ContextPanel>

      <ContextPanel title="Tool Result Materialization">
        {contextGovernance?.materializationEvents.length ? (
          <div className="grid gap-3">
            {contextGovernance.materializationEvents.map((event) => (
              <div key={event.id} className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <EventHeader event={event} />
                {event.referenceIds.length ? <ReferenceLine label="Refs" values={event.referenceIds} /> : null}
              </div>
            ))}
          </div>
        ) : <EmptyGovernanceState text={emptyText} />}
      </ContextPanel>

      <ContextPanel title="External References">
        {contextGovernance?.externalReferences.length ? (
          <div className="grid gap-2">
            {contextGovernance.externalReferences.map((reference) => (
              <div key={reference.id} className="flex min-w-0 flex-wrap items-center gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
                <ToneBadge>{reference.layer || "context"}</ToneBadge>
                <span className="min-w-0 flex-1 truncate font-mono text-slate-950" title={reference.referenceId}>{reference.referenceId}</span>
                <span className="truncate text-slate-500" title={reference.kind}>{reference.kind}</span>
              </div>
            ))}
          </div>
        ) : <EmptyGovernanceState text={emptyText} />}
      </ContextPanel>
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
        {event.layer ? <ToneBadge>{event.layer}</ToneBadge> : null}
        {event.kind ? <ToneBadge>{event.kind}</ToneBadge> : null}
        {event.retryLabel ? <ToneBadge>retry {event.retryLabel}</ToneBadge> : null}
        {event.timeout ? <ToneBadge>timeout</ToneBadge> : null}
      </div>
      {event.message ? <p className="text-sm text-slate-700">{event.message}</p> : null}
    </div>
  );
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
  latestAt: string;
};

function buildTraceSessionGroups(items: TraceItem[]): TraceSessionGroup[] {
  const groups = new Map<string, TraceSessionGroup>();
  for (const trace of items) {
    const sessionId = traceSessionKey(trace);
    const group = groups.get(sessionId) || {
      id: sessionId,
      label: trace.sessionId || "未知会话",
      topic: "",
      traces: [],
      turns: [],
      caseIds: [],
      latestAt: "",
    };
    group.traces.push(trace);
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
        label: trace.turnId || "未知 Turn",
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
    group.topic = group.turns.find((turn) => turn.preview)?.preview || group.label;
    group.traces.sort(compareTraceDesc);
  }

  return Array.from(groups.values()).sort(compareLatestDesc);
}

function traceSessionKey(trace: TraceItem) {
  return trace.sessionId || "unknown-session";
}

function traceTurnKey(trace: TraceItem) {
  return trace.turnId || "unknown-turn";
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
