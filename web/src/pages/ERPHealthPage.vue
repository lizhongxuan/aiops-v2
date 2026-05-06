<script setup>
import { computed, onMounted } from "vue";
import { useAppStore } from "../store";
import BusinessMetricSparkline from "../components/erp/BusinessMetricSparkline.vue";
import ERPHealthMatrix from "../components/erp/ERPHealthMatrix.vue";
import TenantImpactList from "../components/erp/TenantImpactList.vue";
import "./erpSrePages.css";

const store = useAppStore();
const capabilities = computed(() => Array.isArray(store.erp.health?.capabilities) ? store.erp.health.capabilities : []);
const metrics = computed(() => Array.isArray(store.erp.businessMetrics) ? store.erp.businessMetrics : []);
const tenants = computed(() => Array.isArray(store.erp.tenantImpact) ? store.erp.tenantImpact : []);

async function loadERPHealthContext() {
  await Promise.allSettled([
    store.loadERPHealth(),
    store.loadERPBusinessMetrics(),
    store.loadERPTenantImpact(),
  ]);
}

async function createIncidentFromHealth() {
  await store.createIncident({
    source: "erp-health",
    title: "ERP 健康异常",
    severity: "SEV2",
    environment: "prod",
    businessCapability: capabilities.value.find((item) => item.status && item.status !== "healthy")?.name || "ERP",
  });
}

onMounted(() => {
  void loadERPHealthContext();
});
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">ERP Health</p>
          <h2>ERP 健康</h2>
          <p>按业务能力、关键指标和租户影响查看 ERP 生产状态。</p>
        </div>
        <div class="erp-sre-actions">
          <button class="erp-sre-link" type="button" data-testid="erp-create-incident" @click="createIncidentFromHealth">创建事故</button>
          <RouterLink class="erp-sre-link-secondary" to="/incidents">事故工作台</RouterLink>
          <RouterLink class="erp-sre-link-secondary" to="/opsgraph">ERP 图谱</RouterLink>
          <RouterLink class="erp-sre-link-secondary" to="/coroot">Coroot 高级详情</RouterLink>
        </div>
      </header>

      <section class="erp-sre-panel">
        <h3>业务能力健康矩阵</h3>
        <ERPHealthMatrix :capabilities="capabilities" />
      </section>

      <section class="erp-sre-grid two">
        <article class="erp-sre-panel">
          <h3>关键业务指标</h3>
          <div class="erp-sre-stack">
            <BusinessMetricSparkline v-for="metric in metrics" :key="metric.id || metric.name" :metric="metric" />
            <p v-if="!metrics.length">暂无关键指标</p>
          </div>
        </article>
        <article class="erp-sre-panel">
          <h3>受影响租户</h3>
          <TenantImpactList :tenants="tenants" />
        </article>
      </section>
    </div>
  </section>
</template>

<style scoped>
.erp-sre-stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>
