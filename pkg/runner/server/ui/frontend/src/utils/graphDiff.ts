import type { WorkflowEdge, WorkflowGraph, WorkflowNode } from "../types/workflow";

export type GraphDiffKind = "execution" | "layout" | "metadata";

export interface GraphDiffSection {
  kind: GraphDiffKind;
  title: string;
  changed: boolean;
  paths: string[];
}

export interface GraphDiffSummary {
  changed: boolean;
  sections: GraphDiffSection[];
}

export function buildGraphDiffSummary(base: WorkflowGraph | null | undefined, current: WorkflowGraph | null | undefined): GraphDiffSummary {
  const sections: GraphDiffSection[] = [
    diffSection("execution", "Execution semantics", executionProjection(base), executionProjection(current)),
    diffSection("layout", "Layout", layoutProjection(base), layoutProjection(current)),
    diffSection("metadata", "Metadata", metadataProjection(base), metadataProjection(current)),
  ];
  return {
    changed: sections.some((section) => section.changed),
    sections,
  };
}

function diffSection(kind: GraphDiffKind, title: string, base: unknown, current: unknown): GraphDiffSection {
  const paths = diffPaths(normalizeValue(base), normalizeValue(current));
  return {
    kind,
    title,
    changed: paths.length > 0,
    paths,
  };
}

function executionProjection(graph: WorkflowGraph | null | undefined) {
  if (!graph) return null;
  return {
    nodes: recordByID(graph.nodes, executionNodeProjection),
    edges: recordByID(graph.edges, executionEdgeProjection),
  };
}

function layoutProjection(graph: WorkflowGraph | null | undefined) {
  if (!graph) return null;
  return {
    layout: graph.layout,
    nodes: recordByID(graph.nodes, (node) => ({
      position: node.position,
      parent_id: node.parent_id,
      collapsed: node.collapsed,
      ui: node.ui,
    })),
    edges: recordByID(graph.edges, (edge) => ({
      ui: edge.ui,
    })),
    ui: graph.ui,
  };
}

function metadataProjection(graph: WorkflowGraph | null | undefined) {
  if (!graph) return null;
  const { steps: _steps, ...workflowMetadata } = graph.workflow;
  return {
    version: graph.version,
    workflow: workflowMetadata,
  };
}

function executionNodeProjection(node: WorkflowNode) {
  return {
    type: node.type,
    step_id: node.step_id,
    step_name: node.step_name,
    step: node.step,
    handler_name: node.handler_name,
    handler: node.handler,
    approval: node.approval,
    subflow: node.subflow,
    join: node.join,
    label: node.label,
  };
}

function executionEdgeProjection(edge: WorkflowEdge) {
  return {
    source: edge.source,
    source_port: edge.source_port,
    target: edge.target,
    target_port: edge.target_port,
    kind: edge.kind,
    condition: edge.condition,
  };
}

function recordByID<T extends { id: string }>(items: T[], project: (item: T) => unknown): Record<string, unknown> {
  return Object.fromEntries([...items].sort((left, right) => left.id.localeCompare(right.id)).map((item) => [item.id, project(item)]));
}

function diffPaths(base: unknown, current: unknown, path = ""): string[] {
  if (stableStringify(base) === stableStringify(current)) return [];
  if (!isRecord(base) || !isRecord(current)) return [path || "root"];

  const keys = [...new Set([...Object.keys(base), ...Object.keys(current)])].sort();
  const paths: string[] = [];
  for (const key of keys) {
    const childPath = path ? `${path}.${key}` : key;
    paths.push(...diffPaths(base[key], current[key], childPath));
  }
  return compactChangedPaths(paths);
}

function compactChangedPaths(paths: string[]): string[] {
  const out: string[] = [];
  for (const path of paths) {
    if (out.some((existing) => path.startsWith(`${existing}.`))) continue;
    out.push(path);
  }
  return out;
}

function normalizeValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeValue);
  if (!isRecord(value)) return value === undefined ? null : value;

  const out: Record<string, unknown> = {};
  for (const key of Object.keys(value).sort()) {
    const item = normalizeValue(value[key]);
    if (item === undefined) continue;
    out[key] = item;
  }
  return out;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(normalizeValue(value));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
