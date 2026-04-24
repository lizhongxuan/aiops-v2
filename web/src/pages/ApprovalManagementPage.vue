<script setup>
import { computed, h, onMounted, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useAppStore } from "../store";
import { NTag, NButton, NSwitch } from "naive-ui";
import {
  ArrowLeftIcon,
  CheckCircleIcon,
  ClockIcon,
  FilterIcon,
  RefreshCcwIcon,
  ShieldCheckIcon,
  TerminalIcon,
  XCircleIcon,
} from "lucide-vue-next";
import {
  disableApprovalGrant,
  enableApprovalGrant,
  fetchApprovalAudits,
  fetchApprovalGrants,
  revokeApprovalGrant,
} from "../api/approvalManagement";

const router = useRouter();
const store = useAppStore();

const activeTab = ref("audits");
const stats = reactive({ todayTotal: 0, pending: 0, autoAccepted: 0, grantedCommands: 0 });
const filters = reactive({ timeRange: null, sessionKind: "", host: "", operator: "", decision: "", toolName: "" });
const audits = ref([]);
const auditPage = ref(1);
const auditPageSize = ref(20);
const auditTotal = ref(0);
const auditLoading = ref(false);
const auditTotalPages = computed(() => Math.max(1, Math.ceil(auditTotal.value / auditPageSize.value)));
const drawerVisible = ref(false);
const drawerRecord = ref(null);
const grants = ref([]);
const grantsLoading = ref(false);
const grantsHostId = ref("");

function buildAuditQuery() {
  const params = new URLSearchParams();
  params.set("page", String(auditPage.value));
  params.set("pageSize", String(auditPageSize.value));
  if (filters.timeRange) params.set("timeRange", filters.timeRange);
  if (filters.sessionKind) params.set("sessionKind", filters.sessionKind);
  if (filters.host) params.set("host", filters.host);
  if (filters.operator) params.set("operator", filters.operator);
  if (filters.decision) params.set("decision", filters.decision);
  if (filters.toolName) params.set("toolName", filters.toolName);
  return params.toString();
}

async function fetchAudits() {
  auditLoading.value = true;
  try {
    const query = buildAuditQuery();
    const data = await fetchApprovalAudits(Object.fromEntries(new URLSearchParams(query).entries()));
    audits.value = data.items || [];
    auditTotal.value = data.total || 0;
    stats.todayTotal = data.stats?.todayTotal ?? stats.todayTotal;
    stats.pending = data.stats?.pending ?? stats.pending;
    stats.autoAccepted = data.stats?.autoAccepted ?? stats.autoAccepted;
    stats.grantedCommands = data.stats?.grantedCommands ?? stats.grantedCommands;
  } catch (err) { console.error("[ApprovalManagement] fetchAudits failed:", err); }
  finally { auditLoading.value = false; }
}

async function fetchGrants(hostId) {
  grantsLoading.value = true;
  try {
    const data = await fetchApprovalGrants(hostId);
    grants.value = data.items || [];
  } catch (err) { console.error("[ApprovalManagement] fetchGrants failed:", err); }
  finally { grantsLoading.value = false; }
}

async function grantAction(grantId, action) {
  try {
    if (action === "revoke") await revokeApprovalGrant(grantId);
    else if (action === "disable") await disableApprovalGrant(grantId);
    else if (action === "enable") await enableApprovalGrant(grantId);
    await fetchGrants(grantsHostId.value);
  } catch (err) { console.error(`[ApprovalManagement] grantAction(${action}) failed:`, err); }
}

function revokeGrant(grantId) { return grantAction(grantId, "revoke"); }
function disableGrant(grantId) { return grantAction(grantId, "disable"); }
function enableGrant(grantId) { return grantAction(grantId, "enable"); }

function applyFilters() { auditPage.value = 1; fetchAudits(); }
function resetFilters() {
  Object.assign(filters, { timeRange: null, sessionKind: "", host: "", operator: "", decision: "", toolName: "" });
  applyFilters();
}

function decisionTagType(decision) {
  switch (decision) {
    case "approved": return "success";
    case "rejected": return "error";
    case "auto_accepted": return "info";
    case "pending": return "warning";
    default: return "default";
  }
}
function decisionLabel(decision) {
  switch (decision) {
    case "approved": return "已批准";
    case "rejected": return "已拒绝";
    case "auto_accepted": return "自动放行";
    case "pending": return "待审核";
    default: return decision || "未知";
  }
}
function grantStatusType(status) {
  switch (status) {
    case "active": return "success";
    case "disabled": return "warning";
    case "revoked": return "error";
    default: return "default";
  }
}
function grantStatusLabel(status) {
  switch (status) {
    case "active": return "生效中";
    case "disabled": return "已禁用";
    case "revoked": return "已撤销";
    default: return status || "未知";
  }
}
function formatTime(value) {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("zh-CN", { hour12: false });
}

function openDrawer(record) { drawerRecord.value = record; drawerVisible.value = true; }
function closeDrawer() { drawerVisible.value = false; drawerRecord.value = null; }

// Audit table columns
const auditColumns = [
  { title: "时间", key: "createdAt", render: (row) => formatTime(row.createdAt) },
  { title: "会话类型", key: "sessionKind", render: (row) => row.sessionKind || "-" },
  { title: "主机", key: "host", render: (row) => row.host || "-" },
  { title: "操作人", key: "operator", render: (row) => row.operator || "-" },
  { title: "工具", key: "toolName", render: (row) => row.toolName || "-" },
  {
    title: "决策",
    key: "decision",
    render: (row) => h(NTag, { type: decisionTagType(row.decision), size: "small" }, { default: () => decisionLabel(row.decision) }),
  },
  {
    title: "操作",
    key: "actions",
    render: (row) => h(NButton, { size: "small", quaternary: true, onClick: () => openDrawer(row) }, { default: () => "详情" }),
  },
];

// Grants table columns
const grantColumns = [
  { title: "授权 ID", key: "id" },
  { title: "主机", key: "hostId", render: (row) => row.hostId || "-" },
  { title: "命令/工具", key: "command", render: (row) => row.command || row.toolName || "-" },
  { title: "授权时间", key: "grantedAt", render: (row) => formatTime(row.grantedAt) },
  {
    title: "状态",
    key: "status",
    render: (row) => h(NTag, { type: grantStatusType(row.status), size: "small" }, { default: () => grantStatusLabel(row.status) }),
  },
  {
    title: "操作",
    key: "actions",
    render: (row) => {
      const buttons = [];
      if (row.status === "active") {
        buttons.push(h(NButton, { size: "small", quaternary: true, onClick: () => revokeGrant(row.id) }, { default: () => "撤销" }));
        buttons.push(h(NButton, { size: "small", quaternary: true, onClick: () => disableGrant(row.id) }, { default: () => "禁用" }));
      }
      if (row.status === "disabled") {
        buttons.push(h(NButton, { size: "small", type: "primary", quaternary: true, onClick: () => enableGrant(row.id) }, { default: () => "启用" }));
      }
      return h("div", { style: "display:flex;gap:6px" }, buttons);
    },
  },
];

watch(activeTab, (tab) => { if (tab === "grants") fetchGrants(grantsHostId.value); });
watch(grantsHostId, (hostId) => { if (activeTab.value === "grants") fetchGrants(hostId); });
onMounted(() => { fetchAudits(); });
</script>

<template>
  <div class="approval-page" data-testid="approval-management-page">
    <header class="approval-header">
      <n-button text @click="router.push('/')"><ArrowLeftIcon :size="16" /> 返回工作区</n-button>
      <div class="header-copy">
        <h1>审批管理</h1>
        <p>集中查看审批流水、授权记录，管理命令授权的生命周期。</p>
      </div>
    </header>

    <n-grid :cols="4" :x-gap="12" :y-gap="12" responsive="screen" :item-responsive="true" data-testid="stats-row">
      <n-gi span="4 m:1">
        <n-card size="small"><n-statistic label="今日审批" :value="stats.todayTotal" data-testid="stat-today-total" /></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card size="small"><n-statistic label="待处理" :value="stats.pending" data-testid="stat-pending" /></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card size="small"><n-statistic label="自动放行" :value="stats.autoAccepted" data-testid="stat-auto-accepted" /></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card size="small"><n-statistic label="已授权命令" :value="stats.grantedCommands" data-testid="stat-granted-commands" /></n-card>
      </n-gi>
    </n-grid>

    <n-tabs v-model:value="activeTab" type="line" data-testid="tab-bar">
      <n-tab-pane name="audits" tab="审核流水">
        <n-card size="small" style="margin-bottom:16px;" data-testid="filter-bar">
          <n-space align="end" :wrap="true">
            <n-date-picker v-model:value="filters.timeRange" type="daterange" clearable placeholder="时间范围" size="small" data-testid="filter-time-range" />
            <n-select v-model:value="filters.sessionKind" :options="[{label:'全部',value:''},{label:'chat',value:'chat'},{label:'workspace',value:'workspace'}]" size="small" style="width:120px;" placeholder="会话类型" data-testid="filter-session-kind" />
            <n-input v-model:value="filters.host" size="small" placeholder="主机 ID" style="width:120px;" data-testid="filter-host" />
            <n-input v-model:value="filters.operator" size="small" placeholder="操作人" style="width:120px;" data-testid="filter-operator" />
            <n-select v-model:value="filters.decision" :options="[{label:'全部',value:''},{label:'已批准',value:'approved'},{label:'已拒绝',value:'rejected'},{label:'自动放行',value:'auto_accepted'},{label:'待审核',value:'pending'}]" size="small" style="width:120px;" placeholder="决策" data-testid="filter-decision" />
            <n-input v-model:value="filters.toolName" size="small" placeholder="工具名称" style="width:120px;" data-testid="filter-tool-name" />
            <n-button type="primary" size="small" @click="applyFilters" data-testid="apply-filters-btn">筛选</n-button>
            <n-button size="small" @click="resetFilters" data-testid="reset-filters-btn">重置</n-button>
          </n-space>
        </n-card>

        <n-data-table
          :columns="auditColumns"
          :data="audits"
          :loading="auditLoading"
          :row-key="(row) => row.id"
          :row-props="(row) => ({ style: 'cursor:pointer', onClick: () => openDrawer(row) })"
          :bordered="false"
          size="small"
          :pagination="{ page: auditPage, pageSize: auditPageSize, pageCount: auditTotalPages, onChange: (p) => { auditPage = p; fetchAudits(); } }"
          data-testid="audit-table"
        />
      </n-tab-pane>

      <n-tab-pane name="grants" tab="授权列表">
        <n-space align="end" style="margin-bottom:12px;">
          <n-input v-model:value="grantsHostId" size="small" placeholder="按主机筛选" style="width:200px;" data-testid="grants-host-filter" />
          <n-button size="small" @click="fetchGrants(grantsHostId)" data-testid="grants-refresh-btn">
            <template #icon><RefreshCcwIcon :size="14" /></template>
            刷新
          </n-button>
        </n-space>

        <n-data-table
          :columns="grantColumns"
          :data="grants"
          :loading="grantsLoading"
          :row-key="(row) => row.id"
          :bordered="false"
          size="small"
          data-testid="grants-table"
        />
      </n-tab-pane>
    </n-tabs>

    <n-drawer v-model:show="drawerVisible" :width="420" placement="right" data-testid="detail-drawer">
      <n-drawer-content title="审批详情" closable>
        <template v-if="drawerRecord">
          <n-descriptions :column="1" label-placement="left" bordered size="small">
            <n-descriptions-item label="审批 ID">{{ drawerRecord.id }}</n-descriptions-item>
            <n-descriptions-item label="时间">{{ formatTime(drawerRecord.createdAt) }}</n-descriptions-item>
            <n-descriptions-item label="会话类型">{{ drawerRecord.sessionKind || "-" }}</n-descriptions-item>
            <n-descriptions-item label="主机">{{ drawerRecord.host || "-" }}</n-descriptions-item>
            <n-descriptions-item label="操作人">{{ drawerRecord.operator || "-" }}</n-descriptions-item>
            <n-descriptions-item label="工具">{{ drawerRecord.toolName || "-" }}</n-descriptions-item>
            <n-descriptions-item label="决策">
              <n-tag :type="decisionTagType(drawerRecord.decision)" size="small">{{ decisionLabel(drawerRecord.decision) }}</n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="命令"><n-code>{{ drawerRecord.command || "-" }}</n-code></n-descriptions-item>
            <n-descriptions-item label="备注">{{ drawerRecord.notes || "-" }}</n-descriptions-item>
          </n-descriptions>
        </template>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.approval-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background: linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}
.approval-header { display: flex; flex-direction: column; gap: 10px; }
.header-copy h1 { margin: 8px 0 4px; font-size: 28px; color: #0f172a; }
.header-copy p { margin: 0; color: #475569; line-height: 1.7; }
</style>
