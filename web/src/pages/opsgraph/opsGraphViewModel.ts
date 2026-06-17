import type { Edge, Node } from "@xyflow/react";

import type {
  OpsGraphNode,
  OpsGraphRecord,
  OpsGraphRelationship,
  OpsGraphRelationshipType,
} from "./opsGraphTypes";

export function nodeTypeLabel(value = "") {
  const labels: Record<string, string> = {
    business: "业务",
    service: "服务",
    endpoint: "接口",
    middleware: "中间件",
    middleware_cluster: "中间件集群",
    middleware_instance: "中间件实例",
    host: "主机",
    k8s: "K8s",
    case: "Case",
    workflow: "Workflow",
  };
  return labels[value] || value || "实体";
}

export function relationshipLabel(value: OpsGraphRelationshipType | string = "") {
  const labels: Record<string, string> = {
    owns: "拥有",
    contains: "包含",
    calls: "调用",
    depends_on: "依赖",
    runs_on: "部署于",
    affects: "影响",
    owned_by: "归属",
    handled_by: "处置",
  };
  return labels[value] || value || "关联";
}

export function buildCanvasModel(graph: OpsGraphRecord): { nodes: Node[]; edges: Edge[] } {
  const relationships = graph.edges || [];
  const occupiedTopLevelRects: Rect[] = [];
  const nodes = (graph.nodes || []).map((item, index) => {
    const parentId = canvasParentId(item, relationships);
    const position = canvasPosition(item, index, parentId);
    const size = nodeSize(item);
    const canvasPositionValue = parentId ? position : avoidTopLevelOverlap(position, size, occupiedTopLevelRects);
    if (!parentId) occupiedTopLevelRects.push({ ...canvasPositionValue, ...size });
    return {
      id: item.id,
      type: isGroupNode(item) ? "opsgraphGroup" : "opsgraphNode",
      position: canvasPositionValue,
      parentId,
      extent: parentId ? "parent" as const : undefined,
      style: isGroupNode(item) ? { width: 260, height: item.type === "middleware_cluster" ? 170 : 190 } : undefined,
      data: {
        node: item,
        label: item.name || item.id,
        typeLabel: nodeTypeLabel(item.type),
        clusterSummary: item.type === "middleware_cluster"
          ? summarizeClusterDeployment(item.id, graph.nodes || [], relationships)
          : undefined,
      },
    };
  }).sort((left, right) => Number(Boolean(left.parentId)) - Number(Boolean(right.parentId)));

  const edges = relationships
    .filter((edge) => edge.type !== "runs_on" && edge.type !== "contains")
    .map((edge) => ({
      id: edge.id,
      source: edge.from,
      target: edge.to,
      type: "opsgraphEdge",
      label: relationshipLabel(edge.type),
      animated: false,
      data: { relationship: edge },
    }));

  return { nodes, edges };
}

export function summarizeClusterDeployment(clusterId: string, nodes: OpsGraphNode[], relationships: OpsGraphRelationship[]) {
  const instances = nodes.filter((node) => (
    node.parentId === clusterId ||
    relationships.some((edge) => edge.type === "contains" && edge.from === clusterId && edge.to === node.id)
  ));
  const deploymentTargets = new Set(
    instances
      .map((instance) => relationships.find((edge) => edge.type === "runs_on" && edge.from === instance.id)?.to)
      .filter((value): value is string => Boolean(value)),
  );
  const targetNodes = Array.from(deploymentTargets).map((id) => nodes.find((node) => node.id === id)).filter(Boolean);
  const k8sTargets = targetNodes.filter((node) => node?.type === "k8s");
  const hostLikeTargetCount = deploymentTargets.size - k8sTargets.length;
  const badges: string[] = [];
  if (deploymentTargets.size > 1) badges.push("跨主机");
  if (k8sTargets.length === 1 && hostLikeTargetCount === 0) badges.push("K8s 内");
  const deploymentLabel = k8sTargets.length === 1 && hostLikeTargetCount === 0
    ? `${instances.length} instances / ${k8sTargets[0]?.name || k8sTargets[0]?.id}`
    : `${instances.length} instances / ${deploymentTargets.size || 0} hosts`;

  return {
    instanceCount: instances.length,
    deploymentLabel,
    badges,
    roles: instances.map((item) => item.properties?.role || "unknown"),
  };
}

function isGroupNode(node: OpsGraphNode) {
  return node.container || node.type === "host" || node.type === "k8s" || node.type === "middleware_cluster";
}

type Rect = { x: number; y: number; width: number; height: number };

function nodeSize(node: OpsGraphNode) {
  if (isGroupNode(node)) return { width: 260, height: node.type === "middleware_cluster" ? 170 : 190 };
  return { width: 128, height: 52 };
}

function avoidTopLevelOverlap(position: { x: number; y: number }, size: { width: number; height: number }, occupiedRects: Rect[]) {
  let next = { ...position };
  for (let guard = 0; guard < 20; guard += 1) {
    const hit = occupiedRects.find((rect) => rectsOverlap({ ...next, ...size }, rect));
    if (!hit) return next;
    next = { x: hit.x + hit.width + 40, y: next.y };
  }
  return next;
}

function rectsOverlap(a: Rect, b: Rect) {
  return a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
}

function canvasParentId(node: OpsGraphNode, relationships: OpsGraphRelationship[]) {
  if (node.parentId) return node.parentId;
  const runsOn = relationships.find((edge) => edge.type === "runs_on" && edge.from === node.id);
  if (runsOn) return runsOn.to;
  return undefined;
}

function canvasPosition(node: OpsGraphNode, index: number, parentId?: string) {
  if (parentId && node.parentId !== parentId) {
    if (node.position && isRelativeContainerPosition(node.position)) return node.position;
    return defaultPosition(index, true);
  }
  return node.position || defaultPosition(index, Boolean(parentId));
}

function isRelativeContainerPosition(position: { x: number; y: number }) {
  return position.x >= 0 && position.x <= 240 && position.y >= 0 && position.y <= 170;
}

function defaultPosition(index: number, hasParent: boolean) {
  if (hasParent) {
    return { x: 24 + (index % 2) * 112, y: 72 + Math.floor(index / 2) * 72 };
  }
  return { x: 80 + (index % 3) * 300, y: 80 + Math.floor(index / 3) * 210 };
}
