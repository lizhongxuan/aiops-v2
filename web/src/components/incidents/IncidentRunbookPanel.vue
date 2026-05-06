<script setup>
import { computed } from "vue";

const props = defineProps({
  matches: {
    type: Array,
    default: () => [],
  },
  instances: {
    type: Array,
    default: () => [],
  },
});

const matchRows = computed(() => props.matches.filter(Boolean));
const instanceRows = computed(() => props.instances.filter(Boolean));
</script>

<template>
  <section class="incident-runbook-panel" data-testid="incident-runbook-panel">
    <div v-if="!matchRows.length && !instanceRows.length" class="incident-runbook-empty">暂无 Runbook 匹配</div>
    <article v-for="item in matchRows" :key="`match-${item.id || item.runbookId || item.title}`" class="incident-runbook-item">
      <strong>{{ item.title || item.name || item.id || item.runbookId }}</strong>
      <span>{{ item.status || "matched" }} · {{ item.risk || "risk 待定" }}</span>
    </article>
    <article v-for="item in instanceRows" :key="`instance-${item.id}`" class="incident-runbook-item">
      <strong>{{ item.runbookId || item.title || item.id }}</strong>
      <span>{{ item.status || "unknown" }}<template v-if="item.currentStep"> · {{ item.currentStep }}</template></span>
    </article>
  </section>
</template>

<style scoped>
.incident-runbook-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.incident-runbook-empty,
.incident-runbook-item {
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.incident-runbook-empty {
  color: #64748b;
}

.incident-runbook-item strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.incident-runbook-item span {
  display: block;
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
}
</style>
