<script setup>
import { computed } from "vue";

const props = defineProps({
  severity: {
    type: String,
    default: "",
  },
});

const normalized = computed(() => String(props.severity || "unknown").trim() || "unknown");
const tone = computed(() => {
  const value = normalized.value.toLowerCase();
  if (["sev0", "sev1", "critical", "p0", "p1"].includes(value)) return "critical";
  if (["sev2", "high", "p2"].includes(value)) return "high";
  if (["sev3", "medium", "p3"].includes(value)) return "medium";
  return "neutral";
});
</script>

<template>
  <span class="incident-severity-badge" :class="`is-${tone}`" data-testid="incident-severity-badge">
    {{ normalized }}
  </span>
</template>

<style scoped>
.incident-severity-badge {
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

.incident-severity-badge.is-critical {
  background: #fee2e2;
  color: #991b1b;
}

.incident-severity-badge.is-high {
  background: #ffedd5;
  color: #9a3412;
}

.incident-severity-badge.is-medium {
  background: #fef3c7;
  color: #92400e;
}
</style>
