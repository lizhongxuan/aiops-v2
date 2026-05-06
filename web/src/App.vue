<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAppStore } from "./store";
import { resolveHostDisplay } from "./lib/hostDisplay";
import { createDiscreteApi } from "naive-ui";
import LoginModal from "./components/LoginModal.vue";
import HostModal from "./components/HostModal.vue";
import SessionHistoryDrawer from "./components/SessionHistoryDrawer.vue";
import McpBundleHost from "./components/mcp/McpBundleHost.vue";
import McpUiCardHost from "./components/mcp/McpUiCardHost.vue";
import { selectApprovalDock, selectRuntimeBusy, selectRuntimeStatus } from "./events/agentEventProjection";

import {
  MessageSquarePlusIcon,
  AppWindowIcon,
  SettingsIcon,
  UserCircleIcon,
  ServerIcon,
  PanelsTopLeftIcon,
  TerminalIcon,
  HistoryIcon,
  EraserIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  ActivityIcon,
  FileSearchIcon,
  WorkflowIcon,
} from "lucide-vue-next";

const store = useAppStore();
const router = useRouter();
const route = useRoute();
const { message, dialog } = createDiscreteApi(["message", "dialog"]);

const isLoginModalOpen = ref(false);
const isHostModalOpen = ref(false);
const isMcpDrawerOpen = ref(false);
const isHistoryDrawerOpen = ref(false);
const isSidebarCollapsed = ref(false);
const historyDrawerMode = ref("single_host");
const settingsMenuRef = ref(null);
const isSettingsMenuOpen = ref(false);
const isRouteHostSyncing = ref(false);
const OPEN_SESSION_HISTORY_EVENT = "codex:open-session-history";
const OPEN_MCP_DRAWER_EVENT = "codex:open-mcp-drawer";

async function initializeAppSession() {
  await store.fetchState();
  await store.fetchSessions();
  store.connectWs();
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

const activeProjectionSessionId = computed(() => store.activeSessionId || store.snapshot.sessionId || "");
const appRuntimeStatus = computed(() => selectRuntimeStatus(store.agentEventState, activeProjectionSessionId.value));
const appRuntimeBusy = computed(() => selectRuntimeBusy(store.agentEventState, activeProjectionSessionId.value) || store.sending);
const appApprovalDock = computed(() => selectApprovalDock(store.agentEventState, activeProjectionSessionId.value));
const appRuntimePhase = computed(() => {
  const status = compactText(appRuntimeStatus.value).toLowerCase();
  if (status === "working") return "executing";
  if (status === "blocked") return appApprovalDock.value.length ? "waiting_approval" : "waiting_input";
  if (status === "failed") return "failed";
  if (status === "canceled") return "aborted";
  if (status === "reviewing") return "finalizing";
  return "idle";
});

const mcpDrawerActiveSurface = computed(() => store.mcpDrawer?.activeSurface || null);
const mcpDrawerPinnedSurfaces = computed(() => store.mcpDrawer?.pinnedSurfaces || []);
const mcpDrawerRecentSurfaces = computed(() => store.mcpDrawer?.recentSurfaces || []);
const enabledMcpEntries = computed(() => {
  if (typeof store.getEnabledMcpEntries === "function") {
    return store.getEnabledMcpEntries();
  }
  return store.enabledMcpEntries || [];
});

function handleOpenMcpDrawerEvent(event) {
  const payload = asObject(event?.detail);
  const surface = store.openMcpDrawerSurface(payload, {
    pin: payload.pin === true,
    reason: payload.pin ? "pin" : "event",
  });
  if (!surface?.model || !surface.id) return;
  const source = compactText(payload.source).toLowerCase();
  const shouldOpen =
    isMcpDrawerOpen.value ||
    payload.pin === true ||
    payload.open === true ||
    source.startsWith("protocol");
  if (shouldOpen) {
    isMcpDrawerOpen.value = true;
  }
}

function pinActiveMcpSurface() {
  if (!mcpDrawerActiveSurface.value) return;
  const surface = store.pinMcpDrawerSurface(mcpDrawerActiveSurface.value);
  if (!surface) return;
  message.success(`${surface.title} 已固定到 MCP 抽屉。`);
}

function pinMcpSurface(surface) {
  const pinnedSurface = store.pinMcpDrawerSurface(surface);
  if (!pinnedSurface) return;
  message.success(`${pinnedSurface.title} 已固定到 MCP 抽屉。`);
}

function handleDrawerSurfaceAction() {
  store.touchActiveMcpDrawerSurface?.("action");
  message.success("该 MCP 操作已定位到当前对话上下文，请在对应 turn 中继续执行。");
}

function handleDrawerSurfaceDetail(payload) {
  const surface = store.openMcpDrawerSurface(payload, { reason: "detail" });
  if (!surface?.model || !surface.id) return;
  isMcpDrawerOpen.value = true;
}

function handleDrawerSurfaceRefresh() {
  store.touchActiveMcpDrawerSurface?.("refresh");
  message.success("已定位到当前 MCP 面板，可在对话页触发刷新并等待结果回写。");
}

function removePinnedMcpSurface(surfaceId = "") {
  store.removePinnedMcpDrawerSurface(surfaceId);
}

function selectMcpDrawerSurface(surface) {
  const selected = store.selectMcpDrawerSurface(surface);
  if (!selected) return;
  isMcpDrawerOpen.value = true;
}

function openEnabledMcpEntry(entry, pin = false) {
  const surface = store.openEnabledMcpEntry?.(entry, {
    pin,
    reason: pin ? "catalog-pin" : "catalog-open",
  });
  if (!surface) return;
  isMcpDrawerOpen.value = true;
  message.success(pin ? `${surface.title} 已加入常驻 MCP 面板。` : `${surface.title} 已打开统一入口。`);
}

function normalizeRouteValue(value) {
  if (Array.isArray(value)) {
    return (value[0] || "").trim();
  }
  return typeof value === "string" ? value.trim() : "";
}

function clearRouteHostQuery() {
  const nextQuery = { ...route.query };
  delete nextQuery.hostId;
  delete nextQuery.hostName;
  delete nextQuery.hostAddress;
  router.replace({ path: route.path, query: nextQuery });
}

function resolveRequestedHostTarget() {
  const requestedId = normalizeRouteValue(route.query.hostId);
  const requestedName = normalizeRouteValue(route.query.hostName);
  const requestedAddress = normalizeRouteValue(route.query.hostAddress);
  const candidates = [requestedId, requestedName, requestedAddress].filter(Boolean);

  if (!candidates.length) {
    return { hostId: "", hostMeta: null };
  }

  const host = store.snapshot.hosts.find((item) =>
    candidates.some((candidate) => candidate === item.id || candidate === item.name || candidate === item.address),
  );

  return {
    hostId: host?.id || requestedId,
    hostMeta: host || (requestedId ? { id: requestedId, name: requestedName, address: requestedAddress } : null),
  };
}

function resolveCurrentChatHostTarget() {
  const selectedHostId = compactText(store.snapshot.selectedHostId || "server-local");
  const host = store.snapshot.hosts.find((item) =>
    selectedHostId === item.id || selectedHostId === item.name || selectedHostId === item.address,
  );
  const hostId = compactText(host?.id || selectedHostId || "server-local");
  return {
    hostId,
    hostMeta: host || { id: hostId, name: hostId },
  };
}

async function syncChatRouteHostSelection() {
  if (route.name !== "chat" || isRouteHostSyncing.value) return;
  const requestedHostTarget = resolveRequestedHostTarget();
  const hasRequestedHost = Boolean(requestedHostTarget.hostId);
  if (!hasRequestedHost && store.snapshot.kind === "single_host") return;
  if (store.loading || appRuntimeBusy.value) return;

  isRouteHostSyncing.value = true;
  try {
    const target = hasRequestedHost ? requestedHostTarget : resolveCurrentChatHostTarget();
    const alreadySingleHost = store.snapshot.kind === "single_host" && target.hostId === store.snapshot.selectedHostId;
    if (alreadySingleHost) {
      if (hasRequestedHost) clearRouteHostQuery();
      return;
    }

    const ok = await store.createOrActivateSingleHostSessionForHost(target.hostId, target.hostMeta || {});
    if (ok && hasRequestedHost) {
      clearRouteHostQuery();
    }
  } finally {
    isRouteHostSyncing.value = false;
  }
}

function toggleMcpDrawer() {
  if (!isMcpDrawerOpen.value) {
    if (!mcpDrawerActiveSurface.value) {
      const fallbackSurface =
        mcpDrawerPinnedSurfaces.value[0] ||
        mcpDrawerRecentSurfaces.value[0] ||
        null;
      if (fallbackSurface) {
        store.selectMcpDrawerSurface(fallbackSurface);
      } else if (enabledMcpEntries.value[0]) {
        store.openEnabledMcpEntry?.(enabledMcpEntries.value[0], { reason: "catalog-default" });
      }
    }
  }
  isMcpDrawerOpen.value = !isMcpDrawerOpen.value;
}

function toggleSidebar() {
  isSidebarCollapsed.value = !isSidebarCollapsed.value;
}

function closeSettingsMenu() {
  isSettingsMenuOpen.value = false;
}

function openGeneralSettings() {
  closeSettingsMenu();
  router.push("/settings");
}

function openAgentProfile() {
  closeSettingsMenu();
  router.push("/settings/agent");
}

function openSkillCatalog() {
  closeSettingsMenu();
  router.push("/settings/skills");
}

function openMcpCatalog() {
  closeSettingsMenu();
  router.push("/mcp");
}

const authBadgeLabel = computed(() => {
  if (store.snapshot.auth.connected) {
    return store.snapshot.config?.model || "AI 已接入";
  }
  if (store.snapshot.auth.pending) {
    return "连接中";
  }
  return "未连接";
});

async function createSession(kind = "single_host") {
  if (store.sending) return;
  const ok = await store.createSession(kind);
  if (!ok) return;
  store.errorMessage = "";
  isHistoryDrawerOpen.value = false;
  if (kind === "workspace") {
    router.push("/protocol");
    return;
  }
  router.push("/");
}

async function startNewThread() {
  await createSession("single_host");
}

async function startWorkspaceSession() {
  await createSession("workspace");
}

async function openWorkspaceEntry() {
  if (store.snapshot.kind === "single_host" && store.workspaceReturnSession?.id) {
    const ok = await store.returnToWorkspaceSession();
    if (!ok) return;
    isHistoryDrawerOpen.value = false;
    router.push("/protocol");
    return;
  }
  router.push("/protocol");
}

function resolveHistorySessionKind(source = "") {
  const normalizedSource = compactText(source).toLowerCase();
  if (normalizedSource.includes("workspace") || normalizedSource.includes("protocol")) {
    return "workspace";
  }
  return route.name === "protocol" ? "workspace" : "single_host";
}

async function openHistoryDrawer(mode = resolveHistorySessionKind()) {
  historyDrawerMode.value = mode === "workspace" ? "workspace" : "single_host";
  await store.fetchSessions();
  isHistoryDrawerOpen.value = true;
}

function openHeaderHistoryDrawer() {
  void openHistoryDrawer(resolveHistorySessionKind());
}

function handleOpenSessionHistoryEvent(event) {
  void openHistoryDrawer(resolveHistorySessionKind(event?.detail?.source));
}

async function switchSession(sessionId) {
  const target = store.sessionList.find((item) => item.id === sessionId);
  const ok = await store.activateSession(sessionId);
  if (ok) {
    isHistoryDrawerOpen.value = false;
    if (target?.kind === "workspace") {
      router.push("/protocol");
      return;
    }
    router.push("/");
  }
}

function handleGlobalKeydown(e) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") {
    e.preventDefault();
    startNewThread();
  }
}

function handleDocumentClick(e) {
  if (!settingsMenuRef.value) return;
  if (!settingsMenuRef.value.contains(e.target)) {
    closeSettingsMenu();
  }
}

function openTerminal() {
  if (!store.canOpenTerminal) return;
  router.push(`/terminal/${store.snapshot.selectedHostId}`);
}

const currentSession = computed(() => {
  return store.activeSessionSummary || {
    title: "新建会话",
    status: "empty",
  };
});

const canResetCurrentSession = computed(() => {
  if (store.loading || appRuntimeBusy.value) return false;
  return store.snapshot.cards.length > 0 || (store.snapshot.approvals || []).length > 0;
});

const resetButtonTitle = computed(() => {
  if (store.loading) return "正在加载当前会话";
  if (appRuntimeBusy.value) return "当前任务执行中，暂不能清空上下文";
  if (!canResetCurrentSession.value) return "当前会话已经是空白";
  return "清空当前会话的消息、审批和运行态";
});

async function resetCurrentSession() {
  if (!canResetCurrentSession.value) return;
  dialog.warning({
    title: "确认清空",
    content: "确认清空当前会话上下文吗？这会移除当前会话的消息、审批和运行态，其他历史会话不会受影响。",
    positiveText: "确认清空",
    negativeText: "取消",
    onPositiveClick: async () => {
      const ok = await store.resetThread();
      if (!ok) return;
      message.success("已清空当前会话上下文");
    },
  });
}

const currentSessionStatus = computed(() => {
  if (appRuntimeBusy.value) return "执行中";
  if (currentSession.value.status === "failed") return "失败";
  if (currentSession.value.status === "completed") return "已保存";
  return "空白";
});

const selectedHostLabel = computed(() => {
  const host = store.selectedHost;
  return resolveHostDisplay(host) || "server-local";
});

function describeTurnPhase(phase) {
  switch (phase) {
    case "thinking":
      return "思考中";
    case "planning":
      return "规划中";
    case "waiting_approval":
      return "等待审批";
    case "waiting_input":
      return "等待补充输入";
    case "executing":
      return "执行中";
    case "finalizing":
      return "收尾中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "aborted":
      return "已停止";
    default:
      return "待命";
  }
}

const workspaceSession = computed(() => {
  if (store.snapshot.kind === "workspace") {
    return store.activeSessionSummary?.kind === "workspace" ? store.activeSessionSummary : null;
  }
  return null;
});

const workspaceNavTitle = computed(() => workspaceSession.value?.title || "协作工作台");
const workspaceNavStatus = computed(() => {
  if (store.snapshot.kind === "workspace") {
    const pendingApprovals = (store.snapshot.approvals || []).filter((approval) => approval.status === "pending").length;
    if (appRuntimeBusy.value) return describeTurnPhase(appRuntimePhase.value);
    if (pendingApprovals > 0) return `${pendingApprovals} 项待审批`;
    return workspaceSession.value?.status === "completed"
      ? "已完成"
      : workspaceSession.value?.status === "failed"
        ? "失败"
        : "可用";
  }
  return "规划 / 调度 / 审批";
});

const settingsNavTitle = computed(() => {
  if (route.path.startsWith("/settings/agent")) return "Agent Profile";
  if (route.path.startsWith("/settings/skills")) return "Skills 管理";
  if (route.path.startsWith("/settings/mcp")) return "MCP 管理";
  if (route.path.startsWith("/settings/experience-packs")) return "Experience Packs";
  if (route.path.startsWith("/settings/hosts")) return "Hosts";
  return "Hosts / Skills / MCP / Agent";
});

const settingsNavStatus = computed(() => {
  if (route.path === "/settings") return "设置中心";
  if (route.path.startsWith("/settings/agent")) return "System Prompt / Skills / MCP";
  if (route.path.startsWith("/settings/skills")) return "Catalog / Defaults / Activation";
  if (route.path.startsWith("/settings/mcp")) return "Catalog / Defaults / Permission";
  if (route.path.startsWith("/settings/experience-packs")) return "Playbooks";
  if (route.path.startsWith("/settings/hosts")) return "Inventory & Scope";
  return "入口";
});

const mainHeaderTitle = computed(() => {
  if (route.name === "incidents") return "事故工作台";
  if (route.name === "incident-detail") return "事故详情";
  if (route.name === "erp-health") return "ERP 健康";
  if (route.name === "opsgraph") return "ERP 图谱";
  if (route.name === "runbooks") return "Runbook";
  if (route.name === "runbook-detail") return "Runbook 详情";
  if (route.name === "runner-ui" || route.name === "runner-workflow") return "Runner 编排";
  if (route.name === "postmortem") return "复盘草稿";
  if (route.name === "chat") return "单机会话";
  if (route.name === "protocol") return "协作工作台";
  if (route.name === "mcp") return "MCP 服务器";
  if (route.name === "prompt-traces") return "Prompt Trace";
  if (route.name === "coroot") return "Coroot 高级详情";
  if (route.name === "settings-hosts") return "主机管理";
  if (route.path.startsWith("/settings")) return "设置";
  return "Codex Workspace";
});

const isChatRoute = computed(() => route.name === "chat");
const isProtocolRoute = computed(() => route.name === "protocol");
const isHostsRoute = computed(() => route.name === "settings-hosts");

const headerHistoryLabel = computed(() => (isProtocolRoute.value ? "历史工作台" : "历史会话"));
const historyDrawerTitle = computed(() => (historyDrawerMode.value === "workspace" ? "历史工作台" : "历史会话"));
const historyDrawerSessionKind = computed(() => (historyDrawerMode.value === "workspace" ? "workspace" : "single_host"));
const historyDrawerHostScopeId = computed(() =>
  historyDrawerMode.value === "single_host"
    ? compactText(store.snapshot.selectedHostId || store.activeSessionSummary?.selectedHostId || "server-local")
    : "",
);
const historyDrawerScopeLabel = computed(() => {
  if (historyDrawerMode.value === "workspace") {
    return "仅显示多主机协作的工作台历史，主体是主 Agent 会话。";
  }
  return `仅显示当前主机 ${selectedHostLabel.value || historyDrawerHostScopeId.value || "server-local"} 下的单机会话历史。`;
});
const historyDrawerSessions = computed(() => {
  const sessions = Array.isArray(store.sessionList) ? store.sessionList : [];
  if (historyDrawerMode.value === "workspace") {
    return sessions.filter((session) => compactText(session?.kind).toLowerCase() === "workspace");
  }
  const targetHostId = historyDrawerHostScopeId.value;
  return sessions.filter((session) => {
    const sessionKind = compactText(session?.kind || "single_host").toLowerCase();
    if (sessionKind && sessionKind !== "single_host") return false;
    if (!targetHostId) return true;
    return compactText(session?.selectedHostId || "server-local") === targetHostId;
  });
});
const showHeaderHistoryButton = computed(() => isChatRoute.value || isProtocolRoute.value);
const showHeaderHostControls = computed(() => isChatRoute.value || isHostsRoute.value);
const showHeaderResetButton = computed(() => isChatRoute.value || isProtocolRoute.value);

/* --- n-menu integration --- */
const menuActiveKey = computed(() => {
  if (route.name === "incidents" || route.name === "incident-detail" || route.name === "postmortem") return "incidents";
  if (route.name === "erp-health" || route.name === "opsgraph" || route.name === "coroot") return "erp";
  if (route.name === "runbooks" || route.name === "runbook-detail") return "runbooks";
  if (route.name === "runner-ui" || route.name === "runner-workflow") return "runner-ui";
  if (route.name === "chat") return "chat";
  if (route.name === "protocol") return "protocol";
  if (route.name === "settings-hosts") return "hosts";
  return "";
});

function renderMenuIcon(icon) {
  return () => h(icon, { size: 16 });
}

const menuOptions = computed(() => [
  {
    label: "事故工作台",
    key: "incidents",
    icon: renderMenuIcon(FileSearchIcon),
  },
  {
    label: "ERP 健康",
    key: "erp",
    icon: renderMenuIcon(ActivityIcon),
  },
  {
    label: "单机会话",
    key: "chat",
    icon: renderMenuIcon(AppWindowIcon),
  },
  {
    label: "协作工作台",
    key: "protocol",
    icon: renderMenuIcon(PanelsTopLeftIcon),
  },
  {
    label: "主机列表",
    key: "hosts",
    icon: renderMenuIcon(ServerIcon),
  },
  {
    label: "Runbook",
    key: "runbooks",
    icon: renderMenuIcon(MessageSquarePlusIcon),
  },
  {
    label: "Runner 编排",
    key: "runner-ui",
    icon: renderMenuIcon(WorkflowIcon),
  },
]);

function handleMenuSelect(key) {
  closeSettingsMenu();
  switch (key) {
    case "incidents":
      router.push("/incidents");
      break;
    case "erp":
      router.push("/erp");
      break;
    case "chat":
      router.push("/");
      break;
    case "protocol":
      openWorkspaceEntry();
      break;
    case "hosts":
      router.push("/settings/hosts");
      break;
    case "runbooks":
      router.push("/runbooks");
      break;
    case "runner-ui":
      router.push("/runner");
      break;
  }
}

onMounted(() => {
  store.hydrateMcpDrawerState?.();
  void initializeAppSession();
  window.addEventListener("keydown", handleGlobalKeydown);
  window.addEventListener(OPEN_SESSION_HISTORY_EVENT, handleOpenSessionHistoryEvent);
  window.addEventListener(OPEN_MCP_DRAWER_EVENT, handleOpenMcpDrawerEvent);
  document.addEventListener("click", handleDocumentClick);
});

onBeforeUnmount(() => {
  store.disconnectWs();
  window.removeEventListener("keydown", handleGlobalKeydown);
  window.removeEventListener(OPEN_SESSION_HISTORY_EVENT, handleOpenSessionHistoryEvent);
  window.removeEventListener(OPEN_MCP_DRAWER_EVENT, handleOpenMcpDrawerEvent);
  document.removeEventListener("click", handleDocumentClick);
});

watch(
  () => [
    route.name,
    route.query.hostId,
    route.query.hostName,
    route.query.hostAddress,
    store.loading,
    appRuntimeBusy.value,
    store.snapshot.kind,
    store.activeSessionId,
    store.snapshot.selectedHostId,
    store.snapshot.hosts.length,
  ],
  () => {
    void syncChatRouteHostSelection();
  },
  { immediate: true },
);

watch(
  () => route.name,
  () => {
    if (!isHistoryDrawerOpen.value) {
      historyDrawerMode.value = resolveHistorySessionKind();
    }
  },
  { immediate: true },
);
</script>

<template>
  <n-config-provider cls-prefix="ops">
  <n-message-provider>
  <n-dialog-provider>
  <n-notification-provider>
  <div class="app-layout">
    <!-- Left Sidebar: n-layout-sider + n-menu -->
    <n-layout-sider
      bordered
      :collapsed="isSidebarCollapsed"
      :collapsed-width="64"
      :width="260"
      collapse-mode="width"
      show-trigger="bar"
      @update:collapsed="(val) => isSidebarCollapsed = val"
      class="app-sidebar"
      :class="{ 'is-sidebar-collapsed': isSidebarCollapsed }"
    >
      <div class="sidebar-top">
        <div class="sidebar-actions">
          <n-button block tertiary @click="startNewThread" class="new-thread-btn">
            <template #icon><MessageSquarePlusIcon size="18" /></template>
            <span v-if="!isSidebarCollapsed" class="nav-label">新建会话</span>
            <span v-if="!isSidebarCollapsed" class="shortcut">⌘ N</span>
          </n-button>
          <n-button block tertiary @click="startWorkspaceSession" v-if="!isSidebarCollapsed" class="new-thread-btn">
            <template #icon><PanelsTopLeftIcon size="18" /></template>
            <span class="nav-label">新建工作台</span>
          </n-button>
        </div>
      </div>

      <n-menu
        :value="menuActiveKey"
        :options="menuOptions"
        :collapsed="isSidebarCollapsed"
        :collapsed-width="64"
        :collapsed-icon-size="20"
        @update:value="handleMenuSelect"
      />

      <div class="sidebar-bottom">
        <div ref="settingsMenuRef" class="settings-menu">
          <n-button
            quaternary
            circle
            class="sidebar-settings-trigger"
            :class="{ 'is-open': isSettingsMenuOpen }"
            data-testid="sidebar-settings-button"
            @click.stop="isSettingsMenuOpen = !isSettingsMenuOpen"
            title="Settings"
          >
            <template #icon><SettingsIcon size="20" /></template>
          </n-button>
          <div
            v-if="isSettingsMenuOpen"
            class="settings-menu-popover"
            data-testid="sidebar-settings-popover"
            @click.stop
          >
            <button class="settings-menu-item" @click="closeSettingsMenu(); router.push('/settings/llm')">
              <span class="settings-menu-title">LLM 配置</span>
              <span class="settings-menu-subtitle">Provider / API Key / Model</span>
            </button>
            <button class="settings-menu-item" @click="openGeneralSettings">
              <span class="settings-menu-title">设置总览</span>
              <span class="settings-menu-subtitle">Hosts / Packs / Agent</span>
            </button>
            <button class="settings-menu-item" @click="openAgentProfile">
              <span class="settings-menu-title">Agent Profile</span>
              <span class="settings-menu-subtitle">System Prompt / Permissions / Skills / MCP</span>
            </button>
            <button class="settings-menu-item" @click="openSkillCatalog">
              <span class="settings-menu-title">Skills 管理</span>
              <span class="settings-menu-subtitle">Skill catalog / 默认值 / 激活方式</span>
            </button>
            <button class="settings-menu-item" @click="openMcpCatalog">
              <span class="settings-menu-title">MCP 服务器</span>
              <span class="settings-menu-subtitle">连接外部工具和数据源</span>
            </button>
            <button class="settings-menu-item" @click="closeSettingsMenu(); router.push('/debug/prompts')">
              <span class="settings-menu-title">Prompt Trace</span>
              <span class="settings-menu-subtitle">开发调试入口</span>
            </button>
            <button class="settings-menu-item" @click="closeSettingsMenu(); router.push('/settings/experience-packs')">
              <span class="settings-menu-title">Experience Packs</span>
              <span class="settings-menu-subtitle">Playbooks / 运维经验包</span>
            </button>
          </div>
        </div>
        <div class="flex-spacer" v-if="!isSidebarCollapsed"></div>
        <div class="ws-badge" :class="store.wsStatus" :title="'WS: ' + store.wsStatus"></div>
      </div>
    </n-layout-sider>

    <!-- Main Canvas -->
    <main class="app-main">
      <header class="main-header">
        <div class="header-left">
          <h1 class="header-title">{{ mainHeaderTitle }}</h1>
        </div>
        
        <div class="header-right">
          <n-button
            v-if="showHeaderHistoryButton"
            quaternary
            size="small"
            :title="headerHistoryLabel"
            @click="openHeaderHistoryDrawer"
            data-testid="header-history-btn"
          >
            <template #icon><HistoryIcon size="14" /></template>
            {{ headerHistoryLabel }}
          </n-button>

          <n-button
            v-if="showHeaderResetButton"
            quaternary
            size="small"
            :disabled="!canResetCurrentSession"
            :title="resetButtonTitle"
            @click="resetCurrentSession"
          >
            <template #icon><EraserIcon size="14" /></template>
            清空上下文
          </n-button>

          <n-button
            v-if="showHeaderHostControls"
            tertiary
            size="small"
            class="header-host-button"
            @click="isHostModalOpen = true"
          >
            <template #icon><ServerIcon size="14" /></template>
            <span class="header-host-content">
              <span class="header-host-label">{{ selectedHostLabel }}</span>
              <span
                class="header-host-status"
                :class="{ 'is-online': store.selectedHost.status === 'online' }"
                aria-hidden="true"
              />
            </span>
          </n-button>

          <n-button
            v-if="showHeaderHostControls"
            tertiary
            size="small"
            :disabled="!store.canOpenTerminal"
            @click="openTerminal"
            title="打开终端"
          >
            <template #icon><TerminalIcon size="14" /></template>
            终端
          </n-button>
          
          <n-button
            tertiary
            size="small"
            @click="router.push('/settings/llm')"
            :type="store.snapshot.config?.codexAlive ? 'success' : 'default'"
          >
            <template #icon><UserCircleIcon size="16" /></template>
            {{ authBadgeLabel }}
          </n-button>
          
          <n-button quaternary circle @click="toggleMcpDrawer" :class="{ 'is-active': isMcpDrawerOpen }" title="Skills & MCP">
            <template #icon><PanelsTopLeftIcon size="20" /></template>
          </n-button>
        </div>
      </header>

      <router-view />


    </main>

    <!-- Right Drawer: MCP & Core Panel (n-drawer) -->
    <n-drawer
      :show="isMcpDrawerOpen"
      placement="right"
      :width="320"
      :mask-closable="true"
      @update:show="(val) => { isMcpDrawerOpen = val; }"
    >
      <n-drawer-content title="MCP Surfaces" :native-scrollbar="false" closable>
        <template #header>
          <div class="mcp-drawer-header">
            <span>MCP Surfaces</span>
            <n-button v-if="mcpDrawerActiveSurface" text size="small" @click="pinActiveMcpSurface">
              固定当前面板
            </n-button>
          </div>
        </template>

        <!-- Active Surface -->
        <section v-if="mcpDrawerActiveSurface" data-testid="app-mcp-active-surface" style="margin-bottom: 8px;">
          <n-divider title-placement="left" style="margin: 8px 0;">
            <span class="mcp-section-kicker">ACTIVE SURFACE</span>
          </n-divider>
          <h4 style="margin: 0 0 2px;">{{ mcpDrawerActiveSurface.title }}</h4>
          <p class="mcp-sub-text">{{ mcpDrawerActiveSurface.source || "来自当前对话" }}</p>

          <McpBundleHost
            v-if="mcpDrawerActiveSurface.kind === 'bundle'"
            :bundle="mcpDrawerActiveSurface.model"
            @action="handleDrawerSurfaceAction"
            @open-detail="handleDrawerSurfaceDetail"
            @pin="pinActiveMcpSurface"
          />
          <McpUiCardHost
            v-else
            :card="mcpDrawerActiveSurface.model"
            @action="handleDrawerSurfaceAction"
            @detail="handleDrawerSurfaceDetail"
            @refresh="handleDrawerSurfaceRefresh"
          />
        </section>

        <!-- Pinned -->
        <n-divider title-placement="left" style="margin: 12px 0 8px;">
          <span class="mcp-section-kicker">PINNED</span>
        </n-divider>
        <section data-testid="app-mcp-pinned-list">
          <n-list v-if="mcpDrawerPinnedSurfaces.length" hoverable clickable bordered>
            <n-list-item
              v-for="surface in mcpDrawerPinnedSurfaces"
              :key="surface.id"
              :class="{ 'mcp-item-active': surface.id === mcpDrawerActiveSurface?.id }"
              @click="selectMcpDrawerSurface(surface)"
            >
              <div class="mcp-item-row">
                <div class="mcp-item-info">
                  <strong>{{ surface.title }}</strong>
                  <span class="mcp-sub-text">{{ surface.source || "来自当前对话" }}</span>
                </div>
                <n-button text type="error" size="small" @click.stop="removePinnedMcpSurface(surface.id)">
                  移除
                </n-button>
              </div>
            </n-list-item>
          </n-list>
          <n-empty v-else description="当前还没有固定的 MCP 面板。" style="margin: 12px 0;" />
        </section>

        <!-- Recent -->
        <n-divider title-placement="left" style="margin: 12px 0 8px;">
          <span class="mcp-section-kicker">RECENT</span>
        </n-divider>
        <section data-testid="app-mcp-recent-list">
          <n-list v-if="mcpDrawerRecentSurfaces.length" hoverable clickable bordered>
            <n-list-item
              v-for="surface in mcpDrawerRecentSurfaces"
              :key="`recent-${surface.id}`"
              :class="{ 'mcp-item-active': surface.id === mcpDrawerActiveSurface?.id }"
              @click="selectMcpDrawerSurface(surface)"
            >
              <div class="mcp-item-row">
                <div class="mcp-item-info">
                  <strong>{{ surface.title }}</strong>
                  <span class="mcp-sub-text">{{ surface.subtitle || surface.source || "最近打开" }}</span>
                </div>
                <n-button text type="info" size="small" @click.stop="pinMcpSurface(surface)">
                  固定
                </n-button>
              </div>
            </n-list-item>
          </n-list>
          <n-empty v-else description="最近还没有 MCP 操作记录。" style="margin: 12px 0;" />
        </section>

        <!-- Enabled MCPs -->
        <n-divider title-placement="left" style="margin: 12px 0 8px;">
          <span class="mcp-section-kicker">ENABLED MCPS</span>
        </n-divider>
        <section data-testid="app-mcp-enabled-list">
          <n-list v-if="enabledMcpEntries.length" hoverable clickable bordered>
            <n-list-item
              v-for="entry in enabledMcpEntries"
              :key="`enabled-${entry.id}`"
              @click="openEnabledMcpEntry(entry)"
            >
              <div class="mcp-item-row">
                <div class="mcp-item-info">
                  <strong>{{ entry.name }}</strong>
                  <span class="mcp-sub-text">{{ entry.permission === "readwrite" ? "读写" : "只读" }} · {{ entry.source || "local" }}</span>
                </div>
                <n-button text type="info" size="small" @click.stop="openEnabledMcpEntry(entry, true)">
                  固定入口
                </n-button>
              </div>
            </n-list-item>
          </n-list>
          <n-empty v-else description="当前 Agent Profile 还没有启用中的 MCP。" style="margin: 12px 0;" />
        </section>
      </n-drawer-content>
    </n-drawer>

    <SessionHistoryDrawer
      v-if="isHistoryDrawerOpen"
      :sessions="historyDrawerSessions"
      :active-session-id="store.activeSessionId"
      :loading="store.historyLoading"
      :switching-disabled="appRuntimeBusy"
      :title="historyDrawerTitle"
      :session-kind="historyDrawerSessionKind"
      :scope-label="historyDrawerScopeLabel"
      @close="isHistoryDrawerOpen = false"
      @create="startNewThread"
      @create-workspace="startWorkspaceSession"
      @select="switchSession"
    />

    <!-- Modals -->
    <LoginModal v-if="isLoginModalOpen" @close="isLoginModalOpen = false" />
    <HostModal v-if="isHostModalOpen" @close="isHostModalOpen = false" />
  </div>
  </n-notification-provider>
  </n-dialog-provider>
  </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
.main-header {
  border-bottom-color: #edf0f3;
}

.header-right :deep(.n-button),
.header-right :deep(.ops-button) {
  --n-height: 34px !important;
  --n-border-radius: 10px !important;
  --n-font-size: 14px !important;
}

.header-right :deep(.n-button .n-button__content),
.header-right :deep(.ops-button .ops-button__content) {
  gap: 7px;
}

@media (max-width: 720px) {
  .app-sidebar {
    display: none !important;
  }

  .main-header {
    gap: 10px;
    padding: 0 14px;
  }

  .header-left {
    min-width: 0;
  }

  .header-title {
    overflow: hidden;
    font-size: 15px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .header-right {
    flex: 0 0 auto;
    gap: 4px;
    min-width: 0;
  }

  .header-right :deep(.n-button),
  .header-right :deep(.ops-button) {
    width: 32px;
    min-width: 32px;
    padding: 0 !important;
  }

  .header-right :deep(.n-button .n-button__content),
  .header-right :deep(.ops-button .ops-button__content) {
    gap: 0;
    font-size: 0;
  }

  .header-right :deep(.n-button .n-button__icon),
  .header-right :deep(.ops-button .ops-button__icon) {
    margin: 0;
  }

  .header-host-content {
    display: none;
  }
}

.app-sidebar {
  display: flex;
  flex-direction: column;
  position: relative;
  z-index: 30;
}

.app-sidebar :deep(.ops-layout-sider-scroll-container) {
  min-height: 100%;
  display: flex;
  flex-direction: column;
}

.sidebar-top {
  padding: 12px 12px 8px;
}

.sidebar-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.new-thread-btn {
  justify-content: flex-start;
  height: 36px;
}

.shortcut {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-subtle, #64748b);
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
}

.header-host-button :deep(.n-button__content) {
  min-width: 0;
}

.header-host-content {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.header-host-label {
  min-width: 0;
}

.header-host-status {
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #94a3b8;
  box-shadow: 0 0 0 1px rgba(148, 163, 184, 0.18);
}

.header-host-status.is-online {
  background: #22c55e;
  box-shadow: 0 0 0 1px rgba(34, 197, 94, 0.18), 0 0 10px rgba(34, 197, 94, 0.28);
}

.sidebar-bottom {
  padding: 10px 16px 16px;
  border-top: 1px solid var(--border-color, #e2e8f0);
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: auto;
  background:
    linear-gradient(180deg, rgba(236, 239, 243, 0), rgba(236, 239, 243, 0.92) 24%),
    var(--sidebar-bg, #eceff3);
}

.settings-menu {
  position: relative;
  display: flex;
  align-items: center;
}

.sidebar-settings-trigger {
  width: 42px;
  height: 42px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.72);
  color: #475569;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.08);
  transition:
    background-color 0.18s ease,
    border-color 0.18s ease,
    box-shadow 0.18s ease,
    transform 0.18s ease;
}

.sidebar-settings-trigger:hover,
.sidebar-settings-trigger.is-open {
  border-color: rgba(37, 99, 235, 0.32);
  background: #ffffff;
  color: #1d4ed8;
  box-shadow: 0 12px 28px rgba(37, 99, 235, 0.14);
  transform: translateY(-1px);
}

.settings-menu-popover {
  position: fixed;
  left: 16px;
  bottom: 70px;
  width: min(300px, calc(100vw - 32px));
  max-height: min(520px, calc(100vh - 92px));
  overflow-y: auto;
  padding: 8px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.96);
  border: 1px solid rgba(148, 163, 184, 0.28);
  box-shadow: 0 24px 70px rgba(15, 23, 42, 0.2);
  backdrop-filter: blur(18px);
  z-index: 1200;
}

.app-sidebar.is-sidebar-collapsed .settings-menu-popover {
  left: 12px;
  width: min(288px, calc(100vw - 24px));
}

.settings-menu-item {
  display: grid;
  gap: 4px;
  width: 100%;
  border: 0;
  border-radius: 14px;
  padding: 11px 12px;
  text-align: left;
  background: transparent;
  color: #0f172a;
  cursor: pointer;
  transition:
    background-color 0.18s ease,
    box-shadow 0.18s ease,
    transform 0.18s ease;
}

.settings-menu-item + .settings-menu-item {
  margin-top: 4px;
}

.settings-menu-item:hover {
  background: rgba(37, 99, 235, 0.08);
  box-shadow: inset 3px 0 0 rgba(37, 99, 235, 0.72);
  transform: translateX(2px);
}

.settings-menu-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}

.settings-menu-subtitle {
  font-size: 12px;
  line-height: 1.4;
  color: #64748b;
}

.flex-spacer {
  flex: 1;
}

.ws-badge {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #cbd5e1;
}
.ws-badge.connected { background: #22c55e; box-shadow: 0 0 8px rgba(34, 197, 94, 0.4); }
.ws-badge.connecting { background: #eab308; }
.ws-badge.error { background: #ef4444; }

.mcp-drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 8px;
}

.mcp-section-kicker {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.mcp-sub-text {
  margin: 2px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: #64748b;
}

.mcp-item-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
}

.mcp-item-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.mcp-item-info strong {
  font-size: 13px;
  color: #0f172a;
}

.mcp-item-active {
  background: rgba(14, 165, 233, 0.06) !important;
}

.is-active {
  background: #e2e8f0;
  border-radius: 8px;
}
</style>
