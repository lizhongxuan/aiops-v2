<script setup>
import { computed } from "vue";

const props = defineProps({
  neighbors: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => props.neighbors.filter(Boolean));
</script>

<template>
  <section class="opsgraph-neighbor-list" data-testid="opsgraph-neighbor-list">
    <div v-if="!rows.length" class="opsgraph-empty">暂无邻域</div>
    <article v-for="item in rows" :key="item.id || item.name">
      <strong>{{ item.name || item.id }}</strong>
      <span>{{ item.relation || item.type || "neighbor" }} · {{ item.status || "unknown" }}</span>
    </article>
  </section>
</template>

<style scoped>
.opsgraph-neighbor-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.opsgraph-neighbor-list article,
.opsgraph-empty {
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.opsgraph-neighbor-list strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.opsgraph-neighbor-list span,
.opsgraph-empty {
  color: #64748b;
  font-size: 12px;
}
</style>
