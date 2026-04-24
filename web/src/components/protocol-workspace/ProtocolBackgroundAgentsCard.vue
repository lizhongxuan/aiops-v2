<script setup>
import { computed, ref } from "vue";
import { BotIcon, ChevronDownIcon, ChevronUpIcon, ExternalLinkIcon } from "lucide-vue-next";
import { NTag } from "naive-ui";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  title: {
    type: String,
    default: "后台 Agent",
  },
  subtitle: {
    type: String,
    default: "点击查看对应 agent 详情",
  },
  agents: {
    type: Array,
    default: () => [],
  },
  emptyLabel: {
    type: String,
    default: "暂无后台 worker。",
  },
  docked: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["select"]);
const expanded = ref(true);
const expandedAgentIds = ref(new Set());

const normalizedAgents = computed(() =>
  (Array.isArray(props.agents) ? props.agents : [])
    .map((agent, index) => {
      if (typeof agent === "string") {
        return {
          id: `agent-${index}`,
          title: agent,
          status: "idle",
          statusLabel: "idle",
          detail: "",
          meta: "",
          messages: [],
        };
      }
      return {
        id: String(agent?.id || agent?.agentId || agent?.hostId || agent?.name || index),
        title: String(agent?.title || agent?.label || agent?.name || agent?.hostName || agent?.hostId || agent?.id || "agent"),
        status: String(agent?.status || agent?.tone || "idle").toLowerCase(),
        statusLabel: String(agent?.statusLabel || agent?.status || "idle"),
        detail: String(agent?.detail || agent?.summary || agent?.text || ""),
        meta: String(agent?.meta || agent?.subtitle || agent?.route || ""),
        type: String(agent?.type || agent?.agentType || "worker"),
        messages: Array.isArray(agent?.messages) ? agent.messages.slice(-5) : [],
      };
    })
    .filter((agent) => agent.title),
);

function toneClass(status) {
  const value = String(status || "").toLowerCase();
  if (!value) return "neutral";
  if (value.includes("run") || value.includes("active") || value.includes("success")) return "success";
  if (value.includes("wait") || value.includes("block") || value.includes("warn")) return "warning";
  if (value.includes("fail") || value.includes("error") || value.includes("off")) return "danger";
  return "neutral";
}

function tagType(status) {
  const value = String(status || "").toLowerCase();
  if (value.includes("run") || value.includes("active") || value.includes("success")) return "success";
  if (value.includes("wait") || value.includes("block") || value.includes("warn")) return "warning";
  if (value.includes("fail") || value.includes("error")) return "error";
  return "default";
}

function selectAgent(agent) {
  emit("select", agent);
}

function toggleExpanded() {
  expanded.value = !expanded.value;
}

function toggleAgentExpand(agentId) {
  const next = new Set(expandedAgentIds.value);
  if (next.has(agentId)) {
    next.delete(agentId);
  } else {
    next.add(agentId);
  }
  expandedAgentIds.value = next;
}

function isAgentExpanded(agentId) {
  return expandedAgentIds.value.has(agentId);
}
</script>

<template>
  <section class="protocol-background-agents-card" :class="{ docked }">
    <button class="background-summary" type="button" @click="toggleExpanded">
      <div class="background-summary-copy">
        <div class="background-summary-title">
          <BotIcon size="13" />
          <span>{{ normalizedAgents.length }} 个后台 Agent</span>
        </div>
      </div>
      <component :is="expanded ? ChevronUpIcon : ChevronDownIcon" size="14" />
    </button>

    <div v-if="expanded && normalizedAgents.length" class="background-list">
      <div
        v-for="agent in normalizedAgents"
        :key="agent.id"
        class="background-agent-wrapper"
      >
        <div class="background-agent-row">
          <button
            type="button"
            class="background-agent"
            @click="toggleAgentExpand(agent.id); selectAgent(agent)"
          >
            <div class="background-agent-inline">
              <strong :class="`tone-${toneClass(agent.status)}`">{{ agent.title }}</strong>
              <n-tag :type="tagType(agent.status)" size="small" round>{{ agent.statusLabel }}</n-tag>
              <span class="agent-type-label">{{ agent.type }}</span>
            </div>
            <span class="background-toggle-label">
              {{ isAgentExpanded(agent.id) ? '收起' : '展开详情' }}
            </span>
          </button>
          <button type="button" class="background-open-link" @click.stop="selectAgent(agent)" title="在新页面打开">
            <ExternalLinkIcon size="13" />
          </button>
        </div>

        <div v-if="isAgentExpanded(agent.id)" class="agent-nested-thread">
          <div v-if="agent.detail" class="agent-detail-text">{{ agent.detail }}</div>
          <div v-if="agent.messages.length" class="agent-messages">
            <div v-for="(msg, idx) in agent.messages" :key="idx" class="agent-message-item">
              <span class="agent-msg-role">{{ msg.role || 'assistant' }}</span>
              <span class="agent-msg-text">{{ (msg.text || msg.summary || '').slice(0, 120) }}{{ (msg.text || msg.summary || '').length > 120 ? '...' : '' }}</span>
            </div>
          </div>
          <div v-else-if="!agent.detail" class="agent-empty-hint">暂无消息记录</div>
        </div>
      </div>
    </div>

    <div v-else-if="expanded" class="background-empty">
      <p>{{ emptyLabel }}</p>
    </div>
  </section>
</template>

<style scoped>
.protocol-background-agents-card {
  margin-left: 36px;
  max-width: 820px;
  border-radius: 10px;
  border: 1px solid rgba(226, 232, 240, 0.96);
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.02);
  overflow: hidden;
}

.protocol-background-agents-card.docked {
  margin-left: 0;
  max-width: 100%;
  border-radius: 10px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.02);
}

.background-summary {
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
  padding: 6px 12px;
  border: 0;
  background: transparent;
  cursor: pointer;
  font: inherit;
  color: inherit;
}

.background-summary-copy {
  min-width: 0;
  text-align: left;
}

.background-summary-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #6b7280;
  font-size: 11px;
  font-weight: 600;
}

.background-list {
  display: flex;
  flex-direction: column;
  border-top: 1px solid rgba(241, 245, 249, 0.95);
}

.background-agent-wrapper + .background-agent-wrapper {
  border-top: 1px solid rgba(243, 244, 246, 0.95);
}

.background-agent-row {
  display: flex;
  align-items: center;
  gap: 4px;
}

.background-agent {
  flex: 1;
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
  padding: 6px 12px;
  border: 0;
  background: transparent;
  cursor: pointer;
  font: inherit;
  min-width: 0;
}

.background-agent-inline {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  text-align: left;
  overflow: hidden;
}

.background-agent-inline strong {
  font-size: 12px;
  line-height: 1.4;
  font-weight: 600;
  white-space: nowrap;
}

.agent-type-label {
  color: #94a3b8;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.background-toggle-label {
  flex: none;
  color: #2563eb;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.background-open-link {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  background: transparent;
  cursor: pointer;
  color: #9ca3af;
  border-radius: 6px;
  transition: background 0.15s, color 0.15s;
  margin-right: 4px;
}

.background-open-link:hover {
  background: #f1f5f9;
  color: #475569;
}

/* Task 8.2: Visual indent for nested thread */
.agent-nested-thread {
  border-left: 2px solid #e2e8f0;
  padding-left: 16px;
  margin: 0 12px 8px 24px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.agent-detail-text {
  color: #475569;
  font-size: 12px;
  line-height: 1.5;
}

.agent-messages {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.agent-message-item {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  font-size: 11.5px;
  line-height: 1.45;
}

.agent-msg-role {
  flex: none;
  padding: 1px 5px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #64748b;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
}

.agent-msg-text {
  color: #334155;
  word-break: break-word;
}

.agent-empty-hint {
  color: #94a3b8;
  font-size: 11px;
}

.background-empty {
  border-top: 1px solid rgba(241, 245, 249, 0.95);
  padding: 6px 12px;
  color: #6b7280;
  font-size: 11px;
}

.tone-success {
  color: #059669;
}

.tone-warning {
  color: #d97706;
}

.tone-danger {
  color: #dc2626;
}

.tone-neutral {
  color: #0f172a;
}

@media (max-width: 900px) {
  .protocol-background-agents-card {
    margin-left: 0;
    max-width: 100%;
  }
}
</style>
