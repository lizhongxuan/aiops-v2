import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  ArrowLeft,
  BookOpen,
  Bot,
  CheckCircle,
  Database,
  Download,
  FlaskConical,
  Maximize2,
  MoreHorizontal,
  PanelRight,
  Play,
  Plus,
  Rocket,
  Save,
  Trash2,
  Upload,
  X,
} from "lucide-react";

import { useRegisterAppShellHeader, useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import { RunnerCanvas } from "@/components/runner/RunnerCanvas";
import { createInputParam, normalizeInputParams, valueSourceLabel, variableToValueSource } from "@/components/runner/io/ioTypes";
import { createOutputParam, normalizeOutputParams } from "@/components/runner/io/outputTypes";
import { FALLBACK_RUNNER_ACTIONS } from "@/components/runner/fallbackActionCatalog";
import { getNodeCanvasMeta } from "@/components/runner/nodeTypeRegistry";
import { collectRunnerVariables } from "@/components/runner/runnerVariables";
import { firstRunnableNodeId } from "@/components/runner/runnerRunVisualState";
import { extractRunnerRunEvents, finalRunnerRunStatus, isRunnerRunHistoryTerminal, mapRunnerRunEventsToGraph, unwrapRunnerPayload } from "@/components/runner/runEventHistory";
import { createInitialRunState, reduceRunEvents } from "@/components/runner/runStateReducer";
import { removeGraphNode, type RunnerEdge, type RunnerGraph, type RunnerNode } from "@/components/runner/canvasGraphAdapter";
import "@/components/runner/runnerStudio.css";

type RunnerAction = { action?: string; name?: string; label?: string; title?: string; category?: string; defaults?: Record<string, unknown>; [key: string]: unknown };
type Workflow = {
  id?: string;
  name: string;
  title?: string;
  status?: string;
  workflow_type?: string;
  workflowType?: string;
  case_id?: string;
  caseId?: string;
  host_profile_snapshot?: Record<string, unknown>;
  hostProfileSnapshot?: Record<string, unknown>;
  host_profile_snapshots?: Record<string, unknown>[];
  hostProfileSnapshots?: Record<string, unknown>[];
  host_lease?: Record<string, unknown>;
  hostLease?: Record<string, unknown>;
  host_leases?: Record<string, unknown>[];
  hostLeases?: Record<string, unknown>[];
  experience_pack_binding?: Record<string, unknown>;
  experiencePackBinding?: Record<string, unknown>;
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

type RunState = ReturnType<typeof createInitialRunState>;
type RunnerRunRecord = {
  runId: string;
  status: string;
  message: string;
  startedAt: string;
  finishedAt: string;
  caseId?: string;
  hostLeaseId?: string;
  failedStep?: string;
  failedReason?: string;
  rollbackResult?: string;
  verificationRefs?: string[];
  events: Record<string, unknown>[];
  state: RunState;
};
type RunnerHostGroup = { label: string; hosts: string[] };
type RunnerEndCallback = { event: string; url: string; payload: string };

type ToolbarActionKey = "save" | "validate" | "dry-run" | "run" | "variables" | "run-details" | "publish" | "ai-generate" | "ops-manual" | "import" | "export";

const FALLBACK_ACTIONS: RunnerAction[] = FALLBACK_RUNNER_ACTIONS
  .filter((action) => action.action !== "wait.event")
  .map((action) => {
    if (action.action === "condition.branch") return { ...action, action: "condition.evaluate" };
    if (action.action === "approval.wait") return { ...action, action: "manual.approval" };
    return action;
  });

const LOCAL_DRAFTS_KEY = "runner.studio.localDrafts";

const PRIMARY_TOOLBAR_ACTIONS = [
  ["save", "保存", Save],
  ["run", "运行", Play],
  ["run-details", "运行详情", PanelRight],
] as const;

const SECONDARY_TOOLBAR_ACTIONS = [
  ["ops-manual", "生成运维手册", BookOpen],
  ["import", "导入", Upload],
  ["export", "导出", Download],
  ["validate", "校验", CheckCircle],
  ["dry-run", "Dry Run", FlaskConical],
  ["variables", "变量", Database],
  ["publish", "发布", Rocket],
  ["ai-generate", "AI 生成", Bot],
] as const;

const RUN_SUBMIT_COOLDOWN_MS = 8000;
const WORKFLOW_EXPORT_KIND = "aiops.runner.workflow";
const WORKFLOW_EXPORT_VERSION = 1;
const IMPORT_LAYOUT_START_X = 80;
const IMPORT_LAYOUT_START_Y = 160;
const IMPORT_LAYOUT_COLUMN_GAP = 320;
const IMPORT_LAYOUT_ROW_GAP = 140;
const WORKFLOW_EXPORT_KEYS = ["name", "title", "description", "workflow_type", "workflowType", "category", "inventory", "inputs", "outputs", "variables", "vars"];
const NODE_EXPORT_KEYS = ["id", "type", "label", "description", "ports", "step", "inputs", "outputs", "risk", "ui", "condition", "aggregator", "branches"];

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
      workflow: {
        name,
        title,
        inventory: {
          groups: { local: { hosts: ["local"] } },
          hosts: { local: { address: "local" } },
        },
      },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 160 }, ports: [{ id: "next", type: "output", label: "下一步" }], ui: { host_groups: DEFAULT_HOST_GROUPS } },
        { id: "end", type: "end", label: "End", position: { x: 720, y: 160 }, ports: [{ id: "in", type: "input", label: "输入" }], ui: { callbacks: [] } },
      ],
      edges: [{ id: "start-end", source: "start", source_port: "next", target: "end", target_port: "in", kind: "next" }],
    },
    name,
  );
}

function compactJsonValue(value: unknown): unknown {
  if (value === undefined || value === null || value === "") return undefined;
  if (Array.isArray(value)) {
    const items = value.map((item) => compactJsonValue(item)).filter((item) => item !== undefined);
    return items.length ? items : undefined;
  }
  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>)
      .map(([key, item]) => [key, compactJsonValue(item)] as const)
      .filter(([, item]) => item !== undefined);
    return entries.length ? Object.fromEntries(entries) : undefined;
  }
  return value;
}

function cloneCompactValue(value: unknown) {
  if (value === undefined) return undefined;
  try {
    return compactJsonValue(JSON.parse(JSON.stringify(value)));
  } catch (_error) {
    return undefined;
  }
}

function pickCompactRecord(source: Record<string, unknown>, keys: string[]) {
  const result: Record<string, unknown> = {};
  for (const key of keys) {
    const value = cloneCompactValue(source[key]);
    if (value !== undefined) result[key] = value;
  }
  return result;
}

function compactWorkflowNode(node: RunnerNode) {
  const compact = pickCompactRecord(node, NODE_EXPORT_KEYS);
  compact.id = String(node.id || "").trim();
  return cloneCompactValue(compact) as Record<string, unknown> | undefined;
}

function compactWorkflowEdge(edge: RunnerEdge) {
  const source = String(edge.source || "").trim();
  const target = String(edge.target || "").trim();
  if (!source || !target) return undefined;
  return cloneCompactValue({
    source,
    source_port: edge.source_port || edge.sourceHandle || edge.kind || "next",
    target,
    target_port: edge.target_port || edge.targetHandle || "in",
    kind: edge.kind || edge.source_port || edge.sourceHandle || "next",
  }) as Record<string, unknown> | undefined;
}

function workflowExportPayload(workflow: Workflow, graph: RunnerGraph) {
  const name = workflowKey(workflow);
  const graphWorkflow = objectValue(graph.workflow);
  const workflowInfo = pickCompactRecord({ ...graphWorkflow, name, title: graphWorkflow.title || workflow.title || name }, WORKFLOW_EXPORT_KEYS);
  return {
    kind: WORKFLOW_EXPORT_KIND,
    version: WORKFLOW_EXPORT_VERSION,
    workflow: workflowInfo,
    nodes: (graph.nodes || []).map(compactWorkflowNode).filter(Boolean),
    edges: (graph.edges || []).map(compactWorkflowEdge).filter(Boolean),
  };
}

function importedNodeRank(node: RunnerNode, indexById: Map<string, number>) {
  const type = String(node.type || "").toLowerCase();
  if (type === "start" || node.id === "start") return -10000;
  if (type === "end" || node.id === "end") return 10000;
  return indexById.get(node.id) || 0;
}

function layoutImportedWorkflowGraph(graph: RunnerGraph): RunnerGraph {
  const nodes = graph.nodes || [];
  if (!nodes.length) return graph;
  const nodeIds = new Set(nodes.map((node) => node.id));
  const indexById = new Map(nodes.map((node, index) => [node.id, index]));
  const incoming = new Map(nodes.map((node) => [node.id, [] as string[]]));
  for (const edge of graph.edges || []) {
    const source = String(edge.source || "");
    const target = String(edge.target || "");
    if (!nodeIds.has(source) || !nodeIds.has(target)) continue;
    incoming.get(target)?.push(source);
  }
  const depthMemo = new Map<string, number>();
  const depthForNode = (nodeId: string, visiting = new Set<string>()): number => {
    if (depthMemo.has(nodeId)) return depthMemo.get(nodeId) || 0;
    if (visiting.has(nodeId)) return 0;
    visiting.add(nodeId);
    const depth = (incoming.get(nodeId) || []).reduce((maxDepth, sourceId) => Math.max(maxDepth, depthForNode(sourceId, visiting) + 1), 0);
    visiting.delete(nodeId);
    depthMemo.set(nodeId, depth);
    return depth;
  };
  const layers = new Map(nodes.map((node) => [node.id, depthForNode(node.id)]));
  const layerRows = new Map<number, RunnerNode[]>();
  for (const node of nodes) {
    const layer = layers.get(node.id) || 0;
    layerRows.set(layer, [...(layerRows.get(layer) || []), node]);
  }
  for (const [layer, layerNodes] of layerRows) {
    layerRows.set(layer, [...layerNodes].sort((a, b) => importedNodeRank(a, indexById) - importedNodeRank(b, indexById)));
  }
  const rowById = new Map<string, number>();
  for (const layerNodes of layerRows.values()) layerNodes.forEach((node, row) => rowById.set(node.id, row));
  return {
    ...graph,
    layout: undefined,
    nodes: nodes.map((node) => ({
      ...node,
      position: {
        x: IMPORT_LAYOUT_START_X + (layers.get(node.id) || 0) * IMPORT_LAYOUT_COLUMN_GAP,
        y: IMPORT_LAYOUT_START_Y + (rowById.get(node.id) || 0) * IMPORT_LAYOUT_ROW_GAP,
      },
    })),
  };
}

function importedWorkflowNode(value: unknown, index: number): RunnerNode {
  const source = objectValue(value);
  const compact = pickCompactRecord(source, NODE_EXPORT_KEYS);
  const id = String(compact.id || `node-${index + 1}`).trim();
  return { ...compact, id } as RunnerNode;
}

function importedWorkflowEdge(value: unknown, index: number, nodeIds: Set<string>): RunnerEdge | null {
  const source = objectValue(value);
  const edgeSource = String(source.source || "").trim();
  const edgeTarget = String(source.target || "").trim();
  if (!edgeSource || !edgeTarget || !nodeIds.has(edgeSource) || !nodeIds.has(edgeTarget)) return null;
  const kind = String(source.kind || source.source_port || source.sourceHandle || "next");
  return {
    id: `${edgeSource}-${edgeTarget}-${kind}-${index + 1}`,
    source: edgeSource,
    source_port: String(source.source_port || source.sourceHandle || kind || "next"),
    target: edgeTarget,
    target_port: String(source.target_port || source.targetHandle || "in"),
    kind,
  };
}

function workflowImportPayloadToGraph(payload: unknown, currentName: string): RunnerGraph {
  const record = objectValue(payload);
  const nestedGraph = objectValue(record.graph);
  const workflowSource = objectValue(record.workflow || nestedGraph.workflow);
  const nodesSource = Array.isArray(record.nodes) ? record.nodes : Array.isArray(nestedGraph.nodes) ? nestedGraph.nodes : [];
  const edgesSource = Array.isArray(record.edges) ? record.edges : Array.isArray(nestedGraph.edges) ? nestedGraph.edges : [];
  if (!nodesSource.length) throw new Error("导入失败：JSON 中没有 nodes。");
  const nodes = nodesSource.map(importedWorkflowNode).filter((node) => node.id);
  const nodeIds = new Set(nodes.map((node) => node.id));
  const edges = edgesSource.map((edge, index) => importedWorkflowEdge(edge, index, nodeIds)).filter(Boolean) as RunnerEdge[];
  const workflowInfo = pickCompactRecord({ ...workflowSource, name: currentName }, WORKFLOW_EXPORT_KEYS);
  return layoutImportedWorkflowGraph(normalizeGraph({ version: "v1", workflow: { ...workflowInfo, name: currentName }, nodes, edges }, currentName));
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

function deleteLocalDraft(name: string) {
  const key = String(name || "").trim();
  if (!key) return;
  const current = Object.fromEntries(readLocalDrafts().map((item) => [workflowKey(item), item]));
  delete current[key];
  delete current[slugify(key)];
  window.localStorage.setItem(LOCAL_DRAFTS_KEY, JSON.stringify(current));
}

function isSystemRunnerNode(node: RunnerNode, meta?: { key?: string }) {
  const type = String(node.type || "").toLowerCase();
  return meta?.key === "start" || meta?.key === "end" || type === "start" || type === "end";
}

function systemNodeLabel(node: RunnerNode, meta: { key?: string }) {
  if (meta.key === "end" || String(node.type || "").toLowerCase() === "end") return "结束节点";
  return "开始节点";
}

function systemNodeHint(node: RunnerNode, meta: { key?: string }) {
  if (meta.key === "end" || String(node.type || "").toLowerCase() === "end") return "由工作流自动维护，负责结束流程并汇总最终状态。";
  return "由工作流自动维护，负责接收输入变量并初始化上下文。";
}

function nodeDisplayName(node: RunnerNode, meta?: { key?: string }) {
  if (isSystemRunnerNode(node, meta)) return String(node.label || node.step?.name || node.id);
  return String(node.label || node.step?.name || node.id);
}

function actionEditorKind(action: string) {
  const value = String(action || "").trim();
  if (value === "shell.run" || value === "script.shell" || value === "script.python") return "script";
  if (value === "cmd.run") return "command";
  return "";
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

function delay(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function fetchRunHistoryEvents(runId: string, graph: RunnerGraph) {
  const history = await requestJson(`/api/runner-studio/runs/${encodeURIComponent(runId)}/events/history`);
  return mapRunnerRunEventsToGraph(extractRunnerRunEvents(history), graph) as Record<string, unknown>[];
}

async function waitForRunHistory(runId: string, graph: RunnerGraph) {
  let events: Record<string, unknown>[] = [];
  for (let attempt = 0; attempt < 20; attempt += 1) {
    events = await fetchRunHistoryEvents(runId, graph);
    if (isRunnerRunHistoryTerminal(events)) break;
    await delay(Math.min(1000, 200 + attempt * 100));
  }
  return events;
}

function runStatusMessage(status: string) {
  if (status === "success" || status === "completed") return "运行成功";
  if (status === "failed") return "运行失败";
  if (status === "cancelled" || status === "canceled") return "运行已取消";
  return "运行已提交";
}

function workflowStartNodeId(graph: RunnerGraph) {
  const node = (graph.nodes || []).find((item) => String(item?.type || "").toLowerCase() === "start" || String(item?.id || "").toLowerCase() === "start");
  return String(node?.id || "");
}

function buildRunRecord(runId: string, events: Record<string, unknown>[], fallback: Partial<RunnerRunRecord> = {}): RunnerRunRecord {
  const state = reduceRunEvents(events, createInitialRunState());
  const id = String(runId || state.runId || fallback.runId || `run-${Date.now()}`);
  const derivedStatus = finalRunnerRunStatus(events) || String(state.status || fallback.status || "unknown");
  const traceability = runTraceabilityFromEvents(events, fallback);
  return {
    runId: id,
    status: derivedStatus,
    message: String(state.message || state.error || fallback.message || ""),
    startedAt: String(state.startedAt || fallback.startedAt || new Date().toISOString()),
    finishedAt: String(state.finishedAt || fallback.finishedAt || ""),
    ...traceability,
    events: events.map((event) => ({ ...event, run_id: event.run_id || id })),
    state: { ...state, runId: state.runId || id, status: derivedStatus },
  };
}

function runTraceabilityFromEvents(events: Record<string, unknown>[], fallback: Partial<RunnerRunRecord>) {
  const sources = [fallback as Record<string, unknown>, ...events.map((event) => objectValue(event))];
  return {
    caseId: String(firstScalarValue(sources, ["caseId", "case_id", "incidentId", "incident_id"]) || ""),
    hostLeaseId: String(firstScalarValue(sources, ["hostLeaseId", "host_lease_id", "leaseId", "lease_id"]) || ""),
    failedStep: String(firstScalarValue(sources, ["failedStep", "failed_step", "step"]) || ""),
    failedReason: String(firstScalarValue(sources, ["failedReason", "failed_reason", "reason", "error"]) || ""),
    rollbackResult: String(firstScalarValue(sources, ["rollbackResult", "rollback_result"]) || ""),
    verificationRefs: firstArrayValue(sources, ["verificationRefs", "verification_refs"]).map((item) => String(item)).filter(Boolean),
  };
}

function runRecordFromState(state: RunState, events: Record<string, unknown>[] = []) {
  if (!state.runId && state.status === "idle") return null;
  return buildRunRecord(state.runId || "current-run", events, {
    status: state.status,
    message: state.message || state.error,
    startedAt: state.startedAt,
    finishedAt: state.finishedAt,
  });
}

function errorMessage(error: unknown, fallback = "运行失败") {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
}

function readableFailureMessage(value: unknown) {
  const message = String(value || "").trim();
  if (!message) return "";
  if (/failed to fetch/i.test(message)) {
    return `${message}（网络请求失败，请检查 ai-server 与 Runner 服务是否已启动，或 /api/runner-studio/runs 是否可访问。）`;
  }
  return message;
}

function uniqueFailureLines(items: unknown[]) {
  const seen = new Set<string>();
  return items
    .map(readableFailureMessage)
    .flatMap((item) => item.split("\n").map((line) => line.trim()).filter(Boolean))
    .filter((item) => {
      if (seen.has(item)) return false;
      seen.add(item);
      return true;
    });
}

function isRecoverableApiFailure(error: unknown) {
  const status = Number((error as { status?: number })?.status || 0);
  return status === 404 || status === 503;
}

function formatRunnerStudioNotice(error: unknown): ApiNotice {
  const status = Number((error as { status?: number })?.status || 0);
  if (status === 404) {
    return {
      title: "内置 Runner API 暂不可用",
      message: "当前 ai-server 没有暴露 /api/runner-studio/*，已启用本地动作库，可先创建和编排工作流草稿。",
      hint: "请使用最新 start.sh 重新启动 ai-server。",
    };
  }
  return {
    title: "内置 Runner API 暂不可用",
    message: "当前 ai-server 未返回 Runner Studio API，已启用本地动作库，可先完成工作流草稿。",
    hint: "请确认 ai-server 已用最新 start.sh 启动，且未设置 AIOPS_RUNNER_DISABLED=1。",
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

const DEFAULT_HOST_GROUPS: RunnerHostGroup[] = [{ label: "local", hosts: ["local"] }];

function splitList(value: unknown) {
  if (Array.isArray(value)) return value.map((item) => String(item || "").trim()).filter(Boolean);
  return String(value || "")
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function saveStateLabel(saveState: SaveState) {
  if (saveState.status === "pending") return saveState.message || "未保存";
  if (saveState.status === "saving") return saveState.message || "正在保存";
  if (saveState.status === "saved") {
    const message = saveState.message || "已保存";
    return saveState.lastSavedAt ? `${message} ${saveState.lastSavedAt}` : message;
  }
  if (saveState.status === "local_draft") return saveState.message || "本地草稿，未同步";
  if (saveState.status === "blocked") return saveState.message || "操作被阻止";
  if (saveState.status === "failed" || saveState.status === "error") return saveState.message || "操作失败";
  return saveState.message || "草稿";
}

function workflowTypeLabel(value: unknown) {
  const labels: Record<string, string> = {
    diagnosis: "诊断",
    diagnostic: "诊断",
    diagnose: "诊断",
    repair: "修复",
    remediation: "修复",
    rollback: "回滚",
    verification: "验证",
    verify: "验证",
  };
  const normalized = String(value || "").trim().toLowerCase();
  return labels[normalized] || (normalized ? normalized : "未分类");
}

function workflowContextView(workflow: Partial<Workflow> | null | undefined, graph?: RunnerGraph | null) {
  const graphWorkflow = objectValue(graph?.workflow || workflow?.graph?.workflow);
  const sources = [workflow as Record<string, unknown>, graphWorkflow].filter(Boolean);
  const hostProfile = firstRecordValue(sources, ["host_profile_snapshot", "hostProfileSnapshot", "host_profile"]);
  const hostLease = firstRecordValue(sources, ["host_lease", "hostLease", "lease"]);
  const experienceBinding = firstRecordValue(sources, ["experience_pack_binding", "experiencePackBinding", "experience_pack"]);
  const hostProfiles = firstArrayValue(sources, ["host_profile_snapshots", "hostProfileSnapshots"]);
  const hostLeases = firstArrayValue(sources, ["host_leases", "hostLeases"]);
  const resolvedHostProfile = Object.keys(hostProfile).length ? hostProfile : objectValue(hostProfiles[0]);
  const resolvedHostLease = Object.keys(hostLease).length ? hostLease : objectValue(hostLeases[0]);
  return {
    typeLabel: workflowTypeLabel(firstScalarValue(sources, ["workflow_type", "workflowType", "type", "category"])),
    caseId: String(firstScalarValue(sources, ["case_id", "caseId", "incident_id", "incidentId"]) || ""),
    hostProfileId: String(firstScalarValue([resolvedHostProfile], ["host_id", "hostId", "display_name", "displayName", "id"]) || ""),
    hostProfileSummary: [firstScalarValue([resolvedHostProfile], ["display_name", "displayName"]), firstScalarValue([resolvedHostProfile], ["os"]), firstScalarValue([resolvedHostProfile], ["arch"])].filter(Boolean).join(" / "),
    hostLeaseId: String(firstScalarValue([resolvedHostLease], ["lease_id", "leaseId", "id"]) || ""),
    hostLeaseStatus: String(firstScalarValue([resolvedHostLease], ["status", "state"]) || ""),
    experiencePackId: String(firstScalarValue([experienceBinding], ["pack_id", "packId", "id", "experience_pack_id", "experiencePackId"]) || ""),
    workflowBindable: booleanValue(firstScalarValue([experienceBinding], ["workflow_bindable", "workflowBindable", "bindable", "enabled"]), false),
  };
}

function firstScalarValue(sources: Array<Record<string, unknown> | null | undefined>, keys: string[]) {
  for (const source of sources) {
    if (!source) continue;
    for (const key of keys) {
      const value = source[key];
      if (value !== undefined && value !== null && value !== "" && typeof value !== "object") return value;
    }
  }
  return "";
}

function firstRecordValue(sources: Array<Record<string, unknown> | null | undefined>, keys: string[]) {
  for (const source of sources) {
    if (!source) continue;
    for (const key of keys) {
      const value = source[key];
      if (value && typeof value === "object" && !Array.isArray(value)) return value as Record<string, unknown>;
    }
  }
  return {};
}

function firstArrayValue(sources: Array<Record<string, unknown> | null | undefined>, keys: string[]) {
  for (const source of sources) {
    if (!source) continue;
    for (const key of keys) {
      const value = source[key];
      if (Array.isArray(value)) return value;
    }
  }
  return [];
}

function booleanValue(value: unknown, fallback = false) {
  if (typeof value === "boolean") return value;
  const normalized = String(value || "").trim().toLowerCase();
  if (["true", "1", "yes", "enabled"].includes(normalized)) return true;
  if (["false", "0", "no", "disabled"].includes(normalized)) return false;
  return fallback;
}

function workflowTestId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]+/g, "-");
}

function WorkflowLibrary({ workflows, onSelect, onDelete }: { workflows: Workflow[]; onSelect: (name: string) => void; onDelete: (name: string) => void }) {
  return (
    <section className="runner-workflow-library" data-testid="runner-workflow-library">
      <div className="workflow-quick-list">
        {workflows.length === 0 ? <p className="runner-studio-empty">暂无工作流，打开管理器创建一个 blank workflow。</p> : null}
        {workflows.map((workflow) => {
          const context = workflowContextView(workflow);
          const key = workflowKey(workflow);
          return (
            <div key={key} className="runner-studio-workflow-row">
              <button type="button" className="runner-studio-workflow" onClick={() => onSelect(key)}>
                <span>{workflow.title || workflow.name}</span>
                <small>{workflow.status || "draft"}</small>
                <span className="runner-workflow-card-meta">
                  <em>类型：{context.typeLabel}</em>
                  {context.hostProfileId ? <em>HostProfileSnapshot {context.hostProfileId}</em> : null}
                  {context.hostLeaseId ? <em>HostLease {context.hostLeaseId}</em> : null}
                  <em>{context.workflowBindable ? "可绑定运维手册" : "未开放运维手册绑定"}</em>
                </span>
              </button>
              <button
                type="button"
                className="workflow-icon-button danger"
                aria-label={`删除工作流 ${workflow.title || workflow.name}`}
                data-testid={`runner-delete-workflow-${workflowTestId(key)}`}
                onClick={() => onDelete(key)}
              >
                <Trash2 size={15} />
              </button>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function WorkflowManagerModal({ workflows, onClose, onCreateBlank, onSelect, onDelete }: { workflows: Workflow[]; onClose: () => void; onCreateBlank: (name: string) => void; onSelect: (name: string) => void; onDelete: (name: string) => void }) {
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
              <button
                type="button"
                className="workflow-icon-button danger"
                aria-label={`删除工作流 ${workflow.title || workflow.name}`}
                data-testid={`workflow-manager-delete-${workflowTestId(workflowKey(workflow))}`}
                onClick={() => onDelete(workflowKey(workflow))}
              >
                <Trash2 size={15} />
              </button>
            </div>
          ))}
        </main>
      </div>
    </section>
  );
}

function StartHostGroupsEditor({ groups, onChange }: { groups: RunnerHostGroup[]; onChange: (groups: RunnerHostGroup[]) => void }) {
  const visibleGroups = groups.length ? groups : DEFAULT_HOST_GROUPS;
  const updateGroup = (index: number, patch: Partial<RunnerHostGroup>) => onChange(visibleGroups.map((group, itemIndex) => itemIndex === index ? { ...group, ...patch } : group));
  const removeGroup = (index: number) => onChange(visibleGroups.filter((_, itemIndex) => itemIndex !== index));
  return (
    <section className="runner-host-groups-editor" data-testid="runner-start-host-groups">
      <div className="runner-editor-section-head"><strong>运行主机组</strong><span>每个标签对应一台或多台主机。</span></div>
      {visibleGroups.map((group, index) => (
        <div key={`${group.label}-${index}`} className="runner-host-group-row">
          <label>标签<input data-testid={`runner-host-group-label-${index}`} value={group.label} placeholder="web" onChange={(event) => updateGroup(index, { label: event.target.value })} /></label>
          <label>主机<textarea data-testid={`runner-host-group-hosts-${index}`} value={group.hosts.join("\n")} placeholder={"web-01\nweb-02"} onChange={(event) => updateGroup(index, { hosts: splitList(event.target.value) })} /></label>
          <button type="button" onClick={() => removeGroup(index)} disabled={visibleGroups.length <= 1}>移除</button>
        </div>
      ))}
      <button type="button" data-testid="runner-host-group-add" onClick={() => onChange([...visibleGroups, { label: "", hosts: [] }])}>添加主机标签</button>
    </section>
  );
}

function RunnerNodeTargetsEditor({ groups, targets, onChange }: { groups: RunnerHostGroup[]; targets: string[]; onChange: (targets: string[]) => void }) {
  const toggleTarget = (label: string) => {
    if (targets.includes(label)) onChange(targets.filter((item) => item !== label));
    else onChange([...targets, label]);
  };
  return (
    <section className="runner-node-targets" data-testid="runner-node-targets">
      <label>目标标签<input data-testid="runner-node-target-labels-input" value={targets.join(", ")} placeholder="web, db" onChange={(event) => onChange(normalizeTargetLabels(event.target.value))} /></label>
      <div className="runner-node-target-options" data-testid="runner-node-target-options">
        {groups.map((group) => (
          <button key={group.label} type="button" className={targets.includes(group.label) ? "active" : ""} onClick={() => toggleTarget(group.label)}>
            {group.label}<span>{group.hosts.length} 台</span>
          </button>
        ))}
      </div>
    </section>
  );
}

function EndCallbacksEditor({ callbacks, onChange }: { callbacks: RunnerEndCallback[]; onChange: (callbacks: RunnerEndCallback[]) => void }) {
  const visibleCallbacks = callbacks.length ? callbacks : [{ event: "success", url: "", payload: "" }];
  const updateCallback = (index: number, patch: Partial<RunnerEndCallback>) => onChange(visibleCallbacks.map((callback, itemIndex) => itemIndex === index ? { ...callback, ...patch } : callback));
  const removeCallback = (index: number) => onChange(visibleCallbacks.filter((_, itemIndex) => itemIndex !== index));
  return (
    <section className="runner-end-callbacks" data-testid="runner-end-callbacks">
      <div className="runner-editor-section-head"><strong>结束回调</strong><span>在工作流结束后按状态触发外部回调。</span></div>
      {visibleCallbacks.map((callback, index) => (
        <div key={`${callback.event}-${index}`} className="runner-end-callback-row">
          <label>触发<select data-testid={`runner-end-callback-event-${index}`} value={callback.event} onChange={(event) => updateCallback(index, { event: event.target.value })}><option value="success">成功</option><option value="failed">失败</option><option value="always">总是</option></select></label>
          <label>URL<input data-testid={`runner-end-callback-url-${index}`} value={callback.url} placeholder="https://hooks.example/runner" onChange={(event) => updateCallback(index, { url: event.target.value })} /></label>
          <label>Payload<textarea data-testid={`runner-end-callback-payload-${index}`} value={callback.payload} placeholder='{"run":"{{sys.run_id}}"}' onChange={(event) => updateCallback(index, { payload: event.target.value })} /></label>
          <button type="button" onClick={() => removeCallback(index)} disabled={visibleCallbacks.length <= 1}>移除</button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...visibleCallbacks, { event: "success", url: "", payload: "" }])}>添加回调</button>
    </section>
  );
}

function RunnerNodePanel({ node, graph, runState, onClose, onApply, onRunNode, onDelete }: { node: RunnerNode; graph: RunnerGraph; runState: ReturnType<typeof createInitialRunState>; onClose: () => void; onApply: (node: RunnerNode) => void; onRunNode: (nodeId: string) => void; onDelete: (nodeId: string) => void }) {
  const [draft, setDraft] = useState(() => cloneNode(node));
  const [tab, setTab] = useState("settings");
  const [scriptModalOpen, setScriptModalOpen] = useState(false);
  useEffect(() => { setDraft(cloneNode(node)); setTab("settings"); setScriptModalOpen(false); }, [node]);
  const meta = getNodeCanvasMeta(draft);
  const runNode = runState.nodes?.[draft.id];
  const inputs = normalizeInputParams(draft.inputs);
  const outputs = normalizeOutputParams(draft.outputs);
  const variables = collectRunnerVariables(graph, draft.id, { runState });
  const isCondition = meta.key === "condition" || draft.type === "condition" || draft.step?.action === "condition.evaluate";
  const isAggregator = meta.key === "variable-aggregator" || draft.type === "variable_aggregator" || draft.step?.action === "variable.aggregate";
  const isSystemNode = isSystemRunnerNode(draft, meta);
  const isStartNode = meta.key === "start" || String(draft.type || "").toLowerCase() === "start";
  const isEndNode = meta.key === "end" || String(draft.type || "").toLowerCase() === "end";
  const displayName = nodeDisplayName(draft, meta);
  const actionName = String(draft.step?.action || meta.action || draft.type || "").trim();
  const actionKind = actionEditorKind(actionName);
  const stepArgs = objectValue(draft.step?.args);
  const nodeUi = objectValue(draft.ui);
  const hostGroups = isStartNode ? normalizeEditableHostGroups(nodeUi.host_groups) : workflowHostGroups(graph, draft);
  const stepTargets = normalizeTargetLabels(draft.step?.targets || []);
  const endCallbacks = normalizeEndCallbacks(nodeUi.callbacks);
  const condition = readNodeCondition(draft);
  const aggregator = readNodeAggregator(draft);
  const aggregatorSources = normalizeAggregatorSources(aggregator.sources);
  const continueOnError = Boolean(draft.step?.continue_on_error || draft.step?.continueOnError);
  const isCodeAction = actionKind === "script" || actionKind === "command";
  const updateInput = (index: number, patch: Record<string, unknown>) => setDraft({ ...draft, inputs: inputs.map((item: Record<string, unknown>, itemIndex: number) => itemIndex === index ? { ...item, ...patch } : item) });
  const updateInputSource = (index: number, value_source: Record<string, unknown>) => updateInput(index, { value_source });
  const removeInput = (index: number) => setDraft({ ...draft, inputs: inputs.filter((_, itemIndex: number) => itemIndex !== index) });
  const updateOutput = (index: number, patch: Record<string, unknown>) => setDraft({ ...draft, outputs: outputs.map((item: Record<string, unknown>, itemIndex: number) => itemIndex === index ? { ...item, ...patch } : item) });
  const removeOutput = (index: number) => setDraft({ ...draft, outputs: outputs.filter((_, itemIndex: number) => itemIndex !== index) });
  const appendConditionVariable = (variable: { expression?: string }) => {
    const expression = String(variable.expression || "").trim();
    if (!expression) return;
    const current = String(condition.if || "").trim();
    updateCondition({ if: current ? `${current} ${expression}` : expression });
  };
  const updateCondition = (patch: Record<string, unknown>) => {
    const nextCondition = { ...condition, ...patch };
    setDraft({
      ...draft,
      type: "condition",
      condition: nextCondition,
      data: { ...objectValue(draft.data), condition: nextCondition },
      step: { ...(draft.step || {}), name: draft.step?.name || draft.id, action: draft.step?.action || "condition.evaluate" },
    });
  };
  const updateAggregator = (patch: Record<string, unknown>) => {
    const nextAggregator = { ...aggregator, ...patch };
    const outputKey = String(nextAggregator.output_key || "").trim();
    setDraft({
      ...draft,
      type: "variable_aggregator",
      aggregator: nextAggregator,
      data: { ...objectValue(draft.data), aggregator: nextAggregator },
      outputs: ensureAggregatorOutput(outputs, outputKey),
      step: { ...(draft.step || {}), name: draft.step?.name || draft.id, action: draft.step?.action || "variable.aggregate" },
    });
  };
  const addAggregatorSource = (variable: RunnerVariableOption) => {
    const source = variableToAggregatorSource(variable);
    if (!source.expression) return;
    updateAggregator({ sources: [...aggregatorSources, source] });
  };
  const updateNodeUi = (patch: Record<string, unknown>) => setDraft({ ...draft, ui: { ...objectValue(draft.ui), ...patch } });
  const updateStep = (patch: Record<string, unknown>) => setDraft({ ...draft, step: { ...(draft.step || {}), ...patch } });
  const updateStepArg = (key: string, value: unknown) => updateStep({ args: { ...objectValue(draft.step?.args), [key]: value } });
  const tabs = [
    ["settings", "设置"],
    ...(isAggregator ? [["aggregate", "聚合"]] : []),
    ...(isCondition ? [["branches", "分支"]] : []),
    ["error", "错误处理"],
    ["advanced", "高级"],
    ["last-run", "上次运行"],
  ];
  return (
    <aside className="runner-node-panel" role="complementary" aria-label="节点配置面板" data-testid="runner-node-panel">
      <header className="runner-node-panel-head">
        <div className="runner-node-panel-identity">
          <span className={`runner-node-panel-icon tone-${meta.tone}`}>{meta.iconText}</span>
          <div><p>{isSystemNode ? systemNodeLabel(draft, meta) : "执行节点"}</p><h2 data-testid="runner-node-panel-title">{displayName}</h2><span className={`runner-node-panel-status status-${runNode?.status || "not_run"}`}>{runNode?.status || "not_run"}</span></div>
        </div>
        <div className="runner-node-panel-actions">
          <button type="button" data-testid="runner-node-panel-run" onClick={() => onRunNode(draft.id)}><Play size={15} />运行</button>
          <button type="button" data-testid="runner-node-panel-open-run" onClick={() => setTab("last-run")}><PanelRight size={15} />详情</button>
          <button type="button" className="primary" data-testid="runner-node-panel-apply" onClick={() => onApply(cloneNode(draft))}><CheckCircle size={15} />保存</button>
          {!isSystemNode ? <button type="button" className="danger" aria-label="删除节点" data-testid="runner-node-panel-delete" onClick={() => onDelete(draft.id)}><Trash2 size={15} />删除</button> : null}
          <button type="button" aria-label="关闭节点配置" data-testid="runner-node-panel-close" onClick={onClose}><X size={16} /></button>
        </div>
      </header>
      <nav className="runner-node-panel-tabs" aria-label="节点配置页签" data-testid="runner-node-panel-tabs">
        {tabs.map(([key, label]) => <button key={key} type="button" className={tab === key ? "active" : ""} data-testid={`runner-node-panel-tab-${key}`} onClick={() => setTab(key)}>{label}</button>)}
      </nav>
      <main className="runner-node-panel-body">
        {tab === "settings" ? (
          <section className="node-config-form runner-node-settings" data-testid="runner-node-settings">
            {isSystemNode ? (
              <>
                <section className="runner-node-system-card" data-testid="runner-node-system-card">
                  <strong>{systemNodeLabel(draft, meta)}</strong>
                  <p>{systemNodeHint(draft, meta)}</p>
                  <span>节点类型 <code>{draft.type || meta.key}</code></span>
                </section>
                {isStartNode ? <StartHostGroupsEditor groups={hostGroups} onChange={(groups) => updateNodeUi({ host_groups: groups })} /> : null}
                {isEndNode ? (
                  <>
                    <section className="runner-end-final-variables" data-testid="runner-end-final-variables">
                      <div className="runner-editor-section-head"><strong>最终变量</strong><span>工作流结束时可查看所有上游输出和系统变量。</span></div>
                      <RunnerVariableInspector graph={graph} state={runState} selectedNodeId="" />
                    </section>
                    <EndCallbacksEditor callbacks={endCallbacks} onChange={(callbacks) => updateNodeUi({ callbacks })} />
                  </>
                ) : null}
              </>
            ) : (
              <>
                <RunnerNodeTargetsEditor groups={hostGroups} targets={stepTargets} onChange={(targets) => updateStep({ targets })} />
                {isCodeAction ? (
                  <>
                    <RunnerCodeInputVariablesEditor
                      inputs={inputs}
                      variables={variables}
                      onAdd={() => setDraft({ ...draft, inputs: [...inputs, createInputParam(`input_${inputs.length + 1}`)] })}
                      onUpdate={updateInput}
                      onRemove={removeInput}
                      onUpdateSource={updateInputSource}
                    />
                    <RunnerCodeOutputVariablesEditor
                      outputs={outputs}
                      onAdd={() => setDraft({ ...draft, outputs: [...outputs, createOutputParam(`output_${outputs.length + 1}`)] })}
                      onUpdate={updateOutput}
                      onRemove={removeOutput}
                    />
                  </>
                ) : null}
                {actionKind === "script" ? (
                  <label className="runner-node-script-field">
                    <span className="runner-node-field-label">脚本内容<button type="button" data-testid="runner-node-script-expand" aria-label="放大脚本编辑器" onClick={() => setScriptModalOpen(true)}><Maximize2 size={14} />放大</button></span>
                    <textarea
                      data-testid="runner-node-script-editor"
                      value={String(stepArgs.script || "")}
                      placeholder={actionName === "script.python" ? "print('ok')" : "set -e\necho ok"}
                      spellCheck={false}
                      onChange={(event) => updateStepArg("script", event.target.value)}
                    />
                  </label>
                ) : null}
                {actionKind === "command" ? (
                  <label>命令<input data-testid="runner-node-command-editor" value={String(stepArgs.cmd || "")} placeholder="df -h" onChange={(event) => updateStepArg("cmd", event.target.value)} /></label>
                ) : null}
                {(actionName === "script.shell" || actionName === "script.python") ? (
                  <label>脚本引用<input value={String(stepArgs.script_ref || "")} placeholder="restore.sh" onChange={(event) => updateStepArg("script_ref", event.target.value)} /></label>
                ) : null}
              </>
            )}
            {scriptModalOpen ? (
              <section className="runner-script-editor-backdrop" data-testid="runner-script-editor-modal" role="dialog" aria-modal="true" aria-label="脚本内容编辑器">
                <div className="runner-script-editor-modal">
                  <header>
                    <strong>脚本内容</strong>
                    <button type="button" data-testid="runner-script-editor-modal-close" aria-label="关闭脚本编辑器" onClick={() => setScriptModalOpen(false)}><X size={16} /></button>
                  </header>
                  <textarea
                    data-testid="runner-script-editor-modal-textarea"
                    value={String(stepArgs.script || "")}
                    spellCheck={false}
                    onChange={(event) => updateStepArg("script", event.target.value)}
                  />
                </div>
              </section>
            ) : null}
          </section>
        ) : null}
        {tab === "aggregate" ? (
          <section className="node-config-form runner-aggregate-editor" data-testid="runner-aggregate-editor">
            <label>输出 Key<input value={String(aggregator.output_key || "")} placeholder="aggregated_value" onChange={(event) => updateAggregator({ output_key: event.target.value })} /></label>
            <label>策略<select value={String(aggregator.strategy || "first_non_empty")} onChange={(event) => updateAggregator({ strategy: event.target.value })}><option value="first_non_empty">first_non_empty</option><option value="prefer_success">prefer_success</option><option value="array">array</option></select></label>
            <div className="runner-aggregate-sources">
              <strong>候选变量</strong>
              {aggregatorSources.length ? aggregatorSources.map((source: Record<string, unknown>, index: number) => (
                <div key={`${source.expression}-${index}`} className="runner-aggregate-source">
                  <code>{String(source.expression || "")}</code>
                  <button type="button" onClick={() => updateAggregator({ sources: aggregatorSources.filter((_, itemIndex) => itemIndex !== index) })}>移除</button>
                </div>
              )) : <p>从下方变量列表选择候选源。</p>}
            </div>
            <RunnerVariablePicker variables={variables} onPick={addAggregatorSource} />
          </section>
        ) : null}
        {tab === "branches" ? (
          <section className="node-config-form runner-branch-editor" data-testid="runner-branch-editor">
            <label>IF 表达式<input value={String(condition.if || "")} placeholder="deploy == true" onChange={(event) => updateCondition({ if: event.target.value })} /></label>
            <RunnerVariablePicker variables={variables} onPick={appendConditionVariable} />
            <label className="runner-checkbox-field"><input type="checkbox" checked={Boolean(condition.else ?? true)} onChange={(event) => updateCondition({ else: event.target.checked })} />启用 ELSE 兜底分支</label>
            <p>IF 端口命中表达式为 true 的路径；ELSE 端口在没有条件命中时执行。</p>
          </section>
        ) : null}
        {tab === "error" ? (
          <section className="node-config-form runner-error-editor" data-testid="runner-error-editor">
            <label className="runner-checkbox-field"><input type="checkbox" checked={continueOnError} onChange={(event) => updateStep({ continue_on_error: event.target.checked })} />失败后继续执行可用分支</label>
            <label>重试次数<input type="number" min="0" value={Number(draft.step?.retries || 0)} onChange={(event) => updateStep({ retries: Number(event.target.value) || 0 })} /></label>
            <label>超时<input value={String(draft.step?.timeout || "")} placeholder="30s / 5m" onChange={(event) => updateStep({ timeout: event.target.value })} /></label>
          </section>
        ) : null}
        {tab === "advanced" ? <pre>{JSON.stringify(draft, null, 2)}</pre> : null}
        {tab === "last-run" ? <NodeLastRunView nodeId={draft.id} runState={runState} /> : null}
      </main>
    </aside>
  );
}

type RunnerVariableOption = {
  expression?: string;
  scope?: string;
  name?: string;
  type?: string;
  nodeId?: string;
  node_id?: string;
  sourceNodeId?: string;
  displayValue?: string;
};

function RunnerVariablePicker({ variables, onPick }: { variables: RunnerVariableOption[]; onPick: (variable: RunnerVariableOption) => void }) {
  const [query, setQuery] = useState("");
  const visible = variables.filter((variable) => {
    const text = [variable.expression, variable.scope, variable.name, variable.nodeId || variable.node_id || variable.sourceNodeId].join(" ").toLowerCase();
    return !query.trim() || text.includes(query.trim().toLowerCase());
  }).slice(0, 30);
  return (
    <section className="runner-variable-picker" data-testid="runner-variable-picker">
      <input value={query} placeholder="搜索变量" onChange={(event) => setQuery(event.target.value)} />
      <div>
        {visible.length ? visible.map((variable) => (
          <button key={variable.expression} type="button" onClick={() => onPick(variable)}>
            <code>{variable.expression}</code>
            <span>{variable.type || "any"}{variable.displayValue !== undefined ? ` · ${variable.displayValue}` : ""}</span>
          </button>
        )) : <p>当前节点暂无可用变量。</p>}
      </div>
    </section>
  );
}

function RunnerCodeInputVariablesEditor({
  inputs,
  variables,
  onAdd,
  onUpdate,
  onRemove,
  onUpdateSource,
}: {
  inputs: Array<Record<string, unknown>>;
  variables: RunnerVariableOption[];
  onAdd: () => void;
  onUpdate: (index: number, patch: Record<string, unknown>) => void;
  onRemove: (index: number) => void;
  onUpdateSource: (index: number, source: Record<string, unknown>) => void;
}) {
  const [openPickerIndex, setOpenPickerIndex] = useState<number | null>(null);
  return (
    <section className="runner-code-variable-editor" data-testid="runner-code-input-variables">
      <div className="runner-code-variable-editor-head">
        <strong>输入变量</strong>
        <button type="button" data-testid="runner-code-input-add" aria-label="添加输入变量" onClick={onAdd}><Plus size={15} /></button>
      </div>
      {inputs.length ? inputs.map((item, index) => {
        const key = String(item.key || `input_${index + 1}`);
        const source = objectValue(item.value_source);
        const label = valueSourceLabel(source) || "设置变量值";
        return (
          <div key={`${key}-${index}`} className="runner-code-variable-row">
            <input value={key} aria-label="输入变量名" onChange={(event) => onUpdate(index, { key: event.target.value })} />
            <button
              type="button"
              className="runner-code-variable-value"
              data-testid={`runner-code-input-value-${index}`}
              onClick={() => setOpenPickerIndex(openPickerIndex === index ? null : index)}
            >
              {label}
            </button>
            <button type="button" className="runner-code-variable-remove" aria-label="删除输入变量" onClick={() => onRemove(index)}><Trash2 size={15} /></button>
            {openPickerIndex === index ? (
              <div className="runner-code-variable-picker" data-testid={`runner-code-input-variable-picker-${index}`}>
                <RunnerVariablePicker
                  variables={variables}
                  onPick={(variable) => {
                    onUpdateSource(index, variableToValueSource(variable));
                    setOpenPickerIndex(null);
                  }}
                />
              </div>
            ) : null}
          </div>
        );
      }) : <p>暂无输入变量。</p>}
    </section>
  );
}

function RunnerCodeOutputVariablesEditor({
  outputs,
  onAdd,
  onUpdate,
  onRemove,
}: {
  outputs: Array<Record<string, unknown>>;
  onAdd: () => void;
  onUpdate: (index: number, patch: Record<string, unknown>) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <section className="runner-code-variable-editor" data-testid="runner-code-output-variables">
      <div className="runner-code-variable-editor-head">
        <strong>输出变量</strong>
        <button type="button" data-testid="runner-code-output-add" aria-label="添加输出变量" onClick={onAdd}><Plus size={15} /></button>
      </div>
      {outputs.length ? outputs.map((item, index) => {
        const key = String(item.key || `output_${index + 1}`);
        return (
          <div key={`${key}-${index}`} className="runner-code-variable-row">
            <input value={key} aria-label="输出变量名" onChange={(event) => onUpdate(index, { key: event.target.value })} />
            <select value={String(item.type || "string")} aria-label="输出变量类型" onChange={(event) => onUpdate(index, { type: event.target.value })}>
              <option value="string">String</option>
              <option value="number">Number</option>
              <option value="boolean">Boolean</option>
              <option value="object">Object</option>
              <option value="array">Array</option>
              <option value="any">Any</option>
            </select>
            <button type="button" className="runner-code-variable-remove" aria-label="删除输出变量" onClick={() => onRemove(index)}><Trash2 size={15} /></button>
          </div>
        );
      }) : <p>暂无输出变量。</p>}
    </section>
  );
}

function NodeLastRunView({ nodeId, runState }: { nodeId: string; runState: RunState }) {
  const runNode = runState.nodes?.[nodeId];
  const logs = (runState.logs || []).filter((log: { nodeId?: string }) => log.nodeId === nodeId);
  const outputs = (runState.variables?.outputs || []).filter((item: { nodeId?: string }) => item.nodeId === nodeId);
  const result = runNode?.result;
  const failureDetails = collectNodeRunFailureDetails(runState, nodeId);
  if (!runNode && !logs.length && !outputs.length && !failureDetails.length) {
    return <section className="runner-last-run-empty">这个节点还没有运行记录。</section>;
  }
  return (
    <section className="runner-last-run-view" data-testid="runner-node-last-run-view">
      {failureDetails.length ? (
        <div className="runner-last-run-block runner-last-run-failure" data-testid="runner-node-last-run-failure">
          <strong>失败原因</strong>
          <ul>
            {failureDetails.map((item) => <li key={item}>{item}</li>)}
          </ul>
        </div>
      ) : null}
      <div className="runner-last-run-grid">
        <span>状态<strong>{runNode?.status || "unknown"}</strong></span>
        <span>耗时<strong>{runNode?.durationMs ? `${runNode.durationMs} ms` : "-"}</strong></span>
      </div>
      {result !== undefined ? <div className="runner-last-run-block"><strong>输出</strong><pre>{formatRuntimeValue(result)}</pre></div> : null}
      {outputs.length ? <div className="runner-last-run-block"><strong>变量输出</strong>{outputs.map((item: { key?: string; name?: string; value?: unknown }, index: number) => <p key={`${item.key || item.name}-${index}`}><code>{item.key || item.name}</code><span>{formatRuntimeValue(item.value)}</span></p>)}</div> : null}
      {logs.length ? <div className="runner-last-run-block"><strong>日志</strong>{logs.map((log: { stream?: string; message?: string }, index: number) => <pre key={`${log.stream}-${index}`}>{log.stream || "log"}: {log.message || ""}</pre>)}</div> : null}
    </section>
  );
}

function RunnerVariableInspector({ graph, state, selectedNodeId }: { graph: RunnerGraph; state: RunState; selectedNodeId: string }) {
  const fallbackNode = graph.nodes?.length ? graph.nodes[graph.nodes.length - 1]?.id || "" : "";
  const variables = collectRunnerVariables(graph, selectedNodeId || fallbackNode, { runState: state });
  const grouped = variables.reduce((acc: Record<string, typeof variables>, variable: typeof variables[number]) => {
    const scope = variable.scope || "other";
    if (!acc[scope]) acc[scope] = [];
    acc[scope].push(variable);
    return acc;
  }, {});
  const scopes = Object.keys(grouped);
  return (
    <section className="publish-review-card runner-variable-inspector" data-testid="runner-variable-inspector">
      <h3>变量检查器</h3>
      <p>{selectedNodeId ? `当前节点：${selectedNodeId}` : "按工作流末端上下文展示"}</p>
      {scopes.length ? scopes.map((scope) => (
        <div key={scope} className="runner-variable-scope">
          <strong>{scope}</strong>
          {grouped[scope].map((variable) => (
            <div key={variable.expression} className="runner-variable-row">
              <code>{variable.expression}</code>
              <span>{variable.type || "any"}</span>
              <small>{variable.displayValue !== undefined ? variable.displayValue : "no run value"}</small>
            </div>
          ))}
        </div>
      )) : <p>暂无可见变量。</p>}
    </section>
  );
}

function runnerNodeFailureMessage(node: { nodeId?: string; error?: unknown; message?: unknown; summary?: unknown; status?: unknown; result?: unknown }, logs: { nodeId?: string; stream?: string; message?: string }[]) {
  const failed = ["failed", "error"].includes(String(node.status || "").toLowerCase());
  if (!failed) return "";
  const nodeId = String(node.nodeId || "").trim();
  const result = objectValue(node.result);
  const nodeLogs = failed && nodeId
    ? logs
      .filter((log) => log.nodeId === nodeId && ["stderr", "stdout"].includes(String(log.stream || "")))
      .sort((a, b) => (a.stream === "stderr" ? -1 : 0) - (b.stream === "stderr" ? -1 : 0))
      .map((log) => `${log.stream || "log"}: ${String(log.message || "").trim()}`)
    : [];
  const details = uniqueFailureLines([
    node.error,
    node.message,
    node.summary,
    typeof node.result === "string" ? node.result : "",
    result.error,
    result.stderr,
    result.stdout,
    ...nodeLogs,
  ]);
  return details.join("\n") || (failed ? "未返回具体错误" : "");
}

function collectNodeRunFailureDetails(state: RunState, nodeId: string) {
  const node = nodeId ? state.nodes?.[nodeId] : null;
  const nodeFailed = ["failed", "error"].includes(String(node?.status || "").toLowerCase());
  const runFailed = ["failed", "error"].includes(String(state.status || "").toLowerCase());
  if (node && !nodeFailed) return [];
  if (!node && !runFailed) return [];
  const logs = (state.logs || []).filter((log: { nodeId?: string }) => log.nodeId === nodeId);
  return uniqueFailureLines([
    runFailed ? state.message : "",
    runFailed ? state.error : "",
    node?.error,
    node?.message,
    runnerNodeFailureMessage(node || {}, logs),
  ]);
}

function RunnerRunPanel({ state, graph, selectedNodeId, onSelectNode }: { state: RunState; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const logs = state.logs || [];
  const runFailure = readableFailureMessage(state.message || state.error);
  const runFailureSummary = runFailure
    ? /^运行提交失败[:：]/.test(runFailure)
      ? runFailure
      : `运行失败：${runFailure}`
    : "";
  const failedNodes = Object.values(state.nodes || {}).filter((node: { status?: string }) => ["failed", "error"].includes(String(node.status || "").toLowerCase()));
  const hasFailureDetails = Boolean(runFailure || failedNodes.length);
  return (
    <section className="runner-run-panel" data-testid="runner-run-panel">
      <div className="publish-review-card">
        <h3>运行概览</h3>
        <p>{state.runId || "尚无运行"} · {state.status}</p>
        {runFailureSummary ? <p className="runner-run-failure-summary">{runFailureSummary}</p> : null}
      </div>
      <div className="publish-review-card">
        <h3>节点</h3>
        <div className="runner-run-node-list">
          {Object.values(state.nodes || {}).map((node) => {
            const message = runnerNodeFailureMessage(node, logs);
            return (
              <button key={node.nodeId} type="button" className={selectedNodeId === node.nodeId ? "active" : ""} onClick={() => onSelectNode(node.nodeId)}>
                <strong>{node.nodeId}{node.stepName && node.stepName !== node.nodeId ? ` · ${node.stepName}` : ""} · {node.status}</strong>
                {message ? <span>{message}</span> : null}
              </button>
            );
          })}
        </div>
        {graph.nodes?.length ? <small>{graph.nodes.length} graph nodes</small> : null}
      </div>
      {hasFailureDetails ? (
        <div className="publish-review-card runner-run-failure-card">
          <h3>失败原因</h3>
          {runFailure ? <p>{runFailure}</p> : null}
          {failedNodes.length ? (
            <ul>
              {failedNodes.map((node) => {
                const message = runnerNodeFailureMessage(node, logs);
                return <li key={node.nodeId}><strong>{node.nodeId}</strong><span>{message}</span></li>;
              })}
            </ul>
          ) : null}
        </div>
      ) : null}
      <RunnerVariableInspector graph={graph} state={state} selectedNodeId={selectedNodeId} />
      {state.approvals?.length ? <div className="publish-review-card"><h3>审批</h3>{state.approvals.map((approval: { id: string; nodeId?: string; status?: string; summary?: string }) => <p key={approval.id}><strong>{approval.nodeId || approval.id}</strong> · {approval.status || "pending"} · {approval.summary || ""}</p>)}</div> : null}
      <div className="publish-review-card"><h3>stdout / stderr / SSE</h3>{logs.length ? logs.map((log, index) => <pre key={`${log.nodeId}-${index}`}>{log.nodeId} {log.stream}: {log.message}</pre>) : <p>{hasFailureDetails ? "暂无运行日志，请查看上方失败原因。" : "暂无日志。"}</p>}</div>
    </section>
  );
}

function RunnerRunHistoryPanel({ records, currentState, currentEvents, graph, selectedNodeId, onSelectNode }: { records: RunnerRunRecord[]; currentState: RunState; currentEvents: Record<string, unknown>[]; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const [selectedRunId, setSelectedRunId] = useState("");
  const currentRecord = runRecordFromState(currentState, currentEvents);
  const visibleRecords = records.length ? records : currentRecord ? [currentRecord] : [];
  const activeRecord = visibleRecords.find((record) => record.runId === selectedRunId) || null;
  const failedNodesFor = (record: RunnerRunRecord) => Object.values(record.state.nodes || {}).filter((node: { status?: string }) => ["failed", "error"].includes(String(node.status || "").toLowerCase()));
  if (activeRecord) {
    return (
      <section className="runner-run-history-panel runner-run-detail-panel" data-testid="runner-run-history-panel">
        <div className="runner-run-detail-nav">
          <button type="button" data-testid="runner-run-history-back" onClick={() => setSelectedRunId("")}><ArrowLeft size={14} />运行记录</button>
          <span>{activeRecord.runId} · {activeRecord.status}</span>
        </div>
        <section data-testid="runner-run-detail-panel">
          <RunnerRunTraceabilityCard record={activeRecord} />
          <RunnerRunPanel state={activeRecord.state} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={onSelectNode} />
        </section>
      </section>
    );
  }
  return (
    <section className="runner-run-history-panel" data-testid="runner-run-history-panel">
      <div className="publish-review-card runner-run-history-list">
        <h3>运行记录</h3>
        {visibleRecords.length ? visibleRecords.map((record) => {
          const failedNodes = failedNodesFor(record);
          return (
            <button key={record.runId} type="button" className={`runner-run-history-row status-${record.status}`} data-testid={`runner-run-history-row-${record.runId}`} onClick={() => setSelectedRunId(record.runId)}>
              <div><strong>{record.runId}</strong><span>{record.status}</span></div>
              <small>{record.finishedAt || record.startedAt || record.message || "尚无时间"}</small>
              <span>{failedNodes.length ? `失败步骤：${failedNodes.map((node: { nodeId?: string }) => node.nodeId).filter(Boolean).join(", ")}` : `节点：${Object.keys(record.state.nodes || {}).length}`}</span>
              {record.caseId ? <span>Case：{record.caseId}</span> : null}
              {record.hostLeaseId ? <span>HostLease：{record.hostLeaseId}</span> : null}
              {record.rollbackResult ? <span>回滚：{record.rollbackResult}</span> : null}
              {record.verificationRefs?.length ? <span>验证：{record.verificationRefs.join(", ")}</span> : null}
            </button>
          );
        }) : <p>暂无运行记录。</p>}
      </div>
    </section>
  );
}

function RunnerRunTraceabilityCard({ record }: { record: RunnerRunRecord }) {
  const hasTrace = Boolean(record.caseId || record.hostLeaseId || record.failedStep || record.failedReason || record.rollbackResult || record.verificationRefs?.length);
  if (!hasTrace) return null;
  return (
    <section className="publish-review-card runner-run-traceability" data-testid="runner-run-traceability">
      <h3>运行追溯</h3>
      <dl>
        {record.caseId ? <div><dt>Case</dt><dd><a href={`/incidents/${encodeURIComponent(record.caseId)}`}>{record.caseId}</a></dd></div> : null}
        {record.hostLeaseId ? <div><dt>HostLease</dt><dd>{record.hostLeaseId}</dd></div> : null}
        {record.failedStep ? <div><dt>failed_step</dt><dd>{record.failedStep}</dd></div> : null}
        {record.failedReason ? <div><dt>failed_reason</dt><dd>{record.failedReason}</dd></div> : null}
        {record.rollbackResult ? <div><dt>rollback_result</dt><dd>{record.rollbackResult}</dd></div> : null}
        {record.verificationRefs?.length ? <div><dt>verification_refs</dt><dd>{record.verificationRefs.join(", ")}</dd></div> : null}
      </dl>
    </section>
  );
}

function RunnerNodeRunDetails({ state, graph, selectedNodeId, onSelectNode }: { state: RunState; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const fallbackNodeId = Object.keys(state.nodes || {})[0] || graph.nodes?.[0]?.id || "";
  const nodeId = selectedNodeId || fallbackNodeId;
  const node = nodeId ? state.nodes?.[nodeId] : null;
  const logs = (state.logs || []).filter((log: { nodeId?: string }) => log.nodeId === nodeId);
  const failureDetails = collectNodeRunFailureDetails(state, nodeId);
  return (
    <section className="runner-node-run-details" data-testid="runner-node-run-details">
      {failureDetails.length ? (
        <div className="publish-review-card runner-run-failure-card">
          <h3>失败原因</h3>
          <ul>
            {failureDetails.map((item) => <li key={item}><span>{item}</span></li>)}
          </ul>
        </div>
      ) : null}
      <NodeLastRunView nodeId={nodeId} runState={state} />
      <div className="publish-review-card"><h3>节点日志</h3>{logs.length ? logs.map((log, index) => <pre key={`${log.nodeId}-${index}`}>{log.stream}: {log.message}</pre>) : <p>{failureDetails.length ? "暂无节点日志，请查看上方失败原因。" : "这个节点暂无日志。"}</p>}</div>
      <div className="publish-review-card">
        <h3>节点列表</h3>
        <div className="runner-run-node-list">
          {Object.values(state.nodes || {}).map((item: { nodeId?: string; status?: string }) => (
            <button key={item.nodeId} type="button" className={nodeId === item.nodeId ? "active" : ""} onClick={() => onSelectNode(item.nodeId || "")}>
              <strong>{item.nodeId} · {item.status}</strong>
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}

function RunnerDebugDock({ graph, state, selectedNodeId, onSelectNode, onClose }: { graph: RunnerGraph; state: RunState; selectedNodeId: string; onSelectNode: (id: string) => void; onClose: () => void }) {
  const [tab, setTab] = useState("variables");
  const selectedEdges = Object.values(state.edges || {});
  const logs = state.logs || [];
  return (
    <section className="runner-debug-dock" data-testid="runner-debug-dock">
      <header className="runner-debug-dock-head">
        <div><strong>调试</strong><span>变量、路径和日志</span></div>
        <nav>{[["variables", "变量"], ["path", "路径"], ["logs", "日志"]].map(([key, label]) => <button key={key} type="button" className={tab === key ? "active" : ""} onClick={() => setTab(key)}>{label}</button>)}</nav>
        <button type="button" aria-label="关闭调试面板" onClick={onClose}>x</button>
      </header>
      <main>
        {tab === "variables" ? <RunnerVariableInspector graph={graph} state={state} selectedNodeId={selectedNodeId} /> : null}
        {tab === "path" ? (
          <section className="runner-debug-path">
            {selectedEdges.length ? selectedEdges.map((edge: { edgeId?: string; source?: string; target?: string; kind?: string; status?: string }) => (
              <button key={edge.edgeId} type="button" onClick={() => onSelectNode(edge.target || "")}>
                {edge.source} {" -> "} {edge.target}
                <span>{edge.kind || edge.status}</span>
              </button>
            )) : <p>暂无已选择路径。</p>}
          </section>
        ) : null}
        {tab === "logs" ? <section className="runner-debug-logs">{logs.length ? logs.map((log: { nodeId?: string; stream?: string; message?: string }, index: number) => <pre key={`${log.nodeId}-${index}`}>{log.nodeId} {log.stream}: {log.message}</pre>) : <p>暂无日志。</p>}</section> : null}
      </main>
    </section>
  );
}

function formatRuntimeValue(value: unknown) {
  if (value === undefined) return "";
  if (value === null) return "null";
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  return String(value);
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function normalizeHostGroups(value: unknown): RunnerHostGroup[] {
  if (Array.isArray(value)) {
    return value
      .map((item, index) => {
        const group = objectValue(item);
        return {
          label: String(group.label || group.name || `group-${index + 1}`).trim(),
          hosts: splitList(group.hosts),
        };
      })
      .filter((group) => group.label && group.hosts.length);
  }
  const record = objectValue(value);
  return Object.entries(record)
    .map(([label, group]) => ({ label, hosts: splitList(objectValue(group).hosts || group) }))
    .filter((group) => group.label && group.hosts.length);
}

function normalizeEditableHostGroups(value: unknown): RunnerHostGroup[] {
  if (Array.isArray(value)) {
    const groups = value.map((item, index) => {
      const group = objectValue(item);
      return {
        label: String(group.label || group.name || (index === 0 ? "local" : "")).trim(),
        hosts: splitList(group.hosts),
      };
    });
    return groups.length ? groups : DEFAULT_HOST_GROUPS;
  }
  const normalized = normalizeHostGroups(value);
  return normalized.length ? normalized : DEFAULT_HOST_GROUPS;
}

function hostGroupsFromInventory(graph: RunnerGraph): RunnerHostGroup[] {
  const inventory = objectValue(objectValue(graph.workflow).inventory);
  const groups = objectValue(inventory.groups);
  return Object.entries(groups)
    .map(([label, group]) => ({ label, hosts: splitList(objectValue(group).hosts || group) }))
    .filter((group) => group.label && group.hosts.length);
}

function readNodeHostGroups(node?: RunnerNode | null): RunnerHostGroup[] {
  if (!node) return [];
  return normalizeHostGroups(objectValue(node.ui).host_groups);
}

function workflowHostGroups(graph: RunnerGraph, draft?: RunnerNode): RunnerHostGroup[] {
  const draftGroups = draft && (draft.type === "start" || getNodeCanvasMeta(draft).key === "start") ? readNodeHostGroups(draft) : [];
  if (draftGroups.length) return draftGroups;
  const startNode = (graph.nodes || []).find((item) => String(item.type || "").toLowerCase() === "start" || item.id === "start");
  const nodeGroups = readNodeHostGroups(startNode);
  if (nodeGroups.length) return nodeGroups;
  const inventoryGroups = hostGroupsFromInventory(graph);
  return inventoryGroups.length ? inventoryGroups : DEFAULT_HOST_GROUPS;
}

function normalizeTargetLabels(value: unknown): string[] {
  return Array.from(new Set(splitList(value)));
}

function normalizeEndCallbacks(value: unknown): RunnerEndCallback[] {
  const callbacks = Array.isArray(value)
    ? value.map((item) => {
      const callback = objectValue(item);
      return {
        event: String(callback.event || "success"),
        url: String(callback.url || ""),
        payload: String(callback.payload || ""),
      };
    })
    : [];
  return callbacks.length ? callbacks : [{ event: "success", url: "", payload: "" }];
}

function inventoryFromHostGroups(graph: RunnerGraph, groups: RunnerHostGroup[]) {
  const workflow = objectValue(graph.workflow);
  const currentInventory = objectValue(workflow.inventory);
  const currentHosts = objectValue(currentInventory.hosts);
  const nextGroups: Record<string, { hosts: string[] }> = {};
  const nextHosts: Record<string, Record<string, unknown>> = { ...currentHosts } as Record<string, Record<string, unknown>>;
  for (const group of groups) {
    const label = String(group.label || "").trim();
    const hosts = normalizeTargetLabels(group.hosts);
    if (!label || !hosts.length) continue;
    nextGroups[label] = { hosts };
    for (const host of hosts) {
      nextHosts[host] = { ...objectValue(nextHosts[host]), address: String(objectValue(nextHosts[host]).address || host) };
    }
  }
  return {
    ...currentInventory,
    groups: nextGroups,
    hosts: nextHosts,
  };
}

function reconcileRunnerRuntimeConfig(graph: RunnerGraph): RunnerGraph {
  const normalized = normalizeGraph(graph, String(objectValue(graph.workflow).name || ""));
  const startNode = (normalized.nodes || []).find((node) => String(node.type || "").toLowerCase() === "start" || node.id === "start");
  const groups = workflowHostGroups(normalized, startNode);
  if (!groups.length) return normalized;
  const nodes = (normalized.nodes || []).map((node) => {
    if (node.id !== startNode?.id) return node;
    return { ...node, ui: { ...objectValue(node.ui), host_groups: groups } };
  });
  return {
    ...normalized,
    workflow: { ...objectValue(normalized.workflow), inventory: inventoryFromHostGroups(normalized, groups) },
    nodes,
  };
}

function readNodeCondition(node: RunnerNode): Record<string, unknown> {
  const data = objectValue(node.data);
  return { ...objectValue(data.condition), ...objectValue(node.condition) };
}

function readNodeAggregator(node: RunnerNode): Record<string, unknown> {
  const data = objectValue(node.data);
  return {
    output_key: "aggregated_value",
    strategy: "first_non_empty",
    sources: [],
    ...objectValue(data.aggregator),
    ...objectValue(node.aggregator),
  };
}

function normalizeAggregatorSources(sources: unknown): Record<string, unknown>[] {
  if (!Array.isArray(sources)) return [];
  return sources
    .map((source) => objectValue(source))
    .filter((source) => String(source.expression || objectValue(source.variable).name || "").trim());
}

function variableToAggregatorSource(variable: RunnerVariableOption): Record<string, unknown> {
  const expression = String(variable.expression || "").trim();
  const valueSource = variableToValueSource(variable);
  return {
    expression,
    variable: objectValue(valueSource.variable),
  };
}

function ensureAggregatorOutput(outputs: Record<string, unknown>[], outputKey: string) {
  if (!outputKey) return outputs;
  if (outputs.some((output) => String(output.key || "").trim() === outputKey)) return outputs;
  return [
    ...outputs,
    {
      key: outputKey,
      type: "any",
      description: "变量聚合输出",
    },
  ];
}

function PublishReviewModal({ workflow, onClose, onPublished }: { workflow: Workflow; onClose: () => void; onPublished: (payload: Partial<Workflow>) => void }) {
  const [note, setNote] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const validation = workflow.validation_result || workflow.validationResult || { valid: false, errors: [], warnings: [] };
  const validatedHash = workflow.validated_graph_hash || workflow.validatedGraphHash || "";
  const dryHash = workflow.dry_run_graph_hash || workflow.dryRunGraphHash || "";
  const diffSummary = workflow.diff_summary || workflow.diffSummary || {};
  const semanticChanges = Array.isArray(diffSummary.semantic_changes) ? diffSummary.semantic_changes : [];
  const riskSummary = workflow.risk_summary || workflow.riskSummary || {};
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
        <main className="publish-review-body">
          <section className="publish-review-card">
            <h3>校验结果</h3>
            <p>{validationPassed ? "校验通过" : "校验未通过或未提供结果"}</p>
            <ul>
              <li>状态：{workflow.status || "draft"}</li>
              <li>Validated hash：{validatedHash || "-"}</li>
              <li>Dry Run hash：{dryHash || "-"}</li>
              <li>风险：{riskSummary.level || "unknown"}</li>
            </ul>
          </section>
          <section className="publish-review-card" data-testid="publish-diff-summary">
            <h3>发布 Diff</h3>
            {semanticChanges.length ? (
              <ul>
                {semanticChanges.map((item: { title?: string; detail?: string; type?: string }, index: number) => (
                  <li key={`${item.title || item.type || "change"}-${index}`}>
                    <strong>{item.title || item.type || "变更"}</strong>
                    {item.detail ? <span>{item.detail}</span> : null}
                  </li>
                ))}
              </ul>
            ) : <p>暂无语义变更摘要，发布前请确认当前 graph 与上次版本差异。</p>}
          </section>
          <label className="publish-note-field"><span>发布说明</span><textarea data-testid="publish-note" value={note} placeholder="记录变更窗口、审批单或发布原因" onChange={(event) => setNote(event.target.value)} /></label>
          {disabledReason ? <p className="publish-review-warning">{disabledReason}</p> : null}
          {error ? <p className="publish-review-error" role="alert">{error}</p> : null}
        </main>
        <footer className="publish-review-footer"><button type="button" onClick={onClose}>取消</button><button type="button" className="primary" disabled={!canPublish} data-testid="publish-confirm" onClick={publish}><Rocket size={15} />确认发布</button></footer>
      </div>
    </section>
  );
}

function OpsManualCandidateModal({ workflow, graph, onClose }: { workflow: Workflow; graph: RunnerGraph; onClose: () => void }) {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState("");
  const name = workflowKey(workflow);
  const title = workflow.title || workflow.name || name;
  const status = workflow.status || "draft";
  const candidateTitle = `${title} 运维手册候选`;
  const manualPath = `/settings/ops-manuals?workflow=${encodeURIComponent(name)}`;
  const nodeCount = graph.nodes?.length || 0;
  const payload = {
    workflow_id: name,
    workflow_name: name,
    source_type: "runner_workflow",
    source_refs: [name],
    draft_manual: {
      title: candidateTitle,
      status: "draft",
      workflow_ref: { workflow_id: name },
      operation: { target_type: "runner_workflow", action: objectValue(graph.workflow).workflow_type || workflow.workflow_type || workflow.workflowType || "review_required" },
      required_context: { required_inputs: [], required_evidence: [] },
      preconditions: ["确认 Workflow 已完成校验或 Dry Run", "确认执行目标、权限和回滚窗口"],
      validation: ["复核候选手册内容", "绑定执行记录后再进入已验证手册"],
      cannot_use_when: ["Workflow 尚未明确适用范围", "缺少后续复核"],
      document_markdown: `# ${candidateTitle}\n\n由 Runner Workflow ${name} 准备，只读候选内容需在运维手册页审核后使用。`,
    },
  };

  async function prepareCandidate() {
    if (loading || result) return;
    setLoading(true);
    setError("");
    try {
      const response = await requestJson("/api/v1/ops-manuals/candidates/prepare", { method: "POST", body: JSON.stringify(payload) });
      setResult(objectValue(response));
    } catch (cause) {
      setError(errorMessage(cause, "候选手册准备失败"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="publish-review-backdrop" data-testid="runner-ops-manual-modal">
      <div className="publish-review-modal" role="dialog" aria-modal="true" aria-label="准备运维手册候选">
        <header className="publish-review-head">
          <div><p>OPS MANUAL CANDIDATE</p><h2>准备运维手册候选</h2></div>
          <button type="button" className="workflow-icon-button" aria-label="关闭" onClick={onClose}><X size={16} /></button>
        </header>
        <main className="publish-review-body">
          <section className="publish-review-card">
            <h3>确认范围</h3>
            <p>将为当前 Runner Workflow 准备一个待审核 OpsManual 候选，不会发布或替换已验证手册。</p>
            <ul>
              <li>Workflow：{title}</li>
              <li>状态：{status}</li>
              <li>绑定：只绑定 1 个 Runner Workflow</li>
              <li>节点数：{nodeCount}</li>
            </ul>
          </section>
          <section className="publish-review-card">
            <h3>只读预览</h3>
            <p>{candidateTitle}</p>
            <ul>
              <li>AI Chat 引用键：ops_manual_candidate:{name}</li>
              <li>运维手册页：{manualPath}</li>
              <li>后续仍需在运维手册页审核、补全适用范围和验证记录。</li>
            </ul>
          </section>
          <section className="publish-review-card">
            <h3>检查清单</h3>
            <ul>
              <li>确认候选只来自当前选中的 Workflow。</li>
              <li>确认候选不会自动发布，必须经过验证和发布检查。</li>
              <li>确认手册页和 AI Chat 可用该引用定位候选。</li>
            </ul>
          </section>
          {result ? <p className="publish-review-warning">已准备候选：{String(result.id || result.candidate_id || "pending_review")}</p> : null}
          {error ? <p className="publish-review-error" role="alert">{error}</p> : null}
        </main>
        <footer className="publish-review-footer">
          <button type="button" onClick={onClose}>取消</button>
          <button type="button" className="primary" data-testid="runner-ops-manual-prepare" disabled={loading || Boolean(result)} onClick={() => void prepareCandidate()}>
            <BookOpen size={15} />{loading ? "准备中" : result ? "已准备候选" : "准备候选"}
          </button>
        </footer>
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
  const [runDrawerMode, setRunDrawerMode] = useState<"history" | "node">("history");
  const [runEvents, setRunEvents] = useState<Record<string, unknown>[]>([]);
  const [runRecords, setRunRecords] = useState<RunnerRunRecord[]>([]);
  const [runFocusNodeId, setRunFocusNodeId] = useState("");
  const [runSubmitting, setRunSubmitting] = useState(false);
  const [runLockedUntil, setRunLockedUntil] = useState(0);
  const [clockNow, setClockNow] = useState(() => Date.now());
  const [publishOpen, setPublishOpen] = useState(false);
  const [aiOpen, setAiOpen] = useState(false);
  const [opsManualOpen, setOpsManualOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [debugDockOpen, setDebugDockOpen] = useState(false);
  const [toolbarMoreOpen, setToolbarMoreOpen] = useState(false);
  const runInFlightRef = useRef(false);
  const runLockUntilRef = useRef(0);
  const importInputRef = useRef<HTMLInputElement | null>(null);
  const headerActionsRef = useRef<{ back: () => void; toolbar: (key: ToolbarActionKey) => void }>({ back: () => {}, toolbar: () => {} });

  const selectedWorkflow = useMemo(() => workflows.find((workflow) => workflowKey(workflow) === selectedWorkflowName) || null, [workflows, selectedWorkflowName]);
  const graph = normalizeGraph(selectedWorkflow?.graph || { workflow: { name: selectedWorkflowName || "draft" }, nodes: [], edges: [] }, selectedWorkflowName || "draft");
  const selectedNode = graph.nodes?.find((node) => node.id === selectedNodeId) || null;
  const runState = useMemo(() => reduceRunEvents(runEvents, createInitialRunState()), [runEvents]);
  const serverActionsDisabled = Boolean(apiNotice);
  const runCooldownRemainingMs = Math.max(0, runLockedUntil - clockNow);
  const runActionDisabled = runSubmitting || runState.status === "running" || runCooldownRemainingMs > 0;
  const runActionTitle = runActionDisabled
    ? runSubmitting || runState.status === "running"
      ? "运行中，8 秒内不能重复提交。"
      : `${Math.max(1, Math.ceil(runCooldownRemainingMs / 1000))} 秒后可再次运行（8 秒防重复提交）。`
    : "运行工作流";

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
    setRunFocusNodeId("");
    setRunDrawerOpen(false);
    setRunEvents([]);
    runInFlightRef.current = false;
    runLockUntilRef.current = 0;
    setRunSubmitting(false);
    setRunLockedUntil(0);
    setClockNow(Date.now());
    navigate(name ? `/runner/${encodeURIComponent(name)}` : "/runner");
  }, [navigate]);

  useEffect(() => { setSelectedWorkflowName(routeWorkflowName); }, [routeWorkflowName]);

  useEffect(() => {
    if (!runLockedUntil || Date.now() >= runLockedUntil) return undefined;
    const timer = window.setInterval(() => setClockNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [runLockedUntil]);

  useEffect(() => {
    if (!toolbarMoreOpen) return undefined;
    function closeToolbarMenu(event: PointerEvent) {
      const target = event.target as Element | null;
      if (target?.closest?.("[data-testid='runner-toolbar-more-container']")) return;
      setToolbarMoreOpen(false);
    }
    function closeToolbarMenuByKeyboard(event: KeyboardEvent) {
      if (event.key === "Escape") setToolbarMoreOpen(false);
    }
    document.addEventListener("pointerdown", closeToolbarMenu);
    document.addEventListener("keydown", closeToolbarMenuByKeyboard);
    return () => {
      document.removeEventListener("pointerdown", closeToolbarMenu);
      document.removeEventListener("keydown", closeToolbarMenuByKeyboard);
    };
  }, [toolbarMoreOpen]);

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
      const remoteKeys = new Set(remote.map(workflowKey));
      setWorkflows([...local.filter((workflow) => !remoteKeys.has(workflowKey(workflow))), ...remote]);
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

  function exportWorkflow() {
    if (!selectedWorkflow) return;
    const payload = workflowExportPayload(selectedWorkflow, graph);
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json;charset=utf-8" });
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${slugify(workflowKey(selectedWorkflow) || "workflow")}.workflow.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => window.URL.revokeObjectURL(url), 0);
  }

  async function importWorkflowFile(event: { currentTarget: HTMLInputElement }) {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file || !selectedWorkflow) return;
    try {
      const text = await file.text();
      const payload = JSON.parse(text);
      const importedGraph = workflowImportPayloadToGraph(payload, workflowKey(selectedWorkflow));
      updateGraph(importedGraph);
      setSelectedNodeId("");
      setRunEvents([]);
      setRunRecords([]);
      setRunDrawerOpen(false);
      setSaveState({ status: "pending", message: "已导入，未保存" });
    } catch (error) {
      setSaveState({ status: "failed", message: "导入失败", error: errorMessage(error) });
    }
  }

  async function deleteWorkflow(name: string) {
    const key = String(name || "").trim();
    if (!key) return;
    const workflow = workflows.find((item) => workflowKey(item) === key);
    if (!window.confirm(`确认删除工作流 ${workflow?.title || workflow?.name || key}？`)) return;
    try {
      if (workflow && !workflow.local_draft && !serverActionsDisabled) {
        await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(key)}`, { method: "DELETE" });
      }
      deleteLocalDraft(key);
      setWorkflows((current) => current.filter((item) => workflowKey(item) !== key));
      if (selectedWorkflowName === key) {
        selectWorkflow("");
      }
      setSaveState({ status: "saved", message: "工作流已删除", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "删除失败", error: errorMessage(error) });
    }
  }

  function updateGraph(nextGraph: RunnerGraph) {
    if (!selectedWorkflowName) return;
    const normalized = reconcileRunnerRuntimeConfig(normalizeGraph(nextGraph, selectedWorkflowName));
    upsertWorkflow(selectedWorkflowName, { graph: normalized, status: "draft", validated_graph_hash: "", dry_run_graph_hash: "", validation_result: { valid: false, errors: [], warnings: [] } });
    saveLocalDraft({ ...(selectedWorkflow || { name: selectedWorkflowName }), graph: normalized, status: "draft", local_draft: true });
    setSaveState({ status: "pending", message: "未保存" });
  }

  function deleteGraphNodeById(nodeId: string) {
    const nextGraph = removeGraphNode(graph, nodeId);
    if (nextGraph === graph) return;
    updateGraph(nextGraph);
    setSelectedNodeId("");
  }

  function upsertRunRecord(record: RunnerRunRecord) {
    setRunRecords((current) => [record, ...current.filter((item) => item.runId !== record.runId)].slice(0, 25));
  }

  async function flushWorkflow(reason: string) {
    if (!selectedWorkflow) return null;
    const name = workflowKey(selectedWorkflow);
    const normalized = reconcileRunnerRuntimeConfig(normalizeGraph(selectedWorkflow.graph, name));
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
    deleteLocalDraft(name);
    upsertWorkflow(name, { ...saved, graph: savedGraph, local_draft: false, status: saved.status || selectedWorkflow.status || "draft" });
    setSaveState({ status: "saved", message: "已保存", lastSavedAt: formatSaveTime() });
    return savedGraph;
  }

  async function ensureWorkflowPersisted(reason: string) {
    if (!selectedWorkflow) return null;
    const name = workflowKey(selectedWorkflow);
    if (selectedWorkflow.local_draft || saveState.status === "pending" || saveState.status === "local_draft") {
      return flushWorkflow(reason);
    }
    return normalizeGraph(selectedWorkflow.graph, name);
  }

  async function validateWorkflow() {
    if (!selectedWorkflow) return;
    const name = workflowKey(selectedWorkflow);
    try {
      const savedGraph = await ensureWorkflowPersisted("validate");
      if (!savedGraph) return;
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
      const savedGraph = await ensureWorkflowPersisted("dry-run");
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
    const submitStartedAt = Date.now();
    if (runInFlightRef.current || submitStartedAt < runLockUntilRef.current) return;
    runInFlightRef.current = true;
    runLockUntilRef.current = submitStartedAt + RUN_SUBMIT_COOLDOWN_MS;
    setRunSubmitting(true);
    setRunLockedUntil(runLockUntilRef.current);
    setClockNow(submitStartedAt);
    const name = workflowKey(selectedWorkflow);
    const initialGraph = normalizeGraph(selectedWorkflow.graph, name);
    let focusNodeId = nodeId || workflowStartNodeId(initialGraph) || firstRunnableNodeId(initialGraph);
    if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
    try {
      const savedGraph = await ensureWorkflowPersisted("run");
      if (!savedGraph || serverActionsDisabled) return;
      focusNodeId = nodeId || workflowStartNodeId(savedGraph) || firstRunnableNodeId(savedGraph) || focusNodeId;
      if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
      const response = await requestJson("/api/runner-studio/runs", { method: "POST", body: JSON.stringify({ workflow_name: name, graph: savedGraph, vars: {}, triggered_by: "ui", risk_acknowledged: true, ...(nodeId ? { node_id: nodeId, run_scope: "single_node" } : {}) }) });
      const runResponse = unwrapRunnerPayload(response) as Record<string, unknown>;
      const runId = String(runResponse.run_id || runResponse.runId || "");
      if (!runId) throw new Error("Runner did not return a run_id");
      const now = new Date().toISOString();
      const queuedEvents = mapRunnerRunEventsToGraph([
        { type: "run_queued", run_id: runId, workflow: name, status: "queued", timestamp: now },
        ...(focusNodeId ? [{ type: "node.started", run_id: runId, node_id: focusNodeId, status: "running", timestamp: now, message: "运行中" }] : []),
      ], savedGraph) as Record<string, unknown>[];
      setRunEvents(queuedEvents);
      upsertRunRecord(buildRunRecord(runId, queuedEvents, { status: "running", message: "运行中", startedAt: now }));
      setSaveState({ status: "saved", message: "运行已提交", lastSavedAt: formatSaveTime() });
      await delay(350);
      try {
        const events = await waitForRunHistory(runId, savedGraph);
        const normalizedEvents = (events.length ? events : queuedEvents).map((event: Record<string, unknown>) => ({ ...event, run_id: event.run_id || runId }));
        setRunEvents(normalizedEvents);
        upsertRunRecord(buildRunRecord(runId, normalizedEvents));
        setSaveState({ status: "saved", message: runStatusMessage(finalRunnerRunStatus(normalizedEvents)), lastSavedAt: formatSaveTime() });
      } catch (historyError) {
        const message = errorMessage(historyError, "运行历史读取失败");
        const failedEvents = [
          ...queuedEvents,
          { type: "run.failed", run_id: runId, status: "failed", message: `运行历史读取失败：${message}`, error: message, timestamp: new Date().toISOString() },
          ...(focusNodeId ? [{ type: "node.failed", run_id: runId, node_id: focusNodeId, status: "failed", message, error: message, timestamp: new Date().toISOString() }] : []),
        ];
        setRunEvents(failedEvents);
        upsertRunRecord(buildRunRecord(runId, failedEvents, { status: "failed", message }));
        setSaveState({ status: "failed", message: "运行状态刷新失败", error: message });
      }
    } catch (error) {
      const message = errorMessage(error);
      const now = new Date().toISOString();
      const runId = `submit-${Date.now()}`;
      if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
      const failedEvents = [
        { type: "run.failed", run_id: runId, status: "failed", message: `运行提交失败：${message}`, error: message, timestamp: now },
        ...(focusNodeId ? [{ type: "node.failed", run_id: runId, node_id: focusNodeId, status: "failed", message, error: message, timestamp: now }] : []),
      ];
      setRunEvents(failedEvents);
      upsertRunRecord(buildRunRecord(runId, failedEvents, { status: "failed", message }));
      setSaveState({ status: "failed", message: "运行失败", error: message });
    } finally {
      runInFlightRef.current = false;
      setRunSubmitting(false);
      setClockNow(Date.now());
    }
  }

  function handleToolbarAction(key: ToolbarActionKey) {
    setToolbarMoreOpen(false);
    if (key === "save") void flushWorkflow("manual");
    if (key === "validate") void validateWorkflow();
    if (key === "dry-run") void dryRunWorkflow();
    if (key === "run" && !runActionDisabled) void runWorkflow();
    if (key === "variables") setDebugDockOpen((value) => !value);
    if (key === "import") importInputRef.current?.click();
    if (key === "export") exportWorkflow();
    if (key === "run-details") {
      setRunDrawerMode("history");
      setRunDrawerOpen(true);
    }
    if (key === "publish") setPublishOpen(true);
    if (key === "ai-generate") setAiOpen(true);
    if (key === "ops-manual") setOpsManualOpen(true);
  }

  headerActionsRef.current = {
    back: () => selectWorkflow(""),
    toolbar: handleToolbarAction,
  };

  const runnerHeaderContent = useMemo(() => selectedWorkflow ? (
    <div className="runner-studio-topbar" data-testid="runner-studio-topbar">
      <div className="runner-studio-current-workflow"><button type="button" className="runner-studio-back-button" data-testid="runner-back-to-library" onClick={() => headerActionsRef.current.back()}><ArrowLeft size={15} />工作流</button><h1>{selectedWorkflow.title || selectedWorkflow.name}</h1><span className="runner-studio-status">{selectedWorkflow.status || "draft"}</span><span className={`runner-studio-save-state status-${saveState.status || "idle"}`} data-testid="runner-save-state">{saveStateLabel(saveState)}</span></div>
      <div className="runner-studio-toolbar-actions" aria-label="Runner Studio 操作">
        {PRIMARY_TOOLBAR_ACTIONS.map(([key, label, Icon]) => {
          const isRunAction = key === "run";
          return <button key={key} type="button" className={`runner-studio-action-button ${isRunAction ? "primary" : ""}`} data-testid={`runner-toolbar-${key}`} disabled={isRunAction ? runActionDisabled : false} title={isRunAction ? runActionTitle : undefined} onClick={() => headerActionsRef.current.toolbar(key)}><Icon size={15} />{label}</button>;
        })}
        <div className="runner-toolbar-more" data-testid="runner-toolbar-more-container">
          <button type="button" className="runner-studio-action-button" data-testid="runner-toolbar-more" aria-expanded={toolbarMoreOpen} aria-haspopup="menu" onClick={() => setToolbarMoreOpen((value) => !value)}><MoreHorizontal size={15} />更多</button>
          {toolbarMoreOpen ? (
            <div className="runner-toolbar-more-menu" role="menu" data-testid="runner-toolbar-more-menu">
              {SECONDARY_TOOLBAR_ACTIONS.map(([key, label, Icon]) => <button key={key} type="button" role="menuitem" className="runner-toolbar-more-item" data-testid={`runner-toolbar-${key}`} onClick={() => headerActionsRef.current.toolbar(key)}><Icon size={15} />{label}</button>)}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  ) : null, [runActionDisabled, runActionTitle, saveState.error, saveState.lastSavedAt, saveState.message, saveState.status, selectedWorkflow?.name, selectedWorkflow?.status, selectedWorkflow?.title, toolbarMoreOpen]);

  const runnerLibraryHeaderActions = useMemo(
    () => selectedWorkflow ? null : (
      <button type="button" className="runner-studio-action-button primary" data-testid="runner-open-manager" onClick={() => setManagerOpen(true)} disabled={loading}>管理工作流</button>
    ),
    [loading, selectedWorkflow],
  );

  useRegisterAppShellHeader(runnerHeaderContent);
  useRegisterAppShellPageChrome({
    title: selectedWorkflow ? null : "工作流",
    description: selectedWorkflow ? null : "Workflow 编排",
    actions: runnerLibraryHeaderActions,
  });

  return (
    <section className="runner-studio-page" data-testid="runner-studio-page">
      <input
        ref={importInputRef}
        type="file"
        accept=".json,application/json"
        data-testid="runner-workflow-import-input"
        style={{ display: "none" }}
        onChange={(event) => void importWorkflowFile(event)}
      />
      {apiNotice && !apiNoticeDismissed ? <section className="runner-studio-api-notice" data-testid="runner-studio-api-notice"><strong>{apiNotice.title}</strong><span>{apiNotice.message} {apiNotice.hint}</span><button type="button" data-testid="runner-api-notice-close" onClick={() => setApiNoticeDismissed(true)}>关闭</button></section> : null}
      <section className={`runner-studio-shell ${fullscreen ? "fullscreen" : ""}`} data-testid="runner-studio-shell" aria-busy={loading ? "true" : "false"}>
        {!selectedWorkflow ? <WorkflowLibrary workflows={workflows} onSelect={selectWorkflow} onDelete={(name) => void deleteWorkflow(name)} /> : (
          <>
            <div className={`runner-studio-workspace ${selectedNode ? "with-node-panel" : ""}`}>
              <section className="runner-studio-main"><section className="runner-studio-canvas" aria-label="工作流画布" data-testid="runner-studio-canvas"><RunnerCanvas graph={graph} actions={actions} runState={runState} focusNodeId={runFocusNodeId} selectedNodeId={selectedNodeId} fullscreen={fullscreen} onUpdateGraph={updateGraph} onSelectNode={setSelectedNodeId} onOpenNodeConfig={setSelectedNodeId} onDeleteNode={deleteGraphNodeById} onNodeAction={() => {}} onToggleFullscreen={() => setFullscreen((value) => !value)} /></section></section>
              {selectedNode ? <section className="runner-node-panel-modal" role="dialog" aria-modal="false" aria-label="节点配置面板" data-testid="runner-node-panel-modal"><RunnerNodePanel node={selectedNode} graph={graph} runState={runState} onClose={() => setSelectedNodeId("")} onApply={(node) => { updateGraph({ ...graph, nodes: (graph.nodes || []).map((item) => item.id === node.id ? node : item) }); setSelectedNodeId(node.id); }} onRunNode={(nodeId) => void runWorkflow(nodeId)} onDelete={deleteGraphNodeById} /></section> : null}
              {debugDockOpen ? <RunnerDebugDock graph={graph} state={runState} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} onClose={() => setDebugDockOpen(false)} /> : null}
            </div>
          </>
        )}
        {selectedWorkflow && runDrawerOpen ? <section className="runner-studio-run-drawer-backdrop" role="dialog" aria-modal="true" aria-label={runDrawerMode === "history" ? "运行详情" : "节点运行详情"} data-testid="runner-run-drawer"><aside className="runner-studio-run-drawer-panel"><header className="runner-studio-run-drawer-head"><div><strong>{runDrawerMode === "history" ? "运行详情" : "节点运行详情"}</strong><span>{runDrawerMode === "history" ? "每次运行一行记录，可快速定位失败步骤。" : "当前节点的上次运行、日志和变量输出。"}</span></div><button type="button" className="runner-run-drawer-close" data-testid="runner-run-drawer-close" aria-label="关闭运行详情" onClick={() => setRunDrawerOpen(false)}><X size={18} /></button></header><div className="runner-studio-run-drawer-body">{runDrawerMode === "history" ? <RunnerRunHistoryPanel records={runRecords} currentState={runState} currentEvents={runEvents} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} /> : <RunnerNodeRunDetails state={runState} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} />}</div></aside></section> : null}
      </section>
      {managerOpen ? <WorkflowManagerModal workflows={workflows} onClose={() => setManagerOpen(false)} onCreateBlank={createBlankWorkflow} onSelect={(name) => { setManagerOpen(false); selectWorkflow(name); }} onDelete={(name) => void deleteWorkflow(name)} /> : null}
      {publishOpen && selectedWorkflow ? <PublishReviewModal workflow={selectedWorkflow} onClose={() => setPublishOpen(false)} onPublished={(payload) => { upsertWorkflow(selectedWorkflowName, { ...payload, status: payload.status || "published" }); setPublishOpen(false); }} /> : null}
      {opsManualOpen && selectedWorkflow ? <OpsManualCandidateModal workflow={selectedWorkflow} graph={graph} onClose={() => setOpsManualOpen(false)} /> : null}
      {aiOpen && selectedWorkflow ? <AiAssistantModal workflow={selectedWorkflow} graph={graph} onClose={() => setAiOpen(false)} onApply={(nextGraph) => { updateGraph(nextGraph); setAiOpen(false); }} /> : null}
    </section>
  );
}
