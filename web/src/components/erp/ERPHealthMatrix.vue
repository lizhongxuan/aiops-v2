<script setup>
import { computed } from "vue";

const props = defineProps({
  capabilities: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => props.capabilities.filter(Boolean));
</script>

<template>
  <section class="erp-health-matrix" data-testid="erp-health-matrix">
    <div v-if="!rows.length" class="erp-matrix-empty">暂无健康矩阵</div>
    <article v-for="item in rows" :key="item.id || item.name" :class="`is-${item.status || 'unknown'}`">
      <strong>{{ item.name || item.id }}</strong>
      <span>{{ item.status || "unknown" }}</span>
      <p v-if="item.summary">{{ item.summary }}</p>
    </article>
  </section>
</template>

<style scoped>
.erp-health-matrix {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}

.erp-health-matrix article,
.erp-matrix-empty {
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.erp-health-matrix article.is-degraded {
  border-color: #fbbf24;
  background: #fffbeb;
}

.erp-health-matrix article.is-healthy {
  border-color: #bbf7d0;
  background: #f0fdf4;
}

.erp-health-matrix strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.erp-health-matrix span,
.erp-matrix-empty {
  color: #64748b;
  font-size: 12px;
}

.erp-health-matrix p {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 12px;
}
</style>
