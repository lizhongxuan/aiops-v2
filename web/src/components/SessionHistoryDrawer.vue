<script setup>
import { computed, reactive } from "vue";
import { HistoryIcon, PlusIcon, PanelsTopLeftIcon, ChevronRightIcon } from "lucide-vue-next";

const props = defineProps({
  sessions: { type: Array, default: () => [] },
  title: { type: String, default: "历史会话" },
  scopeLabel: { type: String, default: "" },
  sessionKind: { type: String, default: "" },
  activeSessionId: { type: String, default: "" },
  loading: { type: Boolean, default: false },
  switchingDisabled: { type: Boolean, default: false },
});

const emit = defineEmits(["close", "create", "create-workspace", "select"]);

const DEFAULT_VISIBLE = 5;
const expandedGroups = reactive({});

const HOST_COLORS = ["#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#ec4899", "#06b6d4", "#84cc16"];

function hostColor(index) {
  return HOST_COLORS[index % HOST_COLORS.length];
}

const visibleSessions = computed(() => (Array.isArray(props.sessions) ? props.sessions : []));
const hasSessions = computed(() => visibleSessions.value.length > 0);

// Group sessions by host
const groupedByHost = computed(() => {
  const groups = new Map();
  for (const session of visibleSessions.value) {
    const kind = String(session?.kind || "").trim().toLowerCase();
    const hostKey = kind === "workspace" ? "__workspace__" : String(session?.selectedHostId || "server-local");
    if (!groups.has(hostKey)) {
      groups.set(hostKey, { hostId: hostKey, hostLabel: hostKey === "__workspace__" ? "协作工作台" : hostKey, sessions: [] });
    }
    groups.get(hostKey).sessions.push(session);
  }
  // Sort: workspace first, then alphabetical
  const entries = [...groups.values()];
  entries.sort((a, b) => {
    if (a.hostId === "__workspace__") return -1;
    if (b.hostId === "__workspace__") return 1;
    return a.hostLabel.localeCompare(b.hostLabel);
  });
  return entries;
});

function visibleSessionsForGroup(group) {
  const limit = expandedGroups[group.hostId] ? group.sessions.length : DEFAULT_VISIBLE;
  return group.sessions.slice(0, limit);
}

function hasMore(group) {
  return !expandedGroups[group.hostId] && group.sessions.length > DEFAULT_VISIBLE;
}

function loadMore(group) {
  expandedGroups[group.hostId] = true;
}

function isSessionActive(session) {
  return session.status === "running" || session.status === "executing";
}

function getWorkerSessions(session) {
  if (String(session?.kind || "").trim().toLowerCase() !== "workspace") return [];
  return Array.isArray(session.workerSessions) ? session.workerSessions : [];
}

function formatTime(value) {
  if (!value) return "刚刚";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "刚刚";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).format(date);
}

function statusLabel(status) {
  switch (status) {
    case "running": return "执行中";
    case "waiting_approval": return "待确认";
    case "failed": return "失败";
    case "completed": return "已完成";
    default: return "空白";
  }
}

function statusType(status) {
  switch (status) {
    case "running": return "info";
    case "waiting_approval": return "warning";
    case "failed": return "error";
    case "completed": return "success";
    default: return "default";
  }
}

function handleSelect(sessionId) {
  emit("select", sessionId);
}
</script>

<template>
  <n-drawer
    :show="true"
    :width="380"
    placement="left"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <n-drawer-content :title="title" :native-scrollbar="false" closable>
      <template #header>
        <div class="drawer-header-content">
          <HistoryIcon size="18" />
          <span>{{ title }}</span>
        </div>
      </template>

      <p v-if="scopeLabel" class="scope-label">{{ scopeLabel }}</p>

      <div class="create-actions">
        <n-button block @click="emit('create')" :disabled="switchingDisabled">
          <template #icon><PlusIcon size="16" /></template>
          新建会话
        </n-button>
        <n-button block quaternary @click="emit('create-workspace')" :disabled="switchingDisabled">
          <template #icon><PanelsTopLeftIcon size="16" /></template>
          新建工作台
        </n-button>
      </div>

      <n-alert v-if="switchingDisabled" type="warning" :bordered="false" style="margin: 12px 0;">
        当前任务执行中，完成后再切换或新建会话。
      </n-alert>

      <div v-if="loading" class="history-empty">
        <n-spin size="small" />
        <span>正在加载会话...</span>
      </div>

      <div v-else-if="!hasSessions" class="history-empty">
        <n-empty :description="`还没有${title}，先开始第一段${sessionKind === 'workspace' ? '工作台' : '对话'}。`" />
      </div>

      <!-- Grouped by host -->
      <div v-else class="host-groups">
        <div v-for="(group, gIdx) in groupedByHost" :key="group.hostId" class="host-group">
          <!-- Group header: colored dot + host name -->
          <div class="host-group-header">
            <span class="host-dot" :style="{ background: hostColor(gIdx) }"></span>
            <span class="host-group-label">{{ group.hostLabel }}</span>
            <span class="host-group-count">{{ group.sessions.length }}</span>
          </div>

          <n-list hoverable clickable size="small">
            <n-list-item
              v-for="session in visibleSessionsForGroup(group)"
              :key="session.id"
              :class="{ 'session-active': session.id === activeSessionId }"
              @click="handleSelect(session.id)"
            >
              <div class="session-top">
                <span v-if="isSessionActive(session)" class="pulse-dot amber"></span>
                <span class="session-title">{{ session.title }}</span>
                <span class="session-time">{{ formatTime(session.lastActivityAt) }}</span>
              </div>
              <p class="session-preview">{{ session.preview }}</p>
              <n-space size="small" align="center">
                <n-tag :type="statusType(session.status)" size="small" round>{{ statusLabel(session.status) }}</n-tag>
                <span class="session-count">{{ session.messageCount || 0 }} 条消息</span>
              </n-space>

              <!-- Worker sessions indented under workspace sessions -->
              <div v-if="getWorkerSessions(session).length" class="worker-sessions">
                <div
                  v-for="worker in getWorkerSessions(session)"
                  :key="worker.id"
                  class="worker-session-item"
                  @click.stop="handleSelect(worker.id)"
                >
                  <ChevronRightIcon size="12" class="worker-icon" />
                  <span class="worker-title">{{ worker.title || worker.id }}</span>
                  <n-tag :type="statusType(worker.status)" size="tiny" round>{{ statusLabel(worker.status) }}</n-tag>
                </div>
              </div>
            </n-list-item>
          </n-list>

          <!-- Load more -->
          <div v-if="hasMore(group)" class="load-more-row">
            <n-button text size="small" @click="loadMore(group)">
              加载更多（还有 {{ group.sessions.length - DEFAULT_VISIBLE }} 个）
            </n-button>
          </div>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.drawer-header-content {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.scope-label {
  margin: 0 0 12px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.create-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 16px;
}

.history-empty {
  min-height: 160px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #64748b;
  text-align: center;
  font-size: 13px;
}

/* Host group */
.host-groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.host-group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0 6px;
}

.host-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.host-group-label {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
}

.host-group-count {
  font-size: 11px;
  color: #94a3b8;
  margin-left: auto;
}

/* Session items */
.session-active {
  background: #eff6ff !important;
  border-left: 3px solid #3b82f6;
}

.session-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.session-title {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.session-time {
  font-size: 11px;
  color: #94a3b8;
  flex-shrink: 0;
}

.session-preview {
  margin: 6px 0 8px;
  font-size: 12px;
  line-height: 1.5;
  color: #475569;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.session-count {
  font-size: 11px;
  color: #64748b;
}

/* Amber pulse dot for active sessions */
.pulse-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.pulse-dot.amber {
  background: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.5);
  animation: pulse-amber 1.5s ease-in-out infinite;
}

@keyframes pulse-amber {
  0%, 100% { opacity: 1; box-shadow: 0 0 4px rgba(245, 158, 11, 0.4); }
  50% { opacity: 0.6; box-shadow: 0 0 10px rgba(245, 158, 11, 0.7); }
}

/* Worker sessions indented */
.worker-sessions {
  margin-top: 8px;
  padding-left: 16px;
  border-left: 2px solid #e2e8f0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.worker-session-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  color: #475569;
}

.worker-session-item:hover {
  background: #f1f5f9;
}

.worker-icon {
  color: #94a3b8;
  flex-shrink: 0;
}

.worker-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Load more */
.load-more-row {
  text-align: center;
  padding: 6px 0;
}
</style>
