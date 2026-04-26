<script setup>
import { computed } from "vue";

const props = defineProps({
  agents: {
    type: Array,
    default: () => [],
  },
});

function agentHandle(agent = {}) {
  const handle = String(agent.handle || agent.name || "").trim();
  if (!handle) return "@agent";
  return handle.startsWith("@") ? handle : `@${handle}`;
}

function statusLabel(status = "") {
  const normalized = String(status || "").toLowerCase();
  if (normalized === "running" || normalized === "queued") return "运行中";
  if (normalized === "completed") return "已完成";
  if (normalized === "failed") return "失败";
  if (normalized === "blocked" || normalized === "waiting") return "等待";
  if (normalized === "canceled") return "已停止";
  return "空闲";
}

const visibleAgents = computed(() =>
  (props.agents || []).map((agent) => ({
    ...agent,
    handleLabel: agentHandle(agent),
    statusLabel: statusLabel(agent.status),
  })),
);
</script>

<template>
  <aside v-if="visibleAgents.length" class="agent-status-rail" data-testid="agent-status-rail">
    <div class="agent-status-rail-title">Agents</div>
    <div class="agent-status-list">
      <div v-for="agent in visibleAgents" :key="agent.id || agent.handleLabel" class="agent-status-row">
        <div class="agent-status-main">
          <span class="agent-handle">{{ agent.handleLabel }}</span>
          <span class="agent-status">{{ agent.statusLabel }}</span>
        </div>
        <div v-if="agent.lastAction || agent.lastSummary" class="agent-action">
          {{ agent.lastAction || agent.lastSummary }}
        </div>
        <div v-if="agent.stats" class="agent-stats">
          <span v-if="agent.stats.commandsRun">{{ agent.stats.commandsRun }} cmd</span>
          <span v-if="agent.stats.filesRead">{{ agent.stats.filesRead }} read</span>
          <span v-if="agent.stats.filesChanged">{{ agent.stats.filesChanged }} changed</span>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.agent-status-rail {
  width: min(920px, 100%);
  margin: 0 auto 4px;
  padding: 10px 12px;
  border: 1px solid #e7e5e4;
  border-radius: 14px;
  background: #fbfaf8;
}

.agent-status-rail-title {
  margin-bottom: 8px;
  color: #8a8f98;
  font-size: 12px;
  font-weight: 500;
}

.agent-status-list {
  display: grid;
  gap: 8px;
}

.agent-status-row {
  display: grid;
  gap: 3px;
}

.agent-status-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.agent-handle {
  color: #262626;
  font-size: 13.5px;
  font-weight: 600;
}

.agent-status,
.agent-action,
.agent-stats {
  color: #8a8f98;
  font-size: 12.5px;
  line-height: 1.45;
}

.agent-stats {
  display: flex;
  gap: 8px;
}
</style>
