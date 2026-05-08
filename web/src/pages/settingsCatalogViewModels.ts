import type { AgentProfileRecord, McpCatalogItem, SkillCatalogItem } from "@/pages/settingsApi";

type JsonMap = Record<string, unknown>;

export type SkillCatalogDraft = Required<Pick<SkillCatalogItem, "id" | "name">> &
  Pick<SkillCatalogItem, "description" | "source" | "defaultEnabled" | "defaultActivationMode"> & {
    originalId: string;
  };

export type McpCatalogDraft = Required<Pick<McpCatalogItem, "id" | "name">> &
  Pick<McpCatalogItem, "type" | "source" | "defaultEnabled" | "permission" | "requiresExplicitUserApproval"> & {
    originalId: string;
  };

export type AgentProfileDraft = AgentProfileRecord & {
  id: string;
  name: string;
  description: string;
  systemPrompt: string;
  skills: SkillCatalogItem[];
  mcps: McpCatalogItem[];
  runtime: JsonMap;
};

export function compactText(value: unknown) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

export function generateUniqueId(prefix: string, items: Array<{ id?: string }>) {
  const existing = new Set(items.map((item) => compactText(item.id)).filter(Boolean));
  let index = 1;
  let candidate = `${prefix}-${index}`;
  while (existing.has(candidate)) {
    index += 1;
    candidate = `${prefix}-${index}`;
  }
  return candidate;
}

export function normalizeActivationMode(value: unknown, enabled = true) {
  const mode = compactText(value).toLowerCase();
  if (mode === "default" || mode === "default_enabled") return "default_enabled";
  if (mode === "explicit" || mode === "explicit_only") return "explicit_only";
  if (mode === "disabled") return "disabled";
  return enabled ? "default_enabled" : "explicit_only";
}

export function normalizeMcpPermission(value: unknown) {
  const permission = compactText(value).toLowerCase();
  if (permission === "readwrite" || permission === "read-write") return "readwrite";
  return "readonly";
}

export function createBlankSkillDraft(seed = 1): SkillCatalogDraft {
  return {
    originalId: "",
    id: `custom-skill-${seed}`,
    name: "Custom Skill",
    description: "",
    source: "local",
    defaultEnabled: false,
    defaultActivationMode: "explicit_only",
  };
}

export function normalizeSkillItem(item: Partial<SkillCatalogItem> & { originalId?: string } = {}): SkillCatalogDraft {
  const defaultEnabled = typeof item.defaultEnabled === "boolean" ? item.defaultEnabled : Boolean(item.enabled);
  return {
    originalId: compactText(item.originalId || item.id),
    id: compactText(item.id),
    name: compactText(item.name),
    description: compactText(item.description),
    source: compactText(item.source) || "local",
    defaultEnabled,
    defaultActivationMode: normalizeActivationMode(item.defaultActivationMode ?? item.activationMode, defaultEnabled),
  };
}

export function buildSkillPayload(item: SkillCatalogDraft): SkillCatalogItem {
  const normalized = normalizeSkillItem(item);
  return {
    id: normalized.id,
    name: normalized.name,
    description: normalized.description,
    source: normalized.source,
    defaultEnabled: normalized.defaultEnabled,
    defaultActivationMode: normalized.defaultActivationMode,
  };
}

export function skillSignature(item: Partial<SkillCatalogItem> | null) {
  if (!item) return "null";
  return JSON.stringify(buildSkillPayload(normalizeSkillItem(item)));
}

export function matchesSkillSearch(item: SkillCatalogDraft, query: string) {
  const keyword = compactText(query).toLowerCase();
  if (!keyword) return true;
  return [item.id, item.name, item.description, item.source, item.defaultActivationMode]
    .map((value) => compactText(value).toLowerCase())
    .some((value) => value.includes(keyword));
}

export function createBlankMcpDraft(seed = 1): McpCatalogDraft {
  return {
    originalId: "",
    id: `custom-mcp-${seed}`,
    name: "Custom MCP",
    type: "http",
    source: "local",
    defaultEnabled: false,
    permission: "readonly",
    requiresExplicitUserApproval: false,
  };
}

export function normalizeMcpItem(item: Partial<McpCatalogItem> & { originalId?: string } = {}): McpCatalogDraft {
  const defaultEnabled = typeof item.defaultEnabled === "boolean" ? item.defaultEnabled : Boolean(item.enabled);
  return {
    originalId: compactText(item.originalId || item.id),
    id: compactText(item.id),
    name: compactText(item.name),
    type: compactText(item.type) || "http",
    source: compactText(item.source) || "local",
    defaultEnabled,
    permission: normalizeMcpPermission(item.permission),
    requiresExplicitUserApproval: Boolean(item.requiresExplicitUserApproval),
  };
}

export function buildMcpPayload(item: McpCatalogDraft): McpCatalogItem {
  const normalized = normalizeMcpItem(item);
  return {
    id: normalized.id,
    name: normalized.name,
    type: normalized.type,
    source: normalized.source,
    defaultEnabled: normalized.defaultEnabled,
    permission: normalized.permission,
    requiresExplicitUserApproval: normalized.requiresExplicitUserApproval,
  };
}

export function mcpSignature(item: Partial<McpCatalogItem> | null) {
  if (!item) return "null";
  return JSON.stringify(buildMcpPayload(normalizeMcpItem(item)));
}

export function matchesMcpSearch(item: McpCatalogDraft, query: string) {
  const keyword = compactText(query).toLowerCase();
  if (!keyword) return true;
  return [item.id, item.name, item.type, item.source, item.permission]
    .map((value) => compactText(value).toLowerCase())
    .some((value) => value.includes(keyword));
}

function normalizeSystemPrompt(value: unknown) {
  if (typeof value === "string") return value;
  if (value && typeof value === "object" && "content" in value) {
    return compactText((value as { content?: unknown }).content);
  }
  return "";
}

export function normalizeAgentProfile(
  profile: Partial<AgentProfileRecord> = {},
  catalogs: { skills?: SkillCatalogItem[]; mcps?: McpCatalogItem[] } = {},
): AgentProfileDraft {
  const fallbackSkills = catalogs.skills?.filter((item) => item.defaultEnabled || item.enabled) || [];
  const fallbackMcps = catalogs.mcps?.filter((item) => item.defaultEnabled || item.enabled) || [];
  return {
    ...profile,
    id: compactText(profile.id) || "main-agent",
    name: compactText(profile.name) || "Main Agent",
    description: compactText(profile.description),
    systemPrompt: normalizeSystemPrompt(profile.systemPrompt),
    runtime: profile.runtime && typeof profile.runtime === "object" ? profile.runtime : {},
    skills: Array.isArray(profile.skills) ? profile.skills.map((item) => normalizeSkillItem(item)) : fallbackSkills.map((item) => normalizeSkillItem(item)),
    mcps: Array.isArray(profile.mcps) ? profile.mcps.map((item) => normalizeMcpItem(item)) : fallbackMcps.map((item) => normalizeMcpItem(item)),
  };
}

export function buildAgentProfilePayload(profile: AgentProfileDraft): AgentProfileRecord {
  return {
    ...profile,
    systemPrompt: {
      content: profile.systemPrompt,
    },
    skills: profile.skills.map((item) => buildSkillPayload(normalizeSkillItem(item))),
    mcps: profile.mcps.map((item) => buildMcpPayload(normalizeMcpItem(item))),
  };
}

export function agentProfileSignature(profile: AgentProfileDraft) {
  return JSON.stringify(buildAgentProfilePayload(profile));
}
