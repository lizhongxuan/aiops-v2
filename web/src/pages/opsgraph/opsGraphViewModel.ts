import { MarkerType, Position, type Edge, type Node } from "@xyflow/react";

import type {
  OpsGraphNode,
  OpsGraphRecord,
  OpsGraphRelationship,
  OpsGraphRelationshipType,
} from "./opsGraphTypes";

export type TopologyTone = "service" | "middleware" | "cache" | "database" | "queue" | "routing" | "external" | "legacy";

export function nodeTypeLabel(value = "") {
  const labels: Record<string, string> = {
    business: "业务",
    service: "服务",
    endpoint: "接口",
    middleware: "中间件",
    middleware_cluster: "中间件集群",
    middleware_instance: "中间件实例",
    external: "外部服务",
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
    publishes: "发布",
    consumes: "消费",
    proxies_to: "代理",
  };
  return labels[value] || value || "关联";
}

export function topologyNodeMeta(node: OpsGraphNode): {
  typeLabel: string;
  subtypeLabel?: string;
  tone: TopologyTone;
  iconLabel: string;
  summary: string;
  chips: string[];
} {
  const properties = node.properties || {};
  if (node.type === "service") {
    return {
      typeLabel: "服务",
      tone: "service",
      iconLabel: "S",
      summary: firstNonEmpty(
        deploymentSummary(properties),
        properties.ports,
        properties.owner,
        node.description,
      ),
      chips: compact([properties.environment, properties.ports, properties.owner]),
    };
  }

  if (node.type === "middleware") {
    const subtype = (node.subtype || "generic").trim() || "generic";
    const meta = middlewareSubtypeMeta(subtype);
    return {
      typeLabel: meta.label,
      subtypeLabel: subtype,
      tone: meta.tone,
      iconLabel: meta.iconLabel,
      summary: firstNonEmpty(
        properties.role,
        properties.database,
        properties.queue,
        properties.exchange,
        properties.listener,
        properties.upstream,
        properties.ports,
        properties.host,
        meta.hint,
      ),
      chips: compact([properties.environment, properties.ports, properties.host, properties.role]),
    };
  }

  if (node.type === "external") {
    return {
      typeLabel: "外部服务",
      tone: "external",
      iconLabel: "EXT",
      summary: firstNonEmpty(properties.domain, properties.provider, properties.ports, node.description),
      chips: compact([properties.provider, properties.domain, properties.ports]),
    };
  }

  return {
    typeLabel: nodeTypeLabel(node.type),
    tone: "legacy",
    iconLabel: "L",
    summary: firstNonEmpty(deploymentSummary(properties), properties.ports, properties.role, node.description),
    chips: compact([properties.environment, properties.ports, properties.host]),
  };
}

export function relationshipFacts(graph: OpsGraphRecord, nodeId: string): {
  upstreams: OpsGraphRelationship[];
  downstreams: OpsGraphRelationship[];
} {
  const relationships = graph.edges || [];
  return {
    upstreams: relationships.filter((edge) => edge.to === nodeId),
    downstreams: relationships.filter((edge) => edge.from === nodeId),
  };
}

export function buildLLMContextPreview(graph: OpsGraphRecord, nodeId: string): string {
  const node = (graph.nodes || []).find((item) => item.id === nodeId);
  if (!node) return "";

  const lines = [
    `entity: ${node.id}`,
    `name: ${node.name || node.id}`,
    `type: ${node.type}${node.subtype ? `/${node.subtype}` : ""}`,
  ];
  if (node.aliases?.length) lines.push(`aliases: ${node.aliases.join(", ")}`);
  if (node.tags?.length) lines.push(`tags: ${node.tags.join(", ")}`);

  const properties = node.properties || {};
  const propertyKeys = ["environment", "host", "k8sCluster", "namespace", "workload", "ports", "owner", "slo", "runbook", "observabilityUrl"];
  const propertyFacts = propertyKeys
    .map((key) => properties[key] ? `${key}=${properties[key]}` : "")
    .filter(Boolean);
  if (propertyFacts.length) lines.push(`properties: ${propertyFacts.join("; ")}`);

  const nodeById = new Map((graph.nodes || []).map((item) => [item.id, item]));
  const facts = relationshipFacts(graph, nodeId);
  if (facts.upstreams.length) {
    lines.push("upstreams:");
    for (const edge of facts.upstreams) {
      lines.push(`- upstream: ${edge.from} -> ${edge.type}${relationshipDetail(edge, nodeById.get(edge.from))}`);
    }
  }
  if (facts.downstreams.length) {
    lines.push("downstreams:");
    for (const edge of facts.downstreams) {
      lines.push(`- downstream: ${edge.type} -> ${edge.to}${relationshipDetail(edge, nodeById.get(edge.to))}`);
    }
  }
  return lines.join("\n");
}

export function buildCanvasModel(graph: OpsGraphRecord): { nodes: Node[]; edges: Edge[] } {
  const visibleGraph = visibleTopologyGraph(graph);
  const relationships = visibleGraph.edges || [];
  const nodes = (visibleGraph.nodes || []).map((item, index) => {
    const meta = topologyNodeMeta(item);
    return {
      id: item.id,
      type: "opsgraphNode",
      position: item.position || defaultPosition(index),
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        node: item,
        label: item.name || item.id,
        typeLabel: meta.typeLabel,
        topologyMeta: meta,
        relationshipFacts: relationshipFacts(visibleGraph, item.id),
        clusterSummary: item.type === "middleware_cluster" ? summarizeClusterDeployment(item.id, visibleGraph.nodes || [], relationships) : undefined,
      },
    };
  });

  const laneOffsets = relationshipLaneOffsets(relationships);
  const edges = relationships
    .map((edge) => ({ edge, laneOffset: laneOffsets.get(edge.id) || 0 }))
    .map((edge) => {
      const color = relationshipColor(edge.edge.type);
      return {
      id: edge.edge.id,
      source: edge.edge.from,
      target: edge.edge.to,
      type: "opsgraphEdge",
      label: relationshipLabel(edge.edge.type),
      animated: false,
      reconnectable: true,
      interactionWidth: 26,
      markerEnd: { type: MarkerType.ArrowClosed, width: 18, height: 18, color },
      style: { stroke: color, strokeWidth: 1.8 },
      data: { relationship: edge.edge, laneOffset: edge.laneOffset },
      };
    });

  return { nodes, edges };
}

export function visibleTopologyGraph(graph: OpsGraphRecord): OpsGraphRecord {
  const nodes = visibleTopologyNodes(graph.nodes || []);
  const visibleIds = new Set(nodes.map((node) => node.id));
  const edges = (graph.edges || []).filter((edge) => (
    edge.type !== "runs_on" &&
    visibleIds.has(edge.from) &&
    visibleIds.has(edge.to)
  ));
  return { ...graph, nodes, edges };
}

export function visibleTopologyNodes(nodes: OpsGraphNode[]): OpsGraphNode[] {
  return nodes.filter((node) => !isDeploymentLocationNode(node));
}

export function nextTopologyNodeName(nodes: OpsGraphNode[], baseName: string) {
  const normalizedBaseName = baseName.trim() || "新节点";
  const suffixPattern = new RegExp(`^${escapeRegExp(normalizedBaseName)}-(\\d+)$`);
  let baseNameCount = 0;
  let maxSuffix = 0;
  for (const node of nodes || []) {
    const name = (node.name || "").trim();
    if (name === normalizedBaseName) {
      baseNameCount += 1;
      maxSuffix = Math.max(maxSuffix, baseNameCount);
      continue;
    }
    const match = name.match(suffixPattern);
    if (match) {
      maxSuffix = Math.max(maxSuffix, Number(match[1]) || 0);
    }
  }
  return maxSuffix > 0 ? `${normalizedBaseName}-${maxSuffix + 1}` : normalizedBaseName;
}

function isDeploymentLocationNode(node: OpsGraphNode): boolean {
  return node.type === "host" || node.type === "k8s";
}

export function reconnectOpsGraphRelationship(
  relationship: OpsGraphRelationship,
  connection: { source?: string | null; target?: string | null },
): OpsGraphRelationship | null {
  const from = (connection.source || "").trim();
  const to = (connection.target || "").trim();
  if (!from || !to) return null;
  return {
    ...relationship,
    from,
    to,
  };
}

export function autoLayoutOpsGraphLeftToRight(graph: OpsGraphRecord): OpsGraphRecord {
  const nodes = graph.nodes || [];
  if (!nodes.length) return graph;

  const nodeById = new Map(nodes.map((node) => [node.id, node]));
  const orderById = new Map(nodes.map((node, index) => [node.id, index]));
  const outgoing = new Map<string, OpsGraphRelationship[]>();
  const indegree = new Map(nodes.map((node) => [node.id, 0]));

  for (const edge of graph.edges || []) {
    if (!nodeById.has(edge.from) || !nodeById.has(edge.to)) continue;
    outgoing.set(edge.from, [...(outgoing.get(edge.from) || []), edge]);
    indegree.set(edge.to, (indegree.get(edge.to) || 0) + 1);
  }

  const levelById = new Map(nodes.map((node) => [node.id, 0]));
  const visited = new Set<string>();
  const queue = nodes
    .filter((node) => (indegree.get(node.id) || 0) === 0)
    .sort((a, b) => compareTopologyNodes(a, b, orderById));

  while (queue.length) {
    const node = queue.shift();
    if (!node) break;
    if (visited.has(node.id)) continue;
    visited.add(node.id);
    const sourceLevel = levelById.get(node.id) || 0;
    for (const edge of outgoing.get(node.id) || []) {
      levelById.set(edge.to, Math.max(levelById.get(edge.to) || 0, sourceLevel + 1));
      indegree.set(edge.to, Math.max(0, (indegree.get(edge.to) || 0) - 1));
      if ((indegree.get(edge.to) || 0) === 0) {
        const target = nodeById.get(edge.to);
        if (target) {
          queue.push(target);
          queue.sort((a, b) => compareTopologyNodes(a, b, orderById));
        }
      }
    }
  }

  const cyclicNodes = nodes.filter((node) => !visited.has(node.id));
  if (cyclicNodes.length) {
    const fallbackLevel = Math.max(0, ...Array.from(levelById.values()));
    for (const node of cyclicNodes) {
      levelById.set(node.id, Math.max(levelById.get(node.id) || 0, fallbackLevel));
    }
  }

  const columns = new Map<number, OpsGraphNode[]>();
  for (const node of nodes) {
    const level = levelById.get(node.id) || 0;
    columns.set(level, [...(columns.get(level) || []), node]);
  }
  for (const [level, columnNodes] of columns) {
    columns.set(level, [...columnNodes].sort((a, b) => compareTopologyNodes(a, b, orderById)));
  }

  const positions = new Map<string, { x: number; y: number }>();
  const columnGap = 320;
  const rowGap = 170;
  const baseX = 96;
  const baseY = 96;
  for (const level of Array.from(columns.keys()).sort((a, b) => a - b)) {
    const columnNodes = columns.get(level) || [];
    columnNodes.forEach((node, row) => {
      positions.set(node.id, { x: baseX + level * columnGap, y: baseY + row * rowGap });
    });
  }

  return {
    ...graph,
    nodes: nodes.map((node) => ({
      ...node,
      position: positions.get(node.id) || node.position,
    })),
  };
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

function middlewareSubtypeMeta(subtype: string): { label: string; tone: TopologyTone; iconLabel: string; hint: string } {
  const known: Record<string, { label: string; tone: TopologyTone; iconLabel: string; hint: string }> = {
    generic: { label: "通用中间件", tone: "middleware", iconLabel: "MW", hint: "中间件" },
    redis: { label: "Redis", tone: "cache", iconLabel: "R", hint: "6379/redis" },
    postgres: { label: "Postgres", tone: "database", iconLabel: "PG", hint: "5432/postgres" },
    mysql: { label: "MySQL", tone: "database", iconLabel: "MY", hint: "3306/mysql" },
    zk: { label: "Zookeeper", tone: "middleware", iconLabel: "ZK", hint: "2181/zk" },
    rabbitmq: { label: "RabbitMQ", tone: "queue", iconLabel: "MQ", hint: "5672/amqp" },
    nginx: { label: "Nginx", tone: "routing", iconLabel: "NX", hint: "80/http, 443/https" },
  };
  return known[subtype] || { label: subtype, tone: "middleware", iconLabel: subtype.slice(0, 2).toUpperCase(), hint: "" };
}

function relationshipColor(value: OpsGraphRelationshipType | string) {
  switch (value) {
    case "calls":
      return "#2563eb";
    case "depends_on":
      return "#475569";
    case "publishes":
    case "consumes":
      return "#7c3aed";
    case "proxies_to":
      return "#059669";
    case "runs_on":
      return "#64748b";
    default:
      return "#64748b";
  }
}

function relationshipLaneOffsets(relationships: OpsGraphRelationship[]) {
  const incomingGroups = new Map<string, OpsGraphRelationship[]>();
  for (const relationship of relationships) {
    incomingGroups.set(relationship.to, [...(incomingGroups.get(relationship.to) || []), relationship]);
  }

  const offsets = new Map<string, number>();
  for (const group of incomingGroups.values()) {
    if (group.length <= 1) continue;
    group.forEach((relationship, index) => {
      offsets.set(relationship.id, (index - (group.length - 1) / 2) * 36);
    });
  }
  return offsets;
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function compareTopologyNodes(a: OpsGraphNode, b: OpsGraphNode, orderById: Map<string, number>) {
  const toneA = topologyNodeMeta(a).tone;
  const toneB = topologyNodeMeta(b).tone;
  const toneCompare = topologyToneRank(toneA) - topologyToneRank(toneB);
  if (toneCompare !== 0) return toneCompare;
  const nameCompare = (a.name || a.id).localeCompare(b.name || b.id, "zh-Hans-CN");
  if (nameCompare !== 0) return nameCompare;
  return (orderById.get(a.id) || 0) - (orderById.get(b.id) || 0);
}

function topologyToneRank(tone: TopologyTone) {
  const ranks: Record<TopologyTone, number> = {
    routing: 0,
    service: 1,
    external: 2,
    database: 3,
    cache: 4,
    queue: 5,
    middleware: 6,
    legacy: 7,
  };
  return ranks[tone] ?? 99;
}

function relationshipDetail(edge: OpsGraphRelationship, node?: OpsGraphNode) {
  const properties = edge.properties || {};
  const details = compact([properties.protocol, properties.port, properties.path, edge.note, edge.reason]);
  const nodeName = node?.name && node.name !== node.id ? ` (${node.name})` : "";
  return `${nodeName}${details.length ? ` [${details.join(", ")}]` : ""}`;
}

function deploymentSummary(properties: Record<string, string>) {
  const k8s = compact([properties.k8sCluster, properties.namespace]).join(" / ");
  return firstNonEmpty(k8s, properties.workload, properties.host, properties.environment);
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find((value) => Boolean(value && value.trim())) || "";
}

function compact(values: Array<string | undefined>) {
  return values.filter((value): value is string => Boolean(value && value.trim()));
}

function defaultPosition(index: number) {
  return { x: 80 + (index % 3) * 300, y: 80 + Math.floor(index / 3) * 210 };
}
