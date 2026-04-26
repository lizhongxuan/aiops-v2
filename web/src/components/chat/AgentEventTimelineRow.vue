<script setup>
import { computed } from "vue";

const props = defineProps({
  row: {
    type: Object,
    required: true,
  },
  runtimeStatus: {
    type: String,
    default: "idle",
  },
});

const kind = computed(() => String(props.row?.kind || "").trim());
const status = computed(() => String(props.row?.status || "").trim().toLowerCase());
const isUserTask = computed(() => kind.value === "turn");
const isAssistantFinal = computed(() => kind.value === "assistant_final");
const title = computed(() => String(props.row?.title || props.row?.summary || props.row?.id || "").trim());
const summary = computed(() => String(props.row?.summary || "").trim());
const text = computed(() => String(props.row?.text || props.row?.summary || props.row?.title || "").trim());

const statusLabel = computed(() => {
  if (isUserTask.value) return "";
  if (status.value === "running" || status.value === "queued") return "正在处理";
  if (status.value === "completed") return "已完成";
  if (status.value === "failed") return "失败";
  if (status.value === "blocked") return "等待确认";
  if (status.value === "canceled") return "已停止";
  if (props.runtimeStatus === "working") return "Working";
  return "";
});
</script>

<template>
  <div
    class="agent-event-row"
    :class="[
      `kind-${kind || 'unknown'}`,
      `status-${status || 'unknown'}`,
      { 'is-user-task': isUserTask, 'is-final': isAssistantFinal },
    ]"
    :data-testid="`agent-event-row-${kind || 'unknown'}`"
  >
    <div v-if="isUserTask" class="agent-event-user-task">
      <div class="agent-event-user-pill" data-testid="agent-event-user-task">
        {{ title || "用户任务" }}
      </div>
      <div v-if="summary" class="agent-event-user-status">
        {{ summary }}
      </div>
    </div>

    <div v-else class="agent-event-content">
      <div class="agent-event-meta">
        <span class="agent-event-dot" />
        <span v-if="statusLabel" class="agent-event-status">{{ statusLabel }}</span>
        <span v-if="kind === 'tool'" class="agent-event-kind">工具</span>
        <span v-else-if="kind === 'agent'" class="agent-event-kind">Agent</span>
        <span v-else-if="kind === 'system'" class="agent-event-kind">系统</span>
      </div>

      <p v-if="isAssistantFinal" class="agent-event-final-text">{{ text }}</p>
      <div v-else class="agent-event-copy">
        <strong v-if="title">{{ title }}</strong>
        <span v-if="summary && summary !== title">{{ summary }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.agent-event-row {
  width: min(920px, 100%);
  margin: 0 auto;
  animation: agentEventAppend 0.18s ease-out;
}

.agent-event-row.is-user-task {
  display: flex;
  justify-content: flex-end;
}

.agent-event-user-task {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 5px;
}

.agent-event-user-pill {
  max-width: min(560px, 78%);
  padding: 8px 13px;
  border: 1px solid #e5e7eb;
  border-radius: 999px;
  background: #f7f7f5;
  color: #262626;
  font-size: 14px;
  line-height: 1.5;
}

.agent-event-user-status {
  color: #8a8f98;
  font-size: 12.5px;
  line-height: 1.4;
}

.agent-event-content {
  display: grid;
  gap: 5px;
  padding: 3px 0;
  color: #2f343b;
}

.agent-event-meta {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 18px;
  color: #8a8f98;
  font-size: 12.5px;
}

.agent-event-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #b8bec7;
}

.status-running .agent-event-dot,
.status-queued .agent-event-dot {
  background: #8c939f;
  animation: softPulse 1.4s ease-in-out infinite;
}

.status-completed .agent-event-dot {
  background: #9ca3af;
}

.status-failed .agent-event-dot {
  background: #b4535f;
}

.status-blocked .agent-event-dot {
  background: #b7791f;
}

.agent-event-kind {
  color: #a1a1aa;
}

.agent-event-copy {
  display: grid;
  gap: 2px;
  padding-left: 13px;
  color: #4b5563;
  font-size: 14px;
  line-height: 1.55;
}

.agent-event-copy strong {
  color: #2f343b;
  font-weight: 500;
}

.agent-event-final-text {
  margin: 0;
  padding-left: 13px;
  color: #171717;
  font-size: 15px;
  line-height: 1.75;
  white-space: pre-wrap;
}

@keyframes softPulse {
  0%, 100% {
    opacity: 0.45;
  }
  50% {
    opacity: 1;
  }
}

@keyframes agentEventAppend {
  from {
    opacity: 0;
    transform: translateY(3px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
