import { useEffect, useMemo, useState } from "react";
import type { CSSProperties } from "react";

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
  userPromptPreview?: string;
  relativePath?: string;
  jsonPath?: string;
  markdownPath?: string;
  diffPath?: string;
  promptFingerprint?: Record<string, string>;
};

type TraceListPayload = { traces?: TraceItem[]; rootDir?: string; selectedId?: string };
type FilePayload = { content?: string };
type AgentUiSourceArtifact = {
  id: string;
  artifactId: string;
  type: string;
  title: string;
  evidenceRef?: string;
  caseId?: string;
  redactionStatus?: string;
  redactionStatusLabel?: string;
  generatedBy?: {
    kind?: string;
    id?: string;
    name?: string;
    label?: string;
    llmRequestId?: string;
  };
};
type AgentUiSourceToolCall = { id: string; name?: string };
type AgentUiSourceLlmRequest = {
  id: string;
  label: string;
  detail?: {
    systemPrompt?: string;
    developerPrompt?: string;
    userPrompt?: string;
    toolMessages?: string;
    retrievalContext?: string;
    output?: string;
    error?: string;
    tokens?: string;
    duration?: string;
  };
  toolCalls: AgentUiSourceToolCall[];
  generatedArtifacts: AgentUiSourceArtifact[];
};
type AgentUiSourceUserRequest = {
  id: string;
  turnId?: string;
  title: string;
  content?: string;
  preview?: string;
  llmRequests: AgentUiSourceLlmRequest[];
};
type AgentUiSources = {
  session?: { id?: string; caseId?: string };
  summary?: { artifactCount?: number; userRequestCount?: number; llmRequestCount?: number };
  userRequests?: AgentUiSourceUserRequest[];
};

const ARTIFACT_TYPE_LABELS: Record<string, string> = {
  coroot_chart: "Coroot 图表",
  trace_summary: "Trace 摘要",
  topology_slice: "拓扑片段",
  workflow_result: "Workflow 结果",
  verification_result: "验证结果",
  experience_match: "经验命中",
  unsupported: "暂不支持",
};

const TRACE_LIST_CARD_CLASS = "h-28 min-h-28 max-h-28 shrink-0 overflow-hidden rounded-lg border p-3 text-left text-sm";
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

function artifactTypeLabel(type = "") {
  return ARTIFACT_TYPE_LABELS[type] || type || "未知类型";
}

function compactTraceLabel(value = "", maxLength = 30) {
  const text = String(value || "");
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength - 3)}...`;
}

function sessionCardTitle(group: TraceSessionGroup) {
  return [
    group.label,
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

function llmCardTitle(trace: TraceItem, index: number) {
  return [
    `LLM 请求 ${index + 1}`,
    `iteration ${trace.iteration ?? "-"}`,
    trace.relativePath || trace.id,
    trace.promptFingerprint?.stableHash ? `stable ${trace.promptFingerprint.stableHash}` : "",
    trace.visibleTools?.length ? `工具 ${trace.visibleTools.join("，")}` : "",
  ].filter(Boolean).join("\n");
}

export function PromptTracePage() {
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState<TraceItem[]>([]);
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [activeView, setActiveView] = useState("sources");
  const [activeRaw, setActiveRaw] = useState("markdown");
  const [fileCache, setFileCache] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [detailOpen, setDetailOpen] = useState(false);

  async function loadTraces() {
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson<TraceListPayload>("/api/v1/debug/model-input-traces?limit=150");
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
    ["sources", "来源"],
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
      description="按会话、用户请求和 LLM 请求查看本次对话的 Prompt、工具和 Agent-to-UI 来源。"
    >
      {error ? <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div> : null}
      <div
        data-testid="prompt-trace-scroll"
        className="h-[calc(100vh-8rem)] min-h-[560px] overflow-x-auto overflow-y-hidden pb-2"
      >
        <div
          data-testid="prompt-trace-layout"
          className="grid h-full min-w-[900px] grid-cols-[minmax(220px,280px)_minmax(260px,340px)_minmax(360px,1fr)] gap-4 overflow-hidden"
        >
        <Card className="flex min-h-0 min-w-0 flex-col rounded-lg bg-white">
          <CardHeader>
            <CardTitle>会话列表</CardTitle>
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
                  onClick={() => { setSelectedId(group.traces[0]?.id || ""); setActiveView("sources"); }}
                >
                  <span data-testid="prompt-trace-session-title" className="block line-clamp-2 break-all font-medium leading-5" style={TWO_LINE_CLAMP_STYLE}>{group.label}</span>
                  <span className="mt-1 block truncate text-xs text-slate-500">最近 {displayTime(group.latestAt)}</span>
                  <span className="mt-2 flex flex-wrap gap-2">
                    <ToneBadge>用户请求 {group.turns.length}</ToneBadge>
                    <ToneBadge>LLM 请求 {group.traces.length}</ToneBadge>
                  </span>
                  {group.caseIds.length ? <span className="mt-1 block truncate text-xs text-slate-500" title={`Case ${group.caseIds.join("，")}`}>Case {group.caseIds.join("，")}</span> : null}
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
              return (
                <button
                  key={turn.id}
                  type="button"
                  data-testid="prompt-trace-turn-card"
                  title={turnCardTitle(turn, preview)}
                  className={`${TRACE_LIST_CARD_CLASS} ${turn.id === selectedTurnGroup?.id ? TRACE_LIST_CARD_ACTIVE_CLASS : TRACE_LIST_CARD_IDLE_CLASS}`}
                  onClick={() => { setSelectedId(turn.traces[0]?.id || ""); setActiveView("sources"); }}
                >
                  <div data-testid="prompt-trace-turn-preview" className="line-clamp-2 overflow-hidden text-sm font-medium leading-5 text-slate-950" style={TWO_LINE_CLAMP_STYLE}>{preview}</div>
                  <div className="mt-2 flex min-w-0 flex-wrap gap-2 overflow-hidden">
                    <ToneBadge><span className="block max-w-[180px] truncate">{compactTraceLabel(turn.label)}</span></ToneBadge>
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
              {(selectedTurnGroup?.traces || []).map((trace, index) => (
                <button
                  key={trace.id}
                  type="button"
                  data-testid="prompt-trace-llm-card"
                  title={llmCardTitle(trace, index)}
                  className={`${TRACE_LIST_CARD_CLASS} min-w-0 ${trace.id === selectedTrace?.id ? TRACE_LIST_CARD_ACTIVE_CLASS : TRACE_LIST_CARD_IDLE_CLASS}`}
                  onClick={() => { setSelectedId(trace.id); setActiveView("sources"); setDetailOpen(true); }}
                >
                  <span className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-medium">LLM 请求 {index + 1}</span>
                    <ToneBadge>iteration {trace.iteration ?? "-"}</ToneBadge>
                  </span>
                  <span data-testid="prompt-trace-llm-path" className="mt-2 block max-w-full truncate font-mono text-xs text-slate-500" title={trace.relativePath || trace.id}>{trace.relativePath || trace.id}</span>
                  <span className="mt-2 flex min-w-0 flex-wrap gap-2 overflow-hidden">
                    {trace.promptFingerprint?.stableHash ? <ToneBadge>stable {shortHash(trace.promptFingerprint.stableHash)}</ToneBadge> : null}
                    {trace.visibleTools?.length ? <ToneBadge>工具 {trace.visibleTools.length}</ToneBadge> : null}
                    <ToneBadge>查看详情</ToneBadge>
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
            {activeView === "sources" ? <AgentUiSourcesView sources={traceViewModel?.agentUiSources as AgentUiSources | undefined} /> : null}
            {activeView === "overview" ? (
              <section className="grid gap-3 md:grid-cols-3">
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Messages</CardDescription><CardTitle>{traceViewModel?.summary.messageCount ?? selectedTraceMessageCount ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Tools</CardDescription><CardTitle>{traceViewModel?.summary.visibleToolCount ?? selectedTraceVisibleTools?.length ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Prompt chars</CardDescription><CardTitle>{traceViewModel?.summary.promptCharCount ?? 0}</CardTitle></CardHeader></Card>
                <pre className="md:col-span-3 overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{JSON.stringify({ summary: traceViewModel?.summary, warnings: traceViewModel?.warnings }, null, 2)}</pre>
              </section>
            ) : null}

            {activeView === "layers" ? <section className="grid gap-3">{(traceViewModel?.layers || []).map((layer) => <Card key={layer.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{layer.title}</CardTitle><CardDescription>{layer.providerRole} / {layer.promptLayer}</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{layer.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "messages" ? <section className="grid gap-3">{(traceViewModel?.messages || []).map((message) => <Card key={message.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{message.providerRole || message.semanticRole || "message"}</CardTitle><CardDescription>{message.charCount} chars</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{message.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "tools" ? <section className="grid gap-3"><div className="flex flex-wrap gap-2">{(traceViewModel?.tools.visible || selectedTraceVisibleTools || []).map((tool) => <ToneBadge key={tool}>{tool}</ToneBadge>)}</div><pre className="max-h-[520px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{traceViewModel?.tools.registryText || ""}</pre></section> : null}
            {activeView === "diff" ? <pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{redactSensitiveText(diffContent) || "暂无 diff 文件"}</pre> : null}
            {activeView === "raw" ? <section className="grid gap-3"><div className="flex gap-2">{["markdown", "json"].map((key) => <button key={key} type="button" className={`rounded-lg border px-3 py-1 text-sm ${activeRaw === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveRaw(key)}>{key}</button>)}</div><pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{redactSensitiveText(rawContent) || "Loading..."}</pre></section> : null}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function AgentUiSourcesView({ sources }: { sources?: AgentUiSources | null }) {
  const userRequests = sources?.userRequests || [];
  const summary = sources?.summary || {};
  const session = sources?.session || {};
  const selectedUserRequest = userRequests[0] || null;
  const selectedLlmRequests = selectedUserRequest?.llmRequests || [];
  const selectedLlmRequest = selectedLlmRequests[0] || null;

  return (
    <section className="grid gap-3">
      <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-slate-950">Agent-to-UI 来源</h2>
            <p className="mt-1 text-sm text-slate-600">会话 → 用户请求 → LLM 请求 → Artifact</p>
          </div>
          <div className="flex flex-wrap gap-2 text-xs">
            <ToneBadge>会话 {session.id || "-"}</ToneBadge>
            {session.caseId ? <ToneBadge>Case {session.caseId}</ToneBadge> : null}
            <ToneBadge>Artifact {summary.artifactCount ?? 0}</ToneBadge>
          </div>
        </div>
      </div>

      {userRequests.length ? (
        <div className="grid gap-3">
          <section className="rounded-lg border border-slate-200 bg-white p-4">
            <h3 className="text-sm font-semibold text-slate-950">当前链路</h3>
            <div className="mt-3 grid gap-3 md:grid-cols-3">
              <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm">
                <div className="text-xs font-medium text-slate-500">会话</div>
                <div className="mt-1 break-all font-medium text-slate-950">{session.id || "-"}</div>
              </div>
              <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm">
                <div className="text-xs font-medium text-slate-500">用户请求</div>
                <div className="mt-1 line-clamp-3 font-medium text-slate-950">{selectedUserRequest?.content || selectedUserRequest?.preview || selectedUserRequest?.turnId || "-"}</div>
              </div>
              <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm">
                <div className="text-xs font-medium text-slate-500">LLM 请求</div>
                <div className="mt-1 break-all font-mono text-slate-950">{selectedLlmRequest?.label || "-"}</div>
              </div>
            </div>
          </section>

          {selectedLlmRequest ? <LlmRequestDetail request={selectedLlmRequest} /> : <div className="rounded-lg border border-dashed border-slate-200 p-4 text-sm text-slate-500">本次 LLM 请求没有生成 Agent-to-UI Artifact；Prompt 内容请切换到 Prompt 层、消息或 Raw 查看。</div>}
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-slate-200 bg-white p-6 text-sm text-slate-500">
          暂无 Agent-to-UI Artifact 来源。
        </div>
      )}
    </section>
  );
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
    group.turns = Array.from(turns.values()).sort(compareLatestDesc);
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

function LlmRequestDetail({ request }: { request: AgentUiSourceLlmRequest }) {
  const detail = request.detail || {};
  const fields = [
    ["System Prompt", detail.systemPrompt],
    ["Developer Prompt", detail.developerPrompt],
    ["User Prompt", detail.userPrompt],
    ["Tool Messages", detail.toolMessages],
    ["Retrieval Context", detail.retrievalContext],
    ["输出", detail.output],
    ["错误", detail.error],
    ["Token", detail.tokens],
    ["耗时", detail.duration],
  ];

  return (
    <div className="grid gap-3">
      <div className="grid gap-2">
        {fields.map(([label, value]) => (
          <section key={label} className="rounded-lg border border-slate-200 bg-slate-50 p-3">
            <div className="text-xs font-medium text-slate-500">{label}</div>
            <pre className="mt-1 max-h-36 overflow-auto whitespace-pre-wrap break-words text-xs text-slate-900">{value || "暂无"}</pre>
          </section>
        ))}
      </div>

      <div className="grid gap-2">
        {request.generatedArtifacts.map((artifact) => (
          <article key={artifact.id || artifact.artifactId} className="rounded-lg border border-slate-200 bg-white p-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="font-medium text-slate-950">{artifact.title || artifact.artifactId}</div>
                <div className="mt-1 break-all font-mono text-xs text-slate-500">{artifact.artifactId}</div>
              </div>
              <div className="flex flex-wrap gap-2">
                <ToneBadge>{artifactTypeLabel(artifact.type)}</ToneBadge>
                {artifact.redactionStatus || artifact.redactionStatusLabel ? <ToneBadge>{artifact.redactionStatusLabel || artifact.redactionStatus}</ToneBadge> : null}
              </div>
            </div>
            <dl className="mt-3 grid gap-2 text-xs text-slate-600 md:grid-cols-2">
              <div>
                <dt className="font-medium text-slate-500">生成来源</dt>
                <dd>{artifact.generatedBy?.label || artifact.generatedBy?.id || "LLM 请求"}</dd>
              </div>
              {artifact.evidenceRef ? (
                <div>
                  <dt className="font-medium text-slate-500">EvidenceRef</dt>
                  <dd>EvidenceRef {artifact.evidenceRef}</dd>
                </div>
              ) : null}
              {artifact.caseId ? (
                <div>
                  <dt className="font-medium text-slate-500">Case</dt>
                  <dd>Case {artifact.caseId}</dd>
                </div>
              ) : null}
              {artifact.redactionStatus ? (
                <div>
                  <dt className="font-medium text-slate-500">脱敏状态</dt>
                  <dd>{artifact.redactionStatusLabel || artifact.redactionStatus}</dd>
                </div>
              ) : null}
            </dl>
          </article>
        ))}
      </div>
    </div>
  );
}
