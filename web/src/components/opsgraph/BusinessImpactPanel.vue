<script setup>
import { computed } from "vue";

const props = defineProps({
  impact: {
    type: Object,
    default: () => ({}),
  },
});

const capabilities = computed(() => Array.isArray(props.impact.capabilities) ? props.impact.capabilities : []);
const tenants = computed(() => Array.isArray(props.impact.tenants) ? props.impact.tenants : []);
</script>

<template>
  <section class="business-impact-panel" data-testid="business-impact-panel">
    <div v-if="!capabilities.length && !tenants.length" class="impact-empty">暂无业务影响</div>
    <article v-for="item in capabilities" :key="`cap-${item.id || item.name}`">
      <strong>{{ item.name || item.id }}</strong>
      <span>{{ item.impact || item.status || "capability" }}</span>
    </article>
    <article v-for="item in tenants" :key="`tenant-${item.id || item.name}`">
      <strong>{{ item.name || item.id }}</strong>
      <span>{{ item.impact || item.status || "tenant" }}</span>
    </article>
  </section>
</template>

<style scoped>
.business-impact-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.business-impact-panel article,
.impact-empty {
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.business-impact-panel strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.business-impact-panel span,
.impact-empty {
  color: #64748b;
  font-size: 12px;
}
</style>
