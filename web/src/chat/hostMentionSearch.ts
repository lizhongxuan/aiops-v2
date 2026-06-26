import type { HostInventoryItem } from "@/api/hostInventory";

export type ActiveHostMentionToken = {
  start: number;
  end: number;
  query: string;
  raw: string;
};

export type HostMentionSuggestion = {
  key: string;
  mention: string;
  label: string;
  description: string;
  hostId?: string;
  address?: string;
  status?: string;
  score: number;
};

const TOKEN_TERMINATOR_PATTERN = /[\s,，。、:：;；()（）[\]{}<>《》"'“”‘’]/;
const EMAIL_LOCAL_PART_PATTERN = /[A-Za-z0-9._%+-]/;

export function findActiveHostMentionToken(text: string, cursor: number): ActiveHostMentionToken | null {
  const boundedCursor = Math.max(0, Math.min(cursor, text.length));
  let atIndex = -1;
  for (let index = boundedCursor - 1; index >= 0; index -= 1) {
    const char = text[index];
    if (char === "@") {
      atIndex = index;
      break;
    }
    if (TOKEN_TERMINATOR_PATTERN.test(char)) {
      return null;
    }
  }
  if (atIndex < 0) {
    return null;
  }
  const previous = atIndex > 0 ? text[atIndex - 1] : "";
  if (previous && EMAIL_LOCAL_PART_PATTERN.test(previous)) {
    return null;
  }
  const raw = text.slice(atIndex, boundedCursor);
  const query = raw.slice(1);
  return { start: atIndex, end: boundedCursor, query, raw };
}

export function searchHostMentionSuggestions(
  hosts: HostInventoryItem[],
  query: string,
  options: { limit?: number } = {},
): HostMentionSuggestion[] {
  const limit = Math.max(1, options.limit ?? DEFAULT_LIMIT);
  const normalizedQuery = normalizeQuery(query);
  const localSuggestion = buildLocalSuggestion(normalizedQuery);
  const hostSuggestions = hosts
    .map((host, index) => buildSuggestion(host, normalizedQuery, index))
    .filter((item): item is HostMentionSuggestion & { index: number } => Boolean(item))
    .sort((a, b) => b.score - a.score || statusRank(b.status) - statusRank(a.status) || a.label.localeCompare(b.label) || a.index - b.index)
    .map(({ index: _index, ...item }) => item);
  return [...(localSuggestion ? [localSuggestion] : []), ...hostSuggestions].slice(0, limit);
}

export function replaceActiveHostMention(
  text: string,
  token: ActiveHostMentionToken,
  suggestion: HostMentionSuggestion,
): { text: string; cursor: number } {
  const replacement = suggestion.mention.endsWith(" ") ? suggestion.mention : `${suggestion.mention} `;
  const nextText = `${text.slice(0, token.start)}${replacement}${text.slice(token.end)}`;
  return { text: nextText, cursor: token.start + replacement.length };
}

const DEFAULT_LIMIT = 10;

function buildLocalSuggestion(query: string): HostMentionSuggestion | null {
  const score = query ? scoreText("local", query) : 1000;
  if (query && score <= 0) {
    return null;
  }
  return {
    key: "local",
    mention: "@local",
    label: "local",
    description: "server-local · local",
    hostId: "server-local",
    address: "server-local",
    status: "online",
    score: score + 1000,
  };
}

function buildSuggestion(host: HostInventoryItem, query: string, index: number): (HostMentionSuggestion & { index: number }) | null {
  const extendedHost = host as HostInventoryItem & { address?: string; status?: string };
  const name = cleanText(host.name);
  const address = cleanText(host.ip) || cleanText(extendedHost.address);
  if (!name && !address) {
    return null;
  }
  const searchable = [name, address].filter(Boolean);
  const score = query ? Math.max(...searchable.map((value) => scoreText(value, query))) : 1;
  if (query && score <= 0) {
    return null;
  }
  const label = name || address;
  const mentionValue = address || name;
  const status = cleanText(extendedHost.status);
  return {
    key: cleanText(host.id) || cleanText(host.hostId) || `${label}-${address}-${index}`,
    mention: `@${mentionValue}`,
    label,
    description: [address, status].filter(Boolean).join(" · "),
    hostId: cleanText(host.id) || cleanText(host.hostId) || undefined,
    address: address || undefined,
    status: status || undefined,
    score: score + statusRank(status),
    index,
  };
}

function scoreText(value: string, query: string) {
  const normalized = normalizeQuery(value);
  if (!normalized || !query) return 0;
  if (normalized === query) return 100;
  if (normalized.startsWith(query)) return 80;
  if (normalized.split(/[-_.]/).some((part) => part.startsWith(query))) return 60;
  if (normalized.includes(query)) return 35;
  return fuzzyInOrder(normalized, query) ? 20 : 0;
}

function fuzzyInOrder(value: string, query: string) {
  let valueIndex = 0;
  for (const queryChar of query) {
    valueIndex = value.indexOf(queryChar, valueIndex);
    if (valueIndex < 0) return false;
    valueIndex += 1;
  }
  return true;
}

function statusRank(status = "") {
  const normalized = normalizeQuery(status);
  if (normalized === "online") return 10;
  if (normalized === "managed") return 8;
  return 0;
}

function normalizeQuery(value: string) {
  return cleanText(value).replace(/^@+/, "").toLowerCase();
}

function cleanText(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
