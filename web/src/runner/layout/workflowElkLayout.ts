import type { ElkExtendedEdge, ElkNode, ElkPort } from "elkjs/lib/elk-api.js";

import type { RunnerEdge, RunnerGraph, RunnerNode, RunnerPosition } from "@/components/runner/canvasGraphAdapter";
import type { WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";
import type { WorkflowLayoutWorkerRequest, WorkflowLayoutWorkerResponse } from "./workflowLayoutWorker";

export type WorkflowUpdateLayoutOperation = Extract<WorkflowOperation, { op: "update_layout" }>;
export type WorkflowElkRunner = (graph: ElkNode) => Promise<ElkNode>;
export type WorkflowLayoutOptions = {
  direction?: "RIGHT" | "DOWN";
  scope?: { nodeIds?: string[] };
  pinnedNodeIds?: string[];
  engine?: "elk" | "depth";
};

const START = { x: 80, y: 160 };
const NODE = { width: 204, height: 118 };
const GAP = 52;

export async function createWorkflowLayoutOperation(graph: RunnerGraph, options: WorkflowLayoutOptions = {}, runElk: WorkflowElkRunner = layoutWithWorker): Promise<WorkflowUpdateLayoutOperation> {
  const direction = options.direction === "DOWN" ? "DOWN" : "RIGHT";
  const nodeById = validateGraphNodes(graph);
  const selected = selectedNodeIds(nodeById, options.scope?.nodeIds, options.pinnedNodeIds);
  let positions: Map<string, RunnerPosition>;
  if (options.engine === "depth") {
    positions = depthLayout(graph, direction);
  } else {
    try {
      const result = await runElk(buildWorkflowElkGraph(graph, direction));
      positions = elkPositions(result);
    } catch {
      positions = depthLayout(graph, direction);
    }
  }
  const translated = anchorLayout(positions, nodeById, options.pinnedNodeIds);
  const layoutNodes = avoidFixedNodeCollisions(translated, nodeById, selected, direction)
    .map(([nodeId, position]) => ({ nodeId, position }));
  return { op: "update_layout", layout: { nodes: layoutNodes } };
}

export function buildWorkflowElkGraph(graph: RunnerGraph, direction: "RIGHT" | "DOWN"): ElkNode {
  const nodeById = validateGraphNodes(graph);
  const portKinds = collectPortKinds(graph, nodeById);
  const children = [...nodeById.values()].sort(byId).map((node) => elkNode(node, direction, portKinds.get(node.id)));
  const portIds = new Set(children.flatMap((node) => node.ports?.map((port) => port.id) || []));
  const edges = [...(graph.edges || [])]
    .filter((edge) => nodeById.has(String(edge.source || "")) && nodeById.has(String(edge.target || "")))
    .sort((left, right) => compareText(edgeIdentity(left), edgeIdentity(right)))
    .map((edge): ElkExtendedEdge => {
      const source = String(edge.source);
      const target = String(edge.target);
      const sourcePort = elkPortId(source, "output", sourcePortId(edge));
      const targetPort = elkPortId(target, "input", targetPortId(edge));
      return { id: edgeIdentity(edge), sources: [portIds.has(sourcePort) ? sourcePort : source], targets: [portIds.has(targetPort) ? targetPort : target] };
    });
  return {
    id: "workflow",
    children,
    edges,
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": direction,
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.spacing.nodeNode": "72",
      "elk.layered.spacing.nodeNodeBetweenLayers": "96",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
      "elk.layered.cycleBreaking.strategy": "MODEL_ORDER",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.randomSeed": "1",
    },
  };
}

export function projectWorkflowLayout(graph: RunnerGraph, operation: WorkflowUpdateLayoutOperation): RunnerGraph {
  const known = new Set((graph.nodes || []).map((node) => node.id));
  const positions = new Map<string, RunnerPosition>();
  for (const item of operation.layout.nodes) {
    if (!known.has(item.nodeId)) throw new TypeError(`unknown workflow node ${item.nodeId}`);
    if (positions.has(item.nodeId)) throw new TypeError(`duplicate workflow node ${item.nodeId}`);
    if (!Number.isFinite(item.position.x) || !Number.isFinite(item.position.y)) throw new TypeError(`invalid workflow node position ${item.nodeId}`);
    positions.set(item.nodeId, { ...item.position });
  }
  return {
    ...graph,
    nodes: (graph.nodes || []).map((node) => positions.has(node.id) ? { ...node, position: positions.get(node.id) } : node),
  };
}

function elkNode(node: RunnerNode, direction: "RIGHT" | "DOWN", kinds?: Map<string, "input" | "output">): ElkNode {
  const dimensions = nodeDimensions(node.type);
  const ports = [...(kinds || new Map())].sort(([leftId, leftKind], [rightId, rightKind]) => compareText(leftKind, rightKind) || compareText(leftId, rightId))
    .map(([key, kind]): ElkPort => {
      const id = key.slice(key.indexOf(":") + 1);
      return {
        id: elkPortId(node.id, kind, id),
        width: 8,
        height: 8,
        layoutOptions: { "org.eclipse.elk.port.side": direction === "RIGHT" ? (kind === "input" ? "WEST" : "EAST") : (kind === "input" ? "NORTH" : "SOUTH") },
      };
    });
  return {
    id: node.id,
    ...dimensions,
    ports,
    layoutOptions: {
      "org.eclipse.elk.portConstraints": "FIXED_ORDER",
      ...(node.type === "group" || node.type === "subflow" ? { "elk.nodeSize.constraints": "MINIMUM_SIZE" } : {}),
    },
  };
}

function collectPortKinds(graph: RunnerGraph, nodeById: Map<string, RunnerNode>) {
  const result = new Map<string, Map<string, "input" | "output">>();
  const add = (nodeId: string, id: string, kind: "input" | "output") => {
    if (!id || !nodeById.has(nodeId)) return;
    if (!result.has(nodeId)) result.set(nodeId, new Map());
    result.get(nodeId)?.set(`${kind}:${id}`, kind);
  };
  for (const node of nodeById.values()) {
    const ports = node.ports;
    if (Array.isArray(ports)) {
      for (const port of ports) add(node.id, String(port.id || ""), port.type === "input" ? "input" : "output");
    } else {
      for (const port of ports?.inputs || []) add(node.id, String(port.id || ""), "input");
      for (const port of ports?.outputs || []) add(node.id, String(port.id || ""), "output");
    }
  }
  for (const edge of graph.edges || []) {
    add(String(edge.source || ""), sourcePortId(edge), "output");
    add(String(edge.target || ""), targetPortId(edge), "input");
  }
  return result;
}

function validateGraphNodes(graph: RunnerGraph) {
  const result = new Map<string, RunnerNode>();
  for (const node of graph.nodes || []) {
    const id = String(node.id || "").trim();
    if (!id) throw new TypeError("workflow node id is required");
    if (result.has(id)) throw new TypeError(`duplicate workflow node ${id}`);
    result.set(id, node);
  }
  return result;
}

function selectedNodeIds(nodes: Map<string, RunnerNode>, scope?: string[], pinned?: string[]) {
  const requested = scope ? new Set(scope.map(String)) : new Set(nodes.keys());
  const fixed = new Set((pinned || []).map(String));
  for (const id of [...requested, ...fixed]) if (!nodes.has(id)) throw new TypeError(`unknown workflow node ${id}`);
  return new Set([...requested].filter((id) => !fixed.has(id)));
}

function elkPositions(graph: ElkNode) {
  const result = new Map<string, RunnerPosition>();
  for (const node of graph.children || []) {
    if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) throw new TypeError(`ELK omitted position for ${node.id}`);
    result.set(node.id, { x: round(START.x + Number(node.x)), y: round(START.y + Number(node.y)) });
  }
  return result;
}

function anchorLayout(positions: Map<string, RunnerPosition>, nodes: Map<string, RunnerNode>, pinned?: string[]) {
  const anchorId = [...new Set((pinned || []).map(String))].sort()[0];
  const anchor = anchorId ? nodes.get(anchorId)?.position : undefined;
  const computed = anchorId ? positions.get(anchorId) : undefined;
  if (!anchor || !computed) return positions;
  const dx = Number(anchor.x) - computed.x;
  const dy = Number(anchor.y) - computed.y;
  return new Map([...positions].map(([id, position]) => [id, { x: round(position.x + dx), y: round(position.y + dy) }]));
}

function avoidFixedNodeCollisions(positions: Map<string, RunnerPosition>, nodes: Map<string, RunnerNode>, selected: Set<string>, direction: "RIGHT" | "DOWN") {
  const obstacles = [...nodes].filter(([id]) => !selected.has(id)).map(([id, node]) => box(id, node.position || { x: 0, y: 0 }, node.type));
  const output: Array<[string, RunnerPosition]> = [];
  for (const id of [...selected].sort()) {
    const node = nodes.get(id);
    const initial = positions.get(id);
    if (!node || !initial) continue;
    const position = { ...initial };
    let candidate = box(id, position, node.type);
    while (obstacles.some((item) => overlaps(candidate, item))) {
      if (direction === "RIGHT") position.y += candidate.height + GAP;
      else position.x += candidate.width + GAP;
      candidate = box(id, position, node.type);
    }
    obstacles.push(candidate);
    output.push([id, { x: round(position.x), y: round(position.y) }]);
  }
  return output;
}

function depthLayout(graph: RunnerGraph, direction: "RIGHT" | "DOWN") {
  const nodes = [...(graph.nodes || [])].sort(byId);
  const ids = new Set(nodes.map((node) => node.id));
  const incoming = new Map(nodes.map((node) => [node.id, [] as string[]]));
  for (const edge of graph.edges || []) if (ids.has(String(edge.source)) && ids.has(String(edge.target))) incoming.get(String(edge.target))?.push(String(edge.source));
  for (const sources of incoming.values()) sources.sort();
  const memo = new Map<string, number>();
  const depth = (id: string, visiting = new Set<string>()): number => {
    if (memo.has(id)) return memo.get(id) || 0;
    if (visiting.has(id)) return 0;
    const next = new Set(visiting).add(id);
    const value = (incoming.get(id) || []).reduce((maximum, source) => Math.max(maximum, depth(source, next) + 1), 0);
    memo.set(id, value);
    return value;
  };
  const rows = new Map<number, string[]>();
  for (const node of nodes) {
    const layer = depth(node.id);
    rows.set(layer, [...(rows.get(layer) || []), node.id].sort());
  }
  const result = new Map<string, RunnerPosition>();
  for (const [layer, row] of rows) row.forEach((id, index) => result.set(id, direction === "RIGHT"
    ? { x: START.x + layer * 300, y: START.y + index * 170 }
    : { x: START.x + index * 300, y: START.y + layer * 170 }));
  return result;
}

async function layoutWithWorker(graph: ElkNode): Promise<ElkNode> {
  if (typeof Worker === "undefined") throw new Error("workflow layout worker is unavailable");
  const worker = new Worker(new URL("./workflowLayoutWorker.ts", import.meta.url), { type: "module", name: "workflow-elk-layout" });
  const requestId = `layout-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return new Promise((resolve, reject) => {
    worker.onmessage = (event: MessageEvent<WorkflowLayoutWorkerResponse>) => {
      if (event.data.requestId !== requestId) return;
      worker.terminate();
      if (event.data.graph) resolve(event.data.graph); else reject(new Error(event.data.error || "ELK layout failed"));
    };
    worker.onerror = (event) => { worker.terminate(); reject(new Error(event.message || "ELK layout worker failed")); };
    worker.postMessage({ requestId, graph } satisfies WorkflowLayoutWorkerRequest);
  });
}

function nodeDimensions(type: unknown) {
  if (type === "group") return { width: 268, height: 142 };
  if (type === "subflow") return { width: 236, height: 126 };
  if (type === "parallel" || type === "join") return { width: 220, height: 118 };
  return NODE;
}

function box(id: string, position: RunnerPosition, type: unknown) { return { id, ...position, ...nodeDimensions(type) }; }
function overlaps(left: ReturnType<typeof box>, right: ReturnType<typeof box>) { return left.x < right.x + right.width && left.x + left.width > right.x && left.y < right.y + right.height && left.y + left.height > right.y; }
function sourcePortId(edge: RunnerEdge) { return String(edge.sourcePort || edge.source_port || edge.sourceHandle || "next"); }
function targetPortId(edge: RunnerEdge) { return String(edge.targetPort || edge.target_port || edge.targetHandle || "in"); }
function elkPortId(nodeId: string, kind: "input" | "output", portId: string) { return `${nodeId}:${kind}:${portId}`; }
function edgeIdentity(edge: RunnerEdge) { return String(edge.id || `${edge.source}-${sourcePortId(edge)}-${edge.target}-${targetPortId(edge)}`); }
function byId(left: RunnerNode, right: RunnerNode) { return compareText(left.id, right.id); }
function compareText(left: string, right: string) { return left < right ? -1 : left > right ? 1 : 0; }
function round(value: number) { return Math.round(value * 100) / 100; }
