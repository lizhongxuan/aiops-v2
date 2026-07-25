import type { WorkflowCandidateFact } from "@/runner/workspace/workflowWorkspaceTypes";
import type { WorkflowDiagnostic, WorkflowJsonValue, WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";

import type { RunnerEdge, RunnerGraph, RunnerNode } from "./canvasGraphAdapter";
export type WorkflowCanvasChangeKind = "added" | "changed" | "deleted";
export type WorkflowCanvasBlockedKind = "warning" | "conflict";
export type WorkflowCanvasFieldDiff = { path: string; before: unknown; after: unknown };
export type WorkflowCanvasDeltaVisual = {
  change?: WorkflowCanvasChangeKind; blocked?: WorkflowCanvasBlockedKind;
  fieldPaths: string[]; fieldDiffs: WorkflowCanvasFieldDiff[]; messages: string[];
};
export type WorkflowCanvasProjection = {
  status: "committed" | "candidate" | "blocked";
  graph: RunnerGraph; nodes: Record<string, WorkflowCanvasDeltaVisual>; edges: Record<string, WorkflowCanvasDeltaVisual>;
  workflow: WorkflowCanvasDeltaVisual; candidateId?: string; candidateRevision?: number; error?: string;
};
export type WorkflowCanvasProjectionOptions = { committedResourceVersion?: string };
export class WorkflowCanvasProjectionError extends Error {
  constructor(message: string) { super(message); this.name = "WorkflowCanvasProjectionError"; }
}
const EMPTY_VISUAL = (): WorkflowCanvasDeltaVisual => ({ fieldPaths: [], fieldDiffs: [], messages: [] });
const FORBIDDEN_PATHS = new Set(["__proto__", "prototype", "constructor"]);
export function projectWorkflowCanvas(
  committed: RunnerGraph,
  fact?: WorkflowCandidateFact,
  options: WorkflowCanvasProjectionOptions = {},
): WorkflowCanvasProjection {
  if (!fact) return { status: "committed", graph: committed, nodes: {}, edges: {}, workflow: EMPTY_VISUAL() };
  validateCandidate(fact, options);
  validateCommitted(committed);
  const nodes = [...(committed.nodes || [])];
  const edges = [...(committed.edges || [])];
  const nodeIndex = new Map(nodes.map((node, index) => [node.id, index]));
  const edgeIndex = new Map(edges.map((edge, index) => [workflowCanvasEdgeId(edge), index]));
  const nodeVisuals: Record<string, WorkflowCanvasDeltaVisual> = {};
  const edgeVisuals: Record<string, WorkflowCanvasDeltaVisual> = {};
  let workflow = committed.workflow || {};
  let workflowVisual = EMPTY_VISUAL();
  const nodeVisual = (id: string) => (nodeVisuals[id] ||= EMPTY_VISUAL());
  const edgeVisual = (id: string) => (edgeVisuals[id] ||= EMPTY_VISUAL());
  const nodeAt = (id: string) => {
    const index = nodeIndex.get(id);
    if (index == null) fail(`unknown workflow node ${id}`);
    return { index, node: nodes[index] };
  };
  const edgeAt = (id: string) => {
    const index = edgeIndex.get(id);
    if (index == null) fail(`unknown workflow edge ${id}`);
    return { index, edge: edges[index] };
  };
  for (const operation of fact.changeSet.operations) {
    switch (operation.op) {
      case "add_node": {
        const id = requiredId(operation.node.id || operation.clientRef, "candidate node");
        if (nodeIndex.has(id)) fail(`duplicate workflow node ${id}`);
        const node = normalizeNode(operation, id);
        nodeIndex.set(id, nodes.length);
        nodes.push(node);
        nodeVisual(id).change = "added";
        break;
      }
      case "update_node": {
        const { index, node } = nodeAt(operation.nodeId);
        if (nodeVisual(operation.nodeId).change === "deleted") fail(`cannot update deleted workflow node ${operation.nodeId}`);
        const updated = applySet(node, operation.set, ["id"]);
        nodes[index] = updated.value as RunnerNode;
        mergeDiffs(nodeVisual(operation.nodeId), updated.diffs, nodeVisual(operation.nodeId).change || "changed");
        break;
      }
      case "delete_node": {
        const { node } = nodeAt(operation.nodeId);
        const visual = nodeVisual(operation.nodeId);
        if (visual.change === "added" || visual.change === "deleted") fail(`invalid delete for workflow node ${operation.nodeId}`);
        visual.change = "deleted";
        for (const edge of edges) {
          if (edge.source !== node.id && edge.target !== node.id) continue;
          const id = workflowCanvasEdgeId(edge);
          if (!operation.cascadeEdges && edgeVisual(id).change !== "deleted") fail(`workflow node ${node.id} still has edge ${id}`);
          edgeVisual(id).change = "deleted";
        }
        break;
      }
      case "add_edge": {
        const id = requiredId(operation.edge.id || operation.clientRef, "candidate edge");
        if (edgeIndex.has(id)) fail(`duplicate workflow edge ${id}`);
        const edge = normalizeEdge(operation, id);
        edgeIndex.set(id, edges.length);
        edges.push(edge);
        edgeVisual(id).change = "added";
        break;
      }
      case "update_edge": {
        const { index, edge } = edgeAt(operation.edgeId);
        if (edgeVisual(operation.edgeId).change === "deleted") fail(`cannot update deleted workflow edge ${operation.edgeId}`);
        const updated = applySet(edge, normalizeEdgeSet(operation.set), ["id"]);
        edges[index] = updated.value as RunnerEdge;
        mergeDiffs(edgeVisual(operation.edgeId), updated.diffs, edgeVisual(operation.edgeId).change || "changed");
        break;
      }
      case "delete_edge": {
        edgeAt(operation.edgeId);
        const visual = edgeVisual(operation.edgeId);
        if (visual.change === "added" || visual.change === "deleted") fail(`invalid delete for workflow edge ${operation.edgeId}`);
        visual.change = "deleted";
        break;
      }
      case "update_layout": {
        const seen = new Set<string>();
        for (const item of operation.layout.nodes) {
          if (seen.has(item.nodeId)) fail(`duplicate workflow node ${item.nodeId} in layout`);
          seen.add(item.nodeId);
          const { index, node } = nodeAt(item.nodeId);
          if (nodeVisual(item.nodeId).change === "deleted") fail(`cannot move deleted workflow node ${item.nodeId}`);
          const position = { x: finite(item.position.x, "layout x"), y: finite(item.position.y, "layout y") };
          nodes[index] = { ...node, position };
          mergeDiffs(nodeVisual(item.nodeId), [{ path: "position", before: node.position, after: position }], nodeVisual(item.nodeId).change || "changed");
        }
        break;
      }
      case "update_workflow": {
        const updated = applySet(workflow, operation.set);
        workflow = updated.value as Record<string, unknown>;
        mergeDiffs(workflowVisual, updated.diffs, "changed");
        break;
      }
      case "attach_manual_binding":
        mergeDiffs(workflowVisual, [{ path: "manualBinding", before: undefined, after: operation.manualRef.manualId }], "changed");
        break;
      default:
        assertNever(operation);
    }
  }
  validateEffectiveEdges(edges, nodeIndex, nodeVisuals, edgeVisuals);
  applyDiagnostics(fact.changeSet.validation.diagnostics, nodeVisuals, edgeVisuals, nodeIndex, edgeIndex);
  if (fact.changeSet.state === "conflict" || fact.candidate.state === "conflict") {
    markConflict(nodeVisuals);
    markConflict(edgeVisuals);
    workflowVisual.blocked = "conflict";
    workflowVisual.messages.push("候选修改与最新工作流存在冲突");
  }
  finalizeVisuals(nodeVisuals);
  finalizeVisuals(edgeVisuals);
  finalizeVisual(workflowVisual);
  return {
    status: "candidate",
    graph: { ...committed, workflow, nodes, edges },
    nodes: nodeVisuals,
    edges: edgeVisuals,
    workflow: workflowVisual,
    candidateId: fact.candidate.id,
    candidateRevision: fact.candidate.currentRevision,
  };
}
export function tryProjectWorkflowCanvas(
  committed: RunnerGraph,
  fact?: WorkflowCandidateFact,
  options: WorkflowCanvasProjectionOptions = {},
): WorkflowCanvasProjection {
  try {
    return projectWorkflowCanvas(committed, fact, options);
  } catch (error) {
    return {
      status: "blocked",
      graph: committed,
      nodes: {}, edges: {}, workflow: EMPTY_VISUAL(),
      candidateId: fact?.candidate.id,
      candidateRevision: fact?.candidate.currentRevision,
      error: error instanceof Error ? error.message : "候选画布投影失败",
    };
  }
}
export function workflowCanvasEdgeId(edge: RunnerEdge): string {
  return requiredId(edge.id || `${edge.source || ""}-${edge.target || ""}-${edge.kind || edge.source_port || "next"}`, "workflow edge");
}
function validateCandidate(fact: WorkflowCandidateFact, options: WorkflowCanvasProjectionOptions) {
  const { candidate, changeSet } = fact;
  if (candidate.id !== changeSet.candidateId || candidate.currentChangeSetId !== changeSet.id ||
      candidate.currentRevision !== changeSet.candidateRevision || candidate.currentPreviewHash !== changeSet.previewHash ||
      candidate.workflowKey.tenantId !== changeSet.workflowKey.tenantId || candidate.workflowKey.workflowId !== changeSet.workflowKey.workflowId) {
    fail("candidate and change set identity mismatch");
  }
  if (!["proposed", "previewed", "approved", "applying", "conflict"].includes(changeSet.state)) fail(`inactive candidate state ${changeSet.state}`);
  if (options.committedResourceVersion && changeSet.base.resourceVersion !== options.committedResourceVersion) {
    fail(`stale candidate basis ${changeSet.base.resourceVersion}`);
  }
}
function validateCommitted(graph: RunnerGraph) {
  const nodes = new Set<string>();
  for (const node of graph.nodes || []) {
    const id = requiredId(node.id, "workflow node");
    if (nodes.has(id)) fail(`duplicate workflow node ${id}`);
    nodes.add(id);
  }
  const edges = new Set<string>();
  for (const edge of graph.edges || []) {
    const id = workflowCanvasEdgeId(edge);
    if (edges.has(id)) fail(`duplicate workflow edge ${id}`);
    edges.add(id);
  }
}
function normalizeNode(operation: Extract<WorkflowOperation, { op: "add_node" }>, id: string): RunnerNode {
  const source = clone(operation.node) as unknown as RunnerNode & { stepName?: string; stepId?: string };
  const step = source.step && typeof source.step === "object" && !Array.isArray(source.step) ? { ...source.step } : source.step;
  return {
    ...source, id,
    position: { x: finite(source.position?.x, "node x"), y: finite(source.position?.y, "node y") },
    ...(step || source.stepName ? { step: { ...(step || {}), ...(source.stepName ? { name: source.stepName } : {}) } } : {}),
  };
}
function normalizeEdge(operation: Extract<WorkflowOperation, { op: "add_edge" }>, id: string): RunnerEdge {
  const edge = clone(operation.edge);
  return {
    ...edge, id,
    source: requiredId(edge.source, "edge source"), target: requiredId(edge.target, "edge target"),
    ...(edge.sourcePort ? { source_port: edge.sourcePort } : {}),
    ...(edge.targetPort ? { target_port: edge.targetPort } : {}),
  };
}
function normalizeEdgeSet(set: Record<string, WorkflowJsonValue>) {
  const normalized: Record<string, WorkflowJsonValue> = {};
  for (const [path, value] of Object.entries(set)) {
    normalized[path === "sourcePort" ? "source_port" : path === "targetPort" ? "target_port" : path] = value;
  }
  return normalized;
}
function applySet(source: unknown, set: Record<string, WorkflowJsonValue>, denied: string[] = []) {
  const value = clone(source) as Record<string, unknown>;
  const diffs: WorkflowCanvasFieldDiff[] = [];
  for (const [path, next] of Object.entries(set)) {
    const parts = path.split(".");
    if (!path || parts.some((part) => !part || FORBIDDEN_PATHS.has(part)) || denied.includes(parts[0])) fail(`invalid candidate field ${path}`);
    let cursor = value;
    for (const part of parts.slice(0, -1)) {
      const current = cursor[part];
      if (current == null) cursor[part] = {};
      else if (typeof current !== "object" || Array.isArray(current)) fail(`candidate field ${path} crosses a scalar`);
      cursor = cursor[part] as Record<string, unknown>;
    }
    const leaf = parts[parts.length - 1];
    const before = cursor[leaf];
    if (same(before, next)) continue;
    cursor[leaf] = clone(next);
    diffs.push({ path, before, after: next });
  }
  return { value, diffs };
}
function validateEffectiveEdges(
  edges: RunnerEdge[],
  nodeIndex: Map<string, number>,
  nodes: Record<string, WorkflowCanvasDeltaVisual>,
  edgeVisuals: Record<string, WorkflowCanvasDeltaVisual>,
) {
  for (const edge of edges) {
    const id = workflowCanvasEdgeId(edge);
    if (edgeVisuals[id]?.change === "deleted") continue;
    const source = requiredId(edge.source, `edge ${id} source`);
    const target = requiredId(edge.target, `edge ${id} target`);
    if (!nodeIndex.has(source) || !nodeIndex.has(target) || nodes[source]?.change === "deleted" || nodes[target]?.change === "deleted") {
      fail(`dangling workflow edge ${id}`);
    }
  }
}
function applyDiagnostics(
  diagnostics: WorkflowDiagnostic[],
  nodes: Record<string, WorkflowCanvasDeltaVisual>,
  edges: Record<string, WorkflowCanvasDeltaVisual>,
  nodeIndex: Map<string, number>,
  edgeIndex: Map<string, number>,
) {
  for (const diagnostic of diagnostics) {
    if ((diagnostic.nodeId && !nodeIndex.has(diagnostic.nodeId)) || (diagnostic.edgeId && !edgeIndex.has(diagnostic.edgeId))) fail(`diagnostic references an unknown canvas object`);
    const visual = diagnostic.nodeId ? (nodes[diagnostic.nodeId] ||= EMPTY_VISUAL()) : diagnostic.edgeId ? (edges[diagnostic.edgeId] ||= EMPTY_VISUAL()) : undefined;
    if (!visual) continue;
    if (["warning", "error"].includes(diagnostic.severity.toLowerCase())) visual.blocked = visual.blocked || "warning";
    visual.messages.push(diagnostic.message);
  }
}
function mergeDiffs(visual: WorkflowCanvasDeltaVisual, diffs: WorkflowCanvasFieldDiff[], change: WorkflowCanvasChangeKind) {
  if (!diffs.length) return;
  if (!visual.change) visual.change = change;
  visual.fieldDiffs.push(...diffs);
}
function markConflict(visuals: Record<string, WorkflowCanvasDeltaVisual>) {
  for (const visual of Object.values(visuals)) visual.blocked = "conflict";
}
function finalizeVisuals(visuals: Record<string, WorkflowCanvasDeltaVisual>) {
  for (const visual of Object.values(visuals)) finalizeVisual(visual);
}
function finalizeVisual(visual: WorkflowCanvasDeltaVisual) {
  visual.fieldDiffs.sort((left, right) => left.path.localeCompare(right.path));
  visual.fieldPaths = [...new Set(visual.fieldDiffs.map((diff) => diff.path))];
  visual.messages = [...new Set(visual.messages)];
}
function clone<T>(value: T): T {
  if (value === undefined) return value;
  return JSON.parse(JSON.stringify(value)) as T;
}
function same(left: unknown, right: unknown) {
  return JSON.stringify(left) === JSON.stringify(right);
}
function finite(value: unknown, label: string) {
  const result = Number(value);
  if (!Number.isFinite(result)) fail(`${label} must be finite`);
  return result;
}
function requiredId(value: unknown, label: string) {
  const result = String(value || "").trim();
  if (!result) fail(`${label} id is required`);
  return result;
}
function fail(message: string): never {
  throw new WorkflowCanvasProjectionError(message);
}
function assertNever(value: never): never {
  return fail(`unsupported candidate operation ${String((value as { op?: unknown }).op || "")}`);
}
