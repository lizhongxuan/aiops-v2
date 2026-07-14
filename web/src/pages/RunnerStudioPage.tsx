import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import {
  ArrowLeft,
  AlertTriangle,
  BookOpen,
  Bot,
  CheckCircle,
  ChevronLeft,
  ChevronRight,
  Circle,
  Database,
  Download,
  FlaskConical,
  LoaderCircle,
  Maximize2,
  MoreHorizontal,
  PanelRight,
  Play,
  Plus,
  Rocket,
  Save,
  Search,
  Trash2,
  Upload,
  X,
} from "lucide-react";

import { useRegisterAppShellHeader, useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import { sendMessage as sendChatMessage } from "@/api/chat";
import { fetchState } from "@/api/state";
import {
  applyRunnerStudioWorkflowAiPatch,
  createRunnerStudioWorkflowAiDraftFromPlan,
  createRunnerStudioWorkflowAiSession,
  proposeRunnerStudioWorkflowAiPlan,
  undoRunnerStudioWorkflowAiPatch,
} from "@/api/runnerStudioClient";
import { RunnerCanvas } from "@/components/runner/RunnerCanvas";
import { createInputParam, normalizeInputParams, valueSourceLabel, variableToValueSource } from "@/components/runner/io/ioTypes";
import { createOutputParam, normalizeOutputParams } from "@/components/runner/io/outputTypes";
import { FALLBACK_RUNNER_ACTIONS } from "@/components/runner/fallbackActionCatalog";
import { getNodeCanvasMeta } from "@/components/runner/nodeTypeRegistry";
import { collectRunnerVariables } from "@/components/runner/runnerVariables";
import { firstRunnableNodeId, getRunnerNodeRunState } from "@/components/runner/runnerRunVisualState";
import { extractRunnerRunEvents, finalRunnerRunStatus, isRunnerRunHistoryTerminal, mapRunnerRunEventsToGraph, unwrapRunnerPayload } from "@/components/runner/runEventHistory";
import { createInitialRunState, reduceRunEvents } from "@/components/runner/runStateReducer";
import { graphToFlowModel, removeGraphNode, type RunnerEdge, type RunnerGraph, type RunnerNode } from "@/components/runner/canvasGraphAdapter";
import { WorkflowAiDrawer } from "@/runner/WorkflowAiDrawer";
import { WorkflowEventDrawer } from "@/runner/WorkflowEventDrawer";
import { parseWorkflowAiPlanReply } from "@/runner/workflowAiViewModel";
import type { WorkflowAiActiveStep, WorkflowAiEffectStatus, WorkflowAiEvent, WorkflowAiStepHistoryItem, WorkflowAiToolLogEntry, WorkflowAiVariableSpec, WorkflowEditPlan, WorkflowPatch, WorkflowPatchResult } from "@/runner/workflowAiTypes";
import "@/components/runner/runnerStudio.css";

type RunnerAction = { action?: string; name?: string; label?: string; title?: string; category?: string; defaults?: Record<string, unknown>; [key: string]: unknown };
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
  ["ai-generate", "AI", Bot],
] as const;

const SECONDARY_TOOLBAR_ACTIONS = [
  ["ops-manual", "生成运维手册", BookOpen],
  ["import", "导入", Upload],
  ["export", "导出", Download],
  ["validate", "校验", CheckCircle],
  ["dry-run", "发布前检查", FlaskConical],
  ["variables", "变量", Database],
  ["publish", "发布", Rocket],
] as const;

const SINGLE_WORKFLOW_PANEL_BREAKPOINT = 1180;

function shouldUseSingleWorkflowSidePanel() {
  return typeof window !== "undefined" && window.innerWidth <= SINGLE_WORKFLOW_PANEL_BREAKPOINT;
}

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
const WORKFLOW_AI_CHAT_POLL_INTERVAL_MS = import.meta.env.MODE === "test" ? 10 : 700;
const WORKFLOW_AI_CHAT_POLL_TIMEOUT_MS = import.meta.env.MODE === "test" ? 1500 : 90000;

function workflowKey(workflow: Partial<Workflow> | null | undefined) {
  return String(workflow?.name || workflow?.id || "").trim();
}

const WORKFLOW_AI_WORKFLOW_CREATE_PATTERNS = [
  /(?:创建|新建|生成|做|搭建|设计)(?:一个|一套|一条)?.{0,32}(?:工作流|workflow|流程)/i,
  /\b(?:create|generate|build|design)\b.{0,40}\b(?:workflow|flow)\b/i,
];

const WORKFLOW_AI_GRAPH_EDIT_PATTERNS = [
  /(?:添加|新增|插入|删除|移除|修改|调整|改成|改为|重命名|替换|连接|断开|更新|修复|补充).{0,56}(?:节点|步骤|连线|边|画布|工作流|workflow|流程|Start|End)/i,
  /(?:把|将).{0,56}(?:节点|步骤|连线|边|工作流|workflow|流程|Start|End).{0,56}(?:添加|新增|插入|删除|移除|修改|调整|改成|改为|移动到|连接到|断开|重命名|替换)/i,
  /\b(?:add|insert|delete|remove|edit|modify|update|rename|replace|connect|disconnect|fix)\b.{0,56}\b(?:node|step|edge|workflow|flow|canvas|start|end)\b/i,
];

const WORKFLOW_AI_ADVICE_OR_READONLY_PATTERNS = [
  /(?:看看|看下|解释|说明|分析|评估|建议|给建议|可以|能不能|是否|为什么|怎么|如何|哪些|哪里|what|why|how|explain|suggest|advice)/i,
];

const WORKFLOW_AI_NEGATED_EDIT_PATTERNS = [
  /(?:先别|暂时别|暂时不要|不要直接|别直接|不要|无需|不需要|不用|别).{0,18}(?:修改|改动|编辑|调整|删除|移除|生成计划|生成修改计划|改画布|修改画布)/gi,
  /(?:do not|don't|dont|no need to|without).{0,32}\b(?:edit|change|modify|update|delete|remove|generate a plan|plan)\b/gi,
];

function removeWorkflowAiNegatedEditClauses(message: string) {
  const normalized = String(message || "").trim().toLowerCase();
  if (!normalized) return "";
  return WORKFLOW_AI_NEGATED_EDIT_PATTERNS.reduce((current, pattern) => current.replace(pattern, " "), normalized).trim();
}

function isWorkflowAiEditRequest(message: string) {
  const normalized = String(message || "").trim().toLowerCase();
  if (!normalized) return false;
  const candidate = removeWorkflowAiNegatedEditClauses(normalized);
  if (!candidate) return false;
  if (WORKFLOW_AI_ADVICE_OR_READONLY_PATTERNS.some((pattern) => pattern.test(candidate))) {
    const explicitEdit = WORKFLOW_AI_GRAPH_EDIT_PATTERNS.some((pattern) => pattern.test(candidate));
    const explicitCreate = WORKFLOW_AI_WORKFLOW_CREATE_PATTERNS.some((pattern) => pattern.test(candidate));
    return explicitEdit || explicitCreate;
  }
  return (
    WORKFLOW_AI_WORKFLOW_CREATE_PATTERNS.some((pattern) => pattern.test(candidate)) ||
    WORKFLOW_AI_GRAPH_EDIT_PATTERNS.some((pattern) => pattern.test(candidate))
  );
}

function workflowAiChatResponseText(response: unknown) {
  const value = objectValue(response);
  return String(value.output || value.content || value.message || value.answer || "").trim();
}

function workflowAiVisibleGraph(graph: RunnerGraph): RunnerGraph {
  const model = graphToFlowModel(graph);
  const visibleNodeIds = new Set(model.nodes.map((node) => String(node.id || "")));
  const nodes = (Array.isArray(graph.nodes) ? graph.nodes : []).filter((node) => visibleNodeIds.has(String(node.id || "")));
  const edges = (Array.isArray(graph.edges) ? graph.edges : []).filter((edge) => {
    const source = String(edge.source || "");
    const target = String(edge.target || "");
    return visibleNodeIds.has(source) && visibleNodeIds.has(target);
  });
  return { ...graph, nodes, edges };
}

function workflowAiChatModelContent(message: string, workflow: Partial<Workflow> | null, graph: RunnerGraph) {
  const visibleGraph = workflowAiVisibleGraph(graph);
  const workflowInfo = objectValue(visibleGraph.workflow);
  const name = String(workflow?.title || workflow?.name || workflowInfo.title || workflowInfo.name || "当前流程").trim();
  const status = String(workflow?.status || workflowInfo.status || "draft").trim();
  const nodes = Array.isArray(visibleGraph.nodes) ? visibleGraph.nodes : [];
  const edges = Array.isArray(visibleGraph.edges) ? visibleGraph.edges : [];
  const nodeLabelById = new Map(nodes.map((node) => [String(node.id || ""), workflowAiNodeLabel(node)]));
  const labels = workflowAiOrderedNodes(visibleGraph).map(workflowAiNodeLabel).slice(0, 8);
  const visibleNodeText = labels.length ? labels.join("、") : "无";
  const visibleEdges = edges
    .map((edge) => {
      const source = String(edge.source || "").trim();
      const target = String(edge.target || "").trim();
      if (!source || !target) return "";
      return `${nodeLabelById.get(source) || source} -> ${nodeLabelById.get(target) || target}`;
    })
    .filter(Boolean)
    .slice(0, 8);
  const visibleEdgeText = visibleEdges.length ? visibleEdges.join("、") : "无";
  const instructions = [
    "你是 AIOps Workflow AI，正在流程编辑页右侧对话框中和用户交流。",
    "本次是普通对话，不会修改画布。自然、简洁地回复用户。",
    "解释当前工作流时只依据下面列出的当前可见节点和当前可见连线；没有列出的节点不要假设存在。",
    `当前对象：${name}（${status}），${nodes.length} 个节点、${edges.length} 条连线。`,
    `当前可见节点：${visibleNodeText}。`,
    `当前可见连线：${visibleEdgeText}。`,
    labels.length ? `画布摘要：${labels.join(" -> ")}。` : "画布摘要：暂无节点。",
    "如果用户问当前工作流做了什么，请按实际可见节点和连线说明；节点少或无连线时直接说目前还没有完整编排。",
    `用户消息：${message}`,
  ];
  return instructions.filter(Boolean).join("\n");
}

function workflowAiChatResponseTurnId(response: unknown) {
  const value = objectValue(response);
  return String(value.turnId || value.turnID || value.id || "").trim();
}

function workflowAiChatFinalTextFromState(state: unknown, turnId: string, clientTurnId: string, chatSessionId: string) {
  const root = objectValue(state);
  const isChatSessionState = chatSessionId && String(root.sessionId || "") === chatSessionId;
  const turns = objectValue(root.turns);
  const directTurn = objectValue(turns[turnId]);
  const matchedTurn = Object.values(turns)
    .map(objectValue)
    .find((turn) => {
      const user = objectValue(turn.user);
      return String(turn.id || "") === turnId || String(user.clientTurnId || "") === clientTurnId;
    });
  const turn = directTurn.id || directTurn.final || directTurn.status ? directTurn : objectValue(matchedTurn);
  const final = objectValue(turn.final);
  const finalText = String(final.answerText || final.text || turn.output || turn.message || "").trim();
  if (finalText) {
    return { status: String(turn.status || "completed"), text: finalText };
  }

  const cards = Array.isArray(root.cards) ? root.cards.map(objectValue) : [];
  const cardText = workflowAiChatCardTextForTurn(cards, turnId, clientTurnId, isChatSessionState);
  if (cardText) {
    return { status: "completed", text: cardText };
  }

  return { status: String(turn.status || root.status || ""), text: "" };
}

function workflowAiChatCardTextForTurn(cards: Record<string, unknown>[], turnId: string, clientTurnId: string, isChatSessionState: boolean) {
  if (clientTurnId) {
    let matchedUser = false;
    for (const card of cards) {
      const role = String(card.role || "").toLowerCase();
      if (role === "user") {
        if (matchedUser) break;
        matchedUser = String(card.clientTurnId || "").trim() === clientTurnId;
        continue;
      }
      if (matchedUser && role === "assistant") {
        const text = String(card.text || card.message || "").trim();
        if (text) return text;
      }
    }
    return "";
  }
  const directCardText = cards
    .filter((card) => {
      if (String(card.role || "").toLowerCase() !== "assistant") return false;
      const cardClientTurnId = String(card.clientTurnId || "").trim();
      const cardTurnId = String(card.turnId || card.id || "").trim();
      if (turnId) return cardTurnId === turnId;
      return isChatSessionState && !cardClientTurnId;
    })
    .map((card) => String(card.text || card.message || "").trim())
    .filter(Boolean)
    .at(-1);
  return directCardText || "";
}

async function waitForWorkflowAiChatResponse(response: unknown, clientTurnId: string, chatSessionId: string) {
  const immediate = workflowAiChatResponseText(response);
  if (immediate) return immediate;

  const turnId = workflowAiChatResponseTurnId(response);
  if (!turnId && !clientTurnId) {
    throw new Error("模型没有返回回复内容");
  }

  const deadline = Date.now() + WORKFLOW_AI_CHAT_POLL_TIMEOUT_MS;
  let lastStatus = "";
  while (Date.now() < deadline) {
    const state = await fetchState();
    const result = workflowAiChatFinalTextFromState(state, turnId, clientTurnId, chatSessionId);
    if (result.text) return result.text;
    lastStatus = result.status || lastStatus;
    if (["failed", "canceled", "cancelled"].includes(String(result.status || "").toLowerCase())) {
      throw new Error(`模型回复失败：${result.status}`);
    }
    await delay(WORKFLOW_AI_CHAT_POLL_INTERVAL_MS);
  }
  throw new Error(lastStatus ? `等待模型回复超时：${lastStatus}` : "等待模型回复超时");
}

function workflowAiNodeLabel(node: RunnerNode) {
  const ui = objectValue(node.ui);
  const step = objectValue(node.step);
  return String(node.label || ui.title || ui.label || step.name || step.action || node.id || "未命名节点").trim();
}

function workflowAiOrderedNodes(graph: RunnerGraph) {
  const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
  const nodeById = new Map(nodes.map((node) => [node.id, node]));
  const outgoing = new Map<string, string[]>();
  for (const edge of graph.edges || []) {
    if (!edge.source || !edge.target) continue;
    outgoing.set(edge.source, [...(outgoing.get(edge.source) || []), edge.target]);
  }
  const startNode = nodes.find((node) => {
    const id = String(node.id || "").toLowerCase();
    const type = String(node.type || "").toLowerCase();
    return id === "start" || type === "start" || type === "input" || workflowAiNodeLabel(node).toLowerCase() === "start";
  }) || nodes[0];
  const ordered: RunnerNode[] = [];
  const visited = new Set<string>();
  let cursor = startNode?.id || "";
  while (cursor && nodeById.has(cursor) && !visited.has(cursor)) {
    const node = nodeById.get(cursor);
    if (!node) break;
    ordered.push(node);
    visited.add(cursor);
    cursor = (outgoing.get(cursor) || []).find((target) => !visited.has(target)) || "";
  }
  const remaining = nodes
    .filter((node) => !visited.has(node.id))
    .sort((left, right) => (left.position?.x || 0) - (right.position?.x || 0) || (left.position?.y || 0) - (right.position?.y || 0));
  return [...ordered, ...remaining];
}

function describeWorkflowForAi(workflow: Partial<Workflow> | null, graph: RunnerGraph) {
  const visibleGraph = workflowAiVisibleGraph(graph);
  const workflowInfo = objectValue(visibleGraph.workflow);
  const name = workflow?.title || workflow?.name || workflowInfo.title || workflowInfo.name || "当前工作流";
  const status = workflow?.status || workflowInfo.status || "draft";
  const nodes = workflowAiOrderedNodes(visibleGraph);
  const edges = Array.isArray(visibleGraph.edges) ? visibleGraph.edges : [];
  const labels = nodes.map(workflowAiNodeLabel);
  const visibleLabels = labels.slice(0, 8);
  const flowText = visibleLabels.length ? `${visibleLabels.join(" -> ")}${labels.length > visibleLabels.length ? " -> ..." : ""}` : "暂无节点";
  const runnableLabels = nodes.filter((node) => String(node.type || "").toLowerCase() !== "input").map(workflowAiNodeLabel).slice(0, 5);
  const purpose = runnableLabels.length
    ? `它的作用是从入口开始，按连线顺序执行 ${runnableLabels.join("、")} 等步骤。`
    : "它目前只有入口或空画布，还没有可执行步骤。";
  return [
    "Workflow 摘要：",
    `这个 Workflow 是 ${name}（${status}），包含 ${nodes.length} 个节点、${edges.length} 条连线。`,
    `主流程：${flowText}。`,
    purpose,
  ].join("\n");
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

function workflowAiRevision(workflow: Partial<Workflow> | null | undefined, graph: RunnerGraph) {
  return String(
    workflow?.version ||
      workflow?.validated_graph_hash ||
      workflow?.validatedGraphHash ||
      objectValue(graph.ui).revision ||
      "local-draft",
  );
}

function applyWorkflowAiPatchToRunnerGraph(
  graph: RunnerGraph,
  patch: WorkflowPatch,
): { graph: RunnerGraph; affectedNodes: string[]; metadataChanged: boolean } {
  const next: RunnerGraph = JSON.parse(JSON.stringify(graph || {}));
  const affectedNodes: string[] = [];
  let metadataChanged = false;

  for (const operation of patch.operations || []) {
    if (operation.op === "add_node" && operation.node) {
      const node = operation.node as RunnerNode;
      if (node.id && !(next.nodes || []).some((item) => item.id === node.id)) {
        next.nodes = [...(next.nodes || []), node];
        affectedNodes.push(node.id);
      }
    }
    if (operation.op === "update_node" && operation.nodeId) {
      next.nodes = (next.nodes || []).map((node) => {
        if (node.id !== operation.nodeId) return node;
        affectedNodes.push(operation.nodeId || node.id);
        const fields = operation.fields || {};
        const ui = { ...objectValue(node.ui) };
        let nextNode = { ...node };
        for (const [key, value] of Object.entries(fields)) {
          if (key === "label" && typeof value === "string") {
            nextNode = { ...nextNode, label: value };
          } else if (key === "position") {
            const position = objectValue(value);
            const x = Number(position.x);
            const y = Number(position.y);
            if (Number.isFinite(x) && Number.isFinite(y)) {
              nextNode = { ...nextNode, position: { x, y } };
            }
          } else {
            ui[key.replace(/^ui\./, "")] = value;
          }
        }
        return { ...nextNode, ui: { ...ui, ai_last_patch_id: patch.id } };
      });
    }
    if (operation.op === "delete_edge" && operation.edgeId) {
      const before = (next.edges || []).length;
      next.edges = (next.edges || []).filter((edge) => edge.id !== operation.edgeId);
      if ((next.edges || []).length !== before) metadataChanged = true;
    }
    if (operation.op === "add_edge" && operation.edge) {
      const edge = operation.edge as RunnerEdge;
      if (edge.source && edge.target && !(next.edges || []).some((item) => item.id === edge.id)) {
        next.edges = [...(next.edges || []), edge];
        metadataChanged = true;
      }
    }
    if (operation.op === "update_workflow_metadata") {
      next.ui = { ...objectValue(next.ui), ...(operation.fields || {}), ai_last_patch_id: patch.id };
      metadataChanged = true;
    }
  }

  next.ui = { ...objectValue(next.ui), ai_last_patch_id: patch.id, ai_last_patch_summary: patch.summary || patch.id };
  return { graph: next, affectedNodes: Array.from(new Set(affectedNodes)), metadataChanged };
}

function workflowAiStepNodeId(itemId: string) {
  const safe = String(itemId || "step").replace(/[^a-zA-Z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").toLowerCase();
  return `ai-step-${safe || "step"}`;
}

function workflowAiNodePorts() {
  return [{ id: "in", type: "input", label: "输入" }, { id: "next", type: "output", label: "下一步" }];
}

type WorkflowAiPlanItem = WorkflowEditPlan["items"][number];

function workflowAiStepDelayMs() {
  return import.meta.env.MODE === "test" ? 0 : 700;
}

function oneLine(value: unknown) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function pythonComment(value: unknown) {
  return oneLine(value).replace(/\n/g, " ").replace(/\r/g, " ");
}

function workflowAiStringField(item: WorkflowAiPlanItem, ...keys: string[]) {
  const record = item as unknown as Record<string, unknown>;
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function workflowAiEnvironmentField(item: WorkflowAiPlanItem, fallback: string) {
  const record = item as unknown as Record<string, unknown>;
  const value = record.environment ?? record.environments;
  if (Array.isArray(value)) {
    const entries = value.map(oneLine).filter(Boolean);
    if (entries.length) return entries;
  }
  if (typeof value === "string" && value.trim()) return value.trim();
  return fallback;
}

function workflowAiPlanNodeLabel(item: WorkflowAiPlanItem, fallback: string) {
  return oneLine(workflowAiStringField(item, "nodeLabel", "node_label", "nodeName", "node_name") || fallback);
}

function workflowAiPlanNodeType(item: WorkflowAiPlanItem) {
  const value = oneLine(workflowAiStringField(item, "nodeType", "node_type"));
  if (!value || value === "start" || value === "end") return "action";
  return value;
}

function workflowAiPlanNodeAction(item: WorkflowAiPlanItem) {
  return oneLine(workflowAiStringField(item, "nodeAction", "node_action", "action")) || "script.python";
}

function workflowAiVariableSpecs(value: unknown, fallback: WorkflowAiVariableSpec[]): WorkflowAiVariableSpec[] {
  if (!Array.isArray(value)) return fallback;
  const variables = value
    .map((entry) => {
      const record = objectValue(entry);
      const name = String(record.name || record.key || "").trim();
      if (!name) return null;
      return {
        name,
        type: String(record.type || "").trim() || undefined,
        required: Boolean(record.required),
        source: String(record.source || "").trim() || undefined,
      } satisfies WorkflowAiVariableSpec;
    })
    .filter((entry): entry is WorkflowAiVariableSpec => Boolean(entry));
  return variables.length ? variables : fallback;
}

function workflowAiStepDetails(item: WorkflowAiPlanItem, message: string, index: number): WorkflowAiActiveStep & { script: string; outputKey: string; validation: string } {
  const title = oneLine(item.title || `步骤 ${index + 1}`);
  const description = oneLine(item.description || message || title);
  const inputVariables = workflowAiVariableSpecs((item as unknown as Record<string, unknown>).inputVariables ?? (item as unknown as Record<string, unknown>).input_variables, [
    { name: "workflow_context", type: "object", required: false },
  ]);
  const outputVariables = workflowAiVariableSpecs((item as unknown as Record<string, unknown>).outputVariables ?? (item as unknown as Record<string, unknown>).output_variables, [
    { name: `step_${index + 1}_result`, type: "object" },
  ]);
  const outputKey = outputVariables[0]?.name || `step_${index + 1}_result`;
  const goal = workflowAiStringField(item, "goal") || description || `完成 ${title}`;
  const environment = workflowAiEnvironmentField(item, "根据当前 Workflow 图层、上游输出和用户确认信息执行。");
  const scriptSummary = workflowAiStringField(item, "scriptSummary", "script_summary") || description || goal;
  const validation = workflowAiStringField(item, "validationSummary", "validation_summary", "validation") || `确认“${title}”输出可供后续节点使用。`;
  const generatedScript = workflowAiStringField(item, "script");
  const script = generatedScript || [
    "# Workflow AI generated step",
    `# 目标: ${pythonComment(goal)}`,
    `# 环境: ${pythonComment(environment)}`,
    `# 脚本: ${pythonComment(scriptSummary)}`,
    `# 验证: ${pythonComment(validation)}`,
    "import json",
    "",
    `STEP_TITLE = ${JSON.stringify(title)}`,
    `STEP_GOAL = ${JSON.stringify(goal)}`,
    "",
    "result = {",
    "    'ok': True,",
    "    'step': STEP_TITLE,",
    "    'goal': STEP_GOAL,",
    "    'note': '请在执行前按目标环境补充真实动作。',",
    "}",
    "",
    `print(json.dumps({'${outputKey}': result}, ensure_ascii=False))`,
  ].join("\n");

  return {
    index: index + 1,
    total: 1,
    title,
    goal,
    environment,
    scriptSummary,
    inputVariables,
    outputVariables,
    validationSummary: validation,
    script,
    outputKey,
    validation,
  };
}

function workflowAiAppendSourceNode(graph: RunnerGraph, selectedNodeId = "", appendAfterNodeId = "") {
  const nodes = graph.nodes || [];
  const startNode = nodes.find((node) => node.id === "start" || String(node.type || "").toLowerCase() === "start");
  const endNode = nodes.find((node) => node.id === "end" || String(node.type || "").toLowerCase() === "end");
  const explicitAppendNode = appendAfterNodeId ? nodes.find((node) => node.id === appendAfterNodeId) : undefined;
  if (explicitAppendNode) return { sourceNode: explicitAppendNode, startNode, endNode };
  const selectedNode = selectedNodeId ? nodes.find((node) => node.id === selectedNodeId) : undefined;
  if (selectedNode) return { sourceNode: selectedNode, startNode, endNode };
  const orderedTail = [...workflowAiOrderedNodes(graph)]
    .reverse()
    .find((node) => node.id !== startNode?.id && node.id !== endNode?.id);
  return {
    sourceNode: orderedTail || startNode || nodes[0],
    startNode,
    endNode,
  };
}

function workflowAiDefaultTargets(graph: RunnerGraph): string[] {
  return workflowDefaultTargetLabels(workflowHostGroups(graph));
}

function workflowAiVisibleStepPatch(graph: RunnerGraph, patchId: string, item: WorkflowAiPlanItem, message: string, workflowId: string, baseRevision: string, selectedNodeId = "", index = 0, appendAfterNodeId = ""): WorkflowPatch {
  const details = workflowAiStepDetails(item, message, index);
  const title = details.title;
  const nodeLabel = workflowAiPlanNodeLabel(item, title);
  const nodeAction = workflowAiPlanNodeAction(item);
  const itemId = item.id || title;
  const nodes = graph.nodes || [];
  const edges = graph.edges || [];
  const existingIds = new Set(nodes.map((node) => node.id));
  let nodeId = workflowAiStepNodeId(itemId);
  let suffix = 2;
  while (existingIds.has(nodeId)) {
    nodeId = `${workflowAiStepNodeId(itemId)}-${suffix}`;
    suffix += 1;
  }
  const { sourceNode, endNode } = workflowAiAppendSourceNode(graph, selectedNodeId, appendAfterNodeId);
  const outgoing = edges.find((edge) => edge.source === sourceNode?.id && (!endNode || edge.target === endNode.id)) ||
    edges.find((edge) => edge.source === sourceNode?.id);
  const targetNode = nodes.find((node) => node.id === outgoing?.target) || endNode;
  const sourcePosition = sourceNode?.position || { x: 120, y: 160 };
  const targetPosition = targetNode?.position || { x: Number(sourcePosition.x || 0) + 320, y: sourcePosition.y || 160 };
  const position = {
    x: Math.round(Number(sourcePosition.x || 0) + 320),
    y: Math.round(Number(sourcePosition.y || 160)),
  };
  const nextPort = sourceNode?.ports && Array.isArray(sourceNode.ports)
    ? sourceNode.ports.find((port) => port.type === "output")?.id || "next"
    : "next";
  const targetPort = targetNode?.ports && Array.isArray(targetNode.ports)
    ? targetNode.ports.find((port) => port.type === "input")?.id || "in"
    : "in";
  const operations: WorkflowPatch["operations"] = [{
    op: "add_node",
    node: {
      id: nodeId,
      type: workflowAiPlanNodeType(item),
      label: nodeLabel,
      description: item.description || message,
      position,
      ports: workflowAiNodePorts(),
      inputs: details.inputVariables?.map((variable) => ({
        key: variable.name,
        type: variable.type || "string",
        label: variable.name,
        required: Boolean(variable.required),
      })) || [],
      outputs: details.outputVariables?.map((variable) => ({
        key: variable.name,
        type: variable.type || "object",
        label: variable.name,
      })) || [],
      step: {
        name: nodeLabel,
        action: nodeAction,
        targets: workflowAiDefaultTargets(graph),
        args: {
          generated_by: "workflow_ai",
          instruction: message,
          goal: details.goal,
          environment: details.environment,
          script_summary: details.scriptSummary,
          validation: details.validation,
          script: details.script,
          env: {},
        },
      },
      ui: {
        ai_generated: true,
        ai_patch_id: patchId,
        ai_goal: details.goal,
        ai_environment: details.environment,
        ai_script_summary: details.scriptSummary,
      },
    },
  }];
  if (outgoing?.id) {
    operations.push({ op: "delete_edge", edgeId: outgoing.id });
  }
  if (sourceNode?.id) {
    operations.push({
      op: "add_edge",
      edge: {
        id: `${sourceNode.id}-${nodeId}`,
        source: sourceNode.id,
        source_port: nextPort,
        target: nodeId,
        target_port: "in",
        kind: "next",
      },
    });
  }
  if (targetNode?.id && targetNode.id !== nodeId) {
    operations.push({
      op: "add_edge",
      edge: {
        id: `${nodeId}-${targetNode.id}`,
        source: nodeId,
        source_port: "next",
        target: targetNode.id,
        target_port: targetPort,
        kind: "next",
      },
    });
    const targetX = Number(targetPosition.x || 0);
    if (targetX <= position.x + 220) {
      operations.push({
        op: "update_node",
        nodeId: targetNode.id,
        fields: {
          position: { x: position.x + 320, y: Number(targetPosition.y || position.y) },
        },
      });
    }
  }
  return {
    id: patchId,
    workflowId,
    baseRevision,
    summary: title,
    operations,
  };
}

function workflowAiCreateDraftPlanPayload(plan?: WorkflowEditPlan, fallbackMessage = "") {
  const message = String(plan?.message || fallbackMessage || plan?.items?.[0]?.title || "Workflow draft").trim();
  const title = message.length > 48 ? `${message.slice(0, 48)}...` : message;
  return {
    version: 1,
    title: title || "Workflow draft",
    intent: message,
    trigger: { type: "manual", summary: "Workflow AI Chat create mode" },
    nodes: [{
      id: "collect",
      kind: "search",
      title: "收集证据",
      description: message,
      action: "script.python",
    }],
    outputs: [{ id: "summary", target: "return", description: "Workflow result summary" }],
    validation_strategy: { enabled: false, provider: "none" },
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
    hint: "请确认 ai-server 已用最新 start.sh 启动，Runner Studio API 会随 ai-server 默认挂载。",
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
  const nodeId = String(node.id || "");
  useEffect(() => { setDraft(cloneNode(node)); setTab("settings"); setScriptModalOpen(false); }, [nodeId]);
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
    ...(actionKind === "script" ? [["script", "脚本"]] : []),
    ["settings", "设置"],
    ...(isCodeAction ? [["io", "输入输出"]] : []),
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
                {actionKind === "command" ? (
                  <label>命令<input data-testid="runner-node-command-editor" value={String(stepArgs.cmd || "")} placeholder="df -h" onChange={(event) => updateStepArg("cmd", event.target.value)} /></label>
                ) : null}
              </>
            )}
          </section>
        ) : null}
        {tab === "script" && actionKind === "script" ? (
          <section className="node-config-form runner-node-script-tab" data-testid="runner-node-script-tab">
            <section className="runner-script-context" data-testid="runner-script-context">
              <strong>脚本上下文</strong>
              <p>目标：{displayName}</p>
              <p>动作：{actionName || "-"}</p>
            </section>
            <label className="runner-node-script-field">
              <span className="runner-node-field-label">脚本内容<button type="button" data-testid="runner-node-script-expand" aria-label="放大脚本编辑器" onClick={() => setScriptModalOpen(true)}><Maximize2 size={14} />放大</button></span>
              <textarea
                data-testid="runner-node-script-editor"
                value={String(stepArgs.script || "")}
                placeholder={actionName === "script.python" ? "print('ok')" : "set -e\necho ok"}
                rows={9}
                spellCheck={false}
                onChange={(event) => updateStepArg("script", event.target.value)}
              />
            </label>
            {(actionName === "script.shell" || actionName === "script.python") ? (
              <label>脚本引用<input value={String(stepArgs.script_ref || "")} placeholder="restore.sh" onChange={(event) => updateStepArg("script_ref", event.target.value)} /></label>
            ) : null}
          </section>
        ) : null}
        {tab === "io" && isCodeAction ? (
          <section className="node-config-form runner-node-io-tab" data-testid="runner-node-io-tab">
            <div className="runner-editor-section-head">
              <strong>输入输出</strong>
              <span>配置当前脚本或命令节点读取的变量，以及它向后续节点暴露的输出。</span>
            </div>
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
        {scriptModalOpen && actionKind === "script" ? (
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
    <details className="publish-review-card runner-variable-inspector runner-run-collapsible" data-testid="runner-variable-inspector">
      <summary>
        <span>变量检查器</span>
        <small>{selectedNodeId ? `当前节点：${selectedNodeId}` : "按工作流末端上下文展示"}</small>
      </summary>
      <div className="runner-run-collapsible-body">
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
      </div>
    </details>
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

function normalizedRunnerRunStatus(status: string) {
  return String(status || "unknown").toLowerCase();
}

function runnerRunStatusLabel(status: string) {
  const normalized = normalizedRunnerRunStatus(status);
  if (normalized === "success" || normalized === "completed") return "成功";
  if (normalized === "failed" || normalized === "error") return "失败";
  if (normalized === "running") return "运行中";
  if (normalized === "queued" || normalized === "pending" || normalized === "waiting") return "等待中";
  return "未知";
}

function runnerRunNodeStatusIcon(status: string) {
  const normalized = normalizedRunnerRunStatus(status);
  if (normalized === "success" || normalized === "completed") return <CheckCircle size={14} aria-hidden="true" />;
  if (normalized === "failed" || normalized === "error") return <AlertTriangle size={14} aria-hidden="true" />;
  if (normalized === "running" || normalized === "queued" || normalized === "pending" || normalized === "waiting") return <LoaderCircle size={14} aria-hidden="true" />;
  return <Circle size={14} aria-hidden="true" />;
}

function RunnerRunStatusBadge({ status, testId }: { status: string; testId?: string }) {
  const normalized = normalizedRunnerRunStatus(status);
  return (
    <span className={`runner-run-node-status status-${normalized}`} data-testid={testId} title={status}>
      <span className="runner-run-node-status-icon">{runnerRunNodeStatusIcon(normalized)}</span>
      <span>{runnerRunStatusLabel(normalized)}</span>
    </span>
  );
}

function RunnerRunPanel({ state, graph, selectedNodeId, onSelectNode }: { state: RunState; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const logs = state.logs || [];
  const selectedLogs = selectedNodeId ? logs.filter((log: { nodeId?: string }) => String(log.nodeId || "") === selectedNodeId) : [];
  const selectedLogLabel = selectedNodeId ? (selectedLogs.length ? `${selectedLogs.length} 条日志` : "当前节点暂无日志") : "选择节点后查看";
  const runFailure = readableFailureMessage(state.message || state.error);
  const failedNodes = Object.values(state.nodes || {}).filter((node: { status?: string }) => ["failed", "error"].includes(String(node.status || "").toLowerCase()));
  const hasFailureDetails = Boolean(runFailure || failedNodes.length);
  return (
    <section className="runner-run-panel" data-testid="runner-run-panel">
      <div className="publish-review-card">
        <h3>节点</h3>
        <div className="runner-run-node-list">
          {Object.values(state.nodes || {}).map((node) => {
            const message = runnerNodeFailureMessage(node, logs);
            const nodeState = getRunnerNodeRunState(state, node.nodeId);
            const nodeStatus = nodeState.status || normalizedRunnerRunStatus(String(node.status || "unknown"));
            const nodeStatusLabel = nodeState.label || runnerRunStatusLabel(nodeStatus);
            return (
              <button key={node.nodeId} type="button" className={selectedNodeId === node.nodeId ? "active" : ""} data-testid={`runner-run-node-${node.nodeId}`} onClick={() => onSelectNode(node.nodeId)}>
                <strong>
                  <span className="runner-run-node-title">{node.nodeId}{node.stepName && node.stepName !== node.nodeId ? ` · ${node.stepName}` : ""}</span>
                  <span className={`runner-run-node-status status-${nodeStatus}`} data-testid={`runner-run-node-status-${node.nodeId}`} title={String(node.status || "")}>
                    <span className="runner-run-node-status-icon" data-testid={`runner-run-node-status-icon-${node.nodeId}`}>{runnerRunNodeStatusIcon(nodeStatus)}</span>
                    <span>{nodeStatusLabel}</span>
                  </span>
                </strong>
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
      <details className="publish-review-card runner-run-collapsible runner-run-log-card" data-testid="runner-run-logs" open={hasFailureDetails}>
        <summary>
          <span>stdout / stderr / SSE</span>
          <small>{selectedLogLabel}</small>
        </summary>
        <div className="runner-run-collapsible-body">
          {selectedNodeId ? (
            selectedLogs.length
              ? selectedLogs.map((log, index) => <pre key={`${log.nodeId}-${index}`}>{log.nodeId} {log.stream}: {log.message}</pre>)
              : <p>{hasFailureDetails ? "当前节点暂无日志，请查看上方失败原因。" : "当前节点暂无日志。"}</p>
          ) : <p>请选择上方节点查看 stdout / stderr / SSE。</p>}
        </div>
      </details>
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
          <span className="runner-run-record-meta"><span>{activeRecord.runId}</span><RunnerRunStatusBadge status={activeRecord.status} testId="runner-run-active-status" /></span>
        </div>
        <section data-testid="runner-run-detail-panel">
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
              <div><strong>{record.runId}</strong><RunnerRunStatusBadge status={record.status} testId={`runner-run-record-status-${record.runId}`} /></div>
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

function RunnerNodeRunDetails({ state, graph, selectedNodeId, onSelectNode }: { state: RunState; graph: RunnerGraph; selectedNodeId: string; onSelectNode: (id: string) => void }) {
  const fallbackNodeId = Object.keys(state.nodes || {})[0] || graph.nodes?.[0]?.id || "";
  const nodeId = selectedNodeId || fallbackNodeId;
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

function workflowDefaultTargetLabels(groups: RunnerHostGroup[]): string[] {
  const firstLabel = groups.map((group) => String(group.label || "").trim()).find(Boolean);
  return firstLabel ? [firstLabel] : ["local"];
}

function shouldFillDefaultTargets(node: RunnerNode) {
  const action = String(node.step?.action || "").trim();
  if (!action) return false;
  if (normalizeTargetLabels(node.step?.targets || []).length) return false;
  const type = String(node.type || "").toLowerCase();
  if (["start", "end"].includes(type) || ["start", "end"].includes(node.id)) return false;
  const kind = actionEditorKind(action);
  return kind === "script" || kind === "command";
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
  const disabledReason = !validatedHash ? "缺少当前 validated_graph_hash，发布前必须先校验当前 graph。" : dryHash !== validatedHash ? "发布前检查未通过或已过期，发布前必须重新检查当前 graph。" : !validationPassed ? "校验未通过，修复错误后才能发布。" : !note.trim() ? "发布说明不能为空。" : "";
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

export function RunnerStudioPage() {
  const params = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const routeWorkflowName = String(params.workflowName || "").trim();
  const workflowAiSearchParams = useMemo(() => new URLSearchParams(location.search), [location.search]);
  const workflowAiCreateMode = workflowAiSearchParams.get("workflow_ai") === "create";
  const workflowAiInitialPrompt = workflowAiSearchParams.get("prompt") || "";
  const [loading, setLoading] = useState(false);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [workflowSearchQuery, setWorkflowSearchQuery] = useState("");
  const [workflowPage, setWorkflowPage] = useState(1);
  const [actions, setActions] = useState<RunnerAction[]>(FALLBACK_ACTIONS);
  const [selectedWorkflowName, setSelectedWorkflowName] = useState(routeWorkflowName);
  const [saveState, setSaveState] = useState<SaveState>({ status: "idle" });
  const [apiNotice, setApiNotice] = useState<ApiNotice>(null);
  const [apiNoticeDismissed, setApiNoticeDismissed] = useState(false);
  const [deleteWorkflowName, setDeleteWorkflowName] = useState("");
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
  const [workflowEventDrawerOpen, setWorkflowEventDrawerOpen] = useState(false);
  const [workflowAiEvents, setWorkflowAiEvents] = useState<WorkflowAiEvent[]>([]);
  const [workflowAiStage, setWorkflowAiStage] = useState("context_loaded");
  const [workflowAiPlan, setWorkflowAiPlan] = useState<WorkflowEditPlan | undefined>();
  const [workflowAiPatch, setWorkflowAiPatch] = useState<WorkflowPatch | undefined>();
  const [workflowAiResult, setWorkflowAiResult] = useState<WorkflowPatchResult | undefined>();
  const [workflowAiEffectStatus, setWorkflowAiEffectStatus] = useState<WorkflowAiEffectStatus | undefined>();
  const [workflowAiConflict, setWorkflowAiConflict] = useState("");
  const [workflowAiReadonlyAnswer, setWorkflowAiReadonlyAnswer] = useState("");
  const [workflowAiReadonlyAnswerTitle, setWorkflowAiReadonlyAnswerTitle] = useState("工作流说明");
  const [workflowAiPreviewGraph, setWorkflowAiPreviewGraph] = useState<RunnerGraph | undefined>();
  const [workflowAiActiveStep, setWorkflowAiActiveStep] = useState<WorkflowAiActiveStep | undefined>();
  const [workflowAiStepHistory, setWorkflowAiStepHistory] = useState<WorkflowAiStepHistoryItem[]>([]);
  const [workflowAiToolLog, setWorkflowAiToolLog] = useState<WorkflowAiToolLogEntry[]>([]);
  const [workflowAiSessionNonce, setWorkflowAiSessionNonce] = useState(0);
  const [aiHighlightedNodeIds, setAiHighlightedNodeIds] = useState<string[]>([]);
  const [opsManualOpen, setOpsManualOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [debugDockOpen, setDebugDockOpen] = useState(false);
  const [toolbarMoreOpen, setToolbarMoreOpen] = useState(false);
  const [editingWorkflowTitle, setEditingWorkflowTitle] = useState(false);
  const [workflowTitleDraft, setWorkflowTitleDraft] = useState("");
  const runInFlightRef = useRef(false);
  const runLockUntilRef = useRef(0);
  const workflowAiUndoGraphRef = useRef<RunnerGraph | null>(null);
  const workflowAiAppliedGraphRef = useRef<RunnerGraph | null>(null);
  const workflowAiEventSequenceRef = useRef(0);
  const importInputRef = useRef<HTMLInputElement | null>(null);
  const headerActionsRef = useRef<{ back: () => void; toolbar: (key: ToolbarActionKey) => void }>({ back: () => {}, toolbar: () => {} });

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
  const selectedNode = graph.nodes?.find((node) => node.id === selectedNodeId) || null;
  const openNodePanel = useCallback((nodeId: string) => {
    if (nodeId && shouldUseSingleWorkflowSidePanel()) {
      setAiOpen(false);
      setWorkflowEventDrawerOpen(false);
      setRunDrawerOpen(false);
    }
    setSelectedNodeId(nodeId);
  }, []);
  const openWorkflowAiPanel = useCallback(() => {
    if (shouldUseSingleWorkflowSidePanel()) {
      setSelectedNodeId("");
      setRunDrawerOpen(false);
    }
    setWorkflowEventDrawerOpen(false);
    setAiOpen(true);
  }, []);
  const openWorkflowEventPanel = useCallback(() => {
    if (shouldUseSingleWorkflowSidePanel()) {
      setSelectedNodeId("");
      setRunDrawerOpen(false);
    }
    setAiOpen(false);
    setWorkflowEventDrawerOpen(true);
  }, []);
  const selectNodesFromWorkflowEvent = useCallback((nodeIds: string[]) => {
    setAiHighlightedNodeIds(nodeIds);
    const nodeId = nodeIds[0];
    if (!nodeId) return;
    setWorkflowEventDrawerOpen(false);
    if (shouldUseSingleWorkflowSidePanel()) setAiOpen(false);
    setSelectedNodeId(nodeId);
  }, []);
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
    setWorkflowAiPlan(undefined);
    setWorkflowAiPatch(undefined);
    setWorkflowAiResult(undefined);
    setWorkflowAiEffectStatus(undefined);
    setWorkflowAiConflict("");
    setWorkflowAiReadonlyAnswer("");
    setWorkflowAiReadonlyAnswerTitle("工作流说明");
    setWorkflowAiPreviewGraph(undefined);
    setWorkflowAiActiveStep(undefined);
    setWorkflowAiStepHistory([]);
    setWorkflowAiToolLog([]);
    setWorkflowAiEvents([]);
    setWorkflowAiSessionNonce((current) => current + 1);
    setWorkflowEventDrawerOpen(false);
    setAiHighlightedNodeIds([]);
    setEditingWorkflowTitle(false);
    setWorkflowTitleDraft("");
    setDeleteWorkflowName("");
    workflowAiUndoGraphRef.current = null;
    workflowAiAppliedGraphRef.current = null;
    navigate(name ? `/runner/${encodeURIComponent(name)}` : "/runner");
  }, [navigate]);

  useEffect(() => { setSelectedWorkflowName(routeWorkflowName); }, [routeWorkflowName]);

  useEffect(() => {
    setWorkflowPage((current) => Math.min(Math.max(current, 1), workflowPageCount));
  }, [workflowPageCount]);

  useEffect(() => {
    if (!workflowAiCreateMode) return;
    setAiOpen(true);
    setWorkflowAiStage("context_loaded");
  }, [workflowAiCreateMode, workflowAiInitialPrompt]);

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

  function updateWorkflowTitle(nextTitle: string) {
    if (!selectedWorkflow || !selectedWorkflowName) return;
    const title = nextTitle.trim() || selectedWorkflow.title || selectedWorkflow.name || selectedWorkflowName;
    const normalized = reconcileRunnerRuntimeConfig(normalizeGraph({
      ...graph,
      workflow: { ...objectValue(graph.workflow), title },
    }, selectedWorkflowName));
    const nextWorkflow = {
      ...selectedWorkflow,
      title,
      graph: normalized,
      status: "draft",
      local_draft: true,
      validated_graph_hash: "",
      dry_run_graph_hash: "",
      validation_result: { valid: false, errors: [], warnings: [] },
    };
    upsertWorkflow(selectedWorkflowName, nextWorkflow);
    saveLocalDraft(nextWorkflow);
    setSaveState({ status: "pending", message: "未保存" });
  }

  function appendWorkflowAiEvent(event: Omit<WorkflowAiEvent, "id" | "createdAt"> & { id?: string; createdAt?: string }) {
    const createdAt = event.createdAt || new Date().toISOString();
    workflowAiEventSequenceRef.current += 1;
    const id = event.id || `wf-ai-event-${createdAt}-${workflowAiEventSequenceRef.current}`;
    setWorkflowAiEvents((current) => [...current, { ...event, id, createdAt }]);
  }

  function updateWorkflowAiToolLog(entry: WorkflowAiToolLogEntry) {
    setWorkflowAiToolLog((current) => {
      const index = current.findIndex((item) => item.id === entry.id);
      if (index < 0) return [...current, entry];
      const next = [...current];
      next[index] = { ...next[index], ...entry };
      return next;
    });
  }

  function currentWorkflowAiDrawerSessionId() {
    return `drawer-${workflowKey(selectedWorkflow || {}) || "workflow"}-${workflowAiSessionNonce}`;
  }

  function startNewWorkflowAiSession() {
    setWorkflowAiSessionNonce((current) => current + 1);
    setWorkflowAiStage("context_loaded");
    setWorkflowAiPlan(undefined);
    setWorkflowAiPatch(undefined);
    setWorkflowAiResult(undefined);
    setWorkflowAiEffectStatus(undefined);
    setWorkflowAiConflict("");
    setWorkflowAiReadonlyAnswer("");
    setWorkflowAiReadonlyAnswerTitle("工作流说明");
    setWorkflowAiPreviewGraph(undefined);
    setWorkflowAiActiveStep(undefined);
    setWorkflowAiStepHistory([]);
    setWorkflowAiToolLog([]);
    setAiHighlightedNodeIds([]);
    workflowAiUndoGraphRef.current = null;
    workflowAiAppliedGraphRef.current = null;
  }

  function localWorkflowAiPatchForGraph(baseGraph: RunnerGraph, item: WorkflowAiPlanItem, index = 0, appendAfterNodeId = ""): WorkflowPatch {
    const patchId = `patch-${Date.now()}`;
    return workflowAiVisibleStepPatch(
      baseGraph,
      `${patchId}-${index}`,
      item,
      item.description || item.title || workflowAiPlan?.message || "Workflow AI edit",
      workflowKey(selectedWorkflow || {}),
      workflowAiRevision(selectedWorkflow, graph),
      selectedNodeId,
      index,
      appendAfterNodeId,
    );
  }

  function appendWorkflowAiGraphOperationEvents({
    workflowId,
    sessionId,
    planId,
    planItemId,
    patch,
    item,
  }: {
    workflowId: string;
    sessionId: string;
    planId: string;
    planItemId?: string;
    patch: WorkflowPatch;
    item: WorkflowAiPlanItem;
  }) {
    for (const operation of patch.operations) {
      const node = objectValue(operation.node);
      const edge = objectValue(operation.edge);
      const eventNodeId = String(operation.nodeId || node.id || "");
      if (operation.op === "add_node" && eventNodeId) {
        appendWorkflowAiEvent({
          workflowId,
          sessionId,
          planId,
          planItemId,
          patchId: patch.id,
          type: "workflow.graph.node.added",
          actor: "tool",
          summary: `添加节点：${String(node.label || item.title)}`,
          visibleNodeIds: [eventNodeId],
        });
        if (objectValue(objectValue(node.step).args).script) {
          appendWorkflowAiEvent({
            workflowId,
            sessionId,
            planId,
            planItemId,
            patchId: patch.id,
            type: "workflow.node.script.generated",
            actor: "assistant",
            summary: `生成脚本：${String(node.label || item.title)}`,
            visibleNodeIds: [eventNodeId],
          });
        }
      }
      if (operation.op === "add_edge" && edge.id) {
        const edgeNodeIds = [String(edge.source || ""), String(edge.target || "")].filter(Boolean);
        appendWorkflowAiEvent({
          workflowId,
          sessionId,
          planId,
          planItemId,
          patchId: patch.id,
          type: "workflow.graph.edge.added",
          actor: "tool",
          summary: `连接节点：${String(edge.source || "-")} -> ${String(edge.target || "-")}`,
          visibleNodeIds: edgeNodeIds,
        });
      }
    }
  }

  async function handleWorkflowAiSubmit(rawMessage: string) {
    let message = rawMessage;
    const workflowId = workflowKey(selectedWorkflow || {});
    const drawerSessionId = currentWorkflowAiDrawerSessionId();
    const baseRevision = workflowAiRevision(selectedWorkflow, graph);
    appendWorkflowAiEvent({
      workflowId,
      sessionId: drawerSessionId,
      type: "message.user",
      actor: "user",
      summary: message,
    });
    if (workflowAiStage === "plan_review" && workflowAiPlan) {
      const reply = parseWorkflowAiPlanReply(message);
      appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        planId: workflowAiPlan.id,
        type: `plan.reply.${reply.type}`,
        actor: "user",
        summary: message,
      });
      if (reply.type === "approve_plan") {
        await handleWorkflowAiConfirmPlan(workflowAiPlan.id);
        return;
      }
      if (reply.type === "cancel_plan") {
        setWorkflowAiPatch(undefined);
        setWorkflowAiResult(undefined);
        setWorkflowAiEffectStatus(undefined);
        setWorkflowAiConflict("");
        setWorkflowAiReadonlyAnswer("已取消本次计划，我没有修改画布。你可以继续描述新的调整目标。");
        setWorkflowAiReadonlyAnswerTitle("工作流说明");
        setWorkflowAiPreviewGraph(undefined);
        setWorkflowAiActiveStep(undefined);
        setWorkflowAiStepHistory([]);
        setWorkflowAiToolLog([]);
        appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        planId: workflowAiPlan.id,
          type: "workflow.ai.plan.cancelled",
          actor: "assistant",
          summary: "用户取消计划，本次未修改 Workflow。",
        });
        setWorkflowAiStage("complete");
        return;
      }
      appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        planId: workflowAiPlan.id,
        type: "workflow.ai.plan.revised",
        actor: "assistant",
        summary: `用户要求调整计划：${reply.instruction}`,
      });
      message = `${workflowAiPlan.message || ""}\n\n用户要求调整计划：${reply.instruction}`;
    }
    setWorkflowAiPlan(undefined);
    setWorkflowAiPatch(undefined);
    setWorkflowAiResult(undefined);
    setWorkflowAiEffectStatus(undefined);
    setWorkflowAiConflict("");
    setWorkflowAiReadonlyAnswer("");
    setWorkflowAiReadonlyAnswerTitle("工作流说明");
    setWorkflowAiPreviewGraph(undefined);
    setWorkflowAiActiveStep(undefined);
    setWorkflowAiStepHistory([]);
    setWorkflowAiToolLog([]);
    workflowAiAppliedGraphRef.current = null;
    if (!workflowAiCreateMode && !isWorkflowAiEditRequest(message)) {
      setWorkflowAiStage("chatting");
      const chatClientTurnId = `workflow-ai-chat-${Date.now()}`;
      const chatClientMessageId = `${chatClientTurnId}-user`;
      const chatSessionId = `${drawerSessionId}-chat-v2`;
      try {
        const response = await sendChatMessage({
          sessionId: chatSessionId,
          sessionType: "workflow",
          mode: "chat",
          role: "user",
          content: workflowAiChatModelContent(message, selectedWorkflow || null, graph),
          clientTurnId: chatClientTurnId,
          clientMessageId: chatClientMessageId,
          metadata: {
            source: "workflow_ai_chat",
            workflowId,
            drawerSessionId,
            intent: "chat",
            workflowSummary: selectedWorkflow ? describeWorkflowForAi(selectedWorkflow, graph) : "当前还没有打开具体 Workflow。",
          },
        });
        const answer = await waitForWorkflowAiChatResponse(response, chatClientTurnId, chatSessionId);
        setWorkflowAiReadonlyAnswer(answer);
        setWorkflowAiReadonlyAnswerTitle("工作流说明");
        appendWorkflowAiEvent({
          workflowId,
          sessionId: drawerSessionId,
          type: "workflow.ai.chat",
          actor: "assistant",
          summary: "已生成普通对话回复，未修改 Workflow。",
        });
        setWorkflowAiStage("complete");
        return;
      } catch (error) {
        const messageText = errorMessage(error, "Workflow AI Chat 连接失败");
        setWorkflowAiConflict(`Workflow AI Chat 连接失败：${messageText}`);
        appendWorkflowAiEvent({
          workflowId,
          sessionId: drawerSessionId,
          type: "workflow.ai.chat.failed",
          actor: "assistant",
          summary: `Workflow AI Chat 连接失败：${messageText}`,
        });
        setWorkflowAiStage("conflict");
        return;
      }
    }
    setWorkflowAiStage("planning");
    let plan: WorkflowEditPlan;
    try {
      await createRunnerStudioWorkflowAiSession({
        workflowId,
        baseRevision,
        sessionIntent: "edit",
        drawerSessionId,
      });
      const apiPlan = await proposeRunnerStudioWorkflowAiPlan({
        workflowId,
        drawerSessionId,
        message,
      }) as WorkflowEditPlan;
      if (apiPlan?.items?.length) {
        plan = apiPlan;
      } else {
        throw new Error("计划接口没有返回可确认的步骤");
      }
    } catch (error) {
      const messageText = errorMessage(error, "计划生成失败");
      setWorkflowAiConflict(`计划生成失败：${messageText}`);
      appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        type: "workflow.ai.plan.failed",
        actor: "assistant",
        summary: `计划生成失败：${messageText}`,
      });
      setWorkflowAiStage("conflict");
      return;
    }
    setWorkflowAiPlan(plan);
    appendWorkflowAiEvent({
      workflowId,
      sessionId: drawerSessionId,
      planId: plan.id,
      type: "workflow.ai.plan.created",
      actor: "assistant",
      summary: `生成计划：${plan.items.map((item) => item.title).join("、")}`,
    });
    setWorkflowAiStage("plan_review");
  }

  async function handleWorkflowAiConfirmPlan(planId: string) {
    if (!workflowAiPlan || workflowAiPlan.id !== planId) return;
    const workflowId = workflowKey(selectedWorkflow || {}) || slugify(workflowAiPlan.message || "workflow-draft");
    const drawerSessionId = currentWorkflowAiDrawerSessionId();
    appendWorkflowAiEvent({
      workflowId,
      sessionId: drawerSessionId,
      planId,
      type: "workflow.ai.plan.confirmed",
      actor: "user",
      summary: "用户确认计划，开始连续修改 Workflow。",
    });
    setWorkflowAiStage("applying_plan");
    setWorkflowAiStepHistory([]);

    if (workflowAiCreateMode && !selectedWorkflow) {
      let draft;
      try {
        draft = await createRunnerStudioWorkflowAiDraftFromPlan({
          drawerSessionId,
          userConfirmationId: `confirm-plan-${Date.now()}`,
          plan: workflowAiCreateDraftPlanPayload(workflowAiPlan),
        }) as { workflowId?: string; graph?: RunnerGraph; revision?: string; validation?: { valid?: boolean; errors?: string[]; warnings?: string[] }; describe?: { summary?: string; nodeCount?: number; edgeCount?: number } };
      } catch {
        const localName = slugify(workflowAiPlan.message || "workflow-draft");
        const localGraph = createBlankWorkflowGraph(localName, workflowAiPlan.message || localName);
        draft = {
          workflowId: localName,
          graph: localGraph,
          revision: `local-${Date.now()}`,
          validation: { valid: false, warnings: ["created as local fallback draft"] },
          describe: { summary: `${localGraph.nodes.length} nodes, ${localGraph.edges.length} edges`, nodeCount: localGraph.nodes.length, edgeCount: localGraph.edges.length },
        };
      }
      const createdWorkflowId = String(draft.workflowId || draft.graph?.workflow?.name || slugify(workflowAiPlan.message || "workflow-draft"));
      const createdGraph = normalizeGraph(draft.graph || createBlankWorkflowGraph(createdWorkflowId), createdWorkflowId);
      const createdWorkflow = saveLocalDraft({
        name: createdWorkflowId,
        title: String(createdGraph.workflow?.title || workflowAiPlan.message || createdWorkflowId),
        status: "draft",
        graph: createdGraph,
        local_draft: true,
        validated_graph_hash: draft.revision || "",
        validation_result: {
          valid: draft.validation?.valid !== false,
          errors: draft.validation?.errors || [],
          warnings: draft.validation?.warnings || [],
        },
      });
      upsertWorkflow(createdWorkflowId, createdWorkflow);
      setSelectedWorkflowName(createdWorkflowId);
      navigate(`/runner/${encodeURIComponent(createdWorkflowId)}`);
      setSaveState({ status: "local_draft", message: "本地草稿", lastSavedAt: formatSaveTime() });
      setWorkflowAiResult({
        patchId: `plan-${planId}`,
        workflowId: createdWorkflowId,
        revisionBefore: "new-draft",
        revisionAfter: draft.revision || "local-draft",
        effect: { status: "changed", summary: "created workflow draft", affectedNodes: (createdGraph.nodes || []).map((node) => node.id) },
        undoCheckpoint: { id: `undo-plan-${planId}`, patchId: `plan-${planId}` },
        describe: draft.describe || { summary: `${createdGraph.nodes.length} nodes, ${createdGraph.edges.length} edges`, nodeCount: createdGraph.nodes.length, edgeCount: createdGraph.edges.length },
      });
      appendWorkflowAiEvent({
        workflowId: createdWorkflowId,
        sessionId: drawerSessionId,
        planId,
        type: "workflow.ai.generation.completed",
        actor: "tool",
        summary: "创建 Workflow 草稿。",
        visibleNodeIds: (createdGraph.nodes || []).map((node) => node.id),
      });
      setWorkflowAiStage("post_apply_check");
      return;
    }

    workflowAiUndoGraphRef.current = graph;
    let currentGraph = graph;
    const affectedNodeIds: string[] = [];
    let lastPatch: WorkflowPatch | undefined;
    let appendAfterNodeId = "";
    for (const [index, item] of workflowAiPlan.items.entries()) {
      const stepDetails = workflowAiStepDetails(item, item.description || item.title || workflowAiPlan.message, index);
      const activeStep = { ...stepDetails, total: workflowAiPlan.items.length };
      const generatedNodeLabel = workflowAiPlanNodeLabel(item, stepDetails.title);
      const toolLogId = `workflow-ai-step-${planId}-${item.id || index}`;
      setWorkflowAiActiveStep(activeStep);
      setWorkflowAiStepHistory((current) => [
        ...current.filter((step) => step.index !== activeStep.index),
        { ...activeStep, status: "running" },
      ]);
      updateWorkflowAiToolLog({
        id: toolLogId,
        toolName: "workflow.generate_step",
        status: "running",
        inputSummary: `准备生成节点：${generatedNodeLabel}`,
        outputSummary: activeStep.goal ? `目标：${activeStep.goal}` : `正在根据计划生成 ${generatedNodeLabel}`,
      });
      appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        planId,
        planItemId: item.id,
        type: "workflow.ai.step.generating",
        actor: "assistant",
        summary: `正在生成步骤：${item.title}`,
      });
      await delay(workflowAiStepDelayMs());
      const patch = localWorkflowAiPatchForGraph(currentGraph, item, index, appendAfterNodeId);
      const applied = applyWorkflowAiPatchToRunnerGraph(currentGraph, patch);
      currentGraph = applied.graph;
      lastPatch = patch;
      affectedNodeIds.push(...applied.affectedNodes);
      const generatedNodeId = applied.affectedNodes.find((nodeId) => nodeId !== appendAfterNodeId);
      if (generatedNodeId) appendAfterNodeId = generatedNodeId;
      const normalizedStepGraph = reconcileRunnerRuntimeConfig(normalizeGraph(currentGraph, selectedWorkflowName || workflowId || "workflow"));
      updateGraph(normalizedStepGraph);
      setAiHighlightedNodeIds(Array.from(new Set(affectedNodeIds)));
      updateWorkflowAiToolLog({
        id: toolLogId,
        toolName: "workflow.generate_step",
        status: "completed",
        durationMs: workflowAiStepDelayMs(),
        inputSummary: `生成节点：${generatedNodeLabel}`,
        outputSummary: `已添加节点“${generatedNodeLabel}”，并写入目标、环境、脚本和验证说明。`,
      });
      setWorkflowAiStepHistory((current) => current.map((step) => step.index === activeStep.index ? { ...step, status: "completed" } : step));
      appendWorkflowAiGraphOperationEvents({
        workflowId,
        sessionId: drawerSessionId,
        planId,
        planItemId: item.id,
        patch,
        item,
      });
      appendWorkflowAiEvent({
        workflowId,
        sessionId: drawerSessionId,
        planId,
        planItemId: item.id,
        patchId: patch.id,
        type: "workflow.ai.step.completed",
        actor: "assistant",
        summary: `完成步骤：${item.title}`,
        visibleNodeIds: applied.affectedNodes,
      });
    }
    const normalizedAppliedGraph = reconcileRunnerRuntimeConfig(normalizeGraph(currentGraph, selectedWorkflowName || workflowId || "workflow"));
    updateGraph(normalizedAppliedGraph);
    workflowAiAppliedGraphRef.current = normalizedAppliedGraph;
    const uniqueAffected = Array.from(new Set(affectedNodeIds));
    setAiHighlightedNodeIds(uniqueAffected);
    setWorkflowAiPatch(undefined);
    setWorkflowAiPreviewGraph(undefined);
    setWorkflowAiEffectStatus("changed");
    setWorkflowAiResult({
      patchId: lastPatch?.id || `plan-${planId}`,
      workflowId,
      revisionBefore: workflowAiRevision(selectedWorkflow, graph),
      revisionAfter: `local-${Date.now()}`,
      effect: { status: "changed", affectedNodes: uniqueAffected },
      undoCheckpoint: { id: `undo-plan-${planId}`, patchId: lastPatch?.id },
      describe: { summary: `已按计划完成 ${workflowAiPlan.items.length} 个 Workflow 修改`, nodeCount: normalizedAppliedGraph.nodes?.length || 0, edgeCount: normalizedAppliedGraph.edges?.length || 0 },
    });
    appendWorkflowAiEvent({
      workflowId,
      sessionId: drawerSessionId,
      planId,
      type: "workflow.ai.generation.completed",
      actor: "assistant",
      summary: "Workflow AI 已按计划完成修改。",
      visibleNodeIds: uniqueAffected,
    });
    setWorkflowAiActiveStep(undefined);
    setWorkflowAiStage("post_apply_check");
  }

  async function handleWorkflowAiApplyPatch() {
    if (!workflowAiPatch) return;
    if (workflowAiCreateMode && !selectedWorkflow) {
      let draft;
      try {
        draft = await createRunnerStudioWorkflowAiDraftFromPlan({
          drawerSessionId: currentWorkflowAiDrawerSessionId(),
          userConfirmationId: `confirm-${Date.now()}`,
          plan: workflowAiCreateDraftPlanPayload(workflowAiPlan, workflowAiPatch.summary),
        }) as { workflowId?: string; graph?: RunnerGraph; revision?: string; validation?: { valid?: boolean; errors?: string[]; warnings?: string[] }; describe?: { summary?: string; nodeCount?: number; edgeCount?: number } };
      } catch {
        const localName = slugify(workflowAiPatch.summary || workflowAiPlan?.message || "workflow-draft");
        const localGraph = createBlankWorkflowGraph(localName, workflowAiPatch.summary || workflowAiPlan?.message || localName);
        draft = {
          workflowId: localName,
          graph: localGraph,
          revision: `local-${Date.now()}`,
          validation: { valid: false, warnings: ["created as local fallback draft"] },
          describe: { summary: `${localGraph.nodes.length} nodes, ${localGraph.edges.length} edges`, nodeCount: localGraph.nodes.length, edgeCount: localGraph.edges.length },
        };
      }
      const workflowId = String(draft.workflowId || draft.graph?.workflow?.name || slugify(workflowAiPatch.summary || "workflow-draft"));
      const createdGraph = normalizeGraph(draft.graph || createBlankWorkflowGraph(workflowId), workflowId);
      const createdWorkflow = saveLocalDraft({
        name: workflowId,
        title: String(createdGraph.workflow?.title || workflowAiPatch.summary || workflowId),
        status: "draft",
        graph: createdGraph,
        local_draft: true,
        validated_graph_hash: draft.revision || "",
        validation_result: {
          valid: draft.validation?.valid !== false,
          errors: draft.validation?.errors || [],
          warnings: draft.validation?.warnings || [],
        },
      });
      upsertWorkflow(workflowId, createdWorkflow);
      setSelectedWorkflowName(workflowId);
      navigate(`/runner/${encodeURIComponent(workflowId)}`);
      setSaveState({ status: "local_draft", message: "本地草稿", lastSavedAt: formatSaveTime() });
      setAiHighlightedNodeIds([]);
      setWorkflowAiResult({
        patchId: workflowAiPatch.id,
        workflowId,
        revisionBefore: "new-draft",
        revisionAfter: draft.revision || "local-draft",
        effect: { status: "changed", summary: "created workflow draft" },
        undoCheckpoint: { id: `undo-${workflowAiPatch.id}`, patchId: workflowAiPatch.id },
        describe: draft.describe || { summary: `${createdGraph.nodes.length} nodes, ${createdGraph.edges.length} edges`, nodeCount: createdGraph.nodes.length, edgeCount: createdGraph.edges.length },
      });
      setWorkflowAiEffectStatus("changed");
      setWorkflowAiStage("post_apply_check");
      return;
    }
    workflowAiUndoGraphRef.current = graph;
    const localApplied = applyWorkflowAiPatchToRunnerGraph(graph, workflowAiPatch);
    const nextGraph = workflowAiPreviewGraph || localApplied.graph;
    let result: WorkflowPatchResult = {
      patchId: workflowAiPatch.id,
      workflowId: workflowKey(selectedWorkflow || {}),
      revisionBefore: workflowAiPatch.baseRevision,
      revisionAfter: `local-${Date.now()}`,
      effect: {
        status: localApplied.affectedNodes.length || localApplied.metadataChanged ? "changed" : "metadata_only",
        affectedNodes: localApplied.affectedNodes,
      },
      undoCheckpoint: { id: `undo-${workflowAiPatch.id}`, patchId: workflowAiPatch.id },
      describe: {
        summary: `${nextGraph.nodes?.length || 0} nodes, ${nextGraph.edges?.length || 0} edges`,
        nodeCount: nextGraph.nodes?.length || 0,
        edgeCount: nextGraph.edges?.length || 0,
      },
    };
    try {
      result = await applyRunnerStudioWorkflowAiPatch({
        workflowId: workflowAiPatch.workflowId || workflowKey(selectedWorkflow || {}),
        baseRevision: workflowAiPatch.baseRevision,
        patchId: workflowAiPatch.id,
        patch: workflowAiPatch,
        userConfirmationId: `confirm-${Date.now()}`,
        drawerSessionId: currentWorkflowAiDrawerSessionId(),
        reason: workflowAiPatch.summary || "Workflow AI patch",
      }) as WorkflowPatchResult;
    } catch {
      // Fall back to local draft application; the drawer remains on the new workflow-ai path.
    }
    const normalizedAppliedGraph = reconcileRunnerRuntimeConfig(normalizeGraph(nextGraph, selectedWorkflowName || workflowAiPatch.workflowId || "workflow"));
    updateGraph(normalizedAppliedGraph);
    workflowAiAppliedGraphRef.current = normalizedAppliedGraph;
    setAiHighlightedNodeIds(result.effect?.affectedNodes?.length ? result.effect.affectedNodes : localApplied.affectedNodes);
    setWorkflowAiResult(result);
    setWorkflowAiEffectStatus(result.effect?.status);
    setWorkflowAiStage("post_apply_check");
  }

  async function handleWorkflowAiUndo() {
    if (!workflowAiUndoGraphRef.current) {
      setWorkflowAiConflict("undo checkpoint is not available or graph changed manually");
      setWorkflowAiStage("conflict");
      return;
    }
    if (workflowAiAppliedGraphRef.current) {
      const currentGraphSignature = JSON.stringify(reconcileRunnerRuntimeConfig(normalizeGraph(graph, selectedWorkflowName || workflowKey(selectedWorkflow || {}) || "workflow")));
      const appliedGraphSignature = JSON.stringify(workflowAiAppliedGraphRef.current);
      if (currentGraphSignature !== appliedGraphSignature) {
        setWorkflowAiConflict("current workflow changed after the AI patch; undo would overwrite manual edits");
        setWorkflowAiStage("conflict");
        return;
      }
    }
    try {
      await undoRunnerStudioWorkflowAiPatch({
        workflowId: workflowKey(selectedWorkflow || {}),
        drawerSessionId: currentWorkflowAiDrawerSessionId(),
        reason: "user requested workflow ai undo",
      });
    } catch {
      // Local undo is still valid for draft-only fallback sessions.
    }
    updateGraph(workflowAiUndoGraphRef.current);
    workflowAiUndoGraphRef.current = null;
    workflowAiAppliedGraphRef.current = null;
    setWorkflowAiResult(undefined);
    setWorkflowAiPatch(undefined);
    setWorkflowAiEffectStatus(undefined);
    setWorkflowAiPreviewGraph(undefined);
    setWorkflowAiActiveStep(undefined);
    setWorkflowAiStepHistory([]);
    setWorkflowAiToolLog([]);
    setAiHighlightedNodeIds([]);
    setWorkflowAiConflict("");
    setWorkflowAiStage("context_loaded");
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
      setSaveState({ status: "saved", message: "发布前检查通过", lastSavedAt: formatSaveTime() });
    } catch (error) {
      setSaveState({ status: "failed", message: "发布前检查失败", error: (error as Error).message });
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
      if (shouldUseSingleWorkflowSidePanel()) {
        setSelectedNodeId("");
        setWorkflowEventDrawerOpen(false);
        setAiOpen(false);
      }
      setRunDrawerMode("history");
      setRunDrawerOpen(true);
    }
    if (key === "publish") setPublishOpen(true);
    if (key === "ai-generate") openWorkflowAiPanel();
    if (key === "ops-manual") setOpsManualOpen(true);
  }

  headerActionsRef.current = {
    back: () => selectWorkflow(""),
    toolbar: handleToolbarAction,
  };

  const runnerHeaderContent = useMemo(() => selectedWorkflow ? (
    <div className="runner-studio-topbar" data-testid="runner-studio-topbar">
      <div className="runner-studio-current-workflow"><button type="button" className="runner-studio-back-button" data-testid="runner-back-to-library" onClick={() => headerActionsRef.current.back()}><ArrowLeft size={15} />工作流</button>{editingWorkflowTitle ? <input className="runner-workflow-title-input" data-testid="runner-workflow-title-input" value={workflowTitleDraft} autoFocus onChange={(event) => setWorkflowTitleDraft(event.currentTarget.value)} onBlur={() => { updateWorkflowTitle(workflowTitleDraft); setEditingWorkflowTitle(false); }} onKeyDown={(event) => { if (event.key === "Enter") { updateWorkflowTitle(workflowTitleDraft); setEditingWorkflowTitle(false); } if (event.key === "Escape") { setEditingWorkflowTitle(false); setWorkflowTitleDraft(selectedWorkflow.title || selectedWorkflow.name); } }} /> : <button type="button" className="runner-workflow-title-button" data-testid="runner-workflow-title-display" onClick={() => { setWorkflowTitleDraft(selectedWorkflow.title || selectedWorkflow.name); setEditingWorkflowTitle(true); }} title="编辑工作流名称"><h1>{selectedWorkflow.title || selectedWorkflow.name}</h1></button>}<span className="runner-studio-status">{selectedWorkflow.status || "draft"}</span><span className={`runner-studio-save-state status-${saveState.status || "idle"}`} data-testid="runner-save-state">{saveStateLabel(saveState)}</span></div>
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
  ) : null, [editingWorkflowTitle, runActionDisabled, runActionTitle, saveState.error, saveState.lastSavedAt, saveState.message, saveState.status, selectedWorkflow?.name, selectedWorkflow?.status, selectedWorkflow?.title, toolbarMoreOpen, workflowTitleDraft]);

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

  const workflowAiRevisionValue = workflowAiRevision(selectedWorkflow, graph);
  const workflowAiValidationSource = selectedWorkflow?.validation_result || selectedWorkflow?.validationResult;
  const workflowAiValidation = {
    valid: workflowAiValidationSource?.valid !== false,
    errors: (workflowAiValidationSource?.errors || []).map(String),
    warnings: (workflowAiValidationSource?.warnings || []).map(String),
  };
  const workflowAiWorkflowId = workflowKey(selectedWorkflow || {});
  const workflowAiDrawerVisible = Boolean(selectedWorkflow || workflowAiCreateMode);
  const workflowAiWorkflowName = selectedWorkflow ? (selectedWorkflow.title || selectedWorkflow.name) : "新建 Workflow";
  const canvasGraph = workflowAiPreviewGraph || graph;

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
          <>
            <div className={`runner-studio-workspace ${selectedNode ? "with-node-panel" : ""}`}>
              <section className="runner-studio-main"><section className="runner-studio-canvas" aria-label="工作流画布" data-testid="runner-studio-canvas"><RunnerCanvas graph={canvasGraph} actions={actions} runState={runState} focusNodeId={runFocusNodeId} selectedNodeId={selectedNodeId} aiHighlightedNodeIds={aiHighlightedNodeIds} fullscreen={fullscreen} onUpdateGraph={updateGraph} onSelectNode={openNodePanel} onOpenNodeConfig={openNodePanel} onDeleteNode={deleteGraphNodeById} onNodeAction={() => {}} onToggleFullscreen={() => setFullscreen((value) => !value)} /></section></section>
              {selectedNode ? <section className="runner-node-panel-modal" role="dialog" aria-modal="false" aria-label="节点配置面板" data-testid="runner-node-panel-modal"><RunnerNodePanel node={selectedNode} graph={graph} runState={runState} onClose={() => setSelectedNodeId("")} onApply={(node) => { updateGraph({ ...graph, nodes: (graph.nodes || []).map((item) => item.id === node.id ? node : item) }); setSelectedNodeId(node.id); }} onRunNode={(nodeId) => void runWorkflow(nodeId)} onDelete={deleteGraphNodeById} /></section> : null}
              {debugDockOpen ? <RunnerDebugDock graph={graph} state={runState} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} onClose={() => setDebugDockOpen(false)} /> : null}
            </div>
          </>
        )}
        {workflowAiDrawerVisible ? (
          <WorkflowAiDrawer
            open={aiOpen}
            context={{
              workflowId: workflowAiWorkflowId,
              workflowName: workflowAiWorkflowName,
              revision: selectedWorkflow ? workflowAiRevisionValue : "new-draft",
              selectedNodeId: selectedWorkflow ? selectedNodeId : "",
              saveState: saveState.status,
              lastModifiedLabel: saveState.lastSavedAt ? `修改于 ${saveState.lastSavedAt}` : (saveState.status === "pending" ? "有未保存修改" : "修改时间 -"),
              validation: selectedWorkflow ? workflowAiValidation : { valid: false, warnings: ["create mode"] },
              manualBinding: null,
            }}
            stage={workflowAiStage}
            session={{
              id: currentWorkflowAiDrawerSessionId(),
              workflowId: workflowAiWorkflowId,
              baseRevision: selectedWorkflow ? workflowAiRevisionValue : "new-draft",
              activeRevision: selectedWorkflow ? workflowAiRevisionValue : "new-draft",
              sessionIntent: selectedWorkflow ? "edit" : "create",
              status: workflowAiStage === "budget_paused" ? "budget_paused" : "active",
              stepBudget: {
                maxPatchReviewsPerTurn: 3,
                usedPatchReviews: workflowAiStage === "budget_paused" ? 3 : 0,
              },
            }}
            plan={workflowAiPlan}
            patch={workflowAiPatch}
            result={workflowAiResult}
            effectStatus={workflowAiEffectStatus}
            conflictReason={workflowAiConflict}
            readonlyAnswer={workflowAiReadonlyAnswer}
            readonlyAnswerTitle={workflowAiReadonlyAnswerTitle}
            toolLog={workflowAiToolLog}
            stepHistory={workflowAiStepHistory}
            activeStep={workflowAiActiveStep}
            onClose={() => setAiOpen(false)}
            onSubmit={handleWorkflowAiSubmit}
            onApplyPatch={handleWorkflowAiApplyPatch}
            onRejectApply={() => {}}
            onUndo={handleWorkflowAiUndo}
            onContinue={() => setWorkflowAiStage("patch_generating")}
            onNewSession={startNewWorkflowAiSession}
            onOpenEvents={openWorkflowEventPanel}
            initialMessage={workflowAiCreateMode ? workflowAiInitialPrompt : ""}
          />
        ) : null}
        <WorkflowEventDrawer
          open={workflowEventDrawerOpen}
          events={workflowAiEvents}
          onClose={() => setWorkflowEventDrawerOpen(false)}
          onBackToAi={() => {
            setWorkflowEventDrawerOpen(false);
            openWorkflowAiPanel();
          }}
          onSelectNodeIds={selectNodesFromWorkflowEvent}
        />
        {selectedWorkflow && runDrawerOpen ? <section className="runner-studio-run-drawer-backdrop" role="dialog" aria-modal="true" aria-label={runDrawerMode === "history" ? "运行详情" : "节点运行详情"} data-testid="runner-run-drawer"><aside className="runner-studio-run-drawer-panel"><header className="runner-studio-run-drawer-head"><div><strong>{runDrawerMode === "history" ? "运行详情" : "节点运行详情"}</strong><span>{runDrawerMode === "history" ? "每次运行一行记录，可快速定位失败步骤。" : "当前节点的上次运行、日志和变量输出。"}</span></div><button type="button" className="runner-run-drawer-close" data-testid="runner-run-drawer-close" aria-label="关闭运行详情" onClick={() => setRunDrawerOpen(false)}><X size={18} /></button></header><div className="runner-studio-run-drawer-body">{runDrawerMode === "history" ? <RunnerRunHistoryPanel records={runRecords} currentState={runState} currentEvents={runEvents} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} /> : <RunnerNodeRunDetails state={runState} graph={graph} selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} />}</div></aside></section> : null}
      </section>
      {deleteWorkflowTarget ? <WorkflowDeleteConfirmDialog workflow={deleteWorkflowTarget} onCancel={() => setDeleteWorkflowName("")} onConfirm={() => void deleteWorkflow(workflowKey(deleteWorkflowTarget))} /> : null}
      {publishOpen && selectedWorkflow ? <PublishReviewModal workflow={selectedWorkflow} onClose={() => setPublishOpen(false)} onPublished={(payload) => { upsertWorkflow(selectedWorkflowName, { ...payload, status: payload.status || "published" }); setPublishOpen(false); }} /> : null}
      {opsManualOpen && selectedWorkflow ? <OpsManualCandidateModal workflow={selectedWorkflow} graph={graph} onClose={() => setOpsManualOpen(false)} /> : null}
    </section>
  );
}
