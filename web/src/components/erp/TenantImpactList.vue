<script setup>
import { computed } from "vue";

const props = defineProps({
  tenants: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => props.tenants.filter(Boolean));
</script>

<template>
  <section class="tenant-impact-list" data-testid="tenant-impact-list">
    <div v-if="!rows.length" class="tenant-impact-empty">暂无租户影响</div>
    <article v-for="item in rows" :key="item.id || item.name">
      <strong>{{ item.name || item.id }}</strong>
      <span>{{ item.impact || item.severity || item.status || "impact" }}</span>
    </article>
  </section>
</template>

<style scoped>
.tenant-impact-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tenant-impact-list article,
.tenant-impact-empty {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.tenant-impact-list strong {
  color: #0f172a;
  font-size: 13px;
}

.tenant-impact-list span,
.tenant-impact-empty {
  color: #64748b;
  font-size: 12px;
}
</style>
