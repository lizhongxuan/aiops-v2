<script setup>
import { computed, onMounted } from "vue";
import { useAppStore } from "../store";
import RunbookRiskBadge from "../components/runbooks/RunbookRiskBadge.vue";
import "./erpSrePages.css";

const store = useAppStore();
const runbooks = computed(() => Array.isArray(store.runbooks.items) ? store.runbooks.items : []);

onMounted(() => {
  void store.loadRunbooks();
});
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Runbook Catalog</p>
          <h2>Runbook</h2>
          <p>管理可计划、可审计、需审批的生产操作方案。</p>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link-secondary" to="/incidents">事故工作台</RouterLink>
        </div>
      </header>

      <table class="erp-sre-table" aria-label="Runbook 列表">
        <thead>
          <tr>
            <th>Runbook</th>
            <th>Scope</th>
            <th>Risk</th>
            <th>关联能力</th>
            <th>最后更新</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!runbooks.length">
            <td colspan="6">
              <div class="erp-sre-empty">
                <strong>暂无 Runbook</strong>
                <p>这里只展示目录和匹配信息，动作提案由事故上下文触发。</p>
              </div>
            </td>
          </tr>
          <tr v-for="item in runbooks" :key="item.id || item.title">
            <td>{{ item.title || item.name || item.id }}</td>
            <td>{{ item.scope || item.environment || "-" }}</td>
            <td><RunbookRiskBadge :risk="item.risk" /></td>
            <td>{{ Array.isArray(item.capabilities) ? item.capabilities.join(", ") : item.capability || "-" }}</td>
            <td>{{ item.updatedAt || item.updated_at || "-" }}</td>
            <td><RouterLink class="erp-sre-link-secondary" :to="`/runbooks/${encodeURIComponent(item.id)}`">查看</RouterLink></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
