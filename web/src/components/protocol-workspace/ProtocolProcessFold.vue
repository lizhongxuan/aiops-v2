<script setup>
import { computed, ref, watch } from "vue";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-vue-next";

const props = defineProps({
  turn: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["item-select"]);

function isFailedStatus(status) {
  return ["failed", "error", "rejected"].includes(String(status || "").trim().toLowerCase());
}

function requiresAttention(turn = {}) {
  const phase = String(turn?.phase || "").trim().toLowerCase();
  if (["failed", "aborted", "waiting_approval", "waiting_input"].includes(phase)) return true;
  return (turn?.processItems || []).some((item) => isFailedStatus(item?.status));
}

function defaultExpanded(turn = {}) {
  if (turn?.active) return true;
  if (requiresAttention(turn)) return true;
  return !turn?.collapsedByDefault;
}

const expanded = ref(defaultExpanded(props.turn));

watch(
  () => [props.turn?.id, props.turn?.collapsedByDefault, props.turn?.phase, props.turn?.active],
  () => {
    expanded.value = defaultExpanded(props.turn);
  },
);

const hasItems = computed(() => Array.isArray(props.turn?.processItems) && props.turn.processItems.length > 0);
const hasBody = computed(() => hasItems.value || !!props.turn?.liveHint);

function toggleExpanded() {
  if (!hasBody.value) return;
  expanded.value = !expanded.value;
}

function itemMeta(item = {}) {
  return [item.hostId, item.time].filter(Boolean).join(" · ");
}

function selectItem(item) {
  emit("item-select", item);
}
</script>

<template>
  <section
    v-if="turn?.processItems?.length || turn?.liveHint || turn?.summary"
    class="protocol-process-fold"
    :data-testid="`protocol-process-fold-${turn.id}`"
  >
    <div class="protocol-process-divider">
      <span class="divider-line" />
      <button
        class="protocol-process-toggle"
        type="button"
        :aria-expanded="expanded"
        :disabled="!hasBody"
        @click="toggleExpanded"
      >
        <span class="toggle-label">{{ turn.processLabel || "已处理" }}</span>
        <span v-if="turn.summary" class="toggle-summary">{{ turn.summary }}</span>
        <component :is="expanded ? ChevronUpIcon : ChevronDownIcon" v-if="hasBody" size="14" class="toggle-icon" />
      </button>
      <span class="divider-line" />
    </div>

    <div v-if="expanded && hasBody" class="protocol-process-body">
      <div v-if="turn.liveHint" class="protocol-process-live">
        {{ turn.liveHint }}
      </div>

      <ul v-if="hasItems" class="protocol-process-list">
        <li
          v-for="item in turn.processItems"
          :key="item.id"
          class="protocol-process-item"
          :class="`tone-${item.tone || 'neutral'}`"
        >
          <button
            type="button"
            class="protocol-process-item-button"
            :data-testid="`protocol-process-item-${item.id}`"
            @click="selectItem(item)"
          >
            <div class="protocol-process-item-text">{{ item.text }}</div>
            <div class="protocol-process-item-footer">
              <div v-if="itemMeta(item)" class="protocol-process-item-meta">
                {{ itemMeta(item) }}
              </div>
              <div class="protocol-process-item-link">查看详情</div>
            </div>
          </button>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.protocol-process-fold {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 4px 0 7px;
}

.protocol-process-divider {
  display: flex;
  align-items: center;
  gap: 10px;
}

.divider-line {
  flex: 1;
  height: 1px;
  background: rgba(226, 232, 240, 0.82);
}

.protocol-process-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  padding: 4px 11px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  border-radius: 999px;
  background: rgba(248, 250, 252, 0.96);
  color: #64748b;
  font-size: 11px;
  line-height: 1.4;
  cursor: pointer;
  transition: background 0.18s ease, border-color 0.18s ease;
}

.protocol-process-toggle:disabled {
  cursor: default;
}

.protocol-process-toggle:hover:not(:disabled) {
  background: rgba(241, 245, 249, 0.98);
  border-color: rgba(203, 213, 225, 0.96);
}

.toggle-label {
  color: #475569;
  font-weight: 700;
  white-space: nowrap;
}

.toggle-summary {
  color: #64748b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.toggle-icon {
  color: #94a3b8;
  flex-shrink: 0;
}

.protocol-process-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.74);
}

.protocol-process-live {
  color: #64748b;
  font-size: 12px;
  line-height: 1.48;
}

.protocol-process-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.protocol-process-item {
  padding-left: 10px;
  border-left: 2px solid rgba(203, 213, 225, 0.92);
}

.protocol-process-item.tone-warning {
  border-left-color: #f59e0b;
}

.protocol-process-item.tone-danger {
  border-left-color: #ef4444;
}

.protocol-process-item.tone-success {
  border-left-color: #22c55e;
}

.protocol-process-item-button {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.protocol-process-item-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.protocol-process-item-text {
  color: #0f172a;
  font-size: 12.25px;
  line-height: 1.48;
  white-space: pre-wrap;
}

.protocol-process-item-meta {
  color: #94a3b8;
  font-size: 11px;
  line-height: 1.4;
}

.protocol-process-item-link {
  color: #2563eb;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
  white-space: nowrap;
}
</style>
