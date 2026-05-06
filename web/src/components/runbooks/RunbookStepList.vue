<script setup>
import { computed } from "vue";

const props = defineProps({
  steps: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => props.steps.filter(Boolean));
</script>

<template>
  <ol class="runbook-step-list" data-testid="runbook-step-list">
    <li v-if="!rows.length">暂无步骤</li>
    <li v-for="step in rows" :key="step.id || step.title || step.name">
      <strong>{{ step.title || step.name || step.id }}</strong>
      <span>{{ step.action || step.tool || step.type || "step" }}</span>
    </li>
  </ol>
</template>

<style scoped>
.runbook-step-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.runbook-step-list li {
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.runbook-step-list strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.runbook-step-list span {
  color: #64748b;
  font-size: 12px;
}
</style>
