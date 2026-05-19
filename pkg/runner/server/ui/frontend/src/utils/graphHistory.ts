import type { WorkflowGraph } from "../types/workflow";

export interface GraphHistoryEvent {
  id: string;
  type: "graph_edit";
  label: string;
  sessionKey: string;
  before: WorkflowGraph;
  after: WorkflowGraph;
  selectedBefore: string | null;
  selectedAfter: string | null;
}

export interface GraphHistoryDraft {
  id: string;
  label: string;
  sessionKey: string;
  before: WorkflowGraph;
  selectedBefore: string | null;
}

export interface GraphHistoryStacks {
  past: GraphHistoryEvent[];
  future: GraphHistoryEvent[];
}

const maxHistoryEvents = 50;

export function createGraphHistorySessionKey(graph: WorkflowGraph | null | undefined, baseline: WorkflowGraph | null | undefined): string {
  return [
    baseline?.workflow.name || graph?.workflow.name || "",
    baseline?.workflow.version || graph?.workflow.version || "",
    resourceVersion(baseline) || resourceVersion(graph),
  ].join("|");
}

export function beginGraphHistoryEvent(input: {
  graph: WorkflowGraph | null;
  selectedNodeId: string | null;
  sessionKey: string;
  label?: string;
}): GraphHistoryDraft | null {
  if (!input.graph) return null;
  return {
    id: `edit-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
    label: input.label || "Edit graph",
    sessionKey: input.sessionKey,
    before: sanitizeEditableGraph(input.graph),
    selectedBefore: input.selectedNodeId,
  };
}

export function commitGraphHistoryEvent(
  stacks: GraphHistoryStacks,
  draft: GraphHistoryDraft | null,
  graph: WorkflowGraph | null,
  selectedNodeId: string | null,
  sessionKey: string,
): GraphHistoryStacks {
  if (!draft || !graph || draft.sessionKey !== sessionKey) return stacks;
  const after = sanitizeEditableGraph(graph);
  if (sameGraph(draft.before, after)) return stacks;
  const event: GraphHistoryEvent = {
    id: draft.id,
    type: "graph_edit",
    label: draft.label,
    sessionKey,
    before: draft.before,
    after,
    selectedBefore: draft.selectedBefore,
    selectedAfter: selectedNodeId,
  };
  return {
    past: [...stacks.past, event].slice(-maxHistoryEvents),
    future: [],
  };
}

export function undoGraphHistory(input: {
  graph: WorkflowGraph | null;
  selectedNodeId: string | null;
  sessionKey: string;
  past: GraphHistoryEvent[];
  future: GraphHistoryEvent[];
}): { graph: WorkflowGraph; selectedNodeId: string | null; past: GraphHistoryEvent[]; future: GraphHistoryEvent[] } | null {
  if (!input.graph || input.past.length === 0) return null;
  const event = input.past[input.past.length - 1];
  if (!isCurrentUndoEvent(input.graph, input.sessionKey, event)) return null;
  return {
    graph: sanitizeEditableGraph(event.before),
    selectedNodeId: selectExistingNode(event.before, event.selectedBefore ?? input.selectedNodeId),
    past: input.past.slice(0, -1),
    future: [event, ...input.future].slice(0, maxHistoryEvents),
  };
}

export function redoGraphHistory(input: {
  graph: WorkflowGraph | null;
  selectedNodeId: string | null;
  sessionKey: string;
  past: GraphHistoryEvent[];
  future: GraphHistoryEvent[];
}): { graph: WorkflowGraph; selectedNodeId: string | null; past: GraphHistoryEvent[]; future: GraphHistoryEvent[] } | null {
  if (!input.graph || input.future.length === 0) return null;
  const event = input.future[0];
  if (!isCurrentRedoEvent(input.graph, input.sessionKey, event)) return null;
  return {
    graph: sanitizeEditableGraph(event.after),
    selectedNodeId: selectExistingNode(event.after, event.selectedAfter ?? input.selectedNodeId),
    past: [...input.past, event].slice(-maxHistoryEvents),
    future: input.future.slice(1),
  };
}

export function isCurrentUndoEvent(graph: WorkflowGraph, sessionKey: string, event: GraphHistoryEvent): boolean {
  return event.sessionKey === sessionKey && sameGraph(sanitizeEditableGraph(graph), event.after);
}

export function isCurrentRedoEvent(graph: WorkflowGraph, sessionKey: string, event: GraphHistoryEvent): boolean {
  return event.sessionKey === sessionKey && sameGraph(sanitizeEditableGraph(graph), event.before);
}

export function sanitizeEditableGraph(graph: WorkflowGraph): WorkflowGraph {
  const clone = cloneGraph(graph);
  clone.nodes = clone.nodes.map((node) => {
    const next = { ...node };
    delete next.state;
    return next;
  });
  clone.edges = clone.edges.map((edge) => {
    const next = { ...edge };
    delete next.state;
    return next;
  });
  return clone;
}

export function isEditableGraphEqual(left: WorkflowGraph | null | undefined, right: WorkflowGraph | null | undefined): boolean {
  if (!left || !right) return left === right;
  return sameGraph(sanitizeEditableGraph(left), sanitizeEditableGraph(right));
}

function selectExistingNode(graph: WorkflowGraph, preferredNodeId: string | null): string | null {
  if (preferredNodeId && graph.nodes.some((node) => node.id === preferredNodeId)) return preferredNodeId;
  return graph.nodes[0]?.id || null;
}

function resourceVersion(graph: WorkflowGraph | null | undefined): string {
  const value = graph?.ui?.resource_version;
  return typeof value === "string" ? value : "";
}

function sameGraph(left: WorkflowGraph, right: WorkflowGraph): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function cloneGraph(graph: WorkflowGraph): WorkflowGraph {
  return JSON.parse(JSON.stringify(graph)) as WorkflowGraph;
}
