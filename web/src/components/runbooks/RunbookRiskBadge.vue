<script setup>
import { computed } from "vue";

const props = defineProps({
  risk: {
    type: String,
    default: "",
  },
});

const label = computed(() => String(props.risk || "unknown").trim() || "unknown");
const tone = computed(() => {
  const value = label.value.toLowerCase();
  if (["high", "critical"].includes(value)) return "high";
  if (["medium"].includes(value)) return "medium";
  if (["low"].includes(value)) return "low";
  return "neutral";
});
</script>

<template>
  <span class="runbook-risk-badge" :class="`is-${tone}`" data-testid="runbook-risk-badge">{{ label }}</span>
</template>

<style scoped>
.runbook-risk-badge {
  display: inline-flex;
  align-items: center;
  min-height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 800;
}

.runbook-risk-badge.is-high {
  background: #fee2e2;
  color: #991b1b;
}

.runbook-risk-badge.is-medium {
  background: #fef3c7;
  color: #92400e;
}

.runbook-risk-badge.is-low {
  background: #dcfce7;
  color: #166534;
}
</style>
