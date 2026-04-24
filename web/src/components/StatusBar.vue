<script setup>
import { computed } from "vue";
import { SettingsIcon } from "lucide-vue-next";

const props = defineProps({
  wsStatus: { type: String, default: "disconnected" },
  selectedHost: { type: Object, default: () => ({}) },
  selectedHostLabel: { type: String, default: "server-local" },
  turnPhase: { type: String, default: "idle" },
  turnActive: { type: Boolean, default: false },
  describeTurnPhase: { type: Function, default: () => "待命" },
});

const emit = defineEmits(["open-settings"]);

const connectionLabel = computed(() => {
  switch (props.wsStatus) {
    case "connected": return "AI 已连接";
    case "connecting": return "AI 连接中...";
    case "error": return "AI 连接断开";
    default: return "AI 未连接";
  }
});

const connectionType = computed(() => {
  switch (props.wsStatus) {
    case "connected": return "success";
    case "connecting": return "warning";
    case "error": return "error";
    default: return "error";
  }
});

const hostStatusType = computed(() => {
  return props.selectedHost?.status === "online" ? "success" : "default";
});

const turnPhaseLabel = computed(() => {
  if (props.turnActive) return props.describeTurnPhase(props.turnPhase);
  return props.describeTurnPhase(props.turnPhase);
});
</script>

<template>
  <div class="status-bar">
    <div class="status-left">
      <n-badge dot :type="connectionType" :offset="[0, 0]">
        <span class="status-dot-anchor" :class="wsStatus"></span>
      </n-badge>
      <span class="status-text">{{ connectionLabel }}</span>
      <n-divider vertical />
      <n-badge dot :type="hostStatusType" :offset="[0, 0]">
        <span class="status-dot-anchor"></span>
      </n-badge>
      <span class="status-text">{{ selectedHostLabel }}</span>
    </div>
    <div class="status-right">
      <span class="status-text phase-text">{{ turnPhaseLabel }}</span>
      <n-button quaternary circle size="tiny" @click="emit('open-settings')" title="设置">
        <template #icon><SettingsIcon size="14" /></template>
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.status-bar {
  height: 48px;
  min-height: 48px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-top: 1px solid var(--border-color, #e2e8f0);
  background: var(--canvas-bg, #ffffff);
  font-size: 12px;
  color: var(--text-subtle, #64748b);
}

.status-left,
.status-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-text {
  white-space: nowrap;
}

.phase-text {
  color: var(--text-meta, #9ca3af);
}

.status-dot-anchor {
  display: inline-block;
  width: 8px;
  height: 8px;
}

.status-dot-anchor.connected {
  animation: pulse-green 2s ease-in-out infinite;
}

@keyframes pulse-green {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
