import { defineStore } from "pinia";
import { cloneUiFixturePayload } from "./lib/uiFixturePresets";
import { resolveUiFixtureRuntime } from "./lib/uiFixtureRuntime";
import { createAppSocketClient } from "./realtime/appSocket";
import { fetchState as fetchStateApi, resetThread as resetThreadApi, selectHost as selectHostApi } from "./api/state";
import { activateSession as activateSessionApi, createSession as createSessionApi, fetchSessions as fetchSessionsApi } from "./api/sessions";
import { fetchSettings as fetchSettingsApi, updateSettings as updateSettingsApi } from "./api/settings";
import {
  deleteMcpCatalogItem as deleteMcpCatalogItemApi,
  deleteSkillCatalogItem as deleteSkillCatalogItemApi,
  exportAgentProfiles as exportAgentProfilesApi,
  fetchAgentProfilePreview as fetchAgentProfilePreviewApi,
  fetchAgentProfiles as fetchAgentProfilesApi,
  fetchMcpCatalog as fetchMcpCatalogApi,
  fetchSingleAgentProfile as fetchSingleAgentProfileApi,
  fetchSkillCatalog as fetchSkillCatalogApi,
  importAgentProfiles as importAgentProfilesApi,
  resetAgentProfile as resetAgentProfileApi,
  saveAgentProfile as saveAgentProfileApi,
  saveMcpCatalogItem as saveMcpCatalogItemApi,
  saveSkillCatalogItem as saveSkillCatalogItemApi,
} from "./api/agentProfile";

const MCP_DRAWER_STORAGE_KEY = "codex:mcp-drawer:v1";
const MCP_DRAWER_PIN_LIMIT = 8;
const MCP_DRAWER_RECENT_LIMIT = 6;

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function cloneMcpPayload(value) {
  if (Array.isArray(value)) {
    return value.map((item) => cloneMcpPayload(item));
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [key, cloneMcpPayload(item)]),
  );
}

function coerceIsoStamp(value) {
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? new Date(stamp).toISOString() : new Date().toISOString();
}

function inferMcpDrawerSurfaceKind(surface = {}) {
  const source = asObject(surface);
  if (compactText(source.kind).toLowerCase() === "bundle") return "bundle";
  if (compactText(source.kind).toLowerCase() === "mcp_bundle") return "bundle";
  if (source.bundle || source.model?.bundleKind || source.bundleKind || Array.isArray(source.sections)) return "bundle";
  return "card";
}

function resolveMcpDrawerSurfaceModel(surface = {}) {
  const source = asObject(surface);
  const kind = inferMcpDrawerSurfaceKind(source);
  if (kind === "bundle") {
    return cloneMcpPayload(source.bundle || source.model || source);
  }
  return cloneMcpPayload(source.card || source.model || source);
}

function resolveMcpDrawerSurfaceTitle(kind, model = {}, surface = {}) {
  const source = asObject(model);
  const subject = asObject(source.subject);
  const fallback = asObject(surface);
  if (kind === "bundle") {
    return (
      compactText(source.summary || source.title || subject.name || fallback.title || "MCP 聚合面板") ||
      "MCP 聚合面板"
    );
  }
  return compactText(source.title || source.summary || subject.name || fallback.title || "MCP 卡片") || "MCP 卡片";
}

function resolveMcpDrawerSurfaceSubtitle(kind, model = {}, surface = {}) {
  const source = asObject(model);
  const scope = asObject(source.scope);
  const subject = asObject(source.subject);
  const fallback = asObject(surface);
  if (compactText(fallback.subtitle)) {
    return compactText(fallback.subtitle);
  }
  if (kind === "bundle") {
    return compactText(subject.type || scope.service || scope.resourceType || "");
  }
  return compactText(scope.resourceType || scope.service || subject.type || "");
}

function buildMcpDrawerSurfaceIdentity(kind, source = {}, model = {}) {
  const record = asObject(source);
  const payload = asObject(model);
  const candidate =
    compactText(record.id || record.surfaceId || record.bundleId || payload.id || payload.bundleId || payload.cardId) ||
    compactText(payload.title || payload.summary || payload.subject?.name || "");
  if (candidate) {
    return candidate.replace(/\s+/g, "-");
  }
  return `${kind}-mcp-surface`;
}

function normalizeMcpDrawerSurfacePayload(payload = {}) {
  const wrapper = asObject(payload);
  const source = asObject(wrapper.surface || wrapper);
  const kind = inferMcpDrawerSurfaceKind(source);
  const model = resolveMcpDrawerSurfaceModel(source);
  if (!Object.keys(asObject(model)).length) {
    return null;
  }
  const sourceTag = compactText(wrapper.source || source.source || model.source || "");
  const surfaceId = buildMcpDrawerSurfaceIdentity(kind, source, model);
  const touchedAt = coerceIsoStamp(wrapper.touchedAt || source.touchedAt || source.openedAt || model.touchedAt || model.openedAt);
  const pinnedAt = compactText(wrapper.pinnedAt || source.pinnedAt || model.pinnedAt)
    ? coerceIsoStamp(wrapper.pinnedAt || source.pinnedAt || model.pinnedAt)
    : "";
  return {
    id: compactText(wrapper.id || source.id || `${kind}:${surfaceId}`) || `${kind}:${surfaceId}`,
    surfaceId,
    kind,
    source: sourceTag,
    title: resolveMcpDrawerSurfaceTitle(kind, model, source),
    subtitle: resolveMcpDrawerSurfaceSubtitle(kind, model, source),
    model,
    touchedAt,
    openedAt: touchedAt,
    pinnedAt,
    lastReason: compactText(wrapper.lastReason || source.lastReason || ""),
  };
}

function upsertMcpDrawerSurfaceList(list, surface, { limit = MCP_DRAWER_RECENT_LIMIT, reason = "", pin = false } = {}) {
  const normalizedSurface = normalizeMcpDrawerSurfacePayload(surface);
  if (!normalizedSurface?.id) {
    return Array.isArray(list) ? list.slice(0, limit) : [];
  }
  const previous = (list || []).find((item) => item.id === normalizedSurface.id) || null;
  const touchedAt = coerceIsoStamp(normalizedSurface.touchedAt || previous?.touchedAt);
  const nextSurface = {
    ...(previous || {}),
    ...normalizedSurface,
    touchedAt,
    openedAt: touchedAt,
    lastReason: compactText(reason || normalizedSurface.lastReason || previous?.lastReason || ""),
    pinnedAt: pin
      ? coerceIsoStamp(normalizedSurface.pinnedAt || previous?.pinnedAt || touchedAt)
      : compactText(normalizedSurface.pinnedAt || previous?.pinnedAt || ""),
  };
  return [nextSurface, ...(list || []).filter((item) => item.id !== nextSurface.id)].slice(0, limit);
}

function getMcpDrawerStorage() {
  try {
    if (typeof window !== "undefined" && window.localStorage) {
      return window.localStorage;
    }
  } catch {
    return null;
  }
  return null;
}

function readPersistedMcpDrawerState() {
  const storage = getMcpDrawerStorage();
  if (!storage) {
    return {
      activeSurface: null,
      pinnedSurfaces: [],
      recentSurfaces: [],
    };
  }
  try {
    const raw = storage.getItem(MCP_DRAWER_STORAGE_KEY);
    if (!raw) {
      return {
        activeSurface: null,
        pinnedSurfaces: [],
        recentSurfaces: [],
      };
    }
    const parsed = asObject(JSON.parse(raw));
    return {
      activeSurface: normalizeMcpDrawerSurfacePayload(parsed.activeSurface || null),
      pinnedSurfaces: (Array.isArray(parsed.pinnedSurfaces) ? parsed.pinnedSurfaces : [])
        .map((item) => normalizeMcpDrawerSurfacePayload(item))
        .filter(Boolean)
        .slice(0, MCP_DRAWER_PIN_LIMIT),
      recentSurfaces: (Array.isArray(parsed.recentSurfaces) ? parsed.recentSurfaces : [])
        .map((item) => normalizeMcpDrawerSurfacePayload(item))
        .filter(Boolean)
        .slice(0, MCP_DRAWER_RECENT_LIMIT),
    };
  } catch (error) {
    console.error("Failed to read persisted MCP drawer state:", error);
    return {
      activeSurface: null,
      pinnedSurfaces: [],
      recentSurfaces: [],
    };
  }
}

function persistMcpDrawerState(state = {}) {
  const storage = getMcpDrawerStorage();
  if (!storage) return false;
  try {
    storage.setItem(
      MCP_DRAWER_STORAGE_KEY,
      JSON.stringify({
        activeSurface: normalizeMcpDrawerSurfacePayload(state.activeSurface),
        pinnedSurfaces: (state.pinnedSurfaces || []).map((item) => normalizeMcpDrawerSurfacePayload(item)).filter(Boolean),
        recentSurfaces: (state.recentSurfaces || []).map((item) => normalizeMcpDrawerSurfacePayload(item)).filter(Boolean),
      }),
    );
    return true;
  } catch (error) {
    console.error("Failed to persist MCP drawer state:", error);
    return false;
  }
}

function buildCatalogMcpDrawerSurface(entry = {}) {
  const item = asObject(entry);
  const name = compactText(item.name || item.id || "MCP");
  const permission = compactText(item.permission || "readonly").toLowerCase() === "readwrite" ? "读写" : "只读";
  return normalizeMcpDrawerSurfacePayload({
    source: "mcp-catalog",
    surface: {
      kind: "card",
      id: `catalog:${compactText(item.id || name)}`,
      card: {
        id: `catalog:${compactText(item.id || name)}`,
        uiKind: "readonly_summary",
        placement: "drawer",
        title: name,
        summary: `${name} 已启用，可在任一 chat 中直接复用对应的监控与操作入口。`,
        freshness: {
          label: permission,
        },
        scope: {
          resourceType: "mcp",
          resourceId: compactText(item.id || name),
        },
        source: compactText(item.source || "local"),
      },
      source: "mcp-catalog",
      title: name,
      subtitle: `${permission} · ${compactText(item.source || "local")}`,
    },
  });
}

function normalizeCardText(card) {
  const candidates = [card?.text, card?.message, card?.summary, card?.title];
  for (const candidate of candidates) {
    const text = (candidate || "").trim().replace(/\s+/g, " ");
    if (text) return text;
  }
  return "";
}

function isUserCard(card) {
  return card?.type === "UserMessageCard" || (card?.type === "MessageCard" && card?.role === "user");
}

function isAssistantCard(card) {
  return card?.type === "AssistantMessageCard" || (card?.type === "MessageCard" && card?.role === "assistant");
}

function buildClientId(prefix) {
  const randomPart =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  return `${prefix}-${randomPart}`;
}

function buildOptimisticUserMessageCard(payload) {
  const source = asObject(payload);
  const text = compactText(source.message || source.text || payload);
  if (!text) return null;
  const timestamp = new Date().toISOString();
  const clientMessageId = compactText(source.clientMessageId) || buildClientId("client-msg");
  const clientTurnId = compactText(source.clientTurnId) || buildClientId("client-turn");
  return {
    id: compactText(source.id) || `optimistic-user-${clientMessageId}`,
    clientMessageId,
    clientTurnId,
    sessionId: compactText(source.sessionId),
    hostId: compactText(source.hostId),
    type: "UserMessageCard",
    role: "user",
    text,
    message: text,
    createdAt: timestamp,
    updatedAt: timestamp,
    optimistic: true,
    status: "pending",
  };
}

function reconcileOptimisticUserCards({ incomingCards, existingCards, optimisticCardIds }) {
  const serverCards = Array.isArray(incomingCards) ? [...incomingCards] : [];
  const pendingIds = Array.isArray(optimisticCardIds) ? optimisticCardIds.filter(Boolean) : [];
  if (!pendingIds.length) {
    return {
      cards: serverCards,
      optimisticCardIds: [],
    };
  }

  const currentCards = Array.isArray(existingCards) ? existingCards : [];
  const pendingCards = currentCards.filter((card) => pendingIds.includes(card?.id) && card?.optimistic === true);
  if (!pendingCards.length) {
    return {
      cards: serverCards,
      optimisticCardIds: [],
    };
  }

  const acknowledgedTexts = new Set(
    serverCards
      .filter((card) => isUserCard(card))
      .map((card) => normalizeCardText(card))
      .filter(Boolean),
  );

  const remainingOptimisticCards = pendingCards.filter((card) => {
    const text = normalizeCardText(card);
    return text && !acknowledgedTexts.has(text);
  });

  return {
    cards: [...serverCards, ...remainingOptimisticCards],
    optimisticCardIds: remainingOptimisticCards.map((card) => card.id),
  };
}

function isTerminalTurnPhase(phase) {
  return ["idle", "completed", "failed", "aborted"].includes(String(phase || "").trim().toLowerCase());
}

function normalizeTurnRuntime(turn = {}, fallbackHostId = "server-local", previousTurn = null) {
  const phase = String(turn?.phase || "idle").trim().toLowerCase() || "idle";
  const active = !isTerminalTurnPhase(phase) && !!turn?.active;
  const pendingStart =
    active || isTerminalTurnPhase(phase)
      ? false
      : Boolean(turn?.pendingStart ?? previousTurn?.pendingStart);
  return {
    active,
    phase,
    hostId: turn?.hostId || fallbackHostId,
    startedAt: turn?.startedAt || null,
    pendingStart,
    ...turn,
    active,
    phase,
    hostId: turn?.hostId || fallbackHostId,
    startedAt: turn?.startedAt || null,
    pendingStart,
  };
}

const KNOWN_TURN_EVENT_TYPES = new Set([
  "turn.started",
  "assistant.intent.delta",
  "assistant.final.delta",
  "tool.call.start",
  "tool.status.delta",
  "tool.result.done",
  "tool.result.error",
  "phase.end",
  "process.summary",
  "approval.required",
  "turn.done",
  "turn.error",
  "turn.aborted",
]);

function normalizeTurnEvent(event = {}) {
  const source = asObject(event);
  const type = compactText(source.type);
  const turnId = compactText(source.turnId);
  const eventId = compactText(source.eventId) || `${turnId || "turn"}:${type}:${source.seq ?? Date.now()}`;
  if (!type || !turnId || !KNOWN_TURN_EVENT_TYPES.has(type)) return null;
  return {
    type,
    sessionId: compactText(source.sessionId),
    turnId,
    eventId,
    seq: Number.isFinite(Number(source.seq)) ? Number(source.seq) : 0,
    createdAt: coerceIsoStamp(source.createdAt),
    payload: asObject(source.payload),
  };
}

function resolveToolEventId(event) {
  return (
    compactText(event.payload.toolCallId) ||
    compactText(event.payload.id) ||
    compactText(event.payload.callId) ||
    `${event.turnId}:tool:${event.seq || event.eventId}`
  );
}

function upsertProcessItem(items = [], nextItem = {}) {
  const normalized = asObject(nextItem);
  if (!compactText(normalized.id)) return items;
  const existing = Array.isArray(items) ? items : [];
  const index = existing.findIndex((item) => item?.id === normalized.id);
  if (index < 0) return [...existing, normalized];
  const merged = {
    ...existing[index],
    ...normalized,
    detail: compactText(normalized.detail || existing[index]?.detail),
    text: compactText(normalized.text || existing[index]?.text),
  };
  const next = [...existing];
  next.splice(index, 1, merged);
  return next;
}

function processItemFromTurnEvent(event, status) {
  const toolName = compactText(event.payload.toolName || event.payload.name || event.payload.tool);
  const title = compactText(event.payload.title || event.payload.text || event.payload.summary || toolName || "tool");
  return {
    id: resolveToolEventId(event),
    kind: "tool",
    processKind: toolName || "tool",
    toolName,
    text: title,
    detail: compactText(event.payload.detail || event.payload.inputSummary || event.payload.outputSummary || event.payload.result || event.payload.error),
    status,
    tone: status === "failed" ? "danger" : status === "completed" ? "success" : "neutral",
    createdAt: event.createdAt,
    updatedAt: event.createdAt,
  };
}

function toolInvocationFromTurnEvent(event, status) {
  const item = processItemFromTurnEvent(event, status);
  return {
    id: item.id,
    name: item.toolName || item.processKind,
    displayName: item.toolName || item.processKind,
    status,
    inputSummary: compactText(event.payload.inputSummary || event.payload.detail || event.payload.query || event.payload.title),
    outputSummary: compactText(event.payload.outputSummary || event.payload.result || event.payload.error),
    startedAt: event.createdAt,
    completedAt: status === "completed" || status === "failed" ? event.createdAt : "",
  };
}

function deriveSessionStatus(cards, runtime) {
  if (runtime?.turn?.active && !isTerminalTurnPhase(runtime?.turn?.phase)) {
    return runtime.turn.phase === "waiting_approval" ? "waiting_approval" : "running";
  }
  if (!cards?.length) return "empty";
  for (let i = cards.length - 1; i >= 0; i -= 1) {
    const card = cards[i];
    if (card?.type === "ErrorCard" || card?.status === "failed") return "failed";
    if (isUserCard(card) || isAssistantCard(card) || card?.type === "NoticeCard") break;
  }
  return "completed";
}

function deriveSessionSummary(snapshot, runtime) {
  const cards = snapshot?.cards || [];
  let title = "新建会话";
  let preview = "暂无消息";
  let messageCount = 0;

  for (const card of cards) {
    if (isUserCard(card) || isAssistantCard(card)) {
      messageCount += 1;
    }
    if (title === "新建会话" && isUserCard(card)) {
      const text = normalizeCardText(card);
      if (text) title = text.slice(0, 24);
    }
  }

  for (let i = cards.length - 1; i >= 0; i -= 1) {
    const text = normalizeCardText(cards[i]);
    if (text) {
      preview = text.slice(0, 60);
      break;
    }
  }

  return {
    id: snapshot?.sessionId || "",
    kind: snapshot?.kind || "single_host",
    title,
    preview,
    selectedHostId: snapshot?.selectedHostId || "server-local",
    status: deriveSessionStatus(cards, runtime),
    messageCount,
    lastActivityAt: snapshot?.lastActivityAt || "",
  };
}

function normalizeSessionSummary(session) {
  const raw = session && typeof session === "object" ? session : {};
  const cards = Array.isArray(raw.cards) ? raw.cards : [];
  const derived = deriveSessionSummary(
    {
      sessionId: raw.id || raw.sessionId || "",
      kind: raw.kind || "single_host",
      selectedHostId: raw.selectedHostId || "server-local",
      lastActivityAt: raw.lastActivityAt || "",
      cards,
    },
    null,
  );
  return {
    ...derived,
    id: String(raw.id || raw.sessionId || derived.id || ""),
    kind: String(raw.kind || derived.kind || "single_host"),
    title: String(raw.title || derived.title || "新建会话"),
    preview: String(raw.preview || derived.preview || "暂无消息"),
    selectedHostId: String(raw.selectedHostId || derived.selectedHostId || "server-local"),
    status: String(raw.status || derived.status || "empty"),
    messageCount: Number.isFinite(Number(raw.messageCount)) ? Number(raw.messageCount) : derived.messageCount,
    lastActivityAt: String(raw.lastActivityAt || derived.lastActivityAt || ""),
  };
}

function hostStatusLabel(status) {
  switch ((status || "").toLowerCase()) {
    case "online":
      return "在线";
    case "offline":
      return "离线";
    default:
      return status || "未知";
  }
}

function formatHostStatus(host) {
  const current = host || {};
  const id = current.id || "server-local";
  const name = current.name || id;
  return `当前主机 ${name}（${id}）状态 ${hostStatusLabel(current.status)}`;
}

function isConnectionLossMessage(message) {
  return /^与 ai-server 的连接已断开/.test((message || "").trim());
}

function normalizeWorkspaceReturnTargets(rawTargets) {
  if (!rawTargets || typeof rawTargets !== "object" || Array.isArray(rawTargets)) {
    return {};
  }
  const normalized = {};
  for (const [sessionId, workspaceSessionId] of Object.entries(rawTargets)) {
    const key = String(sessionId || "").trim();
    const value = String(workspaceSessionId || "").trim();
    if (key && value) {
      normalized[key] = value;
    }
  }
  return normalized;
}

const COMMAND_CATEGORY_META = [
  { id: "system_inspection", label: "系统检查" },
  { id: "service_read", label: "服务读取" },
  { id: "network_read", label: "网络读取" },
  { id: "file_read", label: "文件读取" },
  { id: "service_mutation", label: "服务变更" },
  { id: "filesystem_mutation", label: "文件系统变更" },
  { id: "package_mutation", label: "包管理变更" },
];

const CAPABILITY_META = [
  { id: "commandExecution", label: "命令执行" },
  { id: "fileRead", label: "文件读取" },
  { id: "fileSearch", label: "文件搜索" },
  { id: "fileChange", label: "文件修改" },
  { id: "terminal", label: "终端访问" },
  { id: "webSearch", label: "网页搜索" },
  { id: "webOpen", label: "网页打开" },
  { id: "approval", label: "审批请求" },
  { id: "multiAgent", label: "多 Agent 并行" },
  { id: "plan", label: "计划生成" },
  { id: "summary", label: "结果总结" },
];

const SKILL_CATALOG = [
  {
    id: "ops-triage",
    name: "Ops Triage",
    description: "快速归类问题并给出最小干预路径。",
    source: "built-in",
    defaultEnabled: true,
    defaultActivationMode: "default_enabled",
  },
  {
    id: "incident-summary",
    name: "Incident Summary",
    description: "把诊断过程整理成可交付摘要。",
    source: "local",
    defaultEnabled: true,
    defaultActivationMode: "default_enabled",
  },
  {
    id: "safe-change-review",
    name: "Safe Change Review",
    description: "在执行前做变更影响检查。",
    source: "built-in",
    defaultEnabled: false,
    defaultActivationMode: "explicit_only",
  },
  {
    id: "host-diagnostics",
    name: "Host Diagnostics",
    description: "收集主机健康与日志摘要。",
    source: "local",
    defaultEnabled: true,
    defaultActivationMode: "default_enabled",
  },
  {
    id: "host-change-review",
    name: "Host Change Review",
    description: "对主机变更做安全复核。",
    source: "built-in",
    defaultEnabled: false,
    defaultActivationMode: "explicit_only",
  },
];

const MCP_CATALOG = [
  {
    id: "filesystem",
    name: "Filesystem MCP",
    type: "stdio",
    source: "built-in",
    defaultEnabled: true,
    permission: "readonly",
    requiresExplicitUserApproval: false,
  },
  {
    id: "docs",
    name: "Docs MCP",
    type: "http",
    source: "local",
    defaultEnabled: true,
    permission: "readonly",
    requiresExplicitUserApproval: true,
  },
  {
    id: "metrics",
    name: "Metrics MCP",
    type: "http",
    source: "built-in",
    defaultEnabled: false,
    permission: "readwrite",
    requiresExplicitUserApproval: true,
  },
  {
    id: "host-files",
    name: "Host Files MCP",
    type: "stdio",
    source: "built-in",
    defaultEnabled: true,
    permission: "readonly",
    requiresExplicitUserApproval: false,
  },
  {
    id: "host-logs",
    name: "Host Logs MCP",
    type: "http",
    source: "local",
    defaultEnabled: true,
    permission: "readonly",
    requiresExplicitUserApproval: true,
  },
];

function cloneCatalogEntries(entries) {
  return (entries || []).map((item) => ({ ...item }));
}

function normalizeSkillCatalogEntry(rawItem, fallbackItem = {}) {
  const base = rawItem || {};
  const fallbackEnabled =
    typeof base.defaultEnabled === "boolean"
      ? base.defaultEnabled
      : typeof fallbackItem.defaultEnabled === "boolean"
        ? fallbackItem.defaultEnabled
        : typeof base.enabled === "boolean"
          ? base.enabled
          : false;
  const mode = normalizeSkillActivationMode(
    base.defaultActivationMode ?? base.default_activation_mode ?? base.activationMode ?? fallbackItem.defaultActivationMode,
    fallbackEnabled,
  );
  return {
    id: String(base.id || fallbackItem.id || ""),
    name: String(base.name || fallbackItem.name || base.id || fallbackItem.id || ""),
    description: String(base.description || fallbackItem.description || ""),
    source: String(base.source || fallbackItem.source || "local"),
    defaultEnabled: fallbackEnabled,
    defaultActivationMode: mode,
    enabled: fallbackEnabled,
    activationMode: mode,
  };
}

function normalizeMcpCatalogEntry(rawItem, fallbackItem = {}) {
  const base = rawItem || {};
  return {
    id: String(base.id || fallbackItem.id || ""),
    name: String(base.name || fallbackItem.name || base.id || fallbackItem.id || ""),
    type: String(base.type || fallbackItem.type || "stdio"),
    source: String(base.source || fallbackItem.source || "local"),
    defaultEnabled:
      typeof base.defaultEnabled === "boolean"
        ? base.defaultEnabled
        : typeof fallbackItem.defaultEnabled === "boolean"
          ? fallbackItem.defaultEnabled
          : typeof base.enabled === "boolean"
            ? base.enabled
            : false,
    enabled:
      typeof base.defaultEnabled === "boolean"
        ? base.defaultEnabled
        : typeof fallbackItem.defaultEnabled === "boolean"
          ? fallbackItem.defaultEnabled
          : typeof base.enabled === "boolean"
            ? base.enabled
            : false,
    permission: normalizeMcpPermission(base.permission, fallbackItem.permission),
    requiresExplicitUserApproval:
      typeof base.requiresExplicitUserApproval === "boolean"
        ? base.requiresExplicitUserApproval
        : base.requires_explicit_user_approval ?? fallbackItem.requiresExplicitUserApproval ?? false,
  };
}

function createSkillCatalog(entries = SKILL_CATALOG) {
  return normalizeSkillCatalogEntries(cloneCatalogEntries(entries), SKILL_CATALOG);
}

function createMcpCatalog(entries = MCP_CATALOG) {
  return normalizeMcpCatalogEntries(cloneCatalogEntries(entries), MCP_CATALOG);
}

function normalizeSkillCatalogEntries(entries, fallbackEntries = SKILL_CATALOG) {
  const fallbackMap = new Map((fallbackEntries || []).map((item) => [String(item?.id || ""), item]));
  return (entries || [])
    .map((item) => normalizeSkillCatalogEntry(item, fallbackMap.get(String(item?.id || "")) || {}))
    .filter((item) => item.id)
    .sort((left, right) => left.id.localeCompare(right.id));
}

function normalizeMcpCatalogEntries(entries, fallbackEntries = MCP_CATALOG) {
  const fallbackMap = new Map((fallbackEntries || []).map((item) => [String(item?.id || ""), item]));
  return (entries || [])
    .map((item) => normalizeMcpCatalogEntry(item, fallbackMap.get(String(item?.id || "")) || {}))
    .filter((item) => item.id)
    .sort((left, right) => left.id.localeCompare(right.id));
}

function normalizeSkillActivationMode(value, fallbackEnabled) {
  const mode = String(value || "").trim().toLowerCase();
  if (mode === "default" || mode === "default_enabled" || mode === "enabled") return "default_enabled";
  if (mode === "explicit" || mode === "explicit_only") return "explicit_only";
  if (mode === "disabled") return "disabled";
  if (typeof fallbackEnabled === "boolean") {
    return fallbackEnabled ? "default_enabled" : "disabled";
  }
  return "disabled";
}

function normalizeSkillEnabled(value, mode, fallbackEnabled) {
  if (mode === "disabled") return false;
  if (typeof value === "boolean") return value;
  if (typeof fallbackEnabled === "boolean") return fallbackEnabled;
  return mode === "default_enabled";
}

function normalizeMcpPermission(value, fallbackPermission) {
  const permission = String(value || fallbackPermission || "").trim().toLowerCase();
  if (permission === "readwrite" || permission === "read-write") return "readwrite";
  if (permission === "readonly" || permission === "read-only") return "readonly";
  return "readonly";
}

function normalizeMcpEnabled(value, fallbackEnabled) {
  if (typeof value === "boolean") return value;
  if (typeof fallbackEnabled === "boolean") return fallbackEnabled;
  return false;
}

function normalizeSkillItems(rawSkills, fallbackSkills = [], catalog = SKILL_CATALOG) {
  const fallbackMap = new Map((fallbackSkills || []).map((item) => [String(item?.id || ""), item]));
  const catalogMap = new Map(createSkillCatalog(catalog).map((item) => [String(item?.id || ""), item]));
  return (rawSkills || [])
    .map((item) => {
      const id = String(item?.id || "").trim();
      if (!id) return null;
      const fallback = fallbackMap.get(id) || {};
      const catalogItem = catalogMap.get(id) || {};
      const mode = normalizeSkillActivationMode(
        item?.activationMode ?? item?.activation_mode ?? fallback.activationMode ?? catalogItem.defaultActivationMode,
        item?.enabled ?? fallback.enabled ?? catalogItem.defaultEnabled,
      );
      return {
        id,
        name: String(item?.name || fallback.name || catalogItem.name || id),
        description: String(item?.description || fallback.description || catalogItem.description || ""),
        source: String(item?.source || fallback.source || catalogItem.source || "local"),
        enabled: normalizeSkillEnabled(item?.enabled, mode, fallback.enabled ?? catalogItem.defaultEnabled),
        activationMode: mode,
      };
    })
    .filter(Boolean);
}

function normalizeMcpItems(rawMcps, fallbackMcps = [], catalog = MCP_CATALOG) {
  const fallbackMap = new Map((fallbackMcps || []).map((item) => [String(item?.id || ""), item]));
  const catalogMap = new Map(createMcpCatalog(catalog).map((item) => [String(item?.id || ""), item]));
  return (rawMcps || [])
    .map((item) => {
      const id = String(item?.id || "").trim();
      if (!id) return null;
      const fallback = fallbackMap.get(id) || {};
      const catalogItem = catalogMap.get(id) || {};
      return {
        id,
        name: String(item?.name || fallback.name || catalogItem.name || id),
        type: String(item?.type || fallback.type || catalogItem.type || "stdio"),
        source: String(item?.source || fallback.source || catalogItem.source || "local"),
        enabled: normalizeMcpEnabled(item?.enabled, fallback.enabled ?? catalogItem.defaultEnabled),
        permission: normalizeMcpPermission(item?.permission, fallback.permission || catalogItem.permission),
        requiresExplicitUserApproval:
          typeof item?.requiresExplicitUserApproval === "boolean"
            ? item.requiresExplicitUserApproval
            : item?.requires_explicit_user_approval ?? fallback.requiresExplicitUserApproval ?? catalogItem.requiresExplicitUserApproval,
      };
    })
    .filter(Boolean);
}

function alignAgentProfileCollections(profile, skillCatalog = createSkillCatalog(), mcpCatalog = createMcpCatalog()) {
  if (!profile || typeof profile !== "object") {
    return profile;
  }
  return {
    ...profile,
    skills: normalizeSkillItems(profile.skills || [], profile.skills || [], skillCatalog),
    mcps: normalizeMcpItems(profile.mcps || [], profile.mcps || [], mcpCatalog),
  };
}

function serializeSkillItems(skills, catalog = createSkillCatalog()) {
  return normalizeSkillItems(skills || [], [], catalog).map((item) => ({
    id: String(item.id || ""),
    name: String(item.name || ""),
    description: String(item.description || ""),
    source: String(item.source || ""),
    enabled: Boolean(item.enabled),
    activationMode: normalizeSkillActivationMode(item.activationMode, item.enabled),
  }));
}

function serializeMcpItems(mcps, catalog = createMcpCatalog()) {
  return normalizeMcpItems(mcps || [], [], catalog).map((item) => ({
    id: String(item.id || ""),
    name: String(item.name || ""),
    type: String(item.type || ""),
    source: String(item.source || ""),
    enabled: Boolean(item.enabled),
    permission: normalizeMcpPermission(item.permission),
    requiresExplicitUserApproval: Boolean(item.requiresExplicitUserApproval),
  }));
}

function profileHasHighRiskCombination(profile) {
  const commandPermissions = profile?.commandPermissions || {};
  const categoryPolicies = Array.isArray(commandPermissions.categoryPolicies) ? commandPermissions.categoryPolicies : [];
  const packageMutation = categoryPolicies.find((item) => item?.id === "package_mutation")?.mode;
  const filesystemMutation = categoryPolicies.find((item) => item?.id === "filesystem_mutation")?.mode;
  const serviceMutation = categoryPolicies.find((item) => item?.id === "service_mutation")?.mode;
  return Boolean(
    (commandPermissions.enabled && commandPermissions.allowSudo) ||
      packageMutation === "allow" ||
      filesystemMutation === "allow" ||
      serviceMutation === "allow" ||
      String(profile?.runtime?.sandboxMode || "") === "danger-full-access",
  );
}

function createDefaultAgentProfiles() {
  return [
    {
      id: "main-agent",
      name: "main-agent",
      type: "main-agent",
      description: "系统默认主 Agent 配置，用于会话编排、规划和结果收敛。",
      updatedAt: "2026-03-28 00:00:00",
      updatedBy: "system",
      runtime: {
        model: "gpt-5.4",
        reasoningEffort: "medium",
        approvalPolicy: "untrusted",
        sandboxMode: "workspace-write",
      },
      systemPrompt: {
        content:
          "你是主 Agent。优先收敛目标、分解任务、控制风险，并在输出中保持清晰、可执行和可回溯。遇到高风险变更时，先说明边界，再给出最小影响方案。",
        preview:
          "你是主 Agent。优先收敛目标、分解任务、控制风险，并在输出中保持清晰、可执行和可回溯。",
        notes: "面向会话层编排与统一决策，不包含主机运行态。",
      },
      commandPermissions: {
        enabled: true,
        defaultMode: "approval_required",
        allowShellWrapper: true,
        allowSudo: false,
        defaultTimeoutSeconds: 300,
        allowedWritableRoots: ["/workspace", "/tmp"],
        categoryPolicies: [
          { id: "system_inspection", label: "系统检查", mode: "allow" },
          { id: "service_read", label: "服务读取", mode: "allow" },
          { id: "network_read", label: "网络读取", mode: "approval_required" },
          { id: "file_read", label: "文件读取", mode: "allow" },
          { id: "filesystem_mutation", label: "文件系统变更", mode: "approval_required" },
          { id: "service_mutation", label: "服务变更", mode: "approval_required" },
          { id: "package_mutation", label: "包管理变更", mode: "deny" },
        ],
      },
      capabilityPermissions: [
        { id: "commandExecution", label: "命令执行", state: "approval_required" },
        { id: "fileRead", label: "文件读取", state: "enabled" },
        { id: "fileSearch", label: "文件搜索", state: "enabled" },
        { id: "fileChange", label: "文件修改", state: "approval_required" },
        { id: "terminal", label: "终端访问", state: "approval_required" },
        { id: "webSearch", label: "网页搜索", state: "enabled" },
        { id: "webOpen", label: "网页打开", state: "approval_required" },
        { id: "approval", label: "审批请求", state: "enabled" },
        { id: "multiAgent", label: "多 Agent 并行", state: "enabled" },
        { id: "plan", label: "计划生成", state: "enabled" },
        { id: "summary", label: "结果总结", state: "enabled" },
      ],
      skills: [
        {
          id: "ops-triage",
          name: "Ops Triage",
          description: "快速归类问题并给出最小干预路径。",
          source: "built-in",
          enabled: true,
          activationMode: "default",
        },
        {
          id: "incident-summary",
          name: "Incident Summary",
          description: "把诊断过程整理成可交付摘要。",
          source: "local",
          enabled: true,
          activationMode: "default",
        },
        {
          id: "safe-change-review",
          name: "Safe Change Review",
          description: "在执行前做变更影响检查。",
          source: "built-in",
          enabled: false,
          activationMode: "explicit",
        },
      ],
      mcps: [
        {
          id: "filesystem",
          name: "Filesystem MCP",
          type: "stdio",
          source: "built-in",
          enabled: true,
          permission: "read-only",
          requiresExplicitUserApproval: false,
        },
        {
          id: "docs",
          name: "Docs MCP",
          type: "http",
          source: "local",
          enabled: true,
          permission: "read-only",
          requiresExplicitUserApproval: true,
        },
        {
          id: "metrics",
          name: "Metrics MCP",
          type: "http",
          source: "built-in",
          enabled: false,
          permission: "read-write",
          requiresExplicitUserApproval: true,
        },
      ],
    },
    {
      id: "host-agent-default",
      name: "host-agent-default",
      type: "host-agent-default",
      description: "默认 host-agent 静态配置，偏向安全读取和受限执行。",
      updatedAt: "2026-03-28 00:00:00",
      updatedBy: "system",
      runtime: {
        model: "gpt-5.4-mini",
        reasoningEffort: "low",
        approvalPolicy: "untrusted",
        sandboxMode: "workspace-write",
      },
      systemPrompt: {
        content:
          "你是 host-agent。只负责在受控边界内执行局部操作，优先只读、低风险和可回滚的动作。任何变更都要保持最小范围，并在必要时请求审批。",
        preview:
          "你是 host-agent。只负责在受控边界内执行局部操作，优先只读、低风险和可回滚的动作。",
        notes: "面向主机侧默认执行边界，不展示主机心跳或在线信息。",
      },
      commandPermissions: {
        enabled: true,
        defaultMode: "readonly_only",
        allowShellWrapper: true,
        allowSudo: false,
        defaultTimeoutSeconds: 180,
        allowedWritableRoots: ["/tmp"],
        categoryPolicies: [
          { id: "system_inspection", label: "系统检查", mode: "allow" },
          { id: "service_read", label: "服务读取", mode: "allow" },
          { id: "network_read", label: "网络读取", mode: "allow" },
          { id: "file_read", label: "文件读取", mode: "allow" },
          { id: "filesystem_mutation", label: "文件系统变更", mode: "approval_required" },
          { id: "service_mutation", label: "服务变更", mode: "approval_required" },
          { id: "package_mutation", label: "包管理变更", mode: "deny" },
        ],
      },
      capabilityPermissions: [
        { id: "commandExecution", label: "命令执行", state: "approval_required" },
        { id: "fileRead", label: "文件读取", state: "enabled" },
        { id: "fileSearch", label: "文件搜索", state: "enabled" },
        { id: "fileChange", label: "文件修改", state: "disabled" },
        { id: "terminal", label: "终端访问", state: "enabled" },
        { id: "webSearch", label: "网页搜索", state: "disabled" },
        { id: "webOpen", label: "网页打开", state: "disabled" },
        { id: "approval", label: "审批请求", state: "enabled" },
        { id: "multiAgent", label: "多 Agent 并行", state: "disabled" },
        { id: "plan", label: "计划生成", state: "enabled" },
        { id: "summary", label: "结果总结", state: "enabled" },
      ],
      skills: [
        {
          id: "host-diagnostics",
          name: "Host Diagnostics",
          description: "收集主机健康与日志摘要。",
          source: "local",
          enabled: true,
          activationMode: "default",
        },
        {
          id: "host-change-review",
          name: "Host Change Review",
          description: "对主机变更做安全复核。",
          source: "built-in",
          enabled: false,
          activationMode: "explicit",
        },
      ],
      mcps: [
        {
          id: "host-files",
          name: "Host Files MCP",
          type: "stdio",
          source: "built-in",
          enabled: true,
          permission: "read-only",
          requiresExplicitUserApproval: false,
        },
        {
          id: "host-logs",
          name: "Host Logs MCP",
          type: "http",
          source: "local",
          enabled: true,
          permission: "read-only",
          requiresExplicitUserApproval: true,
        },
      ],
    },
  ];
}

function realignAgentProfiles(store) {
  const skillCatalog = Array.isArray(store.skillCatalog) ? store.skillCatalog : createSkillCatalog();
  const mcpCatalog = Array.isArray(store.mcpCatalog) ? store.mcpCatalog : createMcpCatalog();
  store.agentProfiles = (store.agentProfiles || []).map((profile) => alignAgentProfileCollections(profile, skillCatalog, mcpCatalog));
  store.agentProfileDefaults = (store.agentProfileDefaults || []).map((profile) => alignAgentProfileCollections(profile, skillCatalog, mcpCatalog));
}

function normalizeCategoryPolicies(rawPolicies, fallbackPolicies = []) {
  const fallbackMap = new Map((fallbackPolicies || []).map((item) => [item.id, item]));
  const policies = [];
  const pushPolicy = (id, mode) => {
    if (!id) return;
    const fallback = fallbackMap.get(id) || {};
    policies.push({
      id,
      label: fallback.label || COMMAND_CATEGORY_META.find((item) => item.id === id)?.label || id,
      mode: String(mode || fallback.mode || "approval_required"),
      description: String(fallback.description || ""),
    });
  };
  if (Array.isArray(rawPolicies)) {
    rawPolicies.forEach((item) => pushPolicy(String(item?.id || ""), item?.mode || item?.state));
  } else if (rawPolicies && typeof rawPolicies === "object") {
    Object.entries(rawPolicies).forEach(([id, mode]) => pushPolicy(id, mode));
  }
  if (!policies.length) {
    return [...fallbackPolicies];
  }
  return COMMAND_CATEGORY_META.map((item) => policies.find((entry) => entry.id === item.id) || {
    id: item.id,
    label: item.label,
    mode: fallbackMap.get(item.id)?.mode || "approval_required",
    description: fallbackMap.get(item.id)?.description || "",
  });
}

function normalizeCapabilityPermissions(rawCapabilities, fallbackCapabilities = []) {
  const fallbackMap = new Map((fallbackCapabilities || []).map((item) => [item.id, item]));
  const capabilities = [];
  const pushCapability = (id, state) => {
    if (!id) return;
    const fallback = fallbackMap.get(id) || {};
    capabilities.push({
      id,
      label: fallback.label || CAPABILITY_META.find((item) => item.id === id)?.label || id,
      state: String(state || fallback.state || "enabled"),
    });
  };
  if (Array.isArray(rawCapabilities)) {
    rawCapabilities.forEach((item) => pushCapability(String(item?.id || ""), item?.state));
  } else if (rawCapabilities && typeof rawCapabilities === "object") {
    Object.entries(rawCapabilities).forEach(([id, state]) => pushCapability(id, state));
  }
  if (!capabilities.length) {
    return [...fallbackCapabilities];
  }
  return CAPABILITY_META.map((item) => capabilities.find((entry) => entry.id === item.id) || {
    id: item.id,
    label: item.label,
    state: fallbackMap.get(item.id)?.state || "enabled",
  });
}

function resolveProfileCollection(rawProfile, keys, fallbackItems = []) {
  const source = rawProfile && typeof rawProfile === "object" ? rawProfile : {};
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(source, key)) {
      return Array.isArray(source[key]) ? source[key] : [];
    }
  }
  return Array.isArray(fallbackItems) ? fallbackItems : [];
}

function normalizeAgentProfile(rawProfile, fallbackProfile, catalogs = {}) {
  const fallback = fallbackProfile || {};
  const raw = rawProfile || {};
  const skillCatalog = Array.isArray(catalogs.skillCatalog) ? catalogs.skillCatalog : createSkillCatalog();
  const mcpCatalog = Array.isArray(catalogs.mcpCatalog) ? catalogs.mcpCatalog : createMcpCatalog();
  const runtime = raw.runtime || raw.runtime_settings || {};
  const systemPrompt = raw.systemPrompt || raw.system_prompt || {};
  const commandPermissions = raw.commandPermissions || raw.command_permissions || {};
  const capabilityPermissions = raw.capabilityPermissions || raw.capability_permissions || {};
  const skills = resolveProfileCollection(raw, ["skills"], fallback.skills || []);
  const mcps = resolveProfileCollection(raw, ["mcps", "mcpServers"], fallback.mcps || []);

  const normalized = {
    id: String(raw.id || fallback.id || raw.type || fallback.type || ""),
    name: String(raw.name || fallback.name || raw.id || fallback.id || ""),
    type: String(raw.type || fallback.type || raw.id || fallback.id || "main-agent"),
    description: String(raw.description || fallback.description || ""),
    updatedAt: String(raw.updatedAt || raw.updated_at || fallback.updatedAt || ""),
    updatedBy: String(raw.updatedBy || raw.updated_by || fallback.updatedBy || ""),
    runtime: {
      model: String(runtime.model || fallback.runtime?.model || "gpt-5.4"),
      reasoningEffort: String(runtime.reasoningEffort || runtime.reasoning_effort || fallback.runtime?.reasoningEffort || "medium"),
      approvalPolicy: String(runtime.approvalPolicy || runtime.approval_policy || fallback.runtime?.approvalPolicy || "untrusted"),
      sandboxMode: String(runtime.sandboxMode || runtime.sandbox_mode || fallback.runtime?.sandboxMode || "workspace-write"),
    },
    systemPrompt: {
      content: String(systemPrompt.content || fallback.systemPrompt?.content || ""),
      preview: String(systemPrompt.preview || fallback.systemPrompt?.preview || ""),
      version: String(systemPrompt.version || fallback.systemPrompt?.version || ""),
      notes: String(systemPrompt.notes || fallback.systemPrompt?.notes || ""),
    },
    commandPermissions: {
      enabled: typeof commandPermissions.enabled === "boolean" ? commandPermissions.enabled : fallback.commandPermissions?.enabled ?? true,
      defaultMode: String(commandPermissions.defaultMode || commandPermissions.default_mode || fallback.commandPermissions?.defaultMode || "approval_required"),
      allowShellWrapper:
        typeof commandPermissions.allowShellWrapper === "boolean"
          ? commandPermissions.allowShellWrapper
          : commandPermissions.allow_shell_wrapper ?? fallback.commandPermissions?.allowShellWrapper ?? true,
      allowSudo:
        typeof commandPermissions.allowSudo === "boolean"
          ? commandPermissions.allowSudo
          : commandPermissions.allow_sudo ?? fallback.commandPermissions?.allowSudo ?? false,
      defaultTimeoutSeconds: Number(commandPermissions.defaultTimeoutSeconds || commandPermissions.default_timeout_seconds || fallback.commandPermissions?.defaultTimeoutSeconds || 0),
      allowedWritableRoots: Array.isArray(commandPermissions.allowedWritableRoots || commandPermissions.allowed_writable_roots)
        ? (commandPermissions.allowedWritableRoots || commandPermissions.allowed_writable_roots).map((item) => String(item))
        : [...(fallback.commandPermissions?.allowedWritableRoots || [])],
      categoryPolicies: normalizeCategoryPolicies(
        commandPermissions.categoryPolicies || commandPermissions.category_policies,
        fallback.commandPermissions?.categoryPolicies || [],
      ),
    },
    capabilityPermissions: normalizeCapabilityPermissions(capabilityPermissions, fallback.capabilityPermissions || []),
    skills: normalizeSkillItems(skills, fallback.skills || [], skillCatalog),
    mcps: normalizeMcpItems(mcps, fallback.mcps || [], mcpCatalog),
  };

  return alignAgentProfileCollections(normalized, skillCatalog, mcpCatalog);
}

function serializeAgentProfile(profile, options = {}) {
  const skillCatalog = Array.isArray(options.skillCatalog) ? options.skillCatalog : createSkillCatalog();
  const mcpCatalog = Array.isArray(options.mcpCatalog) ? options.mcpCatalog : createMcpCatalog();
  const normalized = normalizeAgentProfile(profile, null, { skillCatalog, mcpCatalog });
  return {
    id: String(normalized?.id || ""),
    name: String(normalized?.name || ""),
    type: String(normalized?.type || "main-agent"),
    description: String(normalized?.description || ""),
    runtime: {
      model: String(normalized?.runtime?.model || "gpt-5.4"),
      reasoningEffort: String(normalized?.runtime?.reasoningEffort || "medium"),
      approvalPolicy: String(normalized?.runtime?.approvalPolicy || "untrusted"),
      sandboxMode: String(normalized?.runtime?.sandboxMode || "workspace-write"),
    },
    systemPrompt: {
      content: String(normalized?.systemPrompt?.content || ""),
      preview: String(normalized?.systemPrompt?.preview || ""),
      version: String(normalized?.systemPrompt?.version || ""),
      notes: String(normalized?.systemPrompt?.notes || ""),
    },
    commandPermissions: {
      enabled: Boolean(normalized?.commandPermissions?.enabled),
      defaultMode: String(normalized?.commandPermissions?.defaultMode || "approval_required"),
      allowShellWrapper: Boolean(normalized?.commandPermissions?.allowShellWrapper),
      allowSudo: Boolean(normalized?.commandPermissions?.allowSudo),
      defaultTimeoutSeconds: Number(normalized?.commandPermissions?.defaultTimeoutSeconds || 120),
      allowedWritableRoots: Array.isArray(normalized?.commandPermissions?.allowedWritableRoots)
        ? normalized.commandPermissions.allowedWritableRoots.map((item) => String(item).trim()).filter(Boolean)
        : [],
      categoryPolicies: Object.fromEntries(
        (normalized?.commandPermissions?.categoryPolicies || []).map((item) => [String(item.id || ""), String(item.mode || "approval_required")]),
      ),
    },
    capabilityPermissions: Object.fromEntries(
      (normalized?.capabilityPermissions || []).map((item) => [String(item.id || ""), String(item.state || "enabled")]),
    ),
    skills: serializeSkillItems(normalized?.skills || [], skillCatalog),
    mcps: serializeMcpItems(normalized?.mcps || [], mcpCatalog),
    riskConfirmed: Boolean(options?.riskConfirmed),
  };
}

function normalizeAgentProfileFieldErrors(rawFieldErrors) {
  if (!rawFieldErrors || typeof rawFieldErrors !== "object" || Array.isArray(rawFieldErrors)) {
    return {};
  }
  const fieldErrors = {};
  for (const [field, value] of Object.entries(rawFieldErrors)) {
    const key = String(field || "").trim();
    if (!key) continue;
    if (Array.isArray(value)) {
      const message = value.map((item) => String(item || "").trim()).filter(Boolean).join("；");
      if (message) fieldErrors[key] = message;
      continue;
    }
    if (value && typeof value === "object") {
      const nested = normalizeAgentProfileFieldErrors(value);
      for (const [nestedKey, nestedValue] of Object.entries(nested)) {
        fieldErrors[`${key}.${nestedKey}`] = nestedValue;
      }
      continue;
    }
    const message = String(value || "").trim();
    if (message) fieldErrors[key] = message;
  }
  return fieldErrors;
}

function clearAgentProfileErrorState(store) {
  store.agentProfilesError = "";
  store.agentProfileFieldErrors = {};
}

function applyAgentProfileErrorState(store, error, fieldErrors) {
  store.agentProfilesError = String(error || "").trim() || "Agent Profile 请求失败";
  store.agentProfileFieldErrors = normalizeAgentProfileFieldErrors(fieldErrors);
}

function cloneAgentProfilesForExport(profiles, catalogs = {}) {
  return (profiles || []).map((profile) => normalizeAgentProfile(profile, null, catalogs));
}

function buildAgentProfilesExportPayload(profiles, overrides = {}, catalogs = {}) {
  const items = cloneAgentProfilesForExport(profiles, catalogs);
  return {
    version: 1,
    configVersion: overrides.configVersion || 1,
    exportedAt: overrides.exportedAt || new Date().toISOString(),
    exportedBy: overrides.exportedBy || "",
    count: items.length,
    profiles: items,
  };
}

function parseAgentProfilesImportPayload(payload) {
  if (!payload) return [];
  if (Array.isArray(payload)) return payload;
  if (typeof payload !== "object") return [];
  if (Array.isArray(payload.profiles)) return payload.profiles;
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.agentProfiles)) return payload.agentProfiles;
  if (payload.profile && typeof payload.profile === "object") return [payload.profile];
  if (payload.data && Array.isArray(payload.data)) return payload.data;
  return [];
}

function normalizeImportedAgentProfiles(rawProfiles, catalogs = {}) {
  const defaults = createDefaultAgentProfiles();
  const skillCatalog = Array.isArray(catalogs.skillCatalog) ? catalogs.skillCatalog : createSkillCatalog();
  const mcpCatalog = Array.isArray(catalogs.mcpCatalog) ? catalogs.mcpCatalog : createMcpCatalog();
  const imported = Array.isArray(rawProfiles) ? rawProfiles.filter((item) => item && typeof item === "object") : [];
  const importedById = new Map();
  for (const profile of imported) {
    const key = String(profile.id || profile.type || "").trim();
    if (!key) continue;
    importedById.set(key, profile);
  }

  const merged = defaults.map((fallbackProfile) => {
    const incoming =
      importedById.get(fallbackProfile.id) ||
      importedById.get(fallbackProfile.type) ||
      null;
    return normalizeAgentProfile(incoming || fallbackProfile, fallbackProfile, { skillCatalog, mcpCatalog });
  });

  for (const profile of imported) {
    const key = String(profile.id || profile.type || "").trim();
    if (!key) continue;
    if (merged.some((item) => item.id === key || item.type === key)) continue;
    merged.push(normalizeAgentProfile(profile, null, { skillCatalog, mcpCatalog }));
  }

  return merged;
}

async function readAgentProfilesImportSource(source) {
  if (typeof source === "string") {
    return source;
  }
  if (source && typeof source.text === "function") {
    return await source.text();
  }
  if (source && typeof source.content === "string") {
    return source.content;
  }
  throw new Error("unsupported import source");
}

export const useAppStore = defineStore("app", {
  state: () => ({
    snapshot: {
      sessionId: "",
      kind: "single_host",
      selectedHostId: "server-local",
      auth: {
        connected: false,
        pending: false,
        mode: "",
        planType: "",
        email: "",
        lastError: "",
      },
      hosts: [],
      cards: [],
      approvals: [],
      agentLoop: null,
      agentLoopIterations: [],
      incidentEvents: [],
      verificationRecords: [],
      currentStage: "",
      currentMode: "",
      currentLane: "",
      requiredNextTool: "",
      finalGateStatus: "",
      missingRequirements: [],
      turnPolicy: null,
      promptEnvelope: null,
      toolInvocations: [],
      evidenceSummaries: [],
      config: {
        oauthConfigured: false,
        codexAlive: false,
      },
    },
    /* Turn-level and connection-level runtime state */
    runtime: {
      turn: {
        active: false,
        phase: "idle", // idle | thinking | planning | waiting_approval | waiting_input | executing | finalizing | completed | failed | aborted
        hostId: "",
        startedAt: null,
        pendingStart: false,
      },
      codex: {
        status: "connected", // connected | reconnecting | disconnected | stopped
        retryAttempt: 0,
        retryMax: 5,
        lastError: "",
      },
      activity: {
        filesViewed: 0,
        searchCount: 0,
        searchLocationCount: 0,
        listCount: 0,
        commandsRun: 0,
        filesChanged: 0,
        currentReadingFile: "",
        currentChangingFile: "",
        currentListingPath: "",
        currentSearchKind: "",
        currentSearchQuery: "",
        viewedFiles: [],
        currentWebSearchQuery: "",
        searchedWebQueries: [],
        searchedContentQueries: [],
      },
    },
    authForm: {
      mode: "chatgpt",
      apiKey: "",
      accessToken: "",
      chatgptAccountId: "",
      chatgptPlanType: "",
      email: "",
    },
    settings: {
      quota: "",
      model: "gpt-4-turbo",
      reasoningEffort: "medium",
      models: [],
    },
    skillCatalog: createSkillCatalog(),
    mcpCatalog: createMcpCatalog(),
    agentProfileDefaults: createDefaultAgentProfiles().map((profile) => normalizeAgentProfile(profile, null)),
    agentProfiles: createDefaultAgentProfiles().map((profile) => normalizeAgentProfile(profile, null)),
    agentProfilesLoading: false,
    agentProfilesError: "",
    agentProfileFieldErrors: {},
    activeAgentProfileId: "main-agent",
    mcpDrawer: readPersistedMcpDrawerState(),
    agentProfilePreview: null,
    agentProfilePreviewLoading: false,
    agentProfileSaving: false,
    optimisticCardIds: [],
    turnEventsById: {},
    processItemsByTurnId: {},
    phaseFoldsByTurnId: {},
    assistantDraftsByTurnId: {},
    finalMessagesByTurnId: {},
    sessionList: [],
    activeSessionId: "",
    workspaceReturnTargets: {},
    historyLoading: false,
    loading: true,
    errorMessage: "",
    noticeMessage: "",
    sending: false,
    wsStatus: "disconnected",
  }),
  getters: {
    selectedHost: (state) => {
      return (
        state.snapshot.hosts.find((host) => host.id === state.snapshot.selectedHostId) || {
          id: state.snapshot.selectedHostId,
          name: state.snapshot.selectedHostId,
          status: "offline",
          executable: false,
          terminalCapable: false,
        }
      );
    },
    canSend: (state) => {
      const host = (
        state.snapshot.hosts.find((h) => h.id === state.snapshot.selectedHostId) || {
          executable: false,
          terminalCapable: false,
          status: "offline",
        }
      );
      return (
        state.snapshot.auth.connected &&
        state.snapshot.config.codexAlive !== false &&
        host.executable &&
        host.status === "online"
      );
    },
    canOpenTerminal: (state) => {
      const host = (
        state.snapshot.hosts.find((h) => h.id === state.snapshot.selectedHostId) || {
          executable: false,
          terminalCapable: false,
          status: "offline",
        }
      );
      return host.status === "online" && (host.terminalCapable || host.executable);
    },
    activeSessionSummary: (state) => {
      return state.sessionList.find((session) => session.id === state.activeSessionId) || null;
    },
    workspaceReturnSessionId: (state) => {
      return String(state.workspaceReturnTargets[state.activeSessionId] || "");
    },
    workspaceReturnSession: (state) => {
      const targetSessionId = String(state.workspaceReturnTargets[state.activeSessionId] || "");
      if (!targetSessionId) {
        return null;
      }
      return state.sessionList.find((session) => session.id === targetSessionId && session.kind === "workspace") || null;
    },
    activeAgentProfile: (state) => {
      return state.agentProfiles.find((profile) => profile.id === state.activeAgentProfileId) || state.agentProfiles[0] || null;
    },
    enabledMcpEntries: (state) => {
      const overrides = new Map((state.activeAgentProfile?.mcps || []).map((item) => [String(item?.id || ""), item]));
      const catalogEntries = Array.isArray(state.mcpCatalog) ? state.mcpCatalog : [];
      return catalogEntries
        .map((entry) => {
          const override = overrides.get(String(entry?.id || ""));
          const enabled =
            typeof override?.enabled === "boolean"
              ? override.enabled
              : typeof entry?.enabled === "boolean"
                ? entry.enabled
                : Boolean(entry?.defaultEnabled);
          return {
            ...entry,
            ...(override || {}),
            enabled,
            permission: normalizeMcpPermission(override?.permission, entry?.permission),
          };
        })
        .filter((entry) => entry.enabled);
    },
  },
  actions: {
    hydrateMcpDrawerState() {
      this.mcpDrawer = readPersistedMcpDrawerState();
      return this.mcpDrawer;
    },
    persistMcpDrawerState() {
      return persistMcpDrawerState(this.mcpDrawer);
    },
    getEnabledMcpEntries() {
      return this.enabledMcpEntries;
    },
    openMcpDrawerSurface(payload, options = {}) {
      const surface = normalizeMcpDrawerSurfacePayload(payload);
      if (!surface) return null;
      const reason = compactText(options.reason || payload?.lastReason || "open");
      this.mcpDrawer.activeSurface = {
        ...(this.mcpDrawer.activeSurface?.id === surface.id ? this.mcpDrawer.activeSurface : {}),
        ...surface,
        lastReason: reason,
        touchedAt: coerceIsoStamp(options.touchedAt || surface.touchedAt),
        openedAt: coerceIsoStamp(options.touchedAt || surface.touchedAt),
      };
      this.mcpDrawer.recentSurfaces = upsertMcpDrawerSurfaceList(this.mcpDrawer.recentSurfaces, this.mcpDrawer.activeSurface, {
        limit: MCP_DRAWER_RECENT_LIMIT,
        reason,
      });
      if (options.pin) {
        this.mcpDrawer.pinnedSurfaces = upsertMcpDrawerSurfaceList(this.mcpDrawer.pinnedSurfaces, this.mcpDrawer.activeSurface, {
          limit: MCP_DRAWER_PIN_LIMIT,
          reason: compactText(options.pinReason || reason || "pin"),
          pin: true,
        });
      }
      this.persistMcpDrawerState();
      return this.mcpDrawer.activeSurface;
    },
    recordRecentMcpSurface(payload, options = {}) {
      const surface = normalizeMcpDrawerSurfacePayload(payload);
      if (!surface) return null;
      this.mcpDrawer.recentSurfaces = upsertMcpDrawerSurfaceList(this.mcpDrawer.recentSurfaces, surface, {
        limit: MCP_DRAWER_RECENT_LIMIT,
        reason: compactText(options.reason || payload?.lastReason || "recent"),
      });
      if (options.activate) {
        this.mcpDrawer.activeSurface = this.mcpDrawer.recentSurfaces[0] || null;
      }
      this.persistMcpDrawerState();
      return this.mcpDrawer.recentSurfaces[0] || null;
    },
    pinMcpDrawerSurface(payload) {
      const baseSurface = normalizeMcpDrawerSurfacePayload(payload || this.mcpDrawer.activeSurface);
      if (!baseSurface) return null;
      const activeSurface = this.openMcpDrawerSurface(baseSurface, { reason: "pin", pin: true, pinReason: "pin" });
      this.mcpDrawer.pinnedSurfaces = upsertMcpDrawerSurfaceList(this.mcpDrawer.pinnedSurfaces, activeSurface, {
        limit: MCP_DRAWER_PIN_LIMIT,
        reason: "pin",
        pin: true,
      });
      this.persistMcpDrawerState();
      return this.mcpDrawer.pinnedSurfaces[0] || activeSurface;
    },
    removePinnedMcpDrawerSurface(surfaceId = "") {
      const normalizedId = compactText(surfaceId);
      if (!normalizedId) return false;
      this.mcpDrawer.pinnedSurfaces = (this.mcpDrawer.pinnedSurfaces || []).filter((item) => item.id !== normalizedId);
      if (this.mcpDrawer.activeSurface?.id === normalizedId) {
        this.mcpDrawer.activeSurface = this.mcpDrawer.pinnedSurfaces[0] || this.mcpDrawer.recentSurfaces[0] || null;
      }
      this.persistMcpDrawerState();
      return true;
    },
    selectMcpDrawerSurface(target) {
      const normalizedTarget = compactText(typeof target === "string" ? target : target?.id);
      const directSurface = typeof target === "string" ? null : normalizeMcpDrawerSurfacePayload(target);
      const surface =
        directSurface ||
        this.mcpDrawer.pinnedSurfaces.find((item) => item.id === normalizedTarget) ||
        this.mcpDrawer.recentSurfaces.find((item) => item.id === normalizedTarget) ||
        null;
      if (!surface) return null;
      this.mcpDrawer.activeSurface = normalizeMcpDrawerSurfacePayload(surface);
      this.recordRecentMcpSurface(this.mcpDrawer.activeSurface, { reason: "select" });
      this.persistMcpDrawerState();
      return this.mcpDrawer.activeSurface;
    },
    touchActiveMcpDrawerSurface(reason = "view") {
      if (!this.mcpDrawer.activeSurface) return null;
      const surface = this.recordRecentMcpSurface(
        {
          ...this.mcpDrawer.activeSurface,
          lastReason: compactText(reason || "view"),
        },
        { reason: compactText(reason || "view"), activate: true },
      );
      this.persistMcpDrawerState();
      return surface;
    },
    openEnabledMcpEntry(entry, options = {}) {
      const surface = buildCatalogMcpDrawerSurface(entry);
      if (!surface) return null;
      return this.openMcpDrawerSurface(surface, {
        reason: compactText(options.reason || "catalog"),
        pin: Boolean(options.pin),
        pinReason: "catalog",
      });
    },
    applySnapshot(data) {
      this.snapshot.sessionId = data.sessionId || this.snapshot.sessionId;
      this.snapshot.kind = data.kind || this.snapshot.kind || "single_host";
      this.activeSessionId = this.snapshot.sessionId;
      this.snapshot.selectedHostId = data.selectedHostId || this.snapshot.selectedHostId;
      this.snapshot.auth = data.auth || this.snapshot.auth;
      this.snapshot.hosts = data.hosts || [];
      const shouldPreserveOptimisticCards = this.sending || this.runtime.turn.pendingStart || this.runtime.turn.active;
      const incomingCards = Array.isArray(data.cards) ? data.cards : this.snapshot.cards;
      if (shouldPreserveOptimisticCards) {
        const reconciled = reconcileOptimisticUserCards({
          incomingCards,
          existingCards: this.snapshot.cards,
          optimisticCardIds: this.optimisticCardIds,
        });
        this.snapshot.cards = reconciled.cards;
        this.optimisticCardIds = reconciled.optimisticCardIds;
      } else {
        this.snapshot.cards = incomingCards;
        this.optimisticCardIds = [];
      }
      this.snapshot.approvals = data.approvals || [];
      this.snapshot.agentLoop = data.agentLoop || null;
      this.snapshot.agentLoopIterations = Array.isArray(data.agentLoopIterations) ? data.agentLoopIterations : [];
      this.snapshot.incidentEvents = Array.isArray(data.incidentEvents) ? data.incidentEvents : [];
      this.snapshot.verificationRecords = Array.isArray(data.verificationRecords) ? data.verificationRecords : [];
      this.snapshot.currentStage = compactText(data.currentStage || "");
      this.snapshot.currentMode = compactText(data.currentMode || "");
      this.snapshot.currentLane = compactText(data.currentLane || "");
      this.snapshot.requiredNextTool = compactText(data.requiredNextTool || "");
      this.snapshot.finalGateStatus = compactText(data.finalGateStatus || "");
      this.snapshot.missingRequirements = Array.isArray(data.missingRequirements) ? data.missingRequirements : [];
      this.snapshot.turnPolicy = data.turnPolicy || null;
      this.snapshot.promptEnvelope = data.promptEnvelope || null;
      this.snapshot.toolInvocations = Array.isArray(data.toolInvocations) ? data.toolInvocations : [];
      this.snapshot.evidenceSummaries = Array.isArray(data.evidenceSummaries) ? data.evidenceSummaries : [];
      this.snapshot.config = data.config || this.snapshot.config;
      /* Merge runtime if server sends it */
      if (data.runtime) {
        this.runtime.turn = normalizeTurnRuntime(
          data.runtime.turn || {},
          this.snapshot.selectedHostId || "server-local",
          this.runtime.turn,
        );
        this.runtime.codex = {
          status: "connected",
          retryAttempt: this.runtime.codex.retryAttempt,
          retryMax: 5,
          lastError: "",
          ...(data.runtime.codex || {}),
        };
        this.runtime.activity = {
          filesViewed: 0,
          searchCount: 0,
          searchLocationCount: 0,
          listCount: 0,
          commandsRun: 0,
          filesChanged: 0,
          currentReadingFile: "",
          currentChangingFile: "",
          currentListingPath: "",
          currentSearchKind: "",
          currentSearchQuery: "",
          viewedFiles: [],
          currentWebSearchQuery: "",
          searchedWebQueries: [],
          searchedContentQueries: [],
          ...(data.runtime.activity || {}),
        };
      }
      const summary = deriveSessionSummary(this.snapshot, this.runtime);
      const index = this.sessionList.findIndex((session) => session.id === summary.id);
      if (index >= 0) {
        this.sessionList[index] = { ...this.sessionList[index], ...summary };
      }
      this.loading = false;
    },
    applyTurnEvent(event) {
      const normalized = normalizeTurnEvent(event);
      if (!normalized) {
        if (import.meta.env?.DEV || import.meta.env?.MODE === "test") {
          console.debug?.("ignored unknown turn event", event);
        }
        return false;
      }
      if (this.turnEventsById[normalized.eventId]) return false;

      this.turnEventsById = {
        ...this.turnEventsById,
        [normalized.eventId]: normalized,
      };

      const turnId = normalized.turnId;
      const existingItems = this.processItemsByTurnId[turnId] || [];
      const payloadText = compactText(normalized.payload.text || normalized.payload.delta || normalized.payload.content);

      if (normalized.type === "turn.started") {
        this.runtime.turn = normalizeTurnRuntime(
          {
            ...this.runtime.turn,
            active: true,
            pendingStart: false,
            phase: "thinking",
            startedAt: normalized.createdAt,
          },
          this.snapshot.selectedHostId || "server-local",
          this.runtime.turn,
        );
      } else if (normalized.type === "assistant.intent.delta") {
        this.assistantDraftsByTurnId = {
          ...this.assistantDraftsByTurnId,
          [turnId]: `${this.assistantDraftsByTurnId[turnId] || ""}${payloadText}`,
        };
      } else if (normalized.type === "assistant.final.delta") {
        const finalText = `${this.finalMessagesByTurnId[turnId] || ""}${payloadText}`;
        this.finalMessagesByTurnId = {
          ...this.finalMessagesByTurnId,
          [turnId]: finalText,
        };
        const finalCardId = `event-final-${turnId}`;
        const finalCard = {
          id: finalCardId,
          type: "AssistantMessageCard",
          role: "assistant",
          text: finalText,
          message: finalText,
          status: "streaming",
          createdAt: normalized.createdAt,
          updatedAt: normalized.createdAt,
        };
        const cardIndex = this.snapshot.cards.findIndex((card) => card?.id === finalCardId);
        if (cardIndex >= 0) {
          const nextCards = [...this.snapshot.cards];
          nextCards.splice(cardIndex, 1, {
            ...nextCards[cardIndex],
            ...finalCard,
          });
          this.snapshot.cards = nextCards;
        } else {
          this.snapshot.cards = [...this.snapshot.cards, finalCard];
        }
      } else if (normalized.type === "tool.call.start") {
        const item = processItemFromTurnEvent(normalized, "running");
        this.processItemsByTurnId = {
          ...this.processItemsByTurnId,
          [turnId]: upsertProcessItem(existingItems, item),
        };
        this.snapshot.toolInvocations = upsertProcessItem(this.snapshot.toolInvocations, toolInvocationFromTurnEvent(normalized, "running"));
      } else if (normalized.type === "tool.status.delta") {
        const item = processItemFromTurnEvent(normalized, compactText(normalized.payload.status || "running") || "running");
        this.processItemsByTurnId = {
          ...this.processItemsByTurnId,
          [turnId]: upsertProcessItem(existingItems, item),
        };
        this.snapshot.toolInvocations = upsertProcessItem(this.snapshot.toolInvocations, toolInvocationFromTurnEvent(normalized, item.status));
      } else if (normalized.type === "tool.result.done") {
        const item = processItemFromTurnEvent(normalized, "completed");
        this.processItemsByTurnId = {
          ...this.processItemsByTurnId,
          [turnId]: upsertProcessItem(existingItems, item),
        };
        this.snapshot.toolInvocations = upsertProcessItem(this.snapshot.toolInvocations, toolInvocationFromTurnEvent(normalized, "completed"));
      } else if (normalized.type === "tool.result.error") {
        const item = processItemFromTurnEvent(normalized, "failed");
        this.processItemsByTurnId = {
          ...this.processItemsByTurnId,
          [turnId]: upsertProcessItem(existingItems, item),
        };
        this.snapshot.toolInvocations = upsertProcessItem(this.snapshot.toolInvocations, toolInvocationFromTurnEvent(normalized, "failed"));
      } else if (normalized.type === "phase.end" || normalized.type === "process.summary") {
        const phaseId = compactText(normalized.payload.phaseId || `${turnId}:phase:${normalized.seq || normalized.eventId}`);
        this.phaseFoldsByTurnId = {
          ...this.phaseFoldsByTurnId,
          [turnId]: upsertProcessItem(this.phaseFoldsByTurnId[turnId] || [], {
            id: phaseId,
            summary: compactText(normalized.payload.summary || normalized.payload.text),
            status: "completed",
            updatedAt: normalized.createdAt,
          }),
        };
      } else if (normalized.type === "turn.done" || normalized.type === "turn.error" || normalized.type === "turn.aborted") {
        const phase = normalized.type === "turn.done" ? "completed" : normalized.type === "turn.aborted" ? "aborted" : "failed";
        this.runtime.turn = normalizeTurnRuntime(
          {
            ...this.runtime.turn,
            active: false,
            pendingStart: false,
            phase,
          },
          this.snapshot.selectedHostId || "server-local",
          this.runtime.turn,
        );
        if (normalized.type === "turn.error") {
          this.errorMessage = compactText(normalized.payload.error || normalized.payload.message || "Turn failed");
        }
      }
      return true;
    },
    applySessions(data) {
      this.activeSessionId = data.activeSessionId || this.activeSessionId || this.snapshot.sessionId;
      this.sessionList = (data.sessions || []).map((session) => normalizeSessionSummary(session));
    },
    rememberWorkspaceReturnTarget(sessionId, workspaceSessionId) {
      const targetSessionId = String(sessionId || this.activeSessionId || "").trim();
      const sourceWorkspaceSessionId = String(workspaceSessionId || "").trim();
      if (!targetSessionId || !sourceWorkspaceSessionId) {
        return false;
      }
      if (this.workspaceReturnTargets[targetSessionId] === sourceWorkspaceSessionId) {
        return true;
      }
      this.workspaceReturnTargets = normalizeWorkspaceReturnTargets({
        ...this.workspaceReturnTargets,
        [targetSessionId]: sourceWorkspaceSessionId,
      });
      return true;
    },
    clearWorkspaceReturnTarget(sessionId = this.activeSessionId) {
      const targetSessionId = String(sessionId || "").trim();
      if (!targetSessionId || !this.workspaceReturnTargets[targetSessionId]) {
        return false;
      }
      const nextTargets = { ...this.workspaceReturnTargets };
      delete nextTargets[targetSessionId];
      this.workspaceReturnTargets = normalizeWorkspaceReturnTargets(nextTargets);
      return true;
    },
    async returnToWorkspaceSession() {
      const workspaceSession = this.workspaceReturnSession;
      if (!workspaceSession?.id) {
        this.errorMessage = "没有可返回的工作台会话";
        return false;
      }
      if (this.runtime.turn.active) {
        this.errorMessage = "当前任务执行中，完成后再返回工作台";
        return false;
      }
      return this.activateSession(workspaceSession.id);
    },
    setTurnPhase(phase) {
      this.runtime.turn = normalizeTurnRuntime(
        {
          ...this.runtime.turn,
          phase,
          active: !isTerminalTurnPhase(phase),
        },
        this.snapshot.selectedHostId || "server-local",
        this.runtime.turn,
      );
    },
    markTurnPendingStart(phase = "thinking") {
      this.startLocalPendingTurn({ phase });
    },
    startLocalPendingTurn(payload = {}) {
      const source = asObject(payload);
      this.runtime.turn = normalizeTurnRuntime(
        {
          ...this.runtime.turn,
          phase: compactText(source.phase || "thinking") || "thinking",
          active: false,
          pendingStart: true,
          startedAt: source.startedAt || new Date().toISOString(),
          clientTurnId: compactText(source.clientTurnId || this.runtime.turn.clientTurnId),
          clientMessageId: compactText(source.clientMessageId || this.runtime.turn.clientMessageId),
          hostId: compactText(source.hostId || this.snapshot.selectedHostId || this.runtime.turn.hostId || "server-local"),
        },
        this.snapshot.selectedHostId || "server-local",
        this.runtime.turn,
      );
    },
    appendOptimisticUserMessage(message) {
      const card = buildOptimisticUserMessageCard(message);
      if (!card) return "";
      this.snapshot.cards = [...this.snapshot.cards, card];
      this.optimisticCardIds = [...this.optimisticCardIds, card.id];
      return card.id;
    },
    removeOptimisticCard(cardId) {
      const targetCardId = compactText(cardId);
      if (!targetCardId) return false;
      const nextCards = this.snapshot.cards.filter((card) => card?.id !== targetCardId);
      const nextOptimisticCardIds = this.optimisticCardIds.filter((id) => id !== targetCardId);
      const changed =
        nextCards.length !== this.snapshot.cards.length || nextOptimisticCardIds.length !== this.optimisticCardIds.length;
      this.snapshot.cards = nextCards;
      this.optimisticCardIds = nextOptimisticCardIds;
      return changed;
    },
    patchOptimisticCard(cardId, patch = {}) {
      const targetCardId = compactText(cardId);
      if (!targetCardId) return false;
      const index = this.snapshot.cards.findIndex((card) => card?.id === targetCardId);
      if (index < 0) return false;
      const currentCard = this.snapshot.cards[index];
      const nextCard = {
        ...currentCard,
        ...asObject(patch),
        updatedAt: new Date().toISOString(),
      };
      const nextCards = [...this.snapshot.cards];
      nextCards.splice(index, 1, nextCard);
      this.snapshot.cards = nextCards;
      return true;
    },
    clearTurnPendingStart() {
      this.runtime.turn = normalizeTurnRuntime(
        {
          ...this.runtime.turn,
          pendingStart: false,
        },
        this.snapshot.selectedHostId || "server-local",
        this.runtime.turn,
      );
    },
    resetActivity() {
      this.runtime.activity = {
        filesViewed: 0,
        searchCount: 0,
        searchLocationCount: 0,
        listCount: 0,
        commandsRun: 0,
        filesChanged: 0,
        currentReadingFile: "",
        currentChangingFile: "",
        currentListingPath: "",
        currentSearchKind: "",
        currentSearchQuery: "",
        viewedFiles: [],
        currentWebSearchQuery: "",
        searchedWebQueries: [],
        searchedContentQueries: [],
      };
    },
    async fetchState() {
      const fixture = resolveUiFixtureRuntime();
      if (fixture?.state) {
        this.applySnapshot(cloneUiFixturePayload(fixture.state));
        return true;
      }
      try {
        const data = await fetchStateApi();
        this.applySnapshot(data);
        return true;
      } catch (e) {
        console.error("Failed to fetch state:", e);
        return false;
      }
    },
    async fetchSessions() {
      const fixture = resolveUiFixtureRuntime();
      if (fixture?.sessions) {
        this.applySessions(cloneUiFixturePayload(fixture.sessions));
        return true;
      }
      this.historyLoading = true;
      try {
        const data = await fetchSessionsApi();
        this.applySessions(data);
        return true;
      } catch (e) {
        console.error("Failed to fetch sessions:", e);
        this.errorMessage = "Load sessions failed";
        return false;
      } finally {
        this.historyLoading = false;
      }
    },
    async fetchSettings() {
      try {
        const data = await fetchSettingsApi();
        this.settings = { ...this.settings, ...data };
      } catch (e) {
        console.error("Failed to fetch settings:", e);
      }
    },
    async updateSettings(newSettings) {
      try {
        const data = await updateSettingsApi(newSettings);
        this.settings = { ...this.settings, ...data };
      } catch (e) {
        console.error("Failed to update settings:", e);
        this.settings = { ...this.settings, ...newSettings }; // Mock fallback
      }
    },
    async fetchSkillCatalog() {
      try {
        const data = await fetchSkillCatalogApi();
        this.skillCatalog = normalizeSkillCatalogEntries(data?.items || data?.skills || []);
        realignAgentProfiles(this);
        return this.skillCatalog;
      } catch (e) {
        console.error("Failed to fetch skill catalog:", e);
        this.skillCatalog = createSkillCatalog();
        realignAgentProfiles(this);
        return this.skillCatalog;
      }
    },
    async saveSkillCatalogItem(item) {
      const normalized = normalizeSkillCatalogEntry(item || {});
      const targetId = String(normalized.id || "");
      if (!targetId) {
        throw new Error("skill id is required");
      }
      const data = await saveSkillCatalogItemApi(normalized);
      this.skillCatalog = normalizeSkillCatalogEntries(data?.items || []);
      await this.fetchAgentProfiles();
      return data?.item || normalized;
    },
    async deleteSkillCatalogItem(skillId) {
      const targetId = String(skillId || "").trim();
      if (!targetId) return false;
      const data = await deleteSkillCatalogItemApi(targetId);
      this.skillCatalog = normalizeSkillCatalogEntries(data?.items || []);
      await this.fetchAgentProfiles();
      return true;
    },
    async fetchMcpCatalog() {
      try {
        const data = await fetchMcpCatalogApi();
        this.mcpCatalog = normalizeMcpCatalogEntries(data?.items || data?.mcps || []);
        realignAgentProfiles(this);
        return this.mcpCatalog;
      } catch (e) {
        console.error("Failed to fetch mcp catalog:", e);
        this.mcpCatalog = createMcpCatalog();
        realignAgentProfiles(this);
        return this.mcpCatalog;
      }
    },
    async saveMcpCatalogItem(item) {
      const normalized = normalizeMcpCatalogEntry(item || {});
      const targetId = String(normalized.id || "");
      if (!targetId) {
        throw new Error("mcp id is required");
      }
      const data = await saveMcpCatalogItemApi(normalized);
      this.mcpCatalog = normalizeMcpCatalogEntries(data?.items || []);
      await this.fetchAgentProfiles();
      return data?.item || normalized;
    },
    async deleteMcpCatalogItem(mcpId) {
      const targetId = String(mcpId || "").trim();
      if (!targetId) return false;
      const data = await deleteMcpCatalogItemApi(targetId);
      this.mcpCatalog = normalizeMcpCatalogEntries(data?.items || []);
      await this.fetchAgentProfiles();
      return true;
    },
    async fetchAgentProfiles() {
      this.agentProfilesLoading = true;
      clearAgentProfileErrorState(this);
      this.agentProfilePreview = null;
      try {
        const defaults = createDefaultAgentProfiles();
        const remoteProfiles = [];

        const tryLoad = async (loader) => {
          try {
            return await loader();
          } catch (e) {
            console.error("Failed to fetch agent profile payload:", e);
            return null;
          }
        };

        const listPayload = await tryLoad(fetchAgentProfilesApi);
        const singlePayload = listPayload ? null : await tryLoad(fetchSingleAgentProfileApi);
        const remoteSkillCatalog = Array.isArray(listPayload?.skillCatalog) ? normalizeSkillCatalogEntries(listPayload.skillCatalog) : null;
        const remoteMcpCatalog = Array.isArray(listPayload?.mcpCatalog) ? normalizeMcpCatalogEntries(listPayload.mcpCatalog) : null;
        if (remoteSkillCatalog) {
          this.skillCatalog = remoteSkillCatalog;
        }
        if (remoteMcpCatalog) {
          this.mcpCatalog = remoteMcpCatalog;
        }
        const catalogs = {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        };

        const ingestPayload = (payload) => {
          if (!payload) return;
          if (Array.isArray(payload.items)) {
            remoteProfiles.push(...payload.items);
            return;
          }
          if (Array.isArray(payload.profiles)) {
            remoteProfiles.push(...payload.profiles);
            return;
          }
          if (Array.isArray(payload)) {
            remoteProfiles.push(...payload);
            return;
          }
          if (typeof payload === "object") {
            remoteProfiles.push(payload);
          }
        };

        ingestPayload(listPayload);
        ingestPayload(singlePayload);

        if (remoteProfiles.length) {
          const overrides = new Map();
          for (const profile of remoteProfiles) {
            const key = String(profile?.id || profile?.type || "");
            if (key) {
              overrides.set(key, profile);
            }
          }
          const mergedProfiles = defaults.map((fallbackProfile) => {
            return normalizeAgentProfile(
              overrides.get(fallbackProfile.id) || overrides.get(fallbackProfile.type) || null,
              fallbackProfile,
              catalogs,
            );
          });
          for (const profile of remoteProfiles) {
            const key = String(profile?.id || profile?.type || "");
            if (!key) continue;
            if (mergedProfiles.some((item) => item.id === key || item.type === profile?.type)) continue;
            mergedProfiles.push(normalizeAgentProfile(profile, null, catalogs));
          }
          this.agentProfiles = mergedProfiles;
        } else {
          this.agentProfiles = defaults.map((profile) => alignAgentProfileCollections(profile, catalogs.skillCatalog, catalogs.mcpCatalog));
        }
        realignAgentProfiles(this);

        if (!this.agentProfiles.some((profile) => profile.id === this.activeAgentProfileId)) {
          this.activeAgentProfileId = this.agentProfiles[0]?.id || "main-agent";
        }

        return true;
      } finally {
        this.agentProfilesLoading = false;
      }
    },
    selectAgentProfile(profileId) {
      const nextId = String(profileId || "");
      if (!nextId) return false;
      const exists = this.agentProfiles.some((profile) => profile.id === nextId);
      if (!exists) return false;
      this.activeAgentProfileId = nextId;
      this.agentProfilePreview = null;
      this.agentProfileFieldErrors = {};
      return true;
    },
    resetAgentProfiles() {
      this.agentProfiles = createDefaultAgentProfiles().map((profile) => alignAgentProfileCollections(profile, this.skillCatalog, this.mcpCatalog));
      this.activeAgentProfileId = "main-agent";
      clearAgentProfileErrorState(this);
      this.agentProfilePreview = null;
      return true;
    },
    async exportAgentProfiles() {
      let payload = null;
      try {
        const data = await exportAgentProfilesApi();
        const exportedProfiles = normalizeImportedAgentProfiles(parseAgentProfilesImportPayload(data), {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
        payload = buildAgentProfilesExportPayload(exportedProfiles, {
          configVersion: Number(data?.configVersion || data?.version || 1),
          exportedAt: data?.exportedAt,
          exportedBy: data?.exportedBy,
        }, {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
      } catch (e) {
        console.error("Failed to export agent profiles from server, using local snapshot:", e);
        payload = buildAgentProfilesExportPayload(this.agentProfiles, {}, {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
      }
      return {
        filename: `agent-profiles-${payload.exportedAt.replace(/[:.]/g, "-")}.json`,
        content: JSON.stringify(payload, null, 2),
        payload,
      };
    },
    async importAgentProfiles(source) {
      this.agentProfilesLoading = true;
      clearAgentProfileErrorState(this);
      try {
        const text = await readAgentProfilesImportSource(source);
        const payload = JSON.parse(text);
        const data = await importAgentProfilesApi(payload);
        const importedProfiles = parseAgentProfilesImportPayload(data);
        const mergedProfiles = normalizeImportedAgentProfiles(importedProfiles, {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
        if (!mergedProfiles.length) {
          throw new Error("服务端未返回可导入的 profiles");
        }
        this.agentProfiles = mergedProfiles;
        if (!this.agentProfiles.some((profile) => profile.id === this.activeAgentProfileId)) {
          this.activeAgentProfileId = this.agentProfiles[0]?.id || "main-agent";
        }
        await this.fetchAgentProfilePreview(this.activeAgentProfileId);
        clearAgentProfileErrorState(this);
        return {
          ok: true,
          count: mergedProfiles.length,
          activeProfileId: this.activeAgentProfileId,
        };
      } catch (e) {
        console.error("Failed to import agent profiles:", e);
        applyAgentProfileErrorState(this, "导入 Agent Profiles 失败", { general: String(e?.message || e || "invalid JSON") });
        return {
          ok: false,
          error: String(e?.message || e || "invalid JSON"),
        };
      } finally {
        this.agentProfilesLoading = false;
      }
    },
    async saveAgentProfile(profile, options = {}) {
      this.agentProfileSaving = true;
      clearAgentProfileErrorState(this);
      try {
        const data = await saveAgentProfileApi(serializeAgentProfile(profile, {
          ...options,
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        }));
        const fallback = createDefaultAgentProfiles().find((item) => item.id === data.id) || null;
        const normalized = normalizeAgentProfile(data, fallback, {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
        const index = this.agentProfiles.findIndex((item) => item.id === normalized.id);
        if (index >= 0) {
          this.agentProfiles[index] = normalized;
        } else {
          this.agentProfiles.push(normalized);
        }
        this.activeAgentProfileId = normalized.id;
        clearAgentProfileErrorState(this);
        await this.fetchAgentProfilePreview(normalized.id);
        return true;
      } catch (e) {
        console.error("Failed to save agent profile:", e);
        applyAgentProfileErrorState(this, "保存 Agent Profile 失败", {});
        return false;
      } finally {
        this.agentProfileSaving = false;
      }
    },
    async resetAgentProfile(profileId) {
      const nextId = String(profileId || this.activeAgentProfileId || "main-agent");
      this.agentProfileSaving = true;
      clearAgentProfileErrorState(this);
      try {
        const data = await resetAgentProfileApi(nextId);
        const fallback = createDefaultAgentProfiles().find((item) => item.id === nextId) || null;
        const normalized = normalizeAgentProfile(data, fallback, {
          skillCatalog: this.skillCatalog,
          mcpCatalog: this.mcpCatalog,
        });
        const index = this.agentProfiles.findIndex((item) => item.id === normalized.id);
        if (index >= 0) {
          this.agentProfiles[index] = normalized;
        } else {
          this.agentProfiles.push(normalized);
        }
        this.activeAgentProfileId = normalized.id;
        clearAgentProfileErrorState(this);
        await this.fetchAgentProfilePreview(normalized.id);
        return true;
      } catch (e) {
        console.error("Failed to reset agent profile:", e);
        applyAgentProfileErrorState(this, "重置 Agent Profile 失败", {});
        return false;
      } finally {
        this.agentProfileSaving = false;
      }
    },
    async fetchAgentProfilePreview(profileId) {
      const nextId = String(profileId || this.activeAgentProfileId || "main-agent");
      this.agentProfilePreviewLoading = true;
      this.agentProfileFieldErrors = {};
      try {
        const data = await fetchAgentProfilePreviewApi(nextId);
        this.agentProfilePreview = data;
        return data;
      } catch (e) {
        console.error("Failed to fetch agent profile preview:", e);
        this.agentProfilesError = "加载预览失败";
        return null;
      } finally {
        this.agentProfilePreviewLoading = false;
      }
    },
    async resetThread() {
      if (this.runtime.turn.active) {
        this.noticeMessage = "";
        this.errorMessage = "当前任务执行中，完成后再清空上下文";
        return false;
      }
      try {
        await resetThreadApi();
        this.errorMessage = "";
        this.resetActivity();
        this.setTurnPhase("idle");
        await this.fetchState();
        await this.fetchSessions();
        return true;
      } catch (e) {
        console.error("Failed to reset thread:", e);
        this.noticeMessage = "";
        this.errorMessage = "清空当前上下文失败";
        return false;
      }
    },
    clearSocketTimers() {
      if (this._snapshotThrottleTimer) {
        clearTimeout(this._snapshotThrottleTimer);
        this._snapshotThrottleTimer = null;
      }
      this._pendingSnapshotData = null;
    },
    disconnectWs() {
      const client = this._appSocketClient;
      this._appSocketClient = null;
      this.clearSocketTimers();
      if (client) {
        try {
          client.disconnect();
        } catch (e) {
          console.error("Failed to close websocket:", e);
        }
      }
    },
    reconnectWs() {
      this.disconnectWs();
      this.runtime.codex.retryAttempt = 0;
      this.connectWs();
    },
    async createSession(kind = "single_host") {
      if (this.runtime.turn.active) {
        this.errorMessage = "当前任务执行中，完成后再新建会话";
        return false;
      }
      try {
        const data = await createSessionApi(kind);
        this.errorMessage = "";
        this.applySessions(data);
        if (data.snapshot) {
          this.applySnapshot(data.snapshot);
        } else {
          await this.fetchState();
        }
        this.resetActivity();
        this.setTurnPhase("idle");
        this.reconnectWs();
        return true;
      } catch (e) {
        console.error("Failed to create session:", e);
        this.errorMessage = "Create session failed";
        return false;
      }
    },
    async activateSession(sessionId) {
      if (!sessionId || sessionId === this.activeSessionId) {
        return true;
      }
      if (this.runtime.turn.active) {
        this.errorMessage = "当前任务执行中，完成后再切换会话";
        return false;
      }
      try {
        const data = await activateSessionApi(sessionId);
        this.errorMessage = "";
        this.applySessions(data);
        if (data.snapshot) {
          this.applySnapshot(data.snapshot);
        } else {
          await this.fetchState();
        }
        this.reconnectWs();
        return true;
      } catch (e) {
        console.error("Failed to activate session:", e);
        this.errorMessage = "Switch session failed";
        return false;
      }
    },
    async createOrActivateSingleHostSessionForHost(hostId, hostMeta = {}) {
      const targetHostId = String(hostId || hostMeta.hostId || hostMeta.id || hostMeta.name || hostMeta.address || "").trim();
      if (!targetHostId) {
        this.errorMessage = "hostId is required";
        return false;
      }
      const sourceWorkspaceSessionId = this.snapshot.kind === "workspace" ? this.snapshot.sessionId : "";
      if (this.runtime.turn.active) {
        this.errorMessage = "当前任务执行中，完成后再切换会话";
        return false;
      }

      if (!this.sessionList.length && !this.historyLoading) {
        await this.fetchSessions();
      }

      const existingSingleHostSession = this.sessionList.find(
        (session) => session.kind === "single_host" && session.selectedHostId === targetHostId,
      );

      if (existingSingleHostSession?.id && existingSingleHostSession.id !== this.activeSessionId) {
        const activated = await this.activateSession(existingSingleHostSession.id);
        if (!activated) {
          return false;
        }
      } else if (this.snapshot.kind !== "single_host") {
        const created = await this.createSession("single_host");
        if (!created) {
          return false;
        }
      }

      if (this.snapshot.selectedHostId !== targetHostId) {
        const selected = await this.selectHost(targetHostId);
        if (!selected) {
          return false;
        }
      }

      if (sourceWorkspaceSessionId) {
        this.rememberWorkspaceReturnTarget(this.activeSessionId, sourceWorkspaceSessionId);
      }

      return true;
    },
    connectWs() {
      this.disconnectWs();
      const fixture = resolveUiFixtureRuntime();
      if (fixture) {
        this.wsStatus = "connected";
        this.runtime.codex.status = "connected";
        this.runtime.codex.lastError = "";
        return;
      }
      this._appSocketClient = createAppSocketClient({
        shouldRestoreState: () => !this.snapshot.sessionId,
        maxRetries: this.runtime.codex.retryMax,
        onStatusChange: (status, meta = {}) => {
          this.wsStatus = status;
          if (status === "connecting" || status === "reconnecting") {
            this.runtime.codex.status = "reconnecting";
            this.runtime.codex.retryAttempt = meta.retryCount || 0;
            return;
          }
          if (status === "connected") {
            this.runtime.codex.status = "connected";
            this.runtime.codex.retryAttempt = 0;
            this.runtime.codex.lastError = "";
          }
        },
        onSnapshot: (data) => {
          const turnPhase = String(data?.runtime?.turn?.phase || "").trim().toLowerCase();
          const isNonStreaming = ["completed", "failed", "aborted", "waiting_approval", "idle"].includes(turnPhase);
          if (isNonStreaming) {
            if (this._snapshotThrottleTimer) {
              clearTimeout(this._snapshotThrottleTimer);
              this._snapshotThrottleTimer = null;
            }
            this.applySnapshot(data);
            return;
          }
          this._pendingSnapshotData = data;
          if (!this._snapshotThrottleTimer) {
            this._snapshotThrottleTimer = setTimeout(() => {
              this._snapshotThrottleTimer = null;
              if (this._pendingSnapshotData) {
                this.applySnapshot(this._pendingSnapshotData);
                this._pendingSnapshotData = null;
              }
            }, 48);
          }
        },
        onTurnEvent: (event) => {
          this.applyTurnEvent(event);
        },
        onError: () => {
          this.wsStatus = "error";
          this.runtime.codex.lastError = "connection error";
        },
        onRestoreRequired: () => {
          void Promise.all([this.fetchState(), this.fetchSessions()]).finally(() => {
            if (isConnectionLossMessage(this.errorMessage)) {
              this.errorMessage = "";
            }
          });
        },
        onStopped: ({ retryCount }) => {
          this.runtime.codex.retryAttempt = retryCount || this.runtime.codex.retryMax;
          this.runtime.codex.status = "stopped";
          this.wsStatus = "error";
          if (!this.runtime.codex.lastError) {
            this.runtime.codex.lastError = "connection closed";
          }
          if (this.runtime.turn.active) {
            this.setTurnPhase("failed");
          }
          this.errorMessage = `与 ai-server 的连接已断开，${formatHostStatus(this.selectedHost)}。请刷新页面或稍后重试。`;
        },
      });
      this._appSocketClient.connect();
    },
    async selectHost(hostId, options = {}) {
      const targetHostId = hostId || "server-local";
      const shouldApply = typeof options?.shouldApply === "function" ? options.shouldApply : null;
      if (targetHostId === this.snapshot.selectedHostId) {
        return true;
      }
      if (this.runtime.turn.active) {
        this.errorMessage = "当前任务执行中，完成后再切换主机";
        return false;
      }
      try {
        const data = await selectHostApi(targetHostId);
        if (shouldApply && shouldApply({ hostId: targetHostId, data, phase: "success" }) === false) {
          return false;
        }
        this.errorMessage = "";
        this.applySnapshot(data.snapshot || data);
        return true;
      } catch (e) {
        if (shouldApply && shouldApply({ hostId: targetHostId, error: e, phase: "error" }) === false) {
          return false;
        }
        console.error("Failed to switch host:", e);
        this.errorMessage = "Switch host failed";
        return false;
      }
    },
  },
});
