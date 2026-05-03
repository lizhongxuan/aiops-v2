<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { NBadge, NButton } from "naive-ui";
import {
  ArrowLeftIcon,
  ActivityIcon,
  NetworkIcon,
  AlertTriangleIcon,
  RefreshCwIcon,
  SparklesIcon,
} from "lucide-vue-next";
import CorootEmbedPanel from "../components/coroot/CorootEmbedPanel.vue";
import MonitorAIDrawer from "../components/monitor-ai/MonitorAIDrawer.vue";
import McpUiCardHost from "../components/mcp/McpUiCardHost.vue";
import { adaptServiceStats, adaptAlerts, adaptServiceOverview } from "../lib/corootCardAdapter";
import { fetchCorootConfig as fetchCorootConfigApi, fetchCorootServices as fetchCorootServicesApi } from "../api/coroot";

const router = useRouter();

const loading = ref(false);
const services = ref([]);
const activeTab = ref("services");
const embedVisible = ref(false);
const embedUrl = ref("");
const embedTitle = ref("");
const aiDrawerVisible = ref(false);

// Coroot config state
const corootConfigured = ref(true);
const corootConfigLoading = ref(true);
const corootBaseUrl = ref("/api/v1/coroot/");

// Dashboard iframe state
const dashboardLoading = ref(true);
const dashboardError = ref(false);

function normalizeServicesPayload(payload) {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.services)) return payload.services;
  if (Array.isArray(payload?.items)) return payload.items;
  if (Array.isArray(payload?.data)) return payload.data;
  return [];
}

function normalizeServiceStatus(status = "") {
  const value = String(status || "").trim().toLowerCase();
  if (value === "healthy") return "ok";
  if (value === "error") return "critical";
  return value || "unknown";
}

const monitorContext = computed(() => ({
  source: "coroot",
  resourceType: "cluster",
  resourceId: "overview",
  timeRange: "latest",
  alerts: services.value
    .filter((s) => s.status === "critical" || s.status === "error" || s.status === "warning")
    .map((s) => ({ id: s.id, name: s.name, status: s.status })),
}));

// MCP UI card payloads derived from services data
const kpiCard = computed(() => adaptServiceStats(services.value));
const statusTableCard = computed(() => adaptAlerts(
  services.value
    .filter((s) => s.status === "critical" || s.status === "error" || s.status === "warning")
    .map((s) => ({ id: s.id, name: s.name, severity: s.status, status: s.status }))
));
const summaryCard = computed(() => {
  if (!services.value.length) return null;
  // Build a summary from the first service or an aggregate overview
  return adaptServiceOverview({
    id: "cluster-overview",
    name: "集群",
    status: services.value.some((s) => s.status === "critical" || s.status === "error")
      ? "critical"
      : services.value.some((s) => s.status === "warning")
        ? "warning"
        : "ok",
    summary: {
      "总服务数": String(services.value.length),
    },
  });
});

async function fetchCorootConfig() {
  corootConfigLoading.value = true;
  try {
    const data = await fetchCorootConfigApi();
    corootConfigured.value = !!data.configured;
    if (data.baseUrl) {
      corootBaseUrl.value = data.baseUrl;
    }
  } catch {
    corootConfigured.value = false;
  } finally {
    corootConfigLoading.value = false;
  }
}

async function fetchServices() {
  loading.value = true;
  try {
    const data = await fetchCorootServicesApi();
    services.value = normalizeServicesPayload(data);
  } catch {
    services.value = [];
  } finally {
    loading.value = false;
  }
}

const healthyCount = computed(() => services.value.filter((s) => normalizeServiceStatus(s.status) === "ok").length);
const warningCount = computed(() => services.value.filter((s) => normalizeServiceStatus(s.status) === "warning").length);
const criticalCount = computed(() => services.value.filter((s) => normalizeServiceStatus(s.status) === "critical").length);
const unknownCount = computed(() => Math.max(services.value.length - healthyCount.value - warningCount.value - criticalCount.value, 0));
const configStateLabel = computed(() => {
  if (corootConfigLoading.value) return "检测中";
  return corootConfigured.value ? "已连接" : "未配置";
});
const configStateClass = computed(() => {
  if (corootConfigLoading.value) return "state-loading";
  return corootConfigured.value ? "state-ok" : "state-warn";
});
const serviceTableColumns = computed(() => [
  { title: "ID", key: "id", ellipsis: { tooltip: true } },
  { title: "名称", key: "name", ellipsis: { tooltip: true } },
  {
    title: "状态",
    key: "status",
    render: (row) => h(NBadge, {
      type: normalizeServiceStatus(row.status) === "ok"
        ? "success"
        : normalizeServiceStatus(row.status) === "warning"
          ? "warning"
          : normalizeServiceStatus(row.status) === "critical"
            ? "error"
            : "default",
      value: row.status || "unknown",
      processing: normalizeServiceStatus(row.status) === "warning" || normalizeServiceStatus(row.status) === "critical",
    }),
  },
  {
    title: "操作",
    key: "actions",
    width: 96,
    render: (row) => h(NButton, { size: "small", quaternary: true, onClick: () => openServiceEmbed(row) }, { default: () => "详情" }),
  },
]);

function openServiceEmbed(service) {
  embedTitle.value = service.name || service.id;
  embedUrl.value = `/api/v1/coroot/api/v1/services/${encodeURIComponent(service.id)}/overview`;
  embedVisible.value = true;
}

function closeEmbed() {
  embedVisible.value = false;
  embedUrl.value = "";
  embedTitle.value = "";
}

function goBack() {
  router.push("/");
}

function onDashboardIframeLoad() {
  dashboardLoading.value = false;
  dashboardError.value = false;
}

function onDashboardIframeError() {
  dashboardLoading.value = false;
  dashboardError.value = true;
}

function openDashboardTab() {
  activeTab.value = "dashboard";
  dashboardError.value = false;
}

function openTopologyPanel() {
  embedTitle.value = "服务拓扑";
  embedUrl.value = "/api/v1/coroot/api/v1/topology";
  embedVisible.value = true;
}

let previousTitle = "";

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "Coroot 监控总览";
  void fetchCorootConfig();
  void fetchServices();
});

onBeforeUnmount(() => {
  if (previousTitle) document.title = previousTitle;
});
</script>

<template>
  <section class="coroot-page">
    <header class="coroot-header">
      <div class="coroot-title-block">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon :size="16" />
          <span>返回首页</span>
        </button>
        <div class="title-row">
          <div>
            <div class="coroot-kicker">Monitoring / Coroot</div>
            <h1>Coroot 监控总览</h1>
          </div>
          <span class="config-state" :class="configStateClass">{{ configStateLabel }}</span>
        </div>
        <p>服务健康、拓扑、告警和完整 Coroot Dashboard 的统一入口。</p>
      </div>
      <div class="coroot-actions">
        <n-button size="small" @click="fetchServices" :disabled="loading || !corootConfigured">
          <template #icon><RefreshCwIcon :size="14" :class="{ spinning: loading }" /></template>
          刷新
        </n-button>
        <n-button size="small" @click="openDashboardTab" :disabled="!corootConfigured">
          <template #icon><ActivityIcon :size="14" /></template>
          Dashboard
        </n-button>
        <n-button size="small" type="primary" @click="aiDrawerVisible = true">
          <template #icon><SparklesIcon :size="14" /></template>
          AI 助手
        </n-button>
      </div>
    </header>

    <section class="status-strip" aria-label="Coroot 服务状态">
      <div class="status-cell">
        <n-statistic label="健康" :value="healthyCount">
          <template #prefix><ActivityIcon :size="18" style="color:#16a34a" /></template>
        </n-statistic>
      </div>
      <div class="status-cell">
        <n-statistic label="告警" :value="warningCount">
          <template #prefix><AlertTriangleIcon :size="18" style="color:#d97706" /></template>
        </n-statistic>
      </div>
      <div class="status-cell">
        <n-statistic label="异常" :value="criticalCount">
          <template #prefix><AlertTriangleIcon :size="18" style="color:#dc2626" /></template>
        </n-statistic>
      </div>
      <div class="status-cell">
        <n-statistic label="未知" :value="unknownCount" />
      </div>
    </section>

    <!-- Degraded state: Coroot not configured -->
    <div v-if="!corootConfigLoading && !corootConfigured" class="config-warning" data-testid="coroot-not-configured">
      <AlertTriangleIcon :size="20" />
      <div>
        <strong>Coroot 未配置</strong>
        <p>请先在系统设置中配置 Coroot 连接信息，才能使用监控 Dashboard 功能。</p>
      </div>
    </div>

    <template v-if="corootConfigured || corootConfigLoading">
      <div v-if="loading" class="loading-hint">加载中…</div>

      <n-tabs v-model:value="activeTab" type="line" data-testid="coroot-tab-bar">
        <n-tab-pane name="services" tab="服务总览" data-testid="tab-content-services">
          <div class="cards-grid">
            <McpUiCardHost v-if="kpiCard" :card="kpiCard" />
            <McpUiCardHost v-if="statusTableCard" :card="statusTableCard" />
            <McpUiCardHost v-if="summaryCard" :card="summaryCard" />
          </div>

          <n-card>
            <template #header>服务列表</template>
            <n-data-table
              v-if="services.length"
              :columns="serviceTableColumns"
              :data="services"
              :row-key="(row) => row.id"
              :bordered="false"
              size="small"
            />
            <n-empty v-else-if="!loading" description="暂无 Coroot 服务数据。请确认 Coroot 已配置。" />
          </n-card>
        </n-tab-pane>

        <n-tab-pane name="dashboard" tab="Dashboard" data-testid="tab-content-dashboard">
          <div class="dashboard-container">
            <div v-if="dashboardLoading && !dashboardError" class="dashboard-loading">
              <n-spin size="medium" />
              <span>Dashboard 加载中…</span>
            </div>
            <div v-if="dashboardError" class="dashboard-error">
              <AlertTriangleIcon :size="18" />
              Dashboard 加载失败，请检查 Coroot 连接
            </div>
            <iframe
              v-show="!dashboardError"
              :src="corootBaseUrl"
              class="dashboard-iframe"
              sandbox="allow-scripts allow-same-origin allow-forms"
              referrerpolicy="no-referrer"
              data-testid="dashboard-iframe"
              @load="onDashboardIframeLoad"
              @error="onDashboardIframeError"
            />
          </div>
        </n-tab-pane>

        <n-tab-pane name="topology" tab="拓扑视图" data-testid="tab-content-topology">
          <n-card>
            <template #header>
              <NetworkIcon :size="18" style="vertical-align: middle; margin-right: 6px;" />
              服务拓扑
            </template>
            <p class="topology-hint">在侧边面板查看 Coroot 拓扑数据，适合和当前服务列表并行对照。</p>
            <n-button @click="openTopologyPanel">
              打开拓扑视图
            </n-button>
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </template>

    <!-- Embed Panel (overlay for service detail / topology) -->
    <CorootEmbedPanel
      v-if="embedVisible"
      :title="embedTitle"
      :url="embedUrl"
      @close="closeEmbed"
    />

    <!-- Monitor AI Drawer -->
    <n-drawer v-model:show="aiDrawerVisible" :width="400" placement="right">
      <n-drawer-content title="AI 助手" closable>
        <MonitorAIDrawer
          v-if="aiDrawerVisible"
          :monitorContext="monitorContext"
          @close="aiDrawerVisible = false"
        />
      </n-drawer-content>
    </n-drawer>
  </section>
</template>

<style scoped>
.coroot-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: #f6f8fb;
}

.coroot-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  padding: 18px 20px;
  border-radius: 10px;
  background: #fff;
  border: 1px solid rgba(226, 232, 240, 0.9);
}

.coroot-title-block {
  min-width: 0;
  flex: 1;
}

.title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-top: 8px;
}

.coroot-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: #475569;
  font: inherit;
  cursor: pointer;
}

.coroot-kicker {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.coroot-header h1 {
  margin: 4px 0 0;
  font-size: 24px;
  line-height: 1.2;
}
.coroot-header p { margin: 8px 0 0; color: #475569; line-height: 1.6; }

.config-state {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}
.state-ok { background: #dcfce7; color: #166534; }
.state-warn { background: #fef3c7; color: #92400e; }
.state-loading { background: #e0f2fe; color: #075985; }

.status-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  background: #fff;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 10px;
  overflow: hidden;
}

.status-cell {
  min-width: 0;
  padding: 14px 16px;
  border-right: 1px solid rgba(226, 232, 240, 0.9);
}
.status-cell:last-child {
  border-right: 0;
}

.config-warning {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 10px;
  background: #fef3c7;
  border: 1px solid #fcd34d;
  color: #92400e;
}
.config-warning strong { display: block; margin-bottom: 4px; }
.config-warning p { margin: 0; font-size: 13px; line-height: 1.6; }

.tab-bar {
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  width: fit-content;
  align-items: center;
}

.tab-bar button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 10px 20px;
  border: none;
  border-radius: 10px;
  background: transparent;
  font: inherit;
  cursor: pointer;
  color: #475569;
  font-weight: 500;
}

.tab-bar button.active {
  background: #0f172a;
  color: #fff;
}

.refresh-btn {
  margin-left: 8px;
  font-size: 13px;
}

.ai-btn {
  margin-left: 4px;
  padding: 10px 16px;
  background: #2563eb;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}
.ai-btn:hover { background: #1d4ed8; }

.spinning { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

.loading-hint { color: #64748b; font-size: 14px; }

.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.topology-hint { color: #64748b; font-size: 13px; margin: 0 0 12px; }

/* Dashboard Tab - inline iframe container */
.dashboard-container {
  position: relative;
  border-radius: 10px;
  background: #fff;
  border: 1px solid rgba(226, 232, 240, 0.9);
  overflow: hidden;
  min-height: 600px;
}

.dashboard-iframe {
  width: 100%;
  height: 80vh;
  min-height: 600px;
  border: none;
  display: block;
}

.dashboard-loading {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #64748b;
  font-size: 14px;
  background: #fff;
  z-index: 1;
}

.spinner {
  display: inline-block;
  width: 24px;
  height: 24px;
  border: 3px solid rgba(100, 116, 139, 0.2);
  border-top-color: #64748b;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.dashboard-error {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #dc2626;
  font-size: 13px;
  padding: 12px;
  margin: 18px;
  background: #fee2e2;
  border-radius: 8px;
}

@media (max-width: 760px) {
  .coroot-page { padding: 16px; }
  .coroot-header { flex-direction: column; }
  .title-row { flex-direction: column; }
  .status-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .status-cell:nth-child(2) { border-right: 0; }
  .status-cell:nth-child(-n + 2) { border-bottom: 1px solid rgba(226, 232, 240, 0.9); }
  .cards-grid { grid-template-columns: 1fr; }
}
</style>
