import { useCallback, useEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  BookOpen,
  ChevronLeft,
  ChevronRight,
  Plus,
  Rocket,
  Search,
  Trash2,
  X,
} from "lucide-react";

import { useRegisterAppShellHeader, useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import {
  dryRunAuthoritativeWorkflowTarget,
  runPublishedAuthoritativeWorkflow,
} from "@/api/runnerStudioClient";
import { FALLBACK_RUNNER_ACTIONS } from "@/components/runner/fallbackActionCatalog";
import { firstRunnableNodeId } from "@/components/runner/runnerRunVisualState";
import { extractRunnerRunEvents, finalRunnerRunStatus, isRunnerRunHistoryTerminal, mapRunnerRunEventsToGraph, unwrapRunnerPayload } from "@/components/runner/runEventHistory";
import { createInitialRunState, reduceRunEvents } from "@/components/runner/runStateReducer";
import { type RunnerEdge, type RunnerGraph, type RunnerNode } from "@/components/runner/canvasGraphAdapter";
import { WorkflowAiTransportSlot } from "@/runner/WorkflowAiTransportSlot";
import type { WorkflowActionMenuItem } from "@/runner/WorkflowActionMenu";
import { requestWorkflowContextDockTab } from "@/runner/WorkflowContextDock";
import { WorkflowEditorShell, workflowCommandForGraphChange } from "@/runner/WorkflowEditorShell";
import type { WorkflowSavePresentation } from "@/runner/WorkflowLifecycleHeader";
import type { WorkflowRunSessionRecord } from "@/runner/WorkflowRunInspector";
import {
  readWorkflowValidationFocusEvent,
  WORKFLOW_VALIDATION_FOCUS_EVENT,
} from "@/runner/WorkflowValidationPanel";
import { parseWorkflowCapabilitySnapshot } from "@/runner/workflowFeatureFlags";
import {
  WorkflowWorkspaceProvider,
  useWorkflowWorkspace,
  type WorkflowAuthoritativeProjection,
  type WorkflowManualCommand,
} from "@/runner/workspace/WorkflowWorkspaceProvider";
import type { WorkflowJsonValue, WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";
import type { WorkflowPrimaryActionTarget } from "@/runner/workspace/workflowInteractionContract";
import { RunnerNodeInspector } from "@/runner/RunnerNodeInspector";
import {
  DEFAULT_HOST_GROUPS,
  normalizeTargetLabels,
  objectValue,
  shouldFillDefaultTargets,
  workflowDefaultTargetLabels,
  workflowHostGroups,
  type RunnerHostGroup,
} from "@/runner/workflowNodeInspectorModel";
import "@/components/runner/runnerStudio.css";
import "@/runner/workflowWorkspace.css";

export type RunnerAction = { action?: string; name?: string; label?: string; title?: string; category?: string; defaults?: Record<string, unknown>; [key: string]: unknown };
type Workflow = {
  id?: string;
  name: string;
  title?: string;
  version?: string;
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
  updated_at?: string;
  updatedAt?: string;
  modified_at?: string;
  modifiedAt?: string;
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
};

type SaveState = { status: string; message?: string; lastSavedAt?: string; error?: string };

type ApiNotice = { title: string; message: string; hint?: string } | null;
type RunnerStudioAppSnapshot = { config?: { workflowWorkspace?: WorkflowAuthoritativeProjection; workflowCapabilities?: unknown } };

type RunState = ReturnType<typeof createInitialRunState>;
type PublishedWorkflowRunTarget = Extract<WorkflowPrimaryActionTarget, { kind: "published_revision" }>;
type RunnerRunRetryTarget =
  | {
      kind: "draft_graph";
      request: {
        workflow_name: string;
        graph: RunnerGraph;
        vars: Record<string, unknown>;
        triggered_by: string;
        risk_acknowledged: boolean;
        node_id?: string;
        run_scope?: "single_node";
      };
      focusNodeId: string;
      targetLabel: string;
    }
  | {
      kind: "published_revision";
      target: PublishedWorkflowRunTarget;
      eventGraph: RunnerGraph;
      vars: Record<string, unknown>;
      triggeredBy: string;
      riskAcknowledged: boolean;
      targetLabel: string;
    };
type RunnerRunRecord = {
  runId: string;
  status: string;
  message: string;
  targetLabel?: string;
  retryTarget?: RunnerRunRetryTarget;
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
const FALLBACK_ACTIONS: RunnerAction[] = FALLBACK_RUNNER_ACTIONS
  .filter((action) => action.action !== "wait.event")
  .map((action) => {
    if (action.action === "condition.branch") return { ...action, action: "condition.evaluate" };
    if (action.action === "approval.wait") return { ...action, action: "manual.approval" };
    return action;
  });

function runnerActionCatalogRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function runnerActionCatalogIdentity(action: RunnerAction) {
  return String(action.action || action.name || action.label || action.title || "").trim();
}

// The legacy Runner catalog describes every known action, not every action
// enabled by the current runtime. Normalize the server-owned execution policy
// onto each item so the palette can fail closed without maintaining a browser
// allowlist. The access-aware shape is accepted for the Workflow Edit catalog.
export function normalizeRunnerActionCatalog(payload: unknown): RunnerAction[] {
  const root = runnerActionCatalogRecord(payload);
  if (!root) return [];
  const capabilities = runnerActionCatalogRecord(root.capabilities);
  const allowedValues = Array.isArray(capabilities?.allowed_actions) ? capabilities.allowed_actions : [];
  const allowedActions = new Set(allowedValues.map((value) => String(value || "").trim()).filter(Boolean));
  const rawItems = Array.isArray(root.items) ? root.items : Array.isArray(root.actions) ? root.actions : [];
  return rawItems.flatMap((rawItem) => {
    const item = runnerActionCatalogRecord(rawItem);
    if (!item) return [];
    const nestedSpec = runnerActionCatalogRecord(item.spec);
    const spec = (nestedSpec || item) as RunnerAction;
    const access = runnerActionCatalogRecord(item.access);
    const executionAvailable = access
      ? access.allowed === true
      : allowedActions.has(runnerActionCatalogIdentity(spec));
    return [{
      ...spec,
      ...(access ? { access } : {}),
      execution_available: executionAvailable,
    }];
  });
}

const LOCAL_DRAFTS_KEY = "runner.studio.localDrafts";

const RUN_SUBMIT_COOLDOWN_MS = 8000;
const WORKFLOW_EXPORT_KIND = "aiops.runner.workflow";
const WORKFLOW_EXPORT_VERSION = 1;
const IMPORT_LAYOUT_START_X = 80;
const IMPORT_LAYOUT_START_Y = 160;
const IMPORT_LAYOUT_COLUMN_GAP = 320;
const IMPORT_LAYOUT_ROW_GAP = 140;
const WORKFLOW_EXPORT_KEYS = ["name", "title", "description", "workflow_type", "workflowType", "category", "inventory", "inputs", "outputs", "variables", "vars"];
const NODE_EXPORT_KEYS = ["id", "type", "label", "description", "ports", "step", "inputs", "outputs", "risk", "ui", "condition", "aggregator", "branches"];
const WORKFLOW_LIBRARY_PAGE_SIZE = 6;
const NO_AI_HIGHLIGHTED_NODE_IDS: string[] = [];

export function workflowOperationForCandidateGraphChange(previous: RunnerGraph, next: RunnerGraph): WorkflowOperation {
  return workflowOperationForCandidateCommand(workflowCommandForGraphChange(previous, next));
}

function workflowOperationForCandidateCommand(command: WorkflowManualCommand): WorkflowOperation {
  switch (command.kind) {
    case "update_node":
      return { op: "update_node", nodeId: command.nodeId, set: workflowJsonRecord(command.nodePatch, "node patch") };
    case "move":
      return {
        op: "update_layout",
        layout: {
          nodes: command.layout.nodes.map((item) => ({
            nodeId: requiredCandidateString(item.nodeId, "layout nodeId"),
            position: candidatePosition(item.position, "layout position"),
          })),
          ...(command.layout.viewport ? {
            viewport: {
              x: finiteCandidateNumber(command.layout.viewport.x, "viewport x"),
              y: finiteCandidateNumber(command.layout.viewport.y, "viewport y"),
              zoom: finiteCandidateNumber(command.layout.viewport.zoom, "viewport zoom"),
            },
          } : {}),
        },
      };
    case "connect_edge": {
      const edge = command.edge;
      return {
        op: "add_edge",
        edge: {
          ...(edge.id ? { id: requiredCandidateString(edge.id, "edge id") } : {}),
          source: requiredCandidateString(edge.source, "edge source"),
          ...(edge.sourcePort ? { sourcePort: requiredCandidateString(edge.sourcePort, "edge sourcePort") } : {}),
          target: requiredCandidateString(edge.target, "edge target"),
          ...(edge.targetPort ? { targetPort: requiredCandidateString(edge.targetPort, "edge targetPort") } : {}),
          ...(edge.kind ? { kind: requiredCandidateString(edge.kind, "edge kind") } : {}),
          ...(edge.condition ? { condition: requiredCandidateString(edge.condition, "edge condition") } : {}),
        },
      };
    }
    case "update_edge":
      return { op: "update_edge", edgeId: command.edgeId, set: workflowJsonRecord(command.edgePatch, "edge patch") };
    case "disconnect_edge":
      return { op: "delete_edge", edgeId: command.edgeId };
    default:
      throw new Error(`candidate canvas change ${command.kind} is not supported`);
  }
}

function workflowJsonRecord(value: Record<string, unknown>, field: string): Record<string, WorkflowJsonValue> {
  const cloned = JSON.parse(JSON.stringify(value)) as unknown;
  if (!cloned || typeof cloned !== "object" || Array.isArray(cloned) || Object.keys(cloned).length === 0) {
    throw new TypeError(`candidate ${field} is required`);
  }
  return cloned as Record<string, WorkflowJsonValue>;
}

function candidatePosition(value: { x: number; y: number }, field: string): { x: number; y: number } {
  return {
    x: finiteCandidateNumber(value?.x, `${field} x`),
    y: finiteCandidateNumber(value?.y, `${field} y`),
  };
}

function requiredCandidateString(value: unknown, field: string): string {
  if (typeof value !== "string" || !value || value !== value.trim()) throw new TypeError(`candidate ${field} is required`);
  return value;
}

function finiteCandidateNumber(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isFinite(value)) throw new TypeError(`candidate ${field} must be finite`);
  return value;
}

function WorkflowEditorShellWithCandidateRevision(props: ComponentProps<typeof WorkflowEditorShell>) {
  const workspace = useWorkflowWorkspace();
  const { reviseCandidate } = workspace;
  const onCommandStateChange = props.onCommandStateChange;
  const onReviseCandidateGraph = useCallback((previous: RunnerGraph, next: RunnerGraph) => {
    const operation = workflowOperationForCandidateGraphChange(previous, next);
    onCommandStateChange?.({ status: "pending", message: "正在更新候选版本" });
    return reviseCandidate(operation, { intent: candidateRevisionIntent(operation) })
      .then(() => onCommandStateChange?.({ status: "accepted", message: "候选版本已更新，尚未应用" }));
  }, [onCommandStateChange, reviseCandidate]);
  const undoTarget = props.durableHistoryEntries?.find((entry) => entry.reversible)?.transactionId;
  const redoTarget = props.durableHistoryEntries?.find((entry) => entry.redoable)?.transactionId;
  return (
    <WorkflowEditorShell
      {...props}
      onReviseCandidateGraph={onReviseCandidateGraph}
      onUndoHistory={props.onUndoHistory || workspace.undoHistory}
      onRedoHistory={props.onRedoHistory || workspace.redoHistory}
      onCanvasUndo={props.onCanvasUndo || (undoTarget ? () => { void workspace.undoHistory(undoTarget); } : undefined)}
      onCanvasRedo={props.onCanvasRedo || (redoTarget ? () => { void workspace.redoHistory(redoTarget); } : undefined)}
    />
  );
}

function candidateRevisionIntent(operation: WorkflowOperation): string {
  switch (operation.op) {
    case "update_node": return "用户调整候选节点参数";
    case "update_layout": return "用户调整候选节点位置";
    case "add_edge": return "用户连接候选节点";
    case "update_edge": return "用户重新连接候选节点";
    case "delete_edge": return "用户断开候选节点连接";
    default: return "用户调整候选版本";
  }
}

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

function snapshotRunnerGraph(graph: RunnerGraph): RunnerGraph {
  return JSON.parse(JSON.stringify(graph)) as RunnerGraph;
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
    targetLabel: String(fallback.targetLabel || ""),
    ...(fallback.retryTarget ? { retryTarget: fallback.retryTarget } : {}),
    startedAt: String(state.startedAt || fallback.startedAt || new Date().toISOString()),
    finishedAt: String(state.finishedAt || fallback.finishedAt || ""),
    ...traceability,
    events: events.map((event) => ({ ...event, run_id: event.run_id || id })),
    state: { ...state, runId: state.runId || id, status: derivedStatus },
  };
}

function runTraceabilityFromEvents(events: Record<string, unknown>[], fallback: Partial<RunnerRunRecord>) {
  const sources = [fallback as Record<string, unknown>, ...events.map((event) => objectValue(event))];
  const failureSources = [
    fallback as Record<string, unknown>,
    ...events
      .map((event) => objectValue(event))
      .filter((event) => {
        const status = String(event.status || "").toLowerCase();
        const type = String(event.type || "").toLowerCase();
        return status === "failed" || type.includes("failed") || Boolean(event.error);
      }),
  ];
  return {
    caseId: String(firstScalarValue(sources, ["caseId", "case_id", "incidentId", "incident_id"]) || ""),
    hostLeaseId: String(firstScalarValue(sources, ["hostLeaseId", "host_lease_id", "leaseId", "lease_id"]) || ""),
    failedStep: String(firstScalarValue(failureSources, ["failedStep", "failed_step", "step", "nodeId", "node_id"]) || ""),
    failedReason: String(firstScalarValue(failureSources, ["failedReason", "failed_reason", "reason", "error"]) || ""),
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

function workflowRunSessionRecords(
  records: RunnerRunRecord[],
  currentState: RunState,
  currentEvents: Record<string, unknown>[],
): WorkflowRunSessionRecord[] {
  const current = runRecordFromState(currentState, currentEvents);
  const storedCurrent = current
    ? records.find((record) => record.runId === current.runId)
    : undefined;
  const hydratedCurrent = current
    ? {
      ...storedCurrent,
      ...current,
      targetLabel: storedCurrent?.targetLabel || current.targetLabel,
    }
    : null;
  const merged = hydratedCurrent
    ? [hydratedCurrent, ...records.filter((record) => record.runId !== hydratedCurrent.runId)]
    : records;
  return merged.slice(0, 25).flatMap((record) => {
    const startedAt = String(record.startedAt || "").trim();
    if (!record.runId || Number.isNaN(Date.parse(startedAt))) return [];
    const normalizedStatus = String(record.status || "").toLowerCase();
    const status: WorkflowRunSessionRecord["status"] = ["success", "completed"].includes(normalizedStatus)
      ? "completed"
      : normalizedStatus === "failed"
        ? "failed"
        : ["cancelled", "canceled"].includes(normalizedStatus)
          ? "canceled"
          : "running";
    const finishedAt = Date.parse(String(record.finishedAt || ""));
    const startedAtMs = Date.parse(startedAt);
    const path = record.events
      .map((event) => String(event.node_id || event.nodeId || "").trim())
      .filter((nodeId, index, values) => nodeId && values.indexOf(nodeId) === index);
    const logs = record.events.flatMap((event) => {
      const values = [event.message, event.stdout, event.stderr]
        .map((value) => String(value || "").trim())
        .filter(Boolean);
      return values.slice(0, 3);
    }).slice(0, 100);
    const targetLabel = record.targetLabel || (record.failedStep ? `步骤 ${record.failedStep}` : "当前工作流");
    return [{
      id: record.runId,
      status,
      targetLabel,
      startedAt,
      ...(Number.isFinite(finishedAt) && finishedAt >= startedAtMs ? { durationMs: finishedAt - startedAtMs } : {}),
      ...(path.length ? { path } : {}),
      ...(logs.length ? { logs } : {}),
      ...(status === "failed" && record.retryTarget ? {
        failure: {
          action: targetLabel,
          savedState: "已保存版本未改变",
          message: record.failedReason || record.message || "运行失败，请核对日志后重试",
        },
      } : {}),
    }];
  });
}

function errorMessage(error: unknown, fallback = "运行失败") {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
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
    hint: "请确认 ai-server 已用最新 start.sh 启动，Runner Studio API 会随 ai-server 默认挂载。",
  };
}

function formatSaveTime(date = new Date()) {
  return date.toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function slugify(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "runner-blank";
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

function formatWorkflowModifiedTime(value: unknown) {
  const raw = String(value || "").trim();
  if (!raw) return "未知";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
  });
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
    updatedAtLabel: formatWorkflowModifiedTime(firstScalarValue(sources, ["updated_at", "updatedAt", "modified_at", "modifiedAt"])),
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

function workflowMatchesSearch(workflow: Workflow, query: string) {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) return true;
  const haystack = [workflow.name, workflow.id, workflow.title].filter(Boolean).join(" ").toLowerCase();
  return normalizedQuery.split(/\s+/).every((token) => haystack.includes(token));
}

function nextBlankWorkflowName(workflows: Workflow[]) {
  const names = new Set(workflows.map((workflow) => workflowKey(workflow)));
  if (!names.has("runner-blank")) return "runner-blank";
  for (let index = 2; index < 1000; index += 1) {
    const name = `runner-blank-${index}`;
    if (!names.has(name)) return name;
  }
  return `runner-blank-${Date.now()}`;
}

function WorkflowLibrary({
  workflows,
  totalCount,
  filteredCount,
  searchQuery,
  page,
  pageCount,
  pageSize,
  onPageChange,
  onSelect,
  onDelete,
}: {
  workflows: Workflow[];
  totalCount: number;
  filteredCount: number;
  searchQuery: string;
  page: number;
  pageCount: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onSelect: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const startItem = filteredCount ? (page - 1) * pageSize + 1 : 0;
  const endItem = filteredCount ? Math.min(filteredCount, startItem + workflows.length - 1) : 0;
  return (
    <section className="runner-workflow-library" data-testid="runner-workflow-library">
      <div className="workflow-quick-list">
        {totalCount === 0 ? <p className="runner-studio-empty">暂无工作流，打开管理器创建一个 blank workflow。</p> : null}
        {totalCount > 0 && filteredCount === 0 ? <p className="runner-studio-empty">没有匹配“{searchQuery}”的工作流。</p> : null}
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
                  <em>最后修改：{context.updatedAtLabel}</em>
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
      {filteredCount > 0 ? (
        <footer className="runner-workflow-pagination" data-testid="runner-workflow-pagination">
          <span>{filteredCount} 个结果 · 显示 {startItem}-{endItem}</span>
          <div>
            <button type="button" data-testid="runner-workflow-page-prev" disabled={page <= 1} onClick={() => onPageChange(page - 1)} aria-label="上一页"><ChevronLeft size={15} />上一页</button>
            <strong>第 {page} / {pageCount} 页</strong>
            <button type="button" data-testid="runner-workflow-page-next" disabled={page >= pageCount} onClick={() => onPageChange(page + 1)} aria-label="下一页">下一页<ChevronRight size={15} /></button>
          </div>
        </footer>
      ) : null}
    </section>
  );
}

function WorkflowDeleteConfirmDialog({ workflow, onCancel, onConfirm }: { workflow: Workflow; onCancel: () => void; onConfirm: () => void }) {
  const title = workflow.title || workflow.name;
  return (
    <section className="workflow-delete-confirm-backdrop" data-testid="workflow-delete-confirm">
      <div className="workflow-delete-confirm-modal" role="dialog" aria-modal="true" aria-label="确认删除工作流">
        <header>
          <div>
            <p>DELETE WORKFLOW</p>
            <h2>确认删除工作流</h2>
          </div>
          <button type="button" className="workflow-icon-button" aria-label="关闭" onClick={onCancel}><X size={16} /></button>
        </header>
        <main>
          <strong>{title}</strong>
          <span>删除后会从工作流列表移除，并同步删除本地草稿缓存。</span>
        </main>
        <footer>
          <button type="button" onClick={onCancel}>取消</button>
          <button type="button" className="danger" data-testid="workflow-delete-confirm-submit" onClick={onConfirm}><Trash2 size={15} />确认删除</button>
        </footer>
      </div>
    </section>
  );
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function renderTextList(value: unknown) {
  const items = arrayValue(value).map((item) => String(item || "").trim()).filter(Boolean);
  return items.length ? <ul>{items.map((item, index) => <li key={`${item}-${index}`}>{item}</li>)}</ul> : <p>暂无。</p>;
}

function renderIssueList(title: string, value: unknown[]) {
  const issues = value.map(objectValue).filter((item) => item.message || item.code);
  if (!issues.length) return null;
  return (
    <div className="runner-ops-manual-issues">
      <p>{title}</p>
      <ul>
        {issues.map((issue, index) => (
          <li key={`${String(issue.code || title)}-${index}`}>
            {String(issue.message || issue.code)}
          </li>
        ))}
      </ul>
    </div>
  );
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
  const defaultTargets = workflowDefaultTargetLabels(groups);
  const nodes = (normalized.nodes || []).map((node) => {
    let nextNode = node;
    if (node.id === startNode?.id) nextNode = { ...nextNode, ui: { ...objectValue(nextNode.ui), host_groups: groups } };
    if (shouldFillDefaultTargets(nextNode)) nextNode = { ...nextNode, step: { ...(nextNode.step || {}), targets: defaultTargets } };
    return nextNode;
  });
  return {
    ...normalized,
    workflow: { ...objectValue(normalized.workflow), inventory: inventoryFromHostGroups(normalized, groups) },
    nodes,
  };
}

function PublishReviewModal({
  workflow,
  target,
  onClose,
  onPublished,
}: {
  workflow: Workflow;
  target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>;
  onClose: () => void;
  onPublished: (payload: Partial<Workflow>) => void;
}) {
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
  const disabledReason = !validatedHash ? "缺少当前 validated_graph_hash，发布前必须先校验当前 graph。" : dryHash !== validatedHash ? "发布前检查未通过或已过期，发布前必须重新检查当前 graph。" : !validationPassed ? "校验未通过，修复错误后才能发布。" : !note.trim() ? "发布说明不能为空。" : "";
  const canPublish = !disabledReason && !loading;
  async function publish() {
    if (!canPublish) return;
    setLoading(true);
    setError("");
    try {
      const payload = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(workflowKey(workflow))}/publish`, {
        method: "POST",
        body: JSON.stringify({
          save_note: note.trim(),
          validated_graph_hash: validatedHash,
          dry_run_graph_hash: dryHash,
          validation_result: validation,
          expected_resource_version: target.resourceVersion,
          expected_semantic_hash: target.semanticHash,
        }),
      });
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
              <li>发布前检查 hash：{dryHash || "-"}</li>
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
  const version = String(workflow.version || objectValue(graph.workflow).version || "");
  const candidate = objectValue(result?.candidate);
  const manual = objectValue(candidate.proposed_manual || candidate.proposedManual);
  const workflowRef = objectValue(manual.workflow_ref || manual.workflowRef);
  const operation = objectValue(manual.operation);
  const validation = objectValue(result?.validation_report || result?.validationReport || candidate.structured_validation_report || candidate.structuredValidationReport);
  const summary = objectValue(result?.user_summary || result?.userSummary || candidate.user_summary || candidate.userSummary);
  const blocking = arrayValue(validation.blocking);
  const warnings = arrayValue(validation.warnings);
  const passed = arrayValue(validation.passed);
  const manualPath = `/settings/ops-manuals?candidate=${encodeURIComponent(String(candidate.id || ""))}`;
  const nodeCount = graph.nodes?.length || 0;
  const documentMarkdown = String(manual.document_markdown || manual.documentMarkdown || "");
  const mainLabel = loading ? "生成中" : result ? (blocking.length ? "保存草稿" : "提交审核") : "生成候选";

  async function prepareCandidate() {
    if (loading || result) return;
    setLoading(true);
    setError("");
    try {
      const response = await requestJson("/api/v1/ops-manuals/candidates/generate-from-workflow", {
        method: "POST",
        body: JSON.stringify({
          workflow_id: name,
          workflow_version: version,
          options: { include_recent_run_records: true, use_llm_summary: false },
        }),
      });
      setResult(objectValue(response));
    } catch (cause) {
      const statusCode = Number((cause as { status?: number })?.status || 0);
      if (statusCode === 503) setError("Runner Studio 服务不可用，无法读取 Workflow。");
      else if (statusCode === 404) setError("Workflow 不存在或已被删除。");
      else if (statusCode === 400) setError("请求参数缺失，无法生成运维手册。");
      else setError(errorMessage(cause, "候选手册生成失败"));
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
            <p>将从当前 Runner Workflow 反向生成待审核运维手册候选，不会发布或替换已验证手册。</p>
            <ul>
              <li>Workflow：{title}</li>
              <li>状态：{status}</li>
              <li>绑定：只绑定 1 个 Runner Workflow</li>
              <li>节点数：{nodeCount}</li>
            </ul>
          </section>
          {result ? <section className="publish-review-card" data-testid="runner-ops-manual-profile">
            <h3>手册画像</h3>
            <ul>
              <li>对象：{String(operation.target_type || operation.targetType || "-")}</li>
              <li>操作：{String(operation.action || "-")}</li>
              <li>风险：{String(operation.risk_level || operation.riskLevel || "-")}</li>
              <li>Workflow ID：{String(workflowRef.workflow_id || workflowRef.workflowId || name)}</li>
              <li>Digest：{String(workflowRef.workflow_digest || workflowRef.workflowDigest || "-")}</li>
            </ul>
          </section> : null}
          {result ? <section className="publish-review-card" data-testid="runner-ops-manual-summary">
            <h3>系统理解</h3>
            {renderTextList(summary.understood)}
          </section> : null}
          {result ? <section className="publish-review-card" data-testid="runner-ops-manual-validation">
            <h3>缺口检查</h3>
            <p>Blocking：{blocking.length}，Warnings：{warnings.length}，Passed：{passed.length}</p>
            {renderIssueList("阻断", blocking)}
            {renderIssueList("提醒", warnings)}
            {passed.length ? <p>已通过 {passed.length} 项结构化检查。</p> : null}
          </section> : null}
          <section className="publish-review-card">
            <h3>手册预览</h3>
            {result ? <>
              <p>{String(manual.title || title)}</p>
              <pre className="runner-ops-manual-preview">{documentMarkdown}</pre>
            </> : <ul>
              <li>生成会读取 Runner Workflow YAML、ActionCatalog 和最近运行记录。</li>
              <li>系统会输出可审核的结构化校验报告。</li>
              <li>后续仍需在运维手册页确认后发布。</li>
            </ul>}
          </section>
          {result ? <p className="publish-review-warning">已生成候选：{String(candidate.id || "pending_review")}</p> : null}
          {error ? <p className="publish-review-error" role="alert">{error}</p> : null}
        </main>
        <footer className="publish-review-footer">
          <button type="button" onClick={onClose}>取消</button>
          {result ? <a className="workflow-secondary-link" data-testid="runner-ops-manual-view-candidate" href={manualPath}>查看候选</a> : null}
          <button type="button" className="primary" data-testid="runner-ops-manual-prepare" disabled={loading || Boolean(result)} onClick={() => void prepareCandidate()}>
            <BookOpen size={15} />{mainLabel}
          </button>
        </footer>
      </div>
    </section>
  );
}

type WorkflowCandidateSwitchGuard = {
  workflowId: string;
  candidateId: string;
  changeSetId: string;
  discard: () => Promise<void>;
};

function WorkflowCandidateSwitchGuardBridge({
  workflowId,
  onChange,
}: {
  workflowId: string;
  onChange: (guard: WorkflowCandidateSwitchGuard | undefined) => void;
}) {
  const workspace = useWorkflowWorkspace();
  const candidate = workspace.state.candidate;
  const candidateId = candidate?.candidate.id || "";
  const changeSetId = candidate?.changeSet.id || "";
  const discard = workspace.discardCandidate;

  useEffect(() => {
    if (!candidateId || !changeSetId) {
      onChange(undefined);
      return undefined;
    }
    onChange({ workflowId, candidateId, changeSetId, discard });
    return () => onChange(undefined);
  }, [candidateId, changeSetId, discard, onChange, workflowId]);
  return null;
}

function WorkflowCandidateSwitchDialog({
  currentTitle,
  targetTitle,
  busy,
  error,
  onKeep,
  onDiscard,
}: {
  currentTitle: string;
  targetTitle: string;
  busy: boolean;
  error?: string;
  onKeep: () => void;
  onDiscard: () => void;
}) {
  return (
    <section className="workflow-delete-confirm-backdrop" data-testid="workflow-candidate-switch-confirm">
      <div className="workflow-delete-confirm-modal" role="dialog" aria-modal="true" aria-label="切换工作流前处理候选修改">
        <header>
          <div>
            <p>UNAPPLIED CANDIDATE</p>
            <h2>先处理未应用的候选修改</h2>
          </div>
        </header>
        <main>
          <strong>{currentTitle}</strong>
          <span>切换到“{targetTitle}”前，请明确选择保留还是放弃当前候选。保留会继续停留在当前工作流，不会丢失修改。</span>
          {error ? <span role="alert">{error}</span> : null}
        </main>
        <footer>
          <button type="button" data-testid="workflow-candidate-switch-keep" disabled={busy} onClick={onKeep}>保留候选，暂不切换</button>
          <button type="button" className="danger" data-testid="workflow-candidate-switch-discard" disabled={busy} onClick={onDiscard}>
            {busy ? "正在放弃候选..." : "放弃候选并切换"}
          </button>
        </footer>
      </div>
    </section>
  );
}

function workflowDiagnosticEditorTarget(field: string): { tab: "settings" | "script" | "io"; selector: string } | undefined {
  const normalized = field.replace(/^step\./, "");
  if (normalized === "args.cmd") return { tab: "settings", selector: '[data-testid="runner-node-command-editor"]' };
  if (normalized === "args.script") return { tab: "script", selector: '[data-testid="runner-node-script-editor"]' };
  if (normalized === "args.script_ref") return { tab: "script", selector: '[data-testid="runner-node-script-tab"] input[placeholder="restore.sh"]' };
  if (normalized === "targets") return { tab: "settings", selector: '[data-testid="runner-node-target-labels-input"]' };
  const inputMatch = /^inputs\.(\d+)/.exec(normalized);
  if (inputMatch) return { tab: "io", selector: `[data-testid="runner-code-input-value-${inputMatch[1]}"]` };
  return undefined;
}

export function RunnerStudioPage({ appSnapshot }: { appSnapshot?: RunnerStudioAppSnapshot } = {}) {
  const params = useParams();
  const navigate = useNavigate();
  const routeWorkflowName = String(params.workflowName || "").trim();
  const [loading, setLoading] = useState(false);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [workflowSearchQuery, setWorkflowSearchQuery] = useState("");
  const [workflowPage, setWorkflowPage] = useState(1);
  const [actions, setActions] = useState<RunnerAction[]>(FALLBACK_ACTIONS);
  const [selectedWorkflowName, setSelectedWorkflowName] = useState(routeWorkflowName);
  const [saveState, setSaveState] = useState<SaveState>({ status: "idle" });
  const [dismissedSaveError, setDismissedSaveError] = useState("");
  const [lifecycleHeaderTarget, setLifecycleHeaderTarget] = useState<HTMLDivElement | null>(null);
  const [apiNotice, setApiNotice] = useState<ApiNotice>(null);
  const [apiNoticeDismissed, setApiNoticeDismissed] = useState(false);
  const [deleteWorkflowName, setDeleteWorkflowName] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState("");
  const [runEvents, setRunEvents] = useState<Record<string, unknown>[]>([]);
  const [runRecords, setRunRecords] = useState<RunnerRunRecord[]>([]);
  const [runFocusNodeId, setRunFocusNodeId] = useState("");
  const [publishTarget, setPublishTarget] = useState<Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>>();
  const [opsManualOpen, setOpsManualOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [candidateSwitchGuard, setCandidateSwitchGuard] = useState<WorkflowCandidateSwitchGuard>();
  const [pendingWorkflowSwitch, setPendingWorkflowSwitch] = useState<{ workflowId: string }>();
  const [candidateSwitchBusy, setCandidateSwitchBusy] = useState(false);
  const [candidateSwitchError, setCandidateSwitchError] = useState("");
  const runInFlightRef = useRef(false);
  const runLockUntilRef = useRef(0);
  const importInputRef = useRef<HTMLInputElement | null>(null);

  const selectedWorkflow = useMemo(() => workflows.find((workflow) => workflowKey(workflow) === selectedWorkflowName) || null, [workflows, selectedWorkflowName]);
  const deleteWorkflowTarget = useMemo(() => workflows.find((workflow) => workflowKey(workflow) === deleteWorkflowName) || null, [deleteWorkflowName, workflows]);
  const filteredWorkflows = useMemo(() => workflows.filter((workflow) => workflowMatchesSearch(workflow, workflowSearchQuery)), [workflowSearchQuery, workflows]);
  const workflowPageCount = Math.max(1, Math.ceil(filteredWorkflows.length / WORKFLOW_LIBRARY_PAGE_SIZE));
  const effectiveWorkflowPage = Math.min(Math.max(workflowPage, 1), workflowPageCount);
  const workflowPageItems = useMemo(() => {
    const start = (effectiveWorkflowPage - 1) * WORKFLOW_LIBRARY_PAGE_SIZE;
    return filteredWorkflows.slice(start, start + WORKFLOW_LIBRARY_PAGE_SIZE);
  }, [effectiveWorkflowPage, filteredWorkflows]);
  const graph = reconcileRunnerRuntimeConfig(normalizeGraph(selectedWorkflow?.graph || { workflow: { name: selectedWorkflowName || "draft" }, nodes: [], edges: [] }, selectedWorkflowName || "draft"));
  const projectedWorkspace = appSnapshot?.config?.workflowWorkspace;
  const workflowCapabilities = parseWorkflowCapabilitySnapshot(appSnapshot?.config?.workflowCapabilities);
  const workflowAgentUiEnabled = workflowCapabilities?.capabilities.workflow_agent_ui === true;
  const projectedWorkflowId = projectedWorkspace?.committed?.workflowKey.workflowId || projectedWorkspace?.workflowKey?.workflowId;
  const activeWorkspaceProjection = projectedWorkflowId === selectedWorkflowName ? projectedWorkspace : undefined;
  const openNodePanel = useCallback((nodeId: string) => {
    setSelectedNodeId(nodeId);
    if (nodeId) requestWorkflowContextDockTab("configuration");
  }, []);
  useEffect(() => {
    let selectFrame: number | undefined;
    let focusFrame: number | undefined;
    const focusDiagnostic = (event: Event) => {
      const location = readWorkflowValidationFocusEvent(event);
      if (!location?.nodeId || !location.field) return;
      const target = workflowDiagnosticEditorTarget(location.field);
      if (!target) return;
      setSelectedNodeId(location.nodeId);
      requestWorkflowContextDockTab("configuration");
      if (selectFrame !== undefined) window.cancelAnimationFrame(selectFrame);
      if (focusFrame !== undefined) window.cancelAnimationFrame(focusFrame);
      selectFrame = window.requestAnimationFrame(() => {
        const panel = document.querySelector<HTMLElement>('[data-testid="runner-node-panel"]');
        const tab = panel?.querySelector<HTMLButtonElement>(`[data-testid="runner-node-panel-tab-${target.tab}"]`);
        if (tab && !tab.classList.contains("active")) tab.click();
        focusFrame = window.requestAnimationFrame(() => {
          const editor = document.querySelector<HTMLElement>(`[data-testid="runner-node-panel"] ${target.selector}`);
          if (!editor) return;
          editor.dataset.workflowDiagnosticFocus = location.field;
          editor.focus({ preventScroll: true });
          editor.scrollIntoView?.({ block: "nearest", inline: "nearest" });
        });
      });
    };
    window.addEventListener(WORKFLOW_VALIDATION_FOCUS_EVENT, focusDiagnostic);
    return () => {
      window.removeEventListener(WORKFLOW_VALIDATION_FOCUS_EVENT, focusDiagnostic);
      if (selectFrame !== undefined) window.cancelAnimationFrame(selectFrame);
      if (focusFrame !== undefined) window.cancelAnimationFrame(focusFrame);
    };
  }, []);
  const runState = useMemo(() => reduceRunEvents(runEvents, createInitialRunState()), [runEvents]);
  const sessionRuns = useMemo(
    () => workflowRunSessionRecords(runRecords, runState, runEvents),
    [runEvents, runRecords, runState],
  );
  const serverActionsDisabled = Boolean(apiNotice);
  const workspaceMutationBlocked = saveState.status === "server_pending" || saveState.status === "manual_failed";
  const saveError = String(saveState.error || "").trim();
  const saveErrorVisible = Boolean(selectedWorkflow && saveError && dismissedSaveError !== saveError);
  const lifecycleSaveState: WorkflowSavePresentation | undefined = saveError || ["pending", "local_draft", "manual_failed", "blocked"].includes(saveState.status)
    ? "unsaved"
    : ["saving", "server_pending"].includes(saveState.status)
      ? "applying"
      : undefined;

  useEffect(() => {
    if (!saveError) setDismissedSaveError("");
  }, [saveError]);

  const upsertWorkflow = useCallback((name: string, patch: Partial<Workflow>) => {
    setWorkflows((current) => {
      const existing = current.find((workflow) => workflowKey(workflow) === name);
      if (!existing) return [{ name, title: name, status: "draft", ...patch }, ...current];
      return current.map((workflow) => workflowKey(workflow) === name ? { ...workflow, ...patch } : workflow);
    });
  }, []);

  const commitWorkflowSelection = useCallback((name: string) => {
    setSelectedWorkflowName(name);
    setSelectedNodeId("");
    setRunFocusNodeId("");
    setRunEvents([]);
    setRunRecords([]);
    runInFlightRef.current = false;
    runLockUntilRef.current = 0;
    setDeleteWorkflowName("");
    navigate(name ? `/runner/${encodeURIComponent(name)}` : "/runner");
  }, [navigate]);

  const selectWorkflow = useCallback((name: string) => {
    const targetWorkflowId = String(name || "").trim();
    if (targetWorkflowId === selectedWorkflowName) return;
    if (candidateSwitchGuard?.workflowId === selectedWorkflowName) {
      setPendingWorkflowSwitch({ workflowId: targetWorkflowId });
      setCandidateSwitchError("");
      return;
    }
    commitWorkflowSelection(targetWorkflowId);
  }, [candidateSwitchGuard, commitWorkflowSelection, selectedWorkflowName]);

  const keepCandidateAndCancelSwitch = useCallback(() => {
    if (candidateSwitchBusy) return;
    setPendingWorkflowSwitch(undefined);
    setCandidateSwitchError("");
  }, [candidateSwitchBusy]);

  const discardCandidateAndSwitch = useCallback(async () => {
    const target = pendingWorkflowSwitch?.workflowId;
    const guard = candidateSwitchGuard;
    if (target === undefined) return;
    if (!guard || guard.workflowId !== selectedWorkflowName) {
      setPendingWorkflowSwitch(undefined);
      commitWorkflowSelection(target);
      return;
    }
    setCandidateSwitchBusy(true);
    setCandidateSwitchError("");
    try {
      await guard.discard();
      setCandidateSwitchGuard(undefined);
      setPendingWorkflowSwitch(undefined);
      commitWorkflowSelection(target);
    } catch (error) {
      setCandidateSwitchError(errorMessage(error, "无法放弃候选，请重试"));
    } finally {
      setCandidateSwitchBusy(false);
    }
  }, [candidateSwitchGuard, commitWorkflowSelection, pendingWorkflowSwitch?.workflowId, selectedWorkflowName]);

  useEffect(() => { setSelectedWorkflowName(routeWorkflowName); }, [routeWorkflowName]);

  useEffect(() => {
    setWorkflowPage((current) => Math.min(Math.max(current, 1), workflowPageCount));
  }, [workflowPageCount]);

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
      const catalog = catalogResult.status === "fulfilled" ? normalizeRunnerActionCatalog(catalogResult.value) : [];
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
      const current = workflows.find((workflow) => workflowKey(workflow) === selectedWorkflowName)
        || readLocalDrafts().find((workflow) => workflowKey(workflow) === selectedWorkflowName);
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

  function createBlankWorkflow(name: string, title = name) {
    const workflow = saveLocalDraft({ name, title, status: "draft", graph: createBlankWorkflowGraph(name, title), local_draft: true, validation_result: { valid: false, errors: [], warnings: [] } });
    upsertWorkflow(name, workflow);
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
      setSaveState({ status: "pending", message: "已导入，未保存" });
    } catch (error) {
      setSaveState({ status: "failed", message: "导入失败", error: errorMessage(error) });
    }
  }

  async function deleteWorkflow(name: string) {
    const key = String(name || "").trim();
    if (!key) return;
    const workflow = workflows.find((item) => workflowKey(item) === key);
    try {
      if (workflow && !workflow.local_draft && !serverActionsDisabled) {
        await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(key)}`, { method: "DELETE" });
      }
      deleteLocalDraft(key);
      setWorkflows((current) => current.filter((item) => workflowKey(item) !== key));
      setDeleteWorkflowName("");
      if (selectedWorkflowName === key) {
        selectWorkflow("");
      }
      setSaveState({ status: "saved", message: "工作流已删除", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "删除失败", error: errorMessage(error) });
    }
  }

  function promptDeleteWorkflow(name: string) {
    const key = String(name || "").trim();
    if (!key) return;
    setDeleteWorkflowName(key);
  }

  function updateGraph(nextGraph: RunnerGraph) {
    if (!selectedWorkflowName) return;
    const normalized = reconcileRunnerRuntimeConfig(normalizeGraph(nextGraph, selectedWorkflowName));
    upsertWorkflow(selectedWorkflowName, { graph: normalized, status: "draft", validated_graph_hash: "", dry_run_graph_hash: "", validation_result: { valid: false, errors: [], warnings: [] } });
    saveLocalDraft({ ...(selectedWorkflow || { name: selectedWorkflowName }), graph: normalized, status: "draft", local_draft: true });
    setSaveState({ status: "pending", message: "未保存" });
  }

  function upsertRunRecord(record: RunnerRunRecord) {
    setRunRecords((current) => [record, ...current.filter((item) => item.runId !== record.runId)].slice(0, 25));
  }

  async function flushWorkflow(reason: string) {
    if (!selectedWorkflow) return null;
    if (saveState.status === "server_pending" || saveState.status === "manual_failed") return null;
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
    let savedGraph = normalizeGraph(saved.graph || normalized, name);
    if (method === "POST") {
      // The first GET binds Runner's authoritative event source to the shared
      // AppSnapshot stream. It is deliberately issued only after bootstrap
      // creation succeeds; later edits use typed commands and domain events.
      try {
        const refreshed = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(name)}/graph`);
        savedGraph = normalizeGraph(refreshed.graph || refreshed.data?.graph || savedGraph, name);
      } catch (_error) {
        // The graph was already persisted. Keep the saved state usable even
        // if this best-effort projection bootstrap cannot refresh immediately.
      }
    }
    deleteLocalDraft(name);
    upsertWorkflow(name, { ...saved, graph: savedGraph, local_draft: false, status: saved.status || selectedWorkflow.status || "draft" });
    setSaveState({ status: "saved", message: "已保存", lastSavedAt: formatSaveTime() });
    return savedGraph;
  }

  async function ensureWorkflowPersisted(reason: string) {
    if (!selectedWorkflow) return null;
    if (workspaceMutationBlocked) return null;
    const name = workflowKey(selectedWorkflow);
    if (selectedWorkflow.local_draft || saveState.status === "pending" || saveState.status === "local_draft") {
      return flushWorkflow(reason);
    }
    return normalizeGraph(selectedWorkflow.graph, name);
  }

  function savedDraftTargetIsCurrent(
    target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>,
  ) {
    const revision = activeWorkspaceProjection?.committed?.revision;
    return Boolean(
      selectedWorkflow &&
      target.workflowId === workflowKey(selectedWorkflow) &&
      revision &&
      target.resourceVersion === revision.resourceVersion &&
      target.semanticHash === revision.semanticHash,
    );
  }

  async function validateWorkflow(
    target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>,
  ) {
    if (!selectedWorkflow || !savedDraftTargetIsCurrent(target)) {
      setSaveState({ status: "blocked", message: "检查依据尚未同步" });
      return;
    }
    const name = workflowKey(selectedWorkflow);
    try {
      const savedGraph = await ensureWorkflowPersisted("validate");
      if (!savedGraph) return;
      if (serverActionsDisabled) return;
      const result = await requestJson(`/api/runner-studio/workflows/${encodeURIComponent(name)}/validate`, {
        method: "POST",
        body: JSON.stringify({
          expected_resource_version: target.resourceVersion,
          expected_semantic_hash: target.semanticHash,
        }),
      });
      const validation = result.data || result;
      upsertWorkflow(name, { status: validation.status || (validation.valid ? "validated" : "draft"), validated_graph_hash: validation.validated_graph_hash || validation.validatedGraphHash || "", dry_run_graph_hash: "", validation_result: validation });
      setSaveState({ status: "saved", message: "校验通过", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "校验失败", error: (error as Error).message });
    }
  }

  function openPublishReview(
    target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>,
  ) {
    if (!savedDraftTargetIsCurrent(target)) {
      setSaveState({ status: "blocked", message: "发布依据尚未同步" });
      return;
    }
    setPublishTarget(target);
  }

  async function dryRunWorkflow(
    target: Extract<WorkflowPrimaryActionTarget, { kind: "draft_non_production" }>,
  ) {
    if (!selectedWorkflow) return;
    if (target.workflowId !== workflowKey(selectedWorkflow)) {
      setSaveState({ status: "blocked", message: "试运行依据尚未同步" });
      requestWorkflowContextDockTab("run");
      return;
    }
    try {
      if (serverActionsDisabled) return;
      setSaveState({ status: "saving", message: "正在试运行当前草稿" });
      await dryRunAuthoritativeWorkflowTarget(target);
      setSaveState({ status: "saved", message: "试运行完成，正在同步结果", lastSavedAt: formatSaveTime() });
      requestWorkflowContextDockTab("run");
    } catch (error) {
      setSaveState({ status: "failed", message: "试运行失败", error: (error as Error).message });
    }
  }

  async function observeSubmittedRun(
    runId: string,
    executionGraph: RunnerGraph,
    focusNodeId: string,
    targetLabel: string,
    retryTarget: RunnerRunRetryTarget,
  ) {
    const now = new Date().toISOString();
    const queuedEvents = mapRunnerRunEventsToGraph([
      { type: "run_queued", run_id: runId, workflow: workflowKey(selectedWorkflow), status: "queued", timestamp: now },
      ...(focusNodeId ? [{ type: "node.started", run_id: runId, node_id: focusNodeId, status: "running", timestamp: now, message: "运行中" }] : []),
    ], executionGraph) as Record<string, unknown>[];
    setRunEvents(queuedEvents);
    upsertRunRecord(buildRunRecord(runId, queuedEvents, {
      status: "running",
      message: "运行中",
      startedAt: now,
      targetLabel,
      retryTarget,
    }));
    setSaveState({ status: "saved", message: "运行已提交", lastSavedAt: formatSaveTime() });
    await delay(350);
    try {
      const events = await waitForRunHistory(runId, executionGraph);
      const normalizedEvents = (events.length ? events : queuedEvents)
        .map((event: Record<string, unknown>) => ({ ...event, run_id: event.run_id || runId }));
      setRunEvents(normalizedEvents);
      upsertRunRecord(buildRunRecord(runId, normalizedEvents, { targetLabel, retryTarget }));
      setSaveState({ status: "saved", message: runStatusMessage(finalRunnerRunStatus(normalizedEvents)), lastSavedAt: formatSaveTime() });
    } catch (historyError) {
      const message = errorMessage(historyError, "运行历史读取失败");
      const failedEvents = [
        ...queuedEvents,
        { type: "run.failed", run_id: runId, status: "failed", message: `运行历史读取失败：${message}`, error: message, timestamp: new Date().toISOString() },
        ...(focusNodeId ? [{ type: "node.failed", run_id: runId, node_id: focusNodeId, status: "failed", message, error: message, timestamp: new Date().toISOString() }] : []),
      ];
      setRunEvents(failedEvents);
      upsertRunRecord(buildRunRecord(runId, failedEvents, { status: "failed", message, targetLabel, retryTarget }));
      setSaveState({ status: "failed", message: "运行状态刷新失败", error: message });
    }
  }

  async function runWorkflow(nodeId = "") {
    if (!selectedWorkflow) return;
    const submitStartedAt = Date.now();
    if (runInFlightRef.current || submitStartedAt < runLockUntilRef.current) return;
    runInFlightRef.current = true;
    runLockUntilRef.current = submitStartedAt + RUN_SUBMIT_COOLDOWN_MS;
    const name = workflowKey(selectedWorkflow);
    const initialGraph = normalizeGraph(selectedWorkflow.graph, name);
    let focusNodeId = nodeId || workflowStartNodeId(initialGraph) || firstRunnableNodeId(initialGraph);
    let retryTarget: RunnerRunRetryTarget | undefined;
    if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
    try {
      const savedGraph = await ensureWorkflowPersisted("run");
      if (!savedGraph || serverActionsDisabled) return;
      focusNodeId = nodeId || workflowStartNodeId(savedGraph) || firstRunnableNodeId(savedGraph) || focusNodeId;
      if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
      const targetLabel = nodeId ? `节点 ${nodeId}` : "当前草稿";
      retryTarget = {
        kind: "draft_graph",
        request: {
          workflow_name: name,
          graph: snapshotRunnerGraph(savedGraph),
          vars: {},
          triggered_by: "ui",
          risk_acknowledged: true,
          ...(nodeId ? { node_id: nodeId, run_scope: "single_node" as const } : {}),
        },
        focusNodeId,
        targetLabel,
      };
      const response = await requestJson("/api/runner-studio/runs", {
        method: "POST",
        body: JSON.stringify(retryTarget.request),
      });
      const runResponse = unwrapRunnerPayload(response) as Record<string, unknown>;
      const runId = String(runResponse.run_id || runResponse.runId || "");
      if (!runId) throw new Error("Runner did not return a run_id");
      await observeSubmittedRun(runId, savedGraph, focusNodeId, targetLabel, retryTarget);
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
      upsertRunRecord(buildRunRecord(runId, failedEvents, {
        status: "failed",
        message,
        targetLabel: nodeId ? `节点 ${nodeId}` : "当前草稿",
        ...(retryTarget ? { retryTarget } : {}),
      }));
      setSaveState({ status: "failed", message: "运行失败", error: message });
    } finally {
      runInFlightRef.current = false;
    }
  }

  async function runPublishedWorkflow(
    target: PublishedWorkflowRunTarget,
  ) {
    if (!selectedWorkflow || target.workflowKey.workflowId !== workflowKey(selectedWorkflow)) {
      setSaveState({ status: "blocked", message: "已发布版本依据尚未同步" });
      return;
    }
    const submitStartedAt = Date.now();
    if (runInFlightRef.current || submitStartedAt < runLockUntilRef.current) return;
    runInFlightRef.current = true;
    runLockUntilRef.current = submitStartedAt + RUN_SUBMIT_COOLDOWN_MS;
    const retryTarget: RunnerRunRetryTarget = {
      kind: "published_revision",
      target: {
        ...target,
        workflowKey: { ...target.workflowKey },
      },
      eventGraph: snapshotRunnerGraph(graph),
      vars: {},
      triggeredBy: "ui",
      riskAcknowledged: true,
      targetLabel: "已发布版本",
    };
    try {
      if (serverActionsDisabled) return;
      const response = await runPublishedAuthoritativeWorkflow({
        target: retryTarget.target,
        vars: retryTarget.vars,
        triggeredBy: retryTarget.triggeredBy,
        idempotencyKey: `published-${target.resourceVersion}-${submitStartedAt}`,
        riskAcknowledged: retryTarget.riskAcknowledged,
      });
      const runResponse = unwrapRunnerPayload(response) as Record<string, unknown>;
      const runId = String(runResponse.run_id || runResponse.runId || "");
      if (!runId) throw new Error("Runner did not return a run_id");
      await observeSubmittedRun(runId, graph, "", "已发布版本", retryTarget);
      requestWorkflowContextDockTab("run");
    } catch (error) {
      const message = errorMessage(error);
      const now = new Date().toISOString();
      const runId = `submit-${Date.now()}`;
      const failedEvents = [
        { type: "run.failed", run_id: runId, status: "failed", message: `已发布版本运行提交失败：${message}`, error: message, timestamp: now },
      ];
      setRunEvents(failedEvents);
      upsertRunRecord(buildRunRecord(runId, failedEvents, {
        status: "failed",
        message,
        targetLabel: "已发布版本",
        retryTarget,
      }));
      setSaveState({ status: "failed", message: "运行已发布版本失败", error: message });
    } finally {
      runInFlightRef.current = false;
    }
  }

  async function retrySessionRun(sourceRunId: string) {
    if (!selectedWorkflow || runInFlightRef.current) return;
    const retryTarget = runRecords.find((record) => record.runId === sourceRunId)?.retryTarget;
    if (!retryTarget) return;
    const selectedWorkflowId = workflowKey(selectedWorkflow);
    const targetWorkflowId = retryTarget.kind === "draft_graph"
      ? retryTarget.request.workflow_name
      : retryTarget.target.workflowKey.workflowId;
    if (targetWorkflowId !== selectedWorkflowId) {
      setSaveState({ status: "blocked", message: "重试目标与当前工作流不一致" });
      return;
    }

    runInFlightRef.current = true;
    const focusNodeId = retryTarget.kind === "draft_graph" ? retryTarget.focusNodeId : "";
    if (focusNodeId) setRunFocusNodeId(`${focusNodeId}:${Date.now()}`);
    try {
      if (serverActionsDisabled) return;
      const response = retryTarget.kind === "draft_graph"
        ? await requestJson("/api/runner-studio/runs", {
            method: "POST",
            body: JSON.stringify(retryTarget.request),
          })
        : await runPublishedAuthoritativeWorkflow({
            target: retryTarget.target,
            vars: retryTarget.vars,
            triggeredBy: retryTarget.triggeredBy,
            idempotencyKey: `published-retry-${retryTarget.target.resourceVersion}-${Date.now()}`,
            riskAcknowledged: retryTarget.riskAcknowledged,
          });
      const runResponse = unwrapRunnerPayload(response) as Record<string, unknown>;
      const runId = String(runResponse.run_id || runResponse.runId || "");
      if (!runId) throw new Error("Runner did not return a run_id");
      const eventGraph = retryTarget.kind === "draft_graph"
        ? retryTarget.request.graph
        : retryTarget.eventGraph;
      await observeSubmittedRun(runId, eventGraph, focusNodeId, retryTarget.targetLabel, retryTarget);
      requestWorkflowContextDockTab("run");
    } catch (error) {
      const message = errorMessage(error);
      const now = new Date().toISOString();
      const runId = `submit-${Date.now()}`;
      const failedEvents = [
        { type: "run.failed", run_id: runId, status: "failed", message: `运行重试提交失败：${message}`, error: message, timestamp: now },
        ...(focusNodeId ? [{ type: "node.failed", run_id: runId, node_id: focusNodeId, status: "failed", message, error: message, timestamp: now }] : []),
      ];
      setRunEvents(failedEvents);
      upsertRunRecord(buildRunRecord(runId, failedEvents, {
        status: "failed",
        message,
        targetLabel: retryTarget.targetLabel,
        retryTarget,
      }));
      setSaveState({ status: "failed", message: "运行重试失败", error: message });
    } finally {
      runInFlightRef.current = false;
    }
  }

  const commandMenuItems = useMemo<readonly WorkflowActionMenuItem[]>(() => selectedWorkflow ? [
    {
      id: "import",
      label: "导入工作流…",
      group: "file",
      onSelect: () => importInputRef.current?.click(),
    },
    {
      id: "export",
      label: "导出工作流…",
      group: "file",
      onSelect: exportWorkflow,
    },
    {
      id: "manual",
      label: "生成运维手册",
      group: "assist",
      onSelect: () => setOpsManualOpen(true),
    },
    {
      id: "delete",
      label: "删除工作流…",
      group: "management",
      danger: true,
      onSelect: () => promptDeleteWorkflow(workflowKey(selectedWorkflow)),
    },
  ] : [], [graph, selectedWorkflow]);

  const runnerHeaderContent = useMemo(() => selectedWorkflow ? (
    <div
      className="runner-studio-lifecycle-slot"
      data-testid="runner-studio-lifecycle-slot"
      ref={setLifecycleHeaderTarget}
    />
  ) : null, [selectedWorkflow?.name]);

  const runnerLibraryHeaderActions = useMemo(
    () => selectedWorkflow ? null : (
      <>
        <label className="runner-workflow-search" aria-label="搜索工作流名称">
          <Search size={15} />
          <input
            data-testid="runner-workflow-search"
            value={workflowSearchQuery}
            placeholder="搜索工作流名称"
            onChange={(event) => {
              setWorkflowSearchQuery(event.currentTarget.value);
              setWorkflowPage(1);
            }}
          />
        </label>
        <button
          type="button"
          className="runner-studio-action-button primary"
          data-testid="runner-create-workflow"
          onClick={() => createBlankWorkflow(nextBlankWorkflowName(workflows), "新建工作流")}
          disabled={loading}
        >
          <Plus size={15} />新建工作流
        </button>
      </>
    ),
    [loading, selectedWorkflow, workflowSearchQuery, workflows],
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
        {saveErrorVisible ? (
          <section className="workflow-command-toast" role="alert" aria-live="assertive" data-testid="workflow-command-error-toast">
            <div><strong>修改未保存</strong><p>{saveError}</p></div>
            <button type="button" aria-label="关闭错误提示" onClick={() => setDismissedSaveError(saveError)}>×</button>
          </section>
        ) : null}
        {!selectedWorkflow ? (
          <WorkflowLibrary
            workflows={workflowPageItems}
            totalCount={workflows.length}
            filteredCount={filteredWorkflows.length}
            searchQuery={workflowSearchQuery}
            page={effectiveWorkflowPage}
            pageCount={workflowPageCount}
            pageSize={WORKFLOW_LIBRARY_PAGE_SIZE}
            onPageChange={setWorkflowPage}
            onSelect={selectWorkflow}
            onDelete={promptDeleteWorkflow}
          />
        ) : (
          <WorkflowWorkspaceProvider workflowId={workflowKey(selectedWorkflow)} projection={activeWorkspaceProjection}>
            <WorkflowCandidateSwitchGuardBridge workflowId={workflowKey(selectedWorkflow)} onChange={setCandidateSwitchGuard} />
            <WorkflowEditorShellWithCandidateRevision
              graph={graph}
              actions={actions}
              runState={runState}
              focusNodeId={runFocusNodeId}
              selectedNodeId={selectedNodeId}
              aiHighlightedNodeIds={NO_AI_HIGHLIGHTED_NODE_IDS}
              fullscreen={fullscreen}
              workflowTitle={selectedWorkflow.title || selectedWorkflow.name}
              workflowEnvironment={activeWorkspaceProjection?.runGateFacts?.environment.name}
              saveState={lifecycleSaveState}
              lifecycleHeaderTarget={lifecycleHeaderTarget}
              runGateFacts={activeWorkspaceProjection?.runGateFacts}
              durableHistoryEntries={activeWorkspaceProjection?.durableHistory || []}
              sessionRuns={sessionRuns}
              commandMenuItems={commandMenuItems}
              onCheckWorkflow={validateWorkflow}
              onDryRunWorkflow={dryRunWorkflow}
              onPublishWorkflow={openPublishReview}
              onRunPublishedWorkflow={runPublishedWorkflow}
              onRetrySessionRun={retrySessionRun}
              onCommandStateChange={(state) => setSaveState(state.status === "failed" ? { status: "manual_failed", message: "未保存", error: state.message } : state.status === "reconciled" ? { status: "saved", message: state.message, lastSavedAt: formatSaveTime() } : { status: "server_pending", message: state.message })}
              onBootstrapGraphChange={selectedWorkflow.local_draft ? updateGraph : undefined}
              onBack={() => selectWorkflow("")}
              onSelectNode={openNodePanel}
              onOpenNodeConfig={openNodePanel}
              onToggleFullscreen={() => setFullscreen((value) => !value)}
              aiTransportPanel={workflowAgentUiEnabled && routeWorkflowName === selectedWorkflowName ? (
                <WorkflowAiTransportSlot
                  graph={graph}
                  environment={activeWorkspaceProjection?.runGateFacts?.environment || { bindingVersion: "" }}
                  open
                  workflowId={workflowKey(selectedWorkflow)}
                />
              ) : undefined}
              renderNodeInspector={({ graph: committedGraph, node, onApplyNode, onDeleteNode, onCloseNode }) => node ? (
                <RunnerNodeInspector
                  node={node}
                  graph={committedGraph}
                  runState={runState}
                  onAction={(action) => {
                    if (action.type === "close") onCloseNode();
                    if (action.type === "run-node") {
                      void runWorkflow(action.nodeId);
                    }
                    if (action.type === "delete-node") onDeleteNode(action.nodeId);
                    if (action.type === "apply-node") onApplyNode(action.node);
                  }}
                />
              ) : undefined}
            />
          </WorkflowWorkspaceProvider>
        )}
      </section>
      {deleteWorkflowTarget ? <WorkflowDeleteConfirmDialog workflow={deleteWorkflowTarget} onCancel={() => setDeleteWorkflowName("")} onConfirm={() => void deleteWorkflow(workflowKey(deleteWorkflowTarget))} /> : null}
      {publishTarget && selectedWorkflow ? (
        <PublishReviewModal
          workflow={selectedWorkflow}
          target={publishTarget}
          onClose={() => setPublishTarget(undefined)}
          onPublished={(payload) => {
            upsertWorkflow(selectedWorkflowName, { ...payload, status: payload.status || "published" });
            setPublishTarget(undefined);
          }}
        />
      ) : null}
      {opsManualOpen && selectedWorkflow ? <OpsManualCandidateModal workflow={selectedWorkflow} graph={graph} onClose={() => setOpsManualOpen(false)} /> : null}
      {pendingWorkflowSwitch && selectedWorkflow ? (
        <WorkflowCandidateSwitchDialog
          currentTitle={selectedWorkflow.title || selectedWorkflow.name}
          targetTitle={workflows.find((workflow) => workflowKey(workflow) === pendingWorkflowSwitch.workflowId)?.title || pendingWorkflowSwitch.workflowId || "工作流列表"}
          busy={candidateSwitchBusy}
          error={candidateSwitchError || undefined}
          onKeep={keepCandidateAndCancelSwitch}
          onDiscard={() => void discardCandidateAndSwitch()}
        />
      ) : null}
    </section>
  );
}
