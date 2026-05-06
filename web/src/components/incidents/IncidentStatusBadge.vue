<script setup>
import { computed } from "vue";

const props = defineProps({
  status: {
    type: String,
    default: "",
  },
});

const normalized = computed(() => String(props.status || "unknown").trim() || "unknown");
const tone = computed(() => {
  const value = normalized.value.toLowerCase();
  if (["resolved", "closed", "completed"].includes(value)) return "success";
  if (["mitigated", "monitoring"].includes(value)) return "info";
  if (["investigating", "active", "triggered", "open"].includes(value)) return "warning";
  return "neutral";
});
</script>

<template>
  <span class="incident-status-badge" :class="`is-${tone}`" data-testid="incident-status-badge">
    {{ normalized }}
  </span>
</template>

<style scoped>
.incident-status-badge {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 800;
}

.incident-status-badge.is-success {
  background: #dcfce7;
  color: #166534;
}

.incident-status-badge.is-info {
  background: #dbeafe;
  color: #1d4ed8;
}

.incident-status-badge.is-warning {
  background: #fef3c7;
  color: #92400e;
}
</style>
