import {
  getActionDefaultPorts,
  getConnectionValidationMessage,
  getNodeCanvasMeta,
  getNodePorts,
  getNodeTypeDefinition,
} from "./nodeTypeRegistry";

export type RunnerPosition = { x: number; y: number };

export type RunnerPort = {
  id: string;
  type?: "input" | "output" | string;
  label?: string;
};

export type RunnerStep = {
  name?: string;
  action?: string;
  targets?: unknown[];
  args?: Record<string, unknown>;
  [key: string]: unknown;
};

export type RunnerNode = {
  id: string;
  type?: string;
  label?: string;
  description?: string;
  position?: RunnerPosition;
  ports?: RunnerPort[] | { inputs?: RunnerPort[]; outputs?: RunnerPort[] };
  step?: RunnerStep;
  inputs?: Array<Record<string, unknown>>;
  outputs?: Array<Record<string, unknown>>;
  risk?: { level?: string } | string;
  [key: string]: unknown;
};

export type RunnerEdge = {
  id?: string;
  source?: string;
  target?: string;
  kind?: string;
  source_port?: string;
  target_port?: string;
  sourceHandle?: string;
  targetHandle?: string;
  state?: { status?: string };
  [key: string]: unknown;
};

export type RunnerGraph = {
  version?: string;
  workflow?: Record<string, unknown>;
  layout?: Record<string, unknown>;
  nodes?: RunnerNode[];
  edges?: RunnerEdge[];
  ui?: Record<string, unknown>;
  [key: string]: unknown;
};

type RunnerAction = {
  action?: string;
  name?: string;
  label?: string;
  title?: string;
  targets?: unknown[];
  defaults?: Record<string, unknown> & { targets?: unknown[] };
  default_ports?: unknown;
  ports?: unknown;
  [key: string]: unknown;
};
type AddCatalogActionNodeOptions = {
  preservePosition?: boolean;
};

type FlowConnection = {
  id?: string;
  source?: string | null;
  target?: string | null;
  sourceHandle?: string | null;
  targetHandle?: string | null;
  source_port?: string;
  target_port?: string;
  kind?: string;
};

type ValidationResult = { valid: true } | { valid: false; code: string; message: string };

function cloneGraph(graph: RunnerGraph = {}): RunnerGraph {
  return {
    ...graph,
    workflow: { ...(graph.workflow || {}) },
    layout: graph.layout ? { ...graph.layout } : graph.layout,
    nodes: Array.isArray(graph.nodes) ? graph.nodes.map((node) => ({ ...node, position: { ...(node.position || {}) } })) : [],
    edges: Array.isArray(graph.edges) ? graph.edges.map((edge) => ({ ...edge })) : [],
  };
}

function slugify(value: unknown): string {
  return (
    String(value || "node")
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "node"
  );
}

function uniqueId(base: string, items: Array<{ id?: string }> = []): string {
  const used = new Set(items.map((item) => item.id).filter(Boolean));
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function randomLetters(length = 4): string {
  const alphabet = "abcdefghijklmnopqrstuvwxyz";
  if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
    const values = crypto.getRandomValues(new Uint8Array(length));
    return Array.from(values, (value) => alphabet[value % alphabet.length]).join("");
  }
  let result = "";
  for (let index = 0; index < length; index += 1) result += alphabet[Math.floor(Math.random() * alphabet.length)];
  return result || "node".slice(0, length);
}

function nextActionOrder(base: string, nodes: RunnerNode[] = []): number {
  const matcher = new RegExp(`^${base}(?:-|$)`);
  const count = nodes.filter((node) => matcher.test(String(node.id || node.step?.name || ""))).length;
  return count + 1;
}

function actionLabel(action: RunnerAction = {}): string {
  return action.label || action.title || action.name || action.action || "Action";
}

function defaultActionTargets(action: RunnerAction = {}): unknown[] {
  if (Array.isArray(action.targets) && action.targets.length) return cloneValue(action.targets);
  if (Array.isArray(action.defaults?.targets) && action.defaults.targets.length) return cloneValue(action.defaults.targets);
  return ["local"];
}

function cloneValue<T>(value: T): T {
  if (value == null) return value;
  return JSON.parse(JSON.stringify(value)) as T;
}

function positionOverlaps(position: RunnerPosition, nodes: RunnerNode[] = []): boolean {
  return nodes.some((node) => {
    const existing = node.position || { x: 0, y: 0 };
    return Math.abs((Number(existing.x) || 0) - position.x) < 270 && Math.abs((Number(existing.y) || 0) - position.y) < 132;
  });
}

function nextAvailablePosition(position: Partial<RunnerPosition>, nodes: RunnerNode[] = []): RunnerPosition {
  const base = {
    x: Number(position.x) || 0,
    y: Number(position.y) || 0,
  };
  let candidate = { ...base };
  let attempt = 0;
  while (positionOverlaps(candidate, nodes)) {
    attempt += 1;
    candidate = {
      x: base.x + attempt * 300,
      y: base.y + Math.floor(attempt / 4) * 144,
    };
  }
  return candidate;
}

function edgeId(edge: RunnerEdge): string {
  return edge.id || `${edge.source}-${edge.target}-${edge.kind || edge.source_port || "next"}`;
}

export function canonicalEdgeKindForPort(port = "next"): string {
  switch (String(port || "next").trim()) {
  case "approved":
    return "approval_approved";
  case "rejected":
    return "approval_rejected";
  case "timeout":
    return "failure";
  default:
    return String(port || "next").trim() || "next";
  }
}

function displayPortForEdgeKind(kind = "next"): string {
  switch (String(kind || "next").trim()) {
  case "approval_approved":
    return "approved";
  case "approval_rejected":
    return "rejected";
  default:
    return String(kind || "next").trim() || "next";
  }
}

function normalizedPosition(position: Partial<RunnerPosition>): RunnerPosition {
  return {
    x: Number(position.x) || 0,
    y: Number(position.y) || 0,
  };
}

function buildCatalogActionNode(
  nodes: RunnerNode[] = [],
  action: RunnerAction = {},
  position: Partial<RunnerPosition> = { x: 0, y: 0 },
  options: AddCatalogActionNodeOptions = {},
): RunnerNode {
  const base = slugify(action.action || actionLabel(action));
  const id = uniqueId(base, nodes);
  const stepName = `${base}-${nextActionOrder(base, nodes)}-${randomLetters(4)}`;
  const nodePosition = options.preservePosition ? normalizedPosition(position) : nextAvailablePosition(position, nodes);
  const defaultPorts = getActionDefaultPorts(action);
  const nodeType = graphNodeTypeForAction(action);
  return {
    id,
    type: nodeType,
    position: nodePosition,
    label: actionLabel(action),
    ports: serializeGraphPorts(defaultPorts),
    step: {
      name: stepName,
      action: action.action || action.name || id,
      targets: defaultActionTargets(action),
      args: cloneValue(action.defaults || {}),
    },
  };
}

function graphNodeTypeForAction(action: RunnerAction = {}) {
  const explicit = String(action.node_type || action.nodeType || "").trim();
  if (explicit) return explicit;
  switch (getNodeTypeDefinition(action).key) {
  case "condition":
    return "condition";
  case "approval":
    return "manual_approval";
  case "subflow":
    return "subflow";
  case "variable-aggregator":
    return "variable_aggregator";
  default:
    return "action";
  }
}

function downstreamNodeIds(graph: RunnerGraph = {}, startNodeId = "") {
  const outgoingBySource = new Map<string, string[]>();
  for (const edge of graph.edges || []) {
    const source = String(edge.source || "").trim();
    const target = String(edge.target || "").trim();
    if (!source || !target) continue;
    if (!outgoingBySource.has(source)) outgoingBySource.set(source, []);
    outgoingBySource.get(source)?.push(target);
  }

  const visited = new Set<string>();
  const queue = [startNodeId].filter(Boolean);
  while (queue.length) {
    const current = queue.shift();
    if (!current || visited.has(current)) continue;
    visited.add(current);
    for (const target of outgoingBySource.get(current) || []) {
      if (!visited.has(target)) queue.push(target);
    }
  }
  return visited;
}

export function graphToCanvasModel(graph: RunnerGraph = {}, options: { selectedNodeId?: string } = {}) {
  const selectedNodeId = options.selectedNodeId || "";
  return {
    nodes: (graph.nodes || []).map((node) => ({
      id: node.id,
      type: "runner-node",
      position: { ...(node.position || { x: 0, y: 0 }) },
      selected: node.id === selectedNodeId,
      data: {
        label: node.label || node.step?.name || node.id,
        meta: getNodeCanvasMeta(node),
        ports: getNodePorts(node),
        node: { ...node, position: { ...(node.position || {}) } },
      },
    })),
    edges: (graph.edges || []).map((edge) => ({
      id: edgeId(edge),
      source: edge.source,
      target: edge.target,
      type: "runner-edge",
      data: {
        kind: edge.kind || "next",
        edge: { ...edge },
      },
    })),
  };
}

export function graphToFlowModel(graph: RunnerGraph = {}, options: { selectedNodeId?: string } = {}) {
  const selectedNodeId = options.selectedNodeId || "";
  const visibleNodes = (graph.nodes || []).filter((node) => String(node.type || "").toLowerCase() !== "end" && getNodeCanvasMeta(node).key !== "end");
  const visibleNodeIds = new Set(visibleNodes.map((node) => node.id));
  return {
    nodes: visibleNodes.map((node) => ({
      id: node.id,
      type: "runner-node",
      position: { ...(node.position || { x: 0, y: 0 }) },
      selected: node.id === selectedNodeId,
      data: {
        label: node.label || node.step?.name || node.id,
        meta: getNodeCanvasMeta(node),
        ports: getNodePorts(node),
        node: { ...node, position: { ...(node.position || {}) } },
      },
    })),
    edges: (graph.edges || []).filter((edge) => visibleNodeIds.has(String(edge.source || "")) && visibleNodeIds.has(String(edge.target || ""))).map((edge) => {
      const sourcePort = edge.source_port || edge.sourceHandle || displayPortForEdgeKind(edge.kind || "next");
      return {
      id: edgeId(edge),
      source: edge.source,
      target: edge.target,
      sourceHandle: sourcePort,
      targetHandle: edge.target_port || edge.targetHandle || "in",
      label: sourcePort,
      type: "runner-edge",
      animated: ["next", "success", "running", "selected"].includes(edge.kind || edge.state?.status || ""),
      className: ["runner-flow-edge", edge.state?.status ? `is-${edge.state.status}` : ""].filter(Boolean).join(" "),
      data: {
        kind: edge.kind || "next",
        displayKind: sourcePort,
        edge: { ...edge },
      },
    };
    }),
  };
}

export function addCatalogActionNode(
  graph: RunnerGraph = {},
  action: RunnerAction = {},
  position: Partial<RunnerPosition> = { x: 0, y: 0 },
  options: AddCatalogActionNodeOptions = {},
) {
  const next = cloneGraph(graph);
  const existingNodes = next.nodes || [];
  const node = buildCatalogActionNode(existingNodes, action, position, options);
  const startNode = existingNodes.find((item) => String(item.type || "").toLowerCase() === "start" || item.id === "start");
  const endNode = existingNodes.find((item) => String(item.type || "").toLowerCase() === "end" || item.id === "end");
  const hasExecutableNode = existingNodes.some((item) => {
    const type = String(item.type || "").toLowerCase();
    return type !== "start" && type !== "end";
  });
  const startToEndEdge = (next.edges || []).find((edge) => edge.source === startNode?.id && edge.target === endNode?.id);
  next.nodes = [...existingNodes, node];
  if (startNode && endNode && !hasExecutableNode && startToEndEdge) {
    next.edges = [
      ...(next.edges || []).filter((edge) => edge !== startToEndEdge),
      { id: `${startNode.id}-${node.id}-next`, source: startNode.id, target: node.id, kind: "next", source_port: "next", target_port: "in" },
      { id: `${node.id}-${endNode.id}-next`, source: node.id, target: endNode.id, kind: "next", source_port: "next", target_port: "in" },
    ];
  }
  return next;
}

function serializeGraphPorts(ports: { inputs?: RunnerPort[]; outputs?: RunnerPort[] } = {}) {
  return [
    ...(ports.inputs || []).map((port) => ({ id: port.id, type: "input", label: port.label || port.id })),
    ...(ports.outputs || []).map((port) => ({ id: port.id, type: "output", label: port.label || port.id })),
  ];
}

export function addGraphEdge(graph: RunnerGraph = {}, connection: FlowConnection = {}) {
  const next = cloneGraph(graph);
  const source = String(connection.source || "").trim();
  const target = String(connection.target || "").trim();
  if (!source || !target) return next;
  if (source === target) return next;
  const sourcePort = String(connection.sourceHandle || connection.source_port || connection.kind || "next").trim() || "next";
  const targetPort = String(connection.targetHandle || connection.target_port || "in").trim() || "in";
  const kind = canonicalEdgeKindForPort(connection.kind || sourcePort || "next");
  if ((next.edges || []).some((edge) => edge.source === source && edge.target === target && (edge.kind || "next") === kind)) {
    return next;
  }
  const id = uniqueId(`${source}-${target}-${kind}`, next.edges);
  const edge: RunnerEdge = { id, source, target, kind };
  if (connection.sourceHandle || connection.source_port) edge.source_port = sourcePort || displayPortForEdgeKind(kind);
  if (connection.targetHandle || connection.target_port) edge.target_port = targetPort;
  next.edges = [...(next.edges || []), edge];
  return next;
}

export function removeGraphEdge(graph: RunnerGraph = {}, edgeId = "") {
  const next = cloneGraph(graph);
  next.edges = (next.edges || []).filter((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) !== edgeId);
  return next;
}

export function removeGraphNode(graph: RunnerGraph = {}, nodeId = "") {
  const targetNodeId = String(nodeId || "").trim();
  if (!targetNodeId) return graph;
  const node = (graph.nodes || []).find((item) => item.id === targetNodeId);
  if (!node || isSystemGraphNode(node)) return graph;
  const next = cloneGraph(graph);
  next.nodes = (next.nodes || []).filter((item) => item.id !== targetNodeId);
  next.edges = (next.edges || []).filter((edge) => edge.source !== targetNodeId && edge.target !== targetNodeId);
  return next;
}

export function updateGraphEdgeKind(graph: RunnerGraph = {}, edgeId = "", kind = "next") {
  const sourcePort = String(kind || "next").trim() || "next";
  const normalizedKind = canonicalEdgeKindForPort(sourcePort);
  const next = cloneGraph(graph);
  next.edges = (next.edges || []).map((edge) => {
    const currentId = edgeIdForUpdate(edge);
    if (currentId !== edgeId) return edge;
    return {
      ...edge,
      kind: normalizedKind,
      source_port: sourcePort,
      target_port: edge.target_port || edge.targetHandle || "in",
    };
  });
  return next;
}

function edgeIdForUpdate(edge: RunnerEdge) {
  return edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`;
}

function isSystemGraphNode(node: RunnerNode = { id: "" }) {
  const type = String(node.type || "").toLowerCase();
  return node.id === "start" || node.id === "end" || type === "start" || type === "end";
}

function normalizeFlowConnection(graph: RunnerGraph = {}, connection: FlowConnection = {}): FlowConnection {
  const source = String(connection.source || "").trim();
  const target = String(connection.target || "").trim();
  const sourceHandle = String(connection.sourceHandle || connection.source_port || connection.kind || "next").trim();
  const targetHandle = String(connection.targetHandle || connection.target_port || "in").trim();
  const sourceNode = (graph.nodes || []).find((node) => node.id === source);
  const targetNode = (graph.nodes || []).find((node) => node.id === target);
  if (!sourceNode || !targetNode || !sourceHandle || !targetHandle) return connection;

  const sourcePorts = getNodePorts(sourceNode);
  const targetPorts = getNodePorts(targetNode);
  const sourceCanOutput = sourcePorts.outputs.some((port: RunnerPort) => port.id === sourceHandle);
  const targetCanInput = targetPorts.inputs.some((port: RunnerPort) => port.id === targetHandle);
  if (sourceCanOutput && targetCanInput) return connection;

  const sourceCanInput = sourcePorts.inputs.some((port: RunnerPort) => port.id === sourceHandle);
  const targetCanOutput = targetPorts.outputs.some((port: RunnerPort) => port.id === targetHandle);
  if (sourceCanInput && targetCanOutput) {
    return {
      ...connection,
      source: target,
      target: source,
      sourceHandle: targetHandle,
      targetHandle: sourceHandle,
      source_port: targetHandle,
      target_port: sourceHandle,
      kind: canonicalEdgeKindForPort(connection.kind || targetHandle || "next"),
    };
  }

  return connection;
}

export function validateGraphConnection(graph: RunnerGraph = {}, connection: FlowConnection = {}): ValidationResult {
  connection = normalizeFlowConnection(graph, connection);
  const source = String(connection.source || "").trim();
  const target = String(connection.target || "").trim();
  const sourceHandle = String(connection.sourceHandle || connection.source_port || connection.kind || "next").trim();
  const targetHandle = String(connection.targetHandle || connection.target_port || "in").trim();
  const connectionId = String(connection.id || "").trim();
  const sourceNode = (graph.nodes || []).find((node) => node.id === source);
  const targetNode = (graph.nodes || []).find((node) => node.id === target);
  const kind = canonicalEdgeKindForPort(connection.kind || sourceHandle || "next");

  if (!sourceNode) return invalidConnection("invalid_source");
  if (!targetNode) return invalidConnection("invalid_target");
  if (source === target) return invalidConnection("self_connection");

  const sourcePorts = getNodePorts(sourceNode).outputs.map((port: RunnerPort) => port.id);
  if (!sourcePorts.includes(sourceHandle)) {
    return invalidConnection("invalid_source_port");
  }

  const targetPorts = getNodePorts(targetNode).inputs.map((port: RunnerPort) => port.id);
  if (!targetPorts.includes(targetHandle)) {
    return invalidConnection("invalid_target_port");
  }

  if ((graph.edges || []).some((edge) => {
    const edgeId = edgeIdForUpdate(edge);
    if (connectionId && edgeId === connectionId) return false;
    const edgeKind = canonicalEdgeKindForPort(edge.kind || edge.source_port || "next");
    const edgeSourcePort = edge.source_port || displayPortForEdgeKind(edgeKind);
    const edgeTargetPort = edge.target_port || "in";
    return edge.source === source && edge.target === target && edgeSourcePort === sourceHandle && edgeTargetPort === targetHandle && edgeKind === kind;
  })) {
    return invalidConnection("duplicate_connection");
  }

  return { valid: true };
}

export function connectFlowEdge(graph: RunnerGraph = {}, connection: FlowConnection = {}) {
  connection = normalizeFlowConnection(graph, connection);
  const validation = validateGraphConnection(graph, connection);
  if (!validation.valid) {
    return { graph, error: validation };
  }
  return { graph: flowConnectionToGraphEdge(graph, connection), error: null };
}

export function getGraphUpstreamNodeIds(graph: RunnerGraph = {}, nodeId = "") {
  const targetNodeId = String(nodeId || "").trim();
  if (!targetNodeId) return [];

  const incomingByTarget = new Map<string, string[]>();
  for (const edge of graph.edges || []) {
    const source = String(edge.source || "").trim();
    const target = String(edge.target || "").trim();
    if (!source || !target) continue;
    if (!incomingByTarget.has(target)) incomingByTarget.set(target, []);
    incomingByTarget.get(target)?.push(source);
  }

  const visited = new Set<string>();
  const queue = [...(incomingByTarget.get(targetNodeId) || [])];
  while (queue.length > 0) {
    const current = queue.shift();
    if (!current || visited.has(current) || current === targetNodeId) continue;
    visited.add(current);
    for (const source of incomingByTarget.get(current) || []) {
      if (!visited.has(source)) queue.push(source);
    }
  }

  const graphOrder = new Map((graph.nodes || []).map((node, index) => [node.id, index]));
  return [...visited].sort((a, b) => (graphOrder.get(a) ?? Number.MAX_SAFE_INTEGER) - (graphOrder.get(b) ?? Number.MAX_SAFE_INTEGER));
}

export function flowConnectionToGraphEdge(graph: RunnerGraph = {}, connection: FlowConnection = {}) {
  const sourcePort = connection.sourceHandle || connection.source_port || connection.kind || "next";
  const kind = canonicalEdgeKindForPort(connection.kind || sourcePort || "next");
  return addGraphEdge(graph, {
    source: connection.source,
    target: connection.target,
    kind,
    sourceHandle: sourcePort,
    targetHandle: connection.targetHandle || "in",
  });
}

export function insertCatalogActionOnEdge(graph: RunnerGraph = {}, targetEdgeId = "", action: RunnerAction = {}) {
  const next = cloneGraph(graph);
  const original = (next.edges || []).find((edge) => edgeId(edge) === targetEdgeId || edgeIdForUpdate(edge) === targetEdgeId);
  if (!original?.source || !original?.target) return next;

  const shifted = downstreamNodeIds(next, original.target);
  next.nodes = (next.nodes || []).map((node) => {
    if (!shifted.has(node.id)) return node;
    const position = node.position || { x: 0, y: 0 };
    return {
      ...node,
      position: {
        x: (Number(position.x) || 0) + 320,
        y: Number(position.y) || 0,
      },
    };
  });

  const sourceNode = (next.nodes || []).find((node) => node.id === original.source);
  const targetNode = (next.nodes || []).find((node) => node.id === original.target);
  const sourcePosition = sourceNode?.position || { x: 0, y: 0 };
  const targetPosition = targetNode?.position || { x: Number(sourcePosition.x || 0) + 640, y: sourcePosition.y || 0 };
  const insertPosition = {
    x: ((Number(sourcePosition.x) || 0) + (Number(targetPosition.x) || 0)) / 2,
    y: ((Number(sourcePosition.y) || 0) + (Number(targetPosition.y) || 0)) / 2,
  };
  const inserted = buildCatalogActionNode(next.nodes || [], action, insertPosition);
  const remainingEdges = (next.edges || []).filter((edge) => edge !== original);
  const originalSourcePort = original.source_port || original.sourceHandle || displayPortForEdgeKind(original.kind || "next");
  const originalKind = canonicalEdgeKindForPort(original.kind || originalSourcePort || "next");
  const originalTargetPort = original.target_port || original.targetHandle || "in";
  const firstEdge: RunnerEdge = {
    id: uniqueId(`${original.source}-${inserted.id}-${originalKind}`, remainingEdges),
    source: original.source,
    target: inserted.id,
    kind: originalKind,
    source_port: originalSourcePort,
    target_port: "in",
  };
  const secondEdge: RunnerEdge = {
    id: uniqueId(`${inserted.id}-${original.target}-next`, [...remainingEdges, firstEdge]),
    source: inserted.id,
    target: original.target,
    kind: "next",
    source_port: "next",
    target_port: originalTargetPort,
  };
  next.nodes = [...(next.nodes || []), inserted];
  next.edges = [...remainingEdges, firstEdge, secondEdge];
  return next;
}

export function validationIssueToCanvasFocus(graph: RunnerGraph = {}, issue: Record<string, unknown> = {}) {
  const field = String(issue.field || "").trim();
  const nodeId = String(issue.node_id || issue.nodeId || "").trim();
  const edgeId = String(issue.edge_id || issue.edgeId || "").trim();
  const suggestion = String(issue.suggestion || "").trim();
  const inferred = inferIssueTargetFromField(graph, field);

  if (nodeId) {
    return {
      kind: "node",
      nodeId,
      edgeId: "",
      field: normalizeIssueField(field),
      suggestion,
    };
  }

  if (edgeId) {
    return {
      kind: "edge",
      nodeId: "",
      edgeId,
      field: normalizeIssueField(field),
      suggestion,
    };
  }

  return {
    ...inferred,
    field: normalizeIssueField(field),
    suggestion,
  };
}

function invalidConnection(code: string): ValidationResult {
  return {
    valid: false,
    code,
    message: getConnectionValidationMessage(code),
  };
}

function normalizeIssueField(field = "") {
  const raw = String(field || "").trim();
  const scoped = raw.match(/^(nodes|edges)\[\d+]\.?(.*)$/);
  if (scoped) return scoped[2] || "";
  return raw;
}

function inferIssueTargetFromField(graph: RunnerGraph = {}, field = "") {
  const raw = String(field || "").trim();
  const nodeMatch = raw.match(/^nodes\[(\d+)]/);
  if (nodeMatch) {
    const node = (graph.nodes || [])[Number(nodeMatch[1])];
    return { kind: "node", nodeId: node?.id || "", edgeId: "" };
  }
  const edgeMatch = raw.match(/^edges\[(\d+)]/);
  if (edgeMatch) {
    const edge = (graph.edges || [])[Number(edgeMatch[1])];
    return { kind: "edge", nodeId: "", edgeId: edge?.id || "" };
  }
  return { kind: "graph", nodeId: "", edgeId: "" };
}

export function updateGraphNodePosition(graph: RunnerGraph = {}, nodeId: string, position: Partial<RunnerPosition> = {}) {
  const next = cloneGraph(graph);
  next.nodes = (next.nodes || []).map((node) => {
    if (node.id !== nodeId) return node;
    return {
      ...node,
      position: { x: Number(position.x) || 0, y: Number(position.y) || 0 },
    };
  });
  return next;
}
