<script setup>
import { computed } from "vue";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const durationText = computed(() => {
  const ms = props.card.durationMs || 0;
  if (ms <= 0) return "已处理";
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts = [];
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds}s`);
  return `已处理 ${parts.join(" ")}`;
});
</script>

<template>
  <div class="task-divider-card">
    <span class="task-divider-line">——{{ durationText }}——</span>
  </div>
</template>

<style scoped>
.task-divider-card {
  display: flex;
  justify-content: center;
  padding: 8px 0;
  margin-left: 48px;
  color: #94a3b8;
  font-size: 12px;
}

.task-divider-line {
  letter-spacing: 0.02em;
  white-space: nowrap;
}
</style>
