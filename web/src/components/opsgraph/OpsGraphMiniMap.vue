<script setup>
import { computed } from "vue";

const props = defineProps({
  entity: {
    type: Object,
    default: () => ({}),
  },
  neighbors: {
    type: Array,
    default: () => [],
  },
});

const visibleNeighbors = computed(() => props.neighbors.filter(Boolean).slice(0, 6));
</script>

<template>
  <section class="opsgraph-minimap" data-testid="opsgraph-minimap">
    <div class="opsgraph-node is-primary">{{ entity.name || entity.id || "实体" }}</div>
    <div class="opsgraph-edges">
      <div v-for="item in visibleNeighbors" :key="item.id || item.name" class="opsgraph-node">
        <span>{{ item.relation || "linked" }}</span>
        <strong>{{ item.name || item.id }}</strong>
      </div>
      <div v-if="!visibleNeighbors.length" class="opsgraph-empty">暂无邻域</div>
    </div>
  </section>
</template>

<style scoped>
.opsgraph-minimap {
  display: grid;
  grid-template-columns: minmax(140px, 0.7fr) minmax(0, 1fr);
  gap: 12px;
  align-items: stretch;
}

.opsgraph-node,
.opsgraph-empty {
  min-width: 0;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.opsgraph-node.is-primary {
  display: flex;
  align-items: center;
  justify-content: center;
  border-color: #86efac;
  background: #f0fdf4;
  color: #14532d;
  font-weight: 800;
}

.opsgraph-edges {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.opsgraph-node span {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.opsgraph-node strong {
  display: block;
  margin-top: 4px;
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.opsgraph-empty {
  color: #64748b;
}

@media (max-width: 720px) {
  .opsgraph-minimap,
  .opsgraph-edges {
    grid-template-columns: 1fr;
  }
}
</style>
