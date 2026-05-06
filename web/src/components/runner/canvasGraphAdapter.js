import {
  getActionDefaultPorts,
  getConnectionValidationMessage,
  getNodeCanvasMeta,
  getNodePorts,
} from "./nodeTypeRegistry";

function cloneGraph(graph = {}) {
  return {
    ...graph,
    workflow: { ...(graph.workflow || {}) },
    layout: graph.layout ? { ...graph.layout } : graph.layout,
    nodes: Array.isArray(graph.nodes) ? graph.nodes.map((node) => ({ ...node, position: { ...(node.position || {}) } })) : [],
    edges: Array.isArray(graph.edges) ? graph.edges.map((edge) => ({ ...edge })) : [],
  };
}

function slugify(value) {
  return String(value || "node")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    || "node";
}

function uniqueId(base, items = []) {
  const used = new Set(items.map((item) => item.id).filter(Boolean));
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function actionLabel(action = {}) {
  return action.label || action.title || action.name || action.action || "Action";
}

function defaultActionTargets(action = {}) {
  if (Array.isArray(action.targets) && action.targets.length) return cloneValue(action.targets);
  if (Array.isArray(action.defaults?.targets) && action.defaults.targets.length) return cloneValue(action.defaults.targets);
  return ["local"];
}

function cloneValue(value) {
  if (value == null) return value;
  return JSON.parse(JSON.stringify(value));
}

function positionOverlaps(position, nodes = []) {
  return nodes.some((node) => {
    const existing = node.position || {};
    return Math.abs((Number(existing.x) || 0) - position.x) < 270 && Math.abs((Number(existing.y) || 0) - position.y) < 132;
  });
}

function nextAvailablePosition(position, nodes = []) {
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

export function graphToCanvasModel(graph = {}, options = {}) {
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
      id: edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`,
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

export function graphToFlowModel(graph = {}, options = {}) {
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
      id: edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.source_port || edge.sourceHandle || edge.kind || "next",
      targetHandle: edge.target_port || edge.targetHandle || "in",
      label: edge.kind || "next",
      type: "runner-edge",
      animated: ["next", "success", "running", "selected"].includes(edge.kind || edge.state?.status || ""),
      class: ["runner-flow-edge", edge.state?.status ? `is-${edge.state.status}` : ""].filter(Boolean).join(" "),
      data: {
        kind: edge.kind || "next",
        edge: { ...edge },
      },
    })),
  };
}

export function addCatalogActionNode(graph = {}, action = {}, position = { x: 0, y: 0 }) {
  const next = cloneGraph(graph);
  const base = slugify(action.action || actionLabel(action));
  const id = uniqueId(base, next.nodes);
  const nodePosition = nextAvailablePosition(position, next.nodes);
  const defaultPorts = getActionDefaultPorts(action);
  next.nodes.push({
    id,
    type: "action",
    position: nodePosition,
    label: actionLabel(action),
    ports: serializeGraphPorts(defaultPorts),
    step: {
      name: id,
      action: action.action || action.name || id,
      targets: defaultActionTargets(action),
      args: cloneValue(action.defaults || {}),
    },
  });
  return next;
}

function serializeGraphPorts(ports = {}) {
  return [
    ...(ports.inputs || []).map((port) => ({ id: port.id, type: "input", label: port.label || port.id })),
    ...(ports.outputs || []).map((port) => ({ id: port.id, type: "output", label: port.label || port.id })),
  ];
}

export function addGraphEdge(graph = {}, connection = {}) {
  const next = cloneGraph(graph);
  const source = String(connection.source || "").trim();
  const target = String(connection.target || "").trim();
  if (!source || !target) return next;
  if (source === target) return next;
  const kind = connection.kind || "next";
  if (next.edges.some((edge) => edge.source === source && edge.target === target && (edge.kind || "next") === kind)) {
    return next;
  }
  const id = uniqueId(`${source}-${target}-${kind}`, next.edges);
  const edge = { id, source, target, kind };
  if (connection.sourceHandle || connection.source_port) edge.source_port = connection.sourceHandle || connection.source_port;
  if (connection.targetHandle || connection.target_port) edge.target_port = connection.targetHandle || connection.target_port;
  next.edges.push(edge);
  return next;
}

export function removeGraphEdge(graph = {}, edgeId = "") {
  const next = cloneGraph(graph);
  next.edges = next.edges.filter((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) !== edgeId);
  return next;
}

export function updateGraphEdgeKind(graph = {}, edgeId = "", kind = "next") {
  const normalizedKind = String(kind || "next").trim() || "next";
  const next = cloneGraph(graph);
  next.edges = next.edges.map((edge) => {
    const currentId = edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`;
    if (currentId !== edgeId) return edge;
    return {
      ...edge,
      kind: normalizedKind,
      source_port: normalizedKind,
      target_port: edge.target_port || edge.targetHandle || "in",
    };
  });
  return next;
}

export function validateGraphConnection(graph = {}, connection = {}) {
  const source = String(connection.source || "").trim();
  const target = String(connection.target || "").trim();
  const sourceHandle = String(connection.sourceHandle || connection.source_port || connection.kind || "next").trim();
  const targetHandle = String(connection.targetHandle || connection.target_port || "in").trim();
  const connectionId = String(connection.id || "").trim();
  const sourceNode = (graph.nodes || []).find((node) => node.id === source);
  const targetNode = (graph.nodes || []).find((node) => node.id === target);
  const kind = connection.kind || sourceHandle || "next";

  if (!sourceNode) return invalidConnection("invalid_source");
  if (!targetNode) return invalidConnection("invalid_target");
  if (source === target) return invalidConnection("self_connection");

  const sourcePorts = getNodePorts(sourceNode).outputs.map((port) => port.id);
  if (!sourcePorts.includes(sourceHandle)) {
    return invalidConnection("invalid_source_port");
  }

  const targetPorts = getNodePorts(targetNode).inputs.map((port) => port.id);
  if (!targetPorts.includes(targetHandle)) {
    return invalidConnection("invalid_target_port");
  }

  if ((graph.edges || []).some((edge) => {
    const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`;
    if (connectionId && edgeId === connectionId) return false;
    const edgeKind = edge.kind || edge.source_port || "next";
    const edgeSourcePort = edge.source_port || edgeKind;
    const edgeTargetPort = edge.target_port || "in";
    return edge.source === source && edge.target === target && edgeSourcePort === sourceHandle && edgeTargetPort === targetHandle && edgeKind === kind;
  })) {
    return invalidConnection("duplicate_connection");
  }

  return { valid: true };
}

export function connectFlowEdge(graph = {}, connection = {}) {
  const validation = validateGraphConnection(graph, connection);
  if (!validation.valid) {
    return { graph, error: validation };
  }
  return { graph: flowConnectionToGraphEdge(graph, connection), error: null };
}

export function getGraphUpstreamNodeIds(graph = {}, nodeId = "") {
  const targetNodeId = String(nodeId || "").trim();
  if (!targetNodeId) return [];

  const incomingByTarget = new Map();
  for (const edge of graph.edges || []) {
    const source = String(edge.source || "").trim();
    const target = String(edge.target || "").trim();
    if (!source || !target) continue;
    if (!incomingByTarget.has(target)) incomingByTarget.set(target, []);
    incomingByTarget.get(target).push(source);
  }

  const visited = new Set();
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

export function flowConnectionToGraphEdge(graph = {}, connection = {}) {
  const kind = connection.kind || connection.sourceHandle || "next";
  return addGraphEdge(graph, {
    source: connection.source,
    target: connection.target,
    kind,
    sourceHandle: connection.sourceHandle || kind,
    targetHandle: connection.targetHandle || "in",
  });
}

export function validationIssueToCanvasFocus(graph = {}, issue = {}) {
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

function invalidConnection(code) {
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

function inferIssueTargetFromField(graph = {}, field = "") {
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

export function updateGraphNodePosition(graph = {}, nodeId, position = {}) {
  const next = cloneGraph(graph);
  next.nodes = next.nodes.map((node) => {
    if (node.id !== nodeId) return node;
    return {
      ...node,
      position: { x: Number(position.x) || 0, y: Number(position.y) || 0 },
    };
  });
  return next;
}
