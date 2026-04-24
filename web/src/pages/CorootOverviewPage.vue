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
  LayoutDashboardIcon,
  ListIcon,
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
    services.value = Array.isArray(data) ? data : [];
  } catch {
    services.value = [];
  } finally {
    loading.value = false;
  }
}

const healthyCount = computed(() => services.value.filter((s) => s.status === "ok" || s.status === "healthy").length);
const warningCount = computed(() => services.value.filter((s) => s.status === "warning").length);
const criticalCount = computed(() => services.value.filter((s) => s.status === "critical" || s.status === "error").length);

function statusBadgeClass(status) {
  if (status === "ok" || status === "healthy") return "badge-ok";
  if (status === "warning") return "badge-warn";
  if (status === "critical" || status === "error") return "badge-crit";
  return "badge-unknown";
}

function openServiceEmbed(service) {
  embedTitle.value = service.name || service.id;
  embedUrl.value = `/api/v1/coroot/api/v1/services/${service.id}/overview`;
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
    <header class="coroot-hero">
      <div class="coroot-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon :size="16" />
          <span>返回首页</span>
        </button>
        <div class="coroot-kicker">Monitoring / Coroot</div>
        <h1>Coroot 监控总览</h1>
        <p>查看 Coroot 监控的服务健康状态、拓扑和告警。</p>
      </div>
      <n-grid :cols="3" :x-gap="12">
        <n-gi>
          <n-card size="small">
            <n-statistic label="健康" :value="healthyCount">
              <template #prefix><ActivityIcon :size="18" style="color:#16a34a" /></template>
            </n-statistic>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small">
            <n-statistic label="告警" :value="warningCount">
              <template #prefix><AlertTriangleIcon :size="18" style="color:#d97706" /></template>
            </n-statistic>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small">
            <n-statistic label="异常" :value="criticalCount">
              <template #prefix><AlertTriangleIcon :size="18" style="color:#dc2626" /></template>
            </n-statistic>
          </n-card>
        </n-gi>
      </n-grid>
    </header>

    <!-- Degraded state: Coroot not configured -->
    <div v-if="!corootConfigLoading && !corootConfigured" class="config-warning" data-testid="coroot-not-configured">
      <AlertTriangleIcon :size="20" />
      <div>
        <strong>Coroot 未配置</strong>
        <p>请先在系统设置中配置 Coroot 连接信息，才能使用监控 Dashboard 功能。</p>
      </div>
    </div>

    <template v-if="corootConfigured || corootConfigLoading">
      <n-space align="center" style="margin-bottom:12px;">
        <n-button size="small" @click="fetchServices" :disabled="loading">
          <template #icon><RefreshCwIcon :size="14" :class="{ spinning: loading }" /></template>
          刷新
        </n-button>
        <n-button size="small" type="primary" @click="aiDrawerVisible = true">
          <template #icon><SparklesIcon :size="14" /></template>
          AI 助手
        </n-button>
      </n-space>

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
              :columns="[
                { title: 'ID', key: 'id' },
                { title: '名称', key: 'name' },
                { title: '状态', key: 'status', render: (row) => h(NBadge, { type: row.status === 'ok' || row.status === 'healthy' ? 'success' : row.status === 'warning' ? 'warning' : row.status === 'critical' || row.status === 'error' ? 'error' : 'default', value: row.status || 'unknown', processing: row.status === 'warning' || row.status === 'critical' }) },
                { title: '操作', key: 'actions', render: (row) => h(NButton, { size: 'small', quaternary: true, onClick: () => openServiceEmbed(row) }, { default: () => '详情' }) },
              ]"
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
            <p class="topology-hint">拓扑视图通过 Coroot 嵌入面板展示。点击下方按钮打开。</p>
            <n-button @click="embedTitle = '服务拓扑'; embedUrl = '/api/v1/coroot/api/v1/topology'; embedVisible = true;">
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
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.coroot-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.05);
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
  display: inline-flex;
  margin-top: 12px;
  padding: 6px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.coroot-hero h1 { margin: 12px 0 8px; font-size: 30px; }
.coroot-hero p { margin: 0; color: #475569; line-height: 1.7; }

.coroot-hero-stats {
  display: flex;
  gap: 12px;
  align-self: end;
}

.coroot-stat {
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.9);
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 80px;
}
.coroot-stat span { color: #64748b; font-size: 12px; margin-top: 4px; }
.coroot-stat strong { font-size: 20px; color: #0f172a; margin-top: 4px; }
.stat-ok strong { color: #16a34a; }
.stat-warn strong { color: #d97706; }
.stat-crit strong { color: #dc2626; }

.config-warning {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 18px 22px;
  border-radius: 16px;
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
  gap: 14px;
  margin-bottom: 18px;
}

.section-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.section-card h2 { margin: 0 0 14px; font-size: 18px; }

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  font-size: 13px;
}

.data-table th {
  color: #64748b;
  font-weight: 600;
  font-size: 12px;
}

.data-table tbody tr:hover { background: #f8fafc; }

.status-badge {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}
.badge-ok { background: #dcfce7; color: #16a34a; }
.badge-warn { background: #fef3c7; color: #d97706; }
.badge-crit { background: #fee2e2; color: #dc2626; }
.badge-unknown { background: #f1f5f9; color: #64748b; }

.action-btn {
  padding: 6px 14px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 8px;
  background: #fff;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  color: #1d4ed8;
}
.action-btn:hover { background: #eff6ff; }

.topology-hint { color: #64748b; font-size: 13px; margin: 0 0 12px; }
.empty-hint { color: #64748b; font-size: 13px; margin: 14px 0 0; }

/* Dashboard Tab - inline iframe container */
.dashboard-container {
  position: relative;
  border-radius: 20px;
  background: #fff;
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
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
  .coroot-hero { flex-direction: column; }
  .coroot-hero-stats { flex-wrap: wrap; }
  .cards-grid { grid-template-columns: 1fr; }
}
</style>
