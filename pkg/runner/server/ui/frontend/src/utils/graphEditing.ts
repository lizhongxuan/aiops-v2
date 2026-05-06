import type { EdgeKind, GraphPosition, NodeType, WorkflowEdge, WorkflowGraph, WorkflowNode } from "../types/workflow";

export type ControlNodeType = Extract<NodeType, "start" | "end" | "parallel" | "join" | "handler" | "group">;

export interface ControlNodeSpec {
  type: ControlNodeType;
  title: string;
  description: string;
}

export const controlNodeSpecs: ControlNodeSpec[] = [
  { type: "start", title: "Start", description: "Entry point for graph execution." },
  { type: "end", title: "End", description: "Terminal graph node." },
  { type: "parallel", title: "Parallel", description: "Fork execution into multiple branches." },
  { type: "join", title: "Join", description: "Wait for branch completion." },
  { type: "handler", title: "Notify Handler", description: "Reusable notification or remediation handler." },
  { type: "group", title: "Group", description: "Visually group related nodes." },
];

export function filterControlNodes(query: string): ControlNodeSpec[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return controlNodeSpecs;
  return controlNodeSpecs.filter((spec) => [spec.title, spec.type, spec.description].join(" ").toLowerCase().includes(normalized));
}

export function createControlNode(type: ControlNodeType, graph: WorkflowGraph, position?: GraphPosition): WorkflowNode {
  const spec = controlNodeSpecs.find((item) => item.type === type);
  const id = uniqueNodeId(graph, type);
  const node: WorkflowNode = {
    id,
    type,
    label: spec?.title || type,
    position: position || nextNodePosition(graph),
  };
  if (type === "join") {
    node.join = { strategy: "all_success" };
  }
  if (type === "handler") {
    node.handler_name = id;
    node.handler = {
      name: id,
      action: "cmd.run",
      args: { cmd: "echo notify" },
    };
  }
  if (type === "group") {
    node.collapsed = false;
  }
  return node;
}

export function addNodeToGraph(graph: WorkflowGraph, node: WorkflowNode): WorkflowGraph {
  return {
    ...graph,
    nodes: [...graph.nodes, node],
  };
}

export function connectGraphNodes(
  graph: WorkflowGraph,
  source: string | null | undefined,
  target: string | null | undefined,
  kind: EdgeKind = "next",
): { graph: WorkflowGraph; error?: string } {
  source = source?.trim();
  target = target?.trim();
  if (!source || !target) return { graph, error: "Connection source and target are required." };
  if (source === target) return { graph, error: "A node cannot connect to itself." };

  const sourceNode = graph.nodes.find((node) => node.id === source);
  const targetNode = graph.nodes.find((node) => node.id === target);
  if (!sourceNode || !targetNode) return { graph, error: "Connection source or target node does not exist." };
  if (sourceNode.type === "end") return { graph, error: "End nodes cannot have outgoing edges." };
  if (targetNode.type === "start") return { graph, error: "Start nodes cannot have incoming edges." };
  if (sourceNode.type === "group" || targetNode.type === "group") return { graph, error: "Group nodes are layout containers and cannot be connected." };
  if (graph.edges.some((edge) => edge.source === source && edge.target === target)) {
    return { graph, error: "This connection already exists." };
  }

  const edge: WorkflowEdge = {
    id: uniqueEdgeId(graph, `edge-${source}-${target}`),
    source,
    target,
    kind,
  };
  return {
    graph: {
      ...graph,
      edges: [...graph.edges, edge],
    },
  };
}

export function deleteNodeFromGraph(graph: WorkflowGraph, nodeId: string | null | undefined): { graph: WorkflowGraph; error?: string } {
  nodeId = nodeId?.trim();
  if (!nodeId) return { graph, error: "Select a node to delete." };
  const node = graph.nodes.find((item) => item.id === nodeId);
  if (!node) return { graph, error: "Selected node does not exist." };
  if ((node.type === "start" || node.type === "end") && graph.nodes.filter((item) => item.type === node.type).length === 1) {
    return { graph, error: `The last ${node.type} node cannot be deleted.` };
  }
  return {
    graph: {
      ...graph,
      nodes: graph.nodes.filter((item) => item.id !== nodeId),
      edges: graph.edges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
    },
  };
}

export function autoLayoutGraph(graph: WorkflowGraph): WorkflowGraph {
  const order = new Map(graph.nodes.map((node, index) => [node.id, index]));
  const nodesByID = new Map(graph.nodes.map((node) => [node.id, node]));
  const outgoing = new Map<string, string[]>();
  for (const edge of graph.edges) {
    if (!nodesByID.has(edge.source) || !nodesByID.has(edge.target)) continue;
    outgoing.set(edge.source, [...(outgoing.get(edge.source) || []), edge.target]);
  }
  for (const targets of outgoing.values()) {
    targets.sort((left, right) => (order.get(left) || 0) - (order.get(right) || 0));
  }

  const layer = new Map<string, number>();
  const queue = graph.nodes.filter((node) => node.type === "start").map((node) => node.id);
  for (const id of queue) layer.set(id, 0);
  for (let index = 0; index < queue.length; index += 1) {
    const id = queue[index];
    const currentLayer = layer.get(id) || 0;
    for (const target of outgoing.get(id) || []) {
      const nextLayer = Math.max(layer.get(target) ?? 0, currentLayer + 1);
      if (nextLayer !== layer.get(target)) {
        layer.set(target, nextLayer);
        queue.push(target);
      }
    }
  }

  let fallbackLayer = Math.max(0, ...layer.values());
  for (const node of graph.nodes) {
    if (layer.has(node.id)) continue;
    fallbackLayer += node.type === "group" ? 0 : 1;
    layer.set(node.id, fallbackLayer);
  }

  const byLayer = new Map<number, WorkflowNode[]>();
  for (const node of graph.nodes) {
    const value = layer.get(node.id) || 0;
    byLayer.set(value, [...(byLayer.get(value) || []), node]);
  }

  const positioned = graph.nodes.map((node) => {
    const value = layer.get(node.id) || 0;
    const peers = byLayer.get(value) || [];
    const index = peers.findIndex((item) => item.id === node.id);
    return {
      ...node,
      position: {
        x: 80 + value * 260,
        y: 120 + Math.max(index, 0) * 128,
      },
    };
  });

  return {
    ...graph,
    layout: {
      ...(graph.layout || {}),
      direction: "LR",
    },
    nodes: positioned,
  };
}

function uniqueNodeId(graph: WorkflowGraph, base: string): string {
  return uniqueName(slugify(base), new Set(graph.nodes.map((node) => node.id)));
}

function uniqueEdgeId(graph: WorkflowGraph, base: string): string {
  return uniqueName(slugify(base), new Set(graph.edges.map((edge) => edge.id)));
}

function uniqueName(base: string, used: Set<string>): string {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function slugify(value: string): string {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "node";
}

function nextNodePosition(graph: WorkflowGraph): GraphPosition {
  const visibleNodes = graph.nodes.filter((node) => node.type !== "group");
  if (visibleNodes.length === 0) return { x: 80, y: 120 };
  const maxX = Math.max(...visibleNodes.map((node) => node.position.x));
  const minY = Math.min(...visibleNodes.map((node) => node.position.y));
  return { x: maxX + 220, y: minY + (visibleNodes.length % 4) * 90 };
}
