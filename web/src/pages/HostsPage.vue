<script setup>
import { computed, h, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useAppStore } from "../store";
import HostEditorModal from "../components/HostEditorModal.vue";
import HostBatchTagModal from "../components/HostBatchTagModal.vue";
import { hostCapabilityLabel, labelList, normalizeHostRecord } from "../data/opsWorkspace";
import { createHost, deleteHost, fetchHostSessions as fetchHostSessionsApi, updateHost, updateHostTags } from "../api/hosts";
import { NTag, NBadge, NButtonGroup, NButton } from "naive-ui";
import { SearchIcon, TerminalIcon, EditIcon, Trash2Icon } from "lucide-vue-next";

const store = useAppStore();
const router = useRouter();

const searchQuery = ref("");
const selectedHostId = ref(store.snapshot.selectedHostId || "");
const checkedRowKeys = ref([]);
const hostSessions = ref([]);
const hostSessionsLoading = ref(false);
const editorHost = ref(null);
const showHostEditor = ref(false);
const showTagModal = ref(false);
const pageError = ref("");
const pageNotice = ref("");
const busyAction = ref("");

function clearMessage(kind = "all") {
  if (kind === "all" || kind === "error") pageError.value = "";
  if (kind === "all" || kind === "notice") pageNotice.value = "";
}
function pushError(message) { pageNotice.value = ""; pageError.value = message; }
function pushNotice(message) { pageError.value = ""; pageNotice.value = message; }

function parseIsoTime(value) {
  if (!value || value === "offline") return null;
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) return null;
  return timestamp;
}

function formatLastSeen(value, status) {
  if (!value) return status === "online" ? "刚注册" : "未建立控制通道";
  if (value === "offline") return "未建立控制通道";
  const timestamp = parseIsoTime(value);
  if (!timestamp) return value;
  const diffSeconds = Math.max(1, Math.floor((Date.now() - timestamp) / 1000));
  if (diffSeconds < 60) return `${diffSeconds}s 前`;
  if (diffSeconds < 3600) return `${Math.floor(diffSeconds / 60)}m 前`;
  if (diffSeconds < 86400) return `${Math.floor(diffSeconds / 3600)}h 前`;
  return `${Math.floor(diffSeconds / 86400)}d 前`;
}

function statusLabel(host) {
  switch (host.status) {
    case "online": return "控制通道活跃";
    case "connecting": return "安装完成，等待回连";
    case "installing": return "SSH 安装中";
    case "pending_install": return "待安装 agent";
    default: return host.status || "未接入";
  }
}

function transportLabel(host) {
  switch (host.transport) {
    case "local": return "本机";
    case "grpc_reverse": return "反向 gRPC";
    case "ssh_bootstrap": return "SSH 引导";
    default: return host.transport || host.kind || "inventory";
  }
}

function statusBadgeType(host) {
  if (host.status === "online") return "success";
  if (host.status === "installing" || host.status === "connecting") return "info";
  if (host.installState === "install_failed") return "error";
  return "warning";
}

const hostRecords = computed(() => {
  return (store.snapshot.hosts || []).map((host, index) => {
    const normalized = normalizeHostRecord(host, index);
    return { ...normalized, lastSeenText: formatLastSeen(normalized.lastHeartbeat, normalized.status) };
  });
});

const filteredHosts = computed(() => {
  const query = searchQuery.value.trim().toLowerCase();
  if (!query) return hostRecords.value;
  return hostRecords.value.filter((host) => {
    const labelText = labelList(host.labels).join(" ").toLowerCase();
    return [host.id, host.name, host.address, host.kind, host.status, host.transport, labelText]
      .filter(Boolean).some((item) => String(item).toLowerCase().includes(query));
  });
});

const selectedHost = computed(() => {
  return filteredHosts.value.find((host) => host.id === selectedHostId.value) || filteredHosts.value[0] || null;
});

const metrics = computed(() => {
  const hosts = hostRecords.value.filter((host) => host.id !== "server-local");
  const online = hosts.filter((host) => host.status === "online").length;
  const executable = hosts.filter((host) => host.executable).length;
  const waitingInstall = hosts.filter((host) => host.installState !== "installed").length;
  const sessions = store.sessionList.filter((s) => s.selectedHostId && s.selectedHostId !== "server-local").length;
  return [
    { label: "控制通道活跃", value: online, meta: `${Math.max(hosts.length - online, 0)} 台不活跃`, type: "success" },
    { label: "待安装 / 待回连", value: waitingInstall, meta: "SSH 安装或等待 agent 回连", type: "warning" },
    { label: "可执行主机", value: executable, meta: `${Math.max(hosts.length - executable, 0)} 台仅在册`, type: "info" },
    { label: "远程会话数", value: sessions, meta: "每个子 agent 对应一个会话", type: "default" },
  ];
});

// n-data-table columns
const tableColumns = computed(() => [
  { type: "selection" },
  {
    title: "名称",
    key: "name",
    render(row) {
      return h("div", { style: "display:flex;align-items:center;gap:8px" }, [
        h(NBadge, { dot: true, type: statusBadgeType(row), offset: [0, 0] }),
        h("div", {}, [
          h("strong", {}, row.name),
          h("div", { style: "font-size:12px;color:#64748b" }, statusLabel(row)),
        ]),
      ]);
    },
  },
  {
    title: "HOST ID",
    key: "id",
    render(row) {
      return h(NTag, { size: "small", bordered: false }, { default: () => row.id });
    },
  },
  {
    title: "接入方式",
    key: "transport",
    render(row) {
      return h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
        h(NTag, { size: "small", type: "info" }, { default: () => transportLabel(row) }),
        h(NTag, { size: "small", type: row.installState === "installed" ? "success" : "warning" }, { default: () => row.installState || "inventory" }),
      ]);
    },
  },
  { title: "OS", key: "os", width: 120 },
  { title: "最近握手", key: "lastSeenText", width: 120, align: "right" },
  {
    title: "操作",
    key: "actions",
    width: 200,
    render(row) {
      return h(NButtonGroup, { size: "small" }, {
        default: () => [
          h(NButton, {
            size: "small",
            disabled: !(row.status === "online" && (row.terminalCapable || row.executable)),
            onClick: (e) => { e.stopPropagation(); openTerminal(row); },
          }, { icon: () => h(TerminalIcon, { size: 14 }), default: () => "终端" }),
          h(NButton, {
            size: "small",
            disabled: row.id === "server-local",
            onClick: (e) => { e.stopPropagation(); openEditModal(row); },
          }, { icon: () => h(EditIcon, { size: 14 }), default: () => "编辑" }),
          h(NButton, {
            size: "small",
            disabled: row.id === "server-local",
            onClick: (e) => { e.stopPropagation(); removeHost(row); },
          }, { icon: () => h(Trash2Icon, { size: 14 }), default: () => "删除" }),
        ],
      });
    },
  },
]);

const rowKey = (row) => row.id;

function handleRowClick(row) {
  selectHost(row);
}

watch(filteredHosts, (hosts) => {
  if (!hosts.length) { selectedHostId.value = ""; return; }
  if (!hosts.some((h) => h.id === selectedHostId.value)) {
    selectedHostId.value = store.snapshot.selectedHostId || hosts[0].id;
  }
  checkedRowKeys.value = checkedRowKeys.value.filter((id) => hosts.some((h) => h.id === id));
}, { immediate: true });

watch(() => store.snapshot.selectedHostId, (nextHostId) => { if (nextHostId) selectedHostId.value = nextHostId; });

const selectedSessionsCount = computed(() => hostSessions.value.length);

async function refreshInventory() { await Promise.all([store.fetchState(), store.fetchSessions()]); }

async function loadHostSessions(hostId) {
  if (!hostId) { hostSessions.value = []; return; }
  hostSessionsLoading.value = true;
  try {
    const data = await fetchHostSessionsApi(hostId, { limit: 8 });
    hostSessions.value = data.items || [];
  } catch (_err) { pushError("加载主机会话失败"); }
  finally { hostSessionsLoading.value = false; }
}

watch(selectedHostId, async (hostId) => { await loadHostSessions(hostId); }, { immediate: true });

async function selectHost(host) { selectedHostId.value = host.id; await store.selectHost(host.id); }
async function openTerminal(host) { await selectHost(host); router.push(`/terminal/${host.id}`); }

function openCreateModal() { editorHost.value = null; showHostEditor.value = true; }
function openEditModal(host) { editorHost.value = host; showHostEditor.value = true; }

async function saveHost(payload) {
  clearMessage();
  const isEditing = !!editorHost.value?.id;
  busyAction.value = isEditing ? `update:${editorHost.value.id}` : "create";
  try {
    const data = isEditing ? await updateHost(editorHost.value.id, payload) : await createHost(payload);
    showHostEditor.value = false;
    await refreshInventory();
    selectedHostId.value = data.host?.id || payload.id;
    await loadHostSessions(selectedHostId.value);
    pushNotice(payload.installViaSsh ? "主机已创建，并已触发 SSH 安装。" : "主机信息已保存。");
  } catch (_err) { pushError("保存主机失败"); }
  finally { busyAction.value = ""; }
}

async function applyBatchTags(payload) {
  clearMessage();
  busyAction.value = "tags";
  try {
    await updateHostTags({ hostIds: checkedRowKeys.value, add: payload.add, remove: payload.remove });
    showTagModal.value = false;
    await refreshInventory();
    await loadHostSessions(selectedHostId.value);
    pushNotice(`已更新 ${checkedRowKeys.value.length} 台主机标签。`);
  } catch (_err) { pushError("批量标签失败"); }
  finally { busyAction.value = ""; }
}

async function removeHost(host) {
  if (!host?.id || host.id === "server-local") return;
  const confirmed = window.confirm(`确认删除主机 ${host.name || host.id} 吗？`);
  if (!confirmed) return;
  clearMessage();
  busyAction.value = `delete:${host.id}`;
  try {
    await deleteHost(host.id);
    await refreshInventory();
    checkedRowKeys.value = checkedRowKeys.value.filter((id) => id !== host.id);
    pushNotice("主机已删除。");
  } catch (_err) { pushError("删除主机失败"); }
  finally { busyAction.value = ""; }
}

async function jumpToSession(sessionId) {
  const ok = await store.activateSession(sessionId);
  if (!ok) return;
  router.push("/");
}

onMounted(async () => { await refreshInventory(); });
</script>

<template>
  <section class="ops-page">
    <div class="ops-page-inner">
      <header class="ops-page-header">
        <div>
          <h2 class="ops-page-title">主机管理</h2>
          <p class="ops-page-subtitle">主机清单、SSH 引导安装、批量标签，以及每台子 agent 对应会话都收敛在一个 inventory 视图里。</p>
        </div>
      </header>

      <div class="ops-scope-bar">
        <div class="ops-scope-left">
          <span class="ops-scope-label">Connection Semantics</span>
          <n-tag type="info" size="small">状态 = 控制通道活跃度</n-tag>
          <n-tag size="small">接入 = reverse gRPC</n-tag>
          <n-tag size="small">安装 = SSH 引导</n-tag>
        </div>
        <div class="ops-actions">
          <n-button quaternary @click="showTagModal = true" :disabled="!checkedRowKeys.length">批量标签</n-button>
          <n-button type="primary" @click="openCreateModal">新增主机</n-button>
        </div>
      </div>

      <n-alert v-if="pageNotice" type="success" closable @close="pageNotice = ''">{{ pageNotice }}</n-alert>
      <n-alert v-if="pageError" type="error" closable @close="pageError = ''">{{ pageError }}</n-alert>

      <n-grid :cols="4" :x-gap="12" :y-gap="12" responsive="screen" :item-responsive="true">
        <n-gi v-for="metric in metrics" :key="metric.label" span="4 m:1">
          <n-card size="small">
            <n-statistic :label="metric.label" :value="metric.value">
              <template #suffix>
                <n-text depth="3" style="font-size:12px;">{{ metric.meta }}</n-text>
              </template>
            </n-statistic>
          </n-card>
        </n-gi>
      </n-grid>

      <div class="ops-grid ops-grid-hosts">
        <n-card>
          <template #header>
            <div class="hosts-toolbar">
              <div>
                <h3 style="margin:0;">主机清单</h3>
                <n-text depth="3">支持新增主机、SSH 安装、批量打标签，以及查看子会话。</n-text>
              </div>
              <n-input
                v-model:value="searchQuery"
                placeholder="搜索主机 / 标签 / 地址"
                clearable
                style="width:280px;"
              >
                <template #prefix><SearchIcon :size="14" /></template>
              </n-input>
            </div>
          </template>

          <n-data-table
            :columns="tableColumns"
            :data="filteredHosts"
            :row-key="rowKey"
            v-model:checked-row-keys="checkedRowKeys"
            :row-props="(row) => ({ style: 'cursor:pointer', onClick: () => handleRowClick(row) })"
            :bordered="false"
            size="small"
            :pagination="{ pageSize: 20 }"
          />
        </n-card>

        <n-card v-if="selectedHost" class="ops-sidebar-card">
          <template #header>
            <h3 style="margin:0;">主机详情</h3>
            <n-text depth="3">{{ selectedHost.name }} · {{ selectedHost.id }}</n-text>
          </template>

          <div class="ops-badge-row">
            <n-tag :type="statusBadgeType(selectedHost)" size="small">{{ statusLabel(selectedHost) }}</n-tag>
            <n-tag size="small">{{ transportLabel(selectedHost) }}</n-tag>
            <n-tag :type="selectedHost.installState === 'installed' ? 'success' : 'warning'" size="small">{{ selectedHost.installState || "inventory" }}</n-tag>
          </div>

          <div class="ops-detail-block">
            <h4>{{ selectedHost.os }} · {{ selectedHost.arch }}</h4>
            <n-text depth="3">target {{ selectedHost.address || "server-local" }} · agent {{ selectedHost.agentVersion || "未注册" }}</n-text>
            <br />
            <n-text depth="3">最近握手 {{ selectedHost.lastSeenText }} · 模式 {{ selectedHost.controlMode || "inventory" }}</n-text>
          </div>

          <div class="ops-subcard">
            <span class="ops-subcard-label">标签</span>
            <div class="ops-chip-row">
              <n-tag v-for="label in labelList(selectedHost.labels)" :key="label" size="small">{{ label }}</n-tag>
              <n-text v-if="!labelList(selectedHost.labels).length" depth="3">暂无标签</n-text>
            </div>
          </div>

          <div class="ops-subcard">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;">
              <span class="ops-subcard-label">主 Agent / 子会话</span>
              <n-text depth="3">{{ selectedSessionsCount }} 个会话</n-text>
            </div>
            <div v-if="hostSessionsLoading"><n-text depth="3">正在加载会话...</n-text></div>
            <div v-else-if="!hostSessions.length"><n-text depth="3">该主机还没有独立子会话。</n-text></div>
            <div v-else class="host-session-list">
              <n-card v-for="session in hostSessions" :key="session.sessionId" size="small" embedded>
                <div style="display:flex;justify-content:space-between;align-items:center;">
                  <strong>{{ session.title || session.sessionId }}</strong>
                  <n-tag size="small">{{ session.status }}</n-tag>
                </div>
                <p class="host-session-copy"><span class="host-session-label">主 Agent 任务</span>{{ session.taskSummary || "暂无任务摘要" }}</p>
                <p class="host-session-copy"><span class="host-session-label">子会话回复</span>{{ session.replySummary || "暂无回复摘要" }}</p>
                <div style="display:flex;justify-content:space-between;align-items:center;margin-top:8px;">
                  <n-text depth="3">{{ session.messageCount }} 条消息 · {{ formatLastSeen(session.lastActivityAt, session.status) }}</n-text>
                  <n-button size="tiny" quaternary @click="jumpToSession(session.sessionId)">切到该会话</n-button>
                </div>
              </n-card>
            </div>
          </div>

          <div class="ops-card-actions">
            <n-button-group>
              <n-button type="primary" @click="openTerminal(selectedHost)" :disabled="!(selectedHost.status === 'online' && (selectedHost.terminalCapable || selectedHost.executable))">进入终端</n-button>
              <n-button @click="selectHost(selectedHost)">设为当前上下文</n-button>
              <n-button @click="openEditModal(selectedHost)" :disabled="selectedHost.id === 'server-local'">编辑主机</n-button>
              <n-button type="error" @click="removeHost(selectedHost)" :disabled="selectedHost.id === 'server-local'">删除主机</n-button>
            </n-button-group>
          </div>
        </n-card>
      </div>
    </div>
  </section>

  <HostEditorModal v-if="showHostEditor" :host="editorHost" @close="showHostEditor = false" @save="saveHost" />
  <HostBatchTagModal v-if="showTagModal" :count="checkedRowKeys.length" @close="showTagModal = false" @save="applyBatchTags" />
</template>

<style scoped>
.hosts-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.ops-scope-bar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.ops-scope-left { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.ops-scope-label { font-size: 12px; font-weight: 700; color: #64748b; text-transform: uppercase; }
.ops-actions { display: flex; gap: 8px; }
.ops-badge-row { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.ops-detail-block { margin: 12px 0; }
.ops-detail-block h4 { margin: 0 0 4px; }
.ops-subcard { margin-top: 16px; }
.ops-subcard-label { font-size: 12px; font-weight: 700; color: #64748b; text-transform: uppercase; display: block; margin-bottom: 8px; }
.ops-chip-row { display: flex; gap: 6px; flex-wrap: wrap; }
.ops-card-actions { margin-top: 16px; }
.ops-grid { display: grid; grid-template-columns: minmax(0, 1fr) 380px; gap: 16px; margin-top: 16px; }
.host-session-list { display: flex; flex-direction: column; gap: 10px; }
.host-session-copy { margin: 6px 0 0; font-size: 13px; line-height: 1.6; color: #1e293b; }
.host-session-label { display: inline-block; min-width: 88px; color: #64748b; font-weight: 600; }
@media (max-width: 960px) { .ops-grid { grid-template-columns: 1fr; } }
</style>
