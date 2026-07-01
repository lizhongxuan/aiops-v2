import type { CapabilityMention } from "./inputMentions";

export type MentionCategory =
  | "host"
  | "monitor"
  | "ops_graph"
  | "ops_manuals";

export type MentionCategorySuggestion = {
  key: string;
  kind: "category";
  category: MentionCategory;
  label: string;
  description: string;
  prefix: string;
  score: number;
};

export type CapabilityMentionSuggestion = {
  key: string;
  mention: string;
  label: string;
  description: string;
  kind: "capability";
  category: Exclude<MentionCategory, "host">;
  path: string;
  payload: { capability: CapabilityMention };
  score: number;
};

export type OpsManualMentionItem = {
  id: string;
  title: string;
  status?: string;
  workflowRef?: { workflowId?: string };
  operation?: { targetType?: string; action?: string };
  applicability?: {
    middleware?: string;
    os?: string[];
    platform?: string[];
    executionSurface?: string[];
  };
};

export type OpsGraphMentionItem = {
  id: string;
  name: string;
  description?: string;
  environment?: string;
  isDefault?: boolean;
  nodeCount?: number;
  relationshipCount?: number;
  issueCount?: number;
};

export type ResourceMentionSuggestion = {
  key: string;
  mention: string;
  label: string;
  description: string;
  kind: "resource";
  category: Extract<MentionCategory, "ops_graph" | "ops_manuals">;
  path: string;
  payload:
    | {
        resourceKind: "ops_manual";
        manualId: string;
        workflowId?: string;
        title: string;
        status?: string;
      }
    | {
        resourceKind: "ops_graph";
        graphId: string;
        name: string;
        environment?: string;
      };
  score: number;
};

const CATEGORY_SUGGESTIONS: MentionCategorySuggestion[] = [
  {
    key: "category-host",
    kind: "category",
    category: "host",
    label: "主机",
    description: "选择具体主机",
    prefix: "@host-",
    score: 1000,
  },
  {
    key: "category-monitor",
    kind: "category",
    category: "monitor",
    label: "监控",
    description: "Coroot RCA",
    prefix: "@monitor-",
    score: 900,
  },
  {
    key: "category-ops-graph",
    kind: "category",
    category: "ops_graph",
    label: "关系图谱",
    description: "OpsGraph",
    prefix: "@opsgraph-",
    score: 800,
  },
  {
    key: "category-ops-manuals",
    kind: "category",
    category: "ops_manuals",
    label: "运维手册",
    description: "检索运维手册",
    prefix: "@manual-",
    score: 700,
  },
];

const CAPABILITY_SUGGESTIONS: CapabilityMentionSuggestion[] = [
  {
    key: "capability-coroot",
    mention: "@Coroot",
    label: "Coroot",
    description: "Coroot RCA",
    kind: "capability",
    category: "monitor",
    path: "capability://coroot",
    payload: { capability: "coroot" },
    score: 1000,
  },
  {
    key: "capability-ops-graph",
    mention: "@ops_graph",
    label: "ops_graph",
    description: "OpsGraph",
    kind: "capability",
    category: "ops_graph",
    path: "capability://ops_graph",
    payload: { capability: "ops_graph" },
    score: 900,
  },
  {
    key: "capability-ops-manuals",
    mention: "@ops_manuals",
    label: "ops_manuals",
    description: "运维手册",
    kind: "capability",
    category: "ops_manuals",
    path: "capability://ops_manuals",
    payload: { capability: "ops_manuals" },
    score: 900,
  },
];

export function searchMentionCategorySuggestions(query: string): MentionCategorySuggestion[] {
  const normalized = normalizeQuery(query);
  return CATEGORY_SUGGESTIONS
    .map((suggestion) => ({
      ...suggestion,
      score: suggestion.score + scoreCategorySuggestion(suggestion, normalized),
    }))
    .filter((suggestion) => !normalized || scoreCategorySuggestion(suggestion, normalized) > 0)
    .sort((a, b) => b.score - a.score || a.label.localeCompare(b.label));
}

export function searchCapabilityMentionSuggestions(
  query: string,
  options: { category?: Exclude<MentionCategory, "host"> } = {},
): CapabilityMentionSuggestion[] {
  const normalized = normalizeQuery(query);
  return CAPABILITY_SUGGESTIONS
    .filter((suggestion) => !options.category || suggestion.category === options.category)
    .map((suggestion) => ({
      ...suggestion,
      score: suggestion.score + scoreSuggestion(suggestion, normalized),
    }))
    .filter((suggestion) => !normalized || scoreSuggestion(suggestion, normalized) > 0)
    .sort((a, b) => b.score - a.score || a.label.localeCompare(b.label));
}

export function searchOpsManualMentionSuggestions(
  manuals: OpsManualMentionItem[],
  query: string,
  options: { limit?: number } = {},
): ResourceMentionSuggestion[] {
  return manuals
    .map((manual, index) => buildOpsManualMentionSuggestion(manual, normalizeQuery(query), index))
    .filter((item): item is ResourceMentionSuggestion & { index: number } => Boolean(item))
    .sort((a, b) => b.score - a.score || a.label.localeCompare(b.label) || a.index - b.index)
    .slice(0, Math.max(1, options.limit ?? 8))
    .map(({ index: _index, ...item }) => item);
}

export function searchOpsGraphMentionSuggestions(
  graphs: OpsGraphMentionItem[],
  query: string,
  options: { limit?: number } = {},
): ResourceMentionSuggestion[] {
  return graphs
    .map((graph, index) => buildOpsGraphMentionSuggestion(graph, normalizeQuery(query), index))
    .filter((item): item is ResourceMentionSuggestion & { index: number } => Boolean(item))
    .sort((a, b) => b.score - a.score || a.label.localeCompare(b.label) || a.index - b.index)
    .slice(0, Math.max(1, options.limit ?? 8))
    .map(({ index: _index, ...item }) => item);
}

function scoreCategorySuggestion(suggestion: MentionCategorySuggestion, query: string) {
  if (!query) return 1;
  const values = [
    suggestion.category,
    suggestion.label,
    suggestion.description,
    suggestion.prefix,
  ].map(normalizeQuery);
  if (values.some((value) => value === query)) return 100;
  if (values.some((value) => value.startsWith(query))) return 80;
  if (values.some((value) => value.includes(query))) return 40;
  return 0;
}

function buildOpsManualMentionSuggestion(
  manual: OpsManualMentionItem,
  query: string,
  index: number,
): (ResourceMentionSuggestion & { index: number }) | null {
  const manualId = cleanText(manual.id);
  const title = cleanText(manual.title) || manualId;
  if (!manualId || !title) return null;
  const workflowId = cleanText(manual.workflowRef?.workflowId);
  const target = cleanText(manual.operation?.targetType);
  const action = cleanText(manual.operation?.action);
  const middleware = cleanText(manual.applicability?.middleware);
  const status = cleanText(manual.status);
  const score = query
    ? Math.max(
        scoreText(manualId, query),
        scoreText(title, query),
        scoreText(workflowId, query),
        scoreText(target, query),
        scoreText(action, query),
        scoreText(middleware, query),
      )
    : 1;
  if (query && score <= 0) return null;
  return {
    key: `ops-manual-${manualId}`,
    mention: `@manual-${safeMentionSuffix(manualId)}`,
    label: title,
    description: [
      workflowId ? `Workflow ${workflowId}` : "",
      [target, action].filter(Boolean).join(" / "),
      status,
    ].filter(Boolean).join(" · "),
    kind: "resource",
    category: "ops_manuals",
    path: `ops-manual://${encodeURIComponent(manualId)}`,
    payload: {
      resourceKind: "ops_manual",
      manualId,
      ...(workflowId ? { workflowId } : {}),
      title,
      ...(status ? { status } : {}),
    },
    score,
    index,
  };
}

function buildOpsGraphMentionSuggestion(
  graph: OpsGraphMentionItem,
  query: string,
  index: number,
): (ResourceMentionSuggestion & { index: number }) | null {
  const graphId = cleanText(graph.id);
  const name = cleanText(graph.name) || graphId;
  if (!graphId || !name) return null;
  const environment = cleanText(graph.environment);
  const description = cleanText(graph.description);
  const score = query
    ? Math.max(
        scoreText(graphId, query),
        scoreText(name, query),
        scoreText(environment, query),
        scoreText(description, query),
      )
    : 1;
  if (query && score <= 0) return null;
  return {
    key: `ops-graph-${graphId}`,
    mention: `@opsgraph-${safeMentionSuffix(graphId)}`,
    label: name,
    description: [
      environment,
      `${Number(graph.nodeCount || 0)} 节点`,
      `${Number(graph.relationshipCount || 0)} 关系`,
      graph.isDefault ? "默认" : "",
    ].filter(Boolean).join(" · "),
    kind: "resource",
    category: "ops_graph",
    path: `ops-graph://${encodeURIComponent(graphId)}`,
    payload: {
      resourceKind: "ops_graph",
      graphId,
      name,
      ...(environment ? { environment } : {}),
    },
    score,
    index,
  };
}

function scoreSuggestion(suggestion: CapabilityMentionSuggestion, query: string) {
  if (!query) return 1;
  const values = [
    suggestion.mention,
    suggestion.label,
    suggestion.description,
  ].map(normalizeQuery);
  if (values.some((value) => value === query)) return 100;
  if (values.some((value) => value.startsWith(query))) return 80;
  if (values.some((value) => value.includes(query))) return 40;
  if (query === "manual" && suggestion.path === "capability://ops_manuals") return 60;
  return 0;
}

function scoreText(value: string, query: string) {
  const normalized = normalizeQuery(value);
  if (!normalized || !query) return 0;
  if (normalized === query) return 100;
  if (normalized.startsWith(query)) return 80;
  if (normalized.split(/[-_.]/).some((part) => part.startsWith(query))) return 60;
  if (normalized.includes(query)) return 40;
  return 0;
}

function safeMentionSuffix(value: string) {
  return cleanText(value)
    .replace(/^@+/, "")
    .replace(/[^A-Za-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "") || "item";
}

function normalizeQuery(value: string) {
  return String(value || "").trim().replace(/^@+/, "").toLowerCase();
}

function cleanText(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
