<script setup>
defineProps({
  entity: {
    type: Object,
    default: () => ({}),
  },
});
</script>

<template>
  <article class="opsgraph-entity-card" data-testid="opsgraph-entity-card">
    <span>{{ entity.type || "entity" }}</span>
    <strong>{{ entity.name || entity.id || "未选择实体" }}</strong>
    <p v-if="entity.status">{{ entity.status }}</p>
  </article>
</template>

<style scoped>
.opsgraph-entity-card {
  padding: 14px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.opsgraph-entity-card span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.opsgraph-entity-card strong {
  display: block;
  margin-top: 5px;
  color: #0f172a;
  font-size: 18px;
}

.opsgraph-entity-card p {
  margin: 6px 0 0;
  color: #64748b;
}
</style>
