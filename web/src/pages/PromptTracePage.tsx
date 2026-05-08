import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import { parsePromptTrace, shortHash } from "@/utils/promptTraceViewModel";

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
  relativePath?: string;
  jsonPath?: string;
  markdownPath?: string;
  diffPath?: string;
  promptFingerprint?: Record<string, string>;
};

type TraceListPayload = { traces?: TraceItem[]; rootDir?: string; selectedId?: string };
type FilePayload = { content?: string };

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

export function PromptTracePage() {
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState<TraceItem[]>([]);
  const [rootDir, setRootDir] = useState("");
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [activeView, setActiveView] = useState("overview");
  const [activeRaw, setActiveRaw] = useState("markdown");
  const [fileCache, setFileCache] = useState<Record<string, string>>({});
  const [error, setError] = useState("");

  async function loadTraces() {
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson<TraceListPayload>("/api/v1/debug/model-input-traces?limit=150");
      setTraces(payload.traces || []);
      setRootDir(payload.rootDir || "");
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

  const selectedTrace = traces.find((item) => item.id === selectedId) || null;
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

  const views = [
    ["overview", "概览"],
    ["layers", "Prompt 层"],
    ["messages", "Messages"],
    ["tools", "Tools"],
    ["diff", "Diff"],
    ["raw", "Raw"],
  ];

  return (
    <SettingsPageFrame
      title="Prompt Trace"
      description="本地模型输入 trace 查看器，保留概览、Prompt 层、Messages、Tools、Diff 和 Raw 卡片视图。"
      actions={<Button variant="outline" disabled={loading} onClick={() => void loadTraces()}>刷新</Button>}
    >
      {error ? <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div> : null}
      <div className="grid min-h-[720px] gap-4 xl:grid-cols-[340px_1fr]">
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>Trace 列表</CardTitle>
            <CardDescription title={rootDir}>{rootDir || ".data/model-input-traces"}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Session / Turn / Hash" />
            <div className="grid max-h-[600px] gap-2 overflow-auto">
              {filteredTraces.map((trace) => (
                <button key={trace.id} type="button" className={`rounded-lg border p-3 text-left text-sm ${trace.id === selectedId ? "border-slate-900 bg-slate-50" : "bg-white"}`} onClick={() => { setSelectedId(trace.id); setActiveView("overview"); }}>
                  <span className="block font-medium">{trace.sessionId || "session"}</span>
                  <span className="block text-xs text-slate-500">turn {trace.turnId || "-"} · iteration {trace.iteration ?? "-"}</span>
                  {trace.caseId ? <span className="mt-1 inline-flex rounded bg-slate-100 px-2 py-0.5 text-xs">case {trace.caseId}</span> : null}
                  <span className="block text-xs text-slate-400">{displayTime(trace.createdAt || trace.modifiedAt)}</span>
                </button>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>{selectedTrace?.relativePath || activePath || "Prompt Trace"}</CardTitle>
            <CardDescription className="flex flex-wrap gap-2">
              {selectedTrace?.promptFingerprint?.stableHash ? <ToneBadge>stable {shortHash(selectedTrace.promptFingerprint.stableHash)}</ToneBadge> : null}
              {selectedTrace?.promptFingerprint?.developerHash ? <ToneBadge>developer {shortHash(selectedTrace.promptFingerprint.developerHash)}</ToneBadge> : null}
              {selectedTrace?.promptFingerprint?.toolRegistryHash ? <ToneBadge>tools {shortHash(selectedTrace.promptFingerprint.toolRegistryHash)}</ToneBadge> : null}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <div className="flex flex-wrap gap-2">
              {views.map(([key, label]) => (
                <button key={key} type="button" className={`rounded-lg border px-3 py-2 text-sm ${activeView === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveView(key)}>{label}</button>
              ))}
            </div>

            {activeView === "overview" ? (
              <section className="grid gap-3 md:grid-cols-3">
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Messages</CardDescription><CardTitle>{traceViewModel?.summary.messageCount ?? selectedTrace?.messageCount ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Tools</CardDescription><CardTitle>{traceViewModel?.summary.visibleToolCount ?? selectedTrace?.visibleTools?.length ?? 0}</CardTitle></CardHeader></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardDescription>Prompt chars</CardDescription><CardTitle>{traceViewModel?.summary.promptCharCount ?? 0}</CardTitle></CardHeader></Card>
                <pre className="md:col-span-3 overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{JSON.stringify({ summary: traceViewModel?.summary, warnings: traceViewModel?.warnings }, null, 2)}</pre>
              </section>
            ) : null}

            {activeView === "layers" ? <section className="grid gap-3">{(traceViewModel?.layers || []).map((layer) => <Card key={layer.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{layer.title}</CardTitle><CardDescription>{layer.providerRole} / {layer.promptLayer}</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{layer.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "messages" ? <section className="grid gap-3">{(traceViewModel?.messages || []).map((message) => <Card key={message.id} className="rounded-lg bg-slate-50"><CardHeader><CardTitle>{message.providerRole || message.semanticRole || "message"}</CardTitle><CardDescription>{message.charCount} chars</CardDescription></CardHeader><CardContent><pre className="max-h-72 overflow-auto whitespace-pre-wrap text-xs">{message.content}</pre></CardContent></Card>)}</section> : null}
            {activeView === "tools" ? <section className="grid gap-3"><div className="flex flex-wrap gap-2">{(traceViewModel?.tools.visible || selectedTrace?.visibleTools || []).map((tool) => <ToneBadge key={tool}>{tool}</ToneBadge>)}</div><pre className="max-h-[520px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{traceViewModel?.tools.registryText || ""}</pre></section> : null}
            {activeView === "diff" ? <pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{diffContent || "暂无 diff 文件"}</pre> : null}
            {activeView === "raw" ? <section className="grid gap-3"><div className="flex gap-2">{["markdown", "json"].map((key) => <button key={key} type="button" className={`rounded-lg border px-3 py-1 text-sm ${activeRaw === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveRaw(key)}>{key}</button>)}</div><pre className="max-h-[640px] overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-white">{rawContent || "Loading..."}</pre></section> : null}
          </CardContent>
        </Card>
      </div>
    </SettingsPageFrame>
  );
}
