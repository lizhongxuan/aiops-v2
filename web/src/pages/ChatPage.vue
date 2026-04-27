<script setup>
import { computed, defineAsyncComponent, ref, watch, nextTick, onMounted, onBeforeUnmount, shallowRef } from "vue";
import { useAppStore } from "../store";
import { resolveHostDisplay } from "../lib/hostDisplay";
import { formatMainChatTurns, isChatConversationCard, shouldExposeActiveFinalMessage } from "../lib/chatTurnFormatter";
import {
  selectActiveProjection,
  selectAgentRows,
  selectApprovalDock,
  selectProjectionActivityLines,
  selectProjectionStartedAt,
  selectFinalMessages,
  selectRuntimeBusy,
  selectRuntimeStatus,
  selectTimelineRows,
} from "../events/agentEventProjection";
import { useChatScrollState } from "../composables/useChatScrollState";
import { useVirtualTurnList } from "../composables/useVirtualTurnList";
import { sendMessage as sendMessageApi, stopMessage as stopMessageApi } from "../api/chat";
import { submitApprovalDecision as submitApprovalDecisionApi, submitChoiceAnswer as submitChoiceAnswerApi } from "../api/approvals";
import { buildMcpDecisionNotice, buildSyntheticMcpApproval, formatMcpActionLabel, formatMcpActionTarget, isMcpMutationAction } from "../lib/mcpActionRuntime";
import CardItem from "../components/CardItem.vue";
import ChatStream from "../components/chat/ChatStream.vue";
import ChatComposerDock from "../components/chat/ChatComposerDock.vue";
import McpBundleHost from "../components/mcp/McpBundleHost.vue";
import McpUiCardHost from "../components/mcp/McpUiCardHost.vue";
import { WifiOffIcon, RefreshCwIcon, TerminalIcon, ChevronDownIcon, ChevronRightIcon } from "lucide-vue-next";
import { NBadge, NDrawer, NDrawerContent } from "naive-ui";

const store = useAppStore();
const WorkspaceHostTerminal = defineAsyncComponent(() => import("../components/workspace/WorkspaceHostTerminal.vue"));
const OPEN_MCP_DRAWER_EVENT = "codex:open-mcp-drawer";

const composerMessage = ref("");
const chatStreamRef = ref(null);
const composerDockRef = ref(null);
const scrollContainer = computed(() => chatStreamRef.value?.scrollContainerEl || null);
const scrollContent = computed(() => chatStreamRef.value?.scrollContentEl || null);
const showFileDetails = ref(false);
const showSearchDetails = ref(false);
const authCardCollapsed = ref(false);
const approvalFollowupMode = ref(false);
const localMcpApprovals = ref([]);
const activeMcpSurface = ref(null);
const mcpPinnedSurfaces = ref([]);
const isMcpDrawerOpen = ref(false);
const assistantFeedbackByMessageId = ref({});
const stopBusy = ref(false);
const terminalDockVisible = ref(false);
const terminalDockHeight = ref(320);
const terminalDockSessionLive = ref(false);
const terminalDockRef = ref(null);
const terminalDockDragging = ref(false);
let terminalDockDragState = null;
let terminalDockMaxHeight = 560;

/* ---- ThinkingCard local state ---- */
const showThinking = ref(false);
const thinkingPhase = ref("thinking");
const thinkingHint = ref("");
const preferredThinkingPhase = ref("");
let thinkingHintTimer = null;

const thinkingDisplayPhase = computed(() => {
  const phase = compactText(thinkingPhase.value).toLowerCase();
  if (["planning", "thinking", "waiting_approval", "waiting_input", "executing", "finalizing"].includes(phase)) {
    return phase;
  }
  return "thinking";
});

const thinkingCard = computed(() => ({
  id: "__thinking__",
  type: "ThinkingCard",
  phase: thinkingDisplayPhase.value,
  hint: activeActivityLine.value || summaryLine.value || thinkingHint.value,
}));
const showThinkingCard = computed(() => showThinking.value);

function clearThinkingPrelude() {
  if (thinkingHintTimer) {
    window.clearTimeout(thinkingHintTimer);
    thinkingHintTimer = null;
  }
  thinkingHint.value = "";
  preferredThinkingPhase.value = "";
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function createClientId(prefix) {
  const randomPart =
    typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  return `${prefix}-${randomPart}`;
}

function normalizeCompletedActivityText(text = "") {
  return compactText(String(text || ""))
    .replace(/^正在搜索网页/u, "已搜索网页")
    .replace(/^正在搜索内容/u, "已搜索内容")
    .replace(/^正在搜索文件/u, "已搜索文件")
    .replace(/^正在浏览网页/u, "已浏览网页")
    .replace(/^正在浏览目录/u, "已浏览目录")
    .replace(/^正在浏览/u, "已浏览")
    .replace(/^正在读取文件/u, "已读取文件")
    .replace(/^正在修改文件/u, "已修改文件")
    .replace(/^正在列出/u, "已列出")
    .replace(/^正在检索页面内容/u, "已在页面中搜索")
    .replace(/^正在运行/u, "已运行")
    .replace(/^正在执行/u, "已执行");
}

function buildPersistedProcessItems(lines = [], { completed = false } = {}) {
  const seenText = new Set();
  return (lines || []).map((line, index) => {
    const text = completed ? normalizeCompletedActivityText(line?.text) : compactText(line?.text);
    if (!text || seenText.has(text)) return null;
    seenText.add(text);
    return {
      id: compactText(line?.id || `activity-line-${index}`),
      kind: compactText(line?.kind) || (text.startsWith("已搜索") ? "search" : "activity"),
      text,
      status: completed ? "completed" : compactText(line?.status),
    };
  }).filter(Boolean);
}

function buildTerminalStatusItem(phase = "", message = "") {
  const normalized = compactText(phase).toLowerCase();
  if (normalized === "failed") {
    const detail = compactText(message);
    return {
      id: "terminal-status-failed",
      kind: "status",
      text: detail ? `请求失败：${detail}` : "请求失败，请重试",
      detail,
      status: "failed",
    };
  }
  if (normalized === "aborted") {
    return {
      id: "terminal-status-aborted",
      kind: "status",
      text: "已停止生成",
      status: "aborted",
    };
  }
  return null;
}

function buildTerminalStatusCard(phase = "", message = "", elapsedLabel = "") {
  const normalized = compactText(phase).toLowerCase();
  if (normalized === "failed") {
    return {
      state: "failed",
      message: compactText(message) || "请求失败，请稍后重试。",
      elapsedLabel: compactText(elapsedLabel) || "0s",
      retryable: true,
    };
  }
  if (normalized === "aborted") {
    return {
      state: "aborted",
      message: "已停止生成，保留当前结果。",
      elapsedLabel: compactText(elapsedLabel) || "0s",
    };
  }
  return null;
}

function focusComposer() {
  nextTick(() => {
    composerDockRef.value?.focus?.();
  });
}

function isMcpBundlePayload(value) {
  const source = value && typeof value === "object" ? value : {};
  const bundleKind = compactText(source.bundleKind || source.bundle_kind).toLowerCase();
  return Boolean(
    bundleKind ||
      source.bundleId ||
      source.bundle_id ||
      Array.isArray(source.sections) ||
      Array.isArray(source.sectionConfig),
  );
}

function normalizeMcpSurfacePayload(payload = {}) {
  const source = payload && typeof payload === "object" ? payload : {};
  const kind = isMcpBundlePayload(source)
    ? "bundle"
    : compactText(source.kind || source.uiKind || source.ui_kind).toLowerCase() === "bundle"
      ? "bundle"
      : "card";
  const bundleId = compactText(source.bundleId || source.bundle_id || source.id || source.key || "");
  const cardId = compactText(source.cardId || source.card_id || source.id || source.key || "");
  const surfaceId = kind === "bundle" ? bundleId || cardId : cardId || bundleId;
  const title =
    compactText(source.title || source.summary || source.label || source.name || source.rootCause || source.root_cause) ||
    (kind === "bundle" ? "MCP 聚合面板" : "MCP 卡片");
  const subtitle = compactText(
    source.subject?.name ||
      source.subject?.service ||
      source.subject?.type ||
      source.scope?.service ||
      source.scope?.hostId ||
      source.scope?.resourceId ||
      source.mcpServer ||
      source.source ||
      "",
  );
  const sourceTag = compactText(source.source || source.mcpServer || source.sourceCardId || source.source_card_id || "");

  return {
    kind,
    id: surfaceId || `${kind}-${title}`,
    title,
    subtitle,
    source: sourceTag,
    bundle: kind === "bundle" ? source : null,
    card: kind === "card" ? source : null,
    raw: source,
  };
}

function mcpSurfaceKey(surface) {
  const record = surface || {};
  return `${record.kind || "card"}:${record.id || record.title || record.subtitle || "surface"}`;
}

function dispatchOpenMcpDrawer(surface, pin = false) {
  if (typeof window === "undefined" || !surface) return;
  window.dispatchEvent(
    new CustomEvent(OPEN_MCP_DRAWER_EVENT, {
      detail: {
        source: "chat-mcp-surface",
        pin,
        surface: {
          kind: surface.kind || "card",
          bundle: surface.kind === "bundle" ? surface.bundle : undefined,
          card: surface.kind === "card" ? surface.card : undefined,
          source: surface.source || "",
          title: surface.title || "",
          subtitle: surface.subtitle || "",
          id: surface.id || "",
        },
      },
    }),
  );
}

function openMcpSurfaceDrawer(payload, { pin = false, silent = false } = {}) {
  const surface = normalizeMcpSurfacePayload(payload);
  activeMcpSurface.value = surface;
  isMcpDrawerOpen.value = true;
  if (pin) {
    const key = mcpSurfaceKey(surface);
    mcpPinnedSurfaces.value = [
      ...mcpPinnedSurfaces.value.filter((item) => mcpSurfaceKey(item) !== key),
      surface,
    ];
  }
  if (!silent) {
    store.noticeMessage = `${surface.title} 已打开完整面板。`;
  }
  dispatchOpenMcpDrawer(surface, pin);
  return surface;
}

async function refreshMcpSurface(payload) {
  const surface = openMcpSurfaceDrawer(payload, { silent: true });
  await store.fetchState();
  store.noticeMessage = `${surface.title} 已刷新。`;
  return surface;
}

function pinMcpSurface(payload) {
  const surface = openMcpSurfaceDrawer(payload, { pin: true });
  store.noticeMessage = `${surface.title} 已固定到 MCP 面板。`;
  return surface;
}

function closeMcpSurfaceDrawer() {
  isMcpDrawerOpen.value = false;
}

const activeProjectionSessionId = computed(() => store.activeSessionId || store.snapshot.sessionId || "");
const activeAgentProjection = computed(() =>
  selectActiveProjection(store.agentEventState, activeProjectionSessionId.value),
);
const hasProjectionRuntime = computed(() => Boolean(activeAgentProjection.value?.sessionId));
const projectionRuntimeStatus = computed(() =>
  selectRuntimeStatus(store.agentEventState, activeProjectionSessionId.value),
);
const projectionRuntimeBusy = computed(() =>
  selectRuntimeBusy(store.agentEventState, activeProjectionSessionId.value),
);
const projectionAgentRows = computed(() =>
  selectAgentRows(store.agentEventState, activeProjectionSessionId.value),
);
const projectionApprovalDock = computed(() =>
  selectApprovalDock(store.agentEventState, activeProjectionSessionId.value),
);
const projectionArtifactRows = computed(() => activeAgentProjection.value?.artifacts || []);
const projectionDiffSummary = computed(() => activeAgentProjection.value?.diff || null);
const projectionStartedAt = computed(() =>
  selectProjectionStartedAt(store.agentEventState, activeProjectionSessionId.value),
);
const projectionActivityLines = computed(() =>
  selectProjectionActivityLines(store.agentEventState, activeProjectionSessionId.value),
);
const projectionTimelineRows = computed(() => {
  const timeline = selectTimelineRows(store.agentEventState, activeProjectionSessionId.value).map((row) => ({
    ...row,
    _sortSeq: Number(row?.seq || 0),
  }));
  const finals = selectFinalMessages(store.agentEventState, activeProjectionSessionId.value).map((final) => ({
    id: `assistant-final-${final.turnId || final.id || "current"}`,
    kind: "assistant_final",
    turnId: final.turnId || "",
    text: final.text || "",
    status: final.status || "running",
    updatedAt: final.updatedAt || "",
    _sortSeq: Number(final.seq || 0) || Number.MAX_SAFE_INTEGER,
  }));
  return [...timeline, ...finals]
    .filter((row) => row.kind !== "turn" || row.title || row.summary)
    .sort((a, b) => {
      if (a._sortSeq !== b._sortSeq) return a._sortSeq - b._sortSeq;
      return String(a.updatedAt || "").localeCompare(String(b.updatedAt || ""));
    })
    .map(({ _sortSeq, ...row }) => row);
});
const hasProjectionRows = computed(() =>
  Boolean(
    projectionTimelineRows.value.length ||
      projectionAgentRows.value.length ||
      projectionApprovalDock.value.length ||
      projectionArtifactRows.value.length ||
      projectionDiffSummary.value,
  ),
);
const projectionTerminalMessage = computed(() => {
  const currentTurnId = compactText(activeAgentProjection.value?.currentTurnId || store.runtime.turn.turnId || store.runtime.turn.clientTurnId || "");
  const rows = projectionTimelineRows.value.filter((row) => row?.kind === "turn");
  const terminalRows = rows.filter((row) => {
    const status = compactText(row?.status).toLowerCase();
    const phase = compactText(row?.phase).toLowerCase();
    return status === "failed" || status === "canceled" || phase === "failed" || phase === "canceled";
  });
  const matchesCurrent = (row) => {
    if (!currentTurnId) return true;
    return [row?.turnId, row?.clientTurnId, row?.id].map((value) => compactText(value)).includes(currentTurnId);
  };
  const row = [...terminalRows].reverse().find(matchesCurrent) || terminalRows[terminalRows.length - 1] || null;
  return compactText(row?.summary || row?.detail);
});
const chatRuntimeBusy = computed(() => projectionRuntimeBusy.value || store.sending);
const chatRuntimePhase = computed(() => {
  const status = compactText(projectionRuntimeStatus.value || "idle").toLowerCase();
  if (status === "idle" && store.sending) return "thinking";
  if (status === "working") return "thinking";
  if (status === "blocked") return projectionApprovalDock.value.length ? "waiting_approval" : "waiting_input";
  if (status === "failed") return "failed";
  if (status === "canceled") return "aborted";
  if (status === "reviewing") return "reviewing";
  if (status === "idle") return "idle";
  return "completed";
});
const chatRuntimeStartedAt = computed(() => projectionStartedAt.value || "");
const composerPrimaryActionOverride = computed(() => (chatRuntimeBusy.value ? "stop" : ""));

function inferThinkingPrelude(message) {
  const text = (message || "").trim();
  const lower = text.toLowerCase();

  const searchLike =
    /a股|股市|行情|指数|港股|美股|财报|新闻|汇率|价格|走势|最新|今天|实时|网页|搜索|查一下|查下|盘面/i.test(text) ||
    /(market|stock|price|latest|news|search|web)/i.test(text);
  if (searchLike) {
    return {
      phase: "searching",
      hint: "我先查一下最新网页信息，再给你一个简洁结论。",
    };
  }

  const browseLike =
    /文件|文档|代码|配置|日志|目录|文件夹|打开|浏览|读取|读一下|看看.*文件|列出/i.test(text) ||
    /(file|folder|directory|read|open|browse|list)/i.test(lower);
  if (browseLike) {
    return {
      phase: "browsing",
      hint: "我先快速浏览相关文件，再给你结论。",
    };
  }

  return {
    phase: "thinking",
    hint: "我先快速理一下思路，再继续处理。",
  };
}

function queueThinkingPrelude(message) {
  clearThinkingPrelude();
  const prelude = inferThinkingPrelude(message);
  preferredThinkingPhase.value = prelude.phase;
  thinkingPhase.value = prelude.phase;
  thinkingHintTimer = window.setTimeout(() => {
    if (!showThinking.value) return;
    if (chatRuntimePhase.value !== "thinking" && chatRuntimePhase.value !== prelude.phase) return;
    if (activeActivityLine.value || summaryLine.value) return;
    thinkingHint.value = prelude.hint;
  }, 900);
}

watch(
  () => [chatRuntimePhase.value, chatRuntimeBusy.value, projectionRuntimeStatus.value],
  ([phase, busy]) => {
    if (!busy && (phase === "idle" || phase === "completed" || phase === "failed" || phase === "aborted" || phase === "reviewing")) {
      showThinking.value = false;
      approvalFollowupMode.value = false;
      clearThinkingPrelude();
    } else {
      const shouldPreferLocalPhase =
        phase === "thinking" &&
        !!preferredThinkingPhase.value &&
        !activeActivityLine.value &&
        !summaryLine.value;
      thinkingPhase.value = shouldPreferLocalPhase ? preferredThinkingPhase.value : phase;
      showThinking.value = true;
    }
    
    // Reset collapse state when a new approval arrives
    if (phase === "waiting_approval") {
      authCardCollapsed.value = false;
    }
  },
  { immediate: true },
);

/* ---- Activity summary ---- */
const activity = computed(() => store.runtime.activity);

/* ---- Codex-style activity status lines (Change 1 & 2) ---- */
let elapsedTimerHandle = null;
const elapsedNow = ref(Date.now());

function startElapsedTimer() {
  stopElapsedTimer();
  elapsedNow.value = Date.now();
  elapsedTimerHandle = window.setInterval(() => {
    elapsedNow.value = Date.now();
  }, 1000);
}

function stopElapsedTimer() {
  if (elapsedTimerHandle) {
    window.clearInterval(elapsedTimerHandle);
    elapsedTimerHandle = null;
  }
}

const elapsedLabel = computed(() => {
  const startedAt = chatRuntimeStartedAt.value;
  if (!startedAt) return "";
  const start = typeof startedAt === "number" ? startedAt : new Date(startedAt).getTime();
  if (!start || isNaN(start)) return "";
  const diff = Math.max(0, Math.floor((elapsedNow.value - start) / 1000));
  if (diff < 60) return `${diff}s`;
  const m = Math.floor(diff / 60);
  const s = diff % 60;
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
});

const workingElapsedLabel = computed(() => elapsedLabel.value || "0s");

function safeTimestamp(value) {
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? stamp : 0;
}

function isFinalCommandState(status = "") {
  const normalized = compactText(status).toLowerCase();
  return (
    normalized.includes("complete") ||
    normalized.includes("done") ||
    normalized.includes("fail") ||
    normalized.includes("error") ||
    normalized.includes("cancel") ||
    normalized.includes("timeout") ||
    normalized.includes("denied") ||
    normalized.includes("disconnect")
  );
}

const currentTurnStartTimestamp = computed(() => safeTimestamp(chatRuntimeStartedAt.value));

const codexActivityLines = computed(() => projectionActivityLines.value);

const isThinking = computed(() => {
  const phase = compactText(chatRuntimePhase.value).toLowerCase();
  return phase === "thinking" || phase === "planning" || phase === "finalizing";
});

const showCodexActivity = computed(() => {
  return chatRuntimeBusy.value && !isWorkspaceSession.value;
});

// Accumulate activity lines during the turn so they survive the backend clearing current fields
const accumulatedActivityLines = shallowRef([]);

function mergeAccumulatedActivityLines(lines = [], { replace = false } = {}) {
  const nextLines = Array.isArray(lines) ? lines : [];
  const existing = new Map(replace ? [] : accumulatedActivityLines.value.map((line) => [line.id, line]));
  for (const line of nextLines) {
    if (!line?.id || !compactText(line?.text)) continue;
    existing.set(line.id, line);
  }
  accumulatedActivityLines.value = [...existing.values()];
}

watch(codexActivityLines, (newLines) => {
  if (chatRuntimeBusy.value) {
    mergeAccumulatedActivityLines(newLines);
    return;
  }

  // After a refresh, runtime.activity is restored from snapshot, but the
  // in-memory accumulated lines are gone. Rehydrate them once so the completed
  // "已处理" fold still shows concrete process rows instead of an empty shell.
  if (!accumulatedActivityLines.value.length && Array.isArray(newLines) && newLines.length) {
    mergeAccumulatedActivityLines(newLines, { replace: true });
  }
}, { deep: true, immediate: true });

watch(
  () => chatRuntimeBusy.value,
  (active, wasActive) => {
    if (active && wasActive === false) {
      // Turn just started (not initial mount)
      startElapsedTimer();
      accumulatedActivityLines.value = [];
    }
    if (active && wasActive === undefined) {
      // Initial mount with active turn — start timer but keep accumulated lines
      startElapsedTimer();
    }
    if (!active && wasActive) {
      // Turn just completed
      stopElapsedTimer();
    }
  },
  { immediate: true },
);

// Approval inline mode (Change 3)
const hasCodexApproval = computed(() => {
  return chatRuntimePhase.value === "waiting_approval" && activeApprovalCard.value && !isWorkspaceSession.value;
});

const chatStreamApprovalDock = computed(() => {
  if (hasCodexApproval.value && !isWorkspaceSession.value) return [];
  return projectionApprovalDock.value;
});

const approvalQuestion = computed(() => {
  const card = activeApprovalCard.value;
  if (!card) return "是否允许执行此操作？";
  if (card.type === "CommandApprovalCard") return "是否允许执行以下命令？";
  if (card.type === "FileChangeApprovalCard") return "是否允许修改以下文件？";
  return "是否允许执行此操作？";
});

const approvalCommand = computed(() => {
  const card = activeApprovalCard.value;
  if (!card) return "";
  return compactText(card.command || card.title || card.summary || "");
});

function resolveCodexApproval(decision) {
  const card = activeApprovalCard.value;
  if (!card) return;
  const approvalId = card.approval?.requestId || card.id;
  decideApproval({ approvalId, decision });
}

function truncateLabel(value, max = 88) {
  if (!value || value.length <= max) return value;
  return `${value.slice(0, max - 3)}...`;
}

const activitySummary = computed(() => {
  const a = activity.value;
  const parts = [];
  if (a.filesViewed > 0) parts.push(`${a.filesViewed} 个文件`);
  if (a.searchCount > 0 && a.searchLocationCount > 0) {
    parts.push(`${a.searchCount} 次搜索（命中 ${a.searchLocationCount} 个位置）`);
  } else if (a.searchCount > 0) {
    parts.push(`${a.searchCount} 次搜索`);
  }
  if (a.listCount > 0) parts.push(`${a.listCount} 个列表`);
  if (a.filesChanged > 0) parts.push(`${a.filesChanged} 个文件修改`);
  if (a.commandsRun > 0) parts.push(`${a.commandsRun} 个命令`);
  if (parts.length === 0) return "";
  if (a.filesViewed > 0 && parts.length === 1) return `已浏览 ${parts[0]}`;
  if (a.searchCount > 0 && parts.length === 1) {
    return a.searchLocationCount > 0
      ? `已搜索 ${a.searchCount} 次，命中 ${a.searchLocationCount} 个位置`
      : `已搜索 ${a.searchCount} 次`;
  }
  if (a.filesChanged > 0 && parts.length === 1) return `已修改 ${a.filesChanged} 个文件`;
  return `已处理 ${parts.join("，")}`;
});

const currentReadingLine = computed(() => {
  const file = activity.value.currentReadingFile;
  return file ? `正在浏览（${file}）` : "";
});

const currentChangingLine = computed(() => {
  const file = activity.value.currentChangingFile;
  return file ? `正在修改（${truncateLabel(file)}）` : "";
});

const currentListingLine = computed(() => {
  const path = activity.value.currentListingPath;
  return path ? `正在列出（${truncateLabel(path)}）` : "";
});

const currentSearchLine = computed(() => {
  const a = activity.value;
  const query = a.currentSearchQuery || a.currentWebSearchQuery;
  if (!query) {
    return "";
  }
  if (a.currentSearchKind === "content") {
    return `正在搜索内容（${truncateLabel(query)}）`;
  }
  if (a.currentSearchKind === "web" || a.currentWebSearchQuery) {
    return `正在搜索网页（${truncateLabel(query)}）`;
  }
  return `正在搜索内容（${truncateLabel(query)}）`;
});

const viewedFileDetails = computed(() => activity.value.viewedFiles || []);
const searchedQueryDetails = computed(() => [
  ...(activity.value.searchedWebQueries || []),
  ...(activity.value.searchedContentQueries || []),
]);
const activeActivityLine = computed(() => currentChangingLine.value || currentReadingLine.value || currentListingLine.value || currentSearchLine.value || "");
const activeActivityKind = computed(() => {
  if (currentChangingLine.value) return "change";
  if (currentReadingLine.value) return "files";
  if (currentSearchLine.value) return "search";
  if (currentListingLine.value) return "list";
  return "";
});
const summaryLine = computed(() => {
  if (activeActivityLine.value) return "";
  return activitySummary.value;
});
const hasTopFeedback = computed(() => !!activeActivityLine.value || !!summaryLine.value);
const singleHostLiveActivityLines = computed(() => {
  const items = [];
  const seenText = new Set();
  const appendLine = (id, text, tone = "history") => {
    const label = compactText(text);
    if (!label || seenText.has(label)) return;
    if (latestRunningCommandCard.value && /^正在运行\s+/u.test(label)) return;
    seenText.add(label);
    items.push({ id, text: label, tone });
  };

  if (activeActivityLine.value) {
    appendLine("current-activity", activeActivityLine.value, "current");
  } else if (summaryLine.value && !accumulatedActivityLines.value.length) {
    appendLine("summary-activity", summaryLine.value, "summary");
  }

  accumulatedActivityLines.value.slice(-6).forEach((line, index) => {
    appendLine(line.id || `accumulated-${index}`, line.text, line.status === "running" ? "current" : "history");
  });

  return items;
});
const activeLineExpandable = computed(() => {
  if (activeActivityKind.value === "files") return viewedFileDetails.value.length > 0;
  if (activeActivityKind.value === "search") return searchedQueryDetails.value.length > 0;
  return false;
});
const summaryExpandable = computed(() => viewedFileDetails.value.length > 0 || searchedQueryDetails.value.length > 0);

function toggleActiveLineDetails() {
  if (activeActivityKind.value === "files" && viewedFileDetails.value.length) {
    showFileDetails.value = !showFileDetails.value;
    return;
  }
  if (activeActivityKind.value === "search" && searchedQueryDetails.value.length) {
    showSearchDetails.value = !showSearchDetails.value;
  }
}

function toggleSummaryDetails() {
  if (viewedFileDetails.value.length) {
    showFileDetails.value = !showFileDetails.value;
    return;
  }
  if (searchedQueryDetails.value.length) {
    showSearchDetails.value = !showSearchDetails.value;
  }
}

const activePlanCard = computed(() => {
  if (!chatRuntimeBusy.value) return null;
  const planCards = store.snapshot.cards.filter((card) => card.type === "PlanCard" && card.items?.length);
  if (!planCards.length) return null;
  return planCards[planCards.length - 1];
});

const pendingApprovalCards = computed(() => {
  return store.snapshot.cards.filter((card) => {
    if (card.status !== "pending") return false;
    return card.type === "CommandApprovalCard" || card.type === "FileChangeApprovalCard";
  });
});

const pendingApprovals = computed(() => {
  return (store.snapshot.approvals || []).filter((approval) => approval.status === "pending");
});

const reconnectErrorPattern = /^Reconnecting\.\.\.\s*\d+\s*\/\s*\d+$/i;

const activeApprovalCard = computed(() => {
  const nextApproval = pendingApprovals.value[0];
  if (!nextApproval) {
    return pendingApprovalCards.value[0] || null;
  }

  const byRequestID = store.snapshot.cards.find((card) => {
    if (card.status !== "pending") return false;
    if (card.type !== "CommandApprovalCard" && card.type !== "FileChangeApprovalCard") return false;
    return card.approval?.requestId === nextApproval.id;
  });
  if (byRequestID) return byRequestID;

  return store.snapshot.cards.find((card) => {
    if (card.status !== "pending") return false;
    if (card.type !== "CommandApprovalCard" && card.type !== "FileChangeApprovalCard") return false;
    return card.id === nextApproval.itemId;
  }) || pendingApprovalCards.value[0] || null;
});

const activeApprovalQueueIndex = computed(() => {
  if (!activeApprovalCard.value?.approval?.requestId) return -1;
  return pendingApprovals.value.findIndex((approval) => approval.id === activeApprovalCard.value.approval.requestId);
});

const activeApprovalQueueCount = computed(() => pendingApprovals.value.length);

const activeApprovalQueueLabel = computed(() => {
  if (!activeApprovalCard.value) return "";
  if (activeApprovalQueueCount.value <= 1) return "当前仅 1 项待确认";
  const position = activeApprovalQueueIndex.value >= 0 ? activeApprovalQueueIndex.value + 1 : 1;
  return `当前 ${position}/${activeApprovalQueueCount.value} 项待确认`;
});

const activeApprovalQueueNote = computed(() => {
  if (!activeApprovalCard.value || activeApprovalQueueCount.value <= 1) return "";
  const position = activeApprovalQueueIndex.value >= 0 ? activeApprovalQueueIndex.value + 1 : 1;
  const remaining = Math.max(activeApprovalQueueCount.value - position, 0);
  return remaining > 0 ? `后面还有 ${remaining} 项排队` : "";
});

const activeMcpApproval = computed(() => localMcpApprovals.value[0] || null);
const hasActiveApprovalOverlay = computed(() => Boolean(activeApprovalCard.value || activeMcpApproval.value));
const allowFollowUpComposer = computed(
  () =>
    approvalFollowupMode.value &&
    !hasActiveApprovalOverlay.value &&
    !chatRuntimeBusy.value,
);
const isWorkspaceSession = computed(() => store.snapshot.kind === "workspace");
const workspaceSessionLabel = computed(() => (isWorkspaceSession.value ? "工作台会话" : ""));
const workspaceDetailLinkLabel = computed(() => (isWorkspaceSession.value ? "查看只读详情" : ""));

const latestTerminalCard = computed(() => {
  const cards = store.snapshot.cards || [];
  for (let i = cards.length - 1; i >= 0; i -= 1) {
    const card = cards[i];
    if (!card || !card.output) {
      continue;
    }
    if (card.hostId && card.hostId !== store.snapshot.selectedHostId) {
      continue;
    }
    return card;
  }
  return null;
});

const terminalDockHost = computed(() => store.selectedHost || { id: store.snapshot.selectedHostId || "server-local" });
const terminalDockHostLabel = computed(() => resolveHostDisplay(terminalDockHost.value) || terminalDockHost.value.id || "server-local");
const terminalDockTitle = computed(() => latestTerminalCard.value?.title || "终端面板");
const terminalDockSubtitle = computed(() => {
  if (latestTerminalCard.value?.summary) return latestTerminalCard.value.summary;
  if (selectedHostAlert.value) return selectedHostAlert.value;
  const status = terminalDockHost.value.status || "unknown";
  return `当前主机 ${terminalDockHostLabel.value} · ${status}`;
});
const terminalDockOutput = computed(() => {
  const card = latestTerminalCard.value;
  if (!card) return "";
  return card.output || card.stdout || card.text || card.summary || "";
});
const terminalDockCanTakeover = computed(() => store.canOpenTerminal);
const terminalDockPanelHeight = computed(() => `${Math.max(140, terminalDockHeight.value - 108)}px`);
const terminalDockToolbarLabel = computed(() => {
  if (!terminalDockVisible.value) return "终端已收起";
  if (terminalDockSessionLive.value) return `终端已连接 · ${terminalDockHostLabel.value}`;
  if (!terminalDockCanTakeover.value) return `终端不可用 · ${terminalDockHostLabel.value}`;
  return `终端准备中 · ${terminalDockHostLabel.value}`;
});
const chatContainerStyle = computed(() => ({
  paddingBottom: terminalDockVisible.value
    ? `${terminalDockHeight.value + (activePlanCard.value || hasActiveApprovalOverlay.value ? 180 : 80)}px`
    : "80px",
}));

function queueLocalMcpApproval(action) {
  const approval = buildSyntheticMcpApproval(action, {
    scope: action?.scope || {},
    summary: action?.confirmText || "等待你确认后继续执行该 MCP 变更操作。",
  });
  localMcpApprovals.value = [
    ...localMcpApprovals.value.filter((item) => item.id !== approval.id),
    approval,
  ];
  store.noticeMessage = `${formatMcpActionLabel(action)} 已进入审批工作台。`;
  authCardCollapsed.value = false;
  approvalFollowupMode.value = false;
}

function completeLocalMcpApproval(approval, decision) {
  localMcpApprovals.value = localMcpApprovals.value.filter((item) => item.id !== approval.id && item.approvalId !== approval.approvalId);
  store.noticeMessage = buildMcpDecisionNotice(approval.action || {}, decision);
  approvalFollowupMode.value = decision === "decline" || decision === "reject";
  nextTick(() => jumpToLatest());
}

function handleTurnMcpAction(action) {
  if (!action || typeof action !== "object") return;
  if (action.type === "assistant_regenerate") {
    void submitMessage(action.message || "");
    return;
  }
  if (action.type === "assistant_feedback") {
    assistantFeedbackByMessageId.value = {
      ...assistantFeedbackByMessageId.value,
      [action.messageId || ""]: action.value || "",
    };
    return;
  }
  if (action.bundleKind === "monitor_bundle") {
    void refreshMcpSurface(action);
    return;
  }
  if (action.bundleKind === "remediation_bundle") {
    openMcpSurfaceDrawer(action);
    return;
  }
  if (action.bundleKind === "coroot_topology" || action.bundleKind === "coroot_host_overview") {
    openMcpSurfaceDrawer(action);
    return;
  }
  if (action.bundleKind === "coroot_service_monitor") {
    void refreshMcpSurface(action);
    return;
  }
  if (action.bundleKind === "coroot_incident_rca") {
    openMcpSurfaceDrawer(action);
    return;
  }
  const intent = compactText(action.intent || action.key || action.action || "").toLowerCase();
  if (intent === "refresh" || intent === "reload" || intent === "open_detail" || intent === "open-detail") {
    void refreshMcpSurface(action);
    return;
  }
  if (action.uiKind || action.ui_kind || action.bundleId || action.bundle_id) {
    openMcpSurfaceDrawer(action);
    return;
  }
  if (isMcpMutationAction(action)) {
    queueLocalMcpApproval(action);
    return;
  }
  store.noticeMessage = `${formatMcpActionLabel(action)} 已作为只读操作加入当前会话。`;
  approvalFollowupMode.value = false;
  nextTick(() => jumpToLatest());
}

function handleMcpSurfaceDetail(payload) {
  openMcpSurfaceDrawer(payload);
}

function handleMcpSurfacePin(payload) {
  pinMcpSurface(payload);
}

function handleMcpSurfaceRefresh(payload) {
  void refreshMcpSurface(payload);
}

function isApprovalCard(card) {
  return card?.type === "CommandApprovalCard" || card?.type === "FileChangeApprovalCard";
}

function isTerminalOutputCard(card) {
  return card?.type === "CommandCard" || (card?.type === "StepCard" && !!card?.command);
}

function isToolMessageCard(card) {
  return compactText(card?.role).toLowerCase() === "tool" || card?.type === "ToolMessageCard";
}

const singleHostCommandCards = computed(() => {
  return (store.snapshot.cards || []).filter((card) => {
    if (!isTerminalOutputCard(card)) return false;
    if (card.hostId && card.hostId !== store.snapshot.selectedHostId) return false;
    return true;
  });
});

const latestRunningCommandCard = computed(() => {
  if (hasProjectionRuntime.value) return null;
  if (!chatRuntimeBusy.value) return null;
  const turnStart = currentTurnStartTimestamp.value;
  const cards = singleHostCommandCards.value.filter((card) => {
    if (isFinalCommandState(card?.status)) return false;
    const compareAt = safeTimestamp(card?.startedAt || card?.updatedAt || card?.createdAt || card?.completedAt);
    if (!turnStart || !compareAt) return true;
    return compareAt >= turnStart;
  });
  return cards[cards.length - 1] || null;
});

const visibleCards = computed(() => {
  return store.snapshot.cards.filter((card) => {
    if (!isWorkspaceSession.value && card.type === "ThinkingCard") {
      return false;
    }
    // Hide active plan card
    if (activePlanCard.value && card.id === activePlanCard.value.id && chatRuntimeBusy.value) {
      return false;
    }
    // Hide all pending approval cards from the chat stream (rendered in overlay)
    if (card.status === "pending" && isApprovalCard(card)) {
      return false;
    }
    if (card.id === "__codex_reconnect__") {
      return false;
    }
    if (isToolMessageCard(card)) {
      return false;
    }
    const reconnectText = (card.message || card.text || "").trim();
    if (card.type === "ErrorCard" && reconnectErrorPattern.test(reconnectText)) {
      return false;
    }
    return true;
  });
});

const projectionTurnRowIds = computed(
  () => new Set(projectionTimelineRows.value.filter((row) => row.kind === "turn").map((row) => compactText(row.id))),
);
const projectionFinalTurnIds = computed(
  () => new Set(projectionTimelineRows.value.filter((row) => row.kind === "assistant_final").map((row) => compactText(row.turnId))),
);

function isCoveredByAgentEventProjection(card = {}) {
  if (!isWorkspaceSession.value) return false;
  if (!hasProjectionRows.value) return false;
  if (isUserConversationCard(card)) {
    const clientMessageId = compactText(card.clientMessageId);
    return Boolean(card.optimistic && clientMessageId && projectionTurnRowIds.value.has(clientMessageId));
  }
  if (isAssistantConversationCard(card)) {
    const turnId = compactText(card.turnId || String(card.id || "").replace(/^agent-final-/, ""));
    return Boolean(turnId && projectionFinalTurnIds.value.has(turnId));
  }
  return false;
}

const streamCards = computed(() =>
  visibleCards.value.filter((card) => {
    if (isCoveredByAgentEventProjection(card)) {
      return false;
    }
    if (isApprovalCard(card)) {
      return false;
    }
    if (isTerminalOutputCard(card)) {
      return false;
    }
    return true;
  }),
);

function isUserConversationCard(card = {}) {
  return card?.type === "UserMessageCard" || card?.role === "user";
}

function isAssistantConversationCard(card = {}) {
  return card?.type === "AssistantMessageCard" || card?.role === "assistant";
}

const currentTurnAssistantCards = computed(() => {
  const cards = streamCards.value.filter((card) => isChatConversationCard(card));
  const assistants = [];
  for (let index = cards.length - 1; index >= 0; index -= 1) {
    const card = cards[index];
    if (isUserConversationCard(card)) {
      break;
    }
    if (isAssistantConversationCard(card)) {
      assistants.unshift(card);
    }
  }
  return assistants;
});

const activeStreamingFinalMessageVisible = computed(() => {
  if (isWorkspaceSession.value) return false;
  if (!(showThinkingCard.value || chatRuntimeBusy.value)) return false;
  const lastAssistantCard = currentTurnAssistantCards.value[currentTurnAssistantCards.value.length - 1];
  return shouldExposeActiveFinalMessage({ card: lastAssistantCard, sourceCard: lastAssistantCard });
});

const mainChatActiveProcess = computed(() => {
  const terminalPhase = compactText(chatRuntimePhase.value).toLowerCase();
  const terminalState = terminalPhase === "failed" || terminalPhase === "aborted";
  const terminalMessage = compactText(store.errorMessage || projectionTerminalMessage.value);
  const turnRunning = showThinkingCard.value || chatRuntimeBusy.value;
  const completed = !turnRunning || activeStreamingFinalMessageVisible.value;
  const latestUserCard = [...streamCards.value].reverse().find((card) => isUserConversationCard(card)) || null;
  const turnKeys = [
    activeAgentProjection.value?.currentTurnId,
    store.runtime.turn.turnId,
    store.runtime.turn.clientTurnId,
    latestUserCard?.turnId,
    latestUserCard?.clientTurnId,
  ].map((value) => compactText(value)).filter(Boolean);
  let items = buildPersistedProcessItems(accumulatedActivityLines.value, { completed });
  const terminalStatusItem = buildTerminalStatusItem(terminalPhase, terminalMessage);
  if (terminalState && terminalStatusItem && !items.some((item) => item.id === terminalStatusItem.id)) {
    items = [...items, terminalStatusItem];
  }

  // Return process data when turn is active OR when there are accumulated items
  // so the "已处理" fold persists after the turn completes.
  if (!turnRunning && !terminalState && !hasTopFeedback.value && items.length === 0) return null;

  return {
    turnId: compactText(activeAgentProjection.value?.currentTurnId || store.runtime.turn.turnId || store.runtime.turn.clientTurnId || ""),
    clientTurnId: compactText(latestUserCard?.clientTurnId || store.runtime.turn.clientTurnId || ""),
    turnKeys: [...new Set(turnKeys)],
    phase: terminalState ? terminalPhase : thinkingDisplayPhase.value,
    liveHint: terminalState ? "" : (completed ? "" : (activeActivityLine.value || thinkingHint.value || "")),
    summary: terminalState ? "" : (completed ? (activitySummary.value || "") : (summaryLine.value || (!activeActivityLine.value ? activitySummary.value : ""))),
    items,
    elapsedLabel: workingElapsedLabel.value,
    statusCard: buildTerminalStatusCard(terminalPhase, terminalMessage, workingElapsedLabel.value),
  };
});

const mainChatTurns = computed(() =>
  formatMainChatTurns({
    conversationCards: streamCards.value.filter((card) => isChatConversationCard(card)),
    commandCards: singleHostCommandCards.value,
    turnActive: showThinkingCard.value || chatRuntimeBusy.value,
    activeProcess: mainChatActiveProcess.value,
    hideLiveProcessDetails: !isWorkspaceSession.value,
  }),
);

const mainChatTurnByAnchorId = computed(() =>
  new Map(mainChatTurns.value.map((turn) => [turn.anchorMessageId, turn])),
);

const singleHostLiveTurnId = computed(() => {
  if (isWorkspaceSession.value) return "";
  if (!(showThinkingCard.value || chatRuntimeBusy.value)) {
    return "";
  }
  const turns = mainChatTurns.value;
  return turns.length ? turns[turns.length - 1].id : "";
});

const baseStreamEntries = computed(() => {
  const entries = [];
  const renderedTurnIds = new Set();

  for (const card of streamCards.value) {
    if (isChatConversationCard(card)) {
      const turn = mainChatTurnByAnchorId.value.get(card.id);
      if (turn && !renderedTurnIds.has(turn.id)) {
        entries.push({
          id: turn.id,
          kind: "turn",
          turn,
        });
        renderedTurnIds.add(turn.id);
      }
      continue;
    }

    entries.push({
      id: `card-${card.id}`,
      kind: "card",
      card,
    });
  }

  if (!mainChatTurns.value.length && showThinking.value && hasTopFeedback.value && !showThinkingCard.value) {
    entries.push({ id: "__activity__", kind: "activity" });
  }
  if (showThinkingCard.value || chatRuntimeBusy.value) {
    const hasVisibleSingleHostLiveTurn =
      !isWorkspaceSession.value &&
      !!singleHostLiveTurnId.value &&
      entries.some((entry) => entry.kind === "turn" && entry.turn?.id === singleHostLiveTurnId.value);
    // Single-host mode prefers rendering the live status inside the active turn,
    // but keeps a standalone working row when the live turn is not yet materialized.
    if (isWorkspaceSession.value || !hasVisibleSingleHostLiveTurn) {
      entries.push({ id: "__thinking__", kind: "thinking" });
    }
  }

  return entries;
});

const streamSessionKey = computed(() => store.activeSessionId || store.snapshot.sessionId || "");
const showEmptyState = computed(() => !streamCards.value.length && !hasProjectionRows.value && !store.loading && !showThinking.value);

watch(streamSessionKey, () => {
  accumulatedActivityLines.value = [];
  if (!chatRuntimeBusy.value && codexActivityLines.value.length) {
    mergeAccumulatedActivityLines(codexActivityLines.value, { replace: true });
  }
});

function entrySignature(entry) {
  if (!entry) return "";
  if (entry.kind === "turn") {
    return [
      entry.id,
      entry.turn.processItems?.length || 0,
      entry.turn.finalMessage?.id || "",
      entry.turn.finalMessage?.card?.text?.length || 0,
      entry.turn.liveHint || "",
      entry.turn.summary || "",
    ].join(":");
  }
  if (entry.kind === "card") {
    const card = entry.card || {};
    return [
      entry.id,
      card.updatedAt || "",
      (card.text || card.message || "").length,
      (card.output || "").length,
      card.status || "",
    ].join(":");
  }
  if (entry.kind === "activity") {
    return [
      entry.id,
      activeActivityLine.value,
      summaryLine.value,
      viewedFileDetails.value.length,
      searchedQueryDetails.value.length,
    ].join(":");
  }
  return [entry.id, thinkingPhase.value, thinkingHint.value].join(":");
}

const streamSignature = computed(() => {
  const list = baseStreamEntries.value;
  const tail = list.slice(-3);
  return [
    tail.map((entry) => entrySignature(entry)).join("|"),
    activeActivityLine.value,
    summaryLine.value,
    showThinkingCard.value ? thinkingPhase.value : "",
    showThinkingCard.value ? thinkingHint.value : "",
  ].join("::");
});

const composerStatusHint = computed(() => {
  if (allowFollowUpComposer.value) {
    return "当前可以直接继续输入 follow-up。";
  }
  return "";
});

const {
  isPinnedToBottom,
  unreadCount,
  unreadAnchorId,
  showUnreadPill,
  forceScrollToBottom,
  handleScroll,
  jumpToLatest,
} = useChatScrollState({
  scrollContainer,
  scrollContent,
  items: baseStreamEntries,
  signature: streamSignature,
  getItemId: (item) => String(item?.id || ""),
  threshold: 80,
});

const renderedStreamEntries = computed(() => {
  const entries = [];
  for (const entry of baseStreamEntries.value) {
    if (unreadAnchorId.value && entry.id === unreadAnchorId.value) {
      entries.push({
        id: `unread-divider-${entry.id}`,
        kind: "divider",
      });
    }
    entries.push(entry);
  }
  return entries;
});

const virtualStream = useVirtualTurnList({
  items: renderedStreamEntries,
  scrollContainer,
  suspended: computed(() => chatRuntimeBusy.value),
  estimateSize(entry) {
    if (entry?.kind === "turn") return 172;
    if (entry?.kind === "divider") return 72;
    if (entry?.kind === "activity") return 96;
    if (entry?.kind === "thinking") return 96;
    return 120;
  },
  overscan: 8,
  minItemCount: 18,
  getItemKey: (entry, index) => entry?.id || index,
});

const virtualizedStreamEntries = computed(() =>
  virtualStream.virtualItems.value.map((entry) => entry.item),
);
const showVirtualTopSpacer = computed(
  () => virtualStream.enabled.value && virtualStream.topSpacerHeight.value > 0,
);
const showVirtualBottomSpacer = computed(
  () => virtualStream.enabled.value && virtualStream.bottomSpacerHeight.value > 0,
);
const virtualTopSpacerHeight = computed(() => virtualStream.topSpacerHeight.value);
const virtualBottomSpacerHeight = computed(() => virtualStream.bottomSpacerHeight.value);

function handleChatScroll(event) {
  handleScroll(event);
  virtualStream.handleScroll(event);
}

function jumpToLatestAndSync() {
  forceScrollToBottom();
  nextTick(() => {
    virtualStream.syncViewport();
    // Smooth scroll: after virtual list sync, ensure the last element is smoothly visible
    const el = scrollContainer.value;
    if (el) {
      const lastChild = el.querySelector("[data-testid]:last-child, .chat-turn-group:last-child, .stream-row:last-child");
      if (lastChild && typeof lastChild.scrollIntoView === "function") {
        lastChild.scrollIntoView({ behavior: "smooth", block: "end" });
      }
    }
  });
}

watch(
  [activeActivityLine, summaryLine, () => chatRuntimePhase.value],
  ([currentLine, currentSummary, phase]) => {
    const hasFeedback = !!currentLine || !!currentSummary;
    const phaseMovedOn =
      phase === "planning" ||
      phase === "waiting_approval" ||
      phase === "waiting_input" ||
      phase === "executing" ||
      phase === "finalizing";

    if (hasFeedback || phaseMovedOn) {
      clearThinkingPrelude();
      if (showThinking.value) {
        thinkingPhase.value = phase;
      }
    } else if (phase === "thinking" && preferredThinkingPhase.value) {
      thinkingPhase.value = preferredThinkingPhase.value;
    }
  }
);

/* ---- Reconnection ---- */
const showReconnectBanner = computed(() => {
  return store.runtime.codex.status === "reconnecting" || isStopped.value;
});

const reconnectHostLabel = computed(() => {
  const host = store.selectedHost;
  if (!host) return "";
  const name = resolveHostDisplay(host);
  const status = host.status === "online" ? "在线" : host.status === "offline" ? "离线" : host.status || "未知";
  return `当前主机 ${name}（${status}）`;
});

const reconnectLabel = computed(() => {
  const c = store.runtime.codex;
  if (c.status === "stopped") return `与 ai-server 的实时连接已断开 · ${reconnectHostLabel.value}`;
  return `与 ai-server 的实时连接重连中 ${c.retryAttempt}/${c.retryMax} · ${reconnectHostLabel.value}`;
});

const isStopped = computed(() => store.runtime.codex.status === "stopped");

const codexReconnectNotice = computed(() => {
  return (
    store.snapshot.cards.find((card) => card.id === "__codex_reconnect__" && card.status === "inProgress") || null
  );
});

const showCodexReconnectBanner = computed(() => {
  return !!codexReconnectNotice.value && store.runtime.codex.status === "connected";
});

const codexReconnectLabel = computed(() => {
  return codexReconnectNotice.value?.message || codexReconnectNotice.value?.text || "与 LLM 的连接波动，正在自动恢复";
});

const selectedHostAlert = computed(() => {
  const host = store.selectedHost;
  if (!host || host.id === "server-local" || host.status === "online") {
    return "";
  }
  return `当前远程主机 ${resolveHostDisplay(host)} 离线，聊天与终端都不会静默回退到 server-local。`;
});

const composerPlaceholder = computed(() => {
  if (!store.snapshot.auth.connected) return "请先登录后再开始对话";
  if (!store.snapshot.config.codexAlive) return "LLM 未连接，请在 设置 → LLM 配置 中设置 API Key";
  if (allowFollowUpComposer.value) return "可以继续输入 follow-up，Cmd+Enter 发送";
  if (store.selectedHost.terminalCapable && !store.selectedHost.executable) {
    return "当前主机已接入远程终端，自动执行链路还未开启";
  }
  if (!store.selectedHost.executable) return "当前主机仅展示，不支持执行";
  if (store.selectedHost.status !== "online") return "当前主机离线，暂时不可执行";
  if (store.selectedHost.kind === "agent") return "输入指令，让 AI 管理这台主机";
  return "输入你的问题或任务";
});

function getRowClass(card) {
  if (card.type === "UserMessageCard" || (card.type === "MessageCard" && card.role === "user")) {
    return "row-user";
  }
  if (card.type === "NoticeCard") {
    return "row-notice";
  }
  return "row-assistant";
}

async function submitMessage(messageInput, { restoreComposerOnFail = false, originalDraft = "" } = {}) {
  const message = String(messageInput || "").trim();
  if (!store.canSend || !message) return;
  if (chatRuntimeBusy.value) return;

  const clientMessageId = createClientId("client-msg");
  const clientTurnId = createClientId("client-turn");
  const sessionId = store.activeSessionId || store.snapshot.sessionId;
  const hostId = store.snapshot.selectedHostId;
  store.errorMessage = "";
  accumulatedActivityLines.value = [];
  if (isWorkspaceSession.value) {
    queueThinkingPrelude(message);
  }
  const optimisticCardId = store.appendOptimisticUserMessage({
    message,
    clientMessageId,
    clientTurnId,
    sessionId,
    hostId,
  });
  store.resetActivity();
  nextTick(() => forceScrollToBottom());
  store.sending = true;

  try {
    const accepted = await sendMessageApi({
      message,
      sessionId,
      hostId,
      clientMessageId,
      clientTurnId,
    });
    const acceptedSessionId = compactText(accepted?.sessionId || sessionId);
    const acceptedTurnId = compactText(accepted?.turnId || clientTurnId);
    const acceptedClientTurnId = compactText(accepted?.clientTurnId || clientTurnId);
    store.applyAgentEvent({
      eventId: `local:${clientMessageId}:turn.started`,
      seq: 0,
      sessionId: acceptedSessionId,
      turnId: acceptedTurnId,
      clientTurnId: acceptedClientTurnId,
      kind: "turn",
      phase: "started",
      status: "running",
      visibility: "primary",
      source: "ui",
      createdAt: new Date().toISOString(),
      payload: {
        title: message,
        prompt: message,
        hostId,
        clientMessageId,
      },
    });
    approvalFollowupMode.value = false;
    nextTick(() => forceScrollToBottom());
  } catch (e) {
    store.errorMessage = e?.message || "Network error";
    if (optimisticCardId) {
      store.patchOptimisticCard(optimisticCardId, {
        status: "failed",
        lastError: store.errorMessage,
      });
    }
    if (restoreComposerOnFail) {
      composerMessage.value = originalDraft || message;
    }
    showThinking.value = false;
    store.setTurnPhase("failed");
    store.clearTurnPendingStart();
    clearThinkingPrelude();
    focusComposer();
  } finally {
    store.sending = false;
  }
}

async function sendMessage() {
  if (!store.canSend || !composerMessage.value.trim()) return;
  const draft = composerMessage.value;
  const message = draft.trim();
  composerMessage.value = "";
  await submitMessage(message, {
    restoreComposerOnFail: true,
    originalDraft: draft,
  });
}

async function stopMessage() {
  if (!chatRuntimeBusy.value) return;
  if (stopBusy.value) return;
  stopBusy.value = true;
  try {
    const turnId = compactText(activeAgentProjection.value?.currentTurnId || store.runtime.turn.turnId || store.runtime.turn.clientTurnId || "");
    await stopMessageApi({
      sessionId: activeProjectionSessionId.value,
      turnId,
    });
    store.errorMessage = "";
    showThinking.value = false;
    store.setTurnPhase("aborted");
    approvalFollowupMode.value = false;
    clearThinkingPrelude();
    focusComposer();
  } catch (e) {
    const message = String(e?.message || "").trim().toLowerCase();
    const payloadError = String(e?.payload?.error || "").trim().toLowerCase();
    if (e?.status === 409 && (message.includes("no active turn found") || payloadError.includes("no active turn found"))) {
      store.errorMessage = "";
      showThinking.value = false;
      store.setTurnPhase("aborted");
      approvalFollowupMode.value = false;
      clearThinkingPrelude();
      focusComposer();
    } else {
      console.error(e);
      store.errorMessage = "stop failed";
    }
  } finally {
    stopBusy.value = false;
  }
}

async function decideApproval({ approvalId, decision }) {
  const localApproval = localMcpApprovals.value.find((item) => item.id === approvalId || item.approvalId === approvalId);
  if (localApproval) {
    completeLocalMcpApproval(localApproval, decision);
    return;
  }
  try {
    store.setTurnPhase("executing");
    await submitApprovalDecisionApi(approvalId, decision);
    if (decision === "decline" || decision === "reject") {
      approvalFollowupMode.value = true;
    } else {
      approvalFollowupMode.value = false;
    }
    nextTick(() => jumpToLatest());
  } catch (e) {
    console.error(e);
    store.errorMessage = e?.message || "approval failed";
  }
}

async function handleChoice({ requestId, answers }) {
  try {
    store.setTurnPhase("thinking");
    await submitChoiceAnswerApi(requestId, answers);
    store.errorMessage = "";
    nextTick(() => jumpToLatest());
  } catch (e) {
    console.error(e);
    store.errorMessage = e?.message || "choice submit failed";
    store.setTurnPhase("failed");
  }
}

function retryConnection() {
  store.runtime.codex.retryAttempt = 0;
  store.runtime.codex.status = "reconnecting";
  store.connectWs();
}

function handleRetry() {
  if (isStopped.value) {
    retryConnection();
    return;
  }
  store.fetchState();
}

function handleRefresh() {
  window.location.reload();
}

function isEditableTarget(target) {
  if (!target || !(target instanceof HTMLElement)) return false;
  const tagName = target.tagName ? target.tagName.toLowerCase() : "";
  return target.isContentEditable || tagName === "input" || tagName === "textarea" || tagName === "select";
}

async function ensureTerminalDockConnected() {
  await nextTick();
  if (!terminalDockVisible.value) return;
  if (terminalDockRef.value?.takeover) {
    await terminalDockRef.value.takeover();
  }
}

function toggleTerminalDock(forceVisible) {
  terminalDockVisible.value = typeof forceVisible === "boolean" ? forceVisible : !terminalDockVisible.value;
  if (!terminalDockVisible.value) {
    terminalDockSessionLive.value = false;
    return;
  }
  terminalDockSessionLive.value = terminalDockSessionLive.value || false;
  void ensureTerminalDockConnected();
}

function syncTerminalDockToHost() {
  if (!terminalDockRef.value) {
    return;
  }
  if (typeof terminalDockRef.value.reconnect === "function") {
    terminalDockRef.value.reconnect();
    return;
  }
  if (typeof terminalDockRef.value.takeover === "function") {
    terminalDockRef.value.takeover();
  }
}

function handleTerminalDockConnected() {
  terminalDockSessionLive.value = true;
}

function handleTerminalDockDisconnected() {
  terminalDockSessionLive.value = false;
}

function handleTerminalDockError() {
  terminalDockSessionLive.value = false;
}

function handleTerminalDockToggleKeydown(e) {
  const isBackquote = e.key === "`" || e.code === "Backquote";
  if (!isBackquote || !e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) {
    return;
  }
  e.preventDefault();
  toggleTerminalDock();
}

function handleTerminalDockResizeStart(e) {
  if (!terminalDockVisible.value) {
    return;
  }
  e.preventDefault();
  terminalDockDragging.value = true;
  const startY = e.clientY;
  const startHeight = terminalDockHeight.value;
  terminalDockMaxHeight = Math.max(260, Math.floor(window.innerHeight * 0.72));

  const handleMove = (moveEvent) => {
    const delta = startY - moveEvent.clientY;
    const nextHeight = Math.max(220, Math.min(terminalDockMaxHeight, startHeight + delta));
    terminalDockHeight.value = nextHeight;
  };

  const handleUp = () => {
    terminalDockDragging.value = false;
    window.removeEventListener("mousemove", handleMove);
    window.removeEventListener("mouseup", handleUp);
    terminalDockDragState = null;
  };

  terminalDockDragState = { handleMove, handleUp };
  window.addEventListener("mousemove", handleMove);
  window.addEventListener("mouseup", handleUp, { once: true });
}

function openWorkspaceProtocol() {
  if (!isWorkspaceSession.value) return;
  window.location.href = "/protocol";
}

watch(
  () => activity.value.viewedFiles,
  () => {
    showFileDetails.value = false;
  },
  { deep: true }
);

watch(
  () => activity.value.searchedWebQueries,
  () => {
    showSearchDetails.value = false;
  },
  { deep: true }
);

watch(
  () => activeApprovalCard.value?.id,
  (approvalID, previousID) => {
    if (!approvalID || approvalID === previousID) return;
    authCardCollapsed.value = false;
    approvalFollowupMode.value = false;
    thinkingPhase.value = "waiting_approval";
    showThinking.value = true;
    clearThinkingPrelude();
  }
);

watch(
  () => store.snapshot.cards[store.snapshot.cards.length - 1]?.id,
  (cardID, previousID) => {
    if (!cardID || cardID === previousID || !showThinking.value) return;
    const lastCard = store.snapshot.cards[store.snapshot.cards.length - 1];
    if (!lastCard) return;
    const isUserCard =
      lastCard.type === "UserMessageCard" ||
      (lastCard.type === "MessageCard" && lastCard.role === "user");
    if (isUserCard) return;
    clearThinkingPrelude();
  }
);

onBeforeUnmount(() => {
  clearThinkingPrelude();
  stopElapsedTimer();
  window.removeEventListener("keydown", handleTerminalDockToggleKeydown);
  if (terminalDockDragState?.handleMove) {
    window.removeEventListener("mousemove", terminalDockDragState.handleMove);
  }
  if (terminalDockDragState?.handleUp) {
    window.removeEventListener("mouseup", terminalDockDragState.handleUp);
  }
  terminalDockDragState = null;
});

onMounted(() => {
  window.addEventListener("keydown", handleTerminalDockToggleKeydown);
  terminalDockMaxHeight = Math.max(260, Math.floor(window.innerHeight * 0.72));
});

watch(
  () => store.snapshot.selectedHostId,
  () => {
    if (!terminalDockSessionLive.value && !terminalDockVisible.value) {
      return;
    }
    syncTerminalDockToHost();
  },
);

watch(
  () => store.canOpenTerminal,
  (canOpenTerminal, previous) => {
    if (!canOpenTerminal || canOpenTerminal === previous) {
      return;
    }
    if (!terminalDockVisible.value && !terminalDockSessionLive.value) {
      return;
    }
    syncTerminalDockToHost();
  },
);

watch(
  () => terminalDockVisible.value,
  async (visible, previous) => {
    if (visible === previous || !visible) {
      return;
    }
    terminalDockVisible.value = true;
    await ensureTerminalDockConnected();
  },
);
</script>

<template>
  <!-- Reconnection Banner -->
  <div class="reconnect-banner" v-if="showReconnectBanner">
    <WifiOffIcon size="14" />
    <span>{{ reconnectLabel }}</span>
    <button v-if="isStopped" class="reconnect-btn" @click="retryConnection">
      <RefreshCwIcon size="12" /> 重试
    </button>
  </div>

  <div class="reconnect-banner subtle" v-if="showCodexReconnectBanner">
    <WifiOffIcon size="14" />
    <span>{{ codexReconnectLabel }}</span>
  </div>

  <ChatStream
    ref="chatStreamRef"
    :container-style="chatContainerStyle"
    :loading="store.loading"
    :show-workspace-banner="isWorkspaceSession"
    :workspace-session-label="workspaceSessionLabel"
    :workspace-detail-link-label="workspaceDetailLinkLabel"
    :show-empty-state="showEmptyState"
    :notice-message="store.noticeMessage"
    :selected-host-alert="selectedHostAlert"
    :error-message="store.errorMessage"
    :entries="renderedStreamEntries"
    :virtual-items="virtualizedStreamEntries"
    :unread-count="unreadCount"
    :show-virtual-top-spacer="showVirtualTopSpacer"
    :show-virtual-bottom-spacer="showVirtualBottomSpacer"
    :virtual-top-spacer-height="virtualTopSpacerHeight"
    :virtual-bottom-spacer-height="virtualBottomSpacerHeight"
    :timeline-rows="projectionTimelineRows"
    :agent-rows="projectionAgentRows"
    :approval-dock="chatStreamApprovalDock"
    :artifact-rows="projectionArtifactRows"
    :diff-summary="projectionDiffSummary"
    :runtime-status="projectionRuntimeStatus"
    :single-host-live-turn-id="singleHostLiveTurnId"
    :working-elapsed-label="workingElapsedLabel"
    :single-host-live-activity-lines="singleHostLiveActivityLines"
    :latest-running-command-card="latestRunningCommandCard"
    :feedback-by-message-id="assistantFeedbackByMessageId"
    :active-activity-line="activeActivityLine"
    :active-line-expandable="activeLineExpandable"
    :summary-line="summaryLine"
    :summary-expandable="summaryExpandable"
    :show-file-details="showFileDetails"
    :viewed-file-details="viewedFileDetails"
    :show-search-details="showSearchDetails"
    :searched-query-details="searchedQueryDetails"
    :thinking-card="thinkingCard"
    :is-workspace-session="isWorkspaceSession"
    :get-row-class="getRowClass"
    @scroll="handleChatScroll"
    @workspace-detail="openWorkspaceProtocol"
    @turn-action="handleTurnMcpAction"
    @mcp-detail="handleMcpSurfaceDetail"
    @mcp-pin="handleMcpSurfacePin"
    @mcp-refresh="handleMcpSurfaceRefresh"
    @approval="decideApproval"
    @choice="handleChoice"
    @retry="handleRetry"
    @card-refresh="handleRefresh"
    @toggle-active-details="toggleActiveLineDetails"
    @toggle-summary-details="toggleSummaryDetails"
  />

  <button
    v-if="showUnreadPill"
    type="button"
    class="chat-unread-pill"
    data-testid="chat-unread-pill"
    @click="jumpToLatestAndSync"
  >
    {{ unreadCount }} 条新结果
  </button>

  <ChatComposerDock
    ref="composerDockRef"
    v-model="composerMessage"
    :placeholder="composerPlaceholder"
    :allow-follow-up="allowFollowUpComposer"
    :disabled="isStopped"
    :plan-card="activePlanCard"
    :session-kind="store.snapshot.kind"
    :status-hint="composerStatusHint"
    :show-composer="(!hasActiveApprovalOverlay || authCardCollapsed) && !hasCodexApproval"
    :is-docked-bottom="!!activePlanCard || hasActiveApprovalOverlay"
    :busy="stopBusy"
    :primary-action-override="composerPrimaryActionOverride"
    @send="sendMessage"
    @stop="stopMessage"
  >
    <!-- Terminal dock removed for cleaner UI -->

    <template #approval>
      <!-- Codex-style inline approval (Change 3) for single-host mode -->
      <div v-if="hasCodexApproval && !authCardCollapsed" class="codex-approval-inline" data-testid="codex-approval-inline">
        <div class="codex-approval-question">{{ approvalQuestion }}</div>
        <div v-if="approvalCommand" class="codex-approval-command">
          <code>{{ approvalCommand }}</code>
        </div>
        <div class="codex-approval-options">
          <button class="codex-approval-option" @click="resolveCodexApproval('accept')">
            <span class="option-number">1.</span> 是
          </button>
          <button class="codex-approval-option" @click="resolveCodexApproval('accept_session')">
            <span class="option-number">2.</span> 是，且对于后续类似命令不再询问
          </button>
          <button class="codex-approval-option" @click="resolveCodexApproval('reject')">
            <span class="option-number">3.</span> 否，请告知如何调整
          </button>
        </div>
        <div class="codex-approval-actions">
          <button class="codex-skip-btn" @click="resolveCodexApproval('reject')">跳过</button>
          <button class="codex-submit-btn" @click="resolveCodexApproval('accept')">提交 ⏎</button>
        </div>
        <div v-if="activeApprovalQueueCount > 1" class="codex-approval-queue-note">
          {{ activeApprovalQueueLabel }}
        </div>
      </div>

      <!-- Fallback: original approval overlay for workspace mode or collapsed state -->
      <div v-else-if="(activeApprovalCard || activeMcpApproval) && (isWorkspaceSession || !hasCodexApproval)" class="auth-overlay-dock">
        <div v-if="!authCardCollapsed" class="auth-overlay-container">
          <div class="auth-overlay-header">
            <div class="auth-overlay-title-group">
              <span class="auth-overlay-title">需要您的确认</span>
              <n-badge v-if="activeApprovalCard && activeApprovalQueueCount > 0" :value="activeApprovalQueueCount" :max="99" />
              <span v-if="activeApprovalCard && activeApprovalQueueLabel" class="auth-overlay-queue-label">{{ activeApprovalQueueLabel }}</span>
              <span v-else-if="activeMcpApproval" class="auth-overlay-queue-label">MCP 变更待确认</span>
            </div>
            <button class="icon-btn auth-collapse-btn" @click="authCardCollapsed = true">折叠审批工作台</button>
          </div>
          <div v-if="activeApprovalCard && activeApprovalQueueNote" class="auth-overlay-queue-note">{{ activeApprovalQueueNote }}</div>
          <CardItem
            v-if="activeApprovalCard"
            :card="activeApprovalCard"
            :session-kind="store.snapshot.kind"
            :is-overlay="true"
            @approval="decideApproval"
          />
          <div
            v-else-if="activeMcpApproval"
            class="chat-mcp-approval"
            data-testid="chat-mcp-approval-overlay"
          >
            <div class="chat-mcp-approval-copy">
              <strong>{{ activeMcpApproval.title }}</strong>
              <p>{{ activeMcpApproval.summary }}</p>
            </div>
            <dl class="chat-mcp-approval-meta">
              <div class="chat-mcp-approval-row">
                <dt>目标</dt>
                <dd>{{ formatMcpActionTarget(activeMcpApproval.action || {}, activeMcpApproval.action?.scope || {}) }}</dd>
              </div>
              <div class="chat-mcp-approval-row">
                <dt>权限</dt>
                <dd>{{ activeMcpApproval.action?.permissionPath || "未声明" }}</dd>
              </div>
            </dl>
            <div class="chat-mcp-approval-actions">
              <button
                type="button"
                class="option-row secondary"
                data-testid="chat-mcp-approval-reject"
                @click="decideApproval({ approvalId: activeMcpApproval.approvalId, decision: 'reject' })"
              >
                拒绝
              </button>
              <button
                type="button"
                class="option-row primary"
                data-testid="chat-mcp-approval-accept"
                @click="decideApproval({ approvalId: activeMcpApproval.approvalId, decision: 'accept' })"
              >
                同意执行
              </button>
            </div>
          </div>
        </div>

        <button v-else class="auth-restore-btn" @click="authCardCollapsed = false">
           <span>当前审批工作台已折叠</span>
           <n-badge v-if="activeApprovalQueueCount > 0" :value="activeApprovalQueueCount" :max="99" />
           <span v-if="activeApprovalQueueLabel" class="auth-restore-queue">{{ activeApprovalQueueLabel }}</span>
        </button>
      </div>
    </template>
  </ChatComposerDock>

  <n-drawer
    :show="isMcpDrawerOpen && !!activeMcpSurface"
    placement="right"
    :width="320"
    :mask-closable="true"
    data-testid="chat-mcp-surface-drawer"
    @update:show="(val) => { if (!val) closeMcpSurfaceDrawer(); }"
  >
    <n-drawer-content :title="activeMcpSurface?.title || 'MCP SURFACE'" :native-scrollbar="false" closable>
      <template #header>
        <div class="chat-mcp-surface-copy">
          <span class="chat-mcp-surface-kicker">MCP SURFACE</span>
          <h3>{{ activeMcpSurface?.title }}</h3>
          <p v-if="activeMcpSurface?.subtitle">{{ activeMcpSurface.subtitle }}</p>
        </div>
      </template>

      <div v-if="activeMcpSurface" :data-surface-kind="activeMcpSurface.kind">
        <div class="chat-mcp-surface-toolbar">
          <button
            type="button"
            class="chat-mcp-surface-toolbar-btn"
            data-testid="chat-mcp-surface-pin"
            @click="pinMcpSurface(activeMcpSurface.raw)"
          >
            固定到 MCP 面板
          </button>
          <button
            type="button"
            class="chat-mcp-surface-toolbar-btn"
            data-testid="chat-mcp-surface-refresh"
            @click="handleMcpSurfaceRefresh(activeMcpSurface.raw)"
          >
            刷新
          </button>
          <button
            type="button"
            class="chat-mcp-surface-toolbar-btn"
            data-testid="chat-mcp-surface-open-global"
            @click="dispatchOpenMcpDrawer(activeMcpSurface, true)"
          >
            同步到全局 drawer
          </button>
        </div>

        <div v-if="mcpPinnedSurfaces.length" class="chat-mcp-surface-pins">
          <span class="chat-mcp-surface-pins-label">已固定</span>
          <div class="chat-mcp-surface-pin-list">
            <button
              v-for="surface in mcpPinnedSurfaces"
              :key="mcpSurfaceKey(surface)"
              type="button"
              class="chat-mcp-surface-pin-chip"
              :data-testid="`chat-mcp-surface-pin-${mcpSurfaceKey(surface)}`"
              @click="openMcpSurfaceDrawer(surface.raw || surface, { pin: true })"
            >
              {{ surface.title }}
            </button>
          </div>
        </div>

        <div class="chat-mcp-surface-body">
          <McpBundleHost
            v-if="activeMcpSurface.kind === 'bundle'"
            :bundle="activeMcpSurface.raw"
            @action="handleTurnMcpAction"
            @open-detail="handleMcpSurfaceDetail"
            @pin="handleMcpSurfacePin"
          />
          <McpUiCardHost
            v-else
            :card="activeMcpSurface.raw"
            @action="handleTurnMcpAction"
            @detail="handleMcpSurfaceDetail"
            @refresh="handleMcpSurfaceRefresh"
          />
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.reconnect-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px 14px;
  background: #fef3c7;
  color: #92400e;
  font-size: 12px;
  font-weight: 500;
  border-bottom: 1px solid #fde68a;
}

.reconnect-banner.subtle {
  background: #eff6ff;
  color: #1d4ed8;
  border-bottom-color: #bfdbfe;
}

.reconnect-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  border: 1px solid #d97706;
  background: white;
  color: #92400e;
  cursor: pointer;
  margin-left: 8px;
}

.reconnect-btn:hover {
  background: #fef3c7;
}

.workspace-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin: 6px 0 3px;
  padding: 8px 10px;
  width: min(1040px, calc(100% - 40px));
  margin-left: auto;
  margin-right: auto;
  border-radius: 12px;
  border: 1px solid #dbeafe;
  background: linear-gradient(135deg, #eff6ff, #ffffff);
}

.workspace-banner-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #1e293b;
}

.workspace-banner-copy strong {
  font-size: 12px;
  font-weight: 700;
}

.workspace-banner-copy span {
  font-size: 11px;
  color: #475569;
  line-height: 1.45;
}

.workspace-banner-btn {
  flex-shrink: 0;
  border: 1px solid #bfdbfe;
  background: #ffffff;
  color: #1d4ed8;
  border-radius: 999px;
  padding: 7px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.workspace-banner-btn:hover {
  background: #eff6ff;
}

.chat-unread-divider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 2px 0 6px;
}

.chat-unread-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(59, 130, 246, 0.18);
}

.chat-unread-divider-label {
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
}

.chat-unread-divider-count {
  color: #64748b;
  font-size: 12px;
  line-height: 1.4;
}

.chat-virtual-spacer {
  width: 100%;
  flex: none;
  pointer-events: none;
}

.chat-unread-pill {
  position: fixed;
  left: 50%;
  bottom: 118px;
  transform: translateX(-50%);
  z-index: 18;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid rgba(59, 130, 246, 0.18);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.12);
  color: #1d4ed8;
  font-size: 12.5px;
  font-weight: 600;
}

.activity-summary {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 2px 0;
  width: min(1040px, calc(100% - 40px));
  margin: 0 auto;
  animation: fadeInUp 0.2s ease-out;
}

.activity-line {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  padding: 0;
  border: none;
  background: transparent;
  font-size: var(--text-meta-size, 11px);
  color: var(--text-meta, #9ca3af);
  font-weight: 500;
  cursor: pointer;
}

.activity-line:disabled {
  cursor: default;
}

.activity-line:not(:disabled):hover {
  color: #6b7280;
}

.activity-line.plain {
  color: #9ca3af;
}

.activity-details {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 2px;
  padding-left: 10px;
  color: #94a3b8;
  font-size: 11px;
}

.activity-detail-item {
  line-height: 1.45;
}

.chat-terminal-dock-wrap {
  width: 100%;
  margin-bottom: 6px;
}

.chat-terminal-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 4px 6px;
}

.chat-terminal-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: rgba(255, 255, 255, 0.86);
  color: #0f172a;
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.chat-terminal-toggle.active {
  background: #eff6ff;
  color: #1d4ed8;
  border-color: rgba(96, 165, 250, 0.45);
}

.chat-terminal-toggle:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.chat-terminal-toolbar-label {
  flex: 1;
  min-width: 0;
  font-size: 11px;
  color: #64748b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chat-terminal-shortcut {
  flex-shrink: 0;
  font-size: 10px;
  color: #94a3b8;
  font-weight: 700;
}

.chat-terminal-dock {
  display: flex;
  flex-direction: column;
  width: 100%;
  border-radius: 14px;
  overflow: hidden;
  background: rgba(15, 23, 42, 0.98);
  border: 1px solid rgba(148, 163, 184, 0.18);
  box-shadow: 0 14px 36px rgba(15, 23, 42, 0.14);
}

.chat-terminal-dock.dragging {
  box-shadow: 0 22px 56px rgba(15, 23, 42, 0.24);
}

.chat-terminal-resizer {
  border: none;
  width: 100%;
  padding: 7px 12px;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.94), rgba(15, 23, 42, 0.9));
  color: #94a3b8;
  font-size: 11px;
  font-weight: 700;
  cursor: ns-resize;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.chat-terminal-frame {
  flex: 1;
  min-height: 0;
}

.omnibar-stack {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 820px;
  margin: 0 auto;
}

.runtime-plan-dock {
  width: 100%;
  z-index: 6;
  position: relative;
  /* Shift down slightly to cover top border of omnibar if needed, though we set it to transparent anyway */
  transform: translateY(1px);
}

.auth-overlay-dock {
  width: 100%;
  z-index: 10;
  margin-bottom: 0;
  position: relative;
  transform: translateY(1px);
}

.auth-overlay-container {
  background: white;
  border: 1px solid var(--border-color);
  border-radius: 12px;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.08);
  overflow: hidden;
}

.auth-overlay-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 12px;
  border-bottom: 1px solid #f1f5f9;
  background: #f8fafc;
}

.auth-overlay-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.auth-overlay-title {
  font-size: 11px;
  font-weight: 600;
  color: #fb923c;
}

.auth-overlay-queue-label {
  font-size: 10px;
  color: #64748b;
  font-weight: 500;
}

.auth-overlay-queue-note {
  padding: 6px 12px 0;
  font-size: 10px;
  color: #94a3b8;
}

/* chat-mcp-surface-drawer styles removed — now using <n-drawer> */

.chat-mcp-surface-copy {
  display: grid;
  gap: 4px;
}

.chat-mcp-surface-kicker {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.chat-mcp-surface-copy h3 {
  margin: 0;
  font-size: 18px;
  color: #0f172a;
}

.chat-mcp-surface-copy p {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #475569;
}

.chat-mcp-surface-toolbar-btn,
.chat-mcp-surface-pin-chip {
  border: none;
  border-radius: 999px;
  background: rgba(226, 232, 240, 0.9);
  color: #0f172a;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.chat-mcp-surface-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chat-mcp-surface-toolbar-btn {
  padding: 8px 12px;
}

.chat-mcp-surface-pins {
  display: grid;
  gap: 8px;
  padding: 12px;
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.96);
  border: 1px solid rgba(148, 163, 184, 0.18);
}

.chat-mcp-surface-pins-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.chat-mcp-surface-pin-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chat-mcp-surface-pin-chip {
  padding: 7px 12px;
}

.chat-mcp-surface-body {
  display: grid;
  gap: 12px;
}

/* chat-mcp-drawer-fade transition removed — n-drawer handles animation */

.chat-mcp-approval {
  display: grid;
  gap: 12px;
  padding: 12px;
}

.chat-mcp-approval-copy {
  display: grid;
  gap: 4px;
}

.chat-mcp-approval-copy strong {
  font-size: 14px;
  color: #0f172a;
}

.chat-mcp-approval-copy p {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #475569;
}

.chat-mcp-approval-meta {
  display: grid;
  gap: 8px;
  margin: 0;
}

.chat-mcp-approval-row {
  display: grid;
  grid-template-columns: 52px 1fr;
  gap: 10px;
  font-size: 13px;
}

.chat-mcp-approval-row dt {
  color: #64748b;
}

.chat-mcp-approval-row dd {
  margin: 0;
  color: #0f172a;
}

.chat-mcp-approval-actions {
  display: flex;
  gap: 8px;
}

.auth-collapse-btn {
  font-size: 11px;
  color: #64748b;
  background: none;
  border: none;
  cursor: pointer;
}
.auth-collapse-btn:hover {
  text-decoration: underline;
}

.auth-restore-btn {
  width: 100%;
  padding: 10px;
  background: #fff7ed;
  border: 1px solid #fed7aa;
  border-radius: 10px;
  color: #c2410c;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  margin-bottom: 6px;
  box-shadow: 0 3px 10px rgba(234, 88, 12, 0.04);
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.auth-restore-btn:hover {
  background: #ffedd5;
}

.auth-restore-queue {
  font-size: 11px;
  color: #ea580c;
  font-weight: 500;
}

.row-notice {
  justify-content: center;
}

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(4px); }
  to   { opacity: 1; transform: translateY(0); }
}

.chat-container :deep(.avatar),
.chat-container :deep(.user-avatar) {
  display: none;
}

.chat-container :deep(.message-wrapper) {
  gap: 0;
  align-items: flex-start;
}

.chat-container :deep(.stream-row) {
  width: min(920px, 100%);
  margin-inline: auto;
}

.chat-container :deep(.message-content) {
  max-width: min(92ch, 100%) !important;
}

.chat-container :deep(.relative-block) {
  max-width: min(92ch, 100%) !important;
}

.chat-container :deep(.message-text) {
  font-size: 14.5px !important;
  line-height: 1.62 !important;
}

.chat-container :deep(.markdown-body p) {
  margin: 0 0 1px;
}

.chat-container :deep(.markdown-body ul),
.chat-container :deep(.markdown-body ol) {
  margin: 0 0 2px;
}

.chat-container :deep(.is-user .message-content) {
  max-width: min(44ch, 60%) !important;
}

/* ---- Codex Inline Approval (Change 3) ---- */
.codex-approval-inline {
  width: 100%;
  padding: 16px;
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
}

.codex-approval-question {
  font-size: 14px;
  font-weight: 600;
  color: #0f172a;
  margin-bottom: 8px;
}

.codex-approval-command {
  margin-bottom: 12px;
  padding: 8px 12px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.codex-approval-command code {
  font-size: 13px;
  color: #334155;
  font-family: "SF Mono", "Fira Code", "Fira Mono", Menlo, Consolas, monospace;
  word-break: break-all;
  white-space: pre-wrap;
}

.codex-approval-options {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}

.codex-approval-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  color: #374151;
  font-size: 13px;
  line-height: 1.5;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s ease, border-color 0.15s ease;
}

.codex-approval-option:hover {
  background: #f9fafb;
  border-color: #d1d5db;
}

.codex-approval-option .option-number {
  color: #9ca3af;
  font-weight: 600;
  font-size: 12px;
  flex-shrink: 0;
}

.codex-approval-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.codex-skip-btn {
  padding: 7px 16px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  color: #6b7280;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}

.codex-skip-btn:hover {
  background: #f9fafb;
}

.codex-submit-btn {
  padding: 7px 16px;
  border: none;
  border-radius: 8px;
  background: #0f172a;
  color: #ffffff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.codex-submit-btn:hover {
  background: #1e293b;
}

.codex-approval-queue-note {
  margin-top: 8px;
  font-size: 11px;
  color: #9ca3af;
  text-align: center;
}
</style>
