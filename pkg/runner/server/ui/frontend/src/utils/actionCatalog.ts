import type { ActionSpec, GraphPosition, JsonSchema, NodeType, WorkflowGraph, WorkflowNode } from "../types/workflow";

export interface ActionCatalogGroup {
  category: string;
  actions: ActionSpec[];
}

export function filterActionCatalog(actions: ActionSpec[], query: string): ActionCatalogGroup[] {
  const normalizedQuery = query.trim().toLowerCase();
  const filtered = normalizedQuery
    ? actions.filter((action) => searchableActionText(action).includes(normalizedQuery))
    : actions;

  const groups = new Map<string, ActionSpec[]>();
  for (const action of filtered) {
    const category = action.category || "uncategorized";
    groups.set(category, [...(groups.get(category) || []), action]);
  }

  return [...groups.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([category, items]) => ({
      category,
      actions: [...items].sort((left, right) => left.title.localeCompare(right.title) || left.action.localeCompare(right.action)),
    }));
}

export function createActionNodeFromSpec(spec: ActionSpec, graph: WorkflowGraph, position?: GraphPosition): WorkflowNode {
  const nodeType = normalizeNodeType(spec.node_type);
  const id = uniqueNodeId(graph, spec.action);
  const name = uniqueStepName(graph, spec.action);
  const defaults = defaultArgsForSpec(spec);
  const step = {
    id,
    name,
    action: spec.action,
    args: Object.keys(defaults).length > 0 ? defaults : undefined,
  };

  const node: WorkflowNode = {
    id,
    type: nodeType,
    label: spec.title || spec.action,
    position: position || nextNodePosition(graph),
    step_id: id,
    step_name: name,
    step,
    ui: {
      catalog_action: spec.action,
      catalog_category: spec.category,
      catalog_risk: spec.risk,
    },
  };

  if (nodeType === "manual_approval") {
    node.approval = {
      subjects: normalizeStringArray(defaults.subjects),
      timeout: typeof defaults.timeout === "string" ? defaults.timeout : undefined,
      on_timeout: typeof defaults.on_timeout === "string" ? defaults.on_timeout : undefined,
    };
  }

  if (nodeType === "subflow") {
    node.subflow = {
      workflow_name: firstString(defaults.workflow_name, defaults.workflow),
      vars: isRecord(defaults.vars) ? defaults.vars : undefined,
    };
  }

  return compactNode(node);
}

export function defaultArgsForSpec(spec: ActionSpec): Record<string, unknown> {
  const schemaDefaults = defaultsFromSchema(spec.args_schema);
  return compactRecord({
    ...schemaDefaults,
    ...(cloneRecord(spec.defaults) || {}),
  });
}

function searchableActionText(action: ActionSpec): string {
  return [action.title, action.action, action.category, action.description, action.risk].filter(Boolean).join(" ").toLowerCase();
}

function defaultsFromSchema(schema: JsonSchema | undefined): Record<string, unknown> {
  const properties = schema?.properties || {};
  const required = new Set(schema?.required || []);
  const out: Record<string, unknown> = {};
  for (const [key, property] of Object.entries(properties)) {
    if (property.default !== undefined) {
      out[key] = cloneValue(property.default);
    } else if (required.has(key)) {
      const fallback = fallbackForSchema(property);
      if (fallback !== undefined) out[key] = fallback;
    }
  }
  return out;
}

function fallbackForSchema(schema: JsonSchema): unknown {
  if (schema.enum?.length) return cloneValue(schema.enum[0]);
  const type = Array.isArray(schema.type) ? schema.type.find((item) => item !== "null") : schema.type;
  if (type === "string") return "";
  if (type === "boolean") return false;
  if (type === "array") return [];
  if (type === "object") return {};
  if (type === "integer" || type === "number") return 0;
  return undefined;
}

function uniqueNodeId(graph: WorkflowGraph, action: string): string {
  const base = slugify(action);
  const used = new Set(graph.nodes.map((node) => node.id));
  return uniqueName(base, used);
}

function uniqueStepName(graph: WorkflowGraph, action: string): string {
  const base = slugify(action);
  const used = new Set(graph.nodes.map((node) => node.step?.name || node.step_name || node.id));
  return uniqueName(base, used);
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
  return slug || "action";
}

function nextNodePosition(graph: WorkflowGraph): GraphPosition {
  const actionNodes = graph.nodes.filter((node) => node.type !== "start" && node.type !== "end");
  if (actionNodes.length === 0) return { x: 320, y: 160 };
  const maxX = Math.max(...actionNodes.map((node) => node.position.x));
  const minY = Math.min(...actionNodes.map((node) => node.position.y));
  return { x: maxX + 260, y: minY + (actionNodes.length % 3) * 90 };
}

function normalizeNodeType(value: NodeType | undefined): NodeType {
  return value || "action";
}

function compactNode(node: WorkflowNode): WorkflowNode {
  return compactObject(node) as unknown as WorkflowNode;
}

function compactRecord<T extends Record<string, unknown>>(input: T): T {
  return compactObject(input) as T;
}

function compactObject(input: object): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(input)) {
    if (value === undefined || value === null) continue;
    if (Array.isArray(value) && value.length === 0) continue;
    if (isRecord(value) && Object.keys(value).length === 0) continue;
    out[key] = value;
  }
  return out;
}

function cloneRecord(value: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  return value ? (cloneValue(value) as Record<string, unknown>) : undefined;
}

function cloneValue<T>(value: T): T {
  if (typeof structuredClone === "function") {
    try {
      return structuredClone(value);
    } catch {
      // UI state proxies cannot always be structured-cloned; JSON is sufficient for catalog defaults.
    }
  }
  return JSON.parse(JSON.stringify(value)) as T;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}

function firstString(...values: unknown[]): string | undefined {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
