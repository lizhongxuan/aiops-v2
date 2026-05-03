<script setup>
import { computed } from "vue";
import { AlertTriangleIcon, InfoIcon } from "lucide-vue-next";

const props = defineProps({
  warnings: { type: Array, default: () => [] },
});

const emit = defineEmits(["select-target"]);
const visibleWarnings = computed(() => props.warnings || []);

function severityClass(severity = "") {
  return `is-${severity || "info"}`;
}

function iconFor(severity = "") {
  return severity === "info" ? InfoIcon : AlertTriangleIcon;
}
</script>

<template>
  <section class="trace-warning-list" data-testid="prompt-trace-warnings">
    <div v-if="visibleWarnings.length" class="trace-warning-stack">
      <button
        v-for="(item, index) in visibleWarnings"
        :key="`${item.message}-${index}`"
        type="button"
        class="trace-warning"
        :class="severityClass(item.severity)"
        @click="emit('select-target', item)"
      >
        <component :is="iconFor(item.severity)" size="15" />
        <span>{{ item.message }}</span>
      </button>
    </div>
    <div v-else class="trace-warning-empty">暂无明显异常</div>
  </section>
</template>

<style scoped>
.trace-warning-stack {
  display: grid;
  gap: 8px;
}

.trace-warning,
.trace-warning-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 10px 12px;
  background: #ffffff;
  color: #374151;
  font-size: 13px;
  text-align: left;
}

.trace-warning {
  cursor: pointer;
}

.trace-warning:hover {
  filter: brightness(0.98);
}

.trace-warning.is-warning {
  border-color: #f59e0b;
  background: #fffbeb;
  color: #92400e;
}

.trace-warning.is-danger {
  border-color: #ef4444;
  background: #fef2f2;
  color: #991b1b;
}

.trace-warning.is-info {
  border-color: #bfdbfe;
  background: #eff6ff;
  color: #1d4ed8;
}

.trace-warning-empty {
  color: #6b7280;
}
</style>
