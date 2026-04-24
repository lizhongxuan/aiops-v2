<script setup>
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { AlertTriangleIcon, Loader2Icon, PanelsTopLeftIcon, RefreshCwIcon, XIcon } from "lucide-vue-next";
import ProtocolApprovalRail from "../components/protocol-workspace/ProtocolApprovalRail.vue";
import ProtocolAgentDetailModal from "../components/protocol-workspace/ProtocolAgentDetailModal.vue";
import ProtocolConversationPane from "../components/protocol-workspace/ProtocolConversationPane.vue";
import ProtocolEvidenceDrawer from "../components/protocol-workspace/ProtocolEvidenceDrawer.vue";
import ProtocolEventTimeline from "../components/protocol-workspace/ProtocolEventTimeline.vue";
import ProtocolEvidenceModal from "../components/protocol-workspace/ProtocolEvidenceModal.vue";
import { sendMessage as sendMessageApi, stopMessage as stopMessageApi } from "../api/chat";
import { submitApprovalDecision as submitApprovalDecisionApi, submitChoiceAnswer as submitChoiceAnswerApi } from "../api/approvals";
import { buildMcpDecisionNotice, buildSyntheticMcpApproval, buildSyntheticMcpEvent, formatMcpActionLabel, formatMcpActionTarget, isMcpMutationAction } from "../lib/mcpActionRuntime";
import { buildProtocolAgentDetailModel, buildProtocolEvidenceTabs, buildProtocolWorkspaceModel, resolveWorkspaceToolLabel } from "../lib/protocolWorkspaceVm";
import { compactText } from "../lib/workspaceViewModel";
import { useAppStore } from "../store";

const store = useAppStore();
const OPEN_SESSION_HISTORY_EVENT = "codex:open-session-history";
const OPEN_MCP_DRAWER_EVENT = "codex:open-mcp-drawer";
const MCP_SURFACE_TAB = "mcp-surface";
const DISMISSED_STATUS_BANNER_STORAGE_KEY = "codex:protocol-workspace-dismissed-status-banners:v1";
const promptDebugEnabled =
  import.meta.env.DEV ||
  import.meta.env.MODE === "test" ||
  (typeof window !== "undefined" && new URLSearchParams(window.location.search).get("promptDebug") === "1");

const refreshBusy = ref(false);
const decisionBusy = ref(false);
const stopBusy = ref(false);
const composerDraft = ref("");
const actionNotice = ref("");
const actionTone = ref("info");
const evidenceOpen = ref(false);
const evidenceTab = ref("main-agent-plan");
const evidenceDrawerOpen = ref(false);
const evidenceDrawerActiveTab = ref("main-agent-plan");
const evidenceDrawerState = ref({
  title: "证据抽屉",
  subtitle: "把当前重细节内容固定到侧边抽屉，方便边看边对照主线程。",
  tabs: [],
  panels: {},
});
const promptDebugOpen = ref(false);
const promptDebugActiveTab = ref("runtime-policy");
const selectedHostId = ref("");
const selectedStepId = ref("");
const selectedApprovalId = ref("");
const selectedMessageId = ref("");
const selectedMcpSurface = ref(null);
const selectedAgentId = ref("");
const selectedCommandEvidence = ref(null);
const selectedProcessEvidence = ref(null);
const selectedToolInvocationId = ref("");
const selectedVerificationId = ref("");
const selectedCitationEvidenceId = ref("");
const selectedEventId = ref("");
const agentDetailOpen = ref(false);
const evidenceSource = ref("mission");
const workspaceBootstrapBusy = ref(false);
const workspaceBootstrapAttempted = ref(false);
const localMcpApprovals = ref([]);
const localMcpEvents = ref([]);
const choiceBusyById = ref({});
const choiceErrorById = ref({});
const dismissedStatusBannerKeys = ref(readDismissedStatusBannerKeys());

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function createClientId(prefix) {
  const randomPart =
    typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  return `${prefix}-${randomPart}`;
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function projectionCandidateIds(value = null) {
  const source = asObject(value);
  const projection = asObject(source.projection);
  return [...new Set([
    compactText(projection.id),
    ...asArray(projection.aliases).map((item) => compactText(item)),
    compactText(source.id),
    compactText(source.approvalId),
    compactText(source.commandCardId),
    compactText(source.actionEventId),
    compactText(source.targetId),
    compactText(source.raw?.id),
  ].filter(Boolean))];
}

function projectionLinksOfKind(value = null, kind = "") {
  const targetKind = compactText(kind).toLowerCase();
  return asArray(value?.projection?.links).filter((link) => compactText(link?.kind).toLowerCase() === targetKind);
}

function resolveProjectionLinkTarget(collection = [], link = null, kind = "") {
  const sourceKind = compactText(link?.kind).toLowerCase();
  const targetKind = compactText(kind || sourceKind).toLowerCase();
  const targetId = compactText(link?.id);
  if (!targetKind || !targetId) return null;
  return asArray(collection).find((item) => {
    const projection = asObject(item?.projection);
    if (compactText(projection.kind).toLowerCase() !== targetKind) return false;
    return projectionCandidateIds(item).includes(targetId);
  }) || null;
}

function resolveProjectionTarget(value = null, kind = "", collection = []) {
  const link = projectionLinksOfKind(value, kind)[0] || null;
  return resolveProjectionLinkTarget(collection, link, kind);
}

function resolveProjectionTargets(value = null, kind = "", collection = []) {
  return projectionLinksOfKind(value, kind)
    .map((link) => resolveProjectionLinkTarget(collection, link, kind))
    .filter(Boolean);
}

function normalizePhaseLabel(value) {
  const phase = compactText(value).toLowerCase();
  switch (phase) {
    case "executing":
    case "running":
      return "执行中";
    case "planning":
      return "规划中";
    case "thinking":
      return "正在思考";
    case "waiting_approval":
      return "等待审批";
    case "waiting_input":
      return "等待补充输入";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "aborted":
      return "已停止";
    default:
      return phase || "待命";
  }
}

function stepStatusLabel(value) {
  const status = compactText(value).toLowerCase();
  if (status.includes("complete") || status.includes("done")) return "已完成";
  if (status.includes("run") || status.includes("progress") || status.includes("active")) return "执行中";
  if (status.includes("wait")) return "等待审批";
  if (status.includes("fail") || status.includes("error")) return "失败";
  return "待执行";
}

function stringifyRaw(value) {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) return value.map((item) => String(item ?? "")).join("\n");
  if (value && typeof value === "object") {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }
  return "";
}

function cloneStructuredValue(value) {
  if (value === null || value === undefined) return value;
  if (["string", "number", "boolean"].includes(typeof value)) return value;
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return value;
  }
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

function commandStatusLabel(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized.includes("permission") || normalized.includes("denied")) return "权限不足";
  if (normalized.includes("fail") || normalized.includes("error")) return "失败";
  if (normalized.includes("complete") || normalized.includes("done")) return "已完成";
  if (normalized.includes("run") || normalized.includes("progress")) return "执行中";
  return compactText(value) || "已处理";
}

function commandOutputText(commandEvidence = {}) {
  return String(commandEvidence.output || commandEvidence.stdout || commandEvidence.stderr || commandEvidence.text || commandEvidence.summary || "").trim();
}

function commandEvidenceFrom(value = {}) {
  const source = asObject(value);
  const card = asObject(source.commandCard || source.raw?.commandCard || source.card || source);
  return {
    ...source,
    ...card,
    command: displayCommand(card.command || source.command),
    rawCommand: card.command || source.command || "",
    output: card.output || source.output || card.stdout || source.stdout || card.stderr || source.stderr || "",
    status: card.status || source.status || "",
    hostId: card.hostId || source.hostId || "",
    cwd: card.cwd || source.cwd || "",
    exitCode: card.exitCode ?? source.exitCode,
    durationMs: card.durationMs ?? source.durationMs,
  };
}

function firstCompactValue(...values) {
  for (const value of values) {
    const text = compactText(value);
    if (text) return text;
  }
  return "";
}

function compactRow(label, value) {
  const text = compactText(value);
  return text ? { label, value: text } : null;
}

function previewText(value, maxLength = 240) {
  const text = compactText(value);
  if (!text) return "";
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength)}...`;
}

function objectRows(value = {}) {
  const source = asObject(value);
  return Object.entries(source)
    .filter(([, entry]) => entry !== undefined && entry !== null && compactText(entry) !== "")
    .map(([key, entry]) => ({ label: key, value: entry }));
}

function toolDisplayName(tool = "") {
  return resolveWorkspaceToolLabel(tool);
}

function selectedStepHostRows(step = null) {
  const hostIds = new Set(
    asArray(step?.hosts)
      .map((host) => compactText(host?.id || host?.hostId || host))
      .filter(Boolean),
  );
  if (!hostIds.size) {
    return selectedHostRow.value ? [selectedHostRow.value] : [];
  }
  return hostRows.value.filter((row) => hostIds.has(compactText(row.hostId)));
}

function dispatchContextForStep(step = null) {
  const matchingHosts = selectedStepHostRows(step);
  const selectedHostMatchesStep = matchingHosts.some((row) => compactText(row.hostId) === compactText(selectedHostRow.value?.hostId));
  const row = selectedHostMatchesStep ? selectedHostRow.value : matchingHosts[0] || selectedHostRow.value || null;
  const dispatch = asObject(row?.dispatch);
  const request = asObject(dispatch.request);
  const taskBinding = asObject(dispatch.taskBinding || dispatch.task_binding);
  const worker = asObject(row?.worker);
  return {
    row,
    matchingHosts,
    dispatch,
    request,
    taskBinding,
    worker,
  };
}

function pushActionNotice(message, tone = "info") {
  actionNotice.value = compactText(message);
  actionTone.value = tone;
}

function readDismissedStatusBannerKeys() {
  if (typeof window === "undefined") return [];
  try {
    const parsed = JSON.parse(window.localStorage?.getItem(DISMISSED_STATUS_BANNER_STORAGE_KEY) || "[]");
    return Array.isArray(parsed) ? parsed.map((item) => compactText(item)).filter(Boolean) : [];
  } catch {
    return [];
  }
}

function persistDismissedStatusBannerKeys(keys) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage?.setItem(DISMISSED_STATUS_BANNER_STORAGE_KEY, JSON.stringify(keys.slice(0, 200)));
  } catch {
    // Ignore storage failures; the banner can still be dismissed for the current render cycle.
  }
}

function statusBannerDismissKey(banner) {
  const cardId = compactText(banner?.cardId);
  if (!cardId) return "";
  const sessionId = compactText(store.snapshot.sessionId || store.activeSessionId || "current");
  return `${sessionId}:${cardId}`;
}

function dismissStatusBanner() {
  const key = statusBannerDismissKey(statusBanner.value);
  if (!key || dismissedStatusBannerKeys.value.includes(key)) return;
  const nextKeys = [key, ...dismissedStatusBannerKeys.value].slice(0, 200);
  dismissedStatusBannerKeys.value = nextKeys;
  persistDismissedStatusBannerKeys(nextKeys);
}

function pushMcpEvent(action, options = {}) {
  localMcpEvents.value = [
    ...localMcpEvents.value,
    buildSyntheticMcpEvent(action, options),
  ];
}

function buildMcpDrawerSurface(surface) {
  const normalized = normalizeMcpSurface(surface);
  const base = {
    kind: normalized.kind,
    source: normalized.source,
  };
  if (normalized.kind === "bundle") {
    return {
      ...base,
      bundle: normalized.bundle || normalized,
    };
  }
  return {
    ...base,
    card: normalized.card || normalized,
  };
}

function dispatchMcpDrawer(surface, pin = false) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent(OPEN_MCP_DRAWER_EVENT, {
      detail: {
        source: "protocol-mcp-surface",
        pin: Boolean(pin),
        surface: buildMcpDrawerSurface(surface),
      },
    }),
  );
}

function normalizeMcpSurface(surface) {
  const source = asObject(surface);
  const bundle = asObject(source.bundle || source);
  const card = asObject(source.card || source);
  const kind = compactText(source.kind || source.surfaceKind || (bundle.bundleKind ? "bundle" : card.uiKind ? "card" : "card")).toLowerCase();
  return {
    ...source,
    kind: kind === "bundle" ? "bundle" : "card",
    bundle: Object.keys(bundle).length ? bundle : null,
    card: Object.keys(card).length ? card : null,
    source: compactText(source.source || bundle.source || card.source || "protocol-workspace"),
  };
}

function mcpSurfaceTitle(surface) {
  const normalized = normalizeMcpSurface(surface);
  if (normalized.kind === "bundle") {
    const bundle = normalized.bundle || {};
    const subject = bundle.subject || {};
    return [subject.type || "service", subject.name || subject.service || bundle.title || "MCP 聚合面板", subject.env || ""]
      .filter(Boolean)
      .join(" / ");
  }
  const card = normalized.card || {};
  const action = card.action || card.actions?.[0] || {};
  return formatMcpActionLabel(action) || card.title || "MCP 操作面板";
}

function mcpSurfaceSummary(surface) {
  const normalized = normalizeMcpSurface(surface);
  if (normalized.kind === "bundle") {
    const bundle = normalized.bundle || {};
    return compactText(bundle.summary || bundle.rootCause || bundle.root_cause || "MCP 聚合面板详情");
  }
  const card = normalized.card || {};
  const action = card.action || card.actions?.[0] || {};
  return compactText(card.summary || action.confirmText || action.description || "MCP 操作详情");
}

function mcpSurfaceItems(surface) {
  const normalized = normalizeMcpSurface(surface);
  if (normalized.kind === "bundle") {
    const bundle = normalized.bundle || {};
    const sections = asArray(bundle.sections);
    const subject = bundle.subject || {};
    const headRows = [
      { label: "类型", value: bundle.bundleKind || "mcp_bundle" },
      { label: "主题", value: [subject.type || "", subject.name || subject.service || ""].filter(Boolean).join(" / ") || "未指定" },
      { label: "范围", value: [bundle.scope?.service, bundle.scope?.hostId, bundle.scope?.env, bundle.scope?.cluster].filter(Boolean).join(" / ") || "未指定" },
      { label: "时效", value: bundle.freshness?.label || bundle.freshness?.capturedAt || "未声明" },
      { label: "分区", value: sections.length ? `${sections.length} 个分区` : "无分区" },
    ];
    const sectionRows = sections.map((section, index) => ({
      label: `${section?.title || section?.kind || `Section ${index + 1}`}`,
      value: [section?.summary || "", `${asArray(section?.cards).length || 0} 个卡片`].filter(Boolean).join(" · ") || "已聚合",
    }));
    return [...headRows, ...sectionRows];
  }

  const card = normalized.card || {};
  const action = card.action || card.actions?.[0] || {};
  const scope = asObject(card.scope || action.scope);
  return [
    { label: "类型", value: card.uiKind || "action_panel" },
    { label: "操作", value: formatMcpActionLabel(action) || card.title || "未命名操作" },
    { label: "目标", value: formatMcpActionTarget(action, scope) },
    { label: "权限路径", value: action.permissionPath || action.permission_path || "未声明" },
    { label: "作用范围", value: [scope.service, scope.hostId, scope.env, scope.cluster].filter(Boolean).join(" / ") || "未指定" },
    { label: "时效", value: card.freshness?.label || card.freshness?.capturedAt || "未声明" },
  ];
}

function openMcpSurfaceEvidence(surface) {
  const normalized = normalizeMcpSurface(surface);
  selectedMcpSurface.value = normalized;
  const hostId = normalized.kind === "bundle"
    ? compactText(normalized.bundle?.scope?.hostId || normalized.bundle?.hostId || "")
    : compactText(normalized.card?.scope?.hostId || normalized.card?.hostId || "");
  if (hostId) {
    selectedHostId.value = hostId;
  }
  evidenceSource.value = "mcp-surface";
  evidenceTab.value = MCP_SURFACE_TAB;
  evidenceOpen.value = true;
}

function handleMcpSurfaceDetail(surface) {
  openMcpSurfaceEvidence(surface);
  pushActionNotice(`${mcpSurfaceTitle(surface)} 已打开详情。`, "info");
}

function handleMcpSurfacePin(surface) {
  dispatchMcpDrawer(surface, true);
  pushActionNotice(`${mcpSurfaceTitle(surface)} 已固定到全局 MCP 抽屉。`, "info");
}

async function handleMcpSurfaceRefresh(surface) {
  pushActionNotice(`${mcpSurfaceTitle(surface)} 已请求刷新。`, "info");
  await refreshProtocolState();
}

function queueLocalMcpApproval(action) {
  const approval = buildSyntheticMcpApproval(action, {
    scope: action?.scope || {},
    summary: action?.confirmText || "等待你确认后继续执行该 MCP 变更操作。",
  });
  localMcpApprovals.value = [
    ...localMcpApprovals.value.filter((item) => item.id !== approval.id),
    approval,
  ];
  selectedApprovalId.value = approval.id;
  pushMcpEvent(action, {
    approvalId: approval.approvalId,
    hostId: approval.hostId,
    text: `${formatMcpActionLabel(action)} 已进入审批队列`,
    tone: "warning",
  });
  pushActionNotice(`${formatMcpActionLabel(action)} 已进入右侧审批栏。`, "warning");
}

function settleLocalMcpApproval(approval, decision) {
  localMcpApprovals.value = localMcpApprovals.value.filter((item) => item.id !== approval.id && item.approvalId !== approval.approvalId);
  const message = buildMcpDecisionNotice(approval.action || {}, decision);
  pushMcpEvent(approval.action || {}, {
    approvalId: approval.approvalId,
    hostId: approval.hostId,
    text: message,
    tone: decision === "decline" || decision === "reject" ? "warning" : "success",
  });
  pushActionNotice(message, decision === "decline" || decision === "reject" ? "warning" : "success");
  if (selectedApprovalId.value === approval.id) {
    selectedApprovalId.value = localMcpApprovals.value[0]?.id || "";
  }
}

function handleMcpAction(action) {
  if (!action || typeof action !== "object") return;
  if (isMcpMutationAction(action)) {
    queueLocalMcpApproval(action);
    return;
  }
  const message = `${formatMcpActionLabel(action)} 已作为只读操作加入当前工作台。`;
  pushMcpEvent(action, {
    text: message,
    tone: "info",
  });
  pushActionNotice(message, "info");
}

function openConversationHistory() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent(OPEN_SESSION_HISTORY_EVENT, {
      detail: { source: "protocol-history-sentinel" },
    }),
  );
}

const isWorkspaceSession = computed(() => store.snapshot.kind === "workspace");
const recentWorkspaceSession = computed(
  () => store.sessionList.find((session) => session.kind === "workspace" && session.id !== store.activeSessionId) || null,
);
const workspaceModel = computed(() => buildProtocolWorkspaceModel(store.snapshot, store.runtime));
const hostRows = computed(() => workspaceModel.value.hostRows || []);
const planCardModel = computed(() => workspaceModel.value.planCardModel || { stepItems: [] });
const choiceCards = computed(() => workspaceModel.value.choiceCards || []);
const workspaceApprovalItems = computed(() => workspaceModel.value.approvalItems || []);
const workspaceEventItems = computed(() => workspaceModel.value.eventItems || []);
const toolInvocations = computed(() => workspaceModel.value.toolInvocations || []);
const evidenceSummaries = computed(() => workspaceModel.value.evidenceSummaries || []);
const verificationRecords = computed(() => workspaceModel.value.verificationRecords || store.snapshot.verificationRecords || []);
const agentLoopRun = computed(() => workspaceModel.value.agentLoop || store.snapshot.agentLoop || null);
const waitingForUserAnswer = computed(() => {
  const loopStatus = compactText(agentLoopRun.value?.status);
  return loopStatus === "waiting_user" || workspaceModel.value.missionPhase === "waiting_input";
});
const waitingForApproval = computed(() => {
  const loopStatus = compactText(agentLoopRun.value?.status);
  return loopStatus === "waiting_approval" || workspaceModel.value.missionPhase === "waiting_approval";
});
const approvalItems = computed(() => [...workspaceApprovalItems.value, ...localMcpApprovals.value]);
const eventItems = computed(() => [...workspaceEventItems.value, ...localMcpEvents.value]);
const timelineItems = computed(() => [...eventItems.value].reverse());
const backgroundAgents = computed(() => workspaceModel.value.backgroundAgents || []);
const selectedAgentBackground = computed(() => {
  if (selectedAgentId.value) {
    return backgroundAgents.value.find((agent) => agent.id === selectedAgentId.value) || null;
  }
  return backgroundAgents.value[0] || null;
});
const selectedAgentHostRow = computed(() => {
  const agentId = compactText(selectedAgentBackground.value?.hostId || selectedAgentBackground.value?.id || selectedAgentId.value);
  if (!agentId) {
    return null;
  }
  return hostRows.value.find((row) => row.hostId === agentId) || null;
});
const selectedAgentDetail = computed(() =>
  buildProtocolAgentDetailModel({
    backgroundAgent: selectedAgentBackground.value,
    hostRow: selectedAgentHostRow.value,
    planCardModel: planCardModel.value,
    approvalItems: approvalItems.value,
    eventItems: eventItems.value,
    formattedTurns: formattedTurns.value,
  }),
);
const canRestartMission = computed(() => workspaceModel.value.nextSendStartsNewMission);
const statusBanner = computed(() => {
  const banner = workspaceModel.value.statusBanner;
  if (!banner || workspaceModel.value.canStopCurrentMission) return null;
  if (dismissedStatusBannerKeys.value.includes(statusBannerDismissKey(banner))) return null;
  return {
    cardId: banner.cardId,
    tone: banner.tone,
    title: banner.title,
    text: banner.detail,
    hint: canRestartMission.value ? "继续发送会在当前 workspace 会话里启动一轮新的 mission。" : "",
  };
});

const selectedApprovalItem = computed(() => {
  return resolveApprovalSelection(selectedApprovalId.value, approvalItems.value);
});

const activeApprovalCardId = computed(() => compactText(selectedApprovalItem.value?.id || approvalItems.value[0]?.id || ""));

const selectedStep = computed(() => {
  const steps = asArray(planCardModel.value.stepItems);
  const stepId = compactText(selectedStepId.value);
  if (!stepId) return null;
  return (
    steps.find((item) => compactText(item.id) === stepId) ||
    steps.find((item) => compactText(item.title) === stepId) ||
    (steps.length === 1 ? steps[0] : null)
  );
});
const selectedHostRow = computed(() => {
  if (selectedHostId.value) {
    const direct = hostRows.value.find((row) => row.hostId === selectedHostId.value);
    if (direct) return direct;
  }

  if (selectedApprovalItem.value?.hostId) {
    const approvalHost = hostRows.value.find((row) => row.hostId === selectedApprovalItem.value.hostId);
    if (approvalHost) return approvalHost;
  }

  const stepHostId = selectedStep.value?.hosts?.[0]?.id;
  if (stepHostId) {
    const stepHost = hostRows.value.find((row) => row.hostId === stepHostId);
    if (stepHost) return stepHost;
  }

  return (
    hostRows.value.find((row) => ["running", "waiting_approval", "queued"].includes(row.statusKey)) ||
    hostRows.value[0] ||
    null
  );
});

const canSendWorkspaceMessage = computed(() => {
  const canAnswerWaitingQuestion = waitingForUserAnswer.value && store.runtime.turn.active && !store.runtime.turn.pendingStart;
  return (
    isWorkspaceSession.value &&
    store.snapshot.auth?.connected !== false &&
    store.snapshot.config?.codexAlive !== false &&
    !store.sending &&
    (!store.runtime.turn.active || canAnswerWaitingQuestion) &&
    !store.runtime.turn.pendingStart
  );
});

const conversationSubtitle = computed(() => {
  if (workspaceModel.value.currentLane === "plan") {
    return "当前处于方案规划中：主 Agent 会先生成计划，再提交计划审批，不会直接执行变更。";
  }
  if (workspaceModel.value.currentLane === "readonly" && workspaceModel.value.requiredNextTool) {
    return `当前处于分析中：先完成 ${workspaceModel.value.requiredNextToolLabel || toolDisplayName(workspaceModel.value.requiredNextTool)}，再形成结论。`;
  }
  if (workspaceModel.value.currentLane === "execute") {
    return "当前处于受控执行中：仅会在已审批计划范围内推进派发和动作执行。";
  }
  if (workspaceModel.value.currentLane === "verify") {
    return "当前处于自动验证中：先核对验证结果和回滚提示，再给出最终结论。";
  }
  const summary = compactText(planCardModel.value.summary);
  if (summary) return summary;
  if (workspaceModel.value.missionPhase === "waiting_approval") return "主 Agent 已产出计划，当前正在等待审批继续推进。";
  return "通过主 Agent 对话直接看 plan、step 分配和 host-agent 执行状态。";
});

const starterCard = computed(() => {
  const connectedHost = compactText(store.snapshot.selectedHostId || selectedHostRow.value?.displayName || "server-local");
  const pendingApprovals = approvalItems.value.length;
  return {
    eyebrow: "SYSTEM CONTEXT",
    title: `${connectedHost} 已连接，工作台已就绪。`,
    text: "可以直接问我当前状态，或者描述你想处理的问题。",
    meta: pendingApprovals
      ? `当前有 ${pendingApprovals} 条待审批操作，处理后我会继续推进。`
      : "当前没有待审批操作。",
  };
});

const composerPrimaryActionOverride = computed(() => {
  if (waitingForUserAnswer.value) return "send";
  return workspaceModel.value.canStopCurrentMission ? "" : "send";
});

const composerPlaceholder = computed(() => {
  if (waitingForUserAnswer.value) return "当前等待澄清回答：可在卡片中选择，或直接输入你的答案";
  if (waitingForApproval.value) return "当前等待审批处理，处理后我会继续推进";
  if (workspaceModel.value.nextSendStartsNewMission) return "上一轮任务已结束，继续输入会在当前工作台启动新 mission";
  return "继续输入需求、约束或补充说明";
});

const planSummaryLabel = computed(() => {
  const total = Number(planCardModel.value.totalSteps || 0);
  const completed = Number(planCardModel.value.completedSteps || 0);
  if (!total) return "计划生成后，会在这里直接展示 step -> host-agent 映射。";
  return `共 ${total} 个任务，已完成 ${completed} 个`;
});

const planOverviewRows = computed(() => [
  compactRow("计划摘要", planCardModel.value.summary || planCardModel.value.title),
  compactRow("范围", planCardModel.value.scope),
  compactRow("风险", planCardModel.value.risk),
  compactRow("假设", planCardModel.value.assumptions),
  compactRow("验证", planCardModel.value.validation),
  compactRow("回滚", planCardModel.value.rollback),
].filter(Boolean));

const runtimePolicyCard = computed(() => {
  const modeLabel = compactText(workspaceModel.value.incidentSummary?.modeLabel || "分析模式");
  const stageLabel = compactText(workspaceModel.value.incidentSummary?.stageLabel || "待命");
  const laneLabel = compactText(workspaceModel.value.currentLaneLabel || "分析中");
  const gateLabel = compactText(workspaceModel.value.finalGateLabel || "待校验");
  const missingRequirements = asArray(workspaceModel.value.missingRequirements).map((item) => compactText(item)).filter(Boolean);
  const nextTool = compactText(workspaceModel.value.requiredNextTool);
  const intentLabel = compactText(workspaceModel.value.turnIntentLabel || "事实问答");
  const detail = missingRequirements.length
    ? `当前回答被 final gate 拦截，需先补齐 ${missingRequirements.join(" / ")}。`
    : compactText(workspaceModel.value.turnPolicy?.classificationReason || workspaceModel.value.incidentSummary?.detail || "当前没有额外 gate 限制。");
  return {
    modeLabel,
    stageLabel,
    laneLabel,
    intentLabel,
    gateLabel,
    nextTool,
    nextToolLabel: workspaceModel.value.requiredNextToolLabel || toolDisplayName(nextTool),
    missingRequirements,
    detail,
    blocked: compactText(workspaceModel.value.finalGateStatus) === "blocked",
  };
});

const planCards = computed(() => {
  const stepCards = asArray(planCardModel.value.stepItems).map((item) => ({
    id: item.id,
    step: {
      id: item.id,
      title: item.title,
      description: item.summary,
    },
    status: item.status,
    statusLabel: stepStatusLabel(item.status),
    hostAgent: item.hosts || [],
    detail: item.summary,
    note: item.constraints?.length ? `约束：${item.constraints.join(" / ")}` : "",
    tags: [
      ...(item.approvalCount ? [{ id: `${item.id}-approval`, label: `待审批 ${item.approvalCount}` }] : []),
      ...asArray(item.constraints).slice(0, 2).map((constraint, index) => ({
        id: `${item.id}-constraint-${index}`,
        label: constraint,
      })),
    ],
    actions: [
      {
        id: `${item.id}-evidence`,
        key: "evidence",
        label: "查看证据",
      },
    ],
    index: item.index,
  }));
  if (stepCards.length) return stepCards;

  const hasPlanProjection = Boolean(
    workspaceModel.value.cards?.planCard ||
      compactText(planCardModel.value.summary) ||
      compactText(planCardModel.value.generatedAt),
  );
  if (!hasPlanProjection) return [];

  return [
    {
      id: "plan-projection",
      step: {
        id: "plan-projection",
        title: compactText(planCardModel.value.title || "主 Agent 已生成计划摘要"),
        description: compactText(planCardModel.value.summary || conversationSubtitle.value),
      },
      status: workspaceModel.value.missionPhase || "planning",
      statusLabel: normalizePhaseLabel(workspaceModel.value.missionPhase),
      hostAgent: [],
      detail: compactText(planCardModel.value.summary || "已收到计划投影，正在同步具体步骤。"),
      note: "当前还在整理 step -> host-agent 映射，稍后会直接显示在这里。",
      tags: [{ id: "plan-projection-tag", label: "已收到计划投影" }],
      actions: [{ id: "plan-projection-evidence", key: "evidence", label: "查看证据" }],
      index: 1,
    },
  ];
});

const conversationStatusCard = computed(() => workspaceModel.value.conversationStatusCard || null);
const formattedTurns = computed(() => workspaceModel.value.formattedTurns || []);

const filteredEventItems = computed(() => {
  const selectedHost = selectedHostRow.value?.hostId;
  if (!selectedHost) return eventItems.value;
  return eventItems.value.filter((item) => !item.hostId || item.hostId === selectedHost || item.targetType === "dispatch");
});

const selectedToolInvocation = computed(() => {
  if (!selectedToolInvocationId.value) return null;
  return toolInvocations.value.find((item) => item.id === selectedToolInvocationId.value) || null;
});

const selectedToolEvidence = computed(() => {
  const evidenceId = compactText(selectedToolInvocation.value?.evidenceId);
  if (!evidenceId) return selectedToolInvocation.value?.evidence || null;
  return evidenceSummaries.value.find((item) => item.id === evidenceId) || selectedToolInvocation.value?.evidence || null;
});

const selectedVerificationRecord = computed(() => resolveVerificationSelection(selectedVerificationId.value, verificationRecords.value));
const selectedTimelineEvent = computed(() => resolveTimelineSelection(selectedEventId.value, timelineItems.value));
const selectedCitationEvidence = computed(() => {
  const evidenceId = compactText(selectedCitationEvidenceId.value);
  if (!evidenceId) return null;
  return evidenceSummaries.value.find((item) => compactText(item.id) === evidenceId) || null;
});
const selectedCitationRelatedEvidence = computed(() => {
  const projectionMatches = resolveProjectionTargets(selectedCitationEvidence.value, "evidence", evidenceSummaries.value);
  if (projectionMatches.length) return projectionMatches;
  const relatedIds = asArray(selectedCitationEvidence.value?.relatedEvidenceIds).map((item) => compactText(item)).filter(Boolean);
  if (!relatedIds.length) return [];
  return relatedIds
    .map((evidenceId) => evidenceSummaries.value.find((item) => compactText(item.id) === evidenceId) || null)
    .filter(Boolean);
});

const selectedToolApprovalItem = computed(() => {
  const projectedApproval = resolveProjectionTarget(selectedToolInvocation.value, "approval", approvalItems.value);
  if (projectedApproval) return projectedApproval;
  const invocation = selectedToolInvocation.value;
  if (!invocation) return null;
  const approvalId = compactText(invocation.output?.approval?.requestId || invocation.input?.approvalId || selectedToolEvidence.value?.metadata?.approvalId);
  const cardId = compactText(selectedToolEvidence.value?.metadata?.cardId || invocation.id.replace(/^tool-/, ""));
  return approvalItems.value.find((item) =>
    (approvalId && compactText(item.approvalId) === approvalId) ||
    (cardId && compactText(item.raw?.id || item.id) === cardId),
  ) || null;
});

const selectedApprovalVerification = computed(() =>
  resolveProjectionTarget(selectedApprovalItem.value, "verification", verificationRecords.value) ||
  resolveVerificationForApproval(selectedApprovalItem.value, verificationRecords.value),
);
const selectedApprovalTimelineEvent = computed(() =>
  resolveProjectionTarget(selectedApprovalItem.value, "event", timelineItems.value) ||
  resolveTimelineEventForApproval(selectedApprovalItem.value, timelineItems.value),
);
const selectedVerificationApproval = computed(() =>
  resolveProjectionTarget(selectedVerificationRecord.value, "approval", approvalItems.value) ||
  resolveApprovalForVerification(selectedVerificationRecord.value, approvalItems.value),
);
const selectedVerificationTimelineEvent = computed(() =>
  resolveProjectionTarget(selectedVerificationRecord.value, "event", timelineItems.value) ||
  resolveTimelineEventForVerification(selectedVerificationRecord.value, timelineItems.value),
);

const evidenceBase = computed(() =>
  buildProtocolEvidenceTabs({
    planCardModel: planCardModel.value,
    hostRow: selectedHostRow.value,
    approvalItem: selectedApprovalItem.value,
    verificationItem: selectedVerificationRecord.value,
    verificationRecords: verificationRecords.value,
    eventItems: filteredEventItems.value,
  }),
);

const citationEvidencePanel = computed(() => {
  const evidence = selectedCitationEvidence.value || {};
  const rows = [
    compactRow("Citation", evidence.citationKey),
    compactRow("Evidence ID", evidence.id),
    compactRow("类型", evidence.kind),
    compactRow("来源类型", evidence.sourceKind),
    compactRow("来源引用", evidence.sourceRef),
    compactRow("标题", evidence.title),
    compactRow("摘要", evidence.summary),
    compactRow("原始内容摘要", previewText(evidence.content)),
    compactRow("创建时间", evidence.createdAt),
    ...objectRows(evidence.metadata),
    ...selectedCitationRelatedEvidence.value.map((item, index) => ({
      label: `关联证据 ${index + 1}`,
      value: [compactText(item.citationKey || item.id), compactText(item.title || item.summary)].filter(Boolean).join(" · "),
    })),
  ].filter(Boolean);
  return {
    title: evidence.title || evidence.citationKey || "证据摘要",
    summary: evidence.summary || "这里展示当前结论引用的 evidence 摘要和原始内容。",
    items: rows.length
      ? rows
      : [{ label: "证据", value: "当前没有可展示的 evidence 详情。" }],
    raw: evidence.content || evidence.summary || evidence || "",
  };
});

const mainAgentPlanPanel = computed(() => {
  if (evidenceSource.value === "process" && selectedProcessEvidence.value) {
    const item = selectedProcessEvidence.value;
    const rows = [
      { label: "过程消息", value: compactText(item.text || item.title || item.summary || "暂无过程消息。") },
      compactText(item.detail) ? { label: "补充说明", value: compactText(item.detail) } : null,
      compactText(item.time) ? { label: "时间", value: compactText(item.time) } : null,
      compactText(item.status) ? { label: "状态", value: compactText(item.status) } : null,
      compactText(item.hostId) ? { label: "Host", value: compactText(item.hostId) } : null,
    ].filter(Boolean);
    return {
      title: "过程详情",
      summary: "这里展示你点击的过程项本身，避免跳到空的计划摘要。",
      items: rows,
      raw: item,
    };
  }
  if (evidenceSource.value === "step") {
    const step = selectedStep.value;
    const projectionCard = step
      ? null
      : planCards.value.find((card) => compactText(card.id) === compactText(selectedStepId.value)) || planCards.value[0] || null;
    const projectionStep = asObject(projectionCard?.step);
    const displayStep = step || (projectionCard
      ? {
          id: projectionCard.id,
          title: projectionStep.title || projectionCard.title,
          summary: projectionStep.description || projectionCard.detail || projectionCard.note,
          status: projectionCard.status,
          statusLabel: projectionCard.statusLabel,
          hosts: projectionCard.hostAgent || [],
        }
      : null);
    const context = dispatchContextForStep(displayStep);
    const hostLabels = (context.matchingHosts.length ? context.matchingHosts : asArray(displayStep?.hosts))
      .map((host) => compactText(host?.displayName || host?.label || host?.hostId || host?.id || host))
      .filter(Boolean)
      .join("、");
    const constraints = [
      ...asArray(displayStep?.constraints),
      ...asArray(context.taskBinding.constraints),
      ...asArray(context.request.constraints),
    ]
      .map((item) => compactText(item))
      .filter(Boolean)
      .join(" / ");
    const dispatchedTask = firstCompactValue(
      context.request.instruction,
      context.taskBinding.instruction,
      context.request.summary,
      context.request.text,
      context.taskBinding.summary,
      displayStep?.summary,
      planCardModel.value.summary,
      displayStep?.title,
      context.request.title,
      context.taskBinding.title,
      planCardModel.value.title,
    );
    const commandOrTarget = firstCompactValue(
      context.request.command,
      context.request.shell,
      context.request.query,
      context.request.summary && context.request.summary !== dispatchedTask ? context.request.summary : "",
    );
    const rows = [
      compactRow("Step", displayStep?.id || selectedStepId.value || "plan-projection"),
      compactRow("子任务标题", displayStep?.title || context.taskBinding.title || context.request.title || planCardModel.value.title),
      compactRow("发送给子 Agent 的任务", dispatchedTask || "主 Agent 还没有同步到具体子任务，当前只收到计划投影。"),
      compactRow("目标 Host", hostLabels || context.row?.displayName || context.row?.hostId),
      compactRow("命令 / 检查线索", commandOrTarget),
      compactRow("约束", constraints),
      compactRow("状态", displayStep?.statusLabel || displayStep?.status || normalizePhaseLabel(workspaceModel.value.missionPhase)),
      compactRow("Worker Session", context.worker.sessionId || context.worker.session_id || context.row?.workerSession),
      compactRow("Worker Thread", context.worker.threadId || context.worker.thread_id),
    ].filter(Boolean);
    return {
      title: "任务派发证据",
      summary: step
        ? "这里展示主 Agent 拆出的子任务，以及实际同步给子 Agent / host-agent 的任务内容。"
        : "当前还没有完整的 step -> host-agent 映射，下面展示主 Agent 计划投影里准备派发或已经记录的任务内容。",
      items: rows.length ? rows : [{ label: "状态", value: "主 Agent 还没有同步可展示的任务派发内容。" }],
      raw: {
        planStep: displayStep || null,
        dispatch: context.dispatch,
        dispatchRequest: context.request,
        taskBinding: context.taskBinding,
        worker: context.worker,
      },
    };
  }
  const items = [];
  const hasPlanSummary = Boolean(
    compactText(planCardModel.value.summary) || asArray(planCardModel.value.stepItems).length || compactText(planCardModel.value.generatedAt),
  );
  if (hasPlanSummary) {
    items.push({
      label: "计划摘要",
      value: compactText(planCardModel.value.summary || "当前还没有可用的计划摘要。"),
    });
  }
  for (const row of [
    compactRow("范围", planCardModel.value.scope),
    compactRow("风险", planCardModel.value.risk),
    compactRow("假设", planCardModel.value.assumptions),
    compactRow("验证", planCardModel.value.validation),
    compactRow("回滚", planCardModel.value.rollback),
  ].filter(Boolean)) {
    items.push(row);
  }
  for (const [index, step] of asArray(planCardModel.value.stepItems).entries()) {
    const hostNames = asArray(step.hosts).map((host) => compactText(host.label || host.id)).filter(Boolean).join("、");
    items.push({
      label: `Step ${step.index || index + 1}`,
      value: [step.title, step.statusLabel || step.status, hostNames ? `Host: ${hostNames}` : ""]
        .map((part) => compactText(part))
        .filter(Boolean)
        .join(" · "),
    });
  }
  return {
    title: "主 Agent 计划摘要",
    summary: compactText(planCardModel.value.summary || "查看主 Agent 如何生成并拆分 plan。"),
    items: items.length ? items : [{ label: "状态", value: "当前还没有可用的计划摘要。" }],
    raw: {
      version: planCardModel.value.version,
      generatedAt: planCardModel.value.generatedAt,
    },
  };
});

const workerConversationPanel = computed(() => {
  const transcript = evidenceBase.value.workerConversation.length
    ? evidenceBase.value.workerConversation.map((item) => ({
        label: item.time || item.title || "Worker",
        value: item.text,
      }))
    : asArray(selectedHostRow.value?.worker?.transcript).map((item, index) => ({
        label: `Transcript ${index + 1}`,
        value: String(item ?? ""),
      }));

  return {
    title: `${selectedHostRow.value?.displayName || "Worker"} 对话`,
    summary: selectedHostRow.value?.taskTitle || "当前 host-agent 与 AI 的对话摘录。",
    items: transcript.length ? transcript : [{ label: "状态", value: "当前 host-agent 还没有可展示的对话摘录。" }],
    raw: selectedHostRow.value?.worker || null,
  };
});

const hostTerminalPanel = computed(() => {
  if (evidenceSource.value === "command" && selectedCommandEvidence.value) {
    const command = commandEvidenceFrom(selectedCommandEvidence.value);
    const output = commandOutputText(command);
    const rows = [
      { label: "Host", value: compactText(command.hostId || selectedHostRow.value?.displayName || selectedHostRow.value?.hostId || "local") },
      { label: "Status", value: commandStatusLabel(command.status) },
      { label: "Command", value: compactText(command.command || command.rawCommand || "未提供命令") },
      compactText(command.cwd) ? { label: "Cwd", value: compactText(command.cwd) } : null,
      command.exitCode !== undefined && command.exitCode !== null ? { label: "Exit Code", value: String(command.exitCode) } : null,
      command.durationMs ? { label: "Duration", value: `${command.durationMs}ms` } : null,
    ].filter(Boolean);
    return {
      title: command.command || "Command terminal",
      summary: "实际执行的命令、状态和终端输出。",
      items: rows,
      raw: [`$ ${command.rawCommand || command.command}`, output || "（命令没有输出）"].filter(Boolean).join("\n\n"),
    };
  }
  return {
    title: `${selectedHostRow.value?.displayName || "Host"} terminal`,
    summary: selectedHostRow.value?.summary || "查看当前 host-agent 对应主机的终端输出。",
    items: asArray(evidenceBase.value.hostTerminalRows).map((row) => ({
      label: row.label || row.key,
      value: row.value || row.text,
    })),
    raw: evidenceBase.value.hostTerminalOutput || selectedHostRow.value?.worker?.terminal || "",
  };
});

const approvalContextPanel = computed(() => {
  const rows = [];
  if (selectedApprovalItem.value) {
    rows.push(
      { label: "主机", value: selectedApprovalItem.value.hostName || selectedApprovalItem.value.hostId || "未指定" },
      { label: "审批ID", value: selectedApprovalItem.value.approvalId || selectedApprovalItem.value.id || "未提供" },
      { label: selectedApprovalItem.value.kind === "plan" ? "计划" : "命令", value: selectedApprovalItem.value.command || selectedApprovalItem.value.summary || "未提供命令或计划" },
    );
    rows.push(
      ...asArray(selectedApprovalItem.value.detailRows).map((item) => ({
        label: compactText(item.label || "详情"),
        value: compactText(item.value || item.text),
      })),
    );
  } else if (evidenceBase.value.approvalContext.length) {
    rows.push(
      ...evidenceBase.value.approvalContext.map((item) => ({
        label: item.label || item.title || "审批上下文",
        value: item.value || item.text,
      })),
    );
  }

  return {
    title: "审批上下文",
    summary: selectedApprovalItem.value
      ? "通过弹框查看当前审批所关联的命令、主机和证据。"
      : "当前没有待处理的审批上下文。",
    actions: [
      selectedApprovalItem.value
        ? { id: "focus-approval", label: "定位审批卡", kind: "focus_approval", approvalId: selectedApprovalItem.value.id }
        : null,
      selectedApprovalTimelineEvent.value
        ? { id: "focus-approval-event", label: "定位时间线事件", kind: "focus_event", eventId: selectedApprovalTimelineEvent.value.id }
        : null,
      selectedApprovalVerification.value
        ? {
            id: "open-approval-verification",
            label: "查看验证结果",
            kind: "open_verification",
            verificationId: selectedApprovalVerification.value.id,
            hostId: selectedApprovalVerification.value.hostId,
          }
        : null,
    ].filter(Boolean),
    items: rows.length
      ? rows
      : [
          {
            label: "状态",
            value: "当前没有待处理的审批上下文。",
          },
        ],
    raw: selectedApprovalItem.value?.raw || null,
  };
});

const verificationPanel = computed(() => {
  const rows = asArray(evidenceBase.value.verificationResults).map((item) => ({
    label: compactText(item.title || item.label || "验证结果"),
    value: compactText(item.text || item.value),
  }));
  return {
    title: "验证结果",
    summary: selectedVerificationRecord.value
      ? "这里展示当前动作的自动验证结论、策略和回滚建议。"
      : "这里汇总当前动作关联的自动验证结果。",
    actions: [
      selectedVerificationApproval.value
        ? {
            id: "open-verification-approval",
            label: "查看审批上下文",
            kind: "open_approval",
            approvalId: selectedVerificationApproval.value.id,
            hostId: selectedVerificationApproval.value.hostId,
          }
        : null,
      selectedVerificationApproval.value
        ? {
            id: "focus-verification-approval",
            label: "定位审批卡",
            kind: "focus_approval",
            approvalId: selectedVerificationApproval.value.id,
          }
        : null,
      selectedVerificationTimelineEvent.value
        ? {
            id: "focus-verification-event",
            label: "定位时间线事件",
            kind: "focus_event",
            eventId: selectedVerificationTimelineEvent.value.id,
          }
        : null,
    ].filter(Boolean),
    items: rows.length
      ? rows
      : [
          {
            label: "状态",
            value: "当前还没有可展示的验证结果。",
          },
        ],
    raw: selectedVerificationRecord.value?.raw || verificationRecords.value || null,
  };
});

const toolInputPanel = computed(() => {
  const invocation = selectedToolInvocation.value || {};
  const rows = [
    compactRow("工具", toolDisplayName(invocation)),
    compactRow("工具名", invocation.name),
    compactRow("状态", invocation.status),
    compactRow("风险级别", invocation.riskLevel),
    compactRow("目标范围", invocation.targetSummary),
    compactRow("需要审批", invocation.requiresApproval ? "是" : ""),
    compactRow("Dry-run", invocation.dryRunSupported ? "支持" : ""),
    compactRow("输入摘要", invocation.inputSummary),
    compactRow("开始时间", invocation.startedAt),
    compactRow("结束时间", invocation.completedAt),
    ...objectRows(invocation.input),
  ].filter(Boolean);
  return {
    title: "工具输入",
    summary: "模型请求执行该工具时传入的结构化参数。",
    items: rows.length ? rows : [{ label: "状态", value: "当前工具调用没有可展示的输入。" }],
    raw: invocation.input || invocation.inputJson || "",
  };
});

const toolOutputPanel = computed(() => {
  const invocation = selectedToolInvocation.value || {};
  const rows = [
    compactRow("输出摘要", invocation.outputSummary),
    compactRow("Evidence", invocation.evidenceId),
    ...objectRows(invocation.output),
  ].filter(Boolean);
  return {
    title: "工具输出",
    summary: "ai-server 记录的工具结果摘要。完整内容可在原始证据 tab 查看。",
    items: rows.length ? rows : [{ label: "状态", value: "当前工具调用还没有可展示的输出。" }],
    raw: invocation.output || invocation.outputJson || "",
  };
});

const rawEvidencePanel = computed(() => {
  const evidence = selectedToolEvidence.value || {};
  return {
    title: evidence.title || "原始证据",
    summary: evidence.summary || "工具调用关联的 evidence 记录。",
    items: [
      compactRow("Evidence ID", evidence.id),
      compactRow("Invocation ID", evidence.invocationId),
      compactRow("类型", evidence.kind),
      compactRow("创建时间", evidence.createdAt),
      ...objectRows(evidence.metadata),
    ].filter(Boolean),
    raw: evidence.content || evidence || "",
  };
});

const toolLinkedPlanPanel = computed(() => {
  const invocation = selectedToolInvocation.value || {};
  const input = asObject(invocation.input);
  const evidence = selectedToolEvidence.value || {};
  const metadata = asObject(evidence.metadata);
  const rows = [
    compactRow("计划标题", input.title || evidence.title || metadata.title),
    compactRow("计划摘要", input.summary || input.plan || evidence.summary),
    compactRow("风险", input.risk || input.risks),
    compactRow("假设", input.assumptions),
    compactRow("回滚", input.rollback),
    compactRow("验证方式", input.validation),
    compactRow("关联 Evidence", evidence.id),
  ].filter(Boolean);
  for (const [index, task] of asArray(input.tasks || input.steps).entries()) {
    rows.push({
      label: `任务 ${index + 1}`,
      value: [
        compactText(task.taskId || task.id),
        compactText(task.hostId),
        compactText(task.title || task.instruction || task.description),
      ].filter(Boolean).join(" · "),
    });
  }
  return {
    title: "关联计划",
    summary: "展示该工具调用关联的计划摘要、风险、验证和候选任务。",
    items: rows.length ? rows : [{ label: "状态", value: "当前工具调用没有关联计划内容。" }],
    raw: input.plan || input.summary || input,
  };
});

const toolLinkedApprovalPanel = computed(() => {
  const approval = selectedToolApprovalItem.value;
  if (!approval) {
    return {
      title: "关联审批",
      summary: "当前工具调用没有关联待处理审批。",
      items: [{ label: "状态", value: "没有找到关联审批。" }],
      raw: "",
    };
  }
  return {
    title: approval.kind === "plan" ? "计划审批" : "关联审批",
    summary: approval.summary || approval.command || "当前工具调用关联的审批上下文。",
    items: [
      compactRow("审批ID", approval.approvalId || approval.id),
      compactRow("类型", approval.kind === "plan" ? "计划审批" : "操作审批"),
      compactRow(approval.kind === "plan" ? "计划" : "命令", approval.command || approval.summary),
      ...asArray(approval.detailRows).map((item) => ({
        label: compactText(item.label || "详情"),
        value: compactText(item.value || item.text),
      })),
    ].filter((item) => item && (item.label || item.value)),
    raw: approval.raw || approval,
  };
});

const toolLinkedWorkerPanel = computed(() => {
  const invocation = selectedToolInvocation.value || {};
  const input = asObject(invocation.input);
  const tasks = asArray(input.tasks);
  const rows = tasks.flatMap((task, index) => [
    compactRow(`Task ${index + 1}`, compactText(task.title || task.taskId || task.id)),
    compactRow(`Host ${index + 1}`, task.hostId),
    compactRow(`Instruction ${index + 1}`, task.instruction),
    compactRow(`Constraints ${index + 1}`, asArray(task.constraints).map((item) => compactText(item)).filter(Boolean).join(" / ")),
  ]).filter(Boolean);
  return {
    title: "关联 Worker",
    summary: "展示主 Agent 准备派发给子 Agent / host-agent 的任务全文。",
    items: rows.length ? rows : [{ label: "状态", value: "当前工具调用没有关联 worker 派发任务。" }],
    raw: tasks.length ? tasks : input,
  };
});

const mcpSurfacePanel = computed(() => {
  if (!selectedMcpSurface.value) {
    return {
      title: "MCP 面板",
      summary: "当前没有选中的 MCP 面板。",
      items: [{ label: "状态", value: "尚未选择任何 MCP surface。" }],
      raw: "",
      sections: [],
    };
  }

  const surface = selectedMcpSurface.value;
  return {
    title: mcpSurfaceTitle(surface),
    summary: mcpSurfaceSummary(surface),
    items: mcpSurfaceItems(surface),
    raw: surface,
    sections: surface.kind === "bundle"
      ? asArray(surface.bundle?.sections).map((section) => ({
          ...section,
          cards: asArray(section?.cards),
        }))
      : [],
  };
});

const evidencePanels = computed(() => ({
  "citation-evidence": citationEvidencePanel.value,
  "main-agent-plan": mainAgentPlanPanel.value,
  "worker-conversation": workerConversationPanel.value,
  "host-terminal": hostTerminalPanel.value,
  [MCP_SURFACE_TAB]: mcpSurfacePanel.value,
  "approval-context": approvalContextPanel.value,
  "verification-results": verificationPanel.value,
  "tool-input": toolInputPanel.value,
  "tool-output": toolOutputPanel.value,
  "raw-evidence": rawEvidencePanel.value,
  "linked-plan": toolLinkedPlanPanel.value,
  "linked-approval": toolLinkedApprovalPanel.value,
  "linked-worker": toolLinkedWorkerPanel.value,
}));

const primaryEvidenceTabLabel = computed(() => {
  if (evidenceSource.value === "step") return "任务派发";
  if (evidenceSource.value === "process") return "过程详情";
  return "主 Agent 计划摘要";
});

const evidenceTabs = computed(() => [
  ...(evidenceSource.value === "citation"
    ? [
        { value: "citation-evidence", label: "证据摘要", badge: citationEvidencePanel.value.items?.length || 0 },
      ]
    : evidenceSource.value === "tool_invocation"
	    ? [
	        { value: "tool-input", label: "输入", badge: toolInputPanel.value.items?.length || 0 },
	        { value: "tool-output", label: "输出", badge: toolOutputPanel.value.items?.length || 0 },
	        { value: "raw-evidence", label: "原始 evidence", badge: rawEvidencePanel.value.items?.length || 0 },
	        { value: "linked-approval", label: "关联审批", badge: toolLinkedApprovalPanel.value.items?.length || 0 },
	        { value: "linked-worker", label: "关联 worker", badge: toolLinkedWorkerPanel.value.items?.length || 0 },
	        { value: "linked-plan", label: "关联计划", badge: toolLinkedPlanPanel.value.items?.length || 0 },
	      ]
    : [
        { value: "main-agent-plan", label: primaryEvidenceTabLabel.value, badge: mainAgentPlanPanel.value.items?.length || 0 },
        { value: "worker-conversation", label: "Worker 对话", badge: workerConversationPanel.value.items?.length || 0 },
        { value: "host-terminal", label: "Host Terminal", badge: hostTerminalPanel.value.items?.length || 0 },
        { value: MCP_SURFACE_TAB, label: "MCP 面板", badge: mcpSurfacePanel.value.items?.length || 0 },
        { value: "approval-context", label: "审批上下文", badge: approvalContextPanel.value.items?.length || 0 },
        { value: "verification-results", label: "验证结果", badge: verificationPanel.value.items?.length || 0 },
      ]),
]);

const evidenceTitle = computed(() => {
  if (evidenceSource.value === "citation" && selectedCitationEvidence.value) {
    return `证据摘要 · ${selectedCitationEvidence.value.citationKey || selectedCitationEvidence.value.id || "Evidence"}`;
  }
  if (evidenceSource.value === "verification" && selectedVerificationRecord.value) {
    return `验证结果 · ${selectedVerificationRecord.value.hostName || selectedVerificationRecord.value.hostId || "Host"}`;
  }
  if (evidenceSource.value === "approval" && selectedApprovalItem.value) {
    return `审批上下文 · ${selectedApprovalItem.value.hostName || selectedApprovalItem.value.hostId || "Host"}`;
  }
  if (evidenceSource.value === "step" && selectedStep.value) {
    return `任务派发证据 · ${selectedStep.value.title}`;
  }
  if (evidenceSource.value === "step") {
    const projectionCard = planCards.value.find((card) => compactText(card.id) === compactText(selectedStepId.value)) || planCards.value[0] || null;
    const title = compactText(projectionCard?.step?.title || projectionCard?.title || planCardModel.value.title);
    return title ? `任务派发证据 · ${title}` : "任务派发证据";
  }
  if (evidenceSource.value === "host" && selectedHostRow.value) {
    const hostLabel = selectedHostRow.value.displayName || selectedHostRow.value.hostId || "Host";
    if (evidenceTab.value === "host-terminal") {
      return `命令执行详情 · ${hostLabel}`;
    }
    return `执行详情 · ${hostLabel}`;
  }
  if (evidenceSource.value === "command" && selectedCommandEvidence.value) {
    const command = commandEvidenceFrom(selectedCommandEvidence.value);
    return `命令执行详情 · ${command.command || command.hostId || "local"}`;
  }
  if (evidenceSource.value === "tool_invocation" && selectedToolInvocation.value) {
    return `${toolDisplayName(selectedToolInvocation.value)} · ${selectedToolInvocation.value.inputSummary || selectedToolInvocation.value.id}`;
  }
  if (evidenceSource.value === "process" && selectedProcessEvidence.value) {
    return "过程详情";
  }
  if (evidenceSource.value === "mcp-surface" && selectedMcpSurface.value) {
    return `MCP 面板 · ${mcpSurfaceTitle(selectedMcpSurface.value)}`;
  }
  if (evidenceSource.value === "message" || evidenceSource.value === "dispatch" || evidenceSource.value === "event") {
    return "主 Agent 计划摘要";
  }
  return "执行证据";
});

const evidenceSubtitle = computed(() => {
  if (evidenceSource.value === "citation") {
    return "这里展示当前结论引用的 evidence 摘要和原始内容，不把长证据正文直接回灌到消息气泡。";
  }
  if (evidenceSource.value === "verification") {
    return "这里展示动作执行后的自动验证结论，以及可复用的回滚提示。";
  }
  if (evidenceSource.value === "approval") {
    return "审批详情通过弹框查看，不占用主页面空间。";
  }
  if (evidenceSource.value === "host") {
    return "这里汇总当前 worker 对话和 Host Terminal 上下文。";
  }
  if (evidenceSource.value === "command") {
    return "这里展示实际执行的命令、状态和终端输出。";
  }
  if (evidenceSource.value === "tool_invocation") {
    return "这里按工具调用展示输入、输出、原始 evidence 以及关联审批 / worker / 计划，避免落到空的计划摘要。";
  }
  if (evidenceSource.value === "process") {
    return "这里展示你点击的过程消息本身；如需命令输出，请点击命令类过程项或右侧实时事件。";
  }
  if (evidenceSource.value === "mcp-surface") {
    return "这里展示当前 MCP 面板的完整详情，不会把长图表和长表格重新灌回正文。";
  }
  if (evidenceSource.value === "step" || evidenceSource.value === "message" || evidenceSource.value === "dispatch" || evidenceSource.value === "event") {
    if (evidenceSource.value === "step") {
      return "这里展示主 Agent 拆出的子任务，以及同步给子 Agent / host-agent 的任务内容。";
    }
    return "这里汇总主 Agent 计划摘要、Worker 对话、Host Terminal 与审批上下文。";
  }
  return "按 tab 切换主 Agent 计划摘要、Worker 对话、Host Terminal 与审批上下文。";
});

function captureEvidenceDrawerSnapshot(activeTab = evidenceTab.value) {
  const tabs = cloneStructuredValue(evidenceTabs.value);
  const panels = cloneStructuredValue(evidencePanels.value);
  const normalizedTabs = Array.isArray(tabs) ? tabs : [];
  const preferredTab = compactText(activeTab);
  const fallbackTab = compactText(normalizedTabs[0]?.value || "main-agent-plan");
  return {
    title: compactText(evidenceTitle.value || "证据抽屉"),
    subtitle: compactText(evidenceSubtitle.value || "把当前重细节内容固定到侧边抽屉，方便边看边对照主线程。"),
    tabs: normalizedTabs,
    panels: panels && typeof panels === "object" ? panels : {},
    activeTab:
      preferredTab && normalizedTabs.some((tab) => compactText(tab?.value) === preferredTab)
        ? preferredTab
        : fallbackTab,
  };
}

const promptDebugState = computed(() => {
  const turnPolicy = asObject(workspaceModel.value.turnPolicy);
  const promptEnvelope = asObject(workspaceModel.value.promptEnvelope);
  const staticSections = asArray(promptEnvelope.staticSections);
  const laneSections = asArray(promptEnvelope.laneSections);
  const contextAttachments = asArray(promptEnvelope.contextAttachments);
  const runtimePolicySection = asObject(promptEnvelope.runtimePolicy);
  const visibleTools = asArray(promptEnvelope.visibleTools);
  const hiddenTools = asArray(promptEnvelope.hiddenTools);
  const missingRequirements = asArray(workspaceModel.value.missingRequirements).map((item) => compactText(item)).filter(Boolean);
  return {
    title: "Prompt Debug",
    subtitle: "查看本轮发给模型的上下文、tool visibility 与 final gate 命中情况。",
    tabs: [
      { value: "runtime-policy", label: "Runtime Policy", badge: 6 },
      { value: "final-gate", label: "Final Gate", badge: missingRequirements.length },
      { value: "prompt-context", label: "Prompt Context", badge: staticSections.length + laneSections.length + contextAttachments.length + (runtimePolicySection.name ? 1 : 0) },
      { value: "tool-visibility", label: "Tool Visibility", badge: visibleTools.length },
    ],
    panels: {
      "runtime-policy": {
        title: "Turn Policy",
        summary: "当前 turn classifier 产出的 intent、lane 与制度化约束。",
        items: [
          compactRow("Intent", workspaceModel.value.turnIntentLabel),
          compactRow("Lane", workspaceModel.value.currentLaneLabel),
          compactRow("Current Mode", workspaceModel.value.incidentSummary?.modeLabel),
          compactRow("Current Stage", workspaceModel.value.incidentSummary?.stageLabel),
          compactRow("Required Next Tool", workspaceModel.value.requiredNextToolLabel || toolDisplayName(workspaceModel.value.requiredNextTool || "")),
          compactRow("Classification Reason", turnPolicy.classificationReason),
        ].filter(Boolean),
        raw: turnPolicy,
      },
      "final-gate": {
        title: "Final Answer Gate",
        summary: "解释当前回答为什么被放行、待校验或被拦截。",
        items: [
          compactRow("Gate Status", workspaceModel.value.finalGateLabel),
          compactRow("Required Next Tool", workspaceModel.value.requiredNextTool ? `${workspaceModel.value.requiredNextToolLabel || toolDisplayName(workspaceModel.value.requiredNextTool)} (${workspaceModel.value.requiredNextTool})` : ""),
          compactRow("Missing Requirements", missingRequirements.join(" / ")),
        ].filter(Boolean),
        raw: {
          finalGateStatus: workspaceModel.value.finalGateStatus,
          missingRequirements,
        },
      },
      "prompt-context": {
        title: "Prompt Envelope",
        summary: "按静态提示、lane 提示和上下文附件查看本轮 prompt 组装结果。",
        items: [
          compactRow("Compression", promptEnvelope.compressionState),
          compactRow("Token Estimate", promptEnvelope.tokenEstimate ? String(promptEnvelope.tokenEstimate) : ""),
          ...staticSections.map((section) => compactRow(`Static · ${section.name}`, previewText(section.content, 280))),
          ...laneSections.map((section) => compactRow(`Lane · ${section.name}`, previewText(section.content, 280))),
          ...(runtimePolicySection.name ? [compactRow(`Policy · ${runtimePolicySection.name}`, previewText(runtimePolicySection.content, 280))] : []),
          ...contextAttachments.map((section) => compactRow(`Context · ${section.name}`, previewText(section.content, 280))),
        ].filter(Boolean),
        raw: promptEnvelope,
      },
      "tool-visibility": {
        title: "Tool Visibility",
        summary: "展示本轮对模型可见和被隐藏的工具，以及对应原因。",
        items: [
          ...visibleTools.map((tool) => compactRow(`Visible · ${tool.displayName || toolDisplayName(tool)}`, tool.reason)),
          ...hiddenTools.map((tool) => compactRow(`Hidden · ${tool.displayName || toolDisplayName(tool)}`, tool.reason)),
        ].filter(Boolean),
        raw: {
          visibleTools,
          hiddenTools,
        },
      },
    },
  };
});

const runtimeStatus = computed(() => {
  if (workspaceModel.value.statusBanner?.runtimeText) {
    return workspaceModel.value.statusBanner.runtimeText;
  }
  const laneLabel = compactText(workspaceModel.value.currentLaneLabel || "");
  const missingRequirements = asArray(workspaceModel.value.missingRequirements).map((item) => compactText(item)).filter(Boolean);
  if (compactText(workspaceModel.value.finalGateStatus) === "blocked" && missingRequirements.length) {
    return `${laneLabel || "分析中"} | 缺口: ${missingRequirements.join(" / ")}`;
  }
  const phase = normalizePhaseLabel(workspaceModel.value.missionPhase);
  const total = Number(planCardModel.value.totalSteps || 0);
  const completed = Number(planCardModel.value.completedSteps || 0);
  if (workspaceModel.value.missionPhase === "aborted") {
    return `已停止 | ${canRestartMission.value ? "可直接发送启动新一轮 mission" : "当前任务已结束"}`;
  }
  if (workspaceModel.value.missionPhase === "failed") {
    return `失败 | ${compactText(workspaceModel.value.currentFailureCard?.title || "查看最近一次失败原因")}`;
  }
  if (workspaceModel.value.missionPhase === "completed") {
    return "已完成 | 可继续补充下一轮需求";
  }
  if (!total) {
    if (workspaceModel.value.missionPhase === "thinking") {
      return laneLabel || "正在思考";
    }
    if (workspaceModel.value.cards?.planCard || compactText(planCardModel.value.generatedAt || planCardModel.value.summary)) {
      return `${laneLabel || phase} | 已收到计划投影，等待步骤同步`;
    }
    if (workspaceModel.value.currentLane === "readonly") {
      return `${laneLabel || phase} | 正在收集证据`;
    }
    if (workspaceModel.value.currentLane === "verify") {
      return `${laneLabel || phase} | 正在核对执行结果`;
    }
    return `${laneLabel || phase} | 等待主 Agent 生成计划`;
  }
  return `${laneLabel || phase} | 共 ${total} 个任务，已完成 ${completed} 个`;
});

const toolbarTone = computed(() => {
  if (store.errorMessage) return "danger";
  if (!actionNotice.value && statusBanner.value?.tone) return statusBanner.value.tone;
  if (actionTone.value) return actionTone.value;
  return "info";
});

const toolbarMessage = computed(() => {
  const raw = store.errorMessage || actionNotice.value || statusBanner.value?.text || store.noticeMessage || "";
  // Replace raw "approval not found" with a user-friendly message
  if (/approval.*not\s*found|not\s*found.*approval/i.test(raw)) {
    return "该审批已过期或已被处理，请刷新页面查看最新状态。";
  }
  return raw;
});

watch(
  approvalItems,
  (items) => {
    if (resolveApprovalSelection(selectedApprovalId.value, items)) return;
    selectedApprovalId.value = items[0]?.id || "";
  },
  { immediate: true, deep: true },
);

watch(
  verificationRecords,
  (items) => {
    if (resolveVerificationSelection(selectedVerificationId.value, items)) return;
    selectedVerificationId.value = "";
  },
  { immediate: true, deep: true },
);

watch(
  timelineItems,
  (items) => {
    if (resolveTimelineSelection(selectedEventId.value, items)) return;
    selectedEventId.value = "";
  },
  { immediate: true, deep: true },
);

function resolveApprovalSelection(selectionId, items) {
  const list = asArray(items);
  if (!list.length) return null;
  const selection = compactText(selectionId);
  if (!selection) return list[0] || null;
  return list.find((item) => compactText(item.id) === selection || compactText(item.approvalId) === selection) || list[0] || null;
}

function resolveVerificationSelection(selectionId, items) {
  const list = asArray(items);
  if (!list.length) return null;
  const selection = compactText(selectionId);
  if (!selection) return null;
  return list.find((item) => compactText(item.id || item.raw?.id) === selection) || null;
}

function resolveTimelineSelection(selectionId, items) {
  const list = asArray(items);
  if (!list.length) return null;
  const selection = compactText(selectionId);
  if (!selection) return null;
  return list.find((item) => compactText(item.id) === selection || compactText(item.raw?.id) === selection) || null;
}

function resolveVerificationForApproval(approval, records) {
  const item = approval || {};
  const approvalId = compactText(item.approvalId || item.id);
  const cardId = compactText(item.raw?.id || item.id);
  return asArray(records).find((record) =>
    (approvalId && compactText(record.approvalId || record.raw?.approvalId || record.raw?.metadata?.approvalId) === approvalId) ||
    (cardId && (
      compactText(record.commandCardId || record.raw?.commandCardId || record.raw?.metadata?.cardId) === cardId ||
      compactText(record.raw?.actionEventId) === cardId
    ))
  ) || null;
}

function resolveApprovalForVerification(record, approvals) {
  const item = record || {};
  const approvalId = compactText(item.approvalId || item.raw?.approvalId || item.raw?.metadata?.approvalId);
  const cardId = compactText(item.commandCardId || item.raw?.commandCardId || item.raw?.metadata?.cardId || item.raw?.actionEventId);
  return asArray(approvals).find((approval) =>
    (approvalId && compactText(approval.approvalId || approval.id) === approvalId) ||
    (cardId && compactText(approval.raw?.id || approval.id) === cardId)
  ) || null;
}

function resolveTimelineEventForApproval(approval, items) {
  const approvalId = compactText(approval?.approvalId || approval?.id);
  return asArray(items).find((item) =>
    compactText(item.targetType).toLowerCase() === "approval" &&
    (compactText(item.targetId) === approvalId || compactText(item.raw?.targetId) === approvalId)
  ) || null;
}

function resolveTimelineEventForVerification(record, items) {
  const verificationId = compactText(record?.id || record?.raw?.id);
  return asArray(items).find((item) =>
    compactText(item.targetType).toLowerCase() === "verification" &&
    (compactText(item.targetId) === verificationId || compactText(item.raw?.targetId) === verificationId)
  ) || null;
}

function scrollToTestId(testId) {
  if (typeof window === "undefined" || !testId) return;
  const node = window.document.querySelector(`[data-testid="${testId}"]`);
  if (node && typeof node.scrollIntoView === "function") {
    node.scrollIntoView({ block: "nearest", inline: "nearest" });
  }
}

watch(
  hostRows,
  (items) => {
    if (selectedHostId.value && items.some((item) => item.hostId === selectedHostId.value)) return;
    selectedHostId.value = items[0]?.hostId || "";
  },
  { immediate: true, deep: true },
);

watch(
  () => ({
    isWorkspace: isWorkspaceSession.value,
    turnActive: store.runtime.turn.active,
    phase: workspaceModel.value.missionPhase,
    stepCount: Number(planCardModel.value.totalSteps || 0),
  }),
  (state, _previous, onCleanup) => {
    if (
      !state.isWorkspace ||
      !state.turnActive ||
      !["planning", "thinking", "executing", "waiting_approval", "waiting_input"].includes(state.phase)
    ) {
      return;
    }

    const timer = window.setInterval(() => {
      if (refreshBusy.value || decisionBusy.value) return;
      void store.fetchState();
    }, state.stepCount > 0 ? 5000 : 3000);

    onCleanup(() => {
      window.clearInterval(timer);
    });
  },
  { immediate: true },
);

watch(
  () => ({
    kind: store.snapshot.kind,
    loading: store.loading,
    turnActive: store.runtime.turn.active,
  }),
  (state) => {
    if (state.kind === "workspace" || state.loading || state.turnActive || workspaceBootstrapAttempted.value) {
      return;
    }
    void bootstrapWorkspaceSession();
  },
  { immediate: false },
);

onMounted(() => {
  if (!isWorkspaceSession.value && !store.loading && !store.runtime.turn.active && !workspaceBootstrapAttempted.value) {
    void bootstrapWorkspaceSession();
  }
});

function openEvidence({ source = "mission", hostId = "", stepId = "", approvalId = "", verificationId = "", evidenceId = "", tab = "main-agent-plan" } = {}) {
  agentDetailOpen.value = false;
  if (hostId) selectedHostId.value = hostId;
  if (stepId) selectedStepId.value = stepId;
  if (approvalId) selectedApprovalId.value = approvalId;
  if (verificationId) {
    selectedVerificationId.value = verificationId;
  } else if (source !== "verification") {
    selectedVerificationId.value = "";
  }
  if (evidenceId) {
    selectedCitationEvidenceId.value = evidenceId;
  } else if (source !== "citation") {
    selectedCitationEvidenceId.value = "";
  }
  if (source !== "command") selectedCommandEvidence.value = null;
  if (source !== "process") selectedProcessEvidence.value = null;
  if (source !== "tool_invocation") selectedToolInvocationId.value = "";
  evidenceSource.value = source;
  evidenceTab.value = tab;
  evidenceOpen.value = true;
}

function openToolInvocationEvidence(invocationId = "") {
  const id = compactText(invocationId);
  if (!id) return;
  selectedToolInvocationId.value = id;
  openEvidence({
    source: "tool_invocation",
    hostId: compactText(selectedToolInvocation.value?.input?.hostId || ""),
    tab: "tool-input",
  });
}

function openCommandEvidence(value = {}, fallbackHostId = "") {
  const command = commandEvidenceFrom(value);
  selectedCommandEvidence.value = command;
  openEvidence({
    source: "command",
    hostId: compactText(command.hostId || fallbackHostId),
    tab: "host-terminal",
  });
}

async function refreshProtocolState() {
  refreshBusy.value = true;
  try {
    await Promise.all([store.fetchState(), store.fetchSessions()]);
    pushActionNotice("工作台状态已刷新。", "info");
  } finally {
    refreshBusy.value = false;
  }
}

async function createWorkspaceSession() {
  if (workspaceBootstrapBusy.value) return false;
  workspaceBootstrapBusy.value = true;
  try {
    const ok = await store.createSession("workspace");
    if (ok) {
      pushActionNotice("已创建新的协作工作台。", "info");
      return true;
    }
    pushActionNotice(store.errorMessage || "创建协作工作台失败。", "danger");
    return false;
  } finally {
    workspaceBootstrapBusy.value = false;
    workspaceBootstrapAttempted.value = true;
  }
}

async function activateRecentWorkspaceSession() {
  if (!recentWorkspaceSession.value?.id || workspaceBootstrapBusy.value) return false;
  workspaceBootstrapBusy.value = true;
  try {
    const ok = await store.activateSession(recentWorkspaceSession.value.id);
    if (ok) {
      pushActionNotice("已切换到最近的工作台。", "info");
      return true;
    }
    pushActionNotice(store.errorMessage || "切换最近工作台失败。", "danger");
    return false;
  } finally {
    workspaceBootstrapBusy.value = false;
    workspaceBootstrapAttempted.value = true;
  }
}

async function bootstrapWorkspaceSession() {
  if (isWorkspaceSession.value || workspaceBootstrapBusy.value || store.runtime.turn.active) return false;
  workspaceBootstrapBusy.value = true;
  try {
    await store.fetchSessions();
    const recent = store.sessionList.find((session) => session.kind === "workspace" && session.id !== store.activeSessionId) || null;
    if (recent?.id) {
      const ok = await store.activateSession(recent.id);
      if (ok) {
        pushActionNotice("已切换到最近的协作工作台。", "info");
        return true;
      }
    }
    const ok = await store.createSession("workspace");
    if (ok) {
      pushActionNotice("已自动创建新的协作工作台。", "info");
      return true;
    }
    pushActionNotice(store.errorMessage || "进入协作工作台失败。", "danger");
    return false;
  } finally {
    workspaceBootstrapBusy.value = false;
    workspaceBootstrapAttempted.value = true;
  }
}

async function sendWorkspaceMessage(payload = composerDraft.value) {
  if (!canSendWorkspaceMessage.value || !compactText(payload)) return;
  const restartingMission = canRestartMission.value;
  const answeringQuestion = waitingForUserAnswer.value;
  const message = compactText(payload);
  const clientMessageId = createClientId("client-msg");
  const clientTurnId = createClientId("client-turn");
  const sessionId = store.activeSessionId || store.snapshot.sessionId;
  const hostId = selectedHostRow.value?.hostId || store.snapshot.selectedHostId || "server-local";
  store.sending = true;
  store.errorMessage = "";
  actionNotice.value = "";
  if (restartingMission) {
    selectedApprovalId.value = "";
    selectedStepId.value = "";
    selectedHostId.value = "";
    evidenceOpen.value = false;
    pushActionNotice("上一轮 mission 已结束，本次发送会在当前工作台启动新的 mission。", "info");
  }
  store.startLocalPendingTurn({
    clientTurnId,
    clientMessageId,
    hostId,
    phase: "thinking",
  });
  const optimisticCardId = store.appendOptimisticUserMessage({
    message,
    clientMessageId,
    clientTurnId,
    sessionId,
    hostId,
  });
  if (!answeringQuestion) {
    store.resetActivity();
  }
  composerDraft.value = "";

  try {
    await sendMessageApi({
      message,
      sessionId,
      hostId,
      clientMessageId,
      clientTurnId,
    });
    pushActionNotice(answeringQuestion ? "已提交澄清回答。" : restartingMission ? "已在当前会话启动新一轮 mission。" : "消息已发送给主 Agent。", "info");
    await Promise.all([store.fetchState(), store.fetchSessions()]);
  } catch (_error) {
    if (optimisticCardId) {
      store.patchOptimisticCard(optimisticCardId, {
        status: "failed",
        lastError: _error?.message || "Network error",
      });
    }
    composerDraft.value = message;
    store.errorMessage = _error?.message || "Network error";
    store.setTurnPhase("failed");
    store.clearTurnPendingStart();
  } finally {
    store.sending = false;
  }
}

async function stopWorkspaceMessage() {
  if ((!store.runtime.turn.active && !store.runtime.turn.pendingStart) || decisionBusy.value || stopBusy.value) return;
  stopBusy.value = true;
  pushActionNotice("正在中断当前任务...", "info");
  try {
    await stopMessageApi();
    pushActionNotice("已停止当前工作台任务。", "info");
    store.errorMessage = "";
    store.setTurnPhase("aborted");
    await Promise.all([store.fetchState(), store.fetchSessions()]);
  } catch (_error) {
    store.errorMessage = "Network error";
    store.setTurnPhase("failed");
  } finally {
    stopBusy.value = false;
  }
}

async function handleChoice({ requestId, answers }) {
  if (!requestId || !Array.isArray(answers) || !answers.length) return;
  if (choiceBusyById.value[requestId]) return;
  choiceBusyById.value = { ...choiceBusyById.value, [requestId]: true };
  choiceErrorById.value = { ...choiceErrorById.value, [requestId]: "" };
  try {
    store.errorMessage = "";
    store.setTurnPhase("thinking");
    const refreshStatePromise = typeof store.fetchState === "function" ? store.fetchState() : Promise.resolve(true);
    const refreshSessionsPromise = typeof store.fetchSessions === "function" ? store.fetchSessions() : Promise.resolve(true);
    await submitChoiceAnswerApi(requestId, answers);
    choiceErrorById.value = { ...choiceErrorById.value, [requestId]: "" };
    pushActionNotice("已提交补充输入，主 Agent 会基于你的选择继续推进。", "info");
    await Promise.all([refreshStatePromise, refreshSessionsPromise]);
  } catch (_error) {
    const message = _error?.message || "choice submit failed";
    store.errorMessage = message;
    choiceErrorById.value = { ...choiceErrorById.value, [requestId]: message };
    store.setTurnPhase("failed");
  } finally {
    choiceBusyById.value = { ...choiceBusyById.value, [requestId]: false };
  }
}

async function postApprovalDecision(approvalId, decision) {
  await submitApprovalDecisionApi(approvalId, decision);
}

function approvalDecisionNotice(approval, decision) {
  const isPlanApproval =
    compactText(approval?.kind) === "plan" ||
    compactText(approval?.raw?.type) === "PlanApprovalCard" ||
    compactText(approval?.raw?.approval?.type) === "plan_exit";
  if (decision === "decline") {
    return isPlanApproval ? "计划已拒绝，等待主 Agent 调整方案。" : "已提交拒绝，等待主 Agent 调整方案。";
  }
  return isPlanApproval ? "计划审批已通过，主 Agent 将继续推进。" : "审批结果已提交。";
}

function isApprovalPermissionDenied(message = "") {
  return /权限不足|无权|forbidden|not authorized|unauthorized|permission denied/i.test(String(message || ""));
}

async function submitApprovalDecision(approval, decision) {
  if (approval?.mcpSynthetic) {
    settleLocalMcpApproval(approval, decision);
    return;
  }
  const approvalId = compactText(approval?.approvalId || approval?.requestId || approval?.raw?.approval?.requestId);
  if (!approvalId || decisionBusy.value) return;

  const cardId = compactText(approval?.id);
  selectedApprovalId.value = cardId;
  decisionBusy.value = true;
  try {
    store.errorMessage = "";
    await postApprovalDecision(approvalId, decision);
    pushActionNotice(approvalDecisionNotice(approval, decision), decision === "decline" ? "warning" : "info");
    await Promise.all([store.fetchState(), store.fetchSessions()]);
  } catch (error) {
    const msg = error?.message || "approval failed";
    // Friendly message for stale / expired approvals
    if (/not found|expired|过期|不存在/i.test(msg)) {
      // Locally dismiss the stale approval card so it disappears from the UI
      dismissStaleApprovalCard(cardId, approvalId);
      pushActionNotice("该审批已过期或已被处理，已自动清除。", "warning");
      store.errorMessage = "";
      selectedApprovalId.value = "";
    } else if (isApprovalPermissionDenied(msg)) {
      store.errorMessage = msg;
      store.setTurnPhase("waiting_approval");
    } else {
      store.errorMessage = msg;
      store.setTurnPhase("failed");
    }
  } finally {
    decisionBusy.value = false;
  }
}

/** Locally mark a stale approval card as dismissed so it's filtered out of the pending list */
function dismissStaleApprovalCard(cardId, approvalId) {
  const cards = store.snapshot.cards;
  if (!Array.isArray(cards)) return;
  for (const card of cards) {
    if (!card) continue;
    const isMatch =
      card.id === cardId ||
      card.approval?.requestId === approvalId ||
      card.approvalId === approvalId ||
      card.requestId === approvalId;
    if (isMatch && card.status === "pending") {
      card.status = "dismissed";
    }
  }
  // Also remove from snapshot.approvals
  if (Array.isArray(store.snapshot.approvals)) {
    store.snapshot.approvals = store.snapshot.approvals.filter(
      (a) => a.id !== approvalId && a.requestId !== approvalId,
    );
  }
}

function handlePlanAction(payload) {
  const plan = payload?.plan || {};
  if (compactText(payload?.action?.key) === "plan-evidence") {
    openEvidence({
      source: "message",
      tab: "main-agent-plan",
    });
    return;
  }
  const hostId = compactText(payload?.host?.id || asArray(plan.hostAgent || plan.hosts || [])[0]?.id);
  if (compactText(payload?.action?.key) === "host" && hostId) {
    openEvidence({
      source: "host",
      stepId: compactText(plan.id || plan.step?.id || plan.stepId),
      hostId,
      tab: "worker-conversation",
    });
    return;
  }
  openEvidence({
    source: "step",
    stepId: compactText(plan.id || plan.step?.id || plan.stepId),
    hostId,
    tab: "main-agent-plan",
  });
}

function handleAgentSelect(agent) {
  const nextAgentId = compactText(agent?.hostId || agent?.id || agent?.name);
  if (!nextAgentId) return;
  selectedAgentId.value = nextAgentId;
  evidenceOpen.value = false;
  agentDetailOpen.value = true;
}

function handleMessageSelect(message) {
  // Don't open evidence modal when clicking messages — it's confusing
  selectedMessageId.value = compactText(message?.id);
}

function handleEvidenceSelect(payload) {
  const reference = payload?.reference || {};
  const evidenceId = compactText(reference.evidenceId);
  if (!evidenceId) return;
  openEvidence({
    source: "citation",
    evidenceId,
    tab: "citation-evidence",
  });
}

async function focusApprovalContext(approvalId) {
  const targetId = compactText(approvalId);
  if (!targetId) return;
  selectedApprovalId.value = targetId;
  evidenceOpen.value = false;
  await nextTick();
  scrollToTestId(`protocol-approval-${targetId}`);
}

async function focusTimelineContext(eventId) {
  const targetId = compactText(eventId);
  if (!targetId) return;
  selectedEventId.value = targetId;
  evidenceOpen.value = false;
  await nextTick();
  scrollToTestId(`protocol-event-${targetId}`);
}

function handleEvidenceModalAction(action) {
  const kind = compactText(action?.kind).toLowerCase();
  if (kind === "open_verification") {
    openEvidence({
      source: "verification",
      verificationId: compactText(action?.verificationId),
      hostId: compactText(action?.hostId),
      tab: "verification-results",
    });
    return;
  }
  if (kind === "open_approval") {
    openEvidence({
      source: "approval",
      approvalId: compactText(action?.approvalId),
      hostId: compactText(action?.hostId),
      tab: "approval-context",
    });
    return;
  }
  if (kind === "focus_approval") {
    void focusApprovalContext(action?.approvalId);
    return;
  }
  if (kind === "focus_event") {
    void focusTimelineContext(action?.eventId);
  }
}

function handleEvidencePin(payload = {}) {
  const snapshot = captureEvidenceDrawerSnapshot(payload?.activeTab || evidenceTab.value);
  if (!snapshot.tabs.length) return;
  evidenceDrawerState.value = snapshot;
  evidenceDrawerActiveTab.value = snapshot.activeTab;
  promptDebugOpen.value = false;
  evidenceDrawerOpen.value = true;
  evidenceOpen.value = false;
  pushActionNotice(`${snapshot.title || "当前证据"} 已固定到证据抽屉。`, "info");
}

function openPromptDebugDrawer() {
  promptDebugActiveTab.value = "runtime-policy";
  evidenceDrawerOpen.value = false;
  promptDebugOpen.value = true;
}

function handleProcessItemSelect(payload) {
  const item = payload?.item || {};
  const hostId = compactText(item.hostId);
  if (item.kind === "command" || item.commandCard || item.command) {
    openCommandEvidence(item.commandCard || item, hostId);
    return;
  }
  if (item.kind === "assistant_message") {
    selectedProcessEvidence.value = item;
    openEvidence({ source: "process", hostId, tab: "main-agent-plan" });
    return;
  }
  if (hostId) {
    const approval = approvalItems.value.find((entry) => compactText(entry.hostId) === hostId) || null;
    const looksLikeApprovalBlock = /审批|批准|授权|approval/i.test(`${item.text || ""} ${item.detail || ""} ${item.status || ""}`);
    if (approval && looksLikeApprovalBlock) {
      openEvidence({
        source: "approval",
        approvalId: compactText(approval.id),
        hostId,
        tab: "approval-context",
      });
      return;
    }
    openEvidence({
      source: "host",
      hostId,
      tab: "worker-conversation",
    });
    return;
  }
  openEvidence({ source: "message", tab: "main-agent-plan" });
}

function handleEventSelect(item) {
  selectedEventId.value = compactText(item?.id || item?.raw?.id);
  const targetType = compactText(item?.targetType).toLowerCase();
  if (targetType === "tool_invocation") {
    openToolInvocationEvidence(item?.targetId);
    return;
  }
  if (targetType === "command") {
    openCommandEvidence(item?.commandCard || item, item?.hostId);
    return;
  }
  if (targetType === "mcp_approval") {
    selectedApprovalId.value = compactText(item?.targetId);
    pushActionNotice("已定位到 MCP 审批上下文。", "info");
    return;
  }
  if (targetType === "mcp_action") {
    pushActionNotice(compactText(item?.text || "已定位到 MCP 动作事件。"), "info");
    return;
  }
  if (targetType === "approval") {
    openEvidence({ source: "approval", approvalId: compactText(item?.targetId), hostId: compactText(item?.hostId), tab: "approval-context" });
    return;
  }
  if (targetType === "verification") {
    openEvidence({ source: "verification", verificationId: compactText(item?.targetId), hostId: compactText(item?.hostId), tab: "verification-results" });
    return;
  }
  if (targetType === "host") {
    // Show terminal output (commands & errors) instead of worker conversation
    openEvidence({ source: "host", hostId: compactText(item?.hostId || item?.targetId), tab: "host-terminal" });
    return;
  }
  if (targetType === "dispatch") {
    openEvidence({ source: "dispatch", hostId: compactText(item?.hostId), tab: "main-agent-plan" });
    return;
  }
  // Default: show terminal for any event with a hostId, otherwise show plan
  if (item?.hostId) {
    openEvidence({ source: "host", hostId: compactText(item.hostId), tab: "host-terminal" });
    return;
  }
  openEvidence({ source: "event", tab: "main-agent-plan" });
}

function handleApprovalDetail(approval) {
  if (approval?.mcpSynthetic) {
    selectedApprovalId.value = compactText(approval?.id);
    pushActionNotice(`${approval.title || "MCP 操作"} 已定位到右侧审批栏。`, "info");
    return;
  }
  selectedApprovalId.value = compactText(approval?.id);
  openEvidence({
    source: "approval",
    approvalId: compactText(approval?.id),
    hostId: compactText(approval?.hostId),
    tab: "approval-context",
  });
}

function handleApprovalAuthorize(approval) {
  selectedApprovalId.value = compactText(approval?.id);
  void submitApprovalDecision(approval, "accept_session");
}

function handleApprovalReject(approval) {
  selectedApprovalId.value = compactText(approval?.id);
  void submitApprovalDecision(approval, "decline");
}

function handleApprovalAccept(approval) {
  selectedApprovalId.value = compactText(approval?.id);
  void submitApprovalDecision(approval, "accept");
}

function handleMcpSurfaceEventDetail(surface) {
  handleMcpSurfaceDetail(surface);
}

function handleMcpSurfaceEventPin(surface) {
  handleMcpSurfacePin(surface);
}

function handleMcpSurfaceEventRefresh(surface) {
  void handleMcpSurfaceRefresh(surface);
}

</script>

<template>
  <div class="protocol-workspace-page" data-testid="protocol-workspace-page">
    <div v-if="!isWorkspaceSession" class="protocol-workspace-empty">
      <PanelsTopLeftIcon size="30" class="empty-icon" />
      <h2>当前不是协作工作台会话</h2>
      <p>新页面只服务主 Agent 编排工作台。你可以直接新建一个 workspace，或者回到最近的工作台继续处理审批和 plan。</p>
      <p v-if="workspaceBootstrapBusy" class="empty-hint">正在进入协作工作台...</p>
      <p v-else-if="store.errorMessage || actionNotice" class="empty-hint" :class="toolbarTone">
        {{ store.errorMessage || actionNotice }}
      </p>
      <div class="empty-actions">
        <button class="ops-button primary" type="button" :disabled="workspaceBootstrapBusy" @click="createWorkspaceSession">
          {{ workspaceBootstrapBusy ? "处理中..." : "新建工作台" }}
        </button>
        <button
          v-if="recentWorkspaceSession"
          class="ops-button ghost"
          type="button"
          :disabled="workspaceBootstrapBusy"
          @click="activateRecentWorkspaceSession"
        >
          切到最近工作台
        </button>
      </div>
    </div>

    <template v-else>
      <div class="protocol-workspace-toolbar">
        <div v-if="toolbarMessage" class="toolbar-notice" :class="toolbarTone">
          <AlertTriangleIcon v-if="store.errorMessage || toolbarTone === 'danger'" size="14" />
          <span>{{ toolbarMessage }}</span>
        </div>
        <div class="toolbar-actions">
          <button
            v-if="promptDebugEnabled"
            class="toolbar-debug"
            type="button"
            data-testid="protocol-prompt-debug-button"
            @click="openPromptDebugDrawer"
          >
            查看 Prompt Debug
          </button>
          <button class="toolbar-refresh" type="button" :disabled="refreshBusy" @click="refreshProtocolState">
            <RefreshCwIcon size="14" :class="{ spin: refreshBusy }" />
            <span>{{ refreshBusy ? "刷新中..." : "刷新" }}</span>
          </button>
        </div>
      </div>

      <div class="protocol-workspace-shell">
        <section class="workspace-stage-card">
          <div v-if="store.loading" class="stage-empty">
            <Loader2Icon size="18" class="spin" />
            <span>正在载入工作台...</span>
          </div>

          <article v-if="!store.loading && statusBanner" class="workspace-status-banner" :class="statusBanner.tone">
            <div class="workspace-status-banner-head">
              <strong>{{ statusBanner.title }}</strong>
              <button class="workspace-status-banner-close" type="button" title="关闭提示" aria-label="关闭提示" @click="dismissStatusBanner">
                <XIcon size="14" />
              </button>
            </div>
            <p>{{ statusBanner.text }}</p>
            <span v-if="statusBanner.hint" class="workspace-status-banner-hint">{{ statusBanner.hint }}</span>
          </article>

          <article v-if="!store.loading" class="workspace-runtime-policy-card" :class="{ blocked: runtimePolicyCard.blocked }" data-testid="protocol-runtime-policy">
            <div class="workspace-runtime-policy-head">
              <div class="workspace-runtime-policy-chips">
                <span class="runtime-policy-chip">{{ runtimePolicyCard.modeLabel }}</span>
                <span class="runtime-policy-chip">{{ runtimePolicyCard.stageLabel }}</span>
                <span class="runtime-policy-chip strong">{{ runtimePolicyCard.laneLabel }}</span>
                <span class="runtime-policy-chip" :class="{ blocked: runtimePolicyCard.blocked }">Final Gate · {{ runtimePolicyCard.gateLabel }}</span>
              </div>
              <span class="runtime-policy-intent">{{ runtimePolicyCard.intentLabel }}</span>
            </div>
            <p class="workspace-runtime-policy-detail">{{ runtimePolicyCard.detail }}</p>
            <div class="workspace-runtime-policy-meta">
              <span v-if="runtimePolicyCard.nextTool">
                下一步工具：<strong>{{ runtimePolicyCard.nextToolLabel || runtimePolicyCard.nextTool }}</strong>
              </span>
              <span v-else>当前没有额外的强制下一步工具。</span>
            </div>
            <div v-if="runtimePolicyCard.missingRequirements.length" class="workspace-runtime-policy-missing">
              <span
                v-for="item in runtimePolicyCard.missingRequirements"
                :key="item"
                class="runtime-policy-missing-chip"
              >
                {{ item }}
              </span>
            </div>
          </article>

          <ProtocolConversationPane
            v-if="!store.loading"
            title="Main Agent"
            :subtitle="conversationSubtitle"
            :formatted-turns="formattedTurns"
            :history-reset-key="store.activeSessionId || store.snapshot.sessionId || ''"
            :status-card="conversationStatusCard"
            :plan-cards="planCards"
            :plan-summary-label="planSummaryLabel"
            :plan-overview-rows="planOverviewRows"
            :background-agents="backgroundAgents"
            :choice-cards="choiceCards"
            :choice-submitting="choiceBusyById"
            :choice-errors="choiceErrorById"
            :starter-card="starterCard"
            :draft="composerDraft"
            :draft-placeholder="composerPlaceholder"
            :sending="store.sending"
            :busy="stopBusy"
            :primary-action-override="composerPrimaryActionOverride"
            :virtualization-suspended="store.runtime.turn.active || store.runtime.turn.pendingStart || store.sending"
            empty-label="工作台已连接，可以直接开始对话。"
            @update:draft="composerDraft = $event"
            @send="sendWorkspaceMessage"
            @stop="stopWorkspaceMessage"
            @choice="handleChoice"
            @select-message="handleMessageSelect"
            @process-item-select="handleProcessItemSelect"
            @evidence-select="handleEvidenceSelect"
            @plan-action="handlePlanAction"
            @agent-select="handleAgentSelect"
            @action="handleMcpAction"
            @detail="handleMcpSurfaceEventDetail"
            @pin="handleMcpSurfaceEventPin"
            @refresh="handleMcpSurfaceEventRefresh"
            @open-history="openConversationHistory"
          />
        </section>

        <aside class="workspace-side-rail">
          <section class="workspace-side-panel approval-panel" data-testid="protocol-side-panel-approval">
            <ProtocolApprovalRail
              title="待审批决策"
              subtitle="右侧固定审批区，直接快速完成授权、拒绝或同意执行。"
              :queue-items="approvalItems"
              :active-approval-id="activeApprovalCardId"
              :busy="decisionBusy"
              empty-label="当前没有待处理的审批。"
              @detail="handleApprovalDetail"
              @authorize="handleApprovalAuthorize"
              @reject="handleApprovalReject"
              @accept="handleApprovalAccept"
            />
          </section>

          <section class="workspace-side-panel timeline-panel" data-testid="protocol-side-panel-timeline">
            <ProtocolEventTimeline
              title="实时事件"
              subtitle="轻量时间线只保留关键变化，帮助你快速判断当前执行推进到哪里。"
              :items="timelineItems"
              :active-item-id="selectedEventId"
              empty-label="当前还没有可展示的实时事件。"
              :max-items="8"
              @select="handleEventSelect"
            />
          </section>

          <div class="runtime-pill" data-testid="protocol-runtime-pill">
            <span class="runtime-dot"></span>
            <span>{{ runtimeStatus }}</span>
          </div>
        </aside>
      </div>
    </template>

    <ProtocolAgentDetailModal
      v-model:open="agentDetailOpen"
      :agent="selectedAgentDetail"
    />

    <ProtocolEvidenceModal
      v-model:open="evidenceOpen"
      v-model:active-tab="evidenceTab"
      :title="evidenceTitle"
      :subtitle="evidenceSubtitle"
      :tabs="evidenceTabs"
      :panels="evidencePanels"
      @action="handleEvidenceModalAction"
      @pin="handleEvidencePin"
    >
      <template #mcp-surface="{ panel }">
        <section class="mcp-evidence-panel">
          <div class="mcp-evidence-hero">
            <div>
              <h4>{{ panel.title || "MCP 面板" }}</h4>
              <p>{{ panel.summary || "当前没有额外说明。" }}</p>
            </div>
            <button
              type="button"
              class="mcp-evidence-pin"
              @click="dispatchMcpDrawer(panel.raw || selectedMcpSurface, true)"
            >
              固定到 MCP 抽屉
            </button>
          </div>

          <div v-if="panel.items?.length" class="mcp-evidence-grid">
            <article v-for="(item, index) in panel.items" :key="`${item.label || 'mcp'}-${index}`" class="mcp-evidence-card">
              <span>{{ item.label }}</span>
              <strong>{{ item.value }}</strong>
            </article>
          </div>

          <div v-if="panel.sections?.length" class="mcp-evidence-sections">
            <article v-for="section in panel.sections" :key="section.id || section.kind" class="mcp-evidence-section">
              <header>
                <strong>{{ section.title || section.kind }}</strong>
                <span>{{ section.summary || `${section.cards?.length || 0} 个卡片` }}</span>
              </header>
              <div v-if="section.cards?.length" class="mcp-evidence-cards">
                <div v-for="card in section.cards" :key="card.id" class="mcp-evidence-card-row">
                  <strong>{{ card.title || card.uiKind || card.id }}</strong>
                  <span>{{ card.summary || card.detail || card.text || "" }}</span>
                </div>
              </div>
            </article>
          </div>

          <pre class="mcp-evidence-raw">{{ stringifyRaw(panel.raw) || "暂无 MCP 面板详情" }}</pre>
        </section>
      </template>
      <template #host-terminal="{ panel }">
        <section class="terminal-evidence-panel">
          <div class="terminal-summary">
            <h4>{{ panel.title || "Host terminal" }}</h4>
            <p>{{ panel.summary || "当前没有额外说明。" }}</p>
          </div>

          <div v-if="panel.items?.length" class="terminal-meta-grid">
            <article v-for="(item, index) in panel.items" :key="`${item.label || 'meta'}-${index}`" class="terminal-meta-card">
              <span>{{ item.label }}</span>
              <strong>{{ item.value }}</strong>
            </article>
          </div>

          <pre class="terminal-output">{{ stringifyRaw(panel.raw) || "暂无终端输出" }}</pre>
        </section>
      </template>
    </ProtocolEvidenceModal>

    <ProtocolEvidenceDrawer
      v-model:open="evidenceDrawerOpen"
      v-model:active-tab="evidenceDrawerActiveTab"
      :title="evidenceDrawerState.title"
      :subtitle="evidenceDrawerState.subtitle"
      :tabs="evidenceDrawerState.tabs"
      :panels="evidenceDrawerState.panels"
      kicker="EVIDENCE DRAWER"
      offset-right="356px"
      @action="handleEvidenceModalAction"
    />

    <ProtocolEvidenceDrawer
      v-if="promptDebugEnabled"
      v-model:open="promptDebugOpen"
      v-model:active-tab="promptDebugActiveTab"
      title="Prompt Debug"
      subtitle="查看本轮上下文、可见工具、gate 命中项，以及为什么这轮被放行或拦截。"
      :tabs="promptDebugState.tabs"
      :panels="promptDebugState.panels"
      kicker="PROMPT DEBUG"
      test-id="protocol-prompt-debug-drawer"
      offset-right="356px"
    />
  </div>
</template>

<style scoped>
.protocol-workspace-page {
  display: flex;
  flex-direction: column;
  gap: 0;
  min-height: 0;
  height: calc(100vh - 48px);
  overflow: hidden;
}

.protocol-workspace-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  padding: 6px 16px;
  flex-shrink: 0;
  border-bottom: 1px solid #e8ecf1;
  background: #ffffff;
}

.toolbar-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.toolbar-notice {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  height: 32px;
  border-radius: 6px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  color: #334155;
  font-size: 12px;
  font-weight: 500;
}

.toolbar-notice.danger {
  border-color: #fca5a5;
  color: #991b1b;
  background: #fef2f2;
}

.toolbar-debug,
.toolbar-refresh {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px;
  height: 32px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #ffffff;
  color: #475569;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: border-color 0.18s ease, background 0.18s ease, color 0.18s ease;
}

.toolbar-debug {
  border: 1px solid #bfdbfe;
  background: #eff6ff;
  color: #1d4ed8;
}

.toolbar-debug:hover {
  border-color: #93c5fd;
  background: #dbeafe;
}

.toolbar-refresh:hover {
  border-color: #cbd5e1;
  background: #f8fafc;
}

.protocol-workspace-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: 0;
  flex: 1;
  overflow: hidden;
}

.workspace-stage-card {
  min-height: 0;
  padding: 0;
  border-radius: 0;
  border: none;
  border-right: 1px solid #e8ecf1;
  background: #ffffff;
  box-shadow: none;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.workspace-status-banner {
  width: min(980px, calc(100% - 40px));
  margin: 16px auto 0;
  padding: 14px 16px;
  border-radius: 14px;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #334155;
  flex-shrink: 0;
}

.workspace-status-banner.danger {
  border-color: #fecaca;
  background: #fef2f2;
  color: #991b1b;
}

.workspace-status-banner.warning {
  border-color: #fde68a;
  background: #fffbeb;
  color: #92400e;
}

.workspace-runtime-policy-card {
  width: min(980px, calc(100% - 40px));
  margin: 14px auto 0;
  padding: 14px 16px;
  border-radius: 18px;
  border: 1px solid rgba(191, 219, 254, 0.95);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 34%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(248, 250, 252, 0.98));
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.05);
  flex-shrink: 0;
}

.workspace-runtime-policy-card.blocked {
  border-color: rgba(253, 186, 116, 0.95);
  background:
    radial-gradient(circle at top right, rgba(249, 115, 22, 0.12), transparent 34%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(255, 247, 237, 0.98));
}

.workspace-runtime-policy-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.workspace-runtime-policy-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.runtime-policy-chip {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  background: rgba(239, 246, 255, 0.95);
  color: #1e3a8a;
  font-size: 12px;
  font-weight: 700;
}

.runtime-policy-chip.strong {
  background: rgba(219, 234, 254, 0.98);
}

.runtime-policy-chip.blocked {
  background: rgba(255, 237, 213, 0.98);
  color: #9a3412;
}

.runtime-policy-intent {
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.workspace-runtime-policy-detail {
  margin: 12px 0 0;
  color: #334155;
  font-size: 13px;
  line-height: 1.7;
}

.workspace-runtime-policy-meta {
  margin-top: 10px;
  color: #475569;
  font-size: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.workspace-runtime-policy-missing {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.runtime-policy-missing-chip {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  background: rgba(255, 237, 213, 0.98);
  color: #9a3412;
  font-size: 12px;
  font-weight: 700;
}

.workspace-status-banner-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.workspace-status-banner-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: rgba(255, 255, 255, 0.62);
  color: currentColor;
  cursor: pointer;
  flex-shrink: 0;
  opacity: 0.78;
  transition: background 0.15s ease, border-color 0.15s ease, opacity 0.15s ease;
}

.workspace-status-banner-close:hover {
  background: #ffffff;
  border-color: currentColor;
  opacity: 1;
}

.workspace-status-banner p {
  margin: 0;
  font-size: 14px;
  line-height: 1.55;
}

.workspace-status-banner-hint {
  display: block;
  margin-top: 8px;
  font-size: 12px;
  font-weight: 600;
  opacity: 0.82;
}

.workspace-side-rail {
  display: grid;
  grid-template-rows: minmax(320px, 340px) minmax(0, 1fr) auto;
  gap: 0;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  border-left: 1px solid #e8ecf1;
  background: #f8fafc;
}

.workspace-side-panel {
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.approval-panel {
  background: #f8fafc;
  border-bottom: 1px solid #e8ecf1;
}

.timeline-panel {
  background: #fff;
}

.runtime-pill {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-top: 1px solid #e8ecf1;
  background: #f8fafc;
  color: #475569;
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
  border-radius: 0;
  box-shadow: none;
}

.runtime-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
}

.protocol-workspace-empty,
.stage-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px 24px;
  min-height: 360px;
  border: 1px dashed #e2e8f0;
  background: #ffffff;
  text-align: center;
  border-radius: 0;
}

.protocol-workspace-empty h2 {
  margin: 0;
  color: #0f172a;
}

.protocol-workspace-empty p,
.stage-empty span {
  margin: 0;
  max-width: 680px;
  color: #64748b;
  line-height: 1.7;
}

.empty-hint {
  font-size: 13px;
  font-weight: 600;
}

.empty-hint.danger {
  color: #b91c1c;
}

.empty-hint.warning {
  color: #b45309;
}

.empty-icon {
  color: #2563eb;
}

.empty-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
}

.terminal-evidence-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.mcp-evidence-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.mcp-evidence-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.95), rgba(255, 255, 255, 0.98));
  border: 1px solid rgba(191, 219, 254, 0.95);
}

.mcp-evidence-hero h4 {
  margin: 0 0 6px;
  color: #0f172a;
  font-size: 16px;
}

.mcp-evidence-hero p {
  margin: 0;
  color: #475569;
  line-height: 1.6;
}

.mcp-evidence-pin {
  flex-shrink: 0;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 999px;
  padding: 8px 12px;
  background: #ffffff;
  color: #0f172a;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.mcp-evidence-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.mcp-evidence-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(248, 250, 252, 0.94);
}

.mcp-evidence-card span,
.mcp-evidence-section span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.mcp-evidence-card strong,
.mcp-evidence-card-row strong {
  color: #0f172a;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
}

.mcp-evidence-sections {
  display: grid;
  gap: 12px;
}

.mcp-evidence-section {
  display: grid;
  gap: 10px;
  padding: 14px 16px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(255, 255, 255, 0.98);
}

.mcp-evidence-section header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.mcp-evidence-section strong {
  color: #0f172a;
  font-size: 14px;
}

.mcp-evidence-cards {
  display: grid;
  gap: 8px;
}

.mcp-evidence-card-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.94);
  border: 1px solid rgba(226, 232, 240, 0.85);
}

.mcp-evidence-raw {
  margin: 0;
  padding: 14px 16px;
  border-radius: 18px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 13px;
  line-height: 1.6;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.terminal-summary h4 {
  margin: 0 0 6px;
  color: #0f172a;
  font-size: 16px;
}

.terminal-summary p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
}

.terminal-meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.terminal-meta-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(248, 250, 252, 0.92);
}

.terminal-meta-card span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.terminal-meta-card strong {
  color: #0f172a;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
}

.terminal-output {
  margin: 0;
  min-height: 300px;
  padding: 16px;
  border-radius: 20px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 13px;
  line-height: 1.6;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.spin {
  animation: protocol-spin 1s linear infinite;
}

@keyframes protocol-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1200px) {
  .protocol-workspace-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .workspace-stage-card {
    min-height: 0;
  }

  .workspace-side-rail {
    grid-template-rows: auto auto auto;
    overflow: visible;
  }

  .approval-panel {
    min-height: 320px;
  }

  .timeline-panel {
    min-height: 360px;
  }
}

@media (max-width: 720px) {
  .protocol-workspace-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .toolbar-refresh {
    justify-content: center;
  }

  .workspace-stage-card {
    padding: 18px 16px;
    border-radius: 24px;
  }

  .workspace-side-panel {
    min-height: 280px;
  }

  .runtime-pill {
    border-radius: 18px;
  }
}
</style>
