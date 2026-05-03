<script setup>
import { computed } from "vue";
import { CopyIcon } from "lucide-vue-next";
import { formatCount } from "../../utils/promptTraceViewModel";
import PromptTraceWarnings from "./PromptTraceWarnings.vue";

const props = defineProps({
  viewModel: { type: Object, default: null },
});

const emit = defineEmits(["copy", "select-target"]);

const summary = computed(() => props.viewModel?.summary || {});
const fingerprints = computed(() => props.viewModel?.fingerprints || []);
const tools = computed(() => props.viewModel?.tools || { visible: [], risky: [] });
const roleRows = computed(() => Object.entries(summary.value.roleCounts || {}));
const layerRows = computed(() => Object.entries(summary.value.layerCounts || {}));

function copyText(label, value) {
  emit("copy", { label, value });
}
</script>

<template>
  <div class="trace-overview" data-testid="prompt-trace-overview">
    <div class="trace-card-grid">
      <section class="trace-card">
        <div class="trace-card-label">本次 LLM 输入</div>
        <div class="trace-stat-row">
          <span>Messages</span>
          <strong>{{ formatCount(summary.messageCount) }}</strong>
        </div>
        <div class="trace-stat-row">
          <span>Visible tools</span>
          <strong>{{ formatCount(summary.visibleToolCount) }}</strong>
        </div>
        <div class="trace-stat-row">
          <span>Prompt size</span>
          <strong>{{ formatCount(summary.promptCharCount) }} chars</strong>
        </div>
        <div class="trace-stat-row">
          <span>User message</span>
          <strong>{{ summary.hasUserMessage ? "yes" : "missing" }}</strong>
        </div>
      </section>

      <section class="trace-card">
        <div class="trace-card-label">Trace</div>
        <div v-if="summary.caseId" class="trace-meta-line" :title="summary.caseId">Case {{ summary.caseId }}</div>
        <div class="trace-meta-line" :title="summary.sessionId">Session {{ summary.sessionId || "-" }}</div>
        <div class="trace-meta-line" :title="summary.turnId">Turn {{ summary.turnId || "-" }}</div>
        <div class="trace-meta-grid">
          <span>iteration {{ summary.iteration ?? "-" }}</span>
          <span>{{ summary.createdAt || "-" }}</span>
        </div>
      </section>

      <section class="trace-card">
        <div class="trace-card-label">Provider roles</div>
        <div v-if="roleRows.length" class="trace-chip-list">
          <span v-for="[key, value] in roleRows" :key="key" class="trace-chip">{{ key }} {{ value }}</span>
        </div>
        <div v-else class="trace-muted">暂无 role</div>
      </section>

      <section class="trace-card">
        <div class="trace-card-label">Prompt layers</div>
        <div v-if="layerRows.length" class="trace-chip-list">
          <span v-for="[key, value] in layerRows" :key="key" class="trace-chip">{{ key }} {{ value }}</span>
        </div>
        <div v-else class="trace-muted">暂无 layer</div>
      </section>
    </div>

    <section class="trace-section">
      <div class="trace-section-title">Prompt Fingerprint</div>
      <div class="trace-fingerprint-grid">
        <button
          v-for="item in fingerprints"
          :key="item.key"
          type="button"
          class="trace-fingerprint"
          :class="{ 'is-missing': item.missing }"
          :disabled="!item.value"
          @click="copyText(item.label, item.value)"
        >
          <span>{{ item.label }}</span>
          <code>{{ item.shortValue || "missing" }}</code>
          <CopyIcon v-if="item.value" size="13" />
        </button>
      </div>
    </section>

    <section class="trace-section">
      <div class="trace-section-title">Visible Tools</div>
      <div v-if="tools.visible.length" class="trace-chip-list">
        <span
          v-for="tool in tools.visible"
          :key="tool"
          class="trace-chip"
          :class="{ 'is-risky': tools.risky.includes(tool) }"
        >
          {{ tool }}
        </span>
      </div>
      <div v-else class="trace-muted">本轮没有 visible tools</div>
    </section>

    <section class="trace-section">
      <div class="trace-section-title">调试提示</div>
      <PromptTraceWarnings :warnings="viewModel?.warnings || []" @select-target="emit('select-target', $event)" />
    </section>
  </div>
</template>

<style scoped>
.trace-overview {
  display: grid;
  gap: 14px;
}

.trace-card-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.trace-card,
.trace-section {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 14px;
}

.trace-card-label,
.trace-section-title {
  margin-bottom: 10px;
  color: #6b7280;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.trace-stat-row,
.trace-meta-grid {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  color: #4b5563;
  font-size: 13px;
  line-height: 1.8;
}

.trace-stat-row strong {
  color: #111827;
}

.trace-meta-line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #111827;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.8;
}

.trace-meta-grid {
  margin-top: 8px;
  flex-wrap: wrap;
}

.trace-chip-list,
.trace-fingerprint-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.trace-chip {
  max-width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 999px;
  padding: 3px 8px;
  color: #374151;
  background: #f9fafb;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.trace-chip.is-risky {
  border-color: #f59e0b;
  background: #fffbeb;
  color: #92400e;
}

.trace-fingerprint {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  max-width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  background: #ffffff;
  padding: 7px 9px;
  cursor: pointer;
}

.trace-fingerprint code {
  color: #111827;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.trace-fingerprint.is-missing {
  cursor: default;
  opacity: 0.6;
}

.trace-muted {
  color: #6b7280;
  font-size: 13px;
}

@media (max-width: 1200px) {
  .trace-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .trace-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
