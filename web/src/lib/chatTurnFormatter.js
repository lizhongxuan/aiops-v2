import { buildProtocolConversationItems } from "./protocolWorkspaceVm";
import {
  cleanAssistantDisplayText,
  compactText,
  formatShortTime,
  getWorkspaceCardType,
  isApprovalCard,
  parseTimestamp,
  phaseLabel,
  isAssistantMessageCard,
  isChoiceCard,
  isProcessCard,
  isSystemNoticeCard,
  isUserMessageCard,
  sortProcessDisplayItems,
} from "./workspaceViewModel";
import { adaptMcpUiPayloadFromCard } from "./mcpUiPayloadAdapter";
import { normalizeMcpBundle, normalizeMcpPayloadSource, normalizeMcpUiCard } from "./mcpUiCardModel";
import { resolveMcpBundlePreset } from "./mcpBundleResolver";
import { normalizeToolDisplayPayload } from "./toolDisplayModel";

const ACTIVE_PHASES = new Set(["planning", "thinking", "executing", "waiting_approval", "waiting_input", "finalizing"]);
const PROTOCOL_SURFACE_OWNED_PATTERN = /审批|批准|授权|approval|派发|dispatch|host-agent|worker|时间线|timeline|step\s*->\s*host|host-agent 映射|编排执行计划|接管任务|执行位/i;
const PROTOCOL_SURFACE_DETAIL_PATTERN = /审批ID|风险级别|目标环境|目标范围|影响面|blast\s*radius|dry-?run|验证策略|验证来源|审批上下文|证据摘要|原始 evidence|evidence id|citation|关联证据|时间线事件|事件时间线|timeline event/i;
const STRUCTURED_LIST_PATTERN = /(?:^|\n)\s*(?:[-*]|[0-9]+\.)\s+/m;
const USER_FACING_CONCLUSION_PATTERN = /(?:结论[:：]|结论是|根因[:：]|原因[:：]|建议[:：]|建议先|下一步[:：]|推荐[:：]|因此|意味着)/i;
const MAIN_CHAT_PRELUDE_PATTERN = /^(?:我先|我会先|让我先|先帮你|先查|先看|先抓取|先交叉核对|我先交叉核对|我先整理|我先快速|我正在|先快速|先浏览|先读取)/u;
const MAIN_CHAT_RESULT_PATTERN = /(截至|现价|24h|24小时|数据|结果|摘要|趋势|涨跌|百分比|市值|成交额|支撑|压力|来源[:：]|结论[:：]|建议[:：]|一句话判断|短判断[:：]|简判断[:：])/i;

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function buildToolDisplayModel(detail = null) {
  if (!detail || typeof detail !== "object") return null;
  return normalizeToolDisplayPayload(detail.display || detail.displayOutput || detail.display_output || detail);
}

function buildEvidenceIndex(evidenceSummaries = []) {
  const byId = new Map();
  const byCitationKey = new Map();
  for (const record of asArray(evidenceSummaries)) {
    const evidenceId = compactText(record?.id);
    const citationKey = compactText(record?.citationKey);
    if (evidenceId) byId.set(evidenceId, record);
    if (citationKey) byCitationKey.set(citationKey, record);
  }
  return { byId, byCitationKey };
}

function extractCitationKeys(text = "") {
  const matches = String(text || "").match(/\bE-[A-Z0-9-]+\b/g);
  if (!matches) return [];
  return [...new Set(matches.map((item) => compactText(item)).filter(Boolean))];
}

function normalizeMessageEvidenceRef(record = {}) {
  const evidenceId = compactText(record?.id);
  if (!evidenceId) return null;
  const citationKey = compactText(record?.citationKey || evidenceId);
  return {
    evidenceId,
    citationKey,
    title: compactText(record?.title),
    summary: compactText(record?.summary),
    label: citationKey || evidenceId,
  };
}

function buildMessageEvidenceRefs(sourceCard = {}, evidenceIndex = null) {
  const indexes = evidenceIndex || { byId: new Map(), byCitationKey: new Map() };
  const detail = asObject(sourceCard?.detail);
  const candidateIds = [
    compactText(detail.evidenceId),
    ...asArray(detail.relatedEvidenceIds).map((item) => compactText(item)),
  ].filter(Boolean);
  const candidateCitationKeys = [
    compactText(detail.citationKey),
    ...extractCitationKeys(sourceCard?.text || sourceCard?.message || sourceCard?.summary || ""),
  ].filter(Boolean);

  const refs = [];
  const seen = new Set();
  const pushRecord = (record) => {
    const ref = normalizeMessageEvidenceRef(record);
    if (!ref) return;
    if (seen.has(ref.evidenceId)) return;
    seen.add(ref.evidenceId);
    refs.push(ref);
  };

  candidateIds.forEach((evidenceId) => pushRecord(indexes.byId.get(evidenceId)));
  candidateCitationKeys.forEach((citationKey) => pushRecord(indexes.byCitationKey.get(citationKey)));

  return refs;
}

function messageCardFromConversationItem(item = {}, rawCard = null) {
  const role = compactText(item.role).toLowerCase() === "user" ? "user" : "assistant";
  const id = compactText(item.id || rawCard?.id || `${role}-message`);
  const text = cleanAssistantDisplayText(String(item.text || "").trim(), role);
  const detail = rawCard?.detail || null;
  const hasMcpApp = !!detail?.mcpApp?.html;
  if (!text && !hasMcpApp) return null;
  return {
    id,
    role,
    time: compactText(item.time || formatShortTime(rawCard?.updatedAt || rawCard?.createdAt)),
    createdAt: rawCard?.createdAt || "",
    updatedAt: rawCard?.updatedAt || rawCard?.createdAt || "",
    sourceCard: rawCard || null,
    card: {
      id,
      type: role === "user" ? "UserMessageCard" : "AssistantMessageCard",
      role,
      text,
      status: compactText(rawCard?.status || ""),
      detail,
    },
  };
}

function messageCardFromRawCard(card = {}) {
  const role = isUserMessageCard(card) ? "user" : "assistant";
  const id = compactText(card?.id || `${role}-message`);
  const text = cleanAssistantDisplayText(String(card?.text || card?.message || card?.summary || card?.title || "").trim(), role);
  const detail = card?.detail || null;
  const hasMcpApp = !!detail?.mcpApp?.html;
  if (!text && !hasMcpApp) return null;
  return {
    id,
    role,
    turnId: compactText(card?.turnId),
    clientTurnId: compactText(card?.clientTurnId),
    time: formatShortTime(card?.updatedAt || card?.createdAt),
    createdAt: card?.createdAt || "",
    updatedAt: card?.updatedAt || card?.createdAt || "",
    sourceCard: card || null,
    card: {
      id,
      type: role === "user" ? "UserMessageCard" : "AssistantMessageCard",
      role,
      text,
      status: compactText(card?.status || ""),
      detail,
    },
  };
}

function hasRenderableMessageBody(message = null) {
  return Boolean(compactText(message?.card?.text) || message?.card?.detail?.mcpApp?.html);
}

function messageSortTimestamp(message = null) {
  return parseTimestamp(message?.createdAt || message?.updatedAt || message?.sourceCard?.createdAt || message?.sourceCard?.updatedAt);
}

function sortMessagesByChronology(messages = []) {
  return asArray(messages)
    .map((message, index) => ({ message, index }))
    .sort((left, right) => {
      const leftStamp = messageSortTimestamp(left.message);
      const rightStamp = messageSortTimestamp(right.message);
      if (leftStamp && rightStamp && leftStamp !== rightStamp) return leftStamp - rightStamp;
      if (leftStamp && !rightStamp) return -1;
      if (!leftStamp && rightStamp) return 1;
      return left.index - right.index;
    })
    .map((entry) => entry.message);
}

function messageTurnKeys(message = null) {
  const source = message?.sourceCard || {};
  return [
    message?.turnId,
    message?.clientTurnId,
    source?.turnId,
    source?.clientTurnId,
    source?.detail?.turnId,
    source?.detail?.clientTurnId,
  ].map((value) => compactText(value)).filter(Boolean);
}

function bucketTurnKeys(bucket = null) {
  const keys = [
    ...messageTurnKeys(bucket?.userMessage),
    ...asArray(bucket?.assistantMessages).flatMap((message) => messageTurnKeys(message)),
  ];
  return [...new Set(keys)];
}

function activeProcessTurnKeys(activeProcess = null) {
  if (!activeProcess) return [];
  return [
    activeProcess.turnId,
    activeProcess.clientTurnId,
    activeProcess.currentTurnId,
    ...asArray(activeProcess.turnKeys),
  ].map((value) => compactText(value)).filter(Boolean);
}

function activeProcessMatchesBucket(activeProcess = null, bucket = null) {
  const processKeys = activeProcessTurnKeys(activeProcess);
  if (!processKeys.length) return true;
  const keys = bucketTurnKeys(bucket);
  if (!keys.length) return true;
  return processKeys.some((key) => keys.includes(key));
}

function processToneFromStatus(status) {
  const normalized = compactText(status).toLowerCase();
  if (normalized.includes("fail") || normalized.includes("error") || normalized.includes("permission") || normalized.includes("denied")) return "danger";
  if (normalized.includes("complete") || normalized.includes("done")) return "success";
  if (normalized.includes("wait")) return "warning";
  return "neutral";
}

function stripMatchingQuotes(value = "") {
  const text = String(value || "").trim();
  if (text.length >= 2 && ((text.startsWith("'") && text.endsWith("'")) || (text.startsWith('"') && text.endsWith('"')))) {
    return text.slice(1, -1);
  }
  return text;
}

function displayCommand(value = "") {
  const raw = String(value || "").trim();
  if (!raw) return "";
  const shellMatch = raw.match(/^(?:\/[\w./-]+\/)?(?:zsh|bash|sh)\s+-lc\s+([\s\S]+)$/);
  if (shellMatch) return stripMatchingQuotes(shellMatch[1]);
  return raw;
}

function formatDurationLabel(ms) {
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  if (!totalSeconds) return "";
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts = [];
  if (hours) parts.push(`${hours}h`);
  if (minutes) parts.push(`${minutes}m`);
  if (seconds && !hours) parts.push(`${seconds}s`);
  return parts.join(" ");
}

function activeProcessLabel(phase) {
  switch (compactText(phase).toLowerCase()) {
    case "planning":
      return "正在规划步骤";
    case "thinking":
      return "正在思考";
    case "waiting_approval":
      return "等待审批";
    case "waiting_input":
      return "等待补充输入";
    case "finalizing":
      return "正在汇总结果";
    default:
      return "处理中";
  }
}

function latestTimestamp(values = []) {
  let latest = 0;
  for (const value of values) {
    const stamp = parseTimestamp(value);
    if (stamp > latest) latest = stamp;
  }
  return latest;
}

function deriveLiveHint({ statusCard = null, missionPhase = "", approvalItems = [] } = {}) {
  const explicitHint = compactText(statusCard?.hint);
  if (explicitHint) return explicitHint;

  const normalizedPhase = compactText(statusCard?.phase || missionPhase).toLowerCase();
  if (normalizedPhase === "waiting_approval" && approvalItems.length) {
    return "等待审批后继续推进。";
  }
  return compactText(phaseLabel(normalizedPhase))
    .replace("主 Agent ", "")
    .replace("生成计划", "正在规划步骤");
}

function buildAssistantProcessItems(messages = [], options = {}) {
  const exclude = typeof options.exclude === "function" ? options.exclude : () => false;
  return asArray(messages).map((message, index) => ({
    id: `${message.id || "assistant"}-process-${index}`,
    kind: "assistant_message",
    processKind: inferProcessKind(message.card?.text || ""),
    text: String(message.card?.text || "").trim(),
    detail: "",
    display: buildToolDisplayModel(message.card?.detail),
    time: compactText(message.time),
    hostId: "",
    tone: "neutral",
    status: "",
    sortTimestamp: message.updatedAt || message.createdAt || "",
  })).filter((item) => item.text && !exclude(item));
}

function buildProcessLineItems(processCards = [], options = {}) {
  const exclude = typeof options.exclude === "function" ? options.exclude : () => false;
  return asArray(processCards).map((card, index) => {
    const hostId = compactText(card?.hostId || card?.title);
    const primary = compactText(card?.text || card?.summary);
    const secondary = compactText(card?.summary && card?.summary !== card?.text ? card.summary : "");
    const item = {
      id: compactText(card?.id || `process-${index}`),
      kind: "process_line",
      processKind: inferProcessKind(`${card?.title || ""} ${primary} ${secondary}`),
      text: primary,
      detail: secondary,
      display: buildToolDisplayModel(card?.detail),
      time: formatShortTime(card?.updatedAt || card?.createdAt),
      hostId,
      tone: processToneFromStatus(card?.status),
      status: compactText(card?.status),
      sortTimestamp: card?.updatedAt || card?.createdAt || "",
    };
    if (!item.text || exclude(item, card)) {
      return null;
    }
    return item;
  }).filter(Boolean);
}

function buildCommandProcessItems(commandCards = []) {
  return asArray(commandCards)
    .filter((card) => compactText(card?.command))
    .map((card, index) => {
      const command = displayCommand(card.command);
      const status = compactText(card?.status).toLowerCase();
      const running = status.includes("run") || status.includes("progress");
      const failed = status.includes("fail") || status.includes("error");
      const denied = status.includes("permission") || status.includes("denied");
      const cancelled = status.includes("cancel");
      let text = `已运行 ${command}`;
      if (running) text = `正在运行 ${command}`;
      else if (denied) text = `权限不足 ${command}`;
      else if (cancelled) text = `已停止 ${command}`;
      else if (failed) text = `运行失败 ${command}`;
      return {
        id: `command-${compactText(card?.id || index)}`,
        kind: "command",
        processKind: "command",
        text,
        detail: compactText(card?.output || card?.stdout || card?.stderr || card?.text || card?.summary),
        display: buildToolDisplayModel(card?.detail),
        time: formatShortTime(card?.updatedAt || card?.createdAt),
        hostId: compactText(card?.hostId),
        tone: processToneFromStatus(card?.status),
        status: compactText(card?.status),
        sortTimestamp: card?.updatedAt || card?.createdAt || "",
        command,
        output: compactText(card?.output || card?.stdout || card?.stderr || card?.text || card?.summary),
        commandCard: card,
      };
    });
}

function inferProcessKind(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (!normalized) return "notice";
  if (/读取|浏览|打开|read|viewed file|file/.test(normalized)) return "read";
  if (/搜索|search|grep|query/.test(normalized)) return "search";
  if (/\bls\b|list|列出|枚举/.test(normalized)) return "list";
  if (/command|terminal|bash|systemctl|journalctl|npm |go run|执行/.test(normalized)) return "command";
  if (/thinking|思考|分析|整理/.test(normalized)) return "thinking";
  if (/agent|worker|host-agent|background/.test(normalized)) return "agent_status";
  return "notice";
}

export function classifyChatCardSemantic(card = {}) {
  const type = getWorkspaceCardType(card);
  const text = compactText(card?.text || card?.message || card?.summary || card?.title);
  if (isUserMessageCard(card)) {
    return { layer: "user", semantic: "user", processKind: "" };
  }
  if (isApprovalCard(card)) {
    return { layer: "blocking", semantic: "approval", processKind: "" };
  }
  if (isChoiceCard(card)) {
    return { layer: "blocking", semantic: "waiting_input", processKind: "" };
  }
  if (type === "ErrorCard") {
    return { layer: "blocking", semantic: "error", processKind: "" };
  }
  if (isProcessCard(card)) {
    return { layer: "process", semantic: "process", processKind: inferProcessKind(text) };
  }
  if (isSystemNoticeCard(card)) {
    return { layer: "process", semantic: "notice", processKind: "notice" };
  }
  if (isAssistantMessageCard(card)) {
    const cleaned = cleanAssistantDisplayText(text, "assistant");
    if (!cleaned) {
      return { layer: "hidden", semantic: "internal_routing", processKind: "notice", shouldHide: true };
    }
    return { layer: "final", semantic: "assistant_final", processKind: "" };
  }
  return { layer: "other", semantic: type || "unknown", processKind: "" };
}

function looksLikeProtocolSurfaceOwnedCopy(value = "") {
  return PROTOCOL_SURFACE_OWNED_PATTERN.test(compactText(value));
}

function inferProtocolSurfaceDetailKinds(value = "", detail = {}) {
  const text = String(value || "");
  const normalized = compactText(text);
  const detailObject = asObject(detail);
  const kinds = new Set();

  if (
    compactText(detailObject.approvalId) ||
    /审批ID|风险级别|目标环境|目标范围|影响面|dry-?run|验证策略|验证来源|审批上下文|approval/i.test(text)
  ) {
    kinds.add("approval");
  }
  if (
    compactText(detailObject.evidenceId) ||
    compactText(detailObject.citationKey) ||
    asArray(detailObject.relatedEvidenceIds).length ||
    /证据摘要|原始 evidence|evidence id|citation|关联证据|evidence/i.test(text)
  ) {
    kinds.add("evidence");
  }
  if (/时间线|timeline|事件流|timeline event|incident event/i.test(text)) {
    kinds.add("timeline");
  }

  if (!kinds.size && looksLikeProtocolSurfaceOwnedCopy(normalized)) {
    kinds.add("approval");
  }
  return [...kinds];
}

function looksLikeProtocolSurfaceDetailCopy(value = "", detail = {}) {
  const raw = String(value || "").trim();
  if (!raw) return false;
  if (USER_FACING_CONCLUSION_PATTERN.test(raw)) return false;

  const compact = compactText(raw);
  const lineCount = raw.split(/\n+/).filter(Boolean).length;
  const structuredFields = PROTOCOL_SURFACE_DETAIL_PATTERN.test(raw);
  const structuredList = STRUCTURED_LIST_PATTERN.test(raw) || (raw.match(/[：:]/g) || []).length >= 3;
  const kinds = inferProtocolSurfaceDetailKinds(raw, detail);

  if (!kinds.length) return false;
  return (structuredFields || structuredList) && (compact.length >= 72 || lineCount >= 3 || kinds.length > 1);
}

function compactProtocolSurfaceMessageText(value = "", detail = {}) {
  const kinds = inferProtocolSurfaceDetailKinds(value, detail);
  if (!kinds.length) return "";

  const labels = kinds.map((kind) => {
    switch (kind) {
      case "approval":
        return "审批";
      case "evidence":
        return "证据";
      case "timeline":
        return "时间线";
      default:
        return "详情";
    }
  });

  if (labels.length === 1) {
    return `${labels[0]}详情已收进对应面板，可在详情里继续查看。`;
  }
  return `${labels.join("、")}详情已收进对应面板，可在详情里继续查看。`;
}

function compactProtocolFinalMessage(message = null) {
  if (!message?.sourceCard || message.sourceCard.type !== "AssistantMessageCard") return message;
  if (message?.card?.detail?.mcpApp?.html) return message;

  const rawText = String(
    message.sourceCard?.text ||
    message.sourceCard?.summary ||
    message.sourceCard?.message ||
    message.sourceCard?.title ||
    message.card?.text ||
    "",
  ).trim();
  if (!looksLikeProtocolSurfaceDetailCopy(rawText, message.sourceCard?.detail)) return message;

  const compactedText = compactProtocolSurfaceMessageText(rawText, message.sourceCard?.detail);
  if (!compactedText) return message;

  return {
    ...message,
    card: {
      ...message.card,
      text: compactedText,
    },
  };
}

function normalizeComparableMessageText(value = "") {
  return compactText(String(value || "").replace(/\*\*/g, "")).replace(/\s+/g, "");
}

function looksLikeMainChatPrelude(text = "") {
  const value = compactText(text);
  if (!value) return false;
  return (
    MAIN_CHAT_PRELUDE_PATTERN.test(value) &&
    !STRUCTURED_LIST_PATTERN.test(value) &&
    !/\n/.test(value)
  );
}

function looksLikeMainChatResult(text = "") {
  const value = String(text || "");
  if (!value.trim()) return false;
  return (
    MAIN_CHAT_RESULT_PATTERN.test(value) ||
    (STRUCTURED_LIST_PATTERN.test(value) && value.trim().length >= 48) ||
    /(?:^|\n)(?:来源[:：]|https?:\/\/|\[[^\]]+\]\([^)]+\))/m.test(value)
  );
}

export function shouldExposeActiveFinalMessage(message = null) {
  const text = compactText(message?.card?.text || message?.text);
  if (!text) return false;
  if (looksLikeMainChatPrelude(text)) return false;

  const status = compactText(message?.card?.status || message?.sourceCard?.status).toLowerCase();
  if (status === "inprogress" || status === "streaming") {
    return true;
  }
  return looksLikeMainChatResult(text) || text.length >= 96;
}

function isDuplicateAssistantDraft(text = "", finalText = "") {
  const left = normalizeComparableMessageText(text);
  const right = normalizeComparableMessageText(finalText);
  return Boolean(left) && Boolean(right) && (left === right || right.includes(left) || left.includes(right));
}

function isMainChatAssistantProcessRedundant(message = {}, finalMessageText = "") {
  const text = compactText(message?.card?.text || message?.text);
  if (!text) return true;
  if (looksLikeMainChatPrelude(text)) return false;
  if (looksLikeMainChatResult(text)) return true;
  if (finalMessageText && isDuplicateAssistantDraft(text, finalMessageText)) return true;
  return USER_FACING_CONCLUSION_PATTERN.test(text);
}

function isProtocolAssistantProcessRedundant(item = {}) {
  const text = [item.text, item.detail].filter(Boolean).join(" ");
  if (looksLikeProtocolSurfaceDetailCopy(text)) {
    return true;
  }
  return looksLikeProtocolSurfaceOwnedCopy(text);
}

function isProtocolProcessCardRedundant(item = {}, card = {}) {
  const text = [item.text, item.detail, item.status, card?.summary, card?.title].filter(Boolean).join(" ");
  if (/^已处理\s*\d+\s*个命令$/.test(compactText(item.text))) {
    return true;
  }
  if (looksLikeProtocolSurfaceOwnedCopy(text)) {
    return true;
  }
  if (looksLikeProtocolSurfaceDetailCopy(text, card?.detail)) {
    return true;
  }
  const status = compactText(card?.status || item.status).toLowerCase();
  const hostId = compactText(card?.hostId || item.hostId);
  if (!hostId) {
    return false;
  }
  return ["wait", "approval", "progress", "running", "queued", "dispatch", "complete"].some((keyword) => status.includes(keyword));
}

function summarizeTurnProcess({ processItems = [], activeTurn = false, liveHint = "" } = {}) {
  const assistantUpdates = asArray(processItems).filter((item) => item.kind === "assistant_message").length;
  if (assistantUpdates > 1) {
    return `已记录 ${assistantUpdates} 条过程消息`;
  }
  if (!activeTurn && liveHint) return liveHint;
  return "";
}

function summarizeProtocolTurnProcess({ processItems = [], missionPhase = "", activeTurn = false, liveHint = "" } = {}) {
  if (!activeTurn) {
    const itemCount = asArray(processItems).length;
    if (!itemCount) return liveHint || "已完成";
    if (itemCount === 1) return "已记录 1 条过程细项";
    return `已记录 ${itemCount} 条过程细项`;
  }

  const phase = compactText(missionPhase).toLowerCase();
  if (phase === "waiting_approval") {
    return "审批详情已收进右侧审批面板";
  }
  if (phase === "planning") {
    return "计划细节已收进计划与证据面板";
  }
  if (phase === "executing" || phase === "thinking" || phase === "finalizing") {
    return processItems.length ? "执行细节已收进右侧执行面板" : "";
  }
  return liveHint ? "" : summarizeTurnProcess({ processItems, activeTurn, liveHint });
}

function summarizeMainChatProcess({ processItems = [], activeProcess = null, liveHint = "" } = {}) {
  const explicitSummary = compactText(activeProcess?.summary);
  const normalizedItems = asArray(processItems);
  const itemCount = normalizedItems.length;
  if (!itemCount) return explicitSummary || "";
  const groupedSummary = summarizeProcessItemGroups(normalizedItems);
  if (!liveHint && groupedSummary) return groupedSummary;
  if (explicitSummary) return explicitSummary;
  if (liveHint) return "";
  const searchCount = normalizedItems.filter((item) => {
    const kind = compactText(item?.kind || item?.processKind).toLowerCase();
    const text = compactText(item?.text).toLowerCase();
    return kind === "search" || text.includes("已搜索");
  }).length;
  if (searchCount) {
    return `已搜索 ${searchCount} 次`;
  }
  const assistantCount = normalizedItems.filter((item) => {
    const kind = compactText(item?.kind).toLowerCase();
    return kind === "assistant" || kind === "assistant_message" || kind === "message";
  }).length;
  if (assistantCount) {
    return `上 ${assistantCount} 条消息`;
  }
  const commandCount = normalizedItems.filter((item) => compactText(item?.kind).toLowerCase() === "command").length;
  if (commandCount) {
    return `已执行 ${commandCount} 个命令`;
  }
  if (itemCount === 1) return "已记录 1 条过程细项";
  return `已记录 ${itemCount} 条过程细项`;
}

const PROCESS_SUMMARY_ORDER = [
  "web_search",
  "page_browse",
  "page_search",
  "content_search",
  "file_read",
  "directory_browse",
  "command",
  "file_change",
  "assistant_note",
  "tool",
];

const PROCESS_SUMMARY_LABELS = {
  web_search: "搜索网页",
  page_browse: "浏览网页",
  page_search: "检索页面",
  content_search: "搜索内容",
  file_read: "读取文件",
  directory_browse: "浏览目录",
  command: "运行命令",
  file_change: "修改文件",
  assistant_note: "记录过程说明",
  tool: "调用工具",
};

function classifyProcessSummaryGroup(item = {}) {
  const text = compactText(item?.text || item?.detail);
  const kind = compactText(item?.kind).toLowerCase();
  const processKind = compactText(item?.processKind).toLowerCase();
  const haystack = `${kind} ${processKind} ${text}`.toLowerCase();

  if (/搜索网页|web[_\s-]?search|search_web/.test(haystack)) return "web_search";
  if (/浏览网页|open_page|网页|url|https?:\/\//.test(haystack)) return "page_browse";
  if (/检索页面|页面中搜索|find_in_page/.test(haystack)) return "page_search";
  if (/搜索内容|搜索文件|search_files|grep|query/.test(haystack)) return "content_search";
  if (/读取文件|read_file|file_read/.test(haystack)) return "file_read";
  if (/浏览目录|列出|list_dir|list_files|\bls\b/.test(haystack)) return "directory_browse";
  if (/运行命令|已运行|正在运行|exec_command|terminal|shell|command/.test(haystack)) return "command";
  if (/修改文件|apply_patch|write_file|diff|patch/.test(haystack)) return "file_change";
  if (kind === "assistant" || kind === "assistant_message" || kind === "message") return "assistant_note";
  if (kind === "activity" || kind === "status" || kind === "system" || processKind === "activity" || processKind === "status" || processKind === "system") return "";
  if (kind || processKind) return "tool";
  return "";
}

function statusVerbForProcessItems(items = []) {
  const statuses = items.map((item) => compactText(item?.status).toLowerCase()).filter(Boolean);
  if (statuses.some((status) => status.includes("fail") || status.includes("error") || status.includes("denied"))) {
    return "失败";
  }
  if (statuses.some((status) => status.includes("run") || status.includes("progress") || status.includes("queued"))) {
    return "正在";
  }
  return "已";
}

function summarizeProcessItemGroups(processItems = []) {
  const grouped = new Map();
  for (const item of asArray(processItems)) {
    if (!compactText(item?.text) && !item?.display) continue;
    const group = classifyProcessSummaryGroup(item);
    if (!group) continue;
    const bucket = grouped.get(group) || [];
    bucket.push(item);
    grouped.set(group, bucket);
  }
  if (!grouped.size) return "";
  if ([...grouped.keys()].every((group) => group === "assistant_note")) return "";

  const orderedGroups = PROCESS_SUMMARY_ORDER.filter((group) => grouped.has(group));
  const extraGroups = [...grouped.keys()].filter((group) => !PROCESS_SUMMARY_ORDER.includes(group));
  const phrases = [...orderedGroups, ...extraGroups].map((group) => {
    const items = grouped.get(group) || [];
    const verb = statusVerbForProcessItems(items);
    const label = PROCESS_SUMMARY_LABELS[group] || "处理细项";
    return `${verb}${label} ${items.length} 次`;
  });
  const visiblePhrases = phrases.slice(0, 3);
  const hiddenCount = phrases.length - visiblePhrases.length;
  const suffix = hiddenCount > 0 ? `，另有 ${hiddenCount} 类明细` : "";
  return `${visiblePhrases.join("，")}${suffix}；明细已折叠。`;
}

function isAttentionPhase(phase = "") {
  const normalized = compactText(phase).toLowerCase();
  return ["failed", "aborted", "waiting_approval", "waiting_input"].includes(normalized);
}

function resolveTerminalProcessLabel(phase = "", elapsedLabel = "") {
  const normalized = compactText(phase).toLowerCase();
  if (normalized === "failed") {
    return ["失败", elapsedLabel].filter(Boolean).join(" ");
  }
  if (normalized === "aborted") {
    return ["已停止", elapsedLabel].filter(Boolean).join(" ");
  }
  return "";
}

function createTurnBucket(seedMessage = null) {
  const id = compactText(seedMessage?.id || `turn-${Date.now()}`);
  return {
    id: `turn-${id}`,
    userMessage: seedMessage?.role === "user" ? seedMessage : null,
    assistantMessages: seedMessage?.role === "assistant" ? [seedMessage] : [],
  };
}

function pushTurn(turns, bucket) {
  if (!bucket) return;
  if (!bucket.userMessage && !bucket.assistantMessages.length) return;
  turns.push(bucket);
}

function rememberBucketTurnKeys(bucketIndex, bucketKeyMap, bucket) {
  for (const key of bucketTurnKeys(bucket)) {
    if (!bucketKeyMap.has(key)) {
      bucketKeyMap.set(key, bucketIndex);
    }
  }
}

function findBucketIndexForMessageTurnKeys(bucketKeyMap, message = null) {
  for (const key of messageTurnKeys(message)) {
    if (bucketKeyMap.has(key)) {
      return bucketKeyMap.get(key);
    }
  }
  return -1;
}

export function collectMcpUiSurfaceEntries(cards = []) {
  return asArray(cards).flatMap((card, index) => adaptMcpUiPayloadFromCard(card, index).items);
}

function createTurnSurfaceDefaults(card = {}, sourceCardId = "") {
  return {
    sourceCardId: compactText(sourceCardId || card?.id),
    placement: compactText(card?.placement || "inline_final"),
    source: card?.source,
    mcpServer: card?.mcpServer || card?.mcp_server,
    scope: card?.scope,
    freshness: card?.freshness,
    errors: [
      card?.error,
      ...(Array.isArray(card?.errors) ? card.errors : card?.errors ? [card.errors] : []),
    ],
  };
}

function createTurnCardEntry(payload = {}, defaults = {}, index = 0) {
  const model = normalizeMcpUiCard(
    {
      ...payload,
      id: compactText(payload?.id || `${compactText(defaults.sourceCardId || "mcp-ui-card")}-${index + 1}`),
    },
    defaults,
  );
  return {
    id: model.id,
    kind: "mcp_ui_card",
    placement: model.placement,
    source: normalizeMcpPayloadSource(model.source),
    mcpServer: compactText(model.mcpServer),
    freshness: model.freshness,
    scope: model.scope,
    errors: model.errors,
    sourceCardId: compactText(defaults.sourceCardId),
    model,
  };
}

function createTurnBundleEntry(payload = {}, defaults = {}, index = 0) {
  const resolvedPreset = resolveMcpBundlePreset(payload, {
    scope: payload?.scope || defaults.scope,
    bundleKind: payload?.bundleKind || payload?.bundle_kind,
  });
  const model = normalizeMcpBundle(
    {
      ...payload,
      bundleId: compactText(payload?.bundleId || payload?.bundle_id || payload?.id || `${compactText(defaults.sourceCardId || "mcp-bundle")}-${index + 1}`),
      bundleKind: compactText(payload?.bundleKind || payload?.bundle_kind || resolvedPreset.bundleKind),
      sections: asArray(payload?.sections).length ? payload.sections : resolvedPreset.sections,
      scope: payload?.scope || resolvedPreset.scope,
    },
    defaults,
  );
  return {
    id: model.bundleId,
    kind: "mcp_bundle",
    placement: compactText(payload?.placement || defaults.placement || "inline_final") || "inline_final",
    source: normalizeMcpPayloadSource(model.source),
    mcpServer: compactText(model.mcpServer),
    freshness: model.freshness,
    scope: model.scope,
    errors: model.errors,
    sourceCardId: compactText(defaults.sourceCardId),
    model,
  };
}

function pushTurnSurface(target, entry, keys) {
  if (!entry) return;
  const key = `${entry.kind}:${entry.id}:${entry.placement}:${entry.sourceCardId || ""}`;
  if (keys.has(key)) return;
  keys.add(key);
  target.push(entry);
}

function readTurnSurfacePayload(card = {}) {
  const payload = card?.payload && typeof card.payload === "object" ? card.payload : {};
  return {
    resultAttachments: asArray(payload.resultAttachments || payload.result_attachments),
    actionSurfaces: asArray(payload.actionSurfaces || payload.action_surfaces),
    workspaceSurfaces: asArray(payload.workspaceSurfaces || payload.workspace_surfaces),
    resultBundles: asArray(payload.resultBundles || payload.result_bundles),
    actionBundles: asArray(payload.actionBundles || payload.action_bundles),
  };
}

function classifyTurnSurfaceEntry(groups, entry, keys) {
  if (!entry) return;
  if (entry.kind === "mcp_bundle") {
    if (entry.placement === "inline_action") {
      pushTurnSurface(groups.actionBundles, entry, keys);
      return;
    }
    pushTurnSurface(groups.resultBundles, entry, keys);
    return;
  }
  if (entry.placement === "inline_action") {
    pushTurnSurface(groups.actionSurfaces, entry, keys);
    return;
  }
  if (["side_panel", "drawer", "modal"].includes(entry.placement)) {
    pushTurnSurface(groups.workspaceSurfaces, entry, keys);
    return;
  }
  pushTurnSurface(groups.resultAttachments, entry, keys);
}

function collectTurnMcpSurfaceGroups(sourceCards = []) {
  const groups = {
    resultAttachments: [],
    actionSurfaces: [],
    workspaceSurfaces: [],
    resultBundles: [],
    actionBundles: [],
  };
  const keys = new Set();

  asArray(sourceCards).forEach((card, cardIndex) => {
    if (!card || typeof card !== "object") return;
    const defaults = createTurnSurfaceDefaults(card, card?.id || `turn-source-${cardIndex + 1}`);
    const explicit = readTurnSurfacePayload(card);

    explicit.resultAttachments.forEach((item, index) => {
      classifyTurnSurfaceEntry(groups, createTurnCardEntry(item, defaults, index), keys);
    });
    explicit.actionSurfaces.forEach((item, index) => {
      classifyTurnSurfaceEntry(groups, createTurnCardEntry({ ...item, placement: item?.placement || "inline_action" }, defaults, index), keys);
    });
    explicit.workspaceSurfaces.forEach((item, index) => {
      classifyTurnSurfaceEntry(groups, createTurnCardEntry({ ...item, placement: item?.placement || "drawer" }, defaults, index), keys);
    });
    explicit.resultBundles.forEach((item, index) => {
      classifyTurnSurfaceEntry(groups, createTurnBundleEntry(item, defaults, index), keys);
    });
    explicit.actionBundles.forEach((item, index) => {
      classifyTurnSurfaceEntry(groups, createTurnBundleEntry({ ...item, placement: item?.placement || "inline_action" }, defaults, index), keys);
    });

    adaptMcpUiPayloadFromCard(card, cardIndex).items.forEach((entry) => {
      classifyTurnSurfaceEntry(groups, entry, keys);
    });
  });

  return groups;
}

export function formatProtocolChatTurns({
  conversationCards = [],
  processCards = [],
  commandCards = [],
  evidenceSummaries = [],
  missionPhase = "idle",
  turnActive = false,
  statusCard = null,
  approvalItems = [],
} = {}) {
  const conversationItems = buildProtocolConversationItems(conversationCards);
  const rawCardById = new Map(asArray(conversationCards).map((card) => [compactText(card?.id), card]));
  const evidenceIndex = buildEvidenceIndex(evidenceSummaries);
  const normalizedMessages = sortMessagesByChronology(conversationItems
    .map((item) => messageCardFromConversationItem(item, rawCardById.get(compactText(item.id))))
    .filter((item) => hasRenderableMessageBody(item)));

  const buckets = [];
  let currentBucket = null;
  const bucketKeyMap = new Map();

  for (const message of normalizedMessages) {
    if (message.role === "user") {
      pushTurn(buckets, currentBucket);
      currentBucket = createTurnBucket(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
      continue;
    }
    const matchingBucketIndex = findBucketIndexForMessageTurnKeys(bucketKeyMap, message);
    if (matchingBucketIndex >= 0 && buckets[matchingBucketIndex]) {
      buckets[matchingBucketIndex].assistantMessages.push(message);
      rememberBucketTurnKeys(matchingBucketIndex, bucketKeyMap, buckets[matchingBucketIndex]);
      continue;
    }
    if (!currentBucket) {
      currentBucket = createTurnBucket(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
    } else {
      currentBucket.assistantMessages.push(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
    }
  }
  pushTurn(buckets, currentBucket);

  return buckets.map((bucket, index) => {
    const isCurrentTurn = index === buckets.length - 1;
    const isActiveTurn = isCurrentTurn && Boolean(turnActive) && ACTIVE_PHASES.has(compactText(missionPhase).toLowerCase());
    const assistantMessages = asArray(bucket.assistantMessages);
    const rawFinalMessage = isActiveTurn ? null : assistantMessages[assistantMessages.length - 1] || null;
    const finalMessage = rawFinalMessage
      ? compactProtocolFinalMessage({
          ...rawFinalMessage,
          evidenceRefs: buildMessageEvidenceRefs(rawFinalMessage.sourceCard, evidenceIndex),
        })
      : null;
    const assistantProcessMessages = isActiveTurn ? assistantMessages : assistantMessages.slice(0, -1);
    // Include process cards for the last turn even after completion so the fold persists.
    const activeProcessCards = isCurrentTurn ? asArray(processCards) : [];
    const activeCommandCards = isCurrentTurn ? asArray(commandCards) : [];
    const processItems = sortProcessDisplayItems([
      ...buildAssistantProcessItems(assistantProcessMessages, {
        exclude: isProtocolAssistantProcessRedundant,
      }),
      ...buildCommandProcessItems(activeCommandCards),
      ...buildProcessLineItems(activeProcessCards, {
        exclude: isProtocolProcessCardRedundant,
      }),
    ]);
    const liveHint = isActiveTurn
      ? deriveLiveHint({
          statusCard,
          missionPhase,
          approvalItems,
        })
      : "";
    const timestamps = [
      bucket.userMessage?.updatedAt,
      bucket.userMessage?.createdAt,
      finalMessage?.updatedAt,
      finalMessage?.createdAt,
      ...assistantProcessMessages.flatMap((message) => [message.updatedAt, message.createdAt]),
      ...activeCommandCards.flatMap((card) => [card?.updatedAt, card?.createdAt]),
      ...activeProcessCards.flatMap((card) => [card?.updatedAt, card?.createdAt]),
    ];
    const firstTimestamp = parseTimestamp(bucket.userMessage?.createdAt || bucket.userMessage?.updatedAt || assistantMessages[0]?.createdAt);
    const lastTimestamp = latestTimestamp(timestamps);
    const elapsedLabel = firstTimestamp && lastTimestamp && lastTimestamp >= firstTimestamp
      ? formatDurationLabel(lastTimestamp - firstTimestamp)
      : "";
    const normalizedPhase = compactText(missionPhase).toLowerCase();
    const terminalProcessLabel = resolveTerminalProcessLabel(normalizedPhase, elapsedLabel);
    const processLabel = terminalProcessLabel || [isActiveTurn ? activeProcessLabel(normalizedPhase) : "已处理", elapsedLabel].filter(Boolean).join(" ");
    const summary = summarizeProtocolTurnProcess({
      processItems,
      missionPhase: normalizedPhase,
      activeTurn: isActiveTurn,
      liveHint,
    });
    const turnSurfaces = collectTurnMcpSurfaceGroups(assistantMessages.map((message) => message.sourceCard).filter(Boolean));

    return {
      id: bucket.id,
      anchorMessageId: bucket.userMessage?.id || assistantMessages[0]?.id || bucket.id,
      messageIds: [bucket.userMessage?.id, ...assistantMessages.map((message) => message.id)].filter(Boolean),
      userMessage: bucket.userMessage,
      finalMessage,
      processItems,
      processLabel,
      finalLabel: finalMessage && (processItems.length || liveHint) ? "最终消息" : "",
      liveHint,
      summary,
      collapsedByDefault: isAttentionPhase(normalizedPhase)
        ? false
        : !isActiveTurn && Boolean(processItems.length),
      active: isActiveTurn,
      phase: normalizedPhase,
      resultAttachments: turnSurfaces.resultAttachments,
      actionSurfaces: turnSurfaces.actionSurfaces,
      workspaceSurfaces: turnSurfaces.workspaceSurfaces,
      resultBundles: turnSurfaces.resultBundles,
      actionBundles: turnSurfaces.actionBundles,
    };
  });
}

export function isChatConversationCard(card = {}) {
  return isUserMessageCard(card) || isAssistantMessageCard(card);
}

export function formatMainChatTurns({
  conversationCards = [],
  commandCards = [],
  turnActive = false,
  activeProcess = null,
  hideLiveProcessDetails = false,
} = {}) {
  const normalizedMessages = sortMessagesByChronology(asArray(conversationCards)
    .map((card) => messageCardFromRawCard(card))
    .filter((message) => hasRenderableMessageBody(message)));

  const buckets = [];
  let currentBucket = null;
  const bucketKeyMap = new Map();

  for (const message of normalizedMessages) {
    if (message.role === "user") {
      pushTurn(buckets, currentBucket);
      currentBucket = createTurnBucket(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
      continue;
    }
    const matchingBucketIndex = findBucketIndexForMessageTurnKeys(bucketKeyMap, message);
    if (matchingBucketIndex >= 0 && buckets[matchingBucketIndex]) {
      buckets[matchingBucketIndex].assistantMessages.push(message);
      rememberBucketTurnKeys(matchingBucketIndex, bucketKeyMap, buckets[matchingBucketIndex]);
      continue;
    }
    if (!currentBucket) {
      currentBucket = createTurnBucket(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
    } else {
      currentBucket.assistantMessages.push(message);
      rememberBucketTurnKeys(buckets.length, bucketKeyMap, currentBucket);
    }
  }
  pushTurn(buckets, currentBucket);

  return buckets.map((bucket, index) => {
    const isCurrentTurn = index === buckets.length - 1;
    const isActiveTurn = isCurrentTurn && Boolean(turnActive);
    const assistantMessages = asArray(bucket.assistantMessages);
    const lastAssistantMessage = assistantMessages[assistantMessages.length - 1] || null;
    const activeFinalMessage = isActiveTurn && shouldExposeActiveFinalMessage(lastAssistantMessage)
      ? lastAssistantMessage
      : null;
    const hasActiveFinalMessage = Boolean(activeFinalMessage);
    const suppressLiveProcessNarration = Boolean(hideLiveProcessDetails) && isActiveTurn;
    const suppressLiveProcessDetails = Boolean(hideLiveProcessDetails) && isActiveTurn && !hasActiveFinalMessage;
    const processMatchesBucket = activeProcessMatchesBucket(activeProcess, bucket);
    const finalMessage = isActiveTurn ? activeFinalMessage : lastAssistantMessage;
    const rawAssistantProcessMessages = isActiveTurn
      ? (activeFinalMessage ? assistantMessages.slice(0, -1) : assistantMessages)
      : assistantMessages.slice(0, -1);
    const bucketStart = parseTimestamp(
      bucket.userMessage?.createdAt || bucket.userMessage?.updatedAt || assistantMessages[0]?.createdAt || assistantMessages[0]?.updatedAt,
    );
    const nextBucketStart = parseTimestamp(
      buckets[index + 1]?.userMessage?.createdAt ||
      buckets[index + 1]?.userMessage?.updatedAt ||
      buckets[index + 1]?.assistantMessages?.[0]?.createdAt ||
      buckets[index + 1]?.assistantMessages?.[0]?.updatedAt,
    );
    const bucketCommandCards = asArray(commandCards).filter((card) => {
      const compareAt = parseTimestamp(card?.startedAt || card?.updatedAt || card?.createdAt || card?.completedAt);
      if (!bucketStart && !nextBucketStart) return isCurrentTurn;
      if (!compareAt) return isCurrentTurn;
      if (bucketStart && compareAt < bucketStart) return false;
      if (nextBucketStart && compareAt >= nextBucketStart) return false;
      return true;
    });
    const finalMessageText = compactText(finalMessage?.card?.text || "");
    const assistantProcessMessages = rawAssistantProcessMessages.filter((message) => !isMainChatAssistantProcessRedundant(message, finalMessageText));
    // Include activity process items for both active and completed turns
    // so the "已处理" fold persists after the turn completes.
    const activityProcessItems = !suppressLiveProcessDetails && processMatchesBucket && (isActiveTurn || isCurrentTurn)
      ? asArray(activeProcess?.items).map((item, itemIndex) => ({
          id: compactText(item?.id || `activity-${itemIndex}`),
          kind: compactText(item?.kind || "activity"),
          processKind: inferProcessKind(`${item?.kind || ""} ${item?.text || item?.label || item?.value || ""}`),
          text: String(item?.text || item?.label || item?.value || "").trim(),
          detail: String(item?.detail || "").trim(),
          display: buildToolDisplayModel(item?.detail),
          time: compactText(item?.time),
          hostId: compactText(item?.hostId),
          tone: compactText(item?.tone || "neutral"),
          status: compactText(item?.status),
          sortTimestamp: item?.updatedAt || item?.createdAt || "",
        })).filter((item) => item.text)
      : [];
    // Include intermediate assistant messages with card property so ChatProcessFold
    // can render them via MessageCard (model's thinking text inside the fold)
    const messageProcessItems = suppressLiveProcessNarration
      ? []
      : assistantProcessMessages
      .map((msg, msgIndex) => ({
      id: `msg-${msg.id || msgIndex}`,
      kind: "assistant",
      processKind: inferProcessKind(msg.card?.text || ""),
      text: compactText(msg.card?.text || ""),
      detail: "",
      display: buildToolDisplayModel(msg.card?.detail),
      time: compactText(msg.time),
      hostId: "",
      tone: "neutral",
      status: "",
      sortTimestamp: msg.updatedAt || msg.createdAt || "",
      card: msg.card,
    }));
    const commandProcessItems = suppressLiveProcessDetails ? [] : buildCommandProcessItems(bucketCommandCards);
    const filteredActivityProcessItems = activityProcessItems.filter((item) => {
      if (item.processKind !== "command") return true;
      if (!commandProcessItems.length) return true;
      return !commandProcessItems.some((commandItem) => {
        const command = compactText(commandItem.command || "");
        return command && compactText(item.text).includes(command);
      });
    });
    const processItems = sortProcessDisplayItems([
      ...messageProcessItems,
      ...filteredActivityProcessItems,
      ...commandProcessItems,
    ]);
    const liveHint = hasActiveFinalMessage
      ? ""
      : suppressLiveProcessDetails
      ? ""
      : isActiveTurn && processMatchesBucket
        ? compactText(activeProcess?.liveHint || activeProcess?.hint || "")
        : "";
    const summary = suppressLiveProcessDetails || !processMatchesBucket
      ? ""
      : summarizeMainChatProcess({
          processItems,
          activeProcess,
          liveHint,
        });
    const terminalPhase = compactText(activeProcess?.phase || "").toLowerCase();
    const terminalProcessLabel = resolveTerminalProcessLabel(terminalPhase, compactText(activeProcess?.elapsedLabel));
    const statusCard = isCurrentTurn ? asObject(activeProcess?.statusCard) : null;
    const timestamps = [
      bucket.userMessage?.updatedAt,
      bucket.userMessage?.createdAt,
      finalMessage?.updatedAt,
      finalMessage?.createdAt,
      ...assistantProcessMessages.flatMap((message) => [message.updatedAt, message.createdAt]),
      ...bucketCommandCards.flatMap((card) => [card?.updatedAt, card?.createdAt, card?.startedAt, card?.completedAt]),
    ];
    const firstTimestamp = parseTimestamp(bucket.userMessage?.createdAt || bucket.userMessage?.updatedAt || assistantMessages[0]?.createdAt);
    const lastTimestamp = latestTimestamp(timestamps);
    const elapsedLabel = firstTimestamp && lastTimestamp && lastTimestamp >= firstTimestamp
      ? formatDurationLabel(lastTimestamp - firstTimestamp)
      : "";
    const phase = compactText(activeProcess?.phase || "");
    const turnSurfaces = collectTurnMcpSurfaceGroups(assistantMessages.map((message) => message.sourceCard).filter(Boolean));

    return {
      id: bucket.id,
      anchorMessageId: bucket.userMessage?.id || assistantMessages[0]?.id || bucket.id,
      messageIds: [bucket.userMessage?.id, ...assistantMessages.map((message) => message.id)].filter(Boolean),
      userMessage: bucket.userMessage,
      finalMessage,
      processItems,
      processLabel: terminalProcessLabel || [isActiveTurn ? activeProcessLabel(phase) : "已处理", elapsedLabel].filter(Boolean).join(" "),
      finalLabel: "",
      liveHint,
      summary,
      statusCard: statusCard && Object.keys(statusCard).length ? statusCard : null,
      collapsedByDefault: isAttentionPhase(terminalPhase || phase)
        ? false
        : !isActiveTurn && Boolean(processItems.length || summary || liveHint),
      active: isActiveTurn,
      hasActiveFinalMessage,
      phase,
      resultAttachments: turnSurfaces.resultAttachments,
      actionSurfaces: turnSurfaces.actionSurfaces,
      workspaceSurfaces: turnSurfaces.workspaceSurfaces,
      resultBundles: turnSurfaces.resultBundles,
      actionBundles: turnSurfaces.actionBundles,
    };
  });
}
