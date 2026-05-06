<script setup>
import { computed, onMounted } from "vue";
import { useAppStore } from "../store";
import IncidentSeverityBadge from "../components/incidents/IncidentSeverityBadge.vue";
import IncidentStatusBadge from "../components/incidents/IncidentStatusBadge.vue";
import "./erpSrePages.css";

const store = useAppStore();
const incidents = computed(() => Array.isArray(store.incidents.items) ? store.incidents.items : []);

onMounted(() => {
  void store.loadIncidents({ status: "active" });
});
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Incident Operations</p>
          <h2>事故工作台</h2>
          <p>集中查看 ERP 生产事故、业务影响和待处理 Runbook。</p>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link" to="/erp">查看 ERP 健康</RouterLink>
          <RouterLink class="erp-sre-link-secondary" to="/runbooks">Runbook</RouterLink>
        </div>
      </header>

      <section class="erp-sre-grid">
        <article class="erp-sre-panel">
          <h3>活跃事故</h3>
          <p>{{ incidents.length }} 个待处理</p>
        </article>
        <article class="erp-sre-panel">
          <h3>业务影响</h3>
          <p>按租户、业务能力和订单链路汇总。</p>
        </article>
        <article class="erp-sre-panel">
          <h3>待审批动作</h3>
          <p>统一从运行态审批流读取，不在页面内创建第二套审批。</p>
        </article>
      </section>

      <table class="erp-sre-table" aria-label="事故列表">
        <thead>
          <tr>
            <th>事故</th>
            <th>SEV</th>
            <th>状态</th>
            <th>业务能力</th>
            <th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!incidents.length">
            <td colspan="5">
              <div class="erp-sre-empty">
                <strong>暂无事故</strong>
                <p>当 Coroot webhook 或人工创建事故后，会出现在这里。</p>
              </div>
            </td>
          </tr>
          <tr v-for="incident in incidents" :key="incident.id">
            <td>
              <RouterLink :to="`/incidents/${encodeURIComponent(incident.id)}`" class="incident-list-link">
                {{ incident.title || incident.name || incident.id }}
              </RouterLink>
            </td>
            <td><IncidentSeverityBadge :severity="incident.severity || incident.sev" /></td>
            <td><IncidentStatusBadge :status="incident.status" /></td>
            <td>{{ incident.businessCapability || incident.capability || "-" }}</td>
            <td>{{ incident.updatedAt || incident.createdAt || "-" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.incident-list-link {
  color: #14532d;
  font-weight: 800;
  text-decoration: none;
}
</style>
