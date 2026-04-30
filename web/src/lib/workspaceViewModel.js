export function compactText(value) {
  return String(value || "")
    .trim()
    .replace(/\s+/g, " ");
}

/** Resolve a human-friendly host label: IP for remote hosts, "local" for server-local */
export function resolveHostLabel(hostId, hostMeta) {
  const id = compactText(hostId || hostMeta?.id || "");
  if (!id || id === "server-local") return "local";
  const address = compactText(hostMeta?.address || "");
  const name = compactText(hostMeta?.name || "");
  if (address) return address;
  if (name && name !== id) return name;
  return id;
}

export function compactStrings(values) {
  if (!Array.isArray(values)) return [];
  return values.map((value) => compactText(value)).filter(Boolean);
}

export function asNumber(value) {
  const numeric = Number(String(value ?? "").replace(/[^\d-]/g, ""));
  return Number.isFinite(numeric) ? numeric : 0;
}

export function parseTimestamp(value) {
  if (!value) return 0;
  const stamp = new Date(value).getTime();
  return Number.isFinite(stamp) ? stamp : 0;
}

export function formatTime(value) {
  const stamp = parseTimestamp(value);
  if (!stamp) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(stamp));
}

export function formatShortTime(value) {
  const stamp = parseTimestamp(value);
  if (!stamp) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(stamp));
}

export function labelizeKey(key) {
  return String(key || "")
    .replace(/_/g, " ")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^\w/, (char) => char.toUpperCase());
}

export function formatScalar(value) {
  if (value === null || value === undefined) return "";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "number") return String(value);
  if (typeof value === "string") return value.trim();
  if (Array.isArray(value)) {
    return compactStrings(value).join(" / ");
  }
  return "";
}

export function objectRows(source) {
  if (!source || typeof source !== "object") return [];
  return Object.entries(source)
    .map(([key, value]) => {
      const text = formatScalar(value);
      if (!text) return null;
      return {
        key: labelizeKey(key),
        value: text,
      };
    })
    .filter(Boolean);
}

const ORDERED_ROW_KEYS = ["task", "host", "status", "approval", "thread", "session"];

function normalizeOrderedKey(value) {
  return compactText(value).toLowerCase().replace(/[\s_-]+/g, "");
}

function orderedKeyPriority(value, keyOrder = ORDERED_ROW_KEYS) {
  const normalized = normalizeOrderedKey(value);
  const explicitIndex = keyOrder.findIndex((item) => normalized === normalizeOrderedKey(item) || normalized.startsWith(normalizeOrderedKey(item)));
  return explicitIndex >= 0 ? explicitIndex : keyOrder.length + 1;
}

export function orderedObjectRows(source, { keyOrder = ORDERED_ROW_KEYS } = {}) {
  if (!source || typeof source !== "object") return [];
  return Object.entries(source)
    .map(([key, value]) => {
      const text = formatScalar(value);
      if (!text) return null;
      return {
        rawKey: key,
        key: labelizeKey(key),
        value: text,
      };
    })
    .filter(Boolean)
    .sort((left, right) => {
      const priorityDiff = orderedKeyPriority(left.rawKey, keyOrder) - orderedKeyPriority(right.rawKey, keyOrder);
      if (priorityDiff !== 0) return priorityDiff;
      return left.key.localeCompare(right.key, "zh-CN");
    })
    .map(({ key, value }) => ({ key, value }));
}

function displayStatusPriority(status) {
  const normalized = compactText(status).toLowerCase();
  if (!normalized) return 5;
  if (normalized.includes("wait") || normalized.includes("approval") || normalized.includes("blocking")) return 0;
  if (normalized.includes("fail") || normalized.includes("error")) return 1;
  if (normalized.includes("run") || normalized.includes("progress") || normalized.includes("execut")) return 2;
  if (normalized.includes("queue") || normalized.includes("pending") || normalized.includes("dispatch")) return 3;
  if (normalized.includes("complete") || normalized.includes("done") || normalized.includes("success")) return 4;
  return 5;
}

export function sortProcessDisplayItems(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item, index) => ({
      ...item,
      __sortIndex: index,
      __sortStamp: parseTimestamp(item?.sortTimestamp || item?.updatedAt || item?.createdAt || ""),
      __sortStatus: displayStatusPriority(item?.status || item?.tone || item?.kind),
    }))
    .sort((left, right) => {
      const statusDiff = left.__sortStatus - right.__sortStatus;
      if (statusDiff !== 0) return statusDiff;
      const stampDiff = right.__sortStamp - left.__sortStamp;
      if (stampDiff !== 0) return stampDiff;
      return left.__sortIndex - right.__sortIndex;
    })
    .map(({ __sortIndex, __sortStamp, __sortStatus, ...item }) => item);
}

export function sortApprovalDisplayItems(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item, index) => ({
      ...item,
      __sortIndex: index,
      __sortStamp: parseTimestamp(item?.sortTimestamp || item?.raw?.updatedAt || item?.raw?.createdAt || ""),
    }))
    .sort((left, right) => {
      const stampDiff = right.__sortStamp - left.__sortStamp;
      if (stampDiff !== 0) return stampDiff;
      return compactText(left.hostName || left.hostId).localeCompare(compactText(right.hostName || right.hostId), "zh-CN");
    })
    .map(({ __sortIndex, __sortStamp, ...item }) => item);
}

export function sortBackgroundAgentItems(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item, index) => ({
      ...item,
      __sortIndex: index,
      __sortStamp: parseTimestamp(item?.sortTimestamp || ""),
      __sortStatus: displayStatusPriority(item?.status || item?.statusLabel),
    }))
    .sort((left, right) => {
      const statusDiff = left.__sortStatus - right.__sortStatus;
      if (statusDiff !== 0) return statusDiff;
      const stampDiff = right.__sortStamp - left.__sortStamp;
      if (stampDiff !== 0) return stampDiff;
      return compactText(left.name || left.hostId).localeCompare(compactText(right.name || right.hostId), "zh-CN");
    })
    .map(({ __sortIndex, __sortStamp, __sortStatus, ...item }) => item);
}

export function isInternalRoutingMessageText(text = "") {
  const trimmed = compactText(text);
  if (!trimmed) return false;
  if (/^主\s*Agent\s*正在判断/.test(trimmed)) return true;
  if (/^这是简单对话/.test(trimmed)) return true;
  if (/^(这是|当前).*(简单|直接).*(对话|回答|回复)/.test(trimmed)) return true;
  if (/^主\s*Agent\s*(会|将)直接回答/.test(trimmed)) return true;
  if (/不会生成计划或派发\s*worker/.test(trimmed)) return true;
  return false;
}

export function cleanAssistantDisplayText(text, role = "assistant") {
  const normalizedRole = compactText(role).toLowerCase();
  if (normalizedRole === "user") return compactText(text);
  let cleaned = String(text || "");
  cleaned = cleaned.replace(/`{3}json[\s\S]*?`{3}/g, (match) => (/"route"\s*:/.test(match) ? "" : match)).trim();
  cleaned = cleaned.replace(/`{3}json\s*\{[^`]*"route"\s*:[^`]*/g, "").trim();
  cleaned = cleaned.replace(/\{[^{}]*"route"\s*:\s*"[^"]*"[^{}]*\}/g, "").trim();
  if (/(BTC|比特币|CoinMarketCap|CoinGecko|Binance|Crypto\.com|24h|24小时|市值|成交额|A股|上证|深证|创业板|指数)/i.test(cleaned)) {
    cleaned = cleaned.replace(/(?:^|\n)\s*(?:短判断|一句话判断|简判断)[:：]\s*/gu, "\n");
    cleaned = cleaned.replace(/[；;，,]?\s*(?:对应来源|来源)[:：]\s*(?:[-—–]\s*){1,4}(?=$|\n|[。；;，,])/gu, "");
    cleaned = cleaned.replace(/[:：]\s*[；;]\s*/gu, "：");
    cleaned = cleaned.replace(/[:：]\s*[，,]\s*/gu, "：");
    cleaned = cleaned.replace(/[:：]\s*[:：]/gu, "：");
    cleaned = cleaned.replace(
      /(?:^|\n)来源[:：]\s*\n((?:\[[^\]]+\]\([^)]+\)\s*\n?){1,4})/gu,
      (_match, linksBlock) => {
        const labels = [...String(linksBlock || "").matchAll(/\[([^\]]+)\]\([^)]+\)/g)]
          .map((match) => compactText(match[1]))
          .filter(Boolean)
          .slice(0, 2);
        return labels.length ? `\n来源：${labels.join("；")}\n` : "\n";
      },
    );
  }
  cleaned = cleaned.replace(/\n{3,}/g, "\n\n").trim();
  if (isInternalRoutingMessageText(cleaned)) return "";
  return cleaned;
}

export function pickField(source, ...keys) {
  if (!source || typeof source !== "object") return undefined;
  for (const key of keys) {
    if (source[key] !== undefined && source[key] !== null && source[key] !== "") {
      return source[key];
    }
  }
  return undefined;
}

export function phaseLabel(phase) {
  switch (phase) {
    case "thinking":
      return "主 Agent 思考中";
    case "planning":
      return "主 Agent 生成计划";
    case "waiting_approval":
      return "等待审批";
    case "waiting_input":
      return "等待补充输入";
    case "executing":
      return "调度执行中";
    case "finalizing":
      return "结果汇总中";
    case "completed":
      return "已完成";
    case "failed":
      return "执行失败";
    case "aborted":
      return "已停止";
    default:
      return "待命";
  }
}

export function statusTone(statusKey) {
  if (statusKey === "failed") return "danger";
  if (statusKey === "completed") return "success";
  if (statusKey === "waiting_approval") return "warning";
  return "neutral";
}

export function phaseTone(phase) {
  switch (String(phase || "").toLowerCase()) {
    case "completed":
      return "success";
    case "failed":
    case "aborted":
      return "danger";
    case "waiting_approval":
    case "waiting_input":
      return "warning";
    case "planning":
    case "executing":
    case "finalizing":
    case "thinking":
      return "info";
    default:
      return "neutral";
  }
}

export function hostStateLabel(statusKey, fallback = "") {
  switch (statusKey) {
    case "waiting_approval":
      return "等待审批";
    case "running":
      return "执行中";
    case "queued":
      return "排队中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "idle":
      return "待命";
    default:
      return fallback || "未知";
  }
}

export function approvalPreview(card) {
  if (!card) return "";
  if (card.command) return compactText(card.command);
  if (Array.isArray(card.changes) && card.changes.length) {
    const first = card.changes[0];
    const path = compactText(first?.path);
    if (path && card.changes.length === 1) return path;
    if (path) return `${path} 等 ${card.changes.length} 个文件`;
    return `共 ${card.changes.length} 个文件变更`;
  }
  return compactText(card.text || card.summary || "");
}

export function getWorkspaceCardType(card) {
  return String(card?.type || "");
}

export function isUserMessageCard(card) {
  return card?.type === "UserMessageCard" || (card?.type === "MessageCard" && card?.role === "user");
}

export function isAssistantMessageCard(card) {
  return card?.type === "AssistantMessageCard" || (card?.type === "MessageCard" && card?.role === "assistant");
}

export function isApprovalCard(card) {
  return card?.type === "CommandApprovalCard" || card?.type === "FileChangeApprovalCard" || card?.type === "PlanApprovalCard";
}

export function isChoiceCard(card) {
  return card?.type === "ChoiceCard";
}

export function isProcessCard(card) {
  return card?.type === "ProcessLineCard";
}

export function isPlanCard(card) {
  return card?.type === "PlanCard";
}

export function isMissionNoticeCard(card) {
  return card?.type === "NoticeCard" && !!card?.detail?.id && String(card.detail.id).startsWith("mission:");
}

export function isDispatchSummaryCard(card) {
  return card?.type === "ResultSummaryCard" && !!card?.detail?.dispatchSummary;
}

export function isWorkspaceResultCard(card) {
  return card?.type === "ResultSummaryCard" && String(card?.id || "").startsWith("workspace-result-");
}

export function isWorkerResultCard(card) {
  return card?.type === "ResultSummaryCard" && String(card?.id || "").startsWith("worker-result-");
}

export function isReconcileResultCard(card) {
  return card?.type === "ResultSummaryCard" && String(card?.id || "").startsWith("workspace-reconcile-");
}

export function isSystemNoticeCard(card) {
  return card?.type === "NoticeCard" && !isMissionNoticeCard(card);
}

export function streamRowClass(card) {
  if (isUserMessageCard(card)) return "row-user";
  if (card?.type === "NoticeCard") return "row-notice";
  return "row-assistant";
}

export function deriveHostState({ card, rawStatusLabel, queueCount, approvalCard, worker }) {
  const normalized = `${compactText(rawStatusLabel)} ${compactText(card?.status)}`.toLowerCase();
  if (approvalCard || normalized.includes("等待审批") || normalized.includes("waiting")) {
    return "waiting_approval";
  }
  if (normalized.includes("failed") || normalized.includes("失败") || normalized.includes("cancelled") || normalized.includes("取消")) {
    return "failed";
  }
  if (normalized.includes("completed") || normalized.includes("完成")) {
    return "completed";
  }
  if (queueCount > 0 || normalized.includes("queued") || normalized.includes("排队")) {
    return "queued";
  }
  if (worker && typeof worker === "object") {
    const terminal = worker.terminal || {};
    const activeTaskId = compactText(terminal.activeTaskId || terminal.active_task_id);
    if (activeTaskId) {
      return "running";
    }
  }
  if (compactText(card?.text || card?.summary)) {
    return "running";
  }
  return "idle";
}

export function buildWorkspaceHostRows({ cards = [], hosts = [], approvalCards = [] }) {
  const hostMetaById = new Map((hosts || []).map((host) => [host.id, host]));
  const approvalByHostId = new Map();
  for (const card of approvalCards || []) {
    const hostId = compactText(card?.hostId);
    if (!hostId || approvalByHostId.has(hostId)) continue;
    approvalByHostId.set(hostId, card);
  }

  return (cards || [])
    .filter((card) => isProcessCard(card))
    .map((card) => {
      const hostMeta = hostMetaById.get(card.hostId) || {};
      const kvRows = toKeyValueMap(card.kvRows);
      const dispatch = card.detail?.dispatch && typeof card.detail.dispatch === "object" ? card.detail.dispatch : {};
      const worker = card.detail?.worker && typeof card.detail.worker === "object" ? card.detail.worker : {};
      const approvalCard = approvalByHostId.get(card.hostId) || null;
      const rawStatusLabel = kvRows["状态"] || compactText(dispatch.status) || card.status || "";
      const queueCount = asNumber(kvRows["排队"] || worker?.terminal?.queueTaskIds?.length || worker?.terminal?.queue_task_ids?.length || 0);
      const hostLabel = kvRows["主机"] || resolveHostLabel(card.hostId, hostMeta) || compactText(card.title) || compactText(dispatch.host) || "local";
      const taskTitle = kvRows["任务"] || compactText(dispatch?.request?.title) || compactText(card.summary) || "等待调度器分配任务";
      const statusKey = deriveHostState({
        card,
        rawStatusLabel,
        queueCount,
        approvalCard,
        worker,
      });

      return {
        card,
        hostId: compactText(card.hostId) || compactText(hostMeta.id),
        hostMeta,
        displayName: hostLabel,
        address: compactText(hostMeta.address),
        statusKey,
        statusLabel: hostStateLabel(statusKey, rawStatusLabel),
        taskTitle,
        summary: compactText(card.text || card.summary || dispatch?.request?.summary),
        queueCount,
        approvalCard,
        dispatch,
        worker,
        highlights: compactStrings(card.highlights),
        workerSession: kvRows.WorkerSession || "",
        updatedAt: card.updatedAt || card.createdAt || "",
        rawStatusLabel,
      };
    });
}

export function toKeyValueMap(rows) {
  const out = {};
  for (const row of Array.isArray(rows) ? rows : []) {
    const key = compactText(row?.key);
    if (!key) continue;
    out[key] = String(row?.value ?? "").trim();
  }
  return out;
}

export function buildWorkspaceProgressSummary({
  planCard = null,
  missionCard = null,
  resultCard = null,
  hostRows = [],
  dagSummary = {},
  missionStepCount = 0,
} = {}) {
  const planItems = Array.isArray(planCard?.items) ? planCard.items : [];
  const resultRows = toKeyValueMap(resultCard?.kvRows);
  const total = planItems.length || asNumber(dagSummary.nodes) || missionStepCount || hostRows.length;
  const completed = planItems.filter((item) => item.status === "completed").length || asNumber(resultRows["完成"]);
  const running = asNumber(dagSummary.running) || hostRows.filter((row) => row.statusKey === "running").length;
  const waitingApproval = asNumber(dagSummary.waitingApproval) || hostRows.filter((row) => row.statusKey === "waiting_approval").length;
  const queued = asNumber(dagSummary.queued) || hostRows.filter((row) => row.statusKey === "queued").length;
  const failed = asNumber(resultRows["失败"]) || hostRows.filter((row) => row.statusKey === "failed").length;
  const cancelled = asNumber(resultRows["取消"]);
  const finished = completed + failed + cancelled;

  return {
    total,
    completed,
    running,
    waitingApproval,
    queued,
    failed,
    cancelled,
    percent: total > 0 ? Math.max(0, Math.min(100, Math.round((finished / total) * 100))) : 0,
    missionStatus: toKeyValueMap(missionCard?.kvRows).状态 || "",
  };
}

export function buildWorkspaceNotificationItems({
  approvalCards = [],
  errorCards = [],
  dispatchCards = [],
  workerResultCards = [],
  hostRows = [],
  noticeCards = [],
  formatTimeFn = formatShortTime,
  parseTimestampFn = parseTimestamp,
  approvalPreviewFn = approvalPreview,
} = {}) {
  const items = [];

  for (const card of approvalCards) {
    items.push({
      id: `approval-${card.id}`,
      tone: "warning",
      timestamp: parseTimestampFn(card.updatedAt || card.createdAt),
      time: formatTimeFn(card.updatedAt || card.createdAt),
      title: `${resolveHostLabel(card.hostId, null)} 等待审批`,
      text: approvalPreviewFn(card) || compactText(card.text) || "待确认操作",
      action: "approval",
      hostId: card.hostId || "",
      cardId: card.id,
    });
  }

  for (const card of errorCards) {
    items.push({
      id: `error-${card.id}`,
      tone: "danger",
      timestamp: parseTimestampFn(card.updatedAt || card.createdAt),
      time: formatTimeFn(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "执行异常",
      text: compactText(card.message || card.text || card.summary),
      action: "none",
      hostId: card.hostId || "",
      cardId: card.id,
    });
  }

  for (const card of dispatchCards.slice(-3)) {
    items.push({
      id: `dispatch-${card.id}`,
      tone: "info",
      timestamp: parseTimestampFn(card.updatedAt || card.createdAt),
      time: formatTimeFn(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "调度器已接收任务",
      text: compactText(card.text || card.summary),
      action: "dispatch",
      hostId: "",
      cardId: card.id,
    });
  }

  for (const card of workerResultCards.slice(-8)) {
    items.push({
      id: `result-${card.id}`,
      tone: card.status === "failed" ? "danger" : "success",
      timestamp: parseTimestampFn(card.updatedAt || card.createdAt),
      time: formatTimeFn(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "Worker 执行结果",
      text: compactText(card.text || card.summary),
      action: card.hostId ? "host" : "none",
      hostId: card.hostId || "",
      cardId: card.id,
    });
  }

  for (const row of hostRows) {
    const latestEvent = row.highlights[row.highlights.length - 1];
    if (!latestEvent) continue;
    items.push({
      id: `host-event-${row.hostId}`,
      tone: row.statusKey === "failed" ? "danger" : row.statusKey === "waiting_approval" ? "warning" : "neutral",
      timestamp: parseTimestampFn(row.updatedAt),
      time: formatTimeFn(row.updatedAt),
      title: row.displayName,
      text: latestEvent,
      action: "host",
      hostId: row.hostId,
      cardId: row.card.id,
    });
  }

  for (const card of noticeCards.slice(-6)) {
    items.push({
      id: `notice-${card.id}`,
      tone: "neutral",
      timestamp: parseTimestampFn(card.updatedAt || card.createdAt),
      time: formatTimeFn(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "工作台通知",
      text: compactText(card.text || card.summary),
      action: "none",
      hostId: "",
      cardId: card.id,
    });
  }

  return items.sort((left, right) => right.timestamp - left.timestamp);
}

export function buildMissionHeaderSegments(progress = {}) {
  const total = Math.max(asNumber(progress.total), 0);
  const safeTotal = total > 0 ? total : 1;
  const values = [
    { key: "completed", label: "完成", value: asNumber(progress.completed), tone: "success" },
    { key: "running", label: "执行中", value: asNumber(progress.running), tone: "info" },
    { key: "waitingApproval", label: "待审批", value: asNumber(progress.waitingApproval), tone: "warning" },
    { key: "queued", label: "排队", value: asNumber(progress.queued), tone: "neutral" },
    { key: "failed", label: "失败", value: asNumber(progress.failed), tone: "danger" },
  ];

  return values
    .filter((item) => item.value > 0)
    .map((item) => ({
      ...item,
      percent: Math.max(6, Math.round((item.value / safeTotal) * 100)),
    }));
}

function extractStepMeta(line = "") {
  const text = compactText(line);
  const idMatch = text.match(/^([^\s]+)/);
  const statusMatch = text.match(/\[([^\]]+)\]/);
  const hostMatch = text.match(/@([A-Za-z0-9._:-]+)/);
  const title = text
    .replace(/^([^\s]+)/, "")
    .replace(/\[([^\]]+)\]/, "")
    .replace(/@([A-Za-z0-9._:-]+)/, "")
    .trim();

  return {
    raw: text,
    id: compactText(idMatch?.[1]) || `step-${text}`,
    status: compactText(statusMatch?.[1]).toLowerCase() || "pending",
    hostId: compactText(hostMatch?.[1]),
    title: title || text || "未命名步骤",
  };
}

function normalizeStepStatusTone(status) {
  const normalized = compactText(status).toLowerCase();
  if (normalized.includes("fail")) return "danger";
  if (normalized.includes("wait")) return "warning";
  if (normalized.includes("run") || normalized.includes("progress")) return "info";
  if (normalized.includes("complete") || normalized.includes("done")) return "success";
  if (normalized.includes("queue") || normalized.includes("pending")) return "neutral";
  return "neutral";
}

export function buildWorkspaceStepItems({ structuredProcess = [], taskHostBindings = [], hostRows = [] } = {}) {
  const hostById = new Map((hostRows || []).map((row) => [row.hostId, row]));
  const bindingsByTaskID = new Map();
  for (const binding of Array.isArray(taskHostBindings) ? taskHostBindings : []) {
    const taskID = compactText(binding?.taskId || binding?.id);
    if (!taskID) continue;
    if (!bindingsByTaskID.has(taskID)) bindingsByTaskID.set(taskID, []);
    bindingsByTaskID.get(taskID).push(binding);
  }

  const lines =
    Array.isArray(structuredProcess) && structuredProcess.length
      ? structuredProcess
      : (Array.isArray(taskHostBindings) ? taskHostBindings : []).map((binding) => {
          const taskID = compactText(binding?.taskId || binding?.id) || "task";
          const hostID = compactText(binding?.hostId);
          const status = compactText(binding?.status) || "pending";
          const title = compactText(binding?.title || binding?.instruction) || taskID;
          return `${taskID} [${status}]${hostID ? ` @${hostID}` : ""} ${title}`.trim();
        });

  return (lines || []).map((line, index) => {
    const meta = extractStepMeta(line);
    const matchedBindings = bindingsByTaskID.get(meta.id) || [];
    const directHost = hostById.get(meta.hostId || compactText(matchedBindings[0]?.hostId));
    const matchedHosts = matchedBindings.length
      ? matchedBindings
          .map((binding) => hostById.get(compactText(binding?.hostId)))
          .filter(Boolean)
      : directHost
        ? [directHost]
        : hostRows.filter((row) => {
            const haystack = `${row.taskTitle} ${row.summary} ${row.displayName}`.toLowerCase();
            return haystack.includes(meta.title.toLowerCase()) || haystack.includes(meta.id.toLowerCase());
          });

    const hosts = matchedHosts.map((row) => ({
      id: row.hostId,
      label: row.displayName,
      statusKey: row.statusKey,
      statusLabel: row.statusLabel,
      tone: statusTone(row.statusKey),
    }));

    const leadBinding = matchedBindings[0] || null;
    const bindingConstraints = Array.isArray(leadBinding?.constraints) ? leadBinding.constraints : [];
    const bindingStatus = compactText(leadBinding?.status).toLowerCase();
    const approvalCount =
      matchedBindings.filter((binding) => compactText(binding?.approvalState) || compactText(binding?.status).includes("approval")).length ||
      matchedHosts.filter((row) => row.approvalCard).length;

    return {
      id: meta.id || `step-${index + 1}`,
      index: index + 1,
      title: compactText(leadBinding?.title || meta.title),
      summary: compactText(leadBinding?.instruction || meta.raw),
      status: bindingStatus || meta.status,
      tone: normalizeStepStatusTone(bindingStatus || meta.status),
      constraints: bindingConstraints.length ? bindingConstraints : directHost?.dispatch?.request?.constraints || [],
      approvalCount,
      hosts,
      raw: line,
    };
  });
}

export function buildWorkspaceOrchestrationLanes({
  missionTitle = "",
  missionSummary = "",
  phase = "",
  plannerSessionLabel = "",
  planVersion = "",
  schedulerSummary = {},
  hostRows = [],
  pendingApprovalCount = 0,
  pendingChoiceCount = 0,
} = {}) {
  const runningHosts = hostRows.filter((row) => row.statusKey === "running").length;
  const completedHosts = hostRows.filter((row) => row.statusKey === "completed").length;
  const failedHosts = hostRows.filter((row) => row.statusKey === "failed").length;

  return [
    {
      id: "main-agent",
      title: "Main Agent",
      summary: missionTitle || "等待任务输入",
      caption: missionSummary || "通过右侧抽屉继续下发任务、确认需求与追问执行进度。",
      status: phaseLabel(phase),
      tone: phaseTone(phase),
      meta: [
        { key: "需求状态", value: phaseLabel(phase) },
        { key: "待补充", value: pendingChoiceCount || 0 },
      ],
    },
    {
      id: "planner",
      title: "计划与审批",
      summary: plannerSessionLabel || "等待计划审批",
      caption: planVersion ? `当前版本 ${planVersion}` : "负责拆分结构化步骤并解释计划。",
      status: plannerSessionLabel ? "计划就绪" : "待生成",
      tone: plannerSessionLabel ? "info" : "neutral",
      meta: [
        { key: "主 Agent", value: plannerSessionLabel || "-" },
        { key: "待审批", value: pendingApprovalCount || 0 },
      ],
    },
    {
      id: "dispatcher",
      title: "Dispatcher / Host Agents",
      summary: `${hostRows.length} 台主机`,
      caption: `Accepted ${asNumber(schedulerSummary.accepted)} / Running ${runningHosts} / Completed ${completedHosts}`,
      status: failedHosts ? "存在失败主机" : pendingApprovalCount ? "等待审批推进" : "执行中",
      tone: failedHosts ? "danger" : pendingApprovalCount ? "warning" : "info",
      meta: [
        { key: "Accepted", value: asNumber(schedulerSummary.accepted) },
        { key: "Queued", value: asNumber(schedulerSummary.queued) },
      ],
    },
  ];
}

export function buildWorkspaceLiveTimeline({
  planCard = null,
  dispatchEvents = [],
  dispatchCards = [],
  approvalCards = [],
  choiceCards = [],
  resultCard = null,
  hostRows = [],
  noticeCards = [],
} = {}) {
  const items = [];
  // Build a lookup for resolving host display names
  const hostMetaById = new Map(hostRows.map((row) => [row.hostId, row.hostMeta || {}]));
  const resolveHost = (hostId) => resolveHostLabel(hostId, hostMetaById.get(hostId));

  if (planCard) {
    items.push({
      id: `plan-${planCard.id}`,
      source: "主 Agent",
      tone: "info",
      time: formatShortTime(planCard.updatedAt || planCard.createdAt),
      timestamp: parseTimestamp(planCard.updatedAt || planCard.createdAt),
      title: compactText(planCard.title) || "主 Agent 已生成计划",
      text: compactText(planCard.text) || "结构化步骤已准备就绪",
      targetType: "plan",
      targetId: planCard.id,
    });
  }

  if (Array.isArray(dispatchEvents) && dispatchEvents.length) {
    for (const event of dispatchEvents.slice(-10)) {
      const detail = compactText(event?.summary || event?.detail);
      if (!detail) continue;
      items.push({
        id: `relay-${event.id || `${event.type}-${event.taskId}-${event.createdAt}`}`,
        source: compactText(event.hostId || event.sessionId || "Dispatcher"),
        tone: event.approvalId ? "warning" : statusTone(compactText(event.status || event.type)),
        time: formatShortTime(event.createdAt),
        timestamp: parseTimestamp(event.createdAt),
        title: compactText(event.summary || event.type) || "调度事件",
        text: compactText(event.detail || event.summary),
        targetType: event.approvalId ? "approval" : event.hostId ? "host" : "dispatch",
        targetId: compactText(event.approvalId || event.taskId || event.id),
        hostId: compactText(event.hostId),
      });
    }
  }

  for (const card of dispatchCards.slice(-4)) {
    items.push({
      id: `dispatch-${card.id}`,
      source: "Dispatcher",
      tone: "info",
      time: formatShortTime(card.updatedAt || card.createdAt),
      timestamp: parseTimestamp(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "调度器事件",
      text: compactText(card.text || card.summary),
      targetType: "dispatch",
      targetId: card.id,
    });
  }

  for (const card of approvalCards.slice(-6)) {
    items.push({
      id: `approval-${card.id}`,
      source: "Approval",
      tone: "warning",
      time: formatShortTime(card.updatedAt || card.createdAt),
      timestamp: parseTimestamp(card.updatedAt || card.createdAt),
      title: `${resolveHost(card.hostId)} 等待审批`,
      text: approvalPreview(card) || compactText(card.text),
      targetType: "approval",
      targetId: card.id,
      hostId: card.hostId || "",
    });
  }

  for (const card of choiceCards.slice(-4)) {
    items.push({
      id: `choice-${card.id}`,
      source: "Main Agent",
      tone: "warning",
      time: formatShortTime(card.updatedAt || card.createdAt),
      timestamp: parseTimestamp(card.updatedAt || card.createdAt),
      title: compactText(card.title || card.question) || "等待补充输入",
      text: compactText(card.text || card.question),
      targetType: "choice",
      targetId: card.id,
    });
  }

  if (!(Array.isArray(dispatchEvents) && dispatchEvents.length)) {
    for (const row of hostRows) {
      const cmdText = compactText(row.dispatch?.request?.command || row.dispatch?.request?.title || row.dispatch?.request?.summary || "");
      const latest = compactText(row.highlights[row.highlights.length - 1] || row.summary);
      const text = cmdText
        ? (latest ? `${cmdText}\n${latest}` : cmdText)
        : (latest || row.statusLabel || "");
      if (!text) continue;
      const hostDisplayLabel = resolveHostLabel(row.hostId, row.hostMeta);
      items.push({
        id: `host-${row.hostId}`,
        source: hostDisplayLabel,
        tone: statusTone(row.statusKey),
        time: formatShortTime(row.updatedAt),
        timestamp: parseTimestamp(row.updatedAt),
        title: `${hostDisplayLabel} · ${row.statusLabel}`,
        text,
        targetType: "host",
        targetId: row.hostId,
        hostId: row.hostId,
      });
    }
  }

  for (const card of noticeCards.slice(-4)) {
    items.push({
      id: `notice-${card.id}`,
      source: "System",
      tone: "neutral",
      time: formatShortTime(card.updatedAt || card.createdAt),
      timestamp: parseTimestamp(card.updatedAt || card.createdAt),
      title: compactText(card.title) || "系统通知",
      text: compactText(card.text || card.summary),
      targetType: "notice",
      targetId: card.id,
    });
  }

  if (resultCard) {
    items.push({
      id: `result-${resultCard.id}`,
      source: "Main Agent",
      tone: resultCard.status === "failed" ? "danger" : "success",
      time: formatShortTime(resultCard.updatedAt || resultCard.createdAt),
      timestamp: parseTimestamp(resultCard.updatedAt || resultCard.createdAt),
      title: compactText(resultCard.title) || "Mission 总结",
      text: compactText(resultCard.text || resultCard.summary),
      targetType: "result",
      targetId: resultCard.id,
    });
  }

  return items.sort((left, right) => right.timestamp - left.timestamp).slice(0, 14);
}

export function buildWorkspaceTraceItems({
  conversationCards = [],
  planDetail = null,
  plannerTraceRef = {},
  hostRows = [],
} = {}) {
  const items = [];

  for (const card of conversationCards) {
    items.push({
      id: `main-${card.id}`,
      source: isUserMessageCard(card) ? "用户" : "主 Agent",
      sourceKey: "main",
      tone: isUserMessageCard(card) ? "neutral" : "info",
      time: formatShortTime(card.updatedAt || card.createdAt),
      title: compactText(card.title) || (isUserMessageCard(card) ? "用户输入" : "主 Agent 回复"),
      text: compactText(card.text || card.summary || card.message),
    });
  }

  const plannerConversation = Array.isArray(planDetail?.plannerConversation || planDetail?.planner_conversation)
    ? planDetail.plannerConversation || planDetail.planner_conversation
    : [];

  if (plannerConversation.length) {
    for (const item of plannerConversation) {
      const text = compactText(item?.text || item?.summary);
      if (!text) continue;
      items.push({
        id: `planner-${item.id || item.createdAt || text}`,
        source: "主 Agent 计划摘要",
        sourceKey: "main-agent-plan",
        tone: "info",
        time: formatShortTime(item?.createdAt),
        title: compactText(item?.summary || item?.type || item?.role) || "主 Agent 计划摘要",
        text,
      });
    }
  } else if (planDetail?.goal || plannerTraceRef?.sessionId || plannerTraceRef?.threadId) {
    items.push({
      id: "planner-trace-ref",
      source: "主 Agent 计划摘要",
      sourceKey: "main-agent-plan",
      tone: "info",
      time: "",
      title: planDetail?.goal ? "主 Agent 目标摘要" : "主 Agent Trace",
      text: compactText(planDetail?.goal) || `Session ${plannerTraceRef?.sessionId || "-"} / Thread ${plannerTraceRef?.threadId || "-"}`,
    });
  }

  for (const row of hostRows) {
    const workerConversation = Array.isArray(row.worker?.conversation || row.worker?.conversation_excerpts)
      ? row.worker.conversation || row.worker.conversation_excerpts
      : [];
    if (workerConversation.length) {
      for (const item of workerConversation) {
        const text = compactText(item?.text || item?.summary);
        if (!text) continue;
        items.push({
          id: `host-trace-${row.hostId}-${item.id || item.createdAt || text}`,
          source: `${row.displayName} -> AI`,
          sourceKey: "host",
          tone: statusTone(row.statusKey),
          time: formatShortTime(item?.createdAt || row.updatedAt),
          title: compactText(item?.summary || item?.type || `${row.displayName} transcript`),
          text,
        });
      }
    } else if (Array.isArray(row.worker?.transcript) && row.worker.transcript.length) {
      items.push({
        id: `host-trace-${row.hostId}`,
        source: `${row.displayName} -> AI`,
        sourceKey: "host",
        tone: statusTone(row.statusKey),
        time: formatShortTime(row.updatedAt),
        title: `${row.displayName} transcript`,
        text: row.worker.transcript.map((entry) => compactText(entry)).filter(Boolean).join("\n"),
      });
    }
  }

  return items.filter((item) => item.text).slice(0, 24);
}

export function buildRawTraceSections({
  plannerTraceRef = {},
  selectedHostRow = null,
  missionDetail = null,
} = {}) {
  const sections = [];

  if (plannerTraceRef?.sessionId || plannerTraceRef?.threadId) {
    sections.push({
      id: "planner-trace",
      title: "主 Agent Trace",
      rows: [
        ...(plannerTraceRef.sessionId ? [{ key: "Session", value: plannerTraceRef.sessionId }] : []),
        ...(plannerTraceRef.threadId ? [{ key: "Thread", value: plannerTraceRef.threadId }] : []),
      ],
    });
  }

  if (selectedHostRow) {
    sections.push({
      id: `host-trace-${selectedHostRow.hostId}`,
      title: `${selectedHostRow.displayName} Worker`,
      rows: [
        { key: "Host", value: selectedHostRow.displayName },
        { key: "Status", value: selectedHostRow.statusLabel },
        ...(selectedHostRow.workerSession ? [{ key: "WorkerSession", value: selectedHostRow.workerSession }] : []),
      ],
      raw: {
        dispatch: selectedHostRow.dispatch || {},
        worker: selectedHostRow.worker || {},
      },
    });
  }

  if (missionDetail) {
    sections.push({
      id: "mission-summary",
      title: "Mission Detail",
      rows: objectRows(missionDetail).slice(0, 8),
      raw: missionDetail,
    });
  }

  return sections;
}

export function getWorkspacePlanDetail(planCard = null, resultCard = null) {
  const detail = planCard?.detail && typeof planCard.detail === "object" ? planCard.detail : resultCard?.detail?.plan || null;
  if (!detail || typeof detail !== "object") {
    return {
      detail: null,
      dagSummary: {},
      structuredProcess: [],
      plannerTraceRef: {},
      ownerSessionLabel: "",
      plannerSessionLabel: "",
      version: "",
      generatedAt: "",
    };
  }

  return {
    detail,
    dagSummary: pickField(detail, "dagSummary", "dag_summary") || {},
    structuredProcess: (() => {
      const value = pickField(detail, "structuredProcess", "structured_process");
      return Array.isArray(value) ? value : [];
    })(),
    plannerTraceRef: pickField(detail, "rawPlannerTraceRef", "raw_planner_trace_ref") || {},
    ownerSessionLabel: pickField(detail, "ownerSessionLabel", "owner_session_label") || "",
    plannerSessionLabel: pickField(detail, "plannerSessionLabel", "planner_session_label") || "",
    version: pickField(detail, "version") || "",
    generatedAt: pickField(detail, "generatedAt", "generated_at") || "",
  };
}
