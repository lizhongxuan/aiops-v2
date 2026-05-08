import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  ArrowLeft,
  Bot,
  CheckCircle,
  FlaskConical,
  PanelRight,
  Play,
  Rocket,
  Save,
  X,
} from "lucide-react";

import { RunnerCanvas } from "@/components/runner/RunnerCanvas";
import { getNodeCanvasMeta } from "@/components/runner/nodeTypeRegistry";
import { createInitialRunState, reduceRunEvents } from "@/components/runner/runStateReducer";
import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";
import "@/components/runner/runnerStudio.css";

type RunnerAction = { action?: string; name?: string; label?: string; title?: string; category?: string; defaults?: Record<string, unknown> };
type Workflow = {
  id?: string;
  name: string;
  title?: string;
  status?: string;
  graph?: RunnerGraph | null;
  local_draft?: boolean;
  validated_graph_hash?: string;
  validatedGraphHash?: string;
  dry_run_graph_hash?: string;
  dryRunGraphHash?: string;
  validation_result?: { valid?: boolean; errors?: unknown[]; warnings?: unknown[] };
  validationResult?: { valid?: boolean; errors?: unknown[]; warnings?: unknown[] };
  risk_summary?: { level?: string; items?: unknown[] };
  riskSummary?: { level?: string; items?: unknown[] };
  diff_summary?: Record<string, unknown>;
  diffSummary?: Record<string, unknown>;
  ai_generated_draft?: boolean;
  aiGeneratedDraft?: boolean;
};

type SaveState = { status: string; message?: string; lastSavedAt?: string; error?: string };

type ApiNotice = { title: string; message: string; hint?: string } | null;

const FALLBACK_ACTIONS: RunnerAction[] = [
  { action: "cmd.run", label: "Command", category: "基础", defaults: { cmd: "uptime" } },
  { action: "shell.run", label: "Shell Script", category: "基础", defaults: { script: "set -e\ndf -h" } },
  { action: "script.shell", label: "Stored Script", category: "基础", defaults: { script_ref: "restore.sh" } },
];

const LOCAL_DRAFTS_KEY = "runner.studio.localDrafts";

function workflowKey(workflow: Partial<Workflow> | null | undefined) {
  return String(workflow?.name || workflow?.id || "").trim();
}

function normalizeGraph(graph: Partial<RunnerGraph> | null | undefined, name: string): RunnerGraph {
  return {
    version: graph?.version || "v1",
    ...(graph || {}),
    workflow: { ...(graph?.workflow || {}), name: String(graph?.workflow?.name || name) },
    nodes: Array.isArray(graph?.nodes) ? graph.nodes : [],
    edges: Array.isArray(graph?.edges) ? graph.edges : [],
  };
}

function createBlankWorkflowGraph(name: string, title = name): RunnerGraph {
  return normalizeGraph(
    {
      workflow: { name, title },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 160 }, ports: [{ id: "next", type: "output", label: "下一步" }] },
        { id: "end", type: "end", label: "End", position: { x: 720, y: 160 }, ports: [{ id: "in", type: "input", label: "输入" }] },
      ],
      edges: [],
    },
    name,
  );
}

function readLocalDrafts(): Workflow[] {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(LOCAL_DRAFTS_KEY) || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return [];
    return Object.values(parsed) as Workflow[];
  } catch (_error) {
    return [];
  }
}

function saveLocalDraft(workflow: Workflow) {
  const current = Object.fromEntries(readLocalDrafts().map((item) => [workflowKey(item), item]));
  const key = workflowKey(workflow);
  if (!key) return workflow;
  const draft = { ...workflow, id: key, name: key, local_draft: true, updated_at: new Date().toISOString() } as Workflow;
  window.localStorage.setItem(LOCAL_DRAFTS_KEY, JSON.stringify({ ...current, [key]: draft }));
  return draft;
}

async function requestJson(path: string, init: RequestInit = {}) {
  const response = await fetch(path, {
    credentials: "include",
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const error = new Error(payload?.error || payload?.message || `HTTP ${response.status}`) as Error & { status?: number; payload?: unknown; url?: string };
    error.status = response.status;
    error.payload = payload;
    error.url = path;
    throw error;
  }
  return payload;
}

function isRecoverableApiFailure(error: unknown) {
  const status = Number((error as { status?: number })?.status || 0);
  return status === 404 || status === 503;
}

function formatRunnerStudioNotice(error: unknown): ApiNotice {
  const status = Number((error as { status?: number })?.status || 0);
  if (status === 404) {
    return {
      title: "本地编排模式",
      message: "当前 ai-server 尚未接入 /api/runner-studio/*，已启用内置动作库，可先创建和编排工作流。",
      hint: "保存、校验、运行和发布需要重启最新 ai-server。",
    };
  }
  return {
    title: "本地编排模式",
    message: "Runner API upstream 尚未配置，已启用内置动作库，可先完成工作流草稿。",
    hint: "设置 Runner API upstream 后重启 ai-server。",
  };
}

function formatSaveTime(date = new Date()) {
  return date.toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function slugify(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "runner-blank";
}

function cloneNode(node: RunnerNode): RunnerNode {
  return JSON.parse(JSON.stringify(node)) as RunnerNode;
}

function saveStateLabel(saveState: SaveState) {
  if (saveState.status === "pending") return saveState.message || "未保存";
  if (saveState.status === "saving") return saveState.message || "正在保存";
  if (saveState.status === "saved") return saveState.lastSavedAt ? `已保存 ${saveState.lastSavedAt}` : saveState.message || "已保存";
  if (saveState.status === "local_draft") return saveState.message || "本地草稿，未同步";
  if (saveState.status === "blocked") return saveState.message || "操作被阻止";
  if (saveState.status === "failed" || saveState.status === "error") return saveState.message || "操作失败";
  return saveState.message || "草稿";
}

function WorkflowLibrary({ workflows, loading, onOpenManager, onSelect }: { workflows: Workflow[]; loading: boolean; onOpenManager: () => void; onSelect: (name: string) => void }) {
  return (
    <section className="runner-workflow-library" data-testid="runner-workflow-library">
      <header className="runner-workflow-library-head">
        <div>
          <p>RUNNER WORKFLOWS</p>
          <h1>工作流</h1>
        </div>
        <div className="workflow-quick-actions">
          <button type="button" className="runner-studio-action-button primary" data-testid="runner-open-manager" onClick={onOpenManager} disabled={loading}>管理工作流</button>
        </div>
      </header>
      <div className="workflow-quick-list">
        {workflows.length === 0 ? <p className="runner-studio-empty">暂无工作流，打开管理器创建一个 blank workflow。</p> : null}
        {workflows.map((workflow) => (
          <button key={workflowKey(workflow)} type="button" className="runner-studio-workflow" onClick={() => onSelect(workflowKey(workflow))}>
            <span>{workflow.title || workflow.name}</span>
            <small>{workflow.status || "draft"}</small>
          </button>
        ))}
      </div>
    </section>
  );
}

function WorkflowManagerModal({ workflows, onClose, onCreateBlank, onSelect }: { workflows: Workflow[]; onClose: () => void; onCreateBlank: (name: string) => void; onSelect: (name: string) => void }) {
  const [name, setName] = useState("runner-blank");
  return (
    <section className="workflow-manager-backdrop" data-testid="workflow-manager-modal">
      <div className="workflow-manager-modal" role="dialog" aria-modal="true" aria-label="工作流管理">
        <header className="workflow-manager-head">
          <div><p>WORKFLOW MANAGER</p><h2>工作流管理</h2></div>
          <button type="button" className="workflow-icon-button" aria-label="关闭" onClick={onClose}><X size={16} /></button>
        </header>
        <section className="workflow-create-form">
          <div><strong>创建空白工作流</strong><span>保留 visual workflow schema，生成 Start / End 节点。</span></div>
          <label>名称<input value={name} onChange={(event) => setName(event.target.value)} /></label>
          <div><button type="button" data-testid="workflow-create-blank" onClick={() => onCreateBlank(slugify(name))}>创建 blank</button></div>
        </section>
        <main className="workflow-manager-list">
          {workflows.map((workflow) => (
            <div key={workflowKey(workflow)} className="workflow-manager-row">
              <button type="button" className="workflow-manager-main" onClick={() => onSelect(workflowKey(workflow))}>
                <span>{workflow.title || workflow.name}</span><small>{workflow.status || "draft"}</small>
              </button>
            </div>
          ))}
        </main>
      </div>
    </section>
  );
}

function RunnerNodePanel({ node, graph, runState, onClose, onApply, onRunNode, onOpenRunDetails }: { node: RunnerNode; graph: RunnerGraph; runState: ReturnType<typeof createInitialRunState>; onClose: () => void; onApply: (node: RunnerNode) => void; onRunNode: (nodeId: string) => void; onOpenRunDetails: (nodeId: string) => void }) {
  const [draft, setDraft] = useState(() => cloneNode(node));
  const [tab, setTab] = useState("settings");
  useEffect(() => { setDraft(cloneNode(node)); setTab("settings"); }, [node]);
  const meta = getNodeCanvasMeta(draft);
  const runNode = runState.nodes?.[draft.id];
  const inputs = Array.isArray(draft.inputs) ? draft.inputs : [];
  const outputs = Array.isArray(draft.outputs) ? draft.outputs : [];
  const updateInput = (index: number, key: string) => setDraft({ ...draft, inputs: inputs.map((item, itemIndex) => itemIndex === index ? { ...item, key } : item) });
  const updateOutput = (index: number, key: string) => setDraft({ ...draft, outputs: outputs.map((item, itemIndex) => itemIndex === index ? { ...item, key } : item) });
  return (
    <aside className="runner-node-panel" role="complementary" aria-label="节点配置面板" data-testid="runner-node-panel">
      <header className="runner-node-panel-head">
        <div className="runner-node-panel-identity">
          <span className={`runner-node-panel-icon tone-${meta.tone}`}>{meta.iconText}</span>
          <div><p>{meta.action}</p><h2 data-testid="runner-node-panel-title">{draft.step?.name || draft.label || draft.id}</h2><span className={`runner-node-panel-status status-${runNode?.status || "not_run"}`}>{runNode?.status || "not_run"}</span></div>
        </div>
        <div className="runner-node-panel-actions">
          <button type="button" data-testid="runner-node-panel-run" onClick={() => onRunNode(draft.id)}><Play size={15} />运行</button>
          <button type="button" data-testid="runner-node-panel-open-run" onClick={() => onOpenRunDetails(draft.id)}><PanelRight size={15} />详情</button>
          <button type="button" className="primary" data-testid="runner-node-panel-apply" onClick={() => onApply(cloneNode(draft))}><CheckCircle size={15} />应用</button>
          <button type="button" aria-label="关闭节点配置" data-testid="runner-node-panel-close" onClick={onClose}><X size={16} /></button>
        </div>
      </header>
      <nav className="runner-node-panel-tabs" aria-label="节点配置页签" data-testid="runner-node-panel-tabs">
        {[
          ["settings", "设置"], ["input", "输入"], ["output", "输出"], ["advanced", "高级"], ["last-run", "上次运行"],
        ].map(([key, label]) => <button key={key} type="button" className={tab === key ? "active" : ""} data-testid={`runner-node-panel-tab-${key}`} onClick={() => setTab(key)}>{label}</button>)}
      </nav>
      <main className="runner-node-panel-body">
        {tab === "settings" ? (
          <section className="node-config-form"><label>节点名称<input value={draft.step?.name || draft.id} onChange={(event) => setDraft({ ...draft, step: { ...(draft.step || {}), name: event.target.value } })} /></label><label>动作<input value={draft.step?.action || draft.type || ""} onChange={(event) => setDraft({ ...draft, step: { ...(draft.step || {}), action: event.target.value } })} /></label></section>
        ) : null}
        {tab === "input" ? (
          <section data-testid="input-tab" className="runner-schema-editor"><button type="button" data-testid="input-add" onClick={() => setDraft({ ...draft, inputs: [...inputs, { key: `input_${inputs.length + 1}`, type: "string" }] })}>添加输入</button>{inputs.map((item, index) => { const key = String(item.key || `input_${index + 1}`); return <label key={`${key}-${index}`}>Key<input data-testid={`input-key-${key}`} value={key} onChange={(event) => updateInput(index, event.target.value)} /></label>; })}</section>
        ) : null}
        {tab === "output" ? (
          <section data-testid="output-tab" className="runner-schema-editor"><button type="button" data-testid="output-add" onClick={() => setDraft({ ...draft, outputs: [...outputs, { key: `output_${outputs.length + 1}`, type: "string" }] })}>添加输出</button>{outputs.map((item, index) => { const key = String(item.key || `output_${index + 1}`); return <label key={`${key}-${index}`}>Key<input data-testid={`output-key-${key}`} value={key} onChange={(event) => updateOutput(index, event.target.value)} /></label>; })}</section>
        ) : null}
        {tab === "advanced" ? <pre>{JSON.stringify(draft, null, 2)}</pre> : null}
        {tab === "last-run" ? <pre>{JSON.stringify(runNode || {}, null, 2)}</pre> : null}
      </main>
      <section className="runner-next-step-editor"><strong>Graph</strong><span>{graph.nodes?.length || 0} nodes · {graph.edges?.length || 0} edges</span></section>
    </aside>
  );
}

function RunnerRunPanel({ state, graph, selectedNodeId, onSelectNode }: { state: ReturnType<typeof createInitialRunState>; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const logs = state.logs || [];
  return (
    <section className="runner-run-panel" data-testid="runner-run-panel">
      <div className="publish-review-card"><h3>运行概览</h3><p>{state.runId || "尚无运行"} · {state.status}</p></div>
      <div className="publish-review-card"><h3>节点</h3>{Object.values(state.nodes || {}).map((node) => <button key={node.nodeId} type="button" className={selectedNodeId === node.nodeId ? "active" : ""} onClick={() => onSelectNode(node.nodeId)}>{node.nodeId} · {node.status}</button>)}{graph.nodes?.length ? <small>{graph.nodes.length} graph nodes</small> : null}</div>
      <div className="publish-review-card"><h3>stdout / stderr / SSE</h3>{logs.length ? logs.map((log, index) => <pre key={`${log.nodeId}-${index}`}>{log.nodeId} {log.stream}: {log.message}</pre>) : <p>暂无日志。</p>}</div>
    </section>
  );
}

function PublishReviewModal({ workflow, onClose, onPublished }: { workflow: Workflow; onClose: () => void; onPublished: (payload: Partial<Workflow>) => void }) {
  const [note, setNote] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const validation = workflow.validation_result || workflow.validationResult || { valid: false, errors: [], warnings: [] };
  const validatedHash = workflow.validated_graph_hash || workflow.validatedGraphHash || "";
  const dryHash = workflow.dry_run_graph_hash || workflow.dryRunGraphHash || "";
  const validationPassed = Boolean(validation.valid) && !validation.errors?.length;
  const disabledReason = !validatedHash ? "缺少当前 validated_graph_hash，发布前必须先校验当前 graph。" : dryHash !== validatedHash ? "Dry Run 未通过或已过期，发布前必须重新 Dry Run 当前 graph。" : !validationPassed ? "校验未通过，修复错误后才能发布。" : !note.trim() ? "发布说明不能为空。" : "";
  const canPublish = !disabledReason && !loading;
  async function publish() {
    if (!canPublish) return;
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(workflowKey(workflow))}/publish`, { method: "POST", body: JSON.stringify({ save_note: note.trim(), validated_graph_hash: validatedHash, dry_run_graph_hash: dryHash, validation_result: validation }) });
      onPublished(payload);
    } catch (cause) {
      setError((cause as Error).message || "发布失败");
    } finally {
      setLoading(false);
    }
  }
  return (
    <section className="publish-review-backdrop" data-testid="publish-review-modal">
      <div className="publish-review-modal" role="dialog" aria-modal="true" aria-label="发布审阅">
        <header className="publish-review-head"><div><p>PUBLISH REVIEW</p><h2>发布审阅</h2></div><button type="button" className="workflow-icon-button" aria-label="关闭" onClick={onClose}><X size={16} /></button></header>
        <main className="publish-review-body"><section className="publish-review-card"><h3>校验结果</h3><p>{validationPassed ? "校验通过" : "校验未通过或未提供结果"}</p></section><label className="publish-note-field"><span>发布说明</span><textarea data-testid="publish-note" value={note} placeholder="记录变更窗口、审批单或发布原因" onChange={(event) => setNote(event.target.value)} /></label>{disabledReason ? <p className="publish-review-warning">{disabledReason}</p> : null}{error ? <p className="publish-review-error" role="alert">{error}</p> : null}</main>
        <footer className="publish-review-footer"><button type="button" onClick={onClose}>取消</button><button type="button" className="primary" disabled={!canPublish} data-testid="publish-confirm" onClick={publish}><Rocket size={15} />确认发布</button></footer>
      </div>
    </section>
  );
}

function AiAssistantModal({ workflow, graph, onClose, onApply }: { workflow: Workflow; graph: RunnerGraph; onClose: () => void; onApply: (graph: RunnerGraph) => void }) {
  const [instruction, setInstruction] = useState("");
  const [draftGraph, setDraftGraph] = useState<RunnerGraph | null>(null);
  const [summary, setSummary] = useState("");
  const [error, setError] = useState("");
  async function generate() {
    setError("");
    const payload = await requestJson("/api/runner-studio/ai/draft", { method: "POST", body: JSON.stringify({ workflow, graph, instruction }) });
    const patch = payload?.graph_patch || payload?.patch || {};
    setDraftGraph(patch.graph || payload.graph || null);
    const changes = payload?.diff_summary?.semantic_changes || [];
    setSummary(changes.map((item: { title?: string; detail?: string }) => [item.title, item.detail].filter(Boolean).join(" · ")).join("\n") || "AI draft ready");
  }
  async function apply() {
    if (!draftGraph) return;
    const validation = await requestJson("/api/runner-studio/workflows/graph/validate", { method: "POST", body: JSON.stringify({ workflow_name: workflowKey(workflow), graph: draftGraph }) });
    if (validation?.valid === false) {
      setError((validation.errors || []).map((item: { message?: string }) => item.message || String(item)).join("；") || "AI patch validation failed");
      return;
    }
    onApply(normalizeGraph(draftGraph, workflowKey(workflow)));
  }
  return <section className="runner-ai-backdrop" data-testid="runner-ai-modal"><div className="runner-ai-modal" role="dialog" aria-modal="true" aria-label="Runner AI 助手"><header className="runner-ai-head"><div><p>AI RUNNER</p><h2>AI 生成工作流草稿</h2></div><button type="button" onClick={onClose}>关闭</button></header><main className="runner-ai-body"><label className="runner-ai-instruction"><span>指令</span><textarea data-testid="runner-ai-instruction" value={instruction} onChange={(event) => setInstruction(event.target.value)} /></label>{summary ? <pre>{summary}</pre> : null}{error ? <p className="runner-ai-error" role="alert">{error}</p> : null}</main><footer className="runner-ai-footer"><button type="button" data-testid="runner-ai-generate" onClick={() => void generate()}>生成</button><button type="button" className="primary" data-testid="runner-ai-apply" disabled={!draftGraph} onClick={() => void apply()}>应用</button></footer></div></section>;
}

export function RunnerStudioPage() {
  const params = useParams();
  const navigate = useNavigate();
  const routeWorkflowName = String(params.workflowName || "").trim();
  const [loading, setLoading] = useState(false);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [actions, setActions] = useState<RunnerAction[]>(FALLBACK_ACTIONS);
  const [selectedWorkflowName, setSelectedWorkflowName] = useState(routeWorkflowName);
  const [saveState, setSaveState] = useState<SaveState>({ status: "idle" });
  const [apiNotice, setApiNotice] = useState<ApiNotice>(null);
  const [apiNoticeDismissed, setApiNoticeDismissed] = useState(false);
  const [managerOpen, setManagerOpen] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState("");
  const [runDrawerOpen, setRunDrawerOpen] = useState(false);
  const [runEvents, setRunEvents] = useState<Record<string, unknown>[]>([]);
  const [publishOpen, setPublishOpen] = useState(false);
  const [aiOpen, setAiOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);

  const selectedWorkflow = useMemo(() => workflows.find((workflow) => workflowKey(workflow) === selectedWorkflowName) || null, [workflows, selectedWorkflowName]);
  const graph = normalizeGraph(selectedWorkflow?.graph || { workflow: { name: selectedWorkflowName || "draft" }, nodes: [], edges: [] }, selectedWorkflowName || "draft");
  const selectedNode = graph.nodes?.find((node) => node.id === selectedNodeId) || null;
  const runState = useMemo(() => reduceRunEvents(runEvents, createInitialRunState()), [runEvents]);
  const serverActionsDisabled = Boolean(apiNotice);

  const upsertWorkflow = useCallback((name: string, patch: Partial<Workflow>) => {
    setWorkflows((current) => {
      const existing = current.find((workflow) => workflowKey(workflow) === name);
      if (!existing) return [{ name, title: name, status: "draft", ...patch }, ...current];
      return current.map((workflow) => workflowKey(workflow) === name ? { ...workflow, ...patch } : workflow);
    });
  }, []);

  const selectWorkflow = useCallback((name: string) => {
    setSelectedWorkflowName(name);
    setSelectedNodeId("");
    setRunDrawerOpen(false);
    navigate(name ? `/runner/${encodeURIComponent(name)}` : "/runner");
  }, [navigate]);

  useEffect(() => { setSelectedWorkflowName(routeWorkflowName); }, [routeWorkflowName]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      const [workflowResult, catalogResult] = await Promise.allSettled([
        requestJson("/api/runner-studio/workflows"),
        requestJson("/api/runner-studio/actions"),
      ]);
      if (cancelled) return;
      const failures = [workflowResult, catalogResult].filter((result) => result.status === "rejected").map((result) => (result as PromiseRejectedResult).reason);
      const unrecoverable = failures.find((failure) => !isRecoverableApiFailure(failure));
      if (unrecoverable) setSaveState({ status: "error", message: "加载失败", error: (unrecoverable as Error).message });
      const remote = workflowResult.status === "fulfilled" ? (workflowResult.value.workflows || workflowResult.value.items || []) : [];
      const local = readLocalDrafts();
      const localKeys = new Set(local.map(workflowKey));
      setWorkflows([...local, ...remote.filter((workflow: Workflow) => !localKeys.has(workflowKey(workflow)))]);
      const catalog = catalogResult.status === "fulfilled" ? (catalogResult.value.items || catalogResult.value.actions || []) : [];
      setActions(catalog.length ? catalog : FALLBACK_ACTIONS);
      if (failures.length) setApiNotice(formatRunnerStudioNotice(failures[0])); else setApiNotice(null);
      setLoading(false);
    }
    void load();
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!selectedWorkflowName) return;
    let cancelled = false;
    async function ensureGraph() {
      const current = workflows.find((workflow) => workflowKey(workflow) === selectedWorkflowName);
      if (current?.graph) return;
      try {
        const payload = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(selectedWorkflowName)}/graph`);
        if (!cancelled) upsertWorkflow(selectedWorkflowName, { name: selectedWorkflowName, ...(current?.title ? { title: current.title } : {}), graph: normalizeGraph(payload.graph || payload, selectedWorkflowName), local_draft: false });
      } catch (error) {
        const graph = createBlankWorkflowGraph(selectedWorkflowName, current?.title || selectedWorkflowName);
        const draft = saveLocalDraft({ name: selectedWorkflowName, title: current?.title || selectedWorkflowName, status: "draft", graph });
        if (!cancelled) upsertWorkflow(selectedWorkflowName, draft);
      }
    }
    void ensureGraph();
    return () => { cancelled = true; };
  }, [selectedWorkflowName, upsertWorkflow, workflows]);

  function createBlankWorkflow(name: string) {
    const workflow = saveLocalDraft({ name, title: name, status: "draft", graph: createBlankWorkflowGraph(name), local_draft: true, validation_result: { valid: false, errors: [], warnings: [] } });
    upsertWorkflow(name, workflow);
    setManagerOpen(false);
    setSaveState({ status: "local_draft", message: "本地草稿", lastSavedAt: formatSaveTime() });
    selectWorkflow(name);
  }

  function updateGraph(nextGraph: RunnerGraph) {
    if (!selectedWorkflowName) return;
    const normalized = normalizeGraph(nextGraph, selectedWorkflowName);
    upsertWorkflow(selectedWorkflowName, { graph: normalized, status: "draft", validated_graph_hash: "", dry_run_graph_hash: "", validation_result: { valid: false, errors: [], warnings: [] } });
    saveLocalDraft({ ...(selectedWorkflow || { name: selectedWorkflowName }), graph: normalized, status: "draft", local_draft: true });
    setSaveState({ status: "pending", message: "未保存" });
  }

  async function flushWorkflow(reason: string) {
    if (!selectedWorkflow) return null;
    const name = workflowKey(selectedWorkflow);
    const normalized = normalizeGraph(selectedWorkflow.graph, name);
    setSaveState({ status: "saving", message: "正在保存" });
    if (serverActionsDisabled) {
      saveLocalDraft({ ...selectedWorkflow, graph: normalized });
      setSaveState({ status: "local_draft", message: "本地草稿，未同步", lastSavedAt: formatSaveTime() });
      return normalized;
    }
    const path = selectedWorkflow.local_draft ? "/api/runner-studio/workflows/graph" : `/api/runner-studio/workflows/${encodeURIComponent(name)}/graph`;
    const method = selectedWorkflow.local_draft ? "POST" : "PUT";
    const payload = await requestJson(path, { method, body: JSON.stringify({ graph: normalized, save_note: reason }) });
    const saved = payload.data || payload;
    const savedGraph = normalizeGraph(saved.graph || normalized, name);
    upsertWorkflow(name, { ...saved, graph: savedGraph, local_draft: false, status: saved.status || selectedWorkflow.status || "draft" });
    setSaveState({ status: "saved", message: "已保存", lastSavedAt: formatSaveTime() });
    return savedGraph;
  }

  async function validateWorkflow() {
    if (!selectedWorkflow) return;
    const name = workflowKey(selectedWorkflow);
    try {
      await flushWorkflow("validate");
      if (serverActionsDisabled) return;
      const result = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(name)}/validate`, { method: "POST", body: JSON.stringify({}) });
      const validation = result.data || result;
      upsertWorkflow(name, { status: validation.status || (validation.valid ? "validated" : "draft"), validated_graph_hash: validation.validated_graph_hash || validation.validatedGraphHash || "", dry_run_graph_hash: "", validation_result: validation });
      setSaveState({ status: "saved", message: "校验通过", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "校验失败", error: (error as Error).message });
    }
  }

  async function dryRunWorkflow() {
    if (!selectedWorkflow) return;
    const name = workflowKey(selectedWorkflow);
    try {
      const savedGraph = await flushWorkflow("dry-run");
      if (!savedGraph || serverActionsDisabled) return;
      const validationResult = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(name)}/validate`, { method: "POST", body: JSON.stringify({}) });
      const validation = validationResult.data || validationResult;
      const dryRunResult = await requestJson("/api/runner-studio/workflows/graph/dry-run", { method: "POST", body: JSON.stringify({ workflow_name: name, graph: savedGraph, vars: {}, triggered_by: "ui" }) });
      const dryRun = dryRunResult.data || dryRunResult;
      upsertWorkflow(name, { status: dryRun.status || "dry_run_passed", validated_graph_hash: dryRun.validated_graph_hash || validation.validated_graph_hash || "", dry_run_graph_hash: dryRun.dry_run_graph_hash || dryRun.validated_graph_hash || validation.validated_graph_hash || "", validation_result: validation });
      setSaveState({ status: "saved", message: "Dry Run 通过", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "Dry Run 失败", error: (error as Error).message });
    }
  }

  async function runWorkflow(nodeId = "") {
    if (!selectedWorkflow) return;
    const name = workflowKey(selectedWorkflow);
    try {
      const savedGraph = await flushWorkflow("run");
      if (!savedGraph || serverActionsDisabled) return;
      const response = await requestJson("/api/runner-studio/runs", { method: "POST", body: JSON.stringify({ workflow_name: name, graph: savedGraph, vars: {}, triggered_by: "ui", risk_acknowledged: true, ...(nodeId ? { node_id: nodeId, run_scope: "single_node" } : {}) }) });
      const runId = response.run_id || response.runId || "";
      const history = await requestJson(`/api/runner-studio/runs/${encodeURIComponent(runId)}/events/history`);
      const events = Array.isArray(history) ? history : history.items || history.events || [];
      setRunEvents(events.map((event: Record<string, unknown>) => ({ ...event, run_id: event.run_id || runId })));
      setSaveState({ status: "saved", message: "运行已提交", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "运行失败", error: (error as Error).message });
    }
  }

  const toolbarActions = [
    ["save", "保存", Save], ["validate", "校验", CheckCircle], ["dry-run", "Dry Run", FlaskConical], ["run", "运行", Play], ["run-details", "运行详情", PanelRight], ["publish", "发布", Rocket], ["ai-generate", "AI 生成", Bot],
  ] as const;

  return (
    <section className="runner-studio-page" data-testid="runner-studio-page">
      {apiNotice && !apiNoticeDismissed ? <section className="runner-studio-api-notice" data-testid="runner-studio-api-notice"><strong>{apiNotice.title}</strong><span>{apiNotice.message} {apiNotice.hint}</span><button type="button" data-testid="runner-api-notice-close" onClick={() => setApiNoticeDismissed(true)}>关闭</button></section> : null}
      <section className={`runner-studio-shell ${fullscreen ? "fullscreen" : ""}`} data-testid="runner-studio-shell" aria-busy={loading ? "true" : "false"}>
        {!selectedWorkflow ? <WorkflowLibrary workflows={workflows} loading={loading} onOpenManager={() => setManagerOpen(true)} onSelect={selectWorkflow} /> : (
          <>
            <header className="runner-studio-topbar" data-testid="runner-studio-topbar">
              <div className="runner-studio-current-workflow"><button type="button" className="runner-studio-back-button" data-testid="runner-back-to-library" onClick={() => selectWorkflow("")}><ArrowLeft size={15} />工作流</button><h1>{selectedWorkflow.title || selectedWorkflow.name}</h1><span className="runner-studio-status">{selectedWorkflow.status || "draft"}</span><span className={`runner-studio-save-state status-${saveState.status || "idle"}`} data-testid="runner-save-state">{saveStateLabel(saveState)}</span></div>
              <div className="runner-studio-toolbar-actions" aria-label="Runner Studio 操作">{toolbarActions.map(([key, label, Icon]) => <button key={key} type="button" className={`runner-studio-action-button ${key === "run" ? "primary" : ""}`} data-testid={`runner-toolbar-${key}`} onClick={() => { if (key === "save") void flushWorkflow("manual"); if (key === "validate") void validateWorkflow(); if (key === "dry-run") void dryRunWorkflow(); if (key === "run") void runWorkflow(); if (key === "run-details") setRunDrawerOpen(true); if (key === "publish") setPublishOpen(true); if (key === "ai-generate") setAiOpen(true); }}><Icon size={15} />{label}</button>)}</div>
            </header>
            <div className={`runner-studio-workspace ${selectedNode ? "with-node-panel" : ""}`}>
              <section className="runner-studio-main"><section className="runner-studio-canvas" aria-label="工作流画布" data-testid="runner-studio-canvas"><RunnerCanvas graph={graph} actions={actions} selectedNodeId={selectedNodeId} fullscreen={fullscreen} onUpdateGraph={updateGraph} onSelectNode={setSelectedNodeId} onOpenNodeConfig={setSelectedNodeId} onNodeAction={() => {}} onToggleFullscreen={() => setFullscreen((value) => !value)} /></section></section>
              {selectedNode ? <RunnerNodePanel node={selectedNode} graph={graph} runState={runState} onClose={() => setSelectedNodeId("")} onApply={(node) => { updateGraph({ ...graph, nodes: (graph.nodes || []).map((item) => item.id === node.id ? node : item) }); setSelectedNodeId(node.id); }} onRunNode={(nodeId) => void runWorkflow(nodeId)} onOpenRunDetails={(nodeId) => { setSelectedNodeId(nodeId); setRunDrawerOpen(true); }} /> : null}
            </div>
          </>
        )}
        {selectedWorkflow && runDrawerOpen ? <section className="runner-studio-run-drawer-backdrop" role="dialog" aria-modal="true" aria-label="运行详情" data-testid="runner-run-drawer"><aside className="runner-studio-run-drawer-panel"><header className="runner-studio-run-drawer-head"><div><strong>运行详情</strong><span>stdout、stderr、SSE、审批事件、变量和最近节点结果</span></div><button type="button" className="runner-run-drawer-close" data-testid="runner-run-drawer-close" aria-label="关闭运行详情" onClick={() => setRunDrawerOpen(false)}><X size={18} /></button></header><div className="runner-studio-run-drawer-body"><RunnerRunPanel state={runState} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} /></div></aside></section> : null}
      </section>
      {managerOpen ? <WorkflowManagerModal workflows={workflows} onClose={() => setManagerOpen(false)} onCreateBlank={createBlankWorkflow} onSelect={(name) => { setManagerOpen(false); selectWorkflow(name); }} /> : null}
      {publishOpen && selectedWorkflow ? <PublishReviewModal workflow={selectedWorkflow} onClose={() => setPublishOpen(false)} onPublished={(payload) => { upsertWorkflow(selectedWorkflowName, { ...payload, status: payload.status || "published" }); setPublishOpen(false); }} /> : null}
      {aiOpen && selectedWorkflow ? <AiAssistantModal workflow={selectedWorkflow} graph={graph} onClose={() => setAiOpen(false)} onApply={(nextGraph) => { updateGraph(nextGraph); setAiOpen(false); }} /> : null}
    </section>
  );
}
