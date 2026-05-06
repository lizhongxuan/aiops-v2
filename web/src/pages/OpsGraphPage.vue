<script setup>
import { computed, ref } from "vue";
import { useAppStore } from "../store";
import BusinessImpactPanel from "../components/opsgraph/BusinessImpactPanel.vue";
import OpsGraphEntityCard from "../components/opsgraph/OpsGraphEntityCard.vue";
import OpsGraphMiniMap from "../components/opsgraph/OpsGraphMiniMap.vue";
import OpsGraphNeighborList from "../components/opsgraph/OpsGraphNeighborList.vue";
import "./erpSrePages.css";

const store = useAppStore();
const query = ref("");
const selectedEntity = computed(() =>
  store.opsgraph.neighborhood?.entity ||
  (Array.isArray(store.opsgraph.lookup) ? store.opsgraph.lookup[0] : null) ||
  {},
);
const lookupRows = computed(() => Array.isArray(store.opsgraph.lookup) ? store.opsgraph.lookup : []);
const neighbors = computed(() => Array.isArray(store.opsgraph.neighborhood?.neighbors) ? store.opsgraph.neighborhood.neighbors : []);
const businessImpact = computed(() => store.opsgraph.businessImpact || {});

async function searchEntity() {
  const text = query.value.trim();
  if (!text) return;
  const result = await store.lookupOpsGraph({ query: text });
  const entity = result?.matches?.[0] || store.opsgraph.lookup?.[0] || selectedEntity.value;
  const entityId = entity?.id || entity?.name || text;
  await Promise.allSettled([
    store.loadOpsGraphNeighborhood(entityId),
    store.loadOpsGraphBusinessImpact(entityId),
  ]);
}
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Ops Graph</p>
          <h2>ERP 图谱</h2>
          <p>以实体搜索、邻域和业务影响为主，不做复杂全屏图谱。</p>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link-secondary" to="/erp">ERP 健康</RouterLink>
          <RouterLink class="erp-sre-link" to="/incidents">事故工作台</RouterLink>
        </div>
      </header>

      <section class="erp-sre-panel">
        <h3>实体搜索</h3>
        <div class="opsgraph-search">
          <input v-model="query" data-testid="opsgraph-search-input" placeholder="service / tenant / db / job" />
          <button type="button" data-testid="opsgraph-search-button" @click="searchEntity">搜索</button>
        </div>
        <div class="opsgraph-lookup">
          <span v-for="item in lookupRows" :key="item.id || item.name">{{ item.name || item.id }}</span>
        </div>
      </section>

      <section class="erp-sre-layout">
        <main class="erp-sre-panel">
          <h3>邻域概览</h3>
          <OpsGraphEntityCard :entity="selectedEntity" />
          <OpsGraphMiniMap :entity="selectedEntity" :neighbors="neighbors" />
        </main>
        <aside class="erp-sre-grid">
          <article class="erp-sre-panel">
            <h3>邻域列表</h3>
            <OpsGraphNeighborList :neighbors="neighbors" />
          </article>
          <article class="erp-sre-panel">
            <h3>业务影响</h3>
            <BusinessImpactPanel :impact="businessImpact" />
          </article>
        </aside>
      </section>
    </div>
  </section>
</template>

<style scoped>
.opsgraph-search {
  display: flex;
  gap: 8px;
}

.opsgraph-search input {
  flex: 1;
  min-width: 0;
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
}

.opsgraph-search button {
  min-height: 36px;
  padding: 0 14px;
  border: 1px solid #15803d;
  border-radius: 8px;
  background: #dcfce7;
  color: #14532d;
  font-weight: 800;
}

.opsgraph-lookup {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.opsgraph-lookup span {
  padding: 4px 8px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
}
</style>
